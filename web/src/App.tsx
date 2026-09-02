import { useState } from "react";
import { Check, ChevronDown, Eye, Flag, Hand, Info, RotateCcw, Shield, Sparkles, Swords, Target, X, Zap } from "lucide-react";

type Card = { id: string; name: string; cost: number; attack?: number; life?: number; text: string; art: string; keywords?: string[]; type: "随从" | "法术" | "护符" };
const cards: Card[] = [
  { id: "leah", name: "叮当天使·莉亚", cost: 2, attack: 0, life: 2, type: "随从", text: "守护\n谢幕曲：抽取1张卡牌。\n进化时：抽取1张卡牌。", art: "/assets/card-10001120.webp", keywords: ["守护"] },
  { id: "henrietta", name: "煌响使者·亨莉雅妲", cost: 3, attack: 3, life: 3, type: "随从", text: "进化时：回复自己的主战者2点生命值。", art: "/assets/card-10002110.webp" },
  { id: "swordsman", name: "不屈的剑斗士", cost: 1, attack: 2, life: 2, type: "随从", text: "", art: "/assets/card-10001110.webp" },
  { id: "forest", name: "森林的意志", cost: 2, type: "法术", text: "对一个敌方随从造成3点伤害。", art: "/assets/card-10001120.webp" },
];
const field = [cards[0], cards[1]];

export function App() {
  const [hand, setHand] = useState(cards.slice(2));
  const [selected, setSelected] = useState<Card | null>(null);
  const [targeting, setTargeting] = useState(false);
  const [turn, setTurn] = useState(5);
  const [pp, setPp] = useState(5);
  const [message, setMessage] = useState("请选择要进行的操作");
  const [events, setEvents] = useState(["你的回合开始", "抽取了1张卡牌"]);
  const play = () => {
    if (!selected) return;
    if (selected.cost > pp) { setMessage("PP 不足，无法使用这张卡牌"); return; }
    setPp((n) => n - selected.cost); setHand((items) => items.filter((item) => item.id !== selected.id)); setEvents((items) => [`使用了 ${selected.name}`, ...items].slice(0, 3)); setMessage(`${selected.name} 已打出`); setSelected(null);
  };
  const endTurn = () => { setTurn((n) => n + 1); setPp(5); setEvents((items) => ["回合结束", ...items].slice(0, 3)); setMessage("对手的回合"); };
  return <div className="game-shell">
    <header className="game-header"><div className="game-brand"><span className="crest">W</span><div><b>WBOsiris</b><small>超凡世界 · 标准规则</small></div></div><div className="turn-badge"><span className="turn-dot" />你的回合 <strong>{turn}</strong></div><div className="header-actions"><button title="对局信息"><Info size={18} /></button><button title="重新开始" onClick={() => location.reload()}><RotateCcw size={18} /></button></div></header>
    <section className="arena">
      <div className="opponent-zone"><Leader name="对手" className="enemy" life={20} /><div className="deck-stack opponent-deck"><span>?</span><small>牌组 28</small></div><div className="opponent-hand"><i /><i /><i /><i /></div></div>
      <div className="field-zone"><div className="field-row opponent-field"><div className="slot" /><div className="slot" /><div className="slot" /></div><div className="arena-divider"><span>第 {turn} 回合</span></div><div className="field-row own-field">{field.map((card) => <BoardCard key={card.id} card={card} onClick={() => { setTargeting(true); setMessage(`请选择 ${card.name} 的目标`); }} />)}<div className="slot" /></div></div>
      <div className="own-zone"><div className="deck-stack"><span>W</span><small>牌组 24</small></div><Leader name="你的主战者" className="hero" life={20} /><div className="grave"><span>墓场</span><b>3</b></div></div>
    </section>
    <section className="control-bar"><div className="status-message"><Target size={16} /><span>{message}</span></div><div className="event-strip">{events.map((event, index) => <span key={`${event}-${index}`}>{event}</span>)}</div><div className="turn-controls"><button className="icon-control" title="查看战场" onClick={() => setMessage("当前战场状态")}> <Eye size={18} /></button><button className="end-turn" onClick={endTurn}><ChevronDown size={17} />结束回合</button></div></section>
    <section className="hand-dock"><div className="hand-title"><span>手牌 <b>{hand.length}</b></span><small>PP <strong>{pp}</strong> / 5</small></div><div className="hand-row">{hand.map((card) => <HandCard key={card.id} card={card} selected={selected?.id === card.id} onClick={() => { setSelected(selected?.id === card.id ? null : card); setMessage(selected?.id === card.id ? "请选择要进行的操作" : `已选择 ${card.name}`); }} />)}</div><div className="resource-row"><div className="pp-bar"><i style={{ width: `${pp * 20}%` }} /></div><div className="evo"><span>进化点</span><b>2</b><span>超进化点</span><b>1</b></div></div></section>
    {selected && <aside className="card-inspector"><button className="dismiss" onClick={() => setSelected(null)}><X size={17} /></button><img src={selected.art} alt={selected.name} /><div className="card-copy"><small>{selected.type} · 费用 {selected.cost}</small><h2>{selected.name}</h2><p>{selected.text}</p>{selected.attack !== undefined && <div className="inspect-stats"><b>{selected.attack}<small>攻击</small></b><b>{selected.life}<small>生命</small></b></div>}<button className="play-card" onClick={play}><Sparkles size={16} />使用卡牌</button></div></aside>}
    {targeting && <div className="target-banner"><span><Target size={18} />请选择目标</span><button onClick={() => { setTargeting(false); setMessage("已取消目标选择"); }}><X size={16} />取消</button></div>}
  </div>;
}
function Leader({ name, className, life }: { name: string; className: string; life: number }) { return <div className={`leader ${className}`}><div className="leader-portrait"><div>{className === "hero" ? "你" : "敌"}</div></div><div className="leader-name"><b>{name}</b><span><Shield size={14} /> {life}</span></div></div>; }
function BoardCard({ card, onClick }: { card: Card; onClick: () => void }) { return <button className="board-card" onClick={onClick}><div className="board-art"><img src={card.art} alt="" /></div><span className="board-cost">{card.cost}</span><div className="board-name">{card.name}</div><div className="board-values"><b>{card.attack}</b><em>{card.life}</em></div>{card.keywords?.map((key) => <small className="keyword" key={key}>{key}</small>)}</button>; }
function HandCard({ card, selected, onClick }: { card: Card; selected: boolean; onClick: () => void }) { return <button className={`hand-card ${selected ? "selected" : ""}`} onClick={onClick}><div className="hand-art"><img src={card.art} alt={card.name} /><span className="hand-cost">{card.cost}</span></div><div className="hand-info"><b>{card.name}</b>{card.attack !== undefined && <span><i>{card.attack}</i><em>{card.life}</em></span>}</div>{selected && <span className="selected-mark"><Check size={13} /></span>}</button>; }
