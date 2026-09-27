import { test } from "node:test";
import assert from "node:assert/strict";
import { parseMagnet, buildMagnet, validInfohash, trackerSlots } from "./magnet.js";

test("parseMagnet reads name, hash and every tracker", () => {
  const m = parseMagnet(
    "magnet:?xt=urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&dn=Big+Buck+Bunny&tr=udp%3A%2F%2Ftracker.a%3A80&tr=wss%3A%2F%2Ftracker.b",
  );
  assert.deepEqual(m, {
    name: "Big Buck Bunny",
    infohash: "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C",
    trackers: ["udp://tracker.a:80", "wss://tracker.b"],
    extra: [],
  });
});

test("parseMagnet accepts base32 hashes and a missing name", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ"), {
    name: "",
    infohash: "3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ",
    trackers: [],
    extra: [],
  });
});

test("parseMagnet keeps the fields when the hash is invalid", () => {
  assert.deepEqual(parseMagnet("magnet:?xt=urn:btih:zz!&dn=x"), {
    name: "x",
    infohash: "zz!",
    trackers: [],
    extra: [],
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
  const fields = {
    name: "Café + Crème 100% [x]",
    infohash: "abcdef0123abcdef0123abcdef0123abcdef0123",
    trackers: ["udp://a:1", "http://b/announce?k=1&z=2"],
    extra: [],
  };
  assert.deepEqual(parseMagnet(buildMagnet(fields)), fields);
});

test("validInfohash takes 40 hex or 32 base32 characters", () => {
  assert.equal(validInfohash("DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C"), true);
  assert.equal(validInfohash("dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"), true);
  assert.equal(validInfohash("3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBYJ"), true);
  for (const bad of ["", "abcDEF123", "ab c", "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&tr=http://evil", "3I42H3S6NNFQ2MSVX7XZKYAYSCX5QBY1"]) {
    assert.equal(validInfohash(bad), false, bad);
  }
});

const HEX = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c";
const BTMH = "urn:btmh:1220cafe0000000000000000000000000000000000000000000000000000000000ee";

test("parseMagnet finds the btih hash after a btmh one", () => {
  const m = parseMagnet(`magnet:?xt=${BTMH}&xt=urn:btih:${HEX}&dn=x`);
  assert.equal(m.infohash, HEX);
  assert.equal(m.error, undefined);
  assert.deepEqual(m.extra, [["xt", BTMH]]);
});

test("parseMagnet explains a v2-only magnet", () => {
  assert.equal(parseMagnet(`magnet:?xt=${BTMH}&dn=x`).error, "v2-only magnets aren't supported yet");
});

test("editing keeps parameters the editor doesn't show", () => {
  const uri = `magnet:?xt=${BTMH}&xt=urn:btih:${HEX}&dn=x&xl=1234&tr=udp%3A%2F%2Fa%3A1&ws=https%3A%2F%2Fseed.example%2Ff`;
  const m = parseMagnet(uri);
  const edited = parseMagnet(buildMagnet({ ...m, name: "renamed" }));
  assert.equal(edited.name, "renamed");
  assert.equal(edited.infohash, HEX);
  assert.deepEqual(edited.trackers, ["udp://a:1"]);
  assert.deepEqual(edited.extra, [["xt", BTMH], ["xl", "1234"], ["ws", "https://seed.example/f"]]);
});

test("trackerSlots keeps a cleared field in place while its text catches up", () => {
  // the person cleared the first of two trackers: the magnet now lists one
  assert.deepEqual(trackerSlots(["", "udp://b:2"], ["udp://b:2"]), ["", "udp://b:2"]);
  // an edit elsewhere (the omni bar) replaces the slots
  assert.deepEqual(trackerSlots(["", "udp://b:2"], ["udp://c:3"]), ["udp://c:3"]);
  assert.deepEqual(trackerSlots([], ["udp://a:1"]), ["udp://a:1"]);
});
