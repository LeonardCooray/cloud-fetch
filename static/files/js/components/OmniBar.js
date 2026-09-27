import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { classify, droppedText } from "../lib/omni.js";
import { parseMagnet, buildMagnet } from "../lib/magnet.js";
import { providerList, pickProvider, normalizeResults, resolveItem, resolveLookup } from "../lib/search.js";
import { MagnetEditor } from "./MagnetEditor.js";
import { SearchResults } from "./SearchResults.js";

const store = {
  get(k) {
    try { return localStorage.getItem(k) || ""; } catch { return ""; }
  },
  set(k, v) {
    try { localStorage.setItem(k, v); } catch { /* storage unavailable (private mode) */ }
  },
};
const BLANK = { name: "", infohash: "", trackers: [], extra: [] };

export function OmniBar({ api, providers, editorOpen, setEditorOpen, onError }) {
  const [text, setText] = useState(() => store.get("tcOmni"));
  const [stored, setStored] = useState(() => store.get("tcProvider"));
  const [results, setResults] = useState([]);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [searching, setSearching] = useState(false);
  const [inputError, setInputError] = useState(null);
  const [dragging, setDragging] = useState(false);
  const fileInput = useRef(null);
  const latestSearch = useRef("");
  const onDrop = useRef(null);

  const mode = classify(text);
  const query = text.trim();
  const list = providerList(providers);
  const provider = pickProvider(list, stored);
  const magnet = mode === "magnet" ? parseMagnet(text) : null;
  const noResults = !hasMore && results.length === 0;
  const searchKey = provider + "\n" + query;
  latestSearch.current = searchKey;

  useEffect(() => store.set("tcOmni", text), [text]);
  useEffect(() => {
    setResults([]);
    setPage(1);
    setHasMore(true);
  }, [query, provider]);
  // only the person's own edits clear the inline error; a provider list
  // arriving from the server must not wipe it
  useEffect(() => {
    setInputError(null);
    if (mode === "empty") setEditorOpen(false);
  }, [query]);

  const search = async () => {
    if (!provider || searching || !hasMore) return;
    const key = searchKey;
    setSearching(true);
    try {
      const res = await api.search(provider, query, page);
      if (latestSearch.current !== key) return; // a new query or provider meanwhile
      const found = Array.isArray(res) ? res : res ? [res] : [];
      if (found.length === 0) {
        setHasMore(false);
        return;
      }
      setResults((r) => r.concat(normalizeResults(found, providers[provider] && providers[provider].url)));
      setPage((p) => p + 1);
    } catch (e) {
      onError(e);
    } finally {
      setSearching(false);
    }
  };

  const submit = () => {
    setInputError(null);
    if (mode === "torrent-url") api.url(query).catch(onError);
    else if (mode === "magnet" && !magnet.error) api.magnet(query).catch(onError);
    else if (mode === "search") search();
  };

  const addResult = async (r) => {
    try {
      const how = resolveItem(r);
      if (how.error) throw new Error(how.error);
      if (how.kind === "magnet") return await api.magnet(how.value);
      if (how.kind === "url") return await api.url(how.value);
      const got = resolveLookup(await api.searchItem(provider, how.path), r.name);
      if (got.error) throw new Error(got.error);
      await (got.kind === "url" ? api.url(got.value) : api.magnet(got.value));
    } catch (e) {
      onError(e);
    }
  };

  const upload = async (files) => {
    const torrents = Array.from(files || []).filter((f) => f.name.toLowerCase().endsWith(".torrent"));
    if (torrents.length === 0) {
      setInputError("Only .torrent files can be uploaded");
      return;
    }
    setInputError(null);
    for (const f of torrents) {
      try {
        await api.torrentFile(new Uint8Array(await f.arrayBuffer()));
      } catch (e) {
        onError(e);
      }
    }
  };

  // drops land anywhere on the page: .torrent files are uploaded, a dragged
  // link or text goes into the bar
  onDrop.current = (dt) => {
    const files = Array.from((dt && dt.files) || []);
    if (files.some((f) => f.name.toLowerCase().endsWith(".torrent"))) return upload(files);
    const dropped = droppedText(dt);
    if (dropped) return setText(dropped);
    if (files.length) upload(files);
  };
  useEffect(() => {
    let depth = 0; // dragenter/leave fire for every element crossed
    const enter = (e) => { e.preventDefault(); depth++; setDragging(true); };
    const over = (e) => e.preventDefault();
    const leave = () => { depth = Math.max(0, depth - 1); if (depth === 0) setDragging(false); };
    const drop = (e) => {
      depth = 0;
      setDragging(false);
      // text dropped on another field (a tracker, a setting) goes into that field
      const field = e.target.closest && e.target.closest("input, textarea");
      const files = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length;
      if (field && !field.classList.contains("omni-input") && !files) return;
      e.preventDefault();
      onDrop.current(e.dataTransfer);
    };
    const on = [["dragenter", enter], ["dragover", over], ["dragleave", leave], ["drop", drop]];
    for (const [t, fn] of on) document.addEventListener(t, fn);
    return () => { for (const [t, fn] of on) document.removeEventListener(t, fn); };
  }, []);

  const icon = mode === "search" ? "search" : mode === "empty" ? "upload" : "magnet";
  const inlineError = inputError || (magnet && magnet.error);

  return html`<section class="omni">
    ${editorOpen && html`<${MagnetEditor} magnet=${magnet || BLANK} onChange=${(m) => setText(buildMagnet(m))} />`}
    <div class=${"omni-bar" + (dragging ? " drag" : "")}>
      <input class="omni-input" type="text" aria-label="Search, magnet link or torrent URL"
        placeholder="Enter search query, magnet URI, torrent URL or drop a torrent file here"
        value=${text} onInput=${(e) => setText(e.currentTarget.value)}
        onKeyDown=${(e) => { if (e.key === "Enter") submit(); }} />
      <button type="button" class="icon-btn" aria-label="Upload .torrent files" onClick=${() => fileInput.current.click()}>
        <${Icon} name=${icon} />
      </button>
      <input ref=${fileInput} type="file" accept=".torrent" multiple hidden
        onChange=${(e) => { upload(e.currentTarget.files); e.currentTarget.value = ""; }} />
    </div>
    ${inlineError && html`<p class="field-error" role="alert">${inlineError}</p>`}
    ${mode === "torrent-url" && html`<div class="actions">
      <button type="button" class="primary" onClick=${submit}>Load torrent</button>
    </div>`}
    ${mode === "magnet" && html`<div class="actions">
      <button type="button" class="primary" onClick=${submit}>Load magnet</button>
      <button type="button" class=${editorOpen ? "on" : ""} aria-pressed=${editorOpen} onClick=${() => setEditorOpen(!editorOpen)}>Edit</button>
    </div>`}
    ${mode === "search" && list.length === 0 && html`<p class="field-error">You have no search providers</p>`}
    ${mode === "search" && list.length > 0 && html`<div class="actions">
      <select aria-label="Search provider" value=${provider}
        onChange=${(e) => { setStored(e.currentTarget.value); store.set("tcProvider", e.currentTarget.value); }}>
        ${list.map((p) => html`<option key=${p.id} value=${p.id}>${p.name}</option>`)}
      </select>
      <button type="button" class="primary" onClick=${search} disabled=${searching || results.length > 0 || noResults}>
        ${noResults ? "No results" : searching && results.length === 0 ? "Searching…" : "Search"}
      </button>
    </div>`}
    ${mode === "search" && results.length > 0 && html`<${SearchResults} results=${results} hasMore=${hasMore}
      searching=${searching} onMore=${search} onAdd=${addResult} />`}
  </section>`;
}
