import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { ago, bytes } from "../lib/format.js";
import {
  childPath, downloadHref, fileIcon, findTorrentFile, isDir, isDownloading, previewKind, startsClosed,
} from "../lib/tree.js";

// Players take focus when they open, so Space controls them rather than
// pressing the preview toggle that was just clicked.
function Preview({ kind, src }) {
  const player = useRef(null);
  useEffect(() => { if (player.current) player.current.focus(); }, []);
  if (kind === "audio") return html`<audio ref=${player} class="preview" controls src=${src}></audio>`;
  if (kind === "image") return html`<img class="preview" src=${src} alt="" />`;
  return html`<video ref=${player} class="preview" controls autoplay playsinline src=${src}></video>`;
}

function TreeNode({ node, path, torrents, api, onError }) {
  const dir = isDir(node);
  const [open, setOpen] = useState(() => !startsClosed(node));
  const [confirm, setConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [preview, setPreview] = useState(false);
  const armedAt = useRef(0);
  const settle = useRef(0);
  useEffect(() => () => clearTimeout(settle.current), []);
  const kind = dir ? null : previewKind(path);
  const downloading = !dir && isDownloading(findTorrentFile(torrents, path));
  const href = downloadHref(path);

  useEffect(() => {
    if (!confirm) return undefined;
    const id = setTimeout(() => setConfirm(false), 3000);
    return () => clearTimeout(id);
  }, [confirm]);

  const arm = () => {
    armedAt.current = Date.now();
    setConfirm(true);
  };
  const remove = async () => {
    // the confirm button replaces the trash button in place, so the second
    // click of a double-click would land on it; require a deliberate click
    if (Date.now() - armedAt.current < 400) return;
    setDeleting(true);
    try {
      await api.deleteDownload(path);
      // the next listing normally removes this row; if the file is back by
      // then (a torrent still writing it), show it as a file again
      settle.current = setTimeout(() => { setDeleting(false); setConfirm(false); }, 3000);
    } catch (e) {
      setDeleting(false);
      setConfirm(false);
      onError(e);
    }
  };

  return html`<li class="node">
    <div class="row">
      ${dir
        ? html`<button type="button" class="icon-btn" aria-expanded=${open}
            aria-label=${(open ? "Collapse " : "Expand ") + node.Name} onClick=${() => setOpen(!open)}>
            <${Icon} name=${open ? "folderOpen" : "folder"} />
          </button>`
        : html`<${Icon} name=${downloading ? "loader" : fileIcon(path)} class=${downloading ? "spin" : ""} />`}
      ${downloading ? html`<span class="label">${node.Name}</span>` : html`<a class="label" href=${href}>${node.Name}</a>`}
      ${!downloading && html`<span class="controls">
        ${kind && html`<button type="button" class=${"icon-btn" + (preview ? " on" : "")} aria-pressed=${preview}
          aria-label=${(preview ? "Hide preview of " : "Preview ") + node.Name} onClick=${() => setPreview(!preview)}>
          <${Icon} name=${preview ? "x" : "play"} />
        </button>`}
        ${deleting
          ? html`<${Icon} name="loader" class="spin" label="Deleting" />`
          : confirm
            ? html`<button type="button" class="icon-btn danger" aria-label=${"Confirm delete " + node.Name} onClick=${remove}><${Icon} name="check" /></button>`
            : html`<button type="button" class="icon-btn danger" aria-label=${"Delete " + node.Name} onClick=${arm}><${Icon} name="trash" /></button>`}
      </span>`}
    </div>
    <div class="meta">${bytes(node.Size)} · updated ${ago(node.Modified)}</div>
    ${preview && kind && html`<${Preview} kind=${kind} src=${href} />`}
    ${dir && open && node.Children.length > 0 && html`<ul class="tree">
      ${node.Children.map((c) => html`<${TreeNode} key=${c.Name} node=${c} path=${childPath(path, c.Name)}
        torrents=${torrents} api=${api} onError=${onError} />`)}
    </ul>`}
  </li>`;
}

export function DownloadTree({ root, torrents, system, api, onError }) {
  const children = (root && root.Children) || [];
  const free = system && system.set ? `${bytes(system.diskTotal - system.diskUsed)} free` : "";
  return html`<section class="downloads">
    <div class="section-header"><h3>Downloads</h3><span class="muted">${free}</span></div>
    ${children.length === 0
      ? html`<p class="empty">Download files above</p>`
      : html`<ul class="tree">
          ${children.map((n) => html`<${TreeNode} key=${n.Name} node=${n} path=${n.Name} torrents=${torrents} api=${api} onError=${onError} />`)}
        </ul>`}
  </section>`;
}
