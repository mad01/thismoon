import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  BAND_LINGER_MS,
  addMember,
  bandText,
  classifyRole,
  completeMember,
  countsTowardFanout,
  currentBatch,
  decideModel,
  formatDuration,
  isBatchActive,
  isFanoutHoldDue,
  isFanoutToastDue,
  isGeneralPurpose,
  isOnModel,
  memberLabel,
  memberStatus,
  promptLead,
  readConfig,
  staleAfterMs,
} from '../lib/policy.ts'

const config = { enforce: true, reviewModel: 'opus', overrideFrom: ['sonnet'], fanoutToast: 4, fanoutHold: 0, staleMinutes: 30 }
const STALE_MS = 30 * 60_000

const member = (over = {}) => ({
  id: 'a1',
  role: 'review',
  label: 'review the diff',
  startedAt: 1_000,
  endedAt: null,
  turn: 't1',
  ...over,
})

// A pasted artifact that names every review word: the classifier must never
// read it, whether it leads the prompt or follows the instruction.
const artifact = [
  '# RFC: review panel audit of the verifier service',
  '',
  'The panel reviews each audit. A critic verifies the challenger.',
].join('\n')

// Spawn phrasing lifted from the skills the mod serves: the skills' literal
// agent labels as descriptions, and prompts the way the main agent writes
// them when it follows the skill, artifact included.
const spawns = {
  reviewPanelCritical: {
    description: 'Critical correctness reviewer',
    prompt: `Read the brief at /tmp/claude/review-panel-brief-1714400000.md first. Domain focus correctness, intensity critical.\n\n${artifact}`,
  },
  reviewPanelSteelman: {
    description: 'Steelman: strongest case for the design',
    prompt: `You are one reviewer on a panel: make the strongest case for this design for senior backend engineers. 200-400 words, the review only.\n\n${artifact}`,
  },
  sreFailureMode: {
    description: 'Failure Mode Analyst',
    prompt: `**Role:** Senior SRE focused on how this breaks. Adversarial. Assumes everything can fail.\n\n${artifact}`,
  },
  sreReadiness: {
    description: 'Operational Readiness Reviewer',
    prompt: `Role: Senior SRE focused on whether this can be operated safely by a non-domain-expert at 3 AM.\n\n${artifact}`,
  },
  sreDependency: {
    description: 'Dependency Analyst',
    prompt: `${artifact}\n\nRole: Senior SRE mapping the dependency graph and identifying brittleness.`,
  },
  sreToil: {
    description: 'Toil Assessor',
    prompt: `${artifact}\n\nRole: Senior SRE quantifying the operational toil this adds or removes.`,
  },
  docsReaderTest: {
    description: 'reader-test: how do I install it?',
    prompt: `You are a fresh reader. Using ONLY the doc content below, answer this one question: how do I install it?\n\n${artifact}`,
  },
  docsReaderTestDocFirst: {
    description: 'reader-test: what does --foo do?',
    prompt: `${artifact}\n\nAnswer from the doc above only: what does --foo do?`,
  },
  docsDeepPass: {
    description: 'Explore the architecture for the docs deep pass',
    prompt: 'Map the data flow, the abstractions and the top-level package graph of this repo.',
  },
  workOnImpact: {
    description: 'Impact: blast radius, consumers, data shape',
    prompt: 'Blast radius of the change: call sites, consumers, data shape.\nFor each consumer, name a `verify` method: the command, suite or diff that would show it broke.',
  },
  workOnContext: {
    description: 'Research context for MAD-390',
    prompt: 'Read the ticket, linked tickets, PRs and CLAUDE.md. Fill current_state and references.',
  },
  workOnStandardReview: {
    description: 'Review the diff against the coding standards',
    prompt: `Full diff against coding standards, CLAUDE.md rules, SRP, error handling. Produce findings with file:line.\n\n${artifact}`,
  },
  workOnCodeQuality: {
    description: 'Code quality',
    prompt: `You are the code quality reviewer on a 3-agent panel. Review the full diff against standards; file:line for every finding.\n\n${artifact}`,
  },
  workOnPlanAdherence: {
    description: 'Plan adherence',
    prompt: `Diff against the ExecutionPlan. DONE, MISSING or DEVIATED per unit.\n\n${artifact}`,
  },
  workOnSilentValidation: {
    description: 'Silent validation: plan against the research brief',
    prompt: 'Check every unit of the plan against the brief. Report gaps only.',
  },
  workOnTieBreaker: {
    description: 'Tie-breaker: read both reviews and the changed files',
    prompt: 'Required Changes and Optional Suggestions.',
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
    prompt: 'You are a promoter agent. Take ONE verified inbox note and promote it to notes/ with the correct evidence tier and a body that integrates the verification findings.',
  },
  secondOpinion: {
    description: 'Second opinion on the migration plan',
    prompt: 'Give a second opinion on the plan below.',
  },
  verifier: {
    description: 'Verify the build gate',
    prompt: 'Run make test and report.',
  },
  subjectWords: [
    { description: 'Implement review comment threading', prompt: 'Add threads to review comments.' },
    { description: 'Fix cosign verify step', prompt: 'The verify step fails on arm64.' },
    { description: 'Build the panel-policy mod', prompt: 'You are building in an isolated worktree.' },
    { description: 'Update the audit log schema', prompt: 'Add a column.' },
    { description: 'Find the analyst dashboard code', prompt: 'Locate it.' },
  ],
}

