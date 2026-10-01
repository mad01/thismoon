// Build-time constants esbuild injects through `define` in build.mjs.

/**
 * The family names of the theme collection (src/themes/*.json) as a JSON
 * array string, in picker order with default first. The bundle validates a
 * stored palette name against it without a fetch; build.mjs and the node
 * tests pass the list explicitly instead.
 */
declare const WEBKIT_FAMILIES: string;
