#!/usr/bin/env python3
"""Looks eval for the present skill: generate, render, probe, metrics, judge, report.

Every step is resumable: it skips work whose output already exists in the run
directory, results/<timestamp>/. See README.md for the commands.
"""

import argparse
import shutil
import subprocess
import sys
import time
from pathlib import Path

from claude_cli import claude_version
from common import (DEFAULT_MODELS, DEFAULT_SCRATCH, DEFAULT_SKILL_DIR, FONTS, HERE, Ledger, log,
                    model_key, new_run_dir, read_json, resolve_run_dir, skill_files, skill_fingerprint,
                    write_json)


def git_head(path):
    out = subprocess.run(["git", "-C", str(path), "rev-parse", "HEAD"], capture_output=True, text=True)
    return out.stdout.strip()


def check_font(font):
    if font and font not in FONTS:
        sys.exit(f"unknown face {font!r}; webkit offers {', '.join(FONTS)} (common.FONTS)")


def feedback_config(a):
    if a.feedback == "lint" and not a.lint_cmd:
        sys.exit("--feedback lint needs --lint-cmd, the command whose output is fed back")
    return {"mode": a.feedback, "lint_cmd": a.lint_cmd if a.feedback == "lint" else ""}


def start_run(a):
    from cases import load_cases
    skill_dir = Path(a.skill_dir).expanduser().resolve()
    if not (skill_dir / "SKILL.md").exists():
        sys.exit(f"{skill_dir} has no SKILL.md")
    cases = load_cases(a.cases)
    feedback = feedback_config(a)
    run_dir = new_run_dir()
    run = {
        "kind": "absolute",
        "created": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "label": a.label or "",
        "skill_dir": str(skill_dir),
        "skill_fingerprint": skill_fingerprint(skill_dir),
        "skill_files": skill_files(skill_dir),
        "models": a.models.split(","),
        "cases": [c["id"] for c in cases],
        "effort": a.effort or "",
        "feedback": feedback,
        "claude_version": claude_version(),
        "harness_commit": git_head(HERE),
        "max_usd": a.max_usd,
    }
    write_json(run_dir / "run.json", run)
    log(f"new run {run_dir}")
    return run_dir, run


def open_run(a):
    run_dir = resolve_run_dir(a.run)
    return run_dir, read_json(run_dir / "run.json")


def scratch_for(a, run_dir):
    path = Path(a.scratch) / run_dir.name
    path.mkdir(parents=True, exist_ok=True)
    return path


def cmd_generate(a):
    from cases import load_cases
    from generate import generate
    run_dir, run = open_run(a) if a.run else start_run(a)
    ledger = Ledger(run_dir, a.max_usd)
    generate(run_dir, run, load_cases(",".join(run["cases"])), ledger, scratch_for(a, run_dir),
             a.jobs, a.redo_errors)
    log(f"generate done, run spend ${ledger.total():.2f}: {run_dir}")


def render_config(a, run):
    """The run's renderer and face: flags given now replace what run.json holds."""
    check_font(a.font)
    cfg = dict(run.get("render") or {})
    if a.present_bin or a.present_ref:
        cfg["present_bin"] = str(Path(a.present_bin).expanduser().resolve()) if a.present_bin else None
        cfg["present_ref"] = a.present_ref
    if a.font is not None:
        cfg["font"] = a.font or None
    return cfg


def render_run(a, run_dir, run):
    from render import render, resolve_binary
    run["render"] = render_config(a, run)
    write_json(run_dir / "run.json", run)
    scratch = scratch_for(a, run_dir)
    cfg = run["render"]
    present = resolve_binary(run_dir, scratch, cfg.get("present_bin"), cfg.get("present_ref"))
    render(run_dir, run, scratch, a.port, present, cfg.get("font"), a.redo, a.cases)


def cmd_render(a):
    run_dir, run = open_run(a)
    render_run(a, run_dir, run)


