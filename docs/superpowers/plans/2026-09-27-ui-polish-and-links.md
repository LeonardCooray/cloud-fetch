# UI Polish and Copy Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One-tap Copy link / Copy all on download rows, status badges with Pause/Resume and an ETA on torrent cards, dark mode with AA contrast, a phone-friendly layout, and a Playwright e2e suite in the repo.

**Architecture:** UI-only changes to the vendored Preact + htm app in `static/files/`. Pure logic (status, ETA, link building, clipboard fallback) goes into small `js/lib/` modules with `node --test` unit tests; components stay thin. A new dev-only `e2e/` folder (its own npm lockfile) builds the Go binary, runs it offline against temp dirs and a fake search provider, and drives Chromium.

**Tech Stack:** Preact 10.29.8 + htm 3.1.1 (vendored, no build), plain CSS custom properties, Node 24 `node:test`, Playwright (Chromium), Go (only to build the binary under test), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-27-ui-polish-and-links-design.md`

## Global Constraints

- UI only: no Go server, endpoint or JSON shape changes.
- The shipped UI stays build-free and npm-free; nothing under `e2e/` is embedded or needed by `go install`.
- Torrents and Downloads stay two separate sections.
- Links are plain `https://host/download/<path>` (absolute, per-segment encoded), never with credentials.
- Copy must work on plain HTTP (no `navigator.clipboard` outside secure contexts).
- No VLC deep links. No manual dark-mode toggle. Desktop width stays 880px.
- Every text/background token pair used for text must reach 4.5:1 in both themes.
- UI copy is sentence case: "Pause", "Resume", "Copied", "Copied 4", "waiting for peers".
- Run everything from the repo root `/Users/leonard/src/cloud-fetch` unless a step says `e2e/`.
- Commit after each task on branch `feature/ui-polish-links`, message ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Never push to `master`.
- Go is needed for Tasks 5–9 (`go build`). If `go version` fails, stop and ask Leonard to install it (`brew install go`); don't download it yourself.

## Review Focus

1. **A folder whose torrent is still writing some files**: those rows show no copy button and Copy all leaves them out, while finished siblings copy normally (Task 7 e2e `mixedShow`: "Copied 1", E02 has no button).
2. **File and folder names with spaces, `#`, `?`, `%` or non-ASCII** produce links that IDM/VLC fetch exactly (Task 2 unit test for `absoluteHref` with such names; Task 7 e2e uses a folder with a space).
3. **Cloud Fetch behind a subpath or on a non-default port** copies links with that prefix and port (Task 2 unit test with a subpath base).
4. **Clipboard blocked entirely** (insecure context and `execCommand` refused) never fails silently: the manual popover opens with the text selected (Task 3 unit test, Task 7 e2e).
5. **A zero or missing rate, size or file list** never renders NaN, "Infinity" or a negative ETA (Task 1 unit tests).

---

### Task 1: Status and ETA helpers

**Files:**
- Create: `static/files/js/lib/status.js`
- Test: `static/files/js/lib/status.test.mjs`

**Interfaces:**
- Consumes: `bytes(n)` from `static/files/js/lib/format.js` (returns e.g. `"3.1 GB"`, `"0 B"` for 0/invalid).
- Produces:
  - `STATUS_LABELS: { loading, downloading, seeding, done, paused }` → display strings.
  - `torrentStatus(t) -> "loading" | "downloading" | "seeding" | "done" | "paused"`.
  - `eta(t) -> string | null` (e.g. `"about 14 min left"`).
  - `statusLine(t) -> { main: string, rate: string | null, note: string | null }`.

- [ ] **Step 1: Write the failing test**

Create `static/files/js/lib/status.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { STATUS_LABELS, torrentStatus, eta, statusLine } from "./status.js";

const GB = 1e9;
const MB = 1e6;
const t = (over) => ({ Loaded: true, Started: true, Percent: 0, Size: 0, Downloaded: 0, DownloadRate: 0, ...over });

test("torrentStatus follows the spec's rule table", () => {
  assert.equal(torrentStatus(t({ Loaded: false })), "loading");
  assert.equal(torrentStatus(t({ Started: true, Percent: 100 })), "seeding");
  assert.equal(torrentStatus(t({ Started: true, Percent: 42.5 })), "downloading");
  assert.equal(torrentStatus(t({ Started: false, Percent: 100 })), "done");
  assert.equal(torrentStatus(t({ Started: false, Percent: 63 })), "paused");
});

test("torrentStatus tolerates missing fields", () => {
  assert.equal(torrentStatus(null), "loading");
  assert.equal(torrentStatus({}), "loading");
  assert.equal(torrentStatus({ Loaded: true }), "paused");
  assert.equal(torrentStatus({ Loaded: true, Started: true }), "downloading");
});

test("every status has a sentence-case label", () => {
  assert.deepEqual(STATUS_LABELS, {
    loading: "Loading", downloading: "Downloading", seeding: "Seeding", done: "Done", paused: "Paused",
  });
});

test("eta formats the remaining time in friendly units", () => {
  const at = (secondsLeft) => eta(t({ Size: 1000 + secondsLeft * MB, Downloaded: 1000, DownloadRate: MB }));
  assert.equal(at(30), "less than a minute left");
  assert.equal(at(59), "less than a minute left");
  assert.equal(at(60), "about 1 min left");
  assert.equal(at(14 * 60), "about 14 min left");
  assert.equal(at(3590), "about 1 h left"); // rounds to 60 min
  assert.equal(at(2 * 3600 + 20 * 60), "about 2 h 20 min left");
  assert.equal(at(3 * 3600), "about 3 h left");
  assert.equal(at(10 * 3600 + 20 * 60), "about 10 h left"); // minutes dropped from 10 h
  assert.equal(at(23 * 3600 + 59 * 60 + 40), "more than a day left"); // rounds to 1440 min
  assert.equal(at(3 * 86400), "more than a day left");
});

test("eta is null when it can't be estimated", () => {
  assert.equal(eta(t({ Size: 10 * MB, Downloaded: 0, DownloadRate: 0 })), null);
  assert.equal(eta(t({ Size: 0, Downloaded: 0, DownloadRate: MB })), null);
  assert.equal(eta(t({ Size: 10, Downloaded: 20, DownloadRate: MB })), null);
  assert.equal(eta(t({ Started: false, Size: 10 * MB, DownloadRate: MB })), null);
  assert.equal(eta(t({ Percent: 100, Size: 10 * MB, DownloadRate: MB })), null);
  assert.equal(eta({ Loaded: true, Started: true, DownloadRate: MB }), null);
});

test("statusLine while downloading shows progress, rate and ETA", () => {
  assert.deepEqual(
    statusLine(t({ Size: 7.4 * GB, Downloaded: 3.1 * GB, Percent: 41.89, DownloadRate: 5.2 * MB })),
    { main: "3.1 GB of 7.4 GB · 41%", rate: "5.2 MB/s", note: "about 14 min left" },
  );
});

test("statusLine at 0 B/s says it is waiting for peers", () => {
  assert.deepEqual(
    statusLine(t({ Size: 64000, Downloaded: 0, Percent: 0 })),
    { main: "0 B of 64 KB · 0%", rate: "0 B/s", note: "waiting for peers" },
  );
});

test("statusLine for complete, paused and loading torrents", () => {
  assert.deepEqual(statusLine(t({ Started: true, Percent: 100, Size: 12 * MB, Downloaded: 12 * MB })),
    { main: "12 MB · complete", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Started: false, Percent: 100, Size: 12 * MB })),
    { main: "12 MB · complete", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Started: false, Percent: 63.2, Size: 2.1 * GB, Downloaded: 1.3 * GB })),
    { main: "1.3 GB of 2.1 GB · 63%", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Loaded: false })), { main: "", rate: null, note: null });
});

test("statusLine never shows 100% for an unfinished torrent", () => {
  assert.equal(statusLine(t({ Percent: 99.99, Size: 100, Downloaded: 99 })).main, "99 B of 100 B · 99%");
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test static/files/js/lib/status.test.mjs`
Expected: FAIL with `Cannot find module` for `./status.js`.

- [ ] **Step 3: Write minimal implementation**

Create `static/files/js/lib/status.js`:

