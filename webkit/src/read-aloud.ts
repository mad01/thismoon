// read-aloud.ts — <wk-read-aloud>: per-section text-to-speech playback against
// a local OpenAI-compatible speech endpoint (Kokoro via mlx-audio, fronted by
// d-man at http://speak.this). Injects a play/pause button into each `targets`
// match; stays fully inert when the endpoint is unreachable, so pages work
// unchanged on hosts without the speak service. Sections speak serve rendered
// name their pre-synthesized parts (data-ra-chunk) and replay cached audio;
// anything else is synthesized live, a few sentences per request.

import {
  group, joinParts, LIVE_RAMP, partKeys, segmentSentences, speakable, splitKeys, Sentence,
} from './sentences.js';

const DEFAULT_ENDPOINT = 'http://speak.this';
const DEFAULT_VOICE = 'af_heart';
const TTS_MODEL = 'mlx-community/Kokoro-82M-bf16';
const PROBE_TIMEOUT_MS = 1500;
// How long a failure toast stays up: long enough to read a provider's reason.
const ERROR_TOAST_MS = 8000;

// Subtrees that produce no speakable text. Block code and tables read terribly
// as linear speech, so they are skipped rather than mangled. Inline <code> is
// NOT skipped: prose leans on it for identifiers ("the pkg/mixer class"), and
// dropping it leaves holes in sentences.
const SKIP_TAGS = new Set([
  'PRE', 'TABLE', 'SVG', 'SCRIPT', 'STYLE',
  'BUTTON', 'SELECT', 'TEXTAREA', 'WK-BADGE',
]);

// Entering/leaving one of these flushes the current segmentation run, so a
// heading never merges into the following paragraph's first sentence.
const BLOCK_TAGS = new Set([
  'P', 'LI', 'TD', 'TH', 'DIV', 'SECTION', 'ARTICLE', 'BLOCKQUOTE',
  'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'UL', 'OL', 'TR', 'BR',
  'WK-TITLE', 'WK-SUBTITLE', 'WK-SECTION-HEADING', 'WK-SECTION-SUBHEADING',
  'WK-CALLOUT', 'WK-KV-ROW', 'WK-PANEL-TITLE', 'WK-PANEL-SUBTITLE',
  'WK-TOC-TITLE', 'WK-PROGRESS-LABEL', 'WK-CARD',
]);

const SVG_PLAY    = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>';
const SVG_PAUSE   = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 5h4v14H6zM14 5h4v14h-4z"/></svg>';
const SVG_RESTART = '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg>';

interface SpeakConfig {
  endpoint: string;
  voice: string;
  speed: number;
}

// ── Text extraction ──

