import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  BAND_LINGER_MS,
  addMember,
  bandText,
  classifyRole,
  completeMember,
  currentBatch,
  decideModel,
  formatDuration,
  formatPaneRow,
  isFanoutHoldDue,
  isFanoutToastDue,
  isOnModel,
  memberLabel,
  paneRows,
  readConfig,
} from '../lib/policy.ts'

const config = { enforce: true, reviewModel: 'opus', fanoutToast: 4, fanoutHold: 0 }

const member = (over = {}) => ({
  id: 'a1',
  role: 'review',
  label: 'review the diff',
  startedAt: 1_000,
  endedAt: null,
  turn: 't1',
  hasAgentId: true,
  ...over,
})

// Spawn phrasing lifted from the skills the mod serves, as the main agent
// writes the Agent tool's description and prompt when it follows them.
const spawns = {
  reviewPanelCritical: {
    description: 'Critical correctness reviewer',
    prompt:
      'Read the brief at /tmp/claude/review-panel-brief-1714400000.md first. You are one reviewer on a panel: domain focus correctness, intensity critical. Find every weakness. Assume something is wrong. 200-400 words, the review only.',
  },
  reviewPanelSteelman: {
    description: 'Steelman: strongest case for the design',
    prompt: 'Read the brief first. Make the strongest case for this design as a reviewer for senior backend engineers. Review only, no preamble.',
  },
  srePanelAnalyst: {
    description: 'SRE panel: failure mode analyst',
    prompt: 'Role: Senior SRE focused on how this breaks. Adversarial. Assumes everything can fail. Evaluate the artifact below against the failure-mode questions.',
  },
  srePanelReadiness: {
    description: 'Operational Readiness Reviewer',
    prompt: 'Role: Senior SRE focused on whether this can be operated safely by a non-domain-expert at 3 AM. Is there a runbook covering all identified failure modes?',
  },
  srePanelAnalystBare: {
    description: 'Failure mode analyst',
    prompt: 'Role: Senior SRE focused on how this breaks. Adversarial. Assumes everything can fail. List each component and how it fails, with its blast radius. Severity: CRITICAL / HIGH / MEDIUM / LOW.',
  },
  docsReaderTest: {
    description: 'reader-test: how do I install it?',
    prompt: 'You are a fresh reader. Using ONLY the doc content below, answer this one question: how do I install it? If the docs do not say, say so.',
  },
  docsDeepPass: {
    description: 'Explore the architecture for the docs deep pass',
    prompt: 'Map the data flow, the abstractions and the top-level package graph of this repo. Report the conclusion, not file dumps.',
  },
  workOnImpact: {
    description: 'Research: impact and blast radius',
    prompt: 'Blast radius of the change: call sites, consumers, data shape. For each consumer, name a `verify` method: the command, suite or diff that would show it broke. Fill blast_radius.',
  },
  workOnContext: {
    description: 'Research context for MAD-390',
    prompt: 'Read the ticket, linked tickets, PRs and CLAUDE.md. Fill current_state and references.',
  },
  workOnStandardReview: {
    description: 'Review the diff against the coding standards',
    prompt: 'Full diff against coding standards, CLAUDE.md rules, SRP, error handling. Produce findings with file:line and plan_adherence.',
  },
  workOnPlanAdherence: {
    description: '3-agent panel: plan adherence',
    prompt: 'Diff against the ExecutionPlan. DONE, MISSING or DEVIATED per unit.',
  },
  workOnTieBreaker: {
    description: 'Tie-breaker: read both reviews and the changed files',
    prompt: 'Read both reviews and the changed files. Required Changes and Optional Suggestions.',
  },
  workOnChallenger: {
    description: 'Challenger: check this',
    prompt: 'Deep review with file:line references. If fundamentally wrong, explain why.',
  },
  workOnExecutor: {
    description: 'Implement unit 2 of the plan',
    prompt: 'Write the code for unit 2, run the gate, commit on the branch. Verify the suite passes before you return.',
  },
  brainPromoter: {
    description: 'Promote inbox/foo.md',
    prompt:
      'You are a promoter agent. Take ONE verified inbox note and promote it to notes/ with the correct evidence tier and a body that integrates the verification findings. INPUT: inbox_path, verifications: <ClaimVerification[] with verdict, proposed_tier, primary_citations>',
  },
  secondOpinion: {
    description: 'Second opinion on the migration plan',
    prompt: 'Give a second opinion on the plan below.',
  },
  verifier: {
    description: 'Verify the build gate',
    prompt: 'Run make test and report.',
  },
  buildMod: {
    description: 'build-humanizer-gate',
    prompt: 'You are building in an isolated worktree. Build the humanizer-gate mod.',
  },
}

