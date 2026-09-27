import { test } from "node:test";
import assert from "node:assert/strict";
import { classify, droppedText } from "./omni.js";

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

const transfer = (types) => ({ getData: (t) => types[t] || "" });

test("droppedText takes the first link from a uri-list, skipping comments", () => {
  assert.equal(droppedText(transfer({ "text/uri-list": "# from a page\r\nhttps://s.example/a.torrent\r\nhttps://s.example/b" })), "https://s.example/a.torrent");
});

test("droppedText falls back to plain text, trimmed", () => {
  assert.equal(droppedText(transfer({ "text/plain": "  magnet:?xt=urn:btih:abc \n" })), "magnet:?xt=urn:btih:abc");
  assert.equal(droppedText(transfer({})), "");
  assert.equal(droppedText(null), "");
});
