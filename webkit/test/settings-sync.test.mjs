// The settings sync, driven with stubs: fetch, storage, timers and the clock
// are fakes, so each test sets exactly what the endpoint answers and when.
// Run via `node --test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  createSettingsSync, FIRST_PULL_TIMEOUT_MS, PULL_THROTTLE_MS, PUSH_DEBOUNCE_MS, reconcile, SETTINGS_PATH, syncsOn,
} from '../src/settings-sync.ts';
import { SIZE_DEFAULT, SIZE_MAX, SIZE_MIN } from '../src/size.ts';

const CHOICES = {
  families: ['default', 'nord', 'gruvbox'],
  fonts: ['Fira Code', 'Inter', 'Lexend', 'Work Sans'],
  fontFallback: 'Fira Code',
  size: { min: SIZE_MIN, max: SIZE_MAX, fallback: SIZE_DEFAULT },
};

// ── answers the fake endpoint gives ──

function answer({ status = 200, type = 'application/json', body = '' } = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: name => (name.toLowerCase() === 'content-type' ? type : null) },
    json: async () => JSON.parse(body),
  };
}
const doc = settings => answer({ body: JSON.stringify({ v: 1, settings }) });
const networkError = () => Promise.reject(new TypeError('Failed to fetch'));

function deferred() {
  let resolve;
  const promise = new Promise(r => { resolve = r; });
  return { promise, resolve };
}

// Lets every queued promise callback run.
const settle = () => new Promise(r => setImmediate(r));

// A sync over fake deps. `respond(call)` returns the answer (or a promise of
// one) for each request; `calls` records them in order. `noStorage` stands
// for a page that cannot use site storage, and `failWrites` lists keys whose
// writes throw, as a full quota does.
function harness({
  hostname = 'events.this', stored = {}, respond = () => doc({}), noStorage = false, failWrites = [],
} = {}) {
  const storage = new Map(Object.entries(stored));
  const calls = [];
  const applied = [];
  const timers = new Map();
  let clock = 0;
  let nextTimer = 1;
  const sync = createSettingsSync({
    hostname,
    choices: CHOICES,
    storage: noStorage ? null : {
      getItem: k => (storage.has(k) ? storage.get(k) : null),
      setItem: (k, v) => {
        if (failWrites.includes(k)) throw new DOMException('quota exceeded', 'QuotaExceededError');
        storage.set(k, String(v));
      },
      removeItem: k => { storage.delete(k); },
    },
    fetch: (url, init) => {
      const call = {
        url,
        method: init.method ?? 'GET',
        keepalive: init.keepalive,
        settings: init.body ? JSON.parse(init.body).settings : undefined,
      };
      calls.push(call);
      return Promise.resolve().then(() => respond(call));
    },
    setTimeout: (fn, ms) => { const id = nextTimer++; timers.set(id, { at: clock + ms, fn }); return id; },
    clearTimeout: id => { timers.delete(id); },
    now: () => clock,
    applied: written => { applied.push(written); },
  });
  return {
    sync, storage, calls, applied,
    methods: () => calls.map(c => c.method),
    patches: () => calls.filter(c => c.method === 'PATCH').map(c => c.settings),
    /** The reader acting in the page: storage first, as the kit does, then the push. */
    user(changes) {
      for (const [k, v] of Object.entries(changes)) storage.set(k, v);
      sync.push(changes);
    },
    async advance(ms) {
      clock += ms;
      for (const [id, t] of [...timers]) {
        if (t.at <= clock) { timers.delete(id); t.fn(); }
      }
      await settle();
    },
  };
}

// ── reconcile ──

test('reconcile: the server wins for every key it holds', () => {
  const got = reconcile(
    { 'webkit-theme': 'dark', 'webkit-size': '18', 'webkit-fixation': 'true', 'webkit-font': 'Lexend' },
    { 'webkit-theme': 'light', 'webkit-size': '16', 'webkit-fixation': 'false', 'webkit-font': 'Lexend' },
    CHOICES,
  );
  assert.deepEqual(got.apply, { 'webkit-theme': 'dark', 'webkit-size': '18', 'webkit-fixation': 'true' });
  assert.deepEqual(got.seed, {});
});

