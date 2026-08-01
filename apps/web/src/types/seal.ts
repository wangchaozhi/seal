export type SealShape = "circle" | "ellipse" | "square";
export type TextureType = "ink" | "grain" | "edge" | "scratch" | "paper";
export type TextureTarget = "border" | "text" | "center";
export type LayerKind = "arcText" | "text" | "centerText" | "centerImage" | "border" | "innerRing";

export interface SealCanvas {
  logicalWidth: 1000;
  logicalHeight: 1000;
  exportWidth: number;
  transparent: boolean;
}

export interface BorderConfig {
  width: number;
  doubleLine: boolean;
  innerRing: boolean;
  innerAdjust: number;
}

export interface SealLayer {
  id: string;
  kind: LayerKind;
  visible: boolean;
  locked: boolean;
  zIndex: number;
  content?: string;
  fontId?: string;
  fontSize?: number;
  letterSpacing?: number;
  scaleX?: number;
  scaleY?: number;
  rotation?: number;
  offsetX?: number;
  offsetY?: number;
  radiusX?: number;
  radiusY?: number;
  startAngle?: number;
  sweepAngle?: number;
  assetId?: string;
}

export interface TextureConfig {
  enabled: boolean;
  type: TextureType;
  intensity: number;
  density: number;
  grainSize: number;
  edgeWear: number;
  scratchCount: number;
  fade: number;
  seed: number;
  applyTo: TextureTarget[];
}

export interface SealConfig {
  schemaVersion: 2;
  rendererVersion: string;
  shape: SealShape;
  canvas: SealCanvas;
  color: string;
  border: BorderConfig;
  layers: SealLayer[];
  texture: TextureConfig;
}

export const layerIds = {
  border: "border",
  innerRing: "inner-ring",
  main: "main-text",
  inner: "inner-text",
  bottom: "bottom-text",
  header1: "header-1",
  header2: "header-2",
  center: "center",
} as const;

export function getLayer(config: SealConfig, id: string): SealLayer {
  const layer = config.layers.find((item) => item.id === id);
  if (!layer) throw new Error(`缺少图层：${id}`);
  return layer;
}

export function updateLayer(config: SealConfig, id: string, patch: Partial<SealLayer>): SealConfig {
  return {
    ...config,
    layers: config.layers.map((layer) => layer.id === id ? { ...layer, ...patch } : layer),
  };
}

export function cloneSealConfig(config: SealConfig): SealConfig {
  return structuredClone(config);
}

export const defaultSealConfig: SealConfig = {
  schemaVersion: 2,
  rendererVersion: "2.0.0",
  shape: "circle",
  canvas: {
    logicalWidth: 1000,
    logicalHeight: 1000,
    exportWidth: 1200,
    transparent: true,
  },
  color: "#d92626",
  border: {
    width: 6,
    doubleLine: false,
    innerRing: false,
    innerAdjust: 0,
  },
  layers: [
    { id: layerIds.border, kind: "border", visible: true, locked: true, zIndex: 0 },
    { id: layerIds.innerRing, kind: "innerRing", visible: false, locked: false, zIndex: 1 },
    { id: layerIds.main, kind: "arcText", visible: true, locked: false, zIndex: 10, content: "一个有趣实用科技有限公司", fontId: "system-serif", fontSize: 72, letterSpacing: 6, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 20, radiusX: 335, radiusY: 335, startAngle: 180, sweepAngle: 180 },
    { id: layerIds.inner, kind: "arcText", visible: false, locked: false, zIndex: 11, content: "UNIFIED SOCIAL CREDIT CODE", fontId: "system-sans", fontSize: 34, letterSpacing: 4, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 35, radiusX: 270, radiusY: 270, startAngle: 180, sweepAngle: 180 },
    { id: layerIds.bottom, kind: "arcText", visible: true, locked: false, zIndex: 12, content: "91310101XXXXXXXXXX", fontId: "system-sans", fontSize: 38, letterSpacing: 4, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 0, radiusX: 260, radiusY: 260, startAngle: 0, sweepAngle: -180 },
    { id: layerIds.header1, kind: "text", visible: true, locked: false, zIndex: 13, content: "合同专用章", fontId: "system-serif", fontSize: 46, letterSpacing: 2, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 190 },
    { id: layerIds.header2, kind: "text", visible: false, locked: false, zIndex: 14, content: "内部使用", fontId: "system-serif", fontSize: 38, letterSpacing: 2, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 248 },
    { id: layerIds.center, kind: "centerText", visible: true, locked: false, zIndex: 20, content: "★", fontId: "system-serif", fontSize: 220, letterSpacing: 0, scaleX: 1, scaleY: 1, rotation: 0, offsetX: 0, offsetY: 0 },
  ],
  texture: {
    enabled: false,
    type: "ink",
    intensity: 34,
    density: 42,
    grainSize: 6,
    edgeWear: 25,
    scratchCount: 8,
    fade: 22,
    seed: 928341,
    applyTo: ["border", "text", "center"],
  },
};
