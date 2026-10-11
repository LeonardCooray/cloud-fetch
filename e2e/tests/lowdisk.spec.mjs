import { test, expect } from "../fixtures.mjs";
import { makeTorrent } from "../lib/torrent.mjs";

// No machine has a million GB free, so the guard reads the real disk as low.
test.use({ serverOptions: { minFreeGB: 1000000 } });

test("low disk space holds downloads, and lowering the minimum resumes them", async ({ page, app }) => {
  const t = makeTorrent("big.bin", [{ path: "big.bin", size: 65536 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "big.bin" });
  const notice = page.getByRole("status").filter({ hasText: "Downloads are on hold" });
  // the server checks free space every 5 seconds
  await expect(notice).toBeVisible({ timeout: 10_000 });
  await expect(notice).toContainText("less than 1000000 GB is free");
  await expect(card.locator(".badge")).toHaveText("On hold");
  await expect(card.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · waiting for disk space");

  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByLabel("Minimum Free Space GB").fill("0");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(notice).toHaveCount(0, { timeout: 10_000 });
  await expect(card.locator(".badge")).toHaveText("Downloading");
});

test("Resume is refused while space is low", async ({ page, app }) => {
  const t = makeTorrent("paused.bin", [{ path: "paused.bin", size: 65536 }]);
  await app.addTorrent(t.torrent);
  const card = page.getByRole("article", { name: "paused.bin" });
  await expect(page.getByRole("status").filter({ hasText: "Downloads are on hold" })).toBeVisible({ timeout: 10_000 });
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Paused");
  await card.getByRole("button", { name: "Resume" }).click();
  await expect(page.locator(".banner")).toContainText("downloads are on hold until space frees up");
  await expect(card.locator(".badge")).toHaveText("Paused");
});

test("a negative minimum is refused in Settings", async ({ page }) => {
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByLabel("Minimum Free Space GB").fill("-1");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".field-error")).toHaveText("Enter 0 or more GB for Minimum Free Space (0 turns the guard off)");
});
