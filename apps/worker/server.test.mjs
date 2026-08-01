import test from "node:test";
import assert from "node:assert/strict";
import sharp from "sharp";
import { reencodeImage, renderPNG } from "./server.mjs";

test("renders a deterministic PNG", () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><rect width="100" height="100" fill="red"/></svg>';
  const first = renderPNG(svg, 300); const second = renderPNG(svg, 300);
  assert.deepEqual(first, second);
  assert.deepEqual([...first.subarray(0, 8)], [137,80,78,71,13,10,26,10]);
});

test("rejects unsafe dimensions and non-SVG content", () => {
  assert.throws(() => renderPNG("not svg", 300));
  assert.throws(() => renderPNG("<svg/>", 5001));
});

test("decodes and reencodes uploaded images as PNG", async () => {
  const source = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=", "base64");
  const result = await reencodeImage(source);
  assert.deepEqual([...result.data.subarray(0, 8)], [137,80,78,71,13,10,26,10]);
  assert.equal(result.info.width, 1); assert.equal(result.info.height, 1);
});

test("renders production export sizes", async () => {
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000"><circle cx="500" cy="500" r="400" fill="none" stroke="red" stroke-width="10"/></svg>';
  for (const width of [1000, 3000, 5000]) {
    const metadata = await sharp(renderPNG(svg, width)).metadata();
    assert.equal(metadata.width, width);
    assert.equal(metadata.height, width);
  }
});
