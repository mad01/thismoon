// Package launch is what srtspec run does: start a fleet process inside srt
// (anthropics/sandbox-runtime) the way the srt-run shell launcher did. Plan
// resolves the settings file and the label, merges the machine's expansion
// layer and SRT_RUN_ALLOW_DOMAINS through the layers rules into a copy srt
// reads, picks the runtime (bun or node), and returns the exec the caller
// performs: srt with the command tagged `env SRT_RUN=<label> <argv>`, the
// marker sandbox-watch attributes denials by. It fails closed: a layer that
// does more than widen, a missing runtime or a bad label stops the launch
// before srt starts.
package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/access"
	"github.com/mad01/thismoon/tools/srtspec/internal/layers"
)

// ExitError is a launch failure with the exit status the srt-run contract
// gives it: 2 for usage, 127 for a missing runtime, 1 for a refused launch.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

func usage(format string, args ...any) error {
	return &ExitError{Code: 2, Msg: fmt.Sprintf(format, args...)}
}

func missing(format string, args ...any) error {
	return &ExitError{Code: 127, Msg: fmt.Sprintf(format, args...)}
}

func refuse(format string, args ...any) error {
	return &ExitError{Code: 1, Msg: fmt.Sprintf(format, args...)}
}

// Input is one launch.
type Input struct {
	Settings string            // the settings file; a leading ~ expands, a relative path is under Dir
	Argv     []string          // the command to confine
	Home     string            // the user's home directory
	Dir      string            // the directory a relative Settings resolves against; "" means the cwd
	Env      map[string]string // the launcher's environment, read for SRT_RUN_LABEL, SRT_RUN_RUNTIME, SRT_RUN_ALLOW_DOMAINS, SRT_DEBUG, TMPDIR and PATH
	Log      io.Writer         // where SRT_DEBUG lines go; nil discards them
}

// Command is the exec a planned launch ends in.
type Command struct {
	Label   string
	Runtime string   // bun or node
	Path    string   // the program to exec
	Argv    []string // Argv[0] included
	Env     []string // KEY=value, sorted
}

const (
	// shimsDir holds mise's shims, relative to home; srt is one of them.
	shimsDir = ".local/share/mise/shims"
	// systemPath goes ahead of the caller's PATH so srt starts the command
	// through /bin/bash, whatever an env -i wrapper left on PATH.
	systemPath = "/usr/bin:/bin:/usr/sbin:/sbin"
	// defaultTmp is the child's TMPDIR when the caller has none.
	defaultTmp = "/private/tmp"
)

// Plan resolves one launch without starting anything. The returned Command
// is what to exec; an *ExitError says why not and with which status.
func Plan(in Input) (Command, error) {
	if in.Settings == "" || len(in.Argv) == 0 {
		return Command{}, usage("usage: srtspec run <settings-file> <argv...>")
	}
	settings, err := absSettings(in.Settings, in.Home, in.Dir)
	if err != nil {
		return Command{}, err
	}
	label, err := label(in.Env["SRT_RUN_LABEL"], settings)
	if err != nil {
		return Command{}, err
	}
	env := maps.Clone(in.Env)
	if env == nil {
		env = map[string]string{}
	}
	env["PATH"] = searchPath(in.Home, env["PATH"])
	srt, ok := lookPath("srt", env["PATH"])
	if !ok {
		return Command{}, missing(
			"srt not found on PATH (expected the mise shim at %s; install npm:@anthropic-ai/sandbox-runtime with mise)",
			filepath.Join(in.Home, shimsDir, "srt"),
		)
	}
	rt, err := pickRuntime(in.Home, env, srt, label)
	if err != nil {
		return Command{}, err
	}
	settings, err = mergeLayers(in, settings, label, env)
	if err != nil {
		return Command{}, err
	}
	delete(env, "SRT_RUN_ALLOW_DOMAINS")
	tmp := env["TMPDIR"]
	if tmp == "" {
		tmp = defaultTmp
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return Command{}, refuse("cannot create %s for the child's TMPDIR: %v", tmp, err)
	}
	env["CLAUDE_CODE_TMPDIR"] = tmp

	c := Command{Label: label, Runtime: rt.name, Env: environ(env)}
	tail := append([]string{"--settings", settings, "--", "env", "SRT_RUN=" + label}, in.Argv...)
	if rt.name == "bun" {
		c.Path = rt.bun
		c.Argv = append(
			[]string{rt.bun, "--no-env-file", "--config=/dev/null", "--no-install", rt.cli},
			tail...,
		)
		return c, nil
	}
	c.Path = srt
	c.Argv = append([]string{srt}, tail...)
	return c, nil
}

