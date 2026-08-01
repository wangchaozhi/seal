import { useEffect, useState } from "react";
import { createCloudConfig, createGeneration, createOrder, currentUser, deleteCloudConfig, downloadGeneration, getGeneration, listCloudConfigs, listGenerations, listInvoices, listOrders, listRefunds, listSessions, login, logout, register, requestInvoice, requestRefund, retryGeneration, revokeSession, simulateOrderPayment, uploadCenterImage, type CloudConfig, type Generation, type Invoice, type Order, type RefundRequest, type Session, type User } from "../lib/api";
import { cloneSealConfig, layerIds, updateLayer, type SealConfig } from "../types/seal";

interface Props { config: SealConfig; onLoad: (config: SealConfig) => void }

function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a"); link.href = url; link.download = name; link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function CloudPanel({ config, onLoad }: Props) {
  const [user, setUser] = useState<User | null>(null);
  const [email, setEmail] = useState("demo@example.com");
  const [password, setPassword] = useState("");
  const [mfaCode, setMfaCode] = useState("");
  const [name, setName] = useState("");
  const [configs, setConfigs] = useState<CloudConfig[]>([]);
  const [generations, setGenerations] = useState<Generation[]>([]);
  const [orders, setOrders] = useState<Order[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [refunds, setRefunds] = useState<RefundRequest[]>([]);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [refundReason, setRefundReason] = useState("未使用，申请退款");
  const [invoiceTitle, setInvoiceTitle] = useState("");
  const [format, setFormat] = useState<"svg" | "png">("png");
  const [message, setMessage] = useState("");

  const refresh = async () => {
    const [nextConfigs, nextGenerations, nextOrders, nextSessions, nextRefunds, nextInvoices] = await Promise.all([listCloudConfigs(), listGenerations(), listOrders(), listSessions(), listRefunds(), listInvoices()]);
    setConfigs(nextConfigs); setGenerations(nextGenerations); setOrders(nextOrders); setSessions(nextSessions); setRefunds(nextRefunds); setInvoices(nextInvoices);
  };

  useEffect(() => { void currentUser().then((value) => { setUser(value); if (value) void refresh().catch((error) => setMessage(error.message)); }); }, []);

  useEffect(() => {
    const pending = generations.filter((item) => item.status === "queued" || item.status === "rendering");
    if (!pending.length) return;
    const timer = window.setInterval(() => {
      void Promise.all(pending.map((item) => getGeneration(item.id))).then((updates) => {
        setGenerations((current) => current.map((item) => updates.find((next) => next.id === item.id) ?? item));
      }).catch((error) => setMessage(error.message));
    }, 700);
    return () => window.clearInterval(timer);
  }, [generations]);

  const orderProduct = async (product: Order["product"], generationId?: string) => {
    try { const order = await createOrder(product, generationId); setOrders((current) => [order, ...current]); setMessage("订单已创建，请完成支付"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "订单创建失败"); }
  };

  const simulatePayment = async (order: Order) => {
    try { await simulateOrderPayment(order.id); const [nextUser] = await Promise.all([currentUser(), refresh()]); setUser(nextUser); setMessage("模拟支付成功，服务端权益已更新"); }
    catch (error) { setMessage(error instanceof Error ? error.message : "支付失败"); }
  };

  const uploadImage = async (file?: File) => {
    if (!file) return;
    try { setMessage("图片正在服务端安全重编码…"); const asset = await uploadCenterImage(file); onLoad(updateLayer(config, layerIds.center, { kind: "centerImage", assetId: asset.id, visible: true })); setMessage(`图片已上传并重编码：${asset.width}×${asset.height}`); }
    catch (error) { setMessage(error instanceof Error ? error.message : "上传失败"); }
  };

  if (!user) return (
    <section className="panel cloud-panel"><div className="panel-heading"><h2>云端工作台</h2><small>配置、任务与私有下载</small></div>
      <div className="login-row"><label><span>邮箱</span><input type="email" value={email} autoComplete="email" onChange={(event) => setEmail(event.target.value)} /></label><label><span>密码</span><input type="password" value={password} minLength={10} maxLength={128} autoComplete="current-password" onChange={(event) => setPassword(event.target.value)} /></label><label><span>管理员动态码（普通用户留空）</span><input value={mfaCode} inputMode="numeric" pattern="[0-9]{6}" maxLength={6} autoComplete="one-time-code" onChange={(event) => setMfaCode(event.target.value.replace(/\D/g, ""))} /></label><button type="button" onClick={() => void login(email, password, mfaCode).then((value) => { setUser(value); window.dispatchEvent(new Event("auth-changed")); setMessage("登录成功"); return refresh(); }).catch((error) => setMessage(error.message))}>登录</button><button type="button" onClick={() => void register(email, password, mfaCode).then((value) => { setUser(value); window.dispatchEvent(new Event("auth-changed")); setMessage("注册成功"); return refresh(); }).catch((error) => setMessage(error.message))}>注册</button></div>
      {message && <p className="panel-message">{message}</p>}
    </section>
  );

  return (
    <section className="panel cloud-panel"><div className="panel-heading"><h2>云端工作台</h2><div className="account-line"><span>{user.email} · {user.membershipLevel === "vip" ? "VIP" : "免费账户"}</span>{user.membershipLevel !== "vip" && <button type="button" onClick={() => void orderProduct("vip_monthly")}>开通 VIP ¥29.99</button>}<button type="button" onClick={() => void logout().then(() => { setUser(null); setConfigs([]); setGenerations([]); setOrders([]); window.dispatchEvent(new Event("auth-changed")); })}>退出</button></div></div>
      <div className="cloud-actions"><input value={name} maxLength={100} placeholder="云端配置名称" onChange={(event) => setName(event.target.value)} /><button type="button" onClick={() => void createCloudConfig(name.trim(), config).then((record) => { setConfigs((current) => [record, ...current]); setName(""); setMessage("云端配置已保存"); }).catch((error) => setMessage(error.message))}>保存到云端</button><label className="cloud-upload">上传中心图片<input type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => void uploadImage(event.target.files?.[0])} /></label><select value={format} aria-label="高清格式" onChange={(event) => setFormat(event.target.value as "svg" | "png")}><option value="png">PNG</option><option value="svg">SVG</option></select><button className="primary" type="button" onClick={() => void createGeneration(config, format).then((generation) => { setGenerations((current) => [generation, ...current.filter((item) => item.id !== generation.id)]); setMessage("高清任务已提交"); }).catch((error) => setMessage(error.message))}>生成高清任务</button></div>
      {message && <p className="panel-message">{message}</p>}
      <div className="cloud-grid"><div><h3>云端配置</h3><div className="compact-list">{configs.length === 0 ? <p>暂无云端配置。</p> : configs.map((item) => <div key={item.id}><span><b>{item.name}</b><small>{new Date(item.updatedAt).toLocaleString()}</small></span><span><button type="button" onClick={() => onLoad(cloneSealConfig(item.config))}>加载</button><button type="button" onClick={() => void deleteCloudConfig(item.id).then(() => setConfigs((current) => current.filter((value) => value.id !== item.id)))}>删除</button></span></div>)}</div></div>
        <div><h3>高清生成历史</h3><div className="compact-list">{generations.length === 0 ? <p>暂无高清任务。</p> : generations.map((item) => <div key={item.id}><span><b>{item.status === "queued" ? "排队中" : item.status === "rendering" ? "生成中" : item.status === "succeeded" ? "已完成" : "失败"}</b><small>{item.format.toUpperCase()} · {new Date(item.createdAt).toLocaleString()} · {item.watermark ? "带水印" : "无水印"}{item.failureReason ? ` · ${item.failureReason}` : ""}</small></span><span>{item.status === "succeeded" && <button type="button" onClick={() => void downloadGeneration(item.id).then((blob) => saveBlob(blob, `seal-hd.${item.format}`)).catch((error) => setMessage(error.message))}>签发令牌并下载</button>}{item.status === "succeeded" && item.watermark && <button type="button" onClick={() => void orderProduct("single_export", item.id)}>单次解锁 ¥1.99</button>}{item.status === "failed" && <button type="button" onClick={() => void retryGeneration(item.id).then((value) => setGenerations((current) => current.map((entry) => entry.id === value.id ? value : entry))).catch((error) => setMessage(error.message))}>重试</button>}</span></div>)}</div></div></div>
      <div className="orders-block"><h3>订单、退款与发票</h3><div className="cloud-actions"><input value={refundReason} maxLength={300} aria-label="退款原因" onChange={(event) => setRefundReason(event.target.value)} /><input value={invoiceTitle} maxLength={120} placeholder="发票抬头" aria-label="发票抬头" onChange={(event) => setInvoiceTitle(event.target.value)} /></div><div className="compact-list">{orders.length === 0 ? <p>暂无订单。</p> : orders.map((order) => { const refund = refunds.find((item) => item.orderId === order.id); const invoice = invoices.find((item) => item.orderId === order.id); return <div key={order.id}><span><b>{order.product === "vip_monthly" ? "VIP 月卡" : "单次高清解锁"}</b><small>{order.orderNo} · ¥{(order.amountCents / 100).toFixed(2)} · {order.status === "paid" ? "已支付" : order.status === "refunded" ? "已退款" : "待支付"}{refund ? ` · 退款${refund.status}` : ""}{invoice ? ` · 发票${invoice.status}` : ""}</small></span><span>{order.status === "pending" && import.meta.env.DEV && <button type="button" onClick={() => void simulatePayment(order)}>开发环境模拟支付</button>}{order.status === "paid" && !refund && <button type="button" onClick={() => void requestRefund(order.id, refundReason).then((value) => { setRefunds((current) => [value, ...current]); setMessage("退款申请已提交"); }).catch((error) => setMessage(error.message))}>申请退款</button>}{(order.status === "paid" || order.status === "refunded") && !invoice && <button type="button" disabled={!invoiceTitle.trim()} onClick={() => void requestInvoice(order.id, invoiceTitle.trim(), "", user.email).then((value) => { setInvoices((current) => [value, ...current]); setMessage("发票申请已提交"); }).catch((error) => setMessage(error.message))}>申请发票</button>}</span></div>; })}</div></div>
      <div className="orders-block"><h3>登录设备</h3><div className="compact-list">{sessions.map((session) => <div key={session.id}><span><b>设备会话</b><small>登录于 {new Date(session.createdAt).toLocaleString()} · 到期 {new Date(session.expiresAt).toLocaleString()}</small></span><button type="button" onClick={() => void revokeSession(session.id).then(() => { setSessions((current) => current.filter((item) => item.id !== session.id)); setMessage("设备会话已吊销"); void currentUser().then((value) => { if (!value) { setUser(null); window.dispatchEvent(new Event("auth-changed")); } }); }).catch((error) => setMessage(error.message))}>吊销</button></div>)}</div></div>
    </section>
  );
}
