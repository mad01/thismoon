// The belt deny count against the engine's own test kit: `claude plugin
// test mods/session-band`. The hooks registered here stand beneath the
// plugin for the engine: the status line, the debug log and the Bash tool.
// A test's own classic.PreToolUse hook stands for belt, which a session
// runs as a settings hook beneath every mod's tool.call hook.
//
// What these tests cannot catch: the kit seats no built-in guard mod, so a
// hook on an event that guard skips runs here as if nothing skipped it. The
// version before 0.2.1, which counted on classic.PreToolUse, passes every
// count and result assertion below; only its debug-log line differs. CI's
// .github/scripts/mods-skipped-events.mjs is the one check against a hook on
// a skipped event. What the tests do catch: a result passed on changed or
// without core's `ref`, a deny left uncounted, or a subagent's deny left
// out. They also catch a count taken from a successful call, from a later
// line of output, or from another hook's deny.
import { expect, test } from 'claude-code/testing'
import type { On, ToolCallResult } from 'claude-code'

type World = {
  statuses: Array<string | undefined>
  logs: string[]
  // What the Bash tool answers when no PreToolUse hook refused the call.
  bash: ToolCallResult
}

const QUIET: ToolCallResult = { result: { stdout: '', stderr: '', interrupted: false } }

function seat(on: On): World {
  const world: World = { statuses: [], logs: [], bash: QUIET }
  on('ui.status', (_$, e) => {
    world.statuses.push(e.text)
    return { value: undefined }
  })
  on('ui.log', (_$, e) => {
    world.logs.push(e.text)
    return { value: undefined }
  })
  on('tool.call', { tool: 'Bash' }, () => world.bash)
  return world
}

const last = (world: World): string | undefined => world.statuses[world.statuses.length - 1]

const shown = (world: World): string[] => world.statuses.filter((text): text is string => text !== undefined)

const refusalOf = (ran: { deny?: string; isError?: true; text?: string }): string | undefined =>
  ran.deny ?? (ran.isError === true ? ran.text : undefined)

const push = { tool: 'Bash' as const, command: 'git push origin main' }

// A push raised in a subagent's loop. The call type leaves `agentId` out,
// but the kit hands it to the tool.call hooks as a session does.
const pushFrom = (agentId: string) => ({ ...push, agentId })

// belt's guard refusing the call, as its settings PreToolUse hook does.
const beltDeny = () => ({ deny: 'belt[git-push-main]: pushes to main are denied' })

// The errored result core makes of a settings-hook deny. `ref` is core's
// opaque handle on the messages it produced; the mod passes it on as it came.
const refused = (reason: string) => ({ ref: expect.any(Number), result: `Error: ${reason}`, text: reason, isError: true })

test('one belt deny pins the count; the refusal goes back to the caller unchanged', async ($, on) => {
  const world = seat(on)
  on('classic.PreToolUse', beltDeny)
  const ran = await $.tool.call(push)
  expect(ran).toEqual(refused('belt[git-push-main]: pushes to main are denied'))
  expect(last(world)).toBe('belt denies 1')
  expect(world.logs).toContain('session-band: belt deny 1 counted on Bash')
})

test('a subagent\'s belt deny counts beside the main loop\'s', async ($, on) => {
  const world = seat(on)
  on('classic.PreToolUse', beltDeny)
  await $.tool.call(push)
  await $.tool.call(pushFrom('a1'))
  expect(last(world)).toBe('belt denies 2')
})

test('a call whose output quotes belt does not count, successful or failed', async ($, on) => {
  const world = seat(on)
  world.bash = { result: { stdout: 'belt[x]: quoted', stderr: '', interrupted: false }, text: 'belt[x]: quoted from a log' }
  const ran = await $.tool.call(push)
  expect(refusalOf(ran)).toBeUndefined()
  world.bash = { result: { stdout: '', stderr: 'belt[x]: quoted', interrupted: false }, text: 'Exit code 1\nbelt[x]: quoted from a log', isError: true }
  await $.tool.call(push)
  expect(last(world)).toBeUndefined()
  expect(shown(world)).toEqual([])
})

test('a deny from another settings hook does not count', async ($, on) => {
  const world = seat(on)
  on('classic.PreToolUse', () => ({ deny: 'policy: pushes wait for review' }))
  const ran = await $.tool.call(push)
  expect(ran).toEqual(refused('policy: pushes wait for review'))
  expect(last(world)).toBeUndefined()
  expect(shown(world)).toEqual([])
})
