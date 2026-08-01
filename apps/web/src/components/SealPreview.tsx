import { useMemo, useState } from "react";
import { resolveLocalAsset } from "../lib/localAssets";
import { assetURL } from "../lib/api";
import { getLayer, layerIds, type SealConfig, type SealLayer } from "../types/seal";

interface Props { config: SealConfig }
interface Dot { x: number; y: number; radius: number; opacity: number }
interface Scratch { x1: number; y1: number; x2: number; y2: number; width: number; opacity: number }

function xorshift32(seed: number): () => number {
  let value = seed | 0 || 123456789;
  return () => {
    value ^= value << 13;
    value ^= value >>> 17;
    value ^= value << 5;
    return (value >>> 0) / 4294967296;
  };
}

function texturePrimitives(config: SealConfig): { dots: Dot[]; scratches: Scratch[] } {
  if (!config.texture.enabled) return { dots: [], scratches: [] };
  const random = xorshift32(config.texture.seed);
  const count = Math.round(30 + config.texture.density * 2.2);
  const scale = 0.55 + config.texture.intensity / 100;
  const dots = Array.from({ length: count }, () => {
    const a = random();
    const b = random();
    const size = random();
    const alpha = random();
    let x = 90 + a * 820;
    let y = 90 + b * 820;
    if (config.texture.type === "edge") {
      const angle = a * Math.PI * 2;
      const radius = 365 + b * (20 + config.texture.edgeWear * 0.45);
      x = 500 + Math.cos(angle) * radius;
      y = 500 + Math.sin(angle) * radius;
    }
    return {
      x,
      y,
      radius: (0.8 + size * config.texture.grainSize * (config.texture.type === "ink" ? 2.4 : 1.7)) * scale,
      opacity: Math.min(1, 0.3 + alpha * 0.55 + config.texture.fade / 500),
    };
  });
  const lineCount = config.texture.type === "paper"
    ? Math.max(config.texture.scratchCount, Math.round(config.texture.density / 3))
    : config.texture.scratchCount;
  const scratches = Array.from({ length: lineCount }, () => {
    const x = 120 + random() * 760;
    const y = 130 + random() * 740;
    const length = 20 + random() * (45 + config.texture.intensity * 1.3);
    const angle = config.texture.type === "paper" ? (random() - 0.5) * 0.35 : (random() - 0.5) * 1.2;
    return {
      x1: x, y1: y,
      x2: x + Math.cos(angle) * length,
      y2: y + Math.sin(angle) * length,
      width: 1 + random() * config.texture.grainSize * 0.55,
      opacity: 0.4 + random() * 0.5,
    };
  });
  return { dots, scratches };
}

function layerStyle(layer: SealLayer) {
  return {
    fontFamily: layer.fontId === "system-sans" ? "Arial, sans-serif" : "STSong, SimSun, serif",
    fontSize: layer.fontSize,
    letterSpacing: layer.letterSpacing,
    fontWeight: 700,
  };
}

function transform(layer: SealLayer, baseY = 500) {
  const x = 500 + (layer.offsetX ?? 0);
  const y = baseY + (layer.offsetY ?? 0);
  return `translate(${x} ${y}) rotate(${layer.rotation ?? 0}) scale(${layer.scaleX ?? 1} ${layer.scaleY ?? 1}) translate(${-x} ${-y})`;
}

