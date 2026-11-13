"""Paths, JSON files, run directories and the spend ledger shared by every step."""

import hashlib
import json
import os
import sys
import threading
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
CASES_DIR = HERE / "cases"
RESULTS_DIR = HERE / "results"
DEFAULT_SKILL_DIR = HERE.parent.parent  # skills/present
DEFAULT_SCRATCH = Path("/tmp/present-looks/harness")
DEFAULT_MODELS = ["claude-fable-5-2", "claude-opus-5-5"]
# The reading faces webkit offers today (FONT_STACKS in webkit/src/webkit.ts);
# any other stored name falls back to Fira Code.
FONTS = ("Fira Code", "Inter", "Lexend", "Work Sans")
# What the skill fingerprint hashes, relative to the skill directory.
SKILL_FILES = ("SKILL.md",)
SKILL_TREES = ("examples", "references", "gallery")

_print_lock = threading.Lock()


def log(msg):
    stamp = time.strftime("%H:%M:%S")
    with _print_lock:
        print(f"[{stamp}] {msg}", file=sys.stderr, flush=True)


def read_json(path, default=None):
    path = Path(path)
    if not path.exists():
        return default
    return json.loads(path.read_text())


def write_json(path, obj):
    """Write obj as indented JSON through a temp file and a rename."""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(json.dumps(obj, indent=2, ensure_ascii=False) + "\n")
    os.replace(tmp, path)


def write_text(path, text):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(text)
    os.replace(tmp, path)


def new_run_dir():
    # Two runs started in the same second would share a stamp; the second
    # waits for the next one.
    while True:
        path = RESULTS_DIR / time.strftime("%Y%m%d-%H%M%S")
        try:
            path.mkdir(parents=True, exist_ok=False)
            return path
        except FileExistsError:
            time.sleep(1)


def resolve_run_dir(arg):
    """Turn --run (a stamp, a path, or 'latest') into an existing run directory.

    'latest' is the newest generate run; pairwise runs are skipped.
    """
    if arg in (None, "", "latest"):
        runs = sorted(p for p in RESULTS_DIR.glob("*")
                      if (read_json(p / "run.json") or {}).get("kind") == "absolute")
        if not runs:
            sys.exit("no run under results/; start one with: run.py generate")
        return runs[-1]
    path = Path(arg)
    if not path.is_absolute() and not path.exists():
        path = RESULTS_DIR / arg
    if not (path / "run.json").exists():
        sys.exit(f"{path} is not a run directory (no run.json)")
    return path.resolve()


def model_key(model):
    """Directory name for a model id."""
    return model.replace("/", "_")


def file_sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def skill_files(skill_dir):
    """The fingerprinted files of a skill, as sorted paths relative to skill_dir."""
    skill_dir = Path(skill_dir)
    files = [name for name in SKILL_FILES if (skill_dir / name).is_file()]
    for tree in SKILL_TREES:
        files += [p.relative_to(skill_dir).as_posix() for p in (skill_dir / tree).rglob("*") if p.is_file()]
    return sorted(files)


def skill_fingerprint(skill_dir):
    """sha256 over each fingerprinted file's relative path and bytes, in path order,
    so a renamed, added or moved reference changes it as much as an edit does."""
    digest = hashlib.sha256()
    for rel in skill_files(skill_dir):
        digest.update(rel.encode() + b"\0")
        digest.update((Path(skill_dir) / rel).read_bytes() + b"\0")
    return digest.hexdigest()


def arm_fingerprint(run):
    """A run's skill fingerprint for display: the relative-path hash, or the older
    name-only hash over SKILL.md and examples/* marked v1."""
    if run.get("skill_fingerprint"):
        return run["skill_fingerprint"][:12]
    return f"v1:{run.get('skill_sha256', '')[:12]}" if run.get("skill_sha256") else ""


class Ledger:
    """Append-only record of every paid call in a run, with a spend cap."""

    def __init__(self, run_dir, cap_usd):
        self.path = Path(run_dir) / "spend.jsonl"
        self.cap = cap_usd
        self._lock = threading.Lock()

    def total(self):
        if not self.path.exists():
            return 0.0
        total = 0.0
        for line in self.path.read_text().splitlines():
            if line.strip():
                total += float(json.loads(line).get("cost_usd") or 0)
        return total

    def add(self, step, label, cost_usd):
        with self._lock:
            with self.path.open("a") as fh:
                row = {"time": time.strftime("%Y-%m-%dT%H:%M:%S"), "step": step,
                       "label": label, "cost_usd": cost_usd}
                fh.write(json.dumps(row) + "\n")

    def over_cap(self):
        return self.total() >= self.cap
