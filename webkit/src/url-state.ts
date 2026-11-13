// url-state.ts — the query string as a page's view state, shared by the CSR
// consumers. Folded from the hand-written readURL/syncURL pairs in deps,
// events, moondb and csl (MAD-548), none of which brought the view back on the
// back button. Re-exported from webkit.ts as the Webkit.urlState object.
//
// parse, serialize, splitList and nextHref are pure, so node tests cover them
// without a DOM. read, write and subscribe touch location and history, and
// look them up on each call, never at import.

/**
 * A value write puts in the query. '', null, undefined, false, a non-finite
 * number and an empty list are left out; true is written as 1 and a list
 * joins with commas.
 */
export type UrlValue = string | number | boolean | readonly string[] | null | undefined;
export type UrlValues = Record<string, UrlValue>;

/** A parsed query: the first value of each param, absent params undefined. */
export type UrlParams = Record<string, string | undefined>;

export interface UrlWriteOptions {
  /**
   * Add a history entry, so back returns to the view before this write. Use it
   * for a discrete change (a chip, a tab, a submitted search). Default false:
   * replace the current entry, which suits typing.
   */
  push?: boolean;
  /** A value equal to its default here is left out, so the default view keeps a bare URL. */
  defaults?: Record<string, string>;
}

/** The parts of a location nextHref reads. */
export interface UrlLocation {
  pathname: string;
  search: string;
  hash: string;
}

function own(obj: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(obj, key);
}

/** parse reads a query string, with or without its '?', into an object. */
export function parse(search: string): UrlParams {
  // No prototype, so a param named constructor or __proto__ is a plain key.
  const out: UrlParams = Object.create(null);
  new URLSearchParams(search).forEach((value, key) => {
    if (!own(out, key)) out[key] = value;
  });
  return out;
}

function valueText(v: UrlValue): string {
  if (v === null || v === undefined || v === false) return '';
  if (v === true) return '1';
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : '';
  if (typeof v === 'string') return v;
  return v.filter(Boolean).join(',');
}

/**
 * serialize builds a query string without its '?', params in the order
 * `values` names them, encoded as URLSearchParams encodes them.
 */
export function serialize(values: UrlValues, defaults: Record<string, string> = {}): string {
  const p = new URLSearchParams();
  Object.keys(values).forEach((key) => {
    const text = valueText(values[key]);
    if (text === '' || (own(defaults, key) && defaults[key] === text)) return;
    p.set(key, text);
  });
  return p.toString();
}

/** splitList splits a comma list param into its non-empty items; urlState.list. */
export function splitList(value: string | null | undefined): string[] {
  return (value || '').split(',').filter(Boolean);
}

// sortedQuery is a query string in a form two can be compared in: decoded,
// re-encoded and sorted by name, so an old link with its params in another
// order, or a comma left unencoded, is the same view.
function sortedQuery(search: string): string {
  const p = new URLSearchParams(search);
  p.sort();
  return p.toString();
}

/**
 * nextHref is the URL write moves to: the same path and hash with the query
 * `values` give, or null when that query is the one the location has.
 */
export function nextHref(values: UrlValues, loc: UrlLocation, defaults?: Record<string, string>): string | null {
  const query = serialize(values, defaults);
  if (sortedQuery(query) === sortedQuery(loc.search)) return null;
  return loc.pathname + (query ? '?' + query : '') + loc.hash;
}

// seen is the query the page last showed, so a popstate that only moved the
// hash (an in-page anchor) does not count as a new view.
let seen = '';
const listeners: Array<(params: UrlParams) => void> = [];

function onPopState(): void {
  const now = sortedQuery(location.search);
  if (now === seen) return;
  seen = now;
  const params = read();
  listeners.slice().forEach((fn) => fn(params));
}

/** read parses the current location's query. */
export function read(): UrlParams {
  return parse(location.search);
}

/**
 * write puts `values` in the address bar, replacing the whole query and
 * keeping the path and hash. It changes nothing and returns false when the
 * URL already holds that query, so a re-render that writes the same state adds
 * no history entry.
 */
export function write(values: UrlValues, opts: UrlWriteOptions = {}): boolean {
  const href = nextHref(values, location, opts.defaults);
  if (href === null) return false;
  if (opts.push) history.pushState(null, '', href);
  else history.replaceState(null, '', href);
  seen = sortedQuery(location.search);
  return true;
}

/**
 * subscribe calls fn with the parsed query when back or forward lands on an
 * entry of this page with another query, so the page can show that view
 * again. A step that only changes the hash is not one. Returns a function
 * that stops the calls.
 */
export function subscribe(fn: (params: UrlParams) => void): () => void {
  if (!listeners.length) {
    seen = sortedQuery(location.search);
    window.addEventListener('popstate', onPopState);
  }
  listeners.push(fn);
  return () => {
    const i = listeners.indexOf(fn);
    if (i < 0) return;
    listeners.splice(i, 1);
    if (!listeners.length) window.removeEventListener('popstate', onPopState);
  };
}

/** urlState is the global's Webkit.urlState: the functions above as one object. */
export const urlState = { parse, serialize, list: splitList, nextHref, read, write, subscribe };
