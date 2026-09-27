import { test } from "node:test";
import assert from "node:assert/strict";
import {
  childPath, downloadHref, isDir, previewKind, fileIcon,
  findTorrentFile, isDownloading, startsClosed, sortTorrents,
  absoluteHref, finishedFiles,
} from "./tree.js";

test("childPath joins relative to the download root", () => {
  assert.equal(childPath("", "a"), "a");
  assert.equal(childPath("Show", "S01"), "Show/S01");
});

test("downloadHref encodes every segment so IDM and VLC fetch the exact file", () => {
  assert.equal(downloadHref("Show/S01/e 1 #2?.mkv"), "download/Show/S01/e%201%20%232%3F.mkv");
  assert.equal(downloadHref("Café/ü 100%.mp3"), "download/Caf%C3%A9/%C3%BC%20100%25.mp3");
});

test("isDir follows the server's Children field", () => {
  assert.equal(isDir({ Children: [] }), true);
  assert.equal(isDir({ Children: null }), false);
  assert.equal(isDir({}), false);
});

test("previewKind matches extensions case-insensitively", () => {
  assert.equal(previewKind("a/song.MP3"), "audio");
  assert.equal(previewKind("song.m4a"), "audio");
  for (const p of ["x.jpg", "x.JPEG", "x.png", "x.gif"]) assert.equal(previewKind(p), "image");
  for (const p of ["x.mp4", "x.MKV", "x.mov"]) assert.equal(previewKind(p), "video");
  assert.equal(previewKind("x.avi"), null);
  assert.equal(previewKind("notes.txt"), null);
});

test("fileIcon", () => {
  assert.equal(fileIcon("a.mp3"), "music");
  assert.equal(fileIcon("a.png"), "image");
  assert.equal(fileIcon("a.mkv"), "film");
  assert.equal(fileIcon("a.avi"), "film");
  assert.equal(fileIcon("a.txt"), "file");
});

const t1 = { InfoHash: "a", Name: "Show", Loaded: true, Started: true, Files: [{ Path: "Show/e1.mkv", Percent: 50 }, { Path: "Show/e2.mkv", Percent: 100 }] };
const t2 = { InfoHash: "b", Name: "Other", Loaded: false, Started: true, Files: null };

test("findTorrentFile finds the torrent a downloaded file belongs to", () => {
  assert.deepEqual(findTorrentFile({ a: t1, b: t2 }, "Show/e2.mkv"), { torrent: t1, file: t1.Files[1] });
  assert.equal(findTorrentFile({ a: t1, b: t2 }, "Show/e3.mkv"), null);
  assert.equal(findTorrentFile(null, "Show/e1.mkv"), null);
});

test("isDownloading only for loaded, started, unfinished files", () => {
  assert.equal(isDownloading({ torrent: t1, file: t1.Files[0] }), true);
  assert.equal(isDownloading({ torrent: t1, file: t1.Files[1] }), false);
  assert.equal(isDownloading({ torrent: { ...t1, Started: false }, file: t1.Files[0] }), false);
  assert.equal(isDownloading({ torrent: { ...t1, Loaded: false }, file: t1.Files[0] }), false);
  assert.equal(isDownloading(null), false);
});

test("folders untouched for more than a day start closed", () => {
  const now = Date.parse("2026-09-27T12:00:00Z");
  assert.equal(startsClosed({ Modified: new Date(now - 25 * 3600e3).toISOString() }, now), true);
  assert.equal(startsClosed({ Modified: new Date(now - 2 * 3600e3).toISOString() }, now), false);
});

test("sortTorrents orders by name, falling back to the hash", () => {
  const list = sortTorrents({
    b: { InfoHash: "b", Name: "zeta" },
    a: { InfoHash: "a", Name: "Alpha" },
    c: { InfoHash: "c", Name: "" },
  });
  assert.deepEqual(list.map((t) => t.InfoHash), ["a", "c", "b"]);
  assert.deepEqual(sortTorrents(null), []);
});