/** Text nodes of one section, grouped into runs split at block boundaries. */
function collectRuns(root: Element): Text[][] {
  const runs: Text[][] = [];
  let current: Text[] = [];
  const flush = (): void => {
    if (current.length) { runs.push(current); current = []; }
  };
  const walk = (node: Node): void => {
    if (node.nodeType === Node.TEXT_NODE) {
      current.push(node as Text);
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const tag = (node as Element).tagName;
    if (SKIP_TAGS.has(tag)) return;
    const isBlock = BLOCK_TAGS.has(tag);
    if (isBlock) flush();
    Array.from(node.childNodes).forEach(walk);
    if (isBlock) flush();
  };
  walk(root);
  flush();
  return runs;
}

/**
 * Plans a section for playback: sentences (with offsets into the concatenated
 * text of all collected nodes, in document order) plus the nodes themselves.
 */
function planSection(section: Element): { sentences: Sentence[]; nodes: Text[] } {
  const nodes: Text[] = [];
  const sentences: Sentence[] = [];
  let offset = 0;
  for (const run of collectRuns(section)) {
    let runText = '';
    for (const n of run) {
      runText += n.textContent ?? '';
      nodes.push(n);
    }
    sentences.push(...segmentSentences(runText, offset));
    offset += runText.length;
  }
  return { sentences, nodes };
}

/**
 * Wraps the text of each sentence in <span class="wk-ra-sentence"> so the
 * active one can be highlighted. A sentence spanning several text nodes gets
 * several spans. Returns spans grouped by sentence index. Undone by unwrap().
 */
function wrapSentences(nodes: Text[], sentences: Sentence[]): Map<number, HTMLElement[]> {
  const byIdx = new Map<number, HTMLElement[]>();
  let offset = 0;
  for (const node of nodes) {
    const text = node.textContent ?? '';
    const nodeStart = offset;
    const nodeEnd = offset + text.length;
    offset = nodeEnd;
    if (!text || !node.parentNode) continue;

    const overlaps = sentences
      .map((s, i) => ({ s, i }))
      .filter(({ s }) => s.start < nodeEnd && s.end > nodeStart);
    if (!overlaps.length) continue;

    const frag = document.createDocumentFragment();
    let pos = nodeStart;
    for (const { s, i } of overlaps) {
      const from = Math.max(s.start, nodeStart);
      const to = Math.min(s.end, nodeEnd);
      if (from > pos) {
        frag.appendChild(document.createTextNode(text.slice(pos - nodeStart, from - nodeStart)));
      }
      const span = document.createElement('span');
      span.className = 'wk-ra-sentence';
      span.textContent = text.slice(from - nodeStart, to - nodeStart);
      frag.appendChild(span);
      const list = byIdx.get(i);
      if (list) { list.push(span); } else { byIdx.set(i, [span]); }
      pos = to;
    }
    if (pos < nodeEnd) {
      frag.appendChild(document.createTextNode(text.slice(pos - nodeStart)));
    }
    node.parentNode.replaceChild(frag, node);
  }
  return byIdx;
}

/** Replaces highlight spans with plain text and merges adjacent text nodes.
 * Surgical (no innerHTML rewrite) so sibling element identity — which the
 * fixation toggle depends on — is preserved. */
function unwrapSentences(section: Element, spans: Map<number, HTMLElement[]>): void {
  for (const list of spans.values()) {
    for (const span of list) {
      span.replaceWith(document.createTextNode(span.textContent ?? ''));
    }
  }
  section.normalize();
}

// ── Parts: what a session plays ──

/** One speech request's worth of a session: a group of sentences (uncached
 * mode) or one pre-synthesized part (cached mode). */
interface Part {
  /** Fetches the part's clip; resolves to an object URL. */
  fetch: () => Promise<string>;
  /** Elements marked .wk-ra-active while the part plays. */
  highlight: HTMLElement[];
}

interface Plan {
  parts: Part[];
  /** Sentence spans to unwrap when the session ends (uncached mode only). */
  spans: Map<number, HTMLElement[]>;
  /** Cached parts are synthesized at the provider's default speed, so the
   * speed control applies in the player (playbackRate), not the request. */
  cached: boolean;
}

/**
 * Cached mode: speak serve marks each speakable block of a section it
 * rendered with data-ra-chunk="<key> [<key> ...]", the parts that read it,
 * lists the section's parts in play order in data-ra-parts, and synthesizes
 * those parts ahead of time. Each part highlights every block it reads, so no
 * sentence spans are needed. Null when the section carries no keys.
 */
function planCached(section: HTMLElement, cfg: SpeakConfig): Plan | null {
  const readers = new Map<string, HTMLElement[]>();
  const lists: string[] = [];
  section.querySelectorAll<HTMLElement>('[data-ra-chunk]').forEach(el => {
    const list = el.dataset.raChunk ?? '';
    lists.push(list);
    for (const key of splitKeys(list)) {
      const els = readers.get(key);
      if (els) { els.push(el); } else { readers.set(key, [el]); }
    }
  });
  const keys = partKeys(section.dataset.raParts, lists);
  if (!keys.length) return null;
  const parts = keys.map(key => ({
    fetch: () => fetchCachedPart(cfg, key),
    highlight: readers.get(key) ?? [],
  }));
  return { parts, spans: new Map(), cached: true };
}

/**
 * Groups sentences into parts with the live ramp, each synthesized on
 * demand from its joined text. `spans` (by sentence index) supplies each
 * part's highlight; selection playback has none. A sentence with nothing
 * speakable (a bare quote mark, say) is left out rather than asking the
 * engine to synthesize an empty string.
 */
function sentenceParts(
  cfg: SpeakConfig,
  sentences: Sentence[],
  spans: Map<number, HTMLElement[]>,
): Part[] {
  const pieces = sentences
    .map((s, idx) => ({ text: speakable(s.text), idx }))
    .filter(p => p.text);
  return group(pieces.map(p => p.text), LIVE_RAMP).map(members => {
    const text = joinParts(members.map(m => pieces[m].text));
    return {
      fetch: () => fetchSpeech(cfg, text),
      highlight: members.flatMap(m => spans.get(pieces[m].idx) ?? []),
    };
  });
}

/** Uncached mode (present pages, any section without keys): the section's
 * sentences, wrapped in highlight spans. Null when nothing is speakable. */
function planUncached(section: HTMLElement, cfg: SpeakConfig): Plan | null {
  const { sentences, nodes } = planSection(section);
  if (!sentences.some(s => speakable(s.text))) return null;
  const spans = wrapSentences(nodes, sentences);
  return { parts: sentenceParts(cfg, sentences, spans), spans, cached: false };
}

// ── Playback controller (module-level: one session at a time) ──

// Parts fetched ahead of the one playing. A remote model takes seconds per
// part and handles a few requests in parallel, so two ahead keeps the next
// clip ready without flooding a local engine.
const PARTS_AHEAD = 2;

interface Session {
  section: HTMLElement;
  btn: HTMLButtonElement;
  restartBtn: HTMLButtonElement | null; // shown while this section plays; null for selection sessions
  cfg: SpeakConfig;
  plan: Plan;
  audio: HTMLAudioElement;
  pending: (Promise<string> | null)[]; // object URLs by part index
  queued: { url: string; idx: number } | null;
  lit: HTMLElement[]; // elements currently marked .wk-ra-active
  waiting: boolean; // the playing part's clip is still being fetched
  userPaused: boolean;
  cancelled: boolean;
}

let session: Session | null = null;

/** Ends any active playback session. Exported for the fixation toggle: fixation
 * rewrites the target subtrees via innerHTML, which detaches the highlight
 * spans a live session holds — ending the session first keeps the DOM clean.
 * Also on the Webkit global for pages that replace their sections: removing
 * <wk-read-aloud> doesn't end a session, which would play on from the
 * detached section. */
export function stopReadAloud(): void {
  if (session) endSession(session);
}

async function fetchAudio(cfg: SpeakConfig, url: string, init?: RequestInit): Promise<string> {
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch {
    throw new Error('the speech service at ' + (cfg.endpoint || location.origin) + ' is not reachable');
  }
  if (!res.ok) throw new Error(await failureReason(res));
  let blob: Blob;
  try {
    blob = await res.blob();
  } catch {
    throw new Error('the speech service broke off the audio mid-response');
  }
  if (!blob.size) throw new Error('the speech service returned empty audio');
  return URL.createObjectURL(blob);
}

/** Synthesizes text on demand (uncached mode and selections). */
function fetchSpeech(cfg: SpeakConfig, text: string): Promise<string> {
  return fetchAudio(cfg, cfg.endpoint + '/v1/audio/speech', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    // wav: the engine encodes mp3 via ffmpeg, which may not be installed.
    body: JSON.stringify({ model: TTS_MODEL, input: text, voice: cfg.voice, speed: cfg.speed, response_format: 'wav' }),
  });
}

