// fixation.ts — pure fixation-reading transform (no DOM), unit-testable from node.

/**
 * Wraps the first half of every word in <b>…</b> for fixation reading.
 * Single-character words (length <= 1) are returned unchanged.
 */
export function toFixation(text: string): string {
  return text.replace(/\b([a-zA-Z]+)\b/g, (word) => {
    if (word.length <= 1) return word;
    const mid = Math.ceil(word.length / 2);
    return '<b>' + word.slice(0, mid) + '</b>' + word.slice(mid);
  });
}
