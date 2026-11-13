"""Step 2: create each generated page in a scratch present, screenshot it, and probe it.

Shots per page, in shots/ beside render.json, all at 1440x900:
  brief-<theme>-first.png        the first screen
  brief-<theme>-full.png         the whole page
  brief-<theme>-overview.png     the whole page scaled to 1568 px on its long side
  brief-<theme>-screen-NN.png    screen NN (02 on) while scrolling down, at most 10 screens
  brief-<theme>-fig-NN.png       chart, diagram or graph NN centred in the viewport, at most 8
  deck-<theme>-NN.png            slide NN in presenting mode, every step shown, at most 20

probes.json beside them holds the page probes (probes.py), taken in the light
theme: the brief once its figures have drawn, and every slide in presenting
mode. `probe_run` takes the probes alone, without shots, for a run rendered
before the probes existed.
"""

import math
import shutil
import subprocess
import time
from contextlib import contextmanager
from pathlib import Path

from browser import VIEWPORT, Browser, BrowserError
from common import file_sha256, log, model_key, read_json, write_json
from generate import gen_dir
from present_io import (McpClient, Serve, build_present, build_present_ref, describe_binary,
                        prepare_workdir)
from probes import PROBE_THEME, page_record, probe_path, run_probe

THEMES = ("light", "dark")
MAX_SCREENS = 10
MAX_SLIDES = 20
OVERVIEW_PX = 1568
MAX_FIGURES = 8
FIGURE_SETTLE_S = 3.0
SCROLL_SETTLE_S = 1.5
SLIDE_SETTLE_S = 0.6
BUILD_SETTLE_S = 0.4
ACTIVE_SLIDE = "section.slide.active"


def render_dir(run_dir, case_id, model):
    return Path(run_dir) / "render" / case_id / model_key(model)


def image_size(path):
    out = subprocess.run(["sips", "-g", "pixelWidth", "-g", "pixelHeight", str(path)],
                         capture_output=True, text=True, check=True).stdout
    nums = [int(line.split(":")[1]) for line in out.splitlines() if "pixel" in line]
    return nums[0], nums[1]


def make_overview(src, dst):
    """A copy of src no larger than OVERVIEW_PX on its long side (the judge's image limit)."""
    width, height = image_size(src)
    if max(width, height) <= OVERVIEW_PX:
        shutil.copyfile(src, dst)
        return
    subprocess.run(["sips", "-Z", str(OVERVIEW_PX), str(src), "--out", str(dst)],
                   capture_output=True, check=True)


def first_family(css):
    """'Lexend' from 'Lexend, "Helvetica Neue", Arial, sans-serif'."""
    return css.split(",")[0].strip().strip("'\"") if css else ""


def open_themed(browser, url, theme, font):
    """Load url fresh with the reader theme and face set.

    Both live in localStorage on the origin and apply at page load, so they are
    stored from the origin's index page first. Going from there to url is
    always a full load; opening the same deck URL again would be a fragment
    change that keeps the old theme and presenting mode.
    """
    origin = url.split("/p/")[0]
    browser.open(origin + "/")
    browser.set_reader(theme, font)
    browser.open(url)


def check_reader(state, theme, font):
    if state["theme"] != theme:
        raise BrowserError(f"page loaded in theme {state['theme']!r}, wanted {theme!r}")
    if font and first_family(state.get("font")) != font:
        raise BrowserError(f"asked for face {font!r}, the page shows {first_family(state.get('font'))!r}")


def shoot_figures(browser, shots, theme):
    """One viewport shot per chart, diagram or graph, centred, after FIGURE_SETTLE_S.

    Full-page shots catch charts mid-animation (a line with one point, an empty
    sankey), so the judge reads figures from these shots instead.
    """
    total = browser.scroll_to_figure(-1)
    for n in range(min(total, MAX_FIGURES)):
        browser.scroll_to_figure(n)
        time.sleep(FIGURE_SETTLE_S)
        browser.screenshot(shots / f"brief-{theme}-fig-{n + 1:02d}.png")
    return total


def build_figures(browser):
    """Scroll every chart, diagram and graph into view once, so the ones built on sight draw."""
    total = browser.scroll_to_figure(-1)
    for n in range(total):
        browser.scroll_to_figure(n)
        time.sleep(BUILD_SETTLE_S)
    return browser.wait_ready()


