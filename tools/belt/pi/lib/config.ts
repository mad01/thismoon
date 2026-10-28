// @ts-nocheck -- pi resolves extension and Node types from its global runtime.
/**
 * The belt pi package's own settings: `belt.json` in pi's agent directory
 * (`~/.pi/agent`, moved by `PI_CODING_AGENT_DIR`), beside the other
 * machine-local files pi reads. Every key is optional and adds to or
 * overrides the defaults below: the server lists only grow, while an
 * `mcpPrefixes` entry can remap a default prefix. The defaults name this
 * repo's own MCP servers and github.com and nothing else; a machine-specific
 * server or sandbox comes from the consuming repo's rendering of the file.
 *
 * A missing file means the defaults. A file that is present and cannot be
 * read, a dangling symlink included, comes back with `error` set: the
 * permission gate then blocks every call that is not a read, the same rule
 * belt applies to its own config (it cannot tell "nothing configured" from
 * "the settings did not load").
 */

import { readFileSync, readlinkSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

export interface BeltPiConfig {
	// pi-mcp-adapter's direct tools drop the server from the name
	// ("csl_search"); each prefix maps such a tool back to its server.
	mcpPrefixes: Array<[prefix: string, server: string]>;
	// MCP servers whose calls run `belt check external-text` before they go out.
	externalTextGuardServers: Set<string>;
	// MCP servers whose results run `belt hint external-text`.
	externalTextHintServers: Set<string>;
	// Env var a sandbox extension sets to "1" while bash runs confined; ""
	// means no sandbox, so unknown shell commands always confirm.
	sandboxActiveEnv: string;
	// Env var a sandbox extension sets to "0" when it refuses unsandboxed
	// retries; "" means escalations confirm instead of failing.
	sandboxAllowUnsandboxedEnv: string;
}

export interface LoadedConfig {
	config: BeltPiConfig;
	path: string;
	error: string; // "" when the file is absent or valid
	unknownKeys: string[]; // top-level keys the package ignored, for display
}

export const CONFIG_FILE = "belt.json";

const DEFAULT_MCP_PREFIXES: Record<string, string> = {
	csl_: "csl",
	humanizer_: "humanizer",
	worklog_: "worklog",
};
const DEFAULT_GUARD_SERVERS = ["gh_com"];
const DEFAULT_HINT_SERVERS = ["gh_com", "linear"];

export function configPath(env: NodeJS.ProcessEnv = process.env): string {
	const agentDir = env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent");
	return join(agentDir, CONFIG_FILE);
}

export function defaultConfig(): BeltPiConfig {
	return parseConfig({});
}

export function loadConfig(path: string = configPath()): LoadedConfig {
	const defaults = (error: string): LoadedConfig => ({ config: defaultConfig(), path, error, unknownKeys: [] });
	let raw: string;
	try {
		raw = readFileSync(path, "utf-8");
	} catch (err: unknown) {
		if ((err as NodeJS.ErrnoException).code !== "ENOENT") return defaults(String((err as Error).message));
		// A symlink whose target is gone is present, not absent: a rendering
		// deployed as a link must not fall back to the defaults unnoticed.
		const target = linkTarget(path);
		return defaults(target === "" ? "" : `symlink to missing ${target}`);
	}
	try {
		const value = JSON.parse(raw);
		return { config: parseConfig(value), path, error: "", unknownKeys: unknownKeys(value) };
	} catch (err: unknown) {
		return defaults(String((err as Error).message));
	}
}

// Validates the shape of each known key and ignores unknown ones, so a
// rendering written for a newer package still loads; loadConfig reports
// them so a misspelt key is visible.
export function parseConfig(value: unknown): BeltPiConfig {
	if (!isObject(value)) throw new Error("top level must be a JSON object");
	const prefixes = { ...DEFAULT_MCP_PREFIXES, ...stringMap(value, "mcpPrefixes") };
	return {
		mcpPrefixes: Object.entries(prefixes),
		externalTextGuardServers: new Set([
			...DEFAULT_GUARD_SERVERS,
			...stringList(value, "externalTextGuardServers"),
		]),
		externalTextHintServers: new Set([
			...DEFAULT_HINT_SERVERS,
			...stringList(value, "externalTextHintServers"),
		]),
		sandboxActiveEnv: stringValue(value, "sandboxActiveEnv"),
		sandboxAllowUnsandboxedEnv: stringValue(value, "sandboxAllowUnsandboxedEnv"),
	};
}

// The server a direct pi-mcp-adapter tool belongs to, "" for any other name.
export function directToolServer(config: BeltPiConfig, toolName: string): string {
	for (const [prefix, server] of config.mcpPrefixes) {
		if (toolName.startsWith(prefix)) return server;
	}
	return "";
}

// True when the named env var holds the value; an unnamed var never matches.
export function envIs(name: string, value: string): boolean {
	return name !== "" && process.env[name] === value;
}

// The keys of a valid file that are not belt.json keys, which are
// BeltPiConfig's field names.
function unknownKeys(value: Record<string, unknown>): string[] {
	const known = defaultConfig();
	return Object.keys(value).filter((key) => !Object.hasOwn(known, key));
}

// Where a symlink at path points, "" when path is not a symlink.
function linkTarget(path: string): string {
	try {
		return readlinkSync(path);
	} catch {
		return "";
	}
}

function isObject(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringMap(raw: Record<string, unknown>, key: string): Record<string, string> {
	const value = raw[key];
	if (value === undefined) return {};
	if (!isObject(value)) throw new Error(`${key} must be an object of strings`);
	for (const [name, server] of Object.entries(value)) {
		if (!name || typeof server !== "string" || !server) {
			throw new Error(`${key} must map non-empty prefixes to non-empty server names`);
		}
	}
	return value as Record<string, string>;
}

function stringList(raw: Record<string, unknown>, key: string): string[] {
	const value = raw[key];
	if (value === undefined) return [];
	if (!Array.isArray(value) || value.some((entry) => typeof entry !== "string" || !entry)) {
		throw new Error(`${key} must be a list of non-empty strings`);
	}
	return value;
}

function stringValue(raw: Record<string, unknown>, key: string): string {
	const value = raw[key];
	if (value === undefined) return "";
	if (typeof value !== "string") throw new Error(`${key} must be a string`);
	return value;
}
