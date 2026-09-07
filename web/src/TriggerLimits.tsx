import type { Entity } from "./gameTypes";
import "./triggerLimits.css";

export function TriggerLimits({ limits }: { limits?: Entity["triggerLimits"] }) {
  if (!limits?.length) return null;
  const scopes = { own: "持有者回合", oppo: "对方回合", any: "每个回合" };
  return <dl className="trigger-limits" aria-label="每回合触发次数">
    {limits.map((limit, index) => <div key={limit.abilityId}>
      <dt>{scopes[limit.turnScope]}{limits.length > 1 ? ` · 能力 ${index + 1}` : ""}</dt>
      <dd><b>{limit.used ? 1 : 0}/1</b> 已发动</dd>
    </div>)}
  </dl>;
}
