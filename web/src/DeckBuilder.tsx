import { useEffect, useMemo, useRef, useState } from "react";
import { Check, Layers, Minus, Plus, Search, Swords, Trash2, X } from "lucide-react";
import { cardArt, hasEvolvedArt, classNames, deckProblems, typeNames, type CatalogCard } from "./decks";
import { CardArt } from "./CardArt";
import { CounterValues } from "./CounterValues";
import "./decks.css";

type Props = {
  cards: CatalogCard[];
  deck: string[];
  onChange: (deck: string[]) => void;
  onPractice: () => void;
  onBattle: () => void;
  loading: boolean;
  busy: boolean;
  error: string;
  onRetry: () => void;
};

export function DeckBuilder({ cards, deck, onChange, onPractice, onBattle, loading, busy, error, onRetry }: Props) {
  const [query, setQuery] = useState("");
  const [classFilter, setClassFilter] = useState("all");
  const [typeFilter, setTypeFilter] = useState("all");
  const [costFilter, setCostFilter] = useState("all");
  const [availableOnly, setAvailableOnly] = useState(true);
  const [inspected, setInspected] = useState<CatalogCard | null>(null);
  const [evolvedArt, setEvolvedArt] = useState(false);
  const inspect = (card: CatalogCard) => { setEvolvedArt(false); setInspected(card); };
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (!inspected) return;
    const opener = document.activeElement as HTMLElement | null;
    const node = dialog.current;
    node?.showModal();
    return () => { node?.close(); opener?.focus(); };
  }, [inspected]);
  const byId = useMemo(() => new Map(cards.map((card) => [String(card.id), card])), [cards]);
  const counts = new Map<string, number>();
  for (const id of deck) counts.set(id, (counts.get(id) || 0) + 1);
  const deckClass = deck.map((id) => byId.get(id)?.class).find((value) => value && value !== "neutral");
  const problems = cards.length ? deckProblems(deck, cards) : [];
  const filtered = cards.filter((card) => (!availableOnly || card.deckLegal)
    && (classFilter === "all" || card.class === classFilter || card.class === "neutral")
    && (typeFilter === "all" || card.cardType === typeFilter)
    && (costFilter === "all" || Math.min(card.cost, 10) === Number(costFilter))
    && `${card.name} ${card.id} ${card.text}`.toLowerCase().includes(query.trim().toLowerCase()))
    .sort((a, b) => a.cost - b.cost || a.id - b.id);
  const rows = [...counts].sort(([a], [b]) => (byId.get(a)?.cost || 0) - (byId.get(b)?.cost || 0) || Number(a) - Number(b));
  const curve = Array.from({ length: 11 }, (_, cost) => deck.filter((id) => Math.min(byId.get(id)?.cost ?? -1, 10) === cost).length);
  const totalCost = deck.reduce((sum, id) => sum + (byId.get(id)?.cost || 0), 0);
  const reason = (card: CatalogCard) => !card.deckLegal ? "无法编入牌组" : deck.length >= 40 ? "牌组已满"
    : (counts.get(String(card.id)) || 0) >= 3 ? "已达 3 张上限"
    : deckClass && card.class !== "neutral" && card.class !== deckClass ? "职业不符" : "";
  const add = (card: CatalogCard) => { if (!reason(card)) onChange([...deck, String(card.id)]); };
  const remove = (id: string) => { const index = deck.indexOf(id); if (index >= 0) onChange(deck.filter((_, n) => n !== index)); };

  return <section className="workspace-page deck-workspace">
    <div className="deck-heading"><div><span className="eyebrow">COLLECTION</span><h1>卡组构筑</h1></div>
      <button className="primary-action" onClick={onPractice} disabled={loading || !cards.length}><Layers size={17}/>练习牌组</button>
    </div>
    {error && <div className="deck-load-error" role="alert">{error}<button onClick={onRetry}>重试</button></div>}
    <div className="deck-layout">
      <div className="collection-pane">
        <div className="collection-filters">
          <label className="card-search"><Search size={17}/><input aria-label="搜索卡牌" placeholder="搜索卡名、文本或编号" value={query} onChange={(event) => setQuery(event.target.value)}/></label>
          <select aria-label="职业筛选" value={classFilter} onChange={(event) => setClassFilter(event.target.value)}><option value="all">全部职业</option>{Object.entries(classNames).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select>
          <select aria-label="类型筛选" value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}><option value="all">全部类型</option>{Object.entries(typeNames).map(([id, name]) => <option key={id} value={id}>{name}</option>)}</select>
          <select aria-label="费用筛选" value={costFilter} onChange={(event) => setCostFilter(event.target.value)}><option value="all">全部费用</option>{curve.map((_, cost) => <option value={cost} key={cost}>{cost === 10 ? "10+" : cost} 费</option>)}</select>
          <label className="available-filter"><input type="checkbox" checked={availableOnly} onChange={(event) => setAvailableOnly(event.target.checked)}/>可组牌</label>
        </div>
        <div className="collection-count" aria-live="polite">{loading ? "正在加载卡池" : `${filtered.length} 张卡牌`}</div>
        <div className="collection-grid">
          {filtered.map((card) => <article className={`collection-card rarity-${card.rarity}`} key={card.id}>
            <button className="collection-inspect" onClick={() => inspect(card)} aria-label={`查看 ${card.name}`}>
              <div className="collection-art"><CardArt src={cardArt(card.id)} loading="lazy"/><span className="collection-cost">{card.cost}</span></div>
              <strong>{card.name}</strong><small>{classNames[card.class]} · {typeNames[card.cardType]}{card.attack !== undefined ? ` · ${card.attack}/${card.life}` : ""}</small>
            </button>
            <div className="collection-stepper"><button className="deck-icon-button" aria-label={`移除 ${card.name}`} title="移除一张" onClick={() => remove(String(card.id))} disabled={!counts.has(String(card.id))}><Minus size={16}/></button><span>{counts.get(String(card.id)) || 0}/3</span><button className="deck-icon-button" aria-label={`添加 ${card.name}`} title={reason(card) || "添加一张"} disabled={!!reason(card)} onClick={() => add(card)}><Plus size={16}/></button></div>
          </article>)}
        </div>
        {!loading && !filtered.length && !error && <p className="collection-empty">没有匹配的卡牌</p>}
      </div>
      <aside className="deck-summary" aria-label="当前牌组">
        <div className="deck-summary-title"><div><h2>我的牌组</h2><span>{classNames[deckClass || "neutral"]} · 平均 {deck.length ? (totalCost / deck.length).toFixed(1) : "0.0"} 费</span></div><strong className={deck.length !== 40 ? "incomplete" : ""}>{deck.length}<small>/40</small></strong><button className="deck-icon-button" title="清空牌组" aria-label="清空牌组" onClick={() => onChange([])} disabled={!deck.length}><Trash2 size={17}/></button></div>
        <div className="deck-curve" aria-label="费用分布">{curve.map((count, cost) => <div key={cost} title={`${cost === 10 ? "10+" : cost} 费：${count} 张`}><span>{count || ""}</span><i style={{ height: `${count / Math.max(1, ...curve) * 36}px` }}/><small>{cost === 10 ? "10+" : cost}</small></div>)}</div>
        <div className="deck-validation" aria-live="polite">{problems.length ? problems.map((problem) => <p key={problem}>{problem}</p>) : cards.length ? <span><Check size={15}/>可以对战</span> : <span>卡池尚未就绪</span>}</div>
        <button className="deck-battle-button" onClick={onBattle} disabled={busy || loading || !cards.length || !!problems.length}><Swords size={17}/>{busy ? "正在创建房间" : "使用牌组创建房间"}</button>
        <div className="deck-rows">{rows.map(([id, count]) => { const card = byId.get(id); return <div className="deck-row" key={id}>
          <span className="deck-row-cost">{card?.cost ?? "?"}</span><button className="deck-row-name" onClick={() => card && inspect(card)}>{card?.name || `卡牌 ${id}`}<small>{card ? typeNames[card.cardType] : "不在卡池中"}</small></button><button className="deck-icon-button" aria-label={`牌组移除 ${card?.name || id}`} title="移除一张" onClick={() => remove(id)}><Minus size={14}/></button><b>{count}</b><button className="deck-icon-button" aria-label={`牌组添加 ${card?.name || id}`} title={card ? reason(card) || "添加一张" : "不在卡池中"} onClick={() => card && add(card)} disabled={!card || !!reason(card)}><Plus size={14}/></button>
        </div>; })}</div>
      </aside>
    </div>
    {inspected && <dialog className="deck-modal" ref={dialog} aria-label={inspected.name} onCancel={() => setInspected(null)} onKeyDown={(event) => {
      if (event.key !== "Tab") return;
      const buttons = event.currentTarget.querySelectorAll<HTMLButtonElement>("button:not(:disabled)");
      const first = buttons[0], last = buttons[buttons.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }} onClick={(event) => { if (event.target === event.currentTarget) setInspected(null); }}>
      <div className="deck-card-detail">
        <button className="deck-icon-button detail-close" title="关闭" aria-label="关闭卡牌详情" onClick={() => setInspected(null)}><X size={18}/></button>
        <div className="deck-card-media">
          <div className="deck-detail-art"><CardArt src={cardArt(inspected.id, evolvedArt)} alt={inspected.name}/></div>
          {hasEvolvedArt(inspected.id) && <div className="art-form-control" role="group" aria-label="卡图形态"><button aria-pressed={!evolvedArt} onClick={() => setEvolvedArt(false)}>进化前</button><button aria-pressed={evolvedArt} onClick={() => setEvolvedArt(true)}>进化后</button></div>}
        </div>
        <div><small>{classNames[inspected.class]} · {typeNames[inspected.cardType]} · {inspected.cost} 费</small><h2>{inspected.name}</h2>{inspected.attack !== undefined && <strong>基础 {inspected.attack}/{inspected.life}</strong>}<CounterValues counters={inspected.counters}/><p>{inspected.text || "无额外能力"}</p><small>{inspected.id}{!inspected.deckLegal ? " · 无法编入牌组" : ""}</small></div>
      </div>
    </dialog>}
  </section>;
}
