package engine

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// Engine is the Cloud Fetch torrent engine, backed by anacrolix/torrent
type Engine struct {
	mut    sync.Mutex
	client *torrent.Client
	config Config
	store  *store
	ts     map[string]*Torrent
}

func New() *Engine {
	return &Engine{ts: map[string]*Torrent{}}
}

func (e *Engine) Config() Config {
	return e.config
}

func (e *Engine) Configure(c Config) error {
	// validate before touching the running client: a rejected config must
	// leave the current torrents running
	if c.IncomingPort <= 0 || c.IncomingPort > 65535 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}
	e.mut.Lock()
	old, prev := e.client, e.config
	e.mut.Unlock()
	if old != nil {
		old.Close()
		time.Sleep(1 * time.Second)
	}
	client, err := newClient(c)
	if err != nil {
		if old != nil {
			// e.g. the new port is taken: bring the previous setup back
			if back, berr := newClient(prev); berr == nil {
				e.install(prev, back)
			} else {
				log.Printf("reconfigure failed (%s) and restoring the previous config failed too: %s", err, berr)
			}
		}
		return err
	}
	e.install(c, client)
	return nil
}

func newClient(c Config) (*torrent.Client, error) {
	config := torrent.NewDefaultClientConfig()
	config.DataDir = c.DownloadDirectory
	config.NoUpload = !c.EnableUpload
	config.Seed = c.EnableSeeding
	config.ListenPort = c.IncomingPort
	return torrent.NewClient(config)
}

func (e *Engine) install(c Config, client *torrent.Client) {
	e.mut.Lock()
	defer e.mut.Unlock()
	e.config = c
	e.client = client
	// the old client's torrents are closed; the store is the source of truth
	e.ts = map[string]*Torrent{}
	e.store = newStore(filepath.Join(c.DownloadDirectory, storeDirName))
	e.restoreLocked()
}

func (e *Engine) restoreLocked() {
	saved, errs := e.store.load()
	for _, err := range errs {
		log.Printf("restore: skipping %s", err)
	}
	for _, st := range saved {
		var tt *torrent.Torrent
		var err error
		if st.Metainfo != nil {
			tt, err = e.client.AddTorrent(st.Metainfo)
		} else {
			tt, err = e.client.AddMagnet(st.Magnet)
		}
		if err != nil {
			log.Printf("restore: %s: %s", st.InfoHash, err)
			continue
		}
		e.trackLocked(tt, st.Magnet, st.Started, false)
	}
}

func (e *Engine) NewMagnet(magnetURI string) error {
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return e.newTorrent(tt, magnetURI)
}

func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	return e.newTorrent(tt, "")
}

func (e *Engine) newTorrent(tt *torrent.Torrent, magnet string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t := e.trackLocked(tt, magnet, true, true)
	e.saveLocked(t)
	return nil
}

// trackLocked registers tt and applies its started state. Metadata may still
// be unknown (magnets), so the .torrent save and DownloadAll wait for GotInfo.
// verify is set for torrents the person just added: the file storage treats
// a same-size file at the final name as complete without hashing it, so
// existing data is checked once here. Restores skip it; the completion
// database already holds what passed.
func (e *Engine) trackLocked(tt *torrent.Torrent, magnet string, started, verify bool) *Torrent {
	t := e.upsertTorrent(tt)
	t.magnet = magnet
	t.Started = started
	if !started {
		tt.DisallowDataDownload()
		tt.DisallowDataUpload()
	}
	st := e.store
	if tt.Info() != nil {
		// synchronous so a .torrent add is on disk before the API call returns
		saveMetainfo(st, tt)
		if verify {
			verifyInBackground(tt)
		}
		if started {
			tt.DownloadAll()
		}
		return t
	}
	go func() {
		select {
		case <-tt.GotInfo():
		case <-tt.Closed():
			return
		}
		saveMetainfo(st, tt)
		if verify {
			verifyInBackground(tt)
		}
		e.mut.Lock()
		defer e.mut.Unlock()
		if t.Started && !t.Dropped {
			tt.DownloadAll()
		}
	}()
	return t
}

