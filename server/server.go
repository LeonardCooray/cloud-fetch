package server

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/LeonardCooray/cloud-fetch/engine"
	"github.com/LeonardCooray/cloud-fetch/static"
	"github.com/NYTimes/gziphandler"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/scraper/scraper"
	"github.com/jpillora/velox"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/crypto/acme/autocert"
)

// Server is the "State" portion of the diagram
type Server struct {
	//config
	Title      string `help:"Title of this instance" opts:"env=TITLE"`
	Port       int    `help:"Listening port" opts:"env=PORT"`
	Host       string `help:"Listening interface (default: all with --auth, localhost only without)"`
	Auth       string `help:"Optional basic auth in form 'user:password'" opts:"env=AUTH"`
	ConfigPath string `help:"Configuration file path"`
	KeyPath    string `help:"TLS Key file path"`
	CertPath   string `help:"TLS Certicate file path" short:"r"`
	Log        bool   `help:"Enable request logging"`
	Open       bool   `help:"Open now with your default browser"`
	//let's encrypt
	Domain        string `help:"Serve HTTPS with a free, auto-renewing Let's Encrypt certificate for this domain (needs --http-port reachable from the internet)" opts:"short=-,env=DOMAIN"`
	HTTPPort      int    `help:"Plain HTTP port for Let's Encrypt checks and the redirect to HTTPS (only with --domain)" opts:"name=http-port,short=-"`
	CertCache     string `help:"Where to keep Let's Encrypt certificates (default: certs/ next to the config file)" opts:"short=-"`
	ACMEStaging   bool   `help:"Use Let's Encrypt's staging server (untrusted test certificates, generous rate limits)" opts:"name=acme-staging,short=-"`
	ACMEDirectory string `help:"ACME directory URL, to use a certificate authority other than Let's Encrypt" opts:"name=acme-directory,short=-"`
	//search
	SearchConfigURL string `help:"URL of the search provider list, re-checked every 30 minutes; empty uses only the built-in list" opts:"name=search-config-url,short=-"`
	//http handlers
	files, static http.Handler
	scraper       *scraper.Handler
	scraperh      http.Handler
	searchConfig  []byte // normalized; only the fetch loop touches it after Run starts it
	//set by Run so closeListener can stop it
	runMut     sync.Mutex
	httpServer *http.Server
	//share links; the key is loaded on first use
	shareMu     sync.Mutex
	shareSecret []byte
	//torrent engine
	engine *engine.Engine
	state  struct {
		velox.State
		sync.Mutex
		Config          engine.Config
		SearchProviders scraper.Config
		Downloads       *fsNode
		Torrents        map[string]*engine.Torrent
		Users           map[string]string
		Stats           struct {
			Title   string
			Version string
			Runtime string
			Uptime  time.Time
			System  stats
		}
	}
}

