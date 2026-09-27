import { test } from "node:test";
import assert from "node:assert/strict";
import { STATUS_LABELS, torrentStatus, eta, statusLine } from "./status.js";

const GB = 1e9;
const MB = 1e6;
const t = (over) => ({ Loaded: true, Started: true, Percent: 0, Size: 0, Downloaded: 0, DownloadRate: 0, ...over });

test("torrentStatus follows the spec's rule table", () => {
  assert.equal(torrentStatus(t({ Loaded: false })), "loading");
  assert.equal(torrentStatus(t({ Started: true, Percent: 100 })), "seeding");
  assert.equal(torrentStatus(t({ Started: true, Percent: 42.5 })), "downloading");
  assert.equal(torrentStatus(t({ Started: false, Percent: 100 })), "done");
  assert.equal(torrentStatus(t({ Started: false, Percent: 63 })), "paused");
});

test("torrentStatus tolerates missing fields", () => {
  assert.equal(torrentStatus(null), "loading");
  assert.equal(torrentStatus({}), "loading");
  assert.equal(torrentStatus({ Loaded: true }), "paused");
  assert.equal(torrentStatus({ Loaded: true, Started: true }), "downloading");
});

test("every status has a sentence-case label", () => {
  assert.deepEqual(STATUS_LABELS, {
    loading: "Loading", downloading: "Downloading", seeding: "Seeding", done: "Done", paused: "Paused",
  });
});

test("eta formats the remaining time in friendly units", () => {
  const at = (secondsLeft) => eta(t({ Size: 1000 + secondsLeft * MB, Downloaded: 1000, DownloadRate: MB }));
  assert.equal(at(30), "less than a minute left");
  assert.equal(at(59), "less than a minute left");
  assert.equal(at(60), "about 1 min left");
  assert.equal(at(14 * 60), "about 14 min left");
  assert.equal(at(3590), "about 1 h left"); // rounds to 60 min
  assert.equal(at(2 * 3600 + 20 * 60), "about 2 h 20 min left");
  assert.equal(at(3 * 3600), "about 3 h left");
  assert.equal(at(10 * 3600 + 20 * 60), "about 10 h left"); // minutes dropped from 10 h
  assert.equal(at(23 * 3600 + 59 * 60 + 40), "more than a day left"); // rounds to 1440 min
  assert.equal(at(3 * 86400), "more than a day left");
});

test("eta is null when it can't be estimated", () => {
  assert.equal(eta(t({ Size: 10 * MB, Downloaded: 0, DownloadRate: 0 })), null);
  assert.equal(eta(t({ Size: 0, Downloaded: 0, DownloadRate: MB })), null);
  assert.equal(eta(t({ Size: 10, Downloaded: 20, DownloadRate: MB })), null);
  assert.equal(eta(t({ Started: false, Size: 10 * MB, DownloadRate: MB })), null);
  assert.equal(eta(t({ Percent: 100, Size: 10 * MB, DownloadRate: MB })), null);
  assert.equal(eta({ Loaded: true, Started: true, DownloadRate: MB }), null);
});

test("statusLine while downloading shows progress, rate and ETA", () => {
  assert.deepEqual(
    statusLine(t({ Size: 7.4 * GB, Downloaded: 3.1 * GB, Percent: 41.89, DownloadRate: 5.2 * MB })),
    { main: "3.1 GB of 7.4 GB · 41%", rate: "5.2 MB/s", note: "about 14 min left" },
  );
});

test("statusLine at 0 B/s says it is waiting for peers", () => {
  assert.deepEqual(
    statusLine(t({ Size: 64000, Downloaded: 0, Percent: 0 })),
    { main: "0 B of 64 KB · 0%", rate: "0 B/s", note: "waiting for peers" },
  );
});

test("statusLine for complete, paused and loading torrents", () => {
  assert.deepEqual(statusLine(t({ Started: true, Percent: 100, Size: 12 * MB, Downloaded: 12 * MB })),
    { main: "12 MB · complete", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Started: false, Percent: 100, Size: 12 * MB })),
    { main: "12 MB · complete", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Started: false, Percent: 63.2, Size: 2.1 * GB, Downloaded: 1.3 * GB })),
    { main: "1.3 GB of 2.1 GB · 63%", rate: null, note: null });
  assert.deepEqual(statusLine(t({ Loaded: false })), { main: "", rate: null, note: null });
});

test("statusLine never shows 100% for an unfinished torrent", () => {
  assert.equal(statusLine(t({ Percent: 99.99, Size: 100, Downloaded: 99 })).main, "99 B of 100 B · 99%");
});
