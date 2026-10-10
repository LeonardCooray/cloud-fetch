import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

// The point of a share link is that it works without the login, so these
// run against a server with --auth.
test.use({ serverOptions: { auth: "leo:pw" }, httpCredentials: { username: "leo", password: "pw" } });

// E01 is complete; E02 has only its first piece.
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
// each row has two live regions, Copy link's and Share's
const announced = (page, name) => row(page, name).locator('[aria-live="polite"]', { hasText: "Share link copied" });
const clipboard = (page) => page.evaluate(() => navigator.clipboard.readText());
const shareLink = (app, path) => new RegExp(`^${app.url.replace(/\./g, "\\.")}share/(\\d+)/[\\w-]{22}/${path}$`);

test("Share copies a 24-hour link that downloads without the login", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  const share = page.getByRole("button", { name: "Share E01.mkv" });
  await share.click();
  const menu = page.getByRole("group", { name: "Share E01.mkv" });
  await expect(menu).toBeVisible();
  await expect(share).toHaveAttribute("aria-expanded", "true");
  await expect(menu.getByRole("button", { name: "24 hours" })).toBeFocused();
  await menu.getByRole("button", { name: "24 hours" }).click();
  await expect(announced(page, "E01.mkv")).toHaveText("Share link copied, lasts 24 hours");
  await expect(menu).toHaveCount(0);

  const link = await clipboard(page);
  const m = link.match(shareLink(app, "Mixed%20Show/E01\\.mkv"));
  expect(m, link).not.toBeNull();
  const expires = Number(m[1]) - Date.now() / 1000;
  expect(expires).toBeGreaterThan(86400 - 60);
  expect(expires).toBeLessThanOrEqual(86400);

  // Node's fetch carries no credentials
  const res = await fetch(link);
  expect(res.status).toBe(200);
  expect((await res.arrayBuffer()).byteLength).toBe(32768);
  const ranged = await fetch(link, { headers: { Range: "bytes=0-99" } });
  expect(ranged.status).toBe(206);
  expect((await fetch(`${app.url}download/Mixed%20Show/E01.mkv`)).status).toBe(401);
});

test("the 1 hour choice signs a link that lasts an hour", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Share E01.mkv" }).click();
  await page.getByRole("button", { name: "1 hour" }).click();
  await expect(announced(page, "E01.mkv")).toHaveText("Share link copied, lasts 1 hour");
  const expires = Number((await clipboard(page)).match(shareLink(app, "Mixed%20Show/E01\\.mkv"))[1]) - Date.now() / 1000;
  expect(expires).toBeGreaterThan(3600 - 60);
  expect(expires).toBeLessThanOrEqual(3600);
});

test("Share on a folder copies a link per finished file", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Share Mixed Show" }).click();
  await page.getByRole("button", { name: "7 days" }).click();
  await expect(announced(page, "Mixed Show")).toHaveText("Share link copied, lasts 7 days");
  const lines = (await clipboard(page)).split("\n");
  expect(lines).toHaveLength(1);
  expect(lines[0]).toMatch(shareLink(app, "Mixed%20Show/E01\\.mkv"));
});

test("Escape closes the share menu and returns focus", async ({ page, app }) => {
  await mixedShow(app);
  const share = page.getByRole("button", { name: "Share E01.mkv" });
  await share.click();
  await expect(page.getByRole("group", { name: "Share E01.mkv" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("group", { name: "Share E01.mkv" })).toHaveCount(0);
  await expect(share).toBeFocused();
  await expect(share).toHaveAttribute("aria-expanded", "false");
});

test("Revoke all share links in Settings stops links already shared", async ({ page, app }) => {
  await grantClipboard(page, app);
  await mixedShow(app);
  await page.getByRole("button", { name: "Share E01.mkv" }).click();
  await page.getByRole("button", { name: "24 hours" }).click();
  await expect(announced(page, "E01.mkv")).toHaveText("Share link copied, lasts 24 hours");
  const link = await clipboard(page);
  expect((await fetch(link)).status).toBe(200);

  await page.getByRole("button", { name: "Settings" }).click();
  const revoke = page.getByRole("button", { name: "Revoke all share links" });
  await revoke.click();
  const confirm = page.getByRole("button", { name: "Click again to revoke all" });
  await expect(confirm).toBeVisible();
  // a double-click mustn't revoke; the second click needs a moment
  await page.waitForTimeout(450);
  await confirm.click();
  await expect(page.locator(".share-revoke [aria-live=polite]")).toHaveText("All share links revoked");
  expect((await fetch(link)).status).toBe(403);
});
