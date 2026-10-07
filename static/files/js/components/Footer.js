import { html } from "../html.js";
import { systemUsage } from "../lib/format.js";

export function Footer({ stats = {}, users }) {
  const n = users ? Object.keys(users).length : 0;
  const usage = systemUsage(stats.System);
  return html`<footer class="footer">
    <a href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">Cloud Fetch</a>
    ${stats.Version ? ` ${stats.Version}` : ""}
    ${stats.Runtime ? html` · <a href="https://go.dev" target="_blank" rel="noopener">Go</a> ${stats.Runtime}` : ""}
    ${n > 1 ? ` · ${n} users connected` : ""}
    ${usage ? ` · CPU ${usage.cpu}` : ""}
    ${usage && usage.memory ? html` · <span title=${usage.memoryDetail}>memory ${usage.memory}</span>` : ""}
  </footer>`;
}