def shoot_brief(browser, url, shots, font, probes):
    out = {}
    for theme in THEMES:
        open_themed(browser, url, theme, font)
        state = browser.wait_ready()
        check_reader(state, theme, font)
        browser.scroll_to(0)
        browser.screenshot(shots / f"brief-{theme}-first.png")
        screens = min(MAX_SCREENS, max(1, math.ceil(state["height"] / VIEWPORT[1])))
        for n in range(2, screens + 1):
            browser.scroll_to((n - 1) * VIEWPORT[1])
            time.sleep(SCROLL_SETTLE_S)
            browser.screenshot(shots / f"brief-{theme}-screen-{n:02d}.png")
        state["figures_total"] = shoot_figures(browser, shots, theme)
        state["figures_shot"] = min(state["figures_total"], MAX_FIGURES)
        if theme == PROBE_THEME:
            build_figures(browser)
            probes["brief"] = run_probe(browser, "brief")
            probes["font_used"] = first_family(state["font"])
        # Last: the full-page capture resizes the viewport, which restarts chart animations.
        browser.scroll_to(0)
        browser.screenshot(shots / f"brief-{theme}-full.png", full=True)
        make_overview(shots / f"brief-{theme}-full.png", shots / f"brief-{theme}-overview.png")
        state["screens_total"] = math.ceil(state["height"] / VIEWPORT[1])
        state["screens_shot"] = screens
        final = browser.wait_ready()
        state["images"], state["images_missing"] = final["images"], final["images"] - final["images_loaded"]
        out[theme] = state
    return out


def enter_presenting(browser, deck_url, theme, font, scratch_shot):
    """Open the deck at slide 1 and put it in presenting mode; return slide 1's state."""
    open_themed(browser, deck_url + "#1", theme, font)
    first = browser.wait_ready(scope=ACTIVE_SLIDE)
    check_reader(first, theme, font)
    browser.press("p")  # presenting mode: the slide alone, as the room sees it
    # The first frame headless Chrome draws after P shows the title slide
    # tiled 2x2 (seen on every first deck of a session). A throwaway shot
    # forces that frame, and the real shots come after it.
    browser.screenshot(scratch_shot)
    scratch_shot.unlink()
    time.sleep(SLIDE_SETTLE_S)
    browser.wait_ready(scope=ACTIVE_SLIDE)
    if not browser.eval("document.documentElement.classList.contains('presenting')"):
        raise BrowserError("the deck did not enter presenting mode on P")
    return first


def show_slide(browser, n):
    browser.eval(f"location.hash = '#{n}'; true")
    time.sleep(SLIDE_SETTLE_S)
    state = browser.wait_ready(scope=ACTIVE_SLIDE)
    if state["active_slide"] != n:
        raise BrowserError(f"asked for slide {n}, slide {state['active_slide']} is showing")
    if state["charts"] or state["diagrams"] or state["graph"]:
        # Chart.js animates on requestAnimationFrame, which no render marker reports.
        time.sleep(FIGURE_SETTLE_S - SLIDE_SETTLE_S)
    return state


def shoot_deck(browser, deck_url, shots, font, probes):
    out = {}
    for theme in THEMES:
        first = enter_presenting(browser, deck_url, theme, font, shots / "warmup.tmp.png")
        total = first["slides"]
        per_slide = []
        for n in range(1, min(total, MAX_SLIDES) + 1):
            state = show_slide(browser, n)
            browser.screenshot(shots / f"deck-{theme}-{n:02d}.png")
            per_slide.append({k: state[k] for k in ("active_slide", "ready", "charts", "diagrams",
                                                    "notes", "error_text", "images")})
            per_slide[-1]["images_missing"] = state["images"] - state["images_loaded"]
            if theme == PROBE_THEME:
                probes.setdefault("slides", []).append({"slide": n, **run_probe(browser, "slide")})
        if theme == PROBE_THEME:
            probes.setdefault("font_used", first_family(first["font"]))
        out[theme] = {"slides": total, "slides_shot": len(per_slide), "per_slide": per_slide,
                      "theme": first["theme"], "font": first_family(first["font"])}
    return out


