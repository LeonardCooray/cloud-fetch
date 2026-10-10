package engine

import (
	"context"
	"errors"
	"io"

	"github.com/anacrolix/torrent"
)

// ErrStreamPaused is returned for an unfinished file whose torrent is paused:
// a paused torrent requests nothing, so a stream would wait forever on the
// first missing piece.
var ErrStreamPaused = errors.New("torrent is paused: start it to stream this file")

// streamReadahead is how far ahead of the player pieces are requested before
// a contiguous read has built up anacrolix's own readahead, so playback
// doesn't start by fetching one piece at a time.
const streamReadahead = 8 << 20

// StreamFile opens an unfinished torrent file for reading while it downloads.
// path is relative to the download directory, slash-separated, and may carry
// the ".part" suffix the file has on disk until it completes. ok is false
// when no torrent has an unfinished file there; the caller serves it from
// disk. Reads block until their pieces arrive and pass the hash check (no
// SetResponsive: a download manager may save these bytes), and the pieces
// just ahead of the read position are fetched first, even in a deselected
// file. ctx ends any blocked read, so it should be the request's context.
func (e *Engine) StreamFile(ctx context.Context, path string) (r io.ReadSeekCloser, ok bool, err error) {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return nil, false, nil
	}
	for _, t := range e.ts {
		if t.t.Info() == nil {
			continue
		}
		for _, f := range t.t.Files() {
			if (f.Path() != path && f.Path()+".part" != path) || f.BytesCompleted() == f.Length() {
				continue
			}
			if !t.Started {
				return nil, false, ErrStreamPaused
			}
			tr := f.NewReader()
			tr.SetContext(ctx)
			tr.SetReadaheadFunc(func(rc torrent.ReadaheadContext) int64 {
				return max(rc.CurrentPos-rc.ContiguousReadStartPos, streamReadahead)
			})
			return tr, true, nil
		}
	}
	return nil, false, nil
}
