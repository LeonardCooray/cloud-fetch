import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes, systemUsage } from "../lib/format.js";

function StatsPill({ rate, system, connected }) {
  const usage = systemUsage(system);
  const tip = usage ? `CPU ${usage.cpu}` + (usage.memory ? ` · memory ${usage.memory} (${usage.memoryDetail})` : "") : null;
  return html`<div class="pill" title=${tip}>
    <span class="pill-rate"><${Icon} name="download" />${bytes(rate)}/s</span>
    ${system && system.set && html`<span class="pill-free">${bytes(system.diskFree)} free</span>`}
    <span class=${"dot" + (connected ? " ok" : "")} role="img" aria-label=${connected ? "Connected" : "Disconnected"}></span>
  </div>`;
}

export function Header({ title, busy, connected, rate, system, configOpen, editorOpen, onToggleConfig, onToggleEditor }) {
  return html`<header class="header">
    <a class="brand" href="https://github.com/LeonardCooray/cloud-fetch" target="_blank" rel="noopener">
      <span class="logo"><${Icon} name="cloud" /></span><span class="title" title=${title || "Cloud Fetch"}>${title || "Cloud Fetch"}</span>
    </a>
    <div class="status">
      ${busy && html`<${Icon} name="loader" class="spin" label="Working" />`}
      <${StatsPill} rate=${rate} system=${system} connected=${connected} />
      <button type="button" class=${"icon-btn" + (editorOpen ? " on" : "")} aria-pressed=${editorOpen}
        aria-label="Magnet editor" title="Magnet editor" onClick=${onToggleEditor}><${Icon} name="magnet" /></button>
      <button type="button" class=${"icon-btn" + (configOpen ? " on" : "")} aria-pressed=${configOpen}
        aria-label="Settings" title="Settings" onClick=${onToggleConfig}><${Icon} name="settings" /></button>
    </div>
  </header>`;
}
