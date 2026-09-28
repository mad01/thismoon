// fixation-dom.ts — the fixation text-walk over a page. The half-bold markup
// goes in around the words, in place: each text node that holds a word
// becomes a <span class="wk-fixation-run"> with a <b> per word start, and
// toggling off unwraps those spans again. Nothing else under a target is
// touched, so what another component put there (read-aloud's buttons and
// badges, their listeners, data-ra-chunk stamps, highlight classes and
// sentence spans) survives the toggle in both directions. The header owns
// the toggle and the observer; read-aloud borrows the unwrap to plan a live
// session on plain text. The pure segmenter lives in fixation.ts.

import { fixationSegments } from './fixation.js';

export const FIXATION_RUN = 'wk-fixation-run';

// Subtrees the walk leaves alone: code and bold text are already set apart,
// form controls and badges hold labels rather than prose, and SVG has no
// HTML text.
const FIXATION_SKIP = new Set([
  'CODE', 'B', 'STRONG', 'SCRIPT', 'STYLE', 'SVG', 'BUTTON', 'SELECT', 'TEXTAREA', 'WK-BADGE',
]);

/** Replaces one text node with a run of its segments; a node with nothing
 * to bold (whitespace, punctuation, one-letter words) stays as it is. */
function fixateTextNode(node: Text): void {
  const segments = fixationSegments(node.data);
  if (!segments.some(s => s.bold)) return;
  const run = document.createElement('span');
  run.className = FIXATION_RUN;
  for (const s of segments) {
    if (s.bold) {
      const b = document.createElement('b');
      b.textContent = s.text;
      run.appendChild(b);
    } else {
      run.appendChild(document.createTextNode(s.text));
    }
  }
  node.replaceWith(run);
}

/** Half-bolds every word under node that isn't yet. Runs made earlier are
 * skipped, so a re-walk after new content arrived touches only the new
 * text nodes. */
export function wrapFixation(node: Node): void {
  if (node.nodeType === Node.TEXT_NODE) {
    fixateTextNode(node as Text);
    return;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return;
  const el = node as Element;
  if (FIXATION_SKIP.has(el.tagName.toUpperCase()) || el.classList.contains(FIXATION_RUN)) return;
  Array.from(el.childNodes).forEach(wrapFixation);
}

/** Undoes wrapFixation under root: every run's <b> and then the run itself
 * give way to their children, and the text nodes left side by side merge
 * back into one, so the next wrap sees whole words again. Every <b> inside a
 * run is fixation's own: the walk never enters bold text, and nothing else
 * puts bold inside a run. */
export function unwrapFixation(root: Element): void {
  root.querySelectorAll('.' + FIXATION_RUN).forEach(run => {
    run.querySelectorAll('b').forEach(b => b.replaceWith(...b.childNodes));
    run.replaceWith(...run.childNodes);
  });
  root.normalize();
}