```js
import { bytes } from "./format.js";

export const STATUS_LABELS = {
  loading: "Loading",
  downloading: "Downloading",
  seeding: "Seeding",
  done: "Done",
  paused: "Paused",
};

// torrentStatus derives a torrent's state from the fields the server already
// sends; a complete torrent that is still started is seeding.
export function torrentStatus(t) {
  if (!t || !t.Loaded) return "loading";
  const complete = (t.Percent || 0) >= 100;
  if (t.Started) return complete ? "seeding" : "downloading";
  return complete ? "done" : "paused";
}

export function eta(t) {
  if (torrentStatus(t) !== "downloading") return null;
  const rate = t.DownloadRate || 0;
  const left = (t.Size || 0) - (t.Downloaded || 0);
  if (!(rate > 0) || !(left > 0)) return null;
  const seconds = left / rate;
  if (seconds < 60) return "less than a minute left";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `about ${minutes} min left`;
  if (minutes >= 1440) return "more than a day left";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h >= 10 || m === 0) return `about ${h} h left`;
  return `about ${h} h ${m} min left`;
}

// statusLine splits the card's status text so the rate can be emphasised.
// Percent is floored so an unfinished torrent never reads 100%.
export function statusLine(t) {
  const status = torrentStatus(t);
  if (status === "loading") return { main: "", rate: null, note: null };
  if (status === "seeding" || status === "done") return { main: `${bytes(t.Size)} · complete`, rate: null, note: null };
  const main = `${bytes(t.Downloaded)} of ${bytes(t.Size)} · ${Math.floor(t.Percent || 0)}%`;
  if (status === "paused") return { main, rate: null, note: null };
  const rate = t.DownloadRate || 0;
  return { main, rate: `${bytes(rate)}/s`, note: rate > 0 ? eta(t) : "waiting for peers" };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `node --test static/files/js/lib/status.test.mjs`
Expected: PASS, all 9 tests.

- [ ] **Step 5: Run the whole unit suite**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: PASS, no failures.

- [ ] **Step 6: Commit**

```bash
git add static/files/js/lib/status.js static/files/js/lib/status.test.mjs
git commit -m "Add torrent status and ETA helpers

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Absolute links and finished-file collection

**Files:**
- Modify: `static/files/js/lib/tree.js` (append two exports)
- Test: `static/files/js/lib/tree.test.mjs` (extend the import, append tests)

