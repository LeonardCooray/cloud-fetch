import { useState } from "preact/hooks";
import { html } from "../html.js";
import { trackerSlots } from "../lib/magnet.js";

export function MagnetEditor({ magnet, onChange }) {
  const [slots, setSlots] = useState(magnet.trackers);
  const current = trackerSlots(slots, magnet.trackers);
  const fields = [...current, ""];
  const set = (patch) =>
    onChange({ name: magnet.name, infohash: magnet.infohash, trackers: magnet.trackers, extra: magnet.extra, ...patch });
  const setTracker = (i, value) => {
    const next = fields.slice();
    next[i] = value;
    // only trailing blanks go; a cleared field in the middle keeps its place
    while (next.length && !next[next.length - 1]) next.pop();
    setSlots(next);
    set({ trackers: next.filter(Boolean) });
  };
  return html`<form class="card editor" onSubmit=${(e) => e.preventDefault()}>
    <h4>Magnet URI editor</h4>
    <label>Name<input type="text" value=${magnet.name} placeholder="Name" onInput=${(e) => set({ name: e.currentTarget.value })} /></label>
    <label>Info hash<input type="text" value=${magnet.infohash} placeholder="Info hash" onInput=${(e) => set({ infohash: e.currentTarget.value.trim() })} /></label>
    <fieldset>
      <legend>Trackers</legend>
      ${fields.map((t, i) => html`<input key=${i} type="text" aria-label=${"Tracker " + (i + 1)} value=${t}
        placeholder="Tracker" onInput=${(e) => setTracker(i, e.currentTarget.value)} />`)}
    </fieldset>
  </form>`;
}