/** Fetches a pre-synthesized part. One speak has not prepared yet moves to
 * the front of its queue and answers once synthesized, which can take tens
 * of seconds; the button's wait state shows that. */
function fetchCachedPart(cfg: SpeakConfig, key: string): Promise<string> {
  return fetchAudio(cfg, `${cfg.endpoint}/audio/${encodeURIComponent(key)}`);
}

/** The reason a speech request failed. speak answers failures with
 * {"error": {"message"}} (the OpenAI error shape); any other body, from an
 * older speak or another backend, falls back to the status code. */
async function failureReason(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json();
    const msg = (body as { error?: { message?: unknown } } | null)?.error?.message;
    if (typeof msg === 'string' && msg) return msg;
  } catch { /* not JSON */ }
  return 'the speech service returned ' + res.status;
}

/** Ends a session that failed and says why: the button shows an error state
 * until the next click, a toast names the reason, and a wk-read-aloud-error
 * event lets the page react (speak's page re-checks its engine banner). */
function failSession(s: Session, err: unknown): void {
  const reason = err instanceof Error ? err.message : String(err);
  console.warn('wk-read-aloud:', reason);
  endSession(s);
  s.btn.classList.add('wk-ra-error');
  s.btn.title = 'Read-aloud failed: ' + reason + '. Click to try again.';
  s.btn.setAttribute('aria-label', 'Read-aloud failed. Click to try again.');
  showErrorToast('Read-aloud failed: ' + reason);
  document.dispatchEvent(new CustomEvent('wk-read-aloud-error', { detail: { reason } }));
}

