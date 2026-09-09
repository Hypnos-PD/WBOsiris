import assert from 'node:assert/strict';
import test from 'node:test';
import { MatchConnection, decodeRemote } from '../web/src/matchConnection.ts';

const player = () => ({ pp: 1, maxpp: 1, leaderLife: 20, field: [], hand: [] });
const state = (revision = 0, extra = {}) => ({ matchId: 'room', side: 'own', state: { revision, viewer: 'own', own: player(), oppo: player(), turn: { active: 'own', number: 1 } }, legalActions: [], events: [], ...extra });
const tick = () => new Promise(resolve => setTimeout(resolve, 2));

test('opening order is preserved in each viewer perspective and invalid values are rejected', () => {
  const auth = { id: 'room', token: 'token', side: 'own' };
  for (const firstPlayer of ['own', 'oppo', undefined]) {
    const value = state();
    value.state.firstPlayer = firstPlayer;
    assert.equal(decodeRemote(value, auth).state.firstPlayer, firstPlayer);
  }
  for (const firstPlayer of ['host', 'guest', '', null, 1]) {
    const value = state();
    value.state.firstPlayer = firstPlayer;
    assert.throws(() => decodeRemote(value, auth), /对局响应无效/);
  }
});
async function until(predicate) {
  for (let n = 0; n < 500; n++) { if (predicate()) return; await tick(); }
  assert.fail('condition did not settle');
}
function fixture(t, options = {}) {
  const calls = [], frames = [], statuses = [], errors = [];
  const connection = new MatchConnection({
    base: 'http://test', auth: { id: 'room', token: 'token', side: 'own' },
    interval: 10, timeout: 1000,
    onRemote: value => frames.push(value), onStatus: value => statuses.push(value), onError: value => errors.push(value),
    fetch: (url, init) => new Promise((resolve, reject) => calls.push({ url, ...init, reply: (data, status = 200) => resolve(new Response(JSON.stringify(data), { status })), fail: reject })),
    ...options,
  });
  t.after(() => connection.stop());
  connection.start();
  return { connection, calls, frames, statuses, errors };
}

test('polls are serialized and late poll responses cannot roll back a submitted action', async t => {
  const f = fixture(t);
  await new Promise(resolve => setTimeout(resolve, 25));
  assert.equal(f.calls.length, 1);
  f.calls[0].reply(state(1));
  await until(() => f.calls.length === 2);
  const submit = f.connection.submit({ actionId: 'first', kind: 'play' });
  assert.equal(f.calls[1].signal.aborted, true);
  assert.equal(await f.connection.submit({ kind: 'end_turn' }), false);
  assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
  assert.equal(JSON.parse(f.calls[2].body).expectedRevision, 1);
  f.calls[2].reply(state(2, { result: { status: 'completed' } }));
  assert.equal(await submit, true);
  f.calls[1].reply(state(1));
  await tick();
  assert.deepEqual(f.frames.map(frame => frame.state.revision), [1, 2]);
  await until(() => f.calls.length === 4);
  f.calls[3].reply(state(0));
  await tick();
  assert.deepEqual(f.frames.map(frame => frame.state.revision), [1, 2]);
});

test('equal revisions can advance room readiness without a card-state mutation', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(0, { waiting: true, mulliganReady: false, events: null }));
  await until(() => f.calls.length === 2);
  f.calls[1].reply(state(0, { waiting: false, mulliganReady: true }));
  await until(() => f.frames.length === 2);
  assert.equal(f.frames.at(-1).mulliganReady, true);
  assert.equal(f.frames.at(-1).waiting, false);
  assert.deepEqual(f.frames[0].events, []);
});

test('restored host invitations follow room readiness at the same state revision', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(0, { waiting: true, joinCode: 'invite' }));
  await until(() => f.frames.length === 1);
  assert.equal(f.frames[0].joinCode, 'invite');
  await until(() => f.calls.length === 2);
  f.calls[1].reply(state(0, { waiting: false }));
  await until(() => f.frames.length === 2);
  assert.equal(f.frames[1].joinCode, undefined);
});

test('missing rooms and invalid credentials stop polling and cannot submit more commands', async t => {
  for (const status of [401, 404]) {
    for (const duringCommand of [false, true]) {
      const f = fixture(t);
      if (duringCommand) {
        f.calls[0].reply(state(1));
        await until(() => f.frames.length === 1);
        const submit = f.connection.submit({ kind: 'end_turn' });
        f.calls[1].reply('unavailable', status);
        assert.equal(await submit, false);
      } else f.calls[0].reply('unavailable', status);
      await until(() => f.statuses.at(-1) === 'unavailable');
      assert.match(f.errors[0], status === 404 ? /房间已不存在/ : /凭据已失效/);
      const calls = f.calls.length;
      await new Promise(resolve => setTimeout(resolve, 35));
      assert.equal(f.calls.length, calls);
      assert.equal(await f.connection.submit({ kind: 'end_turn' }), false);
    }
  }
});

