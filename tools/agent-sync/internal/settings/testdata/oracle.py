"""Regenerate the want/ files under cases/ from the reference sync_settings.py.

The reference is recipes/claude/sync_settings.py in the dotfiles repo at
commit REFERENCE_COMMIT. It is read with git show, so a later edit to the
script cannot change the goldens. Run it with the Python the script ran on:

    python3 -I oracle.py <dotfiles checkout> [--out DIR] [CASE ...]

Each case runs the way internal/cli/settings_test.go runs it: a fresh temp
HOME laid out by setup_home, then every step in case.json in order, with the
script's TEMPLATE and PRUNE_PATHS pointed at the case. A step writes
<step>.stdout and <step>.result, plus <step>.live when the live path exists.
Without --out the files land in each case's want/, replacing the old ones.
With --out they land in DIR/<case>/want/, which is how a parity check runs
without touching the fixtures. Files ending in .deviation hold agent-sync's
own expectations and are never written or removed.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REFERENCE_COMMIT = "659e35240a14a96d0d489a8dfbda3f17c8500bd6"
REFERENCE_PATH = "recipes/claude/sync_settings.py"
CASES = Path(__file__).resolve().parent / "cases"

# The command-line flags each step name stands for.
FLAGS = {"sync": [], "plan": ["--dry-run"], "check": ["--check"]}

# Runs one step in a child process: loads the script without running it,
# points it at the case, then calls main. Lists in the prune JSON become
# tuples, so a flat list is the one-tuple-missing-its-comma mistake.
DRIVER = """
import json, sys, types
from pathlib import Path

source, home, prune, *flags = sys.argv[1:]

def as_tuple(v):
    return tuple(as_tuple(x) for x in v) if isinstance(v, list) else v

module = types.ModuleType("sync_settings")
module.__file__ = source
exec(compile(Path(source).read_text(), source, "exec"), module.__dict__)
module.TEMPLATE = Path(home) / "template.json"
module.PRUNE_PATHS = as_tuple(json.loads(prune))
sys.argv = ["sync_settings.py", *flags]
module.main()
"""


def setup_home(case, live_link):
    """Lay the case out as settingsTest's setupHome does; return the HOME."""
    home = Path(os.path.realpath(tempfile.mkdtemp(prefix="oracle-home-")))
    claude = home / ".claude"
    claude.mkdir()
    copy_if_present(case / "template.json", home / "template.json")
    copy_if_present(case / "overlay.json", claude / "settings.local.json")
    live = claude / "settings.json"
    target = home / "live-target.json"
    if live_link in ("symlink", "hardlink"):
        if not copy_if_present(case / "live.json", target):
            sys.exit(f"{case.name}: a linked live file needs live.json")
        if live_link == "symlink":
            os.symlink("../live-target.json", live)
        else:
            os.link(target, live)
    elif live_link == "dangling":
        os.symlink("../live-target.json", live)
    else:
        copy_if_present(case / "live.json", live)
    return home


def copy_if_present(src, dst):
    if not src.exists():
        return False
    shutil.copyfile(src, dst)
    return True


def live_state(home):
    """Return the live path's kind and content, as liveState does."""
    live = home / ".claude" / "settings.json"
    try:
        info = live.lstat()
    except FileNotFoundError:
        return "absent", b""
    if live.is_symlink():
        kind = "symlink"
    elif info.st_nlink > 1:
        kind = "hardlink"
    else:
        kind = "file"
    try:
        return kind, live.read_bytes()
    except FileNotFoundError:
        return kind, b""  # a dangling link


def run_case(source, case, want):
    spec = json.loads((case / "case.json").read_text())
    home = setup_home(case, spec.get("live_link", ""))
    env = {k: v for k, v in os.environ.items() if k not in ("CLAUDE_CONFIG_DIR", "XDG_CONFIG_HOME")}
    env["HOME"] = str(home)
    want.mkdir(parents=True, exist_ok=True)
    for old in want.iterdir():
        if not old.name.endswith(".deviation"):
            old.unlink()
    try:
        for i, mode in enumerate(spec["steps"], 1):
            argv = [sys.executable, "-I", "-X", "utf8", "-c", DRIVER, str(source), str(home),
                    json.dumps(spec["prune"]), *FLAGS[mode]]
            proc = subprocess.run(argv, env=env, capture_output=True, cwd=home)
            kind, content = live_state(home)
            step = want / f"{i}-{mode}"
            stdout = proc.stdout.replace(str(home).encode(), b"{HOME}")
            Path(f"{step}.stdout").write_bytes(stdout)
            Path(f"{step}.result").write_text(f"exit: {proc.returncode}\nlive: {kind}\n")
            if kind != "absent":
                Path(f"{step}.live").write_bytes(content)
    finally:
        shutil.rmtree(home)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("dotfiles", type=Path, help="a dotfiles checkout that has the reference commit")
    parser.add_argument("--out", type=Path, help="write DIR/<case>/want/ instead of the fixtures")
    parser.add_argument("cases", nargs="*", help="case names (default: every case)")
    args = parser.parse_args()

    script = subprocess.run(
        ["git", "-C", str(args.dotfiles), "show", f"{REFERENCE_COMMIT}:{REFERENCE_PATH}"],
        check=True, capture_output=True,
    ).stdout
    names = args.cases or sorted(p.name for p in CASES.iterdir() if p.is_dir())
    with tempfile.TemporaryDirectory(prefix="oracle-") as tmp:
        source = Path(tmp) / "sync_settings.py"
        source.write_bytes(script)
        for name in names:
            want = (args.out / name if args.out else CASES / name) / "want"
            run_case(source, CASES / name, want)
            print(f"{name}: {want}")


if __name__ == "__main__":
    main()
