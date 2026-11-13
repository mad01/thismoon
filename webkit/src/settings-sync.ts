// settings-sync.ts: the kit's half of the settings that follow the reader
// between *.this hosts. localStorage is per origin, so on its own a theme
// picked on one host never reaches the next. d-man serves one small document
// on every configured host at /__this/settings.json, same-origin; this module
// pulls it, reconciles it with localStorage (still the first-paint cache
// boot.js reads), and pushes the reader's own changes back.
//
// It talks to the endpoint only on a hostname ending in .this, with site
// storage it can use, and only once the first GET came back as a settings
// document within FIRST_PULL_TIMEOUT_MS. Anything else (a 404, an HTML page
// from an old d-man that proxies the path to the backend, another version, a
// network error, no answer in time) turns it off for the page load, so the
// kit sends no PATCH there and behaves as it did before. The one request
// before that answer is a pagehide while the reader has a change queued: it
// goes out once, so the change is not lost with the page. Against an old
// d-man that PATCH reaches the service's own backend, which routes nothing
// at /__this/ and answers 404 or 405.
//
// A known limit: two PATCHes with the same key can land out of order, leaving
// d-man on the older value until the reader changes that key again.
//
// The module holds no DOM: fetch, storage, the timers, the clock, the
// hostname and the apply callback are passed in, so the node tests drive it
// with stubs. It has no imports either, for the same reason: node runs the
// .ts file straight, and the kit's other modules are imported as .js.

export const SETTINGS_PATH = '/__this/settings.json';
/** The document version this kit reads and writes. */
export const DOC_VERSION = 1;
/** Pushes made within this many ms of each other go out as one PATCH. */
export const PUSH_DEBOUNCE_MS = 200;
/** At most one re-pull per this many ms, however many focus events arrive. */
export const PULL_THROTTLE_MS = 2000;
/** A first pull with no answer after this many ms counts as a bad answer: sync is off for the page load. */
export const FIRST_PULL_TIMEOUT_MS = 1500;

/** The localStorage keys that sync, which are also the wire keys. */
export const SYNC_KEYS = [
  'webkit-theme',
  'webkit-palette-light',
  'webkit-palette-dark',
  'webkit-font',
  'webkit-size',
  'webkit-fixation',
] as const;
export type SyncKey = (typeof SYNC_KEYS)[number];
export type Settings = Partial<Record<SyncKey, string>>;
/** Raw stored values per key; null or absent when the key is not stored. */
export type LocalValues = Partial<Record<SyncKey, string | null>>;

/** What the kit offers, which decides the values each key accepts. */
export interface Choices {
  /** The shipped palette family names (default may be among them). */
  families: readonly string[];
  /** The font picker's names. */
  fonts: readonly string[];
  /** The font a missing or unknown name falls back to. */
  fontFallback: string;
  /** The font-size bounds and default, in px. */
  size: { min: number; max: number; fallback: number };
}

/** How the kit reads, accepts and stores one key. */
interface Rule {
  /** The value a missing key reads as. */
  fallback: string;
  /** Whether a value is one this kit can apply and the endpoint accepts. */
  accepts(value: string): boolean;
  /** The value in effect for a raw stored value (null when absent). */
  read(raw: string | null): string;
  /** Storing the fallback removes the key instead (the palettes). */
  removeAtFallback?: boolean;
}

function oneOf(values: readonly string[], fallback: string): Rule {
  const accepts = (v: string): boolean => values.includes(v);
  return { fallback, accepts, read: raw => (raw !== null && accepts(raw) ? raw : fallback) };
}

function sizeRule({ min, max, fallback }: Choices['size']): Rule {
  return {
    fallback: String(fallback),
    // Written exactly as the kit writes one: no sign, no leading zero, no fraction.
    accepts: v => {
      const n = Number(v);
      return Number.isInteger(n) && String(n) === v && n >= min && n <= max;
    },
    // Read the way the header and boot.js read it: parseInt, then clamp.
    read: raw => {
      const n = parseInt(raw ?? '', 10);
      return String(isNaN(n) ? fallback : Math.max(min, Math.min(max, n)));
    },
  };
}

