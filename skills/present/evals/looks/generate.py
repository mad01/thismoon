"""Step 1: one `claude -p` child per case and model writes the present_create arguments.

The child runs as a stream-json conversation, so its event log (stream.jsonl)
records every file it Read, and an optional second turn can follow the first:
`lint` feeds back a command's output over the turn-1 Doc once, `neutral` asks
for one review and revision with no findings.
"""

import json
import re
import shlex
import subprocess
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from claude_cli import StreamSession, isolation_args, result_record
from common import log, model_key, read_json, write_json, write_text

GEN_MAX_TURNS = 12
GEN_BUDGET_USD = 4.0
GEN_TIMEOUT_S = 1200
LINT_TIMEOUT_S = 120
LINT_MAX_CHARS = 8000
FEEDBACK_MODES = ("none", "lint", "neutral")

REPLY_RULES = (
    "The present MCP is not available in this session, so do not try to call it. "
    "Instead, write the arguments you would pass to `present_create` as one JSON object "
    "with the key `title` and whichever of `content`, `deck`, `graph` and `references` "
    "you use, inside a single ```json fenced block. `content`, `deck` and `graph` may be "
    "JSON objects rather than strings. Put the fenced block last in your reply."
)
REVISE_RULES = (
    "Then reply with the full revised `present_create` arguments the same way as before: one JSON "
    "object in a single ```json fenced block, last in your reply, even if nothing changed."
)
NEUTRAL_PROMPT = "Review your page once and revise it. " + REVISE_RULES
LINT_PROMPT = ("A lint check ran over the page you wrote; its output is between the <lint> markers. "
               "Revise the page once in light of it. " + REVISE_RULES + "\n\n<lint>\n{output}\n</lint>")

ARG_KEYS = ("title", "content", "deck", "graph", "references")
FENCE = re.compile(r"```(?:json)?[ \t]*\n(.*?)\n[ \t]*```", re.S)


def system_prompt(skill_dir):
    skill = (Path(skill_dir) / "SKILL.md").read_text()
    return (
        "The `present` skill is loaded for this conversation. Its SKILL.md follows between "
        "the <skill> markers. The skill's directory is "
        f"{skill_dir}: relative links in SKILL.md, such as examples/stepped-charts.json, "
        "resolve there, and you may Read those files.\n\n<skill>\n" + skill + "\n</skill>\n"
    )


def user_prompt(case):
    src = case["source"]
    return (
        f"{case['ask']}\n\n"
        f"The source is `{src['path']}` from {src['repo']} at commit {src['commit'][:12]}:\n\n"
        f"<source>\n{case['source_text']}\n</source>\n\n{REPLY_RULES}"
    )


def _candidates(reply):
    blocks = FENCE.findall(reply)
    out = list(reversed(blocks)) + [reply.strip()]
    if "{" in reply:
        out.append(reply[reply.find("{"): reply.rfind("}") + 1])
    return out


def extract_args(reply):
    """Return (args, error): the present_create arguments found in the reply."""
    error = "no JSON object in the reply"
    for text in _candidates(reply):
        try:
            obj = json.loads(text)
        except ValueError as exc:
            error = f"reply JSON does not parse: {exc}"
            continue
        if isinstance(obj, dict):
            return normalize_args(obj)
    return None, error


def normalize_args(obj):
    """Shape a parsed object into present_create arguments, strings where the tool wants them."""
    title = obj.get("title")
    if not isinstance(title, str) or not title.strip():
        return None, "the JSON object has no title"
    if not obj.get("content") and not obj.get("deck"):
        return None, "the JSON object has neither content nor deck"
    args = {"title": title}
    for key in ("content", "deck", "graph"):
        value = obj.get(key)
        if value in (None, "", {}):
            continue
        args[key] = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False)
    if obj.get("references"):
        args["references"] = obj["references"]
    extra = sorted(set(obj) - set(ARG_KEYS))
    return args, ("ignored keys: " + ", ".join(extra)) if extra else ""


