// A v1 info hash: 40 hex or 32 base32 characters. Anything else can't be
// added, and a scraped value must never carry extra "&..." parameters.
export function validInfohash(h) {
  return /^([0-9a-f]{40}|[a-z2-7]{32})$/i.test(h || "");
}

// Fields are returned even when the hash is invalid, so the editor can still
// show them while the person fixes the hash. extra holds every parameter the
// editor doesn't show (a btmh xt, xl, ws, ...) as [key, value] pairs, so
// editing the name or trackers doesn't drop them.
export function parseMagnet(uri) {
  const m = /^magnet:\?(.*)$/i.exec((uri || "").trim());
  const params = new URLSearchParams(m ? m[1] : "");
  const out = { name: "", infohash: "", trackers: [], extra: [] };
  let sawBtih = false;
  let sawBtmh = false;
  for (const [key, value] of params) {
    if (key === "dn" && !out.name) out.name = value;
    else if (key === "tr") out.trackers.push(value);
    else if (key === "xt" && /^urn:btih:/i.test(value) && !sawBtih) {
      sawBtih = true;
      out.infohash = value.slice("urn:btih:".length);
    } else {
      if (key === "xt" && /^urn:btmh:/i.test(value)) sawBtmh = true;
      out.extra.push([key, value]);
    }
  }
  if (!sawBtih && sawBtmh) out.error = "v2-only magnets aren't supported yet";
  else if (!validInfohash(out.infohash)) out.error = "Invalid info hash";
  return out;
}

export function buildMagnet({ name = "", infohash = "", trackers = [], extra = [] }) {
  let s = "magnet:?xt=urn:btih:" + infohash;
  if (name) s += "&dn=" + encodeURIComponent(name);
  for (const t of trackers) if (t) s += "&tr=" + encodeURIComponent(t);
  for (const [k, v] of extra) s += "&" + encodeURIComponent(k) + "=" + encodeURIComponent(v);
  return s;
}

// trackerSlots gives the editor's tracker fields. The magnet text can't hold
// an empty tracker, so while the person's slots still describe the same
// trackers they are kept, empty ones included; otherwise the text was
// changed elsewhere and wins.
export function trackerSlots(slots, trackers) {
  const filled = slots.filter(Boolean);
  const same = filled.length === trackers.length && filled.every((t, i) => t === trackers[i]);
  return same ? slots : trackers.slice();
}
