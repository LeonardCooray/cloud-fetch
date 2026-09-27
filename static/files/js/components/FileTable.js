import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes, filename } from "../lib/format.js";

export function FileTable({ infohash, files, size, onSelect }) {
  const list = (files || []).filter(Boolean).slice().sort((a, b) => a.Path.localeCompare(b.Path));
  const toggle = (f, e) => {
    const box = e.currentTarget;
    // the box stays as clicked until the server's next push confirms it
    onSelect(f.Path, box.checked).catch(() => { box.checked = f.Started; });
  };
  return html`<div class="files-panel">
    <p class="hash">#${infohash}</p>
    <table class="files">
      <thead><tr><th>File</th><th class="size">Size</th></tr></thead>
      <tbody>
        ${list.length === 0 && html`<tr><td colspan="2" class="muted">No files</td></tr>`}
        ${list.map((f) => html`<tr key=${f.Path} class=${f.Started ? "" : "skipped"}>
          <td class="name">
            <label>
              <input type="checkbox" checked=${f.Started} aria-label=${"Download " + filename(f.Path)}
                onChange=${(e) => toggle(f, e)} />
              <span>${filename(f.Path)}</span>
            </label>
            ${f.Percent > 0 && f.Percent < 100 && html` <span class="pct">${f.Percent}%</span>
              <div class="progress thin"><div class="bar" style=${{ width: f.Percent + "%" }}></div></div>`}
          </td>
          <td class="size">${bytes(f.Size)} ${f.Percent === 100 && html`<${Icon} name="check" class="ok" label="Complete" />`}</td>
        </tr>`)}
      </tbody>
      ${list.length > 1 && html`<tfoot><tr><th>${list.length} files</th><th class="size">${bytes(size)} total</th></tr></tfoot>`}
    </table>
  </div>`;
}
