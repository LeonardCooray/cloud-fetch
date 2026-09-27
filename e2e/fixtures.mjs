import { test as base, expect } from "@playwright/test";
import { spawn } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import net from "node:net";
import { BINARY } from "./global-setup.mjs";
import { startFakeSearch } from "./lib/fake-search.mjs";

export function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.unref();
    s.on("error", reject);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address();
      s.close(() => resolve(port));
    });
  });
}

async function waitForHttp(url, proc, logs, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null || proc.signalCode !== null) {
      throw new Error(`cloud-fetch exited early:\n${logs.join("")}`);
    }
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`cloud-fetch did not answer on ${url}:\n${logs.join("")}`);
}

function exited(proc) {
  if (proc.exitCode !== null || proc.signalCode !== null) return Promise.resolve();
  return new Promise((r) => proc.once("exit", r));
}

// startServer launches the binary on fresh ports. freePort only proves a
// port was free a moment ago (and only for TCP, while the engine also binds
// UDP), so a bind collision is retried with new ports.
async function startServer({ dir, downloads, configPath, search, serverOptions }, attempts = 3) {
  for (let attempt = 1; ; attempt++) {
    const port = await freePort();
    await writeFile(configPath, JSON.stringify({
      AutoStart: true,
      DisableEncryption: false,
      DownloadDirectory: downloads,
      EnableUpload: false,
      EnableSeeding: false,
      IncomingPort: await freePort(),
    }));
    const args = ["--port", String(port), "--config-path", configPath, "--search-config-url", search.configUrl];
    if (serverOptions.title) args.push("--title", serverOptions.title);
    const logs = [];
    const proc = spawn(BINARY, args, { cwd: dir });
    proc.stdout.on("data", (d) => logs.push(String(d)));
    proc.stderr.on("data", (d) => logs.push(String(d)));
    const url = `http://127.0.0.1:${port}/`;
    try {
      await waitForHttp(url, proc, logs);
      return { proc, url, logs };
    } catch (e) {
      proc.kill("SIGTERM");
      await exited(proc);
      if (attempt >= attempts || !logs.join("").includes("address already in use")) throw e;
    }
  }
}

export const test = base.extend({
  serverOptions: [{}, { option: true }],

  app: async ({ serverOptions }, use) => {
    const dir = await mkdtemp(join(tmpdir(), "cf-e2e-"));
    const downloads = join(dir, "downloads");
    await mkdir(downloads);
    const search = await startFakeSearch();
    const configPath = join(dir, "cloud-fetch.json");
    let proc;
    try {
      let url, logs;
      ({ proc, url, logs } = await startServer({ dir, downloads, configPath, search, serverOptions }));
      const addTorrent = async (buf) => {
        const res = await fetch(url + "api/torrentfile", { method: "POST", body: buf });
        if (!res.ok) throw new Error(`add torrent: ${res.status} ${await res.text()}`);
      };
      await use({ url, downloads, addTorrent, logs });
    } finally {
      if (proc) {
        proc.kill("SIGTERM");
        await exited(proc);
      }
      await search.close();
      await rm(dir, { recursive: true, force: true });
    }
  },

  page: async ({ page, app }, use) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    page.on("console", (m) => {
      // Chrome logs one of these for every 4xx/5xx; tests provoke some on purpose
      if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) errors.push(m.text());
    });
    await page.goto(app.url);
    await expect(page.getByRole("img", { name: "Connected" })).toBeVisible();
    await use(page);
    expect(errors, "browser console errors").toEqual([]);
  },
});

export { expect };
