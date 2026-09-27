import { test } from "node:test";
import assert from "node:assert/strict";
import { copyText } from "./clipboard.js";

function fakeDoc(exec = () => true) {
  const calls = [];
  const button = { focused: 0, focus() { this.focused++; } };
  const doc = {
    activeElement: button,
    body: {
      children: [],
      appendChild(el) { this.children.push(el); },
      removeChild(el) { this.children = this.children.filter((c) => c !== el); },
    },
    createElement(tag) {
      return {
        tag, value: "", style: {}, attrs: {}, selected: false,
        setAttribute(k, v) { this.attrs[k] = v; },
        focus() { doc.activeElement = this; },
        select() { this.selected = true; },
      };
    },
    execCommand(cmd) {
      const area = doc.body.children[0];
      calls.push({ cmd, tag: area && area.tag, value: area && area.value, selected: area && area.selected });
      return exec();
    },
  };
  return { doc, calls, button };
}

function fakeClipboard(fail = false) {
  const written = [];
  return {
    written,
    async writeText(text) {
      if (fail) throw new Error("NotAllowedError");
      written.push(text);
    },
  };
}

test("a secure page uses the async clipboard API", async () => {
  const clipboard = fakeClipboard();
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("https://h/download/a", { secure: true, clipboard, document: doc }), "copied");
  assert.deepEqual(clipboard.written, ["https://h/download/a"]);
  assert.equal(calls.length, 0);
});

test("a plain-HTTP page copies through a hidden textarea", async () => {
  const clipboard = fakeClipboard();
  const { doc, calls, button } = fakeDoc();
  assert.equal(await copyText("a\nb", { secure: false, clipboard, document: doc }), "copied");
  assert.deepEqual(clipboard.written, []);
  assert.deepEqual(calls, [{ cmd: "copy", tag: "textarea", value: "a\nb", selected: true }]);
  assert.equal(doc.body.children.length, 0, "textarea removed");
  assert.equal(button.focused, 1, "focus returns to the button");
});

test("a rejected clipboard write falls back to the textarea", async () => {
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("x", { secure: true, clipboard: fakeClipboard(true), document: doc }), "copied");
  assert.equal(calls.length, 1);
});

test("a missing clipboard API falls back to the textarea", async () => {
  const { doc, calls } = fakeDoc();
  assert.equal(await copyText("x", { secure: true, clipboard: undefined, document: doc }), "copied");
  assert.equal(calls.length, 1);
});

test("a refused execCommand asks for a manual copy", async () => {
  const { doc } = fakeDoc(() => false);
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: doc }), "manual");
  assert.equal(doc.body.children.length, 0);
});

test("a throwing execCommand asks for a manual copy", async () => {
  const { doc } = fakeDoc(() => { throw new Error("SecurityError"); });
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: doc }), "manual");
  assert.equal(doc.body.children.length, 0);
});

test("no document at all asks for a manual copy", async () => {
  assert.equal(await copyText("x", { secure: false, clipboard: undefined, document: undefined }), "manual");
});
