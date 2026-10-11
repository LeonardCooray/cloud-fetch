import { test, expect } from "../fixtures.mjs";
import { mkdir, writeFile, utimes } from "node:fs/promises";
import { join } from "node:path";

const row = (page, name) => page.locator(".downloads .node > .row", { hasText: name });

async function download(dir, name, daysOld) {
  await mkdir(join(dir, name), { recursive: true });
  const file = join(dir, name, "e1.mkv");
  await writeFile(file, "x");
  const when = new Date(Date.now() - daysOld * 86400_000);
  await utimes(file, when, when);
  await utimes(join(dir, name), when, when);
}

test("turning on cleanup deletes downloads older than the set days", async ({ page, app }) => {
  await download(app.downloads, "Old Show", 30);
  await download(app.downloads, "New Show", 1);
  await expect(row(page, "Old Show")).toBeVisible();
  await expect(row(page, "New Show")).toBeVisible();

  await page.getByRole("button", { name: "Settings" }).click();
  await expect(page.getByLabel("Delete Downloads After Days")).toHaveValue("0");
  await page.getByLabel("Delete Downloads After Days").fill("7");
  await page.getByRole("button", { name: "Save" }).click();

  await expect(row(page, "Old Show")).toHaveCount(0, { timeout: 10_000 });
  await expect(row(page, "New Show")).toBeVisible();
  expect(app.logs.join("")).toContain("Cleanup: deleted Old Show, unchanged for 7 days");
});

test("cleanup days outside 0 to 3650 are refused in Settings", async ({ page }) => {
  await page.getByRole("button", { name: "Settings" }).click();
  await page.getByLabel("Delete Downloads After Days").fill("4000");
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.locator(".field-error")).toHaveText("Enter 0 to 3650 days for Delete Downloads After (0 keeps downloads forever)");
});
