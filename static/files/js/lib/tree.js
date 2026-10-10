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

// indexTorrentFiles maps each torrent file's path to { torrent, file }. The
// Downloads tree looks up every row on every push, so it builds this once
// per render; velox merges pushes into the same objects, so it can't be
// cached across renders by identity. The first torrent listing a path wins.
export function indexTorrentFiles(torrents) {
  const index = new Map();
  for (const torrent of Object.values(torrents || {})) {
    for (const file of torrent.Files || []) {
      if (file && !index.has(file.Path)) index.set(file.Path, { torrent, file });
    }
  }
  return index;
}

// The engine keeps an unfinished file on disk as "<name>.part" and renames
// it when it completes, so a .part path matches the torrent file it becomes.
export function findTorrentFile(files, path) {
  const target = finalPath(path);
  return files.get(path) || (target !== path && files.get(target)) || null;
}

// finalPath is the name a .part file gets when it completes. A downloading
// file links there: the server streams it until then, and a player holding
// the link keeps working across the rename. Download rows are keyed by it
// too, so an open preview isn't remounted when the .part row is replaced.
export function finalPath(path) {
  return path.endsWith(".part") ? path.slice(0, -".part".length) : path;
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

// absoluteHref is what Copy link puts on the clipboard: the same link the
// row shows, made absolute so IDM and VLC can use it outside the page.
// Credentials in the page URL (a bookmarked http://user:pass@host/) are
// dropped so a password never reaches the clipboard.
export function absoluteHref(path, base) {
  const url = new URL(downloadHref(path), base);
  url.username = "";
  url.password = "";
  return url.href;
}

// finishedFiles lists the files the copy buttons may copy: complete files
// only, so a paused torrent's half-written file (or a stray .part) never
// ends up in IDM.
export function finishedFiles(node, path, files) {
  if (isDir(node)) return node.Children.flatMap((c) => finishedFiles(c, childPath(path, c.Name), files));
  return isFinished(path, files) ? [path] : [];
}

// hasFinishedFile decides whether a row gets a copy button. It runs for
// every folder on every push, so it stops at the first finished file.
export function hasFinishedFile(node, path, files) {
  if (isDir(node)) return node.Children.some((c) => hasFinishedFile(c, childPath(path, c.Name), files));
  return isFinished(path, files);
}

function isFinished(path, files) {
  if (path.endsWith(".part")) return false;
  const match = findTorrentFile(files, path);
  return !match || match.file.Percent >= 100;
}
