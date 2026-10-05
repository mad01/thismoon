import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CANCELLED_REASON,
  DEFAULT_CONFIG,
  HOLD_OPTIONS,
  basename,
  extractCommitMessage,
  holdQuestion,
  isProseFile,
  parseDetectOutput,
  readConfig,
  statusText,
  summarizeFindings,
} from '../lib/gate.ts'

const empty = { total: 0, errors: 0, warnings: 0, suggestions: 0, matches: [] }

describe('readConfig', () => {
  test('fills the defaults from an empty options object', () => {
    assert.deepEqual(readConfig({}), DEFAULT_CONFIG)
  })

  test('takes booleans and a known severity', () => {
    assert.deepEqual(readConfig({ holdOnError: false, holdOnCommitError: true, minSeverity: 'error' }), {
      holdOnError: false,
      holdOnCommitError: true,
      minSeverity: 'error',
    })
  })

  test('falls back on a value of the wrong kind', () => {
    assert.deepEqual(readConfig({ holdOnError: 'yes', holdOnCommitError: 1, minSeverity: 'loud' }), DEFAULT_CONFIG)
  })
})

describe('isProseFile', () => {
  const table = [
    ['/repo/README.md', true],
    ['/repo/docs/guide.mdx', true],
    ['/repo/docs/GUIDE.MD', true],
    ['/repo/.github/PULL_REQUEST_TEMPLATE.md', true],
    ['/tmp/hg-test.md', true],
    ['/repo/main.go', false],
    ['/repo/notes.markdown', false],
    ['/repo/node_modules/pkg/README.md', false],
    ['/repo/.git/COMMIT_EDITMSG.md', false],
    ['/home/u/.claude/CLAUDE.md', false],
    ['/repo/.vscode/notes.md', false],
    ['/repo/.hidden.md', false],
    ['./docs/relative.md', true],
    ['../docs/relative.md', true],
    ['', false],
  ]
  for (const [path, expected] of table) {
    test(`${JSON.stringify(path)} -> ${expected}`, () => {
      assert.equal(isProseFile(path), expected)
    })
  }
})

describe('basename', () => {
  test('is the last segment', () => {
    assert.equal(basename('/repo/docs/guide.md'), 'guide.md')
    assert.equal(basename('/repo/docs/'), 'docs')
    assert.equal(basename('guide.md'), 'guide.md')
  })
})

describe('extractCommitMessage', () => {
  test('reads a double-quoted -m', () => {
    assert.equal(extractCommitMessage('git commit -m "feat(x): add the thing"'), 'feat(x): add the thing')
  })

  test('reads a single-quoted -m', () => {
    assert.equal(extractCommitMessage("git commit -am 'fix: typo' && echo done".replace('-am', '-a -m')), 'fix: typo')
  })

  test('joins several -m flags with a blank line, as git does', () => {
    const command = 'git commit -m "subject" -m "first paragraph" -m \'second paragraph\''
    assert.equal(extractCommitMessage(command), 'subject\n\nfirst paragraph\n\nsecond paragraph')
  })

  test('reads --message and --message=', () => {
    assert.equal(extractCommitMessage('git commit --message "one"'), 'one')
    assert.equal(extractCommitMessage('git commit --message="two"'), 'two')
  })

  test('unescapes a backslash-escaped quote inside -m', () => {
    assert.equal(extractCommitMessage('git commit -m "say \\"hi\\" there"'), 'say "hi" there')
  })

  test('reads a heredoc on -F -', () => {
    const command = ["git commit -F - <<'EOF'", 'docs: rewrite the intro', '', 'Why: it was stale.', 'EOF'].join('\n')
    assert.equal(extractCommitMessage(command), 'docs: rewrite the intro\n\nWhy: it was stale.')
  })

  test('reads the heredoc inside -m "$(cat <<EOF ...)"', () => {
    const command = [
      'git commit -m "$(cat <<\'EOF\'',
      'feat(mods): add humanizer-gate',
      '',
      'Body line.',
      'EOF',
      ')"',
    ].join('\n')
    assert.equal(extractCommitMessage(command), 'feat(mods): add humanizer-gate\n\nBody line.')
  })

  test('accepts an unquoted and a double-quoted heredoc delimiter', () => {
    assert.equal(extractCommitMessage('git commit -F - <<MSG\nhello\nMSG'), 'hello')
    assert.equal(extractCommitMessage('git commit -F - <<"MSG"\nhello\nMSG\n'), 'hello')
  })

  test('is null for an unterminated heredoc', () => {
    assert.equal(extractCommitMessage("git commit -F - <<'EOF'\nnever closed"), null)
  })

  test('is null when the message is not in the command', () => {
    assert.equal(extractCommitMessage('git commit'), null)
    assert.equal(extractCommitMessage('git commit -F msg.txt'), null)
    assert.equal(extractCommitMessage('git commit -m fix'), null)
    assert.equal(extractCommitMessage('git commit -m ""'), null)
  })

  test('is null for a command that does not commit', () => {
    assert.equal(extractCommitMessage('git log -m "x"'), null)
    assert.equal(extractCommitMessage('echo "git commits are nice"'), null)
  })

  test('sees through git options before the verb', () => {
    assert.equal(extractCommitMessage('git -C /repo commit -m "chore: tidy"'), 'chore: tidy')
    assert.equal(extractCommitMessage('cd /repo && git commit -q -m "chore: tidy"'), 'chore: tidy')
  })
})

