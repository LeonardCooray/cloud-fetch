package engine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
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
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.config
}

var errNotRunning = errors.New("torrent client not running")

// restoreAttempts and restoreBackoff cover the previous port being taken
// briefly by something else while the old client was closed.
const restoreAttempts = 5

var restoreBackoff = 200 * time.Millisecond

func (e *Engine) Configure(c Config) error {
	// validate before touching the running client: a rejected config must
	// leave the current torrents running
	if c.IncomingPort <= 0 || c.IncomingPort > 65535 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}
	e.mut.Lock()
	hadClient, prev := e.client != nil, e.config
	e.closeClientLocked()
	e.mut.Unlock()
	if hadClient {
		time.Sleep(1 * time.Second)
	}
	client, err := newClient(c)
	if err == nil {
		e.install(c, client)
		return nil
	}
	if !hadClient {
		return err
	}
	// e.g. the new port is taken: bring the previous setup back
	var berr error
	for i := 0; i < restoreAttempts; i++ {
		if i > 0 {
			time.Sleep(restoreBackoff)
		}
		var back *torrent.Client
		if back, berr = newClient(prev); berr == nil {
			e.install(prev, back)
			return err
		}
	}
	log.Printf("reconfigure failed (%s) and restoring the previous config failed too: %s", err, berr)
	return err
}

// Close stops the torrent client. The engine has none until Configure
// succeeds again.
func (e *Engine) Close() {
	e.mut.Lock()
	defer e.mut.Unlock()
	e.closeClientLocked()
}

// closeClientLocked stops verifications before closing: anacrolix/torrent
// leaks the client lock if a piece check starts on a closed torrent
// (anacrolix #1119, unfixed as of v1.61.0 and master d913b30f520e).
// Afterwards the engine has no client until install gives it one.
func (e *Engine) closeClientLocked() {
	if e.client == nil {
		return
	}
	for _, t := range e.ts {
		t.stopVerify()
	}
	e.client.Close()
	e.client = nil
	e.ts = map[string]*Torrent{}
}

func clientConfig(c Config) *torrent.ClientConfig {
	config := torrent.NewDefaultClientConfig()
	config.DataDir = c.DownloadDirectory
	config.NoUpload = !c.EnableUpload
	config.Seed = c.EnableSeeding
	config.ListenPort = c.IncomingPort
	// anacrolix/torrent's connection writer arms its wake-up after checking
	// for work, so data queued in between waits for this timer (anacrolix
	// #1070, fixed on master by #1078 but in no release up to v1.61.0). The
	// 1 minute default stalled downloads; 5s bounds it at the cost of a
	// 4-byte keepalive per idle connection every 5s. Drop this once a release
	// with #1078 is in go.mod.
	config.KeepAliveTimeout = 5 * time.Second
	if c.DisableEncryption {
		config.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{Preferred: false, RequirePreferred: true}
	}
	return config
}

// newClient is a variable so tests can make client starts fail.
var newClient = func(c Config) (*torrent.Client, error) {
	return torrent.NewClient(clientConfig(c))
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
		// seeded before tracking so the first priority pass already skips them
		e.upsertTorrent(tt).stopped = toSet(st.StoppedFiles)
		e.trackLocked(tt, st.Magnet, st.Started, false)
	}
}

func (e *Engine) NewMagnet(magnetURI string) error {
	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		return err
	}
	// anacrolix/torrent v1.59 files every v2-only torrent under the zero v1
	// hash, so a second one would be mistaken for the first
	if spec.InfoHash.IsZero() {
		return fmt.Errorf("v2-only magnets (no urn:btih hash) aren't supported yet")
	}
	return e.addSpec(spec, magnetURI)
}

func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	return e.addSpec(spec, "")
}