def copy_gen(src_dir, run_dir, cases, models):
    for case_id in cases:
        for model in models:
            src = src_dir / "gen" / case_id / model_key(model)
            if src.exists():
                shutil.copytree(src, run_dir / "gen" / case_id / model_key(model),
                                ignore=shutil.ignore_patterns("stream.jsonl"))


def cmd_redraw(a):
    """A second render slot: the source run's generated Docs, drawn again into a new run."""
    src_dir = resolve_run_dir(a.source)
    src = read_json(src_dir / "run.json")
    if src.get("kind") != "absolute":
        sys.exit(f"{src_dir} is not a generate run")
    cases = [c for c in src["cases"] if not a.cases or c in a.cases.split(",")]
    models = [m for m in src["models"] if not a.models or m in a.models.split(",")]
    run_dir = new_run_dir()
    copy_gen(src_dir, run_dir, cases, models)
    kept = ("skill_dir", "skill_sha256", "skill_fingerprint", "skill_files", "effort", "feedback",
            "claude_version")
    run = {"kind": "absolute", "created": time.strftime("%Y-%m-%dT%H:%M:%S"), "label": a.label or "",
           **{k: src[k] for k in kept if k in src}, "gen_from": str(src_dir), "models": models,
           "cases": cases, "harness_commit": git_head(HERE), "max_usd": src.get("max_usd", 20.0)}
    write_json(run_dir / "run.json", run)
    log(f"new render slot {run_dir} from {src_dir.name}")
    a.cases = None
    render_run(a, run_dir, run)


def cmd_probe(a):
    from render import probe_run, resolve_binary
    check_font(a.font)
    run_dir, run = open_run(a)
    scratch = scratch_for(a, run_dir)
    present = resolve_binary(run_dir, scratch, a.present_bin, a.present_ref, shots_binary=True)
    font = a.font if a.font is not None else (run.get("render") or {}).get("font")
    probe_run(run_dir, run, scratch, a.port, present, font or None, a.redo, a.cases)


def cmd_metrics(a):
    from metrics import metrics
    run_dir, run = open_run(a)
    metrics(run_dir, run)


def cmd_judge(a):
    from judge import judge
    run_dir, run = open_run(a)
    judge(run_dir, run, Ledger(run_dir, a.max_usd), a.votes, a.jobs, a.judge_model)


def cmd_pairwise(a):
    from judge import pairwise
    if a.run:
        pairwise(None, None, a.votes, a.jobs, a.max_usd, a.label, a.judge_model, resume=resolve_run_dir(a.run))
        return
    if not (a.dir_a and a.dir_b):
        sys.exit("pairwise needs two run directories, or --run with an earlier pairwise run")
    pairwise(Path(a.dir_a), Path(a.dir_b), a.votes, a.jobs, a.max_usd, a.label, a.judge_model, a.cases,
             only_models=a.models)


def cmd_report(a):
    from report import report
    run_dir, run = open_run(a)
    report(run_dir, run)


def common(sp, jobs=True, budget=True):
    sp.add_argument("--run", help="run stamp or path under results/; 'latest' for the newest")
    sp.add_argument("--scratch", default=str(DEFAULT_SCRATCH), help="scratch root (default %(default)s)")
    if jobs:
        sp.add_argument("--jobs", type=int, default=4, help="parallel claude children (default 4)")
    if budget:
        sp.add_argument("--max-usd", type=float, default=20.0,
                        help="stop starting paid calls once the run has spent this (default 20)")


def renderer_args(sp, probe=False):
    sp.add_argument("--port", type=int, default=7512, help="scratch present serve port (default 7512)")
    which = "the binary that took the shots" if probe else "built from this checkout, uncommitted changes included"
    sp.add_argument("--present-bin", help=f"present binary (default: {which})")
    sp.add_argument("--present-ref", help="git ref of this repo to build present from (git archive, checkout untouched)")
    sp.add_argument("--font", help=f"reading face stored before every shot: {', '.join(FONTS)} "
                                   "(default: none stored, the page default)")
    sp.add_argument("--redo", action="store_true", help="do it again even where the output exists")
    sp.add_argument("--cases", help="comma-separated case ids (default: all in the run)")