test('a lost command response blocks further commands until a fresh read and never retries the write', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(1));
  await until(() => f.frames.length === 1);
  const submit = f.connection.submit({ kind: 'play' });
  f.calls[1].fail(new TypeError('Failed to fetch'));
  assert.equal(await submit, false);
  assert.equal(await f.connection.submit({ kind: 'play' }), false);
  assert.deepEqual(f.frames.map(frame => frame.state.revision), [1]);
  f.calls[2].reply(state(2));
  await until(() => f.statuses.at(-1) === 'connected');
  assert.equal(f.calls.filter(call => call.method === 'POST').length, 1);
  assert.equal(f.frames.at(-1).state.revision, 2);
  assert.equal(f.errors.length, 1);
});

test('HTTP errors, malformed responses and a wrong seat never replace the current board', async t => {
  for (const [payload, status] of [['conflict', 409], [{}, 200], [state(2, { side: 'oppo' }), 200]]) {
    const f = fixture(t);
    f.calls[0].reply(state(1));
    await until(() => f.frames.length === 1);
    const submit = f.connection.submit({ kind: 'play' });
    f.calls[1].reply(payload, status);
    assert.equal(await submit, false);
    assert.equal(f.frames.length, 1);
    assert.equal(f.errors.length, 1);
    f.connection.stop();
  }
});

test('illegal actions surface their rule code and retain the authoritative response', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(1));
  await until(() => f.frames.length === 1);
  const submit = f.connection.submit({ kind: 'play' });
  f.calls[1].reply(state(1, { result: { status: 'illegal', illegalCode: 'insufficient_pp' } }));
  assert.equal(await submit, false);
  assert.match(f.errors[0], /insufficient_pp/);
  assert.equal(f.frames.length, 2);
  assert.equal(f.statuses.at(-1), 'connected');
});

test('mulligan writes use the same lock and preserve the selected instance IDs', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(0));
  await until(() => f.frames.length === 1);
  const submit = f.connection.submit({ selectedInstanceIds: ['copy-2'] }, '/mulligan');
  assert.equal(await f.connection.submit({ selectedInstanceIds: ['copy-1'] }, '/mulligan'), false);
  assert.equal(f.calls[1].url, 'http://test/api/matches/room/mulligan');
  assert.deepEqual(JSON.parse(f.calls[1].body).selectedInstanceIds, ['copy-2']);
  assert.equal(JSON.parse(f.calls[1].body).expectedRevision, 0);
  f.calls[1].reply(state(0, { mulliganReady: true }));
  assert.equal(await submit, true);
});

test('stopping a connection discards a late response from a previous room', async t => {
  const f = fixture(t);
  f.connection.stop();
  assert(f.calls[0].signal.aborted);
  f.calls[0].reply(state(99));
  await tick();
  assert.equal(f.frames.length, 0);
  assert.equal(await f.connection.submit({ kind: 'play' }), false);
});

test('a stale command adopts the current state and a subsequent command uses its revision', async t => {
  const f = fixture(t);
  f.calls[0].reply(state(1));
  await until(() => f.frames.length === 1);
  const stale = f.connection.submit({ kind: 'end_turn', expectedRevision: 99 });
  assert.equal(JSON.parse(f.calls[1].body).expectedRevision, 1);
  f.calls[1].reply(state(3, { result: { status: 'rejected', errorCode: 'stale_state' } }));
  assert.equal(await stale, false);
  assert.equal(f.frames.at(-1).state.revision, 3);
  assert.equal(f.errors.at(-1), '局面已更新，操作未执行');
  const fresh = f.connection.submit({ kind: 'end_turn' });
  assert.equal(JSON.parse(f.calls[2].body).expectedRevision, 3);
  f.calls[2].reply(state(4, { result: { status: 'completed' } }));
  assert.equal(await fresh, true);
});

test('a timed-out poll reconnects and keeps writes disabled until a valid response', async t => {
  const f = fixture(t, { timeout: 10 });
  f.calls[0].signal.addEventListener('abort', () => f.calls[0].fail(new DOMException('Aborted', 'AbortError')));
  await until(() => f.statuses.at(-1) === 'reconnecting');
  assert.equal(await f.connection.submit({ kind: 'play' }), false);
  await until(() => f.calls.length === 2);
  f.calls[1].reply(state(1));
  await until(() => f.statuses.at(-1) === 'connected');
  assert.equal(f.frames.length, 1);
});
