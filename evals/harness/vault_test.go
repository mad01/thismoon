package main

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// writeTree writes files, slash path to content, under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeBrain writes a brain stand-in into a temp bin dir. It appends its
// arguments and the two brain variables to $XDG_STATE_HOME/fake-brain.log.
func fakeBrain(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		`printf '%s|%s|%s\n' "$*" "$BRAIN_CONFIG" "$XDG_STATE_HOME" >> "$XDG_STATE_HOME/fake-brain.log"` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "brain"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// brainRoots is a fixture root holding fixture "team" with files, a fake
// brain, and a temp root for the workspaces.
func brainRoots(t *testing.T, files map[string]string) workspaceRoots {
	t.Helper()
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "team"), files)
	return workspaceRoots{FixtureRoot: root, BrainBinDir: fakeBrain(t), TmpRoot: t.TempDir()}
}

func TestBrainWorkspace(t *testing.T) {
	ctx := context.Background()
	roots := brainRoots(t, map[string]string{
		"notes/keep.md":   "keep\n",
		"notes/edit.md":   "before\n",
		"inbox/move.md":   "moving\n",
		"notes/.DS_Store": "junk",
	})
	spec := WorkspaceSpec{Kind: workspaceBrain, Fixture: "team"}
	ws, err := prepareWorkspace(ctx, spec, roots, "cap", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Cleanup() })
	h := ws.brain

	if ws.Dir != h.workDir() || ws.ContentDir() != h.vaultDir() || ws.Canonical() != "" {
		t.Errorf("dirs: work %s, content %s, canonical %q", ws.Dir, ws.ContentDir(), ws.Canonical())
	}
	cfg, err := os.ReadFile(h.configPath())
	if err != nil || !strings.Contains(string(cfg), `vault: "`+h.vaultDir()+`"`) ||
		!strings.Contains(string(cfg), "enabled: false") {
		t.Errorf("config.yaml = %q, %v", cfg, err)
	}
	if entries, err := os.ReadDir(ws.Dir); err != nil || len(entries) != 0 {
		t.Errorf("work dir should exist and be empty: %v %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(h.vaultDir(), "notes", ".DS_Store")); err == nil {
		t.Error(".DS_Store was copied")
	}
	log, err := os.ReadFile(filepath.Join(h.stateDir(), "fake-brain.log"))
	want := "index --literal-only|" + h.configPath() + "|" + h.stateDir() + "\n"
	if err != nil || string(log) != want {
		t.Errorf("preindex ran as %q (%v), want %q", log, err, want)
	}

	vault, err := ws.Vault(ctx)
	if err != nil || len(vault.Changes) != 0 || vault.Diff != "" {
		t.Fatalf("baseline should be clean: %+v %v", vault, err)
	}

	writeTree(t, h.vaultDir(), map[string]string{
		"notes/edit.md": "after\n",
		"notes/move.md": "moving\n",
		"inbox/new.md":  "untracked\n",
	})
	if err := os.Remove(filepath.Join(h.vaultDir(), "inbox", "move.md")); err != nil {
		t.Fatal(err)
	}
	vault, err = ws.Vault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantChanges := []Change{
		{Status: "D", Path: "inbox/move.md"},
		{Status: "A", Path: "inbox/new.md"},
		{Status: "M", Path: "notes/edit.md"},
		{Status: "A", Path: "notes/move.md"},
	}
	if !reflect.DeepEqual(vault.Changes, wantChanges) {
		t.Errorf("changes = %v, want %v", vault.Changes, wantChanges)
	}
	if !strings.Contains(vault.Diff, "+after") || !strings.Contains(vault.Diff, "inbox/new.md") {
		t.Errorf("diff misses the edit or the untracked file:\n%s", vault.Diff)
	}
	wantFiles := []string{"inbox/new.md", "notes/edit.md", "notes/keep.md", "notes/move.md"}
	if !slices.Equal(vault.Files, wantFiles) {
		t.Errorf("files = %v, want %v", vault.Files, wantFiles)
	}

	if err := ws.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.Root); !os.IsNotExist(err) {
		t.Errorf("workspace root still exists: %v", err)
	}
}

