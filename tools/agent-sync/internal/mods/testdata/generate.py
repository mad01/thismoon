#!/usr/bin/env python3
"""Regenerate cases/ from the reference sync_mods.py.

    python3 generate.py <dir holding sync_mods.py and test_sync_mods.py>

The reference is the dotfiles repo's recipes/claude-mods. Each case builds a
home with the reference tests' FakeHome and saves it as cases/<name>/home/,
the home path written as {HOME}. A state the case switches to between steps
is saved beside it as cases/<name>/<label>/, holding only the files it
changes. Then the case runs the way internal/cli/mods_test.go runs it: a
fresh copy of the home, testdata/fake-claude.sh first on PATH, every step
through sync_mods.main. want/<n>-<step>.{stdout,stderr,result,calls} record
the reference's output; an empty stderr or calls file is left out.
Hand-written want/*.deviation files survive a regeneration.

A setup may write {HOST} or {HOST_UPPER} into a file. Both stand for this
machine's hostname and are replaced when the home is copied.
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import shutil
import socket
import stat
import sys
import tempfile
from pathlib import Path
from unittest import mock

HERE = Path(__file__).resolve().parent
CASES_DIR = HERE / "cases"
FAKE_CLAUDE = HERE / "fake-claude.sh"
ARGS = {"check": ("--check",), "plan": ("--dry-run",), "sync": ()}

# The reference tests' names, plus the ids the cases use.
MKT = "thismoon"
SB, PP = "session-band@thismoon", "panel-policy@thismoon"
BOTH_ON = {SB: True, PP: True}
LAB = {"lab-pane": {"enabled": True, "profiles": ["lab"]}}

CASES: dict[str, dict] = {}


def case(name, steps=("check", "plan", "sync"), ports=(), env=None, no_claude=False, states=None, deviation=""):
    """Register a case: setup(fh) builds its home, steps run in order."""

    def register(setup):
        CASES[name] = {
            "setup": setup,
            "steps": list(steps),
            "ports": list(ports),
            "env": env or {},
            "no_claude": no_claude,
            "states": states or {},
            "deviation": deviation,
        }
        return setup

    return register


# --- setup helpers on top of FakeHome ----------------------------------------


def mods(fh, entries, marketplace=MKT, path=None):
    """Replace the base mods.json, the way the reference tests do."""
    doc = fh.mods_file(marketplace, entries)
    if path is not None:
        doc["marketplace"]["path"] = path
    fh.mods_json.write_text(json.dumps(doc))


def raw(fh, rel, text):
    """Write text to a file under the home, creating its parents."""
    path = fh.home / rel
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def known(fh, marketplaces):
    """Rewrite known_marketplaces.json with directory sources at the given paths."""
    entries = {name: {"source": {"source": "directory", "path": p}, "installLocation": p} for name, p in marketplaces.items()}
    raw(fh, ".claude/plugins/known_marketplaces.json", json.dumps(entries))


def in_sync(fh):
    """The base marketplace on disk and session-band installed and enabled."""
    fh.write_marketplace()
    fh.write_state({SB: True}, [SB])


def retire(fh, entries=None, installed=(SB, PP)):
    """RetireTest.retire: panel-policy retired, both installed and enabled."""
    mods(fh, RETIRE_MODS if entries is None else entries)
    fh.write_marketplace()
    fh.write_state({pid: True for pid in installed}, list(installed))


# --- plan and check basics ----------------------------------------------------


@case(
    "empty-home",
    ports=[
        "PlanTest.test_empty_home_registers_marketplace_and_installs_enabled_mods",
        "CheckModeTest.test_empty_home_reports_drift",
        "CheckModeTest.test_check_never_writes_or_calls_claude",
        "CheckModeTest.test_dry_run_prints_commands_and_exits_zero",
        "ApplyTest.test_apply_runs_the_planned_cli_calls_in_order",
    ],
)
def _(fh):
    fh.write_marketplace()


@case(
    "in-sync",
    ports=[
        "PlanTest.test_in_sync_state_plans_nothing",
        "PlanTest.test_disabled_mod_is_never_installed_just_to_be_disabled",
        "UpdatePlanTest.test_equal_versions_plan_nothing",
        "CheckModeTest.test_applied_state_is_clean",
        "ApplyTest.test_in_sync_apply_never_calls_claude",
    ],
)
def _(fh):
    in_sync(fh)


@case("installed-not-enabled", ports=["PlanTest.test_installed_but_disabled_mod_is_enabled_not_reinstalled"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({SB: False}, [SB])


@case("disabled-mod-enabled", ports=["PlanTest.test_disabled_mod_that_is_enabled_gets_disabled"])
def _(fh):
    fh.write_marketplace()
    fh.write_state(BOTH_ON, [SB, PP])


@case("disable-without-record")
def _(fh):
    # disable acts on enabledPlugins alone; no install record needed.
    fh.write_marketplace()
    fh.write_state(BOTH_ON, [SB])


@case("marketplace-repointed", ports=["PlanTest.test_marketplace_at_another_path_is_repointed"])
def _(fh):
    in_sync(fh)
    known(fh, {MKT: str(fh.home / "elsewhere")})


@case("marketplace-path-symlink", steps=["check"])
def _(fh):
    # The recorded path is a symlink to the folder: the same once resolved.
    in_sync(fh)
    (fh.home / "link").symlink_to(fh.marketplace_dir(MKT))
    known(fh, {MKT: str(fh.home / "link")})


@case("marketplace-path-spelling", steps=["check", "plan"])
def _(fh):
    # Recorded with ~ and a trailing slash, declared with a ./ segment.
    in_sync(fh)
    known(fh, {MKT: "~/sources/thismoon/"})
    mods(fh, {"session-band": {"enabled": True}}, path="~//sources/./thismoon/")


@case("every-verb", ports=["CliArgsTest.test_each_verb_maps_to_a_user_scope_plugin_command"])
def _(fh):
    names = ("to-install", "to-enable", "to-update", "to-disable", "to-retire")
    fh.write_marketplace(plugins=names, versions={"to-update": "0.2.0"})
    mods(
        fh,
        {
            "to-install": {"enabled": True},
            "to-enable": {"enabled": True},
            "to-update": {"enabled": True},
            "to-disable": {"enabled": False},
            "to-retire": {"installed": False},
        },
    )
    ids = [f"{n}@thismoon" for n in names[1:]]
    fh.write_state(
        {"to-enable@thismoon": False, "to-update@thismoon": True, "to-disable@thismoon": True, "to-retire@thismoon": True},
        ids,
    )
    known(fh, {MKT: str(fh.home / "old-path")})


@case(
    "unlisted-enabled",
    ports=[
        "PlanTest.test_unlisted_enabled_mod_is_skipped_with_a_note",
        "CheckModeTest.test_notes_follow_the_summary_on_stdout",
    ],
)
def _(fh):
    fh.write_marketplace(plugins=("panel-policy",))
    fh.write_state({}, [])


@case("notes-and-actions")
def _(fh):
    # Sync and plan print the notes before the actions; check after its summary.
    fh.write_marketplace(plugins=("panel-policy",))
    mods(fh, {"session-band": {"enabled": True}, "panel-policy": {"enabled": True}})


@case("unknown-state-untouched", steps=["check", "sync"], ports=["PlanTest.test_unknown_plugins_in_state_are_never_touched"])
def _(fh):
    fh.write_marketplace()
    fh.write_marketplace("other", ("x",))
    ids = [SB, "other@claude-plugins-official", "stray@thismoon"]
    fh.write_state(dict.fromkeys(ids, True), ids, (MKT, "other"))


@case("no-marketplace", ports=["CheckModeTest.test_missing_marketplace_warns_and_exits_zero"])
def _(fh):
    pass


@case(
    "claude-config-dir",
    steps=["check"],
    env={"CLAUDE_CONFIG_DIR": "{HOME}/alt-config"},
    ports=["CheckModeTest.test_claude_config_dir_relocates_the_state_files"],
)
def _(fh):
    fh.write_marketplace()
    fh.write_state({SB: True}, [SB], root=fh.home / "alt-config")


@case("claude-config-dir-tilde", steps=["check"], env={"CLAUDE_CONFIG_DIR": "~/alt-config"})
def _(fh):
    fh.write_marketplace()
    fh.write_state({SB: True}, [SB], root=fh.home / "alt-config")


# --- gating -------------------------------------------------------------------


@case(
    "gated-off-enabled",
    steps=["check", "sync"],
    ports=["PlanTest.test_gated_off_mod_is_left_alone_even_when_enabled"],
)
def _(fh):
    in_sync(fh)
    fh.write_profiles("personal")
    mods(fh, {"session-band": {"enabled": False, "profiles": ["lab"]}})


@case("profile-gated-count", steps=["check"], ports=["CheckModeTest.test_profile_gated_mod_counts_as_gated_off"])
def _(fh):
    in_sync(fh)
    mods(fh, {"session-band": {"enabled": True}, "lab-only": {"enabled": True, "profiles": ["lab"]}})


@case("gate-profiles", steps=["check", "plan"], ports=["GatingTest.test_profile_gate_needs_one_matching_tag"])
def _(fh):
    fh.write_profiles("Lab", "personal")
    entries = {
        "one-tag-matches": {"enabled": True, "profiles": ["work", "LAB"]},
        "no-tag-matches": {"enabled": True, "profiles": ["work"]},
        "no-gate": {"enabled": True},
        "empty-gate": {"enabled": True, "profiles": []},
        "string-gate": {"enabled": True, "profiles": "lab"},
    }
    fh.write_marketplace(plugins=tuple(entries))
    mods(fh, entries)


@case("gate-hosts", steps=["check", "plan"], ports=["GatingTest.test_host_gate_is_case_insensitive"])
def _(fh):
    fh.write_profiles("lab")
    entries = {
        "this-host-upper": {"enabled": True, "hosts": ["{HOST_UPPER}"]},
        "other-host": {"enabled": True, "hosts": ["other-host"]},
        "both-gates-pass": {"enabled": True, "hosts": ["{HOST}"], "profiles": ["lab"]},
        "profile-gate-fails": {"enabled": True, "hosts": ["{HOST}"], "profiles": ["work"]},
        # The profile gate is not looked at when the host gate fails, so its
        # bad value never matters.
        "host-fails-first": {"enabled": True, "hosts": ["other-host"], "profiles": 5},
        "string-gate": {"enabled": True, "hosts": "{HOST}"},
        "object-gate": {"enabled": True, "hosts": {"{HOST}": 1}},
        "empty-gate": {"enabled": True, "hosts": []},
    }
    fh.write_marketplace(plugins=tuple(entries))
    mods(fh, entries)


@case(
    "gate-not-a-list",
    steps=["check"],
    deviation="A gate that is a nonzero number or true crashed the script with a TypeError; agent-sync reports it as an error. Both exit 1.",
)
def _(fh):
    fh.write_marketplace()
    mods(fh, {"session-band": {"enabled": True, "hosts": 1}})


@case("profiles-file-malformed", steps=["check"])
def _(fh):
    fh.write_marketplace(plugins=("lab-only",))
    fh.write_state({}, [])
    raw(fh, "config.local.toml", "profiles = [lab\n")
    mods(fh, {"lab-only": {"enabled": True, "profiles": ["lab"]}})


@case("profiles-not-a-list", steps=["check"])
def _(fh):
    fh.write_marketplace(plugins=("lab-only",))
    fh.write_state({}, [])
    raw(fh, "config.local.toml", 'profiles = "lab"\n')
    mods(fh, {"lab-only": {"enabled": True, "profiles": ["lab"]}})


@case("profiles-non-string", steps=["check"])
def _(fh):
    entries = {"by-number": {"enabled": True, "profiles": [1]}, "by-bool": {"enabled": True, "profiles": [True]}}
    fh.write_marketplace(plugins=tuple(entries))
    fh.write_state({}, [])
    raw(fh, "config.local.toml", "profiles = [1, true]\n")
    mods(fh, entries)


# --- the update rule ----------------------------------------------------------


@case(
    "update-skew",
    steps=["check", "plan", "sync", "state:after", "check"],
    states={"after": lambda fh: fh.write_state({SB: True}, [SB], versions={SB: "0.2.0"})},
    ports=[
        "UpdatePlanTest.test_version_skew_plans_an_update",
        "VersionUpdateTest.test_check_reports_version_skew_as_drift",
        "VersionUpdateTest.test_apply_runs_a_user_scope_update",
    ],
)
def _(fh):
    fh.write_marketplace(versions={"session-band": "0.2.0"})
    fh.write_state({SB: True}, [SB])


@case("update-folder-behind", steps=["check"])
def _(fh):
    # Differs, not newer: a folder behind the record is an update too.
    fh.write_marketplace(versions={"session-band": "0.0.9"})
    fh.write_state({SB: True}, [SB])


@case(
    "update-folder-version-missing",
    steps=["check", "sync"],
    ports=[
        "UpdatePlanTest.test_unknown_folder_version_plans_no_update",
        "VersionUpdateTest.test_missing_plugin_json_is_a_note_not_drift",
    ],
)
def _(fh):
    fh.write_marketplace(versions={"session-band": None})
    fh.write_state({SB: True}, [SB])


@case("update-plugin-json-no-version", steps=["check"], ports=["VersionUpdateTest.test_plugin_json_without_version_is_a_note_not_drift"])
def _(fh):
    fh.write_marketplace()
    raw(fh, "sources/thismoon/mods/session-band/.claude-plugin/plugin.json", json.dumps({"name": "session-band"}))
    fh.write_state({SB: True}, [SB])


@case("update-version-notes", steps=["check", "plan"])
def _(fh):
    # One enabled, installed mod per way a folder version can be unreadable,
    # plus an absolute source that reads fine.
    root = fh.marketplace_dir(MKT)
    sources = {
        "not-json": "./mods/not-json",
        "a-list": "./mods/a-list",
        "number-version": "./mods/number-version",
        "empty-version": "./mods/empty-version",
        "is-a-directory": "./mods/is-a-directory",
        "dotdot-missing": "./mods/../mods/dotdot-missing",
        "remote-source": {"source": "github", "repo": "example/remote"},
        "no-source": None,
        "empty-source": "",
        "absolute-source": str(fh.home / "elsewhere" / "absolute-source"),
    }
    plugins = [{"name": n} if s is None else {"name": n, "source": s} for n, s in sources.items()]
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", json.dumps({"name": MKT, "plugins": plugins}))
    plugin_json = ".claude-plugin/plugin.json"
    raw(fh, f"sources/thismoon/mods/not-json/{plugin_json}", "{not json")
    raw(fh, f"sources/thismoon/mods/a-list/{plugin_json}", "[]")
    raw(fh, f"sources/thismoon/mods/number-version/{plugin_json}", '{"version": 2}')
    raw(fh, f"sources/thismoon/mods/empty-version/{plugin_json}", '{"version": ""}')
    (root / "mods" / "is-a-directory" / ".claude-plugin" / "plugin.json").mkdir(parents=True)
    raw(fh, f"sources/thismoon/mods/is-a-directory/{plugin_json}/keep", "")
    raw(fh, f"elsewhere/absolute-source/{plugin_json}", '{"version": "0.3.0"}')
    mods(fh, {n: {"enabled": True} for n in sources})
    ids = [f"{n}@thismoon" for n in sources]
    fh.write_state(dict.fromkeys(ids, True), ids)


@case("update-record-without-version", steps=["check"], ports=["UpdatePlanTest.test_install_record_without_a_version_plans_no_update"])
def _(fh):
    fh.write_marketplace(versions={"session-band": "0.2.0", "panel-policy": "0.2.0"})
    mods(fh, {"session-band": {"enabled": True}, "panel-policy": {"enabled": True}})
    fh.write_state(BOTH_ON, [SB, PP], versions={SB: None, PP: ""})


@case(
    "update-skew-disabled-or-gated",
    steps=["check"],
    ports=[
        "UpdatePlanTest.test_disabled_and_gated_off_mods_with_skew_are_not_updated",
        "VersionUpdateTest.test_gated_off_mod_with_skew_is_left_alone",
    ],
)
def _(fh):
    fh.write_profiles("personal")
    mods(fh, {"session-band": {"enabled": True, "profiles": ["lab"]}, "panel-policy": {"enabled": False}})
    fh.write_marketplace(versions={"session-band": "0.2.0", "panel-policy": "0.2.0"})
    fh.write_state({SB: True}, [SB, PP])


@case("update-and-enable", ports=["UpdatePlanTest.test_installed_but_disabled_mod_with_skew_is_enabled_and_updated"])
def _(fh):
    fh.write_marketplace(versions={"session-band": "0.2.0"})
    fh.write_state({SB: False}, [SB])


@case("install-record-other-scope", steps=["check"])
def _(fh):
    # Only a user-scope record counts as installed.
    fh.write_marketplace()
    fh.write_state({SB: True}, [])
    records = {SB: ["junk", {"scope": "project", "version": "0.1.0"}]}
    raw(fh, ".claude/plugins/installed_plugins.json", json.dumps({"version": 2, "plugins": records}))


# --- the retire rule ----------------------------------------------------------


@case(
    "retire-installed",
    steps=["check", "plan", "sync", "state:after", "check"],
    states={"after": lambda fh: fh.write_state({SB: True}, [SB])},
    ports=[
        "UninstallPlanTest.test_retired_mod_with_an_install_record_is_uninstalled_not_disabled",
        "RetireTest.test_check_reports_a_pending_uninstall_as_drift",
        "RetireTest.test_dry_run_lists_the_uninstall_command",
        "RetireTest.test_apply_runs_a_user_scope_uninstall",
    ],
)
def _(fh):
    retire(fh)


@case(
    "retire-not-installed",
    steps=["check", "sync"],
    ports=[
        "UninstallPlanTest.test_retired_mod_without_an_install_record_plans_nothing",
        "RetireTest.test_retired_mod_with_no_install_record_is_in_sync",
    ],
)
def _(fh):
    # An enabledPlugins entry alone is not an install record.
    retire(fh, installed=(SB,))
    fh.write_state(BOTH_ON, [SB])


@case("retire-unlisted", ports=["UninstallPlanTest.test_retired_mod_the_marketplace_no_longer_lists_is_still_uninstalled"])
def _(fh):
    retire(fh)
    fh.write_marketplace(plugins=("session-band",))


@case("retire-with-skew", ports=["UninstallPlanTest.test_retired_mod_with_version_skew_is_not_updated"])
def _(fh):
    mods(fh, RETIRE_MODS)
    fh.write_marketplace(versions={"session-band": "0.2.0", "panel-policy": "0.2.0"})
    fh.write_state(BOTH_ON, [SB, PP])


@case("retire-failed-uninstall", steps=["sync"], env={"FAKE_CLAUDE_EXIT": "1"}, ports=["RetireTest.test_failed_uninstall_exits_2"])
def _(fh):
    retire(fh)


@case("retire-gated-off", steps=["check"], ports=["RetireTest.test_gated_off_retired_mod_is_left_alone"])
def _(fh):
    fh.write_profiles("personal")
    retire(fh, {"session-band": {"enabled": True}, "panel-policy": {"installed": False, "profiles": ["lab"]}})


@case("retire-enabled-true", steps=["check", "sync"], ports=["RetireTest.test_installed_false_beside_enabled_true_is_fatal"])
def _(fh):
    retire(fh, {"panel-policy": {"installed": False, "enabled": True}})


@case("retire-enabled-false", steps=["sync"], ports=["RetireTest.test_installed_false_beside_enabled_false_retires"])
def _(fh):
    retire(fh, {"session-band": {"enabled": True}, "panel-policy": {"installed": False, "enabled": False}})


@case("retire-conflict-gated-off", steps=["check"], ports=["RetireTest.test_conflict_is_fatal_even_when_gated_off"])
def _(fh):
    fh.write_profiles("personal")
    retire(fh, {"panel-policy": {"installed": False, "enabled": True, "profiles": ["lab"]}})


@case("retire-installed-not-bool", steps=["check"], ports=["RetireTest.test_non_boolean_installed_is_fatal"])
def _(fh):
    retire(fh, {"panel-policy": {"installed": "false"}})


@case("retire-installed-null", steps=["check"])
def _(fh):
    retire(fh, {"panel-policy": {"installed": None}})


@case("enabled-truthiness", steps=["check", "plan"])
def _(fh):
    entries = {
        "default-on": {},
        "string-yes": {"enabled": "yes"},
        "zero": {"enabled": 0},
        "null": {"enabled": None},
        "empty-list": {"enabled": []},
        "retired-zero": {"installed": False, "enabled": 0},
    }
    fh.write_marketplace(plugins=tuple(entries))
    mods(fh, entries)
    fh.write_state({"zero@thismoon": True}, ["retired-zero@thismoon"])


# --- apply and the claude CLI -------------------------------------------------


@case("apply-failed-call", steps=["sync"], env={"FAKE_CLAUDE_EXIT": "1"}, ports=["ApplyTest.test_failed_cli_call_exits_2_and_stops"])
def _(fh):
    fh.write_marketplace()


@case("claude-absent", no_claude=True)
def _(fh):
    fh.write_marketplace()


@case("claude-absent-in-sync", steps=["sync"], no_claude=True)
def _(fh):
    in_sync(fh)


# --- state file shapes --------------------------------------------------------


@case("state-settings-malformed", steps=["check"], ports=["StateShapeTest.test_malformed_settings_is_fatal"])
def _(fh):
    fh.write_marketplace()
    raw(fh, ".claude/settings.json", "{not json")


@case("state-installed-list", steps=["check"], ports=["StateShapeTest.test_installed_plugins_as_a_list_is_fatal"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/installed_plugins.json", "[]")


@case("state-enabled-plugins-list", steps=["check"], ports=["StateShapeTest.test_enabled_plugins_as_a_list_is_fatal"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/settings.json", json.dumps({"enabledPlugins": ["x"]}))


@case("state-marketplace-not-object", steps=["check"], ports=["StateShapeTest.test_non_object_marketplace_entry_is_fatal"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/known_marketplaces.json", json.dumps({"thismoon": "nope"}))


@case("state-marketplace-source-not-object", steps=["check"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/known_marketplaces.json", json.dumps({"thismoon": {"source": "nope"}}))


@case("state-records-not-list", steps=["check"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/installed_plugins.json", json.dumps({"plugins": {SB: {"scope": "user"}}}))


@case("state-plugins-not-object", steps=["check"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/installed_plugins.json", json.dumps({"plugins": [1]}))


@case("state-known-not-json", steps=["check"])
def _(fh):
    fh.write_marketplace()
    fh.write_state({}, [])
    raw(fh, ".claude/plugins/known_marketplaces.json", "")


@case("state-lenient-shapes", steps=["check"])
def _(fh):
    # Shapes the script reads past: a non-directory or empty source, a record
    # version that is not a string, non-bool enabledPlugins values.
    fh.write_marketplace()
    known_doc = {
        "thismoon": {"source": {"source": "github", "repo": "example/thismoon"}},
        "no-source": {},
        "null-source": {"source": None},
        "empty-path": {"source": {"source": "directory", "path": ""}},
    }
    raw(fh, ".claude/plugins/known_marketplaces.json", json.dumps(known_doc))
    records = {SB: [{"scope": "user", "version": 3}]}
    raw(fh, ".claude/plugins/installed_plugins.json", json.dumps({"version": 2, "plugins": records}))
    raw(fh, ".claude/settings.json", json.dumps({"enabledPlugins": {SB: 1, PP: True}}))


@case("state-falsy-containers", steps=["check"])
def _(fh):
    fh.write_marketplace()
    raw(fh, ".claude/plugins/known_marketplaces.json", "{}")
    raw(fh, ".claude/plugins/installed_plugins.json", json.dumps({"plugins": []}))
    raw(fh, ".claude/settings.json", json.dumps({"enabledPlugins": 0}))


# --- mods file shapes ---------------------------------------------------------


@case(
    "mods-file-missing",
    steps=["check"],
    deviation="A missing mods file is 'mods file not found: <path>'; the script said '<path>: top level must be an object'. Both exit 1.",
)
def _(fh):
    fh.mods_json.unlink()


@case("mods-file-malformed", steps=["check"])
def _(fh):
    raw(fh, "mods.json", "{")


@case("mods-top-not-object", steps=["check"])
def _(fh):
    raw(fh, "mods.json", "[]")


@case("mods-marketplace-not-object", steps=["check"])
def _(fh):
    raw(fh, "mods.json", json.dumps({"marketplace": "thismoon", "mods": {}}))


@case("mods-marketplace-incomplete", steps=["check"])
def _(fh):
    raw(fh, "mods.json", json.dumps({"marketplace": {"name": "thismoon", "path": ""}, "mods": {}}))


@case("mods-mods-missing", steps=["check"])
def _(fh):
    raw(fh, "mods.json", json.dumps({"marketplace": {"name": "thismoon", "path": "~/sources/thismoon"}}))


@case("mods-entry-not-object", steps=["check"])
def _(fh):
    raw(fh, "mods.json", json.dumps(fh.mods_file(MKT, {"session-band": True})))


@case("mods-entry-name-repr", steps=["check"])
def _(fh):
    # repr() picks double quotes for a name with a single quote and escapes
    # an unprintable character.
    raw(fh, "mods.json", json.dumps(fh.mods_file(MKT, {"it's" + chr(0x200B): "on"})))


@case("mods-comments-skipped", steps=["check"])
def _(fh):
    in_sync(fh)
    doc = fh.mods_file(MKT, {"_comment": "skipped", "_old": "not an object", "session-band": {"_comment": "x", "enabled": True}})
    doc["_comment"] = "top-level keys other than marketplace and mods are ignored"
    raw(fh, "mods.json", json.dumps(doc))


@case("mods-marketplace-name-number", steps=["check", "plan"])
def _(fh):
    # str() of the name: 7 becomes "7", which the manifest must match.
    fh.write_marketplace("7", ("session-band",))
    raw(fh, "mods.json", json.dumps({"marketplace": {"name": 7, "path": "~/sources/7"}, "mods": {"session-band": {"enabled": True}}}))


# --- marketplace manifest shapes ----------------------------------------------


@case("manifest-name-mismatch", steps=["check"])
def _(fh):
    fh.write_marketplace(manifest_name="thismoon-mods")


@case("manifest-name-missing", steps=["check"])
def _(fh):
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", json.dumps({"plugins": []}))


@case("manifest-plugins-not-array", steps=["check"])
def _(fh):
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", json.dumps({"name": MKT, "plugins": {}}))


@case("manifest-malformed", steps=["check"])
def _(fh):
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", "{")


@case("manifest-not-object", steps=["check"])
def _(fh):
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", "[]")


@case("manifest-entries-filtered", steps=["check"])
def _(fh):
    in_sync(fh)
    plugins = ["junk", {"source": "./mods/nameless"}, {"name": "", "source": "./mods/empty"}, {"name": "session-band", "source": "./mods/session-band"}]
    raw(fh, "sources/thismoon/.claude-plugin/marketplace.json", json.dumps({"name": MKT, "plugins": plugins}))


# --- drop-ins -----------------------------------------------------------------


@case("dropins-sorted", steps=["check"], ports=["DropInTest.test_dropins_load_in_sorted_order_after_base"])
def _(fh):
    fh.write_marketplace()
    fh.write_marketplace("alpha", ("a-mod",))
    fh.write_marketplace("beta", ("b-mod",))
    fh.write_dropin("b-beta.json", "beta", {"b-mod": {"enabled": True}})
    fh.write_dropin("a-alpha.json", "alpha", {"a-mod": {"enabled": True}})


@case(
    "dropin-marketplace-missing",
    steps=["check"],
    ports=["DropInTest.test_dropin_with_missing_marketplace_is_skipped_and_base_stays_clean"],
)
def _(fh):
    in_sync(fh)
    fh.write_dropin("lab.json", "lab", {"lab-pane": {"enabled": True}})


@case("dropin-dangling", steps=["check"], ports=["DropInTest.test_dangling_dropin_symlink_is_skipped_with_a_note"])
def _(fh):
    in_sync(fh)
    fh.overlay_dir.mkdir()
    (fh.overlay_dir / "gone.json").symlink_to(fh.home / "nowhere" / "gone.json")
    # A directory that matches the glob is skipped the same way, and a
    # hidden file still loads.
    raw(fh, "mods.d/a-dir.json/keep", "")
    fh.write_dropin(".hidden.json", "hidden", {"hidden-pane": {"enabled": True}})
    raw(fh, "mods.d/not-a-dropin.txt", "ignored")


@case(
    "dropin-gated-off",
    steps=["check", "state:registered", "check"],
    states={"registered": lambda fh: fh.write_state({SB: True}, [SB], (MKT, "lab"))},
    ports=["DropInTest.test_dropin_profile_gate_off_registers_marketplace_but_leaves_mod_alone"],
)
def _(fh):
    in_sync(fh)
    fh.write_profiles("personal")
    fh.write_marketplace("lab", ("lab-pane",))
    fh.write_dropin("lab.json", "lab", LAB)


@case("dropin-gated-on", ports=["DropInTest.test_dropin_profile_gate_on_installs_its_mod_against_its_marketplace"])
def _(fh):
    in_sync(fh)
    fh.write_profiles("lab")
    fh.write_marketplace("lab", ("lab-pane",))
    fh.write_dropin("lab.json", "lab", LAB)


@case("dropin-summary", steps=["check"], ports=["DropInTest.test_summary_covers_base_and_dropins"])
def _(fh):
    fh.write_profiles("lab")
    fh.write_marketplace()
    fh.write_marketplace("lab", ("lab-pane",))
    fh.write_dropin("lab.json", "lab", LAB)
    fh.write_state({SB: True, "lab-pane@lab": True}, [SB, "lab-pane@lab"], (MKT, "lab"))


@case("dropin-manifest-name-mismatch", steps=["check"], ports=["DropInTest.test_manifest_name_mismatch_is_rejected"])
def _(fh):
    in_sync(fh)
    fh.write_marketplace("lab", ("lab-pane",), manifest_name="lab-mods")
    fh.write_dropin("lab.json", "lab", {"lab-pane": {"enabled": True}})


@case("dropin-duplicate-mod", steps=["check"], ports=["DropInTest.test_duplicate_mod_name_across_files_is_rejected"])
def _(fh):
    in_sync(fh)
    fh.write_marketplace("lab", ("session-band",))
    fh.write_dropin("lab.json", "lab", {"session-band": {"enabled": True}})


@case("dropin-duplicate-marketplace", steps=["check"], ports=["DropInTest.test_duplicate_marketplace_name_is_rejected"])
def _(fh):
    in_sync(fh)
    fh.write_dropin("again.json", MKT, {"extra": {"enabled": True}})


@case("dropin-retired-duplicate-base", steps=["check"], ports=["DropInTest.test_retired_mod_name_repeated_across_files_is_rejected"])
def _(fh):
    in_sync(fh)
    mods(fh, {"panel-policy": {"installed": False}})
    fh.write_marketplace("lab", ("panel-policy",))
    fh.write_dropin("lab.json", "lab", {"panel-policy": {"enabled": True}})


@case("dropin-retired-duplicate-dropin", steps=["check"], ports=["DropInTest.test_retired_mod_name_repeated_across_files_is_rejected"])
def _(fh):
    in_sync(fh)
    mods(fh, {"panel-policy": {"enabled": True}})
    fh.write_marketplace("lab", ("panel-policy",))
    fh.write_dropin("lab.json", "lab", {"panel-policy": {"installed": False}})


@case(
    "dropin-update",
    steps=["check", "sync"],
    ports=["VersionUpdateTest.test_dropin_mod_is_updated_against_its_own_marketplace"],
)
def _(fh):
    fh.write_profiles("lab")
    fh.write_marketplace()
    fh.write_marketplace("lab", ("lab-pane",), versions={"lab-pane": "1.1.0"})
    fh.write_dropin("lab.json", "lab", LAB)
    fh.write_state(
        {SB: True, "lab-pane@lab": True},
        [SB, "lab-pane@lab"],
        (MKT, "lab"),
        versions={"lab-pane@lab": "1.0.0"},
    )


@case(
    "dropin-retire-skips-folder-version",
    steps=["check"],
    ports=["DropInTest.test_dropin_retire_plans_the_uninstall_without_reading_its_folder_version"],
)
def _(fh):
    fh.write_marketplace()
    fh.write_marketplace("lab", ("lab-pane",), versions={"lab-pane": None})
    fh.write_dropin("lab.json", "lab", {"lab-pane": {"installed": False}})
    installed = [SB, "lab-pane@lab"]
    fh.write_state(dict.fromkeys(installed, True), installed, (MKT, "lab"))


@case("dropin-unrelated-state", steps=["check"], ports=["DropInTest.test_dropin_never_prunes_unrelated_state"])
def _(fh):
    fh.write_profiles("lab")
    fh.write_marketplace()
    fh.write_marketplace("lab", ("lab-pane",))
    fh.write_dropin("lab.json", "lab", LAB)
    ids = [SB, "lab-pane@lab", "stray@lab"]
    fh.write_state(dict.fromkeys(ids, True), ids, (MKT, "lab", "untracked"))


@case("dropin-malformed", steps=["check"])
def _(fh):
    in_sync(fh)
    raw(fh, "mods.d/lab.json", "{")


# --- build and run -------------------------------------------------------------


def load_reference(ref: Path):
    spec = importlib.util.spec_from_file_location("test_sync_mods", ref / "test_sync_mods.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def snapshot(home: Path) -> tuple[dict[str, bytes], dict[str, str]]:
    """Files and symlinks under home, minus the fake claude and its log."""
    files: dict[str, bytes] = {}
    links: dict[str, str] = {}
    marker = str(home).encode()
    for dirpath, dirnames, filenames in os.walk(home):
        for name in dirnames + filenames:
            path = Path(dirpath) / name
            rel = str(path.relative_to(home))
            if rel in ("bin", "claude.log") or rel.startswith("bin/"):
                continue
            if path.is_symlink():
                links[rel] = os.readlink(path).replace(str(home), "{HOME}")
            elif path.is_file():
                files[rel] = path.read_bytes().replace(marker, b"{HOME}")
        dirnames[:] = [d for d in dirnames if not (Path(dirpath) / d).is_symlink()]
    return files, links


def save(files: dict[str, bytes], root: Path) -> None:
    for rel, data in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)


def materialize(src: Path, home: Path, host: str) -> None:
    """Copy a saved tree into home with {HOME} and the host placeholders filled in."""
    if not src.is_dir():
        return
    for path in sorted(src.rglob("*")):
        if path.is_dir():
            continue
        dst = home / path.relative_to(src)
        dst.parent.mkdir(parents=True, exist_ok=True)
        data = path.read_bytes()
        data = data.replace(b"{HOME}", str(home).encode())
        data = data.replace(b"{HOST_UPPER}", host.upper().encode()).replace(b"{HOST}", host.encode())
        dst.write_bytes(data)


def clean(case_dir: Path) -> None:
    """Empty a case directory, keeping hand-written want/*.deviation files."""
    if not case_dir.exists():
        return
    for child in case_dir.iterdir():
        if child.name == "want":
            for f in child.iterdir():
                if not f.name.endswith(".deviation"):
                    f.unlink()
        elif child.is_dir():
            shutil.rmtree(child)
        else:
            child.unlink()


def build(name: str, spec: dict, ref) -> dict:
    case_dir = CASES_DIR / name
    clean(case_dir)
    case_dir.mkdir(exist_ok=True)
    with ref.FakeHome() as fh:
        spec["setup"](fh)
        files, links = snapshot(fh.home)
        save(files, case_dir / "home")
        for label, change in spec["states"].items():
            change(fh)
            after, _ = snapshot(fh.home)
            save({rel: data for rel, data in after.items() if files.get(rel) != data}, case_dir / label)
    doc = {"ports": spec["ports"], "steps": spec["steps"]}
    for key in ("env", "no_claude", "deviation"):
        if spec[key]:
            doc[key] = spec[key]
    if links:
        doc["symlinks"] = links
    (case_dir / "case.json").write_text(json.dumps(doc, indent=2) + "\n")
    return doc


def run(name: str, doc: dict, ref, host: str) -> None:
    case_dir = CASES_DIR / name
    want = case_dir / "want"
    want.mkdir(exist_ok=True)
    sync = ref.SYNC
    with tempfile.TemporaryDirectory() as tmp:
        home = Path(tmp)
        materialize(case_dir / "home", home, host)
        for rel, target in doc.get("symlinks", {}).items():
            link = home / rel
            link.parent.mkdir(parents=True, exist_ok=True)
            link.symlink_to(target.replace("{HOME}", str(home)))
        bin_dir = home / "bin"
        bin_dir.mkdir()
        if not doc.get("no_claude"):
            shutil.copy(FAKE_CLAUDE, bin_dir / "claude")
            (bin_dir / "claude").chmod(stat.S_IRWXU)
        log = home / "claude.log"
        env = {
            "HOME": str(home),
            "PATH": f"{bin_dir}:/usr/bin:/bin",
            "FAKE_CLAUDE_LOG": str(log),
            "FAKE_CLAUDE_EXIT": "0",
        }
        env.update({k: v.replace("{HOME}", str(home)) for k, v in doc.get("env", {}).items()})
        patches = [
            mock.patch.dict(os.environ, env),
            mock.patch.object(sync, "MODS_PATH", home / "mods.json"),
            mock.patch.object(sync, "OVERLAY_DIR", home / "mods.d"),
            mock.patch.object(sync, "LOCAL_CONFIG_PATH", home / "config.local.toml"),
        ]
        for p in patches:
            p.start()
        try:
            if "CLAUDE_CONFIG_DIR" not in doc.get("env", {}):
                os.environ.pop("CLAUDE_CONFIG_DIR", None)
            for i, step in enumerate(doc["steps"], 1):
                if step.startswith("state:"):
                    materialize(case_dir / step.removeprefix("state:"), home, host)
                    continue
                before = snapshot(home)
                code, out, err = run_main(ref, ARGS[step])
                calls = log.read_text() if log.exists() else ""
                log.unlink(missing_ok=True)
                if snapshot(home) != before:
                    sys.exit(f"{name} step {i}: the reference wrote a file")
                outputs = {
                    "stdout": out,
                    "stderr": err,
                    "result": f"exit: {code}\n",
                    "calls": calls,
                }
                for field, text in outputs.items():
                    text = text.replace(str(home), "{HOME}")
                    if text or field in ("stdout", "result"):
                        (want / f"{i}-{step}.{field}").write_text(text)
        finally:
            for p in reversed(patches):
                p.stop()


def run_main(ref, argv) -> tuple[int, str, str]:
    """The reference tests' run_main, plus what a crash looks like from outside:
    exit 1 and, in place of the traceback, its last line."""
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        try:
            code = ref.SYNC.main(list(argv))
        except SystemExit as exc:
            code = exc.code
        except Exception as exc:  # noqa: BLE001 - a crash is an outcome to record
            return 1, out.getvalue(), f"traceback: {type(exc).__name__}: {exc}\n"
    return code, out.getvalue(), err.getvalue()


def main() -> int:
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    ref = load_reference(Path(sys.argv[1]).resolve())
    global RETIRE_MODS
    RETIRE_MODS = ref.RETIRE_MODS
    # macOS hands out /var/... temp dirs that resolve to /private/var/...;
    # start from the resolved path so {HOME} matches what the script prints.
    tempfile.tempdir = os.path.realpath(tempfile.gettempdir())
    host = socket.gethostname().lower()
    CASES_DIR.mkdir(exist_ok=True)
    for stale in CASES_DIR.iterdir():
        if stale.name not in CASES:
            shutil.rmtree(stale)
    for name, spec in CASES.items():
        doc = build(name, spec, ref)
        run(name, doc, ref, host)
    print(f"wrote {len(CASES)} cases to {CASES_DIR}")
    return 0


RETIRE_MODS: dict = {}

if __name__ == "__main__":
    sys.exit(main())
