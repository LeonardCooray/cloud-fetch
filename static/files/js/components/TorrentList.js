import { useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { bytes } from "../lib/format.js";
import { sortTorrents } from "../lib/tree.js";
import { FileTable } from "./FileTable.js";

function TorrentCard({ t, api, onError }) {
  const [showFiles, setShowFiles] = useState(false);
  const act = (action) => api.torrent(action, t.InfoHash).catch(onError);
  const pct = t.Percent || 0;
  return html`<article class="card torrent" aria-label=${t.Name || t.InfoHash}>
    ${!t.Loaded && html`<div class="overlay"><${Icon} name="loader" class="spin" /> Loading</div>`}
    <div class="torrent-top">
      <div class="info">
        <div class="name">${t.Name || t.InfoHash}</div>
        <div class="hash">#${t.InfoHash}</div>
      </div>
      <div class="buttons">
        <button type="button" class=${showFiles ? "on" : ""} aria-pressed=${showFiles} onClick=${() => setShowFiles(!showFiles)}>
          <${Icon} name="file" /> Files
        </button>
        <button type="button" class="go" disabled=${t.Started} onClick=${() => act("start")}><${Icon} name="play" /> Start</button>
        ${t.Started && html`<button type="button" class="danger" onClick=${() => act("stop")}><${Icon} name="stop" /> Stop</button>`}
        ${!t.Started && html`<button type="button" class="danger" onClick=${() => act("delete")}>
          <${Icon} name=${t.Loaded ? "trash" : "x"} /> ${t.Loaded ? "Remove" : "Cancel"}
        </button>`}
      </div>
    </div>
    <div class="progress" role="progressbar" aria-label="Progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow=${pct}>
      <div class="bar" style=${{ width: pct + "%" }}></div>
    </div>
    ${t.Started && html`<div class="status">
      ${bytes(t.Downloaded)} / ${bytes(t.Size)} · ${pct}% · <strong>${bytes(t.DownloadRate)}/s</strong>
    </div>`}
    ${showFiles && t.Loaded && html`<${FileTable} files=${t.Files} size=${t.Size}
      onSelect=${(path, on) => api.file(on ? "start" : "stop", t.InfoHash, path).catch((e) => { onError(e); throw e; })} />`}
  </article>`;
}

export function TorrentList({ torrents, api, onError }) {
  const list = sortTorrents(torrents);
  return html`<section class="torrents">
    <div class="section-header">
      <h3>Torrents</h3>
      <span class="muted">${list.length} torrent${list.length === 1 ? "" : "s"}</span>
    </div>
    ${list.length === 0
      ? html`<p class="empty">Add torrents above</p>`
      : list.map((t) => html`<${TorrentCard} key=${t.InfoHash} t=${t} api=${api} onError=${onError} />`)}
  </section>`;
}