**Interfaces:**
- Consumes (same file): `downloadHref(path)`, `childPath(parent, name)`, `isDir(node)`, `findTorrentFile(torrents, path)`, `isDownloading(match)`.
- Produces:
  - `absoluteHref(path: string, base: string) -> string` — absolute URL of the file's `/download/` link resolved against `base` (the page URL).
  - `finishedFiles(node, path: string, torrents) -> string[]` — download-relative paths of every file under `node` (or `node` itself if it's a file) that is not still downloading, in tree order.

- [ ] **Step 1: Write the failing test**

In `static/files/js/lib/tree.test.mjs`, change the import to:

```js
import {
  childPath, downloadHref, isDir, previewKind, fileIcon,
  findTorrentFile, isDownloading, startsClosed, sortTorrents,
  absoluteHref, finishedFiles,
} from "./tree.js";
```

Append:

```js
test("absoluteHref resolves against the page, keeping port and subpath", () => {
  assert.equal(absoluteHref("Show/E01.mkv", "http://127.0.0.1:3000/"), "http://127.0.0.1:3000/download/Show/E01.mkv");
  assert.equal(absoluteHref("a.iso", "https://fetch.example.com/cf/"), "https://fetch.example.com/cf/download/a.iso");
  assert.equal(absoluteHref("a.iso", "https://fetch.example.com/cf/?x=1#top"), "https://fetch.example.com/cf/download/a.iso");
});

test("absoluteHref encodes awkward names so IDM and VLC fetch the exact file", () => {
  assert.equal(
    absoluteHref("My Show/e 1 #2?.mkv", "https://h/"),
    "https://h/download/My%20Show/e%201%20%232%3F.mkv",
  );
  assert.equal(absoluteHref("Café/ü 100%.mp3", "https://h/"), "https://h/download/Caf%C3%A9/%C3%BC%20100%25.mp3");
});

const tree = {
  Name: "Show",
  Children: [
    { Name: "E01.mkv", Children: null },
    { Name: "Extras", Children: [{ Name: "b.txt", Children: null }, { Name: "Empty", Children: [] }] },
    { Name: "E02.mkv", Children: null },
  ],
};
const torrent = (files, over = {}) => ({ h: { Loaded: true, Started: true, Files: files, ...over } });

test("finishedFiles walks a folder in tree order", () => {
  assert.deepEqual(finishedFiles(tree, "Show", {}), ["Show/E01.mkv", "Show/Extras/b.txt", "Show/E02.mkv"]);
});

test("finishedFiles skips files a started torrent is still writing", () => {
  const torrents = torrent([{ Path: "Show/E01.mkv", Percent: 100 }, { Path: "Show/E02.mkv", Percent: 50 }]);
  assert.deepEqual(finishedFiles(tree, "Show", torrents), ["Show/E01.mkv", "Show/Extras/b.txt"]);
});

test("finishedFiles keeps a paused torrent's partial files, as their links show", () => {
  const torrents = torrent([{ Path: "Show/E02.mkv", Percent: 50 }], { Started: false });
  assert.deepEqual(finishedFiles(tree, "Show", torrents), ["Show/E01.mkv", "Show/Extras/b.txt", "Show/E02.mkv"]);
});

test("finishedFiles on a single file", () => {
  assert.deepEqual(finishedFiles({ Name: "a.iso", Children: null }, "a.iso", {}), ["a.iso"]);
  const busy = torrent([{ Path: "a.iso", Percent: 10 }]);
  assert.deepEqual(finishedFiles({ Name: "a.iso", Children: null }, "a.iso", busy), []);
});

test("finishedFiles on an empty folder", () => {
  assert.deepEqual(finishedFiles({ Name: "Empty", Children: [] }, "Empty", {}), []);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test static/files/js/lib/tree.test.mjs`
Expected: FAIL — `absoluteHref` / `finishedFiles` are not exported (SyntaxError on the import).

- [ ] **Step 3: Write minimal implementation**

Append to `static/files/js/lib/tree.js`:

```js
// absoluteHref is what Copy link puts on the clipboard: the same link the
// row shows, made absolute so IDM and VLC can use it outside the page.
export function absoluteHref(path, base) {
  return new URL(downloadHref(path), base).href;
}

// finishedFiles lists the files Copy all should copy: every file under node
// whose link is shown, i.e. not one a started torrent is still writing.
export function finishedFiles(node, path, torrents) {
  if (!isDir(node)) return isDownloading(findTorrentFile(torrents, path)) ? [] : [path];
  return node.Children.flatMap((c) => finishedFiles(c, childPath(path, c.Name), torrents));
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `node --test static/files/js/lib/tree.test.mjs`
Expected: PASS, including the 7 new tests.

- [ ] **Step 5: Commit**

```bash
git add static/files/js/lib/tree.js static/files/js/lib/tree.test.mjs
git commit -m "Add absolute download links and finished-file collection

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Clipboard with an HTTP-safe fallback

**Files:**
- Create: `static/files/js/lib/clipboard.js`
- Test: `static/files/js/lib/clipboard.test.mjs`

**Interfaces:**
- Produces:
  - `copyText(text: string, env?) -> Promise<"copied" | "manual">` — never rejects.
  - `browserEnv() -> { secure: boolean, clipboard, document }` — the real browser environment; the default for `env`.

- [ ] **Step 1: Write the failing test**

Create `static/files/js/lib/clipboard.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { copyText } from "./clipboard.js";

function fakeDoc(exec = () => true) {
  const calls = [];
  const button = { focused: 0, focus() { this.focused++; } };
  const doc = {
    activeElement: button,
    body: {
      children: [],
      appendChild(el) { this.children.push(el); },
      removeChild(el) { this.children = this.children.filter((c) => c !== el); },
    },
    createElement(tag) {
      return {
        tag, value: "", style: {}, attrs: {}, selected: false,
        setAttribute(k, v) { this.attrs[k] = v; },
        focus() { doc.activeElement = this; },
        select() { this.selected = true; },
      };
    },
    execCommand(cmd) {
      const area = doc.body.children[0];
      calls.push({ cmd, tag: area && area.tag, value: area && area.value, selected: area && area.selected });
      return exec();
    },
  };
  return { doc, calls, button };
}

function fakeClipboard(fail = false) {
  const written = [];
  return {
    written,
    async writeText(text) {
      if (fail) throw new Error("NotAllowedError");
      written.push(text);
    },
  };
}

test("a secure page uses the async clipboard API", async () => {
  const clipboard = fakeClipboard();
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("https://h/download/a", { secure: true, clipboard, document: doc }), "copied");
  assert.deepEqual(clipboard.written, ["https://h/download/a"]);
  assert.equal(calls.length, 0);
});

test("a plain-HTTP page copies through a hidden textarea", async () => {
  const clipboard = fakeClipboard();
  const { doc, calls, button } = fakeDoc();
  assert.equal(await copyText("a\nb", { secure: false, clipboard, document: doc }), "copied");
  assert.deepEqual(clipboard.written, []);
  assert.deepEqual(calls, [{ cmd: "copy", tag: "textarea", value: "a\nb", selected: true }]);
  assert.equal(doc.body.children.length, 0, "textarea removed");
  assert.equal(button.focused, 1, "focus returns to the button");
});

test("a rejected clipboard write falls back to the textarea", async () => {
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("x", { secure: true, clipboard: fakeClipboard(true), document: doc }), "copied");
  assert.equal(calls.length, 1);
});

test("a missing clipboard API falls back to the textarea", async () => {
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("x", { secure: true, clipboard: undefined, document: doc }), "copied");
  assert.equal(calls.length, 1);
});

test("a refused execCommand asks for a manual copy", async () => {
  const { doc } = fakeDoc(() => false);
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: doc }), "manual");
  assert.equal(doc.body.children.length, 0);
});

test("a throwing execCommand asks for a manual copy", async () => {
  const { doc } = fakeDoc(() => { throw new Error("SecurityError"); });
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: doc }), "manual");
  assert.equal(doc.body.children.length, 0);
});

test("no document at all asks for a manual copy", async () => {
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: undefined }), "manual");
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test static/files/js/lib/clipboard.test.mjs`
Expected: FAIL with `Cannot find module` for `./clipboard.js`.

- [ ] **Step 3: Write minimal implementation**

Create `static/files/js/lib/clipboard.js`:

```js
// copyText never rejects: it resolves "copied", or "manual" when the caller
// should show the text for copying by hand. navigator.clipboard only exists
// in secure contexts and Cloud Fetch often runs on plain HTTP, so the old
// execCommand route is the fallback.
export async function copyText(text, env = browserEnv()) {
  if (env.secure && env.clipboard && env.clipboard.writeText) {
    try {
      await env.clipboard.writeText(text);
      return "copied";
    } catch {
      // permission refused or document not focused; try the old way
    }
  }
  return legacyCopy(text, env.document) ? "copied" : "manual";
}

function legacyCopy(text, doc) {
  if (!doc || !doc.body) return false;
  const previous = doc.activeElement;
  const area = doc.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "0";
  area.style.left = "-9999px";
  area.style.opacity = "0";
  doc.body.appendChild(area);
  try {
    area.focus();
    area.select();
    return doc.execCommand("copy") === true;
  } catch {
    return false;
  } finally {
    doc.body.removeChild(area);
    if (previous && previous.focus) previous.focus();
  }
}

export function browserEnv() {
  return { secure: window.isSecureContext, clipboard: navigator.clipboard, document };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `node --test static/files/js/lib/clipboard.test.mjs`
Expected: PASS, all 7 tests.

- [ ] **Step 5: Commit**

```bash
git add static/files/js/lib/clipboard.js static/files/js/lib/clipboard.test.mjs
git commit -m "Add clipboard helper that works on plain HTTP

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Colour tokens, dark mode and a contrast test

**Files:**
- Modify: `static/files/css/app.css` (the `:root` block at lines 1–18, `button.primary` at line 34, `.overlay` at ~line 125)
- Modify: `static/files/index.html` (add two metas in `<head>`)
- Test: `static/files/js/lib/contrast.test.mjs` (create)

**Interfaces:**
- Produces CSS tokens used by later tasks: `--ok-bg`, `--warn`, `--warn-bg`, `--on-accent`, `--overlay`, all defined in light and dark.

- [ ] **Step 1: Write the failing test**

Create `static/files/js/lib/contrast.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../../css/app.css", import.meta.url), "utf8");

function block(from) {
  const start = css.indexOf(":root", from);
  assert.ok(start >= 0, "no :root block found");
  return css.slice(start, css.indexOf("}", start));
}
function tokens(text) {
  const out = {};
  for (const m of text.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})\b/g)) out[m[1]] = m[2].toLowerCase();
  return out;
}
const light = tokens(block(0));
const darkAt = css.indexOf("@media (prefers-color-scheme: dark)");
const dark = darkAt >= 0 ? tokens(block(darkAt)) : {};

function luminance(hex) {
  const n = parseInt(hex.slice(1), 16);
  const [r, g, b] = [16, 8, 0].map((s) => {
    const c = ((n >> s) & 255) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
function ratio(a, b) {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

const TEXT = ["text", "muted", "faint", "accent", "ok", "danger", "warn"];
const PAIRS = [
  ...TEXT.flatMap((fg) => [[fg, "surface"], [fg, "bg"]]),
  ["accent", "accent-bg"], ["ok", "ok-bg"], ["danger", "danger-bg"], ["warn", "warn-bg"], ["muted", "bg"],
  ["on-accent", "accent"],
];

test("the dark theme defines every light colour token", () => {
  assert.ok(darkAt >= 0, "no prefers-color-scheme: dark block");
  assert.deepEqual(Object.keys(dark).sort(), Object.keys(light).sort());
});

for (const [name, theme] of [["light", light], ["dark", dark]]) {
  test(`${name} theme text pairs reach WCAG AA (4.5:1)`, () => {
    const failing = [];
    for (const [fg, bg] of PAIRS) {
      assert.ok(theme[fg] && theme[bg], `${name}: missing --${fg} or --${bg}`);
      const r = ratio(theme[fg], theme[bg]);
      if (r < 4.5) failing.push(`--${fg} on --${bg}: ${r.toFixed(2)}`);
    }
    assert.deepEqual(failing, []);
  });
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test static/files/js/lib/contrast.test.mjs`
Expected: FAIL — "no prefers-color-scheme: dark block", and the light test reports missing `--warn`/`--ok-bg`/`--on-accent` plus `--faint on --surface` (#8a94a3 is about 2.9:1).

- [ ] **Step 3: Replace the `:root` block and add the dark theme**

In `static/files/css/app.css`, replace lines 1–18 (the whole `:root { … }` block) with:

```css
:root {
  color-scheme: light dark;
  --bg: #f6f7f9;
  --surface: #ffffff;
  --text: #1d2330;
  --muted: #5f6b7a;
  --faint: #646f80;
  --border: #dde2e8;
  --accent: #1a64c4;
  --accent-bg: #e8f1fc;
  --on-accent: #ffffff;
  --ok: #1a7a43;
  --ok-bg: #e6f4ec;
  --danger: #c23b3b;
  --danger-bg: #fbecec;
  --warn: #8a5a00;
  --warn-bg: #fbf1dc;
  --overlay: rgba(255, 255, 255, 0.75);
  --radius: 8px;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font: 15px/1.5 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  color: var(--text);
  background: var(--bg);
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0f1217;
    --surface: #171b22;
    --text: #e6e9ef;
    --muted: #a3adbb;
    --faint: #8e98a7;
    --border: #2a313c;
    --accent: #6aa8f7;
    --accent-bg: #15263d;
    --on-accent: #0f1217;
    --ok: #4cc27f;
    --ok-bg: #12291c;
    --danger: #f07b7b;
    --danger-bg: #2e1616;
    --warn: #e0a93a;
    --warn-bg: #2d2310;
    --overlay: rgba(15, 18, 23, 0.75);
  }
}
```

- [ ] **Step 4: Remove the remaining hardcoded colours**

In `static/files/css/app.css`:
- `button.primary { background: var(--accent); border-color: var(--accent); color: #fff; }` → change `color: #fff` to `color: var(--on-accent)`.
- In `.overlay { … }`, change `background: rgba(255, 255, 255, 0.75);` to `background: var(--overlay);`.

Then check nothing else is hardcoded:

Run: `grep -nE '#[0-9a-fA-F]{3,6}\b|rgba?\(' static/files/css/app.css`
Expected: matches only inside the two `:root` blocks.

- [ ] **Step 5: Add theme-color metas**

In `static/files/index.html`, after the `<meta name="viewport" …>` line, add:

```html
    <meta name="theme-color" content="#f6f7f9" media="(prefers-color-scheme: light)" />
    <meta name="theme-color" content="#0f1217" media="(prefers-color-scheme: dark)" />
```

- [ ] **Step 6: Run the tests**

Run: `node --test "static/files/js/**/*.test.mjs"`
Expected: PASS, including 3 contrast tests.

- [ ] **Step 7: Commit**

```bash
git add static/files/css/app.css static/files/index.html static/files/js/lib/contrast.test.mjs
git commit -m "Add dark mode and AA-contrast colour tokens with a contrast test

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Playwright e2e harness in the repo, with CI

**Files:**
- Create: `e2e/package.json`, `e2e/package-lock.json` (generated), `e2e/playwright.config.mjs`, `e2e/global-setup.mjs`, `e2e/fixtures.mjs`, `e2e/lib/torrent.mjs`, `e2e/lib/fake-search.mjs`, `e2e/tests/shell.spec.mjs`
- Modify: `.gitignore`, `.github/workflows/ci.yml`, `README.md` (Development section, lines 100–106)

**Interfaces:**
- Produces, from `e2e/fixtures.mjs`:
  - `test` (Playwright test extended with fixtures) and `expect`.
  - Option fixture `serverOptions: { title?: string }` (set with `test.use({ serverOptions: { title } })`).
  - Fixture `app: { url: string /* "http://127.0.0.1:<port>/" */, downloads: string /* abs dir */, addTorrent(buf: Buffer): Promise<void>, logs: string[] }`.
  - Fixture `page`: already at `app.url` and showing the Connected dot; fails the test on any console error or page error (except Chrome's "Failed to load resource" lines).
- Produces, from `e2e/lib/torrent.mjs`:
  - `makeTorrent(name: string, files: { path: string, size: number }[]) -> { name, single: boolean, torrent: Buffer, files: { path, size, data: Buffer }[] }`. A single file whose `path === name` makes a single-file torrent; otherwise `path` is relative to the folder `name`.
  - `writeData(downloads: string, t, { partial?: { [path]: bytes } }) -> Promise<void>` — writes the torrent's data where the engine expects it; `partial` writes only the first N bytes of the named files.
- Produces, from `e2e/lib/fake-search.mjs`: `startFakeSearch() -> Promise<{ configUrl: string, close(): Promise<void> }>` serving one provider `fake` named "Fake" with two results (seeds 42/7, peers 7/1).

- [ ] **Step 1: Check the toolchain**

Run: `go version && node --version`
Expected: a Go version and Node 22+. If Go is missing, stop and ask Leonard (see Global Constraints).

- [ ] **Step 2: Create `e2e/package.json` and install Playwright**

Create `e2e/package.json`:

```json
{
  "name": "cloud-fetch-e2e",
  "private": true,
  "type": "module",
  "description": "Browser tests for the Cloud Fetch web UI. Dev-only: not embedded, not needed by go install.",
  "scripts": {
    "test": "playwright test"
  }
}
```

Run (from `e2e/`): `npm install --save-dev --save-exact @playwright/test && npx playwright install chromium`
Expected: `package-lock.json` created; `package.json` gains `"devDependencies": { "@playwright/test": "<exact version>" }`.

- [ ] **Step 3: Ignore e2e build output**

Append to `.gitignore`:

```
e2e/node_modules/
e2e/.bin/
e2e/test-results/
e2e/playwright-report/
```

- [ ] **Step 4: Playwright config and global setup**

Create `e2e/playwright.config.mjs`:

```js
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.mjs",
  // each test starts its own server; one at a time keeps ports and CPU calm
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [["list"]] : [["list"], ["html", { open: "never" }]],
  use: { ...devices["Desktop Chrome"], trace: "retain-on-failure" },
});
```

Create `e2e/global-setup.mjs`:

```js
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

