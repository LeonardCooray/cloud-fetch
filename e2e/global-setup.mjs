import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

export const ROOT = fileURLToPath(new URL("..", import.meta.url));
export const BINARY = fileURLToPath(new URL("./.bin/cloud-fetch", import.meta.url));

// The UI is embedded in the binary, so rebuild on every run to test the
// current files.
export default function globalSetup() {
  execFileSync("go", ["build", "-o", BINARY, "."], { cwd: ROOT, stdio: "inherit" });
}
