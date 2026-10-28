package discover

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// ActionsEcosystem is OSV's ecosystem string for GitHub Actions.
const ActionsEcosystem = "GitHub Actions"

// movableRefID is the rule id of the signal on an action pinned to a tag or a
// branch.
const movableRefID = "deps-movable-ref"

// GitHubActions discovers the actions a repo's CI runs: every remote `uses:`
// in the workflows under .github/workflows, in the local composite actions
// under .github/actions, and in any other local action a `uses: ./path` ref
// runs (localAction). Each ref becomes a package named owner/repo whose
// version carries the commit it runs (see actionVersion), so OSV can match it
// by commit. A tag or branch ref is resolved to its commit through the action
// repo's ref advertisement and carries a movable-ref signal. A ref that can't
// be resolved is left out and reported in the error, beside the refs that
// were read. docker:// refs are skipped. Actions have no reachability, so
// every ref is Direct and Imported.
type GitHubActions struct {
	// BaseURL serves the ref advertisements; empty means https://github.com.
	BaseURL string
}

func (GitHubActions) Name() string { return ActionsEcosystem }

func (g GitHubActions) Discover(repoRoot string, opts Options) ([]Package, error) {
	files, err := actionFiles(repoRoot, opts)
	if err != nil {
		return nil, err
	}
	cache := opts.refs
	if cache == nil {
		cache = refCache{}
	}
	var (
		pkgs   []Package
		errs   []error
		failed = map[string]bool{}
		// queued holds every file read or due, so a local action that
		// several files run, or that runs itself, is read once.
		queued = map[string]bool{}
	)
	for _, f := range files {
		queued[f] = true
	}
	for i := 0; i < len(files); i++ { // files grows as local actions are found
		refs, locals, err := parseActionFile(files[i])
		if err != nil {
			errs = append(errs, fmt.Errorf("parse %s: %w", files[i], err))
			continue
		}
		for _, uses := range locals {
			if f, ok := localAction(repoRoot, uses, opts); ok && !queued[f] {
				queued[f] = true
				files = append(files, f)
			}
		}
		got, refErrs := g.pkgs(refs, files[i], cache, failed)
		pkgs = append(pkgs, got...)
		errs = append(errs, refErrs...)
	}
	return pkgs, errors.Join(errs...)
}

