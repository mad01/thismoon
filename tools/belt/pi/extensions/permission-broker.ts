// @ts-nocheck -- Pi resolves extension and Node types from its global runtime.
/**
 * Permission Broker Extension
 *
 * Interactive Pi sessions host a private Unix socket. Headless child Pi
 * processes use it to ask the root TUI for one-shot permission without
 * exposing the broker credential to commands executed by the bash tool.
 */

import { randomBytes, timingSafeEqual } from "node:crypto";
import { chmodSync, mkdtempSync, rmSync } from "node:fs";
import { createServer, createConnection } from "node:net";
import { join } from "node:path";
import type {
	ExtensionAPI,
	ExtensionContext,
} from "@earendil-works/pi-coding-agent";

const SOCKET_ENV = "PI_PERMISSION_BROKER_SOCKET";
const TOKEN_ENV = "PI_PERMISSION_BROKER_TOKEN";
const APPROVED = Symbol.for("mad01.pi.permission-approved");
const STATE = Symbol.for("mad01.pi.permission-broker-state");
const MAX_MESSAGE_BYTES = 64 * 1024;
const REQUEST_TIMEOUT_MS = 10 * 60 * 1000;

export interface PermissionRequest {
	action: string;
	cwd: string;
	label: string;
	model?: string;
	reason: string;
	tool: string;
}

export interface PermissionDecision {
	allowed: boolean;
	reason: string;
}

interface BrokerState {
	context?: ExtensionContext;
	directory?: string;
	queue: Promise<void>;
	server?: ReturnType<typeof createServer>;
	socket?: string;
	token?: string;
}

function brokerState(): BrokerState {
	const root = globalThis as typeof globalThis & { [STATE]?: BrokerState };
	root[STATE] ??= { queue: Promise.resolve() };
	return root[STATE];
}

function clipped(value: unknown, maximum = 4096): string {
	return String(value ?? "").slice(0, maximum);
}

function normalizeRequest(value: unknown): PermissionRequest | null {
	if (!value || typeof value !== "object") return null;
	const raw = value as Record<string, unknown>;
	for (const field of ["action", "cwd", "label", "reason", "tool"]) {
		if (typeof raw[field] !== "string" || !raw[field]) return null;
	}
	return {
		action: clipped(raw.action, 256),
		cwd: clipped(raw.cwd),
		label: clipped(raw.label),
		model: raw.model ? clipped(raw.model, 256) : undefined,
		reason: clipped(raw.reason, 1024),
		tool: clipped(raw.tool, 256),
	};
}

function tokenMatches(expected: string, actual: unknown): boolean {
	if (typeof actual !== "string") return false;
	const left = Buffer.from(expected);
	const right = Buffer.from(actual);
	return left.length === right.length && timingSafeEqual(left, right);
}

async function askInRoot(
	ctx: ExtensionContext,
	request: PermissionRequest,
): Promise<PermissionDecision> {
	const origin = request.model ? `Subagent ${request.model}` : "Subagent";
	const title = `${origin}: ${request.action}`.slice(0, 120);
	const message = `${request.reason}\n\n${request.label}\n\nWorking directory: ${request.cwd}`;
	const allowed = await ctx.ui.confirm(title, `${message}\n\nAllow once?`);
	return {
		allowed,
		reason: allowed ? "Approved by user" : "Denied by user",
	};
}

function sendDecision(socket: ReturnType<typeof createConnection>, decision: PermissionDecision) {
	socket.end(`${JSON.stringify(decision)}\n`);
}

function handleConnection(socket: ReturnType<typeof createConnection>, state: BrokerState) {
	let buffer = "";
	let handled = false;
	socket.setEncoding("utf8");
	socket.on("data", (chunk) => {
		if (handled) return;
		buffer += chunk;
		if (Buffer.byteLength(buffer) > MAX_MESSAGE_BYTES) {
			handled = true;
			sendDecision(socket, { allowed: false, reason: "Permission request is too large" });
			return;
		}
		const newline = buffer.indexOf("\n");
		if (newline < 0) return;
		handled = true;
		const line = buffer.slice(0, newline);
		buffer = "";
		let envelope: Record<string, unknown>;
		try {
			envelope = JSON.parse(line);
		} catch {
			sendDecision(socket, { allowed: false, reason: "Invalid permission request" });
			return;
		}
		const request = normalizeRequest(envelope.request);
		if (!state.token || !tokenMatches(state.token, envelope.token) || !request) {
			sendDecision(socket, { allowed: false, reason: "Permission broker authentication failed" });
			return;
		}
		const ask = async () => {
			if (!state.context) {
				sendDecision(socket, { allowed: false, reason: "Root permission UI is unavailable" });
				return;
			}
			try {
				sendDecision(socket, await askInRoot(state.context, request));
			} catch (error) {
				sendDecision(socket, {
					allowed: false,
					reason: error instanceof Error ? error.message : "Permission prompt failed",
				});
			}
		};
		state.queue = state.queue.then(ask, ask);
	});
}