func TestBrainWorkspaceNeedsBinary(t *testing.T) {
	roots := brainRoots(t, map[string]string{"notes/a.md": "a\n"})
	roots.BrainBinDir = ""
	spec := WorkspaceSpec{Kind: workspaceBrain, Fixture: "team"}
	if _, err := prepareWorkspace(context.Background(), spec, roots, "x", time.Now()); err == nil ||
		!strings.Contains(err.Error(), "no brain binary") {
		t.Errorf("err = %v, want the missing binary named", err)
	}
}

func TestBrainEnviron(t *testing.T) {
	h := brainHome{Root: "/tmp/ws", BinDir: "/tmp/bin"}
	got := h.environ([]string{
		"HOME=/home/u", "PATH=/usr/bin", "BRAIN_CONFIG=/real/config.yaml", "BRAIN_VAULT=/real",
		"XDG_STATE_HOME=/home/u/.local/state", "TERM=x",
	})
	want := []string{
		"HOME=/home/u", "TERM=x", "PATH=/tmp/bin:/usr/bin",
		"BRAIN_CONFIG=/tmp/ws/config.yaml", "XDG_STATE_HOME=/tmp/ws/state",
	}
	if !slices.Equal(got, want) {
		t.Errorf("environ = %v, want %v", got, want)
	}
	if ws := (&Workspace{Dir: "/x"}); !slices.Equal(ws.Environ([]string{"A=1"}), []string{"A=1"}) {
		t.Error("a workspace without brain changed the environment")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		glob, name string
		want       bool
	}{
		{"notes/*.md", "notes/a.md", true},
		{"notes/*.md", "notes/sub/a.md", false},
		{"**", "notes/a.md", true},
		{"**/*.md", "a.md", true},
		{"**/*.md", "MOCs/x/y.md", true},
		{"notes/**", "inbox/a.md", false},
		{"inbox/a.md", "inbox/a.md", true},
	}
	for _, tc := range tests {
		if got := matchGlob(tc.glob, tc.name); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.glob, tc.name, got, tc.want)
		}
	}
}

func TestWikilinkTargets(t *testing.T) {
	got := wikilinkTargets(
		"[[a]] [[b|B]] [[c#h]] [[d^blk]] [[notes/e.md]] `[[code]]`\n```\n[[fenced]]\n```",
	)
	if strings.Join(got, ",") != "a,b,c,d,notes/e" {
		t.Errorf("targets = %v", got)
	}
}

// teamFixture is the vault the brain cases run against.
const teamFixture = "fixtures/brain/team"

// fixtureNote is one fixture file: its content and decoded frontmatter.
type fixtureNote struct {
	content string
	front   map[string]any
}

func (n fixtureNote) tags() []string {
	list, _ := n.front["tags"].([]any)
	var out []string
	for _, tag := range list {
		out = append(out, tag.(string))
	}
	return out
}

// fixtureNotes reads every file of the team fixture, sorted by path.
func fixtureNotes(t *testing.T) ([]string, map[string]fixtureNote) {
	t.Helper()
	files, err := listFiles(teamFixture)
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string]fixtureNote{}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(teamFixture, f))
		if err != nil {
			t.Fatal(err)
		}
		front, _, err := splitFrontmatter(string(data))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		n := fixtureNote{content: string(data)}
		if err := yaml.Unmarshal([]byte(front), &n.front); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		notes[f] = n
	}
	return files, notes
}

// brainKeys are the frontmatter keys brain's renderer keeps; brain edit
// drops any other.
var brainKeys = []string{
	"title", "aliases", "tags", "type", "status", "source", "evidence", "created", "updated", "id",
}

var outputName = regexp.MustCompile(`^outputs/\d{4}-\d{2}-\d{2}-[a-z0-9-]+\.md$`)

