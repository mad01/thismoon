// @ts-nocheck -- pi resolves extension types from its runtime; Claude Code never loads this file.
/**
 * session-band, pi face. The Claude face (../hooks/register.tsx) counts belt
 * denials by reading each PreToolUse decision as it comes back up the hook
 * chain. pi has no chain to read: the permission gate (the dotfiles pi
 * recipe's permission-gate.ts) blocks the call and announces it on the
 * extension bus as `belt:deny`. This face counts those announcements, keeps
 * the count in the session so a reload or a branch switch restores it, and
 * pins the same one-line status.
 */
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

import { composeStatusText } from "../lib/band.ts";

const STATUS_KEY = "session-band";
const ENTRY_TYPE = "session-band";

export default function (pi: ExtensionAPI) {
	let beltDenies = 0;
	let current: ExtensionContext | null = null;

	const refresh = (): void => {
		if (!current?.hasUI) return;
		current.ui.setStatus(STATUS_KEY, composeStatusText(beltDenies));
	};

	// The count follows the active branch: the last session-band entry on it
	// wins, and a branch without one starts from zero.
	const restore = (ctx: ExtensionContext): void => {
		beltDenies = 0;
		for (const entry of ctx.sessionManager.getBranch()) {
			if (entry.type !== "custom" || entry.customType !== ENTRY_TYPE) continue;
			const count = (entry.data as { beltDenies?: unknown } | undefined)?.beltDenies;
			if (typeof count === "number") beltDenies = count;
		}
	};

	pi.on("session_start", async (_event, ctx) => {
		current = ctx;
		restore(ctx);
		refresh();
	});
	pi.on("session_tree", async (_event, ctx) => {
		current = ctx;
		restore(ctx);
		refresh();
	});

	pi.events.on("belt:deny", () => {
		beltDenies += 1;
		pi.appendEntry(ENTRY_TYPE, { beltDenies });
		refresh();
	});
}