test('reconcile: equal values compare as the kit reads them, so an absent key matches its default', () => {
  const got = reconcile(
    { 'webkit-theme': 'light', 'webkit-palette-dark': 'default', 'webkit-size': '16', 'webkit-fixation': 'false' },
    {},
    CHOICES,
  );
  assert.deepEqual(got, { apply: {}, seed: {} });
});

test('reconcile: seeds only keys the server lacks whose local value is not the default', () => {
  const got = reconcile({}, {
    'webkit-theme': 'dark',
    'webkit-palette-light': 'default',
    'webkit-palette-dark': 'nord',
    'webkit-font': 'Fira Code',
    'webkit-size': '16',
    'webkit-fixation': 'false',
  }, CHOICES);
  assert.deepEqual(got.seed, { 'webkit-theme': 'dark', 'webkit-palette-dark': 'nord' });
  assert.deepEqual(got.apply, {});
});

test('reconcile: a local value the kit does not use reads as the default and is not seeded', () => {
  const got = reconcile({}, {
    'webkit-theme': 'sepia', 'webkit-palette-dark': 'retired-family', 'webkit-size': 'big', 'webkit-fixation': 'yes',
  }, CHOICES);
  assert.deepEqual(got.seed, {});
});

test('reconcile: the seed is the value in effect, an out-of-range size clamped', () => {
  assert.deepEqual(reconcile({}, { 'webkit-size': '30' }, CHOICES).seed, { 'webkit-size': '24' });
});

test('reconcile: a pulled default palette is applied over a local family', () => {
  const got = reconcile({ 'webkit-palette-dark': 'default' }, { 'webkit-palette-dark': 'nord' }, CHOICES);
  assert.deepEqual(got.apply, { 'webkit-palette-dark': 'default' });
});

test('reconcile: unknown keys and values this kit does not accept are ignored both ways', () => {
  const got = reconcile({
    'webkit-audio': 'off',
    'webkit-ra-speed': '2',
    'something-else': 'x',
    'webkit-palette-dark': 'a-newer-family',
    'webkit-size': '018',
    'webkit-theme': 7,
  }, { 'webkit-palette-dark': 'nord', 'webkit-size': '20', 'webkit-theme': 'dark' }, CHOICES);
  assert.deepEqual(got, { apply: {}, seed: {} });
});

// ── gate ──

test('gate: only a hostname ending in .this syncs', () => {
  assert.equal(syncsOn('events.this'), true);
  for (const host of ['localhost', '127.0.0.1', 'present.example.com', 'this', 'events.this.example.com']) {
    assert.equal(syncsOn(host), false, host);
  }
});

test('gate: off a .this hostname the sync never calls fetch', async () => {
  for (const hostname of ['localhost', '127.0.0.1', 'present.example.com']) {
    const h = harness({ hostname, stored: { 'webkit-theme': 'dark' } });
    assert.equal(h.sync.enabled, false);
    await h.sync.start();
    h.user({ 'webkit-theme': 'light' });
    await h.advance(PUSH_DEBOUNCE_MS);
    await h.advance(PULL_THROTTLE_MS);
    await h.sync.resume();
    h.sync.flush();
    await settle();
    assert.deepEqual(h.calls, [], hostname);
    assert.deepEqual(h.applied, [], hostname);
  }
});

test('gate: without site storage the sync is off and never calls fetch', async () => {
  const h = harness({ noStorage: true });
  assert.equal(h.sync.enabled, false);
  await h.sync.start();
  h.sync.push({ 'webkit-theme': 'dark' });
  h.sync.flush();
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.calls, []);
  assert.deepEqual(h.applied, []);
});

// ── the first pull ──

test('start: a pull writes the server value, removes the key for a default palette, and reports both', async () => {
  const h = harness({
    stored: { 'webkit-theme': 'light', 'webkit-palette-dark': 'nord' },
    respond: () => doc({ 'webkit-theme': 'dark', 'webkit-palette-dark': 'default' }),
  });
  await h.sync.start();
  assert.equal(h.calls[0].url, SETTINGS_PATH);
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  assert.equal(h.storage.has('webkit-palette-dark'), false);
  assert.deepEqual(h.applied, [['webkit-theme', 'webkit-palette-dark']]);
});

