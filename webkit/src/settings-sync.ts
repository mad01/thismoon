// settings-sync.ts: the kit's half of the settings that follow the reader
// between *.this hosts. localStorage is per origin, so on its own a theme
// picked on one host never reaches the next. d-man serves one small document
// on every configured host at /__this/settings.json, same-origin; this module
// pulls it, reconciles it with localStorage (still the first-paint cache
// boot.js reads), and pushes the reader's own changes back.
//
// It talks to the endpoint only on a hostname ending in .this, and only once
// the first GET came back as a settings document. Anything else (a 404, an
// HTML page from an old d-man that proxies the path to the backend, another
// version, a network error) turns it off for the page load, so the kit sends
// no PATCH there and behaves as it did before.
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

export interface SyncDeps {
  hostname: string;
  fetch(url: string, init: RequestInit): Promise<SyncResponse>;
  storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
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
  /** False off a .this hostname: every method is then a no-op. */
  readonly enabled: boolean;
  /** The first pull, at module load. It decides whether sync is on. */
  start(): Promise<void>;
  /** Queues the reader's change; only a user action calls this. Unknown keys and values are dropped. */
  push(changes: Record<string, string>): void;
  /** A throttled re-pull, for focus, visibility and a page restored from the back/forward cache. */
  resume(): Promise<void>;
  /** Sends the queue now, for pagehide. */
  flush(): void;
}

type State = 'idle' | 'starting' | 'on' | 'off';

/** Whether a PATCH answer refuses the patch for good, so it is dropped rather than resent. */
function refused(status: number): boolean {
  return status >= 400 && status < 500;
}

function isJSONType(contentType: string | null): boolean {
  return (contentType ?? '').split(';')[0].trim().toLowerCase() === 'application/json';
}

export function createSettingsSync(deps: SyncDeps): SettingsSync {
  const enabled = syncsOn(deps.hostname);
  const rules = rulesFor(deps.choices);
  let state: State = enabled ? 'idle' : 'off';
  // The queue: keys waiting to go, and the keys of the PATCH in flight.
  let pending: Settings = {};
  let sending: Settings = {};
  let inFlight: Promise<void> | null = null;
  let timer: number | null = null;
  let pulling = false;
  let lastPull = -Infinity;
  // A pull leaves alone any key pushed after its GET went out, even when
  // that push has already been sent and answered.
  let pushSeq = 0;
  const pushedAt: Partial<Record<SyncKey, number>> = {};

  const hasPending = (): boolean => Object.keys(pending).length > 0;

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

  // One PATCH at a time. A network error or a 5xx puts its keys back in the
  // queue under any newer value, to go before the next pull; a 4xx drops them.
  function send(): Promise<void> {
    if (inFlight) return inFlight;
    if (state !== 'on' || !hasPending()) return Promise.resolve();
    const body = pending;
    pending = {};
    sending = body;
    inFlight = deps
      .fetch(SETTINGS_PATH, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ v: DOC_VERSION, settings: body }),
        keepalive: true,
      })
      .then(
        res => (res.ok || refused(res.status) ? 'done' : 'retry'),
        () => 'retry' as const,
      )
      .then(outcome => {
        inFlight = null;
        sending = {};
        if (outcome === 'retry') pending = { ...body, ...pending };
        else if (hasPending() && timer === null) schedule();
      });
    return inFlight;
  }

  function push(changes: Record<string, string>): void {
    if (state === 'off') return;
    let queued = false;
    for (const key of SYNC_KEYS) {
      const value = changes[key];
      if (typeof value !== 'string' || !rules[key].accepts(value)) continue;
      pending[key] = value;
      pushedAt[key] = ++pushSeq;
      queued = true;
    }
    // Before the first pull answers, the queue waits for it.
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

  function readLocal(): LocalValues {
    const local: LocalValues = {};
    for (const key of SYNC_KEYS) local[key] = deps.storage.getItem(key);
    return local;
  }

  function write(key: SyncKey, value: string): void {
    const rule = rules[key];
    if (rule.removeAtFallback && value === rule.fallback) deps.storage.removeItem(key);
    else deps.storage.setItem(key, value);
  }

  // The reader's own change wins over the document: a key still queued, in
  // flight, or pushed since the GET went out is neither applied nor seeded.
  function adopt(server: Record<string, unknown>, seq: number): boolean {
    const mine = (key: SyncKey): boolean =>
      hasOwn(pending, key) || hasOwn(sending, key) || (pushedAt[key] ?? 0) > seq;
    const { apply, seed } = reconcile(server, readLocal(), deps.choices);
    const written: SyncKey[] = [];
    let seeded = false;
    for (const key of SYNC_KEYS) {
      if (mine(key)) continue;
      const theirs = apply[key];
      if (theirs !== undefined) {
        write(key, theirs);
        written.push(key);
      }
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
    if (inFlight) await inFlight;
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
    const server = await getDocument();
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

  function flush(): void {
    cancelTimer();
    void send();
  }

  return { enabled, start, push, resume, flush };
}
