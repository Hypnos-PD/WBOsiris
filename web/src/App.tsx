import {
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { CircleHelp, History, Menu, Shield, Sparkles, X } from "lucide-react";
import { StatusEffects, type StatusEffect } from "./StatusEffects";

type Card = {
  id: string;
  instanceId?: string;
  name: string;
  cost: number;
  attack?: number;
  life?: number;
  text: string;
  art: string;
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
type Entity = {
  instanceId: string;
  cardId: number;
  attack?: number;
  life?: number;
  cardType: string;
  keywords?: string[];
  evolved?: boolean;
  superEvolved?: boolean;
  earthsigil?: number;
  countdown?: number;
  damageReduction?: number;
  attackLimit?: number;
  attacksUsed?: number;
  summoningSick?: boolean;
};
type LegalAction = {
  kind: string;
  actor: string;
  source?: string;
  defender?: string;
};
type ChoiceCandidate = { kind: string; instanceId?: string; optionId?: number };
type ChoiceRequest = {
  requestId: string;
  actionId: string;
  kind: string;
  minSelections: number;
  maxSelections: number;
  candidates: ChoiceCandidate[];
  stateRevision: number;
};
type RuntimeTarget = {
  instanceId?: string;
  InstanceID?: string;
  kind?: string;
  Kind?: string;
};
type RuntimeEvent = {
  kind?: string;
  Kind?: string;
  instanceId?: string;
  InstanceID?: string;
  actual?: number;
  Actual?: number;
  side?: string;
  Side?: string;
  subject?: RuntimeTarget;
  Subject?: RuntimeTarget;
  target?: RuntimeTarget;
  Target?: RuntimeTarget;
};
type Remote = {
  sessionId: string;
  matchId?: string;
  playerToken?: string;
  joinCode?: string;
  side?: string;
  waiting?: boolean;
  matchPhase?: string;
  mulliganReady?: boolean;
  opponentReady?: boolean;
  state: {
    own: {
      leaderLife: number;
      leaderMax: number;
      pp: number;
      maxpp: number;
      ep: number;
      sep: number;
      extraPPAvailable?: boolean;
      extraPPUses?: number;
      extraPPActive?: boolean;
      deckCount?: number;
      handCount?: number;
      hand: Entity[];
      field: Entity[];
      graveyard?: Entity[];
    };
    oppo: {
      leaderLife: number;
      leaderMax: number;
      pp: number;
      maxpp: number;
      ep: number;
      sep: number;
      extraPPAvailable?: boolean;
      extraPPUses?: number;
      handCount?: number;
      deckCount?: number;
      field: Entity[];
      graveyard?: Entity[];
    };
    turn: { active: string; number: number };
    pendingChoice?: ChoiceRequest;
    gameOver?: boolean;
    winner?: string;
  };
  legalActions?: LegalAction[];
  events?: RuntimeEvent[];
  result?: { status: string; errorCode?: string };
};

const catalog: Record<string, Omit<Card, "id" | "instanceId">> = {
  "10001110": {
    name: "不屈的剑斗士",
    cost: 2,
    attack: 2,
    life: 2,
    type: "随从",
    text: "爆能强化 4：本随从 +3/+3。",
    art: "/assets/card-10001110.webp",
  },
  "10012110": {
    name: "冒险精灵·小梅",
    cost: 1,
    attack: 1,
    life: 1,
    type: "随从",
    text: "",
    art: "/assets/card-10012110.webp",
  },
  "10001120": {
    name: "叮当天使·莉亚",
    cost: 2,
    attack: 0,
    life: 2,
    type: "随从",
    text: "守护\n谢幕曲：抽取1张卡牌。\n进化时：抽取1张卡牌。",
    art: "/assets/card-10001120.webp",
    keywords: ["守护"],
  },
  "10002110": {
    name: "煌响使者·亨莉雅妲",
    cost: 3,
    attack: 3,
    life: 3,
    type: "随从",
    text: "进化时：回复自己的主战者2点生命值。",
    art: "/assets/card-10002110.webp",
  },
  "10011130": {
    name: "温厚的树精",
    cost: 4,
    attack: 4,
    life: 4,
    type: "随从",
    text: "入场曲：连击3，本随从进化。",
    art: "/assets/card-10011130.webp",
  },
};
const fallbackHand: Card[] = [
  { id: "fallback-10001110", ...catalog["10001110"] },
  { id: "fallback-10001120", ...catalog["10001120"] },
  { id: "fallback-10002110", ...catalog["10002110"] },
];
const cardFor = (entity: Entity): Card => {
  const base = catalog[String(entity.cardId)] ?? {
    name: "未知卡牌",
    cost: 0,
    type: "随从" as const,
    text: "",
    art: "/assets/card-10001120.webp",
  };
  return {
    id: String(entity.cardId),
    instanceId: entity.instanceId,
    ...base,
    attack: entity.attack ?? base.attack,
    life: entity.life ?? base.life,
    keywords: entity.keywords ?? base.keywords,
    evolved: entity.evolved,
    superEvolved: entity.superEvolved,
    earthsigil: entity.earthsigil,
    countdown: entity.countdown,
    damageReduction: entity.damageReduction,
    attackLimit: entity.attackLimit,
    attacksUsed: entity.attacksUsed,
    summoningSick: entity.summoningSick,
  };
};

export function App() {
  const [remote, setRemote] = useState<Remote | null>(null);
  const [hand, setHand] = useState<Card[]>(fallbackHand);
  const [selected, setSelected] = useState<Card | null>(null);
  const [message, setMessage] = useState("请选择要进行的操作");
  const [events, setEvents] = useState(["你的回合开始"]);
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

  const acceptMatch = (data: Remote, previousToken = "") => {
    const token = data.playerToken || previousToken;
    if (!data.matchId || !token || !data.side) return;
    const auth = { id: data.matchId, token, side: data.side };
    localStorage.setItem(
      `wbo-match-${auth.id}-${auth.side}`,
      JSON.stringify(auth),
    );
    if (data.joinCode) setJoinCode(data.joinCode);
    setMatchAuth(auth);
    setRemote(data);
    sync(data);
  };
  const createMatch = () => {
    fetch("http://127.0.0.1:8080/api/matches", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    })
      .then((response) => response.json())
      .then((data: Remote) => {
        history.replaceState(null, "", `?match=${data.matchId}`);
        acceptMatch(data);
      })
      .catch(() => setMessage("无法连接对局服务"));
  };
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const params = new URLSearchParams(location.search);
    const id = params.get("match");
    const code = params.get("join");
    const saved = id
      ? localStorage.getItem(`wbo-match-${id}-own`) ||
        localStorage.getItem(`wbo-match-${id}-oppo`)
      : null;
    if (id && code) {
      fetch(
        `http://127.0.0.1:8080/api/matches/${id}/join?code=${encodeURIComponent(code)}`,
        { method: "POST" },
      )
        .then((response) => {
          if (!response.ok) throw new Error();
          return response.json();
        })
        .then((data: Remote) => {
          history.replaceState(null, "", `?match=${id}`);
          acceptMatch(data);
        })
        .catch(() => setMessage("加入码无效或房间已满"));
    } else if (id && saved) {
      const auth = JSON.parse(saved) as {
        id: string;
        token: string;
        side: string;
      };
      setMatchAuth(auth);
      return;
    } else createMatch();
  }, []);
  useEffect(() => {
    if (!matchAuth) return;
    const refresh = () =>
      fetch(`http://127.0.0.1:8080/api/matches/${matchAuth.id}`, {
        headers: { Authorization: `Bearer ${matchAuth.token}` },
      })
        .then((response) => (response.ok ? response.json() : Promise.reject()))
        .then((data: Remote) => {
          data.matchId = matchAuth.id;
          data.playerToken = matchAuth.token;
          data.side = matchAuth.side;
          setRemote(data);
          sync(data);
        })
        .catch(() => undefined);
    refresh();
    const timer = window.setInterval(refresh, 700);
    return () => window.clearInterval(timer);
  }, [matchAuth]);

  const sync = (data: Remote) => {
    setHand(data.state.own.hand.map(cardFor));
    if (data.events?.length)
      setEvents(
        data.events
          .slice(-4)
          .reverse()
          .map((event) => eventLabel(event, data)),
      );
  };
  const send = (input: Record<string, unknown>) => {
    if (!remote || !matchAuth) return;
    if (remote.state.gameOver) {
      setMessage("对局已经结束");
      return;
    }
    const actionId =
      typeof input.actionId === "string"
        ? input.actionId
        : `${Date.now().toString(16)}${Math.random().toString(16).slice(2)}`
            .padEnd(32, "0")
            .slice(0, 32);
    fetch(`http://127.0.0.1:8080/api/matches/${matchAuth.id}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${matchAuth.token}`,
      },
      body: JSON.stringify({ ...input, actionId }),
    })
      .then((response) => response.json())
      .then((data: Remote) => {
        data.matchId = matchAuth.id;
        data.playerToken = matchAuth.token;
        data.side = matchAuth.side;
        setRemote(data);
        sync(data);
        setAttacker(null);
        setChoiceSelection([]);
        setChoiceOption(null);
        if (data.result?.errorCode)
          setMessage(`操作未完成：${data.result.errorCode}`);
      });
  };
  const legal = (kind: string, source?: string, defender?: string) =>
    remote?.legalActions?.some(
      (action) =>
        action.kind === kind &&
        (!source || action.source === source) &&
        (!defender || action.defender === defender),
    ) ?? false;
  const submitMulligan = () => {
    if (!matchAuth || remote?.mulliganReady) return;
    fetch(`http://127.0.0.1:8080/api/matches/${matchAuth.id}/mulligan`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${matchAuth.token}`,
      },
      body: JSON.stringify({ selectedInstanceIds: mulliganSelection }),
    })
      .then((response) => response.json())
      .then((data: Remote) => {
        data.matchId = matchAuth.id;
        data.playerToken = matchAuth.token;
        data.side = matchAuth.side;
        setRemote(data);
        sync(data);
        setMulliganSelection([]);
      });
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
    else if (choiceSelection.length >= pending.minSelections)
      send({
        requestId: pending.requestId,
        actionId: pending.actionId,
        stateRevision: pending.stateRevision,
        selectedInstanceIds: choiceSelection,
      });
  };
  const doSourceAction = (
    kind: "evolve" | "superevolve" | "fusion",
    card: Card,
  ) => {
    if (!remote || !card.instanceId) return;
    if (!legal(kind, card.instanceId)) {
      setMessage("当前无法执行这个动作");
      return;
    }
    send({ kind, source: card.instanceId });
    setSelected(null);
    setMessage(
      kind === "fusion"
        ? "请选择融合材料"
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
      setHand((items) =>
        items.filter((item) => item.instanceId !== card.instanceId),
      );
      send({ kind: "play", source: card.instanceId });
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
    if (remote)
      send({
        kind: "end_turn",
        ...(remote.state.turn.active === "oppo" ? { actor: "oppo" } : {}),
      });
    setEvents((items) => ["回合结束", ...items].slice(0, 4));
    setMessage(
      remote?.state.turn.active === "oppo" ? "对手回合结束" : "等待对手回合",
    );
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
    else setMessage("当前没有可用的攻击目标");
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
      <header className="battle-topbar">
        <div className="topbar-actions">
          <button title="帮助">
            <CircleHelp size={18} />
          </button>
          <button
            title="创建新对局"
            onClick={() => {
              if (matchAuth)
                localStorage.removeItem(
                  `wbo-match-${matchAuth.id}-${matchAuth.side}`,
                );
              history.replaceState(null, "", location.pathname);
              setMatchAuth(null);
              setJoinCode("");
              createMatch();
            }}
          >
            <Menu size={18} />
          </button>
        </div>
      </header>
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
              grave={oppo?.graveyard?.length ?? 0}
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
              const selfGlow = hasAttack
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
              grave={own?.graveyard?.length ?? 0}
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
            disabled={remote?.waiting || remote?.state.turn.active !== "own"}
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
      </section>
      <section
        className={`hand-dock ${handExpanded ? "expanded" : "collapsed"}`}
        onFocusCapture={() => setHandExpanded(true)}
        onBlurCapture={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null))
            setHandExpanded(false);
        }}
      >
        <div className="hand-row">
          {hand.map((card, index) => (
            <HandCard
              key={card.instanceId ?? card.id}
              card={card}
              index={index}
              selected={
                selected?.id === card.id ||
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
                      : legal("play", card.instanceId)),
                )
              }
              onClick={() => {
                if (
                  remote?.matchPhase === "mulligan" &&
                  card.instanceId &&
                  !remote.mulliganReady
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
                  setSelected(selected?.id === card.id ? null : card);
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
              <button onClick={submitMulligan}>
                确认 ({mulliganSelection.length})
              </button>
            </>
          )}
          <small>{remote.opponentReady ? "对手已确认" : "对手选择中"}</small>
        </aside>
      )}
      {pending && (
        <aside className="choice-panel">
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
                : `请选择 ${pending.minSelections} 至 ${pending.maxSelections} 张`}
            </small>
          </div>
          <div className="choice-options">
            {pending.candidates.map((candidate, index) =>
              candidate.optionId !== undefined ? (
                <button
                  className={`mode-option ${choiceOption === candidate.optionId ? "selected" : ""}`}
                  key={`option-${candidate.optionId}`}
                  onClick={() => setChoiceOption(candidate.optionId!)}
                >
                  模式 {candidate.optionId}
                </button>
              ) : (
                <button
                  className={`candidate-card ${candidate.instanceId && choiceSelection.includes(candidate.instanceId) ? "selected" : ""}`}
                  key={`candidate-${candidate.instanceId ?? index}`}
                  onClick={() => chooseCandidate(candidate)}
                >
                  {candidateCard(candidate.instanceId) ? (
                    <>
                      <img
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
              pending.kind === "mode"
                ? choiceOption === null
                : choiceSelection.length < pending.minSelections
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
        <aside className="card-inspector">
          <button className="close-card" onClick={() => setSelected(null)}>
            <X size={17} />
          </button>
          <img src={selected.art} alt={selected.name} />
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
              {selected.instanceId && legal("fusion", selected.instanceId) && (
                <button
                  className="use-card"
                  onClick={() => doSourceAction("fusion", selected)}
                >
                  融合
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
  if (keywords.includes("hold")) statuses.push("hold");
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
        <img src={card.art} alt="" />
      </div>
      {statuses.length > 0 && <StatusEffects statuses={statuses} />}
      <div className="board-stats">
        <b>{card.attack ?? 0}</b>
        <em>{card.life ?? 0}</em>
      </div>
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
      className={`hand-card hand-${index} ${selected ? "selected" : ""} ${active ? "active" : "inactive"}`}
      onClick={onClick}
      onDragStart={(event) => {
        if (card.instanceId)
          event.dataTransfer.setData("text/plain", card.instanceId);
      }}
      onDoubleClick={() => onDrop(card)}
    >
      <div className="hand-art">
        <img src={card.art} alt={card.name} />
        <span className="hand-cost">{card.cost}</span>
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

function eventLabel(event: RuntimeEvent, remote: Remote): string {
  const kind = event.kind ?? event.Kind ?? "event";
  const actual = event.actual ?? event.Actual;
  const target =
    event.instanceId ??
    event.InstanceID ??
    event.subject?.instanceId ??
    event.subject?.InstanceID ??
    event.target?.instanceId ??
    event.target?.InstanceID;
  const entity = [
    ...remote.state.own.hand,
    ...remote.state.own.field,
    ...remote.state.oppo.field,
  ].find((item) => item.instanceId === target);
  const name = entity ? cardFor(entity).name : "随从";
  if (kind === "attacked") return `${name} 发起攻击`;
  if (kind === "damaged")
    return `${name} ${actual ? `受到 ${actual} 点伤害` : "受到伤害"}`;
  if (kind === "healed")
    return `${name} ${actual ? `回复 ${actual} 点生命` : "回复生命"}`;
  if (kind === "follower_summoned") return `${name} 入场`;
  if (kind === "evolved") return `${name} 完成进化`;
  if (kind === "super_evolved") return `${name} 完成超进化`;
  if (kind === "card_drawn") return "抽取卡牌";
  if (kind === "destroyed") return `${name} 被破坏`;
  if (kind === "turn_started")
    return event.Side === "own" || event.side === "own"
      ? "你的回合开始"
      : "对手回合开始";
  if (kind === "turn_ended") return "回合结束";
  return kind.replaceAll("_", " ");
}
