"""Step 3: mechanical proxies for looks, read from the Doc JSON and the rendered page.

Definitions (also in README.md):
- words: reader-visible words, inline markdown stripped; code, terminal and the
  inside of a closed `details` block are not counted (details: words_hidden).
- figure: a chart, graph, diagram, stat or image block.
- words before the first figure: summary, meta, chips, headings and blocks in
  page order up to the first figure.
- prose run: consecutive p, list, quote, callout, kv and html blocks; a section
  boundary, an h3 or any other block ends the run.
- palette roles: section tones, panel accents, chart series colours (series-N by
  position when unset), callout severities as their colours, and non-neutral
  diagram and graph tones.
- idea-block: a top-level block of a slide, a columns block counting once.
- titles with a verb: share of slide titles (deck) or section headings (brief)
  holding a word from VERBS or its -s/-ed form; a rough word-list check for
  "the title is a claim", with -ing forms and noun-heavy verbs (store, hook,
  step, release) left out.
- roles per block: distinct palette roles divided by non-container blocks.
"""

import json
import re
import statistics
from collections import Counter
from pathlib import Path

from common import log, read_json, write_json, write_text
from generate import gen_dir
from probes import probe_table, read_probes
from render import render_dir

FIGURES = {"chart", "graph", "diagram", "stat", "image"}
PROSE = {"p", "list", "quote", "callout", "kv", "html"}
CONTAINERS = {"columns", "details"}
SEV_ROLE = {"info": "blue", "warn": "amber", "ok": "green", "error": "red"}
LINK = re.compile(r"\[([^\]]*)\]\([^)]*\)")
TAG = re.compile(r"<[^>]+>")

VERB_BASES = """
accept add allow answer apply ask avoid become begin block break bring build call carry catch
cause choose close come compare contain cost cover create cut decide deny depend describe
drive drop end explain fail fall feed fill find finish fire fit fix follow get give go grow
happen help hide hold improve include keep know land lead leave let lift limit live load look
lose make mean meet miss move need open own pass pay pick point prevent produce protect prove
pull push put raise reach read reduce refuse remove repeat replace require return rise run save
say see send serve set show shrink sit skip slow solve speak split stand start stay stop
surface switch take talk tell think touch track trigger turn use wait want watch win work write
verify merge ship wire inject rebuild restart decide matter outgrow scale trust
""".split()
VERB_FORMS = {
    "is", "are", "was", "were", "be", "been", "has", "have", "had", "do", "does", "did", "can",
    "could", "will", "would", "should", "may", "might", "must", "shall", "cannot", "won't",
    "isn't", "aren't", "doesn't", "don't", "didn't", "wasn't", "can't", "took", "taken", "made",
    "got", "gave", "went", "came", "ran", "kept", "held", "brought", "built", "left", "showed",
    "shown", "found", "told", "said", "saw", "seen", "paid", "met", "grew", "drove", "wrote",
    "written", "read", "fell", "won", "lost", "spent", "sent", "broke", "broken", "chose", "led",
    "began", "became", "knew", "thought", "stood", "sat", "fed",
}


def _inflections(base):
    forms = {base, base + "s", base + "es", base + "ed", base + "d"}
    if base.endswith("y"):
        forms |= {base[:-1] + "ies", base[:-1] + "ied"}
    return forms


VERBS = VERB_FORMS.union(*(_inflections(b) for b in VERB_BASES))


def has_verb(title):
    words = re.findall(r"[a-z']+", (title or "").lower().replace("’", "'"))
    return any(w in VERBS for w in words)


def verb_share(titles):
    titles = [t for t in titles if t]
    return round(sum(has_verb(t) for t in titles) / len(titles), 2) if titles else None


def count_words(*texts):
    total = 0
    for text in texts:
        if text is None:
            continue
        if not isinstance(text, str):
            text = str(text)
        text = TAG.sub(" ", LINK.sub(r"\1", text))
        total += len([w for w in re.split(r"\s+", text.replace("**", "").replace("`", "")) if re.search(r"\w", w)])
    return total


def _items_text(items):
    for item in items or []:
        if isinstance(item, dict):
            yield item.get("text") or item.get("t") or ""
        else:
            yield item


