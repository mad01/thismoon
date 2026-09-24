// read-aloud.ts — <wk-read-aloud>: per-section text-to-speech playback against
// a local OpenAI-compatible speech endpoint (Kokoro via mlx-audio, fronted by
// d-man at http://speak.this). Injects a play/pause button into each `targets`
// match; stays fully inert when the endpoint is unreachable, so pages work
// unchanged on hosts without the speak service. Sections whose blocks name
// pre-synthesized parts (data-ra-chunk) replay cached audio: speak serve
// stamps the pages it renders, and with the `prepare` attribute the element
// registers the page's own text with speak (POST /read), stamps the blocks
// itself and shows how far synthesis got. Anything else is synthesized live,
// a few sentences per request.

import {
  barView, BlockCollector, DocStatus, downloadName, inProgress, parseDocStatus, parseRegistration,
  pollDelay, prepareQuery, readRequest, Registration, SectionStatus, sectionView, statusOfSection,
} from './prepare.js';
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
  /** Prepared mode's controller, once the page is registered with speak;
   * null in every other mode. A session consults it when a cached part
   * comes back 404. */
  prepared: Preparer | null;
}

// ── Text extraction ──

/** The tag name as SKIP_TAGS and BLOCK_TAGS spell it. HTML elements report
 * uppercase, but an inline <svg> (or any foreign element) keeps its case. */
function tagOf(el: Element): string {
  return el.tagName.toUpperCase();
}

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
    const tag = tagOf(node as Element);
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
 * Cached mode: each speakable block of the section carries
 * data-ra-chunk="<key> [<key> ...]", the parts that read it, and the section
 * lists its parts in play order in data-ra-parts; speak synthesizes those
 * parts ahead of time (stamped by speak serve on the pages it renders, or by
 * prepared mode below). Each part highlights every block it reads, so no
 * sentence spans are needed. The section counts as a block of its own when
 * it holds text directly (a summary paragraph that is its own target). Null
 * when the section carries no keys.
 */
