# present looks eval

Measures how good the pages and decks the `present` skill produces look, from the real rendered page. The cases one directory up run under `claude plugin eval`, whose judge reads only the reply text; this harness renders every page in a scratch `present serve`, measures it in the browser, and has a vision judge score the screenshots. Run it before and after a change to the skill or the renderer, then compare the two runs blind.

Python 3, standard library only, macOS (`sips` makes the scaled overviews). It needs `claude` logged in, `go` to build present from this checkout, `agent-browser`, and the present asset cache at `~/.config/present/assets` (`make cache` in `services/present`).

## Steps

Run from this directory. Every step is resumable: it skips work whose output already exists in the run directory, so a step that stopped half way picks up where it left off.

```bash
python3 run.py generate --label before          # new run: 8 cases x 2 models, one claude -p child each
python3 run.py render   --run latest            # create each page in a scratch present, screenshot and probe it
python3 run.py metrics  --run latest            # mechanical proxies per page, probes included
python3 run.py judge    --run latest            # checks and rubric scores, three votes per page
python3 run.py report   --run latest            # report.md and contact-sheet.html
```

Read the results in that order too: the probes, the mechanical metrics and the yes/no checks first, the 1 to 5 scores last. The scores come from one vision model reading screenshots; treat them as a filter that points at pages to open, not the verdict.

`--run` takes a stamp (`20261112-220339`), a path, or `latest` (the newest generate or redraw run; pairwise runs are skipped). `generate` without `--run` starts a new run; with `--run` it fills in what is missing, and `--redo-errors` retries children that ended in an infra error (a timeout, a failed login). `render --redo` takes the screenshots again, and `render --cases a,b` limits it to some cases. `--cases a,b` and `--models x,y` narrow a new run.

`probe --run <stamp>` takes the page probes alone, without shots, for a run rendered before the probes existed. It uses the binary that took the run's shots (from `render/present.json`, rebuilt from its commit if the file is gone) and the run's face, so the values describe the pages the judge saw.

## Comparisons

Three comparisons, each with its own arms:

1. **Renderer, same Docs.** One run's generated Docs drawn by two present binaries, with the reading face held equal. No generation cost.
2. **Skill, same binary.** Two skill directories, each generated and then drawn by the same binary.
3. **End to end.** Main against a branch as a reader would see it, face included.

### Renderer only: two render slots

`redraw` is the second render slot. It copies a generate run's Docs (`gen/`, the stream log left out) into a new run, records where they came from (`gen_from`), and renders them with the binary and face you name. Run it once per binary, with the same `--font`:

```bash
python3 run.py redraw 20261112-220339 --label main   --present-ref main --font Lexend
python3 run.py redraw 20261112-220339 --label branch --font Lexend     # this checkout, uncommitted changes included
python3 run.py metrics --run <main-stamp>   && python3 run.py report --run <main-stamp>
python3 run.py metrics --run <branch-stamp> && python3 run.py report --run <branch-stamp>
python3 run.py pairwise results/<main-stamp> results/<branch-stamp> --label renderer
```

`--present-bin <path>` takes a built binary. `--present-ref <ref>` builds one from any git ref of this repo through `git archive` into `<scratch>/refs/src/<sha>`, so the checkout is never touched. That binary is kept per commit under `<scratch>/refs/bin/`. With neither flag, present is built from this checkout. A build with uncommitted changes under `services/present`, `webkit`, `buildinfo` or `kit` reports a version ending in `-dirty`. Every build stamps the commit and version through `-ldflags`. `redraw --cases a,b --models x` draws some of the Docs only. The binary, ref and face a run was rendered with stay in its `run.json` under `render`, so `render --run <stamp>` resumes with the same ones. Flags given again replace them; re-render with `--redo` after changing the face.

### Holding the face equal

`--font` on `render`, `redraw` and `probe` stores the reading face before each page loads, the way the font control in the page header does. The key is `webkit-font` in localStorage, and the faces are Fira Code, Inter, Lexend and Work Sans. Without it the key is removed and the page shows its own default. The render checks the face the page body uses and fails the page when it is not the one asked for. A face webkit does not know cannot fall back unseen. A shot also waits for the page's web fonts to finish loading.

