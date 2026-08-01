import type { SealConfig } from "../types/seal";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, "") ?? "/api/v1";

export interface User { id: string; email: string; membershipLevel: "free" | "vip"; status: string; role: "user" | "admin"; createdAt: string; vipExpiresAt?: string }
export interface CloudConfig { id: string; userId: string; name: string; config: SealConfig; createdAt: string; updatedAt: string }
export interface Generation { id: string; userId: string; config: SealConfig; rendererVersion: string; format: string; status: "queued" | "rendering" | "succeeded" | "failed"; watermark: boolean; failureReason?: string; createdAt: string; finishedAt?: string }
export interface Order { id: string; orderNo: string; userId: string; generationId?: string; product: "single_export" | "vip_monthly"; amountCents: number; status: "pending" | "paid" | "refunded"; createdAt: string; paidAt?: string }
export interface Asset { id: string; userId: string; mime: "image/png"; width: number; height: number; sha256: string; createdAt: string }
export interface Session { id: string; userId: string; userAgentHash: string; ipHash: string; expiresAt: string; createdAt: string }
export interface RefundRequest { id: string; orderId: string; userId: string; reason: string; status: "pending" | "approved" | "rejected"; createdAt: string; updatedAt: string }
export interface Invoice { id: string; orderId: string; userId: string; title: string; taxNumber?: string; email: string; status: "requested" | "issued"; createdAt: string; updatedAt: string }

async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const isFormData = typeof FormData !== "undefined" && init.body instanceof FormData;
  const response = await fetch(`${API_BASE_URL}${path}`, {
    credentials: "include",
    ...init,
    headers: { ...(init.body && !isFormData ? { "Content-Type": "application/json" } : {}), ...init.headers },
  });
  if (!response.ok) {
    const value = await response.json().catch(() => null) as { error?: { message?: string } } | null;
    throw new Error(value?.error?.message || `请求失败：${response.status}`);
  }
  return response;
}

export async function renderOnServer(config: SealConfig): Promise<Blob> {
  const response = await apiFetch("/seals/render", {
    method: "POST",
    headers: { "X-Client-Version": "web-0.2.0" },
    body: JSON.stringify(config),
  });
  return response.blob();
}

export async function login(email: string, password: string, mfaCode?: string): Promise<User> {
	const response = await apiFetch("/auth/login", { method: "POST", body: JSON.stringify({ email, password, ...(mfaCode ? { mfaCode } : {}) }) });
  return ((await response.json()) as { user: User }).user;
}

export async function register(email: string, password: string, mfaCode?: string): Promise<User> {
	const response = await apiFetch("/auth/register", { method: "POST", body: JSON.stringify({ email, password, ...(mfaCode ? { mfaCode } : {}) }) });
  return ((await response.json()) as { user: User }).user;
}

export async function logout(): Promise<void> { await apiFetch("/auth/logout", { method: "POST" }); }
export async function currentUser(): Promise<User | null> {
  try { const response = await apiFetch("/auth/me"); return ((await response.json()) as { user: User }).user; }
  catch { return null; }
}
export async function listSessions(): Promise<Session[]> { const response = await apiFetch("/auth/sessions"); return ((await response.json()) as { items: Session[] }).items; }
export async function revokeSession(id: string): Promise<void> { await apiFetch(`/auth/sessions/${encodeURIComponent(id)}`, { method: "DELETE" }); }

export async function listCloudConfigs(): Promise<CloudConfig[]> { const response = await apiFetch("/seal-configs"); return ((await response.json()) as { items: CloudConfig[] }).items; }
export async function createCloudConfig(name: string, config: SealConfig): Promise<CloudConfig> { const response = await apiFetch("/seal-configs", { method: "POST", body: JSON.stringify({ name, config }) }); return response.json(); }
export async function deleteCloudConfig(id: string): Promise<void> { await apiFetch(`/seal-configs/${encodeURIComponent(id)}`, { method: "DELETE" }); }

