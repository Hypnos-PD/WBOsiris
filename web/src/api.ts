// 规则服务的地址与少量共享调用。
//
// 优先级：设置里存的地址 > 桌面壳/环境显式注入 > 运行环境默认值。
// 默认值：
//   - 桌面客户端（Wails）：线上 https://sva.hypd.asia/wbo（进大厅需要登录）；
//     想离线玩就在设置里切“本机（离线）”，那是客户端进程内的规则服务。
//   - 本地开发（localhost / 127.0.0.1）：本机 wbo serve，默认端口 23215；
//   - 站点上的网页版：同源 /wbo，由 OpenResty 反代到规则服务。
export const API_BASE_OVERRIDE_KEY = "wbo-api-base";
// 大厅（联机）单独一份地址：默认就是线上 WBA 服务。
export const LOBBY_BASE_KEY = "wbo-lobby-base";

/** 线上服务（部署在 WBArts 站点的 /wbo/ 前缀下）。 */
export const PRODUCTION_API_BASE = "https://sva.hypd.asia/wbo";

type RuntimeWindow = {
  WBO_API_BASE?: unknown;
  WBO_LOCAL_API_BASE?: unknown;
};

function runtime(): RuntimeWindow {
  return window as unknown as RuntimeWindow;
}

/** 桌面客户端注入的“本机离线服务”地址；网页版里是空字符串。 */
export function localApiBase(): string {
  const value = runtime().WBO_LOCAL_API_BASE;
  return typeof value === "string" ? value.trim() : "";
}

function defaultApiBase(): string {
  // 桌面壳可以用 --api-base 显式指定默认地址（测试用）。
  const injected = runtime().WBO_API_BASE;
  if (typeof injected === "string" && injected.trim()) return injected.trim();
  const host = location.hostname;
  if (host === "localhost" || host === "127.0.0.1") return "http://127.0.0.1:23215";
  // Wails 的本地页面（wails:// 或 http://wails.localhost）不是站点，默认走线上服务。
  if (location.protocol.startsWith("wails") || host === "wails.localhost") return PRODUCTION_API_BASE;
  // 站点上的网页版（仅供开发调试）走同源 /wbo，由 OpenResty 反代到规则服务。
  return `${location.origin}/wbo`;
}

export function apiBase(): string {
  try {
    const saved = localStorage.getItem(API_BASE_OVERRIDE_KEY);
    if (saved !== null && saved.trim()) return saved.trim();
  } catch { /* 隐私模式下按默认值 */ }
  return import.meta.env.VITE_API_BASE || defaultApiBase();
}

/** 站点网页版（不是本地开发、也不是桌面壳）用同源 /wbo，等价于线上服务。 */
function hostedOnSite(): boolean {
  const host = location.hostname;
  if (host === "localhost" || host === "127.0.0.1" || host === "wails.localhost") return false;
  return !location.protocol.startsWith("wails");
}

/**
 * 大厅地址：默认线上 WBA（桌面客户端、本地开发、网页版都一样），
 * 设置里改了就用设置里的（存 localStorage 的 wbo-lobby-base，空字符串表示“用本机”）。
 */
export function lobbyApiBase(): string {
  try {
    const saved = localStorage.getItem(LOBBY_BASE_KEY);
    if (saved !== null) {
      const trimmed = saved.trim();
      return trimmed === "local" ? localApiBase() || PRODUCTION_API_BASE : trimmed || PRODUCTION_API_BASE;
    }
  } catch { /* 隐私模式下按默认值 */ }
  if (hostedOnSite()) return `${location.origin}/wbo`;
  return PRODUCTION_API_BASE;
}

export function setLobbyApiBase(value: string): void {
  try {
    if (value.trim() === PRODUCTION_API_BASE || value.trim() === "") localStorage.removeItem(LOBBY_BASE_KEY);
    else localStorage.setItem(LOBBY_BASE_KEY, value.trim());
  } catch { /* 隐私模式下不保存 */ }
}

/** 单人/卡组/回放用的地址：优先本机（桌面壳的进程内服务），否则跟大厅一致。 */
export function soloApiBase(): string {
  const local = localApiBase();
  if (local) return local;
  const host = location.hostname;
  if (host === "localhost" || host === "127.0.0.1") return "http://127.0.0.1:23215";
  return lobbyApiBase();
}

export function setApiBase(value: string): void {
  try {
    if (value.trim()) localStorage.setItem(API_BASE_OVERRIDE_KEY, value.trim());
    else localStorage.removeItem(API_BASE_OVERRIDE_KEY);
  } catch { /* 隐私模式下不保存 */ }
}

/** 卡片/卡组码/插图/单人模式用的地址（本机优先）。 */
export const API_BASE = soloApiBase();
/** 设置页里"当前生效的服务地址"（单人侧）。 */
export const apiBaseValue = apiBase;

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
