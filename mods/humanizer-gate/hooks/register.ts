import type { Register } from 'claude-code'

// A stub: the mod is reserved in the marketplace and registers one debug
// line so a session can tell it loaded. Nothing else runs here yet.
export const register: Register = on => {
  on('session.start', ($, e, next) => {
    $.ui.log('humanizer-gate: stub loaded', { to: 'debug' })
    return next(e)
  })
}