// pkgs builds the packages for the refs in manifest, one per owner/repo@ref.
// A ref that failed in an earlier file is skipped, so its error is reported
// once per repo.
func (g GitHubActions) pkgs(
	refs []actionRef,
	manifest string,
	cache refCache,
	failed map[string]bool,
) ([]Package, []error) {
	var (
		pkgs []Package
		errs []error
		seen = map[string]bool{}
	)
	for _, r := range refs {
		id := r.repo + "@" + r.ref
		if seen[id] || failed[id] {
			continue
		}
		seen[id] = true
		p, err := g.pkg(r, manifest, cache)
		if err != nil {
			failed[id] = true
			errs = append(errs, err)
			continue
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, errs
}

// localAction returns the action file a local `uses: ./path` ref runs: the
// action.yml or action.yaml in path, which GitHub resolves against the repo
// root, so `uses: ./` runs the repo's own action. A path outside the repo or
// under an excluded directory is skipped, and so is one with no action file,
// since a job may generate it before the step runs.
func localAction(repoRoot, uses string, opts Options) (string, bool) {
	root := filepath.Clean(repoRoot)
	dir := filepath.Join(root, filepath.FromSlash(uses))
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	for d := dir; d != root && d != filepath.Dir(d); d = filepath.Dir(d) {
		if opts.excluded(root, d) {
			return "", false
		}
	}
	for _, name := range []string{"action.yml", "action.yaml"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, true
		}
	}
	return "", false
}

// pkg builds the package for one ref in manifest. A SHA pin needs no lookup; a
// tag or branch is resolved to its commit and carries the movable-ref signal.
func (g GitHubActions) pkg(r actionRef, manifest string, cache refCache) (Package, error) {
	p := Package{
		Ecosystem:    ActionsEcosystem,
		Name:         r.repo,
		ManifestPath: manifest,
		Direct:       true,
		Imported:     true, // Actions reachability isn't computed; never hide.
	}
	if isCommitSHA(r.ref) {
		p.Version = actionVersion(r.label, strings.ToLower(r.ref))
		return p, nil
	}
	refs, err := cache.advertisement(g.advertisementURL(r.repo))
	if err != nil {
		return Package{}, fmt.Errorf("resolve %s@%s: %w", r.repo, r.ref, err)
	}
	commit, kind, ok := resolveRef(refs, r.ref)
	if !ok {
		return Package{}, fmt.Errorf("resolve %s@%s: no such tag or branch", r.repo, r.ref)
	}
	p.Version = actionVersion(releaseLabel(refs, r.ref, commit), commit)
	p.Advisories = []store.Advisory{movableRef(kind, r.ref)}
	return p, nil
}

// releaseLabel is the version label of an action pinned to ref, which
// resolved to commit. A full version (v4.2.2) is its own label. Any other
// ref, such as v4 or main, says nothing about which release it runs, so OSV's
// range filter would keep every advisory on the action; the label is instead
// the highest full-version tag on the same commit. With none, it is the ref.
func releaseLabel(refs map[string]string, ref, commit string) string {
	if _, ok := fullVersion(ref); ok {
		return ref
	}
	var (
		label string
		best  [3]uint64
	)
	for name, sha := range refs {
		tag, ok := strings.CutPrefix(name, "refs/tags/")
		if !ok || !strings.EqualFold(sha, commit) {
			continue
		}
		v, ok := fullVersion(tag)
		if !ok {
			continue
		}
		c := slices.Compare(v[:], best[:])
		if label == "" || c > 0 || (c == 0 && tag < label) {
			label, best = tag, v
		}
	}
	if label == "" {
		return ref
	}
	return label
}

// fullVersion reads a full release version, v4.2.2 or 4.2.2: the shape OSV's
// range filter compares.
func fullVersion(s string) ([3]uint64, bool) {
	var v [3]uint64
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != len(v) {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// advertisementURL is the smart-HTTP ref advertisement of the action repo
// owner/repo: the refs `git ls-remote` lists, with no git binary needed.
func (g GitHubActions) advertisementURL(repo string) string {
	base := g.BaseURL
	if base == "" {
		base = "https://github.com"
	}
	return base + "/" + repo + ".git/info/refs?service=git-upload-pack"
}

// movableRef is the signal on an action pinned to a tag or branch: the action
// repo can point the name at other code at any time, as happened to every
// tag of tj-actions/changed-files in March 2025.
func movableRef(kind, ref string) store.Advisory {
	return store.Advisory{
		ID:   movableRefID,
		Kind: store.KindSignal,
		Summary: fmt.Sprintf(
			"pinned to the %s %s, which can be moved to other code; pin a commit SHA instead",
			kind, ref,
		),
	}
}

// actionVersion is the version an action is keyed on: its label (the tag or
// branch, or a SHA pin's comment) and the commit it runs, joined by '+', as in
// v45.0.7+a284dc1814e3fd07f2e34267fc8f81227ed29fb8. The commit makes the
// finding key change whenever the code does, so a tag moved to other code
// reads as a new version. A SHA pin with no comment is its commit alone.
func actionVersion(label, commit string) string {
	if label == "" {
		return commit
	}
	return label + "+" + commit
}

// SplitActionVersion splits a GitHub Actions version into its label and the
// commit after its last '+'. A version with no full commit SHA there is all
// label.
func SplitActionVersion(version string) (label, commit string) {
	i := strings.LastIndexByte(version, '+')
	if !isCommitSHA(version[i+1:]) {
		return version, ""
	}
	if i < 0 {
		return "", version
	}
	return version[:i], version[i+1:]
}

// isCommitSHA reports whether s is a full 40-character hex commit id.
func isCommitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// actionFiles lists the files whose refs a repo's CI runs: the workflows
// directly under .github/workflows (GitHub reads no subdirectory there) and
// every action.yml or action.yaml under .github/actions.
func actionFiles(repoRoot string, opts Options) ([]string, error) {
	if opts.excluded(repoRoot, filepath.Join(repoRoot, ".github")) {
		return nil, nil
	}
	var files []string
	workflows := filepath.Join(repoRoot, ".github", "workflows")
	entries, err := os.ReadDir(workflows)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if !opts.excluded(repoRoot, workflows) {
		for _, e := range entries {
			ext := filepath.Ext(e.Name())
			if e.Type().IsRegular() && (ext == ".yml" || ext == ".yaml") {
				files = append(files, filepath.Join(workflows, e.Name()))
			}
		}
	}
	actions := filepath.Join(repoRoot, ".github", "actions")
	if _, err := os.Stat(actions); errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	err = filepath.WalkDir(actions, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, don't abort the repo
		}
		if d.IsDir() {
			if opts.excluded(repoRoot, path) || (path != actions && isNestedCheckout(path)) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "action.yml" || d.Name() == "action.yaml" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// actionRef is one remote `uses:` ref: the action repo (owner/repo), the ref
// it pins, and the version label from a trailing comment (# v4.2.2), if any.
type actionRef struct {
	repo, ref, label string
}

// parseActionFile returns the remote refs a workflow or composite action
// file uses, in file order, and the local (./) action paths it runs.
func parseActionFile(path string) (refs []actionRef, locals []string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	for _, n := range usesNodes(&doc) {
		if strings.HasPrefix(n.Value, "./") {
			locals = append(locals, n.Value)
			continue
		}
		if r, ok := parseUses(n.Value, n.LineComment); ok {
			refs = append(refs, r)
		}
	}
	return refs, locals, nil
}

// usesNodes returns the `uses:` values GitHub runs: jobs.<id>.uses (a reusable
// workflow), jobs.<id>.steps[].uses, and runs.steps[].uses in a composite
// action. A `uses` key anywhere else, such as an action input, is not a ref.
func usesNodes(doc *yaml.Node) []*yaml.Node {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	var out []*yaml.Node
	add := func(n *yaml.Node) {
		if n != nil && n.Kind == yaml.ScalarNode {
			out = append(out, n)
		}
	}
	addSteps := func(steps *yaml.Node) {
		if steps == nil || steps.Kind != yaml.SequenceNode {
			return
		}
		for _, step := range steps.Content {
			add(mapValue(step, "uses"))
		}
	}
	if jobs := mapValue(root, "jobs"); jobs != nil && jobs.Kind == yaml.MappingNode {
		for i := 1; i < len(jobs.Content); i += 2 {
			job := deref(jobs.Content[i])
			add(mapValue(job, "uses"))
			addSteps(mapValue(job, "steps"))
		}
	}
	addSteps(mapValue(mapValue(root, "runs"), "steps"))
	return out
}

// mapValue returns the value under key in mapping n, with aliases followed, or
// nil when n is not a mapping or has no such key.
func mapValue(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return deref(n.Content[i+1])
		}
	}
	return nil
}

// deref follows a YAML alias to the node it names.
func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// parseUses reads one `uses:` value with its line comment. It keeps remote
// refs (owner/repo[/path]@ref) and skips local ./ actions, docker:// images,
// and values GitHub would reject: no ref, or a ref with '@' or ':' in it.
func parseUses(uses, comment string) (actionRef, bool) {
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
		return actionRef{}, false
	}
	path, ref, ok := strings.Cut(uses, "@")
	if !ok || ref == "" || strings.ContainsAny(ref, "@:") {
		return actionRef{}, false
	}
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return actionRef{}, false
	}
	return actionRef{
		repo:  parts[0] + "/" + parts[1],
		ref:   ref,
		label: commentLabel(comment),
	}, true
}

// commentLabel is the version label in a ref's trailing comment, its first
// word: "# v4.2.2" gives v4.2.2. A word with '@' or ':' in it would break the
// finding key, so it gives no label.
func commentLabel(comment string) string {
	words := strings.Fields(strings.TrimPrefix(strings.TrimSpace(comment), "#"))
	if len(words) == 0 || strings.ContainsAny(words[0], "@:") {
		return ""
	}
	return words[0]
}
