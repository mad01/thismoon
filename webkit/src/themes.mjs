// Theme collection: one JSON file per palette family under src/themes/, each
// with a light and a dark variant of about twenty authored roles. This module
// validates the files, derives every other role the stylesheet, present's
// graph, and its charts read, and emits two build artifacts:
//
//   - the palette CSS blocks build.mjs prepends to dist/webkit.css
//   - dist/themes.json, the resolved collection the themes page renders and
//     the Go package parses for Themes() and Roles()
//
// Every value is emitted as a literal (hex or rgba), never a var() or a
// color-mix(): Cytoscape and Chart.js parse colour strings themselves and
// getComputedStyle hands back a custom property's declared text, so a role
// the graph or chart code reads has to be a literal to be usable.
//
// Plain JS with no dependencies so node --test can import it directly.

import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

/** Roles a theme file must author per variant, in emit order. */
export const AUTHORED_ROLES = [
  'bg', 'paper', 'chip', 'border', 'border-hover',
  'text-1', 'text-2', 'text-3',
  'primary', 'on-primary',
  'red', 'green', 'amber', 'yellow', 'blue', 'purple',
];
export const SEMANTIC_ROLES = ['red', 'green', 'amber', 'yellow', 'blue', 'purple'];
export const SERIES_COUNT = 4;
/** Tones a graph node may name; neutral is derived from the surfaces, the rest from a semantic colour. */
export const TONES = ['neutral', 'green', 'red', 'blue', 'amber', 'purple'];

/**
 * Roles a page may reference by name (a panel accent, a chart series colour,
 * a slide tone). Exported through themes.json as `roles` so webkit.Roles() in
 * Go validates against the same list. Every name here is emitted for every
 * variant; the node test pins that.
 */
export const REFERENCE_ROLES = [
  'primary', ...SEMANTIC_ROLES,
  'series-1', 'series-2', 'series-3', 'series-4',
  'bg', 'paper', 'chip',
  ...TONES.map(t => `tone-${t}-bg`),
];

const HEX_RE = /^#[0-9a-fA-F]{6}$/;
const RGBA_RE = /^rgba\(\d{1,3},\d{1,3},\d{1,3},(0|1|0?\.\d+)\)$/;
const NAME_RE = /^[a-z][a-z0-9-]*$/;

// Two fixed shadow sets, one per mode, shared by every family.
const SHADOWS = {
  light: {
    'shadow-xs': '0 1px 2px rgba(37,35,32,0.05)',
    'shadow-sm': '0 2px 6px rgba(37,35,32,0.07), 0 1px 2px rgba(37,35,32,0.04)',
    'shadow-md': '0 4px 16px rgba(37,35,32,0.09), 0 2px 4px rgba(37,35,32,0.05)',
    'shadow-lg': '0 12px 40px rgba(37,35,32,0.14), 0 4px 8px rgba(37,35,32,0.06)',
  },
  dark: {
    'shadow-xs': '0 1px 2px rgba(0,0,0,0.15)',
    'shadow-sm': '0 2px 6px rgba(0,0,0,0.2), 0 1px 2px rgba(0,0,0,0.12)',
    'shadow-md': '0 4px 16px rgba(0,0,0,0.3), 0 2px 4px rgba(0,0,0,0.18)',
    'shadow-lg': '0 12px 40px rgba(0,0,0,0.4), 0 4px 8px rgba(0,0,0,0.25)',
  },
};

// ── Colour maths (sRGB, 8-bit channels, no gamma; good enough for tints) ──

