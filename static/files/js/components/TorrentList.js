import { useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { sortTorrents } from "../lib/tree.js";
import { STATUS_LABELS, statusLine, torrentStatus } from "../lib/status.js";
import { FileTable } from "./FileTable.js";

function TorrentCard({ t, seeding, api, onError }) {
  const [showFiles, setShowFiles] = useState(false);
  const act = (action) => api.torrent(action, t.InfoHash).catch(onError);
  const status = torrentStatus(t, { seeding });
  const line = statusLine(t, { seeding });
  const pct = t.Percent || 0;
  return html`<article class=${"card torrent is-" + status} aria-label=${t.Name || t.InfoHash}>
    ${!t.Loaded && html`<div class="overlay"><${Icon} name="loader" class="spin" /> Loading</div>`}
    <div class="torrent-top">
      <div class="info">
        <div class="name">${t.Name || t.InfoHash} <span class=${"badge badge-" + status}>${STATUS_LABELS[status]}</span></div>
      </div>
      <div class="buttons">
        <button type="button" class=${showFiles ? "on" : ""} aria-pressed=${showFiles} title="Files"
          onClick=${() => setShowFiles(!showFiles)}><${Icon} name="file" /><span class="btn-label">Files</span></button>
        ${t.Started
          ? html`<button type="button" title="Pause" onClick=${() => act("stop")}><${Icon} name="pause" /><span class="btn-label">Pause</span></button>`
          : html`<button type="button" class="go" title="Resume" onClick=${() => act("start")}><${Icon} name="play" /><span class="btn-label">Resume</span></button>`}
        ${!t.Started && html`<button type="button" class="danger" title=${t.Loaded ? "Remove" : "Cancel"} onClick=${() => act("delete")}>
          <${Icon} name=${t.Loaded ? "trash" : "x"} /><span class="btn-label">${t.Loaded ? "Remove" : "Cancel"}</span>
        </button>`}
      </div>
    </div>
    <div class="progress" role="progressbar" aria-label="Progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow=${pct}>
      <div class="bar" style=${{ width: pct + "%" }}></div>
    </div>
    ${line.main && html`<div class="status">
      ${line.main}${line.rate && html` · <strong>${line.rate}</strong>`}${line.note && ` · ${line.note}`}
    </div>`}
    ${showFiles && t.Loaded && html`<${FileTable} infohash=${t.InfoHash} files=${t.Files} size=${t.Size}
      onSelect=${(path, on) => api.file(on ? "start" : "stop", t.InfoHash, path).catch((e) => { onError(e); throw e; })} />`}
  </article>`;
}

export function TorrentList({ torrents, seeding, api, onError }) {
  const list = sortTorrents(torrents);
  return html`<section class="torrents">
    <div class="section-header">
      <h3>Torrents</h3>
      <span class="count">${list.length || ""}</span>
    </div>
    ${list.length === 0
      ? html`<p class="empty">Add torrents above</p>`
      : list.map((t) => html`<${TorrentCard} key=${t.InfoHash} t=${t} seeding=${seeding} api=${api} onError=${onError} />`)}
  </section>`;
}