export function SealPreview({ config }: Props) {
  const [compare, setCompare] = useState(false);
  const texture = useMemo(() => texturePrimitives(config), [config]);
  const layers = useMemo(() => Object.fromEntries(config.layers.map((layer) => [layer.id, layer])), [config.layers]);
  const main = layers[layerIds.main];
  const inner = layers[layerIds.inner];
  const bottom = layers[layerIds.bottom];
  const header1 = layers[layerIds.header1];
  const header2 = layers[layerIds.header2];
  const center = layers[layerIds.center];
  const isSquare = config.shape === "square";
  const isEllipse = config.shape === "ellipse";
  const mainRX = isEllipse ? Math.min(main.radiusX ?? 335, 355) : main.radiusX ?? 335;
  const mainRY = isEllipse ? Math.min(main.radiusY ?? 335, 280) : main.radiusY ?? 335;
  const maskEnabled = config.texture.enabled && !compare;
  const maskFor = (target: "border" | "text" | "center") => maskEnabled && config.texture.applyTo.includes(target) ? "url(#wearMask)" : undefined;

  // Fail early during development if a required layer disappeared from a loaded configuration.
  getLayer(config, layerIds.border);

  return (
    <section className="panel preview-panel">
      <div className="panel-heading">
        <h2>实时预览</h2>
        <div className="preview-actions">
          <button type="button" onPointerDown={() => setCompare(true)} onPointerUp={() => setCompare(false)} onPointerLeave={() => setCompare(false)}>按住看原图</button>
          <span>{config.canvas.exportWidth} × {config.canvas.exportWidth}px</span>
        </div>
      </div>
      <div className="preview-stage">
        <svg id="seal-preview-svg" viewBox="0 0 1000 1000" role="img" aria-label="印章预览" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <path id="mainArc" d={`M${500 - mainRX} ${500 + (main.offsetY ?? 0)} A${mainRX} ${mainRY} 0 0 1 ${500 + mainRX} ${500 + (main.offsetY ?? 0)}`} />
            <path id="innerArc" d={`M${500 - (inner.radiusX ?? 270)} ${500 + (inner.offsetY ?? 35)} A${inner.radiusX ?? 270} ${inner.radiusY ?? 270} 0 0 1 ${500 + (inner.radiusX ?? 270)} ${500 + (inner.offsetY ?? 35)}`} />
            <path id="bottomArc" d={`M${500 - (bottom.radiusX ?? 260)} ${500 + (bottom.offsetY ?? 155)} A${bottom.radiusX ?? 260} ${bottom.radiusY ?? 260} 0 0 0 ${500 + (bottom.radiusX ?? 260)} ${500 + (bottom.offsetY ?? 155)}`} />
            <mask id="wearMask" maskUnits="userSpaceOnUse">
              <rect width="1000" height="1000" fill="white" />
              {texture.dots.map((dot, index) => <circle key={`d-${index}`} cx={dot.x} cy={dot.y} r={dot.radius} fill="black" opacity={dot.opacity} />)}
              {texture.scratches.map((line, index) => <line key={`s-${index}`} {...line} stroke="black" strokeWidth={line.width} strokeLinecap="round" />)}
            </mask>
          </defs>

          <g fill={config.color} stroke={config.color} mask={maskFor("border")}>
            {isSquare
              ? <rect x="120" y="120" width="760" height="760" rx="14" fill="none" strokeWidth={config.border.width} />
              : isEllipse
                ? <ellipse cx="500" cy="500" rx="410" ry="330" fill="none" strokeWidth={config.border.width} />
                : <circle cx="500" cy="500" r="390" fill="none" strokeWidth={config.border.width} />}
            {config.border.doubleLine && (isSquare
              ? <rect x="138" y="138" width="724" height="724" rx="10" fill="none" strokeWidth={Math.max(2, config.border.width / 2)} />
              : isEllipse
                ? <ellipse cx="500" cy="500" rx="390" ry="312" fill="none" strokeWidth={Math.max(2, config.border.width / 2)} />
                : <circle cx="500" cy="500" r="370" fill="none" strokeWidth={Math.max(2, config.border.width / 2)} />)}
            {config.border.innerRing && (isEllipse
              ? <ellipse cx="500" cy="500" rx={280 + config.border.innerAdjust} ry={220 + config.border.innerAdjust} fill="none" strokeWidth="3" />
              : <circle cx="500" cy="500" r={280 + config.border.innerAdjust} fill="none" strokeWidth="3" />)}
          </g>

          <g fill={config.color} stroke="none" mask={maskFor("text")}>
            {main.visible && (isSquare
              ? <text x={500 + (main.offsetX ?? 0)} y={350 + (main.offsetY ?? 0)} textAnchor="middle" style={layerStyle(main)} transform={transform(main, 350)}>{main.content}</text>
              : <text style={layerStyle(main)} transform={transform(main)}><textPath href="#mainArc" startOffset="50%" textAnchor="middle">{main.content}</textPath></text>)}
            {inner.visible && !isSquare && <text style={layerStyle(inner)} transform={transform(inner)}><textPath href="#innerArc" startOffset="50%" textAnchor="middle">{inner.content}</textPath></text>}
            {bottom.visible && !isSquare && <text style={layerStyle(bottom)} transform={transform(bottom)}><textPath href="#bottomArc" startOffset="50%" textAnchor="middle">{bottom.content}</textPath></text>}
            {header1.visible && <text x={500 + (header1.offsetX ?? 0)} y={500 + (header1.offsetY ?? 0)} textAnchor="middle" style={layerStyle(header1)} transform={transform(header1)}>{header1.content}</text>}
            {header2.visible && <text x={500 + (header2.offsetX ?? 0)} y={500 + (header2.offsetY ?? 0)} textAnchor="middle" style={layerStyle(header2)} transform={transform(header2)}>{header2.content}</text>}
          </g>

          {center.visible && <g fill={config.color} stroke="none" mask={maskFor("center")}>
            {center.kind === "centerImage" && center.assetId
              ? <image href={center.assetId.startsWith("ast_") ? assetURL(center.assetId) : resolveLocalAsset(center.assetId)} x={350 + (center.offsetX ?? 0)} y={350 + (center.offsetY ?? 0)} width="300" height="300" preserveAspectRatio="xMidYMid meet" transform={transform(center)} />
              : <text x={500 + (center.offsetX ?? 0)} y={500 + (center.offsetY ?? 0)} textAnchor="middle" dominantBaseline="middle" style={layerStyle(center)} transform={transform(center)}>{center.content}</text>}
          </g>}

          <g fill="#d92626" aria-label="预览水印">
            <text x="500" y="490" textAnchor="middle" transform="rotate(-25 500 500)" className="watermark">PREVIEW · 未解锁</text>
            <text x="500" y="535" textAnchor="middle" fontSize="20" opacity=".14">SESSION-{config.texture.seed.toString(36).toUpperCase()}</text>
          </g>
        </svg>
      </div>
      <div className="preview-meta"><span>逻辑画布 1000 × 1000</span><span>低清预览 · 带水印</span><span>{config.texture.type} · seed {config.texture.seed}</span></div>
    </section>
  );
}
