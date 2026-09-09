import type { Entity, GameState, PlayerView, RuntimeEvent, RuntimeTarget } from "./gameTypes";

export type ReplayFrame = { revision: number; eventCount?: number; state: GameState };
export type ReplayRecord = {
  id: string;
  matchId: string;
  viewer?: string;
  turn: number;
  events: string[];
  rawEvents?: RuntimeEvent[];
  frames?: ReplayFrame[];
  winner?: string;
  updatedAt: number;
};
export type ReplayResponse = { matchId: string; events?: RuntimeEvent[]; state: GameState; frames?: ReplayFrame[] };
type Names = Record<string, { name: string }>;
const zones = (player: PlayerView): Entity[] => [
  ...(player.hand || []), ...(player.field || []), ...(player.graveyard || []),
  ...(player.resolving || []), ...(player.banished || []), ...(player.destroyed || []),
  ...(player.crests || []),
];

export function eventLabel(event: RuntimeEvent, remote: { state: GameState }, catalog: Names, previous?: GameState): string {
  const state = remote.state;
  const sideLabel = (side?: string) => !side ? "" : side === (state.viewer || "own") ? "我方" : "对手";
  const entities = [...zones(state.own), ...zones(state.oppo), ...(previous ? [...zones(previous.own), ...zones(previous.oppo)] : [])];
  const nameFor = (target?: RuntimeTarget): string => {
    if (!target) return "卡牌";
    if ((target.kind || target.Kind) === "leader") return `${sideLabel(target.side || target.Side)}主战者`;
    const id = target.instanceId || target.InstanceID;
    const cardID = target.cardId || target.CardID || entities.find((item) => item.instanceId === id)?.cardId;
    return cardID ? catalog[String(cardID)]?.name || `卡牌 ${cardID}` : "卡牌";
  };
  const kind = event.kind || event.Kind || "event";
  const actual = event.actual ?? event.Actual;
  const side = sideLabel(event.side || event.Side);
  const target = event.subject || event.Subject || event.target || event.Target || event;
  const name = nameFor(target);
  const crest = entities.find((item) => item.instanceId === (event.instanceId || event.InstanceID) && item.cardType === "crest");
  const crestLabel = crest?.crestLocales?.chs?.name || `纹章：${name}`;
  if (kind === "crest_gained") return `${side}获得 ${crestLabel}`;
  if (kind === "crest_countdown") return `${crestLabel} 吟唱 ${event.count ?? event.Count ?? 0}`;
  if (kind === "crest_destroyed") return `${crestLabel} 被破坏`;
  if (kind === "card_fused") return (event.subject || event.Subject) ? `${name} 融合 ${event.count ?? event.Count ?? 0} 张材料` : `${side}融合 ${event.count ?? event.Count ?? 0} 张材料`;
  if (kind === "card_transformed") return (event.subject || event.Subject) ? `${name} 变身为 ${nameFor(event.target || event.Target)}` : `${side}卡牌变身`;
  if (kind === "pp_restored") return `${side}回复 ${actual ?? 0} 点能量`;
  if (kind === "deck_replaced") return `${side}牌组替换为 ${event.count ?? event.Count ?? 0} 张卡牌`;
  if (kind === "leader_max_life_set") return `${side}主战者生命上限变为 ${event.count ?? event.Count ?? 0}`;
  if (kind === "attacked") return `${nameFor(event.attacker || event.Attacker)} 攻击 ${nameFor(event.defender || event.Defender)}`;
  if (kind === "damaged") return `${name} 受到 ${actual ?? 0} 点伤害`;
  if (kind === "healed") return `${name} 回复 ${actual ?? 0} 点生命`;
  if (kind === "follower_summoned" || kind === "amulet_summoned") return `${name} 入场`;
  if (kind === "amulet_engaged") return `${name} 启动`;
  if (kind === "follower_left") return `${name} 离场`;
  if (kind === "evolved") return `${name} 完成进化`;
  if (kind === "super_evolved") return `${name} 完成超进化`;
  if (kind === "card_drawn") return `${side}抽取 ${event.count ?? event.Count ?? 1} 张卡牌`;
  if (kind === "destroyed") return `${name} 被破坏`;
  if (kind === "card_discarded") return `${side}舍弃 ${name}`;
  if (kind === "turn_started") return `${side}回合开始`;
  if (kind === "turn_ended") return `${side}回合结束`;
  if (kind === "game_ended") return event.reason === "concede" ? `${side}认输，对手胜利` : state.winner === "draw" ? "对局平局" : `${side}胜利`;
  if (kind === "zone_moved") {
    const names: Record<string, string> = { hand: "手牌", field: "战场", graveyard: "墓场", deck: "牌堆", banished: "消失区", resolving: "结算区" };
    const to = event.to || event.To || "";
    return `${name} 移至${names[to] || to || "其他区域"}`;
  }
  return kind.replaceAll("_", " ");
}

