import { test } from "node:test";
import assert from "node:assert/strict";
import { safeHttpUrl, providerList, pickProvider, normalizeResults, resolveItem, resolveLookup } from "./search.js";

test("safeHttpUrl only lets http(s) through", () => {
  assert.equal(safeHttpUrl("https://x.example/a"), "https://x.example/a");
  assert.equal(safeHttpUrl("HTTP://x.example"), "HTTP://x.example");
  for (const bad of ["javascript:alert(1)", " javascript:alert(1)", "data:text/html,x", "//evil.example", "", undefined]) {
    assert.equal(safeHttpUrl(bad), "");
  }
});

test("providerList hides item endpoints and keeps order", () => {
  const list = providerList({ nyaa: { name: "Nyaa" }, "nyaa/item": { name: "Nyaa (Item)" }, abb: { name: "ABB" } });
  assert.deepEqual(list, [{ id: "nyaa", name: "Nyaa" }, { id: "abb", name: "ABB" }]);
  assert.deepEqual(providerList(null), []);
});

test("pickProvider falls back to the first provider", () => {
  const list = [{ id: "nyaa", name: "Nyaa" }, { id: "abb", name: "ABB" }];
  assert.equal(pickProvider(list, "abb"), "abb");
  assert.equal(pickProvider(list, "tpb"), "nyaa");
  assert.equal(pickProvider([], "tpb"), "");
});

test("normalizeResults makes relative links absolute and keeps the path", () => {
  const [r] = normalizeResults(
    [{ name: "x", url: "/t/1", torrent: "/dl/1.torrent", magnet: "magnet:?xt=urn:btih:aa", seeds: "5" }],
    "https://site.example/search?q={{query}}",
  );
  assert.deepEqual(r, {
    name: "x", url: "https://site.example/t/1", path: "/t/1",
    torrent: "https://site.example/dl/1.torrent", magnet: "magnet:?xt=urn:btih:aa", seeds: "5",
  });
});

test("normalizeResults drops script and data links from hostile pages", () => {
  const [r] = normalizeResults(
    [{ name: "x", url: "javascript:alert(1)", magnet: "javascript:alert(2)", torrent: "data:x" }],
    "https://site.example/",
  );
  assert.equal(r.url, "");
  assert.equal(r.magnet, "");
  assert.equal(r.torrent, "");
  assert.equal(r.path, undefined);
});

test("normalizeResults leaves absolute links alone", () => {
  const [r] = normalizeResults([{ name: "x", url: "https://other.example/x" }], "https://site.example/");
  assert.equal(r.url, "https://other.example/x");
  assert.equal(r.path, undefined);
});

test("resolveItem prefers magnet, then torrent, then a lookup", () => {
  assert.deepEqual(resolveItem({ magnet: "magnet:?xt=urn:btih:a", torrent: "https://s/x" }), { kind: "magnet", value: "magnet:?xt=urn:btih:a" });
  assert.deepEqual(resolveItem({ torrent: "https://s/x.torrent" }), { kind: "url", value: "https://s/x.torrent" });
  assert.deepEqual(resolveItem({ path: "/t/1" }), { kind: "lookup", path: "/t/1" });
  assert.deepEqual(resolveItem({}), { error: "No item URL found" });
});

test("resolveLookup turns an item page into something the server can add", () => {
  assert.deepEqual(resolveLookup({ torrent: "https://s/x.torrent", magnet: "magnet:?xt=urn:btih:a" }, "n"), { kind: "url", value: "https://s/x.torrent" });
  assert.deepEqual(resolveLookup({ magnet: "magnet:?xt=urn:btih:a" }, "n"), { kind: "magnet", value: "magnet:?xt=urn:btih:a" });
  assert.deepEqual(
    resolveLookup({ infohash: "abc", tracker: "udp://a:1, http://b/ann ,https://c/ann,wss://d" }, "My Name"),
    { kind: "magnet", value: "magnet:?xt=urn:btih:abc&dn=My%20Name&tr=udp%3A%2F%2Fa%3A1&tr=http%3A%2F%2Fb%2Fann" },
  );
  assert.deepEqual(resolveLookup({ torrent: "javascript:x" }, "n"), { error: "No magnet or infohash found" });
  assert.deepEqual(resolveLookup({}, "n"), { error: "No magnet or infohash found" });
  assert.deepEqual(resolveLookup(null, "n"), { error: "No response" });
});