test('start: local non-defaults the server lacks go out as one seed PATCH', async () => {
  const h = harness({
    stored: { 'webkit-theme': 'dark', 'webkit-size': '18', 'webkit-font': 'Fira Code' },
    respond: c => (c.method === 'GET' ? doc({ 'webkit-fixation': 'true' }) : doc({})),
  });
  await h.sync.start();
  assert.equal(h.storage.get('webkit-fixation'), 'true');
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark', 'webkit-size': '18' }]);
});

test('start: a change made before the first pull answers is not reverted, and goes out after it', async () => {
  const get = deferred();
  const h = harness({ respond: c => (c.method === 'GET' ? get.promise : doc({})) });
  const started = h.sync.start();
  h.user({ 'webkit-size': '19' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.methods(), ['GET']);
  get.resolve(doc({ 'webkit-size': '14' }));
  await started;
  assert.equal(h.storage.get('webkit-size'), '19');
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-size': '19' }]);
});

test('start: a storage write that throws skips that key; start resolves and reports the rest', async () => {
  const h = harness({
    failWrites: ['webkit-size'],
    respond: () => doc({ 'webkit-theme': 'dark', 'webkit-size': '20' }),
  });
  await h.sync.start();
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  assert.equal(h.storage.has('webkit-size'), false);
  assert.deepEqual(h.applied, [['webkit-theme']]);
});

test('start: no first answer within FIRST_PULL_TIMEOUT_MS turns sync off; the late answer changes nothing', async () => {
  const get = deferred();
  const h = harness({ stored: { 'webkit-theme': 'dark' }, respond: c => (c.method === 'GET' ? get.promise : doc({})) });
  const started = h.sync.start();
  h.user({ 'webkit-size': '20' });
  await h.advance(FIRST_PULL_TIMEOUT_MS);
  await started;
  get.resolve(doc({ 'webkit-theme': 'light' }));
  await settle();
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  assert.deepEqual(h.applied, []);
  h.user({ 'webkit-fixation': 'true' });
  await h.advance(PUSH_DEBOUNCE_MS);
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  h.sync.flush();
  await settle();
  assert.deepEqual(h.methods(), ['GET']);
});

test('start: a first answer just inside FIRST_PULL_TIMEOUT_MS turns sync on', async () => {
  const get = deferred();
  const h = harness({ respond: c => (c.method === 'GET' ? get.promise : doc({})) });
  const started = h.sync.start();
  await h.advance(FIRST_PULL_TIMEOUT_MS - 1);
  get.resolve(doc({ 'webkit-theme': 'dark' }));
  await started;
  await h.advance(1);
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  h.user({ 'webkit-size': '20' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-size': '20' }]);
});

const BAD_ANSWERS = {
  '404': () => answer({ status: 404, type: 'text/plain', body: 'not found' }),
  'HTML with 200': () => answer({ type: 'text/html; charset=utf-8', body: '<!doctype html><p>a backend page</p>' }),
  'JSON with 200 that is not JSON': () => answer({ body: '<html>' }),
  'wrong v': () => answer({ body: JSON.stringify({ v: 2, settings: { 'webkit-theme': 'dark' } }) }),
  'settings not an object': () => answer({ body: JSON.stringify({ v: 1, settings: ['webkit-theme'] }) }),
  'network error': networkError,
};

for (const [name, bad] of Object.entries(BAD_ANSWERS)) {
  test(`start: a bad first answer (${name}) applies nothing and sends no PATCH later`, async () => {
    const h = harness({ stored: { 'webkit-theme': 'dark' }, respond: c => (c.method === 'GET' ? bad() : doc({})) });
    await h.sync.start();
    assert.deepEqual(h.applied, []);
    assert.equal(h.storage.get('webkit-theme'), 'dark');
    h.user({ 'webkit-size': '20' });
    await h.advance(PUSH_DEBOUNCE_MS);
    await h.advance(PULL_THROTTLE_MS);
    await h.sync.resume();
    h.sync.flush();
    await settle();
    assert.deepEqual(h.methods(), ['GET']);
  });
}

// ── pushing ──

test('push: changes within the debounce merge per key into one PATCH, newest value winning', async () => {
  const h = harness();
  await h.sync.start();
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS - 50);
  h.user({ 'webkit-size': '17' });
  await h.advance(PUSH_DEBOUNCE_MS - 50);
  h.user({ 'webkit-size': '18' });
  assert.deepEqual(h.patches(), []);
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark', 'webkit-size': '18' }]);
  assert.equal(h.calls[1].keepalive, true);
});

test('push: unknown keys and values the endpoint would refuse are not sent', async () => {
  const h = harness();
  await h.sync.start();
  h.sync.push({ 'webkit-audio': 'off', 'webkit-size': '99', 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark' }]);
});

test('push: one PATCH in flight; a change made meanwhile waits for it, then goes out', async () => {
  const first = deferred();
  let patch = 0;
  const h = harness({ respond: c => (c.method === 'GET' ? doc({}) : (++patch === 1 ? first.promise : doc({}))) });
  await h.sync.start();
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  h.user({ 'webkit-size': '20' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark' }]);
  first.resolve(doc({ 'webkit-theme': 'dark' }));
  await settle();
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark' }, { 'webkit-size': '20' }]);
});

// ── pagehide ──

test('flush: pagehide sends the queue at once, keepalive', async () => {
  const h = harness();
  await h.sync.start();
  h.user({ 'webkit-fixation': 'true' });
  h.sync.flush();
  await settle();
  assert.deepEqual(h.patches(), [{ 'webkit-fixation': 'true' }]);
  assert.equal(h.calls.at(-1).keepalive, true);
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.equal(h.patches().length, 1);
});

test('flush: with a PATCH in flight, pagehide sends the keys queued behind it at once', async () => {
  const first = deferred();
  let patch = 0;
  const h = harness({ respond: c => (c.method === 'GET' ? doc({}) : (++patch === 1 ? first.promise : doc({}))) });
  await h.sync.start();
  h.user({ 'webkit-size': '17' });
  await h.advance(PUSH_DEBOUNCE_MS);
  h.user({ 'webkit-size': '18' });
  h.sync.flush();
  await settle();
  assert.deepEqual(h.patches(), [{ 'webkit-size': '17' }, { 'webkit-size': '18' }]);
  assert.equal(h.calls.at(-1).keepalive, true);
  first.resolve(doc({ 'webkit-size': '17' }));
  await settle();
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.equal(h.patches().length, 2);
});

test('flush: an older PATCH that fails after the flush does not put its value back over the newer one', async () => {
  const first = deferred();
  let server = {};
  let patch = 0;
  const h = harness({
    respond: c => {
      if (c.method === 'GET') return doc(server);
      if (++patch === 1) return first.promise;
      server = { ...server, ...c.settings };
      return doc(server);
    },
  });
  await h.sync.start();
  h.user({ 'webkit-size': '17' });
  await h.advance(PUSH_DEBOUNCE_MS);
  h.user({ 'webkit-size': '18' });
  h.sync.flush();
  await settle();
  first.resolve(answer({ status: 503, type: 'text/plain' }));
  await settle();
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.patches(), [{ 'webkit-size': '17' }, { 'webkit-size': '18' }]);
  assert.deepEqual(server, { 'webkit-size': '18' });
  assert.equal(h.storage.get('webkit-size'), '18');
});

test('flush: before the first pull answers, pagehide sends the queue, and that answer leaves it alone', async () => {
  const get = deferred();
  const h = harness({ respond: c => (c.method === 'GET' ? get.promise : doc({})) });
  const started = h.sync.start();
  h.user({ 'webkit-theme': 'dark' });
  h.sync.flush();
  await settle();
  assert.deepEqual(h.methods(), ['GET', 'PATCH']);
  assert.deepEqual(h.patches(), [{ 'webkit-theme': 'dark' }]);
  assert.equal(h.calls[1].keepalive, true);
  // The page came back from the back/forward cache; the GET sent before the
  // toggle answers with the old value.
  get.resolve(doc({ 'webkit-theme': 'light' }));
  await started;
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.equal(h.patches().length, 1);
});

test('flush: behind an old d-man, the pagehide PATCH before the first answer is the only one', async () => {
  const get = deferred();
  const h = harness({ respond: c => (c.method === 'GET' ? get.promise : answer({ status: 404, type: 'text/plain' })) });
  const started = h.sync.start();
  h.user({ 'webkit-theme': 'dark' });
  h.sync.flush();
  await settle();
  get.resolve(BAD_ANSWERS['HTML with 200']());
  await started;
  h.user({ 'webkit-size': '20' });
  await h.advance(PUSH_DEBOUNCE_MS);
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  h.sync.flush();
  await settle();
  assert.deepEqual(h.methods(), ['GET', 'PATCH']);
  assert.equal(h.storage.get('webkit-theme'), 'dark');
});

test('push: a failed PATCH is resent before the next pull, and that pull does not revert it', async () => {
  let server = {};
  let patchStatus = 503;
  const h = harness({
    respond: c => {
      if (c.method === 'GET') return doc(server);
      if (patchStatus !== 200) return answer({ status: patchStatus, type: 'text/plain', body: 'down' });
      server = { ...server, ...c.settings };
      return doc(server);
    },
  });
  await h.sync.start();
  server = { 'webkit-theme': 'light' }; // another host wrote light meanwhile
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.methods(), ['GET', 'PATCH']);

  // The resend fails again: the pull's light does not land over dark.
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.methods(), ['GET', 'PATCH', 'PATCH', 'GET']);
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  assert.deepEqual(h.applied.at(-1), []);

  // The next one goes through, so the server holds dark too.
  patchStatus = 200;
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.methods(), ['GET', 'PATCH', 'PATCH', 'GET', 'PATCH', 'GET']);
  assert.deepEqual(server, { 'webkit-theme': 'dark' });
  assert.equal(h.storage.get('webkit-theme'), 'dark');
});

test('push: a network error keeps the PATCH pending like a 5xx', async () => {
  let fail = true;
  const h = harness({ respond: c => (c.method === 'PATCH' && fail ? networkError() : doc({})) });
  await h.sync.start();
  h.user({ 'webkit-size': '21' });
  await h.advance(PUSH_DEBOUNCE_MS);
  fail = false;
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.patches(), [{ 'webkit-size': '21' }, { 'webkit-size': '21' }]);
});

test('push: a 400 on one key is not resent, and pulls keep the local value until the reader changes it', async () => {
  let server = { 'webkit-theme': 'light' };
  let refuse = true;
  const h = harness({
    respond: c => {
      if (c.method === 'GET') return doc(server);
      if (refuse) return answer({ status: 400, type: 'text/plain' });
      server = { ...server, ...c.settings };
      return doc(server);
    },
  });
  await h.sync.start();
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.methods(), ['GET', 'PATCH', 'GET']);
  // The server refused dark and still holds light; the pull leaves dark.
  assert.equal(h.storage.get('webkit-theme'), 'dark');

  // A new choice goes out, and once the server holds it, another host's
  // change applies again.
  refuse = false;
  h.user({ 'webkit-theme': 'system' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(server, { 'webkit-theme': 'system' });
  server = { 'webkit-theme': 'light' };
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.equal(h.storage.get('webkit-theme'), 'light');
});

test('push: a 400 on a patch of several keys resends them one by one, so the valid ones land', async () => {
  // An older d-man that does not know the gruvbox family refuses any patch
  // that carries it, the valid keys beside it included.
  let server = { 'webkit-theme': 'light', 'webkit-palette-dark': 'nord' };
  const h = harness({
    respond: c => {
      if (c.method === 'GET') return doc(server);
      if (c.settings['webkit-palette-dark'] === 'gruvbox') return answer({ status: 400, type: 'text/plain' });
      server = { ...server, ...c.settings };
      return doc(server);
    },
  });
  await h.sync.start();
  h.user({ 'webkit-palette-dark': 'gruvbox' });
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [
    { 'webkit-theme': 'dark', 'webkit-palette-dark': 'gruvbox' },
    { 'webkit-theme': 'dark' },
    { 'webkit-palette-dark': 'gruvbox' },
  ]);
  assert.deepEqual(server, { 'webkit-theme': 'dark', 'webkit-palette-dark': 'nord' });
  // On the next focus neither flips back, and the refused key is not resent.
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.equal(h.storage.get('webkit-theme'), 'dark');
  assert.equal(h.storage.get('webkit-palette-dark'), 'gruvbox');
  assert.equal(h.patches().length, 3);
});

test('push: another 4xx drops the patch whole, and pulls leave its keys alone', async () => {
  for (const status of [403, 413, 415]) {
    const h = harness({
      respond: c => (c.method === 'GET'
        ? doc({ 'webkit-theme': 'light', 'webkit-size': '16' })
        : answer({ status, type: 'text/plain' })),
    });
    await h.sync.start();
    h.user({ 'webkit-theme': 'dark', 'webkit-size': '18' });
    await h.advance(PUSH_DEBOUNCE_MS);
    await h.advance(PULL_THROTTLE_MS);
    await h.sync.resume();
    assert.deepEqual(h.methods(), ['GET', 'PATCH', 'GET'], String(status));
    assert.equal(h.storage.get('webkit-theme'), 'dark', String(status));
    assert.equal(h.storage.get('webkit-size'), '18', String(status));
  }
});

test('pull: a change pushed after the GET went out is not reverted, even once it is answered', async () => {
  const get = deferred();
  let gets = 0;
  const h = harness({ respond: c => (c.method === 'GET' ? (++gets === 2 ? get.promise : doc({})) : doc({})) });
  await h.sync.start();
  await h.advance(PULL_THROTTLE_MS);
  const pulled = h.sync.resume();
  await settle();
  h.user({ 'webkit-theme': 'dark' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.methods(), ['GET', 'GET', 'PATCH']);
  get.resolve(doc({ 'webkit-theme': 'light' }));
  await pulled;
  assert.equal(h.storage.get('webkit-theme'), 'dark');
});

// ── re-pulls ──

test('resume: focus bursts are throttled to one pull per window', async () => {
  const h = harness();
  await h.sync.start();
  await h.sync.resume(); // inside the window the first pull opened
  assert.deepEqual(h.methods(), ['GET']);
  await h.advance(PULL_THROTTLE_MS);
  await Promise.all([h.sync.resume(), h.sync.resume(), h.sync.resume()]);
  assert.deepEqual(h.methods(), ['GET', 'GET']);
  await h.advance(PULL_THROTTLE_MS - 1);
  await h.sync.resume();
  assert.deepEqual(h.methods(), ['GET', 'GET']);
  await h.advance(1);
  await h.sync.resume();
  assert.deepEqual(h.methods(), ['GET', 'GET', 'GET']);
});

test('resume: a re-pull applies what another host changed and reports every answered pull', async () => {
  let server = {};
  const h = harness({ respond: () => doc(server) });
  await h.sync.start();
  server = { 'webkit-size': '20', 'webkit-fixation': 'true' };
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.equal(h.storage.get('webkit-size'), '20');
  assert.equal(h.storage.get('webkit-fixation'), 'true');
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.deepEqual(h.applied, [[], ['webkit-size', 'webkit-fixation'], []]);
});

test('resume: a failed re-pull applies nothing and keeps sync on', async () => {
  let down = false;
  const h = harness({ respond: c => (down ? networkError() : doc({ 'webkit-theme': 'dark' })) });
  await h.sync.start();
  down = true;
  await h.advance(PULL_THROTTLE_MS);
  await h.sync.resume();
  assert.equal(h.applied.length, 1);
  down = false;
  h.user({ 'webkit-size': '13' });
  await h.advance(PUSH_DEBOUNCE_MS);
  assert.deepEqual(h.patches(), [{ 'webkit-size': '13' }]);
});
