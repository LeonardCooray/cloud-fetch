![Cloud Fetch web UI showing a seeding torrent with its files open, a finished paused torrent, one still downloading, and the downloads folder with copy-link buttons](docs/screenshot.png)

**Cloud Fetch** is a self-hosted remote torrent client written in Go. You add torrents from the browser, they download to the server's disk, and you fetch or stream the files over HTTP(S) with a browser, a download manager such as IDM, or a player such as VLC.

### Features

* Single binary, cross platform
* Torrents, their metadata and their started/stopped state survive restarts
* Files already on disk are hash-checked when you add a torrent, so wrong or partial data is downloaded again
* Optional HTTPS with a free, auto-renewing Let's Encrypt certificate
* Resumable, seekable downloads (HTTP Range) for download managers and media players
* Embedded torrent search, with the provider list kept in this repo
* Real-time updates in a mobile-friendly UI

### Install

**From source** (needs [Go](https://go.dev/dl/) 1.25 or later)

``` sh
go install github.com/LeonardCooray/cloud-fetch@latest
```

**Binaries and Docker**

Tagging a `v*` release builds binaries on the [releases page](https://github.com/LeonardCooray/cloud-fetch/releases) and a multi-arch image at `ghcr.io/leonardcooray/cloud-fetch`.

``` sh
docker run -d --restart always -p 3000:3000 \
  -v /srv/cloud-fetch/downloads:/app/downloads \
  -v /srv/cloud-fetch/config:/config \
  ghcr.io/leonardcooray/cloud-fetch \
  --host 0.0.0.0 --auth user:password --config-path /config/cloud-fetch.json
```

The container's working directory is `/app`, so downloads land in `/app/downloads` by default. Keep the config on a volume too, or settings and Let's Encrypt certificates are lost when the container is replaced.

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

`TITLE`, `PORT`, `AUTH` and `DOMAIN` can also be set as environment variables.

### Security defaults

Without `--auth` the server only listens on localhost. To expose it, set `--auth user:password`, or pass `--host 0.0.0.0` if something in front of it already handles access.

`--auth` uses HTTP basic auth, which browsers, IDM and VLC all support (VLC also accepts `https://user:password@host/...` links). Over plain HTTP that password crosses the network unencrypted, so use HTTPS for anything reachable from the internet.

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
