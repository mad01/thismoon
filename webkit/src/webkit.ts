// webkit.ts — shared web-chrome bundle (globalName: Webkit)
// Exports: init (backward-compat shim), toFixation, bootSnippet

import { toFixation } from './fixation.js';
import { clampSize, SIZE_STEP, SIZE_DEFAULT } from './size.js';
import { WkReadAloud, stopReadAloud } from './read-aloud.js';
import { rankSites } from './fuzzy.js';
import type { Site, SiteMatch } from './fuzzy.js';
import { initProse } from './prose.js';

export { toFixation } from './fixation.js';
export { segmentSentences } from './sentences.js';
export { enhanceProse } from './prose.js';
export { escapeHtml, el, poll } from './render.js';
export type { ElChild, ElAttrs, PollHandle } from './render.js';

export type Control = 'cmdk' | 'fixation' | 'font' | 'size' | 'speed' | 'reload' | 'theme' | 'help';

export interface WebkitConfig {
  mount?: string;
  brand?: string;
  brandHref?: string;
  back?: { label: string; href: string };
  title?: string;
  nav?: { label: string; href: string; active?: boolean }[];
  extra?: {
    id: string;
    label?: string;
    title?: string;
    icon?: 'refresh' | 'add';
    variant?: 'ghost' | 'primary';
  }[];
  controls?: Control[];
  pageWidth?: number;
  fixationTargets?: string;
  /** @deprecated — listen to document 'wk-themechange' instead */
  onThemeChange?: (theme: 'light' | 'dark') => void;
}

// ── Storage keys (global, not per-page) ──
const THEME_KEY  = 'webkit-theme';
const FONT_KEY   = 'webkit-font';
const SIZE_KEY   = 'webkit-size';
const FIXATION_KEY = 'webkit-fixation';
const SPEED_KEY  = 'webkit-ra-speed';

const FONT_STACKS: Record<string, string> = {
  'Fira Code': "'Fira Code', 'SF Mono', 'Cascadia Code', monospace",
  'Inter':     "'Inter', 'Helvetica Neue', Arial, sans-serif",
  'Lexend':    "'Lexend', 'Helvetica Neue', Arial, sans-serif",
  'Work Sans': "'Work Sans', 'Helvetica Neue', Arial, sans-serif",
};

// ── Default fixation targets — component-aware ──
const DEFAULT_FIXATION_TARGETS =
  '[data-fixation], wk-panel-title, wk-panel-subtitle, wk-card, .callout, main p, main li, main td';

// ── SVG assets ──

const SVG_FIXATION = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 7V4h16v3"/><path d="M9 20h6"/><path d="M12 4v16"/></svg>`;
const SVG_SUN    = `<svg class="icon-sun" viewBox="0 0 24 24" fill="currentColor"><path d="M12 7c-2.76 0-5 2.24-5 5s2.24 5 5 5 5-2.24 5-5-2.24-5-5-5zM2 13h2c.55 0 1-.45 1-1s-.45-1-1-1H2c-.55 0-1 .45-1 1s.45 1 1 1zm18 0h2c.55 0 1-.45 1-1s-.45-1-1-1h-2c-.55 0-1 .45-1 1s.45 1 1 1zM11 2v2c0 .55.45 1 1 1s1-.45 1-1V2c0-.55-.45-1-1-1s-1 .45-1 1zm0 18v2c0 .55.45 1 1 1s1-.45 1-1v-2c0-.55-.45-1-1-1s-1 .45-1 1zM5.99 4.58a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0s.39-1.03 0-1.41L5.99 4.58zm12.37 12.37a.996.996 0 00-1.41 0 .996.996 0 000 1.41l1.06 1.06c.39.39 1.03.39 1.41 0a.996.996 0 000-1.41l-1.06-1.06zm1.06-10.96a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06zM7.05 18.36a.996.996 0 000-1.41.996.996 0 00-1.41 0l-1.06 1.06c-.39.39-.39 1.03 0 1.41s1.03.39 1.41 0l1.06-1.06z"/></svg>`;
const SVG_MOON   = `<svg class="icon-moon" viewBox="0 0 24 24" fill="currentColor"><path d="M12 3a9 9 0 109 9c0-.46-.04-.92-.1-1.36a5.389 5.389 0 01-4.4 2.26 5.403 5.403 0 01-3.14-9.8c-.44-.06-.9-.1-1.36-.1z"/></svg>`;
const SVG_RELOAD  = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/></svg>`;
const SVG_REFRESH = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/></svg>`;
const SVG_ADD     = `<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14"/><path d="M5 12h14"/></svg>`;
const SVG_SEARCH  = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></svg>`;
const SVG_HELP    = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M8.6 8.5a3.4 3.4 0 0 1 6.6 1.1c0 2.2-3.2 2.9-3.2 5"/><path d="M12 18.5h.01"/></svg>`;
const SVG_CLOSE   = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6L6 18"/><path d="M6 6l12 12"/></svg>`;

