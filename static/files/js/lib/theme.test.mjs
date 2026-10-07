import { test } from "node:test";
import assert from "node:assert/strict";
import { THEMES, nextTheme, readTheme, saveTheme, applyTheme } from "./theme.js";

const memory = (init = {}) => {
  const data = { ...init };
  return {
    data,
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => { data[k] = String(v); },
    removeItem: (k) => { delete data[k]; },
  };
};
const broken = {
  getItem() { throw new Error("blocked"); },
  setItem() { throw new Error("blocked"); },
  removeItem() { throw new Error("blocked"); },
};

test("nextTheme cycles auto, light, dark and back", () => {
  assert.deepEqual(THEMES, ["auto", "light", "dark"]);
  assert.equal(nextTheme("auto"), "light");
  assert.equal(nextTheme("light"), "dark");
  assert.equal(nextTheme("dark"), "auto");
  assert.equal(nextTheme("nonsense"), "light");
});

test("readTheme falls back to auto for missing, unknown or unreadable values", () => {
  assert.equal(readTheme(memory()), "auto");
  assert.equal(readTheme(memory({ tcTheme: "dark" })), "dark");
  assert.equal(readTheme(memory({ tcTheme: "light" })), "light");
  assert.equal(readTheme(memory({ tcTheme: "purple" })), "auto");
  assert.equal(readTheme(broken), "auto");
  assert.equal(readTheme(null), "auto");
});

test("saveTheme stores a forced theme and forgets auto", () => {
  const s = memory();
  saveTheme(s, "dark");
  assert.equal(s.data.tcTheme, "dark");
  saveTheme(s, "auto");
  assert.equal("tcTheme" in s.data, false);
  assert.doesNotThrow(() => saveTheme(broken, "light"));
});

test("applyTheme sets data-theme for forced themes and clears it for auto", () => {
  const root = { dataset: {} };
  applyTheme(root, "dark");
  assert.equal(root.dataset.theme, "dark");
  applyTheme(root, "light");
  assert.equal(root.dataset.theme, "light");
  applyTheme(root, "auto");
  assert.equal("theme" in root.dataset, false);
});
