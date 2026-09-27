import { test, expect } from "../fixtures.mjs";

async function searchFixture(page) {
  await page.getByRole("textbox", { name: "Search, magnet link or torrent URL" }).fill("fixture");
  const provider = page.getByRole("combobox", { name: "Search provider" });
  await expect(provider.locator("option", { hasText: "Fake" })).toHaveCount(1);
  await provider.selectOption({ label: "Fake" });
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByText("Fixture Result One")).toBeVisible();
}

test("search results have a header row on desktop", async ({ page }) => {
  await searchFixture(page);
  const table = page.locator("table.results");
  for (const name of ["Name", "Size", "Seeds", "Peers"]) {
    await expect(table.getByRole("columnheader", { name, exact: true })).toBeVisible();
  }
  const first = table.locator("tbody tr").first();
  await expect(first.locator("td.seeds")).toHaveText("42", { useInnerText: true });
  await expect(first.locator("td.peers")).toHaveText("7", { useInnerText: true });
});

test("search results stack with labelled counts on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await searchFixture(page);
  const first = page.locator("table.results tbody tr").first();
  await expect(first).toContainText("42 seeds");
  await expect(first).toContainText("7 peers");
  const name = await first.locator("td.name").boundingBox();
  const add = await first.getByRole("button", { name: /^Add / }).boundingBox();
  expect(add.y).toBeGreaterThan(name.y);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("settings fit a phone screen", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.getByRole("button", { name: "Settings" }).click();
  await expect(page.getByRole("button", { name: "Save" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test.describe("with a very long one-word title", () => {
  test.use({ serverOptions: { title: "Supercalifragilisticexpialidociousandthensomemoreletters" } });

  test("the header truncates instead of overflowing on a phone", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    const settings = await page.getByRole("button", { name: "Settings" }).boundingBox();
    expect(settings.x + settings.width).toBeLessThanOrEqual(375);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });
});