const SPEED_OPTIONS: { value: string; label: string }[] = [
  { value: '0.75', label: '0.75×' },
  { value: '1',    label: '1×' },
  { value: '1.25', label: '1.25×' },
  { value: '1.5',  label: '1.5×' },
  { value: '2',    label: '2×' },
];

// ── Utilities ──

function walkText(node: Node): void {
  if (node.nodeType === Node.TEXT_NODE) {
    const span = document.createElement('span');
    span.innerHTML = toFixation((node as Text).textContent ?? '');
    node.parentNode?.replaceChild(span, node);
  } else if (
    node.nodeType === Node.ELEMENT_NODE &&
    !['CODE', 'B', 'STRONG', 'SCRIPT', 'STYLE'].includes((node as Element).tagName)
  ) {
    Array.from(node.childNodes).forEach(walkText);
  }
}

function renderBrand(text: string, href: string): string {
  const last = text[text.length - 1];
  let inner: string;
  if (last === '.' || last === '·') {
    inner = escHtml(text.slice(0, -1)) + `<span class="dot">${escHtml(last)}</span>`;
  } else {
    inner = escHtml(text);
  }
  return `<a class="topbar-brand" href="${escAttr(href)}">${inner}</a>`;
}

function escHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escAttr(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
}

function iconSvg(icon: 'refresh' | 'add'): string {
  return icon === 'refresh' ? SVG_REFRESH : SVG_ADD;
}

function applyFont(name: string, selectEl: HTMLSelectElement): void {
  // Fall back the NAME, not just the stack: a persisted font that is no longer
  // in FONT_STACKS would otherwise leave the select blank and rewrite the
  // unknown name to localStorage forever.
  if (!(name in FONT_STACKS)) name = 'Fira Code';
  document.body.style.fontFamily = FONT_STACKS[name];
  selectEl.value = name;
  localStorage.setItem(FONT_KEY, name);
}

function applySize(px: number): number {
  const clamped = clampSize(px);
  document.documentElement.style.fontSize = clamped + 'px';
  localStorage.setItem(SIZE_KEY, String(clamped));
  return clamped;
}

// ── <wk-header> custom element ──

class WkHeader extends HTMLElement {
  private _initialized = false;

  connectedCallback(): void {
    // One-time guard: a detach+reattach (DOM move / htmx swap) must NOT
    // re-run _render() — it reads the now-consumed [data-nav]/[data-extra]
    // light-DOM children and would overwrite the built header with empties.
    if (this._initialized) return;
    this._render();
    this._applyTheme();
    this._wire();
    this._initialized = true;
  }

  private _attr(name: string, fallback = ''): string {
    return this.getAttribute(name) ?? fallback;
  }

  private _controls(): Control[] {
    // `speed` (read-aloud playback speed) is intentionally NOT in the default —
    // it only makes sense on pages with <wk-read-aloud> content (speak, present
    // briefings). Those consumers opt in via controls="…,speed,…".
    const raw = this._attr('controls', 'cmdk,font,fixation,size,reload,theme,help');
    return raw.split(',').map(s => s.trim()).filter(Boolean) as Control[];
  }

  /** Whether this header lists the ⌘K site picker; the keyboard shortcut follows it. */
  wantsCmdK(): boolean {
    return this._controls().includes('cmdk');
  }