export function frameEvents(record: ReplayRecord, index: number): RuntimeEvent[] | undefined {
  const frames = record.frames || [];
  const end = frames[index]?.eventCount;
  const start = index === 0 ? 0 : frames[index - 1]?.eventCount;
  if (!record.rawEvents || end === undefined || start === undefined || !Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < 0 || end < start || end > record.rawEvents.length) return undefined;
  return record.rawEvents.slice(start, end);
}

export function mergeReplay(current: ReplayRecord[], data: ReplayResponse, catalog: Names): ReplayRecord[] {
  const viewer = data.state.viewer || "own";
  const id = `${data.matchId}-${viewer}`;
  const existing = current.find((record) => record.id === id);
  const frames = data.frames || [];
  const rawEvents = data.events || [];
  const record: ReplayRecord = {
    id, matchId: data.matchId, viewer, turn: data.state.turn.number,
    events: [], rawEvents, frames, winner: data.state.winner, updatedAt: Date.now(),
  };
  record.events = frames.flatMap((frame, index) => (frameEvents(record, index) || []).map((event) => eventLabel(event, { state: frame.state }, catalog, frames[index - 1]?.state)));
  if (!record.events.length) record.events = rawEvents.map((event) => eventLabel(event, data, catalog));
  if (existing && existing.frames?.length === frames.length && existing.frames?.at(-1)?.revision === frames.at(-1)?.revision && existing.rawEvents?.length === rawEvents.length && existing.events.length === record.events.length && existing.events.every((label, index) => label === record.events[index])) return current;
  return [record, ...current.filter((item) => item.id !== id)].slice(0, 20);
}

function validState(value: unknown): value is GameState {
  if (!value || typeof value !== "object") return false;
  const state = value as GameState;
  const validEntities = (items: unknown) => items === undefined || items === null || Array.isArray(items) && items.every((item) => item && typeof item.instanceId === "string" && typeof item.cardId === "number");
  return !!state.turn && typeof state.turn.number === "number" && [state.own, state.oppo].every((player) => player &&
    typeof player.leaderLife === "number" && [player.field, player.hand, player.graveyard, player.banished, player.destroyed, player.resolving, player.crests].every(validEntities));
}

export function readReplays(storage?: Pick<Storage, "getItem">): ReplayRecord[] {
  try {
    const parsed: unknown = JSON.parse((storage || localStorage).getItem("wbo-replays") || "[]");
    if (!Array.isArray(parsed)) return [];
    return parsed.flatMap((item, index): ReplayRecord[] => {
      if (typeof item === "string") return [{ id: `legacy-${index}`, matchId: "本地记录", turn: 0, events: [item], updatedAt: 0 }];
      if (!item || typeof item.id !== "string" || typeof item.matchId !== "string" || !Array.isArray(item.events)) return [];
      return [{ ...item, turn: typeof item.turn === "number" ? item.turn : 0,
        events: item.events.filter((event: unknown) => typeof event === "string"),
        rawEvents: Array.isArray(item.rawEvents) && item.rawEvents.every((event: RuntimeEvent) => event && typeof (event.kind || event.Kind) === "string") ? item.rawEvents : undefined,
        frames: Array.isArray(item.frames) ? item.frames.filter((frame: ReplayFrame) => frame && Number.isInteger(frame.revision) && validState(frame.state)) : [],
      }];
    }).slice(0, 20);
  } catch { return []; }
}

export function persistReplays(records: ReplayRecord[], storage?: Pick<Storage, "setItem">): boolean {
  try { (storage || localStorage).setItem("wbo-replays", JSON.stringify(records)); return true; }
  catch { return false; }
}
