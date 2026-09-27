# Web UI Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the AngularJS 1.3.15 + Semantic UI frontend in `static/files/` with a Preact + htm frontend at feature parity, with no build step.

**Architecture:** Plain ES modules embedded in the Go binary as today. Three vendored library files are wired by an import map. Pure logic lives in `js/lib/` and is unit-tested with `node --test`. Preact components in `js/components/` render the velox live state and call the unchanged `/api/*`, `/search/*` and `/download/*` endpoints. End-to-end behaviour is checked with Playwright + headless Chromium in the sandbox, outside the repo.

**Tech Stack:** Preact 10.29.8, htm 3.1.1, velox client (served by Go at `/js/velox.js`), Node 22+ built-in test runner, Playwright 1.63.0 (sandbox only), Go 1.25.4.

**Spec:** `docs/superpowers/specs/2026-09-27-web-ui-rewrite-design.md`

## Global Constraints

- No build step: `go build` and `go install github.com/LeonardCooray/cloud-fetch@latest` must produce a complete binary with no other toolchain.
- No npm packages in the repo. The Playwright harness lives at `/sessions/zen-kind-bohr/e2e`, outside the repo.
- Vendored exactly: preact 10.29.8 (`dist/preact.module.js`, `hooks/dist/hooks.module.js`) and htm 3.1.1 (`dist/htm.module.js`), each checksummed in `static/files/js/vendor/VENDOR.md`.
- Modern evergreen browsers only: ES modules, import maps, `fetch`, CSS custom properties.
- Server endpoints and behaviour unchanged. Server code changes are limited to Go tests.
- `localStorage` keys stay `tcOmni` and `tcProvider`.
- Look: system font stack, one blue accent, green for start, red for stop/delete, 1 px borders, 8 px radius, monospace info hashes, light theme only.
- UI copy is sentence case.
- Only `http(s)://` links from search results are rendered; only `magnet:` or `http(s)://` values are sent to the server.
- Leonard's repo rules: **never commit**. Each task ends in a checkpoint (tests green, diff reviewed) and leaves changes uncommitted. Use `git --no-optional-locks` for every read-only git command from the sandbox.
- Sandbox: run `. ~/goenv.sh` before Go commands. If `df -h /` shows under 500 MB free, run `go clean -cache` first.
- Unit test command (used in every task): `node --test "static/files/js/**/*.test.mjs"` from the repo root (`/sessions/zen-kind-bohr/mnt/cloud-fetch`).

## Review Focus

1. **Hostile links in scraped search results** (`javascript:`, `data:`, protocol-relative `//host`): a person expects clicking a result to never run script in their logged-in session. Pinned by `safeHttpUrl` / `normalizeResults` tests in Task 5 and the `evil` query e2e test in Task 8.
2. **File and folder names with spaces, `#`, `?`, `%` or non-ASCII characters**: a person expects the download link, IDM and VLC to fetch that exact file. Pinned by `downloadHref` tests in Task 4 and the `notes #1 ?.txt` e2e test in Task 10.
3. **Magnet links in the wild**: 32-character base32 hashes, uppercase hex, `+` for spaces in `dn`, repeated `tr`, no `dn`. A person expects them to load and to round-trip through the editor unchanged. Pinned by `parseMagnet` tests in Task 3.
4. **Very long names without spaces on a phone**: a person expects wrapping, not a sideways-scrolling page. Pinned by the 390 px layout e2e test in Task 12.
5. **Space pressed while a button or field has focus**: a person expects only that control to act, not a video to start or stop as well. Pinned by the Space e2e test in Task 10.

---

## File Structure

```
static/files/index.html              shell: stylesheet, import map, velox, main.js      (rewrite)
static/files/css/app.css             all styles                                          (rewrite)
static/files/js/vendor/preact.mjs    vendored preact 10.29.8                             (new)
static/files/js/vendor/hooks.mjs     vendored preact/hooks 10.29.8                       (new)
static/files/js/vendor/htm.mjs       vendored htm 3.1.1                                  (new)
static/files/js/vendor/VENDOR.md     versions, sources, SHA-256                          (new)
static/files/js/vendor/vendor.test.mjs  checksum test                                    (new)
static/files/js/html.js              htm bound to preact's h                             (new)
static/files/js/state.js             velox wrapper + useSync hook                        (new)
static/files/js/icons.js             inline SVG Icon component                           (new)
static/files/js/keys.js              Space play/pause shortcut                           (new)
static/files/js/main.js              entry: start sync, mount App                        (new)
static/files/js/lib/format.js        bytes, ago, hoursSince, filename, addSpaces, inputType
static/files/js/lib/omni.js          classify
static/files/js/lib/magnet.js        parseMagnet, buildMagnet, validInfohash
static/files/js/lib/tree.js          paths, hrefs, preview kinds, torrent lookup, sorting
static/files/js/lib/search.js        safe URLs, provider list, result/lookup resolution
static/files/js/lib/api.js           createApi (fetch wrappers + busy tracking), errorMessage
static/files/js/lib/*.test.mjs       unit tests, one per module
static/files/js/components/App.js, Header.js, Footer.js, ErrorBanner.js,
  OmniBar.js, MagnetEditor.js, SearchResults.js, TorrentList.js, FileTable.js,
  DownloadTree.js, ConfigForm.js
server/ui_test.go                    shell served, modules served as JS, old files 404  (new)
.github/workflows/ci.yml             add Node + unit test step                           (modify)
README.md                            add a short Development section                     (modify)
Removed: static/files/js/{run.js,utils.js,*-controller.js,semantic-checkbox.js},
  static/files/js/vendor/{angular.min.js,moment.min.js,query-string.js},
  static/files/template/, static/files/css/{semantic.min.css,themes/,Lato/,sections/}
Kept: static/files/cloud-favicon.png
```

Outside the repo (sandbox only): `/sessions/zen-kind-bohr/e2e/{package.json,playwright.config.mjs,env.sh,fixtures.mjs,mktorrent.mjs,fake-provider.mjs,tests/*.spec.mjs,shots/}`.

---

### Task 1: Vendor Preact and htm with checksums

**Files:**
- Create: `static/files/js/vendor/preact.mjs`, `hooks.mjs`, `htm.mjs`, `VENDOR.md`, `vendor.test.mjs`
- Create: `static/files/js/html.js`

**Interfaces:**
- Produces: import-map specifiers `preact` and `preact/hooks` (wired in Task 7); `export const html` from `js/html.js`, used by every component.

- [ ] **Step 1: Write the failing test** at `static/files/js/vendor/vendor.test.mjs`

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const dir = dirname(fileURLToPath(import.meta.url));
const rows = readFileSync(join(dir, "VENDOR.md"), "utf8")
  .split("\n")
  .map((l) => /^\|\s*`([^`]+)`\s*\|.*\|\s*([0-9a-f]{64})\s*\|\s*$/.exec(l))
  .filter(Boolean);

test("VENDOR.md lists exactly the three vendored files", () => {
  assert.deepEqual(rows.map((r) => r[1]).sort(), ["hooks.mjs", "htm.mjs", "preact.mjs"]);
});

test("every vendored file matches its recorded SHA-256", () => {
  for (const [, file, sum] of rows) {
    const got = createHash("sha256").update(readFileSync(join(dir, file))).digest("hex");
    assert.equal(got, sum, `${file} changed without updating VENDOR.md`);
  }
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `ENOENT ... VENDOR.md`.

- [ ] **Step 3: Fetch the pinned packages into a scratch dir and copy the three files**

```bash
rm -rf /tmp/vendor && mkdir -p /tmp/vendor && cd /tmp/vendor
npm pack preact@10.29.8 htm@3.1.1 --silent
mkdir preact htm
tar xzf preact-10.29.8.tgz -C preact && tar xzf htm-3.1.1.tgz -C htm
V=/sessions/zen-kind-bohr/mnt/cloud-fetch/static/files/js/vendor
cp preact/package/dist/preact.module.js      $V/preact.mjs
cp preact/package/hooks/dist/hooks.module.js $V/hooks.mjs
cp htm/package/dist/htm.module.js            $V/htm.mjs
grep -o 'from"[^"]*"' $V/hooks.mjs | sort -u   # expect exactly: from"preact"
grep -c 'from"' $V/preact.mjs $V/htm.mjs       # expect 0 for both (no imports)
```

`npm pack` verifies each tarball against the registry's integrity hash. If `hooks.mjs` imports anything other than `preact`, stop and report it.

- [ ] **Step 4: Write `VENDOR.md` with the computed checksums**

```bash
cd /sessions/zen-kind-bohr/mnt/cloud-fetch/static/files/js/vendor
{
  echo "# Vendored browser libraries"
  echo
  echo "Pinned copies, loaded through the import map in \`index.html\`. To update one: run \`npm pack <package>@<version>\` in a scratch directory, copy the file listed under Source over the vendored file, then update its version and SHA-256 here. \`vendor.test.mjs\` fails until the checksum matches."
  echo
  echo "| File | Package | Version | Source (inside the npm tarball) | SHA-256 |"
  echo "|---|---|---|---|---|"
  echo "| \`preact.mjs\` | preact | 10.29.8 | package/dist/preact.module.js | $(sha256sum preact.mjs | cut -d' ' -f1) |"
  echo "| \`hooks.mjs\` | preact | 10.29.8 | package/hooks/dist/hooks.module.js | $(sha256sum hooks.mjs | cut -d' ' -f1) |"
  echo "| \`htm.mjs\` | htm | 3.1.1 | package/dist/htm.module.js | $(sha256sum htm.mjs | cut -d' ' -f1) |"
} > VENDOR.md
cat VENDOR.md
```

- [ ] **Step 5: Create `static/files/js/html.js`**

```js
import { h } from "preact";
import htm from "./vendor/htm.mjs";

export const html = htm.bind(h);
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: 2 tests pass.

- [ ] **Step 7: Mutation check.** Append one byte to `htm.mjs` (`printf ' ' >> static/files/js/vendor/htm.mjs`), rerun, and confirm "every vendored file matches" fails. Then restore it with `cp /tmp/vendor/htm/package/dist/htm.module.js static/files/js/vendor/htm.mjs` and confirm the tests are green again.

- [ ] **Step 8: Checkpoint.** Leave uncommitted.

---

### Task 2: `lib/format.js`

**Files:**
- Create: `static/files/js/lib/format.js`
- Test: `static/files/js/lib/format.test.mjs`

**Interfaces:**
- Produces: `bytes(n: number): string`, `ago(t: string|Date, now?: number): string`, `hoursSince(t, now?): number`, `filename(path: string): string`, `addSpaces(key: string): string`, `inputType(value): "checkbox"|"number"|"text"`.

- [ ] **Step 1: Write the failing test** at `static/files/js/lib/format.test.mjs`

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { bytes, ago, hoursSince, filename, addSpaces, inputType } from "./format.js";

test("bytes uses metric units with at most one decimal", () => {
  assert.equal(bytes(0), "0 B");
  assert.equal(bytes(999), "999 B");
  assert.equal(bytes(1000), "1 KB");
  assert.equal(bytes(300000), "300 KB");
  assert.equal(bytes(1536000), "1.5 MB");
  assert.equal(bytes(631000000), "631 MB");
  assert.equal(bytes(5.94e9), "5.9 GB");
  assert.equal(bytes(2e15), "2 PB");
});

test("bytes treats missing or invalid sizes as zero", () => {
  for (const v of [undefined, null, NaN, -5, "12", Infinity]) assert.equal(bytes(v), "0 B");
});

const now = Date.parse("2026-09-27T12:00:00Z");
const secondsAgo = (s) => new Date(now - s * 1000).toISOString();

test("ago reads like moment's fromNow", () => {
  assert.equal(ago(secondsAgo(10), now), "just now");
  assert.equal(ago(secondsAgo(60), now), "a minute ago");
  assert.equal(ago(secondsAgo(90), now), "2 minutes ago");
  assert.equal(ago(secondsAgo(3600), now), "an hour ago");
  assert.equal(ago(secondsAgo(5 * 3600), now), "5 hours ago");
  assert.equal(ago(secondsAgo(30 * 3600), now), "a day ago");
  assert.equal(ago(secondsAgo(3 * 86400), now), "3 days ago");
  assert.equal(ago(secondsAgo(45 * 86400), now), "2 months ago");
  assert.equal(ago(secondsAgo(400 * 86400), now), "a year ago");
});

test("ago copes with clock skew and bad input", () => {
  assert.equal(ago(secondsAgo(-120), now), "just now");
  assert.equal(ago("not a date", now), "");
});

test("hoursSince", () => {
  assert.equal(hoursSince(secondsAgo(25 * 3600), now), 25);
});

test("filename returns the last path segment", () => {
  assert.equal(filename("Show/S01/e1.mkv"), "e1.mkv");
  assert.equal(filename("e1.mkv"), "e1.mkv");
});

test("addSpaces splits Go field names into words", () => {
  assert.equal(addSpaces("DownloadDirectory"), "Download Directory");
  assert.equal(addSpaces("EnableUpload"), "Enable Upload");
  assert.equal(addSpaces("AutoStart"), "Auto Start");
  assert.equal(addSpaces(5), 5);
});

