import "./counters.css";

export function CounterValues({ counters, compact = false }: { counters?: Record<string, number>; compact?: boolean }) {
  const entries = Object.entries(counters || {}).filter(([name]) => !compact || name === "x").sort(([a], [b]) => a.localeCompare(b));
  if (!entries.length) return null;
  return <span className={`card-counters${compact ? " compact" : ""}`}>
    {entries.map(([name, value]) => <span key={name} aria-label={`${name.toUpperCase()} ${value}`}><span>{name.toUpperCase()}</span><b>{value}</b></span>)}
  </span>;
}
