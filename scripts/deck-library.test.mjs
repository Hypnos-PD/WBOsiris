import assert from 'node:assert/strict';
import test from 'node:test';
import { addDeck, emptyLibrary, exportDeck, importDeck, LIBRARY_KEY, LEGACY_KEY, newDeck, parseLibrary, readLibrary, recoverLibrary, saveLibrary } from '../web/src/deckLibrary.ts';

function storage(entries = {}) {
  const data = new Map(Object.entries(entries));
  return { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) };
}

test('legacy decks migrate without changing order, duplicates, unknown IDs or the original data', () => {
  for (const cards of [[], [10001110, '99999999', 10001110], Array(41).fill(10001110)]) {
    const raw = JSON.stringify(cards), local = storage({ [LEGACY_KEY]: raw });
    const snapshot = readLibrary(local);
    assert.equal(snapshot.fresh, false);
    assert.deepEqual(snapshot.library.decks[0].cards, cards.map(String));
    saveLibrary(local, snapshot, snapshot.library);
    assert.equal(local.getItem(LEGACY_KEY), raw);
    assert.deepEqual(readLibrary(local).library, snapshot.library);
  }
});

test('new visitors are distinct from intentionally empty saved decks', () => {
  assert.equal(readLibrary(storage()).fresh, true);
  assert.equal(readLibrary(storage({ [LEGACY_KEY]: '[]' })).fresh, false);
  const local = storage(), snapshot = readLibrary(local);
  saveLibrary(local, snapshot, snapshot.library);
  assert.equal(readLibrary(local).fresh, false);
});

test('malformed and future storage is preserved and cannot be silently overwritten', () => {
  for (const [key, raw] of [[LIBRARY_KEY, '{'], [LIBRARY_KEY, '{"version":2}'], [LEGACY_KEY, '{}'], [LEGACY_KEY, '[null]']]) {
    const local = storage({ [key]: raw }), snapshot = readLibrary(local);
    assert.ok(snapshot.error);
    assert.throws(() => saveLibrary(local, snapshot, emptyLibrary()));
    assert.equal(local.getItem(key), raw);
    if (key === LEGACY_KEY) assert.equal(local.getItem(LIBRARY_KEY), null);
  }
});

test('failed reads and writes preserve the previous library', () => {
  const local = storage(), snapshot = readLibrary(local);
  const next = saveLibrary(local, snapshot, snapshot.library);
  const raw = local.getItem(LIBRARY_KEY);
  assert.ok(readLibrary({ getItem() { throw new Error('denied'); } }).error);
  assert.throws(() => saveLibrary({ ...local, setItem() { throw new Error('quota'); } }, next, addDeck(next.library, newDeck('Draft'))), /quota/);
  assert.equal(local.getItem(LIBRARY_KEY), raw);
  assert.equal(next.library.decks.length, 1);
});

test('stale tabs and legacy migration reject newer stored data', () => {
  const local = storage(), a = readLibrary(local), b = readLibrary(local);
  const winner = saveLibrary(local, a, addDeck(a.library, newDeck('Other tab')));
  assert.throws(() => saveLibrary(local, b, b.library), /其他页面/);
  assert.deepEqual(readLibrary(local).library, winner.library);
  const legacy = storage({ [LEGACY_KEY]: '[]' }), old = readLibrary(legacy);
  legacy.setItem(LEGACY_KEY, '[10001110]');
  assert.throws(() => saveLibrary(legacy, old, old.library), /其他页面/);
});

test('deck export roundtrip keeps drafts and gives imports independent identity and cards', () => {
  const original = newDeck('测试牌组', ['10001110', '10001110', '99999999']);
  const imported = importDeck(exportDeck(original));
  assert.notEqual(imported.id, original.id);
  assert.equal(imported.name, original.name);
  assert.deepEqual(imported.cards, original.cards);
  imported.cards.pop();
  assert.equal(original.cards.length, 3);
  const library = addDeck(emptyLibrary(), original);
  assert.equal(library.activeId, original.id);
  assert.deepEqual(parseLibrary(JSON.stringify(library)), library);
});

test('imports reject invalid identifiers, versions, names and oversized input', () => {
  const valid = { format: 'wbo-deck', version: 1, name: 'Draft', cards: [] };
  for (const change of [{ version: 2 }, { format: 'other' }, { name: ' ' }, { name: 'x'.repeat(61) }, { cards: [0] }, { cards: [-1] }, { cards: [1.2] }, { cards: [null] }, { cards: ['01'] }, { cards: ['1e2'] }, { cards: [Number.MAX_SAFE_INTEGER + 1] }, { cards: Array(1001).fill(1) }]) {
    assert.throws(() => importDeck(JSON.stringify({ ...valid, ...change })));
  }
  assert.throws(() => importDeck(' '.repeat(100001)), /100 KB/);
  assert.throws(() => importDeck('not JSON'));
});

test('library validates active identity, unique IDs and capacity', () => {
  const value = emptyLibrary();
  assert.throws(() => parseLibrary(JSON.stringify({ ...value, activeId: 'missing' })));
  assert.throws(() => parseLibrary(JSON.stringify({ ...value, decks: [...value.decks, value.decks[0]] })));
  assert.throws(() => parseLibrary(JSON.stringify({ ...value, decks: [] })));
  while (value.decks.length < 100) value.decks.push(newDeck());
  assert.throws(() => addDeck(value, newDeck()), /100/);
});

test('recovery backs up exact original bytes before replacing damaged data, and stops on backup failure', () => {
  for (const key of [LIBRARY_KEY, LEGACY_KEY]) {
    const raw = '{broken', local = storage({ [key]: raw }), snapshot = readLibrary(local), writes = [];
    const recovered = recoverLibrary({ ...local, setItem(k, v) { writes.push([k, v]); local.setItem(k, v); } }, snapshot);
    assert.ok(writes[0][0].startsWith(`${LIBRARY_KEY}-backup-`));
    assert.equal(JSON.parse(writes[0][1])[key === LIBRARY_KEY ? 'library' : 'legacy'], raw);
    assert.equal(writes[1][0], LIBRARY_KEY);
    assert.equal(recovered.library.decks[0].cards.length, 0);
    assert.equal(readLibrary(local).error, '');
    const failing = storage({ [key]: raw });
    assert.throws(() => recoverLibrary({ ...failing, setItem() { throw new Error('full'); } }, readLibrary(failing)), /full/);
    assert.equal(failing.getItem(key), raw);
  }
});
