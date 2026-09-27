import { test } from "node:test";
import assert from "node:assert/strict";
import { createApi, errorMessage } from "./api.js";

function stubFetch(...responses) {
  const calls = [];
  const f = async (url, init = {}) => {
    calls.push({ url, method: init.method, body: init.body });
    const r = responses.shift();
    if (r instanceof Error) throw r;
    return { ok: r.status >= 200 && r.status < 300, status: r.status, text: async () => r.body };
  };
  f.calls = calls;
  return f;
}
const ok = (body = "OK") => ({ status: 200, body });

test("torrent actions post the exact bodies the server expects", async () => {
  const f = stubFetch(ok(), ok(), ok(), ok(), ok());
  const api = createApi(f);
  const bytes = new Uint8Array([1, 2, 3]);
  await api.magnet("magnet:?xt=urn:btih:a");
  await api.url("https://s/x.torrent");
  await api.torrentFile(bytes);
  await api.torrent("stop", "abc");
  await api.configure({ IncomingPort: 50007, AutoStart: true });
  assert.deepEqual(f.calls, [
    { url: "api/magnet", method: "POST", body: "magnet:?xt=urn:btih:a" },
    { url: "api/url", method: "POST", body: "https://s/x.torrent" },
    { url: "api/torrentfile", method: "POST", body: bytes },
    { url: "api/torrent", method: "POST", body: "stop:abc" },
    { url: "api/configure", method: "POST", body: '{"IncomingPort":50007,"AutoStart":true}' },
  ]);
});

test("search and item lookups encode their parameters and parse JSON", async () => {
  const f = stubFetch(ok('[{"name":"a"}]'), ok('{"infohash":"abc"}'));
  const api = createApi(f);
  assert.deepEqual(await api.search("nyaa", "ubuntu 24 & co", 2), [{ name: "a" }]);
  assert.deepEqual(await api.searchItem("nyaa", "/view/1?x=2"), { infohash: "abc" });
  assert.equal(f.calls[0].url, "search/nyaa?query=ubuntu+24+%26+co&page=2");
  assert.equal(f.calls[0].method, "GET");
  assert.equal(f.calls[1].url, "search/nyaa/item?item=%2Fview%2F1%3Fx%3D2");
});

test("deleteDownload sends DELETE to the encoded path", async () => {
  const f = stubFetch(ok(""));
  await createApi(f).deleteDownload("Show/e 1 #2.mkv");
  assert.deepEqual(f.calls[0], { url: "download/Show/e%201%20%232.mkv", method: "DELETE", body: undefined });
});

test("server errors reject with the server's message", async () => {
  const api = createApi(stubFetch({ status: 400, body: "Already started" }, { status: 500, body: '{"error":"Endpoint /x not found"}' }, { status: 502, body: "  " }));
  await assert.rejects(api.torrent("start", "a"), { message: "Already started" });
  await assert.rejects(api.search("x", "q", 1), { message: "Endpoint /x not found" });
  await assert.rejects(api.url("https://s/x"), { message: "Request failed (502)" });
});

test("network failures reject with the fetch error", async () => {
  const api = createApi(stubFetch(new TypeError("Failed to fetch")));
  await assert.rejects(api.magnet("magnet:?xt=urn:btih:a"), { message: "Failed to fetch" });
});

test("errorMessage caps long bodies", () => {
  const msg = errorMessage("x".repeat(1000), 500);
  assert.equal(msg.length, 301);
  assert.ok(msg.endsWith("…"));
});

test("onBusy reports true while any request is in flight, then false", async () => {
  let release;
  const gate = new Promise((r) => (release = r));
  const f = async () => { await gate; return { ok: true, status: 200, text: async () => "OK" }; };
  const api = createApi(f);
  const seen = [];
  const off = api.onBusy((b) => seen.push(b));
  const a = api.magnet("magnet:?xt=urn:btih:a");
  const b = api.magnet("magnet:?xt=urn:btih:b");
  release();
  await Promise.all([a, b]);
  assert.deepEqual(seen, [true, false]);
  off();
  await api.magnet("magnet:?xt=urn:btih:c");
  assert.deepEqual(seen, [true, false]);
});

test("file selection posts action, infohash and a path that may contain colons", async () => {
  const f = stubFetch(ok(), ok());
  const api = createApi(f);
  await api.file("stop", "abc", "pack/a: b.bin");
  await api.file("start", "abc", "pack/c.bin");
  assert.deepEqual(f.calls, [
    { url: "api/file", method: "POST", body: "stop:abc:pack/a: b.bin" },
    { url: "api/file", method: "POST", body: "start:abc:pack/c.bin" },
  ]);
});
