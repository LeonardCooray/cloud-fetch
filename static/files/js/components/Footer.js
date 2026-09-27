import { html } from "../html.js";

export function Footer({ stats = {}, users }) {
  const n = users ? Object.keys(users).length : 0;
  return html`<footer class="footer">
    <a href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">github.com/LeonardCooray/cloud-fetch</a>
    ${stats.Version ? ` version ${stats.Version}` : ""}
    ${" (fork of "}<a href="https://github.com/jpillora/cloud-torrent" target="_blank" rel="noopener">jpillora/cloud-torrent</a>${", AGPL-3.0)"}
    ${n > 1 ? ` · ${n} users connected` : ""}
    ${stats.Runtime ? html` · <a href="https://go.dev" target="_blank" rel="noopener">Go</a> ${stats.Runtime}` : ""}
  </footer>`;
}
