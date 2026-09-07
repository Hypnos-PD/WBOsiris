import {
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { CircleHelp, History, Menu, Shield, Sparkles, X, Home, Swords, Layers, Film, DoorOpen, Plus, Play, Combine, Zap, ChevronUp, ChevronDown } from "lucide-react";
import { StatusEffects, type StatusEffect } from "./StatusEffects";
import { DeckBuilder } from "./DeckBuilder";
import { cardArt, cardText, classNames, deckProblems, readDeck, typeNames, type CatalogCard } from "./decks";
import { CardArt } from "./CardArt";
import type { Entity, ChoiceCandidate, Remote } from "./gameTypes";
import { eventLabel, readReplays, mergeReplay, persistReplays, type ReplayResponse } from "./replays";
import { ReplayViewer } from "./ReplayViewer";
import { FusionDetails } from "./FusionDetails";
import { CounterValues } from "./CounterValues";
import { MatchConnection, type ConnectionStatus } from "./matchConnection";

const API_BASE = import.meta.env.VITE_API_BASE || "http://127.0.0.1:8080";
const validPages = new Set(["home", "battle", "decks", "replays", "rooms"]);
const initialPage = (): "home" | "battle" | "decks" | "replays" | "rooms" => {
  const params = new URLSearchParams(location.search);
  const page = params.get("page");
  if (page && validPages.has(page)) return page as ReturnType<typeof initialPage>;
  return params.has("match") ? "battle" : "home";
};

type Card = {
  counters?: Record<string, number>;
  fusion?: Entity["fusion"];
  id: string;
  instanceId?: string;
  name: string;
  cost: number;
  attack?: number;
  life?: number;
  text: string;
  art: string | undefined;
  keywords?: string[];
  evolved?: boolean;
  superEvolved?: boolean;
  earthsigil?: number;
  countdown?: number;
  damageReduction?: number;
  attackLimit?: number;
  attacksUsed?: number;
  summoningSick?: boolean;
  type: "随从" | "法术" | "护符";
};
type RoomSummary = { id: string; waiting: boolean };

const fallbackCatalog: Record<string, Omit<Card, "id" | "instanceId">> = {
  "10001110": {
    name: "不屈的剑斗士",
    cost: 2,
    attack: 2,
    life: 2,
    type: "随从",
    text: "爆能强化 4：本随从 +3/+3。",
    art: cardArt(10001110),
  },
  "10012110": {
    name: "冒险精灵·小梅",
    cost: 1,
    attack: 1,
    life: 1,
    type: "随从",
    text: "",
    art: cardArt(10012110),
  },
  "10001120": {
    name: "叮当天使·莉亚",
    cost: 2,
    attack: 0,
    life: 2,
    type: "随从",
    text: "守护\n谢幕曲：抽取1张卡牌。\n进化时：抽取1张卡牌。",
    art: cardArt(10001120),
    keywords: ["守护"],
  },
  "10002110": {
    name: "煌响使者·亨莉雅妲",
    cost: 3,
    attack: 3,
    life: 3,
    type: "随从",
    text: "进化时：回复自己的主战者2点生命值。",
    art: cardArt(10002110),
  },
  "10011130": {
    name: "温厚的树精",
    cost: 4,
    attack: 4,
    life: 4,
    type: "随从",
    text: "入场曲：连击3，本随从进化。",
    art: cardArt(10011130),
  },
};
const fallbackHand: Card[] = [
  { id: "fallback-10001110", ...fallbackCatalog["10001110"] },
  { id: "fallback-10001120", ...fallbackCatalog["10001120"] },
  { id: "fallback-10002110", ...fallbackCatalog["10002110"] },
];
const displayCardFor = (entity: Entity, catalog = fallbackCatalog): Card => {
  const base = catalog[String(entity.cardId)] ?? {
    name: "未知卡牌",
    cost: 0,
    type: typeNames[entity.cardType] || "随从",
    text: "",
    art: undefined,
  };
  return {
    id: String(entity.cardId),
    instanceId: entity.instanceId,
    ...base,
    art: cardArt(entity.cardId, entity.evolved || entity.superEvolved),
    cost: entity.cost ?? base.cost,
    attack: entity.cardType === "follower" ? (entity.attack ?? base.attack) : undefined,
    life: entity.cardType === "follower" ? (entity.life ?? base.life) : undefined,
    keywords: entity.keywords ?? base.keywords,
    evolved: entity.evolved,
    superEvolved: entity.superEvolved,
    earthsigil: entity.earthsigil,
    countdown: entity.countdown,
    damageReduction: entity.damageReduction,
    attackLimit: entity.attackLimit,
    attacksUsed: entity.attacksUsed,
    summoningSick: entity.summoningSick,
    fusion: entity.fusion,
    counters: entity.counters,
  };
};

export function App() {
  const [catalog, setCatalog] = useState(fallbackCatalog);
  const [catalogCards, setCatalogCards] = useState<CatalogCard[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  const [catalogError, setCatalogError] = useState("");
  const [catalogVersion, setCatalogVersion] = useState(0);
  const [practiceCards, setPracticeCards] = useState<string[]>([]);
  const [requestError, setRequestError] = useState("");
  const [roomBusy, setRoomBusy] = useState(false);
  const roomRequest = useRef(false);
  const cardFor = (entity: Entity) => displayCardFor(entity, catalog);
  const [activePage, setActivePage] = useState<"home" | "battle" | "decks" | "replays" | "rooms">(initialPage);
  const [remote, setRemote] = useState<Remote | null>(null);
  const connection = useRef<MatchConnection | null>(null);
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>("connecting");
  const matchCallbacks = useRef({ receive: (_data: Remote) => {} });
  const [hand, setHand] = useState<Card[]>(fallbackHand);
  const [selected, setSelected] = useState<Card | null>(null);
  const [message, setMessage] = useState("请选择要进行的操作");
  const [events, setEvents] = useState(["你的回合开始"]);
  const [savedReplays, setSavedReplays] = useState(readReplays);
  const [replayError, setReplayError] = useState("");
  const [replayStorageError, setReplayStorageError] = useState(false);
  const [replayRetry, setReplayRetry] = useState(0);
  const [replaySelection, setReplaySelection] = useState<string | null>(null);
  const [deckCards, setDeckCards] = useState<string[]>(readDeck);
  const deckInitialized = useRef(false);
  const deckErrors = catalogCards.length ? deckProblems(deckCards, catalogCards) : ["卡池尚未就绪"];
  const deckClass = catalogCards.find((card) => card.class !== "neutral" && deckCards.includes(String(card.id)))?.class || "neutral";
  const changeDeck = (cards: string[]) => {
    setDeckCards(cards);
    try { localStorage.setItem("wbo-deck-cards", JSON.stringify(cards)); }
    catch { setRequestError("无法保存牌组到本地存储"); }
  };
  const [roomList, setRoomList] = useState<RoomSummary[]>([]);
  const [joinRoomId, setJoinRoomId] = useState("");
  const [joinRoomCode, setJoinRoomCode] = useState("");
  const [demoPP, setDemoPP] = useState(5);
  const [attacker, setAttacker] = useState<string | null>(null);
  const [choiceSelection, setChoiceSelection] = useState<string[]>([]);
  const [choiceOption, setChoiceOption] = useState<number | null>(null);
  const [feedback, setFeedback] = useState<{
    attacker?: string;
    defender?: string;
    leader?: boolean;
  }>({});
  const [matchAuth, setMatchAuth] = useState<{
    id: string;
    token: string;
    side: string;
  } | null>(null);
  const [joinCode, setJoinCode] = useState("");
  const started = useRef(false);
  const [handExpanded, setHandExpanded] = useState(false);
  const [leftPanel, setLeftPanel] = useState<"history" | "card" | null>(null);
  const [dragGuide, setDragGuide] = useState<{
    kind: "attack" | "evolve" | "superevolve";
    source?: string;
    x1: number;
    y1: number;
    x2: number;
    y2: number;
  } | null>(null);
  const [mulliganSelection, setMulliganSelection] = useState<string[]>([]);
  const fillPracticeDeck = () => {
    if (practiceCards.length) changeDeck(practiceCards);
  };

  useEffect(() => {
    const controller = new AbortController();
    setCatalogLoading(true);
    setCatalogError("");
    fetch(`${API_BASE}/api/cards`, { signal: controller.signal })
      .then((response) => { if (!response.ok) throw new Error(); return response.json(); })
      .then((data: { cards: CatalogCard[]; practiceDeck: number[] }) => {
        const cards = data.cards.map((card) => ({ ...card, text: cardText(card.text) }));
        setCatalogCards(cards);
        const next = { ...fallbackCatalog };
        for (const card of cards) next[String(card.id)] = { name: card.name, text: card.text, cost: card.cost, attack: card.attack, life: card.life, type: typeNames[card.cardType], art: cardArt(card.id), counters: card.counters };
        setCatalog(next);
        setHand((hand) => hand.map((card) => next[card.id] ? { ...card, name: next[card.id].name, text: next[card.id].text, art: next[card.id].art, type: next[card.id].type } : card));
        const practice = data.practiceDeck.map(String);
        setPracticeCards(practice);
        if (!deckInitialized.current) {
          deckInitialized.current = true;
          try { if (localStorage.getItem("wbo-deck-cards") === null) changeDeck(practice); }
          catch { setRequestError("无法读取本地牌组"); }
        }
      })
      .catch((error) => { if (error.name !== "AbortError") setCatalogError("无法加载卡池，请确认规则服务已连接"); })
      .finally(() => { if (!controller.signal.aborted) setCatalogLoading(false); });
    return () => controller.abort();
  }, [catalogVersion]);

  const acceptMatch = (data: Remote, previousToken = "") => {
    const token = data.playerToken || previousToken;
    if (!data.matchId || !token || !data.side) return;
    const auth = { id: data.matchId, token, side: data.side };
    connection.current?.stop();
    connection.current = null;
    try { localStorage.setItem(`wbo-match-${auth.id}-${auth.side}`, JSON.stringify(auth)); }
    catch { setRequestError("无法保存房间凭据，刷新后将无法恢复对局"); }
    setJoinCode(data.joinCode || "");
    setMatchAuth(auth);
    setRemote(data);
    sync(data);
  };
  const createMatch = async () => {
    if (roomRequest.current) return;
    if (deckErrors.length) { setRequestError(deckErrors[0]); navigate("decks"); return; }
    roomRequest.current = true;
    setRoomBusy(true);
    setRequestError("");
    try {
      const response = await fetch(`${API_BASE}/api/matches`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ deck: deckCards.map(Number) }),
      });
      if (!response.ok) throw new Error(await response.text());
      const data: Remote = await response.json();
      if (!data.matchId || !data.playerToken || data.side !== "own" || !data.state) throw new Error("房间响应无效");
      history.replaceState(null, "", `?match=${data.matchId}`);
      acceptMatch(data);
      setActivePage("battle");
    } catch (error) { setRequestError(error instanceof Error ? error.message : "无法连接对局服务"); }
    finally { roomRequest.current = false; setRoomBusy(false); }
  };
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const params = new URLSearchParams(location.search);
    const id = params.get("match");
    const code = params.get("join");
    let saved: string | null = null;
    try { saved = id ? localStorage.getItem(`wbo-match-${id}-own`) || localStorage.getItem(`wbo-match-${id}-oppo`) : null; }
    catch { setRequestError("无法读取本地房间凭据"); }
    if (id && saved) {
      try {
        const auth = JSON.parse(saved);
        if (auth.id === id && typeof auth.token === "string" && (auth.side === "own" || auth.side === "oppo")) { setMatchAuth(auth); return; }
      } catch { setRequestError("本地房间凭据无效"); }
    }
    if (id && code) {
      setJoinRoomId(id);
      setJoinRoomCode(code);
      setActivePage("rooms");
    }
  }, []);
  useEffect(() => {
    if (!matchAuth) return;
    const client = new MatchConnection({
      base: API_BASE,
      auth: matchAuth,
      onRemote: (data) => matchCallbacks.current.receive(data),
      onStatus: setConnectionStatus,
      onError: setRequestError,
    });
    connection.current = client;
    client.start();
    return () => { client.stop(); if (connection.current === client) connection.current = null; };
  }, [matchAuth]);
  useEffect(() => {
    if (activePage !== "rooms") return;
    const controller = new AbortController();
    const refreshRooms = () => fetch(`${API_BASE}/api/matches`, { signal: controller.signal }).then((r) => r.ok ? r.json() : []).then(setRoomList).catch((error) => { if (error?.name !== "AbortError") setRoomList([]); });
    refreshRooms();
    const timer = window.setInterval(refreshRooms, 5000);
    return () => { window.clearInterval(timer); controller.abort(); };
  }, [activePage, matchAuth, remote?.waiting]);
  useEffect(() => {
    if (!matchAuth) return;
    const controller = new AbortController();
    fetch(`${API_BASE}/api/matches/${matchAuth.id}/replay`, {
      headers: { Authorization: `Bearer ${matchAuth.token}` },
      signal: controller.signal,
    })
      .then((response) => response.ok ? response.json() : Promise.reject())
      .then((data: ReplayResponse) => {
        if (controller.signal.aborted) return;
        setReplayError("");
        setSavedReplays((current) => mergeReplay(current, data, catalog));
      })
      .catch(() => { if (!controller.signal.aborted) setReplayError("录像同步失败"); });
    return () => controller.abort();
  }, [matchAuth, remote?.state.revision, catalog, replayRetry]);
  useEffect(() => {
    setReplayStorageError(!persistReplays(savedReplays));
  }, [savedReplays]);

  const sync = (data: Remote) => {
    setHand((data.state.own.hand || []).map(cardFor));
    setSelected((current) => {
      if (!current?.instanceId) return current;
      const visible = [...(data.state.own.hand || []), ...data.state.own.field, ...data.state.oppo.field];
      const entity = visible.find((item) => item.instanceId === current.instanceId);
      return entity ? cardFor(entity) : null;
    });
    setEvents(
        (data.events ?? [])
          .slice(-4)
          .reverse()
          .map((event) => eventLabel(event, data, catalog)),
      );
  };
  matchCallbacks.current.receive = (data) => {
    if (remote?.state.pendingChoice?.requestId !== data.state.pendingChoice?.requestId) {
      setChoiceSelection([]);
      setChoiceOption(null);
    }
    setRemote(data);
    sync(data);
  };
  const send = async (input: Record<string, unknown>) => {
    if (!remote || !matchAuth || !connection.current) return false;
    if (remote.state.gameOver) {
      setMessage("对局已经结束");
      return false;
    }
    const actionId =
      typeof input.actionId === "string"
        ? input.actionId
        : `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`
            .padEnd(32, "0")
            .slice(0, 32);
    const client = connection.current;
    const accepted = await client.submit({ ...input, actionId });
    if (accepted && connection.current === client) {
        setAttacker(null);
        setChoiceSelection([]);
        setChoiceOption(null);
        setSelected(null);
        setRequestError("");
    }
    return accepted;
  };
  const legal = (kind: string, source?: string, defender?: string) =>
    connectionStatus === "connected" && (remote?.legalActions?.some(
      (action) =>
        action.kind === kind &&
        (!source || action.source === source) &&
        (!defender || action.defender === defender),
    ) ?? false);
  const submitMulligan = async () => {
    if (!matchAuth || remote?.mulliganReady || !connection.current) return;
    const client = connection.current;
    if (await client.submit({ selectedInstanceIds: mulliganSelection }, "/mulligan") && connection.current === client) {
        setMulliganSelection([]);
        setRequestError("");
    }
  };
  const submitAttack = (source: string, defender?: string) => {
    setFeedback({ attacker: source, defender, leader: !defender });
    window.setTimeout(() => setFeedback({}), 520);
    send({ kind: "attack", source, ...(defender ? { defender } : {}) });
  };
  const pending = remote?.state.pendingChoice;
  const candidateCard = (instanceId?: string) => {
    if (!instanceId) return null;
    const entity = [
      ...(own?.hand ?? []),
      ...(own?.field ?? []),
      ...(oppo?.field ?? []),
    ].find((item) => item.instanceId === instanceId);
    return entity ? cardFor(entity) : null;
  };
  const chooseCandidate = (candidate: ChoiceCandidate) => {
    if (!pending || !candidate.instanceId) return;
    setChoiceSelection((current) =>
      current.includes(candidate.instanceId!)
        ? current.filter((id) => id !== candidate.instanceId)
        : current.length >= pending.maxSelections
          ? current
          : [...current, candidate.instanceId!],
    );
  };
  const confirmChoice = () => {
    if (!pending) return;
    if (pending.kind === "mode" && choiceOption !== null)
      send({
        requestId: pending.requestId,
        actionId: pending.actionId,
        stateRevision: pending.stateRevision,
        selectedOptionId: choiceOption,
      });
    else if (choiceSelection.length >= pending.minSelections && choiceSelection.length <= pending.maxSelections)
      send({
        requestId: pending.requestId,
        actionId: pending.actionId,
        stateRevision: pending.stateRevision,
        selectedInstanceIds: choiceSelection,
      });
  };
  const doSourceAction = (
    kind: "evolve" | "superevolve" | "fusion" | "engage",
    card: Card,
  ) => {
    if (!remote || !card.instanceId) return;
    if (!legal(kind, card.instanceId)) {
      setMessage("当前无法执行这个动作");
      return;
    }
    send({ kind, source: card.instanceId });
    setMessage(
      kind === "fusion"
        ? "请选择融合材料"
        : kind === "engage"
          ? "正在启动"
          : kind === "superevolve"
            ? "正在超进化"
            : "正在进化",
    );
  };
  const playCard = (card: Card) => {
    const pp = remote?.state.own.pp ?? demoPP;
    if (card.cost > pp) {
      setMessage("PP 不足，无法使用这张卡牌");
      return;
    }
    if (remote && card.instanceId) {
      if (!legal("play", card.instanceId)) {
        setMessage("这张卡牌当前无法使用");
        return;
      }
      send({ kind: "play", source: card.instanceId });
      return;
    } else {
      setDemoPP((value) => value - card.cost);
      setHand((items) => items.filter((item) => item.id !== card.id));
    }
    setEvents((items) => [`使用 ${card.name}`, ...items].slice(0, 4));
    setMessage(`${card.name} 已打出`);
    setSelected(null);
  };
  const dropCard = (event: DragEvent) => {
    event.preventDefault();
    const id = event.dataTransfer.getData("text/plain");
    const card = hand.find((item) => item.instanceId === id);
    if (card) playCard(card);
  };
  const endTurn = () => {
    if (remote && remote.state.turn.active === "own" && !legal("end_turn")) {
      setMessage("当前不能结束回合");
      return;
    }
    if (remote) {
      send({
        kind: "end_turn",
        ...(remote.state.turn.active === "oppo" ? { actor: "oppo" } : {}),
      });
      return;
    }
    setEvents((items) => ["回合结束", ...items].slice(0, 4));
    setMessage("等待对手回合");
  };
  const selectOwn = (card: Card) => {
    if (!card.instanceId) return;
    if (pending) {
      chooseCandidate({ kind: "instance", instanceId: card.instanceId });
      return;
    }
    const canAttack =
      legal("attack_leader", card.instanceId) ||
      oppoField.some((target) =>
        legal("attack_entity", card.instanceId, target.instanceId),
      );
    setSelected(card);
    if (canAttack) {
      setAttacker(attacker === card.instanceId ? null : card.instanceId);
      setMessage(
        attacker === card.instanceId
          ? `已选择 ${card.name}`
          : `已选择 ${card.name}，请选择攻击目标或右侧动作`,
      );
    } else setMessage(`已选择 ${card.name}`);
  };
  const selectEnemy = (card: Card) => {
    if (pending) {
      chooseCandidate({ kind: "instance", instanceId: card.instanceId });
      return;
    }
    if (attacker && legal("attack_entity", attacker, card.instanceId))
      submitAttack(attacker, card.instanceId);
    else {
      setSelected(card);
      setMessage(`已选择 ${card.name}`);
    }
  };
  const attackLeader = () => {
    if (attacker && legal("attack_leader", attacker)) submitAttack(attacker);
    else setMessage("请先选择可以攻击的随从");
  };
  const beginDrag = (
    event: ReactPointerEvent<HTMLElement>,
    kind: "attack" | "evolve" | "superevolve",
    source?: string,
  ) => {
    if (
      kind === "attack" &&
      (!source ||
        (!legal("attack_leader", source) &&
          !oppoField.some((target) =>
            legal("attack_entity", source, target.instanceId),
          )))
    )
      return;
    if (
      kind !== "attack" &&
      !remote?.legalActions?.some((action) => action.kind === kind)
    )
      return;
    event.preventDefault();
    event.stopPropagation();
    const box = event.currentTarget.getBoundingClientRect();
    setDragGuide({
      kind,
      source,
      x1: box.left + box.width / 2,
      y1: box.top + box.height / 2,
      x2: event.clientX,
      y2: event.clientY,
    });
  };
  const moveDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if (dragGuide)
      setDragGuide((current) =>
        current ? { ...current, x2: event.clientX, y2: event.clientY } : null,
      );
  };
  const finishDrag = (event: ReactPointerEvent<HTMLElement>) => {
    if (!dragGuide) return;
    const target = document
      .elementFromPoint(event.clientX, event.clientY)
      ?.closest<HTMLElement>("[data-instance-id],[data-leader-target]");
    const targetID = target?.dataset.instanceId;
    if (dragGuide.kind === "attack") {
      if (
        dragGuide.source &&
        target?.dataset.leaderTarget === "oppo" &&
        legal("attack_leader", dragGuide.source)
      )
        submitAttack(dragGuide.source);
      else if (
        dragGuide.source &&
        targetID &&
        legal("attack_entity", dragGuide.source, targetID)
      )
        submitAttack(dragGuide.source, targetID);
    } else if (targetID) {
      const card = ownField.find((item) => item.instanceId === targetID);
      if (card && legal(dragGuide.kind, targetID))
        doSourceAction(dragGuide.kind, card);
    }
    setDragGuide(null);
  };
  const own = remote?.state.own;
  const oppo = remote?.state.oppo;
  const ownField = own?.field.map(cardFor) ?? [];
  const oppoField = oppo?.field.map(cardFor) ?? [];
  const pp = own?.pp ?? demoPP;
  const maxPP = own?.maxpp ?? 5;

  const navigate = (page: typeof activePage) => {
    setActivePage(page);
    const params = new URLSearchParams(location.search);
    if (page === "battle") params.delete("page"); else params.set("page", page);
    history.replaceState(null, "", `${location.pathname}${params.toString() ? `?${params}` : ""}`);
    if (page === "battle" && !matchAuth) createMatch();
  };
  const joinRoom = async () => {
    if (roomRequest.current || !joinRoomId.trim() || !joinRoomCode.trim()) return;
    if (deckErrors.length) { setRequestError(deckErrors[0]); navigate("decks"); return; }
    roomRequest.current = true;
    setRoomBusy(true);
    setRequestError("");
    try {
      const response = await fetch(`${API_BASE}/api/matches/${encodeURIComponent(joinRoomId.trim())}/join?code=${encodeURIComponent(joinRoomCode.trim())}`, {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ deck: deckCards.map(Number) }),
      });
      if (!response.ok) throw new Error(response.status === 401 ? "邀请码无效" : response.status === 409 ? "房间已满" : await response.text());
      const data: Remote = await response.json();
      if (!data.matchId || !data.playerToken || data.side !== "oppo" || !data.state) throw new Error("房间响应无效");
      history.replaceState(null, "", `?match=${data.matchId}`);
      acceptMatch(data);
      setActivePage("battle");
    } catch (error) { setRequestError(error instanceof Error ? error.message : "无法加入房间"); }
    finally { roomRequest.current = false; setRoomBusy(false); }
  };
  const workspacePage = activePage === "home" ? (
    <section className="workspace-page home-page">
      <div className="workspace-hero"><span className="eyebrow">WBO OSIRIS</span><h1>战术牌桌，随时开战</h1><p>构筑卡组，加入房间，记录每一次精彩对局。</p><button className="primary-action" onClick={() => navigate("battle")}><Play size={17}/>快速开始</button></div>
      <div className="workspace-grid"><article><small>当前房间</small><strong>{matchAuth ? matchAuth.id : "尚未加入"}</strong><button onClick={() => navigate("rooms")}>管理房间</button></article><article><small>最近对局</small><strong>{events[0] ?? "暂无记录"}</strong><button onClick={() => navigate("replays")}>查看录像</button></article><article><small>当前牌组</small><strong>{classNames[deckClass]} · {deckCards.length}/40</strong><button onClick={() => navigate("decks")}>打开卡组</button></article></div>
    </section>
  ) : activePage === "decks" ? (
    <DeckBuilder cards={catalogCards} deck={deckCards} onChange={changeDeck} onPractice={fillPracticeDeck} onBattle={createMatch} loading={catalogLoading} busy={roomBusy} error={catalogError} onRetry={() => setCatalogVersion((version) => version + 1)}/>
  ) : activePage === "replays" ? (
    <section className="workspace-page replay-page">
      <div className="page-heading"><div><span className="eyebrow">MATCH LOG</span><h1>录像</h1></div></div>
      {replayError && <p role="alert">{replayError} <button onClick={() => setReplayRetry((value) => value + 1)}>重试</button></p>}
      {replayStorageError && <p role="alert">本地存储空间不足或不可用，新录像仅保留在当前页面。</p>}
      {replaySelection ? (() => {
        const record = savedReplays.find((item) => item.id === replaySelection);
        return record ? <ReplayViewer key={record.id} record={record} catalog={catalog} onClose={() => setReplaySelection(null)} /> : null;
      })() : <div className="replay-list">{savedReplays.length ? savedReplays.map((record) => <article key={record.id}>
        <Film size={18}/><div><strong>{record.matchId === "本地记录" ? record.events[0] : `房间 ${record.matchId}`}</strong>
          <small>第 {record.turn} 回合 · {record.events.length} 条事件 · {record.frames?.length || 0} 帧{record.viewer ? ` · ${record.viewer === "own" ? "房主" : "客方"}视角` : ""}{record.winner ? ` · ${record.winner === "draw" ? "平局" : record.winner === "own" ? "我方胜利" : "对手胜利"}` : ""}</small>
          <p className="replay-events">{record.events.slice(-3).join(" · ")}</p>
        </div><button onClick={() => setReplaySelection(record.id)}><Play size={15}/>查看</button>
      </article>) : <p className="empty-state">暂无录像</p>}</div>}
    </section>
  ) : activePage === "rooms" ? (
    <section className="workspace-page"><div className="page-heading"><div><span className="eyebrow">NETWORK</span><h1>房间</h1></div><button className="primary-action" disabled={roomBusy || catalogLoading} onClick={createMatch}><Plus size={17}/>创建房间</button></div><div className="room-deck-status">当前牌组：{deckCards.length}/40 · {deckErrors.length ? deckErrors[0] : "可用于对战"}<button onClick={() => navigate("decks")}>编辑牌组</button></div><div className="room-panel">{matchAuth ? <><span className="status-dot"/>当前房间 <strong>{matchAuth.id}</strong><small>{remote?.waiting ? `等待对手加入 · 邀请码 ${joinCode}` : "对局进行中"}</small><button onClick={() => navigate("battle")}><DoorOpen size={15}/>进入牌桌</button></> : <p className="empty-state">暂无活动房间</p>}</div><div className="join-panel"><h2>加入房间</h2><input aria-label="房间 ID" value={joinRoomId} onChange={(e) => setJoinRoomId(e.target.value)} placeholder="房间 ID"/><input aria-label="邀请码" value={joinRoomCode} onChange={(e) => setJoinRoomCode(e.target.value)} placeholder="邀请码"/><button disabled={roomBusy || catalogLoading || !joinRoomId.trim() || !joinRoomCode.trim()} onClick={joinRoom}><DoorOpen size={15}/>{roomBusy ? "连接中" : "使用牌组加入"}</button></div><div className="room-directory"><h2>公开房间</h2>{roomList.length ? roomList.map((room) => <div className="room-entry" key={room.id}><span>{room.id}</span><small>{room.waiting ? "等待加入" : "对局进行中"}</small></div>) : <p className="empty-state">暂无公开房间</p>}</div></section>
  ) : null;

  const arc = dragGuide
    ? `M ${dragGuide.x1} ${dragGuide.y1} Q ${(dragGuide.x1 + dragGuide.x2) / 2} ${Math.min(dragGuide.y1, dragGuide.y2) - Math.max(55, Math.abs(dragGuide.x2 - dragGuide.x1) * 0.18)} ${dragGuide.x2} ${dragGuide.y2}`
    : "";

  return (
    <main
      className="battle-screen"
      onPointerMove={moveDrag}
      onPointerUp={finishDrag}
      onPointerCancel={() => setDragGuide(null)}
    >
      <div className="battle-vignette" />
      {requestError && <div className="network-notice" role="alert"><span>{requestError}</span><button aria-label="关闭提示" title="关闭" onClick={() => setRequestError("")}><X size={17}/></button></div>}
      {matchAuth && activePage === "battle" && connectionStatus !== "connected" && (
        <div className={`match-connection ${connectionStatus}`} role="status">
          {connectionStatus === "sending" ? "正在提交" : connectionStatus === "connecting" ? "正在连接对局" : "连接中断，正在同步"}
        </div>
      )}
      <header className="battle-topbar">
        <nav className="main-nav">{([["home", Home, "主页"],["battle", Swords, "对战"],["decks", Layers, "卡组"],["replays", Film, "录像"],["rooms", DoorOpen, "房间"]] as const).map(([page, Icon, label]) => <button key={page} className={activePage === page ? "active" : ""} onClick={() => navigate(page)} title={label}><Icon size={17}/><span>{label}</span></button>)}</nav>
        <div className="topbar-actions">
          <button title="帮助">
            <CircleHelp size={18} />
          </button>
          <button
            title="创建新对局"
            disabled={roomBusy}
            onClick={createMatch}
          >
            <Menu size={18} />
          </button>
        </div>
      </header>
      {activePage !== "battle" && workspacePage}
      <div className={activePage !== "battle" ? "battle-table dimmed" : "battle-table"}>
      <section className="battle-table">
        <div className="leader-hud opponent-hud">
          <div className="opponent-side-meta">
            <div className="emblem-zone">
              {Array.from({ length: 5 }).map((_, index) => (
                <span className="emblem-slot" key={index} />
              ))}
            </div>
            <ZoneBanner
              hand={oppo?.handCount ?? 0}
              deck={oppo?.deckCount ?? 0}
              grave={oppo?.shadows ?? 0}
            />
          </div>
          <div className="leader-core">
            <EvoPip kind="ep" value={oppo?.ep ?? 0} />
            <Leader
              name="对手主战者"
              life={oppo?.leaderLife ?? 20}
              max={oppo?.leaderMax ?? 20}
              enemy
              active={Boolean(attacker && legal("attack_leader", attacker))}
              feedback={feedback.leader ? "hit" : ""}
              onClick={attackLeader}
            />
            <EvoPip kind="sep" value={oppo?.sep ?? 0} />
          </div>
        </div>
        <div className="opponent-hand">
          {Array.from({ length: Math.min(oppo?.handCount ?? 4, 7) }).map(
            (_, index) => (
              <span key={index} />
            ),
          )}
        </div>
        <div className="deck-zone opponent-deck">
          <Deck type="deck" count={oppo?.deckCount ?? 28} />
        </div>
        <div
          className="field-zone"
          onDragOver={(event) => event.preventDefault()}
          onDrop={dropCard}
        >
          <div className="field-row opponent-field">
            {oppoField.map((card) => (
              <BoardCard
                key={card.instanceId}
                card={card}
                enemy
                feedback={feedback.defender === card.instanceId ? "hit" : ""}
                active={
                  Boolean(
                    attacker &&
                      legal("attack_entity", attacker, card.instanceId),
                  ) ||
                  Boolean(
                    pending?.candidates.some(
                      (candidate) => candidate.instanceId === card.instanceId,
                    ),
                  )
                }
                selected={choiceSelection.includes(card.instanceId ?? "")}
                onClick={() => selectEnemy(card)}
              />
            ))}
            {Array.from({ length: Math.max(0, 5 - oppoField.length) }).map(
              (_, index) => (
                <span className="field-anchor" key={`oppo-${index}`} />
              ),
            )}
          </div>
          <div className="table-glow">
            <span />
          </div>
          <div className="field-row own-field">
            {ownField.map((card) => {
              const canLeader = legal("attack_leader", card.instanceId);
              const canFollower = oppoField.some((target) =>
                legal("attack_entity", card.instanceId, target.instanceId),
              );
              const hasAttack =
                (card.attacksUsed ?? 0) < (card.attackLimit ?? 1);
              const storm = Boolean(card.keywords?.includes("storm"));
              const rushOnly =
                Boolean(card.keywords?.includes("rush")) ||
                Boolean(card.summoningSick && card.evolved && !storm);
              const selfGlow = card.type === "随从" && hasAttack
                ? rushOnly
                  ? "follower-ready"
                  : !card.summoningSick || storm
                    ? "leader-ready"
                    : ""
                : "";
              return (
                <BoardCard
                  key={card.instanceId}
                  card={card}
                  attackGlow={selfGlow}
                  feedback={
                    feedback.attacker === card.instanceId ? "attack" : ""
                  }
                  active={
                    canLeader ||
                    canFollower ||
                    legal("engage", card.instanceId) ||
                    Boolean(
                      pending?.candidates.some(
                        (candidate) => candidate.instanceId === card.instanceId,
                      ),
                    )
                  }
                  selected={
                    attacker === card.instanceId ||
                    choiceSelection.includes(card.instanceId ?? "")
                  }
                  onPointerDown={(event) =>
                    beginDrag(event, "attack", card.instanceId)
                  }
                  onClick={() => selectOwn(card)}
                />
              );
            })}
            {Array.from({ length: Math.max(0, 5 - ownField.length) }).map(
              (_, index) => (
                <span className="field-anchor" key={`own-${index}`} />
              ),
            )}
          </div>
        </div>
        <div className="leader-hud own-hud">
          <div className="own-side-meta">
            <div className="emblem-zone">
              {Array.from({ length: 5 }).map((_, index) => (
                <span
                  className="emblem-slot"
                  key={index}
                  aria-label={`纹章/信仰槽位 ${index + 1}`}
                />
              ))}
            </div>
            <ZoneBanner
              hand={own?.handCount ?? hand.length}
              deck={own?.deckCount ?? 0}
              grave={own?.shadows ?? 0}
            />
          </div>
          <div className="leader-core">
            <EvoPip
              kind="ep"
              value={own?.ep ?? 2}
              onPointerDown={(event) => beginDrag(event, "evolve")}
            />
            <Leader
              name="你的主战者"
              life={own?.leaderLife ?? 20}
              max={own?.leaderMax ?? 20}
            />
            <EvoPip
              kind="sep"
              value={own?.sep ?? 1}
              onPointerDown={(event) => beginDrag(event, "superevolve")}
            />
          </div>
        </div>
        <div className="deck-zone own-deck">
          <Deck type="deck" count={own?.deckCount ?? 36} />
        </div>
        <div className="right-controls">
          <ResourcePanel pp={oppo?.pp ?? 0} max={oppo?.maxpp ?? 0} enemy />
          <button
            className="end-turn"
            disabled={!legal("end_turn")}
            onClick={endTurn}
          >
            <small>第 {remote?.state.turn.number ?? 1} 回合</small>
            <span>
              {remote?.state.turn.active === "own" ? "结束回合" : "对手回合"}
            </span>
          </button>
          <ResourcePanel
            pp={pp}
            max={maxPP}
            extraAvailable={legal("use_extra_pp")}
            extraActive={own?.extraPPActive}
            extraUses={own?.extraPPUses}
            onExtra={() => send({ kind: "use_extra_pp" })}
          />
        </div>
        <button
          className="history-button"
          onClick={() =>
            setLeftPanel(leftPanel === "history" ? null : "history")
          }
        >
          <History size={18} />
        </button>
        {leftPanel === "history" && (
          <aside className="left-panel">
            <header>
              对战记录
              <button onClick={() => setLeftPanel(null)}>
                <X size={15} />
              </button>
            </header>
            {events.map((event, index) => (
              <p key={`${event}-${index}`}>{event}</p>
            ))}
          </aside>
        )}
      </section></div>
      <section
        className={`hand-dock ${handExpanded ? "expanded" : "collapsed"}`}
        aria-label="手牌"
        onFocusCapture={(event) => {
          if (event.target.matches(".hand-card:focus-visible"))
            setHandExpanded(true);
        }}
        onBlurCapture={(event) => {
          const next = event.relatedTarget as HTMLElement | null;
          if (!event.currentTarget.contains(next) && !next?.closest(".card-inspector"))
            setHandExpanded(false);
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            setSelected(null);
            setHandExpanded(false);
          }
        }}
      >
        <button
          className="hand-toggle"
          title={handExpanded ? "收起手牌" : "展开手牌"}
          aria-label={handExpanded ? "收起手牌" : "展开手牌"}
          aria-expanded={handExpanded}
          aria-controls="battle-hand"
          onClick={() => setHandExpanded((expanded) => !expanded)}
        >
          {handExpanded ? <ChevronDown size={18}/> : <ChevronUp size={18}/>}
          <span>{hand.length}</span>
        </button>
        <div className="hand-row" id="battle-hand">
          {hand.map((card, index) => (
            <HandCard
              key={card.instanceId ?? card.id}
              card={card}
              index={index}
              selected={
                Boolean(selected && (selected.instanceId ?? selected.id) === (card.instanceId ?? card.id)) ||
                Boolean(
                  card.instanceId &&
                    (choiceSelection.includes(card.instanceId) ||
                      mulliganSelection.includes(card.instanceId)),
                )
              }
              active={
                remote?.matchPhase === "mulligan" ||
                !remote ||
                Boolean(
                  card.instanceId &&
                    (pending
                      ? pending.candidates.some(
                          (candidate) =>
                            candidate.instanceId === card.instanceId,
                        )
                      : legal("play", card.instanceId) || legal("fusion", card.instanceId)),
                )
              }
              onClick={() => {
                setHandExpanded(true);
                if (
                  remote?.matchPhase === "mulligan" &&
                  card.instanceId &&
                  !remote.mulliganReady && connectionStatus === "connected"
                )
                  setMulliganSelection((items) =>
                    items.includes(card.instanceId!)
                      ? items.filter((id) => id !== card.instanceId)
                      : [...items, card.instanceId!],
                  );
                else if (pending && card.instanceId)
                  chooseCandidate({
                    kind: "instance",
                    instanceId: card.instanceId,
                  });
                else {
                  setSelected((current) => current && (current.instanceId ?? current.id) === (card.instanceId ?? card.id) ? null : card);
                  setMessage(`已选择 ${card.name}`);
                }
              }}
              onDrop={playCard}
            />
          ))}
        </div>
      </section>
      {dragGuide && (
        <svg className={`drag-guide ${dragGuide.kind}`}>
          <path d={arc} />
          <circle cx={dragGuide.x2} cy={dragGuide.y2} r="8" />
        </svg>
      )}
      {remote?.waiting && matchAuth && (
        <aside className="match-waiting">
          <b>等待 2P 加入</b>
          <span>房间号 {matchAuth.id}</span>
          <small>2P 加入码 {joinCode}</small>
          <button
            disabled={!joinCode}
            onClick={() =>
              navigator.clipboard.writeText(
                `${location.origin}${location.pathname}?match=${matchAuth.id}&join=${joinCode}`,
              )
            }
          >
            复制 2P 邀请链接
          </button>
        </aside>
      )}
      {!remote?.waiting && remote?.matchPhase === "mulligan" && (
        <aside className="mulligan-panel">
          <b>重新抽牌</b>
          {remote.mulliganReady ? (
            <span>已确认，等待对手</span>
          ) : (
            <>
              <span>选择要替换的起始手牌</span>
              <button disabled={connectionStatus !== "connected"} onClick={submitMulligan}>
                确认 ({mulliganSelection.length})
              </button>
            </>
          )}
          <small>{remote.opponentReady ? "对手已确认" : "对手选择中"}</small>
        </aside>
      )}
      {pending && (
        <aside className={`choice-panel ${pending.kind === "mode" ? "mode-panel" : ""}`} aria-label={pending.kind === "mode" ? "选择模式" : "选择目标"}>
          <div className="choice-title">
            <b>
              {pending.kind === "fusion_material"
                ? "选择融合材料"
                : pending.kind === "mode"
                  ? "选择模式"
                  : "选择目标"}
            </b>
            <small>
              {pending.kind === "mode"
                ? "请选择一个模式"
                : pending.minSelections === pending.maxSelections
                  ? `选择 ${pending.maxSelections} 张`
                  : `选择 ${pending.minSelections} 至 ${pending.maxSelections} 张`}
            </small>
          </div>
          <div className="choice-options" role={pending.kind === "mode" ? "radiogroup" : undefined} aria-label={pending.kind === "mode" ? "模式" : undefined}>
            {pending.candidates.map((candidate, index) =>
              candidate.optionId !== undefined ? (
                <label
                  className={`mode-option ${choiceOption === candidate.optionId ? "selected" : ""}`}
                  key={`option-${candidate.optionId}`}
                >
                  <input
                    type="radio"
                    name={`mode-${pending.requestId}`}
                    value={candidate.optionId}
                    checked={choiceOption === candidate.optionId}
                    disabled={connectionStatus !== "connected"}
                    onChange={() => setChoiceOption(candidate.optionId!)}
                  />
                  <span>
                    <b>模式 {candidate.optionId}</b>
                    {(candidate.labels?.chs || candidate.labels?.eng) && (
                      <span className="mode-description">{candidate.labels?.chs || candidate.labels?.eng}</span>
                    )}
                  </span>
                </label>
              ) : (
                <button
                  className={`candidate-card ${candidate.instanceId && choiceSelection.includes(candidate.instanceId) ? "selected" : ""}`}
                  key={`candidate-${candidate.instanceId ?? index}`}
                  aria-pressed={!!candidate.instanceId && choiceSelection.includes(candidate.instanceId)}
                  disabled={choiceSelection.length >= pending.maxSelections && !choiceSelection.includes(candidate.instanceId ?? "")}
                  onClick={() => chooseCandidate(candidate)}
                >
                  {candidateCard(candidate.instanceId) ? (
                    <>
                      <CardArt
                        src={candidateCard(candidate.instanceId)!.art}
                        alt=""
                      />
                      <span>{candidateCard(candidate.instanceId)!.name}</span>
                    </>
                  ) : (
                    <span>未知卡牌</span>
                  )}
                </button>
              ),
            )}
          </div>
          <button
            className="choice-confirm"
            disabled={
              connectionStatus !== "connected" || (pending.kind === "mode"
                ? choiceOption === null
                : choiceSelection.length < pending.minSelections || choiceSelection.length > pending.maxSelections)
            }
            onClick={confirmChoice}
          >
            确认选择 (
            {pending.kind === "mode"
              ? (choiceOption ?? "-")
              : choiceSelection.length}
            )
          </button>
        </aside>
      )}
      {remote?.state.gameOver && (
        <aside className="game-over">
          <strong>
            {remote.state.winner === "own"
              ? "胜利"
              : remote.state.winner === "oppo"
                ? "败北"
                : "对局结束"}
          </strong>
          <span>规则引擎已结束本局对战</span>
        </aside>
      )}
      {selected && (
        <aside className="card-inspector" data-instance-id={selected.instanceId}>
          <button className="close-card" aria-label="关闭卡牌详情" title="关闭卡牌详情" onClick={() => setSelected(null)}>
            <X size={17} />
          </button>
          <CardArt src={selected.art} alt={selected.name} />
          <div>
            <small>
              {selected.type} · 费用 {selected.cost}
            </small>
            <h2>{selected.name}</h2>
            <p>{selected.text}</p>
            {selected.attack !== undefined && (
              <div className="large-stats">
                <b>
                  {selected.attack}
                  <small>攻击</small>
                </b>
                <b>
                  {selected.life}
                  <small>生命</small>
                </b>
              </div>
            )}
            <div className="card-actions">
              {selected.instanceId && legal("play", selected.instanceId) && (
                <button className="use-card" onClick={() => playCard(selected)}>
                  <Sparkles size={15} />
                  使用卡牌
                </button>
              )}
              {selected.instanceId && legal("engage", selected.instanceId) && (
                <button
                  className="use-card"
                  onClick={() => doSourceAction("engage", selected)}
                >
                  <Zap size={15} />启动
                </button>
              )}
              {selected.instanceId && legal("fusion", selected.instanceId) && (
                <button
                  className="use-card"
                  onClick={() => doSourceAction("fusion", selected)}
                >
                  <Combine size={15}/>融合
                </button>
              )}
              {selected.instanceId && legal("evolve", selected.instanceId) && (
                <button
                  className="use-card"
                  onClick={() => doSourceAction("evolve", selected)}
                >
                  进化
                </button>
              )}
              {selected.instanceId &&
                legal("superevolve", selected.instanceId) && (
                  <button
                    className="use-card"
                    onClick={() => doSourceAction("superevolve", selected)}
                  >
                    超进化
                  </button>
                )}
            </div>
            <CounterValues counters={selected.counters}/>
            <FusionDetails fusion={selected.fusion} catalog={catalog}/>
          </div>
        </aside>
      )}
    </main>
  );
}

