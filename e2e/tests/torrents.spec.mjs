import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

test("a complete torrent seeds, pauses to Done and resumes", async ({ page, app }) => {
  const t = makeTorrent("Fixture Show", [{ path: "E01.mkv", size: 32768 }, { path: "E02.mkv", size: 32768 }]);
  await writeData(app.downloads, t);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "Fixture Show" });
  await expect(card.locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await expect(card.locator(".status")).toHaveText("65.5 KB · complete");
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Done");
  await expect(card.getByRole("button", { name: "Remove" })).toBeVisible();
  await card.getByRole("button", { name: "Resume" }).click();
  await expect(card.locator(".badge")).toHaveText("Seeding");
  await expect(card.getByRole("button", { name: "Remove" })).toHaveCount(0);
});

test("an incomplete torrent downloads, waits for peers and pauses", async ({ page, app }) => {
  const t = makeTorrent("lonely.bin", [{ path: "lonely.bin", size: 65536 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "lonely.bin" });
  await expect(card.locator(".badge")).toHaveText("Downloading");
  await expect(card.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 0 B/s · waiting for peers");
  await expect(card.getByRole("button", { name: "Start" })).toHaveCount(0);
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Paused");
  await expect(card.locator(".status")).toHaveText("0 B of 65.5 KB · 0%");
  await expect(card.getByRole("button", { name: "Remove" })).toBeVisible();
});

test("the infohash lives in the Files panel, not the card header", async ({ page, app }) => {
  const t = makeTorrent("hash.bin", [{ path: "hash.bin", size: 16384 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "hash.bin" });
  await expect(card.locator(".badge")).toBeVisible();
  await expect(card.getByText(/^#[0-9a-f]{40}$/)).toHaveCount(0);
  await card.getByRole("button", { name: "Files" }).click();
  await expect(card.locator(".files-panel .hash")).toHaveText(/^#[0-9a-f]{40}$/);
});

test("card buttons get their own full-width row on a phone", async ({ page, app }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  const t = makeTorrent("phone.bin", [{ path: "phone.bin", size: 16384 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "phone.bin" });
  await expect(card.locator(".badge")).toBeVisible();
  const name = await card.locator(".name").boundingBox();
  const buttons = card.locator(".buttons button");
  for (const b of await buttons.all()) {
    const box = await b.boundingBox();
    expect(box.y).toBeGreaterThanOrEqual(name.y + name.height - 1);
    expect(box.height).toBeGreaterThanOrEqual(36);
    expect(box.width).toBeGreaterThanOrEqual(80);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