export async function listGenerations(): Promise<Generation[]> { const response = await apiFetch("/generations"); return ((await response.json()) as { items: Generation[] }).items; }
export async function createGeneration(config: SealConfig, format: "svg" | "png"): Promise<Generation> { const response = await apiFetch("/generations", { method: "POST", body: JSON.stringify({ format, config }) }); return response.json(); }
export async function getGeneration(id: string): Promise<Generation> { const response = await apiFetch(`/generations/${encodeURIComponent(id)}`); return response.json(); }
export async function retryGeneration(id: string): Promise<Generation> { const response = await apiFetch(`/generations/${encodeURIComponent(id)}/retry`, { method: "POST", body: "{}" }); return response.json(); }
export async function downloadGeneration(id: string): Promise<Blob> {
  const tokenResponse = await apiFetch(`/generations/${encodeURIComponent(id)}/download-token`, { method: "POST", body: "{}" });
  const token = (await tokenResponse.json()) as { downloadUrl: string };
  const path = token.downloadUrl.startsWith("/api/v1") ? token.downloadUrl.slice("/api/v1".length) : token.downloadUrl;
  return (await apiFetch(path)).blob();
}

export async function listOrders(): Promise<Order[]> { const response = await apiFetch("/orders"); return ((await response.json()) as { items: Order[] }).items; }
export async function createOrder(product: Order["product"], generationId?: string): Promise<Order> { const response = await apiFetch("/orders", { method: "POST", body: JSON.stringify({ product, ...(generationId ? { generationId } : {}) }) }); return response.json(); }
export async function simulateOrderPayment(id: string): Promise<Order> { const response = await apiFetch(`/orders/${encodeURIComponent(id)}/simulate-payment`, { method: "POST", body: "{}" }); return response.json(); }
export async function listRefunds(): Promise<RefundRequest[]> { const response = await apiFetch("/refunds"); return ((await response.json()) as { items: RefundRequest[] }).items; }
export async function requestRefund(orderId: string, reason: string): Promise<RefundRequest> { const response = await apiFetch(`/orders/${encodeURIComponent(orderId)}/refund`, { method: "POST", body: JSON.stringify({ reason }) }); return response.json(); }
export async function listInvoices(): Promise<Invoice[]> { const response = await apiFetch("/invoices"); return ((await response.json()) as { items: Invoice[] }).items; }
export async function requestInvoice(orderId: string, title: string, taxNumber: string, email: string): Promise<Invoice> { const response = await apiFetch(`/orders/${encodeURIComponent(orderId)}/invoice`, { method: "POST", body: JSON.stringify({ title, taxNumber, email }) }); return response.json(); }
export async function uploadCenterImage(file: File): Promise<Asset> { const form = new FormData(); form.append("file", file); const response = await apiFetch("/uploads/images", { method: "POST", body: form }); return ((await response.json()) as { asset: Asset }).asset; }
export function assetURL(id: string): string { return `${API_BASE_URL}/assets/${encodeURIComponent(id)}`; }
export interface AuditEvent { id: string; userId?: string; type: string; targetId?: string; details?: Record<string, unknown>; createdAt: string }
export async function adminUsers(): Promise<User[]> { const response=await apiFetch("/admin/users");return ((await response.json()) as {items:User[]}).items; }
export async function adminOrders(): Promise<Order[]> { const response=await apiFetch("/admin/orders");return ((await response.json()) as {items:Order[]}).items; }
export async function adminGenerations(): Promise<Generation[]> { const response=await apiFetch("/admin/generations");return ((await response.json()) as {items:Generation[]}).items; }
export async function adminAuditEvents(): Promise<AuditEvent[]> { const response=await apiFetch("/admin/audit-events");return ((await response.json()) as {items:AuditEvent[]}).items; }
export async function adminRefunds(): Promise<RefundRequest[]> { const response=await apiFetch("/admin/refunds");return ((await response.json()) as {items:RefundRequest[]}).items; }
export async function adminDecideRefund(id:string,status:"approved"|"rejected"):Promise<RefundRequest>{const response=await apiFetch(`/admin/refunds/${encodeURIComponent(id)}`,{method:"PUT",body:JSON.stringify({status})});return response.json();}
export async function adminInvoices(): Promise<Invoice[]> { const response=await apiFetch("/admin/invoices");return ((await response.json()) as {items:Invoice[]}).items; }
export async function adminIssueInvoice(id:string):Promise<Invoice>{const response=await apiFetch(`/admin/invoices/${encodeURIComponent(id)}`,{method:"PUT",body:"{}"});return response.json();}
export async function adminUpdateUser(id:string,patch:{status?:"active"|"banned";membershipLevel?:"free"|"vip"}):Promise<User>{const response=await apiFetch(`/admin/users/${encodeURIComponent(id)}`,{method:"PUT",body:JSON.stringify(patch)});return ((await response.json()) as {user:User}).user;}
