import { useEffect, useRef, useState } from "react";
import { addDeck, emptyLibrary, LIBRARY_KEY, LEGACY_KEY, newDeck, readLibrary, recoverLibrary, saveLibrary, type DeckLibrary, type LibrarySnapshot } from "./deckLibrary";

function read(): LibrarySnapshot {
  try { return readLibrary(localStorage); }
  catch { return { library: emptyLibrary(), raw: null, legacy: null, fresh: false, error: "无法读取本地存储；请允许网站使用存储后重试" }; }
}
export function useDeckLibrary() {
  const [snapshot, setSnapshot] = useState(read);
  const current = useRef(snapshot);
  const [error, setError] = useState("");
  const update = (next: LibrarySnapshot) => { current.current = next; setSnapshot(next); };
  const reload = () => { update(read()); setError(""); };
  const commit = (change: (library: DeckLibrary) => DeckLibrary): boolean => {
    try {
      update(saveLibrary(localStorage, current.current, change(current.current.library)));
      setError("");
      return true;
    } catch (reason) {
      setError(`未保存，本次修改未应用：${reason instanceof Error ? reason.message : "本地存储不可用"}`);
      return false;
    }
  };
  useEffect(() => {
    const listener = (event: StorageEvent) => {
      if (event.storageArea === localStorage && (event.key === LIBRARY_KEY || event.key === LEGACY_KEY || event.key === null)) reload();
    };
    window.addEventListener("storage", listener);
    return () => window.removeEventListener("storage", listener);
  }, []);
  const initialize = (cards: string[]) => {
    if (current.current.error || current.current.raw !== null) return;
    commit((library) => current.current.fresh ? { ...library, decks: [{ ...library.decks[0], name: "练习牌组", cards: [...cards] }] } : library);
  };
  const practice = (cards: string[]) => commit((library) => {
    const existing = library.decks.find((deck) => deck.name === "练习牌组" && JSON.stringify(deck.cards) === JSON.stringify(cards));
    return existing ? { ...library, activeId: existing.id } : addDeck(library, newDeck("练习牌组", cards));
  });
  const recover = () => {
    try { update(recoverLibrary(localStorage, current.current)); setError(""); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "备份失败，原数据已保留"); }
  };
  return { library: snapshot.library, active: snapshot.library.decks.find((deck) => deck.id === snapshot.library.activeId)!, error: error || snapshot.error, blocked: !!snapshot.error, damagedData: snapshot.error && (snapshot.raw !== null || snapshot.legacy !== null) ? JSON.stringify({ library: snapshot.raw, legacy: snapshot.legacy }, null, 2) : null, recover, commit, reload, initialize, practice };
}