test("absoluteHref resolves against the page, keeping port and subpath", () => {
  assert.equal(absoluteHref("Show/E01.mkv", "http://127.0.0.1:3000/"), "http://127.0.0.1:3000/download/Show/E01.mkv");
  assert.equal(absoluteHref("a.iso", "https://fetch.example.com/cf/"), "https://fetch.example.com/cf/download/a.iso");
  assert.equal(absoluteHref("a.iso", "https://fetch.example.com/cf/?x=1#top"), "https://fetch.example.com/cf/download/a.iso");
});

test("absoluteHref encodes awkward names so IDM and VLC fetch the exact file", () => {
  assert.equal(
    absoluteHref("My Show/e 1 #2?.mkv", "https://h/"),
    "https://h/download/My%20Show/e%201%20%232%3F.mkv",
  );
  assert.equal(absoluteHref("Café/ü 100%.mp3", "https://h/"), "https://h/download/Caf%C3%A9/%C3%BC%20100%25.mp3");
});

const tree = {
  Name: "Show",
  Children: [
    { Name: "E01.mkv", Children: null },
    { Name: "Extras", Children: [{ Name: "b.txt", Children: null }, { Name: "Empty", Children: [] }] },
    { Name: "E02.mkv", Children: null },
  ],
};
const torrent = (files, over = {}) => ({ h: { Loaded: true, Started: true, Files: files, ...over } });

test("finishedFiles walks a folder in tree order", () => {
  assert.deepEqual(finishedFiles(tree, "Show", {}), ["Show/E01.mkv", "Show/Extras/b.txt", "Show/E02.mkv"]);
});

test("finishedFiles skips files a started torrent is still writing", () => {
  const torrents = torrent([{ Path: "Show/E01.mkv", Percent: 100 }, { Path: "Show/E02.mkv", Percent: 50 }]);
  assert.deepEqual(finishedFiles(tree, "Show", torrents), ["Show/E01.mkv", "Show/Extras/b.txt"]);
});

test("finishedFiles keeps a paused torrent's partial files, as their links show", () => {
  const torrents = torrent([{ Path: "Show/E02.mkv", Percent: 50 }], { Started: false });
  assert.deepEqual(finishedFiles(tree, "Show", torrents), ["Show/E01.mkv", "Show/Extras/b.txt", "Show/E02.mkv"]);
});

test("finishedFiles on a single file", () => {
  assert.deepEqual(finishedFiles({ Name: "a.iso", Children: null }, "a.iso", {}), ["a.iso"]);
  const busy = torrent([{ Path: "a.iso", Percent: 10 }]);
  assert.deepEqual(finishedFiles({ Name: "a.iso", Children: null }, "a.iso", busy), []);
});

test("finishedFiles on an empty folder", () => {
  assert.deepEqual(finishedFiles({ Name: "Empty", Children: [] }, "Empty", {}), []);
});

// anacrolix keeps an unfinished file on disk as "<name>.part" and renames it
// when it completes, so the Downloads tree lists the .part name
test("findTorrentFile matches a file still named .part on disk", () => {
  const torrents = torrent([{ Path: "Show/E02.mkv", Percent: 50 }]);
  assert.equal(findTorrentFile(torrents, "Show/E02.mkv.part").file.Path, "Show/E02.mkv");
  assert.equal(findTorrentFile(torrents, "Show/E03.mkv.part"), null);
});

test("finishedFiles skips a .part file a started torrent is still writing", () => {
  const partTree = { Name: "Show", Children: [{ Name: "E01.mkv", Children: null }, { Name: "E02.mkv.part", Children: null }] };
  const torrents = torrent([{ Path: "Show/E01.mkv", Percent: 100 }, { Path: "Show/E02.mkv", Percent: 50 }]);
  assert.deepEqual(finishedFiles(partTree, "Show", torrents), ["Show/E01.mkv"]);
});
