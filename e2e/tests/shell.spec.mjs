import { test, expect } from "../fixtures.mjs";

test("the page loads connected, with the empty states", async ({ page }) => {
  await expect(page.getByRole("heading", { name: "Torrents" })).toBeVisible();
  await expect(page.getByText("Add torrents above")).toBeVisible();
  await expect(page.getByText("Download files above")).toBeVisible();
});

test("the light theme applies by default", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  const bg = await page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor);
  expect(bg).toBe("rgb(244, 245, 250)");
});

test("the dark theme follows the system setting", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  const colors = await page.evaluate(() => {
    const root = getComputedStyle(document.documentElement);
    return { bg: root.backgroundColor, text: root.color };
  });
  expect(colors).toEqual({ bg: "rgb(11, 13, 20)", text: "rgb(236, 238, 245)" });
});
