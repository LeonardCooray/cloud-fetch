![Cloud Fetch web UI showing a seeding torrent with its files open, a finished paused torrent, one still downloading, and the downloads folder with copy-link buttons](docs/screenshot.png)

**Cloud Fetch** is a self-hosted remote torrent client written in Go. You add torrents from the browser, they download to the server's disk, and you fetch or stream the files over HTTP(S) with a browser, a download manager such as IDM, or a player such as VLC.

### Features

* Single binary, cross platform
* Torrents, their metadata and their started/stopped state survive restarts
* Files already on disk are hash-checked when you add a torrent, so wrong or partial data is downloaded again
* Optional HTTPS with a free, auto-renewing Let's Encrypt certificate
* Resumable, seekable downloads (HTTP Range) for download managers and media players
* Watch a file while it downloads: its link streams it, fetching the pieces just ahead of wherever the player is
* Embedded torrent search, with the provider list kept in this repo
* Real-time updates in a mobile-friendly UI

### Install

**From source** (needs [Go](https://go.dev/dl/) 1.25.4 or later; the Go in Ubuntu's `apt` is too old)

``` sh
go install github.com/LeonardCooray/cloud-fetch@latest
```

**Binaries and packages**

Every `v*` tag publishes binaries for Linux, macOS and Windows, plus `.deb`, `.rpm` and `.apk` packages, on the [releases page](https://github.com/LeonardCooray/cloud-fetch/releases), and a multi-arch Docker image at `ghcr.io/leonardcooray/cloud-fetch`. To try it on your own machine, run `cloud-fetch` and open http://localhost:3000. To run it on a server, follow the next section.

### Run it on a server

Pick **Docker** or **no Docker** below; the steps are the same shape. Before you start:

* Point a DNS record (for example `fetch.example.com`) at the server. This is only needed for HTTPS.
* Open ports **80** and **443** (HTTPS and the Let's Encrypt check) and **50007** over TCP and UDP (incoming peers; optional, but downloads are slower without it). Open them in your provider's firewall as well as on the machine, for example `sudo ufw allow 80,443/tcp` and `sudo ufw allow 50007`.
* Choose a login. Without `--auth` (or the `AUTH` variable) the server only listens on localhost, so a forgotten password fails closed rather than leaving it open.

Both setups below use `/srv/cloud-fetch` for data. Torrents, settings and certificates all live under it, so they survive upgrades.

#### With Docker

Install Docker if you don't have it (`curl -fsSL https://get.docker.com | sh`), then create the folders:

``` sh
sudo mkdir -p /srv/cloud-fetch/downloads /srv/cloud-fetch/config
```

Start it with HTTPS. Use `--acme-staging` first: its certificates are untrusted by browsers, but a mistake there doesn't count against Let's Encrypt's rate limits.

``` sh
docker run -d --name cloud-fetch --restart always \
  -p 80:80 -p 443:443 -p 50007:50007 -p 50007:50007/udp \
  -v /srv/cloud-fetch/downloads:/app/downloads \
  -v /srv/cloud-fetch/config:/config \
  -e AUTH=user:password \
  ghcr.io/leonardcooray/cloud-fetch \
  --domain fetch.example.com --port 443 --acme-staging \
  --config-path /config/cloud-fetch.json
```

Watch the logs with `docker logs -f cloud-fetch`, then open `https://fetch.example.com`. The first HTTPS request fetches the certificate; the browser warns that it's untrusted, which is expected for staging. Once it loads and asks for your login, switch to the real certificate:

``` sh
docker rm -f cloud-fetch && sudo rm -rf /srv/cloud-fetch/config/certs
```

Run the same `docker run` command again without `--acme-staging`. The certificate is stored in `/srv/cloud-fetch/config/certs` and renews itself about 30 days before it expires.

For plain HTTP (a home network, or a reverse proxy in front), drop `--domain`, `--port 443` and `--acme-staging`, publish `-p 3000:3000` instead of 80 and 443, and keep `-e AUTH=...`.

To upgrade, pull the new image and recreate the container with the same command:

``` sh
docker pull ghcr.io/leonardcooray/cloud-fetch && docker rm -f cloud-fetch
```

Pin a version by adding a tag, for example `ghcr.io/leonardcooray/cloud-fetch:0.1.0`.

#### Without Docker (Linux binary and systemd)

Any Linux works; Ubuntu 24.04 LTS or Debian 12 are good choices. Run these as root, or put `sudo` in front.

1. Install the binary. Download it from the latest release (use `arm64` instead of `amd64` on an ARM server):

   ``` sh
   VERSION=0.1.0
   curl -fsSL https://github.com/LeonardCooray/cloud-fetch/releases/download/v$VERSION/cloud-fetch_${VERSION}_linux_amd64.gz | gunzip > /usr/local/bin/cloud-fetch && chmod +x /usr/local/bin/cloud-fetch
   ```

   Or build it from source with a current Go:

   ``` sh
   curl -fsSL https://go.dev/dl/go1.25.4.linux-amd64.tar.gz | tar -C /usr/local -xz
   export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
   go install github.com/LeonardCooray/cloud-fetch@latest
   install -m 755 ~/go/bin/cloud-fetch /usr/local/bin/cloud-fetch
   ```

2. Create a user to run it, and keep the login in a file only root can read:

   ``` sh
   useradd --system --create-home --home-dir /srv/cloud-fetch --shell /usr/sbin/nologin cloudfetch
   install -m 600 /dev/null /etc/cloud-fetch.env && echo 'AUTH=user:password' > /etc/cloud-fetch.env
   ```

3. Create the service. `AmbientCapabilities` lets it bind ports 80 and 443 without running as root.

   ``` sh
   cat > /etc/systemd/system/cloud-fetch.service <<'EOF'
   [Unit]
   Description=Cloud Fetch
   After=network-online.target
   Wants=network-online.target

   [Service]
   User=cloudfetch
   WorkingDirectory=/srv/cloud-fetch
   EnvironmentFile=/etc/cloud-fetch.env
   ExecStart=/usr/local/bin/cloud-fetch --domain fetch.example.com --port 443 --acme-staging
   AmbientCapabilities=CAP_NET_BIND_SERVICE
   Restart=always

   [Install]
   WantedBy=multi-user.target
   EOF
   ```

   For plain HTTP, use `ExecStart=/usr/local/bin/cloud-fetch --port 3000` instead. Settings and downloads land in `/srv/cloud-fetch`, and certificates in `/srv/cloud-fetch/certs`.

4. Start it and watch the log:

   ``` sh
   systemctl daemon-reload && systemctl enable --now cloud-fetch
   journalctl -u cloud-fetch -f
   ```

   You should see `Enabled HTTP authentication` and a listen address of `0.0.0.0:443`. Open `https://fetch.example.com`; the staging certificate warning is expected.

5. Switch to the real certificate:

   ``` sh
   systemctl stop cloud-fetch && rm -rf /srv/cloud-fetch/certs
   sed -i 's/ --acme-staging//' /etc/systemd/system/cloud-fetch.service
   systemctl daemon-reload && systemctl start cloud-fetch
   ```

To upgrade, replace `/usr/local/bin/cloud-fetch` with the new binary (step 1) and run `systemctl restart cloud-fetch`.

### Usage

```
$ cloud-fetch --help

  Usage: cloud-fetch [options]

  Options:
  --title, -t          Title of this instance (default Cloud Fetch)
  --port, -p           Listening port (default 3000)
  --host, -h           Listening interface (default: all with --auth, localhost only without)
  --auth, -a           Optional basic auth in form 'user:password'
  --config-path, -c    Configuration file path (default cloud-fetch.json)
  --key-path, -k       TLS Key file path
  --cert-path          TLS Certicate file path
  --log, -l            Enable request logging
  --open, -o           Open now with your default browser
  --domain             Serve HTTPS with a free, auto-renewing Let's Encrypt certificate for this
                       domain (needs --http-port reachable from the internet)
  --http-port          Plain HTTP port for Let's Encrypt checks and the redirect to HTTPS (only
                       with --domain, default 80)
  --cert-cache         Where to keep Let's Encrypt certificates (default: certs/ next to the config
                       file)
  --acme-staging       Use Let's Encrypt's staging server (untrusted test certificates, generous
                       rate limits)
  --acme-directory     ACME directory URL, to use a certificate authority other than Let's Encrypt
  --search-config-url  URL of the search provider list, re-checked every 30 minutes; empty uses
                       only the built-in list (default
                       https://raw.githubusercontent.com/LeonardCooray/cloud-fetch/master/server/search-config.json)
  --version, -v        display version
  --help               display help
```

`TITLE`, `PORT`, `AUTH` and `DOMAIN` can also be set as environment variables (from v0.1.0; earlier builds ignored them).

### Security defaults

Without `--auth` the server only listens on localhost. To expose it, set `--auth user:password`, or pass `--host 0.0.0.0` if something in front of it already handles access.

`--auth` uses HTTP basic auth, which browsers, IDM and VLC all support (VLC also accepts `https://user:password@host/...` links). Over plain HTTP that password crosses the network unencrypted, so use HTTPS for anything reachable from the internet.

An address that gets the login wrong 5 times within 15 minutes is locked out for 15 minutes (HTTP 429, and a `Blocked` line in the log); IPv6 addresses are grouped by /64. Opening the page without credentials, which is how the browser shows its login prompt, doesn't count. Credentials that have already worked are never held back, so IDM's parallel connections are fine. The lockout keys on the connecting address, so behind a reverse proxy every client shares one address and five bad logins from anyone lock everyone out.

### HTTPS with Let's Encrypt

Point a DNS record at the server, open ports 80 and 443, then:

``` sh
cloud-fetch --domain fetch.example.com --port 443 --auth user:password
```

The first HTTPS request obtains the certificate. It's cached in `certs/` next to the config file and renewed in the background about 30 days before it expires, as long as the process is running and port 80 is reachable. Anything else arriving on port 80 is redirected to HTTPS.

Binding ports 80 and 443 needs root or the `CAP_NET_BIND_SERVICE` capability, for example `sudo setcap cap_net_bind_service=+ep ./cloud-fetch` or `AmbientCapabilities=CAP_NET_BIND_SERVICE` in a systemd unit.

Run once with `--acme-staging` first. Staging certificates aren't trusted by browsers, but a mistake there doesn't count against Let's Encrypt's rate limits. Delete `certs/` before switching to the real server.

### Search providers

The built-in provider list is [`server/search-config.json`](server/search-config.json). Running instances re-read it from this repo's `master` branch every 30 minutes, so a pushed edit reaches them without a new release. Set `--search-config-url ""` to use only the list compiled into the binary.

Every provider is searched live once a week, and on any pull request that changes the list, by `go test -count=1 -tags live -run TestLiveSearchProviders ./server`. A new provider needs a query in that test's `liveQueries`. A provider whose search gets a Cloudflare challenge is skipped rather than failed, since some sites challenge GitHub's runners but not home connections; skips are listed on the run's summary page.

### Upgrading from Cloud Torrent

On first start, an existing `cloud-torrent.json` next to the default config path is renamed to `cloud-fetch.json`. A custom `--config-path` is never touched.

### Development

The web UI is plain ES modules in `static/files/`, with Preact and htm vendored in `static/files/js/vendor/` (versions and checksums in `VENDOR.md`). There is no build step: edit a file, run `go build`, reload the page. The UI logic has unit tests that run on Node 22 or later:

``` sh
node --test "static/files/js/**/*.test.mjs"
```

Browser tests live in `e2e/` (Playwright, dev-only; nothing there is embedded or needed by `go install`). They build the binary and run it offline against temporary folders:

``` sh
cd e2e && npm ci && npx playwright install chromium
npx playwright test
```

### Credits

Cloud Fetch is based on [jpillora/cloud-torrent](https://github.com/jpillora/cloud-torrent), Copyright (c) 2017 Jaime Pillora, itself a rewrite of [node-torrent-cloud](https://github.com/jpillora/node-torrent-cloud). The torrent engine is [anacrolix/torrent](https://github.com/anacrolix/torrent).

Licensed under the [GNU Affero General Public License v3.0](LICENSE).
