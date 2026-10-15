// Dependency-free static server for dist/ with SPA fallback.
import { createServer, request as httpRequest } from "node:http";
import { connect } from "node:net";
import { readFile, stat } from "node:fs/promises";
import { dirname, extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";
import { gzipSync } from "node:zlib";

const dist = join(dirname(fileURLToPath(import.meta.url)), "..", "dist");
const port = Number(process.env.PORT || 4173);
const types = {
  ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
  ".json": "application/json", ".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg",
  ".ico": "image/x-icon", ".map": "application/json", ".webmanifest": "application/manifest+json",
};

// Optional dev proxy: API_PROXY=http://localhost:8080 forwards /api/* to the Go API (stripping "/api"),
// so the browser sees one origin and no CORS setup is needed locally.
const apiProxy = process.env.API_PROXY ? new URL(process.env.API_PROXY) : null;

/* ───────── production hardening ─────────
 * - Security headers always. A strict CSP when NODE_ENV=production or CSP=1 (off in plain local dev so tooling keeps working).
 *   MEDIA_ORIGIN = the storage host browsers upload to / load files from (e.g. https://xyz.r2.cloudflarestorage.com or http://localhost:9000).
 * - /config.js is generated from env (KIKBITE_LIVE=1 → live auth + chat (this is the default when NODE_ENV=production; KIKBITE_LIVE=0 forces the offline mock); KIKBITE_CONFIG = extra JSON) so no index.html editing is needed.
 *   It never overrides a config the page already defined (tests / inline scripts win).
 * - gzip for text assets, ETag + 304, /healthz for the platform health check.
 */
const prod = process.env.NODE_ENV === "production" || process.env.CSP === "1";
const mediaOrigin = process.env.MEDIA_ORIGIN || "";
const csp = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
  "font-src 'self' https://fonts.gstatic.com",
  `img-src 'self' data: blob: ${mediaOrigin}`.trim(),
  `media-src 'self' blob: ${mediaOrigin}`.trim(),
  `connect-src 'self' ${mediaOrigin} ${process.env.EXTRA_CONNECT_SRC || ""}`.replace(/\s+/g, " ").trim(),
  "object-src 'none'",
  "base-uri 'self'",
  "frame-ancestors 'none'",
  "form-action 'self'",
].join("; ");
const secHeaders = {
  "X-Content-Type-Options": "nosniff",
  "X-Frame-Options": "DENY",
  "Referrer-Policy": "strict-origin-when-cross-origin",
  "Permissions-Policy": "camera=(self), microphone=(self), geolocation=()",
  ...(prod ? { "Content-Security-Policy": csp } : {}),
  ...(prod && process.env.HSTS !== "0" ? { "Strict-Transport-Security": "max-age=15552000" } : {}),
};
const runtimeConfig = () => {
  let extra = {};
  try { extra = process.env.KIKBITE_CONFIG ? JSON.parse(process.env.KIKBITE_CONFIG) : {}; } catch { /* ignore a malformed value */ }
  const base = (process.env.KIKBITE_LIVE === "1" || (process.env.NODE_ENV === "production" && process.env.KIKBITE_LIVE !== "0")) ? { mock: true, liveAuth: true, liveChat: true } : {};
  return `window.__KIKBITE_CONFIG__=Object.assign(${JSON.stringify({ ...base, ...extra })},window.__KIKBITE_CONFIG__||{});`;
};
const compressible = /^(text\/|application\/(json|javascript)|image\/svg)/;
function reply(req, res, status, type, body, cache = "no-cache") {
  const etag = `"${createHash("sha1").update(body).digest("base64url")}"`;
  const headers = { ...secHeaders, "Content-Type": type, "Cache-Control": cache, ETag: etag };
  if (req.headers["if-none-match"] === etag) { res.writeHead(304, headers).end(); return; }
  if (compressible.test(type) && /\bgzip\b/.test(req.headers["accept-encoding"] || "") && body.length > 1024) {
    body = gzipSync(body);
    headers["Content-Encoding"] = "gzip";
    headers.Vary = "Accept-Encoding";
  }
  res.writeHead(status, headers).end(req.method === "HEAD" ? undefined : body);
}

const server = createServer(async (req, res) => {
  if (req.url === "/healthz") { res.writeHead(200, { "Content-Type": "text/plain" }).end("ok"); return; }
  if (req.url.split("?")[0] === "/config.js") return reply(req, res, 200, "text/javascript; charset=utf-8", Buffer.from(runtimeConfig()));
  if (apiProxy && req.url.startsWith("/api/")) {
    const up = httpRequest({ hostname: apiProxy.hostname, port: apiProxy.port, method: req.method, path: req.url.slice(4), headers: { ...req.headers, host: apiProxy.host } }, (r) => {
      res.writeHead(r.statusCode, r.headers);
      r.pipe(res);
    });
    up.on("error", () => res.writeHead(502).end("API unreachable"));
    req.pipe(up);
    return;
  }
  try {
    const urlPath = decodeURIComponent(new URL(req.url, "http://x").pathname);
    let file = normalize(join(dist, urlPath));
    if (!file.startsWith(dist)) { res.writeHead(403).end(); return; }
    let ok = await stat(file).then((s) => s.isFile()).catch(() => false);
    if (!ok) { file = join(dist, "index.html"); ok = true; } // SPA fallback
    const body = await readFile(file);
    reply(req, res, 200, types[extname(file)] || "application/octet-stream", body);
  } catch (e) {
    res.writeHead(500).end(String(e));
  }
});

// WebSocket pass-through for the live chat: /ws → the Go API's /ws. The Origin header is forwarded untouched, so the
// backend's browser Origin allow-list is exercised exactly as in production (APP_ENV=local allows http://localhost:4173).
if (apiProxy) {
  server.on("upgrade", (req, socket, head) => {
    if (!req.url.startsWith("/ws")) return socket.destroy();
    const up = connect(Number(apiProxy.port) || 80, apiProxy.hostname, () => {
      const lines = [`${req.method} ${req.url} HTTP/1.1`];
      for (let i = 0; i < req.rawHeaders.length; i += 2) {
        const k = req.rawHeaders[i];
        lines.push(`${k}: ${k.toLowerCase() === "host" ? apiProxy.host : req.rawHeaders[i + 1]}`);
      }
      up.write(lines.join("\r\n") + "\r\n\r\n");
      if (head?.length) up.write(head);
      socket.pipe(up).pipe(socket);
    });
    up.on("error", () => socket.destroy());
    socket.on("error", () => up.destroy());
  });
}

server.listen(port, () => console.log(`KikBite web on http://localhost:${port}${apiProxy ? ` (API → ${apiProxy.origin})` : ""}`));