async function startBroker(ctx: ExtensionContext): Promise<void> {
	const state = brokerState();
	if (state.server || process.env[SOCKET_ENV]) return;
	state.context = ctx;
	state.directory = mkdtempSync("/tmp/pi-permissions-");
	chmodSync(state.directory, 0o700);
	state.socket = join(state.directory, "broker.sock");
	state.token = randomBytes(32).toString("hex");
	state.server = createServer((socket) => handleConnection(socket, state));
	await new Promise<void>((resolve, reject) => {
		state.server!.once("error", reject);
		state.server!.listen(state.socket, resolve);
	});
	process.env[SOCKET_ENV] = state.socket;
	process.env[TOKEN_ENV] = state.token;
}

function stopBroker(): void {
	const state = brokerState();
	state.context = undefined;
	state.server?.close();
	state.server = undefined;
	if (process.env[SOCKET_ENV] === state.socket) delete process.env[SOCKET_ENV];
	if (process.env[TOKEN_ENV] === state.token) delete process.env[TOKEN_ENV];
	if (state.directory?.startsWith("/tmp/pi-permissions-")) {
		rmSync(state.directory, { recursive: true, force: true });
	}
	state.directory = undefined;
	state.socket = undefined;
	state.token = undefined;
}

async function askParent(request: PermissionRequest): Promise<PermissionDecision> {
	const socketPath = process.env[SOCKET_ENV];
	const token = process.env[TOKEN_ENV];
	if (!socketPath || !token) {
		return { allowed: false, reason: "No parent permission broker is available" };
	}
	return new Promise((resolve) => {
		const socket = createConnection(socketPath);
		let buffer = "";
		let settled = false;
		const finish = (decision: PermissionDecision) => {
			if (settled) return;
			settled = true;
			clearTimeout(timeout);
			socket.destroy();
			resolve(decision);
		};
		const timeout = setTimeout(
			() => finish({ allowed: false, reason: "Parent permission request timed out" }),
			REQUEST_TIMEOUT_MS,
		);
		socket.setEncoding("utf8");
		socket.on("connect", () => {
			socket.write(`${JSON.stringify({ token, request })}\n`);
		});
		socket.on("data", (chunk) => {
			buffer += chunk;
			const newline = buffer.indexOf("\n");
			if (newline < 0) return;
			try {
				const decision = JSON.parse(buffer.slice(0, newline));
				finish({ allowed: decision.allowed === true, reason: clipped(decision.reason, 1024) });
			} catch {
				finish({ allowed: false, reason: "Invalid response from permission broker" });
			}
		});
		socket.on("error", (error) => finish({ allowed: false, reason: error.message }));
	});
}

export async function requestPermission(
	ctx: ExtensionContext,
	request: PermissionRequest,
): Promise<PermissionDecision> {
	if (ctx.hasUI) return askInRoot(ctx, { ...request, model: undefined });
	return askParent(request);
}

export function markPermissionApproved(input: Record<string | symbol, unknown>): void {
	input[APPROVED] = true;
}

export function permissionWasApproved(input: Record<string | symbol, unknown>): boolean {
	return input[APPROVED] === true;
}

export function stripBrokerCredentials(env: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
	const sanitized = { ...env };
	delete sanitized[SOCKET_ENV];
	delete sanitized[TOKEN_ENV];
	return sanitized;
}

export default function permissionBroker(pi: ExtensionAPI) {
	pi.on("session_start", async (_event, ctx) => {
		if (ctx.hasUI && !process.env[SOCKET_ENV]) await startBroker(ctx);
	});
	pi.on("session_shutdown", async () => {
		if (brokerState().server) stopBroker();
	});
}
