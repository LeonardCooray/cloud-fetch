# UI polish and copy links: design

Date: 27 Sep 2026. Status: approved in conversation, awaiting written-spec review.

## Goal

Make the daily jobs faster and the page easier to read, on desktop and phone, without restructuring it. The main friction is getting file links into IDM and VLC; after that, reading each torrent's state at a glance, a visual refresh with dark mode, and a phone layout that is comfortable to tap.

Success means: from phone or desktop, any finished file's link is on the clipboard in one tap, a folder's links are on the clipboard in one tap, and every torrent's state (loading, downloading, seeding, done, paused) is readable without doing sums.

## Constraints

- UI only. No Go server, endpoint or JSON shape changes.
- The shipped UI stays build-free and npm-free (vendored Preact + htm, `go install` needs no other toolchain).
- Two sections stay: Torrents and Downloads. Merging them is out of scope.
- Links are plain `https://host/download/<path>` with no credentials in them. IDM's saved site login and VLC's one-time prompt handle `--auth`.
- Copying must work on plain HTTP: the VPS serves HTTP until the Let's Encrypt run, and `navigator.clipboard` is unavailable outside secure contexts.
- No VLC deep links; a copy button is enough.

## Approach

Polish in place. A CSS-only restyle was rejected because it fixes neither links nor status; merging Torrents and Downloads was rejected as the full redesign Leonard ruled out.

## New and changed files

All under `static/files/`.

```
js/lib/status.js           torrentStatus(t), eta(t) -> text           (new)
js/lib/status.test.mjs                                                  (new)
js/lib/clipboard.js        copyText(text, env) with fallback chain      (new)
js/lib/clipboard.test.mjs                                               (new)
js/lib/tree.js             + absoluteHref(path, base), finishedFiles(node, path, torrents)
js/lib/tree.test.mjs       + tests for the two helpers
js/lib/contrast.test.mjs   parses css/app.css, checks AA pairs in both themes (new)
js/components/TorrentList.js   badge, Pause/Resume toggle, ETA, hash moved
js/components/FileTable.js     infohash line at the top of the Files panel
js/components/DownloadTree.js  Copy link / Copy all, Copied state, bigger rows
js/components/CopyFallback.js  popover with the URL pre-selected (new)
js/components/SearchResults.js header row, labelled seeds/peers, stacked on phones
js/icons.js                + copy, pause
css/app.css                tokens, dark theme, badges, phone layout
index.html                 + theme-color meta (light and dark)
```

Repo root: `e2e/` (new, see Testing) and a new CI job.

## Torrent cards

**Status** comes from fields the server already sends, in this order:

| Status | Rule | Badge colour | Bar colour |
|---|---|---|---|
| Loading | `!Loaded` | neutral | accent |
| Seeding | `Started && Percent >= 100` | success | success |
| Downloading | `Started && Percent < 100` | accent | accent |
| Done | `!Started && Percent >= 100` | success | success |
| Paused | `!Started && Percent < 100` | warning | muted |

The loading overlay stays as it is.

**Actions:** Files, then one toggle: "Pause" (pause icon) while `Started`, "Resume" (play icon) while not. It calls the existing `start`/`stop` actions. Remove (or Cancel while loading) shows only while not started, as today. The disabled Start button is gone.

**Status line:**
- Downloading: `3.1 GB of 7.4 GB · 42% · 5.2 MB/s · about 14 min left`
- Downloading at 0 B/s: `… · 0 B/s · waiting for peers`
- Seeding and Done: `12 MB · complete` (`DownloadRate` is download-only, so a seeding torrent shows no rate)
- Paused: `1.3 GB of 2.1 GB · 63%`

`eta` is `(Size - Downloaded) / DownloadRate`, formatted as "less than a minute", "about N min", "about N h M min" (minutes dropped from 10 h up), or "more than a day". It returns null when the rate is 0 or the torrent isn't downloading.

**Infohash** leaves the card header and becomes the first line of the Files panel (`#<hash>`, mono, selectable).

## Download rows