def chart_words(b):
    texts = [b.get("title"), b.get("unit"), b.get("xunit")]
    for s in b.get("series") or []:
        texts.append(s.get("name"))
        texts += [p.get("x") for p in s.get("points") or [] if isinstance(p, dict) and isinstance(p.get("x"), str)]
    for f in b.get("flows") or []:
        texts += [f.get("from"), f.get("to")]
    texts += [s.get("caption") for s in b.get("steps") or []]
    return count_words(*texts)


def diagram_words(b):
    texts = [b.get("caption")]
    texts += [x for n in b.get("nodes") or [] for x in (n.get("label"), n.get("text"))]
    texts += [e.get("label") for e in b.get("edges") or []]
    texts += [g.get("label") for g in b.get("groups") or []]
    texts += [s.get("caption") for s in b.get("steps") or []]
    return count_words(*texts)


def block_words(b):
    """Visible words of one block, children excluded."""
    t = b.get("t")
    if t in ("p", "h3", "callout", "html"):
        return count_words(b.get("text"))
    if t == "quote":
        return count_words(b.get("text"), b.get("cite"))
    if t == "list":
        return count_words(*_items_text(b.get("items")))
    if t == "table":
        cells = [c for row in b.get("rows") or [] for c in (row if isinstance(row, list) else [row])]
        return count_words(*(b.get("cols") or []), *cells)
    if t == "kv":
        return count_words(*[x for kv in b.get("kv") or [] for x in (kv.get("k"), kv.get("v"))])
    if t == "panel":
        return count_words(b.get("title"), b.get("sub"))
    if t == "progress":
        return count_words(b.get("label"), b.get("note"))
    if t == "stat":
        return count_words(b.get("value"), b.get("label"), b.get("sub"))
    if t == "image":
        return count_words(b.get("caption"))
    if t == "chart":
        return chart_words(b)
    if t == "diagram":
        return diagram_words(b)
    if t == "details":
        return count_words(b.get("summary"))
    return 0


def walk(blocks):
    """Every block in page order, containers before their children."""
    for b in blocks or []:
        if not isinstance(b, dict):
            continue
        yield b
        if b.get("t") == "columns":
            for col in b.get("cols") or []:
                yield from walk(col)
        elif b.get("t") == "details":
            yield from walk(b.get("blocks"))


def hidden_words(blocks):
    total = 0
    for b in walk(blocks):
        if b.get("t") == "details":
            total += sum(block_words(x) for x in walk(b.get("blocks")))
    return total


def visible_blocks(blocks):
    """Blocks a reader sees without opening anything: details children left out."""
    for b in blocks or []:
        if not isinstance(b, dict):
            continue
        yield b
        if b.get("t") == "columns":
            for col in b.get("cols") or []:
                yield from visible_blocks(col)


def prose_runs(blocks):
    """Word counts of each run of prose blocks within one block list."""
    runs, current = [], 0
    for b in blocks or []:
        if not isinstance(b, dict):
            continue
        if b.get("t") in PROSE:
            current += block_words(b)
            continue
        if current:
            runs.append(current)
        current = 0
        if b.get("t") == "columns":
            for col in b.get("cols") or []:
                runs += prose_runs(col)
    if current:
        runs.append(current)
    return runs


def palette_roles(doc, graph):
    roles = set()
    for s in doc.get("sections") or []:
        if s.get("tone"):
            roles.add(s["tone"])
        for b in walk(s.get("blocks")):
            t = b.get("t")
            if t == "panel" and b.get("accent"):
                roles.add(b["accent"])
            elif t == "callout" and b.get("sev"):
                roles.add(SEV_ROLE.get(b["sev"], b["sev"]))
            elif t == "chart":
                for i, series in enumerate(b.get("series") or []):
                    roles.add(series.get("color") or f"series-{i % 4 + 1}")
            elif t == "diagram":
                for x in (b.get("nodes") or []) + (b.get("groups") or []):
                    if x.get("tone") and x["tone"] != "neutral":
                        roles.add(x["tone"])
    for n in (graph or {}).get("nodes") or []:
        if isinstance(n, dict) and n.get("tone") and n["tone"] != "neutral":
            roles.add(n["tone"])
    return sorted(roles)


