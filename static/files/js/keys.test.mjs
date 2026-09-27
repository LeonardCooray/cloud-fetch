import { test } from "node:test";
import assert from "node:assert/strict";
import { installSpaceToggle } from "./keys.js";

function fakePage(media) {
  const listeners = {};
  const doc = {
    activeElement: { tagName: "BODY" },
    addEventListener: (type, fn) => { listeners[type] = fn; },
    querySelectorAll: () => [media],
  };
  const win = { innerHeight: 800 };
  installSpaceToggle(doc, win);
  return (e) => listeners.keydown({ key: " ", repeat: false, preventDefault() {}, ...e });
}

test("Space swallows a refused play() instead of leaving it unhandled", async () => {
  let caught = false;
  const refused = { catch(fn) { caught = true; fn(new Error("NotAllowedError")); return this; } };
  const media = { paused: true, play: () => refused, pause() {}, getBoundingClientRect: () => ({ top: 10, bottom: 100 }) };
  fakePage(media)({});
  assert.equal(caught, true);
});

test("Space pauses a playing player", () => {
  let paused = false;
  const media = { paused: false, play() {}, pause() { paused = true; }, getBoundingClientRect: () => ({ top: 10, bottom: 100 }) };
  fakePage(media)({});
  assert.equal(paused, true);
});
