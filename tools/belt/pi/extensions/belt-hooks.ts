// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * Belt Hooks Extension: PostToolUse and SessionStart parity for pi.
 *
 * permission-gate.ts owns Belt's blocking pre-tool checks. This extension owns
 * advisory hints and adapts Pi events/session entries to Belt's Claude hook
 * payload. Advice is appended to tool results or the next turn's system prompt,
 * so the model receives it at the same lifecycle point as in Claude/Codex.
 * Which MCP servers count as publishing text comes from belt.json
 * (../lib/config.ts); an unreadable file falls back to the defaults, since a
 * hint never blocks.
 */

import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import type { BeltPiConfig } from "../lib/config.ts";
import { directToolServer, loadConfig } from "../lib/config.ts";

interface McpIdentity {
	server: string;
	operation: string;
	claudeToolName: string;
}

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

function appendAdvice(
	event: { content: Array<{ type: string; text?: string }> },
	advice: string,
) {
	if (!advice) return undefined;
	return {
		content: [...event.content, { type: "text" as const, text: `\n${advice}` }],
	};
}

export default function (pi: ExtensionAPI) {
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

	pi.on("tool_result", async (event, ctx) => {
		const input = (event.input ?? {}) as Record<string, unknown>;
		if (isBashTool(event.toolName) && typeof input.command === "string") {
			const advice = runBeltHint("bash", {
				...payloadBase(ctx),
				tool_name: "Bash",
				tool_input: { command: input.command },
			});
			return appendAdvice(event, advice);
		}

		const identity = mcpIdentity(config, event.toolName, input);
		if (isExternalTextTool(config, identity)) {
			const advice = withClaudeLikeTranscript(config, ctx, (transcriptPath) =>
				runBeltHint("external-text", {
					...payloadBase(ctx),
					transcript_path: transcriptPath,
					tool_name: identity.claudeToolName,
					tool_input:
						input.args && typeof input.args === "object" ? input.args : input,
				}),
			);
			return appendAdvice(event, advice);
		}

		return undefined;
	});
}
