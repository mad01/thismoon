import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  analyzeBash,
  canonicalFrom,
  classifyPath,
  denyReason,
  detectLayout,
  isWeaveArgs,
  paneLines,
  parseCount,
  parseDefaultRef,
  parseWorktreeList,
  removalQuestion,
  resolvePath,
  statusText,
  tokenizeShell,
  worktreeOfBranch,
} from '../lib/loom.ts'

const HOME = '/home/u'
const CANONICAL = '/home/u/code/thismoon'
const OWN = '/home/u/.worktrees/thismoon/feat-pane'
const OTHER = '/home/u/.worktrees/thismoon/fix-band'
const layout = { canonical: CANONICAL, ownWorktree: OWN, worktreeRoot: '/home/u/.worktrees/thismoon' }

const porcelain = [
  `worktree ${CANONICAL}`,
  'HEAD 1111111111111111111111111111111111111111',
  'branch refs/heads/main',
  '',
  `worktree ${OWN}`,
  'HEAD 2222222222222222222222222222222222222222',
  'branch refs/heads/feat/pane',
  '',
  '/tmp/loose',
  `worktree ${OTHER}`,
  'HEAD 3333333333333333333333333333333333333333',
  'detached',
  '',
  'worktree /home/u/code/bare.git',
  'bare',
  '',
].join('\n')

describe('parseWorktreeList', () => {
  const entries = parseWorktreeList(porcelain)

  test('reads path, head and branch per entry', () => {
    assert.equal(entries.length, 4)
    assert.deepEqual(entries[0], {
      path: CANONICAL,
      head: '1111111111111111111111111111111111111111',
      branch: 'main',
      isBare: false,
      isDetached: false,
    })
    assert.equal(entries[1].branch, 'feat/pane')
  })

  test('marks detached and bare entries', () => {
    assert.equal(entries[2].branch, null)
    assert.equal(entries[2].isDetached, true)
    assert.equal(entries[3].isBare, true)
  })

  test('is empty for empty output', () => {
    assert.deepEqual(parseWorktreeList(''), [])
  })

  test('finds the worktree of a branch', () => {
    assert.equal(worktreeOfBranch(entries, 'feat/pane')?.path, OWN)
    assert.equal(worktreeOfBranch(entries, 'nope'), null)
  })
})

describe('canonicalFrom', () => {
  test('is the parent of an absolute common dir', () => {
    assert.equal(canonicalFrom(`${CANONICAL}/.git\n`, OWN), CANONICAL)
  })

  test('resolves a relative common dir against the cwd', () => {
    assert.equal(canonicalFrom('.git', CANONICAL), CANONICAL)
    assert.equal(canonicalFrom('../.git', `${CANONICAL}/mods`), CANONICAL)
  })

  test('is null outside git', () => {
    assert.equal(canonicalFrom(null, '/tmp'), null)
    assert.equal(canonicalFrom('', '/tmp'), null)
  })
})

describe('detectLayout', () => {
  const common = `${CANONICAL}/.git`

  test('is owner inside ~/.worktrees/<repo>/<slug>', () => {
    const found = detectLayout({ cwd: `${OWN}/mods`, toplevel: OWN, commonDir: common, home: HOME, weaveSeen: false })
    assert.equal(found.role, 'owner')
    assert.equal(found.ownWorktree, OWN)
    assert.equal(found.repo, 'thismoon')
    assert.equal(found.worktreeRoot, '/home/u/.worktrees/thismoon')
  })

  test('is weaver in the canonical checkout once a weave was asked for', () => {
    const base = { cwd: CANONICAL, toplevel: CANONICAL, commonDir: '.git', home: HOME }
    assert.equal(detectLayout({ ...base, weaveSeen: true }).role, 'weaver')
    assert.equal(detectLayout({ ...base, weaveSeen: false }).role, 'none')
  })

  test('is none in a worktree outside the loom layout', () => {
    const agent = `${CANONICAL}/.claude/worktrees/agent-1`
    const found = detectLayout({ cwd: agent, toplevel: agent, commonDir: common, home: HOME, weaveSeen: true })
    assert.equal(found.role, 'none')
    assert.equal(found.canonical, CANONICAL)
  })

  test('is none with no repo or no home', () => {
    assert.equal(detectLayout({ cwd: '/tmp', toplevel: null, commonDir: null, home: HOME, weaveSeen: false }).role, 'none')
    const noHome = detectLayout({ cwd: OWN, toplevel: OWN, commonDir: common, home: null, weaveSeen: false })
    assert.equal(noHome.role, 'none')
    assert.equal(noHome.worktreeRoot, null)
  })
})

