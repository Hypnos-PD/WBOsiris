import type { MatchAuth } from "./matchConnection.ts";

export type SavedMatch = MatchAuth & { updatedAt: number };
type ReadStorage = Pick<Storage, "length" | "key" | "getItem">;
export function matchKey(auth: MatchAuth): string { return `wbo-match-${auth.id}-${auth.side}`; }
function decode(key: string, raw: string | null): SavedMatch | null {
  try {
    const value = JSON.parse(raw || "null");
    if (!value || typeof value.id !== "string" || !/^[a-f0-9]{12}$/.test(value.id) ||
      (value.side !== "own" && value.side !== "oppo") || typeof value.token !== "string" ||
      !/^[a-f0-9]{48}$/.test(value.token) || matchKey(value) !== key) return null;
    return { id: value.id, side: value.side, token: value.token, updatedAt: Number.isSafeInteger(value.updatedAt) && value.updatedAt >= 0 ? value.updatedAt : 0 };
  } catch { return null; }
}
export function readSavedMatches(storage: ReadStorage): SavedMatch[] {
  const rooms: SavedMatch[] = [];
  for (let i = 0; i < storage.length; i++) {
    const key = storage.key(i);
    if (!key?.startsWith("wbo-match-")) continue;
    const room = decode(key, storage.getItem(key));
    if (room) rooms.push(room);
  }
  return rooms.sort((a, b) => b.updatedAt - a.updatedAt || a.id.localeCompare(b.id) || a.side.localeCompare(b.side));
}
export function readMatchAuth(storage: Pick<Storage, "getItem">, id: string, side?: string | null): SavedMatch | null {
  const sides = side === "own" || side === "oppo" ? [side] : ["own", "oppo"];
  for (const candidate of sides) {
    const key = matchKey({ id, side: candidate, token: "" });
    const auth = decode(key, storage.getItem(key));
    if (auth) return auth;
  }
  return null;
}
export function saveMatchAuth(storage: Pick<Storage, "setItem">, auth: MatchAuth): void {
  const value = { id: auth.id, side: auth.side, token: auth.token, updatedAt: Date.now() };
  const key = matchKey(auth), raw = JSON.stringify(value);
  if (!decode(key, raw)) throw new Error("房间凭据无效");
  storage.setItem(key, raw);
}
export function invitationURL(base: string, id: string, code: string): string {
  const url = new URL(base);
  url.search = "";
  url.hash = "";
  url.searchParams.set("match", id);
  url.searchParams.set("join", code);
  return url.toString();
}
