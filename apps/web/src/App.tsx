import { useEffect, useState } from "react";
import { EditorPanel } from "./components/EditorPanel";
import { CloudPanel } from "./components/CloudPanel";
import { AdminPanel } from "./components/AdminPanel";
import { SealPreview } from "./components/SealPreview";
import { TexturePanel } from "./components/TexturePanel";
import { useSealHistory } from "./hooks/useSealHistory";
import { renderOnServer } from "./lib/api";
import { cloneSealConfig, defaultSealConfig, layerIds, updateLayer, type SealConfig } from "./types/seal";
import "./styles.css";

const draftKey = "seal-platform-draft-v2";
const savedKey = "seal-platform-configs-v2";
const generationHistoryKey = "seal-platform-generation-history-v2";
const favoriteTemplatesKey = "seal-platform-favorite-templates-v2";

interface SavedConfig { id: string; name: string; updatedAt: string; config: SealConfig }
interface GenerationHistory { id: string; createdAt: string; format: "SVG" | "PNG" | "SERVER_SVG"; name: string; config: SealConfig }
interface TemplateDefinition { id: string; name: string; category: "企业" | "财务" | "个人" | "创意"; description: string; tags: string[] }

const templates: TemplateDefinition[] = [
  { id: "company", name: "标准企业圆章", category: "企业", description: "标准圆章、主体弧形文字与统一代码", tags: ["企业", "标准", "圆章"] },
  { id: "ellipse", name: "双环椭圆章", category: "企业", description: "双外圈、内圈英文与椭圆排版", tags: ["企业", "英文", "椭圆"] },
  { id: "finance", name: "财务专用章", category: "财务", description: "中心财字、专用章抬头与内部核算", tags: ["财务", "专用章"] },
  { id: "contract", name: "合同专用章", category: "企业", description: "合同业务常用文字布局", tags: ["合同", "企业"] },
  { id: "personal", name: "个人方章", category: "个人", description: "方形姓名章与大字布局", tags: ["个人", "方章"] },
  { id: "aged", name: "自然做旧章", category: "创意", description: "固定 seed 的自然油墨缺损", tags: ["做旧", "纹理", "创意"] },
];

function loadList<T>(key: string): T[] {
  try { return JSON.parse(localStorage.getItem(key) ?? "[]") as T[]; } catch { return []; }
}

function loadDraft(): SealConfig {
  try {
    const raw = JSON.parse(localStorage.getItem(draftKey) ?? "null") as Record<string, unknown> | null;
    if (!raw) return cloneSealConfig(defaultSealConfig);
    if (raw.schemaVersion === 2 && Array.isArray(raw.layers)) {
      const config = raw as unknown as SealConfig;
      const bottom = config.layers.find((layer) => layer.id === layerIds.bottom);
      // Repair drafts saved with the original bottom-arc default, whose baseline
      // was shifted beyond the outer ring. User-customized positions are kept.
      if (bottom?.offsetY === 155 && bottom.radiusX === 260 && bottom.radiusY === 260) {
        bottom.offsetY = 0;
      }
      return config;
    }

    // One-time migration from the original skeleton's schemaVersion 1 draft.
    const legacy = raw as Record<string, any>;
    let next = cloneSealConfig(defaultSealConfig);
    if (["circle", "ellipse", "square"].includes(legacy.shape)) next.shape = legacy.shape;
    if (typeof legacy.size === "number") next.canvas.exportWidth = legacy.size;
    if (typeof legacy.color === "string") next.color = legacy.color;
    if (typeof legacy.lineWidth === "number") next.border.width = legacy.lineWidth;
    if (typeof legacy.mainText === "string") next = updateLayer(next, layerIds.main, { content: legacy.mainText });
    if (typeof legacy.bottomText === "string") next = updateLayer(next, layerIds.bottom, { content: legacy.bottomText });
    if (typeof legacy.centerText === "string") next = updateLayer(next, layerIds.center, { content: legacy.centerText });
    if (legacy.texture && typeof legacy.texture === "object") {
      const apply = legacy.texture.applyTo;
      next.texture = {
        ...next.texture,
        ...legacy.texture,
        applyTo: Array.isArray(apply) ? apply : (["border", "text", "center"] as const).filter((key) => apply?.[key]),
      };
    }
    return next;
  } catch {
    return cloneSealConfig(defaultSealConfig);
  }
}