// addSpec also handles re-adding a torrent the client already has:
// anacrolix hands back the existing one. A re-add never pauses it, and with
// AutoStart on it resumes a paused one.
func (e *Engine) addSpec(spec *torrent.TorrentSpec, magnet string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return errNotRunning
	}
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	existing, known := e.ts[tt.InfoHash().HexString()]
	started := e.config.AutoStart || (known && existing.Started)
	t := e.trackLocked(tt, magnet, started, !known)
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
	if magnet != "" {
		// a later .torrent add of the same torrent must not erase it
		t.magnet = magnet
	}
	t.Started = started
	if started {
		tt.AllowDataDownload()
		tt.AllowDataUpload()
	} else {
		tt.DisallowDataDownload()
		tt.DisallowDataUpload()
	}
	st := e.store
	if tt.Info() != nil {
		// synchronous so a .torrent add is on disk before the API call returns
		saveMetainfo(st, tt)
		if verify {
			t.startVerify()
		}
		applyFileSelection(t)
		return t
	}
	go func() {
		select {
		case <-tt.GotInfo():
		case <-tt.Closed():
			return
		}
		saveMetainfo(st, tt)
		e.mut.Lock()
		defer e.mut.Unlock()
		// torrents are only closed under e.mut, so this check holds while
		// the verification starts
		select {
		case <-tt.Closed():
			return
		default:
		}
		if verify {
			t.startVerify()
		}
		applyFileSelection(t)
	}()
	return t
}

// applyFileSelection sets each file's priority from its selection. Paused
// torrents get priorities too; DisallowDataDownload is what keeps them idle.
// A piece shared by a selected and an unselected file is still downloaded.
func applyFileSelection(t *Torrent) {
	for _, f := range t.t.Files() {
		if t.stopped[f.Path()] {
			f.SetPriority(torrent.PiecePriorityNone)
		} else {
			f.Download()
		}
	}
}

func toSet(paths []string) map[string]bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return set
}

type verification struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// startVerify hashes every piece against the metainfo in the background;
// failed pieces become incomplete and download normally. Call it with e.mut
// held, on an open torrent.
func (t *Torrent) startVerify() {
	t.stopVerify()
	ctx, cancel := context.WithCancel(context.Background())
	v := &verification{cancel: cancel, done: make(chan struct{})}
	t.verify = v
	tt := t.t
	go func() {
		defer close(v.done)
		// piece by piece rather than VerifyDataContext, so a cancel is seen
		// before the next piece check can start on a closed torrent
		for i := 0; i < tt.NumPieces(); i++ {
			if ctx.Err() != nil {
				return
			}
			if err := tt.Piece(i).VerifyDataContext(ctx); err != nil {
				if ctx.Err() == nil {
					log.Printf("verify %s: piece %d: %s", tt.InfoHash().HexString(), i, err)
				}
				return
			}
		}
	}()
}

// stopVerify cancels a running verification and waits for it to return.
// Every close of the torrent must come after it (see closeClientLocked).
func (t *Torrent) stopVerify() {
	if t.verify == nil {
		return
	}
	t.verify.cancel()
	<-t.verify.done
	t.verify = nil
}

func saveMetainfo(st *store, tt *torrent.Torrent) {
	ih := tt.InfoHash().HexString()
	mi := tt.Metainfo()
	if err := st.saveMetainfo(ih, &mi); err != nil {
		log.Printf("save %s: %s", ih, err)
	}
}

func (e *Engine) saveLocked(t *Torrent) {
	var stopped []string
	for p := range t.stopped {
		stopped = append(stopped, p)
	}
	sort.Strings(stopped)
	r := record{InfoHash: t.InfoHash, Magnet: t.magnet, Started: t.Started, StoppedFiles: stopped}
	if err := e.store.save(r); err != nil {
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
	if e.client == nil {
		return nil, errNotRunning
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
	t.t.AllowDataDownload()
	t.t.AllowDataUpload()
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
	t.stopVerify()
	t.t.Drop()
	return nil
}

// StartFile and StopFile select which files of a torrent get downloaded.
// They don't start or pause the torrent itself, and repeating one is a no-op.
func (e *Engine) StartFile(infohash, filepath string) error {
	return e.selectFile(infohash, filepath, true)
}

func (e *Engine) StopFile(infohash, filepath string) error {
	return e.selectFile(infohash, filepath, false)
}

func (e *Engine) selectFile(infohash, path string, selected bool) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	var file *torrent.File
	if t.t.Info() != nil {
		for _, f := range t.t.Files() {
			if f.Path() == path {
				file = f
				break
			}
		}
	}
	if file == nil {
		return fmt.Errorf("Missing file %s", path)
	}
	if selected {
		delete(t.stopped, path)
		file.Download()
	} else {
		if t.stopped == nil {
			t.stopped = map[string]bool{}
		}
		t.stopped[path] = true
		file.SetPriority(torrent.PiecePriorityNone)
	}
	e.upsertTorrent(t.t)
	e.saveLocked(t)
	return nil
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
