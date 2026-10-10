import { html } from "./html.js";

const PATHS = {
  cloud: "M7 18h10a4 4 0 0 0 .6-7.96A6 6 0 0 0 6.1 9.1 4.5 4.5 0 0 0 7 18z",
  search: "M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0zM21 21l-6-6",
  magnet: "M5 13V9a7 7 0 0 1 14 0v4h-4V9a3 3 0 0 0-6 0v4zM5 13a7 7 0 0 0 14 0",
  settings: "M19.3 10L21.9 10.6L21.9 13.4L19.3 14L18.6 15.8L20 18L18 20L15.8 18.6L14 19.3L13.4 21.9L10.6 21.9L10 19.3L8.2 18.6L6 20L4 18L5.4 15.8L4.7 14L2.1 13.4L2.1 10.6L4.7 10L5.4 8.2L4 6L6 4L8.2 5.4L10 4.7L10.6 2.1L13.4 2.1L14 4.7L15.8 5.4L18 4L20 6L18.6 8.2zM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  sun: "M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4",
  moon: "M20 14.5A8.5 8.5 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z",
  themeAuto: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0zM12 3v18M12 7h4.5M12 11h6.8M12 15h6.2M12 19h3",
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
  link: "M10 14a4 4 0 0 0 5.66 0l3-3a4 4 0 0 0-5.66-5.66l-1 1M14 10a4 4 0 0 0-5.66 0l-3 3a4 4 0 0 0 5.66 5.66l1-1",
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
