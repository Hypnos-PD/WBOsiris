import { useEffect, useRef, useState } from "react";
import { addDeck, deckName, exportDeck, importDeck, newDeck, type SavedDeck } from "./deckLibrary";
import type { useDeckLibrary } from "./useDeckLibrary";

function downloadFile(text: string, filename: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function DeckLibraryBar({ store }: { store: ReturnType<typeof useDeckLibrary> }) {
  const { library, active, commit } = store;
  const [name, setName] = useState(active.name);
  const [notice, setNotice] = useState("");
  const [inputError, setInputError] = useState("");
  const [deleted, setDeleted] = useState<SavedDeck | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  useEffect(() => { setName(active.name); setInputError(""); setNotice(""); }, [active.id, active.name]);
  const create = (deck: SavedDeck) => commit((value) => addDeck(value, deck));
  const download = () => {
    downloadFile(exportDeck(active), `${active.name.replace(/[\\/:*?"<>|\x00-\x1f]/g, "_")}.wbo-deck.json`);
    setNotice("已导出当前牌组");
  };
  return <div className="deck-library" aria-label="牌组库">
    <div className="library-selection">
      <label>当前牌组<select aria-label="选择牌组" value={active.id} disabled={store.blocked} onChange={(event) => commit((value) => ({ ...value, activeId: event.target.value }))}>{library.decks.map((deck) => <option key={deck.id} value={deck.id}>{deck.name} · {deck.cards.length}/40</option>)}</select></label>
      <span className="library-save-status">{store.error ? "本地保存需处理" : `${library.decks.length} 副牌组 · 修改自动保存`}</span>
    </div>
    <form className="library-rename" onSubmit={(event) => {
      event.preventDefault();
      try {
        const nextName = deckName(name);
        if (commit((value) => ({ ...value, decks: value.decks.map((deck) => deck.id === active.id ? { ...deck, name: nextName } : deck) }))) { setName(nextName); setInputError(""); }
      } catch (error) { setInputError(error instanceof Error ? error.message : "名称无效"); }
    }}>
      <input aria-label="牌组名称" value={name} maxLength={60} disabled={store.blocked} onChange={(event) => setName(event.target.value)} />
      <button disabled={store.blocked || name === active.name}>重命名</button>
    </form>
    <div className="library-actions">
      <button disabled={store.blocked} onClick={() => create(newDeck())}>新建牌组</button>
      <button disabled={store.blocked} onClick={() => create(newDeck(`${active.name.slice(0, 55)} 副本`, active.cards))}>复制牌组</button>
      <button disabled={store.blocked} onClick={download}>导出牌组</button>
      <button disabled={store.blocked} onClick={() => fileInput.current?.click()}>导入牌组</button>
      <input ref={fileInput} type="file" hidden accept=".json,application/json" aria-label="导入牌组文件" onChange={async (event) => {
        const file = event.target.files?.[0];
        event.target.value = "";
        if (!file) return;
        try {
          if (file.size > 100_000) throw new Error("牌组文件超过 100 KB");
          const deck = importDeck(await file.text());
          if (create(deck)) setInputError("");
        } catch (error) { setInputError(`导入失败：${error instanceof Error ? error.message : "无法读取文件"}`); }
      }} />
      <button disabled={store.blocked || library.decks.length === 1} onClick={() => {
        if (commit((value) => {
          const decks = value.decks.filter((deck) => deck.id !== active.id);
          return { ...value, activeId: decks[0].id, decks };
        })) setDeleted(active);
      }}>删除牌组</button>
    </div>
    {deleted && <div className="library-message" role="status">已删除「{deleted.name}」<button onClick={() => { if (create(deleted)) setDeleted(null); }}>撤销删除</button></div>}
    {(store.error || inputError) && <div className="library-message library-error" role="alert">{store.error || inputError}{store.error && <button onClick={store.reload}>重新读取牌组库</button>}</div>}
    {store.damagedData && <details className="library-recovery"><summary>恢复异常牌组库</summary><p>可先下载原始数据。重建会在本机备份原数据，再创建空牌组库。</p><div className="library-actions"><button onClick={() => downloadFile(store.damagedData!, "wbo-deck-library-backup.json")}>下载原始数据</button><button onClick={store.recover}>备份并重建空牌组库</button></div></details>}
    {notice && <div className="library-message" role="status">{notice}</div>}
  </div>;
}