test("inputType picks the form control for a config value", () => {
  assert.equal(inputType(true), "checkbox");
  assert.equal(inputType(50007), "number");
  assert.equal(inputType("/downloads"), "text");
  assert.equal(inputType(null), "text");
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `Cannot find module .../lib/format.js`.

- [ ] **Step 3: Implement** `static/files/js/lib/format.js`

```js
const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

export function bytes(n) {
  if (typeof n !== "number" || !Number.isFinite(n) || n <= 0) return "0 B";
  const i = Math.min(Math.floor(Math.log10(n) / 3), UNITS.length - 1);
  const scaled = Math.round((n / 10 ** (i * 3)) * 10) / 10;
  return `${scaled} ${UNITS[i]}`;
}

export function hoursSince(t, now = Date.now()) {
  return (now - new Date(t).getTime()) / 3600000;
}

export function ago(t, now = Date.now()) {
  const ms = now - new Date(t).getTime();
  if (!Number.isFinite(ms)) return "";
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 45) return m <= 1 ? "a minute ago" : `${m} minutes ago`;
  const h = Math.round(m / 60);
  if (h < 22) return h <= 1 ? "an hour ago" : `${h} hours ago`;
  const d = Math.round(h / 24);
  if (d < 26) return d <= 1 ? "a day ago" : `${d} days ago`;
  const mo = Math.round(d / 30);
  if (mo < 11) return mo <= 1 ? "a month ago" : `${mo} months ago`;
  const y = Math.round(d / 365);
  return y <= 1 ? "a year ago" : `${y} years ago`;
}

export function filename(path) {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
}

export function addSpaces(key) {
  if (typeof key !== "string") return key;
  return key.replace(/([A-Z]+[a-z]*)/g, " $1").trim();
}

export function inputType(value) {
  if (typeof value === "boolean") return "checkbox";
  if (typeof value === "number") return "number";
  return "text";
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: all pass. If Node prints a warning about module type detection for `.js` files, record it in the task report (Node 22.7+ and 24 detect ESM syntax without a `package.json`).

- [ ] **Step 5: Checkpoint.** Leave uncommitted.

---

### Task 3: `lib/omni.js` and `lib/magnet.js`

**Files:**
- Create: `static/files/js/lib/omni.js`, `static/files/js/lib/magnet.js`
- Test: `static/files/js/lib/omni.test.mjs`, `static/files/js/lib/magnet.test.mjs`

**Interfaces:**
- Produces: `classify(input: string): "empty"|"torrent-url"|"magnet"|"search"`; `parseMagnet(uri): { name: string, infohash: string, trackers: string[], error?: string }` (fields are always present, even with `error`, so the editor can show them); `buildMagnet({ name, infohash, trackers }): string`; `validInfohash(h): boolean`.

- [ ] **Step 1: Write the failing tests**

`static/files/js/lib/omni.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { classify } from "./omni.js";

test("classify decides what the omni bar will do", () => {
  assert.equal(classify(""), "empty");
  assert.equal(classify("   "), "empty");
  assert.equal(classify(undefined), "empty");
  assert.equal(classify("https://site.example/a.torrent"), "torrent-url");
  assert.equal(classify("HTTP://SITE.EXAMPLE/A"), "torrent-url");
  assert.equal(classify("magnet:?xt=urn:btih:abc"), "magnet");
  assert.equal(classify("ubuntu 24.04"), "search");
  assert.equal(classify("magnet"), "search");
});
```

`static/files/js/lib/magnet.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { parseMagnet, buildMagnet, validInfohash } from "./magnet.js";

test("parseMagnet reads name, hash and every tracker", () => {
  const m = parseMagnet(
    "magnet:?xt=urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&dn=Big+Buck+Bunny&tr=udp%3A%2F%2Ftracker.a%3A80&tr=wss%3A%2F%2Ftracker.b",
  );
  assert.deepEqual(m, {
    name: "Big Buck Bunny",
    infohash: "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C",
    trackers: ["udp://tracker.a:80", "wss://tracker.b"],
  });
});

test("parseMagnet accepts base32 hashes and a missing name", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ"), {
    name: "",
    infohash: "3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ",
    trackers: [],
  });
});

test("parseMagnet keeps the fields when the hash is invalid", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:zz!&dn=x"), {
    name: "x",
    infohash: "zz!",
    trackers: [],
    error: "Invalid info hash",
  });
  assert.equal(parseMagnet("magnet:?dn=x").error, "Invalid info hash");
  assert.equal(parseMagnet("magnet:?dn=x").infohash, "");
});

test("buildMagnet keeps spaces and symbols in the name", () => {
  assert.equal(
    buildMagnet({ name: "Big Buck Bunny (2008) & co #1", infohash: "abc", trackers: ["udp://t:80", ""] }),
    "magnet:?xt=urn:btih:abc&dn=Big%20Buck%20Bunny%20(2008)%20%26%20co%20%231&tr=udp%3A%2F%2Ft%3A80",
  );
  assert.equal(buildMagnet({ name: "", infohash: "abc", trackers: [] }), "magnet:?xt=urn:btih:abc");
});

test("build then parse round-trips what a person typed", () => {
  const fields = { name: "Café + Crème 100% [x]", infohash: "abcdef0123", trackers: ["udp://a:1", "http://b/announce?k=1&z=2"] };
  assert.deepEqual(parseMagnet(buildMagnet(fields)), fields);
});