describe('classifyPath', () => {
  test('tells own, canonical, another worktree and outside apart', () => {
    assert.equal(classifyPath(`${OWN}/lib/loom.ts`, layout), 'own')
    assert.equal(classifyPath(`${CANONICAL}/README.md`, layout), 'canonical')
    assert.equal(classifyPath(`${OTHER}/x`, layout), 'other')
    assert.equal(classifyPath('/tmp/notes.md', layout), 'outside')
  })

  test('does not take a sibling with a longer name for the root', () => {
    assert.equal(classifyPath(`${CANONICAL}-two/x`, layout), 'outside')
  })

  test('is outside everywhere with nothing known', () => {
    assert.equal(classifyPath(CANONICAL, { canonical: null, ownWorktree: null, worktreeRoot: null }), 'outside')
  })

  test('names the holder in the reason', () => {
    assert.match(denyReason('/x', 'canonical'), /belongs to the canonical checkout; owners edit only their own worktree/)
    assert.match(denyReason('/x', 'other'), /belongs to another worktree/)
  })
})

describe('resolvePath', () => {
  test('resolves absolute, relative and ~ paths', () => {
    assert.equal(resolvePath('/a/b/../c', '/cwd', HOME), '/a/c')
    assert.equal(resolvePath('sub/./x', '/cwd', HOME), '/cwd/sub/x')
    assert.equal(resolvePath('~/code', '/cwd', HOME), '/home/u/code')
  })

  test('is null for what the shell would have to expand', () => {
    assert.equal(resolvePath('$HOME/x', '/cwd', HOME), null)
    assert.equal(resolvePath('~', '/cwd', null), null)
    assert.equal(resolvePath('-', '/cwd', HOME), null)
  })
})

describe('tokenizeShell', () => {
  test('strips quotes and keeps spaces inside them', () => {
    const { tokens, isUncertain } = tokenizeShell(`git -C "/my repo" commit -m 'a b'`)
    assert.equal(isUncertain, false)
    assert.deepEqual(
      tokens.map(t => t.text),
      ['git', '-C', '/my repo', 'commit', '-m', 'a b'],
    )
  })

  test('splits operators and redirections', () => {
    const { tokens } = tokenizeShell('a && b 2>&1 > out || c; d | e')
    assert.deepEqual(
      tokens.map(t => (t.kind === 'op' ? `<${t.text}>` : t.text)),
      ['a', '<&&>', 'b', '<2>&>', '1', '<>>', 'out', '<||>', 'c', '<;>', 'd', '<|>', 'e'],
    )
  })

  test('is uncertain on substitutions, heredocs and open quotes', () => {
    assert.equal(tokenizeShell('echo $(pwd)').isUncertain, true)
    assert.equal(tokenizeShell('echo `pwd`').isUncertain, true)
    assert.equal(tokenizeShell('cat <<EOF\nx\nEOF').isUncertain, true)
    assert.equal(tokenizeShell(`echo "open`).isUncertain, true)
  })
})

describe('analyzeBash', () => {
  const env = { cwd: OWN, home: HOME }

  test('cd a && git -C b commit: the commit acts on b', () => {
    const found = analyzeBash(`cd ${CANONICAL} && git -C ${OTHER} commit -m x`, env)
    assert.equal(found.isUncertain, false)
    assert.deepEqual(found.writes, [OTHER])
  })

  test('cd then a bare git verb acts where cd went', () => {
    assert.deepEqual(analyzeBash(`cd ${CANONICAL} && git add -A`, env).writes, [CANONICAL])
    assert.deepEqual(analyzeBash('cd mods; git commit -m x', env).writes, [`${OWN}/mods`])
  })

  test('a quoted path with a space resolves whole', () => {
    const found = analyzeBash(`git -C "${HOME}/.worktrees/thismoon/with space" push`, env)
    assert.deepEqual(found.writes, [`${HOME}/.worktrees/thismoon/with space`])
  })

  test('a path outside the repo is reported and classifies outside', () => {
    const found = analyzeBash('git -C /tmp/elsewhere commit -m x', env)
    assert.deepEqual(found.writes, ['/tmp/elsewhere'])
    assert.equal(classifyPath(found.writes[0], layout), 'outside')
  })

  test('read-only commands write nowhere', () => {
    const found = analyzeBash(`git -C ${CANONICAL} status && git log --oneline -3 && ls ${CANONICAL} && cat ${OTHER}/x`, env)
    assert.equal(found.isUncertain, false)
    assert.deepEqual(found.writes, [])
  })

  test('a redirection writes its target, not an fd', () => {
    assert.deepEqual(analyzeBash(`echo x > ${CANONICAL}/notes 2>&1`, env).writes, [`${CANONICAL}/notes`])
    assert.deepEqual(analyzeBash('make test > /dev/null 2>&1', env).writes, ['/dev/null'])
    assert.deepEqual(analyzeBash('echo x >> log.txt', env).writes, [`${OWN}/log.txt`])
  })

  test('absolute operands of a mutating verb count', () => {
    assert.deepEqual(analyzeBash(`git add ${CANONICAL}/x`, env).writes, [OWN, `${CANONICAL}/x`])
  })

  test('is uncertain on what it cannot follow', () => {
    assert.equal(analyzeBash('cd $DIR && git commit -m x', env).isUncertain, true)
    assert.equal(analyzeBash('bash -c "git commit"', env).isUncertain, true)
    assert.equal(analyzeBash('git --work-tree=/x commit', env).isUncertain, true)
    assert.equal(analyzeBash('echo x > $OUT', env).isUncertain, true)
  })

  test('finds a branch force-delete', () => {
    assert.deepEqual(analyzeBash('git branch -D feat/x', env).removals, [{ kind: 'branch', name: 'feat/x', location: OWN }])
    assert.deepEqual(analyzeBash(`git -C ${CANONICAL} branch --delete --force a b`, env).removals, [
      { kind: 'branch', name: 'a', location: CANONICAL },
      { kind: 'branch', name: 'b', location: CANONICAL },
    ])
    assert.deepEqual(analyzeBash('git branch -d feat/x', env).removals, [])
  })

  test('finds a worktree removal by path', () => {
    const found = analyzeBash(`git -C ${CANONICAL} worktree remove --force ~/.worktrees/thismoon/fix-band`, env)
    assert.deepEqual(found.removals, [{ kind: 'worktree', path: OTHER }])
    assert.deepEqual(analyzeBash('git worktree list', env).removals, [])
  })
})

