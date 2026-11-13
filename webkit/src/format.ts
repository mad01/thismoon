// format.ts — pure text formatters for times and durations, shared by the CSR
// consumers. Folded from the hand-written copies in csl, events, deps and
// moondb (MAD-595). Re-exported from webkit.ts, so they land on the global as
// Webkit.timeAgo / Webkit.formatDuration. No DOM access; the clock is read only
// when the caller leaves `now` out, so tests pass it and stay deterministic.

/** A moment: a Date, an ISO 8601 string, or epoch milliseconds. */
export type TimeValue = Date | string | number;

/**
 * The wording of timeAgo:
 * - `short` (default, csl and events): 42s ago, 5m ago, 3h ago, 2d ago
 * - `medium` (moondb): just now, 5 min ago, 3 h ago, 2 d ago
 * - `long` (deps): just now, 1 minute ago, 5 minutes ago, 3 hours ago, 2 days ago
 */
export type TimeAgoStyle = 'short' | 'medium' | 'long';

export interface TimeAgoOptions {
  /** The moment to measure from, in any TimeValue shape. Default: the current time. */
  now?: TimeValue;
  style?: TimeAgoStyle;
  /** The hour count at which the text switches to days. Default 24. */
  daysAfterHours?: number;
  /** Returned for a missing or invalid value. Default ''. */
  fallback?: string;
}

/**
 * The wording of formatDuration:
 * - `precise` (default, moondb's trace): 850ms, 4.2s, 3m12s, 1h02m
 * - `coarse` (moondb's lists): <1m, 12m, 1h 05m, 2d 3h
 */
export type DurationStyle = 'precise' | 'coarse';

export interface DurationOptions {
  style?: DurationStyle;
  /** Returned for a missing or non-numeric value. Default ''. */
  fallback?: string;
}

type Unit = 'minute' | 'hour' | 'day';

const SECOND = 1000;
const MINUTE_S = 60;
const HOUR_S = 3600;
const DAY_S = 86400;
const MINUTE_MS = 60 * SECOND;
const HOUR_MIN = 60;
const DAY_H = 24;

// UNIT_TEXT is the unit suffix each abbreviated style puts after the count.
const UNIT_TEXT: Record<'short' | 'medium', Record<Unit, string>> = {
  short: { minute: 'm', hour: 'h', day: 'd' },
  medium: { minute: ' min', hour: ' h', day: ' d' },
};

// toEpochMs reads a TimeValue as epoch milliseconds, NaN when it is missing or
// unparseable. A number is always milliseconds: no caller passes epoch seconds,
// so there is no guessing between the two. A Go zero time (year 1, what a Go
// time.Time with no value serializes to) counts as missing, as deps and moondb
// already treated it.
function toEpochMs(value: unknown): number {
  let ms = NaN;
  if (value instanceof Date) ms = value.getTime();
  else if (typeof value === 'number') ms = value;
  else if (typeof value === 'string' && value !== '') ms = Date.parse(value);
  // NaN, or a number outside the Date range, has no year and fails the test too.
  return new Date(ms).getUTCFullYear() > 1 ? ms : NaN;
}

function pad2(n: number): string {
  return n < 10 ? '0' + n : '' + n;
}

function agoPhrase(n: number, unit: Unit, style: TimeAgoStyle): string {
  if (style === 'long') return n + ' ' + unit + (n === 1 ? '' : 's') + ' ago';
  return n + UNIT_TEXT[style][unit] + ' ago';
}

/**
 * timeAgo says how long before `now` a moment was, rounded down to whole
 * seconds, minutes, hours or days; there are no weeks or dates. A moment after
 * `now` reads as no time at all. A missing or invalid value, or an invalid
 * `now`, returns `fallback`.
 *
 *   timeAgo('2026-11-13T10:30:00Z', { now: Date.parse('2026-11-13T10:35:00Z') }) // '5m ago'
 */
export function timeAgo(value: TimeValue | null | undefined, opts: TimeAgoOptions = {}): string {
  const fallback = opts.fallback ?? '';
  const style: TimeAgoStyle = opts.style === 'medium' || opts.style === 'long' ? opts.style : 'short';
  const hours = opts.daysAfterHours;
  const dayFrom = (typeof hours === 'number' && hours > 0 ? hours : DAY_H) * HOUR_S;
  const then = toEpochMs(value);
  const now = opts.now === undefined ? Date.now() : toEpochMs(opts.now);
  if (Number.isNaN(then) || Number.isNaN(now)) return fallback;

  const secs = Math.floor(Math.max(0, now - then) / SECOND);
  if (secs < MINUTE_S) return style === 'short' ? secs + 's ago' : 'just now';
  if (secs < HOUR_S) return agoPhrase(Math.floor(secs / MINUTE_S), 'minute', style);
  if (secs < dayFrom) return agoPhrase(Math.floor(secs / HOUR_S), 'hour', style);
  return agoPhrase(Math.floor(secs / DAY_S), 'day', style);
}

function preciseDuration(ms: number): string {
  if (ms < SECOND) return Math.round(ms) + 'ms';
  if (ms < MINUTE_MS) return (ms / SECOND).toFixed(1) + 's';
  const m = Math.floor(ms / MINUTE_MS);
  if (m < HOUR_MIN) return m + 'm' + pad2(Math.floor(ms / SECOND) % MINUTE_S) + 's';
  return Math.floor(m / HOUR_MIN) + 'h' + pad2(m % HOUR_MIN) + 'm';
}

function coarseDuration(ms: number): string {
  const m = Math.floor(ms / MINUTE_MS);
  if (m < 1) return '<1m';
  if (m < HOUR_MIN) return m + 'm';
  const h = Math.floor(m / HOUR_MIN);
  if (h < DAY_H) return h + 'h ' + pad2(m % HOUR_MIN) + 'm';
  return Math.floor(h / DAY_H) + 'd ' + (h % DAY_H) + 'h';
}

/**
 * formatDuration renders a span of milliseconds. A negative span counts as
 * zero; a value that is not a finite number returns `fallback`.
 *
 *   formatDuration(192000)                      // '3m12s'
 *   formatDuration(3900000, { style: 'coarse' }) // '1h 05m'
 */
export function formatDuration(ms: number | null | undefined, opts: DurationOptions = {}): string {
  if (typeof ms !== 'number' || !Number.isFinite(ms)) return opts.fallback ?? '';
  const span = Math.max(0, ms);
  return opts.style === 'coarse' ? coarseDuration(span) : preciseDuration(span);
}
