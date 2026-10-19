#!/usr/bin/env node
// Free structural check of the factory eval suites: no model calls, no cost.
//
// It catches what `claude plugin eval` would refuse or quietly mis-grade:
// unknown frontmatter keys, missing tags, grader shapes, and regex patterns
// that do not compile as the JS RegExp the runner builds. Graders that share
// a file name must not drift apart. The regex graders of six cases run over
// the sample messages in ../contract.md (every grader must pass) and over
// broken copies of them (the targeted grader must fail). Every embedded copy
// of the sample task must equal the one in ../contract.md. Each context case
// must quote the brief in ../worker-brief.md word for word, and so must each
// step of the brief that a context rubric quotes.
//
// Usage: node skills/factory/evals/scripts/check-graders.mjs

import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const evalsDir = join(dirname(fileURLToPath(import.meta.url)), '..');
const skillDir = join(evalsDir, '..');

const SUITES = ['brief', 'report', 'round-trip', 'context'];
const PROVENANCE = ['hand-written', 'from-spec', 'from-transcript'];
// The keys the runner accepts in prompt.md frontmatter (claude 2.1.301).
const PROMPT_KEYS = new Set([
  'schema_version', 'name', 'description', 'tags', 'plugins', 'runs', 'expected_outcome',
  'model', 'max_turns', 'timeout_seconds', 'allowed_tools', 'disallowed_tools',
  'artifact_publish', 'growthbook_overrides', 'append_system_prompt', 'env',
]);
const GRADER_KEYS = {
  regex: new Set(['type', 'name', 'target', 'pattern', 'flags', 'match', 'weight', 'arm']),
  llm: new Set(['type', 'name', 'focus', 'weight', 'arm']),
  tool_used: new Set(['type', 'name', 'tool', 'input_match', 'min', 'max', 'weight', 'arm']),
};
// Grader file names whose content must be byte-identical in every case.
const SHARED = new Set([
  'skill-fired', 'warnings-first', 'no-warning-after-steps', 'no-hedging', 'report-format-last',
  'files-changed', 'gates-run', 'fields-in-order', 'question-field', 'not-done-field', 'asks',
  'kind-ready', 'not-ready', 'no-git-option-first',
]);
// Grader file names that hold case-specific facts and may differ.
const PER_CASE = new Set([
  'criteria', 'where', 'routing', 'reply-to', 'commit-range', 'unresolved-term', 'unresolved-none',
  'blocker-verbatim', 'blocker-names-test', 'names-missing-path', 'no-em-dash', 'no-csl-step',
  'lists-extra-file', 'names-section', 'options-field', 'both-options', 'git-commands-field',
  'runs-in-worktree',
]);
// Files that embed the sample task from ../contract.md.
const SAMPLE_COPIES = [
  '../SKILL.md',
  'brief/brief-spec-sample/graders/criteria.md',
  'report/report-ready-unresolved-term/prompt.md',
  'round-trip/roundtrip-sample-done/prompt.md',
  'round-trip/roundtrip-sample-not-done/prompt.md',
  'context/context-denial/prompt.md',
];
const SAMPLE_RE = /kind: task\nunit: docs-sweep\/events\n[\s\S]*?List each term you could not resolve\.\n/g;

const problems = [];
const fail = (where, msg) => problems.push(`${where}: ${msg}`);

function parseValue(raw, where) {
  if (raw.startsWith("'")) return raw.slice(1, -1).replaceAll("''", "'");
  if (raw.startsWith('"')) return JSON.parse(raw);
  if (raw.startsWith('[')) return raw.slice(1, -1).split(',').map((s) => s.trim()).filter(Boolean);
  if (/^\d+$/.test(raw)) return Number(raw);
  if (raw.includes(': ') || raw.includes(' #')) fail(where, `bare value YAML would misread: ${raw}`);
  return raw;
}

function parseFrontmatter(text, where) {
  const m = text.match(/^---\n([\s\S]*?)\n---\n?([\s\S]*)$/);
  if (!m) {
    fail(where, 'no frontmatter');
    return { fm: {}, body: text };
  }
  const fm = {};
  for (const line of m[1].split('\n')) {
    const kv = line.match(/^([\w-]+):\s*(.*)$/);
    if (kv) fm[kv[1]] = parseValue(kv[2], where);
    else fail(where, `unparsed frontmatter line: ${line}`);
  }
  return { fm, body: m[2] };
}

