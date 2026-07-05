// render.ts — small, opt-in client render helpers shared by the CSR consumers.
// Extracted from the hand-rolled copies that csl/catalog (escapeHtml, el) and
// events (poll) each carry, so tools stop reinventing them. Re-exported from
// webkit.ts, so they land on the global as Webkit.escapeHtml / Webkit.el /
// Webkit.poll. Deliberately tiny and vanilla — webkit ships chrome, components,
// and these primitives, not a rendering framework.

// The five HTML-significant characters, mapped to their entities. A typed
// Record so escapeHtml can be indexed by the regex match without a cast.
const HTML_ENTITIES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

// escapeHtml escapes a value for safe interpolation into innerHTML. Verbatim in
// behaviour with the identical csl/catalog implementations.
export function escapeHtml(s: unknown): string {
  return String(s).replace(/[&<>"']/g, (c) => HTML_ENTITIES[c]);
}

export type ElChild = string | Node | null | undefined;
export type ElAttrs = Record<string, unknown>;

// el builds a DOM node from a tag, attributes, and children. Attribute keys are
// interpreted as: `class` -> className; `html` -> innerHTML (caller-trusted);
// `on*` whose value is a function -> addEventListener(name without "on"); any
// other non-nullish value -> setAttribute. Children are appended in order;
// string children become text nodes, so untrusted data is XSS-safe by
// construction. Ported from catalog's helper.
export function el(tag: string, attrs: ElAttrs = {}, children: ElChild | ElChild[] = []): HTMLElement {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') node.className = String(v);
    else if (k === 'html') node.innerHTML = String(v);
    else if (k.startsWith('on') && typeof v === 'function') {
      node.addEventListener(k.slice(2), v as EventListener);
    } else if (v !== null && v !== undefined) {
      node.setAttribute(k, String(v));
    }
  }
  for (const c of ([] as ElChild[]).concat(children)) {
    if (c == null) continue;
    node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return node;
}

// PollHandle is returned by poll; call stop() to end the loop.
export interface PollHandle {
  stop(): void;
}

// poll fetches JSON from url and hands it to onData, immediately and then every
// ms. Fetch/parse errors go to onError (if given) and never break the loop, so
// a transient backend hiccup doesn't freeze the page. Returns a handle whose
// stop() clears the timer and suppresses any in-flight callback. This replaces
// the consumers' setTimeout(location.reload) loops with re-fetch-and-re-render.
export function poll<T = unknown>(
  url: string,
  onData: (data: T) => void,
  ms: number,
  onError?: (err: unknown) => void,
): PollHandle {
  let stopped = false;
  async function tick(): Promise<void> {
    try {
      const res = await fetch(url, { headers: { accept: 'application/json' } });
      if (!res.ok) throw new Error('request failed (' + res.status + ')');
      const data = (await res.json()) as T;
      if (!stopped) onData(data);
    } catch (err) {
      if (!stopped && onError) onError(err);
    }
  }
  void tick();
  const timer = window.setInterval(() => void tick(), ms);
  return {
    stop(): void {
      stopped = true;
      window.clearInterval(timer);
    },
  };
}
