// read-aloud.ts — <wk-read-aloud>: per-section text-to-speech playback against
// a local OpenAI-compatible speech endpoint (Kokoro via mlx-audio, fronted by
// d-man at http://speak.this). Injects a play/pause button into each `targets`
// match; stays fully inert when the endpoint is unreachable, so pages work
// unchanged on hosts without the speak service.

import { segmentSentences, speakable, Sentence } from './sentences.js';

const DEFAULT_ENDPOINT = 'http://speak.this';
const DEFAULT_VOICE = 'af_heart';
const TTS_MODEL = 'mlx-community/Kokoro-82M-bf16';
const PROBE_TIMEOUT_MS = 1500;

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
 * bionic toggle depends on — is preserved. */
function unwrapSentences(section: Element, spans: Map<number, HTMLElement[]>): void {
  for (const list of spans.values()) {
    for (const span of list) {
      span.replaceWith(document.createTextNode(span.textContent ?? ''));
    }
  }
  section.normalize();
}

// ── Playback controller (module-level: one session at a time) ──

interface Session {
  section: HTMLElement;
  btn: HTMLButtonElement;
  restartBtn: HTMLButtonElement | null; // shown while this section plays; null for selection sessions
  cfg: SpeakConfig;
  sentences: Sentence[];
  spans: Map<number, HTMLElement[]>;
  audio: HTMLAudioElement;
  pending: (Promise<string> | null)[]; // object URLs by sentence index
  queued: { url: string; idx: number } | null;
  userPaused: boolean;
  cancelled: boolean;
}

let session: Session | null = null;

/** Ends any active playback session. Exported for the bionic toggle: bionic
 * rewrites the target subtrees via innerHTML, which detaches the highlight
 * spans a live session holds — ending the session first keeps the DOM clean. */
export function stopReadAloud(): void {
  if (session) endSession(session);
}

async function fetchClip(cfg: SpeakConfig, text: string): Promise<string> {
  const res = await fetch(cfg.endpoint + '/v1/audio/speech', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    // wav: the engine encodes mp3 via ffmpeg, which may not be installed.
    body: JSON.stringify({ model: TTS_MODEL, input: speakable(text), voice: cfg.voice, speed: cfg.speed, response_format: 'wav' }),
  });
  if (!res.ok) throw new Error('speak endpoint returned ' + res.status);
  return URL.createObjectURL(await res.blob());
}

function ensureClip(s: Session, idx: number): Promise<string> {
  let p = s.pending[idx];
  if (!p) {
    p = fetchClip(s.cfg, s.sentences[idx].text);
    s.pending[idx] = p;
  }
  return p;
}

function setBtn(btn: HTMLButtonElement, state: 'idle' | 'playing' | 'paused'): void {
  btn.innerHTML = state === 'playing' ? SVG_PAUSE : SVG_PLAY;
  btn.classList.toggle('active', state !== 'idle');
  btn.setAttribute('aria-pressed', String(state === 'playing'));
  btn.setAttribute('aria-label', state === 'playing' ? 'Pause reading' : 'Read section aloud');
}

function highlight(s: Session, idx: number): void {
  const prev = s.spans.get(idx - 1);
  if (prev) prev.forEach(el => el.classList.remove('wk-ra-active'));
  const cur = s.spans.get(idx);
  if (!cur || !cur.length) return;
  cur.forEach(el => el.classList.add('wk-ra-active'));
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  cur[0].scrollIntoView({ behavior: reduced ? 'auto' : 'smooth', block: 'nearest' });
}

function endSession(s: Session): void {
  s.cancelled = true;
  s.audio.pause();
  s.audio.removeAttribute('src');
  for (const p of s.pending) {
    if (p) p.then(url => URL.revokeObjectURL(url)).catch(() => { /* fetch already failed */ });
  }
  unwrapSentences(s.section, s.spans);
  setBtn(s.btn, 'idle');
  if (s.restartBtn) s.restartBtn.hidden = true;
  if (session === s) session = null;
}

async function playFrom(s: Session, idx: number): Promise<void> {
  if (s.cancelled) return;
  if (idx >= s.sentences.length) { endSession(s); return; }
  // Nothing left after normalization (e.g. a bare quote mark) — skip ahead
  // rather than asking the engine to synthesize an empty string.
  if (!speakable(s.sentences[idx].text)) { return playFrom(s, idx + 1); }
  highlight(s, idx);
  let url: string;
  try {
    url = await ensureClip(s, idx);
  } catch (err) {
    console.warn('wk-read-aloud:', err);
    endSession(s);
    return;
  }
  if (s.cancelled) return;
  if (idx + 1 < s.sentences.length) {
    void ensureClip(s, idx + 1).catch(() => { /* surfaced when played */ });
  }
  if (s.userPaused) { s.queued = { url, idx }; return; }
  s.audio.src = url;
  s.audio.onended = (): void => { void playFrom(s, idx + 1); };
  try {
    await s.audio.play();
  } catch (err) {
    // Most likely NotAllowedError: no user activation (autoplay policy).
    console.warn('wk-read-aloud: play failed', err);
    endSession(s);
  }
}

function newSession(
  section: HTMLElement,
  btn: HTMLButtonElement,
  restartBtn: HTMLButtonElement | null,
  cfg: SpeakConfig,
  sentences: Sentence[],
  spans: Map<number, HTMLElement[]>,
): void {
  const s: Session = {
    section, btn, restartBtn, cfg, sentences, spans,
    audio: new Audio(),
    pending: new Array<Promise<string> | null>(sentences.length).fill(null),
    queued: null,
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
  const { sentences, nodes } = planSection(section);
  if (!sentences.length) return;
  newSession(section, btn, restartBtn, cfg, sentences, wrapSentences(nodes, sentences));
}

// Selection playback has no highlight spans — the user's own selection is the
// visual. The float button doubles as the section anchor so the pause/resume
// toggle in onButton works unchanged.
function startSelectionSession(text: string, btn: HTMLButtonElement, cfg: SpeakConfig): void {
  if (session) endSession(session);
  const sentences = segmentSentences(text);
  if (!sentences.length) return;
  newSession(btn, btn, null, cfg, sentences, new Map());
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
      if (s.queued) {
        const q = s.queued;
        s.queued = null;
        s.audio.src = q.url;
        s.audio.onended = (): void => { void playFrom(s, q.idx + 1); };
        void s.audio.play().catch(() => endSession(s));
      } else if (s.audio.src) {
        void s.audio.play().catch(() => endSession(s));
      }
      // else: paused and resumed before the first clip arrived — nothing is
      // queued and the audio element has no src, so play() would reject and
      // kill the session. With userPaused cleared, the in-flight playFrom
      // plays the clip itself when the fetch resolves.
    } else {
      s.userPaused = true;
      s.audio.pause();
      setBtn(btn, 'paused');
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
