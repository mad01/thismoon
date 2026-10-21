// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * Permission Gate Extension: Claude Code permission parity for pi
 *
 * Sources of truth (no hardcoded rule lists):
 * - ~/.claude/settings.json + ~/.claude/settings.local.json permissions
 *   (allow / deny / ask), same entries Claude Code enforces:
 *   Bash(prefix:*), Read(glob), mcp__server__tool, mcp__server__*
 * - belt guards, invoked per tool call via `belt check bash <cmd>`,
 *   `belt check write --file <path> --content <text>`, and
 *   `belt check external-text --tool <name> --input <json>` for MCP calls
 *   that publish text (same guards the Claude PreToolUse hooks run).
 * - belt.json (../lib/config.ts): which MCP servers reach the external-text
 *   guard, how direct adapter tools map back to a server, and which env vars
 *   signal a sandbox. The defaults name no machine-specific server.
 *
 * Order per call: belt → deny → ask (confirm) → allow → tier defaults
 * (read allowed, in-project edits allowed, shell confirms, MCP allowed).
 * "Always deny" answers persist as session rules; /permissions manages them.
 */

import { execFileSync, execSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, relative, resolve, sep } from "node:path";
import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";
import type { BeltPiConfig, LoadedConfig } from "../lib/config.ts";
import { directToolServer, envIs, loadConfig } from "../lib/config.ts";
import {
	markPermissionApproved,
	requestPermission,
} from "./permission-broker.ts";

interface PermissionRule {
	pattern: string;
	toolName: string;
	action: "allow" | "deny";
}

interface ClaudeRule {
	tool: string; // Bash, Read, Write, Edit, WebFetch, mcp__server__tool, plain tool name
	spec: string | null; // inside the parens, null when the entry is a bare tool name
}

interface ClaudePermissions {
	allow: ClaudeRule[];
	deny: ClaudeRule[];
	ask: ClaudeRule[];
}

const SETTINGS_FILES = [
	"~/.claude/settings.json",
	"~/.claude/settings.local.json",
];
const BASH_TOOL_NAMES = new Set(["bash", "ctx_shell", "shell"]);
const READ_TOOL_NAMES = new Set(["read", "ctx_read"]);
const MUTATION_TOOL_NAMES = new Set([
	"write",
	"edit",
	"ctx_edit",
	"ctx_patch",
	"ast_grep_replace",
]);

interface MutationTarget {
	path: string;
	content: string;
}

function expandPath(p: string): string {
	if (p.startsWith("~/") || p === "~") {
		return p.replace(/^~/, process.env.HOME || "~");
	}
	return p;
}

function parseEntry(entry: string): ClaudeRule {
	const m = entry.match(/^([^()]+)\((.*)\)$/);
	if (m) return { tool: m[1], spec: m[2] };
	return { tool: entry, spec: null };
}

function loadClaudePermissions(): ClaudePermissions {
	const perms: ClaudePermissions = { allow: [], deny: [], ask: [] };
	for (const file of SETTINGS_FILES) {
		try {
			const raw = JSON.parse(readFileSync(expandPath(file), "utf-8"));
			const p = raw?.permissions ?? {};
			for (const key of ["allow", "deny", "ask"] as const) {
				for (const entry of p[key] ?? []) {
					if (typeof entry === "string") perms[key].push(parseEntry(entry));
				}
			}
		} catch {
			// missing or unparsable settings file: skip
		}
	}
	return perms;
}

// Glob → regex for Read/Write path specs: ** crosses directories, * does not.
function globToRegex(glob: string): RegExp {
	const expanded = expandPath(glob);
	let regex = "";
	for (let index = 0; index < expanded.length; index++) {
		const char = expanded[index];
		if (char === "*" && expanded[index + 1] === "*") {
			regex += ".*";
			index++;
		} else if (char === "*") {
			regex += "[^/]*";
		} else {
			regex += char.replace(/[.+^${}()|[\]\\]/g, "\\$&");
		}
	}
	return new RegExp(`^${regex}$`);
}

function bashSpecMatches(spec: string, command: string): boolean {
	const cmd = command.trimStart();
	if (spec.endsWith(":*")) return cmd.startsWith(spec.slice(0, -2));
	return cmd === spec;
}

// Match Claude-style MCP rules against the adapter's proxy identity. Older
// prefixed direct tools are also accepted for session compatibility.
function mcpGlob(pattern: string): RegExp {
	const escaped = pattern
		.replace(/[.+^${}()|[\]\\]/g, "\\$&")
		.replace(/\*/g, ".*");
	return new RegExp(`^${escaped}$`);
}

