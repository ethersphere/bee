import { file } from "bun";
import { extname, join, normalize } from "path";

const ROOT = import.meta.dir;
const PORT = Number(process.env.PORT ?? 8080);

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".wasm": "application/wasm",
  ".map": "application/json",
};

Bun.serve({
  port: PORT,
  idleTimeout: 0,
  async fetch(req) {
    let { pathname } = new URL(req.url);
    if (pathname === "/") pathname = "/index.html";

    // Resolve within ROOT and guard against path traversal.
    const safe = normalize(pathname).replace(/^(\.\.(\/|\\|$))+/, "");
    const path = join(ROOT, safe);
    const f = file(path);
    if (!(await f.exists())) {
      return new Response("Not found", { status: 404 });
    }

    const type = MIME[extname(path).toLowerCase()] || "application/octet-stream";
    return new Response(f, {
      headers: {
        "Content-Type": type,
        // Allow the service worker (served at /sw_bundle.js) to claim root scope.
        "Service-Worker-Allowed": "/",
        "Cache-Control": "no-cache",
      },
    });
  },
});

console.log(`bee wasm demo: http://localhost:${PORT}`);
