package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// brainPackage is the brain CLI's main package, relative to the repo root
// the harness runs from.
const brainPackage = "./services/brain/cmd/brain"

// The variables a brain workspace points brain at its scratch directory
// with: the config file, and the state directory brain keeps its indexes in.
const (
	brainConfigEnv  = "BRAIN_CONFIG"
	xdgStateHomeEnv = "XDG_STATE_HOME"
)

// vaultGitEnv keeps the caller's git config out of the vault's scratch
// repo, so global hooks, signing and templates do not run in it.
var vaultGitEnv = []string{"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}

// brainHome is a brain workspace's scratch directory:
//
//	<Root>/vault        the fixture copy, committed as the git baseline
//	<Root>/config.yaml  brain's config: the vault, semantic search off
//	<Root>/state        XDG_STATE_HOME, where brain keeps its indexes
//	<Root>/work         claude's working directory, empty
//
// BinDir holds the brain binary built for the run.
type brainHome struct {
	Root   string
	BinDir string
}

func (h brainHome) vaultDir() string   { return filepath.Join(h.Root, "vault") }
func (h brainHome) configPath() string { return filepath.Join(h.Root, "config.yaml") }
func (h brainHome) stateDir() string   { return filepath.Join(h.Root, "state") }
func (h brainHome) workDir() string    { return filepath.Join(h.Root, "work") }

// Change is one vault path that differs from the baseline. Status is A
// (added), M (modified) or D (deleted); a moved file is a D and an A.
type Change struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// vaultState is what a run left in a brain workspace's vault: every file,
// as a slash path relative to the vault, and how it differs from the
// baseline, as name-status changes and as a diff.
type vaultState struct {
	Files   []string
	Changes []Change
	Diff    string
}

// prepareBrain copies fixture into a fresh brain workspace, commits it as
// the baseline, writes a config that points brain at the copy with
// semantic search off, and builds the literal index the way a set-up
// machine has it, so the first search does not pay for indexing.
func prepareBrain(
	ctx context.Context,
	fixture string,
	roots workspaceRoots,
	caseID string,
) (*Workspace, error) {
	if roots.BrainBinDir == "" {
		return nil, errors.New("brain workspace: no brain binary was built for the run")
	}
	root, err := os.MkdirTemp(roots.TmpRoot, "harness-eval-"+caseID+"-")
	if err != nil {
		return nil, fmt.Errorf("brain workspace: %w", err)
	}
	// Resolve /var to /private/var on macOS, so brain and the graders see
	// the same path strings.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	h := &brainHome{Root: root, BinDir: roots.BrainBinDir}
	ws := &Workspace{Dir: h.workDir(), Kind: workspaceBrain, brain: h}
	if err := h.populate(ctx, fixture); err != nil {
		return nil, errors.Join(fmt.Errorf("brain workspace: %w", err), ws.Cleanup())
	}
	return ws, nil
}

func (h brainHome) populate(ctx context.Context, fixture string) error {
	if err := copyTree(fixture, h.vaultDir()); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	for _, dir := range []string{h.stateDir(), h.workDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	config := fmt.Sprintf("vault: %q\nsemantic:\n  enabled: false\n", h.vaultDir())
	if err := os.WriteFile(h.configPath(), []byte(config), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{
			"-c", "user.name=harness-eval", "-c", "user.email=harness-eval@example.invalid",
			"commit", "-q", "--no-verify", "--no-gpg-sign", "-m", "baseline",
		},
	} {
		if _, err := runGit(ctx, vaultGitEnv, h.vaultDir(), args...); err != nil {
			return err
		}
	}
	return h.preindex(ctx)
}

// preindex runs `brain index --literal-only` with the run's environment.
func (h brainHome) preindex(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, filepath.Join(h.BinDir, "brain"), "index", "--literal-only")
	cmd.Dir = h.workDir()
	cmd.Env = h.environ(claudeEnv(os.Environ()))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("brain index: %w: %s", err, tail(string(out), 500))
	}
	return nil
}

// environ is base with brain pointed at h: the built binary first on PATH,
// BRAIN_CONFIG at h's config and XDG_STATE_HOME at h's state directory.
// The caller's BRAIN_* variables are dropped, so the user's own brain setup
// stays out of the run.
func (h brainHome) environ(base []string) []string {
	searchPath := h.BinDir
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		name, value, _ := strings.Cut(kv, "=")
		switch {
		case name == "PATH":
			if value != "" {
				searchPath += string(os.PathListSeparator) + value
			}
		case name == xdgStateHomeEnv, strings.HasPrefix(name, "BRAIN_"):
		default:
			out = append(out, kv)
		}
	}
	return append(out,
		"PATH="+searchPath,
		brainConfigEnv+"="+h.configPath(),
		xdgStateHomeEnv+"="+h.stateDir(),
	)
}

