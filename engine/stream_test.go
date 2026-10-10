package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

// packData is what multiFileTorrent writes for file i.
func packData(i int) []byte {
	data := make([]byte, 4*pieceLen)
	for j := range data {
		data[j] = byte((j*31 + i*7 + 3) % 251)
	}
	return data
}

// A deselected file downloads nothing on its own, so the bytes coming back
// prove the reader fetched them.
func TestStreamFetchesADeselectedFileAsItIsRead(t *testing.T) {
	seed, mi := seeder(t)
	ih := mi.HashInfoBytes().HexString()
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: false})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
		t.Fatal(err)
	}
	if err := leech.StopFile(ih, "pack/b.bin"); err != nil {
		t.Fatal(err)
	}
	if err := leech.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	connect(t, leech, seed, ih)
	filePercentEventually(t, leech, ih, "pack/a.bin", 100)
	stillAt(t, leech, ih, "pack/b.bin", 0, time.Second)

	r, ok, err := leech.StreamFile(context.Background(), "pack/b.bin")
	if err != nil || !ok {
		t.Fatalf("StreamFile = %v, %v; want a stream", ok, err)
	}
	defer r.Close()
	if n, err := r.Seek(0, io.SeekEnd); err != nil || n != 4*pieceLen {
		t.Fatalf("size %d (%v), want %d", n, err, 4*pieceLen)
	}
	// a player seeking into the middle, as a Range request does
	if _, err := r.Seek(2*pieceLen+100, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, pieceLen)
	within(t, 30*time.Second, "stream read", func() {
		_, err = io.ReadFull(r, got)
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := packData(1)[2*pieceLen+100 : 3*pieceLen+100]; !bytes.Equal(got, want) {
		t.Fatal("streamed bytes differ from the file's")
	}
}

func TestStreamAcceptsThePartName(t *testing.T) {
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(multiFileTorrent(t, t.TempDir()))); err != nil {
		t.Fatal(err)
	}
	r, ok, err := leech.StreamFile(context.Background(), "pack/a.bin.part")
	if err != nil || !ok {
		t.Fatalf("StreamFile = %v, %v; want a stream", ok, err)
	}
	r.Close()
}

func TestStreamRefusesAPausedTorrent(t *testing.T) {
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: false})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(multiFileTorrent(t, t.TempDir()))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := leech.StreamFile(context.Background(), "pack/a.bin"); !errors.Is(err, ErrStreamPaused) {
		t.Fatalf("err %v, want ErrStreamPaused", err)
	}
}

func TestStreamLeavesFinishedAndUnknownFilesToDisk(t *testing.T) {
	seed, _ := seeder(t)
	for _, p := range []string{"pack/a.bin", "pack/missing.bin", "pack"} {
		if _, ok, err := seed.StreamFile(context.Background(), p); ok || err != nil {
			t.Fatalf("%s: StreamFile = %v, %v; want disk", p, ok, err)
		}
	}
}

// With no peers the read waits on its first piece; the request ending must
// release it.
func TestStreamReadEndsWithItsContext(t *testing.T) {
	leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
	if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(multiFileTorrent(t, t.TempDir()))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r, ok, err := leech.StreamFile(ctx, "pack/a.bin")
	if err != nil || !ok {
		t.Fatalf("StreamFile = %v, %v; want a stream", ok, err)
	}
	defer r.Close()
	time.AfterFunc(200*time.Millisecond, cancel)
	within(t, 10*time.Second, "read after cancel", func() {
		_, err = r.Read(make([]byte, 10))
	})
	if err == nil {
		t.Fatal("read with no peers returned data")
	}
}

// A read blocked on a missing piece must end when the torrent is deleted or
// paused under it, and leave the client answering (anacrolix #1119 was a
// lock leaked on exactly this kind of close).
func TestStreamReadEndsWhenTheTorrentGoesAway(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(e *Engine, ih string) error
	}{
		{"delete", (*Engine).DeleteTorrent},
		{"pause", (*Engine).StopTorrent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leech := startEngineWith(t, Config{DownloadDirectory: t.TempDir(), AutoStart: true})
			mi := multiFileTorrent(t, t.TempDir())
			ih := mi.HashInfoBytes().HexString()
			if err := leech.NewTorrent(torrent.TorrentSpecFromMetaInfo(mi)); err != nil {
				t.Fatal(err)
			}
			r, ok, err := leech.StreamFile(context.Background(), "pack/a.bin")
			if err != nil || !ok {
				t.Fatalf("StreamFile = %v, %v; want a stream", ok, err)
			}
			defer r.Close()
			readErr := make(chan error, 1)
			go func() {
				_, err := r.Read(make([]byte, 10))
				readErr <- err
			}()
			time.Sleep(200 * time.Millisecond)
			within(t, 10*time.Second, tc.name, func() {
				if err := tc.act(leech, ih); err != nil {
					t.Error(err)
				}
			})
			select {
			case err := <-readErr:
				if err == nil {
					t.Fatal("read with no peers returned data")
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("read still blocked 10s after %s", tc.name)
			}
			within(t, 10*time.Second, "GetTorrents", func() { leech.GetTorrents() })
		})
	}
}
