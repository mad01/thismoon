// @ts-nocheck -- pi resolves extension types from its runtime; Claude Code never loads this file.
/**
 * humanizer-gate, pi face. Same contract as the Claude face
 * (../hooks/register.ts), on pi's split events: a `tool_call` handler scans
 * a commit message or a pull request body before the call and may hold it
 * for a Proceed/Cancel; a `tool_result` handler scans a markdown file after
 * the write landed and pins the finding counts. Config is the lib's
 * DEFAULT_CONFIG (pi has no per-plugin userConfig). The gate never blocks on
 * its own failure: a missing binary, a detect error, or a dialog nobody can
 * answer all let the call through.
 */
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";

import {
	CANCELLED_REASON,
	DEFAULT_CONFIG,
	DETECT_BUDGET_MS,
	HOLD_OPTIONS,
	basename,
	extractCommitMessage,
	holdQuestion,
	isProseFile,
	parseDetectOutput,
	statusText,
} from "../lib/gate.ts";
import type { FindingsSummary, Severity } from "../lib/gate.ts";

const STATUS_KEY = "humanizer-gate";
const WRITE_TOOLS = new Set(["write", "edit"]);
const PR_OPERATIONS = new Set(["create_pull_request", "update_pull_request"]);

type DetectSource = { file: string } | { text: string };

// One `humanizer detect --json` run under the budget; null when it failed or
// wrote no payload.
function detect(binary: string, source: DetectSource, level: Severity): FindingsSummary | null {
	const argv = ["detect", "--json", "--min-severity", level];
	if ("file" in source) argv.push(source.file);
	const ran = spawnSync(binary, argv, {
		encoding: "utf-8",
		input: "text" in source ? source.text : undefined,
		timeout: DETECT_BUDGET_MS,
	});
	if (ran.error || ran.status !== 0) return null;
	return parseDetectOutput(ran.stdout ?? "");
}

// The body of a github.com pull request call, whether the model called
// mcp__gh_com__<op> directly or went through the `mcp` proxy with
// server: gh_com. Null for every other call.
function pullRequestBody(toolName: string, input: Record<string, unknown>): string | null {
	let server = "";
	let operation = "";
	let args: Record<string, unknown> = input;
	if (toolName === "mcp" && typeof input.tool === "string") {
		server = typeof input.server === "string" ? input.server : "";
		operation = input.tool;
		args = input.args && typeof input.args === "object" ? (input.args as Record<string, unknown>) : {};
	} else if (toolName.startsWith("mcp__")) {
		const body = toolName.slice(5);
		const split = body.lastIndexOf("__");
		server = body.slice(0, split);
		operation = body.slice(split + 2);
	} else {
		return null;
	}
	if (server !== "gh_com" || !PR_OPERATIONS.has(operation)) return null;
	return typeof args.body === "string" ? args.body : "";
}

export default function (pi: ExtensionAPI) {
	const config = DEFAULT_CONFIG;
	let binary: string | null = null;

	const report = (ctx: ExtensionContext, summary: FindingsSummary | null, target: string): void => {
		if (summary === null || !ctx.hasUI) return;
		ctx.ui.setStatus(STATUS_KEY, statusText(summary, target));
	};

	// True only when the person picked Cancel; no UI or a failed dialog lets
	// the call through.
	const cancelled = async (ctx: ExtensionContext, summary: FindingsSummary, target: string): Promise<boolean> => {
		if (!ctx.hasUI) return false;
		try {
			const answer = await ctx.ui.select(holdQuestion(summary, target), [...HOLD_OPTIONS]);
			return answer === HOLD_OPTIONS[1];
		} catch {
			return false;
		}
	};

	pi.on("session_start", async () => {
		const candidate = join(homedir(), "code", "bin", "humanizer");
		binary = existsSync(candidate) ? candidate : null;
	});

	pi.on("tool_call", async (event, ctx) => {
		if (binary === null) return undefined;
		const input = (event.input ?? {}) as Record<string, unknown>;

		if (event.toolName === "bash" && typeof input.command === "string") {
			const message = extractCommitMessage(input.command);
			if (message === null) return undefined;
			const summary = detect(binary, { text: message }, config.minSeverity);
			report(ctx, summary, "commit message");
			const held = summary !== null && summary.errors > 0 && config.holdOnCommitError;
			if (held && (await cancelled(ctx, summary, "commit message"))) {
				return { block: true, reason: CANCELLED_REASON };
			}
			return undefined;
		}

		const body = pullRequestBody(event.toolName, input);
		if (body === null || body.trim() === "") return undefined;
		const summary = detect(binary, { text: body }, config.minSeverity);
		report(ctx, summary, "PR body");
		const held = summary !== null && summary.errors > 0 && config.holdOnError;
		if (held && (await cancelled(ctx, summary, "PR body"))) {
			return { block: true, reason: CANCELLED_REASON };
		}
		return undefined;
	});

	// The write happened first; the scan only reports on what landed.
	pi.on("tool_result", async (event, ctx) => {
		if (binary === null || !WRITE_TOOLS.has(event.toolName) || event.isError) return undefined;
		const input = (event.input ?? {}) as Record<string, unknown>;
		const path = typeof input.path === "string" ? input.path : "";
		if (!isProseFile(path)) return undefined;
		report(ctx, detect(binary, { file: resolve(ctx.cwd, path) }, config.minSeverity), basename(path));
		return undefined;
	});
}