export const ROOT = fileURLToPath(new URL("..", import.meta.url));
export const BINARY = fileURLToPath(new URL("./.bin/cloud-fetch", import.meta.url));

// The UI is embedded in the binary, so rebuild on every run to test the
// current files.
export default function globalSetup() {
  execFileSync("go", ["build", "-o", BINARY, "."], { cwd: ROOT, stdio: "inherit" });
}
```

- [ ] **Step 5: Torrent fixtures**

Create `e2e/lib/torrent.mjs`:

```js
import { createHash, randomBytes } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

export const PIECE = 16384;

export function bencode(v) {
  if (Buffer.isBuffer(v)) return Buffer.concat([Buffer.from(`${v.length}:`), v]);
  if (typeof v === "string") return bencode(Buffer.from(v));
  if (Number.isInteger(v)) return Buffer.from(`i${v}e`);
  if (Array.isArray(v)) return Buffer.concat([Buffer.from("l"), ...v.map(bencode), Buffer.from("e")]);
  const keys = Object.keys(v).sort();
  return Buffer.concat([Buffer.from("d"), ...keys.flatMap((k) => [bencode(k), bencode(v[k])]), Buffer.from("e")]);
}

// makeTorrent builds a trackerless .torrent with random data. A single file
// whose path equals the name is a single-file torrent; otherwise paths are
// relative to the folder `name`.
export function makeTorrent(name, files) {
  const withData = files.map((f) => ({ ...f, data: randomBytes(f.size) }));
  const all = Buffer.concat(withData.map((f) => f.data));
  const pieces = [];
  for (let i = 0; i < all.length; i += PIECE) {
    pieces.push(createHash("sha1").update(all.subarray(i, i + PIECE)).digest());
  }
  const single = withData.length === 1 && withData[0].path === name;
  const info = { name, "piece length": PIECE, pieces: Buffer.concat(pieces) };
  if (single) info.length = withData[0].size;
  else info.files = withData.map((f) => ({ length: f.size, path: f.path.split("/") }));
  return { name, single, torrent: bencode({ info }), files: withData };
}

export async function writeData(downloads, t, { partial = {} } = {}) {
  for (const f of t.files) {
    const target = t.single ? join(downloads, t.name) : join(downloads, t.name, ...f.path.split("/"));
    await mkdir(dirname(target), { recursive: true });
    const n = f.path in partial ? partial[f.path] : f.size;
    await writeFile(target, f.data.subarray(0, n));
  }
}
```

- [ ] **Step 6: Fake search provider**

Create `e2e/lib/fake-search.mjs`:

```js
import { createServer } from "node:http";

const HASH_A = "a".repeat(40);
const HASH_B = "b".repeat(40);

const page = `<html><body><table>
<tr class="r"><td class="n">Fixture Result One</td><td class="m"><a href="magnet:?xt=urn:btih:${HASH_A}&dn=One">m</a></td>
<td class="s">1.2 GB</td><td class="se">42</td><td class="p">7</td></tr>
<tr class="r"><td class="n">Fixture Result Two</td><td class="m"><a href="magnet:?xt=urn:btih:${HASH_B}&dn=Two">m</a></td>
<td class="s">700 MB</td><td class="se">7</td><td class="p">1</td></tr>
</table></body></html>`;

// startFakeSearch serves a search-config with one provider, "fake", whose
// results page is served by the same server, so search works offline.
export function startFakeSearch() {
  const server = createServer((req, res) => {
    const { port } = server.address();
    if (req.url === "/config.json") {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        fake: {
          name: "Fake",
          url: `http://127.0.0.1:${port}/search?q={{query}}&p={{page:1}}`,
          list: "tr.r",
          result: {
            name: "td.n",
            magnet: ["td.m a", "@href"],
            size: "td.s",
            seeds: "td.se",
            peers: "td.p",
          },
        },
      }));
      return;
    }
    if (req.url.startsWith("/search")) {
      const pageNo = new URL(req.url, "http://x").searchParams.get("p");
      res.setHeader("content-type", "text/html");
      res.end(pageNo === "1" ? page : "<html><body><table></table></body></html>");
      return;
    }
    res.statusCode = 404;
    res.end();
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      resolve({
        configUrl: `http://127.0.0.1:${port}/config.json`,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}
```

- [ ] **Step 7: Fixtures**

Create `e2e/fixtures.mjs`:

```js
import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import net from "node:net";
import { BINARY } from "./global-setup.mjs";
import { startFakeSearch } from "./lib/fake-search.mjs";

export function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.unref();
    s.on("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address();
      s.close(() => resolve(port));
    });
  });
}

async function waitForHttp(url, proc, logs, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null || proc.signalCode !== null) {
      throw new Error(`cloud-fetch exited early:\n${logs.join("")}`);
    }
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`cloud-fetch did not answer on ${url}:\n${logs.join("")}`);
}

function exited(proc) {
  if (proc.exitCode !== null || proc.signalCode !== null) return Promise.resolve();
  return new Promise((r) => proc.once("exit", r));
}

export const test = base.extend({
  serverOptions: [{}, { option: true }],

  app: async ({ serverOptions }, use) => {
    const dir = await mkdtemp(join(tmpdir(), "cf-e2e-"));
    const downloads = join(dir, "downloads");
    await mkdir(downloads);
    const search = await startFakeSearch();
    const port = await freePort();
    const configPath = join(dir, "cloud-fetch.json");
    await writeFile(configPath, JSON.stringify({
      AutoStart: true,
      DisableEncryption: false,
      DownloadDirectory: downloads,
      EnableUpload: false,
      EnableSeeding: false,
      IncomingPort: await freePort(),
    }));
    const args = ["--port", String(port), "--config-path", configPath, "--search-config-url", search.configUrl];
    if (serverOptions.title) args.push("--title", serverOptions.title);
    const logs = [];
    const proc = spawn(BINARY, args, { cwd: dir });
    proc.stdout.on("data", (d) => logs.push(String(d)));
    proc.stderr.on("data", (d) => logs.push(String(d)));
    const url = `http://127.0.0.1:${port}/`;
    try {
      await waitForHttp(url, proc, logs);
      const addTorrent = async (buf) => {
        const res = await fetch(url + "api/torrentfile", { method: "POST", body: buf });
        if (!res.ok) throw new Error(`add torrent: ${res.status} ${await res.text()}`);
      };
      await use({ url, downloads, addTorrent, logs });
    } finally {
      proc.kill("SIGTERM");
      await exited(proc);
      await search.close();
      await rm(dir, { recursive: true, force: true });
    }
  },

  page: async ({ page, app }, use) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => {
      // Chrome logs one of these for every 4xx/5xx; tests provoke some on purpose
      if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) errors.push(m.text());
    });
    await page.goto(app.url);
    await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
    await use(page);
    expect(errors, "browser console errors").toEqual([]);
  },
});

