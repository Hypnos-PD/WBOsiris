import assert from 'node:assert/strict';
import test from 'node:test';
import { invitationURL, matchKey, readMatchAuth, readSavedMatches, saveMatchAuth } from '../web/src/matchStorage.ts';

function storage(entries = {}) {
  const data = new Map(Object.entries(entries));
  return { get length() { return data.size; }, key: i => [...data.keys()][i] ?? null, getItem: k => data.get(k) ?? null, setItem: (k, v) => data.set(k, v) };
}
const host = { id: '0123456789ab', side: 'own', token: 'a'.repeat(48) };
const guest = { ...host, side: 'oppo', token: 'b'.repeat(48) };

test('existing credentials recover both seats and explicit seat selection never falls back to another player', () => {
  const local = storage({ [matchKey(host)]: JSON.stringify(host), [matchKey(guest)]: JSON.stringify(guest) });
  assert.equal(readSavedMatches(local).length, 2);
  assert.deepEqual(readMatchAuth(local, host.id), { ...host, updatedAt: 0 });
  assert.deepEqual(readMatchAuth(local, host.id, 'oppo'), { ...guest, updatedAt: 0 });
  const guestOnly = storage({ [matchKey(guest)]: JSON.stringify(guest) });
  assert.equal(readMatchAuth(guestOnly, host.id, 'own'), null);
  assert.equal(readMatchAuth(guestOnly, host.id).side, 'oppo');
});

test('malformed host credentials cannot hide a valid guest and are left intact', () => {
  const local = storage({ [matchKey(host)]: '{bad', [matchKey(guest)]: JSON.stringify(guest), 'unrelated': 'other' });
  assert.equal(readMatchAuth(local, host.id).side, 'oppo');
  assert.equal(readSavedMatches(local).length, 1);
  assert.equal(local.getItem(matchKey(host)), '{bad');
  for (const bad of [{ ...host, id: '../other' }, { ...host, id: '000000000000' }, { ...host, token: '' }, { ...host, side: 'oppo' }]) {
    assert.equal(readSavedMatches(storage({ [matchKey(host)]: JSON.stringify(bad) })).length, 0);
  }
});

test('saving timestamps a seat without deleting other seats or serializing response data', () => {
  const local = storage({ [matchKey(guest)]: JSON.stringify(guest) });
  saveMatchAuth(local, { ...host, joinCode: 'not-persisted', state: { hidden: true } });
  assert.equal(local.length, 2);
  const saved = JSON.parse(local.getItem(matchKey(host)));
  assert.deepEqual(Object.keys(saved).sort(), ['id', 'side', 'token', 'updatedAt']);
  assert(saved.updatedAt > 0);
  assert.equal(readSavedMatches(local)[0].side, 'own');
  assert.throws(() => saveMatchAuth({ setItem() { throw new Error('quota'); } }, host), /quota/);
  assert.throws(() => readSavedMatches({ length: 1, key() { throw new Error('denied'); } }), /denied/);
});

test('invitation URLs retain app location and contain only room ID and join code', () => {
  const link = new URL(invitationURL('https://example.test/play/?seat=own&page=rooms&token=secret#private', host.id, 'invite code'));
  assert.equal(link.pathname, '/play/');
  assert.deepEqual([...link.searchParams], [['match', host.id], ['join', 'invite code']]);
  assert.equal(link.hash, '');
  assert(!link.toString().includes('secret'));
});
