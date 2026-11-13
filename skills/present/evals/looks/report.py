"""Step 6: report.md and contact-sheet.html for a run (absolute or pairwise).

The report leads with what is measured (metrics), then the judge's yes/no
checks, then the 1 to 5 scores: the scores filter which pages to look at,
they are not the verdict.
"""

import html
import statistics
from pathlib import Path

from common import Ledger, log, model_key, read_json, write_text
from judge import arm_info, checks, criteria, judge_dir, judge_for
from metrics import page_metrics, table
from probes import probe_table

SHORT = {"first_screen_impact": "first", "hierarchy": "hier", "rhythm_density": "rhythm",
         "figure_quality_fit": "figures", "restraint": "restraint",
         "dyslexic_scannability": "scan", "one_idea_per_slide": "1-idea", "room_legibility": "room"}
CHECK_SHORT = {"point_in_three_seconds": "point in 3 s", "figure_on_first_screen": "figure on 1st screen",
               "charts_labelled_directly": "charts labelled", "colour_for_one_meaning": "colour, one meaning",
               "slide_titles_are_claims": "titles are claims", "one_idea_each_slide": "one idea each"}
ALL_CRITERIA = [cid for cid, _ in criteria("deck")]
ALL_CHECKS = [cid for cid, _ in checks("deck")]


def fmt(value, digits=2):
    if value is None or value == "":
        return ""
    return f"{value:.{digits}f}" if isinstance(value, float) else str(value)


def page_row(run_dir, run, case_id, model):
    row = page_metrics(run_dir, case_id, model)
    row["gen"] = read_json(Path(run_dir) / "gen" / case_id / model_key(model) / "gen.json") or {}
    row["judge_model"] = judge_for(model, run.get("judge_model", "cross"))
    row["judge"] = read_json(judge_dir(run_dir, case_id, model, row["judge_model"]) / "summary.json")
    return row


def check_cell(summary, cid):
    """'yes 3/3', 'no 1/3', 'split 1/2' (yes votes over answered votes), or 'n/a' when most votes said so."""
    c = (summary or {}).get("checks", {}).get(cid)
    if not c:
        return ""
    answered = c["yes"] + c["no"]
    if c["na"] > answered:
        return f"n/a {c['na']}/{c['na'] + answered}"
    if not answered:
        return "n/a"
    label = "yes" if c["yes"] > c["no"] else "no" if c["no"] > c["yes"] else "split"
    return f"{label} {c['yes']}/{answered}"


def checks_table(rows):
    head = ["case", "model", "judge", *[CHECK_SHORT.get(c, c) for c in ALL_CHECKS]]
    lines = ["| " + " | ".join(head) + " |", "|" + "---|" * len(head)]
    for r in rows:
        lines.append("| " + " | ".join([r["case"], r["model"], r["judge_model"] if r["judge"] else "",
                                        *[check_cell(r["judge"], c) for c in ALL_CHECKS]]) + " |")
    return lines


def points_read(rows):
    out = []
    for r in rows:
        c = (r["judge"] or {}).get("checks", {}).get("point_in_three_seconds")
        if c and c["notes"]:
            out.append(f"- {r['case']} / {r['model']}: {c['notes'][0]}")
    return out or ["- no votes"]


def scores_table(rows):
    head = ["case", "model", "judge", *[SHORT[c] for c in ALL_CRITERIA], "overall", "votes"]
    lines = ["| " + " | ".join(head) + " |", "|" + "---|" * len(head)]
    for r in rows:
        j = r["judge"]
        cells = ([fmt(j["criteria"].get(c, {}).get("mean")) for c in ALL_CRITERIA] + [fmt(j["overall"]), str(j["votes"])]
                 if j else [""] * (len(ALL_CRITERIA) + 2))
        lines.append("| " + " | ".join([r["case"], r["model"], r["judge_model"] if j else "", *cells]) + " |")
    return lines


def yes_rate(mine):
    yes = answered = 0
    for r in mine:
        for c in ((r["judge"] or {}).get("checks") or {}).values():
            yes += c["yes"]
            answered += c["yes"] + c["no"]
    return f"{yes}/{answered}" if answered else ""