export { expect };
```

- [ ] **Step 8: Write the shell spec**

Create `e2e/tests/shell.spec.mjs`:

```js
import { test, expect } from "../fixtures.mjs";

test("the page loads connected, with the empty states", async ({ page }) => {
  await expect(page.getByRole("heading", { name: "Torrents" })).toBeVisible();
  await expect(page.getByText("Add torrents above")).toBeVisible();
  await expect(page.getByText("Download files above")).toBeVisible();
});

test("the light theme applies by default", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  const bg = await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor);
  expect(bg).toBe("rgb(246, 247, 249)");
});

test("the dark theme follows the system setting", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  const colors = await page.evaluate(() => {
    const root = getComputedStyle(document.documentElement);
    return { bg: root.backgroundColor, text: root.color };
  });
  expect(colors).toEqual({ bg: "rgb(15, 18, 23)", text: "rgb(230, 233, 239)" });
});
```

- [ ] **Step 9: Run it**

Run (from `e2e/`): `npx playwright test`
Expected: the Go build runs, then `3 passed`. If "the page loads" fails on the Connected dot, read `app.logs` in the error output before changing anything.

- [ ] **Step 10: Add the CI job**

In `.github/workflows/ci.yml`, add this job after the `test` job (before `release_binaries`):

```yaml
  # ================
  # BROWSER TESTS
  # ================
  e2e:
    name: Browser tests
    needs: test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v7
      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - name: Set up Node
        uses: actions/setup-node@v7
        with:
          node-version: "24"
          cache: npm
          cache-dependency-path: e2e/package-lock.json
      - name: Install Playwright
        working-directory: e2e
        run: npm ci && npx playwright install --with-deps chromium
      - name: Browser tests
        working-directory: e2e
        run: npx playwright test
```

- [ ] **Step 11: Document it**

In `README.md`, replace the Development section's code block and the sentence before it (lines ~102–106) with:

````markdown
The web UI is plain ES modules in `static/files/`, with Preact and htm vendored in `static/files/js/vendor/` (versions and checksums in `VENDOR.md`). There is no build step: edit a file, run `go build`, reload the page. The UI logic has unit tests that run on Node 22 or later:

``` sh
node --test "static/files/js/**/*.test.mjs"
```

Browser tests live in `e2e/` (Playwright, dev-only; nothing there is embedded or needed by `go install`). They build the binary and run it offline against temporary folders:

``` sh
cd e2e && npm ci && npx playwright install chromium
npx playwright test
```
````

- [ ] **Step 12: Commit**

```bash
git add .gitignore README.md .github/workflows/ci.yml e2e/package.json e2e/package-lock.json e2e/playwright.config.mjs e2e/global-setup.mjs e2e/fixtures.mjs e2e/lib e2e/tests/shell.spec.mjs
git commit -m "Add Playwright browser tests in e2e/ with a CI job

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Torrent cards — status badge, Pause/Resume, ETA, hash in Files

**Files:**
- Modify: `static/files/js/components/TorrentList.js` (whole `TorrentCard`)
- Modify: `static/files/js/components/FileTable.js` (add `infohash` prop and wrapper)
- Modify: `static/files/js/icons.js` (add `pause`, `copy` paths)
- Modify: `static/files/css/app.css` (badges, bar colours, phone buttons)
- Test: `e2e/tests/torrents.spec.mjs` (create)

**Interfaces:**
- Consumes: `STATUS_LABELS`, `torrentStatus(t)`, `statusLine(t)` (Task 1); `app.addTorrent`, `app.downloads`, `makeTorrent`, `writeData` (Task 5); tokens `--ok-bg`, `--warn`, `--warn-bg` (Task 4).
- Produces: `Icon` names `"pause"` and `"copy"` (Task 7 uses `"copy"`); `FileTable({ infohash, files, size, onSelect })`; card markup `article.card.torrent.is-<status>` containing `.badge.badge-<status>` and `.status`.

- [ ] **Step 1: Write the failing e2e test**

Create `e2e/tests/torrents.spec.mjs`:

```js
import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

test("a complete torrent seeds, pauses to Done and resumes", async ({ page, app }) => {
  const t = makeTorrent("Fixture Show", [{ path: "E01.mkv", size: 32768 }, { path: "E02.mkv", size: 32768 }]);
  await writeData(app.downloads, t);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "Fixture Show" });
  await expect(card.locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await expect(card.locator(".status")).toHaveText("65.5 KB · complete");
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Done");
  await expect(card.getByRole("button", { name: "Remove" })).toBeVisible();
  await card.getByRole("button", { name: "Resume" }).click();
  await expect(card.locator(".badge")).toHaveText("Seeding");
  await expect(card.getByRole("button", { name: "Remove" })).toHaveCount(0);
});

test("an incomplete torrent downloads, waits for peers and pauses", async ({ page, app }) => {
  const t = makeTorrent("lonely.bin", [{ path: "lonely.bin", size: 65536 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "lonely.bin" });
  await expect(card.locator(".badge")).toHaveText("Downloading");
  await expect(card.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 0 B/s · waiting for peers");
  await expect(card.getByRole("button", { name: "Start" })).toHaveCount(0);
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Paused");
  await expect(card.locator(".status")).toHaveText("0 B of 65.5 KB · 0%");
  await expect(card.getByRole("button", { name: "Remove" })).toBeVisible();
});

test("the infohash lives in the Files panel, not the card header", async ({ page, app }) => {
  const t = makeTorrent("hash.bin", [{ path: "hash.bin", size: 16384 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "hash.bin" });
  await expect(card.locator(".badge")).toBeVisible();
  await expect(card.getByText(/^#[0-9a-f]{40}$/)).toHaveCount(0);
  await card.getByRole("button", { name: "Files" }).click();
  await expect(card.locator(".files-panel .hash")).toHaveText(/^#[0-9a-f]{40}$/);
});

test("card buttons get their own full-width row on a phone", async ({ page, app }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  const t = makeTorrent("phone.bin", [{ path: "phone.bin", size: 16384 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "phone.bin" });
  await expect(card.locator(".badge")).toBeVisible();
  const name = await card.locator(".name").boundingBox();
  const buttons = card.locator(".buttons button");
  for (const b of await buttons.all()) {
    const box = await b.boundingBox();
    expect(box.y).toBeGreaterThanOrEqual(name.y + name.height - 1);
    expect(box.height).toBeGreaterThanOrEqual(36);
    expect(box.width).toBeGreaterThanOrEqual(80);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
```

Size note: 32768 + 32768 bytes = 65536 bytes, which `bytes()` renders as "65.5 KB" (decimal units).

- [ ] **Step 2: Run it to verify it fails**

Run (from `e2e/`): `npx playwright test tests/torrents.spec.mjs`
Expected: FAIL — no `.badge` element; the card still has Start/Stop.

- [ ] **Step 3: Add the icons**

In `static/files/js/icons.js`, add to `PATHS` (after `stop`):

```js
  pause: "M8 5v14M16 5v14",
  copy: "M9 9h11v11H9zM5 15H4V4h11v1",
```

- [ ] **Step 4: Rewrite the card**

In `static/files/js/components/TorrentList.js`, replace the imports and the whole `TorrentCard` function with:

```js
import { useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { sortTorrents } from "../lib/tree.js";
import { STATUS_LABELS, statusLine, torrentStatus } from "../lib/status.js";
import { FileTable } from "./FileTable.js";

function TorrentCard({ t, api, onError }) {
  const [showFiles, setShowFiles] = useState(false);
  const act = (action) => api.torrent(action, t.InfoHash).catch(onError);
  const status = torrentStatus(t);
  const line = statusLine(t);
  const pct = t.Percent || 0;
  return html`<article class=${"card torrent is-" + status} aria-label=${t.Name || t.InfoHash}>
    ${!t.Loaded && html`<div class="overlay"><${Icon} name="loader" class="spin" /> Loading</div>`}
    <div class="torrent-top">
      <div class="info">
        <div class="name">${t.Name || t.InfoHash} <span class=${"badge badge-" + status}>${STATUS_LABELS[status]}</span></div>
      </div>
      <div class="buttons">
        <button type="button" class=${showFiles ? "on" : ""} aria-pressed=${showFiles} onClick=${() => setShowFiles(!showFiles)}>
          <${Icon} name="file" /> Files
        </button>
        ${t.Started
          ? html`<button type="button" onClick=${() => act("stop")}><${Icon} name="pause" /> Pause</button>`
          : html`<button type="button" onClick=${() => act("start")}><${Icon} name="play" /> Resume</button>`}
        ${!t.Started && html`<button type="button" class="danger" onClick=${() => act("delete")}>
          <${Icon} name=${t.Loaded ? "trash" : "x"} /> ${t.Loaded ? "Remove" : "Cancel"}
        </button>`}
      </div>
    </div>
    <div class="progress" role="progressbar" aria-label="Progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow=${pct}>
      <div class="bar" style=${{ width: pct + "%" }}></div>
    </div>
    ${line.main && html`<div class="status">
      ${line.main}${line.rate && html` · <strong>${line.rate}</strong>`}${line.note && ` · ${line.note}`}
    </div>`}
    ${showFiles && t.Loaded && html`<${FileTable} infohash=${t.InfoHash} files=${t.Files} size=${t.Size}
      onSelect=${(path, on) => api.file(on ? "start" : "stop", t.InfoHash, path).catch((e) => { onError(e); throw e; })} />`}
  </article>`;
}
```

