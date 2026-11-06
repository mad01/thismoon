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

// ghLogEnv names the file the fake gh appends its calls to (see fakegh.go).
const ghLogEnv = "HARNESS_EVAL_GH_LOG"

// vaultGitEnv keeps the caller's git settings out of the vaults' scratch
// repos: no global or system config, so no global hooks, signing or
// templates, and a fixed identity. The harness's own git calls run with it,
// and so does brain, which commits what it changes.
var vaultGitEnv = []string{
	"GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_CONFIG_NOSYSTEM=1",
	"GIT_AUTHOR_NAME=harness-eval",
	"GIT_AUTHOR_EMAIL=harness-eval@example.invalid",
	"GIT_COMMITTER_NAME=harness-eval",
	"GIT_COMMITTER_EMAIL=harness-eval@example.invalid",
}

// The vaults of a two-vault workspace. Each name is the fixture's
// subdirectory, the vault's name in brain's config, and the prefix of its
// paths in the evidence: shared/notes/x.md is brain's shared:notes/x.md.
const (
	privateVault = "private"
	sharedVault  = "shared"
)

// baselineTag names the fixture commit. brain commits its own changes to
// the vault, so HEAD moves during a run; the tag does not.
const baselineTag = "harness-baseline"

// brainHome is a brain workspace's scratch directory. A one-vault
// workspace keeps its vault at
//
//	<Root>/vault           the fixture copy, committed as the git baseline
//
// and a two-vault workspace, made from a fixture with private/ and shared/
// subdirectories, at
//
//	<Root>/vaults/private  the private vault: a local-only repo, no remote
//	<Root>/vaults/shared   the shared vault: a clone of origin.git
//	<Root>/origin.git      a bare repo, the shared vault's origin
//
// Both kinds have
//
//	<Root>/config.yaml     brain's config: the vaults, semantic search off
//	<Root>/state           XDG_STATE_HOME, where brain keeps its indexes
//	<Root>/work            claude's working directory, empty
//	<Root>/gh.log          the fake gh's call log
//
// BinDir holds the brain binary built for the run, and the fake gh.
type brainHome struct {
	Root     string
	BinDir   string
	TwoVault bool
}

func (h brainHome) vaultDir() string   { return filepath.Join(h.Root, "vault") }
func (h brainHome) vaultsDir() string  { return filepath.Join(h.Root, "vaults") }
func (h brainHome) originDir() string  { return filepath.Join(h.Root, "origin.git") }
func (h brainHome) configPath() string { return filepath.Join(h.Root, "config.yaml") }
func (h brainHome) stateDir() string   { return filepath.Join(h.Root, "state") }
func (h brainHome) workDir() string    { return filepath.Join(h.Root, "work") }
func (h brainHome) ghLogPath() string  { return filepath.Join(h.Root, "gh.log") }

// contentDir is where the vault paths of the evidence resolve: the vault,
// or the directory holding both vaults of a two-vault workspace.
func (h brainHome) contentDir() string {
	if h.TwoVault {
		return h.vaultsDir()
	}
	return h.vaultDir()
}

// vaultRepo is one vault of a workspace: the prefix its paths carry in the
// evidence ("" in a one-vault workspace) and its directory.
type vaultRepo struct {
	Prefix string
	Dir    string
}

func (h brainHome) vaults() []vaultRepo {
	if !h.TwoVault {
		return []vaultRepo{{Dir: h.vaultDir()}}
	}
	return []vaultRepo{
		{Prefix: privateVault, Dir: filepath.Join(h.vaultsDir(), privateVault)},
		{Prefix: sharedVault, Dir: filepath.Join(h.vaultsDir(), sharedVault)},
	}
}

