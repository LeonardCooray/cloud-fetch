import { test } from "node:test";
import assert from "node:assert/strict";
import { createSyncStore } from "./sync.js";

const fakeVelox = () => ({});

test("a subscriber that joins after an update is told straight away", () => {
  const store = createSyncStore();
  const v = store.attach(fakeVelox());
  const seenAtRender = store.version;
  v.onupdate(); // the first state lands before the component's effect runs
  const calls = [];
  store.subscribe((n) => calls.push(n), seenAtRender);
  assert.deepEqual(calls, [1]);
});

test("an up-to-date subscriber waits for the next update", () => {
  const store = createSyncStore();
  const v = store.attach(fakeVelox());
  v.onupdate();
  const calls = [];
  const off = store.subscribe((n) => calls.push(n), store.version);
  assert.deepEqual(calls, []);
  v.onupdate();
  assert.deepEqual(calls, [2]);
  off();
  v.onupdate();
  assert.deepEqual(calls, [2]);
});

test("connection changes are tracked, and everConnected sticks", () => {
  const store = createSyncStore();
  const v = store.attach(fakeVelox());
  assert.deepEqual(store.snapshot(), { connected: false, everConnected: false });
  v.onchange(true);
  assert.deepEqual(store.snapshot(), { connected: true, everConnected: true });
  v.onchange(false);
  assert.deepEqual(store.snapshot(), { connected: false, everConnected: true });
  assert.equal(store.version, 2);
});
