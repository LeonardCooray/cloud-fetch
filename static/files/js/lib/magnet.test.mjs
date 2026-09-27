import { test } from "node:test";
import assert from "node:assert/strict";
import { parseMagnet, buildMagnet, validInfohash } from "./magnet.js";

test("parseMagnet reads name, hash and every tracker", () => {
  const m = parseMagnet(
    "magnet:?xt=urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&dn=Big+Buck+Bunny&tr=udp%3A%2F%2Ftracker.a%3A80&tr=wss%3A%2F%2Ftracker.b",
  );
  assert.deepEqual(m, {
    name: "Big Buck Bunny",
    infohash: "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C",
    trackers: ["udp://tracker.a:80", "wss://tracker.b"],
  });
});

test("parseMagnet accepts base32 hashes and a missing name", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ"), {
    name: "",
    infohash: "3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ",
    trackers: [],
  });
});

test("parseMagnet keeps the fields when the hash is invalid", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:zz!&dn=x"), {
    name: "x",
    infohash: "zz!",
    trackers: [],
    error: "Invalid info hash",
  });
  assert.equal(parseMagnet("magnet:?dn=x").error, "Invalid info hash");
  assert.equal(parseMagnet("magnet:?dn=x").infohash, "");
});

test("buildMagnet keeps spaces and symbols in the name", () => {
  assert.equal(
    buildMagnet({ name: "Big Buck Bunny (2008) & co #1", infohash: "abc", trackers: ["udp://t:80", ""] }),
    "magnet:?xt=urn:btih:abc&dn=Big%20Buck%20Bunny%20(2008)%20%26%20co%20%231&tr=udp%3A%2F%2Ft%3A80",
  );
  assert.equal(buildMagnet({ name: "", infohash: "abc", trackers: [] }), "magnet:?xt=urn:btih:abc");
});

test("build then parse round-trips what a person typed", () => {
  const fields = { name: "Café + Crème 100% [x]", infohash: "abcdef0123", trackers: ["udp://a:1", "http://b/announce?k=1&z=2"] };
  assert.deepEqual(parseMagnet(buildMagnet(fields)), fields);
});

test("validInfohash", () => {
  assert.equal(validInfohash("abcDEF123"), true);
  assert.equal(validInfohash(""), false);
  assert.equal(validInfohash("ab c"), false);
});
