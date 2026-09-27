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
