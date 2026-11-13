"""Steps 4 and 5: a vision judge answers the checks and scores the screenshots against rubric.md.

The judge is an isolated `claude -p` with the Read tool only, run in a staging
directory that holds copies of the screenshots it should read, so neither the
author model nor the arm shows in any path it sees. By default (`cross`) no
model judges its own pages: Fable-authored pages go to Opus and the rest to
Fable. Votes live under judge/<case>/<author>/<judge>/, so votes from
different judges never mix.

Pairwise: a pair where a side did not render is recorded as failed, never as
a win; a pair with fewer than max(3, --votes) complete votes is incomplete;
the expected cost is printed, and checked against --max-usd, before any call.
"""

import hashlib
import json
import re
import shutil
import statistics
import time
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from claude_cli import claude_version, isolation_args, run_claude
from common import (HERE, RESULTS_DIR, Ledger, arm_fingerprint, log, model_key, new_run_dir, read_json,
                    write_json, write_text)
from render import render_dir

JUDGE_TURNS = 40
JUDGE_BUDGET_USD = 2.0
JUDGE_TIMEOUT_S = 600
JUDGE_POOL = ["claude-opus-5-5", "claude-fable-5-2"]
MAX_BRIEF_SCREENS = 3
MAX_BRIEF_FIGURES = 5
MAX_JUDGE_SLIDES = 14
CRITERION = re.compile(r"^## (\d+)\. (\w+): (.+)$", re.M)
CHECK = re.compile(r"^Check `(\w+)`( \(decks only\))?: (.+)$", re.M)
ANSWERS = ("yes", "no", "n/a")

SYSTEM = (
    "You are a design reviewer judging how rendered briefing pages and slide decks look and read. "
    "You see them only through screenshots, which you open with the Read tool. Answer each check "
    "with what you see, then score strictly by the rubric anchors and use the whole 1 to 5 range. "
    "Your final reply is one JSON object and nothing else."
)


def rubric_text():
    return (HERE / "rubric.md").read_text()


def rubric_sha():
    return hashlib.sha256(rubric_text().encode()).hexdigest()[:12]


def criteria(kind):
    """[(id, title)] from rubric.md; deck-only criteria only for decks."""
    out = []
    for _num, cid, title in CRITERION.findall(rubric_text()):
        if "decks only" in title and kind != "deck":
            continue
        out.append((cid, title.replace("(decks only)", "").strip()))
    return out


def checks(kind):
    """[(id, question)] from the `Check` lines in rubric.md."""
    return [(cid, question) for cid, deck_only, question in CHECK.findall(rubric_text())
            if not deck_only or kind == "deck"]


def judge_for(author, option):
    """The judge model for pages by author: option itself, or with 'cross' the first
    model in JUDGE_POOL that is not the author."""
    if option != "cross":
        return option
    return next(m for m in JUDGE_POOL if m != author)


# --- which screenshots ------------------------------------------------------

def _existing(shots, names):
    return [n for n in names if (shots / n).exists()]


def brief_images(shots, screens=MAX_BRIEF_SCREENS, figures=MAX_BRIEF_FIGURES):
    names = ["brief-light-first.png"]
    names += [f"brief-light-screen-{n:02d}.png" for n in range(2, screens + 2)]
    names += [f"brief-light-fig-{n:02d}.png" for n in range(1, figures + 1)]
    names += ["brief-light-overview.png", "brief-dark-first.png", "brief-dark-overview.png"]
    return _existing(shots, names)


