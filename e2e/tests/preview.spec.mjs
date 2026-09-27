import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { test, expect } from "../fixtures.mjs";
import { silentMp3 } from "../lib/media.mjs";

const paused = (player) => player.evaluate((el) => el.paused);

// The player takes focus when a preview opens, so Space reaches it rather
// than the preview toggle that was just clicked (which would close it).
test("Space plays and pauses an audio preview without closing it", async ({ page, app }) => {
  await writeFile(join(app.downloads, "tone.mp3"), silentMp3());
  await page.getByRole("button", { name: "Preview tone.mp3" }).click();
  const player = page.locator("audio.preview");
  await expect(player).toBeFocused();
  await expect.poll(() => player.evaluate((el) => el.readyState)).toBeGreaterThanOrEqual(2);
  expect(await paused(player)).toBe(true);

  await page.keyboard.press("Space");
  await expect.poll(() => paused(player)).toBe(false);
  await expect(page.getByRole("button", { name: "Hide preview of tone.mp3" })).toHaveAttribute("aria-pressed", "true");

  await page.keyboard.press("Space");
  await expect.poll(() => paused(player)).toBe(true);
  await expect(player).toBeVisible();
});
