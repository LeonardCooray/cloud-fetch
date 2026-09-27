import { fileURLToPath } from "node:url";
import { test, expect } from "../fixtures.mjs";
import { makeTorrent, writeData } from "../lib/torrent.mjs";

const OUT = fileURLToPath(new URL("../../docs/screenshot.png", import.meta.url));

// Regenerates the README screenshot: SCREENSHOT=1 npx playwright test screenshot
test.skip(!process.env.SCREENSHOT, "set SCREENSHOT=1 to regenerate docs/screenshot.png");
test.use({ viewport: { width: 1000, height: 900 }, deviceScaleFactor: 2 });

test("README screenshot", async ({ page, app }) => {
  const show = makeTorrent("Big Buck Bunny (2008)", [
    { path: "Big Buck Bunny.mp4", size: 16384 * 6 },
    { path: "Big Buck Bunny.en.srt", size: 16384 },
  ]);
  await writeData(app.downloads, show);
  await app.addTorrent(show.torrent);
  const iso = makeTorrent("debian-12.7.0-amd64-netinst.iso", [{ path: "debian-12.7.0-amd64-netinst.iso", size: 16384 * 4 }]);
  await writeData(app.downloads, iso);
  await app.addTorrent(iso.torrent);
  const partial = makeTorrent("ubuntu-24.04.1-desktop-amd64.iso", [{ path: "ubuntu-24.04.1-desktop-amd64.iso", size: 16384 * 8 }]);
  await writeData(app.downloads, partial, { partial: { "ubuntu-24.04.1-desktop-amd64.iso": 16384 * 3 } });
  await app.addTorrent(partial.torrent);
  await expect(page.getByRole("article", { name: "Big Buck Bunny (2008)" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  await expect(page.getByRole("article", { name: "debian-12.7.0-amd64-netinst.iso" }).locator(".badge")).toHaveText("Seeding", { timeout: 30_000 });
  const debian = page.getByRole("article", { name: "debian-12.7.0-amd64-netinst.iso" });
  await debian.getByRole("button", { name: "Pause" }).click();
  await expect(debian.locator(".badge")).toHaveText("Done");
  await page.getByRole("article", { name: "Big Buck Bunny (2008)" }).getByRole("button", { name: "Files" }).click();
  await page.screenshot({ path: OUT, fullPage: true });
});