def words_before_first_figure(doc):
    total = count_words(doc.get("summary"), doc.get("meta"),
                        *[c.get("text") for c in doc.get("chips") or [] if isinstance(c, dict)])
    for s in doc.get("sections") or []:
        total += count_words(s.get("h"))
        for b in visible_blocks(s.get("blocks")):
            if b.get("t") in FIGURES:
                return total, True
            total += block_words(b)
    return total, False


def doc_metrics(doc, graph):
    sections = doc.get("sections") or []
    all_blocks = [b for s in sections for b in walk(s.get("blocks"))]
    leaves = [b for b in all_blocks if b.get("t") not in CONTAINERS]
    figures = [b for b in leaves if b.get("t") in FIGURES]
    visible = [b for s in sections for b in visible_blocks(s.get("blocks"))]
    words = count_words(doc.get("summary"), doc.get("meta"),
                        *[c.get("text") for c in doc.get("chips") or [] if isinstance(c, dict)],
                        *[s.get("h") for s in sections]) + sum(block_words(b) for b in visible)
    before, has_figure = words_before_first_figure(doc)
    runs = [r for s in sections for r in prose_runs(s.get("blocks"))]
    return {
        "words": words,
        "words_hidden": sum(hidden_words(s.get("blocks")) for s in sections),
        "words_before_first_figure": before if has_figure else None,
        "sections": len(sections),
        "blocks": len(leaves),
        "block_types": dict(Counter(b.get("t") for b in all_blocks).most_common()),
        "figures": len(figures),
        "figure_share": round(len(figures) / len(leaves), 2) if leaves else 0.0,
        "longest_prose_run": max(runs) if runs else 0,
        "panels": sum(1 for b in all_blocks if b.get("t") == "panel"),
        "callouts": sum(1 for b in all_blocks if b.get("t") == "callout"),
        **role_metrics(doc, graph, len(leaves)),
        "headings_with_verb": verb_share([s.get("h") for s in sections]),
    }


def role_metrics(doc, graph, blocks):
    roles = palette_roles(doc, graph)
    return {"palette_roles": roles, "palette_role_count": len(roles),
            "roles_per_block": round(len(roles) / blocks, 2) if blocks else 0.0}


def slide_words(section):
    return count_words(section.get("h")) + sum(block_words(b) for b in visible_blocks(section.get("blocks")))


def deck_metrics(deck, graph=None):
    slides = deck.get("sections") or []
    per_slide = [slide_words(s) for s in slides]
    idea_blocks = [len([b for b in s.get("blocks") or [] if isinstance(b, dict)]) for s in slides]
    leaves = [b for s in slides for b in walk(s.get("blocks")) if b.get("t") not in CONTAINERS]
    return {
        "slides": len(slides),
        "words_per_slide_max": max(per_slide) if per_slide else 0,
        "words_per_slide_median": round(statistics.median(per_slide)) if per_slide else 0,
        "words_per_slide": per_slide,
        "slides_multi_idea": sum(1 for n in idea_blocks if n > 1),
        "titles_with_verb": verb_share([s.get("h") for s in slides]),
        "titles_without_verb": [s.get("h") for s in slides if s.get("h") and not has_verb(s.get("h"))],
        "layouts": dict(Counter(s.get("layout") or "default" for s in slides)),
        **role_metrics(deck, graph, len(leaves)),
    }


def rendered_metrics(render):
    out = {}
    brief = (render.get("brief") or {}).get("light")
    if brief:
        out.update(height_px=brief["height"], screens=brief["screens_total"],
                   brief_ready=all(render["brief"][t]["ready"] for t in render["brief"]),
                   charts_drawn=f"{brief['charts_built']}/{brief['charts']}",
                   diagrams_drawn=f"{brief['diagrams_built']}/{brief['diagrams']}",
                   figure_shots=brief.get("figures_shot", 0),
                   images=brief.get("images", 0), images_missing=brief.get("images_missing", 0),
                   error_text=sorted({e for t in render["brief"].values() for e in t["error_text"]}))
    deck = (render.get("deck") or {}).get("light")
    if deck:
        slides = deck["per_slide"]
        out.update(deck_slides_rendered=deck["slides"],
                   deck_not_ready=sum(1 for t in render["deck"].values() for s in t["per_slide"] if not s["ready"]),
                   deck_images=sum(s.get("images", 0) for s in slides),
                   deck_images_missing=sum(s.get("images_missing", 0) for s in slides),
                   deck_error_text=sorted({e for t in render["deck"].values() for s in t["per_slide"]
                                           for e in s["error_text"]}))
    return out