def child_args(model, skill_dir, system_file, effort):
    args = ["--model", model, *isolation_args(),
            "--tools", "Read", "--allowedTools", "Read", "--permission-mode", "dontAsk",
            "--add-dir", str(skill_dir), "--append-system-prompt-file", str(system_file),
            "--max-turns", str(GEN_MAX_TURNS), "--max-budget-usd", str(GEN_BUDGET_USD)]
    if effort:
        args += ["--effort", effort]
    return args


def gen_dir(run_dir, case_id, model):
    return Path(run_dir) / "gen" / case_id / model_key(model)


# --- what the child read --------------------------------------------------------

def relative_to(path, base):
    try:
        return Path(path).resolve().relative_to(Path(base).resolve()).as_posix()
    except ValueError:
        return str(path)


def tool_reads(events, skill_dir):
    """Every Read call in the event log: the path (relative when inside the skill
    directory), the conversation turn it fell in, and whether it returned the file."""
    reads, by_id, turn = [], {}, 1
    for event in events:
        kind = event.get("type")
        if kind == "result":
            turn += 1
            continue
        content = (event.get("message") or {}).get("content")
        for item in content if isinstance(content, list) else []:
            if kind == "assistant" and item.get("type") == "tool_use" and item.get("name") == "Read":
                path = (item.get("input") or {}).get("file_path", "")
                by_id[item.get("id")] = {"path": relative_to(path, skill_dir), "turn": turn, "ok": None}
                reads.append(by_id[item.get("id")])
            elif kind == "user" and item.get("type") == "tool_result" and item.get("tool_use_id") in by_id:
                by_id[item["tool_use_id"]]["ok"] = not item.get("is_error")
    return reads


# --- the turns --------------------------------------------------------------------

def turn_record(session, event):
    rec = result_record(event, "", session.wall_s())
    if event is None:
        rec["error"] = (f"timed out after {GEN_TIMEOUT_S}s" if session.timed_out
                        else "claude ended without a result")
    return rec


def parse_reply(rec):
    if not rec["ok"]:
        return None, rec["error"]
    reply = rec["result"] if isinstance(rec["result"], str) else json.dumps(rec["result"])
    return extract_args(reply)


def run_lint(cmd, out, args):
    """Run the lint command over the turn-1 Doc; return (output for the prompt, record).

    {args}, {content} and {deck} in cmd become the paths of args-turn1.json and
    the turn-1 content and deck Docs ('' when the page has none)."""
    paths = {"args": out / "args-turn1.json"}
    for key in ("content", "deck"):
        if args.get(key):
            paths[key] = out / f"{key}-turn1.json"
            write_text(paths[key], args[key])
    command = cmd
    for key in ("args", "content", "deck"):
        command = command.replace("{" + key + "}", shlex.quote(str(paths[key])) if key in paths else "''")
    try:
        proc = subprocess.run(command, shell=True, capture_output=True, text=True, timeout=LINT_TIMEOUT_S,
                              cwd=str(out))
        output, code = (proc.stdout + proc.stderr).strip(), proc.returncode
    except subprocess.TimeoutExpired:
        output, code = f"the lint command timed out after {LINT_TIMEOUT_S}s", None
    write_text(out / "lint.txt", output + "\n")
    return output[:LINT_MAX_CHARS], {"lint_cmd": cmd, "lint_exit": code, "lint_chars": len(output)}


def feedback_prompt(feedback, out, args):
    if feedback["mode"] == "neutral":
        return NEUTRAL_PROMPT, {}
    output, info = run_lint(feedback["lint_cmd"], out, args)
    return LINT_PROMPT.format(output=output or "(the check printed nothing)"), info


