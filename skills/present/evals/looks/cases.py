"""Load the cases under cases/: one directory each, with case.json and source.md."""

import json
import sys

from common import CASES_DIR


def load_case(case_dir):
    meta = json.loads((case_dir / "case.json").read_text())
    if meta.get("id") != case_dir.name:
        sys.exit(f"{case_dir}: case.json id {meta.get('id')!r} does not match the directory")
    if meta.get("kind") not in ("brief", "deck"):
        sys.exit(f"{case_dir}: kind must be brief or deck")
    meta["source_text"] = (case_dir / "source.md").read_text()
    return meta


def load_cases(only=None):
    """Every case, or the comma-separated ids in only, in directory order."""
    dirs = sorted(p for p in CASES_DIR.iterdir() if (p / "case.json").exists())
    cases = [load_case(d) for d in dirs]
    if only:
        wanted = [c.strip() for c in only.split(",") if c.strip()]
        known = {c["id"] for c in cases}
        missing = [w for w in wanted if w not in known]
        if missing:
            sys.exit(f"unknown case(s): {', '.join(missing)}; known: {', '.join(sorted(known))}")
        cases = [c for c in cases if c["id"] in wanted]
    return cases
