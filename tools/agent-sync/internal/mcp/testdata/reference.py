#!/usr/bin/env python3
"""Regenerate the mcp cases' want/ files from the reference sync_mcp.py.

Usage: python3 reference.py <path to sync_mcp.py> [case ...]

Each case runs in a fresh temporary HOME, laid out where sync_mcp.py looks
(and where internal/cli/mcp_test.go puts the same files for agent-sync),
with fake-claude.sh as `claude` on PATH. For every plan or sync step it
writes want/<n>-<step>.{stdout,stderr,result,argv,stamp}; a traceback on
stderr is written as .traceback instead, and the Go test then needs a
.stderr.deviation. check steps have no counterpart in the script: their
want files are written by hand and left alone here, as are .deviation
files. A case that uses call-<n> responses must put its check steps last,
since the script makes no calls for them.

Placeholders: {HOME}, {HOST} and {HOST_UPPER} in the case's input files
are replaced before the run; {HOME} and {CLAUDE} (the fake's path) replace
those values in the output. The stamp is recorded as the text it hashes,
so the Go test can check it on any machine.
"""

import hashlib
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
CASES = HERE / "cases"
FAKE = HERE / "fake-claude.sh"

# Where sync_mcp.py reads and writes, relative to HOME.
SERVERS_REL = "code/src/github.com/mad01/dotfiles/recipes/claude-mcp/servers.json"
OVERLAY_REL = ".config/claude-mcp/servers.d"
RALPH_REL = ".config/ralph/config.local.toml"
STAMP_REL = ".cache/claude-mcp/sync.hash"
FAKE_DIR_REL = "fake-claude"
BIN_REL = "bin"

FIELDS = ("stdout", "stderr", "result", "argv", "stamp", "traceback")


def substitute(text, home, host):
    return (
        text.replace("{HOME}", str(home))
        .replace("{HOST_UPPER}", host.upper())
        .replace("{HOST}", host)
    )


def place(src, dst, home, host):
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text(substitute(src.read_text(), home, host))


def lay_out(case_dir, meta, home, host):
    if (case_dir / "servers.json").exists():
        place(case_dir / "servers.json", home / SERVERS_REL, home, host)
    if (case_dir / "overlays").is_dir():
        for src in sorted((case_dir / "overlays").iterdir()):
            place(src, home / OVERLAY_REL / src.name, home, host)
    if (case_dir / "ralph.toml").exists():
        place(case_dir / "ralph.toml", home / RALPH_REL, home, host)
    if (case_dir / "stamp").exists():
        place(case_dir / "stamp", home / STAMP_REL, home, host)
    fake_dir = home / FAKE_DIR_REL
    fake_dir.mkdir()
    if (case_dir / "claude").is_dir():
        for src in sorted((case_dir / "claude").iterdir()):
            place(src, fake_dir / src.name, home, host)
    for rel, content in (meta.get("files") or {}).items():
        (home / rel).parent.mkdir(parents=True, exist_ok=True)
        (home / rel).write_text(content)
    bin_dir = home / BIN_REL
    bin_dir.mkdir()
    shutil.copy(FAKE, bin_dir / "claude")
    (bin_dir / "claude").chmod(0o755)


def environment(meta, home):
    path = "/usr/bin:/bin" if meta.get("no_claude") else f"{home / BIN_REL}:/usr/bin:/bin"
    return {"HOME": str(home), "PATH": path, "FAKE_CLAUDE_DIR": str(home / FAKE_DIR_REL)}


def normalize(text, home):
    return text.replace(str(home / BIN_REL / "claude"), "{CLAUDE}").replace(str(home), "{HOME}")


def argv_lines(home):
    log = home / FAKE_DIR_REL / "argv.log"
    return log.read_text().splitlines(keepends=True) if log.exists() else []


def stamp_state(script, home, host, env):
    stamp = home / STAMP_REL
    if not stamp.is_file():
        return "absent\n"
    content = stamp.read_text()
    probe = (
        "import json, sys\n"
        f"sys.path.insert(0, {str(script.parent)!r})\n"
        "import sync_mcp as m\n"
        "d = m.load_desired()\n"
        "sys.stdout.write(json.dumps(d, sort_keys=True) + '\\n' + m.current_host())\n"
    )
    proc = subprocess.run(
        [sys.executable, "-I", "-B", "-c", probe], env=env, capture_output=True, text=True
    )
    preimage = proc.stdout
    if proc.returncode == 0 and hashlib.sha256(preimage.encode()).hexdigest() + "\n" == content:
        body, _, got_host = preimage.rpartition("\n")
        assert got_host == host, (got_host, host)
        return "sha256 of:\n" + normalize(body, home) + "\n{HOST}\n"
    return "raw:\n" + content


def step_id(i, step):
    return f"{i}-" + "-".join(w.lstrip("-") for w in step.split())


def run_case(script, case_dir, host):
    meta = json.loads((case_dir / "case.json").read_text())
    want = case_dir / "want"
    want.mkdir(exist_ok=True)
    for f in want.iterdir():
        if not f.name.endswith(".deviation") and "-check" not in f.name:
            f.unlink()
    with tempfile.TemporaryDirectory() as tmp:
        home = Path(os.path.realpath(tmp))
        lay_out(case_dir, meta, home, host)
        env = environment(meta, home)
        seen = 0
        for i, step in enumerate(meta["steps"], 1):
            words = step.split()
            if words[0] == "check":
                seen = len(argv_lines(home))
                continue
            args = ["--dry-run"] if words[0] == "plan" else []
            if meta.get("prune"):
                args.append("--prune")
            args += words[1:]
            proc = subprocess.run(
                [sys.executable, "-I", "-B", str(script), *args],
                env=env,
                capture_output=True,
                text=True,
            )
            lines = argv_lines(home)
            out = {
                "stdout": normalize(proc.stdout, home),
                "result": f"exit: {proc.returncode}\n",
                "argv": normalize("".join(lines[seen:]), home),
                "stamp": stamp_state(script, home, host, env),
            }
            seen = len(lines)
            if "Traceback (most recent call last)" in proc.stderr:
                out["traceback"] = normalize(proc.stderr.strip().splitlines()[-1], home) + "\n"
            else:
                out["stderr"] = normalize(proc.stderr, home)
            for field in FIELDS:
                if field in out:
                    (want / f"{step_id(i, step)}.{field}").write_text(out[field])


def main():
    script = Path(sys.argv[1]).resolve()
    names = sys.argv[2:] or sorted(p.name for p in CASES.iterdir() if p.is_dir())
    host = socket.gethostname().lower()
    for name in names:
        run_case(script, CASES / name, host)
        print(f"ok {name}")


if __name__ == "__main__":
    main()
