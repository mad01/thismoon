// Package prereq checks what a run needs before it spends anything: the
// tools on PATH, a claude new enough for the unattended flags, and for the
// factory arm the factory and loom skills, the three worker definitions,
// and the belt hook that hands each worker its brief.
package prereq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MinClaude is the oldest claude that takes --permission-prompts none.
var MinClaude = [3]int{2, 1, 259}

// WorkerDefinitions are the three agent definitions the factory skill
// spawns workers from, one per effort level.
var WorkerDefinitions = []string{
	"factory-worker-low",
	"factory-worker-medium",
	"factory-worker-high",
}

// Result is one prerequisite's outcome.
type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// ClaudeProbe is what the one `claude --version` call of a batch returned.
type ClaudeProbe struct {
	Version [3]int
	Raw     string
	Err     error
}

// Env is the host the checks look at. Tests point Home and ConfigDir at
// temp dirs. ConfigDir is Claude Code's config dir, resolved once by the
// caller (see ConfigDir). GoCache is `go env GOCACHE`, and GoCacheErr the
// error when go did not answer. SandboxExclusions is the user's
// sandbox.excludedCommands list, which every arm session inherits.
type Env struct {
	Home              string
	ConfigDir         string
	GoCache           string
	GoCacheErr        error
	SandboxExclusions []string
	LookPath          func(file string) (string, error)
	Claude            ClaudeProbe
	BeltVersion       func(ctx context.Context) (string, error)
}

// Common checks what every arm needs.
func Common(env Env) []Result {
	results := []Result{claudeVersion(env.Claude)}
	for _, bin := range []string{"git", "make", "go", "humanizer"} {
		results = append(results, onPath(env, bin))
	}
	return append(results, goBuildCache(env))
}

// Factory checks what the factory arm needs on top of Common.
func Factory(env Env) []Result {
	claudeDir := env.ConfigDir
	results := []Result{
		file("factory skill", filepath.Join(claudeDir, "skills", "factory", "SKILL.md")),
		file("loom skill", filepath.Join(claudeDir, "skills", "loom", "SKILL.md")),
	}
	for _, w := range WorkerDefinitions {
		results = append(results, file("worker definition "+w,
			filepath.Join(claudeDir, "agents", w+".md")))
	}
	return append(results, beltSubagentHook(filepath.Join(claudeDir, "settings.json")))
}

// Failed returns the results that did not pass.
func Failed(results []Result) []Result {
	var failed []Result
	for _, r := range results {
		if !r.OK {
			failed = append(failed, r)
		}
	}
	return failed
}

// Explain renders failed results as one refusal message.
func Explain(arm string, failed []Result) string {
	lines := []string{
		fmt.Sprintf("the %s arm cannot run, %d prerequisites missing:", arm, len(failed)),
	}
	for _, r := range failed {
		lines = append(lines, fmt.Sprintf("  - %s: %s", r.Name, r.Detail))
	}
	if arm == "factory" {
		lines = append(lines, "MAD-398 installs the worker definitions and the belt hook wiring; "+
			"see the factory-eval README, prerequisites.")
	}
	return strings.Join(lines, "\n")
}

func claudeVersion(p ClaudeProbe) Result {
	r := Result{Name: "claude version"}
	switch {
	case p.Err != nil:
		r.Detail = p.Err.Error()
	case older(p.Version, MinClaude):
		r.Detail = fmt.Sprintf("%s is older than %d.%d.%d, which --permission-prompts needs",
			p.Raw, MinClaude[0], MinClaude[1], MinClaude[2])
	default:
		r.OK, r.Detail = true, p.Raw
	}
	return r
}

func older(v, min [3]int) bool {
	for i := range v {
		if v[i] != min[i] {
			return v[i] < min[i]
		}
	}
	return false
}

func onPath(env Env, bin string) Result {
	p, err := env.LookPath(bin)
	if err != nil {
		return Result{Name: bin, Detail: "not on PATH"}
	}
	return Result{Name: bin, OK: true, Detail: p}
}

// goBuildCache checks that go named an absolute build cache, the one path
// outside the run's temp dir the sandbox lets a run write. A go on PATH
// whose `go env` fails would pass onPath alone, and `GOCACHE=off` answers
// "off", which the sandbox would take as a relative path.
func goBuildCache(env Env) Result {
	r := Result{Name: "go build cache"}
	switch {
	case env.GoCacheErr != nil:
		r.Detail = "go env GOCACHE: " + env.GoCacheErr.Error()
	case !filepath.IsAbs(env.GoCache):
		r.Detail = fmt.Sprintf("go env GOCACHE gave %q, want an absolute path", env.GoCache)
	default:
		r.OK, r.Detail = true, env.GoCache
	}
	return r
}

func file(name, p string) Result {
	if _, err := os.Stat(p); err != nil {
		return Result{Name: name, Detail: p + " not found"}
	}
	return Result{Name: name, OK: true, Detail: p}
}

// settings is the part of the user's settings file the checks read.
type settings struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
	Sandbox struct {
		ExcludedCommands []string `json:"excludedCommands"`
	} `json:"sandbox"`
}

func readSettings(p string) (settings, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return settings{}, err
	}
	var s settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return settings{}, fmt.Errorf("%s: %w", p, err)
	}
	return s, nil
}

// managedDir is where macOS keeps the managed settings: the
// managed-settings.json file and the managed-settings.d drop-in dir beside
// it, which a session loads whatever its setting sources are. Tests point
// it elsewhere.
var managedDir = "/Library/Application Support/ClaudeCode"

// ConfigDir is Claude Code's config dir: override, the value of
// $CLAUDE_CONFIG_DIR, when set, else ~/.claude under home.
func ConfigDir(home, override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(home, ".claude")
}

// SandboxExclusions is the sandbox.excludedCommands list an arm session
// inherits: the user's settings file in configDir, then the managed
// settings file, then every .json file in managed-settings.d in name
// order, the way Claude Code merges them. Exclusion lists combine across
// the settings files a session loads, so the run's own file cannot remove
// an entry. A missing file adds no exclusions. An MDM profile and
// server-managed settings are not read.
func SandboxExclusions(configDir string) ([]string, error) {
	dropIns, err := jsonFiles(filepath.Join(managedDir, "managed-settings.d"))
	if err != nil {
		return nil, fmt.Errorf("sandbox exclusions: %w", err)
	}
	paths := append([]string{
		filepath.Join(configDir, "settings.json"),
		filepath.Join(managedDir, "managed-settings.json"),
	}, dropIns...)
	var all []string
	for _, p := range paths {
		s, err := readSettings(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("sandbox exclusions: %w", err)
		}
		all = append(all, s.Sandbox.ExcludedCommands...)
	}
	return all, nil
}

// jsonFiles is every .json file in dir, in name order, hidden files
// skipped the way Claude Code skips them. A missing dir holds none.
func jsonFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

// beltSubagentHook checks that a SubagentStart hook runs belt, the hint
// that delivers the worker brief before a worker's first prompt.
func beltSubagentHook(p string) Result {
	r := Result{Name: "belt SubagentStart hook"}
	s, err := readSettings(p)
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	for _, group := range s.Hooks["SubagentStart"] {
		for _, h := range group.Hooks {
			if strings.Contains(h.Command, "belt") {
				r.OK, r.Detail = true, h.Command
				return r
			}
		}
	}
	r.Detail = "no SubagentStart hook in " + p + " runs belt"
	return r
}
