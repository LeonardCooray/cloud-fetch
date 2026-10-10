import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

// E01 is complete; E02 has only its first piece, so the torrent keeps
// downloading it and its row links to the stream.
async function mixedShow(app) {
  const t = makeTorrent("Mixed Show", [{ path: "E01.mkv", size: 32768 }, { path: "E02.mkv", size: 32768 }]);
  await writeData(app.downloads, t, { partial: { "E02.mkv": 16384 } });
  await app.addTorrent(t.torrent);
  return t;
}

async function grantClipboard(page, app) {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(app.url).origin });
}

const row = (page, name) => page.locator(".node > .row", { hasText: name });
// the live region is what announces a copy; the button keeps its name
const announced = (scope) => scope.locator('[aria-live="polite"]');

test("Copy link puts the file's absolute link on the clipboard", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(announced(row(page, "E01.mkv"))).toHaveText("Copied");
  await expect(row(page, "E01.mkv").locator(".copied-text")).toHaveText("Copied");
  // button.copied only exists during the 2 s confirmation, so this pins the name then
  await expect(row(page, "E01.mkv").locator("button.copied")).toHaveAttribute("aria-label", "Copy link to E01.mkv");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
  await expect(row(page, "E01.mkv").locator(".copied-text")).toHaveCount(0, { timeout: 4000 });
});

// The link is the final name whatever the file is called on disk, and the
// row has no Delete while the torrent still writes it.
test("a file that is still downloading links to its stream", async ({ page, app }) => {
  const t = await mixedShow(app);
  await expect(page.getByRole("article", { name: "Mixed Show" }).locator(".badge")).toHaveText("Downloading");
  const link = row(page, "E02.mkv").locator("a.label");
  await expect(link).toHaveAttribute("href", "download/Mixed%20Show/E02.mkv");
  await expect(row(page, "E02.mkv").getByRole("button", { name: "Copy link to E02.mkv" })).toBeVisible();
  await expect(row(page, "E02.mkv").getByRole("button", { name: "Preview E02.mkv" })).toBeVisible();
  await expect(row(page, "E02.mkv").getByRole("button", { name: /^Delete/ })).toHaveCount(0);

  // the first piece is the one on disk, so it comes back without peers
  const res = await page.evaluate(async (href) => {
    const r = await fetch(href, { headers: { Range: "bytes=0-16383" } });
    return { status: r.status, range: r.headers.get("Content-Range"), body: Array.from(new Uint8Array(await r.arrayBuffer())) };
  }, await link.getAttribute("href"));
  expect(res.status).toBe(206);
  expect(res.range).toBe("bytes 0-16383/32768");
  expect(Buffer.from(res.body).equals(t.files[1].data.subarray(0, 16384))).toBe(true);
});

test("Copy all copies only the folder's finished files, one per line", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await expect(row(page, "E02.mkv")).toBeVisible();
  await page.getByRole("button", { name: "Copy all links in Mixed Show" }).click();
  await expect(announced(row(page, "Mixed Show"))).toHaveText("Copied 1");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
});

test("Copy all on a finished folder copies every file", async ({ page, app }) => {
  await grantClipboard(page, app);
  const t = makeTorrent("Full Show", [{ path: "E01.mkv", size: 16384 }, { path: "E02.mkv", size: 16384 }]);
  await writeData(app.downloads, t);
  await app.addTorrent(t.torrent);
  await expect(page.getByRole("article", { name: "Full Show" }).locator(".badge")).toHaveText("Done", { timeout: 30_000 });
  await page.getByRole("button", { name: "Copy all links in Full Show" }).click();
  await expect(announced(row(page, "Full Show"))).toHaveText("Copied 2");
  await expect(row(page, "Full Show").locator("button.copied")).toHaveAttribute("aria-label", "Copy all links in Full Show");
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
  await expect(announced(row(page, "E01.mkv"))).toHaveText("Copied");
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
  await expect(page.getByRole("button", { name: "Copy link to E01.mkv" })).toBeFocused();
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
  await expect(announced(row(page, "Mixed Show"))).toHaveText("Copied 1");
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${app.url}download/Mixed%20Show/E01.mkv`);
});

test("clicking outside closes the manual-copy popover without grabbing focus", async ({ page, app }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "isSecureContext", { value: false });
    document.execCommand = () => false;
  });
  await page.reload();
  await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
  await mixedShow(app);
  await page.getByRole("button", { name: "Copy link to E01.mkv" }).click();
  await expect(page.getByRole("dialog", { name: "Copy link" })).toBeVisible();
  await page.getByRole("textbox", { name: "Search, magnet link or torrent URL" }).click();
  await expect(page.getByRole("dialog", { name: "Copy link" })).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Search, magnet link or torrent URL" })).toBeFocused();
});
