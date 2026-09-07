import artIndex from "./generated/card-art.json";

export type CatalogCard = {
  counters?: Record<string, number>;
  id: number;
  name: string;
  text: string;
  cardType: string;
  class: string;
  rarity: string;
  pack: number;
  cost: number;
  attack?: number;
  life?: number;
  traits: string[];
  deckLegal: boolean;
};

export const classNames: Record<string, string> = {
  neutral: "中立", forestcraft: "精灵", swordcraft: "皇家护卫", runecraft: "巫师",
  dragoncraft: "龙族", abysscraft: "梦魇", havencraft: "主教", portalcraft: "超越者",
};
export const typeNames: Record<string, "随从" | "法术" | "护符" | "纹章"> = {
  follower: "随从", spell: "法术", amulet: "护符", crest: "纹章",
};
const illustrations: Record<string, { base: string; evolved?: string }> = artIndex;
export const cardArt = (id: number, evolved = false) => evolved ? illustrations[id]?.evolved ?? illustrations[id]?.base : illustrations[id]?.base;
export const hasEvolvedArt = (id: number) => !!illustrations[id]?.evolved;

export function cardText(markup: string): string {
  const document = new DOMParser().parseFromString(markup, "text/html");
  document.querySelectorAll("hr, br").forEach((element) => element.replaceWith("\n"));
  return document.body.textContent?.trim() || "";
}

export function readDeck(): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem("wbo-deck-cards") || "[]");
    return Array.isArray(value) ? value.filter((id) => typeof id === "string" || typeof id === "number").map(String) : [];
  } catch { return []; }
}

export function deckProblems(deck: string[], cards: CatalogCard[]): string[] {
  const byId = new Map(cards.map((card) => [String(card.id), card]));
  const counts = new Map<string, number>();
  const classes = new Set<string>();
  const errors: string[] = [];
  if (deck.length !== 40) errors.push(`卡牌数量 ${deck.length}/40`);
  for (const id of deck) counts.set(id, (counts.get(id) || 0) + 1);
  for (const [id, count] of counts) {
    const card = byId.get(id);
    if (!card) { errors.push(`卡牌 ${id} 不在当前卡池中`); continue; }
    if (!card.deckLegal) errors.push(`${card.name} 无法编入牌组`);
    if (count > 3) errors.push(`${card.name} 超过 3 张`);
    if (card.class !== "neutral") classes.add(card.class);
  }
  if (classes.size > 1) errors.push("牌组包含多个职业");
  return errors;
}
