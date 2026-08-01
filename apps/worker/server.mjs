import http from "node:http";
import { pathToFileURL } from "node:url";
import { Resvg } from "@resvg/resvg-js";
import sharp from "sharp";

export const MAX_BODY_BYTES = 2 * 1024 * 1024;
export const MAX_UPLOAD_BYTES = 5 * 1024 * 1024;

export function renderPNG(svg, width) {
  if (typeof svg !== "string" || !svg.startsWith("<svg") || Buffer.byteLength(svg) > MAX_BODY_BYTES) throw new Error("invalid SVG");
  if (!Number.isInteger(width) || width < 300 || width > 5000) throw new Error("invalid width");
  const renderer = new Resvg(svg, { fitTo: { mode: "width", value: width }, font: { loadSystemFonts: true, defaultFontFamily: "sans-serif" } });
  return renderer.render().asPng();
}

export async function reencodeImage(input) {
  if (!Buffer.isBuffer(input) || input.length === 0 || input.length > MAX_UPLOAD_BYTES) throw new Error("invalid image size");
  const result = await sharp(input, { limitInputPixels: 25_000_000, failOn: "warning", sequentialRead: true })
    .rotate().resize({ width: 2000, height: 2000, fit: "inside", withoutEnlargement: true })
    .png({ compressionLevel: 9, adaptiveFiltering: true }).toBuffer({ resolveWithObject: true });
  if (!result.info.width || !result.info.height || result.info.width * result.info.height > 25_000_000) throw new Error("invalid image dimensions");
  return result;
}

export function createServer() {
  return http.createServer((request, response) => {
    if (request.method === "GET" && request.url === "/health") {
      response.writeHead(200, { "content-type": "application/json" }); response.end('{"status":"ok"}'); return;
    }
    if (request.method === "POST" && request.url === "/images/reencode") {
      const inputType = String(request.headers["content-type"] ?? "").split(";", 1)[0];
      if (!new Set(["image/png", "image/jpeg", "image/webp"]).has(inputType)) { response.writeHead(415); response.end("unsupported image type"); return; }
      const chunks = []; let size = 0; let rejected = false;
      request.on("data", (chunk) => { size += chunk.length; if (size > MAX_UPLOAD_BYTES) { rejected = true; response.writeHead(413); response.end("image too large"); request.destroy(); return; } chunks.push(chunk); });
      request.on("end", async () => {
        if (rejected) return;
        try { const result = await reencodeImage(Buffer.concat(chunks)); response.writeHead(200, { "content-type": "image/png", "x-image-width": result.info.width, "x-image-height": result.info.height, "content-length": result.data.length, "cache-control": "no-store" }); response.end(result.data); }
        catch (error) { response.writeHead(422, { "content-type": "application/json" }); response.end(JSON.stringify({ error: error.message })); }
      });
      return;
    }
    if (request.method !== "POST" || request.url !== "/render") { response.writeHead(404); response.end(); return; }
    const contentType = request.headers["content-type"] ?? "";
    if (!contentType.startsWith("application/json")) { response.writeHead(415); response.end("JSON required"); return; }
    const chunks = []; let size = 0; let rejected = false;
    request.on("data", (chunk) => {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) { rejected = true; response.writeHead(413); response.end("body too large"); request.destroy(); return; }
      chunks.push(chunk);
    });
    request.on("end", () => {
      if (rejected) return;
      try {
        const body = JSON.parse(Buffer.concat(chunks).toString("utf8"));
        const png = renderPNG(body.svg, body.width);
        response.writeHead(200, { "content-type": "image/png", "content-length": png.length, "cache-control": "no-store" }); response.end(png);
      } catch (error) { response.writeHead(422, { "content-type": "application/json" }); response.end(JSON.stringify({ error: error.message })); }
    });
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const port = Number(process.env.PORT || 8090);
  createServer().listen(port, "0.0.0.0", () => console.log(JSON.stringify({ level: "info", message: "render worker started", port })));
}