function templateConfig(id: string): SealConfig {
  let next = cloneSealConfig(defaultSealConfig);
  if (id === "ellipse") {
    next.shape = "ellipse";
    next.border.doubleLine = true;
    next.border.innerRing = true;
    next = updateLayer(next, layerIds.main, { content: "KALVIN TECHNOLOGY CO., LTD.", fontSize: 54, letterSpacing: 3 });
    next = updateLayer(next, layerIds.inner, { visible: true, fontSize: 30, letterSpacing: 2, radiusY: 215 });
    next = updateLayer(next, layerIds.header1, { visible: false });
  } else if (id === "finance") {
    next = updateLayer(next, layerIds.main, { content: "示例科技有限公司" });
    next = updateLayer(next, layerIds.center, { content: "财", fontSize: 170 });
    next = updateLayer(next, layerIds.header1, { content: "财务专用章", offsetY: 135 });
    next = updateLayer(next, layerIds.header2, { visible: true, content: "内部核算", offsetY: 195 });
  } else if (id === "contract") {
    next = updateLayer(next, layerIds.main, { content: "示例科技有限公司" });
    next = updateLayer(next, layerIds.center, { content: "合同", fontSize: 150 });
    next = updateLayer(next, layerIds.header1, { content: "合同专用章" });
  } else if (id === "personal") {
    next.shape = "square";
    next = updateLayer(next, layerIds.main, { content: "张三之印", fontSize: 150, offsetY: 180 });
    next = updateLayer(next, layerIds.bottom, { visible: false });
    next = updateLayer(next, layerIds.header1, { visible: false });
    next = updateLayer(next, layerIds.center, { visible: false });
  } else if (id === "aged") {
    next.texture = { ...next.texture, enabled: true, type: "ink", intensity: 35, density: 42, grainSize: 6, edgeWear: 24, scratchCount: 8, fade: 22, seed: 928341 };
  }
  return next;
}

