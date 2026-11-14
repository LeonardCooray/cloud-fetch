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
	// downloads are held for low disk space; see FreeSpace
	lowDisk bool
	limits  *limiters
	// the last queue order handed out; see nextOrderLocked
	lastOrder int64
	// the latest health check, running or finished; see Check
	probeMu sync.Mutex
	probe   *healthProbe
}

func New() *Engine {
	return &Engine{ts: map[string]*Torrent{}, limits: newLimiters()}
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
	// the cap keeps the byte count well inside an int64
	if c.MinimumFreeSpaceGB < 0 || c.MinimumFreeSpaceGB > 1000000 {
		return fmt.Errorf("Minimum free space must be between 0 and 1000000 GB")
	}
	if c.DeleteDownloadsAfterDays < 0 || c.DeleteDownloadsAfterDays > 3650 {
		return fmt.Errorf("Delete downloads after days must be between 0 and 3650")
	}
	if c.MaxDownloadKBps < 0 || c.MaxDownloadKBps > maxRateKBps || c.MaxUploadKBps < 0 || c.MaxUploadKBps > maxRateKBps {
		return fmt.Errorf("Speed limits must be between 0 and %d KB/s", maxRateKBps)
	}
	if c.MaxActiveTorrents < 0 || c.MaxActiveTorrents > maxActiveTorrents {
		return fmt.Errorf("Max active torrents must be between 0 and %d", maxActiveTorrents)
	}
	e.mut.Lock()
	if e.client != nil && sameClient(e.config, c) {
		// speed limits and the queue change now; the next FreeSpace and
		// CleanupDownloads calls apply the other values
		e.config = c
		e.limits.set(c)
		e.queueLocked()
		e.mut.Unlock()
		return nil
	}
	hadClient, prev := e.client != nil, e.config
	e.closeClientLocked()
	e.mut.Unlock()
	if hadClient {
		time.Sleep(1 * time.Second)
	}
	client, err := newClient(c, e.limits)
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
		if back, berr = newClient(prev, e.limits); berr == nil {
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

// closeClientLocked leaves running verifications to end on their own: their
// next piece check fails on the closed torrent. That needs anacrolix/torrent
// v1.62.0 or later; earlier releases kept the client lock there (#1119).
// Afterwards the engine has no client until install gives it one.
func (e *Engine) closeClientLocked() {
	if e.client == nil {
		return
	}
	e.client.Close()
	e.client = nil
	e.ts = map[string]*Torrent{}
	e.forgetCheck()
}

// clientConfig also sets l to c's speed limits, since the client shares them.
func clientConfig(c Config, l *limiters) *torrent.ClientConfig {
	config := torrent.NewDefaultClientConfig()
	l.set(c)
	config.DownloadRateLimiter = l.down
	config.UploadRateLimiter = l.up
	config.MaxAllocPeerRequestDataPerConn = maxPeerRequest
	config.DataDir = c.DownloadDirectory
	config.NoUpload = !c.EnableUpload
	config.Seed = c.EnableSeeding
	config.ListenPort = c.IncomingPort
	if c.DisableEncryption {
		config.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{Preferred: false, RequirePreferred: true}
	}
	return config
}

// newClient is a variable so tests can make client starts fail.
var newClient = func(c Config, l *limiters) (*torrent.Client, error) {
	return torrent.NewClient(clientConfig(c, l))
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
	e.forgetCheck()
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
		t := e.upsertTorrent(tt)
		t.stopped = toSet(st.StoppedFiles)
		t.order, t.forced = st.Order, st.Forced
		e.lastOrder = max(e.lastOrder, st.Order)
		e.trackLocked(tt, st.Magnet, st.Started, false)
	}
	e.queueLocked()
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
	wasStarted := known && existing.Started
	started := e.config.AutoStart || wasStarted
	t := e.trackLocked(tt, magnet, started, !known)
	if started && !wasStarted {
		t.order = e.nextOrderLocked()
	}
	e.queueLocked()
	e.saveLocked(t)
	return nil
}

// trackLocked registers tt and applies its started state. With an active
// limit set, a torrent it starts waits as queued until the caller's
// queueLocked finds it a slot.
// Metadata may still be unknown (magnets), so the .torrent save and
// DownloadAll wait for GotInfo.
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
	wasStarted := t.Started
	t.Started = started
	t.SpacePaused = started && e.lowDisk
	if started && !wasStarted && e.config.MaxActiveTorrents > 0 {
		t.Queued = true
	}
	if started {
		tt.AllowDataUpload()
	} else {
		t.Queued, t.QueuePosition, t.forced = false, 0, false
		tt.DisallowDataUpload()
	}
	t.syncDownload()
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
		// piece by piece: Piece.VerifyDataContext queues the hash before it
		// looks at ctx, so a cancel is checked here first, and the log gets
		// the piece index
		for i := 0; i < tt.NumPieces(); i++ {
			if ctx.Err() != nil {
				return
			}
			if err := tt.Piece(i).VerifyDataContext(ctx); err != nil {
				select {
				case <-tt.Closed():
					// a client close ends a verification this way; not an error
				default:
					if ctx.Err() == nil {
						log.Printf("verify %s: piece %d: %s", tt.InfoHash().HexString(), i, err)
					}
				}
				return
			}
		}
	}()
}

