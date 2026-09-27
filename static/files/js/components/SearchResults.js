import { html } from "../html.js";
import { Icon } from "../icons.js";

export function SearchResults({ results, hasMore, searching, onMore, onAdd }) {
  return html`<table class="results">
    <thead><tr>
      <th class="name">Name</th><th class="size">Size</th><th class="seeds">Seeds</th><th class="peers">Peers</th>
      <th class="controls"><span class="sr-only">Add</span></th>
    </tr></thead>
    <tbody>
      ${results.map((r, i) => html`<tr key=${i}>
        <td class="name">${r.url ? html`<a href=${r.url} target="_blank" rel="noopener noreferrer">${r.name}</a>` : r.name}</td>
        <td class="size">${r.size || ""}</td>
        <td class="seeds">${r.seeds && html`${r.seeds}<span class="unit"> seeds</span>`}</td>
        <td class="peers">${r.peers && html`${r.peers}<span class="unit"> peers</span>`}</td>
        <td class="controls">
          <button type="button" class="icon-btn go" aria-label=${"Add " + (r.name || "result")} onClick=${() => onAdd(r)}>
            <${Icon} name="download" />
          </button>
        </td>
      </tr>`)}
      ${hasMore && html`<tr class="more-row"><td colspan="5" class="more">
        <button type="button" onClick=${onMore} disabled=${searching}>${searching ? "Loading…" : "Load more"}</button>
      </td></tr>`}
    </tbody>
  </table>`;
}