def run_turns(session, prompt, feedback, out):
    """Turn 1, then the feedback turn when the run has one.

    Returns (records, args, note, feedback record). A second turn that ends in
    an error or gives no arguments keeps the turn-1 arguments, and says so."""
    first = turn_record(session, session.turn(prompt))
    args, note = parse_reply(first)
    fb = {"mode": feedback.get("mode", "none")}
    if fb["mode"] == "none":
        return [first], args, note, fb
    if args is None:
        fb["turn2"] = "skipped: turn 1 gave no arguments"
        return [first], args, note, fb
    write_text(out / "reply-turn1.md", first["result"])
    write_json(out / "args-turn1.json", args)
    text, extra = feedback_prompt(feedback, out, args)
    fb.update(extra)
    write_text(out / "prompt-turn2.md", text)
    second = turn_record(session, session.turn(text))
    revised, note2 = parse_reply(second)
    if revised is None:
        fb["turn2"] = f"{'invalid' if second['ok'] else 'error'}: {note2}; kept turn 1"
        return [first, second], args, note, fb
    fb["turn2"] = "ok"
    return [first, second], revised, note2, fb


def gen_record(records, session, stderr, case, model, fb, reads):
    first, last = records[0], records[-1]
    gen = {"cost_usd": max(r["cost_usd"] for r in records),  # total_cost_usd counts the session so far
           "turns": sum(r["turns"] for r in records),
           "duration_ms": sum(r["duration_ms"] for r in records),
           "wall_s": round(session.wall_s(), 1), "subtype": last["subtype"] or first["subtype"],
           "session_id": first["session_id"], "usage": first["usage"],
           "case": case, "model": model, "feedback": fb, "reads_logged": True, "reads": reads}
    if len(records) > 1:
        gen["usage_turn2"] = last["usage"]
    if not first["ok"]:
        gen.update(status="error", error=first["error"], stderr=stderr)
    return gen


def generate_one(run_dir, run, case, model, ledger, scratch):
    out = gen_dir(run_dir, case["id"], model)
    label = f"{case['id']} / {model}"
    if ledger.over_cap():
        log(f"gen {label}: skipped, spend cap ${ledger.cap:.2f} reached")
        return
    system_file = out / "system.md"
    write_text(system_file, system_prompt(run["skill_dir"]))
    prompt = user_prompt(case)
    write_text(out / "prompt.md", prompt)
    cwd = Path(scratch) / "cwd" / case["id"] / model_key(model)
    cwd.mkdir(parents=True, exist_ok=True)
    log(f"gen {label}: started")
    session = StreamSession(child_args(model, run["skill_dir"], system_file, run.get("effort")),
                            cwd, out / "stream.jsonl", GEN_TIMEOUT_S)
    try:
        records, args, note, fb = run_turns(session, prompt, run.get("feedback") or {}, out)
    finally:
        stderr = session.close()
    gen = gen_record(records, session, stderr, case["id"], model, fb, tool_reads(session.events, run["skill_dir"]))
    ledger.add("generate", label, gen["cost_usd"])
    final = records[-1] if fb.get("turn2") == "ok" else records[0]
    write_text(out / "reply.md", final["result"] if isinstance(final["result"], str) else json.dumps(final["result"]))
    if "status" not in gen:
        if args is None:
            gen.update(status="invalid", error=note)
        else:
            write_json(out / "args.json", args)
            gen.update(status="ok", note=note)
    write_json(out / "gen.json", gen)
    log(f"gen {label}: {gen['status']} ${gen['cost_usd']:.3f} {gen['turns']} turns {gen['wall_s']}s "
        f"{len(gen['reads'])} reads {gen.get('error', '')}")


def pending(run_dir, cases, models, redo_errors):
    todo = []
    for case in cases:
        for model in models:
            gen = read_json(gen_dir(run_dir, case["id"], model) / "gen.json")
            if gen is None or (redo_errors and gen.get("status") == "error"):
                todo.append((case, model))
    return todo


def generate(run_dir, run, cases, ledger, scratch, jobs, redo_errors):
    todo = pending(run_dir, cases, run["models"], redo_errors)
    log(f"generate: {len(todo)} to run, spend so far ${ledger.total():.2f}")
    with ThreadPoolExecutor(max_workers=max(1, jobs)) as pool:
        futures = [pool.submit(generate_one, run_dir, run, c, m, ledger, scratch) for c, m in todo]
        for fut in futures:
            fut.result()