describe('summarizeFindings', () => {
  const payload = {
    findings: [
      { severity: 'warning', match: 'delve' },
      { severity: 'warning', match: 'tapestry' },
      { severity: 'error', match: 'vibrant' },
      { severity: 'warning', match: 'vibrant' },
      { severity: 'error', match: 'is a testament' },
      { severity: 'suggestion', match: 'Furthermore' },
      { severity: 'error', match: 'is a testament' },
    ],
  }

  test('counts by severity', () => {
    const summary = summarizeFindings(payload)
    assert.equal(summary.total, 7)
    assert.equal(summary.errors, 3)
    assert.equal(summary.warnings, 3)
    assert.equal(summary.suggestions, 1)
  })

  test('quotes three distinct matches, errors first', () => {
    assert.deepEqual(summarizeFindings(payload).matches, ['vibrant', 'is a testament', 'delve'])
  })

  test('is empty for junk', () => {
    assert.deepEqual(summarizeFindings(null), empty)
    assert.deepEqual(summarizeFindings({ findings: 'nope' }), empty)
    assert.deepEqual(summarizeFindings({ findings: [null, 1, 'x'] }), { ...empty, total: 0 })
  })
})

describe('parseDetectOutput', () => {
  test('parses the detect payload', () => {
    const stdout = JSON.stringify({ findings: [{ severity: 'warning', match: 'delve' }], summary: {}, engine: 'vale' })
    assert.deepEqual(parseDetectOutput(stdout), { ...empty, total: 1, warnings: 1, matches: ['delve'] })
  })

  test('is null for non-JSON or a JSON without findings', () => {
    assert.equal(parseDetectOutput('vale: not found'), null)
    assert.equal(parseDetectOutput('{"engine":"vale"}'), null)
    assert.equal(parseDetectOutput(''), null)
  })
})

describe('statusText', () => {
  test('clears on a clean scan', () => {
    assert.equal(statusText(empty, 'README.md'), undefined)
  })

  test('names the counts in severity order, singular and plural', () => {
    assert.equal(statusText({ ...empty, total: 1, warnings: 1 }, 'README.md'), 'humanizer: 1 warning in README.md')
    assert.equal(
      statusText({ ...empty, total: 5, errors: 2, warnings: 3 }, 'PR body'),
      'humanizer: 2 errors, 3 warnings in PR body',
    )
    assert.equal(
      statusText({ ...empty, total: 2, warnings: 1, suggestions: 1 }, 'commit message'),
      'humanizer: 1 warning, 1 suggestion in commit message',
    )
  })
})

describe('holdQuestion', () => {
  test('names the error count and quotes the matches', () => {
    const summary = { ...empty, total: 3, errors: 2, warnings: 1, matches: ['vibrant', 'is a testament', 'delve'] }
    assert.equal(
      holdQuestion(summary, 'PR body'),
      'humanizer found 2 error-level findings in the PR body ("vibrant", "is a testament", "delve"). Send it anyway?',
    )
  })

  test('skips the quotes when there is nothing to quote', () => {
    assert.equal(
      holdQuestion({ ...empty, total: 1, errors: 1 }, 'commit message'),
      'humanizer found 1 error-level finding in the commit message. Send it anyway?',
    )
  })

  test('the dialog labels and deny text are fixed strings', () => {
    assert.deepEqual([...HOLD_OPTIONS], ['Proceed', 'Cancel'])
    assert.equal(CANCELLED_REASON, 'humanizer-gate: cancelled by user')
  })
})
