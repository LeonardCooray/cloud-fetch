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

// The engine keeps an unfinished file on disk as "<name>.part" and renames
// it when it completes, so a .part path matches the torrent file it becomes.
export function findTorrentFile(torrents, path) {
  const target = path.endsWith(".part") ? path.slice(0, -".part".length) : path;
  for (const torrent of Object.values(torrents || {})) {
    const file = (torrent.Files || []).find((f) => f && (f.Path === path || f.Path === target));
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
export function finishedFiles(node, path, torrents) {
  if (isDir(node)) return node.Children.flatMap((c) => finishedFiles(c, childPath(path, c.Name), torrents));
  if (path.endsWith(".part")) return [];
  const match = findTorrentFile(torrents, path);
  return !match || match.file.Percent >= 100 ? [path] : [];
}
