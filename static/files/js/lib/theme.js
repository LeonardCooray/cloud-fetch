// The theme choice is per browser. "auto" follows prefers-color-scheme and
// is stored as no key at all; js/theme-init.js applies a stored choice
// before first paint, so keep the key and values in step with it.
export const THEMES = ["auto", "light", "dark"];
const KEY = "tcTheme";

export const THEME_LABELS = { auto: "Auto (follows system)", light: "Light", dark: "Dark" };

export function nextTheme(mode) {
  const i = Math.max(0, THEMES.indexOf(mode)); // unknown counts as auto
  return THEMES[(i + 1) % THEMES.length];
}

export function readTheme(storage) {
  try {
    const v = storage && storage.getItem(KEY);
    return v === "light" || v === "dark" ? v : "auto";
  } catch {
    return "auto";
  }
}

export function saveTheme(storage, mode) {
  try {
    if (mode === "auto") storage.removeItem(KEY);
    else storage.setItem(KEY, mode);
  } catch {
    // storage unavailable (private mode): the choice lasts this page only
  }
}

export function applyTheme(root, mode) {
  if (mode === "light" || mode === "dark") root.dataset.theme = mode;
  else delete root.dataset.theme;
}