  private _render(): void {
    const brand       = this._attr('brand');
    const brandHref   = this._attr('brand-href', '/');
    const backLabel   = this._attr('back-label');
    const backHref    = this._attr('back-href');
    const title       = this._attr('title');
    const pageWidth   = parseInt(this._attr('page-width', '1080'), 10) || 1080;

    // Set CSS custom property
    document.documentElement.style.setProperty('--page-width', pageWidth + 'px');

    // Collect light-DOM children that should be relocated
    const navChildren   = Array.from(this.querySelectorAll<HTMLElement>('[data-nav]'));
    const extraChildren = Array.from(this.querySelectorAll<HTMLElement>('[data-extra]'));

    // Build topbar-left
    const leftParts: string[] = [];

    if (backLabel) {
      leftParts.push(`<a class="topbar-link" href="${escAttr(backHref)}">${escHtml(backLabel)}</a>`);
    }
    if (brand) {
      leftParts.push(renderBrand(brand, brandHref));
    }
    if (title) {
      leftParts.push(`<span class="topbar-title">${escHtml(title)}</span>`);
    }

    // Build nav
    const navHtml = navChildren.map(a => a.outerHTML).join('');
    if (navHtml) {
      leftParts.push(`<nav class="topbar-nav">${navHtml}</nav>`);
    }

    // Build controls
    const controlParts: string[] = [];
    let needsDivider = false;

    function divider(): string {
      if (needsDivider) { needsDivider = false; return '<span class="ctrl-divider"></span>'; }
      return '';
    }

    for (const control of this._controls()) {
      switch (control) {
        case 'cmdk':
          controlParts.push(divider());
          controlParts.push(
            `<button class="ctrl-btn" id="webkit-cmdk" aria-label="Jump to site" title="Jump to site (⌘K)">` +
            SVG_SEARCH + ` <kbd class="ctrl-kbd" data-cmdk-kbd>⌘K</kbd></button>`
          );
          needsDivider = true;
          break;

        case 'font':
          controlParts.push(divider());
          controlParts.push(
            `<select class="ctrl-select" id="webkit-font" aria-label="Font family">` +
            Object.keys(FONT_STACKS).map(k => `<option value="${escAttr(k)}">${escHtml(k)}</option>`).join('') +
            `</select>`
          );
          needsDivider = true;
          break;

        case 'fixation':
          controlParts.push(divider());
          controlParts.push(
            `<button class="ctrl-btn" id="webkit-fixation" aria-label="Toggle fixation reading" aria-pressed="false">` +
            SVG_FIXATION + ` Fixation</button>`
          );
          needsDivider = true;
          break;

        case 'size':
          controlParts.push(divider());
          controlParts.push(
            `<div class="ctrl-group">` +
            `<button class="size-btn" id="webkit-size-down" aria-label="Decrease font size">&minus;</button>` +
            `<button class="size-btn" id="webkit-size-up" aria-label="Increase font size">+</button>` +
            `</div>`
          );
          needsDivider = true;
          break;

        case 'speed':
          controlParts.push(divider());
          controlParts.push(
            `<select class="ctrl-select" id="webkit-ra-speed" aria-label="Speech speed">` +
            SPEED_OPTIONS.map(o => `<option value="${escAttr(o.value)}">${escHtml(o.label)}</option>`).join('') +
            `</select>`
          );
          needsDivider = true;
          break;

        case 'reload':
          controlParts.push(divider());
          controlParts.push(
            `<button class="ctrl-btn" id="webkit-reload" aria-label="Reload page">` +
            SVG_RELOAD + ` Reload</button>`
          );
          needsDivider = true;
          break;

        case 'theme':
          controlParts.push(divider());
          controlParts.push(
            `<button class="theme-toggle" id="webkit-theme" aria-label="Switch to dark mode" aria-pressed="false">` +
            SVG_SUN + SVG_MOON + `</button>`
          );
          needsDivider = false;
          break;

        case 'help':
          // Always separate the guide from whatever precedes it (theme ends the
          // divider chain), so the ? reads as its own affordance at the end.
          controlParts.push('<span class="ctrl-divider"></span>');
          controlParts.push(
            `<button class="ctrl-btn" id="webkit-help" aria-label="Feature guide" aria-haspopup="dialog" title="Feature guide">` +
            SVG_HELP + `</button>`
          );
          needsDivider = false;
          break;
      }
    }

    // Extra buttons
    if (extraChildren.length > 0) {
      if (needsDivider) controlParts.push('<span class="ctrl-divider"></span>');
      for (const ex of extraChildren) {
        controlParts.push(ex.outerHTML);
      }
    }

    // Inject into host element (replace existing children)
    this.innerHTML =
      `<div class="topbar">` +
      `<div class="topbar-inner">` +
      `<div class="topbar-left">${leftParts.join('')}</div>` +
      `<div class="topbar-controls">${controlParts.join('')}</div>` +
      `</div>` +
      `</div>`;
  }

  private _applyTheme(): void {
    const saved = (localStorage.getItem(THEME_KEY) ?? 'light') as 'light' | 'dark';
    document.documentElement.setAttribute('data-theme', saved);
  }

