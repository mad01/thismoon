import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  analyzeBash,
  canonicalFrom,
  classifyPath,
  denyReason,
  detectLayout,
  isWeaveArgs,
  loomArgsOf,
  ownerClaimFrom,
  parseCount,
  parseDefaultRef,
  parseWorktreeList,
  removalQuestion,
  resolvePath,
  shouldAskBranchDeletion,
  shouldAskWorktreeRemoval,
  statusText,
  tokenizeShell,
  worktreeAt,
  worktreeClaimedBy,
  worktreeOfBranch,
} from '../lib/loom.ts'

const HOME = '/home/u'
const CANONICAL = '/home/u/code/thismoon'
const ROOT = '/home/u/.worktrees/thismoon'
const OWN = `${ROOT}/feat-pane`
const OTHER = `${ROOT}/fix-band`
const NESTED = `${CANONICAL}/.claude/worktrees/agent-1`
const layout = { canonical: CANONICAL, ownWorktree: OWN, worktreeRoot: ROOT, worktrees: [CANONICAL, OWN, OTHER, NESTED] }

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

  test('finds a worktree by branch and by path', () => {
    assert.equal(worktreeOfBranch(entries, 'feat/pane')?.path, OWN)
    assert.equal(worktreeOfBranch(entries, 'nope'), null)
    assert.equal(worktreeAt(entries, `${OTHER}/`)?.isDetached, true)
    assert.equal(worktreeAt(entries, '/nowhere'), null)
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
    assert.equal(found.worktreeRoot, ROOT)
  })

  test('is weaver anywhere in the repo once a weave was asked for', () => {
    const base = { cwd: CANONICAL, toplevel: CANONICAL, commonDir: '.git', home: HOME }
    assert.equal(detectLayout({ ...base, weaveSeen: true }).role, 'weaver')
    assert.equal(detectLayout({ ...base, weaveSeen: false }).role, 'none')
    const inWorktree = detectLayout({ cwd: OWN, toplevel: OWN, commonDir: common, home: HOME, weaveSeen: true })
    assert.equal(inWorktree.role, 'weaver')
    assert.equal(inWorktree.ownWorktree, null)
  })

  test('is none in a worktree outside the loom layout', () => {
    const found = detectLayout({ cwd: NESTED, toplevel: NESTED, commonDir: common, home: HOME, weaveSeen: false })
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

describe('owner claims', () => {
  test('a path at or below ~/.worktrees/<repo>/<slug> claims that worktree', () => {
    assert.equal(worktreeClaimedBy(OWN, ROOT), OWN)
    assert.equal(worktreeClaimedBy(`${OWN}/mods/x`, ROOT), OWN)
    assert.equal(worktreeClaimedBy(ROOT, ROOT), null)
    assert.equal(worktreeClaimedBy(CANONICAL, ROOT), null)
    assert.equal(worktreeClaimedBy(OWN, null), null)
  })

  test('the first claim in a command wins', () => {
    assert.equal(ownerClaimFrom([CANONICAL, OTHER, OWN], ROOT), OTHER)
    assert.equal(ownerClaimFrom([CANONICAL], ROOT), null)
  })

  test('git worktree add and cd both claim', () => {
    const env = { cwd: CANONICAL, home: HOME }
    assert.deepEqual(analyzeBash(`git worktree add ~/.worktrees/thismoon/feat-pane -b feat/pane`, env).claims, [OWN])
    assert.deepEqual(analyzeBash(`git worktree add -b feat/pane ${OWN}`, env).claims, [OWN])
    assert.deepEqual(analyzeBash(`cd ${OWN} && git commit -m x`, env).claims, [OWN])
    assert.equal(ownerClaimFrom(analyzeBash('cd mods && git status', env).claims, ROOT), null)
  })
})

describe('classifyPath', () => {
  test('tells own, canonical, another worktree and outside apart', () => {
    assert.equal(classifyPath(`${OWN}/lib/loom.ts`, layout), 'own')
    assert.equal(classifyPath(`${CANONICAL}/README.md`, layout), 'canonical')
    assert.equal(classifyPath(`${OTHER}/x`, layout), 'other')
    assert.equal(classifyPath(`${ROOT}/unlisted/x`, layout), 'other')
    assert.equal(classifyPath('/tmp/notes.md', layout), 'outside')
  })

  test('a worktree nested under the canonical checkout is another worktree', () => {
    assert.equal(classifyPath(`${NESTED}/README.md`, layout), 'other')
    assert.equal(classifyPath(`${CANONICAL}/.claude/worktrees/README.md`, layout), 'canonical')
  })

  test('does not take a sibling with a longer name for the root', () => {
    assert.equal(classifyPath(`${CANONICAL}-two/x`, layout), 'outside')
  })

  test('is outside everywhere with nothing known', () => {
    assert.equal(classifyPath(CANONICAL, { canonical: null, ownWorktree: null, worktreeRoot: null, worktrees: [] }), 'outside')
  })

  test('names the holder in the reason', () => {
    assert.match(denyReason('/x', 'canonical'), /belongs to the canonical checkout; owners edit only their own worktree/)
    assert.match(denyReason('/x', 'other'), /belongs to another worktree/)
  })
})

describe('resolvePath', () => {
  test('resolves absolute, relative, .. and ~ paths', () => {
    assert.equal(resolvePath('/a/b/../c', '/cwd', HOME), '/a/c')
    assert.equal(resolvePath('sub/./x', '/cwd', HOME), '/cwd/sub/x')
    assert.equal(resolvePath('~/code', '/cwd', HOME), '/home/u/code')
    assert.equal(resolvePath(`${OWN}/../../../code/thismoon/README.md`, '/cwd', HOME), `${CANONICAL}/README.md`)
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

  test('a cd inside a subshell ends with it', () => {
    const found = analyzeBash(`(cd ${CANONICAL} && git log -1) && git commit -m x`, env)
    assert.deepEqual(found.writes, [OWN])
    assert.deepEqual(analyzeBash(`(cd ${CANONICAL}; git add .) ; git status`, env).writes, [CANONICAL])
  })

  test('a quoted path with a space resolves whole', () => {
    const found = analyzeBash(`git -C "${ROOT}/with space" push`, env)
    assert.deepEqual(found.writes, [`${ROOT}/with space`])
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
    assert.deepEqual(analyzeBash(`git -C ${CANONICAL} stash list && git -C ${CANONICAL} stash show -p`, env).writes, [])
  })

  test('stash with no subcommand, push, pop and drop mutate', () => {
    assert.deepEqual(analyzeBash(`git -C ${CANONICAL} stash`, env).writes, [CANONICAL])
    assert.deepEqual(analyzeBash(`git -C ${CANONICAL} stash pop`, env).writes, [CANONICAL])
  })

  test('a redirection writes its target, not an fd', () => {
    assert.deepEqual(analyzeBash(`echo x > ${CANONICAL}/notes 2>&1`, env).writes, [`${CANONICAL}/notes`])
    assert.deepEqual(analyzeBash('make test > /dev/null 2>&1', env).writes, ['/dev/null'])
    assert.deepEqual(analyzeBash('echo x >> log.txt', env).writes, [`${OWN}/log.txt`])
  })

  test('path operands count for add, rm, mv, checkout, restore and clean only', () => {
    assert.deepEqual(analyzeBash(`git add ${CANONICAL}/x`, env).writes, [OWN, `${CANONICAL}/x`])
    assert.deepEqual(analyzeBash(`git checkout -b feat ${CANONICAL}/x`, env).writes, [OWN, `${CANONICAL}/x`])
    assert.deepEqual(analyzeBash(`git apply ${CANONICAL}/fix.patch`, env).writes, [OWN])
    assert.deepEqual(analyzeBash(`git commit -m "${CANONICAL}/x: fix"`, env).writes, [OWN])
    assert.deepEqual(analyzeBash(`git commit -F ${CANONICAL}/msg.txt`, env).writes, [OWN])
  })

  test('accepts command git, time git and a git by path', () => {
    assert.deepEqual(analyzeBash(`command git -C ${CANONICAL} commit -m x`, env).writes, [CANONICAL])
    assert.deepEqual(analyzeBash(`time git -C ${CANONICAL} push`, env).writes, [CANONICAL])
    assert.deepEqual(analyzeBash(`/usr/bin/git -C ${CANONICAL} add .`, env).writes, [CANONICAL])
  })

  test('is uncertain on what it cannot follow', () => {
    assert.equal(analyzeBash('cd $DIR && git commit -m x', env).isUncertain, true)
    assert.equal(analyzeBash('bash -c "git commit"', env).isUncertain, true)
    assert.equal(analyzeBash('git --work-tree=/x commit', env).isUncertain, true)
    assert.equal(analyzeBash('echo x > $OUT', env).isUncertain, true)
    assert.equal(analyzeBash(`GIT_WORK_TREE=${CANONICAL} git commit -m x`, env).isUncertain, true)
    assert.equal(analyzeBash(`git worktree add $WT -b x`, env).isUncertain, true)
  })

  test('finds a branch force-delete', () => {
    assert.deepEqual(analyzeBash('git branch -D feat/x', env).removals, [{ kind: 'branch', name: 'feat/x', location: OWN }])
    assert.deepEqual(analyzeBash(`git -C ${CANONICAL} branch --delete --force a b`, env).removals, [
      { kind: 'branch', name: 'a', location: CANONICAL },
      { kind: 'branch', name: 'b', location: CANONICAL },
    ])
    assert.deepEqual(analyzeBash('git branch -d feat/x', env).removals, [])
  })

  test('finds a worktree removal by path and whether it is forced', () => {
    const forced = analyzeBash(`git -C ${CANONICAL} worktree remove --force ~/.worktrees/thismoon/fix-band`, env)
    assert.deepEqual(forced.removals, [{ kind: 'worktree', path: OTHER, isForce: true }])
    const plain = analyzeBash(`git -C ${CANONICAL} worktree remove ${OTHER}`, env)
    assert.deepEqual(plain.removals, [{ kind: 'worktree', path: OTHER, isForce: false }])
    assert.deepEqual(analyzeBash('git worktree list', env).removals, [])
  })
})

describe('removal rules', () => {
  test('a worktree removal asks only when forced and dirty, or detached with commits', () => {
    assert.equal(shouldAskWorktreeRemoval({ isForce: false, isDirty: true, isDetached: false, ahead: 3 }), false)
    assert.equal(shouldAskWorktreeRemoval({ isForce: true, isDirty: false, isDetached: false, ahead: 3 }), false)
    assert.equal(shouldAskWorktreeRemoval({ isForce: true, isDirty: true, isDetached: false, ahead: 0 }), true)
    assert.equal(shouldAskWorktreeRemoval({ isForce: false, isDirty: false, isDetached: true, ahead: 2 }), true)
    assert.equal(shouldAskWorktreeRemoval({ isForce: false, isDirty: false, isDetached: true, ahead: null }), false)
  })

  test('a branch deletion asks only when its commits exist nowhere else, or its tree is dirty', () => {
    assert.equal(shouldAskBranchDeletion({ ahead: 2, remoteAhead: null, isWorktreeDirty: false }), true)
    assert.equal(shouldAskBranchDeletion({ ahead: 2, remoteAhead: 1, isWorktreeDirty: false }), true)
    assert.equal(shouldAskBranchDeletion({ ahead: 2, remoteAhead: 0, isWorktreeDirty: false }), false)
    assert.equal(shouldAskBranchDeletion({ ahead: 0, remoteAhead: null, isWorktreeDirty: false }), false)
    assert.equal(shouldAskBranchDeletion({ ahead: null, remoteAhead: null, isWorktreeDirty: false }), false)
    assert.equal(shouldAskBranchDeletion({ ahead: 0, remoteAhead: 0, isWorktreeDirty: true }), true)
  })
})

describe('statusText', () => {
  test('names the owner slug without repeating the mod name', () => {
    assert.equal(statusText('owner', layout), 'owner feat-pane')
    assert.equal(statusText('owner', { ...layout, ownWorktree: null }), 'owner ?')
  })

  test('counts the linked worktrees for the weaver', () => {
    assert.equal(statusText('weaver', layout), 'weaver · 3 worktrees')
    assert.equal(statusText('weaver', { ...layout, worktrees: [CANONICAL, OWN] }), 'weaver · 1 worktree')
    assert.equal(statusText('weaver', { ...layout, worktrees: [] }), 'weaver')
  })

  test('clears for no role', () => {
    assert.equal(statusText('none', layout), undefined)
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

  test('isWeaveArgs needs weave as the first word', () => {
    assert.equal(isWeaveArgs('weave'), true)
    assert.equal(isWeaveArgs('  weave now'), true)
    assert.equal(isWeaveArgs('claim weave-this'), false)
    assert.equal(isWeaveArgs('claim feat/weave-x'), false)
    assert.equal(isWeaveArgs('status'), false)
  })

  test('loomArgsOf reads a /loom prompt the same way the command does', () => {
    assert.equal(loomArgsOf('/loom weave'), 'weave')
    assert.equal(loomArgsOf('/loom'), '')
    assert.equal(loomArgsOf('/loom claim feat/weave-x'), 'claim feat/weave-x')
    assert.equal(isWeaveArgs(loomArgsOf('/loom claim feat/weave-x')), false)
    assert.equal(loomArgsOf('/loom-pane'), null)
    assert.equal(loomArgsOf('please weave'), null)
  })

  test('removalQuestion names each risk', () => {
    assert.equal(removalQuestion('feat/x', 2, true), 'feat/x has 2 unmerged commits and uncommitted changes. Remove anyway?')
    assert.equal(removalQuestion('feat/x', 1, false), 'feat/x has 1 unmerged commit. Remove anyway?')
    assert.equal(removalQuestion('/wt', null, true), '/wt has uncommitted changes. Remove anyway?')
  })
})