function planCached(section: HTMLElement, cfg: SpeakConfig): Plan | null {
  const readers = new Map<string, HTMLElement[]>();
  const lists: string[] = [];
  const blocks = Array.from(section.querySelectorAll<HTMLElement>('[data-ra-chunk]'));
  if (section.hasAttribute('data-ra-chunk')) blocks.unshift(section);
  blocks.forEach(el => {
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
  /** Prepared mode's stamping generation this plan was made from; a 404 on
   * a part from an older generation just needs a re-plan. */
  epoch: number;
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

/** A failed request to the speech service: the reason, plus the HTTP status
 * (0 when no response came) so a missing document can be told apart. */
class SpeechError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

function unreachable(cfg: SpeakConfig): SpeechError {
  return new SpeechError(
    'the speech service at ' + (cfg.endpoint || location.origin) + ' is not reachable', 0,
  );
}

function isGone(err: unknown): boolean {
  return err instanceof SpeechError && err.status === 404;
}

function reasonOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/** Fetches a clip (a part, or a joined download) as a blob. */
async function fetchClip(cfg: SpeakConfig, url: string, init?: RequestInit): Promise<Blob> {
  let res: Response;
  try {
    res = await fetch(url, init);
  } catch {
    throw unreachable(cfg);
  }
  if (!res.ok) throw new SpeechError(await failureReason(res), res.status);
  let blob: Blob;
  try {
    blob = await res.blob();
  } catch {
    throw new SpeechError('the speech service broke off the audio mid-response', res.status);
  }
  if (!blob.size) throw new SpeechError('the speech service returned empty audio', res.status);
  return blob;
}

async function fetchAudio(cfg: SpeakConfig, url: string, init?: RequestInit): Promise<string> {
  return URL.createObjectURL(await fetchClip(cfg, url, init));
}

/** One JSON request to speak's document API (registration, status, prepare);
 * resolves to the parsed body. */
async function requestJSON(cfg: SpeakConfig, method: string, path: string, body?: unknown): Promise<unknown> {
  const headers: Record<string, string> = { accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  let res: Response;
  try {
    res = await fetch(cfg.endpoint + path, {
      method, headers, cache: 'no-store',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw unreachable(cfg);
  }
  if (!res.ok) throw new SpeechError(await failureReason(res), res.status);
  try {
    return await res.json();
  } catch {
    throw new SpeechError('the speech service answered with malformed JSON', res.status);
  }
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
  const reason = reasonOf(err);
  console.warn('wk-read-aloud:', reason);
  endSession(s);
  s.btn.classList.add('wk-ra-error');
  s.btn.title = 'Read-aloud failed: ' + reason + '. Click to try again.';
  s.btn.setAttribute('aria-label', 'Read-aloud failed. Click to try again.');
  showToast('Read-aloud failed: ' + reason);
  document.dispatchEvent(new CustomEvent('wk-read-aloud-error', { detail: { reason } }));
}

/** Shows a toast, red for a failure (the default) or the neutral one for a
 * notice, reusing the page's <wk-toast-host> or adding one. */
function showToast(text: string, error = true): void {
  let host = document.querySelector('wk-toast-host');
  if (!host) {
    host = document.createElement('wk-toast-host');
    document.body.appendChild(host);
  }
  const toast = document.createElement('wk-toast');
  if (error) toast.setAttribute('variant', 'err');
  toast.setAttribute('role', error ? 'alert' : 'status');
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
  // Fetching a cached part makes speak synthesize it, so the status moves.
  if (s.plan.cached) s.cfg.prepared?.refresh();
  s.waiting = true;
  syncWait(s);
  let url: string;
  try {
    url = await clip;
  } catch (err) {
    if (s.cancelled) return;
    if (s.plan.cached && isGone(err) && await recoverSession(s, idx)) return;
    failSession(s, err);
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
  from = 0,
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
    epoch: cfg.prepared?.epoch ?? 0,
  };
  session = s;
  setBtn(btn, 'playing');
  if (restartBtn) restartBtn.hidden = false;
  void playFrom(s, from);
}

/** Starts a section from part `from` (clamped into the plan; 0 is the
 * start). Cached and uncached plans cut parts differently, so a part index
 * only carries over between two cached plans of the same text. */
function startSession(
  section: HTMLElement,
  btn: HTMLButtonElement,
  cfg: SpeakConfig,
  restartBtn: HTMLButtonElement | null = null,
  from = 0,
): void {
  if (session) endSession(session);
  const plan = planCached(section, cfg) ?? planUncached(section, cfg);
  if (!plan) return;
  newSession(section, btn, restartBtn, cfg, plan, Math.min(from, plan.parts.length - 1));
}

/**
 * A cached part answered 404: speak no longer has the document (it
 * restarted), or prepared mode stamped the page anew since this session was
 * planned. Registers again when needed and restarts the section from the
 * same part, whose position is stable for unchanged text. When prepared mode
 * is over (re-registration failed, and said so), the session just ends: the
 * next play reads live. False when this is not prepared mode's business.
 */
async function recoverSession(s: Session, idx: number): Promise<boolean> {
  const prep = s.cfg.prepared;
  if (!prep) return false;
  const ok = prep.epoch !== s.epoch || await prep.docGone();
  if (s.cancelled) return true;
  const { section, btn, cfg, restartBtn } = s;
  endSession(s);
  if (ok) startSession(section, btn, cfg, restartBtn, idx);
  return true;
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
    // Don't offer to read our own controls (the page bar lives in the element).
    const anchor = sel.anchorNode instanceof Element ? sel.anchorNode : sel.anchorNode?.parentElement;
    if (anchor?.closest('.wk-ra-btn, wk-header, wk-read-aloud')) return;

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

// ── Prepared mode: the page registers its own text with speak ──

/** A readable block of a target section: the element the text belongs to
 * (stamped with the keys that read it) and the text speak is sent. */
interface Block {
  el: HTMLElement;
  text: string;
}

/**
 * The section's readable text grouped by block: every text node goes to its
 * nearest block-level ancestor (BLOCK_TAGS) inside the section, or to the
 * section itself when it holds text directly. Blocks come in the order of
 * their first non-blank text, and a block's text is cut where a nested block
 * or a <br> interrupts it, the boundaries collectRuns flushes at, so the
 * registered text reads as live mode would read it. SKIP_TAGS subtrees are
 * left out, as planSection leaves them out.
 */
function collectBlocks(section: HTMLElement): Block[] {
  const blocks = new BlockCollector<HTMLElement>();
  const walk = (node: Node, block: HTMLElement): void => {
    if (node.nodeType === Node.TEXT_NODE) {
      blocks.text(block, node.textContent ?? '');
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const el = node as HTMLElement;
    const tag = tagOf(el);
    if (SKIP_TAGS.has(tag)) return;
    if (!BLOCK_TAGS.has(tag)) {
      Array.from(el.childNodes).forEach(child => walk(child, block));
      return;
    }
    // A nested block (or a <br>, childless) ends the enclosing block's run.
    blocks.cut(block);
    Array.from(el.childNodes).forEach(child => walk(child, el));
  };
  Array.from(section.childNodes).forEach(child => walk(child, section));
  return blocks.blocks();
}

/** Writes speak's keys onto the page: data-ra-chunk on each block that
 * something reads, data-ra-parts on each section with parts, so planCached
 * takes over from here. */
function stamp(sections: HTMLElement[], blocks: Block[][], reg: Registration): void {
  sections.forEach((section, i) => {
    const answer = reg.sections[i];
    blocks[i].forEach((block, j) => {
      const keys = answer.blocks[j];
      if (keys.length) block.el.dataset.raChunk = keys.join(' ');
      else delete block.el.dataset.raChunk;
    });
    if (answer.parts.length) section.dataset.raParts = answer.parts.join(' ');
    else delete section.dataset.raParts;
  });
}

/** Removes every key from the sections, so they read live again. */
function unstamp(sections: HTMLElement[]): void {
  for (const section of sections) {
    delete section.dataset.raParts;
    delete section.dataset.raChunk;
    section.querySelectorAll<HTMLElement>('[data-ra-chunk]').forEach(el => {
      delete el.dataset.raChunk;
    });
  }
}

// The status UI is updated in place, never rebuilt: polling renders every 2s
// while parts are in progress, and a button replaced between mousedown and
// mouseup would swallow the click. Writing only on change also keeps an idle
// poll from touching the DOM at all.
function setText(el: HTMLElement, text: string): void {
  if (el.textContent !== text) el.textContent = text;
}

function setTitle(el: HTMLElement, title: string): void {
  if (el.title !== title) el.title = title;
}

/** A labelled action button in the play buttons' style. */
function textButton(label: string, title: string): HTMLButtonElement {
  const b = document.createElement('button');
  b.className = 'wk-ra-btn wk-ra-text';
  b.type = 'button';
  b.textContent = label;
  b.title = title;
  return b;
}

/** Saves a fetched clip through a temporary object URL. A cross-origin
 * <a download> sends no Origin and speak refuses it, so downloads go
 * fetch → blob → object URL → click. */
function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

/** The page bar: the audio line, the failure reason behind it, and the
 * page-wide actions. Lives inside the <wk-read-aloud> element. */
interface Bar {
  root: HTMLElement;
  line: HTMLElement;
  reason: HTMLElement;
  prepareAll: HTMLButtonElement;
  retryFailed: HTMLButtonElement;
  download: HTMLButtonElement;
}

/** One section's status controls, direct children of the section beside its
 * play buttons. wk-badge and button are SKIP_TAGS, so nothing reads them. */
interface SectionControls {
  badge: HTMLElement;
  retry: HTMLButtonElement;
  download: HTMLButtonElement;
}

/**
 * Prepared mode (the `prepare` attribute): registers the page's readable
 * text with speak as a document (POST /read), stamps the blocks with the
 * part keys speak answers so cached mode plays them, and shows how far
 * synthesis got: a bar in the element for the page and a badge per section,
 * with prepare, retry and download actions. Polls the document while parts
 * are in progress. A document speak has forgotten (it restarted) is
 * registered again; if that fails the stamps come off, a toast says so once,
 * and the page reads live from then on.
 */
class Preparer {
  /** Bumped on every stamping, so a session can tell its plan is stale. */
  epoch = 0;

  private status: DocStatus | null = null;
  private pollTimer: number | null = null;
  /** Consecutive status polls that failed; sets the backoff. */
  private failures = 0;
  private registering: Promise<boolean> | null = null;
  private stopped = false;
  private bar: Bar | null = null;
  private controls: (SectionControls | null)[] = [];

  constructor(
    private readonly host: HTMLElement,
    private readonly cfg: SpeakConfig,
    private readonly name: string,
    private readonly sections: HTMLElement[],
    /** Per section, the restart button the status controls follow. */
    private readonly anchors: HTMLElement[],
  ) {}

  /** Registers the page; false when it could not be (the page reads live,
   * the reason goes to the console only: nothing is wrong for the reader). */
  start(): Promise<boolean> {
    return this.register();
  }

  private async register(): Promise<boolean> {
    const blocks = this.sections.map(collectBlocks);
    if (!blocks.some(b => b.length)) {
      console.debug('wk-read-aloud: nothing to prepare, no section holds readable text');
      return false;
    }
    let reg: Registration | null;
    try {
      const body = readRequest(this.name, blocks.map(b => b.map(block => block.text)));
      reg = parseRegistration(await requestJSON(this.cfg, 'POST', '/read', body), blocks.map(b => b.length));
      if (!reg) console.debug('wk-read-aloud: speak answered the registration in an unexpected shape');
    } catch (err) {
      console.debug('wk-read-aloud: registering the page with speak failed:', reasonOf(err));
      return false;
    }
    if (!reg || this.stopped) return false;
    unstamp(this.sections);
    stamp(this.sections, blocks, reg);
    this.epoch++;
    this.failures = 0;
    this.render(reg.doc);
    return true;
  }

  private render(status: DocStatus): void {
    if (this.stopped) return;
    this.status = status;
    this.renderBar(status);
    this.sections.forEach((_, i) => this.renderSection(i, statusOfSection(status, i)));
    if (inProgress(status.total)) this.schedulePoll();
    else this.stopPolling();
  }

  private renderBar(status: DocStatus): void {
    if (!status.total.parts) {
      this.bar?.root.remove();
      this.bar = null;
      return;
    }
    const bar = this.bar ?? (this.bar = this.buildBar());
    const view = barView(status);
    setText(bar.line, view.line);
    // One line with an ellipsis, full text in the title, so a long reason
    // never moves the buttons.
    setText(bar.reason, view.reason);
    setTitle(bar.reason, view.reason);
    bar.reason.hidden = !view.reason;
    bar.prepareAll.disabled = !view.canPrepare;
    setText(bar.retryFailed, `Retry failed (${view.failed})`);
    bar.retryFailed.hidden = !view.failed;
    bar.download.disabled = !view.canDownload;
  }

  private buildBar(): Bar {
    const line = document.createElement('span');
    line.className = 'wk-ra-bar-line';
    const reason = document.createElement('span');
    reason.className = 'wk-ra-bar-reason';
    reason.hidden = true;
    const prepareAll = textButton('Prepare all', 'Synthesize every part that is not ready yet');
    prepareAll.addEventListener('click', () => void this.prepare(prepareAll, 0, false));
    const retryFailed = textButton('Retry failed', 'Try the failed parts again');
    retryFailed.hidden = true;
    retryFailed.addEventListener('click', () => void this.prepare(retryFailed, 0, true));
    const download = textButton('Download page audio', 'Download the whole page as one audio file');
    download.addEventListener('click', () => void this.download(download, 0));
    const actions = document.createElement('span');
    actions.className = 'wk-ra-bar-actions';
    actions.append(prepareAll, retryFailed, download);
    const root = document.createElement('div');
    root.className = 'wk-ra-bar';
    root.append(line, reason, actions);
    this.host.appendChild(root);
    return { root, line, reason, prepareAll, retryFailed, download };
  }

  private renderSection(i: number, sec: SectionStatus | undefined): void {
    let c = this.controls[i] ?? null;
    if (!sec?.parts) {
      if (c) this.removeControls(i);
      return;
    }
    if (!c) {
      c = this.buildControls(i);
      this.controls[i] = c;
    }
    const view = sectionView(sec);
    if (c.badge.getAttribute('variant') !== view.variant) c.badge.setAttribute('variant', view.variant);
    setText(c.badge, view.text);
    setTitle(c.badge, view.title);
    c.retry.hidden = !view.retry;
    c.download.hidden = !view.download;
  }

  private buildControls(i: number): SectionControls {
    const section = i + 1; // speak numbers sections from 1
    const badge = document.createElement('wk-badge');
    badge.className = 'wk-ra-state';
    const retry = textButton('Retry', "Try this section's failed parts again");
    retry.hidden = true;
    retry.addEventListener('click', () => void this.prepare(retry, section, true));
    const download = textButton('Download', 'Download this section as one audio file');
    download.hidden = true;
    download.addEventListener('click', () => void this.download(download, section));
    // Floats stack right to left in DOM order: the play controls stay
    // rightmost, then download, retry and the badge.
    this.anchors[i].after(download, retry, badge);
    return { badge, retry, download };
  }

  private removeControls(i: number): void {
    const c = this.controls[i];
    if (!c) return;
    c.badge.remove();
    c.retry.remove();
    c.download.remove();
    this.controls[i] = null;
  }

  /** Runs an action with its button disabled, then renders the status again
   * so every control ends in the state the status calls for. */
  private async busy(btn: HTMLButtonElement, action: () => Promise<void>): Promise<void> {
    btn.disabled = true;
    try {
      await action();
    } finally {
      btn.disabled = false;
      if (this.status) this.render(this.status);
    }
  }

  /** Queues parts of one section (1-based) or the whole document (0): the
   * failed ones only (Retry, so a retry never starts synthesis of parts the
   * reader left unprepared), or every idle and failed one (Prepare all). */
  private prepare(btn: HTMLButtonElement, section: number, failedOnly: boolean): Promise<void> {
    return this.busy(btn, async () => {
      const id = this.status?.id;
      if (!id) return;
      try {
        const path = `/doc/${encodeURIComponent(id)}/prepare${prepareQuery(section, failedOnly)}`;
        const status = parseDocStatus(await requestJSON(this.cfg, 'POST', path));
        this.failures = 0; // speak answered
        if (status) this.render(status);
        else this.refresh();
      } catch (err) {
        if (isGone(err)) void this.docGone();
        else showToast('Prepare audio: ' + reasonOf(err));
      }
    });
  }

  /** Downloads the joined audio of one section (1-based) or the page (0).
   * speak answers 409 while parts are missing; that message is a notice,
   * not a failure. */
  private download(btn: HTMLButtonElement, section: number): Promise<void> {
    return this.busy(btn, async () => {
      const st = this.status;
      if (!st) return;
      try {
        const url = `${this.cfg.endpoint}/doc/${encodeURIComponent(st.id)}/audio${section ? `?section=${section}` : ''}`;
        const blob = await fetchClip(this.cfg, url, { cache: 'no-store' });
        saveBlob(blob, downloadName(st.name, section, blob.type));
      } catch (err) {
        if (isGone(err)) void this.docGone();
        else showToast('Download audio: ' + reasonOf(err), !(err instanceof SpeechError && err.status === 409));
      }
    });
  }

  /** Arms the next poll: the normal cadence, or the backoff after failures. */
  private schedulePoll(): void {
    this.stopPolling();
    // A poll that resolves after the page dropped the element must not arm
    // the next one; connectedCallback's refresh() picks polling up again.
    if (!this.host.isConnected) return;
    this.pollTimer = window.setTimeout(() => {
      this.pollTimer = null;
      void this.poll();
    }, pollDelay(this.failures));
  }

  stopPolling(): void {
    if (this.pollTimer !== null) clearTimeout(this.pollTimer);
    this.pollTimer = null;
  }

  /** Asks for the status again at the normal cadence: a session started or a
   * part was fetched, so speak has work (and, for a fetched part, is up),
   * which also ends any backoff. */
  refresh(): void {
    if (this.stopped || !this.status) return;
    this.failures = 0;
    this.schedulePoll();
  }

  /** A status poll that failed for a reason other than a missing document:
   * speak may be restarting, or its proxy answers for it. The next poll
   * tells, after a wait that grows with each failure in a row. */
  private pollFailed(): void {
    this.failures++;
    this.schedulePoll();
  }

  private async poll(): Promise<void> {
    const id = this.status?.id;
    if (!id || this.stopped) return;
    try {
      const status = parseDocStatus(await requestJSON(this.cfg, 'GET', `/doc/${encodeURIComponent(id)}`));
      if (this.stopped || this.status?.id !== id) return; // registered anew meanwhile
      if (!status) { this.pollFailed(); return; }
      this.failures = 0;
      this.render(status);
    } catch (err) {
      if (this.stopped || this.status?.id !== id) return;
      if (isGone(err)) void this.docGone();
      else this.pollFailed();
    }
  }

  /** speak no longer knows the document (it restarted): registers the page
   * again so the keys resolve, and picks up polling. One attempt, shared by
   * whoever noticed first (a poll, a download, a playing part); when it
   * fails, the page falls back to reading live. Resolves to whether prepared
   * mode goes on. */
  docGone(): Promise<boolean> {
    if (this.stopped) return Promise.resolve(false);
    if (!this.registering) {
      this.registering = this.register().then(ok => {
        this.registering = null;
        if (!ok) this.fallback();
        return ok;
      });
    }
    return this.registering;
  }

  private fallback(): void {
    this.teardown();
    unstamp(this.sections);
    this.cfg.prepared = null;
    showToast('Prepared audio is gone: the speech service no longer has this page. Reading live from now on.');
  }

  /** Stops polling and removes the status UI. */
  private teardown(): void {
    this.stopped = true;
    this.stopPolling();
    this.bar?.root.remove();
    this.bar = null;
    this.controls.forEach((_, i) => this.removeControls(i));
    this.controls = [];
    this.status = null;
  }
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

/** Injects a section's play/pause and restart buttons as its first children
 * and returns the restart button, after which prepared mode adds its status
 * controls. */
function installSectionButtons(section: HTMLElement, cfg: SpeakConfig): HTMLButtonElement {
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
  return restartBtn;
}

export class WkReadAloud extends HTMLElement {
  private _initialized = false;
  private _preparer: Preparer | null = null;

  connectedCallback(): void {
    if (this._initialized) {
      this._preparer?.refresh(); // moved in the page: pick up polling again
      return;
    }
    this._initialized = true;
    void this._setup();
  }

  disconnectedCallback(): void {
    // A page that replaces its content drops the element; don't keep
    // polling speak for a page that is gone.
    this._preparer?.stopPolling();
  }

  private async _setup(): Promise<void> {
    const savedSpeed = localStorage.getItem('webkit-ra-speed');
    const attrSpeed = this.getAttribute('speed');
    const initialSpeed = parseFloat(savedSpeed ?? attrSpeed ?? '1') || 1;

    const cfg: SpeakConfig = {
      endpoint: (this.getAttribute('endpoint') ?? DEFAULT_ENDPOINT).replace(/\/+$/, ''),
      voice: this.getAttribute('voice') ?? DEFAULT_VOICE,
      speed: initialSpeed,
      prepared: null,
    };

    document.addEventListener('wk-speedchange', ((e: CustomEvent<{ speed: number }>) => {
      cfg.speed = e.detail.speed;
      // A cached session applies speed in the player, so it follows the
      // control mid-clip; uncached parts take it with their next request.
      if (session?.cfg === cfg) applyRate(session);
    }) as EventListener);

    if (!(await probe(cfg.endpoint))) return;

    const selector = this.getAttribute('targets');
    const sections = selector ? Array.from(document.querySelectorAll<HTMLElement>(selector)) : [];
    const anchors = sections.map(section => installSectionButtons(section, cfg));

    // Independent of targets: select any text on the page to get a floating
    // play button for just that selection.
    installSelectionSpeaker(cfg);

    // Prepared mode registers the page with speak after the buttons are up:
    // a click before it answers reads live, the next one plays cached. The
    // document is named for the status bar and the downloads.
    if (this.hasAttribute('prepare') && sections.length) {
      const name = this.getAttribute('name') || document.title || 'page';
      const preparer = new Preparer(this, cfg, name, sections, anchors);
      this._preparer = preparer;
      cfg.prepared = preparer;
      if (!(await preparer.start())) {
        this._preparer = null;
        cfg.prepared = null;
      }
    }
  }
}
