// 规则服务的地址与少量共享调用。默认端口与 `wbo serve` 的默认监听一致（23215）。
export const API_BASE = import.meta.env.VITE_API_BASE || "http://127.0.0.1:23215";

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
