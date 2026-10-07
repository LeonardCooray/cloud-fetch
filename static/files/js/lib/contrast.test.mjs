import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const css = readFileSync(new URL("../../css/app.css", import.meta.url), "utf8");

function block(from) {
  const start = css.indexOf(":root", from);
  assert.ok(start >= 0, "no :root block found");
  return css.slice(start, css.indexOf("}", start));
}
function tokens(text) {
  const out = {};
  for (const m of text.matchAll(/--([\w-]+):\s*(#[0-9a-fA-F]{6})\b/g)) out[m[1]] = m[2].toLowerCase();
  return out;
}
const light = tokens(block(0));
const darkAt = css.indexOf("@media (prefers-color-scheme: dark)");
const dark = darkAt >= 0 ? tokens(block(darkAt)) : {};

function luminance(hex) {
  const n = parseInt(hex.slice(1), 16);
  const [r, g, b] = [16, 8, 0].map((s) => {
    const c = ((n >> s) & 255) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
function ratio(a, b) {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

const TEXT = ["text", "muted", "faint", "accent", "ok", "danger", "warn"];
const PAIRS = [
  ...TEXT.flatMap((fg) => [[fg, "surface"], [fg, "bg"]]),
  ["accent", "accent-bg"], ["ok", "ok-bg"], ["danger", "danger-bg"], ["warn", "warn-bg"], ["muted", "bg"],
  ["on-accent", "accent"],
];

test("the dark theme defines every light colour token", () => {
  assert.ok(darkAt >= 0, "no prefers-color-scheme: dark block");
  assert.deepEqual(Object.keys(dark).sort(), Object.keys(light).sort());
});

for (const [name, theme] of [["light", light], ["dark", dark]]) {
  test(`${name} theme text pairs reach WCAG AA (4.5:1)`, () => {
    const failing = [];
    for (const [fg, bg] of PAIRS) {
      assert.ok(theme[fg] && theme[bg], `${name}: missing --${fg} or --${bg}`);
      const r = ratio(theme[fg], theme[bg]);
      if (r < 4.5) failing.push(`--${fg} on --${bg}: ${r.toFixed(2)}`);
    }
    assert.deepEqual(failing, []);
  });
}

// The dark palette is written twice: once for the system preference (unless
// light is forced) and once for a forced dark theme. They must not drift.
test("the forced dark theme repeats the system dark palette exactly", () => {
  const forcedAt = css.indexOf(':root[data-theme="dark"]');
  assert.ok(forcedAt >= 0, 'no :root[data-theme="dark"] block');
  const body = (at) => css.slice(css.indexOf("{", at) + 1, css.indexOf("}", at)).replace(/\s+/g, " ").trim();
  const systemAt = css.indexOf(":root", darkAt);
  assert.equal(body(forcedAt).replace(/color-scheme: dark; /, ""), body(systemAt));
});
