"""Run one isolated `claude -p` child and read its JSON result (run_claude), or hold
a stream-json conversation with one for a second turn and its event log
(StreamSession).

Isolation follows evals/harness (claude.go): no setting sources, so no
CLAUDE.md, hooks, permission rules or user MCP servers; hooks and auto memory
off through --settings; no session file; and the variables that would nest
the child in the calling session stripped from its environment.
"""

import json
import os
import signal
import subprocess
import tempfile
import threading
import time

# The names evals/harness/claude.go sessionEnv strips, plus present's own.
STRIP_ENV = {
    "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SESSION_ID",
    "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_SESSION_ATTENDED",
    "CLAUDE_CODE_MESSAGING_SOCKET", "CLAUDE_CODE_MESSAGING_TOKEN",
    "CLAUDE_PROJECT_DIR", "CLAUDE_PID", "CLAUDE_CODE_EFFORT_LEVEL",
    "CLAUDE_EFFORT", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS",
    "REX_SERVER", "REX_SESSION", "REX_BLOCK",
}

ISOLATION_SETTINGS = json.dumps({"disableAllHooks": True, "autoMemoryEnabled": False})


def child_env():
    return {k: v for k, v in os.environ.items() if k not in STRIP_ENV}


def isolation_args():
    return [
        "--setting-sources", "",
        "--no-session-persistence",
        "--strict-mcp-config",
        "--settings", ISOLATION_SETTINGS,
    ]


def claude_version():
    try:
        out = subprocess.run(["claude", "--version"], capture_output=True, text=True,
                             timeout=30, env=child_env())
        return out.stdout.strip()
    except (OSError, subprocess.TimeoutExpired) as exc:
        return f"unknown ({exc})"


def _kill_group(proc):
    try:
        os.killpg(proc.pid, signal.SIGTERM)
        proc.wait(timeout=10)
    except (ProcessLookupError, subprocess.TimeoutExpired):
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def run_claude(args, prompt, cwd, timeout_s):
    """Run `claude -p <args> --output-format json -- <prompt>` and return a record.

    The record always has: ok, result, cost_usd, turns, duration_ms, wall_s,
    is_error, subtype, session_id, usage, stderr, error. A timeout, a bad exit
    or unparseable output is ok=False with error set, never an exception.
    """
    cmd = ["claude", "-p", "--output-format", "json", *args, "--", prompt]
    started = time.monotonic()
    proc = subprocess.Popen(cmd, cwd=str(cwd), env=child_env(), stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, text=True, start_new_session=True)
    try:
        stdout, stderr = proc.communicate(timeout=timeout_s)
    except subprocess.TimeoutExpired:
        _kill_group(proc)
        stdout, stderr = proc.communicate()
        rec = _parse(stdout, stderr, time.monotonic() - started)
        rec.update(ok=False, error=f"timed out after {timeout_s}s")
        return rec
    rec = _parse(stdout, stderr, time.monotonic() - started)
    if proc.returncode != 0 and not rec["error"]:
        rec.update(ok=False, error=f"claude exited {proc.returncode}: {stderr.strip()[-500:]}")
    return rec


def _empty_record(stderr, wall_s):
    return {"ok": False, "result": "", "cost_usd": 0.0, "turns": 0, "duration_ms": 0,
            "wall_s": round(wall_s, 1), "is_error": False, "subtype": "", "session_id": "",
            "usage": {}, "stderr": (stderr or "").strip()[-2000:], "error": ""}


def result_record(data, stderr, wall_s):
    """A record from one result event (the JSON object --output-format json prints,
    or a stream-json line of type result); data None means the child printed none."""
    rec = _empty_record(stderr, wall_s)
    if data is None:
        rec["error"] = "claude printed no result"
        return rec
    rec.update(
        result=data.get("result") or "",
        cost_usd=float(data.get("total_cost_usd") or 0.0),
        turns=int(data.get("num_turns") or 0),
        duration_ms=int(data.get("duration_ms") or 0),
        is_error=bool(data.get("is_error")),
        subtype=data.get("subtype") or "",
        session_id=data.get("session_id") or "",
        usage=data.get("usage") or {},
    )
    if rec["is_error"] or rec["subtype"] != "success":
        rec["error"] = f"claude result {rec['subtype'] or 'error'}: {str(rec['result'])[:500]}"
        if "Not logged in" in str(rec["result"]):
            rec["error"] = "claude is not logged in"
    else:
        rec["ok"] = True
    return rec


def _parse(stdout, stderr, wall_s):
    text = (stdout or "").strip()
    if not text:
        rec = _empty_record(stderr, wall_s)
        rec["error"] = "claude printed nothing on stdout"
        return rec
    try:
        data = json.loads(text.splitlines()[-1])
    except ValueError as exc:
        rec = _empty_record(stderr, wall_s)
        rec["error"] = f"claude output is not JSON: {exc}: {text[:300]}"
        return rec
    return result_record(data, stderr, wall_s)


class StreamSession:
    """One `claude -p` child fed user turns over stdin in stream-json, so a second
    turn can follow the first in the same conversation without a session file.

    Every event line the child prints is appended to log_path and kept in
    events; each turn returns that turn's result event. In a result event
    total_cost_usd counts the whole session so far, num_turns only that turn.
    A watchdog kills the child's process group after timeout_s.
    """

    def __init__(self, args, cwd, log_path, timeout_s):
        cmd = ["claude", "-p", "--input-format", "stream-json", "--output-format", "stream-json",
               "--verbose", *args]
        self.started = time.monotonic()
        self.events = []
        self.timed_out = False
        self._log = open(log_path, "w")
        self._err = tempfile.TemporaryFile(mode="w+")
        self.proc = subprocess.Popen(cmd, cwd=str(cwd), env=child_env(), stdin=subprocess.PIPE,
                                     stdout=subprocess.PIPE, stderr=self._err, text=True,
                                     start_new_session=True)
        self._timer = threading.Timer(timeout_s, self._expire)
        self._timer.daemon = True
        self._timer.start()

    def _expire(self):
        self.timed_out = True
        _kill_group(self.proc)

    def wall_s(self):
        return time.monotonic() - self.started

    def turn(self, text):
        """Send one user message; return its result event, or None if the child ended first."""
        message = {"type": "user", "message": {"role": "user", "content": [{"type": "text", "text": text}]}}
        try:
            self.proc.stdin.write(json.dumps(message) + "\n")
            self.proc.stdin.flush()
        except OSError:
            return None
        for line in self.proc.stdout:
            self._log.write(line)
            try:
                event = json.loads(line)
            except ValueError:
                continue
            self.events.append(event)
            if event.get("type") == "result":
                return event
        return None

    def close(self):
        """End the conversation and wait for the child; return its stderr tail."""
        try:
            self.proc.stdin.close()
        except OSError:
            pass
        for line in self.proc.stdout:
            self._log.write(line)
        try:
            self.proc.wait(timeout=30)
        except subprocess.TimeoutExpired:
            _kill_group(self.proc)
        self._timer.cancel()
        self._log.close()
        self._err.seek(0)
        stderr = self._err.read()
        self._err.close()
        return stderr.strip()[-2000:]