describe('promptLead', () => {
  test('is the first non-empty line, capped', () => {
    assert.equal(promptLead('\n\n  Review this.  \nmore'), 'Review this.')
    assert.equal(promptLead('x'.repeat(300)).length, 200)
    assert.equal(promptLead(''), '')
  })
})

describe('classifyRole', () => {
  test('subagent-review-panel reviewers are review, from the label or the prompt lead', () => {
    assert.equal(classifyRole(spawns.reviewPanelCritical), 'review')
    assert.equal(classifyRole(spawns.reviewPanelSteelman), 'review')
  })

  test('sre-panel analysts, assessors and reviewers are review whichever side the artifact sits', () => {
    assert.equal(classifyRole(spawns.sreFailureMode), 'review')
    assert.equal(classifyRole(spawns.sreReadiness), 'review')
    assert.equal(classifyRole(spawns.sreDependency), 'review')
    assert.equal(classifyRole(spawns.sreToil), 'review')
  })

  test('a Role: opener alone makes a reviewer', () => {
    assert.equal(classifyRole({ description: 'SRE agent 1', prompt: 'Role: Senior SRE focused on how this breaks.' }), 'review')
  })

  test('docs-writer reader tests are other even when the pasted doc names every review word', () => {
    assert.equal(classifyRole(spawns.docsReaderTest), 'other')
    assert.equal(classifyRole(spawns.docsReaderTestDocFirst), 'other')
    assert.equal(classifyRole(spawns.docsDeepPass), 'other')
  })

  test('work-on review agents, the panel members and the challenger are review', () => {
    assert.equal(classifyRole(spawns.workOnStandardReview), 'review')
    assert.equal(classifyRole(spawns.workOnCodeQuality), 'review')
    assert.equal(classifyRole(spawns.workOnPlanAdherence), 'review')
    assert.equal(classifyRole(spawns.workOnSilentValidation), 'review')
    assert.equal(classifyRole(spawns.workOnTieBreaker), 'review')
    assert.equal(classifyRole(spawns.workOnChallenger), 'review')
  })

  test('work-on research agents are other even when a later prompt line says verify', () => {
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

  test('a leading work verb wins over review words in the subject', () => {
    for (const spawn of spawns.subjectWords) assert.equal(classifyRole(spawn), 'other', spawn.description)
  })

  test('a review word that is not a leading verb still wins in the description', () => {
    assert.equal(classifyRole({ description: 'Build-gate verifier', prompt: '' }), 'review')
  })

  test('the role clause stops at sentence punctuation', () => {
    const reader = { description: '', prompt: 'You are a fresh reader. Answer the question about the reviewer tool.' }
    assert.equal(classifyRole(reader), 'other')
    assert.equal(classifyRole({ description: '', prompt: 'You are an auditor of this diff.' }), 'review')
  })

  test('critical alone never matches', () => {
    assert.equal(classifyRole({ description: 'the critical bug', prompt: 'A critical path fails.' }), 'other')
  })
})

describe('isOnModel and isGeneralPurpose', () => {
  test('isOnModel matches the alias and a full id', () => {
    assert.equal(isOnModel('opus', 'opus'), true)
    assert.equal(isOnModel('claude-opus-4-1', 'opus'), true)
    assert.equal(isOnModel('sonnet', 'opus'), false)
    assert.equal(isOnModel(undefined, 'opus'), false)
  })

  test('isGeneralPurpose is true for general-purpose and unset', () => {
    assert.equal(isGeneralPurpose('general-purpose'), true)
    assert.equal(isGeneralPurpose(''), true)
    assert.equal(isGeneralPurpose(undefined), true)
    assert.equal(isGeneralPurpose('Explore'), false)
  })
})

describe('decideModel', () => {
  const review = over => ({ role: 'review', subagentType: 'general-purpose', ...over })

  test('moves a general-purpose reviewer with no model, inherit, or a listed model', () => {
    assert.equal(decideModel(review({}), config), 'opus')
    assert.equal(decideModel(review({ model: 'inherit' }), config), 'opus')
    assert.equal(decideModel(review({ model: 'sonnet' }), config), 'opus')
    assert.equal(decideModel(review({ subagentType: '' }), config), 'opus')
  })

  test('never moves a model the caller chose that the key does not list', () => {
    assert.equal(decideModel(review({ model: 'haiku' }), config), null)
    assert.equal(decideModel(review({ model: 'fable' }), config), null)
    assert.equal(decideModel(review({ model: 'claude-fable-5-1' }), config), null)
  })

  test('leaves a reviewer already on the review model', () => {
    assert.equal(decideModel(review({ model: 'opus' }), config), null)
    assert.equal(decideModel(review({ model: 'claude-opus-4-1' }), config), null)
  })

  test('skips Explore, Plan and plugin agents unless the caller set a listed model', () => {
    assert.equal(decideModel(review({ subagentType: 'Explore' }), config), null)
    assert.equal(decideModel(review({ subagentType: 'Plan', model: 'inherit' }), config), null)
    assert.equal(decideModel(review({ subagentType: 'code-review:reviewer' }), config), null)
    assert.equal(decideModel(review({ subagentType: 'Explore', model: 'sonnet' }), config), 'opus')
    assert.equal(decideModel(review({ subagentType: 'code-review:reviewer', model: 'sonnet' }), config), 'opus')
  })

  test('honours an extended overrideFrom list', () => {
    const wide = { ...config, overrideFrom: ['sonnet', 'haiku'] }
    assert.equal(decideModel(review({ model: 'haiku' }), wide), 'opus')
    assert.equal(decideModel(review({ subagentType: 'Explore', model: 'Haiku' }), wide), 'opus')
  })

  test('leaves non-reviewers, teammates and forks alone', () => {
    assert.equal(decideModel({ role: 'other', model: 'sonnet', subagentType: 'general-purpose' }, config), null)
    assert.equal(decideModel(review({ model: 'sonnet', isTeammate: true }), config), null)
    assert.equal(decideModel(review({ model: 'sonnet', fork: true }), config), null)
  })

  test('does nothing while enforce is off', () => {
    assert.equal(decideModel(review({ model: 'sonnet' }), { ...config, enforce: false }), null)
  })
})

describe('fan-out', () => {
  test('only main-loop, non-teammate spawns count', () => {
    assert.equal(countsTowardFanout({}), true)
    assert.equal(countsTowardFanout({ isTeammate: true }), false)
    assert.equal(countsTowardFanout({ parentAgentId: 'sub-1' }), false)
  })

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

  test('takes typed values, string forms and a comma list', () => {
    assert.deepEqual(
      readConfig({ enforce: false, reviewModel: 'claude-opus-4-1', overrideFrom: ['sonnet', ' haiku '], fanoutToast: 6, fanoutHold: 3, staleMinutes: 5 }),
      { enforce: false, reviewModel: 'claude-opus-4-1', overrideFrom: ['sonnet', 'haiku'], fanoutToast: 6, fanoutHold: 3, staleMinutes: 5 },
    )
    assert.deepEqual(
      readConfig({ enforce: 'false', overrideFrom: 'sonnet, haiku,', fanoutToast: '2', fanoutHold: '-1', reviewModel: ' ', staleMinutes: '0' }),
      {
        enforce: false,
        reviewModel: 'opus',
        overrideFrom: ['sonnet', 'haiku'],
        fanoutToast: 2,
        fanoutHold: 0,
        staleMinutes: 0,
      },
    )
  })

  test('staleAfterMs turns the minutes into a budget; 0 stays 0', () => {
    assert.equal(staleAfterMs(config), STALE_MS)
    assert.equal(staleAfterMs({ staleMinutes: 0 }), 0)
  })

  test('an empty overrideFrom list means no listed model moves', () => {
    assert.deepEqual(readConfig({ overrideFrom: [] }).overrideFrom, [])
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

  test('completeMember marks the running member by agent id only', () => {
    const list = [member({ id: 'a1' }), member({ id: 'a2' }), member({ id: 'a3', endedAt: 2_000 })]
    const done = completeMember(list, 'a2', 5_000)
    assert.equal(done[0].endedAt, null)
    assert.equal(done[1].endedAt, 5_000)
    assert.equal(done[2].endedAt, 2_000)
    assert.deepEqual(completeMember(list, 'zzz', 5_000), list)
    assert.deepEqual(completeMember(list, 'a3', 5_000), list)
  })

  test('two returns applied in either order both land', () => {
    const list = [member({ id: 'a' }), member({ id: 'b' })]
    const both = completeMember(completeMember(list, 'a', 5_000), 'b', 5_001)
    assert.deepEqual(both.map(m => m.endedAt), [5_000, 5_001])
  })

  test('currentBatch is the newest turn', () => {
    const list = [member({ id: 'old', turn: 't0' }), member({ id: 'a', turn: 't1' }), member({ id: 'b', turn: 't1' })]
    assert.deepEqual(currentBatch(list).map(m => m.id), ['a', 'b'])
    assert.deepEqual(currentBatch([]), [])
  })
})

describe('isBatchActive and bandText', () => {
  test('nothing is active or drawn with no members', () => {
    assert.equal(isBatchActive([], 10_000), false)
    assert.equal(bandText([], 10_000, config), null)
  })

  test('counts the batch while members run', () => {
    const list = [member({ id: 'a' }), member({ id: 'b', endedAt: 2_000 }), member({ id: 'c' })]
    assert.equal(isBatchActive(list, 3_000), true)
    assert.equal(bandText(list, 3_000, config), 'panel: 1/3 returned · reviewers on opus')
  })

  test('drops the model suffix when enforce is off or no member is a reviewer', () => {
    assert.equal(bandText([member()], 3_000, { ...config, enforce: false }), 'panel: 0/1 returned')
    assert.equal(bandText([member({ role: 'other' })], 3_000, config), 'panel: 0/1 returned')
  })

  test('lingers a minute after the last member returned, then clears', () => {
    const list = [member({ id: 'a', endedAt: 10_000 }), member({ id: 'b', endedAt: 20_000 })]
    assert.equal(bandText(list, 20_000 + BAND_LINGER_MS, config), 'panel: 2/2 returned · reviewers on opus')
    assert.equal(isBatchActive(list, 20_001 + BAND_LINGER_MS), false)
    assert.equal(bandText(list, 20_001 + BAND_LINGER_MS, config), null)
  })

  test('counts only the newest turn', () => {
    const list = [member({ id: 'old', turn: 't0', endedAt: 2_000 }), member({ id: 'a', turn: 't1' })]
    assert.equal(bandText(list, 3_000, config), 'panel: 0/1 returned · reviewers on opus')
  })
})

describe('staleness', () => {
  test('memberStatus is done once returned, stale past the budget, running before it', () => {
    assert.equal(memberStatus(member({ endedAt: 2_000 }), 1_000 + STALE_MS + 1, STALE_MS), 'done')
    assert.equal(memberStatus(member(), 1_000 + STALE_MS, STALE_MS), 'running')
    assert.equal(memberStatus(member(), 1_000 + STALE_MS + 1, STALE_MS), 'stale')
  })

  test('a budget of 0 never marks a member stale', () => {
    assert.equal(memberStatus(member(), 1_000 + 10 * STALE_MS, 0), 'running')
    assert.equal(isBatchActive([member()], 1_000 + 10 * STALE_MS, 0), true)
  })

  test('a stale member leaves the in-flight count and the band names it', () => {
    const now = 1_000 + STALE_MS + 1
    const list = [
      member({ id: 'a', startedAt: now - 20_000, endedAt: now - 10_000 }),
      member({ id: 'b' }),
      member({ id: 'c', startedAt: now - 5_000 }),
    ]
    assert.equal(bandText(list, now, config), 'panel: 1/3 returned, 1 stale · reviewers on opus')
    assert.deepEqual(list.map(m => memberStatus(m, now, STALE_MS)), ['done', 'stale', 'running'])
  })

  test('the band hides once every member is returned past the linger or stale', () => {
    const list = [member({ id: 'a', endedAt: 5_000 }), member({ id: 'b' })]
    const now = 5_000 + BAND_LINGER_MS + STALE_MS
    assert.equal(isBatchActive(list, now, STALE_MS), false)
    assert.equal(bandText(list, now, config), null)
    assert.equal(bandText(list, now, { ...config, staleMinutes: 0 }), 'panel: 1/2 returned · reviewers on opus')
  })

  test('a late return of a stale member still lands and lingers', () => {
    const now = 1_000 + STALE_MS + 1
    const returned = completeMember([member({ id: 'b' })], 'b', now)
    assert.equal(memberStatus(returned[0], now, STALE_MS), 'done')
    assert.equal(bandText(returned, now, config), 'panel: 1/1 returned · reviewers on opus')
  })
})

describe('formatDuration', () => {
  test('reads in seconds, then minutes', () => {
    assert.equal(formatDuration(400), '0 s')
    assert.equal(formatDuration(12_400), '12 s')
    assert.equal(formatDuration(65_000), '1m 05s')
  })
})
