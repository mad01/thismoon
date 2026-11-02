import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'

// A belt stand-in on PATH. `check` records its argv and denies the
// subcommands FAKE_BELT_DENY lists (comma separated) with a DENY line, the
// way belt reports a guard verdict. `hint` records argv and stdin and
// answers with additionalContext naming the event. `record` runs in the
// background, so it keeps its own log per event, after sleeping
// FAKE_BELT_RECORD_DELAY seconds when set; the argv file lands last, so its
// presence means the run finished.
const FAKE_BELT = `#!/bin/sh
if [ "$1" = record ]; then
  [ -n "$FAKE_BELT_RECORD_DELAY" ] && sleep "$FAKE_BELT_RECORD_DELAY"
  out="$FAKE_BELT_LOG.record-$2"
  cat > "$out.stdin"
  printf '%s\\n' "$@" > "$out.tmp" && mv "$out.tmp" "$out.argv"
  exit 0
fi
printf '%s\\n' "$@" > "$FAKE_BELT_LOG"
if [ "$1" = hint ]; then
  cat > "$FAKE_BELT_LOG.stdin"
  printf '%s' '{"hookSpecificOutput":{"additionalContext":"belt[fake]: advice for '"$2"'"}}'
  exit 0
fi
case ",$FAKE_BELT_DENY," in
  *",$2,"*) echo "DENY  fake-$2  denied by the fake"; exit 1 ;;
esac
exit 0
`

const ENV_KEYS = [
  'HOME',
  'PATH',
  'PI_CODING_AGENT_DIR',
  'PI_PERMISSION_BROKER_SOCKET',
  'PI_PERMISSION_BROKER_TOKEN',
  'FAKE_BELT_LOG',
  'FAKE_BELT_DENY',
  'FAKE_BELT_RECORD_DELAY',
]

// A scratch HOME and pi agent dir for one test, restored afterwards. The
// gate reads ~/.claude/settings.json from HOME and belt.json from the agent
// dir, so nothing outside the temp dir is read. withBelt: false leaves belt
// off PATH entirely.
export function scratch(t, { withBelt = true } = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'belt-pi-test-'))
  const agentDir = join(dir, 'agent')
  const binDir = join(dir, 'bin')
  mkdirSync(agentDir)
  mkdirSync(binDir)
  if (withBelt) writeFileSync(join(binDir, 'belt'), FAKE_BELT, { mode: 0o755 })
  const saved = Object.fromEntries(ENV_KEYS.map((key) => [key, process.env[key]]))
  const log = join(dir, 'argv')
  process.env.HOME = dir
  process.env.PI_CODING_AGENT_DIR = agentDir
  process.env.PATH = withBelt ? `${binDir}:${saved.PATH}` : binDir
  process.env.FAKE_BELT_LOG = log
  delete process.env.FAKE_BELT_DENY
  delete process.env.FAKE_BELT_RECORD_DELAY
  // A test run inside a pi session must not reach that session's broker.
  delete process.env.PI_PERMISSION_BROKER_SOCKET
  delete process.env.PI_PERMISSION_BROKER_TOKEN
  t.after(() => {
    for (const [key, value] of Object.entries(saved)) {
      if (value === undefined) delete process.env[key]
      else process.env[key] = value
    }
    rmSync(dir, { recursive: true, force: true })
  })
  return {
    dir,
    agentDir,
    // The argv of the last belt run, or null when belt never ran.
    argv: () => (existsSync(log) ? readFileSync(log, 'utf8').trimEnd().split('\n') : null),
    stdin: () => JSON.parse(readFileSync(`${log}.stdin`, 'utf8')),
    clearLog: () => rmSync(log, { force: true }),
    // The argv and stdin of the `belt record <event>` run once it finished,
    // or null when none finished within timeoutMs.
    record: async (event, timeoutMs = 5000) => {
      const out = `${log}.record-${event}`
      for (const start = Date.now(); Date.now() - start < timeoutMs; await sleep(20)) {
        if (!existsSync(`${out}.argv`)) continue
        return {
          argv: readFileSync(`${out}.argv`, 'utf8').trimEnd().split('\n'),
          stdin: JSON.parse(readFileSync(`${out}.stdin`, 'utf8')),
        }
      }
      return null
    },
    // Forgets every `belt record` run so far, so the next record() waits for
    // a new one.
    clearRecords: () => {
      for (const name of readdirSync(dir)) {
        if (name.startsWith('argv.record-')) rmSync(join(dir, name), { force: true })
      }
    },
    // The events a `belt record` run has finished for so far.
    recorded: () =>
      readdirSync(dir)
        .filter((name) => name.startsWith('argv.record-') && name.endsWith('.argv'))
        .map((name) => name.slice('argv.record-'.length, -'.argv'.length))
        .sort(),
    writeConfig: (value) =>
      writeFileSync(join(agentDir, 'belt.json'), typeof value === 'string' ? value : JSON.stringify(value)),
    claudeSettings: (value) => {
      mkdirSync(join(dir, '.claude'), { recursive: true })
      writeFileSync(join(dir, '.claude', 'settings.json'), JSON.stringify(value))
    },
  }
}

// A pi stand-in: handlers by event, the bus events emitted, the entries
// appended and the commands registered. The bus delivers each emit to the
// listeners registered for it. call() runs the first handler, callAll()
// every handler in registration order, as pi does when two extensions share
// an event.
export function fakePi() {
  const handlers = new Map()
  const listeners = new Map()
  const emitted = []
  const pi = {
    on(name, handler) {
      handlers.set(name, [...(handlers.get(name) ?? []), handler])
    },
    events: {
      on(name, handler) {
        listeners.set(name, [...(listeners.get(name) ?? []), handler])
        return () => listeners.set(name, listeners.get(name).filter((h) => h !== handler))
      },
      emit(name, data) {
        emitted.push([name, data])
        for (const handler of listeners.get(name) ?? []) handler(data)
      },
    },
    appendEntry() {},
    registerCommand() {},
  }
  const call = (name, event, ctx) => handlers.get(name)[0](event, ctx)
  const callAll = async (name, event, ctx) => {
    for (const handler of handlers.get(name) ?? []) await handler(event, ctx)
  }
  return { pi, handlers, listeners, emitted, call, callAll }
}

// A headless context: no UI, so every confirmation goes to the broker, and
// with no broker in the environment it fails closed.
export function headlessCtx(cwd) {
  return {
    cwd,
    hasUI: false,
    sessionManager: {
      getBranch: () => [],
      getSessionId: () => 's1',
      getSessionFile: () => '',
    },
  }
}