/** Shows a red toast, reusing the page's <wk-toast-host> or adding one. */
function showErrorToast(text: string): void {
  let host = document.querySelector('wk-toast-host');
  if (!host) {
    host = document.createElement('wk-toast-host');
    document.body.appendChild(host);
  }
  const toast = document.createElement('wk-toast');
  toast.setAttribute('variant', 'err');
  toast.setAttribute('role', 'alert');
  toast.textContent = text;
  host.appendChild(toast);
  setTimeout(() => toast.remove(), ERROR_TOAST_MS);
}

function ensureClip(s: Session, idx: number): Promise<string> {
  let p = s.pending[idx];
  if (!p) {
    p = s.plan.parts[idx].fetch();
    s.pending[idx] = p;
  }
  return p;
}

/** Starts fetching the parts after idx, so each is ready (or on its way)
 * by the time the one before it ends. */
function prefetch(s: Session, idx: number): void {
  const last = Math.min(idx + PARTS_AHEAD, s.plan.parts.length - 1);
  for (let i = idx + 1; i <= last; i++) {
    void ensureClip(s, i).catch(() => { /* surfaced when played */ });
  }
}

function setBtn(btn: HTMLButtonElement, state: 'idle' | 'playing' | 'paused'): void {
  btn.innerHTML = state === 'playing' ? SVG_PAUSE : SVG_PLAY;
  btn.classList.remove('wk-ra-error');
  btn.title = btn.classList.contains('wk-ra-float') ? 'Read selection aloud' : 'Read aloud';
  btn.classList.toggle('active', state !== 'idle');
  btn.setAttribute('aria-pressed', String(state === 'playing'));
  btn.setAttribute('aria-label', state === 'playing' ? 'Pause reading' : 'Read section aloud');
}

/** Pulses the button while the playing part's clip is still on its way, so
 * a long wait for a part speak has not prepared yet is visible. A paused
 * session doesn't pulse; an ended one leaves the button alone, since a
 * restart may already have handed it to a newer session. */
function syncWait(s: Session): void {
  if (s.cancelled) return;
  s.btn.classList.toggle('wk-ra-wait', s.waiting && !s.userPaused);
}

/** Cached parts carry no speed, so the player applies it. The default rate
 * too: loading a new src resets playbackRate to it. */
function applyRate(s: Session): void {
  const rate = s.plan.cached ? s.cfg.speed : 1;
  s.audio.defaultPlaybackRate = rate;
  s.audio.playbackRate = rate;
}

function highlight(s: Session, idx: number): void {
  const prev = s.lit;
  const cur = s.plan.parts[idx].highlight;
  prev.forEach(el => { if (!cur.includes(el)) el.classList.remove('wk-ra-active'); });
  cur.forEach(el => el.classList.add('wk-ra-active'));
  s.lit = cur;
  // Scroll to the first newly lit element only: a paragraph running on from
  // the previous part stays where the reader has it.
  const fresh = cur.find(el => !prev.includes(el));
  if (!fresh) return;
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  fresh.scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'nearest' });
}

function endSession(s: Session): void {
  s.cancelled = true;
  s.audio.pause();
  s.audio.removeAttribute('src');
  for (const p of s.pending) {
    if (p) p.then(url => URL.revokeObjectURL(url)).catch(() => { /* fetch already failed */ });
  }
  s.lit.forEach(el => el.classList.remove('wk-ra-active'));
  s.lit = [];
  if (s.plan.spans.size) unwrapSentences(s.section, s.plan.spans);
  s.btn.classList.remove('wk-ra-wait');
  setBtn(s.btn, 'idle');
  if (s.restartBtn) s.restartBtn.hidden = true;
  if (session === s) session = null;
}

/** Plays part idx's clip; when it ends, the next part follows. */
function startClip(s: Session, url: string, idx: number): Promise<void> {
  s.audio.src = url;
  applyRate(s);
  s.audio.onended = (): void => { void playFrom(s, idx + 1); };
  return s.audio.play();
}

