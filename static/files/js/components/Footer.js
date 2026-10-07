import { html } from "../html.js";

export function Footer({ stats = {}, users }) {
  const n = users ? Object.keys(users).length : 0;
  return html`<footer class="footer">
    <a href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">Cloud Fetch</a>
    ${stats.Version ? ` ${stats.Version}` : ""}
    ${stats.Runtime ? html` · <a href="https://go.dev" target="_blank" rel="noopener">Go</a> ${stats.Runtime}` : ""}
    ${n > 1 ? ` · ${n} users connected` : ""}
  </footer>`;
}
