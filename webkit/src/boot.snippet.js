// Single source of truth for the pre-paint FOUC-guard boot snippet.
//
// Consumed two ways so the served file and the JS export can never drift:
//   - src/webkit.ts exports `bootSnippet` = bootSnippetFor(the built-in list)
//   - build.mjs writes bootSnippetFor(the collection's names) to dist/boot.js,
//     served at GET /webkit/boot.js and injected via webkit.BootScript().
//
// The snippet is a plain runnable IIFE (no module syntax) so dist/boot.js
// works as a classic blocking <script src> in <head>, before any paint and
// before webkit.js. It mirrors THEME_KEY / PALETTE_KEY_PREFIX / SIZE_KEY, the
// clampSize bounds, and the mode and family resolution of applyTheme in
// src/webkit.ts: `system` follows prefers-color-scheme, and the family key
// for the resolved mode becomes data-palette when it names a family in the
// list (absent for the default family or an unknown name).

/** The boot snippet for a collection: `families` are the family names it may set as data-palette. */
export function bootSnippetFor(families) {
  const list = JSON.stringify(Array.from(families));
  return (
    "(function(){try{" +
    "var t=localStorage.getItem('webkit-theme')||'light';" +
    "if(t==='system'){t=window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';}" +
    "if(t!=='dark'){t='light';}" +
    "document.documentElement.setAttribute('data-theme',t);" +
    "var p=localStorage.getItem('webkit-palette-'+t);" +
    "if(p&&p!=='default'&&" + list + ".indexOf(p)!==-1){document.documentElement.setAttribute('data-palette',p);}" +
    "var s=parseInt(localStorage.getItem('webkit-size'),10);" +
    "if(!isNaN(s)){s=Math.max(12,Math.min(24,s));document.documentElement.style.fontSize=s+'px';}" +
    "}catch(e){}})();"
  );
}