### Skill, same binary

```bash
python3 run.py generate --label old --skill-dir /path/to/old/present
python3 run.py generate --label new --skill-dir /path/to/new/present
python3 run.py render --run <old-stamp> --present-ref <ref> --font Lexend
python3 run.py render --run <new-stamp> --present-ref <ref> --font Lexend
python3 run.py metrics --run <old-stamp> && python3 run.py metrics --run <new-stamp>
python3 run.py pairwise results/<old-stamp> results/<new-stamp> --label skill
```

An arm is a skill directory: copy `skills/present` somewhere, edit it, and point `--skill-dir` at it. Its fingerprint is a sha256 over `SKILL.md`, `examples/**`, `references/**` and `gallery/**`: each file's path relative to the skill directory and its bytes, in path order. A renamed or added reference changes it as an edit does. Runs from before this scheme carry only the older name-only hash over `SKILL.md` and `examples/*`, shown as `v1:<hash>`; the baseline run also has the new one, computed from git at its commit.

### Pairwise

`pairwise` writes its own run directory (`--cases a,b` and `--models x,y` to compare part of it); `pairwise --run <stamp>` resumes one that stopped. For each case and model both runs rendered, the judge sees the two pages as `page-1` and `page-2`, with no arm name in any path, and says which looks better and why. Every vote is two calls, one with A as `page-1` and one with B as `page-1`. A vote goes to an arm only when the judge picks it in both orders; a split, or a tie in either order, is a tie. That cancels the judge's lean toward one position. Three votes per pair, and the arm that wins a majority of the votes wins the pair.

Two kinds of pair are never decided. A failed pair has a side that was refused, skipped (its generation failed) or hit a browser error. It is counted on its own line (A failed, B failed, both failed), never as a win, and costs no judge call. An incomplete pair has fewer complete votes than three, or than `--votes` when that is higher. A spend cap or a failed call can leave a pair there, and `pairwise --run` fills in the missing calls.

`run.json` and `report.md` name both arms: skill fingerprint, present version and binary hash, the face asked for and the face the pages used, the feedback turn, and where the Docs came from. The report says which of these differ between the arms, and counts wins, losses, ties, incomplete and failed pairs per model, decks and briefs apart.

`--judge-model` works as in `judge` below: `cross` (the default) has Opus judge the Fable pages and Fable judge the Opus pages.

## Cost and spend cap

Every paid call is appended to `spend.jsonl` in the run directory. `--max-usd` (default 20) stops starting new paid calls once the run's total reaches it; calls already running finish. Each child also carries `--max-budget-usd` (4 for generation, 2 for a judge vote).

`pairwise` counts its judge calls before it starts and prints them with the expected cost. The price of a call is the mean of the judge calls in earlier pairwise ledgers under `results/`, or $0.25 when there are none. When what the run has spent plus that estimate passes `--max-usd`, it stops before the first call instead of cutting the run midway.

The baseline run, 5 cases on 2 models, `cross` judging:

| Step | Cost | Wall time |
|---|---|---|
| generate, 10 children, 4 at a time | $4.47 | about 5 min |
| render, 10 pages with figure shots | none | about 8 min |
| probe, 10 pages, no shots | none | about 2 min |
| judge, 22 votes, 4 at a time (stopped by the cap; 30 votes would be about $9.30) | $6.79 | about 3 min |

A generation costs $0.22 to $0.54 once the skill text is cached, and about $1.00 for the first child of a run, which writes that cache; a feedback turn adds to that. What a judge call costs depends on the judge model:

| Judge | Per absolute vote | Per pairwise call |
|---|---|---|
| `claude-opus-5-5` | $0.10 to $0.32 | about $0.26 |
| `claude-fable-5-2` | $0.34 to $0.67 | not measured, about twice Opus |

A pairwise vote is two calls. Three votes over all 16 pairs of 8 cases on 2 models is 96 calls, about $37 under `cross`; `--votes 1` never decides a pair (see Pairwise), so narrow with `--models` or `--cases` instead.