async function playFrom(s: Session, idx: number): Promise<void> {
  if (s.cancelled) return;
  if (idx >= s.plan.parts.length) { endSession(s); return; }
  highlight(s, idx);
  const clip = ensureClip(s, idx);
  prefetch(s, idx);
  s.waiting = true;
  syncWait(s);
  let url: string;
  try {
    url = await clip;
  } catch (err) {
    if (!s.cancelled) failSession(s, err);
    return;
  } finally {
    s.waiting = false;
    syncWait(s);
  }
  if (s.cancelled) return;
  if (s.userPaused) { s.queued = { url, idx }; return; }
  try {
    await startClip(s, url, idx);
  } catch (err) {
    // Most likely NotAllowedError: no user activation (autoplay policy).
    if (!s.cancelled) failSession(s, err);
  }
}

function newSession(
  section: HTMLElement,
  btn: HTMLButtonElement,
  restartBtn: HTMLButtonElement | null,
  cfg: SpeakConfig,
  plan: Plan,
): void {
  const s: Session = {
    section, btn, restartBtn, cfg, plan,
    audio: new Audio(),
    pending: new Array<Promise<string> | null>(plan.parts.length).fill(null),
    queued: null,
    lit: [],
    waiting: false,
    userPaused: false,
    cancelled: false,
  };
  session = s;
  setBtn(btn, 'playing');
  if (restartBtn) restartBtn.hidden = false;
  void playFrom(s, 0);
}

function startSession(
  section: HTMLElement,
  btn: HTMLButtonElement,
  cfg: SpeakConfig,
  restartBtn: HTMLButtonElement | null = null,
): void {
  if (session) endSession(session);
  const plan = planCached(section, cfg) ?? planUncached(section, cfg);
  if (!plan) return;
  newSession(section, btn, restartBtn, cfg, plan);
}

// Selection playback has no highlight spans — the user's own selection is the
// visual. The float button doubles as the section anchor so the pause/resume
// toggle in onButton works unchanged.
function startSelectionSession(text: string, btn: HTMLButtonElement, cfg: SpeakConfig): void {
  if (session) endSession(session);
  const parts = sentenceParts(cfg, segmentSentences(text), new Map());
  if (!parts.length) return;
  newSession(btn, btn, null, cfg, { parts, spans: new Map(), cached: false });
}

function onButton(
  section: HTMLElement,
  btn: HTMLButtonElement,
  cfg: SpeakConfig,
  restartBtn: HTMLButtonElement | null = null,
): void {
  if (session && session.section === section) {
    const s = session;
    if (s.userPaused) {
      s.userPaused = false;
      setBtn(btn, 'playing');
      syncWait(s);
      if (s.queued) {
        const q = s.queued;
        s.queued = null;
        void startClip(s, q.url, q.idx).catch(() => endSession(s));
      } else if (!s.waiting && s.audio.src) {
        void s.audio.play().catch(() => endSession(s));
      }
      // else: paused and resumed while the part's clip is still on its way
      // (before the first clip, or between parts). Nothing is queued and the
      // audio element holds no src or the finished previous clip, so play()
      // would reject or replay it. With userPaused cleared, the in-flight
      // playFrom plays the clip itself when the fetch resolves.
    } else {
      s.userPaused = true;
      s.audio.pause();
      setBtn(btn, 'paused');
      syncWait(s);
    }
    return;
  }
  startSession(section, btn, cfg, restartBtn);
}

// ── Selection speaker: select any text, get a floating play button ──

const MIN_SELECTION_CHARS = 2;

