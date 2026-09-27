import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const dir = dirname(fileURLToPath(import.meta.url));
const rows = readFileSync(join(dir, "VENDOR.md"), "utf8")
  .split("\n")
  .map((l) => /^\|\s*`([^`]+)`\s*\|.*\|\s*([0-9a-f]{64})\s*\|\s*$/.exec(l))
  .filter(Boolean);

test("VENDOR.md lists exactly the three vendored files", () => {
  assert.deepEqual(rows.map((r) => r[1]).sort(), ["hooks.mjs", "htm.mjs", "preact.mjs"]);
});

test("every vendored file matches its recorded SHA-256", () => {
  for (const [, file, sum] of rows) {
    const got = createHash("sha256").update(readFileSync(join(dir, file))).digest("hex");
    assert.equal(got, sum, `${file} changed without updating VENDOR.md`);
  }
});
