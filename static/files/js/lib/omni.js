export function classify(input) {
  const s = (input || "").trim();
  if (!s) return "empty";
  if (/^https?:\/\//i.test(s)) return "torrent-url";
  if (/^magnet:\?/i.test(s)) return "magnet";
  return "search";
}

// droppedText reads a link or text dragged in from another page or app: the
// first link of a text/uri-list (# lines are comments), else plain text.
export function droppedText(dt) {
  if (!dt) return "";
  const uris = String(dt.getData("text/uri-list") || "")
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#"));
  if (uris.length) return uris[0];
  return String(dt.getData("text/plain") || "").trim();
}