## Layout

| Path | What it is |
|---|---|
| `run.py` | The command line: one subcommand per step |
| `common.py` | Paths, JSON files, run directories, the spend ledger, the skill fingerprint, the known faces |
| `cases.py` | Loads `cases/<id>/case.json` and `source.md` |
| `claude_cli.py` | Runs one isolated `claude -p` child: one JSON result (judge), or a stream-json conversation (generation) |
| `generate.py` | Step 1: prompt, child, read log, feedback turn, reply parsing |
| `present_io.py` | Builds present (checkout or git ref), runs the scratch serve, creates pages through `present mcp` |
| `browser.py` | agent-browser from Python: viewport, theme and face, render markers, screenshots |
| `render.py` | Step 2: page creation, the screenshot set and the probes; `probe` without shots |
| `probes.py` | The page probes: the script run in the page, the per-page summary, the probe tables |
| `metrics.py` | Step 3: proxies from the Doc JSON and the rendered page |
| `judge.py` | Steps 4 and 5: absolute votes (checks and scores) and the blind pairwise compare in both orders |
| `report.py` | Step 6: `report.md` and `contact-sheet.html` |
| `rubric.md` | The judge's criteria, scored 1 to 5 with anchors for 1, 3 and 5, and the yes/no checks; `judge.py` parses its `## N. id: title` headings and `Check` lines, so keep that shape |
| `cases/` | The eight cases |
| `results/` | One directory per run, ignored by git |

## Cases

Each case is real material from this repo, copied whole at a pinned commit (`case.json` records the path and SHA; none needed trimming), plus a one-line ask in the user's voice.

| Case | Source | Kind | What it stresses |
|---|---|---|---|
| `adr-sandbox-runtime` | `docs/adr/0028-sandbox-runtime-replaces-seatbelt.md` | brief | a decision record: options, trade-offs, consequences |
| `release-process` | `docs/RELEASING.md` | brief | a process: ordered steps, checks, failure paths |
| `memory-stack` | `docs/MEMORY-STACK.md` | brief | a system overview: stores, hooks and how they depend on each other |
| `csl-ask-baseline` | `evals/harness/results/2026-11-01-csl-ask.md` | brief | a change summary with real numbers: four runs, cost, turns and score deltas |
| `getting-started-adhd` | `docs/GETTING-STARTED.md` | brief | the skill's ADHD briefing mode, asked for in the user's voice ("adhd mode please"): the next action first, numbered steps, short lists |
| `fleet-talk-deck` | `docs/HOW-IT-FITS-TOGETHER.md` | deck | a deck for a room: one idea per slide, legible from the back |
| `brain-cli-review-deck` | `evals/harness/results/2026-11-05-brain-cli.md` | deck | a review deck: findings, the fixes they led to, pass counts before and after |
| `effort-sweep-talk-deck` | `services/csl/evals/ask/results/2026-11-01-haiku55-effort-sweep-claude.md` | deck | a talk carried by real numbers: pass rate, latency, tokens and cost over five effort levels |

To add one, make `cases/<id>/` with `source.md` copied from git (`git show <sha>:<path>`) and a `case.json` with `id`, `kind` (`brief` or `deck`), `ask`, `stresses` and `source` (`repo`, `path`, `commit`, `trimmed`). An optional `mode` (`adhd`) records that the ask turns on a skill mode; the ask itself carries the trigger.

## Step 1: generate

One `claude -p` child per case and model, isolated the way `evals/harness` isolates its runs: `--setting-sources ""` (no CLAUDE.md, hooks, permission rules or user MCP servers; the login stays), `--strict-mcp-config` with no servers, hooks and auto memory off through `--settings`, `--no-session-persistence`, and the variables that nest a child in the calling session stripped. The child runs in an empty scratch directory with the Read tool only, in `dontAsk` mode, with `--add-dir` on the skill directory.

