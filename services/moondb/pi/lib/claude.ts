// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * pi's file tools in the shape of the Claude Code PostToolUse payloads
 * moondb record reads (internal/record/payload.go, edit.go): the tool name,
 * tool_input and tool_response of an Edit, MultiEdit or Write, so moondb's
 * derivation runs on them unchanged.
 */

import { lstatSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { Hunk } from "./patch.ts";
import { diffHunks, parseUnifiedPatch } from "./patch.ts";

// moondb's MaxChunkBytes: a file over it is recorded by path and patch, so
// reading more of it here buys nothing.
export const MAX_ORIGINAL_BYTES = 1 << 20;

const UNICODE_SPACES = /[  -   　]/g;

export interface ToolCall {
	tool_name: string;
	tool_input: Record<string, unknown>;
	tool_response: Record<string, unknown>;
	// The absolute path the call changed.
	path: string;
}

/**
 * resolvePiPath resolves a file tool's path the way pi's tools do
 * (resolveToCwd): unicode spaces read as spaces, a leading @ is dropped,
 * ~ is the home directory, and a relative path sits under cwd.
 */
export function resolvePiPath(path: string, cwd: string): string {
	let p = path.replace(UNICODE_SPACES, " ");
	if (p.startsWith("@")) p = p.slice(1);
	if (p === "~") p = homedir();
	else if (p.startsWith("~/")) p = join(homedir(), p.slice(2));
	else if (p.startsWith("file://")) p = fileURLToPath(p);
	return isAbsolute(p) ? resolve(p) : resolve(cwd, p);
}

function pathOf(input: Record<string, unknown>): string | undefined {
	const path = input.path ?? input.file_path;
	return typeof path === "string" && path !== "" ? path : undefined;
}

/** targetPath is the absolute path a file tool's input names, if any. */
export function targetPath(input: Record<string, unknown>, cwd: string): string | undefined {
	const raw = pathOf(input);
	return raw ? resolvePiPath(raw, cwd) : undefined;
}

interface Replacement {
	oldText: string;
	newText: string;
}

// replacementsOf reads pi's edits[], or the legacy single oldText/newText
// pair pi still accepts.
function replacementsOf(input: Record<string, unknown>): Replacement[] {
	const edits = Array.isArray(input.edits) ? [...input.edits] : [];
	if (typeof input.oldText === "string" && typeof input.newText === "string") {
		edits.push({ oldText: input.oldText, newText: input.newText });
	}
	return edits.filter(
		(e) => e && typeof e.oldText === "string" && typeof e.newText === "string",
	);
}

/**
 * editCall translates a pi edit: one replacement is an Edit, several a
 * MultiEdit, whose swaps moondb applies in order. pi matches every
 * replacement against the original file instead; for the disjoint,
 * unique replacements pi requires, the two give the same file. The hunks
 * come from the unified diff pi returns in details.patch.
 */
export function editCall(
	input: Record<string, unknown>,
	details: unknown,
	cwd: string,
): ToolCall | undefined {
	const filePath = targetPath(input, cwd);
	const reps = replacementsOf(input);
	if (!filePath || reps.length === 0) return undefined;
	const swaps = reps.map((r) => ({
		old_string: r.oldText,
		new_string: r.newText,
		replace_all: false,
	}));
	const response: Record<string, unknown> = { filePath, userModified: false };
	const hunks = parseUnifiedPatch((details as { patch?: unknown } | undefined)?.patch);
	if (hunks) response.structuredPatch = hunks;
	if (swaps.length === 1) {
		return {
			tool_name: "Edit",
			tool_input: { file_path: filePath, ...swaps[0] },
			tool_response: response,
			path: filePath,
		};
	}
	return {
		tool_name: "MultiEdit",
		tool_input: { file_path: filePath, edits: swaps },
		tool_response: response,
		path: filePath,
	};
}

/**
 * Before is what a file held when a write was about to run: whether it
 * existed, and its text when it was small enough and not binary.
 */
export interface Before {
	existed: boolean;
	original?: string;
}

/** readBefore reads the state a write is about to replace. */
export function readBefore(abs: string): Before {
	let size: number;
	try {
		const info = lstatSync(abs);
		if (!info.isFile()) return { existed: true };
		size = info.size;
	} catch (err) {
		return { existed: (err as NodeJS.ErrnoException).code !== "ENOENT" };
	}
	if (size > MAX_ORIGINAL_BYTES) return { existed: true };
	try {
		const original = readFileSync(abs, "utf-8");
		return original.includes("\0") ? { existed: true } : { existed: true, original };
	} catch {
		return { existed: true };
	}
}

/**
 * writeCall translates a pi write. pi does not say whether the file was
 * new, so the state read before the write decides create or update; an
 * update carries the original and the hunks from it to the content.
 * Without a before read, the write is an update with neither.
 */
export function writeCall(
	input: Record<string, unknown>,
	before: Before | undefined,
	cwd: string,
): ToolCall | undefined {
	const filePath = targetPath(input, cwd);
	if (!filePath || typeof input.content !== "string") return undefined;
	const content = input.content;
	const response: Record<string, unknown> = { filePath, content };
	if (before && !before.existed) {
		Object.assign(response, { type: "create", originalFile: null, structuredPatch: [] });
	} else {
		response.type = "update";
		if (before?.original !== undefined) {
			const hunks: Hunk[] = diffHunks(before.original, content);
			Object.assign(response, { originalFile: before.original, structuredPatch: hunks });
		}
	}
	return {
		tool_name: "Write",
		tool_input: { file_path: filePath, content },
		tool_response: response,
		path: filePath,
	};
}
