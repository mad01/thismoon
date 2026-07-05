// prose.ts — client-side rich-markdown enhancement for `.wk-prose` blocks and
// standalone `pre.wk-code-block` elements.
//
// Two jobs, both opt-in via the presence of `.wk-prose` or `pre.wk-code-block`
// on the page (pages without either pay nothing beyond one querySelector):
//   1. Syntax-highlight fenced code via Prism (`pre > code[class*="language-"]`).
//   2. Inject a language badge + copy-to-clipboard button onto each code block.
//
// Prism is chosen over Shiki for a far smaller client bundle: Prism's core plus
// a handful of common grammars is a few KB, vs Shiki shipping full TextMate
// grammars + a theme. Only the languages imported here are bundled — unknown
// `language-*` classes simply render unhighlighted.

import Prism from 'prismjs';
// A focused grammar set for the mad01 tools (Go/TS/Python services + shell,
// config, and docs). Unlisted languages render unhighlighted rather than
// bloating the bundle with the full Prism grammar catalogue.
import 'prismjs/components/prism-bash.js';
import 'prismjs/components/prism-json.js';
import 'prismjs/components/prism-go.js';
import 'prismjs/components/prism-python.js';
import 'prismjs/components/prism-typescript.js';
import 'prismjs/components/prism-yaml.js';
import 'prismjs/components/prism-sql.js';

// Prism's default auto-highlight on DOMContentLoaded would fight our scoped,
// idempotent pass — disable it and drive highlighting ourselves.
Prism.manual = true;

const LANG_RE = /\blanguage-(\w+)/;

function langOf(code: Element): string {
  const m = code.className.match(LANG_RE);
  return m ? m[1] : '';
}

function enhanceBlock(pre: HTMLElement, code: HTMLElement): void {
  if (pre.dataset.wkProse === '1') return; // idempotent (htmx swaps, re-runs)
  pre.dataset.wkProse = '1';

  Prism.highlightElement(code);

  const lang = langOf(code);
  const controls = document.createElement('div');
  controls.className = 'wk-code-controls';

  const label = document.createElement('span');
  label.className = 'wk-code-lang';
  label.textContent = lang || 'text';
  controls.appendChild(label);

  const copy = document.createElement('button');
  copy.className = 'wk-code-copy';
  copy.type = 'button';
  copy.textContent = 'Copy';
  copy.setAttribute('aria-label', 'Copy code to clipboard');
  copy.addEventListener('click', () => {
    const text = code.textContent ?? '';
    const done = (): void => {
      copy.textContent = 'Copied';
      copy.classList.add('wk-copied');
      setTimeout(() => {
        copy.textContent = 'Copy';
        copy.classList.remove('wk-copied');
      }, 1500);
    };
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).then(done).catch(() => { /* ignore */ });
    } else {
      // Legacy fallback for non-secure contexts.
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); done(); } catch { /* ignore */ }
      ta.remove();
    }
  });
  controls.appendChild(copy);

  pre.classList.add('wk-has-controls');
  pre.appendChild(controls);
}

/**
 * Highlight and decorate every code block inside `.wk-prose`, plus every
 * standalone `pre.wk-code-block` used outside a prose body. Idempotent.
 */
export function enhanceProse(root: ParentNode = document): void {
  const blocks = root.querySelectorAll<HTMLElement>(
    '.wk-prose pre > code[class*="language-"], pre.wk-code-block > code[class*="language-"]',
  );
  blocks.forEach(code => {
    const pre = code.parentElement;
    if (pre instanceof HTMLElement) enhanceBlock(pre, code);
  });
}

/** Run once on DOM ready, but only if the page actually has prose. */
export function initProse(): void {
  const run = (): void => {
    if (!document.querySelector('.wk-prose, pre.wk-code-block')) return; // pay nothing otherwise
    enhanceProse(document);
  };
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run, { once: true });
  } else {
    run();
  }
}
