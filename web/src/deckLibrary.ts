export const LIBRARY_KEY = "wbo-deck-library";
export const LEGACY_KEY = "wbo-deck-cards";
export type SavedDeck = { id: string; name: string; cards: string[] };
export type DeckLibrary = { version: 1; activeId: string; decks: SavedDeck[] };
type Storage = Pick<globalThis.Storage, "getItem" | "setItem">;
export type LibrarySnapshot = {
  library: DeckLibrary;
  raw: string | null;
  legacy: string | null;
  fresh: boolean;
  error: string;
};

function uniqueId(): string {
  // getRandomValues also works on HTTP LAN origins, where randomUUID is unavailable.
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join("");
}
export function newDeck(name = "未命名牌组", cards: string[] = []): SavedDeck {
  return { id: uniqueId(), name, cards: [...cards] };
}
export function emptyLibrary(): DeckLibrary {
  const deck = newDeck();
  return { version: 1, activeId: deck.id, decks: [deck] };
}
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("格式应为 JSON 对象");
  return value as Record<string, unknown>;
}
export function deckName(value: unknown): string {
  if (typeof value !== "string" || !value.trim() || value.trim().length > 60) throw new Error("牌组名称须为 1–60 个字符");
  return value.trim();
}
function cardIds(value: unknown): string[] {
  // Keep invalid constructions editable, including unknown IDs and oversized legacy decks.
  if (!Array.isArray(value) || value.length > 1000) throw new Error("卡牌列表格式无效或超过 1000 张");
  return value.map((id) => {
    if ((typeof id !== "string" && typeof id !== "number") || !/^[1-9]\d*$/.test(String(id)) || !Number.isSafeInteger(Number(id))) throw new Error("卡牌编号必须为正整数");
    return String(id);
  });
}
export function parseLibrary(raw: string): DeckLibrary {
  const value = object(JSON.parse(raw));
  if (value.version !== 1) throw new Error("不支持的牌组库版本");
  if (!Array.isArray(value.decks) || !value.decks.length || value.decks.length > 100) throw new Error("牌组库须包含 1–100 副牌组");
  const ids = new Set<string>();
  const decks = value.decks.map((entry) => {
    const deck = object(entry);
    if (typeof deck.id !== "string" || !deck.id || deck.id.length > 100 || ids.has(deck.id)) throw new Error("牌组标识无效或重复");
    ids.add(deck.id);
    return { id: deck.id, name: deckName(deck.name), cards: cardIds(deck.cards) };
  });
  if (typeof value.activeId !== "string" || !ids.has(value.activeId)) throw new Error("选中的牌组不存在");
  return { version: 1, activeId: value.activeId, decks };
}
export function readLibrary(storage: Storage): LibrarySnapshot {
  let raw: string | null = null, legacy: string | null = null;
  try {
    raw = storage.getItem(LIBRARY_KEY);
    if (raw !== null) return { library: parseLibrary(raw), raw, legacy, fresh: false, error: "" };
    legacy = storage.getItem(LEGACY_KEY);
    const library = emptyLibrary();
    if (legacy !== null) {
      library.decks[0].name = "我的牌组";
      library.decks[0].cards = cardIds(JSON.parse(legacy));
    }
    return { library, raw, legacy, fresh: legacy === null, error: "" };
  } catch (error) {
    return { library: emptyLibrary(), raw, legacy, fresh: false, error: `无法读取牌组库，原始数据已保留：${error instanceof Error ? error.message : "存储不可用"}` };
  }
}
export function saveLibrary(storage: Storage, snapshot: LibrarySnapshot, library: DeckLibrary): LibrarySnapshot {
  if (snapshot.error) throw new Error(snapshot.error);
  // Check immediately before writing so a stale tab cannot overwrite a newer library.
  if (storage.getItem(LIBRARY_KEY) !== snapshot.raw || (snapshot.raw === null && storage.getItem(LEGACY_KEY) !== snapshot.legacy)) throw new Error("牌组库已在其他页面更新，本次修改未应用；请重新读取后操作");
  const raw = JSON.stringify(parseLibrary(JSON.stringify(library)));
  storage.setItem(LIBRARY_KEY, raw);
  return { library: JSON.parse(raw), raw, legacy: snapshot.legacy, fresh: false, error: "" };
}
export function addDeck(library: DeckLibrary, deck: SavedDeck): DeckLibrary {
  if (library.decks.length >= 100) throw new Error("最多保存 100 副牌组，请先导出并删除不需要的牌组");
  return { ...library, activeId: deck.id, decks: [...library.decks, deck] };
}
export function recoverLibrary(storage: Storage, snapshot: LibrarySnapshot): LibrarySnapshot {
  if (!snapshot.error || (snapshot.raw === null && snapshot.legacy === null)) throw new Error("请先恢复本地存储访问并重新读取");
  if (storage.getItem(LIBRARY_KEY) !== snapshot.raw || (snapshot.raw === null && storage.getItem(LEGACY_KEY) !== snapshot.legacy)) throw new Error("存储已变化，请重新读取牌组库");
  // Never replace damaged data unless its exact bytes have first been backed up.
  storage.setItem(`${LIBRARY_KEY}-backup-${uniqueId()}`, JSON.stringify({ library: snapshot.raw, legacy: snapshot.legacy }));
  return saveLibrary(storage, { ...snapshot, error: "" }, emptyLibrary());
}
export function importDeck(text: string): SavedDeck {
  if (text.length > 100_000) throw new Error("牌组文件超过 100 KB");
  const value = object(JSON.parse(text));
  if (value.format !== "wbo-deck" || value.version !== 1) throw new Error("需要 version 1 的 wbo-deck 文件");
  return newDeck(deckName(value.name), cardIds(value.cards));
}
export function exportDeck(deck: SavedDeck): string {
  return JSON.stringify({ format: "wbo-deck", version: 1, name: deck.name, cards: deck.cards.map(Number) }, null, 2) + "\n";
}