- **Copy link** icon button on every finished file, placed before preview and delete. A file that is still downloading gets no copy button, the same rule that already hides its link.
- It copies `absoluteHref(path, location.href)`: `new URL(downloadHref(path), base).href`, so a subpath deployment and the current scheme and host carry through.
- **Copy all** icon button on folders. It copies every finished file under the folder, recursively, sorted as the tree shows them, one absolute URL per line (IDM's add-batch-from-clipboard format). A folder with no finished files shows no copy button.
- **Confirmation:** the button turns into a success tick labelled "Copied" (files) or "Copied N" (folders) for 2 s, announced via an `aria-live="polite"` region.
- **Clipboard chain** (`copyText`): `navigator.clipboard.writeText` when `window.isSecureContext`; otherwise, or if that rejects, a hidden textarea plus `document.execCommand("copy")`; if that returns false or throws, resolve to `"manual"` and the row opens `CopyFallback`: a small inline popover with the text pre-selected in a read-only textarea, a hint ("Press Ctrl+C or long-press to copy") and a close button. Esc and outside clicks close it.
- **Touch targets:** rows are at least 40px high and icon buttons 36×36px. The meta line (`size · updated …`) stays under the name.

## Visual system

- **Tokens:** keep the `:root` structure. Add `--ok-bg`, `--warn`, `--warn-bg`, `--overlay` (replaces the hardcoded `rgba(255,255,255,0.75)`). Darken `--faint` and `--ok` until they reach 4.5:1 on both `--surface` and `--bg`.
- **Dark mode:** a `@media (prefers-color-scheme: dark)` block redefines every token, with no manual toggle. Add `color-scheme: light dark` so form controls and scrollbars follow. `index.html` gets two `theme-color` metas (light and dark media) so the phone's browser bar matches.
- **Badges:** 12px text, tinted background, same-family darker text, radius as the other controls.
- **Width:** stays 880px.

## Phone layout (under 600px)

- Card buttons move to their own full-width row under the name and badge, sharing the width equally.
- Search results change from a table to stacked rows: name on top, then `size · N seeds · N peers` and the add button on the right.
- On desktop, the results table gets a header row (Name, Size, Seeds, Peers), and seeds/peers get labels on both layouts.
- A long single-word `--title` truncates with an ellipsis instead of overflowing the header.

## Error handling

- The copy chain never throws to the component; its outcomes are `"copied"` or `"manual"`. A failed copy always ends in the manual popover, never a silent no-op.
- Pause/Resume errors go to the existing error banner, as Start/Stop did.
- The status helpers tolerate missing fields (`Size` 0, `Files` null) and fall back to Loading or Paused, never NaN or "Infinity min".

## Testing

**Unit (`node --test`, already in CI):**
- `status.test.mjs`: every row of the status table, the ETA formatting boundaries, zero rate, zero size.
- `clipboard.test.mjs`: secure context uses `writeText`; insecure context skips it; `writeText` rejecting falls through to `execCommand`; `execCommand` false or throwing resolves `"manual"`. Browser APIs are passed in as `env`, so no DOM library is needed.
- `tree.test.mjs`: `absoluteHref` with a root base, a subpath base and names needing encoding; `finishedFiles` skips downloading files, recurses and keeps tree order.
- `contrast.test.mjs`: parses the light and dark token blocks from `css/app.css`, then checks AA (4.5:1) for text/muted/faint/accent/ok/danger/warn on surface and bg, plus badge text on badge background. Zero dependencies.

**End-to-end: `e2e/` in the repo**, Playwright on Chromium with its own pinned `package.json` and `package-lock.json`. It is dev-only: nothing under `e2e/` is embedded or needed by `go install`. It builds the binary, starts it against a temp download dir with a local fixture torrent (no internet), and covers:
- the shell loads with no console errors (carried over from the old harness);
- status badges and Pause/Resume across Downloading, Paused and Done (with no peers the rate stays 0, so e2e sees "waiting for peers"; ETA text is covered by the unit tests);
- Copy link writes the absolute URL (clipboard permission granted, read back via `navigator.clipboard.readText`);
- Copy all writes one URL per finished file;
- the HTTP fallback path (clipboard API stubbed away) and the manual popover (`execCommand` stubbed false);
- a phone-width run (375×812) of the cards, rows, search results and settings;
- a dark-mode run (`colorScheme: "dark"`) that checks the tokens apply.

**CI:** a new `e2e` job after `test`: Go and Node setup, `npm ci` in `e2e/`, `npx playwright install --with-deps chromium`, then the run. The existing test job is unchanged.

**Docs:** refresh `docs/screenshot.png` once it's built.

## Out of scope

- Merging Torrents and Downloads, filters, sorting, a wider desktop layout.
- Peer counts on cards (needs a server field).
- Credentials or signed tokens in links; VLC deep links.
- A manual dark-mode toggle.
- The other backlog items (velox bundle size, `findTorrentFile` performance, search-provider review).