describe('classifyRole', () => {
  test('subagent-review-panel reviewers are review', () => {
    assert.equal(classifyRole(spawns.reviewPanelCritical), 'review')
    assert.equal(classifyRole(spawns.reviewPanelSteelman), 'review')
  })

  test('sre-panel agents are review when the panel or reviewer is named', () => {
    assert.equal(classifyRole(spawns.srePanelAnalyst), 'review')
    assert.equal(classifyRole(spawns.srePanelReadiness), 'review')
  })

  test('an sre-panel analyst named without the panel is other', () => {
    assert.equal(classifyRole(spawns.srePanelAnalystBare), 'other')
  })

  test('docs-writer reader tests and deep passes are other', () => {
    assert.equal(classifyRole(spawns.docsReaderTest), 'other')
    assert.equal(classifyRole(spawns.docsDeepPass), 'other')
  })

  test('work-on review agents, the panel and the challenger are review', () => {
    assert.equal(classifyRole(spawns.workOnStandardReview), 'review')
    assert.equal(classifyRole(spawns.workOnPlanAdherence), 'review')
    assert.equal(classifyRole(spawns.workOnTieBreaker), 'review')
    assert.equal(classifyRole(spawns.workOnChallenger), 'review')
  })

  test('work-on research agents are other even when the prompt says verify', () => {
    assert.equal(classifyRole(spawns.workOnImpact), 'other')
    assert.equal(classifyRole(spawns.workOnContext), 'other')
  })

  test('an executor that verifies its own gate is other', () => {
    assert.equal(classifyRole(spawns.workOnExecutor), 'other')
  })

  test('the brain-research promoter is other despite verified and verification', () => {
    assert.equal(classifyRole(spawns.brainPromoter), 'other')
  })

  test('second opinion, verifier and a hyphenated review description are review', () => {
    assert.equal(classifyRole(spawns.secondOpinion), 'review')
    assert.equal(classifyRole(spawns.verifier), 'review')
    assert.equal(classifyRole({ description: 'review-scaffold', prompt: '' }), 'review')
  })

  test('a build description is other and critical alone never matches', () => {
    assert.equal(classifyRole(spawns.buildMod), 'other')
    assert.equal(classifyRole({ description: 'fix the critical bug', prompt: 'A critical path fails.' }), 'other')
  })

  test('a review word in the description wins over a work word', () => {
    assert.equal(classifyRole({ description: 'Build-gate verifier', prompt: '' }), 'review')
  })

  test('only the first 400 characters of the prompt count', () => {
    const late = { description: 'Explore', prompt: `${'x '.repeat(210)}review this` }
    assert.equal(classifyRole(late), 'other')
    const early = { description: '', prompt: 'Please review this diff.' }
    assert.equal(classifyRole(early), 'review')
  })
})

describe('isOnModel', () => {
  test('matches the alias and a full id', () => {
    assert.equal(isOnModel('opus', 'opus'), true)
    assert.equal(isOnModel('claude-opus-4-1', 'opus'), true)
    assert.equal(isOnModel('sonnet', 'opus'), false)
    assert.equal(isOnModel(undefined, 'opus'), false)
    assert.equal(isOnModel('', 'opus'), false)
  })
})

describe('decideModel', () => {
  test('re-points a reviewer that is not on the review model', () => {
    assert.equal(decideModel({ role: 'review', model: 'sonnet' }, config), 'opus')
    assert.equal(decideModel({ role: 'review' }, config), 'opus')
  })

  test('leaves a reviewer already on the review model', () => {
    assert.equal(decideModel({ role: 'review', model: 'opus' }, config), null)
  })

  test('leaves non-reviewers, teammates and forks alone', () => {
    assert.equal(decideModel({ role: 'other', model: 'sonnet' }, config), null)
    assert.equal(decideModel({ role: 'review', model: 'sonnet', isTeammate: true }, config), null)
    assert.equal(decideModel({ role: 'review', model: 'sonnet', fork: true }, config), null)
  })

  test('does nothing while enforce is off', () => {
    assert.equal(decideModel({ role: 'review', model: 'sonnet' }, { ...config, enforce: false }), null)
  })
})

describe('fan-out thresholds', () => {
  test('the toast is due exactly at the threshold', () => {
    assert.equal(isFanoutToastDue(3, config), false)
    assert.equal(isFanoutToastDue(4, config), true)
    assert.equal(isFanoutToastDue(5, config), false)
    assert.equal(isFanoutToastDue(4, { fanoutToast: 0 }), false)
  })

  test('the hold is off at zero and due at its count', () => {
    assert.equal(isFanoutHoldDue(1, config), false)
    assert.equal(isFanoutHoldDue(3, { fanoutHold: 3 }), true)
    assert.equal(isFanoutHoldDue(4, { fanoutHold: 3 }), false)
  })
})

describe('readConfig', () => {
  test('fills the defaults', () => {
    assert.deepEqual(readConfig({}), config)
  })

  test('takes typed values and string forms', () => {
    assert.deepEqual(readConfig({ enforce: false, reviewModel: 'claude-opus-4-1', fanoutToast: 6, fanoutHold: 3 }), {
      enforce: false,
      reviewModel: 'claude-opus-4-1',
      fanoutToast: 6,
      fanoutHold: 3,
    })
    assert.deepEqual(readConfig({ enforce: 'false', fanoutToast: '2', fanoutHold: '-1', reviewModel: ' ' }), {
      enforce: false,
      reviewModel: 'opus',
      fanoutToast: 2,
      fanoutHold: 0,
    })
  })
})

