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

const LIGHT_BG = "rgb(244, 245, 250)";
const DARK_BG = "rgb(11, 13, 20)";
const rootBg = (page) => page.evaluate(() => getComputedStyle(document.documentElement).backgroundColor);
const themeButton = (page) => page.getByRole("button", { name: /^Theme: / });

test("the theme button forces light or dark over the system setting and remembers it", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Auto (follows system)");
  await themeButton(page).click();
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Light");
  expect(await rootBg(page)).toBe(LIGHT_BG);
  await themeButton(page).click();
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Dark");
  expect(await rootBg(page)).toBe(DARK_BG);

  await page.reload();
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Dark");
  expect(await rootBg(page)).toBe(DARK_BG);

  await themeButton(page).click();
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Auto (follows system)");
  expect(await rootBg(page)).toBe(LIGHT_BG);
  await page.emulateMedia({ colorScheme: "dark" });
  expect(await rootBg(page)).toBe(DARK_BG);
});

test("forced light wins over a dark system", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await themeButton(page).click();
  await expect(themeButton(page)).toHaveAccessibleName("Theme: Light");
  expect(await rootBg(page)).toBe(LIGHT_BG);
  const metas = await page.locator('meta[name="theme-color"]').evaluateAll((ms) => ms.map((m) => m.content));
  expect(metas).toEqual([LIGHT_BG, LIGHT_BG]);
});