  private _wire(): void {
    const fixationTargets = this.getAttribute('fixation-targets') ?? DEFAULT_FIXATION_TARGETS;

    // Font
    const fontEl = this.querySelector('#webkit-font') as HTMLSelectElement | null;
    if (fontEl) {
      const savedFont = localStorage.getItem(FONT_KEY) ?? 'Fira Code';
      applyFont(savedFont, fontEl);
      fontEl.addEventListener('change', () => applyFont(fontEl.value, fontEl));
    }

    // Size
    let currentSize = parseInt(localStorage.getItem(SIZE_KEY) ?? '', 10);
    if (isNaN(currentSize)) currentSize = SIZE_DEFAULT;
    applySize(currentSize);

    this.querySelector('#webkit-size-down')?.addEventListener('click', () => {
      currentSize = applySize(currentSize - SIZE_STEP);
    });
    this.querySelector('#webkit-size-up')?.addEventListener('click', () => {
      currentSize = applySize(currentSize + SIZE_STEP);
    });

    // Speech speed
    const speedEl = this.querySelector('#webkit-ra-speed') as HTMLSelectElement | null;
    if (speedEl) {
      const savedSpeed = localStorage.getItem(SPEED_KEY) ?? '1';
      speedEl.value = savedSpeed;
      speedEl.addEventListener('change', () => {
        localStorage.setItem(SPEED_KEY, speedEl.value);
        document.dispatchEvent(new CustomEvent('wk-speedchange', { detail: { speed: parseFloat(speedEl.value) } }));
      });
    }

    // Fixation — half-bold the first half of every word in the target elements.
    // Under client-side rendering the page content is injected (and re-rendered
    // on poll) *after* this runs, so a one-time walk of the targets present at
    // init misses everything. Instead, while fixation is active a MutationObserver
    // re-walks freshly rendered content, so it survives both the initial CSR
    // render and later poll/version re-renders with zero consumer changes. When
    // fixation is off the observer is disconnected, so the default case is free.
    const fixationBtn = this.querySelector('#webkit-fixation');
    // Per-element original markup, so toggling off restores the plain text. A
    // WeakMap (not the targets at init) keys on the live nodes and lets replaced
    // nodes be garbage-collected on re-render.
    const fixationOriginals = new WeakMap<HTMLElement, string>();
    let fixationActive = localStorage.getItem(FIXATION_KEY) === 'true';

    // walk one target if it hasn't been walked yet (idempotent via a dataset
    // flag, so a re-walk pass skips already-processed nodes and only touches
    // freshly rendered ones).
    const fixationify = (el: HTMLElement): void => {
      if (el.dataset.fixationDone === '1') return;
      fixationOriginals.set(el, el.innerHTML);
      el.classList.add('fixation');
      const frag = document.createElement('div');
      frag.innerHTML = el.innerHTML;
      walkText(frag);
      el.innerHTML = frag.innerHTML;
      el.dataset.fixationDone = '1';
    };

    const unbionify = (el: HTMLElement): void => {
      if (el.dataset.fixationDone !== '1') return;
      const orig = fixationOriginals.get(el);
      if (orig !== undefined) el.innerHTML = orig;
      el.classList.remove('fixation');
      delete el.dataset.fixationDone;
    };

    const applyFixation = (active: boolean): void => {
      fixationBtn?.classList.toggle('active', active);
      fixationBtn?.setAttribute('aria-pressed', String(active));
      document.querySelectorAll<HTMLElement>(fixationTargets).forEach(el => {
        if (active) fixationify(el); else unbionify(el);
      });
    };

    // Re-walk on DOM changes while active. Disconnect across our own writes so
    // the walk doesn't retrigger the observer (disconnect() also drops queued
    // records); a requestAnimationFrame debounce coalesces a render burst — and
    // read-aloud's per-sentence churn — into a single pass per frame.
    let fixationScheduled = false;
    const observe = (): void =>
      fixationObserver.observe(document.body, { childList: true, subtree: true });
    const fixationObserver = new MutationObserver(() => {
      if (fixationScheduled || !fixationActive) return;
      fixationScheduled = true;
      requestAnimationFrame(() => {
        fixationScheduled = false;
        if (!fixationActive) return;
        fixationObserver.disconnect();
        applyFixation(true);
        observe();
      });
    });

    applyFixation(fixationActive);
    if (fixationActive) observe();

    fixationBtn?.addEventListener('click', () => {
      // End any read-aloud session first: fixationify snapshots and rewrites
      // innerHTML, which would detach the session's highlight spans and strand
      // stale .wk-ra-sentence markup in the restored snapshot.
      stopReadAloud();
      fixationActive = !fixationActive;
      localStorage.setItem(FIXATION_KEY, String(fixationActive));
      fixationObserver.disconnect();
      applyFixation(fixationActive);
      if (fixationActive) observe();
    });

    // Reload
    this.querySelector('#webkit-reload')?.addEventListener('click', () => {
      location.reload();
    });

    // Feature guide — the ? button opens a modal explaining the controls this
    // header actually renders (plus a read-aloud section when speed is present
    // and any consumer <template data-wk-help> sections).
    const controls = this._controls();
    this.querySelector('#webkit-help')?.addEventListener('click', () => openHelp(controls));

    // ⌘K site picker — the button just asks the global controller to open.
    const cmdkBtn = this.querySelector('#webkit-cmdk');
    if (cmdkBtn) {
      const kbd = cmdkBtn.querySelector('[data-cmdk-kbd]');
      const isMac = /Mac|iP(hone|ad|od)/.test(navigator.platform || navigator.userAgent);
      if (kbd && !isMac) kbd.textContent = 'Ctrl K';
      cmdkBtn.addEventListener('click', () => {
        document.dispatchEvent(new CustomEvent('wk-cmdk:open'));
      });
    }

    // Theme
    const themeBtn = this.querySelector('#webkit-theme');
    const reflectTheme = (theme: 'light' | 'dark'): void => {
      themeBtn?.setAttribute('aria-pressed', String(theme === 'dark'));
      themeBtn?.setAttribute(
        'aria-label',
        theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode',
      );
    };
    // Reflect the initial (persisted) theme on the toggle.
    reflectTheme((document.documentElement.getAttribute('data-theme') as 'light' | 'dark') ?? 'light');

    themeBtn?.addEventListener('click', () => {
      const current = document.documentElement.getAttribute('data-theme');
      const next: 'light' | 'dark' = current === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', next);
      localStorage.setItem(THEME_KEY, next);
      reflectTheme(next);
      document.dispatchEvent(new CustomEvent('wk-themechange', { detail: { theme: next } }));
    });
  }
}