def model_means(rows, models, reused):
    lines = ["| model | valid | checks yes | overall score | judged by | gen cost | judge cost |",
             "|---|---|---|---|---|---|---|"]
    for m in models:
        mine = [r for r in rows if r["model"] == m]
        overall = [r["judge"]["overall"] for r in mine if r["judge"]]
        judges = sorted({r["judge_model"] for r in mine if r["judge"]})
        gen_cost = "reused" if reused else f"${sum(r['gen'].get('cost_usd', 0) for r in mine):.2f}"
        lines.append(f"| {m} | {sum(r['valid'] for r in mine)}/{len(mine)} | {yes_rate(mine)} | "
                     f"{fmt(statistics.mean(overall)) if overall else ''} | {', '.join(judges)} | "
                     f"{gen_cost} | ${sum((r['judge'] or {}).get('cost_usd', 0) for r in mine):.2f} |")
    return lines


def failures(rows):
    out = []
    for r in rows:
        if not r["valid"]:
            out.append(f"- {r['case']} / {r['model']}: gen {r['gen_status']}, render {r['render_status']}: "
                       f"{r['error'][:300]}")
        rend = r.get("rendered") or {}
        for key in ("error_text", "deck_error_text"):
            if rend.get(key):
                out.append(f"- {r['case']} / {r['model']}: page shows {', '.join(rend[key])}")
        if rend.get("brief_ready") is False or rend.get("deck_not_ready"):
            out.append(f"- {r['case']} / {r['model']}: a figure had not finished drawing when shot")
    return out or ["- none"]


def flaws(rows):
    out = []
    for r in rows:
        if not r["judge"]:
            continue
        out.append(f"- **{r['case']} / {r['model']}** (judge {r['judge_model']})")
        for f in r["judge"]["worst_flaws"]:
            out.append(f"  - {f.get('flaw', '')} ({f.get('screenshot', '')})")
        for b in r["judge"]["broken"]:
            out.append(f"  - broken: {b}")
    return out


def thumbs(run_dir, case_id, model):
    shots = Path("render") / case_id / model_key(model) / "shots"
    full = Path(run_dir) / shots
    pics = [n for n in ["brief-light-first.png", "brief-dark-first.png", "brief-light-overview.png"]
            if (full / n).exists()]
    pics += sorted(p.name for p in full.glob("brief-light-fig-*.png"))
    pics += sorted(p.name for p in full.glob("deck-light-*.png"))
    cells = [f'<a href="{html.escape(str(shots / n))}"><img src="{html.escape(str(shots / n))}" '
             f'title="{html.escape(n)}" class="{"tall" if "overview" in n else "wide"}"></a>' for n in pics]
    return f'<div class=cell>{"".join(cells)}</div>' if cells else "<p>no screenshots</p>"


def contact_sheet(run_dir, run, rows):
    parts = ["<!doctype html><meta charset=utf-8><title>looks contact sheet</title><style>",
             "body{font:14px system-ui;margin:16px}table{border-collapse:collapse;width:100%;table-layout:fixed}"
             "tr>th:first-child{width:120px}td,th{vertical-align:top;"
             "border:1px solid #ccc;padding:6px}.cell{display:flex;flex-wrap:wrap;gap:4px;align-items:flex-start}"
             "img{border:1px solid #ddd}img.wide{width:320px}img.tall{height:420px}"
             ".s{color:#555;margin-bottom:4px}</style>",
             f"<h1>{html.escape(Path(run_dir).name)} {html.escape(run.get('label', ''))}</h1>",
             "<table><tr><th>case</th>" + "".join(f"<th>{html.escape(m)}</th>" for m in run["models"]) + "</tr>"]
    for case_id in run["cases"]:
        parts.append(f"<tr><th>{html.escape(case_id)}</th>")
        for m in run["models"]:
            r = next(x for x in rows if x["case"] == case_id and x["model"] == m)
            score = (f"overall {fmt(r['judge']['overall'])} by {html.escape(r['judge_model'])}"
                     if r["judge"] else "not judged")
            status = "valid" if r["valid"] else f"invalid: {html.escape(r['error'][:200])}"
            parts.append(f"<td><div class=s>{status}, {score}</div>{thumbs(run_dir, case_id, m)}</td>")
        parts.append("</tr>")
    parts.append("</table>")
    write_text(Path(run_dir) / "contact-sheet.html", "\n".join(parts) + "\n")