function mcpRuleMatches(rule: ClaudeRule, toolName: string): boolean {
	if (!rule.tool.startsWith("mcp__")) return false;
	if (mcpGlob(rule.tool).test(toolName)) return true;
	// A rule that names only a server (mcp__linear) covers every tool on
	// it, as in Claude.
	if (!rule.tool.slice(5).includes("__")) {
		return mcpGlob(`${rule.tool}__*`).test(toolName);
	}
	const m = rule.tool.match(/^mcp__([^_]+(?:_[^_]+)*?)__(.+)$/);
	if (m && m[2].startsWith(`${m[1]}_`)) {
		const stripped = `mcp__${m[1]}__${m[2].slice(m[1].length + 1)}`;
		return mcpGlob(stripped).test(toolName);
	}
	return false;
}

function toolKind(toolName: string): "bash" | "read" | "mutation" | "other" {
	const lower = toolName.toLowerCase();
	if (BASH_TOOL_NAMES.has(lower)) return "bash";
	if (READ_TOOL_NAMES.has(lower)) return "read";
	if (MUTATION_TOOL_NAMES.has(lower)) return "mutation";
	return "other";
}

function mutationContent(input: Record<string, unknown>): string {
	const chunks: string[] = [];
	for (const key of [
		"content",
		"newText",
		"new_string",
		"new_text",
		"replace",
		"text",
	]) {
		const value = input[key];
		if (typeof value === "string" && value) chunks.push(value);
	}
	for (const key of ["edits", "ops"]) {
		const values = input[key];
		if (!Array.isArray(values)) continue;
		for (const value of values) {
			if (value && typeof value === "object") {
				chunks.push(mutationContent(value as Record<string, unknown>));
			}
		}
	}
	return chunks.filter(Boolean).join("\n");
}

function mutationTargets(input: Record<string, unknown>): MutationTarget[] {
	const topPath = [input.path, input.file, input.file_path].find(
		(value) => typeof value === "string",
	) as string | undefined;
	if (Array.isArray(input.ops) && input.ops.length > 0) {
		const targets: MutationTarget[] = [];
		for (const value of input.ops) {
			if (!value || typeof value !== "object") continue;
			const operation = value as Record<string, unknown>;
			targets.push({
				path:
					([operation.path, operation.file, operation.file_path, topPath].find(
						(entry) => typeof entry === "string",
					) as string | undefined) ?? "",
				content: mutationContent(operation),
			});
		}
		return targets;
	}
	return [{ path: topPath ?? "", content: mutationContent(input) }];
}

// One way pi-mcp-adapter may run a tool call: the server and operation it
// reaches, their Claude-style name (mcp__<server>__<op>) and the arguments
// the MCP server receives. The adapter resolves a tool name loosely (with or
// without the server, bare or under a display or legacy server prefix), so
// one call can have several readings, and every check takes the strictest
// of them. unresolved marks the placeholder reading of a proxy call whose
// server stays unknown.
interface McpReading extends McpTarget {
	name: string;
	args: Record<string, unknown>;
	unresolved?: boolean;
}

interface McpTarget {
	server: string;
	operation: string;
}

// The readings of a call, none for a tool that is not MCP. A proxy call is
// read by the tool it names. A namespace proxy name may also be an mcp-mode
// direct tool (mcp__<server>_<op>), so its direct readings count too.
function mcpReadings(
	config: BeltPiConfig,
	toolName: string,
	input: Record<string, unknown>,
): McpReading[] {
	const direct = toolTargets(config, toolName).map((target) => reading(target, input));
	const proxy = proxyTarget(config, toolName, input);
	if (!proxy) return direct;
	const args = proxyArgs(input);
	const targets = proxy.server
		? serverTargets(config, proxy.server, proxy.tool)
		: toolTargets(config, proxy.tool);
	// The adapter's documented proxy call omits the server
	// (mcp({ tool, args })), and it then finds the tool by name on every
	// enabled server, so a name that places itself nowhere may still land on
	// a guarded one.
	if (targets.length === 0) {
		const placeholder = reading({ server: "mcp", operation: proxy.tool }, args);
		return [{ ...placeholder, unresolved: true }, ...direct];
	}
	return [...targets.map((target) => reading(target, args)), ...direct];
}

function reading(target: McpTarget, args: Record<string, unknown>): McpReading {
	return { ...target, name: `mcp__${target.server}__${target.operation}`, args };
}

