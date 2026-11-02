// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * moondb record for pi: the pi twin of the Claude Code hook lines that feed
 * `moondb record` (services/moondb/CLAUDE.md, "Capture"), less SubagentStop,
 * which pi has no event for. Each pi event
 * becomes the Claude-shaped payload moondb already reads, piped on stdin to
 * a `moondb record` child:
 *
 *   before_agent_start   UserPromptSubmit  opens a turn
 *   tool_result edit     PostToolUse Edit or MultiEdit
 *   tool_result write    PostToolUse Write
 *   tool_result bash     PostToolUse Bash with a bashEditDiff (../lib/worktree.ts)
 *   agent_settled        Stop              closes the turn
 *   session_shutdown     Stop for a turn still open, then waits for records
 *
 * The edit and write records are waited for, up to the 3 s the Claude Edit
 * hook is registered with, because moondb reads the file from disk and a
 * later tool call must not change it first. The others start and run on
 * their own, as the async Claude hooks do. A handler never throws and never
 * changes the event it sees; with no moondb on PATH every handler is a no-op.
 */

import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import type { Before } from "../lib/claude.ts";
import { editCall, readBefore, targetPath, writeCall } from "../lib/claude.ts";
import type { Tree } from "../lib/worktree.ts";
import { bashEditDiff, changedBetween, readTree } from "../lib/worktree.ts";

// The Claude Code Edit hook's timeout, which moondb's record ceiling sits
// under.
export const RECORD_TIMEOUT_MS = 3_000;

// The variable moondb reads a session's entrypoint from (record.EntrypointEnv).
const ENTRYPOINT_ENV = "CLAUDE_CODE_ENTRYPOINT";
const TAG = /^[a-z0-9][a-z0-9-]{0,31}$/;

let moondbMissing = false;

/**
 * entrypoint names how pi runs, for moondb's session entrypoint: pi-tui,
 * pi-print, pi-json or pi-rpc. It replaces any CLAUDE_CODE_ENTRYPOINT pi
 * inherited, say from a Claude Code shell, so a pi session never reads as
 * a Claude one.
 */
export function entrypoint(mode: unknown): string {
	const value = typeof mode === "string" ? `pi-${mode}` : "pi";
	return TAG.test(value) ? value : "pi";
}

// record runs `moondb record` with payload on stdin and resolves when it
// exits, fails to start or runs past RECORD_TIMEOUT_MS. It never rejects.
// The child gets its own process group, so a Ctrl-C meant for pi does not
// cut a record short. Child and timer stay referenced: a record someone
// waits on, the edit's or shutdown's, must keep the process alive until it
// ends, and print mode exits as soon as nothing does.
function record(payload: Record<string, unknown>, mode: unknown): Promise<void> {
	if (moondbMissing) return Promise.resolve();
	return new Promise((resolve) => {
		let timer: NodeJS.Timeout | undefined;
		const done = () => {
			clearTimeout(timer);
			resolve();
		};
		try {
			const child = spawn("moondb", ["record"], {
				stdio: ["pipe", "ignore", "ignore"],
				detached: true,
				env: { ...process.env, [ENTRYPOINT_ENV]: entrypoint(mode) },
			});
			timer = setTimeout(done, RECORD_TIMEOUT_MS);
			child.on("error", (err: NodeJS.ErrnoException) => {
				if (err.code === "ENOENT") moondbMissing = true;
				done();
			});
			child.on("exit", done);
			child.stdin.on("error", () => {});
			child.stdin.end(JSON.stringify(payload));
		} catch {
			done();
		}
	});
}

function lastAssistantText(messages: unknown): string | undefined {
	if (!Array.isArray(messages)) return undefined;
	for (let i = messages.length - 1; i >= 0; i--) {
		const message = messages[i];
		if (message?.role !== "assistant") continue;
		const content = message.content;
		if (typeof content === "string") return content;
		if (!Array.isArray(content)) return undefined;
		return content
			.filter((block) => block?.type === "text" && typeof block.text === "string")
			.map((block) => block.text)
			.join("\n");
	}
	return undefined;
}

interface Turn {
	promptId: string;
	lastMessage: string;
}

// A bash call's tree before it ran, and the file-tool write count then.
interface BashStart {
	tree: Tree;
	writes: number;
}

