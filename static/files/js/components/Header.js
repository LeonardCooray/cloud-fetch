import { html } from "../html.js";
import { Icon } from "../icons.js";

export function Header({ title, busy, connected, configOpen, editorOpen, onToggleConfig, onToggleEditor }) {
  return html`<header class="header">
    <a class="brand" href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">
      <${Icon} name="cloud" /><span class="title" title=${title || "Cloud Fetch"}>${title || "Cloud Fetch"}</span>
    </a>
    <div class="status">
      ${busy && html`<${Icon} name="loader" class="spin" label="Working" />`}
      <button type="button" class=${"icon-btn" + (editorOpen ? " on" : "")} aria-pressed=${editorOpen}
        aria-label="Magnet editor" onClick=${onToggleEditor}><${Icon} name="magnet" /></button>
      <button type="button" class=${"icon-btn" + (configOpen ? " on" : "")} aria-pressed=${configOpen}
        aria-label="Settings" onClick=${onToggleConfig}><${Icon} name="settings" /></button>
      <span class=${"dot" + (connected ? " ok" : "")} role="img" aria-label=${connected ? "Connected" : "Disconnected"}></span>
    </div>
  </header>`;
}
