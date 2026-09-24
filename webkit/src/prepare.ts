// prepare.ts — pure helpers for <wk-read-aloud prepare> (no DOM), unit-testable
// from node: the registration request and its answer, what the page bar and
// the section badges say about a document's audio, and the download filename.
// Mirrors the JSON speak serve answers (services/speak/internal/web/docs.go:
// DocStatus, SectionStatus, PartCounts); change both together.

/** A set of parts tallied by audio state, as speak counts them. */
export interface PartCounts {
  parts: number;
  ready: number;
  generating: number;
  queued: number;
  /** Failed, another attempt scheduled. */
  retrying: number;
  idle: number;
  /** Out of attempts, or stopped by a shared failure. */
  failed: number;
}

/** One section's audio state. `section` is 1-based, in registration order. */
export interface SectionStatus extends PartCounts {
  section: number;
  /** The last failure with its attempt count, or empty. */
  reason: string;
}

/** A registered document's audio state: what GET /doc/{id}, POST
 * /doc/{id}/prepare and POST /read answer. */
export interface DocStatus {
  id: string;
  name: string;
  total: PartCounts;
  sections: SectionStatus[];
  reason: string;
}

/** POST /read's answer: the document plus, per registered section, its part
 * keys in play order and the keys that read each block. */
export interface Registration {
  name: string;
  doc: DocStatus;
  sections: { parts: string[]; blocks: string[][] }[];
}

/** The text a block is registered with: its runs of text (cut where a nested
 * block or a <br> interrupted them), each whitespace-collapsed, joined with
 * single spaces so "line one<br>line two" doesn't fuse; blank runs drop out.
 * Sentence normalization for the engine is speak's job. */
export function blockText(runs: readonly string[]): string {
  return runs.map(r => r.replace(/\s+/g, ' ').trim()).filter(r => r).join(' ');
}

/**
 * Gathers a section's blocks from a walk of its text nodes: `text(block, s)`
 * for each text node with the block it belongs to, `cut(block)` where a
 * nested block or a <br> interrupts a block's text. A block takes its place
 * in the reading order at its first non-blank text, so a list item whose
 * loose text follows its nested list reads after it, the way it sits on
 * screen; blank text before that places nothing. Blocks left with no text
 * are omitted.
 */
export class BlockCollector<B> {
  private readonly runs = new Map<B, string[]>();

  text(block: B, s: string): void {
    let runs = this.runs.get(block);
    if (!runs) {
      if (!s.trim()) return;
      runs = [''];
      this.runs.set(block, runs);
    }
    runs[runs.length - 1] += s;
  }

  cut(block: B): void {
    const runs = this.runs.get(block);
    if (runs && runs[runs.length - 1]) runs.push('');
  }

  blocks(): { el: B; text: string }[] {
    return Array.from(this.runs, ([el, runs]) => ({ el, text: blockText(runs) })).filter(b => b.text);
  }
}