def probe_deck(browser, deck_url, font, scratch_shot):
    first = enter_presenting(browser, deck_url, PROBE_THEME, font, scratch_shot)
    slides = []
    for n in range(1, min(first["slides"], MAX_SLIDES) + 1):
        show_slide(browser, n)
        slides.append({"slide": n, **run_probe(browser, "slide")})
    return slides, first_family(first["font"])


def ensure_page(out, args, mcp, workdir, binary_sha):
    """Create the page once; a stored page or refusal is reused while the workdir holds
    it and the same binary made it. present stores the HTML it compiled at create time,
    so a page made by another binary is made again."""
    page = read_json(out / "page.json")
    same_binary = page and page.get("present_sha256", binary_sha) == binary_sha
    if page and same_binary and page["status"] == "refused":
        return page
    if page and same_binary and page["status"] == "ok" and (Path(workdir) / "pages" / page["id"]).exists():
        return page
    answer, error = mcp.create(args)
    if error:
        page = {"status": "refused", "error": error}
    else:
        page = {"status": "ok", "id": answer["id"], "url": answer.get("url", ""),
                "deck_url": answer.get("deck_url", ""), "has_deck": bool(answer.get("has_deck"))}
    page["present_sha256"] = binary_sha
    write_json(out / "page.json", page)
    return page


def present_summary(present):
    """The renderer as each page records it."""
    return {k: present.get(k) for k in ("version", "sha256", "source", "ref", "commit")} | {
        "dirty": len(present.get("dirty") or [])}


def page_args(run_dir, case_id, model, out, label):
    """The generated arguments for a page, or None after writing why it is skipped."""
    gen = read_json(gen_dir(run_dir, case_id, model) / "gen.json")
    if not gen:
        log(f"render {label}: not generated yet")
        return None
    if gen.get("status") != "ok":
        reason = f"generation {gen['status']}: {gen.get('error', '')}"
        write_json(out / "render.json", {"status": "skipped", "error": reason})
        log(f"render {label}: skipped, {reason}")
        return None
    return read_json(gen_dir(run_dir, case_id, model) / "args.json")


def render_one(run_dir, case_id, model, ctx, redo):
    out = render_dir(run_dir, case_id, model)
    label = f"{case_id} / {model}"
    done = read_json(out / "render.json")
    if done and done["status"] != "skipped" and not redo:
        log(f"render {label}: done already")
        return
    args = page_args(run_dir, case_id, model, out, label)
    if args is None:
        return
    page = ensure_page(out, args, ctx["mcp"], ctx["workdir"], ctx["present"]["sha256"])
    if page["status"] != "ok":
        write_json(out / "render.json", {"status": "refused", "error": page["error"]})
        log(f"render {label}: present refused it: {page['error'][:200]}")
        return
    shots = out / "shots"
    shots.mkdir(parents=True, exist_ok=True)
    for old in [*shots.glob("brief-*.png"), *shots.glob("deck-*.png")]:
        old.unlink()  # a redo must not leave shots from the earlier pass behind
    result = {"status": "ok", "id": page["id"], "url": page["url"], "brief": {}, "deck": {},
              "present": present_summary(ctx["present"]), "font": {"requested": ctx["font"]}}
    probes = {}
    browser, font = ctx["browser"], ctx["font"]
    try:
        if "content" in args:
            result["brief"] = shoot_brief(browser, page["url"], shots, font, probes)
        if page["has_deck"]:
            result["deck"] = shoot_deck(browser, page["deck_url"], shots, font, probes)
    except (BrowserError, subprocess.SubprocessError) as exc:
        result.update(status="browser-error", error=str(exc))
    result["font"]["used"] = probes.get("font_used", "")
    write_json(out / "render.json", result)
    if result["status"] == "ok":
        write_json(probe_path(out), page_record(result["present"], font, probes.get("font_used", ""),
                                                probes.get("brief"), probes.get("slides")))
    log(f"render {label}: {result['status']} page {page['id']}")


