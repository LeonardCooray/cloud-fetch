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
    file: (action, infohash, path) => post("file", `${action}:${infohash}:${path}`),
    configure: (config) => post("configure", JSON.stringify(config)),
    search: (provider, query, page) =>
      getJSON(`search/${enc(provider)}?${new URLSearchParams({ query, page: String(page) })}`),
    searchItem: (provider, path) => getJSON(`search/${enc(provider)}/item?${new URLSearchParams({ item: path })}`),
    deleteDownload: (path) => request(downloadHref(path), { method: "DELETE" }),
  };
}
