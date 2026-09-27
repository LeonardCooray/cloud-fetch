import { test } from "node:test";
import assert from "node:assert/strict";
import { classify } from "./omni.js";

test("classify decides what the omni bar will do", () => {
  assert.equal(classify(""), "empty");
  assert.equal(classify("   "), "empty");
  assert.equal(classify(undefined), "empty");
  assert.equal(classify("https://site.example/a.torrent"), "torrent-url");
  assert.equal(classify("HTTP://SITE.EXAMPLE/A"), "torrent-url");
  assert.equal(classify("magnet:?xt=urn:btih:abc"), "magnet");
  assert.equal(classify("ubuntu 24.04"), "search");
  assert.equal(classify("magnet"), "search");
});