// The server and tool a proxy call names, null for any other call. Two
// tools proxy: the unified `mcp`, whose server is optional, and the
// mcp__<namespace> namespace proxy the adapter registers for each
// proxy-only server by default, which takes the same { tool, args } and is
// bound to its server.
function proxyTarget(
	config: BeltPiConfig,
	toolName: string,
	input: Record<string, unknown>,
): { server: string; tool: string } | null {
	if (typeof input.tool !== "string") return null;
	if (toolName === "mcp") {
		const server = typeof input.server === "string" ? input.server : "";
		return { server, tool: input.tool };
	}
	const server = namespaceServer(config, toolName);
	return server ? { server, tool: input.tool } : null;
}

// The server a namespace proxy name is bound to, "" when toolName is none.
// A server belt.json names matches in the adapter's namespace form; any
// other mcp__<namespace> with one "__" is taken as that namespace.
function namespaceServer(config: BeltPiConfig, toolName: string): string {
	if (!toolName.startsWith("mcp__")) return "";
	const namespace = toolName.slice(5);
	const known = [
		...config.externalTextGuardServers,
		...config.mcpPrefixes.map(([, server]) => server),
	];
	const server = known.find((name) => serverNamespace(name) === namespace);
	if (server) return server;
	return namespace && !namespace.includes("__") ? namespace : "";
}

// The adapter's own namespace for a server, ported from formatServerNamespace
// in its types.ts: "-" folds to "_"; a name with any other character outside
// letters, digits and "_" is encoded behind _mcpns_ ("_" doubled, the rest as
// _<hex>_); and one longer than the 59 characters a tool name leaves after
// mcp__ is cut and hashed.
function serverNamespace(server: string): string {
	const marker = "_mcpns_";
	const limit = 59;
	const normalized = server.replace(/-/g, "_");
	if (/^[A-Za-z0-9_]*$/.test(normalized) && !normalized.startsWith(marker)) {
		return hashedNamespace(normalized, normalized, limit);
	}
	const body = Array.from(normalized, (char) => {
		if (char === "_") return "__";
		return /^[A-Za-z0-9]$/.test(char) ? char : `_${char.codePointAt(0).toString(16)}_`;
	}).join("");
	return hashedNamespace(`${marker}${body}`, body, limit);
}

function hashedNamespace(namespace: string, body: string, limit: number): string {
	if (namespace.length <= limit) return namespace;
	const digest = createHash("sha256").update(namespace, "utf8").digest("hex").slice(0, 16);
	const prefix = "_mcpns__h_";
	return `${prefix}${body.slice(0, limit - prefix.length - digest.length - 1)}_${digest}`;
}

// The servers and operations a tool name that does not name its server may
// reach: its own when it already has the mcp__<server>__<op> shape, else
// one set for the server its belt.json prefix maps it to and one for each
// guarded server whose display or legacy prefix it carries. Empty when it
// places itself nowhere.
function toolTargets(config: BeltPiConfig, name: string): McpTarget[] {
	const claude = claudeTarget(name);
	if (claude) return [claude];
	const targets: McpTarget[] = [];
	const server = directToolServer(config, name);
	if (server) targets.push(...serverTargets(config, server, name));
	for (const guarded of config.externalTextGuardServers) {
		const stripped = strippedOperations(guarded, name);
		if (stripped.length === 0) continue;
		for (const operation of [...stripped, name]) {
			targets.push({ server: guarded, operation });
		}
	}
	return uniqueTargets(targets);
}

// The operations a call to tool on server may reach: the server's own tool
// by that name, and the tool it names under a display or legacy server
// prefix. A belt.json prefix says the whole name is the tool's own
// (csl_search is csl's), so that reading comes first; otherwise the
// stripped one does.
function serverTargets(
	config: BeltPiConfig,
	server: string,
	tool: string,
): McpTarget[] {
	const claude = claudeTarget(tool);
	if (claude) return [claude];
	const stripped = strippedOperations(server, tool);
	const operations =
		directToolServer(config, tool) === server
			? [tool, ...stripped]
			: [...stripped, tool];
	return uniqueTargets(operations.map((operation) => ({ server, operation })));
}

// The server and operation of a Claude-style MCP name, null for any other.
// The operation is whatever follows the last "__".
function claudeTarget(name: string): McpTarget | null {
	const server = mcpServer(name);
	if (!server) return null;
	return { server, operation: name.slice(name.lastIndexOf("__") + 2) };
}

