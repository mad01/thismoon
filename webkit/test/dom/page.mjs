// A page with the built kit loaded, for the DOM tests: a happy-dom window that
// runs dist/webkit.js the way a consumer's <script> does, so the custom
// elements upgrade and wire themselves as they would in a browser. Each test
// opens its own page, so the element registry, localStorage and history start
// clean, and the page closes when the test ends.
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { Window } from 'happy-dom';

const BUNDLE_URL = new URL('../../dist/webkit.js', import.meta.url);
const BUNDLE = new vm.Script(readFileSync(BUNDLE_URL, 'utf8'), { filename: BUNDLE_URL.pathname });

/**
 * openPage loads `body` into a fresh window, then the kit, so the elements in
 * it upgrade as a page's markup does when webkit.js arrives after it. `storage`
 * seeds localStorage before the kit reads it. `fetch(window, url, init)`
 * answers the page's requests in place of the default 404, and
 * `blockStorage` makes reading localStorage throw, as a browser that blocks
 * site data does.
 */
export function openPage(t, body = '', { url = 'http://kit.test/', storage = {}, fetch, blockStorage = false } = {}) {
  // A happy-dom window is a VM context, and the bundle runs in it as the
  // classic script it is, so its `var Webkit` lands on the window. The page's
  // own <script> evaluation stays off: nothing else runs.
  const window = new Window({ url });
  // The kit polls /webkit/version and loads the ⌘K site list; a test page
  // answers both with a 404 instead of reaching the network.
  window.fetch = fetch
    ? (input, init) => fetch(window, String(input), init ?? {})
    : async () => new window.Response('', { status: 404 });
  for (const [key, value] of Object.entries(storage)) window.localStorage.setItem(key, value);
  if (blockStorage) {
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      get() { throw new window.DOMException('site data is blocked', 'SecurityError'); },
    });
  }
  window.document.body.innerHTML = body;
  BUNDLE.runInContext(window);
  t.after(() => window.happyDOM.close());
  return window;
}

/** key dispatches a keydown for `key` on `el`, as a keyboard press does. */
export function key(window, el, key) {
  const e = new window.KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
  el.dispatchEvent(e);
  return e;
}

/** clicks counts the clicks `el` receives from now on. */
export function clicks(el) {
  const counter = { count: 0 };
  el.addEventListener('click', () => { counter.count++; });
  return counter;
}
