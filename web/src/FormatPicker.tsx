import type { CatalogFormat } from "./decks";

/**
 * 赛制选择：只在"开一局"的地方出现（大厅建房、单人模式）。
 * 卡组页的那套选择是构筑时过滤卡池用的，与这里的对局赛制分开。
 */
export function FormatPicker({ value, onChange, formats, label = "本局赛制" }: {
  value: string;
  onChange: (value: string) => void;
  formats: CatalogFormat[];
  label?: string;
}) {
  const options = formats.length ? formats : [
    { id: "rotation", name: "指定模式", packs: [] },
    { id: "unlimited", name: "无限制模式", packs: [] },
  ];
  return (
    <div className="format-picker" role="radiogroup" aria-label={label}>
      <span className="format-picker-label">{label}</span>
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          role="radio"
          aria-checked={value === option.id}
          className={`format-option ${value === option.id ? "active" : ""}`}
          onClick={() => onChange(option.id)}
        >
          <strong>{option.name}</strong>
          <small>{option.id === "unlimited" ? "全部卡包" : option.packs.length ? `基础卡牌 + 最新 ${option.packs.length - 1} 弹` : "最新卡包"}</small>
        </button>
      ))}
    </div>
  );
}
