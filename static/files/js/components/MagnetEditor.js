import { html } from "../html.js";

export function MagnetEditor({ magnet, onChange }) {
  const trackers = [...magnet.trackers, ""];
  const set = (patch) => onChange({ name: magnet.name, infohash: magnet.infohash, trackers: magnet.trackers, ...patch });
  const setTracker = (i, value) => {
    const next = trackers.slice();
    next[i] = value;
    set({ trackers: next.filter(Boolean) });
  };
  return html`<form class="card editor" onSubmit=${(e) => e.preventDefault()}>
    <h4>Magnet URI editor</h4>
    <label>Name<input type="text" value=${magnet.name} placeholder="Name" onInput=${(e) => set({ name: e.currentTarget.value })} /></label>
    <label>Info hash<input type="text" value=${magnet.infohash} placeholder="Info hash" onInput=${(e) => set({ infohash: e.currentTarget.value.trim() })} /></label>
    <fieldset>
      <legend>Trackers</legend>
      ${trackers.map((t, i) => html`<input key=${i} type="text" aria-label=${"Tracker " + (i + 1)} value=${t}
        placeholder="Tracker" onInput=${(e) => setTracker(i, e.currentTarget.value)} />`)}
    </fieldset>
  </form>`;
}
