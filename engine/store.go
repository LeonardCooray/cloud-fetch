package engine

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// storeDirName lives inside the download directory so it survives in Docker,
// where only the downloads volume is persisted. listFiles hides dot entries.
const storeDirName = ".cloud-torrent"

type record struct {
	InfoHash string `json:"infoHash"`
	Magnet   string `json:"magnet,omitempty"`
	Started  bool   `json:"started"`
}

type savedTorrent struct {
	record
	Metainfo *metainfo.MetaInfo
}

// store persists one <infohash>.json per torrent, plus <infohash>.torrent
// once metadata is known.
type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func validInfoHash(ih string) error {
	if len(ih) != 40 {
		return fmt.Errorf("invalid infohash %q", ih)
	}
	if _, err := hex.DecodeString(ih); err != nil {
		return fmt.Errorf("invalid infohash %q", ih)
	}
	return nil
}

func (s *store) save(r record) error {
	if err := validInfoHash(r.InfoHash); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return s.writeAtomic(r.InfoHash+".json", func(f *os.File) error {
		_, err := f.Write(b)
		return err
	})
}

func (s *store) saveMetainfo(ih string, mi *metainfo.MetaInfo) error {
	if err := validInfoHash(ih); err != nil {
		return err
	}
	return s.writeAtomic(ih+".torrent", func(f *os.File) error {
		return mi.Write(f)
	})
}

func (s *store) remove(ih string) error {
	if err := validInfoHash(ih); err != nil {
		return err
	}
	for _, name := range []string{ih + ".json", ih + ".torrent"} {
		if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// load never fails as a whole: a bad record is reported and skipped so one
// corrupt file can't stop every other torrent from coming back.
func (s *store) load() ([]savedTorrent, []error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}
	var out []savedTorrent
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		ih := strings.TrimSuffix(name, ".json")
		if validInfoHash(ih) != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		var r record
		if err := json.Unmarshal(b, &r); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		r.InfoHash = ih
		st := savedTorrent{record: r}
		tpath := filepath.Join(s.dir, ih+".torrent")
		if _, err := os.Stat(tpath); err == nil {
			mi, err := metainfo.LoadFromFile(tpath)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s.torrent: %w", ih, err))
			} else {
				st.Metainfo = mi
			}
		}
		if st.Metainfo == nil && st.Magnet == "" {
			errs = append(errs, fmt.Errorf("%s: no metainfo or magnet to restore from", name))
			continue
		}
		out = append(out, st)
	}
	return out, errs
}

func (s *store) writeAtomic(name string, write func(*os.File) error) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".tmp-"+name+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := write(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, name)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
