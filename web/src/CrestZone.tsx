import { CardArt } from "./CardArt";
import { cardArt, cardText } from "./decks";
import type { Entity } from "./gameTypes";
import "./crests.css";

export const crestName = (entity: Entity) => cardText(entity.crestLocales?.chs?.name || `纹章 ${entity.cardId}`);

export function CrestZone({ crests = [], label, onInspect }: { crests?: Entity[]; label: string; onInspect: (entity: Entity) => void }) {
  return <div className="crest-zone" aria-label={`${label}纹章`}>
    {Array.from({ length: 5 }, (_, index) => {
      const crest = crests[index];
      if (!crest) return <i className="crest-empty" key={`empty-${index}`} aria-hidden="true"/>;
      const name = crestName(crest);
      const countdown = crest.countdown ? ` · 吟唱 ${crest.countdown}` : "";
      return <button className="crest-token" key={crest.instanceId} data-instance-id={crest.instanceId} title={name + countdown} aria-label={`查看 ${name}${countdown}`} onClick={() => onInspect(crest)}>
        <CardArt src={cardArt(crest.cardId)} alt=""/>
        {!!crest.countdown && <b className="crest-countdown">{crest.countdown}</b>}
      </button>;
    })}
  </div>;
}
