// 规则服务的地址与少量共享调用。
//
// 默认值与 WBArts 的取法一致：
//   - 本地打开（localhost / 127.0.0.1）连本机的 wbo serve（默认端口 23215）；
//   - 否则走同源，由 WBArts 站点的反向代理把 /api/* 转到规则服务。
// 设置页可以覆盖（存在 localStorage 的 wbo-api-base），改动后重新载入页面生效。
export const API_BASE_OVERRIDE_KEY = "wbo-api-base";

function defaultApiBase(): string {
  const host = location.hostname;
  if (host === "localhost" || host === "127.0.0.1") return "http://127.0.0.1:23215";
  return "";
}

export function apiBase(): string {
  try {
    const saved = localStorage.getItem(API_BASE_OVERRIDE_KEY);
    if (saved !== null) return saved.trim();
  } catch { /* 隐私模式下按默认值 */ }
  return import.meta.env.VITE_API_BASE || defaultApiBase();
}

export function setApiBase(value: string): void {
  try {
    if (value.trim()) localStorage.setItem(API_BASE_OVERRIDE_KEY, value.trim());
    else localStorage.removeItem(API_BASE_OVERRIDE_KEY);
  } catch { /* 隐私模式下不保存 */ }
}

export const API_BASE = apiBase();

export type DeckCodeResult = {
  code?: string;
  cards?: number[];
  format?: string;
  class?: string;
  legal: boolean;
  problem?: string;
};

async function postDeckCode(payload: Record<string, unknown>): Promise<DeckCodeResult> {
  const response = await fetch(`${API_BASE}/api/deckcode`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw new Error((await response.text()).trim() || "卡组码服务不可用");
  return (await response.json()) as DeckCodeResult;
}

/** 把牌组编成官网 hash 式分享码（1.职业.卡牌…）。 */
export const encodeDeckCode = (cards: string[], format: string) =>
  postDeckCode({ cards: cards.map(Number), format });

/** 解析分享码或官方卡组链接。 */
export const decodeDeckCode = (code: string) => postDeckCode({ code });

/** 主界面插图（背景 + Spine 立绘）的清单。 */
export type IllustrationEntry = {
  id: string;
  name: string;
  type?: string;
  source: "bundled" | "wbarts" | string;
  skel?: string;
  atlas?: string;
  background?: string;
  thumbnail?: string;
  idleAnimation: string;
  tapAnimations?: string[];
  blendTimes?: number[];
  defaultMix: number;
  skeletonScale: number;
  prefabScale: number;
  aspectLayouts?: Record<string, { x?: number; y?: number; scale_x?: number; scale_y?: number }>;
};

export async function fetchIllustrations(): Promise<IllustrationEntry[]> {
  const bundled = await fetchBundledIllustrations();
  if (bundled.length) return bundled;
  const response = await fetch(`${API_BASE}/api/illustrations`, { cache: "no-store" });
  if (!response.ok) throw new Error("无法读取主界面插图列表");
  const payload = (await response.json()) as { items?: IllustrationEntry[] };
  const items = (payload.items || []).filter((item) => item.skel && item.atlas && item.background);
  // 内置的排在最前，其余按编号。
  return items.sort((a, b) => (a.source === "bundled" ? -1 : b.source === "bundled" ? 1 : a.id.localeCompare(b.id)));
}

/**
 * 打包进客户端的插图清单（scripts/bundle_illustrations.mjs 生成）。
 * 有它就不用请求规则服务，离线也能换主界面。
 */
async function fetchBundledIllustrations(): Promise<IllustrationEntry[]> {
  try {
    const response = await fetch("/assets/home/index.json", { cache: "no-store" });
    if (!response.ok) return [];
    const payload = (await response.json()) as { items?: IllustrationEntry[] };
    return (payload.items || [])
      .filter((item) => item.skel && item.atlas && item.background)
      .sort((a, b) => (a.id === "hi_1001" ? -1 : b.id === "hi_1001" ? 1 : a.id.localeCompare(b.id)));
  } catch {
    return [];
  }
}
