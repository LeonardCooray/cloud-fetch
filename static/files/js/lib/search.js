import { buildMagnet, validInfohash } from "./magnet.js";

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
    const infohash = String(data.infohash).trim();
    if (!validInfohash(infohash)) return { error: "The provider's info hash isn't valid" };
    const trackers = String(data.tracker || "")
      .split(",")
      .map((s) => s.trim())
      .filter((s) => /^(http|udp):\/\//.test(s));
    return { kind: "magnet", value: buildMagnet({ name, infohash, trackers }) };
  }
  return { error: "No magnet or infohash found" };
}
