# Web UI rewrite: design

Date: 27 Sep 2026. Status: approved in conversation, awaiting written-spec review.

## Goal

Replace the AngularJS 1.3.15 + Semantic UI frontend with a maintained stack at **feature parity**. The page looks cleaner but gains no new workflows. The Go server, its endpoints and the single-binary embed are unchanged.

## Constraints

- `go build` and `go install github.com/LeonardCooray/cloud-fetch@latest` must keep producing a complete binary with no other toolchain. No build step, no npm packages in the repo.
- Modern evergreen browsers only (ES modules, import maps, `fetch`, CSS custom properties).
- Every third-party file is pinned, vendored and checksummed.
- Light theme only (dark mode is a possible follow-up, not in scope).

## Approach

Preact + htm as vendored ES modules, no build (option A of three considered; a Vite + TypeScript build with a committed bundle and a CI-only build were rejected for supply-chain size, diff noise, and breaking `go install` respectively).

## Architecture and file layout

All under `static/files/`, still embedded by `static/static.go`.

```
index.html                 shell: stylesheet, import map, /js/velox.js, js/main.js
css/app.css                all styles, CSS custom properties
js/vendor/preact.mjs       preact 10.29.8   (dist/preact.module.js)
js/vendor/hooks.mjs        preact 10.29.8   (hooks/dist/hooks.module.js)
js/vendor/htm.mjs          htm 3.1.1        (dist/htm.module.js)
js/vendor/VENDOR.md        version, source URL and SHA-256 per file
js/main.js                 mounts <App> into #app
js/html.js                 export const html = htm.bind(h)
js/state.js                velox wrapper + useSyncState() hook
js/icons.js                ~12 inline SVG icon components
js/lib/omni.js             classify input: search | magnet | torrent-url | empty
js/lib/magnet.js           parse / build magnet URIs
js/lib/format.js           bytes, ago, filename, addSpaces
js/lib/tree.js             node paths, preview kind, torrent-file lookup, isDownloading
js/lib/search.js           absolutise result URLs, resolve item lookups to magnet/torrent
js/lib/api.js              fetch wrappers for /api/*, /search/*, DELETE /download/*
js/components/App.js
js/components/Header.js
js/components/OmniBar.js       + MagnetEditor, SearchResults
js/components/TorrentList.js   + TorrentCard, FileTable
js/components/DownloadTree.js  + TreeNode, Preview
js/components/ConfigForm.js
js/components/Footer.js
js/components/ErrorBanner.js
```

The import map in `index.html` maps `preact` → `js/vendor/preact.mjs` and `preact/hooks` → `js/vendor/hooks.mjs` (hooks imports `preact` by bare specifier).

`js/lib/*` modules are pure (no DOM, no `window`), so Node can import and test them directly. `api.js` takes an injectable `fetch` for the same reason.

**Removed:** `js/vendor/angular.min.js`, `moment.min.js`, `query-string.js`, `js/semantic-checkbox.js`, the old controllers and `run.js`, `template/`, `css/semantic.min.css`, `css/themes/` (icon fonts, flag sprite), `css/Lato/`, `css/sections/`. `/js/velox.js` stays (served by Go from the velox package). Target page weight: under 60 KB, down from about 600 KB.

## State and data flow

- `state.js` calls `velox("/sync", obj)` once. `onupdate` bumps a version counter that `useSyncState()` subscribers read; `onchange(connected)` feeds a `connected` flag. Components receive plain objects: `Config`, `SearchProviders`, `Downloads`, `Torrents`, `Users`, `Stats`.
- Per-view state stays in its component: omni text and selected provider (persisted in `localStorage` under the existing keys `tcOmni` and `tcProvider`), open/closed torrent file tables and tree folders, delete-confirm timers, preview toggles, the config draft.
- A single `requests in flight` counter in `api.js` drives the header spinner.

## Behaviour (parity checklist)

**Header:** title (`Stats.Title`) linking to the repo; spinner while a request is in flight; toggles for the config form and the magnet editor; green/red connection dot.

