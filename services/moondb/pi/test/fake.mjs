import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'

// A moondb stand-in on PATH. Each run keeps its argv, its stdin and the
// entrypoint it was given under its pid, then appends the pid to order, so
// the calls read back in the order they finished. FAKE_MOONDB_SLEEP holds
// it before it reads, to see whether a caller waits.
const FAKE_MOONDB = `#!/bin/sh
[ -n "$FAKE_MOONDB_SLEEP" ] && sleep "$FAKE_MOONDB_SLEEP"
cat > "$FAKE_MOONDB_LOG/$$.json"
printf '%s\\n' "$@" > "$FAKE_MOONDB_LOG/$$.argv"
printf '%s' "$CLAUDE_CODE_ENTRYPOINT" > "$FAKE_MOONDB_LOG/$$.env"
echo "$$" >> "$FAKE_MOONDB_LOG/order"
`

const ENV_KEYS = ['PATH', 'FAKE_MOONDB_LOG', 'FAKE_MOONDB_SLEEP', 'CLAUDE_CODE_ENTRYPOINT']

// A scratch dir with the fake on PATH, restored afterwards. withMoondb:
// false leaves moondb off PATH entirely. repo: true makes dir/repo a git
// repository with one committed file, a.txt.
export function scratch(t, { withMoondb = true, repo = false } = {}) {
  const dir = realpathSync(mkdtempSync(join(tmpdir(), 'moondb-pi-test-')))
  const binDir = join(dir, 'bin')
  const log = join(dir, 'log')
  mkdirSync(binDir)
  mkdirSync(log)
  if (withMoondb) writeFileSync(join(binDir, 'moondb'), FAKE_MOONDB, { mode: 0o755 })
  const saved = Object.fromEntries(ENV_KEYS.map((key) => [key, process.env[key]]))
  process.env.PATH = withMoondb ? `${binDir}:${saved.PATH}` : `${binDir}:/usr/bin:/bin`
  process.env.FAKE_MOONDB_LOG = log
  delete process.env.FAKE_MOONDB_SLEEP
  // A test run from a Claude Code shell inherits its entrypoint.
  process.env.CLAUDE_CODE_ENTRYPOINT = 'cli'
  const repoDir = join(dir, 'repo')
  if (repo) {
    mkdirSync(repoDir)
    const git = (...args) =>
      execFileSync('git', ['-C', repoDir, '-c', 'user.email=t@example.com', '-c', 'user.name=t', ...args])
    git('init', '-q')
    writeFileSync(join(repoDir, 'a.txt'), 'one\n')
    git('add', 'a.txt')
    git('commit', '-qm', 'init')
  }
  t.after(() => {
    for (const [key, value] of Object.entries(saved)) {
      if (value === undefined) delete process.env[key]
      else process.env[key] = value
    }
    rmSync(dir, { recursive: true, force: true })
  })
  const calls = () => {
    const order = join(log, 'order')
    if (!existsSync(order)) return []
    return readFileSync(order, 'utf8')
      .trim()
      .split('\n')
      .map((pid) => ({
        argv: readFileSync(join(log, `${pid}.argv`), 'utf8').trimEnd().split('\n'),
        payload: JSON.parse(readFileSync(join(log, `${pid}.json`), 'utf8')),
        entrypoint: readFileSync(join(log, `${pid}.env`), 'utf8'),
      }))
  }
  return {
    dir,
    repoDir,
    calls,
    // waitCalls polls until n runs finished, for the records nobody waits on.
    async waitCalls(n) {
      for (let i = 0; i < 300 && calls().length < n; i++) await sleep(10)
      return calls()
    },
  }
}

// A pi stand-in: handlers by event. call() runs the first handler.
export function fakePi() {
  const handlers = new Map()
  const pi = {
    on(name, handler) {
      handlers.set(name, [...(handlers.get(name) ?? []), handler])
    },
  }
  const call = (name, event, ctx) => handlers.get(name)[0](event, ctx)
  return { pi, handlers, call }
}

// A print-mode context for session s1, whose transcript is dir/s1.jsonl.
export function printCtx(cwd, { name } = {}) {
  return {
    cwd,
    mode: 'print',
    hasUI: false,
    sessionManager: {
      getSessionId: () => 's1',
      getSessionFile: () => join(cwd, 's1.jsonl'),
      getSessionName: () => name,
    },
  }
}
