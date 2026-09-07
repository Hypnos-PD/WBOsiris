import { Combine } from "lucide-react";
import { CardArt } from "./CardArt";
import { cardArt } from "./decks";
import type { FusionState } from "./gameTypes";
import "./fusion.css";

export function FusionDetails({ fusion, catalog }: { fusion?: FusionState; catalog: Record<string, { name: string }> }) {
  if (!fusion) return null;
  return <section className="fusion-details" aria-label="融合记录">
    <div className="fusion-heading"><strong><Combine size={15} aria-hidden="true"/>融合材料</strong>
      {fusion.enabled && <span className={fusion.usedThisTurn ? "fusion-used" : ""}>{fusion.usedThisTurn ? "本回合已融合" : "本回合未融合"}</span>}
    </div>
    <dl className="fusion-totals"><div><dt>张数</dt><dd>{fusion.materials.length}</dd></div><div><dt>费用合计</dt><dd>{fusion.totalCost}</dd></div><div><dt>种类</dt><dd>{fusion.distinctKinds}</dd></div></dl>
    {fusion.materials.length > 0 ? <ul className="fusion-materials">{fusion.materials.map((material, index) => <li key={index}>
      <CardArt src={cardArt(material.cardId)} alt=""/>
      <span>{catalog[String(material.cardId)]?.name || `卡牌 ${material.cardId}`}</span><b aria-label={`原始费用 ${material.cost}`}>{material.cost}</b>
    </li>)}</ul> : <p className="fusion-empty">暂无材料</p>}
  </section>;
}