/** The body of POST /read: the page's sections, each its blocks' text. */
export function readRequest(
  name: string,
  sections: readonly (readonly string[])[],
): { name: string; sections: { blocks: string[] }[] } {
  return { name, sections: sections.map(blocks => ({ blocks: [...blocks] })) };
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function isStringList(v: unknown): v is string[] {
  return Array.isArray(v) && v.every(s => typeof s === 'string');
}

function num(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

function counts(v: unknown): PartCounts {
  const r = isRecord(v) ? v : {};
  return {
    parts: num(r.parts),
    ready: num(r.ready),
    generating: num(r.generating),
    queued: num(r.queued),
    retrying: num(r.retrying),
    idle: num(r.idle),
    failed: num(r.failed),
  };
}

/** A DocStatus from a JSON body; null without an id. Missing counts read as
 * zero, so an older speak that lacks a state still renders. */
export function parseDocStatus(body: unknown): DocStatus | null {
  if (!isRecord(body) || typeof body.id !== 'string' || !body.id) return null;
  const sections = Array.isArray(body.sections) ? body.sections : [];
  return {
    id: body.id,
    name: str(body.name),
    total: counts(body.total),
    sections: sections.filter(isRecord).map(s => ({
      ...counts(s), section: num(s.section), reason: str(s.reason),
    })),
    reason: str(body.reason),
  };
}

/**
 * A Registration from POST /read's JSON body, checked against the request:
 * `blockCounts[i]` is how many blocks section i was registered with, and the
 * answer's arrays must mirror that, or the keys could land on the wrong
 * blocks. Null for anything else, so the caller stays in uncached mode.
 */
export function parseRegistration(body: unknown, blockCounts: readonly number[]): Registration | null {
  if (!isRecord(body)) return null;
  const doc = parseDocStatus(body.doc);
  if (!doc) return null;
  if (!Array.isArray(body.sections) || body.sections.length !== blockCounts.length) return null;
  const sections: Registration['sections'] = [];
  for (let i = 0; i < blockCounts.length; i++) {
    const s: unknown = body.sections[i];
    if (!isRecord(s) || !isStringList(s.parts)) return null;
    if (!Array.isArray(s.blocks) || s.blocks.length !== blockCounts[i] || !s.blocks.every(isStringList)) {
      return null;
    }
    sections.push({ parts: s.parts, blocks: s.blocks });
  }
  return { name: str(body.name) || doc.name, doc, sections };
}

/** Parts speak is still working on: while any remain, the status is polled. */
export function inProgress(c: PartCounts): number {
  return c.queued + c.generating + c.retrying;
}

/** The status of the index-th registered section (0-based); speak numbers
 * them from 1 in registration order. Undefined when speak lists none. */
export function statusOfSection(status: DocStatus, index: number): SectionStatus | undefined {
  return status.sections.find(s => s.section === index + 1);
}

/** What the page bar shows: the audio line, the failure reason behind it,
 * and which actions apply. */
export interface BarView {
  line: string;
  reason: string;
  /** Idle or failed parts remain for "Prepare all" to queue. */
  canPrepare: boolean;
  /** Failed parts for "Retry failed"; the button hides at zero. */
  failed: number;
  /** Every part is ready, so the page downloads as one file. */
  canDownload: boolean;
}

/** The page's audio line: "Audio: N of M parts ready", then what the rest
 * are doing, in the order a reader wants to know. */
export function pageLine(t: PartCounts): string {
  let line = `Audio: ${t.ready} of ${t.parts} parts ready`;
  const working = t.queued + t.generating;
  if (working) line += `, ${working} preparing`;
  if (t.retrying) line += `, ${t.retrying} retrying`;
  if (t.idle) line += `, ${t.idle} not prepared`;
  if (t.failed) line += `, ${t.failed} failed`;
  return line;
}

export function barView(status: DocStatus): BarView {
  const t = status.total;
  return {
    line: pageLine(t),
    reason: t.failed ? status.reason : '',
    canPrepare: t.idle + t.failed > 0,
    failed: t.failed,
    canDownload: t.parts > 0 && t.ready === t.parts,
  };
}

export type BadgeVariant = 'ok' | 'info' | 'warn' | 'error' | 'outline';

/** What a section's badge and buttons show. */
export interface SectionView {
  variant: BadgeVariant;
  text: string;
  /** The full story for the badge's title: the text, plus the last failure
   * behind an automatic retry. */
  title: string;
  /** Failed parts remain for "Retry". */
  retry: boolean;
  /** Every part is ready, so the section downloads as one file. */
  download: boolean;
}

/** A section's audio state, most useful fact first: work in progress, then
 * an automatic retry, then a failure, then what is left unprepared. */
export function sectionView(sec: SectionStatus): SectionView {
  const ready = `${sec.ready} of ${sec.parts} ready`;
  let variant: BadgeVariant;
  let text: string;
  if (sec.parts && sec.ready === sec.parts) {
    variant = 'ok'; text = 'audio ready';
  } else if (sec.generating) {
    variant = 'info'; text = `generating ${Math.min(sec.ready + 1, sec.parts)} of ${sec.parts}`;
  } else if (sec.queued) {
    variant = 'info'; text = `queued, ${ready}`;
  } else if (sec.retrying) {
    variant = 'warn'; text = `retrying, ${ready}`;
  } else if (sec.failed) {
    // speak's reason already opens with "failed after 3 attempts".
    const reason = sec.reason || 'unknown reason';
    variant = 'error'; text = /^failed\b/i.test(reason) ? reason : `failed: ${reason}`;
  } else if (sec.ready) {
    variant = 'outline'; text = ready;
  } else {
    variant = 'outline'; text = 'not prepared';
  }
  const title = sec.reason && variant !== 'error' && variant !== 'ok'
    ? `${text} (last failure: ${sec.reason})`
    : text;
  return {
    variant, text, title,
    retry: sec.failed > 0,
    download: sec.parts > 0 && sec.ready === sec.parts,
  };
}

/** The query for POST /doc/{id}/prepare: one section (1-based) or the whole
 * document (0), the failed parts only or every idle and failed one. */
export function prepareQuery(section: number, failedOnly: boolean): string {
  const q: string[] = [];
  if (section > 0) q.push(`section=${section}`);
  if (failedOnly) q.push('failed=1');
  return q.length ? '?' + q.join('&') : '';
}

/** The file extension for a clip's MIME type: speak joins to WAV unless the
 * provider gave MP3. */
export function audioExt(mime: string): string {
  const type = mime.split(';')[0].trim().toLowerCase();
  return type === 'audio/mpeg' || type === 'audio/mp3' ? '.mp3' : '.wav';
}

/**
 * The name a downloaded file gets: the document's name made filename-safe
 * the way speak's own Content-Disposition does (that header is not readable
 * cross-origin), with "-section-N" for one section (1-based; 0 is the whole
 * document) and the extension the blob's type calls for. A markdown upload's
 * extension comes off; a page title keeps its dots.
 */
export function downloadName(name: string, section: number, mime: string): string {
  let base = name.trim().replace(/\.(md|markdown|txt)$/i, '');
  base = Array.from(base).map(ch => (/[\p{L}\p{N}\-_.]/u.test(ch) ? ch : '-')).join('');
  base = base.replace(/^[-.]+|[-.]+$/g, '') || 'speak';
  if (section > 0) base += `-section-${section}`;
  return base + audioExt(mime);
}

/** How often prepared mode asks speak about the document while parts are in
 * progress; speak's own page polls at the same rate. */
export const POLL_MS = 2000;
/** The longest wait between polls while speak keeps not answering. */
export const POLL_MAX_MS = 60_000;

/** The wait before the next poll after `failures` consecutive status errors:
 * the normal cadence, doubling per failure up to the cap, so a tab left open
 * while speak is down asks once a minute, not every two seconds. */
export function pollDelay(failures: number): number {
  return Math.min(POLL_MAX_MS, POLL_MS * 2 ** Math.max(0, failures));
}