Leave `TorrentList` (the export below it) unchanged. Remove the now-unused `bytes` import if your editor flags it (the new imports above already omit it).

- [ ] **Step 5: Move the hash into the Files panel**

Replace `static/files/js/components/FileTable.js` with (only the `infohash` prop, the wrapper `div` and the hash line are new):

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes, filename } from "../lib/format.js";

export function FileTable({ infohash, files, size, onSelect }) {
  const list = (files || []).filter(Boolean).slice().sort((a, b) => a.Path.localeCompare(b.Path));
  const toggle = (f, e) => {
    const box = e.currentTarget;
    // the box stays as clicked until the server's next push confirms it
    onSelect(f.Path, box.checked).catch(() => { box.checked = f.Started; });
  };
  return html`<div class="files-panel">
    <p class="hash">#${infohash}</p>
    <table class="files">
      <thead><tr><th>File</th><th class="size">Size</th></tr></thead>
      <tbody>
        ${list.length === 0 && html`<tr><td colspan="2" class="muted">No files</td></tr>`}
        ${list.map((f) => html`<tr key=${f.Path} class=${f.Started ? "" : "skipped"}>
          <td class="name">
            <label>
              <input type="checkbox" checked=${f.Started} aria-label=${"Download " + filename(f.Path)}
                onChange=${(e) => toggle(f, e)} />
              <span>${filename(f.Path)}</span>
            </label>
            ${f.Percent > 0 && f.Percent < 100 && html` <span class="pct">${f.Percent}%</span>
              <div class="progress thin"><div class="bar" style=${{ width: f.Percent + "%" }}></div></div>`}
          </td>
          <td class="size">${bytes(f.Size)} ${f.Percent === 100 && html`<${Icon} name="check" class="ok" label="Complete" />`}</td>
        </tr>`)}
      </tbody>
      ${list.length > 1 && html`<tfoot><tr><th>${list.length} files</th><th class="size">${bytes(size)} total</th></tr></tfoot>`}
    </table>
  </div>`;
}
```

- [ ] **Step 6: Style badges, bars and phone buttons**

In `static/files/css/app.css`:
- Change `.hash { … }` to `.hash { margin: 10px 0 0; color: var(--faint); font-family: var(--mono); font-size: 11px; overflow-wrap: anywhere; user-select: all; }`
- After the `.torrent .status strong` rule, add:

```css
.badge {
  display: inline-block; margin-left: 6px; padding: 1px 8px; border-radius: var(--radius);
  font-size: 12px; font-weight: 500; vertical-align: 2px; white-space: nowrap;
}
.badge-loading { background: var(--bg); color: var(--muted); }
.badge-downloading { background: var(--accent-bg); color: var(--accent); }
.badge-seeding, .badge-done { background: var(--ok-bg); color: var(--ok); }
.badge-paused { background: var(--warn-bg); color: var(--warn); }
.torrent.is-seeding .progress .bar, .torrent.is-done .progress .bar { background: var(--ok); }
.torrent.is-paused .progress .bar { background: var(--faint); }
```

- In the `@media (max-width: 600px)` block, after `.torrent .buttons { width: 100%; }`, add:

```css
  .torrent .buttons button { flex: 1 1 0; justify-content: center; min-height: 36px; }
```

- [ ] **Step 7: Run the e2e and unit tests**

Run (from `e2e/`): `npx playwright test`
Expected: all specs pass (shell 3 + torrents 4).

Run (repo root): `node --test "static/files/js/**/*.test.mjs"`
Expected: PASS (contrast still green — badges only use checked pairs).

- [ ] **Step 8: Commit**

```bash
git add static/files/js/components/TorrentList.js static/files/js/components/FileTable.js static/files/js/icons.js static/files/css/app.css e2e/tests/torrents.spec.mjs
git commit -m "Show torrent status badges, Pause/Resume and an ETA on cards

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Copy link and Copy all on download rows

**Files:**
- Create: `static/files/js/components/CopyButton.js`, `static/files/js/components/CopyFallback.js`
- Modify: `static/files/js/components/DownloadTree.js` (imports, `TreeNode` controls)
- Modify: `static/files/css/app.css` (rows, copy states, popover, `.sr-only`)
- Test: `e2e/tests/downloads.spec.mjs` (create)

**Interfaces:**
- Consumes: `copyText(text)` (Task 3); `absoluteHref(path, base)`, `finishedFiles(node, path, torrents)` (Task 2); `Icon` name `"copy"` (Task 6); fixtures and `makeTorrent`/`writeData` (Task 5).
- Produces:
  - `CopyButton({ getText: () => string, label: string, done: string | ((text) => string), title: string })`.
  - `CopyFallback({ text: string, title: string, onClose: () => void })` — `role="dialog"` named `title`.
  - Button names: file "Copy link to <name>", folder "Copy all links in <name>"; while confirmed: "Copied" / "Copied <n>".

- [ ] **Step 1: Write the failing e2e test**

Create `e2e/tests/downloads.spec.mjs`:

```js
import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

// E01 is complete; E02 has only its first piece, so the torrent keeps
// downloading it and its row shows no link or copy button.
async function mixedShow(app) {
  const t = makeTorrent("Mixed Show", [{ path: "E01.mkv", size: 32768 }, { path: "E02.mkv", size: 32768 }]);
  await writeData(app.downloads, t, { partial: { "E02.mkv": 16384 } });
  await app.addTorrent(t.torrent);
}

async function grantClipboard(page, app) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(app.url).origin });
}

const row = (page, name) => page.locator(".node > .row", { hasText: name });

test("Copy link puts the file's absolute link on the clipboard", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(row(page, "E01.mkv").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
  await expect(page.getByRole("button", { name: "Copy link to E01.mkv" })).toBeVisible({ timeout: 4000 });
});

test("a file that is still downloading has no copy button", async ({ page, app }) => {
  await mixedShow(app);
  await expect(row(page, "E02.mkv")).toBeVisible();
  await expect(page.getByRole("article", { name: "Mixed Show" }).locator(".badge")).toHaveText("Downloading");
  await expect(row(page, "E02.mkv").getByRole("button", { name: /^Copy/ })).toHaveCount(0);
});

test("Copy all copies only the folder's finished files, one per line", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await expect(row(page, "E02.mkv")).toBeVisible();
  await page.getByRole("button", { name: "Copy all links in Mixed Show" }).click();
  await expect(page.getByRole("button", { name: "Copied 1" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
});

test("Copy all on a finished folder copies every file", async ({ page, app }) => {
  await grantClipboard(page, app);
  const t = makeTorrent("Full Show", [{ path: "E01.mkv", size: 16384 }, { path: "E02.mkv", size: 16384 }]);
  await writeData(app.downloads, t);
  await app.addTorrent(t.torrent);
  await expect(page.getByRole("article", { name: "Full Show" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await page.getByRole("button", { name: "Copy all links in Full Show" }).click();
  await expect(page.getByRole("button", { name: "Copied 2" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    `${app.url}download/Full%20Show/E01.mkv\n${app.url}download/Full%20Show/E02.mkv`,
  );
});

test("on plain HTTP the copy falls back to execCommand", async ({ page, app }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", { value: false });
    window.__copied = [];
    const original = document.execCommand.bind(document);
    document.execCommand = (cmd, ...rest) => {
      if (cmd !== "copy") return original(cmd, ...rest);
      window.__copied.push(document.activeElement && document.activeElement.value);
      return true;
    };
  });
  await page.reload();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(row(page, "E01.mkv").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => window.__copied)).toEqual([`${app.url}download/Mixed%20Show/E01.mkv`]);
});

test("when copying is blocked, the link opens selected for copying by hand", async ({ page, app }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", { value: false });
    document.execCommand = () => false;
  });
  await page.reload();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  const dialog = page.getByRole("dialog", { name: "Copy link" });
  await expect(dialog).toBeVisible();
  const area = dialog.getByRole("textbox");
  await expect(area).toHaveValue(`${app.url}download/Mixed%20Show/E01.mkv`);
  await expect(area).toBeFocused();
  expect(await area.evaluate((el) => el.selectionStart === 0 && el.selectionEnd === el.value.length)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("download rows are comfortable to tap on a phone", async ({ page, app }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await mixedShow(app);
  const button = page.getByRole("button", { name: "Copy link to E01.mkv" });
  const box = await button.boundingBox();
  expect(box.width).toBeGreaterThanOrEqual(36);
  expect(box.height).toBeGreaterThanOrEqual(36);
  expect((await row(page, "E01.mkv").boundingBox()).height).toBeGreaterThanOrEqual(40);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run (from `e2e/`): `npx playwright test tests/downloads.spec.mjs`
Expected: FAIL — no "Copy link to E01.mkv" button (the "still downloading" test may already pass; that's fine).

- [ ] **Step 3: Create the manual-copy popover**

Create `static/files/js/components/CopyFallback.js`:

```js
import { useEffect, useRef } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";