export function parseHex(hex) {
  if (!HEX_RE.test(hex)) throw new Error(`not a 6-digit hex colour: ${hex}`);
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export function toHex([r, g, b]) {
  return '#' + [r, g, b].map(c => Math.round(Math.max(0, Math.min(255, c))).toString(16).padStart(2, '0')).join('');
}

/** mix(a, b, t): a moved t of the way toward b; t=0 is a, t=1 is b. */
export function mix(a, b, t) {
  const ca = parseHex(a), cb = parseHex(b);
  return toHex(ca.map((v, i) => v + (cb[i] - v) * t));
}

/** alpha(hex, a): the colour as an rgba() string with the given opacity. */
export function alpha(hex, a) {
  const [r, g, b] = parseHex(hex);
  return `rgba(${r},${g},${b},${a})`;
}

/** Relative luminance per WCAG, 0 (black) to 1 (white). */
export function luminance(hex) {
  const lin = c => { const s = c / 255; return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4; };
  const [r, g, b] = parseHex(hex).map(lin);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG 2.x contrast ratio between two colours, 1 (the same colour) to 21 (black on white). */
export function contrast(a, b) {
  const la = luminance(a), lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

/**
 * Ink for text on a fill: white or the variant's deepest ink, whichever reads
 * with more contrast on it. The deepest ink is text-1 in light and bg in dark;
 * in dark, text-1 is pale and never beats white. A luminance threshold used to
 * pick white for every fill under 0.4, which left the dark semantic fills
 * between 2.4 and 4.3 (MAD-376). The Go contrast test holds every dark ink to
 * 4.5 on its fill, and a family pins on-<colour> under overrides where the
 * rule must not decide.
 */
function onColour(fill, variant, mode) {
  const deep = mode === 'light' ? variant['text-1'] : variant.bg;
  return contrast('#FFFFFF', fill) >= contrast(deep, fill) ? '#FFFFFF' : deep;
}

// ── Derivation ──

/**
 * Resolves one variant to the full ordered role map: the authored roles, the
 * four series, then every derived role, with the file's overrides applied
 * last. The rules here are the only place the old hand-tuned relationships
 * live; a theme author touches none of them.
 */
export function deriveVariant(v, mode) {
  const r = {};
  for (const role of AUTHORED_ROLES) r[role] = v[role];
  v.series.forEach((c, i) => { r[`series-${i + 1}`] = c; });

  const light = mode === 'light';
  r['card-bg'] = v.paper;
  r['chip-bg'] = v.chip;
  r['chip-active-bg'] = v.primary;
  r['chip-active-text'] = v['on-primary'];
  r['progress-bg'] = v.chip;
  r['progress-fill'] = v.primary;
  r['primary-soft'] = mix(v.primary, v.paper, 0.2);
  r['text-body'] = mix(v['text-1'], v['text-2'], 0.5);
  r['ra-highlight'] = mix(v.paper, v.primary, 0.25);
  r['tag-a'] = mix(v.paper, v.amber, 0.2);
  r['tag-a-text'] = mix(v.amber, v['text-1'], 0.5);
  r['tag-b'] = mix(v.paper, v.green, 0.2);
  r['tag-b-text'] = mix(v.green, v['text-1'], 0.5);
  r['tag-c'] = mix(v.paper, v.blue, 0.2);
  r['tag-c-text'] = mix(v.blue, v['text-1'], 0.5);
  for (const c of SEMANTIC_ROLES) r[`on-${c}`] = onColour(v[c], v, mode);
  r['focus-ring'] = alpha(v.primary, 0.15);
  r['topbar-bg'] = alpha(v.bg, 0.88);
  r['scrim'] = alpha(light ? v['text-1'] : v.paper, 0.45);

  r['code-bg'] = light ? v.paper : v.bg;
  r['code-header-bg'] = v.chip;
  r['code-border'] = v.border;
  r['code-keyword'] = v.primary;
  r['code-string'] = v.green;
  r['code-number'] = v.amber;
  r['code-symbol'] = v.blue;

  r['graph-bg'] = v.paper;
  r['graph-leaf-bg'] = v.paper;
  r['graph-leaf-border'] = v.border;
  r['graph-leaf-text'] = v['text-1'];
  r['graph-center-bg'] = mix(v.paper, v.primary, 0.12);
  r['graph-center-border'] = v.primary;
  r['graph-center-text'] = v['text-1'];
  r['graph-registry-bg'] = v.chip;
  r['graph-registry-border'] = v.border;
  r['graph-registry-text'] = v['text-2'];
  r['graph-edge'] = v.border;
  r['graph-edge-arrow'] = v['border-hover'];
  r['graph-hot'] = v.primary;
  [v.primary, v.blue, v.green, v.purple].forEach((c, i) => { r[`graph-module-${i + 1}`] = c; });

  r['tone-neutral-bg'] = v.chip;
  r['tone-neutral-border'] = v['border-hover'];
  r['tone-neutral-text'] = v['text-2'];
  for (const t of TONES) {
    if (t === 'neutral') continue;
    r[`tone-${t}-bg`] = mix(v.paper, v[t], 0.14);
    r[`tone-${t}-border`] = v[t];
    r[`tone-${t}-text`] = mix(v[t], v['text-1'], 0.6);
  }

  r['chart-grid'] = v.border;
  r['chart-text'] = v['text-2'];
  r['chart-label'] = v['text-1'];

  Object.assign(r, SHADOWS[mode]);

  for (const [role, value] of Object.entries(v.overrides ?? {})) r[role] = value;
  return r;
}

/** Roles that must resolve to a plain hex because canvas code reads them. */
export function mustBeHex(role) {
  return /^(graph-|tone-|chart-|series-)/.test(role) || ['bg', 'paper', 'chip', 'card-bg', 'primary'].includes(role);
}

// ── Validation ──

const AUTHORED_KEYS = new Set(['variant', 'notes', 'series', 'overrides', ...AUTHORED_ROLES]);

function fail(file, msg) {
  throw new Error(`theme ${file}: ${msg}`);
}

function validateVariant(file, mode, v) {
  if (typeof v !== 'object' || v === null) fail(file, `${mode} must be an object`);
  if (typeof v.variant !== 'string' || !v.variant) fail(file, `${mode}.variant must name the variant`);
  for (const role of AUTHORED_ROLES) {
    if (!HEX_RE.test(v[role] ?? '')) fail(file, `${mode}.${role} must be a 6-digit hex colour, got ${JSON.stringify(v[role])}`);
  }
  if (!Array.isArray(v.series) || v.series.length !== SERIES_COUNT || !v.series.every(c => HEX_RE.test(c))) {
    fail(file, `${mode}.series must be ${SERIES_COUNT} hex colours`);
  }
  for (const k of Object.keys(v)) if (!AUTHORED_KEYS.has(k)) fail(file, `${mode}.${k} is not an authored role (derived roles go under overrides)`);
  if (v.overrides !== undefined) {
    if (typeof v.overrides !== 'object' || v.overrides === null) fail(file, `${mode}.overrides must be an object`);
    const derived = deriveVariant({ ...v, overrides: {} }, mode);
    for (const [role, value] of Object.entries(v.overrides)) {
      if (!(role in derived) || AUTHORED_ROLES.includes(role)) fail(file, `${mode}.overrides.${role} is not a derived role`);
      if (!HEX_RE.test(value) && !RGBA_RE.test(value)) fail(file, `${mode}.overrides.${role} must be hex or rgba(), got ${JSON.stringify(value)}`);
    }
  }
}

export function validateFamily(file, f) {
  if (typeof f !== 'object' || f === null) fail(file, 'must be an object');
  if (!NAME_RE.test(f.name ?? '')) fail(file, `name must match ${NAME_RE}`);
  for (const k of ['label', 'source', 'license']) {
    if (typeof f[k] !== 'string' || !f[k]) fail(file, `${k} must be a non-empty string`);
  }
  if (!f.light && !f.dark) fail(file, 'needs a light or a dark variant');
  if (f.light) validateVariant(file, 'light', f.light);
  if (f.dark) validateVariant(file, 'dark', f.dark);
  return f;
}

// ── Resolution and emit ──

/** Resolves a validated family: each variant becomes {variant, notes, roles}. */
export function resolveFamily(f) {
  const out = { name: f.name, label: f.label, source: f.source, license: f.license };
  for (const mode of ['light', 'dark']) {
    if (!f[mode]) continue;
    const roles = deriveVariant(f[mode], mode);
    for (const role of REFERENCE_ROLES) {
      if (!(role in roles)) throw new Error(`theme ${f.name}: ${mode} does not resolve reference role ${role}`);
    }
    for (const [role, value] of Object.entries(roles)) {
      if (mustBeHex(role) && !HEX_RE.test(value)) {
        throw new Error(`theme ${f.name}: ${mode}.${role} must be a literal hex (graph and chart code read it), got ${value}`);
      }
    }
    out[mode] = { variant: f[mode].variant, roles };
    if (f[mode].notes) out[mode].notes = f[mode].notes;
  }
  return out;
}

function block(selector, roles) {
  const lines = Object.entries(roles).map(([k, v]) => `  --${k}: ${v};`);
  return `${selector} {\n${lines.join('\n')}\n}`;
}

/**
 * The palette stylesheet. The default family's light block is also :root and
 * its dark block is also the bare [data-theme="dark"], so a page with no
 * palette attribute, or one naming a family that no longer ships, renders the
 * default. No selector is :root-scoped, so a subtree can carry its own palette
 * (the themes page renders every card inside its own data-palette element).
 */
export function renderCSS(families) {
  const parts = ['/* generated by build.mjs from src/themes/*.json; do not edit */'];
  for (const f of families) {
    const isDefault = f.name === 'default';
    if (f.light) {
      const sel = `[data-palette="${f.name}"][data-theme="light"]`;
      parts.push(block(isDefault ? `:root, ${sel}` : sel, f.light.roles));
    }
    if (f.dark) {
      const sel = `[data-palette="${f.name}"][data-theme="dark"]`;
      parts.push(block(isDefault ? `[data-theme="dark"], ${sel}` : sel, f.dark.roles));
    }
  }
  return parts.join('\n') + '\n';
}

export function renderJSON(families) {
  return JSON.stringify({ roles: REFERENCE_ROLES, families }, null, 1) + '\n';
}

/** Loads, validates, and resolves every family file; default first, then by name. */
export function loadFamilies(dir) {
  const files = readdirSync(dir).filter(f => f.endsWith('.json')).sort();
  const families = files.map(file => {
    let parsed;
    try { parsed = JSON.parse(readFileSync(join(dir, file), 'utf8')); } catch (e) { fail(file, `invalid JSON: ${e.message}`); }
    const f = validateFamily(file, parsed);
    if (f.name !== file.replace(/\.json$/, '')) fail(file, `name ${f.name} must match the file name`);
    return resolveFamily(f);
  });
  const def = families.find(f => f.name === 'default');
  if (!def || !def.light || !def.dark) throw new Error('theme default.json must exist and ship both variants');
  return [def, ...families.filter(f => f !== def)];
}

/** Everything build.mjs needs: the palette CSS and the resolved collection JSON. */
export function buildThemes(dir) {
  const families = loadFamilies(dir);
  return { families, css: renderCSS(families), json: renderJSON(families) };
}