// ── Self-register all custom elements ──

customElements.define('wk-header', WkHeader);
customElements.define('wk-read-aloud', WkReadAloud);

// CSS-only components: registered as no-op elements purely for semantics +
// upgrade. Each tag needs its OWN constructor — customElements.define rejects a
// constructor that's already registered — so the loop mints a fresh class per tag.
const NOOP_ELEMENTS = [
  // base kit
  'wk-page-header', 'wk-title', 'wk-subtitle', 'wk-card', 'wk-panel',
  'wk-panel-title', 'wk-panel-subtitle', 'wk-badge', 'wk-button', 'wk-search', 'wk-table',
  // segmented control
  'wk-seg',
  // key/value rows
  'wk-kv', 'wk-kv-row', 'wk-kv-label', 'wk-kv-value',
  // section block
  'wk-section', 'wk-section-heading', 'wk-section-id', 'wk-section-subheading',
  // callout
  'wk-callout',
  // progress bar
  'wk-progress', 'wk-progress-bar', 'wk-progress-fill', 'wk-progress-label',
  // table of contents
  'wk-toc', 'wk-toc-title',
  // modal + form
  'wk-modal', 'wk-modal-panel', 'wk-modal-head', 'wk-modal-body', 'wk-modal-actions',
  'wk-form', 'wk-form-row', 'wk-form-hint',
  // toasts
  'wk-toast', 'wk-toast-host',
];
for (const tag of NOOP_ELEMENTS) {
  customElements.define(tag, class extends HTMLElement {});
}

// ── ⌘K site picker ──
// A global overlay (no markup needed) that fuzzy-jumps between the local .this
// sites. Opens on ⌘K / Ctrl+K or a 'wk-cmdk:open' event (the header button).
// The site list comes from d-man at /__this/sites.json (same-origin); a route
// added to routes.toml shows up here on every site, no rebuild.

const CMDK_SITES_PATH = '/__this/sites.json';
// Cross-origin fallback for pages not served through d-man (e.g. a service hit
// directly on localhost:<port> in dev). d-man sends Access-Control-Allow-Origin
// on this path, so the fetch is permitted.
const CMDK_SITES_FALLBACK = 'http://present.this' + CMDK_SITES_PATH;

let cmdkSites: Site[] | null = null;
let cmdkInFlight: Promise<Site[]> | null = null;
let cmdkOverlay: HTMLElement | null = null;
let cmdkInput: HTMLInputElement | null = null;
let cmdkListEl: HTMLElement | null = null;
let cmdkMatches: SiteMatch[] = [];
let cmdkActive = 0;
let cmdkPrevFocus: Element | null = null;

async function cmdkLoadSites(): Promise<Site[]> {
  if (cmdkSites) return cmdkSites;
  if (!cmdkInFlight) {
    cmdkInFlight = (async (): Promise<Site[]> => {
      for (const url of [CMDK_SITES_PATH, CMDK_SITES_FALLBACK]) {
        try {
          const res = await fetch(url, { credentials: 'omit' });
          if (!res.ok) continue;
          const data = await res.json();
          if (Array.isArray(data) && data.length) {
            cmdkSites = data as Site[];
            return cmdkSites;
          }
        } catch { /* try the next source */ }
      }
      cmdkInFlight = null; // nothing loaded — let the next open retry
      return [];
    })();
  }
  return cmdkInFlight;
}

function cmdkHostOf(url: string): string {
  try { return new URL(url).host; } catch { return url; }
}

function cmdkHighlight(name: string, indices: number[]): string {
  if (!indices.length) return escHtml(name);
  const hit = new Set(indices);
  let out = '';
  for (let i = 0; i < name.length; i++) {
    const ch = escHtml(name[i]);
    out += hit.has(i) ? `<mark>${ch}</mark>` : ch;
  }
  return out;
}

function cmdkIsOpen(): boolean {
  return !!cmdkOverlay && !cmdkOverlay.hasAttribute('hidden');
}

function cmdkBuild(): void {
  if (cmdkOverlay) return;
  const overlay = document.createElement('div');
  overlay.className = 'wk-cmdk-overlay';
  overlay.setAttribute('hidden', '');
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', 'Jump to site');
  overlay.innerHTML =
    `<div class="wk-cmdk-panel">` +
    `<input class="wk-cmdk-input" type="text" placeholder="Jump to site…" ` +
    `autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" aria-label="Jump to site">` +
    `<div class="wk-cmdk-list" role="listbox"></div>` +
    `</div>`;
  document.body.appendChild(overlay);

  cmdkOverlay = overlay;
  cmdkInput = overlay.querySelector('.wk-cmdk-input');
  cmdkListEl = overlay.querySelector('.wk-cmdk-list');

  overlay.addEventListener('mousedown', e => { if (e.target === overlay) cmdkClose(); });
  cmdkInput?.addEventListener('input', () => cmdkRender());
  cmdkInput?.addEventListener('keydown', cmdkOnKey);
}

