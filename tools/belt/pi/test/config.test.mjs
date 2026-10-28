import assert from 'node:assert/strict'
import { symlinkSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'

import { configPath, defaultConfig, directToolServer, envIs, loadConfig, parseConfig } from '../lib/config.ts'
import { scratch } from './fake.mjs'

test('the defaults name only this repo\'s servers and github.com, and no sandbox', () => {
  const config = defaultConfig()
  assert.deepEqual(Object.fromEntries(config.mcpPrefixes), {
    csl_: 'csl',
    humanizer_: 'humanizer',
    worklog_: 'worklog',
  })
  assert.deepEqual([...config.externalTextGuardServers], ['gh_com'])
  assert.deepEqual([...config.externalTextHintServers], ['gh_com', 'linear'])
  assert.equal(config.sandboxActiveEnv, '')
  assert.equal(config.sandboxAllowUnsandboxedEnv, '')
})

test('every key adds to the defaults and unknown keys are ignored', () => {
  const config = parseConfig({
    mcpPrefixes: { chat_: 'chat' },
    externalTextGuardServers: ['vcs'],
    externalTextHintServers: ['chat'],
    sandboxActiveEnv: 'SANDBOX_ON',
    sandboxAllowUnsandboxedEnv: 'SANDBOX_ESCAPE',
    someFutureKey: true,
  })
  assert.equal(directToolServer(config, 'chat_send_message'), 'chat')
  assert.equal(directToolServer(config, 'csl_search'), 'csl')
  assert.equal(directToolServer(config, 'bash'), '')
  assert.deepEqual([...config.externalTextGuardServers], ['gh_com', 'vcs'])
  assert.deepEqual([...config.externalTextHintServers], ['gh_com', 'linear', 'chat'])
  assert.equal(config.sandboxActiveEnv, 'SANDBOX_ON')
  assert.equal(config.sandboxAllowUnsandboxedEnv, 'SANDBOX_ESCAPE')
})

test('an mcpPrefixes entry overrides the default prefix it names', () => {
  const config = parseConfig({ mcpPrefixes: { csl_: 'other' } })
  assert.equal(directToolServer(config, 'csl_search'), 'other')
  assert.equal(directToolServer(config, 'worklog_show'), 'worklog')
})

test('a wrong shape is an error, never a silent default', () => {
  for (const bad of [
    [],
    'gh_com',
    { mcpPrefixes: ['csl_'] },
    { mcpPrefixes: { chat_: '' } },
    { externalTextGuardServers: 'vcs' },
    { externalTextHintServers: [''] },
    { sandboxActiveEnv: 1 },
  ]) {
    assert.throws(() => parseConfig(bad), undefined, JSON.stringify(bad))
  }
})

test('the file sits in the pi agent dir and falls back to the defaults', (t) => {
  const env = scratch(t)
  const path = join(env.agentDir, 'belt.json')
  assert.equal(configPath(), path)
  assert.equal(configPath({}), join(env.dir, '.pi', 'agent', 'belt.json'))

  // Missing: the defaults, no error.
  assert.deepEqual(loadConfig(), { config: defaultConfig(), path, error: '', unknownKeys: [] })

  env.writeConfig({ externalTextGuardServers: ['vcs'] })
  const loaded = loadConfig()
  assert.equal(loaded.error, '')
  assert.ok(loaded.config.externalTextGuardServers.has('vcs'))

  // Present but unreadable: the defaults plus the error, for the gate to act on.
  env.writeConfig('{not json')
  const broken = loadConfig()
  assert.notEqual(broken.error, '')
  assert.deepEqual(broken.config, defaultConfig())
})

test('a dangling belt.json symlink is an error, not a missing file', (t) => {
  const env = scratch(t)
  const missing = join(env.dir, 'gone', 'belt.json')
  symlinkSync(missing, join(env.agentDir, 'belt.json'))
  const loaded = loadConfig()
  assert.equal(loaded.error, `symlink to missing ${missing}`)
  assert.deepEqual(loaded.config, defaultConfig())
})

test('loadConfig names the unknown keys of a valid file', (t) => {
  const env = scratch(t)
  env.writeConfig({ externalTextGuardServer: ['vcs'], sandboxActiveEnv: 'S' })
  const loaded = loadConfig()
  assert.equal(loaded.error, '')
  assert.deepEqual(loaded.unknownKeys, ['externalTextGuardServer'])
  assert.deepEqual([...loaded.config.externalTextGuardServers], ['gh_com'])
})

test('envIs matches only a named variable', (t) => {
  const saved = process.env.BELT_PI_TEST_FLAG
  t.after(() => {
    if (saved === undefined) delete process.env.BELT_PI_TEST_FLAG
    else process.env.BELT_PI_TEST_FLAG = saved
  })
  process.env.BELT_PI_TEST_FLAG = '1'
  assert.equal(envIs('BELT_PI_TEST_FLAG', '1'), true)
  assert.equal(envIs('BELT_PI_TEST_FLAG', '0'), false)
  assert.equal(envIs('', '1'), false)
})