describe('paneLines', () => {
  const rows = [
    { path: CANONICAL, branch: 'main', head: null, isDirty: false, ahead: 0, age: '3 days ago' },
    { path: OWN, branch: 'feat/pane', head: null, isDirty: true, ahead: 2, age: '5 minutes ago' },
    { path: OTHER, branch: null, head: '3333333abcdef', isDirty: null, ahead: null, age: null },
  ]

  test('marks own and canonical rows and fills ? for failed calls', () => {
    const lines = paneLines(rows, layout)
    assert.equal(lines[0], '= main              clean    +0  3 days ago')
    assert.equal(lines[1], '> feat/pane         dirty    +2  5 minutes ago')
    assert.equal(lines[2], '  detached 3333333  ?         ?  ?')
  })

  test('cuts to the width given', () => {
    assert.equal(paneLines(rows, layout, 10)[1], '> feat/pan')
  })

  test('is empty with no rows', () => {
    assert.deepEqual(paneLines([], layout), [])
  })
})

describe('statusText', () => {
  const rows = [
    { path: CANONICAL, branch: 'main', head: null, isDirty: false, ahead: 0, age: null },
    { path: OWN, branch: 'a', head: null, isDirty: true, ahead: 1, age: null },
    { path: OTHER, branch: 'b', head: null, isDirty: false, ahead: 0, age: null },
  ]

  test('names the owner slug', () => {
    assert.equal(statusText('owner', layout, []), 'loom: owner feat-pane')
  })

  test('counts linked worktrees and dirty ones for the weaver', () => {
    assert.equal(statusText('weaver', layout, rows), 'loom: weaver · 2 worktrees, 1 dirty')
    assert.equal(statusText('weaver', layout, []), 'loom: weaver')
  })

  test('clears for no role', () => {
    assert.equal(statusText('none', layout, rows), undefined)
  })
})

describe('small parsers', () => {
  test('parseDefaultRef shortens the remote HEAD and falls back to main', () => {
    assert.equal(parseDefaultRef('refs/remotes/origin/main\n'), 'origin/main')
    assert.equal(parseDefaultRef('refs/remotes/origin/develop'), 'origin/develop')
    assert.equal(parseDefaultRef(null), 'main')
    assert.equal(parseDefaultRef(''), 'main')
  })

  test('parseCount reads a number or null', () => {
    assert.equal(parseCount('3\n'), 3)
    assert.equal(parseCount(null), null)
    assert.equal(parseCount('fatal: bad revision'), null)
  })

  test('isWeaveArgs needs the word', () => {
    assert.equal(isWeaveArgs('weave'), true)
    assert.equal(isWeaveArgs('claim weave-this'), false)
    assert.equal(isWeaveArgs('status'), false)
  })

  test('removalQuestion names each risk', () => {
    assert.equal(removalQuestion('feat/x', 2, true), 'feat/x has 2 unmerged commits and uncommitted changes. Remove anyway?')
    assert.equal(removalQuestion('feat/x', 1, false), 'feat/x has 1 unmerged commit. Remove anyway?')
    assert.equal(removalQuestion('/wt', null, true), '/wt has uncommitted changes. Remove anyway?')
  })
})