// Run the server
func (s *Server) Run(version string) error {
	isTLS := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if isTLS && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}
	var certs *autocert.Manager
	if s.Domain != "" {
		var err error
		if certs, err = s.certManager(); err != nil {
			return err
		}
		isTLS = true
		if s.HTTPPort == 0 {
			s.HTTPPort = 80
		}
	}
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	s.state.Stats.System.pusher = velox.Pusher(&s.state)
	//init maps
	s.state.Users = map[string]string{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{
		Log: false, Debug: false,
		Headers: map[string]string{
			//we're a trusty browser :)
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/57.0.2987.133 Safari/537.36",
		},
	}
	if err := s.scraper.LoadConfig(builtinSearchConfig()); err != nil {
		log.Fatal(err)
	}
	//scraper
	s.searchConfig, _ = normalize(builtinSearchConfig())
	s.state.SearchProviders = s.scraper.Config //share scraper config
	s.scraperh = http.StripPrefix("/search", s.scraper)
	//torrent engine
	s.engine = engine.New()
	//configure engine
	c := engine.Config{
		DownloadDirectory: "./downloads",
		EnableUpload:      true,
		AutoStart:         true,
	}
	if moved, err := migrateLegacyConfig(s.ConfigPath); err != nil {
		return fmt.Errorf("migrating %s: %s", legacyConfigName, err)
	} else if moved {
		log.Printf("Renamed %s to %s (Cloud Torrent is now Cloud Fetch)", legacyConfigName, s.ConfigPath)
	}
	if _, err := os.Stat(s.ConfigPath); err == nil {
		if b, err := os.ReadFile(s.ConfigPath); err != nil {
			return fmt.Errorf("Read configuration error: %s", err)
		} else if len(b) == 0 {
			//ignore empty file
		} else if err := json.Unmarshal(b, &c); err != nil {
			return fmt.Errorf("Malformed configuration: %s", err)
		}
	}
	if c.IncomingPort <= 0 || c.IncomingPort >= 65535 {
		c.IncomingPort = 50007
	}
	if err := s.reconfigure(c); err != nil {
		return fmt.Errorf("initial configure failed: %s", err)
	}
	// after reconfigure, so the first snapshot any client gets already has the
	// loaded config; pushes made before this (the search config) are no-ops
	s.initSync()
	go s.fetchSearchConfigLoop()
	//poll torrents and files
	go func() {
		for {
			s.state.Lock()
			s.state.Torrents = s.engine.GetTorrents()
			s.state.Downloads = s.listFiles()
			s.state.Unlock()
			s.state.Push()
			time.Sleep(1 * time.Second)
		}
	}()
	//start collecting stats
	go func() {
		for {
			c := s.engine.Config()
			s.state.Stats.System.loadStats(c.DownloadDirectory, &s.state)
			time.Sleep(5 * time.Second)
		}
	}()

	host := listenHost(s.Host, s.Auth)
	if s.Host == "" && s.Auth == "" {
		log.Printf("No --auth set, so listening on localhost only; pass --host 0.0.0.0 to expose it anyway")
	}
	addr := fmt.Sprintf("%s:%d", host, s.Port)
	proto := "http"
	if isTLS {
		proto += "s"
	}
	if s.Open {
		openhost := host
		if openhost == "0.0.0.0" {
			openhost = "localhost"
		}
		go func() {
			time.Sleep(1 * time.Second)
			open.Run(fmt.Sprintf("%s://%s:%d", proto, openhost, s.Port))
		}()
	}
	h := s.handler()
	log.Printf("Listening at %s://%s", proto, addr)
	//serve!
	server := http.Server{
		//disable http2 due to velox bug
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		//address
		Addr: addr,
		//handler stack
		Handler: h,
	}
	s.runMut.Lock()
	s.httpServer = &server
	s.runMut.Unlock()
	if certs != nil {
		if host == "127.0.0.1" {
			log.Printf("--domain is set but the server only listens on localhost, so Let's Encrypt can't reach it; add --auth or --host 0.0.0.0")
		}
		server.TLSConfig = tlsConfigFor(certs)
		httpAddr := fmt.Sprintf("%s:%d", host, s.HTTPPort)
		log.Printf("Let's Encrypt enabled for %s; challenges and HTTPS redirect on http://%s", s.Domain, httpAddr)
		errc := make(chan error, 2)
		go func() {
			err := http.ListenAndServe(httpAddr, certs.HTTPHandler(httpsRedirect(s.Domain, s.Port)))
			errc <- fmt.Errorf("HTTP listener on %s: %w", httpAddr, err)
		}()
		go func() { errc <- server.ListenAndServeTLS("", "") }()
		return <-errc
	}
	if isTLS {
		return server.ListenAndServeTLS(s.CertPath, s.KeyPath)
	}
	return server.ListenAndServe()
}

