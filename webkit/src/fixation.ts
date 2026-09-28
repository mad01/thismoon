// fixation.ts — pure fixation-reading transform (no DOM), unit-testable from node.

/** A run of text as fixation renders it: bold (the first half of a word) or
 * plain (everything else, runs merged). */
export interface FixationSegment {
  text: string;
  bold: boolean;
}

// A word is a run of ASCII letters on word boundaries; a run touching a digit
// or an underscore ("v2", "snake_case") is left plain, so the bold never cuts
// an identifier in two.
const WORD = /\b([a-zA-Z]+)\b/g;

/**
 * Splits text into the runs fixation reading draws: the first half (rounded
 * up) of every word bold, the rest plain. Single-character words stay plain.
 * Adjacent plain runs are merged, so a text with nothing to bold comes back
 * as one plain segment (or none, for the empty string).
 */
export function fixationSegments(text: string): FixationSegment[] {
  const out: FixationSegment[] = [];
  const push = (t: string, bold: boolean): void => {
    if (!t) return;
    const last = out[out.length - 1];
    if (last && last.bold === bold) last.text += t;
    else out.push({ text: t, bold });
  };
  let pos = 0;
  for (const m of text.matchAll(WORD)) {
    const word = m[0];
    if (word.length <= 1) continue;
    const at = m.index ?? 0;
    const mid = Math.ceil(word.length / 2);
    push(text.slice(pos, at), false);
    push(word.slice(0, mid), true);
    pos = at + mid;
  }
  push(text.slice(pos), false);
  return out;
}

/**
 * Wraps the first half of every word in <b>…</b> for fixation reading.
 * Single-character words (length <= 1) are returned unchanged. The string
 * form of fixationSegments, kept on the Webkit global; the header's own
 * text-walk builds nodes from the segments instead.
 */
export function toFixation(text: string): string {
  return fixationSegments(text)
    .map(s => (s.bold ? '<b>' + s.text + '</b>' : s.text))
    .join('');
}
