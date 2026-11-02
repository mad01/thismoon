// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * Hunks in the shape moondb reads as a payload's structuredPatch: the
 * Claude Code Edit and Write tool_response field (internal/patch in moondb).
 * Lines start with ' ', '-' or '+', and a line starting with '\' marks the
 * line before it as the last of its side, with no newline after it.
 *
 * moondb's why-by-content reads an update's added lines from these hunks,
 * so an edit without them is never named by `moondb why`.
 */

export interface Hunk {
	oldStart: number;
	oldLines: number;
	newStart: number;
	newLines: number;
	lines: string[];
}

const HUNK_HEADER = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;
const NO_NEWLINE = "\\ No newline at end of file";

// Context lines around each change, as in diff -u.
const CONTEXT = 3;

// The edit distance past which diffHunks stops looking for the shortest
// diff and reports the changed middle as one block. It bounds the work and
// memory of one Write to a few milliseconds and a few megabytes.
const MAX_EDIT_DISTANCE = 1_000;

/**
 * parseUnifiedPatch reads the hunks of a one-file unified diff, the text
 * pi's edit tool returns as details.patch. File header lines before the
 * first hunk are skipped. It returns undefined when the text holds no hunk
 * or a hunk whose lines do not add up to its header, which moondb would
 * reject.
 */
export function parseUnifiedPatch(text: unknown): Hunk[] | undefined {
	if (typeof text !== "string") return undefined;
	const hunks: Hunk[] = [];
	let current: Hunk | undefined;
	for (const line of text.split("\n")) {
		const header = HUNK_HEADER.exec(line);
		if (header) {
			current = {
				oldStart: Number(header[1]),
				oldLines: header[2] === undefined ? 1 : Number(header[2]),
				newStart: Number(header[3]),
				newLines: header[4] === undefined ? 1 : Number(header[4]),
				lines: [],
			};
			hunks.push(current);
			continue;
		}
		if (!current || line === "") continue;
		if (!" -+\\".includes(line[0])) return undefined;
		current.lines.push(line);
	}
	if (hunks.length === 0 || !hunks.every(countsAgree)) return undefined;
	return hunks;
}

function countsAgree(h: Hunk): boolean {
	let oldN = 0;
	let newN = 0;
	for (const line of h.lines) {
		if (line[0] === " " || line[0] === "-") oldN++;
		if (line[0] === " " || line[0] === "+") newN++;
	}
	return oldN === h.oldLines && newN === h.newLines;
}

/**
 * diffHunks is the line diff from before to after as hunks with three lines
 * of context, empty when the two are equal.
 */
export function diffHunks(before: string, after: string): Hunk[] {
	const a = splitLines(before);
	const b = splitLines(after);
	return toHunks(a, b, diffSteps(a, b));
}

// splitLines cuts text into lines that keep their "\n", so a last line
// without one differs from the same text with one.
function splitLines(text: string): string[] {
	if (text === "") return [];
	const parts = text.split("\n");
	const lines = parts.map((part, i) => (i < parts.length - 1 ? `${part}\n` : part));
	if (lines.at(-1) === "") lines.pop();
	return lines;
}

const EQUAL = 0;
const DELETE = 1;
const INSERT = 2;

// One step of an edit script. a and b are the old and new line positions
// the step sits at: the line it keeps or deletes in a, the line it keeps or
// inserts in b, and for the other side the count of lines before it.
interface Step {
	op: number;
	a: number;
	b: number;
}

// diffSteps trims the common head and tail, then runs Myers' diff over the
// middle, or marks the whole middle changed when that is too far apart.
function diffSteps(a: string[], b: string[]): Step[] {
	let head = 0;
	while (head < a.length && head < b.length && a[head] === b[head]) head++;
	let tail = 0;
	while (
		tail < a.length - head &&
		tail < b.length - head &&
		a[a.length - 1 - tail] === b[b.length - 1 - tail]
	) {
		tail++;
	}
	const midA = a.slice(head, a.length - tail);
	const midB = b.slice(head, b.length - tail);
	const middle = myers(midA, midB) ?? replaceAll(midA, midB);
	const steps: Step[] = [];
	for (let i = 0; i < head; i++) steps.push({ op: EQUAL, a: i, b: i });
	for (const s of middle) steps.push({ op: s.op, a: s.a + head, b: s.b + head });
	for (let i = 0; i < tail; i++) {
		steps.push({ op: EQUAL, a: a.length - tail + i, b: b.length - tail + i });
	}
	return steps;
}

