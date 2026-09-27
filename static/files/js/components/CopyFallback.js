import { useEffect, useRef } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";

// CopyFallback shows text pre-selected for copying by hand, for browsers
// that refuse both clipboard routes. Esc and the close button hand focus
// back (onClose(true)); a click elsewhere closes it and leaves focus there.
export function CopyFallback({ text, title, onClose }) {
  const box = useRef(null);
  const area = useRef(null);
  useEffect(() => {
    area.current.focus();
    area.current.select();
    const onDown = (e) => { if (box.current && !box.current.contains(e.target)) onClose(false); };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, []);
  const rows = Math.min(text.split("\n").length, 6);
  // Esc is handled here rather than on document so it goes no further
  const onKeyDown = (e) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    onClose(true);
  };
  return html`<div ref=${box} class="copy-fallback" role="dialog" aria-label=${title} onKeyDown=${onKeyDown}>
    <textarea ref=${area} readonly rows=${rows} aria-label=${title} value=${text}></textarea>
    <div class="copy-fallback-foot">
      <span class="muted">Press Ctrl+C or long-press to copy</span>
      <button type="button" class="icon-btn" aria-label="Close" onClick=${() => onClose(true)}><${Icon} name="x" /></button>
    </div>
  </div>`;
}
