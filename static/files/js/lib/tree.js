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
