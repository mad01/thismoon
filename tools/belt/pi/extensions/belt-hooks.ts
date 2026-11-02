// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * Belt Hooks Extension: PostToolUse, SessionStart and ledger parity for pi.
 *
 * permission-gate.ts owns Belt's blocking pre-tool checks. This extension owns
 * advisory hints and the lifecycle ledger, and adapts Pi events/session
 * entries to Belt's Claude hook payload. Advice is appended to tool results or
 * the next turn's system prompt, so the model receives it at the same
 * lifecycle point as in Claude/Codex. Ledger records (`belt record <event>`)
 * run in the background and never reach the model.
 * Which MCP servers count as publishing text comes from belt.json
 * (../lib/config.ts); an unreadable file falls back to the defaults, since a
 * hint never blocks.
 */

import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";
import {
	closeSync,
	mkdtempSync,
	openSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { homedir, tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawn, spawnSync } from "node:child_process";
import type { BeltPiConfig } from "../lib/config.ts";
import { directToolServer, loadConfig } from "../lib/config.ts";

interface McpIdentity {
	server: string;
	operation: string;
	claudeToolName: string;
}

// The operation belt's read hint recognises as csl's file read, whatever
// server name the csl MCP is registered under.
const CSL_READ_OPERATION = "csl_read";

let beltAvailable: boolean | null = null;

function runBeltHint(event: string, payload: Record<string, unknown>): string {
	if (beltAvailable === false) return "";
	const result = spawnSync("belt", ["hint", event], {
		input: JSON.stringify(payload),
		encoding: "utf-8",
		timeout: 10_000,
	});
	if (
		result.error &&
		(result.error as NodeJS.ErrnoException).code === "ENOENT"
	) {
		beltAvailable = false;
		return "";
	}
	beltAvailable = true;
	const output = (result.stdout ?? "").trim();
	if (!output) return "";
	try {
		const decoded = JSON.parse(output);
		return decoded?.hookSpecificOutput?.additionalContext ?? "";
	} catch {
		return "";
	}
}

// Posts one ledger entry through `belt record <event>` without waiting for
// it. The payload goes to a temp file that becomes belt's stdin, so the
// handler returns at once and a pi that quits right after cannot cut the
// payload short; belt bounds its own run (1 s for git, 300 ms for the POST).
// A record never blocks or fails the event it describes, so every error, a
// missing belt included, is dropped.
function runBeltRecord(
	event: string,
	ctx: ExtensionContext,
	fields: Record<string, unknown>,
): void {
	if (beltAvailable === false) return;
	try {
		withPayloadFile({ ...payloadBase(ctx), ...fields }, (fd) => {
			const child = spawn("belt", ["record", event], {
				stdio: [fd, "ignore", "ignore"],
				detached: true,
			});
			child.on("error", (err) => {
				if ((err as NodeJS.ErrnoException).code === "ENOENT") {
					beltAvailable = false;
				}
			});
			child.unref();
		});
	} catch {
		// Dropped on purpose: see above.
	}
}

// Runs start with an open descriptor on a temp file holding payload. The
// child inherits the descriptor at spawn, so the file can go right after.
function withPayloadFile(
	payload: Record<string, unknown>,
	start: (fd: number) => void,
): void {
	const dir = mkdtempSync(join(tmpdir(), "pi-belt-record-"));
	try {
		const file = join(dir, "payload.json");
		writeFileSync(file, JSON.stringify(payload), "utf-8");
		const fd = openSync(file, "r");
		try {
			start(fd);
		} finally {
			closeSync(fd);
		}
	} finally {
		rmSync(dir, { recursive: true, force: true });
	}
}

function mcpIdentity(
	config: BeltPiConfig,
	toolName: string,
	input: Record<string, unknown>,
): McpIdentity | null {
	const direct = directToolServer(config, toolName);
	if (direct) {
		return {
			server: direct,
			operation: toolName,
			claudeToolName: `mcp__${direct}__${toolName}`,
		};
	}
	if (toolName.startsWith("mcp__")) {
		const parts = toolName.slice(5).split("__");
		if (parts.length < 2) return null;
		return {
			server: parts.slice(0, -1).join("__"),
			operation: parts.at(-1) ?? "",
			claudeToolName: toolName,
		};
	}
	if (toolName !== "mcp" || typeof input.tool !== "string") return null;
	const operation = input.tool;
	// The unified proxy may omit the server; the same prefixes name it.
	const server =
		typeof input.server === "string" && input.server
			? input.server
			: directToolServer(config, operation);
	if (!server) return null;
	return { server, operation, claudeToolName: `mcp__${server}__${operation}` };
}

function toolCallNames(config: BeltPiConfig, ctx: ExtensionContext): string[] {
	const names: string[] = [];
	for (const entry of ctx.sessionManager.getBranch()) {
		if (entry.type !== "message" || entry.message?.role !== "assistant")
			continue;
		for (const block of entry.message.content ?? []) {
			if (block.type !== "toolCall" || typeof block.name !== "string") continue;
			const input =
				block.arguments && typeof block.arguments === "object"
					? (block.arguments as Record<string, unknown>)
					: {};
			const identity = mcpIdentity(config, block.name, input);
			names.push(identity?.claudeToolName ?? block.name);
		}
	}
	return names;
}

function withClaudeLikeTranscript<T>(
	config: BeltPiConfig,
	ctx: ExtensionContext,
	run: (path: string) => T,
): T {
	const dir = mkdtempSync(join(tmpdir(), "pi-belt-"));
	const path = join(dir, "transcript.jsonl");
	const lines = toolCallNames(config, ctx).map((name) =>
		JSON.stringify({
			message: { content: [{ type: "tool_use", name }] },
		}),
	);
	writeFileSync(path, lines.length > 0 ? `${lines.join("\n")}\n` : "", "utf-8");
	try {
		return run(path);
	} finally {
		rmSync(dir, { recursive: true, force: true });
	}
}

function payloadBase(ctx: ExtensionContext): Record<string, unknown> {
	return {
		cwd: ctx.cwd,
		session_id: ctx.sessionManager.getSessionId(),
		transcript_path: ctx.sessionManager.getSessionFile() ?? "",
	};
}

function isBashTool(toolName: string): boolean {
	return (
		toolName === "bash" || toolName === "ctx_shell" || toolName === "shell"
	);
}

// The servers whose calls publish text off this machine, the pi twin of the
// Claude PostToolUse external-text matcher.
function isExternalTextTool(
	config: BeltPiConfig,
	identity: McpIdentity | null,
): boolean {
	return identity !== null && config.externalTextHintServers.has(identity.server);
}

// The arguments an MCP call carries: a proxy call's args, else its input.
function callArgs(input: Record<string, unknown>): unknown {
	return input.args && typeof input.args === "object" ? input.args : input;
}

function hasImage(content: Array<{ type: string }> | undefined): boolean {
	return (content ?? []).some((block) => block.type === "image");
}

// The absolute path pi's read tool opened: relative to the session's working
// directory, after the leading `@` and the `~` pi expands. pi also retries a
// missing path under Unicode variants; a path this misses counts no lines,
// which keeps the hint silent.
function readPath(path: string, cwd: string): string {
	const bare = path.startsWith("@") ? path.slice(1) : path;
	const expanded =
		bare === "~" || bare.startsWith("~/") ? homedir() + bare.slice(1) : bare;
	return resolve(cwd, expanded);
}

// The file's line count as pi's read tool counts it (split on "\n"). An
// unranged read pi truncated states it in its details; otherwise the file is
// counted on disk, and one that cannot be read counts 0, which belt takes for
// unknown.
function readTotalLines(
	details: { truncation?: { totalLines?: unknown } } | undefined,
	path: string,
	ranged: boolean,
): number {
	const stated = details?.truncation?.totalLines;
	if (!ranged && typeof stated === "number") return stated;
	try {
		const bytes = readFileSync(path);
		let lines = 1;
		for (let at = bytes.indexOf(10); at !== -1; at = bytes.indexOf(10, at + 1)) {
			lines++;
		}
		return lines;
	} catch {
		return 0;
	}
}

// A pi read as the Claude Code Read payload belt's read hint takes: the
// absolute file, the range the call asked for, and the file's line count. A
// present offset or limit is a range, as belt reads it.
function readHintPayload(
	ctx: ExtensionContext,
	input: Record<string, unknown>,
	details: { truncation?: { totalLines?: unknown } } | undefined,
): Record<string, unknown> {
	const filePath = readPath(input.path as string, ctx.cwd);
	const ranged = input.offset != null || input.limit != null;
	return {
		...payloadBase(ctx),
		tool_name: "Read",
		tool_input: { file_path: filePath, offset: input.offset, limit: input.limit },
		tool_response: {
			type: "text",
			file: { totalLines: readTotalLines(details, filePath, ranged) },
		},
	};
}

// Appends advice as a text block. structuredContent rides along, since pi
// drops it from a result whose content a handler returns without it.
function appendAdvice(
	event: {
		content: Array<{ type: string; text?: string }>;
		structuredContent?: unknown;
	},
	advice: string,
) {
	if (!advice) return undefined;
	const content = [
		...event.content,
		{ type: "text" as const, text: `\n${advice}` },
	];
	return event.structuredContent === undefined
		? { content }
		: { content, structuredContent: event.structuredContent };
}

// Claude Code names a compaction's trigger manual or auto; pi's threshold and
// overflow compactions are both automatic.
function compactTrigger(reason: string): string {
	return reason === "manual" ? "manual" : "auto";
}

// Where pi's providers put an HTTP status in an error text: leading ("529
// {...}", "429: body"), after a provider name ("openrouter (503): body"), or
// named ("status code 502", "HTTP 504"). A status counts only there and never
// as part of a longer word or number, the line pi's own retry classifier
// draws.
const HTTP_STATUS_PATTERNS = [
	/^\s*(\d{3})(?![\w.])/,
	/\((\d{3})\):/,
	/\b(?:status(?:\s+code)?|http(?:\/[\d.]+)?)[\s:=]*(\d{3})(?![\w.])/i,
];

// Account limits answer 429 too, but the user has to act on them, so their
// wording keeps an error from reading as a transient rate limit.
const ACCOUNT_LIMIT_WORDING = /quota|billing|out of budget|usage limit/i;

function httpStatus(text: string): number | undefined {
	for (const pattern of HTTP_STATUS_PATTERNS) {
		const match = pattern.exec(text);
		if (match) return Number(match[1]);
	}
	return undefined;
}

// The StopFailure error type a pi error text shows without doubt, in the
// vocabulary belt's ledger reads (Claude Code's types), and "unknown"
// otherwise. A 429 or 529 status wins, then rate-limit or overloaded wording,
// then any other 5xx. The vocabulary has no context-overflow type, so a
// prompt that is too long stays unknown.
export function failureType(errorText: string): string {
	if (ACCOUNT_LIMIT_WORDING.test(errorText)) return "unknown";
	const status = httpStatus(errorText);
	if (status === 429) return "rate_limit";
	if (status === 529) return "overloaded";
	if (/rate[ _-]?limit|too many requests/i.test(errorText)) return "rate_limit";
	if (/overloaded/i.test(errorText)) return "overloaded";
	if (status !== undefined && status >= 500 && status <= 599) return "server_error";
	return "unknown";
}

// The error text an agent run ended on, when its last assistant message
// stopped on an error; null for a run that ended any other way, an abort
// included.
function runError(messages: unknown[] | undefined): string | null {
	const last = (messages ?? []).findLast(
		(message) => (message as { role?: string })?.role === "assistant",
	) as { stopReason?: string; errorMessage?: unknown } | undefined;
	if (last?.stopReason !== "error") return null;
	return typeof last.errorMessage === "string" ? last.errorMessage : "";
}

// The gate's belt:permission-denied event as a PermissionDenied record. The
// reason leads with who denied the call: the belt guard, a settings deny
// rule or the user.
function recordDenial(ctx: ExtensionContext, data: unknown): void {
	const deny = (data ?? {}) as Record<string, unknown>;
	runBeltRecord("permission-denied", ctx, {
		tool_name: deny.toolName,
		tool_use_id: deny.toolCallId,
		reason:
			typeof deny.source === "string"
				? `${deny.source}: ${deny.reason}`
				: deny.reason,
	});
}

// The advice belt's PostToolUse hints give on one tool result, "" when no
// hint watches the tool. Claude Code runs PostToolUse for a successful call
// only, so a failed read gets none, and an image is never a file csl can
// outline.
function toolResultAdvice(
	config: BeltPiConfig,
	event: {
		toolName: string;
		parentToolCallId?: string;
		input?: Record<string, unknown>;
		content: Array<{ type: string; text?: string }>;
		isError?: boolean;
		details?: { truncation?: { totalLines?: unknown } };
	},
	ctx: ExtensionContext,
): string {
	// A call a codemode script made returns its result to the script, not
	// the model: advice there goes unread and spends read-size's once-per-file
	// nudge.
	if (event.parentToolCallId) return "";
	const input = (event.input ?? {}) as Record<string, unknown>;
	if (isBashTool(event.toolName) && typeof input.command === "string") {
		return runBeltHint("bash", {
			...payloadBase(ctx),
			tool_name: "Bash",
			tool_input: { command: input.command },
		});
	}
	if (event.toolName === "read" && typeof input.path === "string") {
		if (event.isError || hasImage(event.content)) return "";
		return runBeltHint("read", readHintPayload(ctx, input, event.details));
	}

	const identity = mcpIdentity(config, event.toolName, input);
	// belt reads the file and its line count from csl_read's own response.
	if (identity?.operation === CSL_READ_OPERATION) {
		if (event.isError) return "";
		return runBeltHint("read", {
			...payloadBase(ctx),
			tool_name: identity.claudeToolName,
			tool_input: callArgs(input),
			tool_response: event.content,
		});
	}
	if (!isExternalTextTool(config, identity)) return "";
	return withClaudeLikeTranscript(config, ctx, (transcriptPath) =>
		runBeltHint("external-text", {
			...payloadBase(ctx),
			transcript_path: transcriptPath,
			tool_name: identity.claudeToolName,
			tool_input: callArgs(input),
		}),
	);
}

// The hints: session-start advice for the next system prompt, and advice
// appended to the tool results a PostToolUse hint watches.
function registerHints(pi: ExtensionAPI): void {
	let config: BeltPiConfig = loadConfig().config;
	let sessionStartAdvice = "";

	pi.on("session_start", async (event, ctx) => {
		config = loadConfig().config;
		if (event.reason === "reload") return;
		sessionStartAdvice = runBeltHint("session-start", payloadBase(ctx));
	});

	pi.on("before_agent_start", async (event) => {
		const advice = sessionStartAdvice;
		sessionStartAdvice = "";
		if (!advice) return undefined;
		return {
			systemPrompt: `${event.systemPrompt}\n\n## Belt lifecycle hints\n\n${advice}`,
		};
	});

	pi.on("tool_result", async (event, ctx) =>
		appendAdvice(event, toolResultAdvice(config, event, ctx)),
	);
}

// The ledger: the pi events with a twin among Claude Code's `belt record`
// hooks. Each record runs in the background and leaves the event as it was.
function registerLedger(pi: ExtensionAPI): void {
	// Bus events carry no context, so the denial listener is bound to the
	// session it was registered in and goes with that session.
	let stopDenyRecords = () => {};
	// The error the last agent run ended on, recorded once no retry follows.
	let failedRun: string | null = null;

	pi.on("session_start", async (_event, ctx) => {
		stopDenyRecords();
		stopDenyRecords =
			pi.events?.on?.("belt:permission-denied", (data) =>
				recordDenial(ctx, data),
			) ??
			(() => {});
	});

	pi.on("session_shutdown", async (event, ctx) => {
		stopDenyRecords();
		stopDenyRecords = () => {};
		// A reload keeps the session; quitting or leaving it ends one.
		if (event.reason === "reload") return;
		runBeltRecord("session-end", ctx, { reason: event.reason });
	});

	pi.on("session_before_compact", async (event, ctx) => {
		runBeltRecord("pre-compact", ctx, { trigger: compactTrigger(event.reason) });
	});

	pi.on("session_compact", async (event, ctx) => {
		runBeltRecord("post-compact", ctx, { trigger: compactTrigger(event.reason) });
	});
	// session_compact_failed records nothing. Claude Code has no failure
	// event, and the ledger keeps an error only on stop-failure, so a
	// post-compact entry would read as a compaction that happened. The
	// pre-compact entry with no post-compact after it is the record.

	// A failed run can be retried after agent_end, so the failure records on
	// agent_settled, which fires once no retry, compaction or queued
	// continuation will run: the point Claude Code fires StopFailure.
	pi.on("agent_end", async (event) => {
		failedRun = runError(event.messages);
	});

	pi.on("agent_settled", async (event, ctx) => {
		const error = failedRun;
		failedRun = null;
		if (error === null || event.aborted) return;
		runBeltRecord("stop-failure", ctx, {
			error: failureType(error),
			last_assistant_message: error,
		});
	});
}

export default function (pi: ExtensionAPI) {
	registerHints(pi);
	registerLedger(pi);
}
