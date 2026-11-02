// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * The files a bash command changed, for the bashEditDiff that Claude Code
 * puts in a Bash PostToolUse payload and pi does not: git status of the
 * cwd's repository before and after the command, each listed file stamped
 * with its status, size and times. A file whose stamp differs, or that only
 * one side lists, changed. Like Claude Code's list, it covers only the
 * repository of the session's cwd, and moondb's own status check then drops
 * the files a command put back to their committed state.
 *
 * moondb explains a Bash edit's lines only from its hunks, so the first few
 * changed files get them where the before is known without having read
 * it: a new file is all added lines, and a tracked file git listed clean
 * before the command differs from HEAD by exactly what the command did.
 */

import { execFile } from "node:child_process";
import { lstatSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import type { Hunk } from "./patch.ts";
import { diffHunks, parseUnifiedPatch } from "./patch.ts";

// The git call budget. A repository git cannot list in this time gets no
// Bash capture; the turn-edge snapshots still see its changes.
export const GIT_TIMEOUT_MS = 1_000;

// A status listing longer than this is not stamped: the stat calls would
// cost more than the capture is worth.
export const MAX_LISTED = 5_000;

// Claude Code's caps: changedFiles lists up to 200 paths, files gives hunks
// for up to five.
export const MAX_CHANGED = 200;
export const MAX_DIFFED = 5;

// moondb's MaxChunkBytes: a new file over it gets no hunks.
const MAX_NEW_FILE_BYTES = 1 << 20;

/** Tree is one git status of a repository, with each listed file stamped. */
export interface Tree {
	root: string;
	stamps: Map<string, string>;
}

/** BashEditDiff is the tool_response field moondb reads for a Bash call. */
export interface BashEditDiff {
	files: Array<{ filePath: string; hunks: Hunk[]; created?: true }>;
	changedFiles: string[];
}

function git(cwd: string, args: string[]): Promise<string | undefined> {
	return new Promise((resolve) => {
		execFile(
			"git",
			["-C", cwd, "--no-optional-locks", ...args],
			{ timeout: GIT_TIMEOUT_MS, maxBuffer: 16 << 20, encoding: "utf-8" },
			(err, stdout) => resolve(err ? undefined : stdout),
		);
	});
}

/**
 * readTree lists the changed and untracked files of the repository holding
 * cwd, or undefined when cwd is in none, git fails or runs out of time, or
 * the list is too long to stamp. roots caches each cwd's repository root.
 */
export async function readTree(
	cwd: string,
	roots: Map<string, string>,
): Promise<Tree | undefined> {
	let root = roots.get(cwd);
	if (root === undefined) {
		root = (await git(cwd, ["rev-parse", "--show-toplevel"]))?.trim();
		if (!root) return undefined;
		roots.set(cwd, root);
	}
	const out = await git(root, ["status", "--porcelain=v1", "-z", "--untracked-files=all"]);
	if (out === undefined) return undefined;
	const entries = parseStatus(out);
	if (entries.length > MAX_LISTED) return undefined;
	const stamps = new Map<string, string>();
	for (const [code, rel] of entries) {
		const abs = join(root, rel);
		stamps.set(abs, `${code}|${stat(abs)}`);
	}
	return { root, stamps };
}

// parseStatus reads porcelain v1 -z: "XY path", and for a rename or copy
// the original path as the next field, which is not listed.
function parseStatus(out: string): Array<[string, string]> {
	const fields = out.split("\0");
	const entries: Array<[string, string]> = [];
	for (let i = 0; i < fields.length; i++) {
		const field = fields[i];
		if (field.length < 4) continue;
		const code = field.slice(0, 2);
		entries.push([code, field.slice(3)]);
		if (code[0] === "R" || code[0] === "C") i++;
	}
	return entries;
}

function stat(abs: string): string {
	try {
		const info = lstatSync(abs);
		return `${info.size}|${info.mtimeMs}|${info.ctimeMs}`;
	} catch {
		return "gone";
	}
}

/**
 * changedBetween lists the files a command run between before and after
 * changed: every file whose stamp changed, by absolute path and sorted,
 * minus the ones in skip (files pi's own file tools wrote meanwhile, which
 * their own records cover). Nothing when the trees are of different
 * repositories.
 */
export function changedBetween(before: Tree, after: Tree, skip: Set<string> = new Set()): string[] {
	if (before.root !== after.root) return [];
	const paths = new Set([...before.stamps.keys(), ...after.stamps.keys()]);
	return [...paths]
		.filter((p) => !skip.has(p) && before.stamps.get(p) !== after.stamps.get(p))
		.sort()
		.slice(0, MAX_CHANGED);
}

/**
 * bashEditDiff is the payload field for the changed files: all of them in
 * changedFiles, and the first MAX_DIFFED whose hunks can be had in files.
 * A file the tree before did not list was clean or absent then: if it is
 * untracked or added now it is new, otherwise git diff HEAD is its change.
 */
export async function bashEditDiff(before: Tree, after: Tree, changed: string[]): Promise<BashEditDiff> {
	const diff: BashEditDiff = { files: [], changedFiles: changed };
	for (const path of changed) {
		if (diff.files.length >= MAX_DIFFED) break;
		const stamp = after.stamps.get(path);
		if (before.stamps.has(path) || !stamp || stamp.endsWith("|gone")) continue;
		const code = stamp.slice(0, 2);
		if (code.includes("D")) continue;
		if (code === "??" || code[0] === "A") {
			const hunks = newFileHunks(path);
			if (hunks) diff.files.push({ filePath: path, hunks, created: true });
			continue;
		}
		const out = await git(after.root, ["diff", "HEAD", "--no-color", "--no-ext-diff", "-U3", "--", relative(after.root, path)]);
		const hunks = parseUnifiedPatch(out);
		if (hunks) diff.files.push({ filePath: path, hunks });
	}
	return diff;
}

function newFileHunks(path: string): Hunk[] | undefined {
	try {
		if (lstatSync(path).size > MAX_NEW_FILE_BYTES) return undefined;
		const text = readFileSync(path, "utf-8");
		return text.includes("\0") ? undefined : diffHunks("", text);
	} catch {
		return undefined;
	}
}