function cmdkRender(): void {
  if (!cmdkListEl || !cmdkInput) return;
  cmdkMatches = rankSites(cmdkInput.value, cmdkSites ?? []);
  cmdkActive = 0;
  if (cmdkMatches.length === 0) {
    const msg = cmdkSites && cmdkSites.length ? 'No matching site' : 'No sites found';
    cmdkListEl.innerHTML = `<div class="wk-cmdk-empty">${msg}</div>`;
    return;
  }
  cmdkListEl.innerHTML = cmdkMatches.map((m, i) =>
    `<div class="wk-cmdk-item${i === 0 ? ' active' : ''}" role="option" data-i="${i}" aria-selected="${i === 0}">` +
    `<span class="wk-cmdk-name">${cmdkHighlight(m.site.name, m.indices)}</span>` +
    `<span class="wk-cmdk-host">${escHtml(m.site.host ?? cmdkHostOf(m.site.url))}</span>` +
    `</div>`,
  ).join('');
  cmdkListEl.querySelectorAll<HTMLElement>('.wk-cmdk-item').forEach(el => {
    el.addEventListener('click', () => { cmdkActive = parseInt(el.dataset.i ?? '0', 10); cmdkCommit(); });
    el.addEventListener('mousemove', () => {
      const i = parseInt(el.dataset.i ?? '0', 10);
      if (i !== cmdkActive) { cmdkActive = i; cmdkReflect(); }
    });
  });
}

function cmdkReflect(): void {
  cmdkListEl?.querySelectorAll<HTMLElement>('.wk-cmdk-item').forEach((el, i) => {
    const on = i === cmdkActive;
    el.classList.toggle('active', on);
    el.setAttribute('aria-selected', String(on));
    if (on) el.scrollIntoView({ block: 'nearest' });
  });
}

function cmdkMove(delta: number): void {
  if (cmdkMatches.length === 0) return;
  cmdkActive = (cmdkActive + delta + cmdkMatches.length) % cmdkMatches.length;
  cmdkReflect();
}

function cmdkCommit(): void {
  const m = cmdkMatches[cmdkActive];
  if (!m) return;
  cmdkClose();
  window.location.href = m.site.url;
}

function cmdkOnKey(e: KeyboardEvent): void {
  switch (e.key) {
    case 'ArrowDown': e.preventDefault(); cmdkMove(1); break;
    case 'ArrowUp':   e.preventDefault(); cmdkMove(-1); break;
    case 'Enter':     e.preventDefault(); cmdkCommit(); break;
    case 'Escape':    e.preventDefault(); cmdkClose(); break;
    case 'n': if (e.ctrlKey) { e.preventDefault(); cmdkMove(1); } break;  // vim-ish
    case 'p': if (e.ctrlKey) { e.preventDefault(); cmdkMove(-1); } break;
  }
}

async function cmdkOpen(): Promise<void> {
  cmdkBuild();
  if (!cmdkOverlay || !cmdkInput || !cmdkListEl || cmdkIsOpen()) return;
  cmdkPrevFocus = document.activeElement;
  cmdkOverlay.removeAttribute('hidden');
  cmdkInput.value = '';
  cmdkListEl.innerHTML = `<div class="wk-cmdk-empty">Loading…</div>`;
  cmdkInput.focus();
  await cmdkLoadSites();
  if (cmdkIsOpen()) cmdkRender();
}

function cmdkClose(): void {
  if (!cmdkOverlay) return;
  cmdkOverlay.setAttribute('hidden', '');
  if (cmdkPrevFocus instanceof HTMLElement) cmdkPrevFocus.focus();
  cmdkPrevFocus = null;
}

/** Open the ⌘K site picker programmatically. */
export function openCmdK(): void { void cmdkOpen(); }

// The shortcut follows the header: a page whose <wk-header> leaves the cmdk
// control out (a network-facing present instance has no local .this sites
// to jump to) gets no picker from the keyboard either, and the browser keeps
// its own ⌘K. A page with no header at all keeps the shortcut.
function cmdkWanted(): boolean {
  const headers = Array.from(document.querySelectorAll('wk-header'));
  return headers.length === 0 || headers.some(h => h instanceof WkHeader && h.wantsCmdK());
}

let cmdkInstalled = false;
function initCmdK(): void {
  if (cmdkInstalled) return;
  cmdkInstalled = true;
  document.addEventListener('keydown', e => {
    if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
      if (!cmdkWanted()) return;
      e.preventDefault();
      if (cmdkIsOpen()) cmdkClose(); else void cmdkOpen();
    }
  });
  document.addEventListener('wk-cmdk:open', () => { void cmdkOpen(); });
}
initCmdK();

// ── Feature guide (help modal) ──
// The ? header control opens a <wk-modal> explaining the controls THIS header
// renders (so a tool only documents what it shows), plus a read-aloud/speed
// section when the page has read-aloud, plus any consumer-provided
// <template data-wk-help> sections (e.g. speak's file upload). Content is
// rebuilt on each open. Dismiss on X, Escape, or a click outside the panel —
// the same affordances as the ⌘K overlay.