// Each key's rule mirrors how src/webkit.ts reads it: themeMode() reads
// anything but dark or system as light, paletteFor() reads an absent or
// unknown family as default, the header falls back the font name, parses and
// clamps the size, and takes only "true" as fixation on.
function rulesFor(c: Choices): Record<SyncKey, Rule> {
  const palette: Rule = { ...oneOf(['default', ...c.families], 'default'), removeAtFallback: true };
  return {
    'webkit-theme': oneOf(['light', 'dark', 'system'], 'light'),
    'webkit-palette-light': palette,
    'webkit-palette-dark': palette,
    'webkit-font': oneOf(c.fonts, c.fontFallback),
    'webkit-size': sizeRule(c.size),
    'webkit-fixation': oneOf(['true', 'false'], 'false'),
  };
}

function hasOwn(o: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(o, key);
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

export interface Reconciled {
  /** Keys to write locally and apply: the server holds a different value. */
  apply: Settings;
  /** Keys to send: the server lacks them and the local value is not the default. */
  seed: Settings;
}

/**
 * Compares the server's settings with the local ones, key by key. The
 * server wins for every key it holds with a value this kit accepts; a value
 * it does not accept (a family a newer kit added, say) leaves the key alone
 * both ways. A key the server lacks is seeded from the local value in
 * effect, unless that is the default. Unknown server keys are ignored.
 */
export function reconcile(server: Record<string, unknown>, local: LocalValues, choices: Choices): Reconciled {
  const rules = rulesFor(choices);
  const apply: Settings = {};
  const seed: Settings = {};
  for (const key of SYNC_KEYS) {
    const rule = rules[key];
    const mine = rule.read(local[key] ?? null);
    if (!hasOwn(server, key)) {
      if (mine !== rule.fallback) seed[key] = mine;
      continue;
    }
    const theirs = server[key];
    if (typeof theirs === 'string' && rule.accepts(theirs) && theirs !== mine) apply[key] = theirs;
  }
  return { apply, seed };
}

/** Whether a page on this hostname talks to the settings endpoint. */
export function syncsOn(hostname: string): boolean {
  return hostname.endsWith('.this');
}

/** The part of a fetch Response the sync reads. */
export interface SyncResponse {
  ok: boolean;
  status: number;
  headers: { get(name: string): string | null };
  json(): Promise<unknown>;
}

/** The storage calls the sync makes. */
export type SyncStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

export interface SyncDeps {
  hostname: string;
  fetch(url: string, init: RequestInit): Promise<SyncResponse>;
  /** Site storage, or null where the page cannot use it; sync is then off. */
  storage: SyncStorage | null;
  setTimeout(fn: () => void, ms: number): number;
  clearTimeout(id: number): void;
  now(): number;
  choices: Choices;
  /**
   * Called after every pull the server answered with a document, with the
   * keys it wrote to storage (possibly none), so the page can bring its live
   * state in line with storage.
   */
  applied(written: SyncKey[]): void;
}

export interface SettingsSync {
  /** False off a .this hostname or without storage: every method is then a no-op. */
  readonly enabled: boolean;
  /** The first pull, at module load. It decides whether sync is on. */
  start(): Promise<void>;
  /** Queues the reader's change; only a user action calls this. Unknown keys and values are dropped. */
  push(changes: Record<string, string>): void;
  /** A throttled re-pull, for focus, visibility and a page restored from the back/forward cache. */
  resume(): Promise<void>;
  /**
   * For pagehide: sends the queue now, keepalive, even beside a PATCH in
   * flight or before the first pull has answered, since the page may not
   * get another chance.
   */
  flush(): void;
}

type State = 'idle' | 'starting' | 'on' | 'off';

/**
 * What a PATCH answer means for the keys it carried. A 400 is the endpoint
 * refusing a value, which fails the whole patch; another 4xx refuses the
 * patch for good; a 5xx or a network error is worth a resend.
 */
type Outcome = 'done' | 'invalid' | 'refused' | 'retry';

function outcomeOf(res: SyncResponse): Outcome {
  if (res.ok) return 'done';
  if (res.status === 400) return 'invalid';
  return res.status > 400 && res.status < 500 ? 'refused' : 'retry';
}

function isJSONType(contentType: string | null): boolean {
  return (contentType ?? '').split(';')[0].trim().toLowerCase() === 'application/json';
}

/** The keys a settings object holds, in SYNC_KEYS order. */
function keysOf(settings: Settings): SyncKey[] {
  return SYNC_KEYS.filter(key => hasOwn(settings, key));
}

const OFF: SettingsSync = Object.freeze({
  enabled: false,
  start: () => Promise.resolve(),
  push: () => {},
  resume: () => Promise.resolve(),
  flush: () => {},
});

export function createSettingsSync(deps: SyncDeps): SettingsSync {
  const { storage } = deps;
  return syncsOn(deps.hostname) && storage !== null ? liveSync(deps, storage) : OFF;
}

function liveSync(deps: SyncDeps, storage: SyncStorage): SettingsSync {
  const rules = rulesFor(deps.choices);
  let state: State = 'idle';
  // Keys waiting to go.
  let pending: Settings = {};
  // The PATCHes in flight and the keys each carries. The debounced path
  // keeps one at a time; a pagehide flush may send another beside it.
  const flights = new Map<Promise<void>, Settings>();
  // Keys the endpoint refused: a pull leaves the local value alone until the
  // reader changes the key again.
  const held = new Set<SyncKey>();
  let timer: number | null = null;
  let pulling = false;
  let lastPull = -Infinity;
  // A pull leaves alone any key pushed after its GET went out, even when
  // that push has already been sent and answered. The same count tells a
  // PATCH answer which of its keys the reader has changed since.
  let pushSeq = 0;
  const pushedAt: Partial<Record<SyncKey, number>> = {};

  const hasPending = (): boolean => Object.keys(pending).length > 0;
  const pushedSince = (key: SyncKey, seq: number): boolean => (pushedAt[key] ?? 0) > seq;
  const inFlight = (key: SyncKey): boolean => [...flights.values()].some(body => hasOwn(body, key));

  function cancelTimer(): void {
    if (timer === null) return;
    deps.clearTimeout(timer);
    timer = null;
  }

  function schedule(): void {
    cancelTimer();
    timer = deps.setTimeout(() => {
      timer = null;
      void send();
    }, PUSH_DEBOUNCE_MS);
  }

  function take(): Settings {
    const body = pending;
    pending = {};
    return body;
  }

  function patch(body: Settings): Promise<Outcome> {
    return deps
      .fetch(SETTINGS_PATH, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ v: DOC_VERSION, settings: body }),
        keepalive: true,
      })
      .then(outcomeOf, () => 'retry' as const);
  }

  // Acts on one answer and reports whether a key went back in the queue. A
  // key the reader changed since the PATCH went out is left to the newer
  // value. On a retry the key waits in the queue for the next pull; on a
  // refusal it is held, so that pull does not revert the reader's value.
  function settle(body: Settings, outcome: Outcome, seq: number): boolean {
    if (outcome === 'done') return false;
    let requeued = false;
    for (const key of keysOf(body)) {
      if (pushedSince(key, seq)) continue;
      if (outcome !== 'retry') held.add(key);
      else if (!hasOwn(pending, key)) {
        pending[key] = body[key];
        requeued = true;
      }
    }
    return requeued;
  }

  // One refused value fails a whole patch with a 400, so a patch of several
  // keys that gets one goes again key by key: the valid keys land and only
  // the refused one is held. The single-key PATCHes all go out at once: a
  // page that goes away meanwhile cannot flush them, since they belong to
  // this flight and not to the queue.
  async function deliver(body: Settings, seq: number): Promise<boolean> {
    const outcome = await patch(body);
    const keys = keysOf(body);
    if (outcome !== 'invalid' || keys.length < 2) return settle(body, outcome, seq);
    const resends = keys
      .filter(key => !pushedSince(key, seq))
      .map(key => {
        const one: Settings = { [key]: body[key] };
        return patch(one).then(answer => settle(one, answer, seq));
      });
    return (await Promise.all(resends)).some(requeued => requeued);
  }

  function dispatch(body: Settings): Promise<void> {
    const flight = deliver(body, pushSeq).then(requeued => {
      flights.delete(flight);
      if (!requeued && hasPending() && timer === null) schedule();
    });
    flights.set(flight, body);
    return flight;
  }

  async function idle(): Promise<void> {
    while (flights.size > 0) await Promise.all(flights.keys());
  }

  // The debounced path: one PATCH at a time, so a change made meanwhile
  // waits for the answer.
  function send(): Promise<void> {
    if (flights.size > 0 || state !== 'on' || !hasPending()) return Promise.resolve();
    return dispatch(take());
  }

  function push(changes: Record<string, string>): void {
    if (state === 'off') return;
    let queued = false;
    for (const key of SYNC_KEYS) {
      const value = changes[key];
      if (typeof value !== 'string' || !rules[key].accepts(value)) continue;
      pending[key] = value;
      pushedAt[key] = ++pushSeq;
      held.delete(key);
      queued = true;
    }
    // Before the first pull answers, the queue waits for it, or for pagehide.
    if (queued && state === 'on') schedule();
  }

  async function getDocument(): Promise<Record<string, unknown> | null> {
    try {
      const res = await deps.fetch(SETTINGS_PATH, { cache: 'no-store', headers: { Accept: 'application/json' } });
      if (!res.ok || !isJSONType(res.headers.get('Content-Type'))) return null;
      const doc = await res.json();
      return isObject(doc) && doc.v === DOC_VERSION && isObject(doc.settings) ? doc.settings : null;
    } catch {
      return null;
    }
  }

  // The first GET gives up after FIRST_PULL_TIMEOUT_MS, so the reader's
  // changes do not wait on a slow d-man; a later answer is ignored.
  function firstDocument(): Promise<Record<string, unknown> | null> {
    return new Promise(resolve => {
      const id = deps.setTimeout(() => resolve(null), FIRST_PULL_TIMEOUT_MS);
      void getDocument().then(doc => {
        deps.clearTimeout(id);
        resolve(doc);
      });
    });
  }

  function readLocal(): LocalValues {
    const local: LocalValues = {};
    for (const key of SYNC_KEYS) local[key] = storage.getItem(key);
    return local;
  }

  // Stores a pulled value; false when storage refuses the write (over
  // quota, say), so the key is not reported as written.
  function write(key: SyncKey, value: string): boolean {
    const rule = rules[key];
    try {
      if (rule.removeAtFallback && value === rule.fallback) storage.removeItem(key);
      else storage.setItem(key, value);
      return true;
    } catch {
      return false;
    }
  }

  // The reader's own change wins over the document: a key still queued, in
  // flight, pushed since the GET went out, or refused by the endpoint is
  // neither applied nor seeded.
  function adopt(server: Record<string, unknown>, seq: number): boolean {
    const mine = (key: SyncKey): boolean =>
      hasOwn(pending, key) || inFlight(key) || pushedSince(key, seq) || held.has(key);
    const { apply, seed } = reconcile(server, readLocal(), deps.choices);
    const written: SyncKey[] = [];
    let seeded = false;
    for (const key of SYNC_KEYS) {
      if (mine(key)) continue;
      const theirs = apply[key];
      if (theirs !== undefined && write(key, theirs)) written.push(key);
      const ours = seed[key];
      if (ours !== undefined) {
        pending[key] = ours;
        seeded = true;
      }
    }
    deps.applied(written);
    return seeded;
  }

  // Sends whatever is queued or in flight first, so a failed PATCH goes out
  // again before the GET.
  async function drain(): Promise<void> {
    cancelTimer();
    await idle();
    cancelTimer();
    await send();
  }

  async function pull(): Promise<void> {
    if (pulling) return;
    pulling = true;
    try {
      await drain();
      const seq = pushSeq;
      const server = await getDocument();
      if (server && state === 'on' && adopt(server, seq)) schedule();
    } finally {
      pulling = false;
    }
  }

  async function start(): Promise<void> {
    if (state !== 'idle') return;
    state = 'starting';
    lastPull = deps.now();
    const seq = pushSeq;
    const server = await firstDocument();
    if (!server) {
      state = 'off';
      pending = {};
      cancelTimer();
      return;
    }
    state = 'on';
    adopt(server, seq);
    if (hasPending()) schedule();
  }

  function resume(): Promise<void> {
    if (state !== 'on') return Promise.resolve();
    const t = deps.now();
    if (t - lastPull < PULL_THROTTLE_MS) return Promise.resolve();
    lastPull = t;
    return pull();
  }

  // The page may not come back, so the queue goes now: beside a PATCH in
  // flight, and before the first pull has answered.
  function flush(): void {
    cancelTimer();
    if ((state === 'on' || state === 'starting') && hasPending()) void dispatch(take());
  }

  return { enabled: true, start, push, resume, flush };
}
