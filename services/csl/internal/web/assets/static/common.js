// Shared helpers for the csl pages (search, health, file view). Loaded before
// the page script.

// execCommandCopy copies via a temporary textarea + document.execCommand. The
// async Clipboard API is unavailable on the http://<name>.this origins this UI
// is served from (not a secure context), so this legacy path is the one that
// actually runs here. Returns whether the copy succeeded.
function execCommandCopy(text) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.focus();
  ta.select();
  let ok = false;
  try { ok = document.execCommand('copy'); } catch (_) { ok = false; }
  document.body.removeChild(ta);
  return ok;
}

// copyText writes text to the clipboard and briefly flashes the button label so
// the click registers as "done". It prefers the async Clipboard API and falls
// back to execCommand for non-secure-context origins; shows "failed" if both
// paths fail.
function copyText(text, btn) {
  const restore = btn.dataset.label || btn.textContent;
  btn.dataset.label = restore;
  const flash = label => {
    btn.textContent = label;
    setTimeout(() => { btn.textContent = restore; }, 1200);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(
      () => flash('copied'),
      () => flash(execCommandCopy(text) ? 'copied' : 'failed'));
  } else {
    flash(execCommandCopy(text) ? 'copied' : 'failed');
  }
}

// bindCopyButtons wires every .js-copy button under `root` to copy its
// data-copy payload to the clipboard.
function bindCopyButtons(root) {
  root.querySelectorAll('.js-copy').forEach(btn =>
    btn.addEventListener('click', () => copyText(btn.dataset.copy, btn)));
}

// timeAgo turns an ISO timestamp into a short "5m ago" string. Empty input
// yields an empty string.
function timeAgo(iso) {
  if (!iso) return '';
  const secs = Math.max(0, Math.floor((Date.now() - Date.parse(iso)) / 1000));
  if (secs < 60) return secs + 's ago';
  if (secs < 3600) return Math.floor(secs / 60) + 'm ago';
  if (secs < 86400) return Math.floor(secs / 3600) + 'h ago';
  return Math.floor(secs / 86400) + 'd ago';
}

// TABLE_PAGE_SIZE caps how many table rows the health and refresh views render
// at once, so a fleet with many repos does not paint thousands of <tr> nodes in
// one go.
const TABLE_PAGE_SIZE = 200;

// renderPagedTable renders `rows` into `container` as a wk-table, showing at
// most state.shown rows and appending a "Load more" control that reveals the
// next page in place (no refetch). opts.headHtml is the <thead> markup,
// opts.rowFn maps one item to a <tr> string, opts.onRender (optional) runs after
// each paint (e.g. to bind buttons), opts.pageSize (optional) overrides the page
// size, and opts.state (optional) is a { shown } object the caller can persist
// across calls so a re-render — like the refresh page's poll — keeps the reader's
// expanded page instead of collapsing back to the first one.
function renderPagedTable(container, rows, opts) {
  const pageSize = opts.pageSize || TABLE_PAGE_SIZE;
  const state = opts.state || {};
  if (!state.shown || state.shown < pageSize) state.shown = pageSize;
  const onRender = opts.onRender || function () {};

  function paint() {
    const slice = rows.slice(0, state.shown);
    const hidden = rows.length - slice.length;
    let html = '<wk-table><table>' + opts.headHtml +
      '<tbody>' + slice.map(opts.rowFn).join('') + '</tbody></table></wk-table>';
    if (hidden > 0) {
      const next = Math.min(pageSize, hidden);
      html += '<button class="load-more js-load-more">Load ' + next +
        ' more (' + hidden + ' hidden)</button>';
    }
    container.innerHTML = html;
    const more = container.querySelector('.js-load-more');
    if (more) more.addEventListener('click', () => { state.shown += pageSize; paint(); });
    onRender(container);
  }
  paint();
}