// stopVerify cancels a running verification and waits for it to return.
// dropLocked calls it before dropping the torrent, and startVerify before
// starting another; a client close doesn't need to (see closeClientLocked).
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
	r := record{InfoHash: t.InfoHash, Magnet: t.magnet, Started: t.Started, StoppedFiles: stopped, Order: t.order, Forced: t.forced}
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
	// a torrent that just finished frees its slot
	e.queueLocked()
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
		torrent = &Torrent{InfoHash: ih, downloading: true}
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
	if e.lowDisk {
		return e.lowDiskError()
	}
	e.startLocked(t)
	e.saveLocked(t)
	return nil
}

// startLocked resumes a paused torrent at the back of the queue, so it never
// bumps a download that's already running.
func (e *Engine) startLocked(t *Torrent) {
	t.Started = true
	t.order = e.nextOrderLocked()
	t.t.AllowDataUpload()
	e.queueLocked()
}

// StartNow lets a queued torrent download straight away, over
// MaxActiveTorrents, until it finishes or is paused. Nothing running is
// bumped. A torrent that isn't queued (a slot freed up meanwhile) is left
// as it is.
func (e *Engine) StartNow(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if !t.Started {
		return fmt.Errorf("Paused: resume it instead")
	}
	if !t.Queued {
		return nil
	}
	if e.lowDisk {
		return e.lowDiskError()
	}
	e.forceLocked(t)
	return nil
}

func (e *Engine) forceLocked(t *Torrent) {
	t.forced = true
	e.queueLocked()
	// a free slot ends the override at once, and unforceLocked saved that
	if t.forced {
		e.saveLocked(t)
	}
}

// nextOrderLocked hands out queue orders: the time, but never at or below
// one already used, so a clock stepping back can't jump the queue.
func (e *Engine) nextOrderLocked() int64 {
	e.lastOrder = max(time.Now().UnixNano(), e.lastOrder+1)
	return e.lastOrder
}

// maxActiveTorrents caps the MaxActiveTorrents setting.
const maxActiveTorrents = 10000