def _parse(value):
    if not value:
        return None
    return json.loads(value) if isinstance(value, str) else value


def page_metrics(run_dir, case_id, model):
    gen = read_json(gen_dir(run_dir, case_id, model) / "gen.json") or {}
    render = read_json(render_dir(run_dir, case_id, model) / "render.json") or {}
    row = {"case": case_id, "model": model, "gen_status": gen.get("status", "missing"),
           "render_status": render.get("status", "missing")}
    row["valid"] = row["gen_status"] == "ok" and render.get("status") == "ok"
    row["error"] = gen.get("error") or render.get("error") or ""
    args = read_json(gen_dir(run_dir, case_id, model) / "args.json")
    if not args:
        return row
    try:
        content, deck, graph = (_parse(args.get(k)) for k in ("content", "deck", "graph"))
    except ValueError as exc:
        row["error"] = f"args do not parse: {exc}"
        return row
    graph = graph if isinstance(graph, dict) else None
    if isinstance(content, dict):
        row["brief"] = doc_metrics(content, graph)
    if isinstance(deck, dict):
        row["deck"] = deck_metrics(deck, graph)
    row["rendered"] = rendered_metrics(render)
    row["probes"] = read_probes(render_dir(run_dir, case_id, model))
    return row


def histogram_text(hist, limit=6):
    return " ".join(f"{k}:{v}" for k, v in list(hist.items())[:limit])


def brief_line(r):
    b, rend = r["brief"], r.get("rendered", {})
    before = b["words_before_first_figure"]
    cells = [r["case"], r["model"], "yes" if r["valid"] else "**no**", b["words"],
             "none" if before is None else before, b["sections"], f"{b['figure_share']:.2f}",
             b["longest_prose_run"], b["panels"], b["callouts"], b["palette_role_count"],
             f"{b['roles_per_block']:.2f}", fmt_share(b.get("headings_with_verb")),
             rend.get("screens", ""), f"{rend.get('images_missing', 0)}/{rend.get('images', 0)}",
             histogram_text(b["block_types"])]
    return "| " + " | ".join(str(c) for c in cells) + " |"


def deck_line(r):
    d, rend = r["deck"], r.get("rendered", {})
    cells = [r["case"], r["model"], d["slides"], d["words_per_slide_max"], d["words_per_slide_median"],
             d["slides_multi_idea"], fmt_share(d.get("titles_with_verb")), d["palette_role_count"],
             f"{d['roles_per_block']:.2f}",
             f"{rend.get('deck_images_missing', 0)}/{rend.get('deck_images', 0)}", histogram_text(d["layouts"])]
    return "| " + " | ".join(str(c) for c in cells) + " |"


def fmt_share(value):
    return "" if value is None else f"{value:.2f}"


def table(rows):
    lines = ["| case | model | valid | words | before fig | sections | fig share | longest prose | panels | "
             "callouts | roles | roles/block | headings w/ verb | screens | images missing | blocks |",
             "|" + "---|" * 16]
    lines += [brief_line(r) for r in rows if r.get("brief")]
    decks = [r for r in rows if r.get("deck")]
    if decks:
        lines += ["", "| case | model | slides | words/slide max | median | multi-idea slides | titles w/ verb | "
                  "roles | roles/block | images missing | layouts |", "|" + "---|" * 11]
        lines += [deck_line(r) for r in decks]
    invalid = [r for r in rows if not r["valid"]]
    if invalid:
        lines += [""] + [f"- invalid: {r['case']} / {r['model']}: {r['error'][:200]}" for r in invalid]
    return "\n".join(lines) + "\n"


def metrics(run_dir, run):
    rows = [page_metrics(run_dir, c, m) for c in run["cases"] for m in run["models"]]
    write_json(Path(run_dir) / "metrics.json", rows)
    probed = probe_table(rows)
    text = table(rows) + ("\nPage probes (probes.py):\n\n" + "\n".join(probed) + "\n" if probed else "")
    write_text(Path(run_dir) / "metrics.md", text)
    print(text)
    log(f"metrics: {Path(run_dir) / 'metrics.json'}")
    return rows