export default function (pi: ExtensionAPI) {
	let turn: Turn | undefined;
	const writesBefore = new Map<string, Before>();
	const bashStarts = new Map<string, BashStart>();
	// The paths pi's file tools wrote while a bash call ran, in order, so
	// the bash record leaves them to the file tools' own records.
	let fileWrites: string[] = [];
	const roots = new Map<string, string>();
	const inflight = new Set<Promise<void>>();

	const payload = (ctx: ExtensionContext, event: string): Record<string, unknown> => {
		const sessions = ctx.sessionManager;
		const p: Record<string, unknown> = {
			hook_event_name: event,
			session_id: sessions.getSessionId(),
			transcript_path: sessions.getSessionFile() ?? "",
			cwd: ctx.cwd,
		};
		const title = sessions.getSessionName?.();
		if (typeof title === "string" && title) p.session_title = title;
		if (turn) p.prompt_id = turn.promptId;
		return p;
	};

	// start runs a record nobody waits on but session_shutdown.
	const start = (p: Record<string, unknown>, ctx: ExtensionContext) => {
		const pending = record(p, ctx.mode);
		inflight.add(pending);
		pending.then(() => inflight.delete(pending));
	};

	// closeTurn records the open turn's Stop. No tool runs between turns, so
	// the per-call state goes too, including that of a call a gate blocked,
	// which never reaches tool_result.
	const closeTurn = (ctx: ExtensionContext) => {
		writesBefore.clear();
		bashStarts.clear();
		fileWrites = [];
		if (!turn) return;
		const p = payload(ctx, "Stop");
		if (turn.lastMessage) p.last_assistant_message = turn.lastMessage;
		turn = undefined;
		start(p, ctx);
	};

	// recordBash records the files a bash call changed, whatever its exit
	// status: a failed command can still have changed some.
	const recordBash = async (event, ctx: ExtensionContext) => {
		const begun = bashStarts.get(event.toolCallId);
		bashStarts.delete(event.toolCallId);
		const skip = new Set(begun ? fileWrites.slice(begun.writes) : []);
		if (bashStarts.size === 0) fileWrites = [];
		const command = event.input?.command;
		if (!begun || typeof command !== "string") return;
		const after = await readTree(ctx.cwd, roots);
		if (!after) return;
		const changed = changedBetween(begun.tree, after, skip);
		if (changed.length === 0) return;
		const diff = await bashEditDiff(begun.tree, after, changed);
		start(
			{
				...payload(ctx, "PostToolUse"),
				tool_name: "Bash",
				tool_input: { command },
				tool_response: { bashEditDiff: diff },
				tool_use_id: event.toolCallId,
			},
			ctx,
		);
	};

	// recordFile records an edit or write and waits for it.
	const recordFile = async (event, ctx: ExtensionContext, before: Before | undefined) => {
		const input = event.input ?? {};
		const call =
			event.toolName === "edit"
				? editCall(input, event.details, ctx.cwd)
				: writeCall(input, before, ctx.cwd);
		if (!call) return;
		if (bashStarts.size > 0) fileWrites.push(call.path);
		const { path: _path, ...fields } = call;
		await record(
			{ ...payload(ctx, "PostToolUse"), ...fields, tool_use_id: event.toolCallId },
			ctx.mode,
		);
	};

	// guard makes a handler best effort: it never throws, never changes the
	// event, and does nothing once moondb turned out to be missing.
	const guard =
		(handle: (event, ctx: ExtensionContext) => unknown) =>
		async (event, ctx: ExtensionContext) => {
			if (moondbMissing) return undefined;
			try {
				await handle(event, ctx);
			} catch {
				// A capture failure must never reach pi.
			}
			return undefined;
		};

	// A prompt opens a turn. A run an extension started with a custom
	// message opens one too, without a prompt, which moondb gives no origin.
	pi.on(
		"before_agent_start",
		guard((event, ctx) => {
			closeTurn(ctx);
			turn = { promptId: randomUUID(), lastMessage: "" };
			const p = payload(ctx, "UserPromptSubmit");
			const prompt = event.messageType === "user" ? event.prompt : undefined;
			p.prompt = typeof prompt === "string" ? prompt : "";
			start(p, ctx);
		}),
	);

	pi.on(
		"agent_end",
		guard((event) => {
			const text = lastAssistantText(event.messages);
			if (turn && text) turn.lastMessage = text;
		}),
	);

	// agent_end can be followed by a retry or a queued message; settled
	// means pi has nothing left to run for the prompt.
	pi.on("agent_settled", guard((_event, ctx) => closeTurn(ctx)));

	pi.on(
		"tool_call",
		guard(async (event, ctx) => {
			if (event.toolName === "write") {
				const path = targetPath(event.input ?? {}, ctx.cwd);
				if (path) writesBefore.set(event.toolCallId, readBefore(path));
			} else if (event.toolName === "bash") {
				const tree = await readTree(ctx.cwd, roots);
				if (tree) bashStarts.set(event.toolCallId, { tree, writes: fileWrites.length });
			}
		}),
	);

	pi.on(
		"tool_result",
		guard(async (event, ctx) => {
			const before = writesBefore.get(event.toolCallId);
			writesBefore.delete(event.toolCallId);
			if (event.toolName === "bash") await recordBash(event, ctx);
			else if ((event.toolName === "edit" || event.toolName === "write") && !event.isError) {
				await recordFile(event, ctx, before);
			}
		}),
	);

	pi.on(
		"session_shutdown",
		guard(async (_event, ctx) => {
			closeTurn(ctx);
			await Promise.all(inflight);
		}),
	);
}
