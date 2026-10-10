// The expiry choices in the share menu; the middle one is the default.
export const SHARE_TTLS = [
  { label: "1 hour", seconds: 3600 },
  { label: "24 hours", seconds: 86400 },
  { label: "7 days", seconds: 604800 },
];
export const DEFAULT_TTL = 1;

// shareText turns the server's relative links into the text to copy, one
// absolute link per line, without any user:password@ the page was opened with.
export function shareText(links, base) {
  return links
    .map((l) => {
      const url = new URL(l, base);
      url.username = "";
      url.password = "";
      return url.href;
    })
    .join("\n");
}
