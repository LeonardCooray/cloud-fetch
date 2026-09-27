import { useEffect, useState } from "preact/hooks";
import { html } from "../html.js";
import { useSync } from "../state.js";
import { Header } from "./Header.js";
import { Footer } from "./Footer.js";
import { ErrorBanner } from "./ErrorBanner.js";
import { OmniBar } from "./OmniBar.js";
import { TorrentList } from "./TorrentList.js";
import { DownloadTree } from "./DownloadTree.js";
import { ConfigForm } from "./ConfigForm.js";

export function App({ api }) {
  const { state, connected, everConnected } = useSync();
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [configOpen, setConfigOpen] = useState(false);
  const [editorOpen, setEditorOpen] = useState(false);
  const report = (e) => setError((e && e.message) || String(e));
  useEffect(() => api.onBusy(setBusy), [api]);
  const stats = state.Stats || {};
  useEffect(() => {
    if (stats.Title) document.title = stats.Title;
  }, [stats.Title]);

  return html`
    <${Header} title=${stats.Title} busy=${busy} connected=${connected}
      configOpen=${configOpen} editorOpen=${editorOpen}
      onToggleConfig=${() => setConfigOpen(!configOpen)}
      onToggleEditor=${() => setEditorOpen(!editorOpen)} />
    ${everConnected && !connected && html`<div class="reconnecting" role="status">Reconnecting…</div>`}
    ${configOpen && state.Config && html`<${ConfigForm} config=${state.Config} api=${api} onError=${report} onClose=${() => setConfigOpen(false)} />`}
    <${OmniBar} api=${api} providers=${state.SearchProviders} editorOpen=${editorOpen} setEditorOpen=${setEditorOpen} onError=${report} />
    ${error && html`<${ErrorBanner} message=${error} onDismiss=${() => setError(null)} />`}
    <${TorrentList} torrents=${state.Torrents} api=${api} onError=${report} />
    <${DownloadTree} root=${state.Downloads} torrents=${state.Torrents} system=${stats.System} api=${api} onError=${report} />
    <${Footer} stats=${stats} users=${state.Users} />
  `;
}
