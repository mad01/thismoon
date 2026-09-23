// sentences.ts — pure sentence segmentation and part planning (no DOM),
// unit-testable from node.

export interface Sentence {
  /** Trimmed sentence text, suitable for sending to a TTS engine. */
  text: string;
  /** Start offset (inclusive) of the untrimmed segment in the source text. */
  start: number;
  /** End offset (exclusive) of the untrimmed segment in the source text. */
  end: number;
}

/**
 * Normalizes sentence text for the TTS engine: quote marks and backticks are
 * dropped (the engine renders them as awkward pauses or spells them out) and
 * whitespace is collapsed. Only the string sent to the engine is normalized —
 * the page text and highlight offsets are untouched.
 */
export function speakable(text: string): string {
  return text
    .replace(/[`"“”„«»]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

/**
 * Splits text into sentences with source offsets. Uses Intl.Segmenter when
 * available, falling back to a punctuation regex. Whitespace-only segments
 * are dropped. `base` shifts the reported offsets, so callers segmenting
 * one run of a larger document get offsets in document coordinates.
 */
export function segmentSentences(text: string, base = 0): Sentence[] {
  const out: Sentence[] = [];
  if (typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function') {
    const seg = new Intl.Segmenter(undefined, { granularity: 'sentence' });
    for (const s of seg.segment(text)) {
      const trimmed = s.segment.trim();
      if (trimmed) {
        out.push({ text: trimmed, start: base + s.index, end: base + s.index + s.segment.length });
      }
    }
    return out;
  }
  const re = /[^.!?]+[.!?]+\s*|[^.!?]+$/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    const trimmed = m[0].trim();
    if (trimmed) {
      out.push({ text: trimmed, start: base + m.index, end: base + m.index + m[0].length });
    }
  }
  return out;
}

// ── Parts: sentences grouped into one speech request each ──
// Mirrors services/speak/internal/chunk (Ramp, Live, Group, Join); change
// both together. A remote model answers slowly and whole-clip-at-once, so
// live playback sends one sentence first (the first sound comes after one
// short request) and then parts that grow while earlier ones play.

/** Longest part in characters (chunk.MaxChars): about 40 seconds of speech. */
export const MAX_PART_CHARS = 600;

/**
 * Bounds the size of each part in turn: part i holds at most ramp[i]
 * characters and the last bound repeats. A bound of 0 means exactly one
 * piece; a piece larger than its bound still makes a part of its own.
 */
export type Ramp = readonly number[];

/** The ramp for speech that plays while it is synthesized (chunk.Live). */
export const LIVE_RAMP: Ramp = [0, 250, MAX_PART_CHARS];

function bound(ramp: Ramp, i: number): number {
  if (!ramp.length) return MAX_PART_CHARS;
  return ramp[Math.min(i, ramp.length - 1)];
}

// Characters as Go counts them (runes), not UTF-16 code units.
function charCount(s: string): number {
  return Array.from(s).length;
}

/**
 * Packs pieces, in order, into parts sized by ramp, returning each part's
 * piece indices. A part always takes at least one piece.
 */
export function group(pieces: readonly string[], ramp: Ramp): number[][] {
  const parts: number[][] = [];
  let cur: number[] = [];
  let size = 0;
  pieces.forEach((piece, i) => {
    const n = charCount(piece);
    if (cur.length) {
      const limit = bound(ramp, parts.length);
      if (limit === 0 || size + 1 + n > limit) {
        parts.push(cur);
        cur = [];
        size = 0;
      } else {
        size++; // the joining space
      }
    }
    cur.push(i);
    size += n;
  });
  if (cur.length) parts.push(cur);
  return parts;
}

// Closing brackets and quotes that may follow a sentence's punctuation.
const CLOSERS = /[)\]}'’»]+$/u;
const TERMINAL = '.!?:;…';

/** Appends a period unless s already ends in terminal punctuation, looking
 * past closing brackets and quotes. */
function terminate(s: string): string {
  const core = s.replace(CLOSERS, '');
  if (!core) return s;
  const last = Array.from(core).pop() ?? '';
  return TERMINAL.includes(last) ? s : s + '.';
}

/**
 * Makes one part's text from its pieces, joined with single spaces. A piece
 * that does not end in terminal punctuation gets a period, so a heading or
 * list item is read as its own phrase instead of running into the next
 * sentence. Blank pieces are dropped.
 */
export function joinParts(pieces: readonly string[]): string {
  return pieces
    .map(p => p.trim())
    .filter(p => p)
    .map(terminate)
    .join(' ');
}

/** The keys in a space-separated data-ra-parts or data-ra-chunk list. */
export function splitKeys(list: string | null | undefined): string[] {
  return (list ?? '').split(/\s+/).filter(k => k);
}

/**
 * The order a cached section's parts play in. `planned` is the section's
 * data-ra-parts, speak's plan order with repeats kept (a section can say the
 * same thing twice). Without it, the parts follow the blocks' data-ra-chunk
 * lists in document order, each key once; that misorders a list item whose
 * paragraphs straddle a nested list, which is why speak sends the plan.
 */
export function partKeys(planned: string | null | undefined, blocks: readonly string[]): string[] {
  const plan = splitKeys(planned);
  if (plan.length) return plan;
  return [...new Set(blocks.flatMap(splitKeys))];
}
