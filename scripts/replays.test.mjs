import assert from 'node:assert/strict';
import test from 'node:test';
import { eventLabel, frameEvents, mergeReplay, readReplays, persistReplays } from '../web/src/replays.ts';

const player = () => ({ leaderLife: 20, leaderMax: 20, pp: 0, maxpp: 0, ep: 2, sep: 2, shadows: 0, hand: [], field: [], graveyard: [] });
const state = (revision = 0, viewer = 'own') => ({ revision, viewer, own: player(), oppo: player(), turn: { number: 1, active: 'own' } });
const catalog = { 1: { name: 'A' }, 2: { name: 'B' } };
const fixture = () => ({ matchId: 'room', state: state(2), events: [{ Kind: 'turn_started', Side: 'own' }, { Kind: 'card_drawn', Side: 'own', Count: 1 }], frames: [{ revision: 0, eventCount: 0, state: state() }, { revision: 1, eventCount: 1, state: state(1) }, { revision: 2, eventCount: 2, state: state(2) }] });

test('frame boundaries, repeated polls and seat identities preserve exact history', () => {
  const data = fixture();
  const records = mergeReplay([], data, catalog);
  assert.deepEqual(frameEvents(records[0], 0), []);
  assert.deepEqual(frameEvents(records[0], 1), [data.events[0]]);
  assert.deepEqual(frameEvents(records[0], 2), [data.events[1]]);
  for (let n = 0; n < 10; n++) assert.equal(mergeReplay(records, structuredClone(data), catalog), records);
  const guest = structuredClone(data);
  guest.state.viewer = 'oppo';
  guest.frames.forEach(frame => frame.state.viewer = 'oppo');
  const both = mergeReplay(records, guest, catalog);
  assert.equal(both.length, 2);
  assert.notEqual(both[0].id, both[1].id);
  assert.equal(both[0].events[0], '对手回合开始');
  const initial = mergeReplay([], { ...data, events: [], frames: data.frames.slice(0, 1) }, catalog);
  assert.equal(initial[0].frames.length, 1);
});

test('labels resolve uppercase payloads, removed cards, leaders and guest perspective', () => {
  const before = state(1, 'oppo');
  before.oppo.field.push({ instanceId: 'a', cardId: 1 });
  const after = state(2, 'oppo');
  assert.equal(eventLabel({ Kind: 'destroyed', InstanceID: '', Subject: { kind: 'instance', instanceId: 'a' } }, { state: after }, catalog, before), 'A 被破坏');
  assert.equal(eventLabel({ Kind: 'damaged', Actual: 0, Target: { kind: 'leader', side: 'own' } }, { state: after }, catalog), '对手主战者 受到 0 点伤害');
  assert.equal(eventLabel({ Kind: 'attacked', InstanceID: '', Attacker: { kind: 'instance', cardId: 1 }, Defender: { kind: 'leader', side: 'oppo' } }, { state: after }, catalog), 'A 攻击 我方主战者');
  assert.equal(eventLabel({ Kind: 'turn_started', Side: 'oppo' }, { state: after }, catalog), '我方回合开始');
  after.own.graveyard.push({ instanceId: 'b', cardId: 2 });
  assert.equal(eventLabel({ Kind: 'destroyed', Subject: { kind: 'instance', instanceId: 'b' } }, { state: after }, catalog), 'B 被破坏');
});

test('fusion and transformation labels use event identities and respect redacted records', () => {
  const after = state();
  after.own.hand.push({ instanceId: 'source', cardId: 2 });
  const fused = { Kind: 'card_fused', Side: 'own', Count: 2, Subject: { kind: 'instance', instanceId: 'source', cardId: 1 } };
  const transformed = { Kind: 'card_transformed', Side: 'own', Subject: { kind: 'instance', instanceId: 'source', cardId: 1 }, Target: { kind: 'instance', instanceId: 'source', cardId: 2 } };
  assert.equal(eventLabel(fused, { state: after }, catalog), 'A 融合 2 张材料');
  assert.equal(eventLabel(transformed, { state: after }, catalog), 'A 变身为 B');
  after.viewer = 'oppo';
  assert.equal(eventLabel({ Kind: 'card_fused', Side: 'own', Count: 2 }, { state: after }, catalog), '对手融合 2 张材料');
  assert.equal(eventLabel({ Kind: 'card_transformed', Side: 'own' }, { state: after }, catalog), '对手卡牌变身');
  assert.equal(eventLabel({ Kind: 'pp_restored', Side: 'own', Actual: 8 }, { state: after }, catalog), '对手回复 8 点能量');
  assert.equal(eventLabel({ Kind: 'pp_restored', Side: 'oppo', Actual: 0 }, { state: after }, catalog), '我方回复 0 点能量');
});

test('all events survive local storage, including histories longer than 200 events', () => {
  const data = fixture();
  data.events = Array.from({ length: 250 }, () => ({ Kind: 'card_drawn', Count: 1 }));
  data.frames = [{ revision: 2, eventCount: 250, state: data.state }];
  const records = mergeReplay([], data, catalog);
  let json;
  assert(persistReplays(records, { setItem: (_, value) => { json = value; } }));
  const restored = readReplays({ getItem: () => json });
  assert.deepEqual(restored, JSON.parse(JSON.stringify(records)));
  assert.equal(frameEvents(restored[0], 0).length, 250);
});

test('discard labels preserve the publicly discarded identity and viewer side', () => {
  const current = state();
  current.own.graveyard.push({ instanceId: 'discarded', cardId: 2 });
  const event = { kind: 'card_discarded', side: 'own', subject: { kind: 'instance', instanceId: 'discarded', cardId: 1 } };
  assert.equal(eventLabel(event, { state: current }, catalog), '我方舍弃 A');
  current.viewer = 'oppo';
  assert.equal(eventLabel(event, { state: current }, catalog), '对手舍弃 A');
});

test('legacy and malformed storage cannot manufacture frame events or crash loading', () => {
  const records = readReplays({ getItem: () => JSON.stringify(['old log', null, { id: 'bad', events: [] }, { id: 'legacy', matchId: 'room', events: ['ok', {}], frames: [{ revision: 1, state: null }] }]) });
  assert.equal(records.length, 2);
  assert.deepEqual(records[1].events, ['ok']);
  assert.deepEqual(records[1].frames, []);
  assert.equal(frameEvents(records[0], 0), undefined);
  assert.deepEqual(readReplays({ getItem: () => '{broken' }), []);
  const record = mergeReplay([], fixture(), catalog)[0];
  record.frames[1].eventCount = 2000;
  assert.equal(frameEvents(record, 1), undefined);
});

test('storage access and quota failures are reported without throwing or losing in-memory records', () => {
  const records = mergeReplay([], fixture(), catalog);
  assert.equal(persistReplays(records, { setItem: () => { throw new Error('quota'); } }), false);
  assert.equal(records[0].frames.length, 3);
  assert.deepEqual(readReplays({ getItem: () => { throw new Error('disabled'); } }), []);
});
