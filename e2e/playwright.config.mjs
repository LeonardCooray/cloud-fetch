import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.mjs",
  // each test starts its own server; one at a time keeps ports and CPU calm
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: process.env.CI ? [["list"]] : [["list"], ["html", { open: "never" }]],
  use: { ...devices["Desktop Chrome"], trace: "retain-on-failure" },
});