function checkPrompt(suite, caseName, names) {
  const where = `${suite}/${caseName}/prompt.md`;
  const { fm, body } = parseFrontmatter(readFileSync(join(evalsDir, suite, caseName, 'prompt.md'), 'utf8'), where);
  for (const key of Object.keys(fm)) if (!PROMPT_KEYS.has(key)) fail(where, `unknown key ${key}`);
  if (fm.name !== caseName) fail(where, `name ${fm.name} differs from the directory`);
  if (names.has(fm.name)) fail(where, `duplicate case name ${fm.name}`);
  names.add(fm.name);
  const tags = fm.tags ?? [];
  if (tags.length !== 2 || tags[0] !== suite || !PROVENANCE.includes(tags[1])) {
    fail(where, `tags must be [${suite}, <provenance>], got [${tags.join(', ')}]`);
  }
  if (!body.trim()) fail(where, 'empty prompt');
  return { tags, body };
}

function compile(pattern, flags, where) {
  try {
    return new RegExp(pattern, flags);
  } catch (err) {
    fail(where, `pattern does not compile: ${err.message}`);
    return null;
  }
}

function checkGrader(file, where) {
  const raw = readFileSync(file, 'utf8');
  const { fm, body } = parseFrontmatter(raw, where);
  const allowed = GRADER_KEYS[fm.type];
  if (!allowed) return fail(where, `unknown grader type ${fm.type}`);
  for (const key of Object.keys(fm)) if (!allowed.has(key)) fail(where, `key ${key} is not valid for ${fm.type}`);
  const grader = { name: where.split('/').pop().replace(/\.md$/, ''), raw, where, ...fm };
  if (fm.type === 'regex') {
    grader.flags = fm.flags ?? '';
    grader.match = fm.match ?? 'contains';
    if (!/^[dgimsuvy]*$/.test(grader.flags)) fail(where, `bad flags ${grader.flags}`);
    if (!/^(contains|not_contains|count:\d+)$/.test(grader.match)) fail(where, `bad match ${grader.match}`);
    grader.re = compile(fm.pattern ?? '', grader.flags, where);
  }
  if (fm.type === 'llm' && !body.trim()) fail(where, 'llm grader without criteria');
  if (fm.type === 'tool_used' && fm.input_match) compile(fm.input_match, '', where);
  return grader;
}

function loadGraders(suite, caseName) {
  const dir = join(evalsDir, suite, caseName, 'graders');
  if (!existsSync(dir)) {
    fail(`${suite}/${caseName}`, 'no graders/ directory');
    return [];
  }
  const graders = readdirSync(dir).filter((f) => f.endsWith('.md')).sort()
    .map((f) => checkGrader(join(dir, f), `${suite}/${caseName}/graders/${f}`)).filter(Boolean);
  // scripts/ste-pass.sh reads the reply from the llm grader's evidence.
  if (graders.filter((g) => g.type === 'llm').length !== 1) fail(`${suite}/${caseName}`, 'needs exactly one llm grader');
  return graders;
}

const headerPattern = (kind, reply) => `^\\s*kind: ${kind}\\nunit: \\S+\\nworktree: \\S+\\nbranch: \\S+\\n`
  + `model: (opus|sonnet|haiku)\\neffort: (low|medium|high)\\nreply_to: \\S+\\nneeds_reply: ${reply}\\n`;
const sentencePattern = (words) => `(?:[^\\s.!?]+[ \\t]+){${words}}[^\\s.!?]+`;

// header and sentence-length vary on purpose, but only in their one slot.
function checkTemplated(g, suite) {
  if (g.name === 'header') {
    const m = (g.pattern ?? '').match(/^\^\\s\*kind: (.+?)\\nunit:.*needs_reply: (.+?)\\n$/);
    if (!m || g.pattern !== headerPattern(m[1], m[2]) || g.flags || g.match !== 'contains') fail(g.where, 'header drifted from the template');
  } else if (g.pattern !== sentencePattern(suite === 'brief' ? 20 : 25) || g.flags || g.match !== 'not_contains') {
    fail(g.where, 'sentence-length drifted from the template');
  }
}

function checkGraderDrift(gradersByCase) {
  const firstByName = new Map();
  for (const [caseId, graders] of gradersByCase) {
    for (const g of graders) {
      if (g.name === 'header' || g.name === 'sentence-length') checkTemplated(g, caseId.split('/')[0]);
      else if (SHARED.has(g.name)) {
        const first = firstByName.get(g.name);
        if (!first) firstByName.set(g.name, g);
        else if (first.raw !== g.raw) fail(g.where, `differs from ${first.where}`);
      } else if (!PER_CASE.has(g.name)) fail(g.where, 'grader name in neither SHARED nor PER_CASE');
    }
  }
}