export default function App() {
  const { value: config, update: updateConfig, endGroup: endHistoryGroup, undo, redo, canUndo, canRedo } = useSealHistory<SealConfig>(loadDraft(), 40);
  const [status, setStatus] = useState("草稿已加载");
  const [warning, setWarning] = useState("");
  const [configName, setConfigName] = useState("");
  const [savedConfigs, setSavedConfigs] = useState<SavedConfig[]>(() => loadList(savedKey));
  const [generationHistory, setGenerationHistory] = useState<GenerationHistory[]>(() => loadList(generationHistoryKey));
  const [templateQuery, setTemplateQuery] = useState("");
  const [templateCategory, setTemplateCategory] = useState("全部");
  const [favoriteTemplates, setFavoriteTemplates] = useState<string[]>(() => loadList(favoriteTemplatesKey));

  useEffect(() => {
    const timer = window.setTimeout(() => {
      localStorage.setItem(draftKey, JSON.stringify(config));
      setStatus("草稿已保存");
    }, 500);
    return () => window.clearTimeout(timer);
  }, [config]);

  useEffect(() => {
    const onWarning = () => setWarning("开发者工具限制只能提高复制成本，高清文件与商业字体仍由服务端保护。");
    window.addEventListener("security-warning", onWarning);
    return () => window.removeEventListener("security-warning", onWarning);
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
      const key = event.key.toLowerCase();
      if ((key === "y" || (key === "z" && event.shiftKey)) && canRedo) { event.preventDefault(); redo(); setStatus("已重做，正在保存…"); }
      else if (key === "z" && canUndo) { event.preventDefault(); undo(); setStatus("已撤销，正在保存…"); }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [canRedo, canUndo, redo, undo]);

  const changeConfig = (next: SealConfig, group?: string) => {
    setStatus("正在保存…");
    updateConfig(next, { group });
  };

  const recordGeneration = (format: GenerationHistory["format"]) => {
    const item: GenerationHistory = { id: crypto.randomUUID(), createdAt: new Date().toISOString(), format, name: configName.trim() || "未命名印章", config: cloneSealConfig(config) };
    setGenerationHistory((current) => {
      const next = [item, ...current].slice(0, 20);
      localStorage.setItem(generationHistoryKey, JSON.stringify(next));
      return next;
    });
  };

  const serializedPreview = () => {
    const source = document.getElementById("seal-preview-svg");
    if (!(source instanceof SVGSVGElement)) throw new Error("预览尚未就绪");
    const copy = source.cloneNode(true) as SVGSVGElement;
    copy.setAttribute("width", String(config.canvas.exportWidth));
    copy.setAttribute("height", String(config.canvas.exportWidth));
    if (!config.canvas.transparent) {
      const background = document.createElementNS("http://www.w3.org/2000/svg", "rect");
      background.setAttribute("width", "1000"); background.setAttribute("height", "1000"); background.setAttribute("fill", "white");
      copy.insertBefore(background, copy.firstChild);
    }
    return new XMLSerializer().serializeToString(copy);
  };

  const saveBlob = (blob: Blob, filename: string) => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a"); link.href = url; link.download = filename; link.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  const downloadPreviewSVG = () => {
    saveBlob(new Blob([serializedPreview()], { type: "image/svg+xml;charset=utf-8" }), "seal-preview.svg");
    recordGeneration("SVG"); setStatus("预览 SVG 已下载");
  };

  const downloadPreviewPNG = () => {
    const svgURL = URL.createObjectURL(new Blob([serializedPreview()], { type: "image/svg+xml;charset=utf-8" }));
    const image = new Image();
    image.onload = () => {
      const size = Math.min(2400, config.canvas.exportWidth);
      const canvas = document.createElement("canvas"); canvas.width = size; canvas.height = size;
      const context = canvas.getContext("2d");
      if (!context) return;
      if (!config.canvas.transparent) { context.fillStyle = "white"; context.fillRect(0, 0, size, size); }
      context.drawImage(image, 0, 0, size, size); URL.revokeObjectURL(svgURL);
      canvas.toBlob((blob) => blob && saveBlob(blob, "seal-preview.png"), "image/png");
      recordGeneration("PNG"); setStatus("预览 PNG 已下载");
    };
    image.onerror = () => { URL.revokeObjectURL(svgURL); setStatus("PNG 导出失败"); };
    image.src = svgURL;
  };

  const downloadServerSVG = async () => {
    try {
      setStatus("服务端生成中…");
      saveBlob(await renderOnServer(config), "seal-server.svg");
      recordGeneration("SERVER_SVG"); setStatus("服务端 SVG 已下载");
    } catch (error) { setStatus(error instanceof Error ? error.message : "生成失败"); }
  };

  const saveNamedConfig = () => {
    const name = configName.trim();
    if (!name) { setStatus("请先输入配置名称"); return; }
    const item: SavedConfig = { id: crypto.randomUUID(), name, updatedAt: new Date().toISOString(), config: cloneSealConfig(config) };
    setSavedConfigs((current) => { const next = [item, ...current].slice(0, 30); localStorage.setItem(savedKey, JSON.stringify(next)); return next; });
    setConfigName(""); setStatus("配置已保存");
  };

  const removeSavedConfig = (id: string) => setSavedConfigs((current) => { const next = current.filter((item) => item.id !== id); localStorage.setItem(savedKey, JSON.stringify(next)); return next; });

  const persistSavedConfigs = (updater: (current: SavedConfig[]) => SavedConfig[]) => setSavedConfigs((current) => {
    const next = updater(current);
    localStorage.setItem(savedKey, JSON.stringify(next));
    return next;
  });

  const copySavedConfig = (item: SavedConfig) => persistSavedConfigs((current) => [{ ...item, id: crypto.randomUUID(), name: `${item.name} 副本`, updatedAt: new Date().toISOString(), config: cloneSealConfig(item.config) }, ...current].slice(0, 30));

  const renameSavedConfig = (item: SavedConfig) => {
    const name = window.prompt("请输入新的配置名称", item.name)?.trim();
    if (!name) return;
    persistSavedConfigs((current) => current.map((value) => value.id === item.id ? { ...value, name, updatedAt: new Date().toISOString() } : value));
  };

  const toggleFavoriteTemplate = (id: string) => setFavoriteTemplates((current) => {
    const next = current.includes(id) ? current.filter((value) => value !== id) : [...current, id];
    localStorage.setItem(favoriteTemplatesKey, JSON.stringify(next));
    return next;
  });

  const visibleTemplates = templates.filter((template) => {
    const matchesCategory = templateCategory === "全部" || templateCategory === template.category || (templateCategory === "收藏" && favoriteTemplates.includes(template.id));
    const query = templateQuery.trim().toLowerCase();
    return matchesCategory && (!query || `${template.name} ${template.description} ${template.tags.join(" ")}`.toLowerCase().includes(query));
  });

  return (
    <div className="app-shell">
      <header className="topbar"><div className="brand"><span>印</span><div>印章生成平台 <small>V2</small></div></div><div className="top-actions"><span className="legal-pill">仅供合法用途</span><span className="top-status">{status}</span></div></header>
      <main className="workspace">
        {warning && <div className="security-warning">{warning}<button type="button" onClick={() => setWarning("")}>关闭</button></div>}
        <div className="page-heading"><div><h1>在线印章编辑器</h1><p>SVG 实时预览、确定性纹理、配置协议 v2 与服务端安全导出。</p></div><div className="history-toolbar" aria-label="编辑历史"><button type="button" onClick={() => { undo(); setStatus("已撤销，正在保存…"); }} disabled={!canUndo}>撤销</button><button type="button" onClick={() => { redo(); setStatus("已重做，正在保存…"); }} disabled={!canRedo}>重做</button><span>最多 40 步</span></div></div>

        <div className="editor-grid"><div className="controls">
          <EditorPanel config={config} onChange={changeConfig} onInteractionEnd={endHistoryGroup} />
          <TexturePanel config={config} onChange={changeConfig} onInteractionEnd={endHistoryGroup} />
          <section className="panel action-panel">
            <button className="primary" type="button" onClick={downloadServerSVG}>服务端生成 SVG</button>
            <button type="button" onClick={downloadPreviewPNG}>下载预览 PNG</button>
            <button type="button" onClick={downloadPreviewSVG}>下载预览 SVG</button>
            <button type="button" onClick={() => { endHistoryGroup(); updateConfig(cloneSealConfig(defaultSealConfig)); setStatus("已恢复默认，正在保存…"); }}>恢复默认</button>
          </section>
        </div><SealPreview config={config} /></div>

        <section className="panel library-panel"><div className="panel-heading"><h2>模板与我的配置</h2><small>模板加载与历史恢复都会创建撤销点</small></div>
          <div className="template-toolbar"><input type="search" value={templateQuery} placeholder="搜索模板或标签" aria-label="搜索模板" onChange={(event) => setTemplateQuery(event.target.value)} /><div role="group" aria-label="模板分类">{["全部", "企业", "财务", "个人", "创意", "收藏"].map((category) => <button type="button" className={templateCategory === category ? "active" : ""} key={category} onClick={() => setTemplateCategory(category)}>{category}</button>)}</div></div>
          <div className="template-grid">{visibleTemplates.length === 0 ? <p>没有匹配的模板。</p> : visibleTemplates.map((template) => <article className="template-card" key={template.id}><button type="button" className="template-load" onClick={() => { changeConfig(templateConfig(template.id)); setStatus(`已加载模板：${template.name}`); }}><span className="template-preview" aria-hidden="true">印</span><b>{template.name}</b><span>{template.description}</span><small>{template.category} · {template.tags.join(" / ")}</small></button><button type="button" className="template-favorite" aria-label={`${favoriteTemplates.includes(template.id) ? "取消收藏" : "收藏"}${template.name}`} onClick={() => toggleFavoriteTemplate(template.id)}>{favoriteTemplates.includes(template.id) ? "★ 已收藏" : "☆ 收藏"}</button></article>)}</div>
          <div className="save-row"><input value={configName} onChange={(event) => setConfigName(event.target.value)} maxLength={100} placeholder="输入配置名称" /><button type="button" onClick={saveNamedConfig}>保存当前配置</button></div>
          <div className="saved-list">{savedConfigs.length === 0 ? <p>暂无已保存配置。</p> : savedConfigs.map((item) => <div key={item.id}><span><b>{item.name}</b><small>{new Date(item.updatedAt).toLocaleString()}</small></span><span><button type="button" onClick={() => changeConfig(cloneSealConfig(item.config))}>加载</button><button type="button" onClick={() => copySavedConfig(item)}>复制</button><button type="button" onClick={() => renameSavedConfig(item)}>重命名</button><button type="button" className="danger" onClick={() => removeSavedConfig(item.id)}>删除</button></span></div>)}</div>
        </section>

        <section className="panel history-panel"><div className="panel-heading"><h2>生成历史</h2><button type="button" disabled={!generationHistory.length} onClick={() => { setGenerationHistory([]); localStorage.removeItem(generationHistoryKey); }}>清空</button></div>
          <div className="history-list">{generationHistory.length === 0 ? <p>下载预览或服务端文件后会记录在这里。</p> : generationHistory.map((item) => <button type="button" key={item.id} onClick={() => changeConfig(cloneSealConfig(item.config))}><span className="history-symbol">{item.config.layers.find((layer) => layer.id === layerIds.center)?.content || "印"}</span><span><b>{item.name}</b><small>{item.format} · {new Date(item.createdAt).toLocaleString()}</small></span></button>)}</div>
        </section>
        <CloudPanel config={config} onLoad={(next) => changeConfig(next)} />
        <AdminPanel />
      </main>
    </div>
  );
}
