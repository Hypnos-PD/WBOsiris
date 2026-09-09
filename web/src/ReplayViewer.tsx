import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ChevronLeft, ChevronRight, Pause, Play, SkipBack, SkipForward, X } from "lucide-react";
import { CardArt } from "./CardArt";
import { cardArt, cardText, typeNames } from "./decks";
import type { Entity, PlayerView } from "./gameTypes";
import { eventLabel, frameEvents, type ReplayRecord } from "./replays";
import "./replays.css";
import { FusionDetails } from "./FusionDetails";
import { CounterValues } from "./CounterValues";
import { TriggerLimits } from "./TriggerLimits";
import { CrestZone, crestName } from "./CrestZone";

type Catalog = Record<string, { name: string; text: string; cost: number; attack?: number; life?: number }>;

export function ReplayViewer({ record, catalog, onClose }: { record: ReplayRecord; catalog: Catalog; onClose: () => void }) {
  const frames = record.frames || [];
  const [cursor, setCursor] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(1);
  const [selected, setSelected] = useState<Entity | null>(null);
  const detail = useRef<HTMLDialogElement>(null);
  const index = Math.min(cursor, Math.max(0, frames.length - 1));
  const seek = (next: number) => { setPlaying(false); setCursor(Math.max(0, Math.min(frames.length - 1, next))); };
  useEffect(() => {
    if (!playing) return;
    if (index >= frames.length - 1) { setPlaying(false); return; }
    const timer = window.setTimeout(() => setCursor(index + 1), 1200 / speed);
    return () => window.clearTimeout(timer);
  }, [playing, speed, index, frames.length]);
  useEffect(() => {
    if (selected) detail.current?.showModal();
    else detail.current?.close();
  }, [selected]);
  const togglePlay = () => {
    if (!playing && index === frames.length - 1) setCursor(0);
    setPlaying((value) => !value);
  };
  const inspect = (entity: Entity) => { setPlaying(false); setSelected(entity); };
  const nameFor = (entity: Entity) => entity.cardType === "crest" ? crestName(entity) : catalog[String(entity.cardId)]?.name || `卡牌 ${entity.cardId}`;
  const renderCard = (entity: Entity) => <button className={`rp-card${entity.superEvolved ? " super-evolved" : entity.evolved ? " evolved" : ""}`} key={entity.instanceId} onClick={() => inspect(entity)} title={nameFor(entity)} aria-label={`查看 ${nameFor(entity)}`}>
    <div className="rp-art"><CardArt src={cardArt(entity.cardId, entity.evolved || entity.superEvolved)} alt={nameFor(entity)}/>
      <span className="rp-cost">{entity.cost ?? catalog[String(entity.cardId)]?.cost ?? 0}</span>
      <CounterValues counters={entity.counters} compact/>
      {entity.cardType === "follower" && <span className="rp-stats"><b>{entity.attack ?? 0}</b><b>{entity.life ?? 0}</b></span>}
      {entity.cardType === "amulet" && (entity.countdown !== undefined || entity.earthsigil !== undefined) && <span className="rp-counter">{entity.earthsigil !== undefined ? `土 ${entity.earthsigil}` : `倒数 ${entity.countdown}`}</span>}
    </div><span className="rp-name">{nameFor(entity)}</span>
  </button>;
  const field = (player: PlayerView, label: string) => <div className="rp-field" aria-label={`${label}战场`}>
    {Array.from({ length: 5 }, (_, slot) => <div className="rp-slot" key={slot}>{player.field?.[slot] ? renderCard(player.field[slot]) : <span aria-label="空位"/>}</div>)}
  </div>;
  const resources = (player: PlayerView, label: string, active: boolean) => <div className={`rp-resources${active ? " active" : ""}`}>
    <strong>{label}</strong><span className="rp-life">生命 <b>{player.leaderLife}/{player.leaderMax}</b></span>
    <span className="rp-pp">PP <b>{player.pp}/{player.maxpp}</b></span><span>EP <b>{player.ep}</b></span><span>SEP <b>{player.sep}</b></span>
    <span>牌堆 <b>{player.deckCount ?? 0}</b></span><span>墓场 <b>{player.shadows}</b></span>
  </div>;
  const frame = frames[index];
  const state = frame?.state;
  const currentEvents = frameEvents(record, index);
  const card = selected && catalog[String(selected.cardId)];
  return <div className="replay-player" tabIndex={0} onKeyDown={(event) => {
    if (selected || (event.target as HTMLElement).closest("button, input, select, dialog")) return;
    if (event.key === "ArrowLeft") { event.preventDefault(); seek(index - 1); }
    if (event.key === "ArrowRight") { event.preventDefault(); seek(index + 1); }
    if (event.key === " " && frames.length > 1) { event.preventDefault(); togglePlay(); }
  }}>
    <div className="rp-heading"><button onClick={onClose}><ArrowLeft size={17}/>录像列表</button><span>房间 {record.matchId}</span><small>{record.viewer === "oppo" ? "客方视角" : "房主视角"}</small></div>
    {state ? <>
      <div className="rp-transport">
        <div className="rp-buttons">
          <button title="第一帧" aria-label="第一帧" disabled={index === 0} onClick={() => seek(0)}><SkipBack size={18}/></button>
          <button title="上一帧" aria-label="上一帧" disabled={index === 0} onClick={() => seek(index - 1)}><ChevronLeft size={20}/></button>
          <button className="rp-play" title={playing ? "暂停" : "播放"} aria-label={playing ? "暂停" : "播放"} disabled={frames.length < 2} onClick={togglePlay}>{playing ? <Pause size={18}/> : <Play size={18}/>}</button>
          <button title="下一帧" aria-label="下一帧" disabled={index === frames.length - 1} onClick={() => seek(index + 1)}><ChevronRight size={20}/></button>
          <button title="最后一帧" aria-label="最后一帧" disabled={index === frames.length - 1} onClick={() => seek(frames.length - 1)}><SkipForward size={18}/></button>
        </div>
        <output className="rp-position" aria-label="播放位置">{index + 1} / {frames.length}</output>
        <select aria-label="播放速度" value={speed} onChange={(event) => setSpeed(Number(event.target.value))}>{[0.5, 1, 2, 4].map((value) => <option key={value} value={value}>{value}x</option>)}</select>
        <input className="rp-timeline" aria-label="状态帧" type="range" min={0} max={frames.length - 1} value={index} onChange={(event) => seek(Number(event.target.value))}/>
      </div>
      <div className="rp-layout">
        <div className="rp-table">
          {resources(state.oppo, "对手", state.turn.active === "oppo")}
          <CrestZone crests={state.oppo.crests} label="对手" onInspect={inspect}/>
          <div className="rp-hidden-hand" aria-label={`对手手牌 ${state.oppo.handCount || 0} 张`}><div>{Array.from({ length: Math.min(9, state.oppo.handCount || 0) }, (_, i) => <img src="/assets/card-back.webp" alt="隐藏手牌" key={i}/>)}</div><span>手牌 {state.oppo.handCount || 0}</span></div>
          {field(state.oppo, "对手")}
          <div className="rp-turn"><strong>第 {state.turn.number} 回合</strong><span>{state.gameOver ? state.winner === "draw" ? "平局" : state.winner === "own" ? "我方胜利" : "对手胜利" : state.pendingChoice ? "等待选择" : state.phase === "mulligan" ? "起手换牌" : state.turn.active === "own" ? "我方行动" : "对手行动"}</span></div>
          {field(state.own, "我方")}
          {resources(state.own, "我方", state.turn.active === "own")}
          <CrestZone crests={state.own.crests} label="我方" onInspect={inspect}/>
          <div className="rp-hand-heading">我方手牌 <b>{state.own.handCount ?? state.own.hand?.length ?? 0}</b></div>
          <div className="rp-hand" aria-label="我方手牌">{(state.own.hand || []).map(renderCard)}</div>
          {state.pendingChoice && <div className="rp-choice"><strong>{state.pendingChoice.kind === "mode" ? "模式选择" : state.pendingChoice.kind === "fusion_material" ? "选择融合材料" : `选择目标 ${state.pendingChoice.minSelections}`}</strong><span>{state.pendingChoice.candidates.map((candidate) => {
            if (candidate.optionId !== undefined) return `选项 ${candidate.optionId}`;
            if (candidate.kind === "leader") return candidate.leaderSide === state.viewer ? "我方主战者" : "对手主战者";
            const entity = [...(state.own.hand || []), ...(state.own.field || []), ...(state.oppo.field || []), ...(state.own.graveyard || [])].find((item) => item.instanceId === candidate.instanceId);
            return entity ? nameFor(entity) : "卡牌";
          }).join(" · ")}</span></div>}
        </div>
        <aside className="rp-log"><h2>{currentEvents ? "当前帧事件" : "事件日志"}</h2>
          {currentEvents === undefined && <p>此记录没有事件时间信息。</p>}
          <ol key={index}>{(currentEvents ? currentEvents.map((event) => eventLabel(event, { state }, catalog, frames[index - 1]?.state)) : record.events).map((label, i) => <li key={i}>{label}</li>)}</ol>
          {currentEvents?.length === 0 && <p>本帧无事件</p>}
        </aside>
      </div>
    </> : <div className="rp-log"><p>此记录没有状态帧。</p><ol>{record.events.map((label, index) => <li key={index}>{label}</li>)}</ol></div>}
    <dialog className="rp-detail" ref={detail} onCancel={() => setSelected(null)} onClose={() => setSelected(null)} onClick={(event) => { if (event.target === event.currentTarget) setSelected(null); }}>
      {selected && <div className="rp-detail-content"><button className="rp-detail-close" autoFocus aria-label="关闭卡牌详情" title="关闭" onClick={() => setSelected(null)}><X size={20}/></button>
        <div className="rp-detail-art"><CardArt src={cardArt(selected.cardId, selected.evolved || selected.superEvolved)} alt={nameFor(selected)}/></div>
        <div><h2>{nameFor(selected)}</h2><p>{typeNames[selected.cardType]}{selected.cardType === "crest" ? selected.countdown ? ` · 吟唱 ${selected.countdown}` : "" : ` · ${selected.cost ?? card?.cost ?? 0} PP`}{selected.superEvolved ? " · 超进化" : selected.evolved ? " · 进化" : ""}</p>
          {selected.cardType === "follower" && <p>攻击 {selected.attack ?? 0} · 生命 {selected.life ?? 0}{selected.maxLife !== undefined && ` / ${selected.maxLife}`}</p>}
          <p className="rp-rules">{selected.cardType === "crest" ? cardText(selected.crestLocales?.chs?.text || "") : card?.text || "无能力"}</p>
          {selected.cardType !== "crest" && !!selected.keywords?.length && <p>{selected.keywords.join(" · ")}</p>}
          <FusionDetails fusion={selected.fusion} catalog={catalog}/>
          <CounterValues counters={selected.counters}/>
          <TriggerLimits limits={selected.triggerLimits}/>
        </div>
      </div>}
    </dialog>
  </div>;
}
