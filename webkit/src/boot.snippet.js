// Single source of truth for the pre-paint FOUC-guard boot snippet.
//
// Consumed two ways so the served file and the JS export can never drift:
//   - src/webkit.ts re-exports it as `bootSnippet` (back-compat for inliners)
//   - build.mjs writes it verbatim to dist/boot.js, served at GET /webkit/boot.js
//     and injected via the Go helper webkit.BootScript().
//
// Plain runnable IIFE (no module syntax) so dist/boot.js works as a classic
// blocking <script src> in <head>, before any paint and before webkit.js.
// Mirrors THEME_KEY / SIZE_KEY and the clampSize bounds in src/webkit.ts.
export const bootSnippet =
  "(function(){try{" +
  "var t=localStorage.getItem('webkit-theme')||'light';" +
  "document.documentElement.setAttribute('data-theme',t);" +
  "var s=parseInt(localStorage.getItem('webkit-size'),10);" +
  "if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}" +
  "}catch(e){}})();";