// closeListener stops the main listener, which makes Run return. Tests use
// it; the pollers and the Let's Encrypt HTTP listener keep running.
func (s *Server) closeListener() {
	s.runMut.Lock()
	srv := s.httpServer
	s.runMut.Unlock()
	if srv != nil {
		srv.Close()
	}
}

// initSync wires the embedded velox.State to this struct once, so every
// connection subscribes to the same state and later Push calls reach them.
// velox.Sync(gostruct, ...) would build a fresh State per connection, which
// only ever sends the snapshot taken on connect.
func (s *Server) initSync() {
	// SyncHandler sets s.state.Data to a marshaller of the whole struct and
	// initialises it; /sync still goes through s.state.Handle for the Users map
	velox.SyncHandler(&s.state)
}

// listenHost keeps an instance with no password off the network unless the
// user asks for an interface explicitly.
func listenHost(host, auth string) string {
	if host != "" {
		return host
	}
	if auth == "" {
		return "127.0.0.1"
	}
	return "0.0.0.0"
}

// handler builds the middleware chain, from last to first.
func (s *Server) handler() http.Handler {
	base := http.Handler(http.HandlerFunc(s.handle))
	//gzip
	compression := gzip.DefaultCompression
	minSize := 0 //IMPORTANT
	gzipWrap, _ := gziphandler.NewGzipLevelAndMinSize(compression, minSize)
	gz := gzipWrap(base)
	// downloads skip gzip: compressing a Range response breaks resume and
	// seeking, and it drops Content-Length that download managers rely on
	h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/download/") {
			base.ServeHTTP(w, r)
			return
		}
		gz.ServeHTTP(w, r)
	}))
	//auth
	if s.Auth != "" {
		user := s.Auth
		pass := ""
		if s := strings.SplitN(s.Auth, ":", 2); len(s) == 2 {
			user = s[0]
			pass = s[1]
		}
		ca := cookieauth.New().SetUserPass(user, pass)
		h = newLoginThrottle().wrap(ca.Wrap, h)
		log.Printf("Enabled HTTP authentication")
	}
	// share links carry their own signature, so they skip the login, and
	// gzip for the same reason /download/ does
	authed := h
	h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/share/") {
			s.serveShare(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
	if s.Log {
		h = requestlog.Wrap(h)
	}
	return h
}

func (s *Server) reconfigure(c engine.Config) error {
	dldir, err := filepath.Abs(c.DownloadDirectory)
	if err != nil {
		return fmt.Errorf("Invalid path")
	}
	c.DownloadDirectory = dldir
	if err := s.engine.Configure(c); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(&c, "", "  ")
	os.WriteFile(s.ConfigPath, b, 0600)
	// WriteFile keeps an existing file's mode, and older installs wrote 0755
	os.Chmod(s.ConfigPath, 0600)
	s.state.Lock()
	s.state.Config = c
	s.state.Unlock()
	s.state.Push()
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	//handle realtime client library
	if r.URL.Path == "/js/velox.js" {
		serveVeloxJS(w, r)
		return
	}
	//handle realtime client connections
	if r.URL.Path == "/sync" {
		conn, err := s.state.Handle(w, r)
		if err != nil {
			log.Printf("sync failed: %s", err)
			return
		}
		s.state.Lock()
		s.state.Users[conn.ID()] = r.RemoteAddr
		s.state.Unlock()
		s.state.Push()
		conn.Wait()
		s.state.Lock()
		delete(s.state.Users, conn.ID())
		s.state.Unlock()
		s.state.Push()
		return
	}
	//search
	if strings.HasPrefix(r.URL.Path, "/search") {
		s.scraperh.ServeHTTP(w, r)
		return
	}
	//share links, the API calls that answer with a body
	if r.URL.Path == "/api/share" || r.URL.Path == "/api/share-revoke" {
		s.handleShareAPI(w, r)
		return
	}
	//api call
	if strings.HasPrefix(r.URL.Path, "/api/") {
		//only pass request in, expect error out
		if err := s.api(r); err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
		}
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}