interface ControlHelp { term: string; desc: string; }

const CONTROL_HELP: Partial<Record<Control, ControlHelp>> = {
  cmdk:     { term: '⌘K / Ctrl-K', desc: 'Jump to any .this tool — fuzzy-search the site list and press Enter.' },
  font:     { term: 'Font',        desc: 'Switch typeface. Lexend and Work Sans are tuned for easier reading.' },
  size:     { term: '− / +',  desc: 'Shrink or enlarge the text. Your size is remembered across pages.' },
  fixation: { term: 'Fixation',    desc: 'Bolds the first half of every word so your eyes anchor on each one — a reading aid that helps many dyslexic readers move through text faster.' },
  speed:    { term: 'Speed',       desc: 'Playback speed for read-aloud — steps through 0.75× · 1× · 1.25× · 1.5× · 2×.' },
  reload:   { term: 'Reload',      desc: 'Reload the page.' },
  theme:    { term: 'Theme',       desc: 'Toggle light and dark.' },
};

// Row order in the guide, independent of the header's control order.
const HELP_ORDER: Control[] = ['cmdk', 'font', 'size', 'fixation', 'speed', 'reload', 'theme'];

let helpOverlay: HTMLElement | null = null;
let helpPrevFocus: Element | null = null;

function helpIsOpen(): boolean {
  return !!helpOverlay && !helpOverlay.hasAttribute('hidden');
}

function helpRowsHtml(controls: Control[]): string {
  const present = new Set(controls);
  const rows: string[] = [];
  for (const c of HELP_ORDER) {
    const h = present.has(c) ? CONTROL_HELP[c] : undefined;
    if (!h) continue;
    rows.push(
      `<div class="wk-help-row"><div class="wk-help-term">${escHtml(h.term)}</div>` +
      `<div class="wk-help-desc">${escHtml(h.desc)}</div></div>`,
    );
  }
  return rows.join('');
}

function helpReadAloudHtml(): string {
  return (
    `<div class="wk-help-section-title">Read aloud</div>` +
    `<div class="wk-help-row"><div class="wk-help-term">Play a section</div>` +
    `<div class="wk-help-desc">Each section gets a play button. It reads the text sentence by sentence and highlights the current one, so you can follow along or listen hands-free — a strong pairing with fixation for getting through long pages.</div></div>` +
    `<div class="wk-help-row"><div class="wk-help-term">Speed ladder</div>` +
    `<div class="wk-help-desc">The speed selector steps 0.75× · 1× · 1.25× · 1.5× · 2×. A change applies from the next sentence and is remembered across pages.</div></div>`
  );
}

// Consumer-provided sections: the innerHTML of every <template data-wk-help> in
// the document, appended verbatim. Consumers own that markup (trusted, from
// their own shell), so it is not escaped.
function helpExtraHtml(): string {
  let out = '';
  document.querySelectorAll<HTMLTemplateElement>('template[data-wk-help]').forEach(t => {
    out += t.innerHTML;
  });
  return out;
}

function helpBuild(): void {
  if (helpOverlay) return;
  const overlay = document.createElement('wk-modal');
  overlay.setAttribute('hidden', '');
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', 'Feature guide');
  overlay.innerHTML =
    `<wk-modal-panel>` +
    `<wk-modal-head><h3>Feature guide</h3>` +
    `<button class="wk-help-close" aria-label="Close guide">${SVG_CLOSE}</button></wk-modal-head>` +
    `<wk-modal-body></wk-modal-body>` +
    `</wk-modal-panel>`;
  document.body.appendChild(overlay);
  helpOverlay = overlay;

  overlay.addEventListener('mousedown', e => { if (e.target === overlay) closeHelp(); });
  overlay.querySelector('.wk-help-close')?.addEventListener('click', () => closeHelp());
}

function openHelp(controls: Control[]): void {
  helpBuild();
  if (!helpOverlay || helpIsOpen()) return;
  const body = helpOverlay.querySelector('wk-modal-body');
  if (body) {
    body.innerHTML =
      helpRowsHtml(controls) +
      (controls.includes('speed') ? helpReadAloudHtml() : '') +
      helpExtraHtml();
  }
  helpPrevFocus = document.activeElement;
  helpOverlay.removeAttribute('hidden');
  (helpOverlay.querySelector('.wk-help-close') as HTMLElement | null)?.focus();
}

function closeHelp(): void {
  if (!helpOverlay) return;
  helpOverlay.setAttribute('hidden', '');
  if (helpPrevFocus instanceof HTMLElement) helpPrevFocus.focus();
  helpPrevFocus = null;
}

/** Open the feature guide programmatically (documents the given controls). */
export function openFeatureGuide(controls: Control[] = ['cmdk', 'font', 'size', 'fixation', 'reload', 'theme']): void {
  openHelp(controls);
}