describe('memberLabel', () => {
  test('is the head of the description, or the agent type', () => {
    assert.equal(memberLabel('  Review the diff  ', 'general-purpose'), 'Review the diff')
    assert.equal(memberLabel('', 'Explore'), 'Explore')
    assert.equal(memberLabel('x'.repeat(100), 'Explore').length, 60)
  })
})

describe('members', () => {
  test('addMember appends and caps the list', () => {
    const list = Array.from({ length: 100 }, (_, i) => member({ id: `m${i}` }))
    const next = addMember(list, member({ id: 'new' }))
    assert.equal(next.length, 100)
    assert.equal(next[99].id, 'new')
    assert.equal(next[0].id, 'm1')
  })

  test('completeMember marks the member by agent id', () => {
    const list = [member({ id: 'a1' }), member({ id: 'a2' })]
    const done = completeMember(list, 'a2', 5_000)
    assert.equal(done[0].endedAt, null)
    assert.equal(done[1].endedAt, 5_000)
  })

  test('completeMember falls back to the oldest running member without an agent id', () => {
    const list = [
      member({ id: 'tool-1', hasAgentId: false, endedAt: 2_000 }),
      member({ id: 'tool-2', hasAgentId: false }),
      member({ id: 'tool-3', hasAgentId: false }),
    ]
    const done = completeMember(list, 'unknown', 5_000)
    assert.equal(done[1].endedAt, 5_000)
    assert.equal(done[2].endedAt, null)
  })

  test('completeMember leaves the list alone with nothing to match', () => {
    const list = [member({ id: 'a1' })]
    assert.deepEqual(completeMember(list, 'zzz', 5_000), list)
  })

  test('currentBatch is the newest turn', () => {
    const list = [member({ id: 'old', turn: 't0' }), member({ id: 'a', turn: 't1' }), member({ id: 'b', turn: 't1' })]
    assert.deepEqual(
      currentBatch(list).map(m => m.id),
      ['a', 'b'],
    )
    assert.deepEqual(currentBatch([]), [])
  })
})

describe('bandText', () => {
  test('is null with no members', () => {
    assert.equal(bandText([], 10_000, config), null)
  })

  test('counts the batch while members run', () => {
    const list = [member({ id: 'a' }), member({ id: 'b', endedAt: 2_000 }), member({ id: 'c' })]
    assert.equal(bandText(list, 3_000, config), 'panel: 1/3 returned · reviewers on opus')
  })

  test('drops the model suffix when enforce is off', () => {
    assert.equal(bandText([member()], 3_000, { ...config, enforce: false }), 'panel: 0/1 returned')
  })

  test('lingers a minute after the last member returned, then clears', () => {
    const list = [member({ id: 'a', endedAt: 10_000 }), member({ id: 'b', endedAt: 20_000 })]
    assert.equal(bandText(list, 20_000 + BAND_LINGER_MS, config), 'panel: 2/2 returned · reviewers on opus')
    assert.equal(bandText(list, 20_001 + BAND_LINGER_MS, config), null)
  })

  test('counts only the newest turn', () => {
    const list = [member({ id: 'old', turn: 't0', endedAt: 2_000 }), member({ id: 'a', turn: 't1' })]
    assert.equal(bandText(list, 3_000, config), 'panel: 0/1 returned · reviewers on opus')
  })
})

describe('pane rows', () => {
  test('formatDuration reads in seconds, then minutes', () => {
    assert.equal(formatDuration(400), '0 s')
    assert.equal(formatDuration(12_400), '12 s')
    assert.equal(formatDuration(65_000), '1m 05s')
  })

  test('paneRows carries role, label, status and duration', () => {
    const list = [member({ id: 'a', startedAt: 1_000 }), member({ id: 'b', role: 'other', label: 'explore', startedAt: 1_000, endedAt: 4_000 })]
    assert.deepEqual(paneRows(list, 13_000), [
      { role: 'review', label: 'review the diff', status: 'running', duration: '12 s' },
      { role: 'other', label: 'explore', status: 'done', duration: '3 s' },
    ])
  })

  test('paneRows keeps the newest members under the limit', () => {
    const list = [member({ id: 'a' }), member({ id: 'b' }), member({ id: 'c', label: 'last' })]
    const rows = paneRows(list, 2_000, 2)
    assert.equal(rows.length, 2)
    assert.equal(rows[1].label, 'last')
  })

  test('formatPaneRow lines the columns up', () => {
    const line = formatPaneRow({ role: 'other', label: 'explore', status: 'done', duration: '3 s' }, 10)
    assert.equal(line, 'other   explore     done     3 s')
  })
})
