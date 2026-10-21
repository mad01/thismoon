import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// A belt stand-in on PATH. `check` records its argv and denies the
// subcommands FAKE_BELT_DENY lists (comma separated) with a DENY line, the
// way belt reports a guard verdict. `hint` records argv and stdin and
// answers with additionalContext naming the event.
const FAKE_BELT = `#!/bin/sh
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
    writeConfig: (value) =>
      writeFileSync(join(agentDir, 'belt.json'), typeof value === 'string' ? value : JSON.stringify(value)),
    claudeSettings: (value) => {
      mkdirSync(join(dir, '.claude'), { recursive: true })
      writeFileSync(join(dir, '.claude', 'settings.json'), JSON.stringify(value))
    },
  }
}

// A pi stand-in: handlers by event, the bus events emitted, the entries
// appended and the commands registered. call() runs the first handler.
export function fakePi() {
  const handlers = new Map()
  const emitted = []
  const pi = {
    on(name, handler) {
      handlers.set(name, [...(handlers.get(name) ?? []), handler])
    },
    events: {
      on() {},
      emit(name, data) {
        emitted.push([name, data])
      },
    },
    appendEntry() {},
    registerCommand() {},
  }
  const call = (name, event, ctx) => handlers.get(name)[0](event, ctx)
  return { pi, handlers, emitted, call }
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