document.addEventListener('keydown', e => {
  if (e.key === 'Escape' && helpIsOpen()) { e.preventDefault(); closeHelp(); }
});

// ── Version-poll soft-reload ──
// The webkit assets are served no-cache with a content-hash ETag, but a browser
// sitting on an already-loaded page won't notice a redeploy on its own. Poll
// GET /webkit/version (the same content hash, served no-store), remember the
// first value seen, and reload once it changes. Mirrors present's per-page poll
// → compare → reload. Self-contained, unobtrusive, and error-tolerant: a failed
// fetch is ignored and retried on the next tick.

const VERSION_URL = '/webkit/version';
const VERSION_POLL_MS = 5000;
let seenVersion: string | null = null;

async function pollVersion(): Promise<void> {
  try {
    const res = await fetch(VERSION_URL, { cache: 'no-store' });
    if (!res.ok) return;
    const data = await res.json();
    const v = typeof data?.version === 'string' ? data.version : null;
    if (!v) return;
    if (seenVersion === null) {
      seenVersion = v;
    } else if (v !== seenVersion) {
      location.reload();
    }
  } catch {
    // network blip — leave seenVersion intact and try again next tick.
  }
}

function initVersionPoll(): void {
  void pollVersion();
  setInterval(() => void pollVersion(), VERSION_POLL_MS);
}
initVersionPoll();

// ── Rich markdown (.wk-prose) ──
// Lazy/guarded: highlights + decorates code blocks only when the page has prose.
initProse();

// ── FOUC guard ──
// Single canonical pre-paint snippet, sourced from src/boot.snippet.js so the
// JS export and the served dist/boot.js (GET /webkit/boot.js) cannot drift.
// Consumers inline this in <head> once, BEFORE the (deferred) webkit.js loads,
// so persisted theme + size don't flash. Prefer the served asset via
// webkit.BootScript() (Go) or <script src="/webkit/boot.js"></script>.
export { bootSnippet } from './boot.snippet.js';

// ── Backward-compat shim: Webkit.init(cfg) builds a <wk-header> ──

export function init(cfg?: WebkitConfig): void {
  const config: Required<WebkitConfig> = {
    mount:         cfg?.mount          ?? '#webkit-header',
    brand:         cfg?.brand          ?? '',
    brandHref:     cfg?.brandHref      ?? '/',
    back:          cfg?.back           ?? { label: '', href: '' },
    title:         cfg?.title          ?? '',
    nav:           cfg?.nav            ?? [],
    extra:         cfg?.extra          ?? [],
    controls:      cfg?.controls       ?? ['cmdk', 'font', 'fixation', 'size', 'reload', 'theme', 'help'],
    pageWidth:     cfg?.pageWidth      ?? 1080,
    fixationTargets: cfg?.fixationTargets  ?? DEFAULT_FIXATION_TARGETS,
    onThemeChange: cfg?.onThemeChange  ?? ((_t) => { /* no-op */ }),
  };

  const mountEl = document.querySelector(config.mount);
  if (!mountEl) return;

  // Build a <wk-header> element from config and replace the mount point
  const header = document.createElement('wk-header') as WkHeader;

  if (config.brand)    header.setAttribute('brand', config.brand);
  if (config.brandHref) header.setAttribute('brand-href', config.brandHref);
  if (config.back.label) header.setAttribute('back-label', config.back.label);
  if (config.back.href)  header.setAttribute('back-href', config.back.href);
  if (config.title)   header.setAttribute('title', config.title);
  header.setAttribute('controls', config.controls.join(','));
  header.setAttribute('page-width', String(config.pageWidth));
  if (config.fixationTargets) header.setAttribute('fixation-targets', config.fixationTargets);

  // Inject nav as light-DOM children
  for (const item of config.nav) {
    const a = document.createElement('a');
    a.setAttribute('data-nav', '');
    a.href = item.href;
    a.textContent = item.label;
    if (item.active) a.classList.add('active');
    header.appendChild(a);
  }

  // Inject extra buttons as <wk-button> for a single button surface (M2).
  // role/tabindex make the non-<button> element keyboard-operable.
  for (const ex of config.extra) {
    const btn = document.createElement('wk-button');
    btn.setAttribute('data-extra', '');
    btn.setAttribute('variant', ex.variant ?? 'ghost');
    btn.setAttribute('role', 'button');
    btn.setAttribute('tabindex', '0');
    btn.id = ex.id;
    if (ex.title) btn.title = ex.title;
    if (ex.icon) btn.innerHTML = iconSvg(ex.icon);
    if (ex.label) btn.append(document.createTextNode(ex.label));
    header.appendChild(btn);
  }

  // Bridge legacy onThemeChange callback via the new event
  if (cfg?.onThemeChange) {
    document.addEventListener('wk-themechange', ((e: CustomEvent<{ theme: 'light' | 'dark' }>) => {
      cfg.onThemeChange!(e.detail.theme);
    }) as EventListener);
  }

  mountEl.replaceWith(header);
}