// verifyInBackground hashes every piece against the metainfo; failed pieces
// become incomplete and download normally. Stops if the torrent is dropped.
func verifyInBackground(tt *torrent.Torrent) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-tt.Closed():
			cancel()
		case <-ctx.Done():
		}
	}()
	go func() {
		defer cancel()
		if err := tt.VerifyDataContext(ctx); err != nil && ctx.Err() == nil {
			log.Printf("verify %s: %s", tt.InfoHash().HexString(), err)
		}
	}()
}

func saveMetainfo(st *store, tt *torrent.Torrent) {
	ih := tt.InfoHash().HexString()
	mi := tt.Metainfo()
	if err := st.saveMetainfo(ih, &mi); err != nil {
		log.Printf("save %s: %s", ih, err)
	}
}

func (e *Engine) saveLocked(t *Torrent) {
	if err := e.store.save(record{InfoHash: t.InfoHash, Magnet: t.magnet, Started: t.Started}); err != nil {
		log.Printf("save %s: %s", t.InfoHash, err)
	}
}

// GetTorrents refreshes the local cache from anacrolix/torrent and returns a
// copy of it. The copy matters: callers hand it to velox, which marshals it
// on other goroutines while the engine keeps mutating its own map and structs.
func (e *Engine) GetTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()

	if e.client == nil {
		return nil
	}
	for _, tt := range e.client.Torrents() {
		e.upsertTorrent(tt)
	}
	out := make(map[string]*Torrent, len(e.ts))
	for ih, t := range e.ts {
		c := *t
		c.Files = make([]*File, len(t.Files))
		for i, f := range t.Files {
			if f != nil {
				fc := *f
				c.Files[i] = &fc
			}
		}
		out[ih] = &c
	}
	return out
}

func (e *Engine) upsertTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	torrent, ok := e.ts[ih]
	if !ok {
		torrent = &Torrent{InfoHash: ih}
		e.ts[ih] = torrent
	}
	//update torrent fields using underlying torrent
	torrent.Update(tt)
	return torrent
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
	ih, err := str2ih(infohash)
	if err != nil {
		return nil, err
	}
	t, ok := e.ts[ih.HexString()]
	if !ok {
		return t, fmt.Errorf("Missing torrent %x", ih)
	}
	return t, nil
}

func (e *Engine) getOpenTorrent(infohash string) (*Torrent, error) {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (e *Engine) StartTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return err
	}
	if t.Started {
		return fmt.Errorf("Already started")
	}
	t.Started = true
	for _, f := range t.Files {
		if f != nil {
			f.Started = true
		}
	}
	t.t.AllowDataDownload()
	t.t.AllowDataUpload()
	if t.t.Info() != nil {
		t.t.DownloadAll()
	}
	e.saveLocked(t)
	return nil
}

// StopTorrent pauses rather than drops, so the torrent can be started again.
func (e *Engine) StopTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if !t.Started {
		return fmt.Errorf("Already stopped")
	}
	t.t.DisallowDataDownload()
	t.t.DisallowDataUpload()
	t.Started = false
	for _, f := range t.Files {
		if f != nil {
			f.Started = false
		}
	}
	e.saveLocked(t)
	return nil
}

func (e *Engine) DeleteTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if err := e.store.remove(t.InfoHash); err != nil {
		log.Printf("delete %s: %s", t.InfoHash, err)
	}
	t.Dropped = true
	delete(e.ts, t.InfoHash)
	t.t.Drop()
	return nil
}

func (e *Engine) StartFile(infohash, filepath string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return err
	}
	var f *File
	for _, file := range t.Files {
		if file.Path == filepath {
			f = file
			break
		}
	}
	if f == nil {
		return fmt.Errorf("Missing file %s", filepath)
	}
	if f.Started {
		return fmt.Errorf("Already started")
	}
	t.Started = true
	f.Started = true
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	return fmt.Errorf("Unsupported")
}

func str2ih(str string) (metainfo.Hash, error) {
	var ih metainfo.Hash
	e, err := hex.Decode(ih[:], []byte(str))
	if err != nil {
		return ih, fmt.Errorf("Invalid hex string")
	}
	if e != 20 {
		return ih, fmt.Errorf("Invalid length")
	}
	return ih, nil
}