**Omni bar:**
- `http(s)://…` → "Load torrent" → `POST /api/url` (body: the URL).
- `magnet:?…` → "Load magnet" → `POST /api/magnet`, plus "Edit" to open the magnet editor.
- Any other non-empty text → provider `<select>` (providers whose id doesn't end in `/item`) + "Search".
- Enter submits the current mode. Empty input closes the editor.
- Icon click opens a multi-file picker for `.torrent`; dropping files on the bar does the same. Each file is read as bytes and sent to `POST /api/torrentfile`. Non-`.torrent` files are rejected with an inline error.
- Stored provider that no longer exists falls back to the first listed provider.

**Magnet editor:** name, info hash and tracker fields; a trailing empty tracker field is always present; editing any field rebuilds the omni text. Info hash must match `^[A-Za-z0-9]+$`, otherwise an inline error.

**Search results:** `GET /search/<provider>?query=…&page=N` (page starts at 1). Rows: name linking to `url` (relative URLs prefixed with the provider's origin), size, seeds and peers, add button. Add uses `magnet` → `/api/magnet`, else `torrent` → `/api/url`, else `GET /search/<provider>/item?item=<path>` and uses the returned `torrent`, `magnet`, or `infohash` + comma-separated `tracker` list (only `http://`/`udp://` trackers kept) built into a magnet. "Load more" requests the next page. An empty page shows "No results" and hides "Load more". A new query resets results and page.

**Torrent cards** (sorted by name, then info hash):
- "Loading" overlay until `Loaded`.
- Buttons: Files (toggle), Start (disabled while `Started`), Stop (only while `Started`), Remove, or Cancel when not `Loaded` (only while not `Started`). Actions post `start|stop|delete:<infohash>` to `/api/torrent`.
- While `Started`: `Downloaded / Size · Percent% · DownloadRate/s`.
- Info hash shown in monospace; progress bar from `Percent`.
- File table sorted by `Path`: filename, per-file progress while `0 < Percent < 100`, a tick at 100, footer with file count and total size when more than one file.

**Downloads tree:**
- Section header shows free disk space (`Stats.System.diskTotal - diskUsed`) once `Stats.System.set`.
- Folders toggle open/closed; folders whose `Modified` is more than 24 h ago start closed.
- Names link to `download/<path>` (folders are served as zip by the server).
- A file belonging to a started, loaded torrent at under 100% shows a spinner and no link or delete.
- Delete: first click arms a confirm for 3 s, second click sends `DELETE /download/<path>` and shows a spinner until the tree updates.
- Preview toggle for `mp3|m4a` (audio), `jpe?g|png|gif` (image), `mp4|mkv|mov` (video, autoplay).
- Each entry shows `size · updated <ago>`.
- Space plays/pauses the first `<audio>`/`<video>` in view, unless an input, select or button has focus (a focused button already acts on Space).

**Config form:** fields from `state.Config`; booleans as checkboxes, numbers as number inputs, others as text; labels are the key names with spaces before capitals. The form edits a draft copy taken when it opens. Save posts the JSON to `/api/configure` and closes on success; Cancel discards the draft.

**Footer:** repo link, `Stats.Version`, "(fork of jpillora/cloud-torrent, AGPL-3.0)", "N users connected" when more than one, and `Go <Stats.Runtime>`.

## Deliberate changes (fixes, not features)

1. Magnet names keep spaces and symbols (`dn` is `encodeURIComponent`-encoded instead of stripped of non-word characters).
2. Missing stored provider falls back to the first provider (the old `tpb` default no longer exists).
3. Torrent cards sort by name (the old order was by info hash).
4. The config form edits a draft, so live updates can't overwrite typing and Cancel really cancels.
5. Request errors show in the in-page error banner instead of `alert()`.
6. The dead "zip ready" link on torrent names is removed (`state.Uploads` is never sent).
7. Links taken from scraped search results are only rendered when they are `http(s)://`, and only `magnet:` / `http(s)://` values are sent to the server. AngularJS sanitised `javascript:` URLs; Preact does not, and a hostile provider page must not run script in an authenticated session.

## Look

Same single page and section order: header, omni bar (+ editor, results), torrents, downloads, footer. System font stack, one blue accent, green for start, red for stop/delete, 1 px hairline borders, 8 px radius, monospace info hashes. Under 600 px wide, torrent buttons wrap below the name. Icons are inline SVG; no icon font.

## Error handling

- `api.js` turns any non-2xx response into an `Error` carrying the response body text, which the server already sets to a readable message.
- One dismissable error banner under the omni bar shows the latest request error.
- Input validation errors (bad info hash, non-`.torrent` file) show inline next to the input.
- When velox reports disconnected: the header dot turns red and a thin "Reconnecting…" strip shows until it reconnects; the last known state stays on screen.
- A failed delete restores the row and shows the banner.

## Testing

1. **Unit tests** (`static/files/js/lib/*.test.mjs`, run with `node --test static/files/js/lib/`): omni classification; magnet parse/build round trip including spaces, symbols and multiple trackers; bytes formatting boundaries; tree path building, preview kind and in-progress detection; search URL absolutising and item resolution (torrent, magnet, infohash + tracker filtering); `api.js` error mapping with a stub `fetch`. Added to CI as a step using `actions/setup-node` with the Node version pinned in the workflow. The test files are embedded too, which is harmless; if that matters later, move them to `web/test/`.
2. **Go test:** `/` serves the new shell (contains the import map and `js/main.js`), `/js/main.js` and `/js/vendor/preact.mjs` are served, and the removed files 404.
3. **End-to-end** (sandbox only, not CI): Playwright + headless Chromium against the real binary with a temp download directory, a locally generated `.torrent` whose data already exists on disk, and a local fake search provider via `--search-config-url`. Walks the parity checklist: add by file drop and by URL, magnet editor, search + load more + add, start/stop/remove, file table, tree link and zip, two-step delete, preview toggle and Space shortcut, config save and cancel, error banner, and the reconnecting strip by restarting the server. Screenshots at 1280 px and 390 px wide for visual review.

## Out of scope

Dark mode, copy-link buttons for IDM/VLC, filters, per-file start/stop (the engine doesn't support stopping files), and any server changes beyond the Go test above.