function uniqueTargets(targets: McpTarget[]): McpTarget[] {
	const unique = new Map<string, McpTarget>();
	for (const target of targets) {
		const key = `${target.server}\0${target.operation}`;
		if (!unique.has(key)) unique.set(key, target);
	}
	return [...unique.values()];
}

// name without each prefix pi-mcp-adapter may put in front of server's
// tools, so belt reads the server's own operation: create_gist, not
// gh_com_create_gist or gh_5f_com_create_gist. Empty when name carries none.
function strippedOperations(server: string, name: string): string[] {
	const operations = new Set<string>();
	for (const prefix of adapterPrefixes(server)) {
		if (name.startsWith(`${prefix}_`) && name.length > prefix.length + 1) {
			operations.add(name.slice(prefix.length + 1));
		}
	}
	return [...operations];
}

// The prefixes pi-mcp-adapter puts in front of a server's tool names,
// ported from its types.ts (getServerPrefix, getLegacyServerPrefix and
// getToolNameCandidates): the server, short and mcp modes, each in the
// current form, with "-" folded to "_", and in the legacy form that encodes
// "_" and "-" too.
function adapterPrefixes(server: string): string[] {
	const short = server.replace(/-?mcp$/i, "");
	const prefixes = new Set<string>();
	for (const legacy of [false, true]) {
		const full = encodeServerPrefix(server, legacy);
		for (const prefix of [full, encodeServerPrefix(short, legacy) || "mcp", `mcp__${full}`]) {
			prefixes.add(prefix);
			prefixes.add(prefix.replace(/-/g, "_"));
		}
	}
	return [...prefixes];
}

// The adapter's sanitizeServerPrefix: a character outside the allowed set
// becomes _<hex code point>_. The legacy form allows only letters and
// digits, so gh_com becomes gh_5f_com.
function encodeServerPrefix(server: string, legacy: boolean): string {
	const allowed = legacy ? /^[A-Za-z0-9]$/ : /^[A-Za-z0-9_-]$/;
	return Array.from(server, (char) =>
		allowed.test(char) ? char : `_${char.codePointAt(0).toString(16)}_`,
	).join("");
}

// Server segment of a Claude-style MCP name (mcp__<server>__<op>), split at
// the last "__" like claudeTarget so a server name may carry single
// underscores (gh_com). Empty for anything that is not an MCP name.
function mcpServer(permissionName: string): string {
	if (!permissionName.startsWith("mcp__")) return "";
	const body = permissionName.slice(5);
	const split = body.lastIndexOf("__");
	return split > 0 ? body.slice(0, split) : "";
}

// The arguments the MCP server receives from a proxy call: args, as an
// object or as a JSON string encoding one. Args the adapter would reject
// fall back to the whole input, so their text is still scanned.
function proxyArgs(input: Record<string, unknown>): Record<string, unknown> {
	let args = input.args;
	if (typeof args === "string") {
		try {
			args = JSON.parse(args);
		} catch {
			return input;
		}
	}
	if (args && typeof args === "object" && !Array.isArray(args)) {
		return args as Record<string, unknown>;
	}
	return input;
}

function ruleMatches(
	rule: ClaudeRule,
	toolName: string,
	input: Record<string, unknown>,
): boolean {
	const lower = toolName.toLowerCase();
	const kind = toolKind(toolName);

	if (rule.tool.startsWith("mcp__")) return mcpRuleMatches(rule, toolName);

	if (rule.tool === "Bash" && kind === "bash") {
		const command = (input.command as string) ?? "";
		return rule.spec === null || bashSpecMatches(rule.spec, command);
	}

	if (
		(rule.tool === "Read" && kind === "read") ||
		((rule.tool === "Write" || rule.tool === "Edit") && kind === "mutation")
	) {
		const path = (input.path as string) ?? "";
		if (rule.spec === null) return true;
		return globToRegex(rule.spec).test(expandPath(path));
	}

	// Bare tool-name entries (WebSearch, WebFetch, ...): spec-less match only;
	// WebFetch(domain:...) specs are not enforced here.
	if (rule.spec === null) return rule.tool.toLowerCase() === lower;
	return false;
}

function findMatch(
	rules: ClaudeRule[],
	toolName: string,
	input: Record<string, unknown>,
): ClaudeRule | null {
	for (const rule of rules) {
		if (ruleMatches(rule, toolName, input)) return rule;
	}
	return null;
}

// ── belt guards ─────────────────────────────────────────────────────────────

let beltAvailable: boolean | null = null;

