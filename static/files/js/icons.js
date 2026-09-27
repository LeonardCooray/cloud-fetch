import { html } from "./html.js";

const PATHS = {
  cloud: "M7 18h10a4 4 0 0 0 .6-7.96A6 6 0 0 0 6.1 9.1 4.5 4.5 0 0 0 7 18z",
  search: "M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0zM21 21l-6-6",
  magnet: "M5 13V9a7 7 0 0 1 14 0v4h-4V9a3 3 0 0 0-6 0v4zM5 13a7 7 0 0 0 14 0",
  settings: "M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9 7 7M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1",
  upload: "M12 16V4M7 9l5-5 5 5M4 20h16",
  download: "M12 4v12M7 11l5 5 5-5M4 20h16",
  file: "M14 3H6v18h12V7zM14 3v4h4",
  folder: "M3 6h6l2 2h10v11H3z",
  folderOpen: "M3 19V6h6l2 2h8v3M3 19l3-8h16l-3 8z",
  music: "M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  image: "M3 5h18v14H3zM3 16l5-5 5 5 3-3 5 5M15 9h.01",
  film: "M3 5h18v14H3zM7 5v14M17 5v14M3 9h4M3 15h4M17 9h4M17 15h4",
  play: "M7 4l13 8-13 8z",
  stop: "M6 6h12v12H6z",
  pause: "M8 5v14M16 5v14",
  copy: "M9 9h11v11H9zM5 15H4V4h11v1",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13",
  check: "M5 12l5 5 9-10",
  x: "M6 6l12 12M18 6 6 18",
  loader: "M12 3a9 9 0 1 0 9 9",
};

export function Icon({ name, label, class: extra = "" }) {
  return html`<svg
    class=${"icon " + extra}
    viewBox="0 0 24 24"
    role=${label ? "img" : null}
    aria-label=${label || null}
    aria-hidden=${label ? null : "true"}
    focusable="false"
  ><path d=${PATHS[name]} /></svg>`;
}
