import { html } from "../html.js";
import { Icon } from "../icons.js";

export function SearchResults({ results, hasMore, searching, onMore, onAdd }) {
  return html`<table class="results"><tbody>
    ${results.map((r, i) => html`<tr key=${i}>
      <td class="name">${r.url ? html`<a href=${r.url} target="_blank" rel="noopener noreferrer">${r.name}</a>` : r.name}</td>
      <td class="size">${r.size || ""}</td>
      <td class="users"><span class="seeds">${r.seeds || ""}</span> <span class="peers">${r.peers || ""}</span></td>
      <td class="controls">
        <button type="button" class="icon-btn go" aria-label=${"Add " + (r.name || "result")} onClick=${() => onAdd(r)}>
          <${Icon} name="download" />
        </button>
      </td>
    </tr>`)}
    ${hasMore && html`<tr><td colspan="4" class="more">
      <button type="button" onClick=${onMore} disabled=${searching}>${searching ? "Loading…" : "Load more"}</button>
    </td></tr>`}
  </tbody></table>`;
}