function beltCheck(args: string[]): { denied: boolean; reason: string } {
	if (beltAvailable === false) {
		return { denied: true, reason: "belt is unavailable" };
	}
	try {
		execFileSync("belt", ["check", ...args], {
			encoding: "utf-8",
			timeout: 10000,
		});
		beltAvailable = true;
		return { denied: false, reason: "" };
	} catch (err: unknown) {
		const e = err as { status?: number; code?: string; stdout?: string };
		if (e.code === "ENOENT") {
			beltAvailable = false;
			return { denied: true, reason: "belt is unavailable" };
		}
		if (typeof e.status === "number" && e.status !== 0) {
			const out = (e.stdout ?? "").toString();
			const denyLine =
				out.split("\n").find((l: string) => l.startsWith("DENY")) ??
				"blocked by belt";
			return { denied: true, reason: denyLine.trim() };
		}
		return { denied: true, reason: "belt check failed" };
	}
}

function getGitRoot(filePath: string): string | null {
	try {
		return (
			execSync("git rev-parse --show-toplevel 2>/dev/null", {
				cwd: dirname(filePath),
				encoding: "utf-8",
				timeout: 500,
			}).trim() || null
		);
	} catch {
		return null;
	}
}

function configLine(settings: LoadedConfig): string {
	if (settings.error) return `belt pi config: ${settings.path} unreadable (${settings.error})`;
	if (settings.unknownKeys.length > 0) {
		return `belt pi config: ${settings.path} (unknown keys ignored: ${settings.unknownKeys.join(", ")})`;
	}
	return `belt pi config: ${settings.path}`;
}

// Set on globalThis while a gate runtime is live. pi loads nothing and says
// nothing for a declared package path that is missing, so the consuming repo
// ships its own small extension that blocks every tool call while this
// marker is absent. pi emits session_shutdown on the old runtime before
// a reload or a session switch starts the next one, so clearing it there and
// setting it again at load and session start leaves no stale marker behind
// a package that has since gone. Keep the key: it is that extension's
// contract.
const GATE_LOADED = Symbol.for("mad01.pi.belt-gate");

function markGateLoaded(live: boolean) {
	const root = globalThis as typeof globalThis & { [GATE_LOADED]?: true };
	if (live) root[GATE_LOADED] = true;
	else delete root[GATE_LOADED];
}