// TestBrainFixtureFormat checks every note carries the frontmatter brain
// writes for its tier and only keys brain keeps. The data retention
// policy's extra keys are the one exception, by design: the
// edit-preserve-frontmatter case grades that they survive an edit.
func TestBrainFixtureFormat(t *testing.T) {
	files, notes := fixtureNotes(t)
	wantStatus := map[string]string{
		"inbox": "raw", "notes": "validated", "outputs": "validated", "MOCs": "validated",
	}
	for _, f := range files {
		n := notes[f]
		tier, _, _ := strings.Cut(f, "/")
		for _, key := range []string{"title", "type", "created", "updated"} {
			if n.front[key] == nil {
				t.Errorf("%s: missing %s", f, key)
			}
		}
		if n.front["status"] != wantStatus[tier] {
			t.Errorf(
				"%s: status %v, want %q for tier %s",
				f,
				n.front["status"],
				wantStatus[tier],
				tier,
			)
		}
		if tier == "outputs" &&
			(!outputName.MatchString(f) || !slices.Contains(n.tags(), "meta/output")) {
			t.Errorf("%s: outputs are date-prefixed and tagged meta/output", f)
		}
		var extra []string
		for key := range n.front {
			if !slices.Contains(brainKeys, key) {
				extra = append(extra, key)
			}
		}
		slices.Sort(extra)
		wantExtra := []string(nil)
		if f == "notes/data-retention-policy.md" {
			wantExtra = []string{"classification", "owner"}
		}
		if !slices.Equal(extra, wantExtra) {
			t.Errorf("%s: keys brain drops = %v, want %v", f, extra, wantExtra)
		}
	}
}

func TestBrainFixtureLinksResolve(t *testing.T) {
	files, notes := fixtureNotes(t)
	idx := noteIndex(files)
	for _, f := range files {
		for _, target := range wikilinkTargets(notes[f].content) {
			if _, ok := idx[strings.ToLower(target)]; !ok {
				t.Errorf("%s: [[%s]] does not resolve", f, target)
			}
		}
	}
}

// linkers returns the files with a wikilink to stem.
func linkers(files []string, notes map[string]fixtureNote, stem string) []string {
	var out []string
	for _, f := range files {
		if slices.Contains(wikilinkTargets(notes[f].content), stem) {
			out = append(out, f)
		}
	}
	return out
}

// slugOf mirrors brain's stem rule: lowercase, every run of other
// characters one hyphen, no hyphen at either end.
func slugOf(title string) string {
	return strings.Trim(
		regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(title), "-"),
		"-",
	)
}

// TestBrainFixtureTraps pins the facts the brain cases grade against.
func TestBrainFixtureTraps(t *testing.T) {
	files, notes := fixtureNotes(t)

	wantBacklinks := []string{
		"MOCs/databases.md",
		"notes/ledger-replica-rebuild.md",
		"notes/ledger-svc.md",
		"notes/pg-failover-drill.md",
		"outputs/2026-07-10-failover-drill-report.md",
	}
	if got := linkers(files, notes, "pg-failover"); !slices.Equal(got, wantBacklinks) {
		t.Errorf("backlinks-exact: [[pg-failover]] linkers = %v, want %v", got, wantBacklinks)
	}
	for _, decoy := range []string{"notes/oncall-escalation.md", "notes/incident-2026-03-14-ledger-outage.md"} {
		if !slices.Contains(linkers(files, notes, "pg-failover-drill"), decoy) {
			t.Errorf("backlinks-exact: decoy %s should link [[pg-failover-drill]]", decoy)
		}
	}

	if got := linkers(files, notes, "rp-timeouts"); len(got) != 3 {
		t.Errorf("promote-with-links: [[rp-timeouts]] linkers = %v, want 3", got)
	}
	if title, _ := notes["inbox/rp-timeouts.md"].front["title"].(string); slugOf(
		title,
	) == "rp-timeouts" {
		t.Errorf("promote-with-links: promoting must change the stem (title %q)", title)
	}

	var runbooks []string
	for _, f := range files {
		tags := notes[f].tags()
		if strings.HasPrefix(f, "notes/") && slices.Contains(tags, "runbook") &&
			slices.Contains(tags, "postgres") {
			runbooks = append(runbooks, path.Base(f))
		}
	}
	wantRunbooks := []string{"ledger-replica-rebuild.md", "pg-failover-drill.md", "pg-failover.md"}
	if !slices.Equal(runbooks, wantRunbooks) {
		t.Errorf("find-filtered: postgres runbooks = %v, want %v", runbooks, wantRunbooks)
	}

	for f, n := range notes {
		if strings.Contains(strings.ToLower(n.content), "certificate") {
			t.Errorf("find-not-in-vault: %s mentions certificates", f)
		}
	}
	policy := notes["notes/data-retention-policy.md"].front
	if policy["owner"] != "data-governance" || policy["classification"] != "internal-restricted" {
		t.Error("edit-preserve-frontmatter: policy note lost its extra keys")
	}
	if !strings.Contains(notes["notes/ledger-svc.md"].content, "max 40") {
		t.Error("capture-dedupe: ledger-svc should hold the old pool size")
	}
}