// absSettings expands a leading ~ and makes the path absolute; srt does
// neither.
func absSettings(p, home, dir string) (string, error) {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", refuse("cannot resolve %s: %v", p, err)
		}
		dir = cwd
	}
	return filepath.Join(dir, p), nil
}

// label is SRT_RUN_LABEL, else the settings file's name without .json.
func label(override, settings string) (string, error) {
	l := override
	if l == "" {
		l = strings.TrimSuffix(filepath.Base(settings), ".json")
	}
	if !access.ValidLabel(l) {
		return "", usage("label %q must match [A-Za-z0-9._-]+ (set SRT_RUN_LABEL)", l)
	}
	return l, nil
}

// searchPath puts the mise shims first, then the system dirs, then whatever
// the caller had: a launchd agent or an env -i wrapper carries no login PATH.
func searchPath(home, caller string) string {
	p := filepath.Join(home, shimsDir) + ":" + systemPath
	if caller != "" {
		p += ":" + caller
	}
	return p
}

// lookPath finds an executable regular file named name in path's dirs.
func lookPath(name, path string) (string, bool) {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if executable(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// mergeLayers returns the settings file srt should read: the file itself
// when the machine adds nothing, else a merged copy under
// ~/.config/srt/merged/<label>.json (mode 600, written beside and renamed,
// so two launches of one label never read a half-written file).
func mergeLayers(in Input, settings, label string, env map[string]string) (string, error) {
	layerPath := filepath.Join(in.Home, ".config", "srt", label+".local.json")
	_, statErr := os.Stat(layerPath)
	hasLayer := statErr == nil
	hostsVar := env["SRT_RUN_ALLOW_DOMAINS"]
	if !hasLayer && hostsVar == "" {
		return settings, nil
	}
	hosts, err := layers.Hosts(hostsVar)
	if err != nil {
		return "", refuse("%v", err)
	}
	base, err := layers.ReadObject(settings)
	if err != nil {
		return "", refuse("cannot read %s; refusing to launch %s: %v", settings, label, err)
	}
	local := map[string]any{}
	if hasLayer {
		local, err = layers.ReadObject(layerPath)
		if err != nil {
			return "", refuse("%s is not valid JSON; refusing to launch %s", layerPath, label)
		}
		if problems := layers.LocalProblems(base, local, in.Home); len(problems) > 0 {
			return "", refuse(
				"%s may only widen the policy; refusing to launch %s:\n  - %s",
				layerPath,
				label,
				strings.Join(problems, "\n  - "),
			)
		}
	}
	merged := layers.Merge(base, local, hosts)
	data, err := access.Encode(merged)
	if err != nil {
		return "", refuse(
			"merging the layers into %s failed; refusing to launch %s: %v",
			settings,
			label,
			err,
		)
	}
	dir := filepath.Join(in.Home, ".config", "srt", "merged")
	out := filepath.Join(dir, label+".json")
	if err := writeMerged(dir, out, label, data); err != nil {
		return "", refuse(
			"merging the layers into %s failed; refusing to launch %s: %v",
			settings,
			label,
			err,
		)
	}
	if env["SRT_DEBUG"] != "" && in.Log != nil {
		doms, _ := json.Marshal(merged["network"].(map[string]any)["allowedDomains"])
		fmt.Fprintf(in.Log, "srt-run: %s allowedDomains: %s\n", label, doms)
		if hasLayer {
			fmt.Fprintf(in.Log, "srt-run: %s merged %s\n", label, layerPath)
		}
	}
	return out, nil
}

func writeMerged(dir, out, label string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+label+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := fill(f, data); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	if err := os.Rename(tmp, out); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}

// fill writes data to f, sets mode 600 and closes it.
func fill(f *os.File, data []byte) error {
	_, werr := f.Write(data)
	merr := f.Chmod(0o600)
	return errors.Join(werr, merr, f.Close())
}

// environ renders env as sorted KEY=value pairs.
func environ(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}
