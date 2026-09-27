import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes, filename } from "../lib/format.js";

export function FileTable({ files, size }) {
  const list = (files || []).filter(Boolean).slice().sort((a, b) => a.Path.localeCompare(b.Path));
  return html`<table class="files">
    <thead><tr><th>File</th><th class="size">Size</th></tr></thead>
    <tbody>
      ${list.length === 0 && html`<tr><td colspan="2" class="muted">No files</td></tr>`}
      ${list.map((f) => html`<tr key=${f.Path}>
        <td class="name">
          <span>${filename(f.Path)}</span>
          ${f.Percent > 0 && f.Percent < 100 && html` <span class="pct">${f.Percent}%</span>
            <div class="progress thin"><div class="bar" style=${{ width: f.Percent + "%" }}></div></div>`}
        </td>
        <td class="size">${bytes(f.Size)} ${f.Percent === 100 && html`<${Icon} name="check" class="ok" label="Complete" />`}</td>
      </tr>`)}
    </tbody>
    ${list.length > 1 && html`<tfoot><tr><th>${list.length} files</th><th class="size">${bytes(size)} total</th></tr></tfoot>`}
  </table>`;
}