function installSelectionSpeaker(cfg: SpeakConfig): void {
  let floatBtn: HTMLButtonElement | null = null;
  let selectedText = '';

  const removeFloat = (): void => {
    floatBtn?.remove();
    floatBtn = null;
  };

  const showFloat = (): void => {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed) return;
    const text = sel.toString().trim();
    if (text.length < MIN_SELECTION_CHARS) return;
    // Don't offer to read our own controls.
    const anchor = sel.anchorNode instanceof Element ? sel.anchorNode : sel.anchorNode?.parentElement;
    if (anchor?.closest('.wk-ra-btn, wk-header')) return;

    selectedText = text;
    const rect = sel.getRangeAt(0).getBoundingClientRect();
    if (!floatBtn) {
      floatBtn = document.createElement('button');
      floatBtn.className = 'wk-ra-btn wk-ra-float';
      floatBtn.type = 'button';
      floatBtn.title = 'Read selection aloud';
      setBtn(floatBtn, 'idle');
      floatBtn.setAttribute('aria-label', 'Read selection aloud');
      // Keep the selection: a mousedown on the button would collapse it
      // before click fires.
      floatBtn.addEventListener('pointerdown', e => e.preventDefault());
      floatBtn.addEventListener('click', () => {
        if (session && session.btn === floatBtn) {
          onButton(floatBtn, floatBtn, cfg); // toggle pause/resume
          return;
        }
        startSelectionSession(selectedText, floatBtn as HTMLButtonElement, cfg);
      });
      document.body.appendChild(floatBtn);
    }
    floatBtn.style.left = `${Math.round(rect.right + window.scrollX + 6)}px`;
    floatBtn.style.top = `${Math.round(rect.top + window.scrollY - 6)}px`;
  };

  document.addEventListener('pointerup', () => setTimeout(showFloat, 0));
  document.addEventListener('keyup', e => {
    if (e.shiftKey || e.key === 'Shift') setTimeout(showFloat, 0);
  });
  // Esc: drop the selection, its float button, and any selection playback.
  document.addEventListener('keydown', e => {
    if (e.key !== 'Escape') return;
    if (session && floatBtn && session.btn === floatBtn) endSession(session);
    window.getSelection()?.removeAllRanges();
    removeFloat();
  });
  document.addEventListener('selectionchange', () => {
    const sel = window.getSelection();
    if (!sel || sel.isCollapsed) {
      // Keep the button while it drives an active selection session — it's
      // the pause control.
      if (!(session && floatBtn && session.btn === floatBtn)) removeFloat();
    }
  });
}

// ── <wk-read-aloud> custom element ──

async function probe(endpoint: string): Promise<boolean> {
  try {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), PROBE_TIMEOUT_MS);
    // Any HTTP response (even 404) means the service is reachable.
    await fetch(endpoint + '/', { signal: ctrl.signal });
    clearTimeout(timer);
    return true;
  } catch {
    return false;
  }
}

export class WkReadAloud extends HTMLElement {
  private _initialized = false;

  connectedCallback(): void {
    if (this._initialized) return;
    this._initialized = true;
    void this._setup();
  }

  private async _setup(): Promise<void> {
    const savedSpeed = localStorage.getItem('webkit-ra-speed');
    const attrSpeed = this.getAttribute('speed');
    const initialSpeed = parseFloat(savedSpeed ?? attrSpeed ?? '1') || 1;

    const cfg: SpeakConfig = {
      endpoint: (this.getAttribute('endpoint') ?? DEFAULT_ENDPOINT).replace(/\/+$/, ''),
      voice: this.getAttribute('voice') ?? DEFAULT_VOICE,
      speed: initialSpeed,
    };

    document.addEventListener('wk-speedchange', ((e: CustomEvent<{ speed: number }>) => {
      cfg.speed = e.detail.speed;
      // A cached session applies speed in the player, so it follows the
      // control mid-clip; uncached parts take it with their next request.
      if (session?.cfg === cfg) applyRate(session);
    }) as EventListener);

    if (!(await probe(cfg.endpoint))) return;

    const selector = this.getAttribute('targets');
    if (selector) {
      document.querySelectorAll<HTMLElement>(selector).forEach(section => {
        // Restart: hidden until this section is playing, then rewinds to the
        // start. startSession() ends any current session and re-plans from
        // sentence 0, so restarting is just starting a fresh session.
        const restartBtn = document.createElement('button');
        restartBtn.className = 'wk-ra-btn';
        restartBtn.type = 'button';
        restartBtn.hidden = true;
        restartBtn.title = 'Restart from beginning';
        restartBtn.innerHTML = SVG_RESTART;
        restartBtn.setAttribute('aria-label', 'Restart from beginning');
        restartBtn.addEventListener('click', () => startSession(section, btn, cfg, restartBtn));

        const btn = document.createElement('button');
        btn.className = 'wk-ra-btn';
        btn.type = 'button';
        btn.title = 'Read aloud';
        setBtn(btn, 'idle');
        btn.addEventListener('click', () => onButton(section, btn, cfg, restartBtn));

        // btn first → floats rightmost; restartBtn sits to its left.
        section.insertBefore(restartBtn, section.firstChild);
        section.insertBefore(btn, section.firstChild);
      });
    }

    // Independent of targets: select any text on the page to get a floating
    // play button for just that selection.
    installSelectionSpeaker(cfg);
  }
}
