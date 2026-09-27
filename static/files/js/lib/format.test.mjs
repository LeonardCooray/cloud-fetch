import { test } from "node:test";
import assert from "node:assert/strict";
import { bytes, ago, hoursSince, filename, addSpaces, inputType } from "./format.js";

test("bytes uses metric units with at most one decimal", () => {
  assert.equal(bytes(0), "0 B");
  assert.equal(bytes(999), "999 B");
  assert.equal(bytes(1000), "1 KB");
  assert.equal(bytes(300000), "300 KB");
  assert.equal(bytes(1536000), "1.5 MB");
  assert.equal(bytes(631000000), "631 MB");
  assert.equal(bytes(5.94e9), "5.9 GB");
  assert.equal(bytes(2e15), "2 PB");
});

test("bytes treats missing or invalid sizes as zero", () => {
  for (const v of [undefined, null, NaN, -5, "12", Infinity]) assert.equal(bytes(v), "0 B");
});

const now = Date.parse("2026-09-27T12:00:00Z");
const secondsAgo = (s) => new Date(now - s * 1000).toISOString();

test("ago reads like moment's fromNow", () => {
  assert.equal(ago(secondsAgo(10), now), "just now");
  assert.equal(ago(secondsAgo(60), now), "a minute ago");
  assert.equal(ago(secondsAgo(90), now), "2 minutes ago");
  assert.equal(ago(secondsAgo(3600), now), "an hour ago");
  assert.equal(ago(secondsAgo(5 * 3600), now), "5 hours ago");
  assert.equal(ago(secondsAgo(30 * 3600), now), "a day ago");
  assert.equal(ago(secondsAgo(3 * 86400), now), "3 days ago");
  assert.equal(ago(secondsAgo(45 * 86400), now), "2 months ago");
  assert.equal(ago(secondsAgo(400 * 86400), now), "a year ago");
});

test("ago copes with clock skew and bad input", () => {
  assert.equal(ago(secondsAgo(-120), now), "just now");
  assert.equal(ago("not a date", now), "");
});

test("hoursSince", () => {
  assert.equal(hoursSince(secondsAgo(25 * 3600), now), 25);
});

test("filename returns the last path segment", () => {
  assert.equal(filename("Show/S01/e1.mkv"), "e1.mkv");
  assert.equal(filename("e1.mkv"), "e1.mkv");
});

test("addSpaces splits Go field names into words", () => {
  assert.equal(addSpaces("DownloadDirectory"), "Download Directory");
  assert.equal(addSpaces("EnableUpload"), "Enable Upload");
  assert.equal(addSpaces("AutoStart"), "Auto Start");
  assert.equal(addSpaces(5), 5);
});

test("inputType picks the form control for a config value", () => {
  assert.equal(inputType(true), "checkbox");
  assert.equal(inputType(50007), "number");
  assert.equal(inputType("/downloads"), "text");
  assert.equal(inputType(null), "text");
});