def probe_one(run_dir, case_id, model, ctx, redo):
    """Probes alone for a page rendered earlier: no shots, render.json left as it is."""
    out = render_dir(run_dir, case_id, model)
    label = f"{case_id} / {model}"
    if probe_path(out).exists() and not redo:
        log(f"probe {label}: done already")
        return
    if (read_json(out / "render.json") or {}).get("status") != "ok":
        log(f"probe {label}: skipped, the page has no ok render")
        return
    args = read_json(gen_dir(run_dir, case_id, model) / "args.json")
    page = ensure_page(out, args, ctx["mcp"], ctx["workdir"], ctx["present"]["sha256"])
    if page["status"] != "ok":
        log(f"probe {label}: present refused it now: {page['error'][:200]}")
        return
    browser, font = ctx["browser"], ctx["font"]
    brief, slides, used = None, None, ""
    try:
        if "content" in args:
            open_themed(browser, page["url"], PROBE_THEME, font)
            check_reader(browser.wait_ready(), PROBE_THEME, font)
            used = first_family(build_figures(browser)["font"])
            brief = run_probe(browser, "brief")
        if page["has_deck"]:
            slides, used = probe_deck(browser, page["deck_url"], font, ctx["scratch"] / "warmup.tmp.png")
    except (BrowserError, subprocess.SubprocessError) as exc:
        log(f"probe {label}: browser error: {exc}")
        return
    write_json(probe_path(out), page_record(present_summary(ctx["present"]), font, used, brief, slides))
    log(f"probe {label}: ok")


def resolve_binary(run_dir, scratch, present_bin=None, present_ref=None, shots_binary=False):
    """The present binary and what it is: a given path, a git ref built in scratch,
    the binary that took the run's shots (shots_binary, for probes; rebuilt from its
    commit when the file is gone), or a build of this checkout."""
    refs = Path(scratch).parent / "refs"
    if present_bin:
        return describe_binary(Path(present_bin).expanduser().resolve(), {"source": "path"})
    if present_ref:
        return describe_binary(*build_present_ref(present_ref, refs))
    shot = read_json(Path(run_dir) / "render" / "present.json") if shots_binary else None
    if shot:
        binary = Path(shot["binary"])
        if binary.exists() and shot.get("sha256", file_sha256(binary)) == file_sha256(binary):
            return describe_binary(binary, {k: shot[k] for k in ("source", "ref", "commit", "dirty") if k in shot}
                                   or {"source": "shots"})
        commit = (shot.get("version") or {}).get("commit") or shot.get("commit")
        if commit and not shot.get("dirty"):
            log(f"the shots' binary {binary} is gone or changed; building {commit[:12]}")
            return describe_binary(*build_present_ref(commit, refs))
        raise SystemExit(f"the binary that took the shots ({binary}) is gone; pass --present-bin or --present-ref")
    return describe_binary(*build_present(scratch))


@contextmanager
def scratch_present(run_dir, scratch, port, present, font):
    """A present serve, its MCP and one browser session on the run's scratch workdir."""
    binary = Path(present["binary"])
    workdir = prepare_workdir(Path(scratch) / "workdir")
    with Serve(binary, workdir, port, Path(scratch) / "serve.log"):
        mcp = McpClient(binary, workdir, port, Path(scratch) / "mcp.stderr")
        browser = Browser(f"present-looks-{Path(run_dir).name}")
        try:
            browser.start()
            yield {"mcp": mcp, "browser": browser, "workdir": workdir, "present": present,
                   "font": font, "scratch": Path(scratch)}
        finally:
            mcp.close()
            browser.close()


def pages(run, only_cases):
    wanted = only_cases.split(",") if only_cases else None
    return [(c, m) for c in run["cases"] if not wanted or c in wanted for m in run["models"]]


def render(run_dir, run, scratch, port, present, font, redo, only_cases=None):
    write_json(Path(run_dir) / "render" / "present.json",
               {**present, "port": port, "font": font, "viewport": list(VIEWPORT),
                "workdir": str(Path(scratch) / "workdir")})
    with scratch_present(run_dir, scratch, port, present, font) as ctx:
        for case_id, model in pages(run, only_cases):
            render_one(run_dir, case_id, model, ctx, redo)


def probe_run(run_dir, run, scratch, port, present, font, redo, only_cases=None):
    log(f"probe: present {present['version'].get('version', '')} ({present['binary']}), "
        f"face {font or 'page default'}")
    with scratch_present(run_dir, scratch, port, present, font) as ctx:
        for case_id, model in pages(run, only_cases):
            probe_one(run_dir, case_id, model, ctx, redo)
