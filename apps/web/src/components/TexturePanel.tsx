import type { ChangeEvent } from "react";
import type { SealConfig, TextureTarget, TextureType } from "../types/seal";

interface Props {
  config: SealConfig;
  onChange: (next: SealConfig, group?: string) => void;
  onInteractionEnd: () => void;
}

const textureNames: Record<TextureType, string> = {
  ink: "油墨不均",
  grain: "颗粒缺损",
  edge: "边缘磨损",
  scratch: "划痕磨损",
  paper: "纸张纤维",
};

const presets: Array<{ id: string; name: string; patch: Partial<SealConfig["texture"]> }> = [
  { id: "off", name: "关闭", patch: { enabled: false, intensity: 0, density: 0, edgeWear: 0, scratchCount: 0, fade: 0 } },
  { id: "light", name: "轻微", patch: { enabled: true, type: "grain", intensity: 18, density: 25, grainSize: 4, edgeWear: 8, scratchCount: 2, fade: 12 } },
  { id: "natural", name: "自然", patch: { enabled: true, type: "ink", intensity: 35, density: 42, grainSize: 6, edgeWear: 24, scratchCount: 8, fade: 22 } },
  { id: "aged", name: "陈旧", patch: { enabled: true, type: "paper", intensity: 55, density: 60, grainSize: 9, edgeWear: 45, scratchCount: 14, fade: 35 } },
  { id: "heavy", name: "重度", patch: { enabled: true, type: "scratch", intensity: 76, density: 78, grainSize: 13, edgeWear: 68, scratchCount: 28, fade: 48 } },
];

export function TexturePanel({ config, onChange, onInteractionEnd }: Props) {
  const patchTexture = (patch: Partial<SealConfig["texture"]>, group?: string) => {
    onChange({ ...config, texture: { ...config.texture, ...patch } }, group);
  };

  const toggleTarget = (target: TextureTarget) => {
    const selected = config.texture.applyTo.includes(target);
    if (selected && config.texture.applyTo.length === 1) return;
    patchTexture({
      applyTo: selected
        ? config.texture.applyTo.filter((item) => item !== target)
        : [...config.texture.applyTo, target],
    });
  };

  const range = (
    label: string,
    key: keyof Pick<SealConfig["texture"], "intensity" | "density" | "grainSize" | "edgeWear" | "scratchCount" | "fade">,
    min: number,
    max: number,
  ) => (
    <label className="field range-field">
      <span>{label}</span>
      <input type="range" min={min} max={max} value={config.texture[key]}
        onChange={(event: ChangeEvent<HTMLInputElement>) => patchTexture({ [key]: Number(event.target.value) }, `texture:${key}`)}
        onPointerUp={onInteractionEnd} onKeyUp={onInteractionEnd} onBlur={onInteractionEnd} />
      <output>{config.texture[key]}</output>
    </label>
  );

  return (
    <section className="panel texture-panel">
      <div className="panel-heading">
        <h2>做旧纹理</h2>
        <label className="switch-line">
          <input type="checkbox" checked={config.texture.enabled}
            onChange={(event) => patchTexture({ enabled: event.target.checked })} />
          开启
        </label>
      </div>

      <div className="preset-row" aria-label="纹理预设">
        {presets.map((preset) => (
          <button key={preset.id} type="button" onClick={() => patchTexture(preset.patch)}>{preset.name}</button>
        ))}
      </div>

      <div className="texture-body">
        <label className="field">
          <span>纹理类型</span>
          <select value={config.texture.type} onChange={(event) => patchTexture({ type: event.target.value as TextureType })} disabled={!config.texture.enabled}>
            {Object.entries(textureNames).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>

        <fieldset disabled={!config.texture.enabled}>
          {range("强度", "intensity", 0, 100)}
          {range("缺损密度", "density", 0, 100)}
          {range("颗粒大小", "grainSize", 1, 24)}
          {range("边缘磨损", "edgeWear", 0, 100)}
          {range("划痕数量", "scratchCount", 0, 40)}
          {range("渐变", "fade", 0, 100)}

          <label className="field">
            <span>随机种子</span>
            <div className="inline-control">
              <input type="number" min={1} max={2147483647} value={config.texture.seed}
                onChange={(event) => patchTexture({ seed: Number(event.target.value) || 1 }, "texture:seed")}
                onBlur={onInteractionEnd} />
              <button type="button" onClick={() => patchTexture({ seed: Math.floor(1 + Math.random() * 2147483000) })}>随机</button>
            </div>
          </label>

          <div className="field target-field">
            <span>作用图层</span>
            <div className="check-row">
              {([[
                "border", "边框",
              ], ["text", "文字"], ["center", "中心"]] as Array<[TextureTarget, string]>).map(([value, label]) => (
                <label key={value}><input type="checkbox" checked={config.texture.applyTo.includes(value)} onChange={() => toggleTarget(value)} />{label}</label>
              ))}
            </div>
          </div>
        </fieldset>
      </div>
    </section>
  );
}
