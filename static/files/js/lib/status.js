import { bytes } from "./format.js";

export const STATUS_LABELS = {
  loading: "Loading",
  downloading: "Downloading",
  seeding: "Seeding",
  done: "Done",
  paused: "Paused",
};

// torrentStatus derives a torrent's state from the fields the server already
// sends. A complete torrent that is still started is seeding only when the
// engine uploads (EnableUpload and EnableSeeding); otherwise it's done.
// selection sums the files ticked for download. Percent counts unticked
// files too, so a torrent whose selected files are all complete is finished
// even though Percent never reaches 100. Null when nothing is selected.
function selection(t) {
  const picked = (t.Files || []).filter((f) => f && f.Started);
  if (picked.length === 0) return null;
  let size = 0;
  let left = 0;
  for (const f of picked) {
    size += f.Size || 0;
    left += (f.Size || 0) * (1 - Math.min(f.Percent || 0, 100) / 100);
  }
  return { size, left, complete: picked.every((f) => (f.Percent || 0) >= 100) };
}

export function torrentStatus(t, { seeding = true } = {}) {
  if (!t || !t.Loaded) return "loading";
  const picked = selection(t);
  const complete = picked ? picked.complete : (t.Percent || 0) >= 100;
  if (t.Started) return complete ? (seeding ? "seeding" : "done") : "downloading";
  return complete ? "done" : "paused";
}

export function eta(t) {
  if (torrentStatus(t) !== "downloading") return null;
  const rate = t.DownloadRate || 0;
  const picked = selection(t);
  const left = picked ? picked.left : (t.Size || 0) - (t.Downloaded || 0);
  if (!(rate > 0) || !(left > 0)) return null;
  const seconds = left / rate;
  if (seconds < 60) return "less than a minute left";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `about ${minutes} min left`;
  if (minutes >= 1440) return "more than a day left";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h >= 10 || m === 0) return `about ${h} h left`;
  return `about ${h} h ${m} min left`;
}

// statusLine splits the card's status text so the rate can be emphasised.
// Percent is floored so an unfinished torrent never reads 100%.
export function statusLine(t, options) {
  const status = torrentStatus(t, options);
  if (status === "loading") return { main: "", rate: null, note: null };
  if (status === "seeding" || status === "done") {
    const picked = selection(t);
    return { main: `${bytes(picked ? picked.size : t.Size)} · complete`, rate: null, note: null };
  }
  const main = `${bytes(t.Downloaded)} of ${bytes(t.Size)} · ${Math.floor(t.Percent || 0)}%`;
  if (status === "paused") return { main, rate: null, note: null };
  const rate = t.DownloadRate || 0;
  return { main, rate: `${bytes(rate)}/s`, note: rate > 0 ? eta(t) : "waiting for peers" };
}