function replaceAll(a: string[], b: string[]): Step[] {
	const steps: Step[] = [];
	for (let i = 0; i < a.length; i++) steps.push({ op: DELETE, a: i, b: 0 });
	for (let j = 0; j < b.length; j++) steps.push({ op: INSERT, a: a.length, b: j });
	return steps;
}

// myers is the shortest edit script from a to b (Myers 1986), or null when
// it needs more than MAX_EDIT_DISTANCE steps. Each round keeps only the
// diagonals it can reach, so the trace grows with the distance squared.
function myers(a: string[], b: string[]): Step[] | null {
	const n = a.length;
	const m = b.length;
	const max = Math.min(n + m, MAX_EDIT_DISTANCE);
	const off = max + 1;
	const v = new Int32Array(2 * max + 3);
	const trace: Int32Array[] = [];
	for (let d = 0; d <= max; d++) {
		trace.push(v.slice(off - d - 1, off + d + 2));
		for (let k = -d; k <= d; k += 2) {
			const down = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]);
			let x = down ? v[off + k + 1] : v[off + k - 1] + 1;
			let y = x - k;
			while (x < n && y < m && a[x] === b[y]) {
				x++;
				y++;
			}
			v[off + k] = x;
			if (x >= n && y >= m) return backtrack(trace, n, m);
		}
	}
	return null;
}

function backtrack(trace: Int32Array[], n: number, m: number): Step[] {
	const steps: Step[] = [];
	let x = n;
	let y = m;
	for (let d = trace.length - 1; d >= 0; d--) {
		const at = (k: number) => trace[d][k + d + 1];
		const k = x - y;
		const prevK = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
		const prevX = at(prevK);
		const prevY = prevX - prevK;
		while (x > prevX && y > prevY) {
			x--;
			y--;
			steps.push({ op: EQUAL, a: x, b: y });
		}
		if (d > 0) {
			if (x === prevX) {
				y--;
				steps.push({ op: INSERT, a: x, b: y });
			} else {
				x--;
				steps.push({ op: DELETE, a: x, b: y });
			}
		}
		x = prevX;
		y = prevY;
	}
	return steps.reverse();
}

// toHunks groups the changed steps with their context. Two changes closer
// than twice the context share a hunk, as diff -u does.
function toHunks(a: string[], b: string[], steps: Step[]): Hunk[] {
	const hunks: Hunk[] = [];
	let i = 0;
	while (i < steps.length) {
		while (i < steps.length && steps[i].op === EQUAL) i++;
		if (i >= steps.length) break;
		const start = Math.max(0, i - CONTEXT);
		let end = i;
		let j = i;
		while (j < steps.length) {
			if (steps[j].op !== EQUAL) {
				end = ++j;
				continue;
			}
			let run = j;
			while (run < steps.length && steps[run].op === EQUAL) run++;
			if (run === steps.length || run - j > 2 * CONTEXT) break;
			j = run;
		}
		const stop = Math.min(steps.length, end + CONTEXT);
		hunks.push(hunk(a, b, steps.slice(start, stop)));
		i = stop;
	}
	return hunks;
}

function hunk(a: string[], b: string[], steps: Step[]): Hunk {
	const lines: string[] = [];
	let oldLines = 0;
	let newLines = 0;
	for (const s of steps) {
		if (s.op === INSERT) {
			newLines++;
			lines.push(...hunkLines("+", b[s.b]));
		} else {
			oldLines++;
			if (s.op === EQUAL) newLines++;
			lines.push(...hunkLines(s.op === EQUAL ? " " : "-", a[s.a]));
		}
	}
	// An empty side starts at the line before it, as in diff -u.
	const first = steps[0];
	return {
		oldStart: oldLines > 0 ? first.a + 1 : first.a,
		oldLines,
		newStart: newLines > 0 ? first.b + 1 : first.b,
		newLines,
		lines,
	};
}

function hunkLines(prefix: string, line: string): string[] {
	if (line.endsWith("\n")) return [prefix + line.slice(0, -1)];
	return [prefix + line, NO_NEWLINE];
}
