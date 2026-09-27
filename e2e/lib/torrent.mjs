import { createHash, randomBytes } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";

export const PIECE = 16384;

export function bencode(v) {
  if (Buffer.isBuffer(v)) return Buffer.concat([Buffer.from(`${v.length}:`), v]);
  if (typeof v === "string") return bencode(Buffer.from(v));
  if (Number.isInteger(v)) return Buffer.from(`i${v}e`);
  if (Array.isArray(v)) return Buffer.concat([Buffer.from("l"), ...v.map(bencode), Buffer.from("e")]);
  const keys = Object.keys(v).sort();
  return Buffer.concat([Buffer.from("d"), ...keys.flatMap((k) => [bencode(k), bencode(v[k])]), Buffer.from("e")]);
}

// makeTorrent builds a trackerless .torrent with random data. A single file
// whose path equals the name is a single-file torrent; otherwise paths are
// relative to the folder `name`.
export function makeTorrent(name, files) {
  const withData = files.map((f) => ({ ...f, data: randomBytes(f.size) }));
  const all = Buffer.concat(withData.map((f) => f.data));
  const pieces = [];
  for (let i = 0; i < all.length; i += PIECE) {
    pieces.push(createHash("sha1").update(all.subarray(i, i + PIECE)).digest());
  }
  const single = withData.length === 1 && withData[0].path === name;
  const info = { name, "piece length": PIECE, pieces: Buffer.concat(pieces) };
  if (single) info.length = withData[0].size;
  else info.files = withData.map((f) => ({ length: f.size, path: f.path.split("/") }));
  return { name, single, torrent: bencode({ info }), files: withData };
}

export async function writeData(downloads, t, { partial = {} } = {}) {
  for (const f of t.files) {
    const target = t.single ? join(downloads, t.name) : join(downloads, t.name, ...f.path.split("/"));
    await mkdir(dirname(target), { recursive: true });
    const n = f.path in partial ? partial[f.path] : f.size;
    await writeFile(target, f.data.subarray(0, n));
  }
}