// capture stages everything in the vault and reads back how it differs
// from the baseline, untracked files included and renames split into a
// delete and an add.
func (h brainHome) capture(ctx context.Context) (vaultState, error) {
	vault := h.vaultDir()
	if _, err := runGit(ctx, vaultGitEnv, vault, "add", "-A"); err != nil {
		return vaultState{}, err
	}
	status, err := runGit(
		ctx,
		vaultGitEnv,
		vault,
		"diff",
		"--cached",
		"--no-renames",
		"--name-status",
		"HEAD",
	)
	if err != nil {
		return vaultState{}, err
	}
	diff, err := runGit(ctx, vaultGitEnv, vault, "diff", "--cached", "--no-renames", "HEAD")
	if err != nil {
		return vaultState{}, err
	}
	files, err := listFiles(vault)
	if err != nil {
		return vaultState{}, err
	}
	return vaultState{Files: files, Changes: parseNameStatus(status), Diff: diff}, nil
}

// parseNameStatus reads `git diff --name-status` lines ("M\tpath").
func parseNameStatus(out string) []Change {
	var changes []Change
	for line := range strings.SplitSeq(out, "\n") {
		status, p, ok := strings.Cut(line, "\t")
		if !ok || status == "" {
			continue
		}
		changes = append(changes, Change{Status: status[:1], Path: p})
	}
	slices.SortFunc(changes, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
	return changes
}

// copyTree copies the regular files under src to dst, skipping .DS_Store.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Name() == ".DS_Store" || !d.Type().IsRegular():
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// listFiles lists the files under dir as sorted slash paths, skipping .git.
func listFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	slices.Sort(files)
	return files, err
}

// buildBrain compiles the brain CLI from the working tree into dir, so a
// run measures the brain beside the harness, not the one installed.
func buildBrain(ctx context.Context, dir string) error {
	if _, err := os.Stat(brainPackage); err != nil {
		return fmt.Errorf(
			"brain workspaces build %s; run the harness from the repo root: %w",
			brainPackage,
			err,
		)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(dir, "brain"), brainPackage)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s: %w: %s", brainPackage, err, tail(string(out), 1000))
	}
	return nil
}

// frontmatterValue reads key from a file's YAML frontmatter as text. It
// decodes into a generic map, so keys brain's own parser drops still count.
// A list value comes back one item per line, so "(?m)^gdpr$" finds a tag.
func frontmatterValue(file, key string) (string, bool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", false, err
	}
	front, _, err := splitFrontmatter(string(data))
	if err != nil {
		return "", false, err
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return "", false, fmt.Errorf("frontmatter: %w", err)
	}
	v, ok := fm[key]
	if !ok {
		return "", false, nil
	}
	if list, isList := v.([]any); isList {
		items := make([]string, len(list))
		for i, item := range list {
			items[i] = fmt.Sprint(item)
		}
		return strings.Join(items, "\n"), true, nil
	}
	return fmt.Sprint(v), true, nil
}

var (
	wikilinkRE   = regexp.MustCompile(`\[\[([^\[\]]+?)\]\]`)
	codeFenceRE  = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRE = regexp.MustCompile("`[^`\n]*`")
)

// wikilinkTargets returns the target of every [[target#heading|alias]]
// link outside code, in order.
func wikilinkTargets(content string) []string {
	content = inlineCodeRE.ReplaceAllString(codeFenceRE.ReplaceAllString(content, ""), "")
	var out []string
	for _, m := range wikilinkRE.FindAllStringSubmatch(content, -1) {
		target := m[1]
		if i := strings.IndexAny(target, "|#^"); i >= 0 {
			target = target[:i]
		}
		target = strings.TrimSuffix(strings.TrimSpace(target), ".md")
		if target != "" {
			out = append(out, target)
		}
	}
	return out
}

// noteIndex maps the lowercased stem and path (without .md) of every note
// to its vault path, the two ways a wikilink names a note.
func noteIndex(files []string) map[string]string {
	idx := map[string]string{}
	for _, f := range files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		noExt := strings.TrimSuffix(f, ".md")
		idx[strings.ToLower(noExt)] = f
		idx[strings.ToLower(path.Base(noExt))] = f
	}
	return idx
}

// isGlob reports whether p has a glob metacharacter.
func isGlob(p string) bool {
	return strings.ContainsAny(p, `*?[\`)
}

// matchGlob matches a slash path against a glob where "**" is a whole
// segment matching zero or more segments and other segments follow
// path.Match.
func matchGlob(glob, name string) bool {
	return matchSegments(strings.Split(glob, "/"), strings.Split(name, "/"))
}

func matchSegments(glob, name []string) bool {
	for len(glob) > 0 {
		if glob[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegments(glob[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(glob[0], name[0]); err != nil || !ok {
			return false
		}
		glob, name = glob[1:], name[1:]
	}
	return len(name) == 0
}

// validateGlob reports a malformed glob segment; "" is valid (unset).
func validateGlob(glob string) error {
	for seg := range strings.SplitSeq(glob, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("glob %q: %w", glob, err)
		}
	}
	return nil
}