// queueLocked lets the MaxActiveTorrents earliest-started unfinished torrents
// download and queues the rest. Paused and finished torrents, and ones with
// every file deselected, hold no slot; a torrent waiting for metadata holds
// one. A started torrent that wants a slot again (a file ticked, a piece
// gone bad) joins the back, like a Resume, rather than bump a download. A forced torrent downloads
// regardless and turns into an ordinary one when its turn for a slot comes.
// The low-disk guard holds downloads separately (SpacePaused), so a slot it
// holds stays taken and nothing queued starts in its place.
func (e *Engine) queueLocked() {
	var waiting []*Torrent
	for _, t := range e.ts {
		want := t.wantsSlot()
		if want && t.idle {
			t.order = e.nextOrderLocked()
			e.saveLocked(t)
		}
		t.idle = t.Started && !want
		if want {
			waiting = append(waiting, t)
			continue
		}
		t.Queued, t.QueuePosition = false, 0
		e.unforceLocked(t)
		t.syncDownload()
	}
	sort.Slice(waiting, func(i, j int) bool {
		if waiting[i].order != waiting[j].order {
			return waiting[i].order < waiting[j].order
		}
		return waiting[i].InfoHash < waiting[j].InfoHash
	})
	// ahead counts what takes a freed slot before the next queued torrent:
	// those queued earlier, and extras started earlier
	limit, slots, ahead := e.config.MaxActiveTorrents, 0, 0
	for _, t := range waiting {
		t.Queued, t.QueuePosition = false, 0
		switch {
		case limit <= 0 || slots < limit:
			slots++
			e.unforceLocked(t)
		case t.forced:
			ahead++
		default:
			ahead++
			t.Queued, t.QueuePosition = true, ahead
		}
		t.syncDownload()
	}
}

func (e *Engine) unforceLocked(t *Torrent) {
	if t.forced {
		t.forced = false
		e.saveLocked(t)
	}
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
	t.Started = false
	// paused by hand now, so space coming back doesn't resume it
	t.SpacePaused = false
	t.Queued, t.QueuePosition, t.forced = false, 0, false
	t.syncDownload()
	t.t.DisallowDataUpload()
	// its slot goes to the next in the queue
	e.queueLocked()
	e.saveLocked(t)
	return nil
}

// lowDiskMargin is how far free space must climb above the minimum before
// held downloads resume, so they don't flap around the threshold.
const lowDiskMargin = 1 << 30

// FreeSpace takes the download directory's free bytes. Below the
// MinimumFreeSpaceGB setting it holds every started torrent's downloading
// (uploads carry on) and any torrent started meanwhile; once free space is
// back above the minimum plus lowDiskMargin it resumes the ones it held.
// Torrents paused by hand stay paused. It reports whether downloads are held.
func (e *Engine) FreeSpace(free int64) bool {
	e.mut.Lock()
	defer e.mut.Unlock()
	min := int64(e.config.MinimumFreeSpaceGB) << 30
	low := e.lowDisk
	switch {
	case min <= 0:
		low = false
	case free < min:
		low = true
	case free >= min+lowDiskMargin:
		low = false
	}
	if low == e.lowDisk {
		return low
	}
	e.lowDisk = low
	if low {
		log.Printf("Holding downloads: %.1f GB free, below the %d GB minimum", float64(free)/(1<<30), e.config.MinimumFreeSpaceGB)
	} else {
		log.Printf("Resuming held downloads: %.1f GB free", float64(free)/(1<<30))
	}
	for _, t := range e.ts {
		switch {
		case low && t.Started:
			t.holdForSpace()
		case !low && t.SpacePaused:
			t.SpacePaused = false
			t.syncDownload()
		}
	}
	return low
}

func (e *Engine) lowDiskError() error {
	return fmt.Errorf("Less than %d GB free, so downloads are on hold until space frees up", e.config.MinimumFreeSpaceGB)
}

// DeleteTorrent drops the torrent and keeps its files.
func (e *Engine) DeleteTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	e.dropLocked(t)
	e.queueLocked()
	return nil
}

// ClearFinished drops every complete torrent, keeping the files, and returns
// how many it dropped.
func (e *Engine) ClearFinished() int {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return 0
	}
	for _, tt := range e.client.Torrents() {
		e.upsertTorrent(tt)
	}
	n := 0
	for _, t := range e.ts {
		if t.Complete() {
			e.dropLocked(t)
			n++
		}
	}
	return n
}

func (e *Engine) dropLocked(t *Torrent) {
	if err := e.store.remove(t.InfoHash); err != nil {
		log.Printf("delete %s: %s", t.InfoHash, err)
	}
	t.Dropped = true
	delete(e.ts, t.InfoHash)
	t.stopVerify()
	t.t.Drop()
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
	// deselecting what's left can finish a torrent and free its slot, and
	// ticking a file again can make it want one
	e.queueLocked()
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
