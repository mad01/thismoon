// size.ts — pure font-size clamp (no DOM), unit-testable from node.

export const SIZE_MIN     = 12;
export const SIZE_MAX     = 24;
export const SIZE_STEP    = 1;
export const SIZE_DEFAULT = 16;

/** Clamps a px font-size into the supported [SIZE_MIN, SIZE_MAX] range. */
export function clampSize(px: number): number {
  return Math.max(SIZE_MIN, Math.min(SIZE_MAX, px));
}
