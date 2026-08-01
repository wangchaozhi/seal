import { useState } from "react";
import { importCenterImage } from "../lib/localAssets";
import { getLayer, layerIds, updateLayer, type SealConfig, type SealLayer, type SealShape } from "../types/seal";

interface Props {
  config: SealConfig;
  onChange: (next: SealConfig, group?: string) => void;
  onInteractionEnd: () => void;
}

const editableLayers = [
  [layerIds.main, "主体文字"],
  [layerIds.inner, "内圈文字"],
  [layerIds.bottom, "底部文字"],
  [layerIds.header1, "抬头文字"],
  [layerIds.header2, "抬头文字 2"],
  [layerIds.center, "中心内容"],
] as const;

export function EditorPanel({ config, onChange, onInteractionEnd }: Props) {
  const [advancedLayerID, setAdvancedLayerID] = useState<string>(layerIds.main);
  const [uploadStatus, setUploadStatus] = useState("");
  const advanced = getLayer(config, advancedLayerID);
  const changeLayer = (id: string, patch: Partial<SealLayer>, group?: string) => onChange(updateLayer(config, id, patch), group);

  const textRow = (id: string, label: string, maxLength: number, canHide = false) => {
    const layer = getLayer(config, id);
    return (
      <div className="layer-row" key={id}>
        <label className="field">
          <span>{label}</span>
          <input value={layer.content ?? ""} maxLength={maxLength} disabled={layer.locked}
            onChange={(event) => changeLayer(id, { content: event.target.value }, `layer:${id}:content`)} onBlur={onInteractionEnd} />
        </label>
        <div className="layer-actions">
          {canHide && <label title="显示图层"><input type="checkbox" checked={layer.visible} onChange={(event) => changeLayer(id, { visible: event.target.checked })} />显示</label>}
          <button type="button" onClick={() => setAdvancedLayerID(id)} aria-pressed={advancedLayerID === id}>高级</button>
        </div>
      </div>
    );
  };

  const advancedRange = (label: string, key: keyof SealLayer, min: number, max: number, step = 1) => (
    <label className="field range-field">
      <span>{label}</span>
      <input type="range" min={min} max={max} step={step} value={Number(advanced[key] ?? 0)} disabled={advanced.locked}
        onChange={(event) => changeLayer(advanced.id, { [key]: Number(event.target.value) }, `layer:${advanced.id}:${String(key)}`)}
        onPointerUp={onInteractionEnd} onKeyUp={onInteractionEnd} onBlur={onInteractionEnd} />
      <output>{Number(advanced[key] ?? 0).toFixed(step < 1 ? 1 : 0)}</output>
    </label>
  );

  const uploadCenterImage = async (file?: File) => {
    if (!file) return;
    try {
      setUploadStatus("正在安全重编码…");
      const assetId = await importCenterImage(file);
      changeLayer(layerIds.center, { kind: "centerImage", assetId, visible: true });
      setUploadStatus("图片已重编码并保存");
    } catch (error) {
      setUploadStatus(error instanceof Error ? error.message : "图片处理失败");
    }
  };

  return (
    <section className="panel editor-panel">
      <div className="panel-heading"><h1>印章编辑器</h1><small>SealConfig v2</small></div>
      <div className="shape-tabs" aria-label="印章形状">
        {(["circle", "ellipse", "square"] as SealShape[]).map((shape) => (
          <button key={shape} type="button" className={config.shape === shape ? "active" : ""} onClick={() => onChange({ ...config, shape })}>
            {{ circle: "正圆章", ellipse: "椭圆章", square: "方章" }[shape]}
          </button>
        ))}
      </div>

      <div className="form-grid basic-fields">
        {textRow(layerIds.main, "主体文字", 80)}
        {textRow(layerIds.inner, "内圈文字", 80, true)}
        {textRow(layerIds.bottom, "底部文字", 50, true)}
        {textRow(layerIds.center, "中心内容", 20)}
        {textRow(layerIds.header1, "抬头文字", 30, true)}
        {textRow(layerIds.header2, "抬头文字 2", 30, true)}

        <label className="field"><span>印章颜色</span><div className="color-control">
          <input type="color" value={config.color} onChange={(event) => onChange({ ...config, color: event.target.value }, "color")} onBlur={onInteractionEnd} />
          <input value={config.color.toUpperCase()} maxLength={7} onChange={(event) => /^#[0-9a-fA-F]{0,6}$/.test(event.target.value) && onChange({ ...config, color: event.target.value }, "color-text")} onBlur={onInteractionEnd} />
        </div></label>
        <label className="field"><span>导出尺寸</span><input type="number" min="300" max="5000" value={config.canvas.exportWidth}
          onChange={(event) => onChange({ ...config, canvas: { ...config.canvas, exportWidth: Math.min(5000, Math.max(300, Number(event.target.value) || 300)) } }, "export-size")} onBlur={onInteractionEnd} /></label>
        <label className="field"><span>透明背景</span><input type="checkbox" checked={config.canvas.transparent} onChange={(event) => onChange({ ...config, canvas: { ...config.canvas, transparent: event.target.checked } })} /></label>
        <div className="field"><span>中心图案</span><div className="upload-control">
          <select value={getLayer(config, layerIds.center).kind} onChange={(event) => changeLayer(layerIds.center, { kind: event.target.value as "centerText" | "centerImage" })}><option value="centerText">文字</option><option value="centerImage">图片</option></select>
          <input type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => void uploadCenterImage(event.target.files?.[0])} />
          {uploadStatus && <small>{uploadStatus}</small>}
        </div></div>
      </div>

      <details className="advanced-block" open>
        <summary>图层高级参数</summary>
        <div className="advanced-tabs">
          {editableLayers.map(([id, label]) => <button type="button" key={id} className={advancedLayerID === id ? "active" : ""} onClick={() => setAdvancedLayerID(id)}>{label}</button>)}
        </div>
        <div className="advanced-grid">
          <label className="field"><span>字体</span><select value={advanced.fontId ?? "system-serif"} disabled={advanced.locked} onChange={(event) => changeLayer(advanced.id, { fontId: event.target.value })}><option value="system-serif">系统宋体</option><option value="system-sans">系统黑体</option></select></label>
          {advancedRange("字号", "fontSize", 6, 300)}
          {advancedRange("字间距", "letterSpacing", -20, 60)}
          {advancedRange("横向缩放", "scaleX", .2, 3, .1)}
          {advancedRange("纵向缩放", "scaleY", .2, 3, .1)}
          {advancedRange("水平偏移", "offsetX", -300, 300)}
          {advancedRange("垂直偏移", "offsetY", -300, 300)}
          {advancedRange("旋转", "rotation", -180, 180)}
          {advanced.kind === "arcText" && advancedRange("横向半径", "radiusX", 80, 450)}
          {advanced.kind === "arcText" && advancedRange("纵向半径", "radiusY", 80, 450)}
          <label className="field"><span>锁定图层</span><input type="checkbox" checked={advanced.locked} onChange={(event) => changeLayer(advanced.id, { locked: event.target.checked })} /></label>
        </div>
      </details>

      <details className="advanced-block">
        <summary>边框高级参数</summary>
        <div className="advanced-grid">
          <label className="field range-field"><span>线条粗细</span><input type="range" min="1" max="20" value={config.border.width}
            onChange={(event) => onChange({ ...config, border: { ...config.border, width: Number(event.target.value) } }, "border:width")}
            onPointerUp={onInteractionEnd} onKeyUp={onInteractionEnd} onBlur={onInteractionEnd} /><output>{config.border.width}</output></label>
          <label className="field"><span>双外圈</span><input type="checkbox" checked={config.border.doubleLine} onChange={(event) => onChange({ ...config, border: { ...config.border, doubleLine: event.target.checked } })} /></label>
          <label className="field"><span>内环</span><input type="checkbox" checked={config.border.innerRing} onChange={(event) => onChange({ ...config, border: { ...config.border, innerRing: event.target.checked } })} /></label>
          <label className="field range-field"><span>内环微调</span><input type="range" min="-50" max="50" value={config.border.innerAdjust}
            onChange={(event) => onChange({ ...config, border: { ...config.border, innerAdjust: Number(event.target.value) } }, "border:inner")}
            onPointerUp={onInteractionEnd} onKeyUp={onInteractionEnd} onBlur={onInteractionEnd} /><output>{config.border.innerAdjust}</output></label>
        </div>
      </details>
    </section>
  );
}
