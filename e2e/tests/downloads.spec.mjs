import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

// E01 is complete; E02 has only its first piece, so the torrent keeps
// downloading it and its row shows no link or copy button.
async function mixedShow(app) {
  const t = makeTorrent("Mixed Show", [{ path: "E01.mkv", size: 32768 }, { path: "E02.mkv", size: 32768 }]);
  await writeData(app.downloads, t, { partial: { "E02.mkv": 16384 } });
  await app.addTorrent(t.torrent);
}

async function grantClipboard(page, app) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(app.url).origin });
}

const row = (page, name) => page.locator(".node > .row", { hasText: name });

test("Copy link puts the file's absolute link on the clipboard", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(row(page, "E01.mkv").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
  await expect(page.getByRole("button", { name: "Copy link to E01.mkv" })).toBeVisible({ timeout: 4000 });
});

test("a file that is still downloading has no copy button", async ({ page, app }) => {
  await mixedShow(app);
  await expect(row(page, "E02.mkv")).toBeVisible();
  await expect(page.getByRole("article", { name: "Mixed Show" }).locator(".badge")).toHaveText("Downloading");
  await expect(row(page, "E02.mkv").getByRole("button", { name: /^Copy/ })).toHaveCount(0);
});

test("Copy all copies only the folder's finished files, one per line", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await expect(row(page, "E02.mkv")).toBeVisible();
  await page.getByRole("button", { name: "Copy all links in Mixed Show" }).click();
  await expect(page.getByRole("button", { name: "Copied 1" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
});

test("Copy all on a finished folder copies every file", async ({ page, app }) => {
  await grantClipboard(page, app);
  const t = makeTorrent("Full Show", [{ path: "E01.mkv", size: 16384 }, { path: "E02.mkv", size: 16384 }]);
  await writeData(app.downloads, t);
  await app.addTorrent(t.torrent);
  await expect(page.getByRole("article", { name: "Full Show" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await page.getByRole("button", { name: "Copy all links in Full Show" }).click();
  await expect(page.getByRole("button", { name: "Copied 2" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    `${app.url}download/Full%20Show/E01.mkv\n${app.url}download/Full%20Show/E02.mkv`,
  );
});

test("on plain HTTP the copy falls back to execCommand", async ({ page, app }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", { value: false });
    window.__copied = [];
    const original = document.execCommand.bind(document);
    document.execCommand = (cmd, ...rest) => {
      if (cmd !== "copy") return original(cmd, ...rest);
      window.__copied.push(document.activeElement && document.activeElement.value);
      return true;
    };
  });
  await page.reload();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(row(page, "E01.mkv").getByRole("button", { name: "Copied" })).toBeVisible();
  expect(await page.evaluate(() => window.__copied)).toEqual([`${app.url}download/Mixed%20Show/E01.mkv`]);
});

test("when copying is blocked, the link opens selected for copying by hand", async ({ page, app }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", { value: false });
    document.execCommand = () => false;
  });
  await page.reload();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  const dialog = page.getByRole("dialog", { name: "Copy link" });
  await expect(dialog).toBeVisible();
  const area = dialog.getByRole("textbox");
  await expect(area).toHaveValue(`${app.url}download/Mixed%20Show/E01.mkv`);
  await expect(area).toBeFocused();
  expect(await area.evaluate((el) => el.selectionStart === 0 && el.selectionEnd === el.value.length)).toBe(true);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});

test("download rows are comfortable to tap on a phone", async ({ page, app }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await mixedShow(app);
  const button = page.getByRole("button", { name: "Copy link to E01.mkv" });
  const box = await button.boundingBox();
  expect(box.width).toBeGreaterThanOrEqual(36);
  expect(box.height).toBeGreaterThanOrEqual(36);
  expect((await row(page, "E01.mkv").boundingBox()).height).toBeGreaterThanOrEqual(40);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("pausing keeps half-written files out of Copy link and Copy all", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  const card = page.getByRole("article", { name: "Mixed Show" });
  await expect(card.locator(".badge")).toHaveText("Downloading");
  await card.getByRole("button", { name: "Pause" }).click();
  await expect(card.locator(".badge")).toHaveText("Paused");
  await expect(row(page, "E02.mkv")).toBeVisible();
  await expect(row(page, "E02.mkv").getByRole("button", { name: /^Copy/ })).toHaveCount(0);
  await page.getByRole("button", { name: "Copy all links in Mixed Show" }).click();
  await expect(page.getByRole("button", { name: "Copied 1" })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
});