def deck_images(shots, max_slides=MAX_JUDGE_SLIDES):
    light = sorted(p.name for p in shots.glob("deck-light-*.png"))[:max_slides]
    dark = sorted(p.name for p in shots.glob("deck-dark-*.png"))
    picks = [dark[0], dark[len(dark) // 2]] if dark else []
    return light + sorted(set(picks))


def describe(name):
    theme = "dark" if "-dark-" in name else "light"
    number = name.rsplit("-", 1)[1][:2]
    if name.startswith("deck-"):
        return f"slide {int(number)} of the deck in presenting mode, {theme} theme"
    if name.endswith("-first.png"):
        return f"the first screen of the page, {theme} theme"
    if name.endswith("-overview.png"):
        return f"the whole page scaled down to show its rhythm (charts here may be mid-animation), {theme} theme"
    if "-fig-" in name:
        return f"figure {int(number)} of the page centred on screen after it finished drawing, {theme} theme"
    return f"screen {int(number)} of the page while scrolling down, {theme} theme"


def page_images(shots, kind, pair=False):
    """The screenshots the judge reads for one page, and a note on what is missing."""
    has_brief = (shots / "brief-light-first.png").exists()
    has_deck = (shots / "deck-light-01.png").exists()
    small = {"screens": 2, "figures": 3} if pair else {}
    if kind == "deck":
        if not has_deck:
            note = "The ask was for a deck, but the page has no deck: score criteria 7 and 8 as 1 and answer the deck checks no."
            return brief_images(shots, **small), note
        images = deck_images(shots, 10 if pair else MAX_JUDGE_SLIDES)
        note = ""
        if has_brief and not pair:
            images += _existing(shots, ["brief-light-first.png", "brief-light-overview.png"])
            note = "The page has a brief beside the deck; judge the deck, glance at the brief."
        return images, note
    if not has_brief and has_deck:
        return deck_images(shots, 10), "The page is a deck only; judge its slides as the page."
    return brief_images(shots, **small), ""


def stage(dest, shots, names):
    dest.mkdir(parents=True, exist_ok=True)
    for name in names:
        shutil.copyfile(shots / name, dest / name)  # always: a re-render replaces the shots


def image_list(names, prefix=""):
    return "\n".join(f"- {prefix}{n}: {describe(n)}" for n in names)


# --- absolute votes ---------------------------------------------------------

def vote_shape(kind):
    return {
        "checks": {cid: {"answer": "yes | no | n/a" if "n/a" in question else "yes | no",
                         "screenshot": "<file name>", "note": "<one line: what you saw>"}
                   for cid, question in checks(kind)},
        "scores": {cid: {"score": "<integer 1 to 5>", "screenshot": "<file name>", "why": "<one sentence>"}
                   for cid, _ in criteria(kind)},
        "worst_flaw": {"flaw": "<the one flaw>", "fix": "<the one fix>", "screenshot": "<file name>"},
        "broken": "<anything broken or unfinished: an error note, an empty or half-drawn figure; empty if none>",
    }


def absolute_prompt(case, kind, names, note):
    return (
        f"{rubric_text()}\n\n---\n\nThe page was made for this ask: \"{case['ask']}\" "
        f"(kind: {kind}). {note}\n\n"
        "Screenshots, in the current directory, taken in a 1440x900 browser window:\n"
        f"{image_list(names)}\n\n"
        "Read every screenshot listed above with the Read tool before you answer; read them in "
        "parallel. Then reply with only this JSON object, the checks first:\n"
        f"{json.dumps(vote_shape(kind), indent=1)}"
    )


def judge_args(workdir, model):
    return ["--model", model, *isolation_args(), "--tools", "Read", "--allowedTools", "Read",
            "--permission-mode", "dontAsk", "--add-dir", str(workdir), "--max-turns", str(JUDGE_TURNS),
            "--max-budget-usd", str(JUDGE_BUDGET_USD), "--system-prompt", SYSTEM]


def parse_json_reply(text):
    text = text if isinstance(text, str) else json.dumps(text)
    start, end = text.find("{"), text.rfind("}")
    if start < 0 or end <= start:
        raise ValueError("no JSON object in the reply")
    return json.loads(text[start:end + 1])


def _named_screenshot(entry, what):
    if not str(entry.get("screenshot") or "").strip():
        raise ValueError(f"{what} names no screenshot")


def check_vote(vote, kind):
    """Require an answer per check and a 1 to 5 score per criterion, each with its
    screenshot, and a worst flaw. A numeric string such as "4" becomes 4."""
    for cid, _ in checks(kind):
        entry = (vote.get("checks") or {}).get(cid) or {}
        answer = str(entry.get("answer") or "").strip().lower()
        if answer not in ANSWERS:
            raise ValueError(f"check {cid} has no yes, no or n/a answer")
        entry["answer"] = answer
        _named_screenshot(entry, f"check {cid}")
    for cid, _ in criteria(kind):
        entry = (vote.get("scores") or {}).get(cid) or {}
        try:
            score = float(entry.get("score"))
        except (TypeError, ValueError):
            raise ValueError(f"criterion {cid} has no score from 1 to 5") from None
        if not 1 <= score <= 5:
            raise ValueError(f"criterion {cid} has no score from 1 to 5")
        _named_screenshot(entry, f"criterion {cid}")
        entry["score"] = int(score) if score.is_integer() else score
    if not str((vote.get("worst_flaw") or {}).get("flaw") or "").strip():
        raise ValueError("the vote names no worst flaw")


def call_judge(prompt, cwd, model, ledger, label, checker):
    """One paid judge call; returns the parsed vote with cost and judge model, or a dict with error."""
    if ledger.over_cap():
        return {"error": f"spend cap ${ledger.cap:.2f} reached", "skipped": True}
    rec = run_claude(judge_args(cwd, model), prompt, cwd, JUDGE_TIMEOUT_S)
    ledger.add("judge", f"{label} ({model})", rec["cost_usd"])
    meta = {"judge_model": model, "cost_usd": rec["cost_usd"], "turns": rec["turns"], "wall_s": rec["wall_s"]}
    if not rec["ok"]:
        return {**meta, "error": rec["error"], "raw": str(rec["result"])[:2000]}
    try:
        vote = parse_json_reply(rec["result"])
        checker(vote)
    except ValueError as exc:
        return {**meta, "error": f"judge reply unusable: {exc}", "raw": str(rec["result"])[:4000]}
    return {**vote, **meta}


def token(*parts):
    return hashlib.sha1("|".join(parts).encode()).hexdigest()[:10]


def judge_dir(run_dir, case_id, author, judge_model):
    return Path(run_dir) / "judge" / case_id / model_key(author) / model_key(judge_model)


def judge_page(run_dir, case, author, judge_model, ledger, votes):
    """Return the (vote_path, label, job) triples still to run for one page."""
    shots = render_dir(run_dir, case["id"], author) / "shots"
    render = read_json(render_dir(run_dir, case["id"], author) / "render.json") or {}
    out = judge_dir(run_dir, case["id"], author, judge_model)
    if render.get("status") != "ok":
        write_json(out / "skipped.json", {"reason": f"render {render.get('status', 'missing')}"})
        return []
    names, note = page_images(shots, case["kind"])
    workdir = Path(run_dir) / "judge" / "stage" / token(Path(run_dir).name, case["id"], author, judge_model)
    stage(workdir, shots, names)
    prompt = absolute_prompt(case, case["kind"], names, note)
    write_text(out / "prompt.md", prompt)
    jobs = []
    for n in range(1, votes + 1):
        path = out / f"vote-{n}.json"
        if path.exists():
            continue
        label = f"{case['id']} / {author} / vote {n}"
        jobs.append((path, label, lambda p=prompt, w=workdir, lab=label: call_judge(
            p, w, judge_model, ledger, lab, lambda v: check_vote(v, case["kind"]))))
    return jobs


def run_jobs(jobs, workers):
    def one(job):
        path, label, fn = job
        log(f"judge {label}: started")
        vote = fn()
        if "error" in vote:
            if not vote.get("skipped"):
                write_json(path.with_name(path.stem + ".error.json"), vote)
            log(f"judge {label}: {vote['error'][:200]}")
            return
        write_json(path, vote)
        log(f"judge {label}: {vote['judge_model']} ${vote['cost_usd']:.3f} {vote['turns']} turns {vote['wall_s']}s")

    with ThreadPoolExecutor(max_workers=max(1, workers)) as pool:
        list(pool.map(one, jobs))


def summarize_page(out, kind, judge_model):
    votes = [read_json(p) for p in sorted(out.glob("vote-[0-9].json"))]
    if not votes:
        return None
    per = {}
    for cid, _ in criteria(kind):
        scores = [v["scores"][cid]["score"] for v in votes if cid in v.get("scores", {})]
        per[cid] = {"mean": round(statistics.mean(scores), 2), "votes": scores}
    answers = {}
    for cid, _ in checks(kind):
        given = [v["checks"][cid]["answer"] for v in votes if cid in v.get("checks", {})]
        counts = Counter(given)
        answers[cid] = {"answers": given, "yes": counts["yes"], "no": counts["no"], "na": counts["n/a"],
                        "majority": counts.most_common(1)[0][0] if given else "",
                        "notes": [v["checks"][cid].get("note", "") for v in votes if cid in v.get("checks", {})]}
    summary = {
        "judge_model": judge_model,
        "votes": len(votes),
        "checks": answers,
        "criteria": per,
        "overall": round(statistics.mean(c["mean"] for c in per.values()), 2),
        "worst_flaws": [v.get("worst_flaw", {}) for v in votes],
        "broken": [v.get("broken", "") for v in votes if v.get("broken")],
        "cost_usd": round(sum(v.get("cost_usd", 0) for v in votes), 3),
    }
    write_json(out / "summary.json", summary)
    return summary


def judge(run_dir, run, ledger, votes, workers, option):
    from cases import load_cases
    cases = load_cases(",".join(run["cases"]))
    run.setdefault("judge_runs", []).append({"time": time.strftime("%Y-%m-%dT%H:%M:%S"),
                                             "judge_model": option, "rubric_sha": rubric_sha(), "votes": votes})
    run["judge_model"] = option
    write_json(Path(run_dir) / "run.json", run)
    pages = [(c, m, judge_for(m, option)) for c in run["cases"] for m in run["models"]]
    by_id = {c["id"]: c for c in cases}
    jobs = [j for cid, m, jm in pages for j in judge_page(run_dir, by_id[cid], m, jm, ledger, votes)]
    # Vote 1 of every page before any vote 2, so a spend cap leaves every page judged.
    jobs.sort(key=lambda j: int(j[0].stem.split("-")[1]))
    log(f"judge: {len(jobs)} votes to run ({option}), spend so far ${ledger.total():.2f}")
    run_jobs(jobs, workers)
    for cid, m, jm in pages:
        summarize_page(judge_dir(run_dir, cid, m, jm), by_id[cid]["kind"], jm)
    log(f"judge done, run spend ${ledger.total():.2f}")


# --- blind pairwise, both orders --------------------------------------------

PAIR_SHAPE = {
    "better": "page-1 | page-2 | tie",
    "margin": "slight | clear",
    "why": "<two sentences on what decides it>",
    "page-1_worst_flaw": "<one sentence>",
    "page-2_worst_flaw": "<one sentence>",
}
ORDERS = {"AB": {"page-1": "A", "page-2": "B"}, "BA": {"page-1": "B", "page-2": "A"}}
PAIR_CALL_USD = 0.25  # price per pairwise call until an earlier pairwise ledger gives a mean
MIN_COMPLETE_VOTES = 3  # a pair with fewer complete votes is incomplete, never decided


def pair_prompt(case, names_1, names_2):
    return (
        f"{rubric_text()}\n\n---\n\nTwo pages were made for the same ask: \"{case['ask']}\" "
        f"(kind: {case['kind']}). Compare how they look and read, using the rubric criteria and "
        "checks as your lens; content accuracy is not in scope.\n\n"
        f"Page 1, in directory page-1/:\n{image_list(names_1, 'page-1/')}\n\n"
        f"Page 2, in directory page-2/:\n{image_list(names_2, 'page-2/')}\n\n"
        "Read every screenshot of both pages with the Read tool first, in parallel. Then say which "
        "page looks better overall. Reply with only this JSON object:\n"
        f"{json.dumps(PAIR_SHAPE, indent=1)}"
    )


def check_pair(vote):
    if vote.get("better") not in ("page-1", "page-2", "tie"):
        raise ValueError("better must be page-1, page-2 or tie")


def side_status(run_dir, case_id, author):
    return (read_json(render_dir(run_dir, case_id, author) / "render.json") or {}).get("status", "missing")


def pair_out(out_dir, case_id, author):
    return Path(out_dir) / "pairs" / case_id / model_key(author)


def pair_jobs(out_dir, run_a, run_b, case, author, judge_model, votes, ledger):
    """Each vote is two calls, A shown first and B shown first. A pair where a side
    did not render (refused, skipped, browser error) is recorded as failed, not judged."""
    sides = {"A": render_dir(run_a, case["id"], author), "B": render_dir(run_b, case["id"], author)}
    status = {k: side_status(run, case["id"], author) for k, run in (("A", run_a), ("B", run_b))}
    out = pair_out(out_dir, case["id"], author)
    failed = [k for k, s in status.items() if s != "ok"]
    if failed:
        write_json(out / "summary.json", {"result": "failed", "kind": case["kind"], "status": status,
                                          "failed": failed, "winner": None, "judge_model": judge_model})
        return []
    names = {k: page_images(d / "shots", case["kind"], pair=True)[0] for k, d in sides.items()}
    jobs = []
    for n in range(1, votes + 1):
        for oid, order in ORDERS.items():
            path = out / f"vote-{n}-{oid}.json"
            if path.exists():
                continue
            workdir = out_dir / "stage" / token(out_dir.name, case["id"], author, oid)
            for page, side in order.items():
                stage(workdir / page, sides[side] / "shots", names[side])
            prompt = pair_prompt(case, names[order["page-1"]], names[order["page-2"]])
            label = f"pair {case['id']} / {author} / vote {n} {oid}"

            def fn(p=prompt, w=workdir, lab=label, o=order):
                vote = call_judge(p, w, judge_model, ledger, lab, check_pair)
                if "error" not in vote:
                    vote["order"] = o
                    vote["winner"] = o.get(vote["better"], "tie")
                return vote
            jobs.append((path, label, fn))
    return jobs


def vote_outcome(out, n):
    """A or B when the judge picked that side in both orders; tie otherwise; None if incomplete."""
    picks = [read_json(out / f"vote-{n}-{oid}.json") for oid in ORDERS]
    if any(p is None for p in picks):
        return None
    winners = {p["winner"] for p in picks}
    return winners.pop() if len(winners) == 1 and "tie" not in winners else "tie"


def summarize_pair(out, votes, judge_model, kind):
    """Decide a pair from its complete votes: the side that wins most of them, else a
    tie. Fewer than max(3, votes) complete votes leaves the pair incomplete."""
    old = read_json(out / "summary.json")
    if old and old.get("result") == "failed":
        return old
    outcomes = [o for o in (vote_outcome(out, n) for n in range(1, votes + 1)) if o]
    counts = Counter(outcomes)
    need = max(MIN_COMPLETE_VOTES, votes)
    calls = [read_json(p) for p in sorted(out.glob("vote-*-*.json")) if not p.name.endswith(".error.json")]
    summary = {"kind": kind, "judge_model": judge_model, "outcomes": outcomes, "votes_complete": len(outcomes),
               "votes_needed": need, "wins": {"A": counts["A"], "B": counts["B"], "tie": counts["tie"]},
               "why": [f"{c['order']['page-1']} shown first, picked {c['winner']}: {c.get('why', '')}" for c in calls],
               "cost_usd": round(sum(c.get("cost_usd", 0) for c in calls), 3)}
    if len(outcomes) < need:
        summary.update(result="incomplete", winner=None)
    else:
        summary.update(result="judged",
                       winner=next((side for side in "AB" if counts[side] > len(outcomes) / 2), "tie"))
    write_json(out / "summary.json", summary)
    return summary


def arm_info(run_dir):
    """What sets an arm apart: skill, renderer, face, feedback turn, and where its Docs came from."""
    run_dir = Path(run_dir)
    run = read_json(run_dir / "run.json") or {}
    present = read_json(run_dir / "render" / "present.json") or {}
    version = present.get("version") or {}
    used = {((read_json(p) or {}).get("font") or {}).get("used", "")
            for p in (run_dir / "render").glob("*/*/render.json")}
    return {"dir": str(run_dir.resolve()), "label": run.get("label", ""), "skill_dir": run.get("skill_dir", ""),
            "skill_fingerprint": arm_fingerprint(run),
            "present": {"version": version.get("version", ""), "commit": version.get("commit", ""),
                        "sha256": (present.get("sha256") or "")[:12], "source": present.get("source", ""),
                        "dirty": len(present.get("dirty") or [])},
            "font": {"requested": (run.get("render") or {}).get("font"), "used": sorted(used - {""})},
            "feedback": (run.get("feedback") or {}).get("mode", "none"), "gen_from": run.get("gen_from", "")}


def select_pairs(dir_a, dir_b, only_cases, only_models):
    run_a, run_b = read_json(dir_a / "run.json"), read_json(dir_b / "run.json")
    if not run_a or not run_b:
        raise SystemExit("both arguments must be run directories with run.json")
    case_ids = [c for c in run_a["cases"] if c in run_b["cases"]
                and (not only_cases or c in only_cases.split(","))]
    models = [m for m in run_a["models"] if m in run_b["models"]
              and (not only_models or m in only_models.split(","))]
    return case_ids, models


def start_pairwise(dir_a, dir_b, case_ids, models, votes, max_usd, label, option):
    arms = {"A": arm_info(dir_a), "B": arm_info(dir_b)}
    out_dir = new_run_dir()
    run = {"kind": "pairwise", "created": time.strftime("%Y-%m-%dT%H:%M:%S"), "label": label or "",
           "dir_a": str(dir_a.resolve()), "dir_b": str(dir_b.resolve()),
           "label_a": arms["A"]["label"], "label_b": arms["B"]["label"],
           "skill_a": arms["A"]["skill_dir"], "skill_b": arms["B"]["skill_dir"], "arms": arms, "cases": case_ids,
           "models": models, "votes": votes, "judge_model": option,
           "judges": {m: judge_for(m, option) for m in models}, "rubric_sha": rubric_sha(),
           "claude_version": claude_version(), "max_usd": max_usd}
    write_json(out_dir / "run.json", run)
    return out_dir, run


def calls_to_run(out_dir, dir_a, dir_b, cases, models, votes):
    """Judge calls still missing over the pairs both sides rendered."""
    calls = 0
    for case in cases:
        for m in models:
            if side_status(dir_a, case["id"], m) != "ok" or side_status(dir_b, case["id"], m) != "ok":
                continue
            out = pair_out(out_dir, case["id"], m) if out_dir else None
            calls += sum(1 for n in range(1, votes + 1) for oid in ORDERS
                         if not (out and (out / f"vote-{n}-{oid}.json").exists()))
    return calls


def call_price():
    """(mean cost per call, calls seen) over the judge calls of earlier pairwise ledgers."""
    costs = []
    for run_dir in RESULTS_DIR.glob("*"):
        ledger = run_dir / "spend.jsonl"
        if (read_json(run_dir / "run.json") or {}).get("kind") != "pairwise" or not ledger.exists():
            continue
        rows = [json.loads(line) for line in ledger.read_text().splitlines() if line.strip()]
        costs += [float(r["cost_usd"]) for r in rows if r.get("step") == "judge" and r.get("cost_usd")]
    return (statistics.mean(costs), len(costs)) if costs else (PAIR_CALL_USD, 0)


def check_budget(calls, spent, max_usd):
    """Print the expected judge calls and cost; refuse to start when they would pass max_usd."""
    price, seen = call_price()
    expected = calls * price
    basis = f"the mean of {seen} earlier pairwise calls" if seen else "the README estimate"
    log(f"pairwise: {calls} judge calls to run, expected ${expected:.2f} at ${price:.3f} a call ({basis}); "
        f"spent ${spent:.2f} so far, cap ${max_usd:.2f}")
    if spent + expected > max_usd:
        raise SystemExit(f"pairwise not started: ${spent:.2f} spent plus ${expected:.2f} expected passes "
                         f"--max-usd {max_usd:.2f}; raise the cap or narrow --cases or --models")


def pairwise(dir_a, dir_b, votes, workers, max_usd, label, option, only_cases=None, resume=None, only_models=None):
    """Compare two rendered runs; resume (a pairwise run directory) picks up its missing calls.
    The expected cost is checked before anything starts."""
    from cases import load_cases
    out_dir = None
    if resume:
        out_dir, run = resume, read_json(resume / "run.json")
        if (run or {}).get("kind") != "pairwise":
            raise SystemExit(f"{resume} is not a pairwise run")
        dir_a, dir_b, votes, option = Path(run["dir_a"]), Path(run["dir_b"]), run["votes"], run["judge_model"]
        case_ids, models = run["cases"], run["models"]
    else:
        case_ids, models = select_pairs(dir_a, dir_b, only_cases, only_models)
    cases = load_cases(",".join(case_ids))
    spent = Ledger(out_dir, max_usd).total() if out_dir else 0.0
    check_budget(calls_to_run(out_dir, dir_a, dir_b, cases, models, votes), spent, max_usd)
    if not resume:
        out_dir, run = start_pairwise(dir_a, dir_b, case_ids, models, votes, max_usd, label, option)
    ledger = Ledger(out_dir, max_usd)
    jobs = [j for c in cases for m in models
            for j in pair_jobs(out_dir, dir_a, dir_b, c, m, judge_for(m, option), votes, ledger)]
    log(f"pairwise {out_dir.name}: {len(jobs)} judge calls to run")
    run_jobs(jobs, workers)
    for c in cases:
        for m in models:
            summarize_pair(pair_out(out_dir, c["id"], m), votes, judge_for(m, option), c["kind"])
    from report import report
    report(out_dir, run)