def add_generate(sub):
    g = sub.add_parser("generate", help="one claude -p child per case and model")
    common(g)
    g.add_argument("--skill-dir", default=str(DEFAULT_SKILL_DIR), help="skill directory with SKILL.md (an arm)")
    g.add_argument("--models", default=",".join(DEFAULT_MODELS), help="comma-separated model ids")
    g.add_argument("--cases", help="comma-separated case ids (default: all)")
    g.add_argument("--effort", help="pass --effort to the children")
    g.add_argument("--label", help="free-text label stored in run.json (e.g. before, after)")
    g.add_argument("--feedback", choices=("none", "lint", "neutral"), default="none",
                   help="a second turn: lint feeds back --lint-cmd's output once, neutral asks for one "
                        "review and revision with no findings (default none)")
    g.add_argument("--lint-cmd", help="shell command for --feedback lint; {args}, {content} and {deck} "
                                      "become paths of the turn-1 arguments and Docs")
    g.add_argument("--redo-errors", action="store_true", help="re-run generations that ended in an infra error")
    g.set_defaults(func=cmd_generate)


def add_render(sub):
    r = sub.add_parser("render", help="create each page in a scratch present, screenshot and probe it")
    common(r, jobs=False, budget=False)
    renderer_args(r)
    r.set_defaults(func=cmd_render)

    d = sub.add_parser("redraw", help="a second render slot: a run's Docs drawn into a new run, no generation")
    d.add_argument("source", help="the generate run whose Docs to draw (stamp, path or latest)")
    d.add_argument("--label", help="free-text label stored in run.json")
    d.add_argument("--models", help="comma-separated author models (default: all in the source run)")
    d.add_argument("--scratch", default=str(DEFAULT_SCRATCH), help="scratch root (default %(default)s)")
    renderer_args(d)
    d.set_defaults(func=cmd_redraw)

    p = sub.add_parser("probe", help="page probes alone, for a run rendered without them")
    common(p, jobs=False, budget=False)
    renderer_args(p, probe=True)
    p.set_defaults(func=cmd_probe)


def add_judging(sub):
    judge_help = ("judge model id, or 'cross' (default): no model judges its own pages; "
                  "Fable-authored pages go to claude-opus-5-5, the rest to claude-fable-5-2")
    j = sub.add_parser("judge", help="checks and rubric scores from a vision judge, three votes per page")
    common(j)
    j.add_argument("--votes", type=int, default=3)
    j.add_argument("--judge-model", default="cross", help=judge_help)
    j.set_defaults(func=cmd_judge)

    pw = sub.add_parser("pairwise", help="blind A/B judge over two rendered runs, both orders per vote")
    pw.add_argument("dir_a", nargs="?", help="first run directory")
    pw.add_argument("dir_b", nargs="?", help="second run directory")
    pw.add_argument("--run", help="resume an earlier pairwise run (stamp or path); takes its arms and votes")
    pw.add_argument("--votes", type=int, default=3, help="votes per pair; each vote is two judge calls")
    pw.add_argument("--jobs", type=int, default=4)
    pw.add_argument("--max-usd", type=float, default=20.0,
                    help="refuse to start when the expected cost would take the run past this (default 20)")
    pw.add_argument("--label", help="free-text label stored in run.json")
    pw.add_argument("--judge-model", default="cross", help=judge_help)
    pw.add_argument("--cases", help="comma-separated case ids (default: every case both runs have)")
    pw.add_argument("--models", help="comma-separated author models (default: every model both runs have)")
    pw.set_defaults(func=cmd_pairwise)


def parser():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    add_generate(sub)
    add_render(sub)
    m = sub.add_parser("metrics", help="mechanical proxies per page")
    common(m, jobs=False, budget=False)
    m.set_defaults(func=cmd_metrics)
    add_judging(sub)
    rp = sub.add_parser("report", help="markdown summary and contact sheet")
    common(rp, jobs=False, budget=False)
    rp.set_defaults(func=cmd_report)
    return p


def main():
    a = parser().parse_args()
    a.func(a)


if __name__ == "__main__":
    main()
