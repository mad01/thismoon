// Package mods reconciles Claude Code mods (plugins from directory
// marketplaces) with the mods files the mods section names, through the
// claude plugin CLI. It is the Go port of the dotfiles repo's sync_mods.py
// and keeps that script's stdout, exit codes and claude calls.
//
// The base mods file comes first, then every *.json drop-in in sorted name
// order. Each file names one directory marketplace and the mods to enable,
// disable or retire from it at user scope. The sync diffs that against the
// plugin state Claude Code keeps (see state) and runs `claude plugin ...`
// to close the gap; it never edits those files itself. Per file, touching
// nothing the files do not name:
//
//	marketplace missing, or recorded at another path    marketplace add <path>
//	enabled mod with no user-scope install record       install
//	enabled mod installed but not enabled               enable
//	enabled mod whose record version differs from the
//	  plugin.json version in its folder                 update
//	disabled mod currently enabled                      disable
//	retired mod (installed: false) with a user-scope
//	  install record, listed by the marketplace or not  uninstall
//
// A mod a gate keeps off this machine is left alone. A mod the marketplace
// does not list yet, a marketplace that is not on disk and a dangling
// drop-in are notes, never errors, so a first run on a new machine passes.
//
// config.go reads the section, desired.go the mods files and marketplace
// manifests, gate.go a mod's hosts and profiles gates, state.go Claude
// Code's plugin state, plan.go the pure plan, and this file prints the
// report and runs the claude CLI.
package mods

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/claudecode"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// Report lines that are fixed text.
const (
	noMarketplaceLine = "claude-mods: no marketplace on disk yet; nothing to do"
	planFooter        = "(dry-run, no changes applied)"
)

// Run reconciles the mods as mode says, printing the report to out.Out.
// Check prints a one-line summary first and returns
// apply.Exit(apply.CodeDrift) on drift; a failed claude call is
// apply.CodeExternal.
func Run(cfg Config, mode apply.Mode, out apply.Streams) error {
	return runner{cfg: cfg, mode: mode, out: out.Out}.run()
}

// runner is one Run: the config, the mode, and where the report goes.
type runner struct {
	cfg  Config
	mode apply.Mode
	out  io.Writer
}

func (r runner) run() error {
	sources, notes, err := loadSources(r.cfg)
	if err != nil {
		return err
	}
	dir, err := claudeDir()
	if err != nil {
		return err
	}
	st, err := readState(dir)
	if err != nil {
		return err
	}
	present, actions, planNotes, err := planAll(sources, st)
	if err != nil {
		return err
	}
	notes = append(notes, planNotes...)
	if len(present) == 0 {
		fmt.Fprintln(r.out, noMarketplaceLine)
		r.printLines(notes)
		return nil
	}
	// The first line is the summary ralph doctor shows; notes follow it.
	switch {
	case len(actions) == 0:
		fmt.Fprintln(r.out, inSyncSummary(present))
	case r.mode == apply.Check:
		fmt.Fprintf(r.out, "drift: %s pending (run 'ralph up' to apply)\n",
			plural(len(actions), "change"))
		for _, a := range actions {
			fmt.Fprintf(r.out, "  %s\n", a)
		}
	}
	r.printLines(notes)
	if len(actions) == 0 {
		return nil
	}
	if r.mode == apply.Check {
		return apply.Exit(apply.CodeDrift)
	}
	return r.execute(actions)
}

// execute prints each action with its claude command and, in Sync, runs it.
// The first failed call stops the run.
func (r runner) execute(actions []action) error {
	claude, err := r.claudePath()
	if err != nil {
		return err
	}
	for _, a := range actions {
		argv := append([]string{claude, "plugin"}, a.args()...)
		fmt.Fprintln(r.out, a)
		fmt.Fprintf(r.out, "  $ %s\n", strings.Join(argv, " "))
		if !r.mode.Writes() {
			continue
		}
		if err := runClaude(argv); err != nil {
			return err
		}
	}
	if !r.mode.Writes() {
		fmt.Fprintln(r.out, planFooter)
		return nil
	}
	restart := ""
	for _, a := range actions {
		if a.verb == verbUpdate {
			restart = "; updated mods load after a restart"
			break
		}
	}
	fmt.Fprintf(r.out, "applied %s; new sessions or /reload-plugins pick them up%s\n",
		plural(len(actions), "change"), restart)
	return nil
}

// claudePath finds the claude CLI on PATH. Plan prints the commands without
// running them, so it falls back to the bare name.
func (r runner) claudePath() (string, error) {
	path, err := claudecode.LookPath()
	if err != nil && !r.mode.Writes() {
		return claudecode.Bin, nil
	}
	return path, err
}

// runClaude runs one claude command with the caller's stdin, so a prompt
// still reaches a person, and its output captured. A non-zero exit is
// apply.CodeExternal with the command's stderr (or stdout) as the detail,
// decoded and stripped as the script's text=True and .strip() did.
func runClaude(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		detail := pyjson.Text(stderr.Bytes())
		if detail == "" {
			detail = pyjson.Text(stdout.Bytes())
		}
		return apply.Fail(apply.CodeExternal, fmt.Errorf("%s failed: %s",
			strings.Join(argv, " "), pyjson.Strip(detail)))
	case err != nil:
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

func (r runner) printLines(lines []string) {
	for _, line := range lines {
		fmt.Fprintln(r.out, line)
	}
}

// inSyncSummary is the one-line summary of a clean run: mod counts by
// desired state, then the marketplaces on disk.
func inSyncSummary(present []modsFile) string {
	counts := map[want]int{}
	registry := make([]string, len(present))
	for i, f := range present {
		for _, m := range f.mods {
			counts[m.want]++
		}
		registry[i] = f.marketplace + " -> " + f.path
	}
	noun := "marketplaces"
	if len(present) == 1 {
		noun = "marketplace"
	}
	return fmt.Sprintf(
		"mods already in sync (%d enabled, %d disabled, %d retired, %d gated off; %s %s)",
		counts[wantEnabled], counts[wantDisabled], counts[wantRetired], counts[gatedOff],
		noun, strings.Join(registry, ", "),
	)
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
