import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { copyText } from "../lib/clipboard.js";
import { CopyFallback } from "./CopyFallback.js";

// CopyButton reads getText() on click, so it copies the tree as it is then,
// and shows the confirmation in place of the icon for 2 seconds.
export function CopyButton({ getText, label, done, title }) {
  const [state, setState] = useState("idle"); // idle | copied | manual
  const [text, setText] = useState("");
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async () => {
    const value = getText();
    setText(value);
    clearTimeout(timer.current);
    if ((await copyText(value)) === "copied") {
      setState("copied");
      timer.current = setTimeout(() => setState("idle"), 2000);
    } else {
      setState("manual");
    }
  };
  const doneText = typeof done === "function" ? done(text) : done;
  const copied = state === "copied";
  return html`<span class="copy">
    <button type="button" class=${"icon-btn" + (copied ? " copied" : "")} aria-label=${copied ? doneText : label} onClick=${copy}>
      <${Icon} name=${copied ? "check" : "copy"} />${copied && html`<span class="copied-text" aria-hidden="true">${doneText}</span>`}
    </button>
    <span class="sr-only" aria-live="polite">${copied ? doneText : ""}</span>
    ${state === "manual" && html`<${CopyFallback} text=${text} title=${title} onClose=${() => setState("idle")} />`}
  </span>`;
}