// isTwoVault reports whether fixture holds a private and a shared vault.
func isTwoVault(fixture string) bool {
	for _, name := range []string{privateVault, sharedVault} {
		if info, err := os.Stat(filepath.Join(fixture, name)); err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// Change is one vault path that differs from the baseline. Status is A
// (added), M (modified) or D (deleted); a moved file is a D and an A.
type Change struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// vaultState is what a run left in a brain workspace: every vault file, as
// a slash path relative to the content directory, and how the vaults
// differ from their baselines, as name-status changes and as a diff. A
// two-vault workspace adds the origin branches the run pushed, whose diffs
// follow the vaults' in Diff. GHCalls are the fake gh's calls.
type vaultState struct {
	Files    []string
	Changes  []Change
	Diff     string
	TwoVault bool
	Branches []OriginBranch
	GHCalls  []string
}

// prepareBrain copies fixture into a fresh brain workspace, commits each
// vault as its baseline, writes a config that points brain at the copies
// with semantic search off, and builds the literal index the way a set-up
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
	h := &brainHome{Root: root, BinDir: roots.BrainBinDir, TwoVault: isTwoVault(fixture)}
	ws := &Workspace{Dir: h.workDir(), Kind: workspaceBrain, brain: h}
	if err := h.populate(ctx, fixture); err != nil {
		return nil, errors.Join(fmt.Errorf("brain workspace: %w", err), ws.Cleanup())
	}
	return ws, nil
}

func (h brainHome) populate(ctx context.Context, fixture string) error {
	for _, dir := range []string{h.stateDir(), h.workDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	populate := h.populateOne
	if h.TwoVault {
		populate = h.populateTwo
	}
	if err := populate(ctx, fixture); err != nil {
		return err
	}
	if err := os.WriteFile(h.configPath(), []byte(h.config()), 0o644); err != nil {
		return err
	}
	return h.preindex(ctx)
}

// populateOne copies fixture to the vault and commits it as the baseline.
func (h brainHome) populateOne(ctx context.Context, fixture string) error {
	if err := copyTree(fixture, h.vaultDir()); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	if _, err := runGit(ctx, vaultGitEnv, h.vaultDir(), "init", "-q", "-b", "main"); err != nil {
		return err
	}
	return commitBaseline(ctx, h.vaultDir())
}

// populateTwo makes the private vault a local-only repo and the shared one
// a clone of a fresh bare origin, then pushes the shared baseline and its
// tag there, so the origin starts with main at the baseline.
func (h brainHome) populateTwo(ctx context.Context, fixture string) error {
	repos := h.vaults()
	private, shared := repos[0].Dir, repos[1].Dir
	if err := copyTree(filepath.Join(fixture, privateVault), private); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	if _, err := runGit(ctx, vaultGitEnv, private, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := commitBaseline(ctx, private); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q", "--bare", "-b", "main", h.originDir()},
		{"clone", "-q", h.originDir(), shared},
		{"-C", shared, "symbolic-ref", "HEAD", "refs/heads/main"},
	} {
		if _, err := runGit(ctx, vaultGitEnv, h.Root, args...); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(fixture, sharedVault), shared); err != nil {
		return fmt.Errorf("copy fixture: %w", err)
	}
	if err := commitBaseline(ctx, shared); err != nil {
		return err
	}
	_, err := runGit(ctx, vaultGitEnv, shared, "push", "-q", "-u", "origin", "main", baselineTag)
	return err
}

// commitBaseline commits everything in the repo at dir and tags it.
func commitBaseline(ctx context.Context, dir string) error {
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "--no-verify", "--no-gpg-sign", "-m", "baseline"},
		{"tag", baselineTag},
	} {
		if _, err := runGit(ctx, vaultGitEnv, dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// config is brain's config for h: the vault, or both vaults with the
// shared one marked, and semantic search off.
func (h brainHome) config() string {
	const semanticOff = "semantic:\n  enabled: false\n"
	if !h.TwoVault {
		return fmt.Sprintf("vault: %q\n", h.vaultDir()) + semanticOff
	}
	repos := h.vaults()
	return fmt.Sprintf(
		"vaults:\n  - name: %s\n    path: %q\n  - name: %s\n    path: %q\n    shared: true\n",
		repos[0].Prefix, repos[0].Dir, repos[1].Prefix, repos[1].Dir,
	) + semanticOff
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

// environ is base with brain pointed at h: the built binary and the fake
// gh first on PATH, BRAIN_CONFIG at h's config, XDG_STATE_HOME at h's state
// directory, the fake gh's log under h, and vaultGitEnv for brain's own
// commits. The caller's BRAIN_* and GIT_* variables are dropped, so the
// user's own brain and git setup stay out of the run.
func (h brainHome) environ(base []string) []string {
	searchPath := h.BinDir
	out := make([]string, 0, len(base)+4+len(vaultGitEnv))
	for _, kv := range base {
		name, value, _ := strings.Cut(kv, "=")
		switch {
		case name == "PATH":
			if value != "" {
				searchPath += string(os.PathListSeparator) + value
			}
		case name == xdgStateHomeEnv, name == ghLogEnv,
			strings.HasPrefix(name, "BRAIN_"), strings.HasPrefix(name, "GIT_"):
		default:
			out = append(out, kv)
		}
	}
	out = append(out,
		"PATH="+searchPath,
		brainConfigEnv+"="+h.configPath(),
		xdgStateHomeEnv+"="+h.stateDir(),
		ghLogEnv+"="+h.ghLogPath(),
	)
	return append(out, vaultGitEnv...)
}

// capture reads back what the run left: how each vault differs from its
// baseline, the shared origin's pushed branches in a two-vault workspace,
// and the fake gh's calls.
func (h brainHome) capture(ctx context.Context) (vaultState, error) {
	st := vaultState{TwoVault: h.TwoVault}
	var diffs []string
	for _, v := range h.vaults() {
		changes, diff, err := captureVault(ctx, v)
		if err != nil {
			return vaultState{}, err
		}
		st.Changes = append(st.Changes, changes...)
		if diff != "" {
			diffs = append(diffs, diff)
		}
	}
	files, err := listFiles(h.contentDir())
	if err != nil {
		return vaultState{}, err
	}
	st.Files = files
	if h.TwoVault {
		branches, branchDiffs, err := originBranches(ctx, h.originDir())
		if err != nil {
			return vaultState{}, err
		}
		st.Branches = branches
		diffs = append(diffs, branchDiffs...)
	}
	st.Diff = strings.Join(diffs, "\n")
	st.GHCalls, err = readGHLog(h.ghLogPath())
	return st, err
}

// captureVault stages everything in v and reads back how it differs from
// the baseline, untracked files included and renames split into a delete
// and an add, with v's prefix on every path. It diffs against the baseline
// tag, not HEAD, so the commits brain made during the run count as changes.
func captureVault(ctx context.Context, v vaultRepo) ([]Change, string, error) {
	if _, err := runGit(ctx, vaultGitEnv, v.Dir, "add", "-A"); err != nil {
		return nil, "", err
	}
	diffArgs := []string{"diff", "--cached", "--no-renames"}
	status, err := runGit(
		ctx, vaultGitEnv, v.Dir, append(diffArgs, "--name-status", baselineTag)...)
	if err != nil {
		return nil, "", err
	}
	diff, err := runGit(
		ctx, vaultGitEnv, v.Dir, append(diffArgs, prefixArgs(v.Prefix, baselineTag)...)...)
	if err != nil {
		return nil, "", err
	}
	return prefixChanges(parseNameStatus(status), v.Prefix), diff, nil
}

// prefixArgs is args after git diff's flags that put prefix in front of
// every path; a one-vault workspace keeps git's a/ and b/.
func prefixArgs(prefix string, args ...string) []string {
	if prefix == "" {
		return args
	}
	flags := []string{"--src-prefix=a/" + prefix + "/", "--dst-prefix=b/" + prefix + "/"}
	return append(flags, args...)
}

// prefixChanges puts prefix in front of every change's path.
func prefixChanges(changes []Change, prefix string) []Change {
	if prefix == "" {
		return changes
	}
	for i := range changes {
		changes[i].Path = prefix + "/" + changes[i].Path
	}
	return changes
}

// OriginBranch is a branch of the shared vault's origin whose tip is not
// the baseline: one the run pushed, or main moved by a direct push.
// Changes are the paths it changes from the baseline, under the shared/
// prefix; content holds each added or modified file as the branch has it.
type OriginBranch struct {
	Name    string   `json:"name"`
	Changes []Change `json:"changes"`
	content map[string]string
}

// originBranches reads every branch of the bare origin at dir that moved
// off the baseline tag, with what it changes, and a diff of each headed
// "# origin branch <name>".
func originBranches(ctx context.Context, dir string) ([]OriginBranch, []string, error) {
	base, err := runGit(ctx, vaultGitEnv, dir, "rev-parse", baselineTag+"^{commit}")
	if err != nil {
		return nil, nil, err
	}
	refs, err := runGit(ctx, vaultGitEnv, dir,
		"for-each-ref", "--format=%(objectname) %(refname)", "refs/heads/")
	if err != nil {
		return nil, nil, err
	}
	var (
		branches []OriginBranch
		diffs    []string
	)
	for line := range strings.SplitSeq(refs, "\n") {
		tip, ref, ok := strings.Cut(line, " ")
		if !ok || tip == base {
			continue
		}
		b, diff, err := readBranch(ctx, dir, strings.TrimPrefix(ref, "refs/heads/"), base, tip)
		if err != nil {
			return nil, nil, err
		}
		branches = append(branches, b)
		diffs = append(diffs, "# origin branch "+b.Name+"\n"+diff)
	}
	return branches, diffs, nil
}

// readBranch reads what branch name, at commit tip, changes from base.
func readBranch(ctx context.Context, dir, name, base, tip string) (OriginBranch, string, error) {
	status, err := runGit(ctx, vaultGitEnv, dir, "diff", "--no-renames", "--name-status", base, tip)
	if err != nil {
		return OriginBranch{}, "", err
	}
	b := OriginBranch{Name: name, content: map[string]string{}}
	for _, c := range parseNameStatus(status) {
		p := sharedVault + "/" + c.Path
		if c.Status != "D" {
			data, err := runGit(ctx, vaultGitEnv, dir, "show", tip+":"+c.Path)
			if err != nil {
				return OriginBranch{}, "", err
			}
			b.content[p] = data
		}
		b.Changes = append(b.Changes, Change{Status: c.Status, Path: p})
	}
	diffArgs := append([]string{"diff", "--no-renames"}, prefixArgs(sharedVault, base, tip)...)
	diff, err := runGit(ctx, vaultGitEnv, dir, diffArgs...)
	return b, diff, err
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
// to its vault path, the two ways a wikilink names a note. linkIndex keeps
// one per vault of a two-vault workspace.
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

// linkIndex resolves wikilinks across a workspace's vaults the way brain
// does. A one-vault workspace has one index, under "". In a two-vault
// workspace every path starts with its vault, and a link resolves in its
// own vault first; a private note's link then tries the shared vault, and a
// shared note's link never reaches the private one.
type linkIndex struct {
	twoVault bool
	byVault  map[string]map[string]string
}

func newLinkIndex(files []string, twoVault bool) linkIndex {
	x := linkIndex{twoVault: twoVault, byVault: map[string]map[string]string{}}
	if !twoVault {
		x.byVault[""] = noteIndex(files)
		return x
	}
	grouped := map[string][]string{}
	for _, f := range files {
		if vault, rel, ok := strings.Cut(f, "/"); ok {
			grouped[vault] = append(grouped[vault], rel)
		}
	}
	for vault, rels := range grouped {
		idx := noteIndex(rels)
		for key, rel := range idx {
			idx[key] = vault + "/" + rel
		}
		x.byVault[vault] = idx
	}
	return x
}

// resolve returns the note a [[target]] in file from links to.
func (x linkIndex) resolve(from, target string) (string, bool) {
	key := strings.ToLower(target)
	if !x.twoVault {
		note, ok := x.byVault[""][key]
		return note, ok
	}
	vault, _, _ := strings.Cut(from, "/")
	if note, ok := x.byVault[vault][key]; ok {
		return note, true
	}
	if vault == sharedVault {
		return "", false
	}
	note, ok := x.byVault[sharedVault][key]
	return note, ok
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
