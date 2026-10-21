#!/usr/bin/env python3
"""Render agents.json, the --agents file for unattended factory workers.

Experimental. claude -p with --setting-sources "" does not load user agents
from ~/.claude/agents, so an unattended worker gets the same three
definitions through --agents instead. This script derives that file from
skills/factory/agents/*.md so the two forms never drift; --check is the
gate agents-json-test.sh runs under make test-recipes.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
AGENTS_DIR = REPO / "skills" / "factory" / "agents"
AGENTS = sorted(AGENTS_DIR.glob("factory-worker-*.md"))
OUTPUT = HERE / "agents.json"


def parse(path: Path) -> tuple[str, dict]:
    """Split one agent file into its name and an --agents definition."""
    _, front, body = path.read_text().split("---\n", 2)
    fields = dict(line.split(": ", 1) for line in front.splitlines() if line)
    definition = {
        "description": fields["description"],
        "prompt": body.strip(),
        "tools": [tool.strip() for tool in fields["tools"].split(",")],
        "model": fields["model"],
        "effort": fields["effort"],
    }
    return fields["name"], definition


def render() -> str:
    return json.dumps(dict(parse(path) for path in AGENTS), indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    expected = render()
    if parser.parse_args().check:
        if not OUTPUT.exists() or OUTPUT.read_text() != expected:
            script = Path(__file__).resolve().relative_to(REPO)
            print(f"stale: {OUTPUT.relative_to(REPO)} (run python3 {script} from the repo root)")
            return 1
        return 0
    OUTPUT.write_text(expected)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