export default function (pi: ExtensionAPI) {
	markGateLoaded(true);
	let claude: ClaudePermissions = loadClaudePermissions();
	let settings: LoadedConfig = loadConfig();
	let rules: PermissionRule[] = [];
	let sessionAllowedTools = new Set<string>(); // tools allowed for this session without prompt
	let approvedEditRepos = new Set<string>(); // git repo roots where edits are auto-approved

	function persistRules() {
		pi.appendEntry("permission-rules", {
			rules,
			allowedTools: Array.from(sessionAllowedTools),
			approvedEditRepos: Array.from(approvedEditRepos),
		});
	}

	function restoreRules(ctx: ExtensionContext) {
		rules = [];
		sessionAllowedTools = new Set();
		approvedEditRepos = new Set();

		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type === "custom" && entry.customType === "permission-rules") {
				const data = entry.data as
					| {
							rules?: PermissionRule[];
							allowedTools?: string[];
							approvedEditRepos?: string[];
					  }
					| undefined;
				if (data?.rules) rules = data.rules;
				if (data?.allowedTools)
					sessionAllowedTools = new Set(data.allowedTools);
				if (data?.approvedEditRepos)
					approvedEditRepos = new Set(data.approvedEditRepos);
			}
		}
	}

	pi.on("session_start", async (_event, ctx) => {
		markGateLoaded(true);
		claude = loadClaudePermissions(); // re-read settings each session
		settings = loadConfig();
		restoreRules(ctx);
		// An unreadable file blocks, and a misspelt key leaves a guard narrower
		// than meant: say so at session start, not only in /permissions.
		if (ctx.hasUI && (settings.error || settings.unknownKeys.length > 0)) {
			ctx.ui.notify(configLine(settings), settings.error ? "error" : "warning");
		}
	});
	pi.on("session_tree", async (_event, ctx) => restoreRules(ctx));
	pi.on("session_shutdown", async () => markGateLoaded(false));

	function matchesPattern(command: string, pattern: string): boolean {
		if (pattern.endsWith(":*")) {
			return command.trimStart().startsWith(pattern.slice(0, -2));
		}
		return command.trimStart().startsWith(pattern);
	}

	// A belt denial blocks the call and is announced on the extension bus so a
	// mod (session-band) can count it; pi has no decision chain to read it from.
	const beltDenied = (
		kind: "bash" | "write" | "external-text",
		target: string,
		res: { reason: string },
	) => {
		pi.events?.emit?.("belt:deny", { kind, target, reason: res.reason });
		return { block: true, reason: res.reason };
	};

	// Main permission gate
	pi.on("tool_call", async (event, ctx) => {
		const toolName = event.toolName;
		const input = event.input as Record<string, unknown>;
		const kind = toolKind(toolName);

		// 0. A belt.json that is present but unreadable could be the one that
		// adds a guarded server, so nothing but a read runs until it is fixed.
		if (settings.error && kind !== "read") {
			return {
				block: true,
				reason: `${configLine(settings)}; fix or remove it, then reload with /permissions`,
			};
		}

		// Every name the call may run as; a tool that is not MCP has only its
		// own.
		const readings = mcpReadings(settings.config, toolName, input);
		const permissionNames =
			readings.length > 0 ? readings.map((reading) => reading.name) : [toolName];
		const permissionName = permissionNames[0];
		// Deny and ask rules also match the tool's own name, so a rule on a
		// proxy (mcp, mcp__linear) still covers every call through it. Allow
		// rules do not: an allowed proxy name must not allow every tool behind
		// it.
		const restrictedNames = [...new Set([...permissionNames, toolName])];
		const targets = kind === "mutation" ? mutationTargets(input) : [];
		const ruleInputs =
			targets.length > 0
				? targets.map((target) => ({ ...input, path: target.path }))
				: [input];
		// The first of rules that any deny or ask name matches for any input.
		const matchAny = (rules: ClaudeRule[]): ClaudeRule | null => {
			for (const name of restrictedNames) {
				for (const candidate of ruleInputs) {
					const rule = findMatch(rules, name, candidate);
					if (rule) return rule;
				}
			}
			return null;
		};

		// 1. belt guards (same guards as the Claude/Codex PreToolUse hooks).
		// Pi routes normal work through ctx_shell/ctx_edit/ctx_patch, so those
		// aliases must not become guard bypasses.
		if (kind === "bash" && typeof input.command === "string") {
			const res = beltCheck(["bash", input.command, "--cwd", ctx.cwd]);
			if (res.denied) return beltDenied("bash", input.command, res);
		}
		for (const target of targets) {
			if (!target.path) continue;
			const filePath = resolve(ctx.cwd, expandPath(target.path));
			const res = beltCheck([
				"write",
				"--file",
				filePath,
				"--content",
				target.content,
			]);
			if (res.denied) return beltDenied("write", filePath, res);
		}
		// MCP calls that publish text off this machine: belt scans the call's
		// string fields for internal names (publish-internal-names), whether
		// the model called mcp__<server>__<op> directly or went through a
		// proxy. belt resolves each call's owner/repo against its public_repos
		// list, so the default set stays at github.com and a rendering adds any
		// other server. Every guarded reading is checked. A proxy call whose
		// server stays unknown may still land on a guarded one, so it is
		// checked too, under the placeholder server `mcp`; belt keys on the
		// operation and the input, never the server, and passes reads by their
		// verb.
		for (const reading of readings) {
			const guarded =
				reading.unresolved ||
				settings.config.externalTextGuardServers.has(reading.server);
			if (!guarded) continue;
			const res = beltCheck([
				"external-text",
				"--tool",
				reading.name,
				"--input",
				JSON.stringify(reading.args),
			]);
			if (res.denied) return beltDenied("external-text", reading.name, res);
		}

		// 2. Claude settings: deny. A multi-file mutation is denied when any
		// target matches a deny rule, and an MCP call when any reading or its
		// own tool name does.
		const denied = matchAny(claude.deny);
		if (denied) {
			return {
				block: true,
				reason: `Denied by Claude settings: ${denied.tool}(${denied.spec ?? ""})`,
			};
		}

		const confirm = async (label: string, reason = "Requires confirmation") => {
			const short = label.length > 80 ? label.slice(0, 77) + "..." : label;
			const decision = await requestPermission(ctx, {
				action: permissionName,
				cwd: ctx.cwd,
				label: short,
				model: ctx.model ? `${ctx.model.provider}/${ctx.model.id}` : undefined,
				reason,
				tool: toolName,
			});
			return decision.allowed
				? undefined
				: { block: true, reason: decision.reason };
		};

		// pi-mcp-adapter's mcpScript runs the MCP calls in its script inside
		// the adapter, where no tool_call event fires, so neither belt nor the
		// Claude lists see them. Every script asks, whatever the allow list
		// says; headless with no broker that is a block.
		if (toolName === "mcpScript") {
			return confirm(
				`mcpScript: ${typeof input.code === "string" ? input.code : ""}`,
				"An MCP script runs its calls without belt checks and requires confirmation",
			);
		}

		const wantsEscalation = input.sandbox_permissions === "require_escalated";
		if (
			kind === "bash" &&
			wantsEscalation &&
			envIs(settings.config.sandboxAllowUnsandboxedEnv, "0")
		) {
			return {
				block: true,
				reason: "Unsandboxed commands are disabled by Pi sandbox settings",
			};
		}

		// Claude has a dedicated PreToolUse guard for closing PRs through the
		// broad update_pull_request tool. Keep that protection for both direct
		// MCP tools and pi-mcp-adapter's proxies, whose args may be a JSON
		// string.
		const closing = readings.find(
			(reading) =>
				reading.operation.endsWith("update_pull_request") &&
				reading.args.state === "closed",
		);
		if (closing) {
			return confirm(
				`${closing.operation}: close pull request`,
				"Closing a pull request requires confirmation",
			);
		}

		// 3. Claude settings: ask. Ask must precede allow: the settings contain
		// broad gh_* wildcards plus specific merge_pull_request ask rules.
		const asked = matchAny(claude.ask);
		if (asked) {
			const label =
				kind === "bash"
					? (input.command as string)
					: `${permissionName} ${targets
							.map((target) => target.path)
							.filter(Boolean)
							.join(", ")}`;
			const result = await confirm(
				label,
				wantsEscalation
					? "Tool call requests execution outside the macOS sandbox"
					: "Requires confirmation",
			);
			if (!result && wantsEscalation) markPermissionApproved(input);
			return result;
		}

		// If a proxy call omitted the server name, a Claude matcher cannot
		// identify which gh_* merge rule applies. Fail toward a prompt.
		const merge = readings.find((reading) =>
			reading.operation.endsWith("merge_pull_request"),
		);
		if (merge) {
			return confirm(merge.operation, "Merging a pull request requires confirmation");
		}

		// Sandboxed Bash is the default. Leaving Seatbelt always requires a
		// one-shot approval, even when the Bash command itself matches an allow
		// rule. The approval marker is a Symbol, so model-provided JSON cannot
		// forge it.
		if (kind === "bash" && wantsEscalation) {
			const result = await confirm(
				(input.command as string) ?? "Bash command",
				"Tool call requests execution outside the macOS sandbox",
			);
			if (!result) markPermissionApproved(input);
			return result;
		}

		// 4. Claude settings: allow. Every target in a multi-file mutation and
		// every reading of an MCP call must be covered; one allowed temp file
		// must not allow the whole batch.
		if (
			permissionNames.every((name) =>
				ruleInputs.every(
					(candidate) => findMatch(claude.allow, name, candidate) !== null,
				),
			)
		) {
			return undefined;
		}

		// 5. Defaults per tool tier
		if (kind === "read") return undefined;

		if (kind !== "bash" && sessionAllowedTools.has(toolName)) return undefined;

		if (kind === "bash") {
			const command = input.command as string;

			// Session rules from "always deny" answers
			for (const rule of rules) {
				if (rule.toolName === "bash" && matchesPattern(command, rule.pattern)) {
					if (rule.action === "deny") {
						return { block: true, reason: `Denied by rule: ${rule.pattern}` };
					}
					return undefined;
				}
			}

			// Unknown commands need no blanket prompt while a sandbox confines
			// bash. Hard denies, ask rules, and belt guards already ran above.
			if (envIs(settings.config.sandboxActiveEnv, "1")) return undefined;

			if (!ctx.hasUI) {
				return confirm(
					command,
					"Shell command requires approval because the sandbox is unavailable",
				);
			}

			const shortCmd =
				command.length > 80 ? command.slice(0, 77) + "..." : command;
			const ok = await ctx.ui.confirm(shortCmd, "Allow?");

			if (!ok) {
				const firstWord = command.trimStart().split(/\s+/)[0];
				const always = await ctx.ui.confirm(
					`Always deny "${firstWord}"?`,
					"Create rule?",
				);
				if (always) {
					const pattern = `${firstWord}:*`;
					rules.push({ pattern, toolName: "bash", action: "deny" });
					persistRules();
					ctx.ui.notify(`Rule: always deny "${pattern}"`, "success");
				}
				return { block: true, reason: "Denied by user" };
			}

			return undefined;
		}

		// Mutations: auto-allow in-project file edits (recoverable via git).
		if (kind === "mutation") {
			const resolvedTargets = targets.map((target) => ({
				...target,
				resolved: resolve(ctx.cwd, expandPath(target.path)),
			}));
			const autoAllowed =
				resolvedTargets.length > 0 &&
				resolvedTargets.every((target) => {
					if (!target.path) return false;
					const inProject =
						target.resolved.startsWith(ctx.cwd + "/") ||
						target.resolved === ctx.cwd;
					const rel = relative(ctx.cwd, target.resolved);
					const isProtected =
						rel === "CLAUDE.md" ||
						rel === "AGENTS.md" ||
						rel === ".pi" ||
						rel.startsWith(`.pi${sep}`) ||
						rel === ".claude" ||
						rel.startsWith(`.claude${sep}`);
					if (!isProtected) {
						for (const repo of approvedEditRepos) {
							if (
								target.resolved.startsWith(repo + "/") ||
								target.resolved === repo
							)
								return true;
						}
					}
					return inProject && !isProtected;
				});
			if (autoAllowed) return undefined;

			if (!ctx.hasUI) {
				return confirm(
					`${toolName}: ${targets.map((target) => target.path).join(", ")}`,
					"Out-of-project mutation requires approval",
				);
			}

			const shownPaths = targets
				.map((target) => target.path || "(unknown path)")
				.join(", ");
			const shortPath =
				shownPaths.length > 60 ? "..." + shownPaths.slice(-57) : shownPaths;
			const repoRoots = new Set<string>();
			for (const target of resolvedTargets) {
				const root = getGitRoot(target.resolved);
				if (root) repoRoots.add(root);
			}
			const repoRoot = repoRoots.size === 1 ? Array.from(repoRoots)[0] : null;
			const options = ["Allow this edit"];
			if (repoRoot) options.push("Allow all edits in this repo");
			options.push("Deny");

			const choice = await ctx.ui.select(`${toolName}: ${shortPath}`, options);

			if (choice?.startsWith("Allow all edits") && repoRoot) {
				approvedEditRepos.add(repoRoot);
				persistRules();
				const short = repoRoot.replace(expandPath("~/"), "~/");
				ctx.ui.notify(`Edits approved for repo: ${short}`, "success");
				return undefined;
			}

			if (choice === "Deny" || !choice) {
				return { block: true, reason: "Denied by user" };
			}

			return undefined;
		}

		// Everything else (MCP tools, web search, ...): allowed unless the
		// Claude settings said otherwise above.
		return undefined;
	});

	// /permissions command
	pi.registerCommand("permissions", {
		description: "View and manage permission rules",
		handler: async (_args, ctx) => {
			const lines: string[] = [];
			lines.push(
				`Claude settings: ${claude.allow.length} allow, ${claude.deny.length} deny, ${claude.ask.length} ask`,
			);
			lines.push(
				`belt guards: ${beltAvailable === false ? "unavailable" : "active"}`,
			);
			lines.push(configLine(settings));
			if (rules.length > 0) {
				lines.push("Persistent rules:");
				for (const r of rules) {
					lines.push(
						`  ${r.action === "allow" ? "✓" : "✗"} ${r.toolName}: ${r.pattern}`,
					);
				}
			}
			if (sessionAllowedTools.size > 0) {
				lines.push("Session-allowed tools:");
				for (const t of sessionAllowedTools) {
					lines.push(`  ✓ ${t}`);
				}
			}
			if (approvedEditRepos.size > 0) {
				lines.push("Approved edit repos:");
				for (const r of approvedEditRepos) {
					lines.push(`  📁 ${r}`);
				}
			}

			const choice = await ctx.ui.select(lines.join("\n"), [
				"Keep rules",
				"Clear session rules",
				"Reload settings",
			]);
			if (choice === "Clear session rules") {
				rules = [];
				sessionAllowedTools = new Set();
				approvedEditRepos = new Set();
				persistRules();
				ctx.ui.notify("Session permission rules cleared", "success");
			} else if (choice === "Reload settings") {
				claude = loadClaudePermissions();
				settings = loadConfig();
				ctx.ui.notify(
					`Reloaded: ${claude.allow.length} allow, ${claude.deny.length} deny, ${claude.ask.length} ask; ${configLine(settings)}`,
					settings.error ? "error" : "success",
				);
			}
		},
	});
}
