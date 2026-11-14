import { test, expect } from "../fixtures.mjs";
import { makeTorrent } from "../lib/torrent.mjs";

// Torrents here have no peers, so none ever finishes and frees its slot.
test.use({ serverOptions: { maxActive: 1 } });

async function addAll(app, page, names) {
  const cards = {};
  for (const name of names) {
    await app.addTorrent(makeTorrent(name, [{ path: name, size: 65536 }]).torrent);
    cards[name] = page.getByRole("article", { name });
    // added one at a time, so the add order (the queue order) is certain
    await expect(cards[name]).toBeVisible();
  }
  return cards;
}

test("torrents over the limit wait in line, and Start now starts one as an extra", async ({ page, app }) => {
  const { "a.bin": a, "b.bin": b, "c.bin": c } = await addAll(app, page, ["a.bin", "b.bin", "c.bin"]);
  await expect(a.locator(".badge")).toHaveText("Downloading");
  await expect(b.locator(".badge")).toHaveText("Queued");
  await expect(b.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 1st in line");
  await expect(c.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 2nd in line");
  await expect(a.getByRole("button", { name: "Start now" })).toHaveCount(0);

  await b.getByRole("button", { name: "Start now" }).click();
  await expect(b.locator(".badge")).toHaveText("Downloading");
  await expect(b.getByRole("button", { name: "Start now" })).toHaveCount(0);
  await expect(a.locator(".badge")).toHaveText("Downloading");
  // b, started earlier, takes the next freed slot before c
  await expect(c.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 2nd in line");
  await a.getByRole("button", { name: "Pause" }).click();
  await expect(b.locator(".badge")).toHaveText("Downloading");
  await expect(c.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 1st in line");
});

test("pausing a running torrent hands its slot on, and Resume joins the back of the line", async ({ page, app }) => {
  const { "a.bin": a, "b.bin": b, "c.bin": c } = await addAll(app, page, ["a.bin", "b.bin", "c.bin"]);
  await expect(b.locator(".badge")).toHaveText("Queued");
  await a.getByRole("button", { name: "Pause" }).click();
  await expect(a.locator(".badge")).toHaveText("Paused");
  await expect(b.locator(".badge")).toHaveText("Downloading");
  await expect(c.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 1st in line");
  await a.getByRole("button", { name: "Resume" }).click();
  await expect(a.locator(".badge")).toHaveText("Queued");
  await expect(a.locator(".status")).toHaveText("0 B of 65.5 KB · 0% · 2nd in line");
});

test("Start now from the keyboard hands focus to the card's Pause button", async ({ page, app }) => {
  const { "b.bin": b } = await addAll(app, page, ["a.bin", "b.bin"]);
  const startNow = b.getByRole("button", { name: "Start now" });
  await startNow.focus();
  await page.keyboard.press("Enter");
  await expect(b.locator(".badge")).toHaveText("Downloading");
  await expect(b.getByRole("button", { name: "Pause" })).toBeFocused();
});

test("setting Max Active Torrents to 0 starts everything queued", async ({ page, app }) => {
  const { "b.bin": b, "c.bin": c } = await addAll(app, page, ["a.bin", "b.bin", "c.bin"]);
  await expect(c.locator(".badge")).toHaveText("Queued");
  await page.getByRole("button", { name: "Settings" }).click();
  await expect(page.getByLabel("Max Active Torrents")).toHaveValue("1");
  await page.getByLabel("Max Active Torrents").fill("0");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(b.locator(".badge")).toHaveText("Downloading");
  await expect(c.locator(".badge")).toHaveText("Downloading");
});

test("speed limits are saved from Settings, and out-of-range values are refused", async ({ page }) => {
  await page.getByRole("button", { name: "Settings" }).click();
  const down = page.getByLabel("Max Download KB/s");
  const up = page.getByLabel("Max Upload KB/s");
  await expect(down).toHaveValue("0");
  await expect(up).toHaveValue("0");

  await down.fill("-1");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".field-error")).toHaveText("Enter 0 to 2000000 KB/s for Max Download KB/s (0 is unlimited)");
  await down.fill("500");
  await up.fill("2000001");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".field-error")).toHaveText("Enter 0 to 2000000 KB/s for Max Upload KB/s (0 is unlimited)");
  await up.fill("50");
  await page.getByLabel("Max Active Torrents").fill("10001");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".field-error")).toHaveText("Enter 0 to 10000 for Max Active Torrents (0 is no limit)");
  await page.getByLabel("Max Active Torrents").fill("3");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator("form.config")).toHaveCount(0);

  // the form copies the synced config when it opens, and the push with the
  // saved values can land a moment after Save, so reopen until it has
  await expect(async () => {
    await page.getByRole("button", { name: "Settings" }).click();
    try {
      await expect(down).toHaveValue("500", { timeout: 200 });
      await expect(up).toHaveValue("50", { timeout: 200 });
      await expect(page.getByLabel("Max Active Torrents")).toHaveValue("3", { timeout: 200 });
    } catch (e) {
      await page.getByRole("button", { name: "Cancel" }).click();
      throw e;
    }
  }).toPass();
});