function Leader({
  name,
  life,
  max,
  enemy = false,
  active = false,
  feedback = "",
  onClick,
}: {
  name: string;
  life: number;
  max: number;
  enemy?: boolean;
  active?: boolean;
  feedback?: string;
  onClick?: () => void;
}) {
  return (
    <button
      aria-label={name}
      data-leader-target={enemy ? "oppo" : "own"}
      className={`leader ${enemy ? "enemy" : ""} ${active ? "active" : ""} ${feedback}`}
      onClick={onClick}
    >
      <SpinePortrait />
      <div className="life-badge">
        <Shield size={12} />
        {life}
        <small>/{max}</small>
      </div>
    </button>
  );
}
function SpinePortrait() {
  const container = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let player: { dispose?: () => void } | undefined;
    let timer: number | undefined;
    const load = () => {
      const Spine = (
        window as unknown as {
          spine?: {
            SpinePlayer?: new (
              element: HTMLElement,
              config: Record<string, unknown>,
            ) => typeof player;
          };
        }
      ).spine?.SpinePlayer;
      if (!container.current || !Spine) {
        timer = window.setTimeout(load, 250);
        return;
      }
      const battleViewport = { x: -229.3, y: -68, width: 457, height: 438.7 };
      player = new Spine(container.current, {
        skelUrl: "/assets/leader-class-1007.skel",
        atlasUrl: "/assets/leader-class-1007.atlas",
        animation: "00_idle",
        skin: "JP",
        alpha: true,
        showControls: false,
        premultipliedAlpha: true,
        backgroundColor: "#00000000",
        viewport: battleViewport,
        rawDataURIs: { "class_1007.png": "/assets/class_1007.png" },
      });
    };
    load();
    return () => {
      if (timer) window.clearTimeout(timer);
      player?.dispose?.();
    };
  }, []);
  return <div className="leader-portrait" ref={container} />;
}
function EvoPip({
  kind,
  value,
  onPointerDown,
}: {
  kind: "ep" | "sep";
  value: number;
  onPointerDown?: (event: ReactPointerEvent<HTMLDivElement>) => void;
}) {
  return (
    <div
      className={`evo-pip ${kind} ${onPointerDown ? "draggable" : ""}`}
      onPointerDown={onPointerDown}
    >
      <img
        src={
          kind === "ep"
            ? "/assets/evolve-normal.png"
            : "/assets/evolve-super.png"
        }
        alt={kind === "ep" ? "进化点" : "超进化点"}
      />
      <b>{value}</b>
    </div>
  );
}
function ResourcePanel({
  pp,
  max,
  enemy = false,
  extraAvailable = false,
  extraActive = false,
  extraUses = 0,
  onExtra,
}: {
  pp: number;
  max: number;
  enemy?: boolean;
  extraAvailable?: boolean;
  extraActive?: boolean;
  extraUses?: number;
  onExtra?: () => void;
}) {
  return (
    <div className={`resource-panel ${enemy ? "enemy" : ""}`}>
      <div className="pp-title">
        <b>{enemy ? "对手" : "己方"}能量点</b>
        <strong>
          {pp}
          <small>/{max}</small>
        </strong>
      </div>
      <div className="pp-lights">
        {Array.from({ length: 10 }).map((_, index) => (
          <i
            className={`${index < pp ? "on" : ""} ${index >= max ? "locked" : ""}`}
            key={index}
          />
        ))}
      </div>
      {onExtra && (
        <button
          className="extra-pp"
          disabled={!extraAvailable}
          onClick={onExtra}
        >
          {extraActive ? "额外能量已激活" : "额外能量 +1"}
          <small>剩余 {extraUses}</small>
        </button>
      )}
    </div>
  );
}
function ZoneBanner({
  hand,
  deck,
  grave,
}: {
  hand: number;
  deck: number;
  grave: number;
}) {
  return (
    <div className="zone-banner">
      <span>
        手牌 <b>{hand}</b>
      </span>
      <span>
        牌组 <b>{deck}</b>
      </span>
      <span>
        墓场 <b>{grave}</b>
      </span>
    </div>
  );
}
function Deck({ type, count }: { type: "deck" | "grave"; count: number }) {
  return (
    <div className={`deck ${type}`}>
      <div className="deck-icon">
        {type === "deck" ? (
          <img src="/assets/card-back.webp" alt="牌组" />
        ) : (
          <span>墓</span>
        )}
      </div>
      <b>{count}</b>
    </div>
  );
}
function BoardCard({
  card,
  enemy = false,
  active = false,
  selected = false,
  feedback = "",
  attackGlow = "",
  onClick,
  onPointerDown,
}: {
  card: Card;
  enemy?: boolean;
  active?: boolean;
  selected?: boolean;
  feedback?: string;
  attackGlow?: string;
  onClick: () => void;
  onPointerDown?: (event: ReactPointerEvent<HTMLButtonElement>) => void;
}) {
  const keywords = card.keywords ?? [];
  const ward = keywords.includes("ward") || keywords.includes("守护");
  const statuses: StatusEffect[] = [];
  if (keywords.includes("aura")) statuses.push("aura");
  if (keywords.includes("barrier")) statuses.push("barrier");
  if (keywords.includes("stealth")) statuses.push("stealth");
  if (keywords.includes("intimidate")) statuses.push("intimidate");
  if (keywords.includes("ability_protected")) statuses.push("abilityProtected");
  if (keywords.includes("hold") || keywords.includes("cannot_attack"))
    statuses.push("hold");
  if (keywords.includes("selfdestruction")) statuses.push("selfDestruction");
  if (card.damageReduction) statuses.push("damageReduction");
  const skills = [
    ["engage", "/assets/skill-activation.png"],
    ["drain", "/assets/skill-drain.png"],
    ["bane", "/assets/skill-bane.png"],
    ["lastwords", "/assets/skill-lastwords.png"],
    ["triggered", "/assets/skill-trigger.png"],
  ];
  if ((card.attackLimit ?? 1) > 1)
    skills.push(["attack_limit", "/assets/skill-attack-limit.png"]);
  const visibleSkills = skills.filter(
    ([keyword]) =>
      keywords.includes(keyword) ||
      (keyword === "attack_limit" && (card.attackLimit ?? 1) > 1),
  );
  return (
    <button
      data-instance-id={card.instanceId}
      aria-label={card.name}
      className={`board-card ${enemy ? "enemy" : ""} ${active ? "active" : ""} ${selected ? "selected" : ""} ${card.evolved ? "evolved" : ""} ${card.superEvolved ? "super-evolved" : ""} ${ward ? "ward" : ""} ${attackGlow} ${feedback}`}
      onPointerDown={onPointerDown}
      onClick={onClick}
    >
      <div className="board-art">
        <CardArt src={card.art} alt="" />
      </div>
      {statuses.length > 0 && <StatusEffects statuses={statuses} />}
      <CounterValues counters={card.counters} compact/>
      {card.type === "随从" && (
        <div className="board-stats">
          <b>{card.attack ?? 0}</b>
          <em>{card.life ?? 0}</em>
        </div>
      )}
      {ward && (
        <span className="ward-overlay">
          <img src="/assets/status-ward.png" alt="" />
        </span>
      )}
      {card.evolved && !card.superEvolved && <span className="evolved-wings" />}
      <span className="skill-icons">
        {visibleSkills.map(([keyword, src]) => (
          <span className={`skill-icon ${keyword}`} key={keyword}>
            <img src={src} alt="" />
            {keyword === "attack_limit" && <b>{card.attackLimit}</b>}
          </span>
        ))}
      </span>
      {card.earthsigil ? (
        <span className="earth-count">
          <img src="/assets/earth-sigil.png" alt="" />
          <b>{card.earthsigil}</b>
        </span>
      ) : card.countdown ? (
        <span className="countdown-count">{card.countdown}</span>
      ) : null}
    </button>
  );
}
function HandCard({
  card,
  index,
  selected,
  active,
  onClick,
  onDrop,
}: {
  card: Card;
  index: number;
  selected: boolean;
  active: boolean;
  onClick: () => void;
  onDrop: (card: Card) => void;
}) {
  return (
    <button
      draggable={Boolean(card.instanceId)}
      aria-label={`${card.name}，费用 ${card.cost}`}
      aria-pressed={selected}
      data-instance-id={card.instanceId}
      className={`hand-card hand-${index} ${selected ? "selected" : ""} ${active ? "active" : "inactive"}`}
      onClick={onClick}
      onDragStart={(event) => {
        if (card.instanceId)
          event.dataTransfer.setData("text/plain", card.instanceId);
      }}
      onDoubleClick={() => onDrop(card)}
    >
      <div className="hand-art">
        <CardArt src={card.art} alt={card.name} />
        <span className="hand-cost">{card.cost}</span>
        <CounterValues counters={card.counters} compact/>
      </div>
      <div className="hand-info">
        <b>{card.name}</b>
        {card.attack !== undefined && (
          <span>
            <i>{card.attack}</i>
            <em>{card.life}</em>
          </span>
        )}
      </div>
    </button>
  );
}