// CopyFallback shows text pre-selected for copying by hand, for browsers
// that refuse both clipboard routes. Esc or a click outside closes it.
export function CopyFallback({ text, title, onClose }) {
  const box = useRef(null);
  const area = useRef(null);
  useEffect(() => {
    area.current.focus();
    area.current.select();
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    const onDown = (e) => { if (box.current && !box.current.contains(e.target)) onClose(); };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onDown);
    };
  }, []);
  const rows = Math.min(text.split("\n").length, 6);
  return html`<div ref=${box} class="copy-fallback" role="dialog" aria-label=${title}>
    <textarea ref=${area} readonly rows=${rows} aria-label=${title} value=${text}></textarea>
    <div class="copy-fallback-foot">
      <span class="muted">Press Ctrl+C or long-press to copy</span>
      <button type="button" class="icon-btn" aria-label="Close" onClick=${onClose}><${Icon} name="x" /></button>
    </div>
  </div>`;
}
```

- [ ] **Step 4: Create the copy button**

Create `static/files/js/components/CopyButton.js`:

```js
import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { copyText } from "../lib/clipboard.js";
import { CopyFallback } from "./CopyFallback.js";

// CopyButton reads getText() on click, so it copies the tree as it is then,
// and shows the confirmation in place of the icon for 2 seconds.
export function CopyButton({ getText, label, done, title }) {
  const [state, setState] = useState("idle"); // idle | copied | manual
  const [text, setText] = useState("");
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    const value = getText();
    setText(value);
    clearTimeout(timer.current);
    if ((await copyText(value)) === "copied") {
      setState("copied");
      timer.current = setTimeout(() => setState("idle"), 2000);
    } else {
      setState("manual");
    }
  };
  const doneText = typeof done === "function" ? done(text) : done;
  const copied = state === "copied";
  return html`<span class="copy">
    <button type="button" class=${"icon-btn" + (copied ? " copied" : "")} aria-label=${copied ? doneText : label} onClick=${copy}>
      <${Icon} name=${copied ? "check" : "copy"} />${copied && html`<span class="copied-text" aria-hidden="true">${doneText}</span>`}
    </button>
    <span class="sr-only" aria-live="polite">${copied ? doneText : ""}</span>
    ${state === "manual" && html`<${CopyFallback} text=${text} title=${title} onClose=${() => setState("idle")} />`}
  </span>`;
}
```

- [ ] **Step 5: Wire it into the tree**

In `static/files/js/components/DownloadTree.js`:

Change the tree import to:

```js
import {
  absoluteHref, childPath, downloadHref, fileIcon, findTorrentFile, finishedFiles,
  isDir, isDownloading, previewKind, startsClosed,
} from "../lib/tree.js";
import { CopyButton } from "./CopyButton.js";
```

In `TreeNode`, after `const href = downloadHref(path);` add:

```js
  const finished = dir ? finishedFiles(node, path, torrents) : null;
  const linksOf = (paths) => paths.map((p) => absoluteHref(p, window.location.href)).join("\n");
```

In the returned template, replace the whole `!downloading` controls expression (from `${!downloading && html` through its closing `</span>`}`) with this (the copy block is new; the preview and delete markup are unchanged):

```js
      ${!downloading && html`<span class="controls">
        ${dir
          ? finished.length > 0 && html`<${CopyButton} title="Copy links"
              getText=${() => linksOf(finishedFiles(node, path, torrents))}
              label=${"Copy all links in " + node.Name}
              done=${(text) => "Copied " + text.split("\n").length} />`
          : html`<${CopyButton} title="Copy link" getText=${() => linksOf([path])}
              label=${"Copy link to " + node.Name} done="Copied" />`}
        ${kind && html`<button type="button" class=${"icon-btn" + (preview ? " on" : "")} aria-pressed=${preview}
          aria-label=${(preview ? "Hide preview of " : "Preview ") + node.Name} onClick=${() => setPreview(!preview)}>
          <${Icon} name=${preview ? "x" : "play"} />
        </button>`}
        ${deleting
          ? html`<${Icon} name="loader" class="spin" label="Deleting" />`
          : confirm
            ? html`<button type="button" class="icon-btn danger" aria-label=${"Confirm delete " + node.Name} onClick=${remove}><${Icon} name="check" /></button>`
            : html`<button type="button" class="icon-btn danger" aria-label=${"Delete " + node.Name} onClick=${arm}><${Icon} name="trash" /></button>`}
      </span>`}
```

- [ ] **Step 6: Style rows, confirmation and popover**

In `static/files/css/app.css`:
- Change `.node .row { display: flex; align-items: center; gap: 6px; min-height: 28px; }` to `.node .row { position: relative; display: flex; align-items: center; gap: 6px; min-height: 40px; }`.
- After `.node .controls { display: flex; gap: 2px; }` add:

```css
.node .controls .icon-btn { min-width: 36px; min-height: 36px; justify-content: center; }
.copy { display: inline-flex; }
.icon-btn.copied { gap: 4px; color: var(--ok); font-size: 12px; }
.copy-fallback {
  position: absolute; top: 100%; right: 0; z-index: 2; width: min(420px, 100%); padding: 8px;
  border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface);
}
.copy-fallback textarea {
  width: 100%; padding: 6px; resize: none; border: 1px solid var(--border); border-radius: var(--radius);
  background: var(--bg); color: var(--text); font: 12px/1.4 var(--mono);
}
.copy-fallback-foot { display: flex; align-items: center; justify-content: space-between; margin-top: 4px; font-size: 12px; }
.sr-only {
  position: absolute; width: 1px; height: 1px; overflow: hidden;
  clip: rect(0 0 0 0); clip-path: inset(50%); white-space: nowrap;
}
```

- [ ] **Step 7: Run the tests**

Run (from `e2e/`): `npx playwright test`
Expected: all specs pass (shell 3, torrents 4, downloads 7).

Run (repo root): `node --test "static/files/js/**/*.test.mjs"`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add static/files/js/components/CopyButton.js static/files/js/components/CopyFallback.js static/files/js/components/DownloadTree.js static/files/css/app.css e2e/tests/downloads.spec.mjs
git commit -m "Add Copy link and Copy all to download rows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Search results header, phone layout and long titles

**Files:**
- Modify: `static/files/js/components/SearchResults.js` (whole component)
- Modify: `static/files/css/app.css` (results rules, `.brand`, phone block)
- Test: `e2e/tests/layout.spec.mjs` (create)

**Interfaces:**
- Consumes: fixtures, `serverOptions.title` and the fake provider "Fake" (Task 5).
- Produces: results markup `table.results > thead` with column headers Name, Size, Seeds, Peers; cells `td.name`, `td.size`, `td.seeds`, `td.peers`, `td.controls`; `.unit` spans (" seeds", " peers") shown only on phones.

- [ ] **Step 1: Write the failing e2e test**

Create `e2e/tests/layout.spec.mjs`:

