const assetPrefix = "seal-platform-local-asset-v2:";
const allowedTypes = new Set(["image/png", "image/jpeg", "image/webp"]);

export async function importCenterImage(file: File): Promise<string> {
  if (!allowedTypes.has(file.type)) throw new Error("仅支持 PNG、JPEG 或 WebP 图片");
  if (file.size > 5 * 1024 * 1024) throw new Error("图片不能超过 5MB");

  const bitmap = await createImageBitmap(file);
  if (bitmap.width * bitmap.height > 25_000_000) {
    bitmap.close();
    throw new Error("图片像素总数不能超过 2500 万");
  }
  const ratio = Math.min(1, 1200 / Math.max(bitmap.width, bitmap.height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(bitmap.width * ratio));
  canvas.height = Math.max(1, Math.round(bitmap.height * ratio));
  const context = canvas.getContext("2d");
  if (!context) { bitmap.close(); throw new Error("浏览器不支持图片重编码"); }
  context.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  bitmap.close();

  const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((value) => value ? resolve(value) : reject(new Error("图片重编码失败")), "image/webp", .9));
  const dataURL = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error("图片读取失败"));
    reader.readAsDataURL(blob);
  });
  const id = `local-${crypto.randomUUID()}`;
  try { localStorage.setItem(assetPrefix + id, dataURL); } catch { throw new Error("浏览器存储空间不足，请使用更小的图片"); }
  return id;
}

export function resolveLocalAsset(id?: string): string | undefined {
  if (!id) return undefined;
  return id.startsWith("local-") ? localStorage.getItem(assetPrefix + id) ?? undefined : id;
}
