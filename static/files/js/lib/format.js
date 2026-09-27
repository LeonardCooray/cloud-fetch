const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

export function bytes(n) {
  if (typeof n !== "number" || !Number.isFinite(n) || n <= 0) return "0 B";
  const i = Math.min(Math.floor(Math.log10(n) / 3), UNITS.length - 1);
  const scaled = Math.round((n / 10 ** (i * 3)) * 10) / 10;
  return `${scaled} ${UNITS[i]}`;
}

// systemUsage turns Stats.System into footer text; null until the server
// has gathered its first sample (set is false before then).
export function systemUsage(system) {
  if (!system || !system.set) return null;
  const known = system.memoryTotal > 0;
  return {
    cpu: `${Math.round(system.cpu || 0)}%`,
    memory: known ? `${Math.round((100 * system.memoryUsed) / system.memoryTotal)}%` : "",
    memoryDetail: known ? `${bytes(system.memoryUsed)} of ${bytes(system.memoryTotal)}` : "",
  };
}

export function hoursSince(t, now = Date.now()) {
  return (now - new Date(t).getTime()) / 3600000;
}

export function ago(t, now = Date.now()) {
  const ms = now - new Date(t).getTime();
  if (!Number.isFinite(ms)) return "";
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 45) return m <= 1 ? "a minute ago" : `${m} minutes ago`;
  const h = Math.round(m / 60);
  if (h < 22) return h <= 1 ? "an hour ago" : `${h} hours ago`;
  const d = Math.round(h / 24);
  if (d < 26) return d <= 1 ? "a day ago" : `${d} days ago`;
  const mo = Math.round(d / 30);
  if (mo < 11) return mo <= 1 ? "a month ago" : `${mo} months ago`;
  const y = Math.round(d / 365);
  return y <= 1 ? "a year ago" : `${y} years ago`;
}

export function filename(path) {
  const i = path.lastIndexOf("/");
  return i >= 0 ? path.slice(i + 1) : path;
}

export function addSpaces(key) {
  if (typeof key !== "string") return key;
  return key.replace(/([A-Z]+[a-z]*)/g, " $1").trim();
}

export function inputType(value) {
  if (typeof value === "boolean") return "checkbox";
  if (typeof value === "number") return "number";
  return "text";
}