def arm_line(info):
    """One line naming what sets a run apart: skill, renderer, face, feedback turn."""
    present = info["present"]
    renderer = present["version"] or present["commit"][:12] or "not recorded"
    if present["sha256"]:
        renderer += f" (binary {present['sha256']}, {present['source'] or 'unknown source'})"
    face = ", ".join(info["font"]["used"]) or "not recorded"
    if info["font"]["requested"]:
        face += f" (asked for {info['font']['requested']})"
    reused = f", Docs reused from `{Path(info['gen_from']).name}`" if info["gen_from"] else ""
    return (f"skill `{info['skill_dir']}` ({info['skill_fingerprint']}), present {renderer}, face {face}, "
            f"feedback turn {info['feedback']}{reused}")


def reads_section(rows, run):
    """Per skill file, how many children read it at least once, per model."""
    logged = [r for r in rows if r["gen"].get("reads_logged")]
    if not logged:
        return ["Not logged: this run's children ran before the read log existed."]
    seen = {x["path"] for r in logged for x in r["gen"].get("reads", [])}
    files = list(run.get("skill_files") or []) + sorted(seen - set(run.get("skill_files") or []))
    models = run["models"]
    lines = ["| file | " + " | ".join(models) + " | all |", "|" + "---|" * (len(models) + 2)]
    for name in files:
        cells = []
        for group in [[r for r in logged if r["model"] == m] for m in models] + [logged]:
            hits = sum(1 for r in group if any(x["path"] == name for x in r["gen"].get("reads", [])))
            cells.append(f"{hits}/{len(group)}")
        lines.append(f"| `{name}` | " + " | ".join(cells) + " |")
    any_read = sum(1 for r in logged if r["gen"].get("reads"))
    total = sum(len(r["gen"].get("reads", [])) for r in logged)
    return [f"{any_read} of {len(logged)} children read any file; {total} Read calls in all.", "", *lines]


def probes_note(rows):
    probes = [r["probes"] for r in rows if r.get("probes")]
    if not probes:
        return "No probes yet: `run.py probe --run <stamp>` takes them on an earlier render."
    first = probes[0]
    renderer = (first.get("present") or {}).get("version") or {}
    return (f"Measured on the rendered page in the {first.get('theme', 'light')} theme at 1440x900 "
            f"(`probes.py`), present {renderer.get('version', '')}, face {first['font'].get('used') or 'unknown'}. "
            "Label px is the drawn text box height, font px the computed size times the drawn scale; "
            "slide shares divide by the slide width. Collisions count label boxes that overlap another "
            "label or that a node box cuts.")


def absolute_report(run_dir, run):
    rows = [page_row(run_dir, run, c, m) for c in run["cases"] for m in run["models"]]
    reused = bool(run.get("gen_from"))
    gen_cost = 0.0 if reused else sum(r["gen"].get("cost_usd", 0) for r in rows)
    judge_cost = sum((r["judge"] or {}).get("cost_usd", 0) for r in rows)
    spend_gen = (f"generation reused from `{Path(run['gen_from']).name}`" if reused
                 else f"generation ${gen_cost:.2f}")
    lines = [
        f"# Looks eval {Path(run_dir).name} {run.get('label', '')}".rstrip(), "",
        f"{arm_line(arm_info(run_dir))}; {run['claude_version']}, harness {run['harness_commit'][:12]}, "
        f"judge `{run.get('judge_model', 'cross')}`.", "",
        "Read in this order: the metrics and probes are measured from the Doc JSON and the page; the checks "
        "are the judge's yes or no answers, each tied to a screenshot; the 1 to 5 scores are a filter for "
        "which pages to look at, not the verdict.", "",
        f"Spend: {spend_gen}, judge ${judge_cost:.2f} for the votes below; the run "
        f"ledger, which also counts votes set aside and run again, is ${Ledger(run_dir, 0).total():.2f}.", "",
        "## Metrics", "", "Definitions in `metrics.py` and README.md.", "", table(rows).rstrip(), "",
        "## Probes", "", probes_note(rows), "", *probe_table(rows), "",
        "## Files the children read", "", *reads_section(rows, run), "",
        "## Checks", "", "Majority answer, then yes votes over answered votes.", "",
        *checks_table(rows), "", "The point each page made in three seconds, as the first vote read it:", "",
        *points_read(rows), "",
        "## Scores", "", "Rubric means over the votes, 1 to 5 (`rubric.md`). Deck criteria are blank for briefs.", "",
        *scores_table(rows), "", *model_means(rows, run["models"], reused), "",
        "## Failures", "", *failures(rows), "",
        "## Worst flaw per vote", "", *flaws(rows), "",
        "Screenshots side by side: [contact-sheet.html](contact-sheet.html).", ""]
    write_text(Path(run_dir) / "report.md", "\n".join(lines))
    contact_sheet(run_dir, run, rows)


