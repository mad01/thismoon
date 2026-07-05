// sentences.ts — pure sentence segmentation (no DOM), unit-testable from node.

export interface Sentence {
  /** Trimmed sentence text, suitable for sending to a TTS engine. */
  text: string;
  /** Start offset (inclusive) of the untrimmed segment in the source text. */
  start: number;
  /** End offset (exclusive) of the untrimmed segment in the source text. */
  end: number;
}

/**
 * Splits text into sentences with source offsets. Uses Intl.Segmenter when
 * available, falling back to a punctuation regex. Whitespace-only segments
 * are dropped. `base` shifts the reported offsets, so callers segmenting
 * one run of a larger document get offsets in document coordinates.
 */
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