The whole SKILL.md goes in through `--append-system-prompt-file`, saying where the skill directory is, so the child can Read the examples the way a real session does. The prompt is the case's ask, the source, and the reply rule the plugin-eval cases use: the MCP is not available, so reply with the `present_create` arguments as one JSON object in a fenced block. A reply with no parseable object is recorded as `invalid` with the parse error.

The child runs with `--input-format stream-json --output-format stream-json --verbose`. That prints every event, tool calls included, and keeps the conversation open for a second turn without a session file. `stream.jsonl` keeps the events; `gen.json` lists every Read with its path (relative to the skill directory when inside it), the turn, and whether it returned the file. The report gives, per skill file and model, how many children read it; runs from before the log say "not logged".

`--feedback` adds one second turn in the same conversation, recorded in `run.json` and in each `gen.json`:

- `lint --lint-cmd '<command>'` runs the command once over the turn-1 Doc and sends its output back, asking for a revised Doc. `{args}`, `{content}` and `{deck}` in the command become the paths of `args-turn1.json` and the turn-1 content and deck Docs; the output is kept in `lint.txt`.
- `neutral` sends "Review your page once and revise it" with no findings. It is the control arm for `lint`.

The revised arguments become `args.json`; the turn-1 ones stay in `args-turn1.json`. A second turn that errors or gives no arguments keeps turn 1, and `gen.json` says so; a turn 1 with no arguments gets no second turn. `cost_usd` is the session total, which the last result event carries.

Per child, under `gen/<case>/<model>/`: `system.md`, `prompt.md`, `reply.md`, `args.json` (the arguments, `content`, `deck` and `graph` as strings), `stream.jsonl`, and `gen.json` (status, cost, turns, duration, usage, reads, feedback); with a feedback turn also `reply-turn1.md`, `args-turn1.json` and `prompt-turn2.md`.

## Step 2: render

Builds present (see the renderer-only comparison above for `--present-ref` and `--present-bin`), starts `present serve` on port 7512 (`--port`) with `PRESENT_WORKDIR` under `/tmp/present-looks/harness/<stamp>/`, the asset cache linked in and read-aloud off, and stops it at the end. Each page goes in through `present mcp`'s `present_create`, so the tool's own checks run; a refusal is recorded with the tool's error text. present stores the HTML it compiles at create time, so a page made by another binary is created again (`page.json` records the binary's sha256).

agent-browser drives one headless session at 1440x900. The theme and face are the reader's own settings: `webkit-theme` and `webkit-font` in localStorage, with the `webkit-palette-*` keys removed so the default palette shows. They apply at load, so the harness opens the server's index, sets them, then opens the page. Before each shot the harness waits for the page's render markers: every chart and diagram built, no diagram entry animation running, the graph `.ready`, the web fonts loaded; then it settles 1.5 s. It does not wait for images: an image that never loads (a remote URL, say) is counted as missing, not treated as a failed render. Scrolls use `behavior: 'instant'` because a smooth scroll never finishes headless.

Chart.js draws its charts over about a second after they are built, and no marker says when it is done. A full-page shot catches charts half drawn: a line with one point, an empty sankey, a doughnut still spinning. So every chart, diagram and graph also gets its own shot: the harness scrolls it to the middle of the viewport, waits 3 s, and takes a 1440x900 shot. The judge reads charts from those shots only.

A deck is opened at `#1` and put into presenting mode with the `p` key, the same as the Present button. It is stepped by setting `location.hash` to each step number, so every step of every slide is shown. Before each shot the harness checks the active slide number.

Under `render/<case>/<model>/shots/`, light and dark:

- `brief-<theme>-first.png`, `brief-<theme>-full.png`, and `brief-<theme>-overview.png` (the full page scaled to 1568 px on its long side, the judge's limit)
- `brief-<theme>-screen-NN.png`, screens 02 to 10 while scrolling down
- `brief-<theme>-fig-NN.png`, one per chart, diagram or graph, at most 8
- `deck-<theme>-NN.png`, every slide in presenting mode (the slide alone, as the room sees it) with every step shown, at most 20

`render.json` records each theme's page height, what drew, the figures and screens shot, images and missing images, any error text on the page ("did not load", "Diagram could not be drawn"), the present version and binary hash, and the face asked for and used. For a deck it records the same per slide. `render/present.json` records the binary, where it came from (checkout, ref or path), its version and sha256, and for a checkout build the uncommitted files.

## Probes

`probes.json` beside the shots holds measurements taken in the page, in the light theme (`probes.py` has the script and the definitions). A computed font size reads the same at any scale, so the probes measure what was drawn: the bounding box of each piece of text, in CSS px at 1440x900. Text height is the box height (the font's ascent plus descent). The font px beside it is the computed size times the scale the text is drawn at.

Brief, once every figure has drawn:

- **diagram labels**: every text in each diagram (node names, node text lines, edge labels, group labels); the smallest and median box height in px, and the smallest drawn font size
- **label collisions**: label boxes that overlap a label of another node, edge or group, or that a node box cuts. A node box cuts an edge or group label that lies over it, and a node's own text that sticks out of it. An edge label's box is its halo. Each label counts once; `examples` in the file names the first few
- **node ellipsis**: node names and text lines that end in an ellipsis
- **figure in the first 900px**: whether the first chart, diagram, graph, stat or image starts above y = 900 of the page, with its kind and top
- **characters per line**: the median over every wrapped line of body prose (`p` outside figures, tables, callouts and closed `details`), the last line of each paragraph left out
- **line charts**: whether the drawn line is smoothed (tension above 0, or monotone interpolation), read from the Chart.js instance on the block; also the value axis minimum and the data minimum

Deck, every slide in presenting mode: the smallest text box on the slide as a share of the slide (canvas) width, and where it is. Then whether the slide scrolls, the diagram label probes above with shares of the canvas width, and the same line chart reading. Chart.js axis and legend text is drawn on a canvas, so its box is measured from the chart's own font with `measureText`.

The report and `metrics.md` show two probe tables, briefs and decks apart.

## Step 3: metrics

`metrics.json` and `metrics.md`, per page:

- **words**: reader-visible words with inline markdown stripped; code, terminals and the inside of a closed `details` are left out (`words_hidden` counts the latter)
- **words before the first figure**: summary, meta, chips, headings and blocks up to the first chart, graph, diagram, stat or image
- **sections**, **block-type histogram**, **figure share** (figure blocks over all non-container blocks)
- **longest prose run**: the most words in consecutive p, list, quote, callout, kv and html blocks; a section boundary, an h3 or any other block ends a run
- **panels** and **callouts**
- **palette roles**: distinct section tones, panel accents, chart series colours, callout severities as their colours, and non-neutral diagram and graph tones
- **roles per block**: palette roles over visible blocks
- **headings with a verb**: the share of section headings that hold a verb from a short word list with its plain inflections. It is a rough test for a heading that makes a claim ("Releases merge themselves") rather than naming a topic ("Release process")
- decks: **slides**, **words per slide** (max and median), and **multi-idea slides** (more than one top-level block, a `columns` counting once)
- decks: **slide titles with a verb**, the same word-list test, with the titles that fail it listed; and roles per block over the slides
- from the render: page height, screens, figures shot, and **images missing** (images on the page that did not load, over all images)
- the page's probes, as `probes` in `metrics.json`

## Steps 4 and 5: judge

No model judges its own pages. `--judge-model cross` (the default) sends Fable-authored pages to `claude-opus-5-5` and every other author's pages to `claude-fable-5-2`; `--judge-model <id>` uses one judge for all. The judge model is recorded in every vote, every summary, `run.json` and the report. Each author has a different judge, so a gap between the two authors' scores mixes the pages with the judges. The pairwise compare between two arms holds the judge fixed per author.

The judge is `claude -p --model <judge>`, isolated as above, with the Read tool only and a short system prompt of its own. It runs in `judge/stage/<hash>/`, which holds copies of the screenshots it should read, so no model name is in any path it sees. The prompt is `rubric.md`, the case's ask, and the list of screenshots:

- brief: the light first screen, light screens 02 to 04, light figure shots 01 to 05, the light overview, the dark first screen and the dark overview
- deck: the light slides (at most 14), plus the first and the middle dark slide
- pairwise, per page: two screens, three figure shots, ten slides

A criterion with a `Check` line gets a yes, no or n/a answer before its score. Each answer names its screenshot and says in one line what the judge saw:

| Check | Pages |
|---|---|
| `point_in_three_seconds`: after a three-second look at the first screen, can you state the page's point (the note holds the point as read) | all |
| `figure_on_first_screen`: is there a figure on the first screen | all |
| `charts_labelled_directly`: are chart lines and bars labelled on or beside the marks, not only in a legend | all (n/a without a chart) |
| `colour_for_one_meaning`: is colour used for one thing that matters, not decoration | all |
| `slide_titles_are_claims`: is each slide title a full-sentence claim | decks |
| `one_idea_each_slide`: does each slide carry one idea | decks |

Then every criterion gets a 1 to 5 score with a screenshot, and the vote names the single most damaging flaw. A vote missing any check, score, screenshot or the flaw is an error and runs again; a numeric string such as `"4"` is accepted as 4. Three independent votes per page. `judge/<case>/<author>/<judge>/vote-N.json` keeps each vote, and `summary.json` the check counts and majority, the score means, and the judge model. A vote whose reply does not parse is kept as `vote-N.error.json` and runs again on the next `judge`.

Changing `rubric.md` does not invalidate old votes on its own. Move the old `vote-*.json`, `summary.json` and `prompt.md` aside (the baseline keeps them in `superseded-*` directories beside the new ones, which the report never reads) and run `judge` again.

## Step 6: report

`report.md` runs in reading order. First the arm (skill fingerprint, present version and binary, face, feedback turn, where the Docs came from), the spend, the metrics table and the probe tables. Then the files the children read. Then the checks: each page's majority answer, how many votes agreed, and the yes rate per author. Then the scores per page with the judge beside them, per-author means, failures, and every vote's worst flaw. The ledger total also counts votes that were set aside and run again; a redraw run's generation cost stays with the run it came from. `contact-sheet.html` lays the first screens, figure shots, overviews and deck slides side by side with relative image paths; open it from the run directory.

## Known limits

- A slide is shot with every step shown, so a stepped diagram shows its last step. When that step has a `focus`, the rest of the diagram is dimmed in the shot. The judge reads that as faded, or as caught mid-animation (the baseline Fable deck, slide 2).
- The first frame headless Chrome draws after a deck enters presenting mode shows the title slide tiled 2x2. `render.py` takes a throwaway shot first; a new rendering path that skips it will show the glitch.
- The three votes on a page see the same prompt and images. In the first baseline 54 of 64 criterion scores were unanimous and the rest differed by one point, so three votes catch a misread more than they measure spread.
- With `cross` judging, each author has its own judge, so the absolute scores of the two authors are not on one scale. Compare an author with itself across arms (pairwise, or the same author's rows in two reports), not Fable against Opus.
- The verb test behind "headings with a verb" and "titles with a verb" is a word list. It passes a question fragment such as "Where it runs" and misses a verb it does not list.
- Decks are shot per step, so a deck of 10 slides with two stepped slides gives 12 shots; the metrics count slides from the Doc JSON.
- In the baseline no child read the examples: every generation was one turn. An arm that changes only the examples may move nothing unless SKILL.md sends the agent to them; the read log now shows whether it did.
- The child replies with the arguments as text, not through the MCP tool, so a reply cut off at the output limit is recorded as `invalid`.
- The probes do not measure Cytoscape graph labels (canvas text with no font record the page exposes) or ribbon charts (drawn with d3, no Chart.js instance; counted as unread).
- A label box is the text's font box, taller than its glyphs. A collision needs more than 2 px of overlap both ways, so two labels that only touch are not counted.
- "Figure in the first 900px" tests where a figure starts. A figure whose top edge is at y = 893 counts, though only 7 px of it is on the first screen.
- The baseline run recorded no face: its shots and probes used the page default, Fira Code. Runs rendered since record the face per page.
- A `--votes` below 3 never decides a pair; every pair stays incomplete.