TALLY = ("B wins", "B losses", "ties", "incomplete", "A failed", "B failed", "both failed")


def pair_bucket(summary):
    result = summary.get("result")
    if result == "failed":
        failed = summary.get("failed") or []
        return "both failed" if len(failed) == 2 else f"{failed[0]} failed"
    if result != "judged":
        return "incomplete"
    return {"B": "B wins", "A": "B losses"}.get(summary.get("winner"), "ties")


def pair_line(case_id, m, s):
    kind = s.get("kind", "")
    if s.get("result") == "failed":
        detail = ", ".join(f"{k} {v}" for k, v in (s.get("status") or {}).items())
        return f"| {case_id} | {kind} | {m} | {s.get('judge_model', '')} | {detail} | not judged | |"
    votes = f"{' '.join(s.get('outcomes', []))} ({s.get('votes_complete', 0)}/{s.get('votes_needed', '')} complete)"
    result = s.get("winner") if s.get("result") == "judged" else "incomplete"
    return f"| {case_id} | {kind} | {m} | {s.get('judge_model', '')} | {votes} | {result} | ${s.get('cost_usd', 0):.2f} |"


def arms_lines(run):
    arms = run.get("arms") or {}
    out = []
    for side in "AB":
        info = arms.get(side)
        target = run["dir_" + side.lower()]
        out.append(f"{side}: `{target}` ({run.get('label_' + side.lower(), '')}): "
                   + (arm_line(info) if info else f"skill `{run['skill_' + side.lower()]}`"))
    if len(arms) == 2:
        differ = [k for k in ("skill_fingerprint", "present", "font", "feedback") if arms["A"][k] != arms["B"][k]]
        out += ["", "The arms differ in: " + (", ".join(differ) if differ else "nothing recorded") + "."]
    return out


def pairwise_report(run_dir, run):
    lines = [f"# Looks pairwise {Path(run_dir).name} {run.get('label', '')}".rstrip(), "", *arms_lines(run), "",
             "Blind, both orders: each vote shows the pair twice, A first and B first. A vote goes to a "
             "side only when the judge picks it both times; otherwise it is a tie. A case goes to the side "
             "that wins most of its votes, else it is a tie. A pair with fewer than three complete votes "
             "(or fewer than were asked for) is incomplete, and a pair where a side did not render is "
             "failed; neither is a win.", "",
             "| case | kind | model | judge | vote outcomes | case result | cost |", "|---|---|---|---|---|---|---|"]
    tally = {}
    reasons = []
    for case_id in run["cases"]:
        for m in run["models"]:
            s = read_json(Path(run_dir) / "pairs" / case_id / model_key(m) / "summary.json") or {}
            key = (s.get("kind", "?"), m)
            tally.setdefault(key, dict.fromkeys(TALLY, 0))[pair_bucket(s)] += 1
            lines.append(pair_line(case_id, m, s))
            reasons += [f"- {case_id} / {m}: {w}" for w in s.get("why", [])]
    lines += ["", "B against A, counted in cases, decks and briefs apart:", "",
              "| kind | model | " + " | ".join(TALLY) + " |", "|" + "---|" * (len(TALLY) + 2)]
    lines += [f"| {kind} | {m} | " + " | ".join(str(t[k]) for k in TALLY) + " |"
              for (kind, m), t in sorted(tally.items())]
    lines += ["", f"Spend ${Ledger(run_dir, 0).total():.2f}.", "", "## Why", "", *reasons, ""]
    write_text(Path(run_dir) / "report.md", "\n".join(lines))


def report(run_dir, run):
    if run.get("kind") == "pairwise":
        pairwise_report(run_dir, run)
    else:
        absolute_report(run_dir, run)
    log(f"report: {Path(run_dir) / 'report.md'}")