test("validInfohash", () => {
  assert.equal(validInfohash("abcDEF123"), true);
  assert.equal(validInfohash(""), false);
  assert.equal(validInfohash("ab c"), false);
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `Cannot find module .../lib/omni.js` and `.../lib/magnet.js`.

- [ ] **Step 3: Implement**

`static/files/js/lib/omni.js`:

```js
export function classify(input) {
  const s = (input || "").trim();
  if (!s) return "empty";
  if (/^https?:\/\//i.test(s)) return "torrent-url";
  if (/^magnet:\?/i.test(s)) return "magnet";
  return "search";
}
```

`static/files/js/lib/magnet.js`:

```js
export function validInfohash(h) {
  return /^[A-Za-z0-9]+$/.test(h || "");
}

// Fields are returned even when the hash is invalid, so the editor can still
// show them while the person fixes the hash.
export function parseMagnet(uri) {
  const m = /^magnet:\?(.*)$/i.exec((uri || "").trim());
  const params = new URLSearchParams(m ? m[1] : "");
  const xt = params.get("xt") || "";
  const infohash = /^urn:btih:/i.test(xt) ? xt.slice("urn:btih:".length) : "";
  const out = { name: params.get("dn") || "", infohash, trackers: params.getAll("tr") };
  if (!validInfohash(infohash)) out.error = "Invalid info hash";
  return out;
}

export function buildMagnet({ name = "", infohash = "", trackers = [] }) {
  let s = "magnet:?xt=urn:btih:" + infohash;
  if (name) s += "&dn=" + encodeURIComponent(name);
  for (const t of trackers) if (t) s += "&tr=" + encodeURIComponent(t);
  return s;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: all pass.

- [ ] **Step 5: Checkpoint.** Leave uncommitted.

---

### Task 4: `lib/tree.js`

**Files:**
- Create: `static/files/js/lib/tree.js`
- Test: `static/files/js/lib/tree.test.mjs`

**Interfaces:**
- Consumes: `hoursSince` from `lib/format.js`.
- Produces:
  - `childPath(parent: string, name: string): string`
  - `downloadHref(path: string): string` (percent-encodes each segment)
  - `isDir(node): boolean`
  - `previewKind(path): "audio"|"image"|"video"|null`
  - `fileIcon(path): "music"|"image"|"film"|"file"`
  - `findTorrentFile(torrents, path): { torrent, file } | null`
  - `isDownloading(match): boolean`
  - `startsClosed(node, now?): boolean`
  - `sortTorrents(torrents): Torrent[]`

  Node shape from the server is `{ Name, Size, Modified, Children: Node[] | null }`. Torrent shape is `{ InfoHash, Name, Loaded, Started, Percent, Files: [{ Path, Size, Percent }] | null, ... }`.

- [ ] **Step 1: Write the failing test** at `static/files/js/lib/tree.test.mjs`

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  childPath, downloadHref, isDir, previewKind, fileIcon,
  findTorrentFile, isDownloading, startsClosed, sortTorrents,
} from "./tree.js";

test("childPath joins relative to the download root", () => {
  assert.equal(childPath("", "a"), "a");
  assert.equal(childPath("Show", "S01"), "Show/S01");
});

test("downloadHref encodes every segment so IDM and VLC fetch the exact file", () => {
  assert.equal(downloadHref("Show/S01/e 1 #2?.mkv"), "download/Show/S01/e%201%20%232%3F.mkv");
  assert.equal(downloadHref("Café/ü 100%.mp3"), "download/Caf%C3%A9/%C3%BC%20100%25.mp3");
});

test("isDir follows the server's Children field", () => {
  assert.equal(isDir({ Children: [] }), true);
  assert.equal(isDir({ Children: null }), false);
  assert.equal(isDir({}), false);
});

test("previewKind matches extensions case-insensitively", () => {
  assert.equal(previewKind("a/song.MP3"), "audio");
  assert.equal(previewKind("song.m4a"), "audio");
  for (const p of ["x.jpg", "x.JPEG", "x.png", "x.gif"]) assert.equal(previewKind(p), "image");
  for (const p of ["x.mp4", "x.MKV", "x.mov"]) assert.equal(previewKind(p), "video");
  assert.equal(previewKind("x.avi"), null);
  assert.equal(previewKind("notes.txt"), null);
});

test("fileIcon", () => {
  assert.equal(fileIcon("a.mp3"), "music");
  assert.equal(fileIcon("a.png"), "image");
  assert.equal(fileIcon("a.mkv"), "film");
  assert.equal(fileIcon("a.avi"), "film");
  assert.equal(fileIcon("a.txt"), "file");
});

const t1 = { InfoHash: "a", Name: "Show", Loaded: true, Started: true, Files: [{ Path: "Show/e1.mkv", Percent: 50 }, { Path: "Show/e2.mkv", Percent: 100 }] };
const t2 = { InfoHash: "b", Name: "Other", Loaded: false, Started: true, Files: null };

test("findTorrentFile finds the torrent a downloaded file belongs to", () => {
  assert.deepEqual(findTorrentFile({ a: t1, b: t2 }, "Show/e2.mkv"), { torrent: t1, file: t1.Files[1] });
  assert.equal(findTorrentFile({ a: t1, b: t2 }, "Show/e3.mkv"), null);
  assert.equal(findTorrentFile(null, "Show/e1.mkv"), null);
});

test("isDownloading only for loaded, started, unfinished files", () => {
  assert.equal(isDownloading({ torrent: t1, file: t1.Files[0] }), true);
  assert.equal(isDownloading({ torrent: t1, file: t1.Files[1] }), false);
  assert.equal(isDownloading({ torrent: { ...t1, Started: false }, file: t1.Files[0] }), false);
  assert.equal(isDownloading({ torrent: { ...t1, Loaded: false }, file: t1.Files[0] }), false);
  assert.equal(isDownloading(null), false);
});

test("folders untouched for more than a day start closed", () => {
  const now = Date.parse("2026-09-27T12:00:00Z");
  assert.equal(startsClosed({ Modified: new Date(now - 25 * 3600e3).toISOString() }, now), true);
  assert.equal(startsClosed({ Modified: new Date(now - 2 * 3600e3).toISOString() }, now), false);
});

test("sortTorrents orders by name, falling back to the hash", () => {
  const list = sortTorrents({
    b: { InfoHash: "b", Name: "zeta" },
    a: { InfoHash: "a", Name: "Alpha" },
    c: { InfoHash: "c", Name: "" },
  });
  assert.deepEqual(list.map((t) => t.InfoHash), ["a", "c", "b"]);
  assert.deepEqual(sortTorrents(null), []);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `Cannot find module .../lib/tree.js`.

- [ ] **Step 3: Implement** `static/files/js/lib/tree.js`

```js
import { hoursSince } from "./format.js";

export function childPath(parent, name) {
  return parent ? `${parent}/${name}` : name;
}

export function downloadHref(path) {
  return "download/" + path.split("/").map(encodeURIComponent).join("/");
}

export function isDir(node) {
  return Array.isArray(node && node.Children);
}

export function previewKind(path) {
  if (/\.(mp3|m4a)$/i.test(path)) return "audio";
  if (/\.(jpe?g|png|gif)$/i.test(path)) return "image";
  if (/\.(mp4|mkv|mov)$/i.test(path)) return "video";
  return null;
}

export function fileIcon(path) {
  const kind = previewKind(path);
  if (kind === "audio") return "music";
  if (kind === "image") return "image";
  if (kind === "video" || /\.avi$/i.test(path)) return "film";
  return "file";
}

export function findTorrentFile(torrents, path) {
  for (const torrent of Object.values(torrents || {})) {
    const file = (torrent.Files || []).find((f) => f && f.Path === path);
    if (file) return { torrent, file };
  }
  return null;
}

export function isDownloading(match) {
  return Boolean(match && match.torrent.Loaded && match.torrent.Started && match.file.Percent < 100);
}

export function startsClosed(node, now = Date.now()) {
  return hoursSince(node.Modified, now) > 24;
}

export function sortTorrents(torrents) {
  const key = (t) => t.Name || t.InfoHash;
  return Object.values(torrents || {}).sort(
    (a, b) => key(a).localeCompare(key(b), undefined, { sensitivity: "base" }) || a.InfoHash.localeCompare(b.InfoHash),
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: all pass.

- [ ] **Step 5: Checkpoint.** Leave uncommitted.

---

### Task 5: `lib/search.js`

**Files:**
- Create: `static/files/js/lib/search.js`
- Test: `static/files/js/lib/search.test.mjs`

**Interfaces:**
- Consumes: `buildMagnet` from `lib/magnet.js`.
- Produces:
  - `safeHttpUrl(u): string` (the URL, or `""` unless it's `http(s)://`)
  - `providerList(searchProviders): { id, name }[]` (excludes `*/item`)
  - `pickProvider(list, stored): string`
  - `normalizeResults(results, providerUrl): Result[]` (absolute, safe `url`, `torrent` and `magnet`, plus `path` for relative URLs)
  - `resolveItem(result): { kind: "magnet"|"url", value } | { kind: "lookup", path } | { error }`
  - `resolveLookup(data, name): { kind: "magnet"|"url", value } | { error }`

- [ ] **Step 1: Write the failing test** at `static/files/js/lib/search.test.mjs`

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { safeHttpUrl, providerList, pickProvider, normalizeResults, resolveItem, resolveLookup } from "./search.js";

test("safeHttpUrl only lets http(s) through", () => {
  assert.equal(safeHttpUrl("https://x.example/a"), "https://x.example/a");
  assert.equal(safeHttpUrl("HTTP://x.example"), "HTTP://x.example");
  for (const bad of ["javascript:alert(1)", " javascript:alert(1)", "data:text/html,x", "//evil.example", "", undefined]) {
    assert.equal(safeHttpUrl(bad), "");
  }
});

test("providerList hides item endpoints and keeps order", () => {
  const list = providerList({ nyaa: { name: "Nyaa" }, "nyaa/item": { name: "Nyaa (Item)" }, abb: { name: "ABB" } });
  assert.deepEqual(list, [{ id: "nyaa", name: "Nyaa" }, { id: "abb", name: "ABB" }]);
  assert.deepEqual(providerList(null), []);
});

test("pickProvider falls back to the first provider", () => {
  const list = [{ id: "nyaa", name: "Nyaa" }, { id: "abb", name: "ABB" }];
  assert.equal(pickProvider(list, "abb"), "abb");
  assert.equal(pickProvider(list, "tpb"), "nyaa");
  assert.equal(pickProvider([], "tpb"), "");
});

test("normalizeResults makes relative links absolute and keeps the path", () => {
  const [r] = normalizeResults(
    [{ name: "x", url: "/t/1", torrent: "/dl/1.torrent", magnet: "magnet:?xt=urn:btih:aa", seeds: "5" }],
    "https://site.example/search?q={{query}}",
  );
  assert.deepEqual(r, {
    name: "x", url: "https://site.example/t/1", path: "/t/1",
    torrent: "https://site.example/dl/1.torrent", magnet: "magnet:?xt=urn:btih:aa", seeds: "5",
  });
});

test("normalizeResults drops script and data links from hostile pages", () => {
  const [r] = normalizeResults(
    [{ name: "x", url: "javascript:alert(1)", magnet: "javascript:alert(2)", torrent: "data:x" }],
    "https://site.example/",
  );
  assert.equal(r.url, "");
  assert.equal(r.magnet, "");
  assert.equal(r.torrent, "");
  assert.equal(r.path, undefined);
});

test("normalizeResults leaves absolute links alone", () => {
  const [r] = normalizeResults([{ name: "x", url: "https://other.example/x" }], "https://site.example/");
  assert.equal(r.url, "https://other.example/x");
  assert.equal(r.path, undefined);
});

test("resolveItem prefers magnet, then torrent, then a lookup", () => {
  assert.deepEqual(resolveItem({ magnet: "magnet:?xt=urn:btih:a", torrent: "https://s/x" }), { kind: "magnet", value: "magnet:?xt=urn:btih:a" });
  assert.deepEqual(resolveItem({ torrent: "https://s/x.torrent" }), { kind: "url", value: "https://s/x.torrent" });
  assert.deepEqual(resolveItem({ path: "/t/1" }), { kind: "lookup", path: "/t/1" });
  assert.deepEqual(resolveItem({}), { error: "No item URL found" });
});

test("resolveLookup turns an item page into something the server can add", () => {
  assert.deepEqual(resolveLookup({ torrent: "https://s/x.torrent", magnet: "magnet:?xt=urn:btih:a" }, "n"), { kind: "url", value: "https://s/x.torrent" });
  assert.deepEqual(resolveLookup({ magnet: "magnet:?xt=urn:btih:a" }, "n"), { kind: "magnet", value: "magnet:?xt=urn:btih:a" });
  assert.deepEqual(
    resolveLookup({ infohash: "abc", tracker: "udp://a:1, http://b/ann ,https://c/ann,wss://d" }, "My Name"),
    { kind: "magnet", value: "magnet:?xt=urn:btih:abc&dn=My%20Name&tr=udp%3A%2F%2Fa%3A1&tr=http%3A%2F%2Fb%2Fann" },
  );
  assert.deepEqual(resolveLookup({ torrent: "javascript:x" }, "n"), { error: "No magnet or infohash found" });
  assert.deepEqual(resolveLookup({}, "n"), { error: "No magnet or infohash found" });
  assert.deepEqual(resolveLookup(null, "n"), { error: "No response" });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `Cannot find module .../lib/search.js`.

- [ ] **Step 3: Implement** `static/files/js/lib/search.js`

```js
import { buildMagnet } from "./magnet.js";

export function safeHttpUrl(u) {
  return typeof u === "string" && /^https?:\/\//i.test(u) ? u : "";
}

const safeMagnet = (m) => (typeof m === "string" && /^magnet:\?/i.test(m) ? m : "");

export function providerList(searchProviders) {
  return Object.entries(searchProviders || {})
    .filter(([id]) => !/\/item$/.test(id))
    .map(([id, p]) => ({ id, name: (p && p.name) || id }));
}

export function pickProvider(list, stored) {
  if (list.some((p) => p.id === stored)) return stored;
  return list.length ? list[0].id : "";
}

export function normalizeResults(results, providerUrl) {
  const m = /^(https?:\/\/[^/]+)/i.exec(providerUrl || "");
  const origin = m ? m[1] : "";
  return (results || []).map((r) => {
    const out = { ...r };
    if (typeof r.url === "string" && r.url.startsWith("/") && !r.url.startsWith("//")) {
      out.path = r.url;
      out.url = origin + r.url;
    }
    if (typeof r.torrent === "string" && r.torrent.startsWith("/") && !r.torrent.startsWith("//")) {
      out.torrent = origin + r.torrent;
    }
    if ("url" in out) out.url = safeHttpUrl(out.url);
    if ("torrent" in out) out.torrent = safeHttpUrl(out.torrent);
    if ("magnet" in out) out.magnet = safeMagnet(out.magnet);
    return out;
  });
}

export function resolveItem(result) {
  if (result.magnet) return { kind: "magnet", value: result.magnet };
  if (result.torrent) return { kind: "url", value: result.torrent };
  if (result.path) return { kind: "lookup", path: result.path };
  return { error: "No item URL found" };
}

export function resolveLookup(data, name) {
  if (!data) return { error: "No response" };
  const torrent = safeHttpUrl(data.torrent);
  if (torrent) return { kind: "url", value: torrent };
  const magnet = safeMagnet(data.magnet);
  if (magnet) return { kind: "magnet", value: magnet };
  if (data.infohash) {
    const trackers = String(data.tracker || "")
      .split(",")
      .map((s) => s.trim())
      .filter((s) => /^(http|udp):\/\//.test(s));
    return { kind: "magnet", value: buildMagnet({ name, infohash: data.infohash, trackers }) };
  }
  return { error: "No magnet or infohash found" };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: all pass.

- [ ] **Step 5: Checkpoint.** Leave uncommitted.

---

### Task 6: `lib/api.js`

**Files:**
- Create: `static/files/js/lib/api.js`
- Test: `static/files/js/lib/api.test.mjs`

**Interfaces:**
- Consumes: `downloadHref` from `lib/tree.js`.
- Produces: `errorMessage(text, status): string` and `createApi(fetchImpl)`. `createApi` returns:
  - `onBusy(fn: (busy: boolean) => void): () => void`
  - `magnet(uri)`, `url(u)`, `torrentFile(bytes: Uint8Array)`, `torrent(action: "start"|"stop"|"delete", infohash)` and `configure(config: object)`: each is `Promise<string>`
  - `search(provider, query, page): Promise<any>`
  - `searchItem(provider, path): Promise<any>`
  - `deleteDownload(path): Promise<string>`

  Any non-2xx response rejects with `Error(errorMessage(body, status))`.

- [ ] **Step 1: Write the failing test** at `static/files/js/lib/api.test.mjs`

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { createApi, errorMessage } from "./api.js";

function stubFetch(...responses) {
  const calls = [];
  const f = async (url, init = {}) => {
    calls.push({ url, method: init.method, body: init.body });
    const r = responses.shift();
    if (r instanceof Error) throw r;
    return { ok: r.status >= 200 && r.status < 300, status: r.status, text: async () => r.body };
  };
  f.calls = calls;
  return f;
}
const ok = (body = "OK") => ({ status: 200, body });

test("torrent actions post the exact bodies the server expects", async () => {
  const f = stubFetch(ok(), ok(), ok(), ok(), ok());
  const api = createApi(f);
  const bytes = new Uint8Array([1, 2, 3]);
  await api.magnet("magnet:?xt=urn:btih:a");
  await api.url("https://s/x.torrent");
  await api.torrentFile(bytes);
  await api.torrent("stop", "abc");
  await api.configure({ IncomingPort: 50007, AutoStart: true });
  assert.deepEqual(f.calls, [
    { url: "api/magnet", method: "POST", body: "magnet:?xt=urn:btih:a" },
    { url: "api/url", method: "POST", body: "https://s/x.torrent" },
    { url: "api/torrentfile", method: "POST", body: bytes },
    { url: "api/torrent", method: "POST", body: "stop:abc" },
    { url: "api/configure", method: "POST", body: '{"IncomingPort":50007,"AutoStart":true}' },
  ]);
});

test("search and item lookups encode their parameters and parse JSON", async () => {
  const f = stubFetch(ok('[{"name":"a"}]'), ok('{"infohash":"abc"}'));
  const api = createApi(f);
  assert.deepEqual(await api.search("nyaa", "ubuntu 24 & co", 2), [{ name: "a" }]);
  assert.deepEqual(await api.searchItem("nyaa", "/view/1?x=2"), { infohash: "abc" });
  assert.equal(f.calls[0].url, "search/nyaa?query=ubuntu+24+%26+co&page=2");
  assert.equal(f.calls[0].method, "GET");
  assert.equal(f.calls[1].url, "search/nyaa/item?item=%2Fview%2F1%3Fx%3D2");
});

test("deleteDownload sends DELETE to the encoded path", async () => {
  const f = stubFetch(ok(""));
  await createApi(f).deleteDownload("Show/e 1 #2.mkv");
  assert.deepEqual(f.calls[0], { url: "download/Show/e%201%20%232.mkv", method: "DELETE", body: undefined });
});

test("server errors reject with the server's message", async () => {
  const api = createApi(stubFetch({ status: 400, body: "Already started" }, { status: 500, body: '{"error":"Endpoint /x not found"}' }, { status: 502, body: "  " }));
  await assert.rejects(api.torrent("start", "a"), { message: "Already started" });
  await assert.rejects(api.search("x", "q", 1), { message: "Endpoint /x not found" });
  await assert.rejects(api.url("https://s/x"), { message: "Request failed (502)" });
});

test("network failures reject with the fetch error", async () => {
  const api = createApi(stubFetch(new TypeError("Failed to fetch")));
  await assert.rejects(api.magnet("magnet:?xt=urn:btih:a"), { message: "Failed to fetch" });
});

test("errorMessage caps long bodies", () => {
  const msg = errorMessage("x".repeat(1000), 500);
  assert.equal(msg.length, 301);
  assert.ok(msg.endsWith("…"));
});

test("onBusy reports true while any request is in flight, then false", async () => {
  let release;
  const gate = new Promise((r) => (release = r));
  const f = async () => { await gate; return { ok: true, status: 200, text: async () => "OK" }; };
  const api = createApi(f);
  const seen = [];
  const off = api.onBusy((b) => seen.push(b));
  const a = api.magnet("magnet:?xt=urn:btih:a");
  const b = api.magnet("magnet:?xt=urn:btih:b");
  release();
  await Promise.all([a, b]);
  assert.deepEqual(seen, [true, false]);
  off();
  await api.magnet("magnet:?xt=urn:btih:c");
  assert.deepEqual(seen, [true, false]);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: FAIL with `Cannot find module .../lib/api.js`.

- [ ] **Step 3: Implement** `static/files/js/lib/api.js`

```js
import { downloadHref } from "./tree.js";

const MAX_MESSAGE = 300;

export function errorMessage(text, status) {
  try {
    const j = JSON.parse(text);
    if (j && typeof j.error === "string") return j.error;
  } catch {
    // not JSON; fall through to the plain text body
  }
  const t = String(text || "").trim();
  if (!t) return `Request failed (${status})`;
  return t.length > MAX_MESSAGE ? t.slice(0, MAX_MESSAGE) + "…" : t;
}

export function createApi(fetchImpl) {
  let inflight = 0;
  const listeners = new Set();
  const emit = () => {
    for (const fn of listeners) fn(inflight > 0);
  };

  async function request(url, init) {
    inflight++;
    if (inflight === 1) emit();
    try {
      const res = await fetchImpl(url, init);
      const text = await res.text();
      if (!res.ok) throw new Error(errorMessage(text, res.status));
      return text;
    } finally {
      inflight--;
      if (inflight === 0) emit();
    }
  }

  const post = (action, body) => request("api/" + action, { method: "POST", body });
  const getJSON = async (url) => JSON.parse(await request(url, { method: "GET" }));
  const enc = encodeURIComponent;

  return {
    onBusy(fn) {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    magnet: (uri) => post("magnet", uri),
    url: (u) => post("url", u),
    torrentFile: (bytes) => post("torrentfile", bytes),
    torrent: (action, infohash) => post("torrent", `${action}:${infohash}`),
    configure: (config) => post("configure", JSON.stringify(config)),
    search: (provider, query, page) =>
      getJSON(`search/${enc(provider)}?${new URLSearchParams({ query, page: String(page) })}`),
    searchItem: (provider, path) => getJSON(`search/${enc(provider)}/item?${new URLSearchParams({ item: path })}`),
    deleteDownload: (path) => request(downloadHref(path), { method: "DELETE" }),
  };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: all pass.

- [ ] **Step 5: Mutation check.** One at a time, confirm each change fails a test, then revert it:
  - drop `inflight === 1 &&` gating so `emit()` runs on every request
  - change `post("torrent", \`${action}:${infohash}\`)` to use `/` as the separator
  - remove the `j.error` branch

- [ ] **Step 6: Checkpoint.** Leave uncommitted.

---

### Task 7: Shell, live state, base components, styles, and the e2e harness

This task swaps the old UI out. After it, the page shows the header, reconnect strip, error banner and footer. Later tasks mount the remaining sections into `App.js`.

**Files:**
- Rewrite: `static/files/index.html`, `static/files/css/app.css`
- Create: `static/files/js/state.js`, `static/files/js/icons.js`, `static/files/js/main.js`
- Create: `static/files/js/components/App.js`, `Header.js`, `Footer.js`, `ErrorBanner.js`
- Delete: `static/files/js/run.js`, `utils.js`, `config-controller.js`, `downloads-controller.js`, `omni-controller.js`, `torrents-controller.js`, `semantic-checkbox.js`, `static/files/js/vendor/angular.min.js`, `moment.min.js`, `query-string.js`, `static/files/template/`, `static/files/css/semantic.min.css`, `static/files/css/themes/`, `static/files/css/Lato/`, `static/files/css/sections/`
- Test: `server/ui_test.go`
- Harness (outside the repo): `/sessions/zen-kind-bohr/e2e/`

**Interfaces:**
- Consumes: `html` (Task 1), `createApi` (Task 6).
- Produces:
  - `startSync(velox?)` and `useSync(): { state, connected, everConnected }` from `js/state.js`
  - `Icon({ name, label?, class? })` from `js/icons.js`. Names: `cloud search magnet settings upload download file folder folderOpen music image film play stop trash check x loader`.
  - `App({ api })`, with local state `error`, `busy`, `configOpen`, `editorOpen` and a `report(err)` helper that later tasks pass as `onError`
  - Harness exports from `fixtures.mjs`: `test`, `expect`, and an `app` fixture with `{ dir, dl, port, server: { base }, fake, errors, restart(), stop(), makeTorrent(name, size, opts?), makeFolderTorrent(dir, files), addTorrent(buf), addMagnet(uri) }`

- [ ] **Step 1: Write the failing Go test** at `server/ui_test.go`

```go
package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootServesNewShell(t *testing.T) {
	s, _ := newTestServer(t, "")
	w := do(t, s.handler(), httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `<script type="importmap">`) || !strings.Contains(body, `src="js/main.js"`) {
		t.Fatalf("status %d; shell is missing the import map or main.js:\n%s", w.Code, body)
	}
}

// module scripts are refused by browsers unless served with a JavaScript MIME type
func TestModulesServedAsJavaScript(t *testing.T) {
	s, _ := newTestServer(t, "")
	h := s.handler()
	for _, p := range []string{"/js/main.js", "/js/vendor/preact.mjs", "/js/vendor/hooks.mjs", "/js/vendor/htm.mjs", "/js/components/App.js"} {
		w := do(t, h, httptest.NewRequest("GET", p, nil))
		ct := w.Header().Get("Content-Type")
		if w.Code != 200 || !(strings.HasPrefix(ct, "text/javascript") || strings.HasPrefix(ct, "application/javascript")) {
			t.Errorf("%s: status %d, Content-Type %q", p, w.Code, ct)
		}
	}
}

func TestOldUIFilesAreGone(t *testing.T) {
	s, _ := newTestServer(t, "")
	h := s.handler()
	for _, p := range []string{"/js/vendor/angular.min.js", "/css/semantic.min.css", "/template/omni.html", "/js/run.js", "/css/Lato/Lato.css"} {
		if w := do(t, h, httptest.NewRequest("GET", p, nil)); w.Code != 404 {
			t.Errorf("%s still served (status %d)", p, w.Code)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go test -count=1 -run 'Shell|Modules|OldUI' ./server/`
Expected: FAIL on all three (no import map, no `main.js`, old files still served).

- [ ] **Step 3: Delete the old UI**

```bash
cd /sessions/zen-kind-bohr/mnt/cloud-fetch/static/files
rm -f js/run.js js/utils.js js/config-controller.js js/downloads-controller.js js/omni-controller.js js/torrents-controller.js js/semantic-checkbox.js
rm -f js/vendor/angular.min.js js/vendor/moment.min.js js/vendor/query-string.js
rm -rf template css/semantic.min.css css/themes css/Lato css/sections
ls -R . | head -40
```

Expected remaining: `index.html`, `cloud-favicon.png`, `css/app.css` (rewritten next) and `js/` (`html.js`, `lib/`, `vendor/` with the three `.mjs` files, `VENDOR.md` and `vendor.test.mjs`).

- [ ] **Step 4: Write `static/files/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Cloud Fetch</title>
    <link rel="icon" href="cloud-favicon.png" />
    <link rel="stylesheet" href="css/app.css" />
    <script type="importmap">
      { "imports": { "preact": "./js/vendor/preact.mjs", "preact/hooks": "./js/vendor/hooks.mjs" } }
    </script>
    <script src="js/velox.js"></script>
    <script type="module" src="js/main.js"></script>
  </head>
  <body>
    <div id="app" class="container"></div>
    <noscript>Cloud Fetch needs JavaScript.</noscript>
  </body>
</html>
```

`js/velox.js` is served by the Go handler from the velox package, not from `static/files`.

- [ ] **Step 5: Write `static/files/css/app.css`**

```css
:root {
  --bg: #f6f7f9;
  --surface: #fff;
  --text: #1d2330;
  --muted: #5f6b7a;
  --faint: #8a94a3;
  --border: #dde2e8;
  --accent: #1f6fd1;
  --accent-bg: #e8f1fc;
  --ok: #1f8a4c;
  --danger: #c23b3b;
  --danger-bg: #fbecec;
  --radius: 8px;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font: 15px/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  color: var(--text);
  background: var(--bg);
}
* { box-sizing: border-box; }
body { margin: 0; }
.container { max-width: 880px; margin: 0 auto; padding: 16px; }
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
h3, h4 { margin: 0; font-weight: 600; }
button, select, input { font: inherit; color: inherit; }
button, select {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--surface); padding: 5px 10px; cursor: pointer;
}
button:hover:not(:disabled) { border-color: var(--faint); }
button:disabled { opacity: 0.5; cursor: default; }
button.primary { background: var(--accent); border-color: var(--accent); color: #fff; }
button.go { color: var(--ok); border-color: var(--ok); }
button.danger { color: var(--danger); border-color: var(--danger); }
button.on { background: var(--accent-bg); border-color: var(--accent); color: var(--accent); }
.icon-btn { border: 0; background: none; padding: 4px; color: var(--muted); }
.icon-btn.go { color: var(--ok); }
.icon-btn.danger { color: var(--danger); }
.icon-btn.on { background: var(--accent-bg); color: var(--accent); }
input[type="text"], input[type="number"] {
  width: 100%; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--surface); padding: 7px 10px;
}
input:focus, select:focus, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
.icon {
  width: 18px; height: 18px; flex: none;
  fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round;
}
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }

.header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 14px; }
.brand { display: flex; align-items: center; gap: 8px; font-size: 20px; font-weight: 600; color: var(--text); }
.brand .icon { width: 24px; height: 24px; color: var(--accent); }
.status { display: flex; align-items: center; gap: 6px; }
.dot { width: 10px; height: 10px; margin-left: 4px; border-radius: 50%; background: var(--danger); }
.dot.ok { background: var(--ok); }
.reconnecting {
  margin-bottom: 10px; padding: 4px 10px; border-radius: var(--radius);
  background: var(--danger-bg); color: var(--danger); font-size: 13px;
}
.banner {
  display: flex; align-items: flex-start; justify-content: space-between; gap: 8px;
  margin: 10px 0; padding: 8px 12px; border: 1px solid var(--danger); border-radius: var(--radius);
  background: var(--danger-bg); color: var(--danger); overflow-wrap: anywhere;
}
.banner .icon-btn { color: var(--danger); }
.field-error { margin: 6px 0; color: var(--danger); font-size: 13px; }

.card {
  position: relative; margin-bottom: 10px; padding: 12px 14px;
  border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
}
.card h4 { margin-bottom: 10px; }
.card label { display: block; margin-bottom: 10px; color: var(--muted); font-size: 13px; }
.card label input { margin-top: 4px; color: var(--text); font-size: 15px; }
.card label.check { display: flex; align-items: center; gap: 8px; color: var(--text); font-size: 15px; }
.card fieldset { margin: 0; padding: 0; border: 0; }
.card legend { margin-bottom: 4px; color: var(--muted); font-size: 13px; }
.card fieldset input { margin-bottom: 6px; }

.omni { margin-bottom: 18px; }
.omni-bar {
  display: flex; align-items: center; gap: 4px; padding: 2px 4px 2px 2px;
  border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
}
.omni-bar:focus-within { outline: 2px solid var(--accent); outline-offset: 1px; }
.omni-bar.drag { border-color: var(--accent); background: var(--accent-bg); }
.omni-bar .omni-input { border: 0; background: transparent; }
.omni-bar .omni-input:focus { outline: none; }
.actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.results {
  width: 100%; margin-top: 10px; border-collapse: collapse; font-size: 14px;
  border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
}
.results td { padding: 7px 10px; border-top: 1px solid var(--border); vertical-align: middle; }
.results tr:first-child td { border-top: 0; }
.results .name { overflow-wrap: anywhere; }
.results .size, .results .users { color: var(--muted); white-space: nowrap; }
.results .seeds { color: var(--ok); }
.results .controls { width: 1%; }
.results .more { text-align: center; }

.section-header { display: flex; align-items: baseline; justify-content: space-between; margin: 18px 0 8px; }
.muted { color: var(--muted); }
.empty {
  margin: 0; padding: 12px; color: var(--muted);
  border: 1px dashed var(--border); border-radius: var(--radius); background: var(--surface);
}
.torrent-top { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 10px; }
.torrent .info { flex: 1 1 260px; min-width: 0; }
.torrent .name { font-weight: 600; overflow-wrap: anywhere; }
.hash { color: var(--faint); font-family: var(--mono); font-size: 11px; overflow-wrap: anywhere; }
.torrent .buttons { position: relative; z-index: 1; display: flex; flex-wrap: wrap; gap: 6px; font-size: 13px; }
.overlay {
  position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; gap: 6px;
  border-radius: var(--radius); background: rgba(255, 255, 255, 0.75); color: var(--muted); pointer-events: none;
}
.progress { height: 6px; margin: 10px 0 6px; overflow: hidden; border-radius: 3px; background: var(--bg); }
.progress.thin { height: 3px; margin: 4px 0 0; }
.progress .bar { height: 100%; background: var(--accent); transition: width 0.5s; }
.torrent .status { color: var(--muted); font-size: 13px; }
.torrent .status strong { color: var(--text); }
.files { width: 100%; margin-top: 10px; border-collapse: collapse; font-size: 13px; }
.files th, .files td { padding: 5px 6px; border-top: 1px solid var(--border); text-align: left; }
.files .name { overflow-wrap: anywhere; }
.files .size { width: 1%; text-align: right; white-space: nowrap; }
.files .pct { color: var(--accent); }
.files .ok { color: var(--ok); vertical-align: -4px; }

.tree { margin: 0; padding: 0; list-style: none; }
.downloads > .tree { padding: 8px 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); }
.tree .tree { padding-left: 22px; }
.node .row { display: flex; align-items: center; gap: 6px; min-height: 28px; }
.node .row > .icon { margin: 0 4px; color: var(--muted); }
.node .label { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.node .controls { display: flex; gap: 2px; }
.node .meta { margin: -4px 0 4px 30px; color: var(--faint); font-size: 12px; }
.preview { display: block; max-width: 100%; max-height: 60vh; margin: 6px 0 8px 30px; }

.footer { margin-top: 24px; color: var(--faint); font-size: 12px; text-align: center; overflow-wrap: anywhere; }

@media (max-width: 600px) {
  .container { padding: 10px; }
  .torrent .buttons { width: 100%; }
  .preview { margin-left: 0; }
}
```

- [ ] **Step 6: Write `static/files/js/state.js`**

```js
import { useEffect, useState } from "preact/hooks";

// velox merges every server push into this one object in place
const state = {};
let connected = false;
let everConnected = false;
const subscribers = new Set();

function notify() {
  for (const bump of subscribers) bump((n) => n + 1);
}

export function startSync(velox = window.velox) {
  const v = velox("/sync", state);
  v.onupdate = notify;
  v.onchange = (c) => {
    connected = c;
    if (c) everConnected = true;
    notify();
  };
  return v;
}

export function useSync() {
  const [, bump] = useState(0);
  useEffect(() => {
    subscribers.add(bump);
    return () => subscribers.delete(bump);
  }, []);
  return { state, connected, everConnected };
}
```

- [ ] **Step 7: Write `static/files/js/icons.js`**

```js
import { html } from "./html.js";

const PATHS = {
  cloud: "M7 18h10a4 4 0 0 0 .6-7.96A6 6 0 0 0 6.1 9.1 4.5 4.5 0 0 0 7 18z",
  search: "M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0zM21 21l-6-6",
  magnet: "M5 13V9a7 7 0 0 1 14 0v4h-4V9a3 3 0 0 0-6 0v4zM5 13a7 7 0 0 0 14 0",
  settings: "M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9 7 7M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1",
  upload: "M12 16V4M7 9l5-5 5 5M4 20h16",
  download: "M12 4v12M7 11l5 5 5-5M4 20h16",
  file: "M14 3H6v18h12V7zM14 3v4h4",
  folder: "M3 6h6l2 2h10v11H3z",
  folderOpen: "M3 19V6h6l2 2h8v3M3 19l3-8h16l-3 8z",
  music: "M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  image: "M3 5h18v14H3zM3 16l5-5 5 5 3-3 5 5M15 9h.01",
  film: "M3 5h18v14H3zM7 5v14M17 5v14M3 9h4M3 15h4M17 9h4M17 15h4",
  play: "M7 4l13 8-13 8z",
  stop: "M6 6h12v12H6z",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13",
  check: "M5 12l5 5 9-10",
  x: "M6 6l12 12M18 6 6 18",
  loader: "M12 3a9 9 0 1 0 9 9",
};

export function Icon({ name, label, class: extra = "" }) {
  return html`<svg
    class=${"icon " + extra}
    viewBox="0 0 24 24"
    role=${label ? "img" : null}
    aria-label=${label || null}
    aria-hidden=${label ? null : "true"}
    focusable="false"
  ><path d=${PATHS[name]} /></svg>`;
}
```

- [ ] **Step 8: Write the base components**

`static/files/js/components/ErrorBanner.js`:

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";

export function ErrorBanner({ message, onDismiss }) {
  return html`<div class="banner" role="alert">
    <span>${message}</span>
    <button type="button" class="icon-btn" aria-label="Dismiss" onClick=${onDismiss}><${Icon} name="x" /></button>
  </div>`;
}
```

`static/files/js/components/Header.js`:

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";

export function Header({ title, busy, connected, configOpen, editorOpen, onToggleConfig, onToggleEditor }) {
  return html`<header class="header">
    <a class="brand" href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">
      <${Icon} name="cloud" />${title || "Cloud Fetch"}
    </a>
    <div class="status">
      ${busy && html`<${Icon} name="loader" class="spin" label="Working" />`}
      <button type="button" class=${"icon-btn" + (editorOpen ? " on" : "")} aria-pressed=${editorOpen}
        aria-label="Magnet editor" onClick=${onToggleEditor}><${Icon} name="magnet" /></button>
      <button type="button" class=${"icon-btn" + (configOpen ? " on" : "")} aria-pressed=${configOpen}
        aria-label="Settings" onClick=${onToggleConfig}><${Icon} name="settings" /></button>
      <span class=${"dot" + (connected ? " ok" : "")} role="img" aria-label=${connected ? "Connected" : "Disconnected"}></span>
    </div>
  </header>`;
}
```

`static/files/js/components/Footer.js`:

```js
import { html } from "../html.js";

export function Footer({ stats = {}, users }) {
  const n = users ? Object.keys(users).length : 0;
  return html`<footer class="footer">
    <a href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">github.com/LeonardCooray/cloud-fetch</a>
    ${stats.Version ? ` version ${stats.Version}` : ""}
    ${" (fork of "}<a href="https://github.com/jpillora/cloud-torrent" target="_blank" rel="noopener">jpillora/cloud-torrent</a>${", AGPL-3.0)"}
    ${n > 1 ? ` · ${n} users connected` : ""}
    ${stats.Runtime ? html` · <a href="https://go.dev" target="_blank" rel="noopener">Go</a> ${stats.Runtime}` : ""}
  </footer>`;
}
```

`static/files/js/components/App.js`:

```js
import { useEffect, useState } from "preact/hooks";
import { html } from "../html.js";
import { useSync } from "../state.js";
import { Header } from "./Header.js";
import { Footer } from "./Footer.js";
import { ErrorBanner } from "./ErrorBanner.js";

export function App({ api }) {
  const { state, connected, everConnected } = useSync();
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);
  const report = (e) => setError((e && e.message) || String(e));
  useEffect(() => api.onBusy(setBusy), [api]);
  const stats = state.Stats || {};
  useEffect(() => {
    if (stats.Title) document.title = stats.Title;
  }, [stats.Title]);

  return html`
    <${Header} title=${stats.Title} busy=${busy} connected=${connected}
      configOpen=${configOpen} editorOpen=${editorOpen}
      onToggleConfig=${() => setConfigOpen(!configOpen)}
      onToggleEditor=${() => setEditorOpen(!editorOpen)} />
    ${everConnected && !connected && html`<div class="reconnecting" role="status">Reconnecting…</div>`}
    ${error && html`<${ErrorBanner} message=${error} onDismiss=${() => setError(null)} />`}
    <${Footer} stats=${stats} users=${state.Users} />
  `;
}
```

(`report`, `configOpen` and `editorOpen` get their consumers in Tasks 8 to 11.)

`static/files/js/main.js`:

```js
import { render } from "preact";
import { html } from "./html.js";
import { App } from "./components/App.js";
import { startSync } from "./state.js";
import { createApi } from "./lib/api.js";

startSync();
const api = createApi(window.fetch.bind(window));
render(html`<${App} api=${api} />`, document.getElementById("app"));
```

- [ ] **Step 9: Run the Go test to verify it passes**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go vet ./... && go test -count=1 -race ./...`
Expected: all pass, including the three new UI tests. If `TestModulesServedAsJavaScript` fails only for `.mjs` with a non-JS Content-Type, add `mime.AddExtensionType(".mjs", "text/javascript; charset=utf-8")` in an `init()` in `static/static.go`, then rerun. Report which branch happened.

- [ ] **Step 10: Create the e2e harness** in `/sessions/zen-kind-bohr/e2e`

`package.json`:

```json
{ "private": true, "type": "module", "devDependencies": { "@playwright/test": "1.63.0" } }
```

`playwright.config.mjs`:

```js
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  timeout: 30000,
  workers: 1,
  reporter: "list",
  use: { headless: true, viewport: { width: 1280, height: 900 } },
});
```

`mktorrent.mjs`:

```js
import { createHash } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

function bencode(v) {
  if (typeof v === "number") return Buffer.from(`i${v}e`);
  if (Buffer.isBuffer(v)) return Buffer.concat([Buffer.from(`${v.length}:`), v]);
  if (typeof v === "string") return bencode(Buffer.from(v));
  if (Array.isArray(v)) return Buffer.concat([Buffer.from("l"), ...v.map(bencode), Buffer.from("e")]);
  const keys = Object.keys(v).sort();
  return Buffer.concat([Buffer.from("d"), ...keys.flatMap((k) => [bencode(k), bencode(v[k])]), Buffer.from("e")]);
}

const PIECE = 16384;
const fill = (size, seed) => {
  const b = Buffer.alloc(size);
  for (let i = 0; i < size; i++) b[i] = (i * 31 + seed) % 251;
  return b;
};
const pieces = (data) => {
  const out = [];
  for (let o = 0; o < data.length; o += PIECE) out.push(createHash("sha1").update(data.subarray(o, o + PIECE)).digest());
  return Buffer.concat(out);
};
const finish = (info) => ({ torrent: bencode({ info }), infohash: createHash("sha1").update(bencode(info)).digest("hex") });

// Single-file torrent. With { partial: true } only the first half of the data
// is written correctly, so the engine sees it as about 50% downloaded.
export function makeTorrent(dl, name, size = 300000, { partial = false } = {}) {
  const data = fill(size, 7);
  const onDisk = partial ? Buffer.concat([data.subarray(0, size / 2), Buffer.alloc(size - size / 2)]) : data;
  writeFileSync(join(dl, name), onDisk);
  return { ...finish({ length: size, name, "piece length": PIECE, pieces: pieces(data) }), path: name };
}

// Multi-file torrent in folder `dir`; files is [{ name, size }].
export function makeFolderTorrent(dl, dir, files) {
  mkdirSync(join(dl, dir), { recursive: true });
  const parts = files.map((f, i) => {
    const data = fill(f.size, i + 1);
    writeFileSync(join(dl, dir, f.name), data);
    return data;
  });
  const info = {
    files: files.map((f) => ({ length: f.size, path: [f.name] })),
    name: dir,
    "piece length": PIECE,
    pieces: pieces(Buffer.concat(parts)),
  };
  return { ...finish(info), path: dir };
}
```

`fake-provider.mjs`:

```js
import http from "node:http";

// A local stand-in for a torrent site: serves a provider list the server loads
// via --search-config-url, the HTML pages the scraper parses, and any extra
// files a test registers with serve().
export function startFakeProvider() {
  const extra = new Map();
  const server = http.createServer((req, res) => {
    const u = new URL(req.url, "http://x");
    const base = `http://127.0.0.1:${server.address().port}`;
    if (extra.has(u.pathname)) return res.end(extra.get(u.pathname));
    if (u.pathname === "/config.json") {
      res.setHeader("Content-Type", "application/json");
      return res.end(JSON.stringify({
        fake: {
          name: "Fake",
          url: `${base}/search?q={{query}}&p={{page:1}}`,
          list: "table tr.r",
          result: { name: "td.n a", url: ["td.n a", "@href"], magnet: ["a.m", "@href"], size: "td.s", seeds: "td.se", peers: "td.pe" },
        },
        "fake/item": { name: "Fake (Item)", url: `${base}{{item}}`, result: { infohash: "span.ih", tracker: "span.tr" } },
      }));
    }
    if (u.pathname === "/search") {
      const q = u.searchParams.get("q");
      const p = Number(u.searchParams.get("p"));
      const rows = q === "none" || p > 2 ? [] : [1, 2];
      const href = (i) => (q === "evil" ? "javascript:alert(1)" : `/item/${p}-${i}`);
      const magnet = (i) => `magnet:?xt=urn:btih:${"a".repeat(39)}${p}&dn=${encodeURIComponent(`${q} result ${p}-${i}`)}`;
      res.setHeader("Content-Type", "text/html");
      return res.end(`<table>${rows.map((i) => `<tr class="r"><td class="n"><a href="${href(i)}">${q} result ${p}-${i}</a></td><td class="s">1.${i} GB</td><td class="se">${10 * i}</td><td class="pe">${i}</td>${i === 1 && q !== "evil" ? `<td><a class="m" href="${magnet(i)}">m</a></td>` : ""}</tr>`).join("")}</table>`);
    }
    if (u.pathname.startsWith("/item/")) {
      res.setHeader("Content-Type", "text/html");
      return res.end(`<span class="ih">${"b".repeat(39)}${u.pathname.slice(-1)}</span><span class="tr">udp://tracker.example:80,https://ignored.example/announce</span>`);
    }
    res.statusCode = 404;
    res.end("not found");
  });
  return new Promise((resolve) =>
    server.listen(0, "127.0.0.1", () => {
      const base = `http://127.0.0.1:${server.address().port}`;
      resolve({
        base,
        configURL: `${base}/config.json`,
        serve: (path, body) => extra.set(path, body),
        close: () => new Promise((c) => server.close(c)),
      });
    }),
  );
}
```

`fixtures.mjs`:

```js
import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import net from "node:net";
import { startFakeProvider } from "./fake-provider.mjs";
import { makeTorrent, makeFolderTorrent } from "./mktorrent.mjs";

const BIN = process.env.CF_BIN || "/tmp/cf-e2e";

export function freePort() {
  return new Promise((resolve) => {
    const s = net.createServer();
    s.listen(0, "127.0.0.1", () => {
      const p = s.address().port;
      s.close(() => resolve(p));
    });
  });
}

async function startServer({ dir, port, searchURL }) {
  const args = ["--port", String(port), "--host", "127.0.0.1", "--config-path", join(dir, "cloud-fetch.json"), "--search-config-url", searchURL];
  const proc = spawn(BIN, args, { stdio: ["ignore", "pipe", "pipe"] });
  let log = "";
  proc.stdout.on("data", (d) => (log += d));
  proc.stderr.on("data", (d) => (log += d));
  const baseURL = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(baseURL + "/")).ok) return { proc, base: baseURL, log: () => log };
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("server did not start:\n" + log);
}

async function stopServer(s) {
  if (s.proc.exitCode === null) {
    s.proc.kill("SIGTERM");
    await new Promise((r) => s.proc.once("exit", r));
  }
}

export const test = base.extend({
  app: async ({ page }, use) => {
    const dir = mkdtempSync(join(tmpdir(), "cf-e2e-"));
    const dl = join(dir, "downloads");
    mkdirSync(dl);
    writeFileSync(join(dir, "cloud-fetch.json"), JSON.stringify({ DownloadDirectory: dl, IncomingPort: await freePort(), AutoStart: true, EnableUpload: false }));
    const fake = await startFakeProvider();
    const port = await freePort();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
    const app = {
      dir, dl, port, fake, errors,
      server: await startServer({ dir, port, searchURL: fake.configURL }),
      async restart() {
        await stopServer(app.server);
        app.server = await startServer({ dir, port, searchURL: fake.configURL });
      },
      stop: () => stopServer(app.server),
      makeTorrent: (name, size, opts) => makeTorrent(dl, name, size, opts),
      makeFolderTorrent: (folder, files) => makeFolderTorrent(dl, folder, files),
      addTorrent: (buf) => fetch(app.server.base + "/api/torrentfile", { method: "POST", body: buf }),
      addMagnet: (uri) => fetch(app.server.base + "/api/magnet", { method: "POST", body: uri }),
    };
    await page.goto(app.server.base + "/");
    await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
    await use(app);
    await stopServer(app.server);
    await fake.close();
    expect(errors, "browser console errors").toEqual([]);
  },
});
export { expect };
```

`env.sh`:

```bash
export PLAYWRIGHT_BROWSERS_PATH=/tmp/pw
export LD_LIBRARY_PATH=/tmp/xlibs/usr/lib/aarch64-linux-gnu:/tmp/xlibs/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
export CF_BIN=/tmp/cf-e2e
```

`tests/shell.spec.mjs`:

```js
import { test, expect } from "../fixtures.mjs";

test("shell renders from live state with no console errors", async ({ page }) => {
  await expect(page).toHaveTitle("Cloud Fetch");
  await expect(page.getByRole("link", { name: "Cloud Fetch" })).toHaveAttribute("href", "https://github.com/LeonardCooray/cloud-fetch");
  const footer = page.locator("footer");
  await expect(footer).toContainText("fork of jpillora/cloud-torrent, AGPL-3.0");
  await expect(footer).toContainText(/Go \d+\.\d+/);
  await expect(page.getByRole("button", { name: "Settings" })).toHaveAttribute("aria-pressed", "false");
});
```

- [ ] **Step 11: Install Playwright and headless Chromium in the sandbox**

```bash
cd /sessions/zen-kind-bohr/e2e && npm install --no-audit --no-fund
. ./env.sh && npx playwright install --only-shell chromium
SHELL_BIN=$(find /tmp/pw -name chrome-headless-shell -type f | head -1)
ldd "$SHELL_BIN" | grep "not found"
```

For each missing library, fetch its Ubuntu package without root and extract it into `/tmp/xlibs`. `libXdamage.so.1` is already known missing:

```bash
mkdir -p /tmp/debs /tmp/xlibs && cd /tmp/debs
apt-get download libxdamage1          # plus any other package ldd reported
for d in *.deb; do dpkg -x "$d" /tmp/xlibs; done
. /sessions/zen-kind-bohr/e2e/env.sh && ldd "$SHELL_BIN" | grep "not found"   # expect no output
```

Check `df -h /` before and after. The download is about 150 MB.

- [ ] **Step 12: Build and run the shell e2e test**

```bash
. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e .
cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/shell.spec.mjs
```

Expected: 1 passed, with no console errors (the fixture asserts this).

- [ ] **Step 13: Checkpoint.** Run `node --test "static/files/js/**/*.test.mjs"` and the Go suite again, and review `git --no-optional-locks status -s`: the deletions plus the new files, nothing else. Leave uncommitted.

---

### Task 8: Omni bar, magnet editor and search results

**Files:**
- Create: `static/files/js/components/OmniBar.js`, `MagnetEditor.js`, `SearchResults.js`
- Modify: `static/files/js/components/App.js` (mount `OmniBar`)
- Test (harness): `/sessions/zen-kind-bohr/e2e/tests/omni.spec.mjs`

**Interfaces:**
- Consumes: `classify`; `parseMagnet`, `buildMagnet`; `providerList`, `pickProvider`, `normalizeResults`, `resolveItem`, `resolveLookup`; the `api` methods `url`, `magnet`, `torrentFile`, `search` and `searchItem`; `Icon`; and from `App`: `editorOpen`, `setEditorOpen`, `report`.
- Produces: `OmniBar({ api, providers, editorOpen, setEditorOpen, onError })`, `MagnetEditor({ magnet, onChange })`, `SearchResults({ results, hasMore, searching, onMore, onAdd })`.

- [ ] **Step 1: Write the failing e2e tests** at `/sessions/zen-kind-bohr/e2e/tests/omni.spec.mjs`

```js
import { test, expect } from "../fixtures.mjs";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

const omni = (page) => page.getByRole("textbox", { name: "Search, magnet link or torrent URL" });
const record = (app, ih) => join(app.dl, ".cloud-fetch", ih + ".json");
const added = (app, ih) => expect.poll(() => existsSync(record(app, ih)), { timeout: 10000 }).toBe(true);
const saved = (app, ih) => JSON.parse(readFileSync(record(app, ih), "utf8"));
const HASH_A1 = "a".repeat(39) + "1";
const HASH_B2 = "b".repeat(39) + "2";
const search = (page) => page.getByRole("button", { name: "Search", exact: true });

test("dropping a .torrent file on the bar adds it", async ({ page, app }) => {
  const t = app.makeTorrent("dropped.bin", 50000);
  const dt = await page.evaluateHandle((bytes) => {
    const d = new DataTransfer();
    d.items.add(new File([new Uint8Array(bytes)], "dropped.torrent"));
    return d;
  }, [...t.torrent]);
  await page.locator(".omni-bar").dispatchEvent("drop", { dataTransfer: dt });
  await added(app, t.infohash);
});

test("the file picker uploads several .torrent files", async ({ page, app }) => {
  const a = app.makeTorrent("a.bin", 20000);
  const b = app.makeTorrent("b.bin", 30000);
  await page.locator('input[type="file"]').setInputFiles([
    { name: "a.torrent", mimeType: "application/x-bittorrent", buffer: a.torrent },
    { name: "b.torrent", mimeType: "application/x-bittorrent", buffer: b.torrent },
  ]);
  await added(app, a.infohash);
  await added(app, b.infohash);
});

test("non-.torrent files are refused inline", async ({ page }) => {
  await page.locator('input[type="file"]').setInputFiles({ name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("x") });
  await expect(page.getByText("Only .torrent files can be uploaded")).toBeVisible();
});

test("a torrent URL loads through the server", async ({ page, app }) => {
  const t = app.makeTorrent("remote.bin", 40000);
  app.fake.serve("/remote.torrent", t.torrent);
  await omni(page).fill(app.fake.base + "/remote.torrent");
  await page.getByRole("button", { name: "Load torrent" }).click();
  await added(app, t.infohash);
});

test("a bad torrent URL shows the server error in a dismissable banner", async ({ page, app }) => {
  await omni(page).fill(app.fake.base + "/missing.torrent");
  await page.getByRole("button", { name: "Load torrent" }).click();
  const banner = page.locator(".banner");
  await expect(banner).toBeVisible();
  await expect(banner).not.toHaveText("");
  await banner.getByRole("button", { name: "Dismiss" }).click();
  await expect(page.locator(".banner")).toHaveCount(0);
});

test("the magnet editor round-trips names with spaces and symbols", async ({ page, app }) => {
  const hash = "c".repeat(40);
  await omni(page).fill(`magnet:?xt=urn:btih:${hash}&dn=Big+Buck+Bunny`);
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const name = page.getByRole("textbox", { name: "Name", exact: true });
  await expect(name).toHaveValue("Big Buck Bunny");
  await name.fill("Big Buck Bunny (2008) & co #1");
  await page.getByRole("textbox", { name: "Tracker 1", exact: true }).fill("udp://t.example:80");
  await expect(omni(page)).toHaveValue(`magnet:?xt=urn:btih:${hash}&dn=Big%20Buck%20Bunny%20(2008)%20%26%20co%20%231&tr=udp%3A%2F%2Ft.example%3A80`);
  await expect(page.getByRole("textbox", { name: "Tracker 2", exact: true })).toHaveValue("");
  const infohash = page.getByRole("textbox", { name: "Info hash", exact: true });
  await infohash.fill("zz!");
  await expect(page.getByText("Invalid info hash")).toBeVisible();
  await infohash.fill(hash);
  await expect(page.getByText("Invalid info hash")).toHaveCount(0);
  await page.getByRole("button", { name: "Load magnet" }).click();
  await added(app, hash);
  expect(saved(app, hash).magnet).toContain("dn=Big%20Buck%20Bunny%20(2008)%20%26%20co%20%231");
});

test("search, load more, and add results by magnet and by item lookup", async ({ page, app }) => {
  await omni(page).fill("ubuntu");
  await expect(page.getByRole("combobox", { name: "Search provider" })).toHaveValue("fake");
  await search(page).click();
  const rows = page.locator(".results tr:has(td.name)");
  await expect(rows).toHaveCount(2);
  await expect(page.getByRole("link", { name: "ubuntu result 1-1" })).toHaveAttribute("href", app.fake.base + "/item/1-1");
  await page.getByRole("button", { name: "Load more" }).click();
  await expect(rows).toHaveCount(4);
  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByRole("button", { name: "Load more" })).toHaveCount(0);
  await expect(rows).toHaveCount(4);
  await page.getByRole("button", { name: "Add ubuntu result 1-1" }).click();
  await added(app, HASH_A1);
  await page.getByRole("button", { name: "Add ubuntu result 1-2" }).click();
  await added(app, HASH_B2);
  const magnet = saved(app, HASH_B2).magnet;
  expect(magnet).toContain("dn=ubuntu%20result%201-2");
  expect(magnet).toContain("tr=udp%3A%2F%2Ftracker.example%3A80");
  expect(magnet).not.toContain("ignored.example");
});

test("an empty search says so", async ({ page }) => {
  await omni(page).fill("none");
  await search(page).click();
  await expect(page.getByRole("button", { name: "No results" })).toBeVisible();
});

test("script links from a hostile provider page are not rendered", async ({ page }) => {
  await omni(page).fill("evil");
  await search(page).click();
  await expect(page.getByText("evil result 1-1")).toBeVisible();
  await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0);
});

test("the omni text survives a reload and a vanished provider falls back", async ({ page }) => {
  await omni(page).fill("debian");
  await page.evaluate(() => localStorage.setItem("tcProvider", "tpb"));
  await page.reload();
  await expect(omni(page)).toHaveValue("debian");
  await expect(page.getByRole("combobox", { name: "Search provider" })).toHaveValue("fake");
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/omni.spec.mjs`
Expected: all 10 fail, because there's no omni bar yet.

- [ ] **Step 3: Write `static/files/js/components/MagnetEditor.js`**

```js
import { html } from "../html.js";

export function MagnetEditor({ magnet, onChange }) {
  const trackers = [...magnet.trackers, ""];
  const set = (patch) => onChange({ name: magnet.name, infohash: magnet.infohash, trackers: magnet.trackers, ...patch });
  const setTracker = (i, value) => {
    const next = trackers.slice();
    next[i] = value;
    set({ trackers: next.filter(Boolean) });
  };
  return html`<form class="card editor" onSubmit=${(e) => e.preventDefault()}>
    <h4>Magnet URI editor</h4>
    <label>Name<input type="text" value=${magnet.name} placeholder="Name" onInput=${(e) => set({ name: e.currentTarget.value })} /></label>
    <label>Info hash<input type="text" value=${magnet.infohash} placeholder="Info hash" onInput=${(e) => set({ infohash: e.currentTarget.value.trim() })} /></label>
    <fieldset>
      <legend>Trackers</legend>
      ${trackers.map((t, i) => html`<input key=${i} type="text" aria-label=${"Tracker " + (i + 1)} value=${t}
        placeholder="Tracker" onInput=${(e) => setTracker(i, e.currentTarget.value)} />`)}
    </fieldset>
  </form>`;
}
```

- [ ] **Step 4: Write `static/files/js/components/SearchResults.js`**

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";

export function SearchResults({ results, hasMore, searching, onMore, onAdd }) {
  return html`<table class="results"><tbody>
    ${results.map((r, i) => html`<tr key=${i}>
      <td class="name">${r.url ? html`<a href=${r.url} target="_blank" rel="noopener noreferrer">${r.name}</a>` : r.name}</td>
      <td class="size">${r.size || ""}</td>
      <td class="users"><span class="seeds">${r.seeds || ""}</span> <span class="peers">${r.peers || ""}</span></td>
      <td class="controls">
        <button type="button" class="icon-btn go" aria-label=${"Add " + (r.name || "result")} onClick=${() => onAdd(r)}>
          <${Icon} name="download" />
        </button>
      </td>
    </tr>`)}
    ${hasMore && html`<tr><td colspan="4" class="more">
      <button type="button" onClick=${onMore} disabled=${searching}>${searching ? "Loading…" : "Load more"}</button>
    </td></tr>`}
  </tbody></table>`;
}
```

- [ ] **Step 5: Write `static/files/js/components/OmniBar.js`**

```js
import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { classify } from "../lib/omni.js";
import { parseMagnet, buildMagnet } from "../lib/magnet.js";
import { providerList, pickProvider, normalizeResults, resolveItem, resolveLookup } from "../lib/search.js";
import { MagnetEditor } from "./MagnetEditor.js";
import { SearchResults } from "./SearchResults.js";

const store = {
  get(k) {
    try { return localStorage.getItem(k) || ""; } catch { return ""; }
  },
  set(k, v) {
    try { localStorage.setItem(k, v); } catch { /* storage unavailable (private mode) */ }
  },
};
const BLANK = { name: "", infohash: "", trackers: [] };

export function OmniBar({ api, providers, editorOpen, setEditorOpen, onError }) {
  const [text, setText] = useState(() => store.get("tcOmni"));
  const [stored, setStored] = useState(() => store.get("tcProvider"));
  const [results, setResults] = useState([]);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [searching, setSearching] = useState(false);
  const [inputError, setInputError] = useState(null);
  const [dragging, setDragging] = useState(false);
  const fileInput = useRef(null);
  const latestQuery = useRef("");

  const mode = classify(text);
  const query = text.trim();
  const list = providerList(providers);
  const provider = pickProvider(list, stored);
  const magnet = mode === "magnet" ? parseMagnet(text) : null;
  const noResults = !hasMore && results.length === 0;
  latestQuery.current = query;

  useEffect(() => store.set("tcOmni", text), [text]);
  useEffect(() => {
    setResults([]);
    setPage(1);
    setHasMore(true);
    setInputError(null);
    if (mode === "empty") setEditorOpen(false);
  }, [query, provider]);

  const search = async () => {
    if (!provider || searching || !hasMore) return;
    const q = query;
    setSearching(true);
    try {
      const res = await api.search(provider, q, page);
      if (latestQuery.current !== q) return; // the person typed a new query meanwhile
      const found = Array.isArray(res) ? res : res ? [res] : [];
      if (found.length === 0) {
        setHasMore(false);
        return;
      }
      setResults((r) => r.concat(normalizeResults(found, providers[provider] && providers[provider].url)));
      setPage((p) => p + 1);
    } catch (e) {
      onError(e);
    } finally {
      setSearching(false);
    }
  };

  const submit = () => {
    setInputError(null);
    if (mode === "torrent-url") api.url(query).catch(onError);
    else if (mode === "magnet" && !magnet.error) api.magnet(query).catch(onError);
    else if (mode === "search") search();
  };

  const addResult = async (r) => {
    try {
      const how = resolveItem(r);
      if (how.error) throw new Error(how.error);
      if (how.kind === "magnet") return await api.magnet(how.value);
      if (how.kind === "url") return await api.url(how.value);
      const got = resolveLookup(await api.searchItem(provider, how.path), r.name);
      if (got.error) throw new Error(got.error);
      await (got.kind === "url" ? api.url(got.value) : api.magnet(got.value));
    } catch (e) {
      onError(e);
    }
  };

  const upload = async (files) => {
    const torrents = Array.from(files || []).filter((f) => f.name.toLowerCase().endsWith(".torrent"));
    if (torrents.length === 0) {
      setInputError("Only .torrent files can be uploaded");
      return;
    }
    setInputError(null);
    for (const f of torrents) {
      try {
        await api.torrentFile(new Uint8Array(await f.arrayBuffer()));
      } catch (e) {
        onError(e);
      }
    }
  };

  const icon = mode === "search" ? "search" : mode === "empty" ? "upload" : "magnet";
  const inlineError = inputError || (magnet && magnet.error);

  return html`<section class="omni">
    ${editorOpen && html`<${MagnetEditor} magnet=${magnet || BLANK} onChange=${(m) => setText(buildMagnet(m))} />`}
    <div
      class=${"omni-bar" + (dragging ? " drag" : "")}
      onDragOver=${(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave=${() => setDragging(false)}
      onDrop=${(e) => { e.preventDefault(); setDragging(false); upload(e.dataTransfer && e.dataTransfer.files); }}
    >
      <input class="omni-input" type="text" aria-label="Search, magnet link or torrent URL"
        placeholder="Enter search query, magnet URI, torrent URL or drop a torrent file here"
        value=${text} onInput=${(e) => setText(e.currentTarget.value)}
        onKeyDown=${(e) => { if (e.key === "Enter") submit(); }} />
      <button type="button" class="icon-btn" aria-label="Upload .torrent files" onClick=${() => fileInput.current.click()}>
        <${Icon} name=${icon} />
      </button>
      <input ref=${fileInput} type="file" accept=".torrent" multiple hidden
        onChange=${(e) => { upload(e.currentTarget.files); e.currentTarget.value = ""; }} />
    </div>
    ${inlineError && html`<p class="field-error" role="alert">${inlineError}</p>`}
    ${mode === "torrent-url" && html`<div class="actions">
      <button type="button" class="primary" onClick=${submit}>Load torrent</button>
    </div>`}
    ${mode === "magnet" && html`<div class="actions">
      <button type="button" class="primary" onClick=${submit}>Load magnet</button>
      <button type="button" class=${editorOpen ? "on" : ""} aria-pressed=${editorOpen} onClick=${() => setEditorOpen(!editorOpen)}>Edit</button>
    </div>`}
    ${mode === "search" && list.length === 0 && html`<p class="field-error">You have no search providers</p>`}
    ${mode === "search" && list.length > 0 && html`<div class="actions">
      <select aria-label="Search provider" value=${provider}
        onChange=${(e) => { setStored(e.currentTarget.value); store.set("tcProvider", e.currentTarget.value); }}>
        ${list.map((p) => html`<option key=${p.id} value=${p.id}>${p.name}</option>`)}
      </select>
      <button type="button" class="primary" onClick=${search} disabled=${searching || results.length > 0 || noResults}>
        ${noResults ? "No results" : searching && results.length === 0 ? "Searching…" : "Search"}
      </button>
    </div>`}
    ${mode === "search" && results.length > 0 && html`<${SearchResults} results=${results} hasMore=${hasMore}
      searching=${searching} onMore=${search} onAdd=${addResult} />`}
  </section>`;
}
```

- [ ] **Step 6: Mount it in `App.js`.** Add `import { OmniBar } from "./OmniBar.js";` after the `ErrorBanner` import. Then insert this line directly above the `${error && html\`<${ErrorBanner}` line:

```js
    <${OmniBar} api=${api} providers=${state.SearchProviders} editorOpen=${editorOpen} setEditorOpen=${setEditorOpen} onError=${report} />
```

- [ ] **Step 7: Run the e2e tests to verify they pass**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/shell.spec.mjs tests/omni.spec.mjs`
Expected: 11 passed.

- [ ] **Step 8: Mutation check.** One at a time, confirm each change fails the named test, then revert it:
  - In `normalizeResults`, replace `safeHttpUrl(out.url)` with `out.url`: the hostile-provider test fails.
  - In `upload`, drop the `.torrent` filter: the non-`.torrent` test fails.
  - In `OmniBar`, replace `pickProvider(list, stored)` with `stored`: the reload test fails.

- [ ] **Step 9: Checkpoint.** Unit tests and Go suite green. Leave uncommitted.

---

### Task 9: Torrent list

**Files:**
- Create: `static/files/js/components/TorrentList.js`, `static/files/js/components/FileTable.js`
- Modify: `static/files/js/components/App.js` (mount `TorrentList`)
- Test (harness): `/sessions/zen-kind-bohr/e2e/tests/torrents.spec.mjs`

**Interfaces:**
- Consumes: `sortTorrents` (Task 4); `bytes`, `filename` (Task 2); `api.torrent` (Task 6); `Icon`; `report` from `App`.
- Produces: `TorrentList({ torrents, api, onError })` and `FileTable({ files, size })`.

- [ ] **Step 1: Write the failing e2e tests** at `/sessions/zen-kind-bohr/e2e/tests/torrents.spec.mjs`

```js
import { test, expect } from "../fixtures.mjs";

const card = (page, name) => page.getByRole("article", { name, exact: true });

test("a torrent with its data on disk shows complete, with its files", async ({ page, app }) => {
  const t = app.makeTorrent("sample.bin", 300000);
  await app.addTorrent(t.torrent);
  const c = card(page, "sample.bin");
  await expect(c.getByText("#" + t.infohash)).toBeVisible();
  await expect(c.getByText("300 KB / 300 KB · 100%")).toBeVisible({ timeout: 15000 });
  await c.getByRole("button", { name: "Files" }).click();
  await expect(c.locator("table.files tbody")).toContainText("sample.bin");
  await expect(c.getByRole("img", { name: "Complete" })).toBeVisible();
  await expect(page.locator(".torrents .section-header")).toContainText("1 torrent");
});

test("stop pauses, start resumes, remove deletes", async ({ page, app }) => {
  const t = app.makeTorrent("cycle.bin", 50000);
  await app.addTorrent(t.torrent);
  const c = card(page, "cycle.bin");
  await expect(c.getByRole("button", { name: "Stop" })).toBeVisible();
  await expect(c.getByRole("button", { name: "Start" })).toBeDisabled();
  await c.getByRole("button", { name: "Stop" }).click();
  await expect(c.getByRole("button", { name: "Start" })).toBeEnabled();
  await expect(c.getByRole("button", { name: "Stop" })).toHaveCount(0);
  await c.getByRole("button", { name: "Start" }).click();
  await expect(c.getByRole("button", { name: "Stop" })).toBeVisible();
  await c.getByRole("button", { name: "Stop" }).click();
  await c.getByRole("button", { name: "Remove" }).click();
  await expect(card(page, "cycle.bin")).toHaveCount(0);
  await expect(page.getByText("Add torrents above")).toBeVisible();
});

test("a magnet without metadata shows Loading and can be cancelled through the overlay", async ({ page, app }) => {
  await app.addMagnet("magnet:?xt=urn:btih:" + "d".repeat(40) + "&dn=waiting");
  const c = card(page, "waiting");
  await expect(c.getByText("Loading")).toBeVisible();
  await c.getByRole("button", { name: "Stop" }).click();
  await c.getByRole("button", { name: "Cancel" }).click();
  await expect(card(page, "waiting")).toHaveCount(0);
});

test("cards sort by name and a folder torrent totals its files", async ({ page, app }) => {
  await app.addMagnet("magnet:?xt=urn:btih:" + "e".repeat(40) + "&dn=zeta");
  await app.addMagnet("magnet:?xt=urn:btih:" + "f".repeat(40) + "&dn=Alpha");
  const f = app.makeFolderTorrent("Show", [{ name: "e1.mkv", size: 40000 }, { name: "e2.mkv", size: 20000 }]);
  await app.addTorrent(f.torrent);
  await expect(page.locator("article.torrent .name")).toHaveText(["Alpha", "Show", "zeta"]);
  const c = card(page, "Show");
  await c.getByRole("button", { name: "Files" }).click();
  await expect(c.locator("table.files tbody tr")).toHaveCount(2);
  await expect(c.locator("table.files tfoot")).toContainText("2 files");
  await expect(c.locator("table.files tfoot")).toContainText("60 KB total");
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/torrents.spec.mjs`
Expected: 4 failed, because there are no torrent cards yet.

- [ ] **Step 3: Write `static/files/js/components/FileTable.js`**

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes, filename } from "../lib/format.js";

export function FileTable({ files, size }) {
  const list = (files || []).filter(Boolean).slice().sort((a, b) => a.Path.localeCompare(b.Path));
  return html`<table class="files">
    <thead><tr><th>File</th><th class="size">Size</th></tr></thead>
    <tbody>
      ${list.length === 0 && html`<tr><td colspan="2" class="muted">No files</td></tr>`}
      ${list.map((f) => html`<tr key=${f.Path}>
        <td class="name">
          <span>${filename(f.Path)}</span>
          ${f.Percent > 0 && f.Percent < 100 && html` <span class="pct">${f.Percent}%</span>
            <div class="progress thin"><div class="bar" style=${{ width: f.Percent + "%" }}></div></div>`}
        </td>
        <td class="size">${bytes(f.Size)} ${f.Percent === 100 && html`<${Icon} name="check" class="ok" label="Complete" />`}</td>
      </tr>`)}
    </tbody>
    ${list.length > 1 && html`<tfoot><tr><th>${list.length} files</th><th class="size">${bytes(size)} total</th></tr></tfoot>`}
  </table>`;
}
```

- [ ] **Step 4: Write `static/files/js/components/TorrentList.js`**

```js
import { useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes } from "../lib/format.js";
import { sortTorrents } from "../lib/tree.js";
import { FileTable } from "./FileTable.js";

function TorrentCard({ t, api, onError }) {
  const [showFiles, setShowFiles] = useState(false);
  const act = (action) => api.torrent(action, t.InfoHash).catch(onError);
  const pct = t.Percent || 0;
  return html`<article class="card torrent" aria-label=${t.Name || t.InfoHash}>
    ${!t.Loaded && html`<div class="overlay"><${Icon} name="loader" class="spin" /> Loading</div>`}
    <div class="torrent-top">
      <div class="info">
        <div class="name">${t.Name || t.InfoHash}</div>
        <div class="hash">#${t.InfoHash}</div>
      </div>
      <div class="buttons">
        <button type="button" class=${showFiles ? "on" : ""} aria-pressed=${showFiles} onClick=${() => setShowFiles(!showFiles)}>
          <${Icon} name="file" /> Files
        </button>
        <button type="button" class="go" disabled=${t.Started} onClick=${() => act("start")}><${Icon} name="play" /> Start</button>
        ${t.Started && html`<button type="button" class="danger" onClick=${() => act("stop")}><${Icon} name="stop" /> Stop</button>`}
        ${!t.Started && html`<button type="button" class="danger" onClick=${() => act("delete")}>
          <${Icon} name=${t.Loaded ? "trash" : "x"} /> ${t.Loaded ? "Remove" : "Cancel"}
        </button>`}
      </div>
    </div>
    <div class="progress" role="progressbar" aria-label="Progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow=${pct}>
      <div class="bar" style=${{ width: pct + "%" }}></div>
    </div>
    ${t.Started && html`<div class="status">
      ${bytes(t.Downloaded)} / ${bytes(t.Size)} · ${pct}% · <strong>${bytes(t.DownloadRate)}/s</strong>
    </div>`}
    ${showFiles && t.Loaded && html`<${FileTable} files=${t.Files} size=${t.Size} />`}
  </article>`;
}

export function TorrentList({ torrents, api, onError }) {
  const list = sortTorrents(torrents);
  return html`<section class="torrents">
    <div class="section-header">
      <h3>Torrents</h3>
      <span class="muted">${list.length} torrent${list.length === 1 ? "" : "s"}</span>
    </div>
    ${list.length === 0
      ? html`<p class="empty">Add torrents above</p>`
      : list.map((t) => html`<${TorrentCard} key=${t.InfoHash} t=${t} api=${api} onError=${onError} />`)}
  </section>`;
}
```

- [ ] **Step 5: Mount it in `App.js`.** Add `import { TorrentList } from "./TorrentList.js";` with the other imports. Then insert this line directly above the `<${Footer}` line:

```js
    <${TorrentList} torrents=${state.Torrents} api=${api} onError=${report} />
```

- [ ] **Step 6: Run the e2e tests to verify they pass**

Run: the Step 2 command, adding `tests/shell.spec.mjs tests/omni.spec.mjs` so earlier tests are rerun.
Expected: 15 passed.

- [ ] **Step 7: Checkpoint.** Leave uncommitted.

---

### Task 10: Downloads tree, preview and the Space shortcut

**Files:**
- Create: `static/files/js/components/DownloadTree.js`, `static/files/js/keys.js`
- Modify: `static/files/js/components/App.js` (mount `DownloadTree`), `static/files/js/main.js` (install the shortcut)
- Test (harness): `/sessions/zen-kind-bohr/e2e/tests/downloads.spec.mjs`

**Interfaces:**
- Consumes: `childPath`, `downloadHref`, `isDir`, `previewKind`, `fileIcon`, `findTorrentFile`, `isDownloading`, `startsClosed` (Task 4); `bytes`, `ago` (Task 2); `api.deleteDownload` (Task 6); `Icon`.
- Produces: `DownloadTree({ root, torrents, system, api, onError })` and `installSpaceToggle(doc?, win?)`.

- [ ] **Step 1: Write the failing e2e tests** at `/sessions/zen-kind-bohr/e2e/tests/downloads.spec.mjs`

```js
import { test, expect } from "../fixtures.mjs";
import { existsSync, mkdirSync, utimesSync, writeFileSync } from "node:fs";
import { join } from "node:path";

test("names with spaces, # and ? get links that fetch that exact file", async ({ page, app }) => {
  writeFileSync(join(app.dl, "notes #1 ?.txt"), "hello from notes");
  await expect(page.locator(".downloads .section-header")).toContainText(/[\d.]+ [KMGTP]?B free/);
  const link = page.getByRole("link", { name: "notes #1 ?.txt", exact: true });
  await expect(link).toHaveAttribute("href", "download/notes%20%231%20%3F.txt");
  const res = await page.request.get(app.server.base + "/download/notes%20%231%20%3F.txt");
  expect(await res.text()).toBe("hello from notes");
  await expect(page.getByText(/^16 B · updated /)).toBeVisible();
});

test("old folders start closed, open on click, and download as a zip", async ({ page, app }) => {
  mkdirSync(join(app.dl, "Archive"));
  writeFileSync(join(app.dl, "Archive", "inside.txt"), "x");
  const old = new Date(Date.now() - 3 * 86400e3);
  utimesSync(join(app.dl, "Archive"), old, old);
  const expand = page.getByRole("button", { name: "Expand Archive" });
  await expect(expand).toBeVisible();
  await expect(page.getByRole("link", { name: "inside.txt" })).toHaveCount(0);
  await expand.click();
  await expect(page.getByRole("link", { name: "inside.txt" })).toBeVisible();
  const href = await page.getByRole("link", { name: "Archive", exact: true }).getAttribute("href");
  const zip = await page.request.get(app.server.base + "/" + href);
  expect(zip.headers()["content-type"]).toBe("application/zip");
});

test("delete needs a second click within 3 seconds", async ({ page, app }) => {
  writeFileSync(join(app.dl, "old.iso"), "x");
  const del = page.getByRole("button", { name: "Delete old.iso", exact: true });
  const confirm = page.getByRole("button", { name: "Confirm delete old.iso", exact: true });
  await del.click();
  await expect(confirm).toBeVisible();
  await page.waitForTimeout(3500);
  await expect(confirm).toHaveCount(0);
  await del.click();
  await confirm.click();
  await expect(page.getByRole("link", { name: "old.iso", exact: true })).toHaveCount(0);
  expect(existsSync(join(app.dl, "old.iso"))).toBe(false);
});

test("a file still downloading shows a spinner with no link or delete", async ({ page, app }) => {
  const t = app.makeTorrent("half.bin", 300000, { partial: true });
  await app.addTorrent(t.torrent);
  await expect(page.getByRole("article", { name: "half.bin" })).toContainText(/\b4\d(\.\d+)?%/, { timeout: 15000 });
  const row = page.locator("li.node").filter({ hasText: "half.bin" });
  await expect(row).toBeVisible();
  await expect(row.getByRole("link", { name: "half.bin" })).toHaveCount(0);
  await expect(row.getByRole("button", { name: /Delete/ })).toHaveCount(0);
});

test("preview toggles, and Space plays or pauses only when no control has focus", async ({ page, app }) => {
  writeFileSync(join(app.dl, "song.mp3"), Buffer.alloc(1000));
  await page.getByRole("button", { name: "Preview song.mp3" }).click();
  const audio = page.locator("audio.preview");
  await expect(audio).toHaveAttribute("src", "download/song.mp3");
  await page.evaluate(() => {
    const a = document.querySelector("audio.preview");
    window.calls = [];
    Object.defineProperty(a, "paused", { configurable: true, get: () => true });
    a.play = () => { window.calls.push("play"); return Promise.resolve(); };
    a.pause = () => window.calls.push("pause");
    document.activeElement.blur();
  });
  await page.keyboard.press("Space");
  expect(await page.evaluate(() => window.calls)).toEqual(["play"]);
  await page.getByRole("textbox", { name: "Search, magnet link or torrent URL" }).focus();
  await page.keyboard.press("Space");
  expect(await page.evaluate(() => window.calls)).toEqual(["play"]);
  await page.getByRole("button", { name: "Hide preview of song.mp3" }).focus();
  await page.keyboard.press("Space");
  expect(await page.evaluate(() => window.calls)).toEqual(["play"]);
  await expect(audio).toHaveCount(0);
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/downloads.spec.mjs`
Expected: 5 failed, because there's no downloads section yet.

- [ ] **Step 3: Write `static/files/js/components/DownloadTree.js`**

```js
import { useEffect, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { ago, bytes } from "../lib/format.js";
import {
  childPath, downloadHref, fileIcon, findTorrentFile, isDir, isDownloading, previewKind, startsClosed,
} from "../lib/tree.js";

function Preview({ kind, src }) {
  if (kind === "audio") return html`<audio class="preview" controls src=${src}></audio>`;
  if (kind === "image") return html`<img class="preview" src=${src} alt="" />`;
  return html`<video class="preview" controls autoplay src=${src}></video>`;
}

function TreeNode({ node, path, torrents, api, onError }) {
  const dir = isDir(node);
  const [open, setOpen] = useState(() => !startsClosed(node));
  const [confirm, setConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [preview, setPreview] = useState(false);
  const kind = dir ? null : previewKind(path);
  const downloading = !dir && isDownloading(findTorrentFile(torrents, path));
  const href = downloadHref(path);

  useEffect(() => {
    if (!confirm) return undefined;
    const id = setTimeout(() => setConfirm(false), 3000);
    return () => clearTimeout(id);
  }, [confirm]);

  const remove = async () => {
    setDeleting(true);
    try {
      await api.deleteDownload(path);
    } catch (e) {
      setDeleting(false);
      setConfirm(false);
      onError(e);
    }
  };

  return html`<li class="node">
    <div class="row">
      ${dir
        ? html`<button type="button" class="icon-btn" aria-expanded=${open}
            aria-label=${(open ? "Collapse " : "Expand ") + node.Name} onClick=${() => setOpen(!open)}>
            <${Icon} name=${open ? "folderOpen" : "folder"} />
          </button>`
        : html`<${Icon} name=${downloading ? "loader" : fileIcon(path)} class=${downloading ? "spin" : ""} />`}
      ${downloading ? html`<span class="label">${node.Name}</span>` : html`<a class="label" href=${href}>${node.Name}</a>`}
      ${!downloading && html`<span class="controls">
        ${kind && html`<button type="button" class=${"icon-btn" + (preview ? " on" : "")} aria-pressed=${preview}
          aria-label=${(preview ? "Hide preview of " : "Preview ") + node.Name} onClick=${() => setPreview(!preview)}>
          <${Icon} name=${preview ? "x" : "play"} />
        </button>`}
        ${deleting
          ? html`<${Icon} name="loader" class="spin" label="Deleting" />`
          : confirm
            ? html`<button type="button" class="icon-btn danger" aria-label=${"Confirm delete " + node.Name} onClick=${remove}><${Icon} name="check" /></button>`
            : html`<button type="button" class="icon-btn danger" aria-label=${"Delete " + node.Name} onClick=${() => setConfirm(true)}><${Icon} name="trash" /></button>`}
      </span>`}
    </div>
    <div class="meta">${bytes(node.Size)} · updated ${ago(node.Modified)}</div>
    ${preview && kind && html`<${Preview} kind=${kind} src=${href} />`}
    ${dir && open && node.Children.length > 0 && html`<ul class="tree">
      ${node.Children.map((c) => html`<${TreeNode} key=${c.Name} node=${c} path=${childPath(path, c.Name)}
        torrents=${torrents} api=${api} onError=${onError} />`)}
    </ul>`}
  </li>`;
}

export function DownloadTree({ root, torrents, system, api, onError }) {
  const children = (root && root.Children) || [];
  const free = system && system.set ? `${bytes(system.diskTotal - system.diskUsed)} free` : "";
  return html`<section class="downloads">
    <div class="section-header"><h3>Downloads</h3><span class="muted">${free}</span></div>
    ${children.length === 0
      ? html`<p class="empty">Download files above</p>`
      : html`<ul class="tree">
          ${children.map((n) => html`<${TreeNode} key=${n.Name} node=${n} path=${n.Name} torrents=${torrents} api=${api} onError=${onError} />`)}
        </ul>`}
  </section>`;
}
```

- [ ] **Step 4: Write `static/files/js/keys.js`**

```js
const FOCUS_HANDLES_SPACE = /^(INPUT|TEXTAREA|SELECT|BUTTON|AUDIO|VIDEO)$/;

// Space plays or pauses the first audio/video on screen, unless a control
// has focus: a focused button, field or player already acts on Space itself.
export function installSpaceToggle(doc = document, win = window) {
  doc.addEventListener("keydown", (e) => {
    if (e.key !== " " || e.repeat) return;
    const el = doc.activeElement;
    if (el && FOCUS_HANDLES_SPACE.test(el.tagName)) return;
    const height = win.innerHeight;
    for (const media of doc.querySelectorAll("video, audio")) {
      const r = media.getBoundingClientRect();
      const inView = (r.top >= 0 && r.top <= height) || (r.bottom >= 0 && r.bottom <= height);
      if (!inView) continue;
      if (media.paused) media.play();
      else media.pause();
      e.preventDefault();
      return;
    }
  });
}
```

- [ ] **Step 5: Wire them up.**
  - In `main.js`, add `import { installSpaceToggle } from "./keys.js";` after the other imports, and `installSpaceToggle();` after `startSync();`.
  - In `App.js`, add `import { DownloadTree } from "./DownloadTree.js";`, then insert this line directly above the `<${Footer}` line:

```js
    <${DownloadTree} root=${state.Downloads} torrents=${state.Torrents} system=${stats.System} api=${api} onError=${report} />
```

- [ ] **Step 6: Run the e2e tests to verify they pass**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test`
Expected: 20 passed.

- [ ] **Step 7: Mutation check.** One at a time, confirm each change fails the named test, then revert it:
  - In `keys.js`, remove `BUTTON` from the regex: the Space test fails.
  - In `downloadHref`, drop `.map(encodeURIComponent)`: the `notes #1 ?.txt` test fails.
  - In `TreeNode`, replace the confirm timeout `3000` with `60000`: the delete test fails.

- [ ] **Step 8: Checkpoint.** Leave uncommitted.

---

### Task 11: Settings form and reconnect

**Files:**
- Create: `static/files/js/components/ConfigForm.js`
- Modify: `static/files/js/components/App.js` (mount `ConfigForm`)
- Test (harness): `/sessions/zen-kind-bohr/e2e/tests/config.spec.mjs`

**Interfaces:**
- Consumes: `addSpaces`, `inputType` (Task 2); `api.configure` (Task 6); `configOpen`/`setConfigOpen` and `report` from `App`; the reconnect strip from Task 7.
- Produces: `ConfigForm({ config, api, onError, onClose })`.

- [ ] **Step 1: Write the failing e2e tests** at `/sessions/zen-kind-bohr/e2e/tests/config.spec.mjs`

```js
import { test, expect } from "../fixtures.mjs";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const openSettings = (page) => page.getByRole("button", { name: "Settings" }).click();

test("the settings form shows every config value with readable labels", async ({ page }) => {
  await openSettings(page);
  for (const label of ["Auto Start", "Disable Encryption", "Enable Upload", "Enable Seeding"]) {
    await expect(page.getByRole("checkbox", { name: label, exact: true })).toBeVisible();
  }
  await expect(page.getByRole("spinbutton", { name: "Incoming Port" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Download Directory" })).toBeVisible();
});

test("cancel discards the draft", async ({ page }) => {
  await openSettings(page);
  const port = page.getByRole("spinbutton", { name: "Incoming Port" });
  const before = await port.inputValue();
  await port.fill("1234");
  await page.getByRole("button", { name: "Cancel" }).click();
  await openSettings(page);
  await expect(page.getByRole("spinbutton", { name: "Incoming Port" })).toHaveValue(before);
});

test("live updates don't overwrite what's being typed", async ({ page }) => {
  await openSettings(page);
  const dir = page.getByRole("textbox", { name: "Download Directory" });
  await dir.fill("/tmp/typing-in-progress");
  await page.waitForTimeout(2500);
  await expect(dir).toHaveValue("/tmp/typing-in-progress");
});

test("save sends the draft and closes the form", async ({ page, app }) => {
  await openSettings(page);
  await page.getByRole("checkbox", { name: "Enable Seeding", exact: true }).check();
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByRole("button", { name: "Save" })).toHaveCount(0);
  await expect.poll(() => JSON.parse(readFileSync(join(app.dir, "cloud-fetch.json"), "utf8")).EnableSeeding).toBe(true);
});

test("losing the server shows Reconnecting and recovers", async ({ page, app }) => {
  await app.stop();
  await expect(page.getByText("Reconnecting…")).toBeVisible({ timeout: 15000 });
  await expect(page.getByRole("img", { name: "Disconnected" })).toBeVisible();
  await app.restart();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible({ timeout: 20000 });
  await expect(page.getByText("Reconnecting…")).toHaveCount(0);
  app.errors.length = 0; // the browser logs refused connections while the server was down
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/config.spec.mjs`
Expected: the four form tests fail, because there's no form yet. The reconnect test should already pass, since Task 7 added the strip. If it doesn't, fix the strip in `App.js` before continuing.

- [ ] **Step 3: Write `static/files/js/components/ConfigForm.js`**

```js
import { useState } from "preact/hooks";
import { html } from "../html.js";
import { addSpaces, inputType } from "../lib/format.js";

// Edits a copy taken when the form opens, so the server's once-a-second
// pushes can't overwrite typing and Cancel discards everything.
export function ConfigForm({ config, api, onError, onClose }) {
  const [draft, setDraft] = useState(() => ({ ...config }));
  const [saving, setSaving] = useState(false);
  const set = (k, v) => setDraft((d) => ({ ...d, [k]: v }));

  const save = async () => {
    setSaving(true);
    try {
      await api.configure(draft);
      onClose();
    } catch (e) {
      onError(e);
    } finally {
      setSaving(false);
    }
  };

  return html`<form class="card config" onSubmit=${(e) => { e.preventDefault(); save(); }}>
    <h4>Configuration</h4>
    ${Object.keys(draft).map((k) => {
      const v = draft[k];
      const type = inputType(v);
      if (type === "checkbox") {
        return html`<label class="check" key=${k}>
          <input type="checkbox" checked=${v} onChange=${(e) => set(k, e.currentTarget.checked)} /> ${addSpaces(k)}
        </label>`;
      }
      return html`<label key=${k}>${addSpaces(k)}
        <input type=${type} value=${v}
          onInput=${(e) => set(k, type === "number" ? Number(e.currentTarget.value) : e.currentTarget.value)} />
      </label>`;
    })}
    <div class="actions">
      <button type="submit" class="primary" disabled=${saving}>${saving ? "Saving…" : "Save"}</button>
      <button type="button" onClick=${onClose}>Cancel</button>
    </div>
  </form>`;
}
```

- [ ] **Step 4: Mount it in `App.js`.** Add `import { ConfigForm } from "./ConfigForm.js";`. Then insert this line directly below the `${everConnected && !connected && ...Reconnecting…` line:

```js
    ${configOpen && state.Config && html`<${ConfigForm} config=${state.Config} api=${api} onError=${report} onClose=${() => setConfigOpen(false)} />`}
```

- [ ] **Step 5: Run the full e2e suite to verify it passes**

Run: `. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch && go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test`
Expected: 25 passed.

- [ ] **Step 6: Checkpoint.** Leave uncommitted.

---

### Task 12: CI, README, layout check and final verification

**Files:**
- Modify: `.github/workflows/ci.yml` (Node unit-test step)
- Modify: `README.md` (Development section)
- Test (harness): `/sessions/zen-kind-bohr/e2e/tests/layout.spec.mjs`

- [ ] **Step 1: Write the layout e2e test** at `/sessions/zen-kind-bohr/e2e/tests/layout.spec.mjs`

```js
import { test, expect } from "../fixtures.mjs";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

const LONG = "Very.Long.Release.Name.Without.Any.Spaces.".repeat(4) + "1080p.x265";

for (const [label, width, height] of [["desktop", 1280, 900], ["phone", 390, 844]]) {
  test(`${label}: long names wrap without sideways scrolling`, async ({ page, app }) => {
    await page.setViewportSize({ width, height });
    const t = app.makeTorrent(LONG + ".mkv", 30000);
    await app.addTorrent(t.torrent);
    await app.addMagnet("magnet:?xt=urn:btih:" + "9".repeat(40) + "&dn=" + LONG);
    writeFileSync(join(app.dl, LONG + ".nfo"), "x");
    await expect(page.getByRole("article")).toHaveCount(2);
    await page.getByRole("article", { name: LONG + ".mkv", exact: true }).getByRole("button", { name: "Files" }).click();
    await expect(page.getByRole("link", { name: LONG + ".nfo" })).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    await page.screenshot({ path: `shots/${label}.png`, fullPage: true });
  });
}
```

- [ ] **Step 2: Run it**

Run: `cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test tests/layout.spec.mjs`
Expected: 2 passed. If `overflow` is positive, find the element wider than the viewport with `page.evaluate` (walk `document.querySelectorAll("*")`, compare `getBoundingClientRect().right` to `innerWidth`), fix it with `overflow-wrap: anywhere` or `min-width: 0` in `app.css`, and rerun.

- [ ] **Step 3: Review the screenshots.** Open `shots/desktop.png` and `shots/phone.png` with the Read tool. Compare against the approved layout in the spec's "Look" section. Check the order (header, omni bar, torrents, downloads, footer), one blue accent, green start and red stop/delete buttons, wrapped names, and buttons wrapping below the name on the phone. Fix anything off in `app.css` and rerun Step 2.

- [ ] **Step 4: Add the Node step to CI.** In `.github/workflows/ci.yml`, insert after the `Test` step of the `test` job:

```yaml
      - name: Set up Node
        uses: actions/setup-node@v7
        with:
          node-version: "24"
      - name: UI unit tests
        run: node --test "static/files/js/**/*.test.mjs"
```

Then lint it: `cd /tmp && . ~/goenv.sh && GOFLAGS= go run github.com/rhysd/actionlint/cmd/actionlint@latest /sessions/zen-kind-bohr/mnt/cloud-fetch/.github/workflows/ci.yml`
Expected: no output.

- [ ] **Step 5: Add a Development section to `README.md`,** directly above `### Credits`:

````markdown
### Development

The web UI is plain ES modules in `static/files/`, with Preact and htm vendored in `static/files/js/vendor/` (versions and checksums in `VENDOR.md`). There is no build step: edit a file, run `go build`, reload the page. The UI logic has unit tests that run on Node 22 or later:

``` sh
node --test "static/files/js/**/*.test.mjs"
```
````

- [ ] **Step 6: Check the page weight**

```bash
cd /sessions/zen-kind-bohr/mnt/cloud-fetch/static/files
find . -type f ! -name '*.test.mjs' ! -name 'VENDOR.md' ! -name '*.png' -print0 | xargs -0 cat | wc -c
```

Expected: under 50,000 bytes. With velox.js (about 10 KB, served by Go) the page stays under the spec's 60 KB. Report both numbers.

- [ ] **Step 7: Full verification**

```bash
. ~/goenv.sh && cd /sessions/zen-kind-bohr/mnt/cloud-fetch
gofmt -l . && go vet ./... && go test -count=1 -race ./...
node --test "static/files/js/**/*.test.mjs"
go build -o /tmp/cf-e2e . && cd /sessions/zen-kind-bohr/e2e && . ./env.sh && npx playwright test
cd /sessions/zen-kind-bohr/mnt/cloud-fetch
grep -rn "alert(\|console.log" static/files/js --include=*.js | grep -v vendor   # expect nothing
git --no-optional-locks status -s && git --no-optional-locks diff --stat
```

Expected: `gofmt` prints nothing, Go and unit tests pass, all 27 e2e tests pass, and the grep finds nothing. The status shows only the files in this plan's File Structure.

- [ ] **Step 8: Hand-off.** Leave everything uncommitted. Record the result in the Cloud Fetch workstation `MEMORY.md`. Write the PR description to `Cloud Fetch Resources/pr-web-ui-rewrite.md` in Leonard's voice (read `00_Resources/voice-principles.md` first), and give him the commit and PR commands, using `--repo LeonardCooray/cloud-fetch`.
