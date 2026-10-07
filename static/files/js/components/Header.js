import { useEffect, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes } from "../lib/format.js";
import { THEME_LABELS, applyTheme, nextTheme, readTheme, saveTheme } from "../lib/theme.js";

const THEME_ICONS = { auto: "themeAuto", light: "sun", dark: "moon" };
const storage = () => {
  try { return window.localStorage; } catch { return null; }
};

// The theme-color metas carry media queries for the system scheme, so a
// forced theme points both at the page's background instead.
function syncThemeColor(mode) {
  const root = document.documentElement;
  const bg = mode === "auto" ? null : getComputedStyle(root).backgroundColor;
  for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
    if (!meta.dataset.system) meta.dataset.system = meta.content;
    meta.content = bg || meta.dataset.system;
  }
}

function ThemeButton() {
  const [mode, setMode] = useState(() => readTheme(storage()));
  // theme-init.js already set data-theme before paint; only the metas lag
  useEffect(() => syncThemeColor(mode), []);
  const next = () => {
    const m = nextTheme(mode);
    applyTheme(document.documentElement, m);
    syncThemeColor(m);
    saveTheme(storage(), m);
    setMode(m);
  };
  const label = "Theme: " + THEME_LABELS[mode];
  return html`<button type="button" class="icon-btn" aria-label=${label} title=${label} onClick=${next}>
    <${Icon} name=${THEME_ICONS[mode]} />
  </button>`;
}

function StatsPill({ rate, system, connected }) {
  return html`<div class="pill">
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
      <${ThemeButton} />
    </div>
  </header>`;
}