```js
import { test, expect } from "../fixtures.mjs";

async function searchFixture(page) {
  await page.getByRole("textbox", { name: "Search, magnet link or torrent URL" }).fill("fixture");
  const provider = page.getByRole("combobox", { name: "Search provider" });
  await expect(provider.locator("option", { hasText: "Fake" })).toHaveCount(1);
  await provider.selectOption({ label: "Fake" });
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByText("Fixture Result One")).toBeVisible();
}

test("search results have a header row on desktop", async ({ page }) => {
  await searchFixture(page);
  const table = page.locator("table.results");
  for (const name of ["Name", "Size", "Seeds", "Peers"]) {
    await expect(table.getByRole("columnheader", { name, exact: true })).toBeVisible();
  }
  const first = table.locator("tbody tr").first();
  await expect(first.locator("td.seeds")).toHaveText("42");
  await expect(first.locator("td.peers")).toHaveText("7");
});

test("search results stack with labelled counts on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await searchFixture(page);
  const first = page.locator("table.results tbody tr").first();
  await expect(first).toContainText("42 seeds");
  await expect(first).toContainText("7 peers");
  const name = await first.locator("td.name").boundingBox();
  const add = await first.getByRole("button", { name: /^Add / }).boundingBox();
  expect(add.y).toBeGreaterThan(name.y);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("settings fit a phone screen", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.getByRole("button", { name: "Settings" }).click();
  await expect(page.getByRole("button", { name: "Save" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test.describe("with a very long one-word title", () => {
  test.use({ serverOptions: { title: "Supercalifragilisticexpialidociousandthensomemoreletters" } });

  test("the header truncates instead of overflowing on a phone", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    const settings = await page.getByRole("button", { name: "Settings" }).boundingBox();
    expect(settings.x + settings.width).toBeLessThanOrEqual(375);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });
});
```

If "Save" isn't the Settings form's submit label, check `ConfigForm.js` for the real label and use it.

- [ ] **Step 2: Run it to verify it fails**

Run (from `e2e/`): `npx playwright test tests/layout.spec.mjs`
Expected: FAIL — no column headers; no "42 seeds" text. The settings and long-title tests may already pass; if the long-title one passes, keep it as a regression guard and skip the `.brand` change in Step 4.

- [ ] **Step 3: Rewrite the results table**

Replace `static/files/js/components/SearchResults.js` with:

```js
import { html } from "../html.js";
import { Icon } from "../icons.js";

export function SearchResults({ results, hasMore, searching, onMore, onAdd }) {
  return html`<table class="results">
    <thead><tr>
      <th class="name">Name</th><th class="size">Size</th><th class="seeds">Seeds</th><th class="peers">Peers</th>
      <th class="controls"><span class="sr-only">Add</span></th>
    </tr></thead>
    <tbody>
      ${results.map((r, i) => html`<tr key=${i}>
        <td class="name">${r.url ? html`<a href=${r.url} target="_blank" rel="noopener noreferrer">${r.name}</a>` : r.name}</td>
        <td class="size">${r.size || ""}</td>
        <td class="seeds">${r.seeds && html`${r.seeds}<span class="unit"> seeds</span>`}</td>
        <td class="peers">${r.peers && html`${r.peers}<span class="unit"> peers</span>`}</td>
        <td class="controls">
          <button type="button" class="icon-btn go" aria-label=${"Add " + (r.name || "result")} onClick=${() => onAdd(r)}>
            <${Icon} name="download" />
          </button>
        </td>
      </tr>`)}
      ${hasMore && html`<tr class="more-row"><td colspan="5" class="more">
        <button type="button" onClick=${onMore} disabled=${searching}>${searching ? "Loading…" : "Load more"}</button>
      </td></tr>`}
    </tbody>
  </table>`;
}
```

- [ ] **Step 4: Style it**

In `static/files/css/app.css`, replace the block from `.results td { … }` through `.results .more { … }` with:

```css
.results th {
  padding: 6px 10px; color: var(--muted); font-size: 12px; font-weight: 500; text-align: left; white-space: nowrap;
}
.results td { padding: 7px 10px; border-top: 1px solid var(--border); vertical-align: middle; }
.results .name { overflow-wrap: anywhere; }
.results .size, .results .peers { color: var(--muted); white-space: nowrap; }
.results .seeds { color: var(--ok); white-space: nowrap; }
.results .unit { display: none; }
.results .controls { width: 1%; }
.results .more { text-align: center; }
```

Change `.brand { display: flex; … }` to add `flex: 1 1 auto;` at the start of its declarations.

In the `@media (max-width: 600px)` block, add:

```css
  .results, .results tbody { display: block; }
  .results thead { display: none; }
  .results tr { display: flex; flex-wrap: wrap; align-items: center; column-gap: 8px; padding: 8px 10px; }
  .results tbody tr + tr { border-top: 1px solid var(--border); }
  .results td { padding: 0; border-top: 0; }
  .results td.name { flex: 1 0 100%; }
  .results td:empty { display: none; }
  .results .unit { display: inline; }
  .results td.seeds:not(:empty)::before, .results td.peers:not(:empty)::before { content: "· "; color: var(--faint); }
  .results td.controls { width: auto; margin-left: auto; }
  .results .more-row td { flex: 1; }
```

- [ ] **Step 5: Run the tests**

Run (from `e2e/`): `npx playwright test`
Expected: all specs pass (shell 3, torrents 4, downloads 7, layout 4).

Run (repo root): `node --test "static/files/js/**/*.test.mjs"`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add static/files/js/components/SearchResults.js static/files/css/app.css e2e/tests/layout.spec.mjs
git commit -m "Label search results, stack them on phones, truncate long titles

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Screenshot refresh and whole-branch verification

**Files:**
- Create: `e2e/tests/screenshot.spec.mjs`
- Modify: `docs/screenshot.png` (regenerated)

**Interfaces:**
- Consumes: fixtures, `makeTorrent`, `writeData` (Task 5); the finished UI.

- [ ] **Step 1: Add an opt-in screenshot spec**

Create `e2e/tests/screenshot.spec.mjs`:

```js
import { fileURLToPath } from "node:url";
import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

const OUT = fileURLToPath(new URL("../../docs/screenshot.png", import.meta.url));

// Regenerates the README screenshot: SCREENSHOT=1 npx playwright test screenshot
test.skip(!process.env.SCREENSHOT, "set SCREENSHOT=1 to regenerate docs/screenshot.png");
test.use({ viewport: { width: 1000, height: 900 }, deviceScaleFactor: 2 });

test("README screenshot", async ({ page, app }) => {
  const show = makeTorrent("Big Buck Bunny (2008)", [
    { path: "Big Buck Bunny.mp4", size: 16384 * 6 },
    { path: "Big Buck Bunny.en.srt", size: 16384 },
  ]);
  await writeData(app.downloads, show);
  await app.addTorrent(show.torrent);
  const iso = makeTorrent("debian-12.7.0-amd64-netinst.iso", [{ path: "debian-12.7.0-amd64-netinst.iso", size: 16384 * 4 }]);
  await writeData(app.downloads, iso);
  await app.addTorrent(iso.torrent);
  const partial = makeTorrent("ubuntu-24.04.1-desktop-amd64.iso", [{ path: "ubuntu-24.04.1-desktop-amd64.iso", size: 16384 * 8 }]);
  await writeData(app.downloads, partial, { partial: { "ubuntu-24.04.1-desktop-amd64.iso": 16384 * 3 } });
  await app.addTorrent(partial.torrent);
  await expect(page.getByRole("article", { name: "Big Buck Bunny (2008)" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await expect(page.getByRole("article", { name: "debian-12.7.0-amd64-netinst.iso" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  const debian = page.getByRole("article", { name: "debian-12.7.0-amd64-netinst.iso" });
  await debian.getByRole("button", { name: "Pause" }).click();
  await expect(debian.locator(".badge")).toHaveText("Done");
  await page.getByRole("article", { name: "Big Buck Bunny (2008)" }).getByRole("button", { name: "Files" }).click();
  await page.screenshot({ path: OUT, fullPage: true });
});
```

- [ ] **Step 2: Regenerate and look at it**

Run (from `e2e/`): `SCREENSHOT=1 npx playwright test tests/screenshot.spec.mjs`
Expected: `1 passed`; `docs/screenshot.png` updated. Open it with the Read tool and check: badges visible, Pause/Resume, copy icons on finished files, no overlap. Fix anything that looks wrong before continuing.

Then check the README alt text (line 1) still describes the picture: two complete torrents (one paused), one downloading, and the downloads folder. Update the alt text if not.

- [ ] **Step 3: Full verification**

Run each and confirm the expected output:

- `gofmt -l .` → no output
- `go vet ./...` → no output
- `go test -race -count=1 ./...` → `ok` for every package
- `node --test "static/files/js/**/*.test.mjs"` → `# fail 0`
- (from `e2e/`) `npx playwright test` → `18 passed`, `1 skipped` (the screenshot spec)
- `git status --short` → only the intended files; no `e2e/node_modules`, `.bin` or `test-results`

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/screenshot.spec.mjs docs/screenshot.png README.md
git commit -m "Refresh the README screenshot from an opt-in e2e spec

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 5: Push and open the PR**

```bash
git push -u origin feature/ui-polish-links
gh pr create --repo LeonardCooray/cloud-fetch --base master --title "UI polish: copy links, status badges, dark mode, phone layout" --body-file <body file>
```

The body summarises the spec's four areas, lists the verification output from Step 3, notes the new dev-only `e2e/` npm lockfile and CI job, and ends with:

```
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

Don't merge; Leonard reviews and merges.
