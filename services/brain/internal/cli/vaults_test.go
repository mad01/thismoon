package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// twoVaults are the notes of the personal (default) and team (shared)
// vaults setupVaults writes. notes/shared-name.md exists in both.
var twoVaults = map[string]map[string]string{
	"personal": {
		"notes/mine.md":        "---\ntitle: Mine\n---\nPersonal kubernetes note, see [[team-guide]].\n",
		"notes/shared-name.md": "---\ntitle: Same Name\n---\nPersonal copy.\n",
		"inbox/draft.md":       "---\ntitle: Draft\nstatus: raw\n---\nA kubernetes draft.\n",
	},
	"team": {
		"notes/team-guide.md":  "---\ntitle: Team Guide\n---\nTeam kubernetes guide.\n",
		"notes/shared-name.md": "---\ntitle: Same Name\n---\nTeam copy.\n",
		"inbox/proposal.md":    "---\ntitle: Proposal\nstatus: raw\n---\nA team proposal.\n",
	},
}

// setupVaults isolates brain, writes a config with a private personal vault
// and a shared team vault, and returns the vault roots by name.
func setupVaults(t *testing.T) map[string]string {
	t.Helper()
	home := testhome.Isolate(t)
	roots := map[string]string{}
	for name, notes := range twoVaults {
		root := t.TempDir()
		roots[name] = root
		for rel, content := range notes {
			full := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := "vaults:\n" +
		"  - {name: personal, path: " + roots["personal"] + "}\n" +
		"  - {name: team, path: " + roots["team"] + ", shared: true}\n"
	cfgDir := filepath.Join(home, ".config", "brain")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return roots
}

func TestVaultsSearchTagsEveryHit(t *testing.T) {
	setupVaults(t)
	got := runBrain(t, "", "search", "kubernetes", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	res := decodeStrict[search.Result](t, got.stdout)
	hits := map[string]bool{}
	for _, f := range res.Files {
		hits[f.Vault+":"+f.Path] = true
	}
	for _, want := range []string{"personal:notes/mine.md", "personal:inbox/draft.md", "team:notes/team-guide.md"} {
		if !hits[want] {
			t.Errorf("hits %v lack %s", hits, want)
		}
	}

	got = runBrain(t, "", "search", "kubernetes", "--vault", "team")
	if got.code != exitOK || strings.TrimSpace(got.stdout) != "team:notes/team-guide.md" {
		t.Errorf("--vault team: exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	got = runBrain(t, "", "count", "kubernetes", "--group-by", "vault", "-o", "json")
	counts := decodeStrict[search.Counts](t, got.stdout)
	want := []search.CountResult{{Group: "personal", Count: 2}, {Group: "team", Count: 1}}
	if counts.Total != 3 || len(counts.Groups) != 2 || counts.Groups[0] != want[0] ||
		counts.Groups[1] != want[1] {
		t.Errorf("counts = %+v, want total 3 and %+v", counts, want)
	}
}

func TestVaultsListAcrossVaults(t *testing.T) {
	setupVaults(t)
	got := runBrain(t, "", "list", "inbox", "-o", "json")
	listing := decodeStrict[vault.Listing](t, got.stdout)
	if listing.Total != 2 {
		t.Fatalf("listing = %+v, want both inboxes", listing)
	}
	vaults := map[string]bool{}
	for _, n := range listing.Notes {
		vaults[n.Vault] = true
	}
	if !vaults["personal"] || !vaults["team"] {
		t.Errorf("listing vaults = %v", vaults)
	}
}

func TestVaultsAddressing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
		wantOut    string
	}{
		{
			name:       "bare path in two vaults is ambiguous",
			args:       []string{"read", "notes/shared-name.md"},
			wantCode:   exitUsage,
			wantStderr: "personal:notes/shared-name.md, team:notes/shared-name.md",
		},
		{
			name:     "qualified path picks the vault",
			args:     []string{"read", "team:notes/shared-name.md"},
			wantCode: exitOK,
			wantOut:  "Team copy.",
		},
		{
			name:     "--vault narrows a bare path",
			args:     []string{"read", "notes/shared-name.md", "--vault", "personal"},
			wantCode: exitOK,
			wantOut:  "Personal copy.",
		},
		{
			name:     "bare path in one vault",
			args:     []string{"peek", "notes/team-guide.md"},
			wantCode: exitOK,
			wantOut:  "Team kubernetes guide.",
		},
		{
			name:       "unknown vault",
			args:       []string{"read", "nope:notes/a.md"},
			wantCode:   exitUsage,
			wantStderr: `vault: unknown vault "nope"`,
		},
		{
			name:       "unknown --vault",
			args:       []string{"search", "x", "--vault", "nope"},
			wantCode:   exitUsage,
			wantStderr: `vault: unknown vault "nope"`,
		},
		{
			name:       "missing in a qualified vault",
			args:       []string{"read", "team:notes/mine.md"},
			wantCode:   exitNotFound,
			wantStderr: "vault: note not found: team:notes/mine.md",
		},
		{
			name:     "write goes to the default vault",
			args:     []string{"write", "--title", "Fresh", "--body", "x"},
			wantCode: exitOK,
			wantOut:  "personal:inbox/fresh.md",
		},
		{
			name:       "write to a shared vault is refused",
			args:       []string{"write", "--vault", "team", "--title", "Fresh", "--body", "x"},
			wantCode:   exitReadOnly,
			wantStderr: "vault: read-only: team is a shared vault; shared notes change through a pull request",
		},
		{
			name:       "edit in a shared vault is refused",
			args:       []string{"edit", "team:notes/team-guide.md", "--append", "x"},
			wantCode:   exitReadOnly,
			wantStderr: "vault: read-only",
		},
		{
			name:       "promote in a shared vault is refused",
			args:       []string{"promote", "inbox/proposal.md"},
			wantCode:   exitReadOnly,
			wantStderr: "vault: read-only",
		},
		{
			name:       "write names one vault",
			args:       []string{"write", "--vault", "personal", "--vault", "team", "--title", "X"},
			wantCode:   exitUsage,
			wantStderr: "name at most one --vault",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roots := setupVaults(t)
			got := runBrain(t, "", tc.args...)
			if got.code != tc.wantCode {
				t.Fatalf("exit %d, want %d; stderr %q", got.code, tc.wantCode, got.stderr)
			}
			if !strings.Contains(got.stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tc.wantStderr)
			}
			if !strings.Contains(got.stdout, tc.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", got.stdout, tc.wantOut)
			}
			if tc.wantCode == exitReadOnly {
				assertVaultUnchanged(t, roots["team"], twoVaults["team"])
			}
		})
	}
}

func TestVaultsJSONErrorCodes(t *testing.T) {
	tests := []struct {
		args     []string
		wantCode string
	}{
		{[]string{"read", "notes/shared-name.md"}, "ambiguous"},
		{[]string{"write", "--vault", "team", "--title", "X", "--body", "x"}, "read_only"},
	}
	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			setupVaults(t)
			got := runBrain(t, "", append(tc.args, "-o", "json")...)
			if env := decodeStrict[errorEnvelope](t, got.stdout); env.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}

// TestVaultsReindexOnlyTheChangedVault proves each vault keeps its own
// fingerprint: a change in one vault rebuilds that vault's index alone.
func TestVaultsReindexOnlyTheChangedVault(t *testing.T) {
	roots := setupVaults(t)
	if got := runBrain(t, "", "index"); got.code != exitOK ||
		!strings.Contains(got.stdout, "in personal") || !strings.Contains(got.stdout, "in team") {
		t.Fatalf("first index: exit %d, stdout %q", got.code, got.stdout)
	}
	teamState := stateFile(t, "team")
	before, err := os.ReadFile(teamState)
	if err != nil {
		t.Fatal(err)
	}

	note := filepath.Join(roots["personal"], "notes", "new.md")
	if err := os.WriteFile(note, []byte("---\ntitle: New\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runBrain(t, "", "index")
	if got.code != exitOK || got.stdout != "indexed 4 file(s) in personal (literal)\n" {
		t.Fatalf("second index: exit %d, stdout %q", got.code, got.stdout)
	}
	after, err := os.ReadFile(teamState)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("team state changed although the team vault did not")
	}
}

func stateFile(t *testing.T, vaultName string) string {
	t.Helper()
	dir, err := brain.IndexDir(vaultName)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "state.json")
}

// assertVaultUnchanged fails when root holds anything but notes, as written.
func assertVaultUnchanged(t *testing.T, root string, notes map[string]string) {
	t.Helper()
	seen := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		want, ok := notes[filepath.ToSlash(rel)]
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if !ok || string(data) != want {
			t.Errorf("shared vault changed at %s", rel)
		}
		seen++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != len(notes) {
		t.Errorf("shared vault holds %d files, want %d", seen, len(notes))
	}
}

func TestVaultsMigrateIDs(t *testing.T) {
	roots := setupVaults(t)
	if got := runBrain(t, "", "migrate"); got.code != exitUsage {
		t.Errorf("migrate without --ids: exit %d, want %d", got.code, exitUsage)
	}

	dry := decodeStrict[migrateResult](
		t,
		runBrain(t, "", "migrate", "--ids", "--dry-run", "-o", "json").stdout,
	)
	run := decodeStrict[migrateResult](t, runBrain(t, "", "migrate", "--ids", "-o", "json").stdout)
	if !dry.DryRun || len(dry.Changed) != 3 || len(run.Changed) != 3 {
		t.Fatalf("dry run %+v, run %+v; want three personal notes each", dry, run)
	}
	if len(run.Skipped) != 1 || run.Skipped[0].Vault != "team" || run.Skipped[0].Path != "" {
		t.Errorf("skipped = %+v, want the team vault", run.Skipped)
	}
	assertVaultUnchanged(t, roots["team"], twoVaults["team"])

	got := runBrain(t, "", "read", "id:"+run.Changed[0].ID)
	if got.code != exitOK || !strings.Contains(got.stdout, "id: "+run.Changed[0].ID) {
		t.Errorf("read by id: exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}

	again := runBrain(t, "", "migrate", "--ids")
	if again.code != exitOK || !strings.HasSuffix(again.stdout, "assigned 0 id(s)\n") {
		t.Errorf("second run: exit %d, stdout %q", again.code, again.stdout)
	}
	if got := runBrain(t, "", "migrate", "--ids", "--vault", "team"); got.code != exitReadOnly {
		t.Errorf("migrate a shared vault: exit %d, want %d", got.code, exitReadOnly)
	}
	if got := runBrain(t, "", "migrate", "--ids", "--vault", "team", "--dry-run"); got.code != exitOK ||
		!strings.Contains(got.stdout, "would assign 3 id(s)") {
		t.Errorf("dry run on a shared vault: exit %d, stdout %q", got.code, got.stdout)
	}
	assertVaultUnchanged(t, roots["team"], twoVaults["team"])
}

func TestVaultsEditKeepsTheNoteID(t *testing.T) {
	setupVaults(t)
	const ref = "personal:notes/mine.md"
	if got := runBrain(t, "", "edit", ref, "--set", "id=01JFIRSTFIRSTFIRSTFIRSTFIR"); got.code != exitOK {
		t.Fatalf("giving a note its first id: exit %d, stderr %q", got.code, got.stderr)
	}
	got := runBrain(t, "", "edit", ref, "--set", "id=01JOTHEROTHEROTHEROTHEROTH")
	if got.code != exitUsage ||
		!strings.Contains(got.stderr, "keeps id 01JFIRSTFIRSTFIRSTFIRSTFIR") {
		t.Errorf("changing the id: exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestVaultsGraph(t *testing.T) {
	setupVaults(t)
	got := runBrain(t, "", "graph", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	g := decodeStrict[vault.Graph](t, got.stdout)
	want := vault.GraphEdge{
		Source:   "personal:notes/mine.md",
		Target:   "team:notes/team-guide.md",
		Kind:     vault.EdgeLink,
		Resolved: true,
	}
	if len(g.Edges) != 1 || g.Edges[0] != want || len(g.Nodes) != 6 {
		t.Errorf("graph = %+v, want six notes and the one cross-vault link", g)
	}

	text := runBrain(t, "", "graph", "--center", "notes/team-guide.md")
	wantText := "personal:notes/mine.md -> team:notes/team-guide.md\n2 node(s), 1 edge(s)\n"
	if text.code != exitOK || text.stdout != wantText {
		t.Errorf("graph --center: exit %d, stdout %q, want %q", text.code, text.stdout, wantText)
	}
}

func TestVaultsEditIfMatch(t *testing.T) {
	setupVaults(t)
	const ref = "personal:notes/mine.md"
	read := decodeStrict[vault.Note](t, runBrain(t, "", "read", ref, "-o", "json").stdout)
	if len(read.SHA) != 64 {
		t.Fatalf("read sha = %q, want a sha256", read.SHA)
	}
	got := runBrain(t, "", "edit", ref, "--append", "one", "--if-match", read.SHA, "-o", "json")
	edited := decodeStrict[vault.PathResult](t, got.stdout)
	if got.code != exitOK || edited.SHA == "" || edited.SHA == read.SHA {
		t.Fatalf(
			"edit with the read sha: exit %d, result %+v, stderr %q",
			got.code,
			edited,
			got.stderr,
		)
	}

	got = runBrain(t, "", "edit", ref, "--append", "two", "--if-match", read.SHA)
	if got.code != exitConflict || !strings.Contains(got.stderr, "read it again") {
		t.Errorf("stale edit: exit %d, stderr %q; want %d", got.code, got.stderr, exitConflict)
	}
	env := decodeStrict[errorEnvelope](
		t,
		runBrain(
			t,
			"",
			"edit",
			ref,
			"--append",
			"two",
			"--if-match",
			read.SHA,
			"-o",
			"json",
		).stdout,
	)
	if env.Error.Code != "conflict" {
		t.Errorf("code = %q, want conflict", env.Error.Code)
	}
	if after := decodeStrict[vault.Note](t, runBrain(t, "", "read", ref, "-o", "json").stdout); after.SHA != edited.SHA {
		t.Errorf("refused edits changed the note: sha %s, want %s", after.SHA, edited.SHA)
	}
}
