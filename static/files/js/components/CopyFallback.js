import { useEffect, useRef } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";

// CopyFallback shows text pre-selected for copying by hand, for browsers
// that refuse both clipboard routes. Esc or a click outside closes it.
export function CopyFallback({ text, title, onClose }) {
  const box = useRef(null);
  const area = useRef(null);
  useEffect(() => {
    area.current.focus();
    area.current.select();
    const onKey = (e) => { if (e.key === "Escape") onClose(); };
    const onDown = (e) => { if (box.current && !box.current.contains(e.target)) onClose(); };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onDown);
    };
  }, []);
  const rows = Math.min(text.split("\n").length, 6);
  return html`<div ref=${box} class="copy-fallback" role="dialog" aria-label=${title}>
    <textarea ref=${area} readonly rows=${rows} aria-label=${title} value=${text}></textarea>
    <div class="copy-fallback-foot">
      <span class="muted">Press Ctrl+C or long-press to copy</span>
      <button type="button" class="icon-btn" aria-label="Close" onClick=${onClose}><${Icon} name="x" /></button>
    </div>
  </div>`;
}
