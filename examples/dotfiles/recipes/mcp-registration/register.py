#!/usr/bin/env python3
"""Register the MCP servers declared in servers.json into Claude Code's user scope.

Idempotent by remove-then-add: each run first removes any existing registration
for the server, then re-adds it from servers.json. So a repeated `ralph up`
never errors on an already-registered name, and edits to a definition take
effect. Only stdio entries (those with a `command`) are handled — add http/url
handling if you register remote servers. Requires the `claude` CLI on PATH; the
caller guards on that. `~` in command and args is expanded so paths stay
portable across machines.
"""
import json
import os
import pathlib
import subprocess


def expand(value):
    return os.path.expanduser(value) if isinstance(value, str) else value


def build_add_command(name, spec):
    """Build the `claude mcp add` argv for one stdio server entry."""
    cmd = ["claude", "mcp", "add", "--scope", "user"]
    for key, value in spec.get("env", {}).items():
        cmd += ["--env", f"{key}={expand(value)}"]
    cmd += [name, "--", expand(spec["command"])]
    cmd += [expand(arg) for arg in spec.get("args", [])]
    return cmd


def main():
    here = pathlib.Path(__file__).resolve().parent
    servers = json.loads((here / "servers.json").read_text()).get("servers", {})
    for name, spec in servers.items():
        if name.startswith("_"):
            continue
        if "command" not in spec:
            print(f"skip {name}: this example handles only stdio (command) entries")
            continue
        # Remove any existing registration so the desired definition always wins
        # and a repeated run never fails on an already-registered name.
        subprocess.run(
            ["claude", "mcp", "remove", "--scope", "user", name],
            capture_output=True,
        )
        cmd = build_add_command(name, spec)
        print("+", " ".join(cmd))
        subprocess.run(cmd, check=True)


if __name__ == "__main__":
    main()
