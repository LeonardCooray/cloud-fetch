package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeonardCooray/cloud-fetch/engine"
)

// Live updates: a connected browser must receive pushes made after it
// connected, not just the snapshot it got on connect.
func TestSyncClientsReceiveLaterPushes(t *testing.T) {
	s, _ := newTestServer(t, "")
	s.initSync()
	srv := httptest.NewServer(s.handler())
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/sync", nil)
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if l := sc.Text(); strings.HasPrefix(l, "data:") && !strings.Contains(l, `"ping"`) {
				events <- l
			}
		}
	}()
	waitFor := func(substr string) {
		t.Helper()
		deadline := time.After(3 * time.Second)
		for {
			select {
			case e := <-events:
				if strings.Contains(e, substr) {
					return
				}
			case <-deadline:
				t.Fatalf("no sync event containing %q", substr)
			}
		}
	}

	waitFor(`"body"`)
	s.state.Lock()
	s.state.Stats.Title = "pushed-after-connect"
	s.state.Unlock()
	s.state.Push()
	waitFor("pushed-after-connect")
}

// A browser connecting right after startup must get the loaded config, not
// the zero value the state held before reconfigure ran.
func TestFirstSyncIncludesLoadedConfig(t *testing.T) {
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(builtinSearchConfig())
	}))
	defer search.Close()
	dir := t.TempDir()
	cfg := filepath.Join(dir, DefaultConfigName)
	incoming := freePort(t)
	os.WriteFile(cfg, []byte(fmt.Sprintf(`{"DownloadDirectory":%q,"IncomingPort":%d}`, filepath.Join(dir, "dl"), incoming)), 0600)
	port := freePort(t)
	s := &Server{Title: "t", Port: port, Host: "127.0.0.1", ConfigPath: cfg, SearchConfigURL: search.URL}
	// widen velox's push throttle so an early push (the search config lands
	// before reconfigure) holds the next one back while the client connects
	s.state.Throttle = 2 * time.Second
	go s.Run("test")

	var res *http.Response
	for i := 0; i < 200; i++ {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/sync", port), nil)
		req.Header.Set("Accept", "text/event-stream")
		if r, err := http.DefaultClient.Do(req); err == nil {
			res = r
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if res == nil {
		t.Fatal("server never accepted /sync")
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		if !strings.HasPrefix(l, "data:") || !strings.Contains(l, `"body"`) {
			continue
		}
		var msg struct {
			Body struct {
				Config struct{ IncomingPort int }
			}
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(l, "data:")), &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Body.Config.IncomingPort != incoming {
			t.Fatalf("first sync had IncomingPort %d, want %d", msg.Body.Config.IncomingPort, incoming)
		}
		return
	}
	t.Fatal("no sync body received")
}

// Adding and removing torrents while velox marshals the state every push must
// not race: the engine's own map and structs may not be shared with velox.
func TestSyncSurvivesTorrentChurn(t *testing.T) {
	s, root := newTestServer(t, "")
	if err := s.reconfigure(engine.Config{DownloadDirectory: filepath.Join(root, "downloads"), IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	s.initSync()
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/sync", nil)
	req.Header.Set("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	go io.Copy(io.Discard, res.Body)

	stop := time.After(1500 * time.Millisecond)
	for i := 0; ; i++ {
		select {
		case <-stop:
			return
		default:
		}
		ih := fmt.Sprintf("%040x", i+1)
		s.engine.NewMagnet("magnet:?xt=urn:btih:" + ih)
		s.state.Lock()
		s.state.Torrents = s.engine.GetTorrents()
		s.state.Unlock()
		s.state.Push()
		s.engine.StopTorrent(ih)
		s.engine.DeleteTorrent(ih)
		time.Sleep(2 * time.Millisecond)
	}
}
