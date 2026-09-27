import { createServer } from "node:http";

const HASH_A = "a".repeat(40);
const HASH_B = "b".repeat(40);

const page = `<html><body><table>
<tr class="r"><td class="n">Fixture Result One</td><td class="m"><a href="magnet:?xt=urn:btih:${HASH_A}&dn=One">m</a></td>
<td class="s">1.2 GB</td><td class="se">42</td><td class="p">7</td></tr>
<tr class="r"><td class="n">Fixture Result Two</td><td class="m"><a href="magnet:?xt=urn:btih:${HASH_B}&dn=Two">m</a></td>
<td class="s">700 MB</td><td class="se">7</td><td class="p">1</td></tr>
</table></body></html>`;

// startFakeSearch serves a search-config with one provider, "fake", whose
// results page is served by the same server, so search works offline.
export function startFakeSearch() {
  const server = createServer((req, res) => {
    const { port } = server.address();
    if (req.url === "/config.json") {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        fake: {
          name: "Fake",
          url: `http://127.0.0.1:${port}/search?q={{query}}&p={{page:1}}`,
          list: "tr.r",
          result: {
            name: "td.n",
            magnet: ["td.m a", "@href"],
            size: "td.s",
            seeds: "td.se",
            peers: "td.p",
          },
        },
      }));
      return;
    }
    if (req.url.startsWith("/search")) {
      const pageNo = new URL(req.url, "http://x").searchParams.get("p");
      res.setHeader("content-type", "text/html");
      res.end(pageNo === "1" ? page : "<html><body><table></table></body></html>");
      return;
    }
    res.statusCode = 404;
    res.end();
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      resolve({
        configUrl: `http://127.0.0.1:${port}/config.json`,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}
