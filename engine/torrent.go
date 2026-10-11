package engine

import (
	"time"

	"github.com/anacrolix/torrent"
)

type Torrent struct {
	//anacrolix/torrent
	InfoHash   string
	Name       string
	Loaded     bool
	Downloaded int64
	Size       int64
	Files      []*File
	//cloud fetch
	Started      bool
	Dropped      bool
	Percent      float32
	DownloadRate float32
	t            *torrent.Torrent
	magnet       string
	stopped      map[string]bool // file paths deselected by StopFile
	verify       *verification
	updatedAt    time.Time
}

type File struct {
	//anacrolix/torrent
	Path      string
	Size      int64
	Chunks    int
	Completed int
	//cloud fetch
	Started bool // selected for download
	Percent float32
	f       *torrent.File
}

func (torrent *Torrent) Update(t *torrent.Torrent) {
	torrent.Name = t.Name()
	torrent.Loaded = t.Info() != nil
	if torrent.Loaded {
		torrent.updateLoaded(t)
	}
	torrent.t = t
}

func (torrent *Torrent) updateLoaded(t *torrent.Torrent) {

	torrent.Size = t.Length()
	totalChunks := 0
	totalCompleted := 0

	tfiles := t.Files()
	if len(tfiles) > 0 && torrent.Files == nil {
		torrent.Files = make([]*File, len(tfiles))
	}
	//merge in files
	for i, f := range tfiles {
		path := f.Path()
		file := torrent.Files[i]
		if file == nil {
			file = &File{Path: path}
			torrent.Files[i] = file
		}
		chunks := f.State()

		file.Started = !torrent.stopped[path]
		file.Size = f.Length()
		file.Chunks = len(chunks)
		completed := 0
		for _, p := range chunks {
			if p.Complete {
				completed++
			}
		}
		file.Completed = completed
		file.Percent = percent(int64(file.Completed), int64(file.Chunks))
		file.f = f

		totalChunks += file.Chunks
		totalCompleted += file.Completed
	}

	//cacluate rate
	now := time.Now()
	bytes := t.BytesCompleted()
	torrent.Percent = percent(bytes, torrent.Size)
	if !torrent.updatedAt.IsZero() {
		dt := float32(now.Sub(torrent.updatedAt))
		db := float32(bytes - torrent.Downloaded)
		rate := db * (float32(time.Second) / dt)
		if rate >= 0 {
			torrent.DownloadRate = rate
		}
	}
	torrent.Downloaded = bytes
	torrent.updatedAt = now
}

// Complete reports whether the files picked for download are all there; with
// none picked it goes by the whole torrent. The web UI's torrentStatus
// (static/files/js/lib/status.js) uses the same rule for its Done badge.
// Until verification of data found on disk ends, a same-size file counts
// as complete whatever it holds, so nothing is complete before then.
func (torrent *Torrent) Complete() bool {
	if !torrent.Loaded || torrent.verifying() {
		return false
	}
	picked := 0
	for _, f := range torrent.Files {
		if f == nil || !f.Started {
			continue
		}
		picked++
		if f.Percent < 100 {
			return false
		}
	}
	if picked == 0 {
		return torrent.Percent >= 100
	}
	return true
}

func (torrent *Torrent) verifying() bool {
	if torrent.verify == nil {
		return false
	}
	select {
	case <-torrent.verify.done:
		return false
	default:
		return true
	}
}

func percent(n, total int64) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}
