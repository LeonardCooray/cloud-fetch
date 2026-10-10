import { test } from "node:test";
import assert from "node:assert/strict";
import { DEFAULT_TTL, SHARE_TTLS, shareText } from "./share.js";

test("shareText makes absolute links and drops credentials from the page URL", () => {
  const base = "https://leo:pw@fetch.example.com/";
  assert.equal(
    shareText(["share/1/abc/Show%20S01/e%201.mkv", "share/1/def/b.mkv"], base),
    "https://fetch.example.com/share/1/abc/Show%20S01/e%201.mkv\nhttps://fetch.example.com/share/1/def/b.mkv",
  );
});

test("shareText resolves against a page served under a subpath", () => {
  assert.equal(shareText(["share/1/a/x.mkv"], "http://h:3000/fetch/"), "http://h:3000/fetch/share/1/a/x.mkv");
});

test("the expiry choices are 1 hour, 24 hours and 7 days, defaulting to 24 hours", () => {
  assert.deepEqual(SHARE_TTLS.map((t) => t.seconds), [3600, 86400, 604800]);
  assert.equal(SHARE_TTLS[DEFAULT_TTL].label, "24 hours");
});
