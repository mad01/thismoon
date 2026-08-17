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
