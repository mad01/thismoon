// The themes page (GET /webkit/themes): a picker for the palette family per
// mode, plus the follow-system and reset controls. It runs beside webkit.js
// and goes through the Webkit global for every change, so the header toggle,
// the storage sync, and the wk-themechange event all stay in one code path.
// Cards are live samples: each carries its own data-palette and data-theme,
// and the generated palette blocks in webkit.css colour it.

import type * as WebkitModule from './webkit.js';

declare const Webkit: typeof WebkitModule;

type Theme = WebkitModule.Theme;

interface Variant { variant: string; notes?: string; roles: Record<string, string> }
interface Family { name: string; label: string; source: string; license: string; light?: Variant; dark?: Variant }
interface Collection { roles: string[]; families: Family[] }

const SWATCH_ROLES = ['bg', 'paper', 'text-1', 'primary', 'series-2', 'series-3'];

function card(f: Family, theme: Theme, v: Variant): HTMLElement {
  const el = Webkit.el;
  const swatches = el('span', { class: 'wk-theme-swatches' },
    SWATCH_ROLES.map(r => el('i', { title: r, style: `background:${v.roles[r]}` })));
  const code = el('code', {}, [
    el('span', { class: 'k' }, 'func'), ' f(', el('span', { class: 'n' }, '42'), ', ',
    el('span', { class: 's' }, '"ok"'), ')',
  ]);
  const sample = el('span', { class: 'wk-theme-sample' }, [
    el('wk-badge', { variant: 'a' }, 'badge'),
    el('wk-badge', { variant: 'ok' }, 'ok'),
    el('wk-badge', { variant: 'accent' }, 'accent'),
    code,
    el('wk-callout', { variant: 'info' }, `${v.variant}: a callout in this palette.`),
  ]);
  const source = el('a', { href: f.source, target: '_blank', rel: 'noopener' }, 'source');
  source.addEventListener('click', e => e.stopPropagation());
  const c = el('div', {
    class: 'wk-theme-card', role: 'button', tabindex: '0',
    'data-palette': f.name, 'data-theme': theme, 'data-family': f.name, 'aria-pressed': 'false',
    title: v.notes ?? '',
  }, [
    el('span', { class: 'wk-theme-card-head' }, [
      el('span', {}, [
        el('span', { class: 'wk-theme-card-name' }, f.label), ' ',
        el('span', { class: 'wk-theme-card-variant' }, v.variant),
      ]),
      el('span', { class: 'wk-theme-card-current', hidden: '' }, 'current'),
    ]),
    swatches,
    sample,
    el('span', { class: 'wk-theme-card-license' }, [`${f.license} · `, source]),
  ]);
  const pick = (): void => { Webkit.setPalette(theme, f.name); };
  c.addEventListener('click', pick);
  c.addEventListener('keydown', e => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(); }
  });
  return c;
}

function reflect(): void {
  const s = Webkit.themeState();
  for (const theme of ['light', 'dark'] as Theme[]) {
    const chosen = Webkit.paletteFor(theme);
    document.querySelectorAll<HTMLElement>(`#wk-theme-${theme} .wk-theme-card`).forEach(c => {
      const on = c.dataset.family === chosen;
      c.setAttribute('aria-pressed', String(on));
      const tag = c.querySelector('.wk-theme-card-current');
      if (tag) { if (on) tag.removeAttribute('hidden'); else tag.setAttribute('hidden', ''); }
    });
  }
  const system = document.getElementById('wk-theme-system') as HTMLInputElement | null;
  if (system) system.checked = s.mode === 'system';
  const status = document.getElementById('wk-theme-status');
  if (status) {
    const modeText = s.mode === 'system' ? `system (${s.theme} now)` : s.mode;
    status.textContent = `Mode: ${modeText} · light: ${Webkit.paletteFor('light')} · dark: ${Webkit.paletteFor('dark')}`;
  }
}

function render(col: Collection): void {
  for (const theme of ['light', 'dark'] as Theme[]) {
    const host = document.getElementById(`wk-theme-${theme}`);
    if (!host) continue;
    host.replaceChildren();
    const offered = col.families.filter(f => f[theme]);
    if (!offered.length) {
      host.appendChild(Webkit.el('p', { class: 'wk-theme-empty' }, `No family ships a ${theme} variant.`));
      continue;
    }
    for (const f of offered) host.appendChild(card(f, theme, f[theme]!));
  }
  reflect();
}

function wireControls(): void {
  const system = document.getElementById('wk-theme-system') as HTMLInputElement | null;
  system?.addEventListener('change', () => {
    // Unchecking pins the mode that is showing right now, so nothing flips.
    Webkit.setThemeMode(system.checked ? 'system' : Webkit.themeState().theme);
  });
  const reset = document.getElementById('wk-theme-reset');
  const doReset = (): void => { Webkit.resetTheme(); };
  reset?.addEventListener('click', doReset);
  reset?.addEventListener('keydown', e => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); doReset(); }
  });
  document.addEventListener('wk-themechange', reflect);
}

async function main(): Promise<void> {
  wireControls();
  const res = await fetch('/webkit/themes.json', { headers: { accept: 'application/json' } });
  if (!res.ok) throw new Error(`themes.json failed (${res.status})`);
  render((await res.json()) as Collection);
}

main().catch(err => {
  const msg = err instanceof Error ? err.message : String(err);
  for (const id of ['wk-theme-light', 'wk-theme-dark']) {
    const host = document.getElementById(id);
    if (host) host.replaceChildren(Webkit.el('p', { class: 'wk-theme-empty' }, `Could not load the theme collection: ${msg}`));
  }
});
