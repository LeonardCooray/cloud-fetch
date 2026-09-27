export function classify(input) {
  const s = (input || "").trim();
  if (!s) return "empty";
  if (/^https?:\/\//i.test(s)) return "torrent-url";
  if (/^magnet:\?/i.test(s)) return "magnet";
  return "search";
}