// The runner's regex semantics: test() for contains, the g-flag match count for count:N.
function gradeRegex(grader, text) {
  if (!grader.re) return false;
  if (grader.match === 'contains') return grader.re.test(text);
  if (grader.match === 'not_contains') return !grader.re.test(text);
  const flags = grader.flags.includes('g') ? grader.flags : `${grader.flags}g`;
  return (text.match(new RegExp(grader.re.source, flags)) ?? []).length === Number(grader.match.slice(6));
}

// Samples sit in ``` fences, or ```` when they contain a fenced block.
function contractSamples() {
  const text = readFileSync(join(skillDir, 'contract.md'), 'utf8');
  const samples = {};
  for (const [, , block] of text.matchAll(/^(`{3,4})\n(kind: [\s\S]*?)^\1$/gm)) samples[block.match(/^kind: (\w+)/)[1]] = block;
  return samples;
}

function checkSampleCopies(task) {
  for (const rel of SAMPLE_COPIES) {
    const copies = readFileSync(join(evalsDir, rel), 'utf8').match(SAMPLE_RE) ?? [];
    if (copies.length !== 1) fail(rel, `expected one copy of the sample task, found ${copies.length}`);
    else if (copies[0] !== task) fail(rel, 'sample task differs from contract.md');
  }
}

function briefText() {
  const text = readFileSync(join(skillDir, 'worker-brief.md'), 'utf8');
  return text.match(/^```text\n([\s\S]*?)\n```$/m)[1];
}

const words = (n) => Array.from({ length: n }, (_, i) => `word${i + 1}`).join(' ');
const moveWarningsDown = (t) => {
  const warnings = t.match(/^Do not .*\n/gm).join('');
  return t.replace(/^Do not .*\n/gm, '').replace(/^Report format:/m, `${warnings}\nReport format:`);
};
const moveReportFormatUp = (t) => {
  const line = t.match(/^Report format:[\s\S]*$/m)[0];
  return t.replace(line, '').replace(/^1\. /m, `${line.trim()}\n\n1. `);
};
const swapFirstTwoLines = (t) => t.replace(/^(.*)\n(.*)\n/, '$2\n$1\n');
const onlyKindLine = (t) => t.replace(/^kind: task\n[\s\S]*?\n\n/, 'kind: task\n\n');
const swapModelAndEffort = (t) => t.replace(/^model: (.*)\neffort: (.*)$/m, 'model: $2\neffort: $1');
const swapFilesAndGates = (t) => {
  const files = t.match(/^files changed:\n(?:- .*\n)+/m)[0];
  const gates = t.match(/^gates run:\n(?:- .*\n)+/m)[0];
  return t.replace(files, '\u0000').replace(gates, files).replace('\u0000', gates);
};
const unfenceDenial = (t) => t.replace(/^```\n(.*\n)```\n/m, '$1');
const dropDenialBlock = (t) => t.replace(/^```\n.*\n```\n/m, '');
const emptyBlockerLine = (t) => t.replace(/^blocker: .*$/m, 'blocker: ');
// context-denial's fixture is the blocked sample with another unit and denial.
const toDocsSweepDenial = (t) => t.replaceAll('csl/prune-shards', 'docs-sweep/events')
  .replace('make -C services/csl install', 'humanizer detect services/events/README.md');
// context-plain-git's fixture is the ready sample with another unit and range,
// and a git commands field in place of the unresolved terms.
const PLAIN_GIT_WT = '~/.worktrees/thismoon/alexander-adr-links-catalog';
const toCatalogReady = (t) => t.replaceAll('docs-sweep/events', 'adr-links/catalog')
  .replaceAll('docs-sweep-events', 'adr-links-catalog')
  .replace('3f1c2ab..9e04d17', '4b7e0c2..e91d3a5')
  .replace(/^unresolved terms:[\s\S]*$/m, [
    'git commands:',
    `- \`git -C ${PLAIN_GIT_WT} add services/catalog/README.md\``,
    `- \`git -C ${PLAIN_GIT_WT} commit -m "docs(catalog): fix the ADR link"\``,
    '',
  ].join('\n'));
// -C with the worktree path is the required form; any other option before the
// subcommand, on either side of -C, and a cd in its place, must fail. So must
// a -C to another dir with the worktree path later on the line.
const optionBeforeSubcommand = (t) => t.replace(`git -C ${PLAIN_GIT_WT} add`, `git --no-pager -C ${PLAIN_GIT_WT} add`);
const configAfterDirOption = (t) => t.replace(`git -C ${PLAIN_GIT_WT} commit`, `git -C ${PLAIN_GIT_WT} -c k=v commit`);
const noPagerAfterDirOption = (t) => t.replace(`git -C ${PLAIN_GIT_WT} add`, `git -C ${PLAIN_GIT_WT} --no-pager add`);
const worktreeNotTheDirOption = (t) => t.replaceAll(`git -C ${PLAIN_GIT_WT} `, `git -C ~/code --work-tree ${PLAIN_GIT_WT} `);
const dropDirOption = (t) => t.replaceAll(`git -C ${PLAIN_GIT_WT} `, 'git ');
const cdInsteadOfDirOption = (t) => t.replaceAll(`git -C ${PLAIN_GIT_WT} `, `cd ${PLAIN_GIT_WT} && git `);
const dropGitCommands = (t) => t.replace(/^git commands:[\s\S]*$/m, '');

// Each case with a blocker-verbatim grader, and how the blocked sample fits it.
const BLOCKER_CASES = [
  ['report/report-blocked-denial', (t) => t],
  ['context/context-no-relay', (t) => t],
  ['context/context-denial', toDocsSweepDenial],
];
const BLOCKER_VARIANTS = [
  [unfenceDenial, 'blocked sample with the denial out of its fence'],
  [dropDenialBlock, 'blocked sample without the fenced denial'],
  [emptyBlockerLine, 'blocked sample with an empty blocker line'],
];
const blockerChecks = ([caseId, fit]) => [
  [caseId, 'blocked', null, fit],
  ...BLOCKER_VARIANTS.map(([mutate, label]) => [caseId, 'blocked', 'blocker-verbatim', (t) => mutate(fit(t)), label]),
];

// [case, sample kind, grader that must fail, mutation, label]; a null grader means
// every grader must pass.
const SAMPLE_CHECKS = [
  ['brief/brief-spec-sample', 'task', null, (t) => t],
  ['report/report-ready-unresolved-term', 'ready', null, (t) => t],
  ...BLOCKER_CASES.flatMap(blockerChecks),
  ['brief/brief-spec-sample', 'task', null, (t) => t.replace(/^2\. .*$/m, `2. ${words(20)}.`), 'task sample with a 20-word step'],
  ['brief/brief-spec-sample', 'task', null, (t) => t.replace(/^1\. .*$/m, '1. Open release-please-config.json.'), 'task sample naming release-please-config.json'],
  ['brief/brief-spec-sample', 'task', 'sentence-length', (t) => t.replace(/^2\. .*$/m, `2. ${words(21)}.`)],
  ['brief/brief-spec-sample', 'task', 'header', swapFirstTwoLines],
  ['brief/brief-spec-sample', 'task', 'header', onlyKindLine, 'task sample with only the kind line'],
  ['brief/brief-spec-sample', 'task', 'header', swapModelAndEffort, 'task sample with model and effort swapped'],
  ['brief/brief-spec-sample', 'task', 'routing', swapModelAndEffort, 'task sample with model and effort swapped'],
  ['brief/brief-spec-sample', 'task', 'routing', (t) => t.replace('effort: medium', 'effort: low')],
  ['brief/brief-spec-sample', 'task', 'warnings-first', moveWarningsDown],
  ['brief/brief-spec-sample', 'task', 'no-warning-after-steps', moveWarningsDown],
  ['brief/brief-spec-sample', 'task', 'no-hedging', (t) => t.replace('2. Reorder', '2. Please reorder')],
  ['brief/brief-spec-sample', 'task', 'report-format-last', moveReportFormatUp],
  ['report/report-ready-unresolved-term', 'ready', 'commit-range', (t) => t.replace(/^commit range: .*\n/m, '')],
  ['report/report-ready-unresolved-term', 'ready', 'fields-in-order', swapFilesAndGates],
  ['report/report-ready-unresolved-term', 'ready', 'reply-to', (t) => t.replace('#task-1', '')],
  ['report/report-blocked-denial', 'blocked', 'question-field', (t) => t.replace(/\?$/m, '.')],
  ['report/report-blocked-denial', 'blocked', 'blocker-verbatim', (t) => t.replace('has been denied', 'was refused')],
  ['context/context-plain-git', 'ready', null, toCatalogReady, 'ready sample fitted to context-plain-git'],
  ['context/context-plain-git', 'ready', 'no-git-option-first', (t) => optionBeforeSubcommand(toCatalogReady(t)), 'fitted ready sample with an option before the git subcommand'],
  ['context/context-plain-git', 'ready', 'no-git-option-first', (t) => configAfterDirOption(toCatalogReady(t)), 'fitted ready sample with -c after -C'],
  ['context/context-plain-git', 'ready', 'no-git-option-first', (t) => noPagerAfterDirOption(toCatalogReady(t)), 'fitted ready sample with --no-pager after -C'],
  ['context/context-plain-git', 'ready', 'runs-in-worktree', (t) => dropDirOption(toCatalogReady(t)), 'fitted ready sample with no -C to the worktree'],
  ['context/context-plain-git', 'ready', 'runs-in-worktree', (t) => worktreeNotTheDirOption(toCatalogReady(t)), 'fitted ready sample with -C to another dir and the worktree later on the line'],
  ['context/context-plain-git', 'ready', 'runs-in-worktree', (t) => cdInsteadOfDirOption(toCatalogReady(t)), 'fitted ready sample with cd in place of -C'],
  ['context/context-plain-git', 'ready', 'git-commands-field', (t) => dropGitCommands(toCatalogReady(t)), 'fitted ready sample without the git commands field'],
];

function runSampleChecks(gradersByCase, samples) {
  for (const [caseId, kind, target, mutate, note] of SAMPLE_CHECKS) {
    const sample = samples[kind];
    if (!sample) {
      fail('contract.md', `no ${kind} sample`);
      continue;
    }
    const results = gradersByCase.get(caseId).filter((g) => g.type === 'regex')
      .map((g) => [g.name, gradeRegex(g, mutate(sample))]);
    const label = note ?? (target ? `${kind} sample without ${target}` : `${kind} sample`);
    for (const [name, passed] of results) {
      if (target === null && !passed) fail(caseId, `${label} fails ${name}`);
      if (target === name && passed) fail(caseId, `${label} still passes ${name}`);
    }
    if (target && !results.some(([name]) => name === target)) fail(caseId, `no grader named ${target}`);
  }
}

function checkContextBrief(suite, caseName, body) {
  const m = body.match(/<subagent-start>\nbelt\[factory-worker\]: ([\s\S]*?)\n<\/subagent-start>/);
  if (!m) return fail(`${suite}/${caseName}`, 'no <subagent-start> block');
  if (m[1] !== briefText()) fail(`${suite}/${caseName}`, 'quoted brief differs from worker-brief.md');
}

// The judge sees only the reply, so a rubric repeats the steps of the brief
// it grades against. Each quoted step must match the brief word for word.
function checkRubricStepQuotes(graders) {
  const brief = briefText();
  for (const g of graders.filter((x) => x.type === 'llm')) {
    const quotes = [...g.raw.matchAll(/step (\d+) of the brief says "([^"]*)"/g)];
    const mentions = g.raw.match(/step \d+ of the brief/g) ?? [];
    if (mentions.length !== quotes.length) fail(g.where, 'names a step of the brief without the quote form step N of the brief says "..."');
    for (const [, n, quote] of quotes) {
      const step = brief.match(new RegExp(`^${n}\\. (.*)$`, 'm'));
      if (step?.[1] !== quote) fail(g.where, `step ${n} quote differs from worker-brief.md`);
    }
  }
}

const names = new Set();
const gradersByCase = new Map();
let graderCount = 0;
for (const suite of SUITES) {
  for (const caseName of readdirSync(join(evalsDir, suite)).sort()) {
    const { body } = checkPrompt(suite, caseName, names);
    const graders = loadGraders(suite, caseName);
    graderCount += graders.length;
    gradersByCase.set(`${suite}/${caseName}`, graders);
    if (suite === 'context') {
      checkContextBrief(suite, caseName, body);
      checkRubricStepQuotes(graders);
    }
  }
}
const samples = contractSamples();
checkGraderDrift(gradersByCase);
checkSampleCopies(samples.task);
runSampleChecks(gradersByCase, samples);

console.log(`${names.size} cases, ${graderCount} graders, ${SAMPLE_CHECKS.length} sample checks, ${SAMPLE_COPIES.length} sample copies`);
for (const p of problems) console.log(`FAIL ${p}`);
process.exit(problems.length ? 1 : 0);
