package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// setupVaultsCmd extends setupSharing (a local-only private vault and a
// shared clone with a remote) with a plain directory whose policy sets a
// rule, and a vault whose directory is gone.
func setupVaultsCmd(t *testing.T) (sharingEnv, string, string) {
	t.Helper()
	env := setupSharing(t, false)
	plain := t.TempDir()
	writeNote(t, plain, "notes/a.md", "---\ntitle: A\n---\na\n")
	writeNote(t, plain, "README.md", "not a tier note\n")
	writeNote(t, plain, ".brain/policy.yaml", "rules:\n  link-dangling: error\n  provenance: off\n")
	gone := filepath.Join(t.TempDir(), "gone")
	cfg := "vaults:\n" +
		"  - {name: personal, path: " + env.private + "}\n" +
		"  - {name: team, path: " + env.shared + ", shared: true}\n" +
		"  - {name: plain, path: " + plain + "}\n" +
		"  - {name: gone, path: " + gone + "}\n"
	path := filepath.Join(os.Getenv("HOME"), ".config", "brain", "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return env, plain, gone
}

func TestVaultsCommandDescribesEveryVault(t *testing.T) {
	env, plain, gone := setupVaultsCmd(t)
	got := runBrain(t, "", "vaults", "-o", "json")
	if got.code != exitOK {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	report := decodeStrict[vaultsReport](t, got.stdout)
	var names []string
	for _, v := range report.Vaults {
		names = append(names, v.Name)
	}
	if !slices.Equal(names, []string{"personal", "team", "plain", "gone"}) {
		t.Fatalf("vaults = %q, want config order", names)
	}
	personal, team, pl, gn := report.Vaults[0], report.Vaults[1], report.Vaults[2], report.Vaults[3]
	if personal.Path != env.private || personal.Shared || !personal.Default ||
		personal.Git != gitLocalOnly || personal.Remote != "" || !personal.AcceptsAny ||
		personal.Policy != nil || personal.Notes != 3 || personal.Error != "" {
		t.Errorf("personal = %+v", personal)
	}
	if !team.Shared || team.Default || team.Git != gitRemote || team.Remote != env.sharedRemote ||
		team.AcceptsAny || !slices.Equal(team.Accepts, []string{"public", "internal"}) ||
		team.Policy == nil || len(team.Policy.Rules) != 0 || team.Notes != 1 {
		t.Errorf("team = %+v, policy %+v", team, team.Policy)
	}
	if pl.Path != plain || pl.Git != gitNone || !pl.AcceptsAny || pl.Notes != 1 ||
		pl.Policy == nil ||
		pl.Policy.Rules["link-dangling"] != "error" ||
		pl.Policy.Rules["provenance"] != "off" {
		t.Errorf("plain = %+v, policy %+v", pl, pl.Policy)
	}
	if gn.Path != gone || gn.Error != "the directory does not exist" {
		t.Errorf("gone = %+v", gn)
	}

	text := runBrain(t, "", "vaults")
	if text.code != exitOK {
		t.Fatalf("text: exit %d, stderr %q", text.code, text.stderr)
	}
	want := "personal: " + env.private + " (private, default write target), git local-only, " +
		"3 note(s), accepts any\n" +
		"team: " + env.shared + " (shared, read-only: notes reach it through promote --to), " +
		"git remote " + env.sharedRemote + ", 1 note(s), accepts public, internal\n" +
		"  policy .brain/policy.yaml: every rule at its default level\n" +
		"plain: " + plain + " (private), git none, 1 note(s), accepts any\n" +
		"  policy .brain/policy.yaml: rules link-dangling=error, provenance=off\n" +
		"gone: " + gone + " (private), error: the directory does not exist\n"
	if text.stdout != want {
		t.Errorf("text =\n%s\nwant\n%s", text.stdout, want)
	}
	if state := stateDirEntries(t); len(state) != 0 {
		t.Errorf("brain vaults wrote the state dir: %v", state)
	}
}

func TestVaultsCommandFiltersAndRefuses(t *testing.T) {
	setupVaultsCmd(t)
	got := runBrain(t, "", "vaults", "--vault", "team", "-o", "json")
	if report := decodeStrict[vaultsReport](t, got.stdout); got.code != exitOK ||
		len(report.Vaults) != 1 || report.Vaults[0].Name != "team" {
		t.Errorf("--vault team: exit %d, stdout %q", got.code, got.stdout)
	}
	if got := runBrain(t, "", "vaults", "--vault", "nope"); got.code != exitUsage {
		t.Errorf("--vault nope: exit %d, stderr %q", got.code, got.stderr)
	}
	if got := runBrain(t, "", "vaults", "extra"); got.code != exitUsage {
		t.Errorf("an argument: exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestVaultsCommandReportsAnUnusablePolicy(t *testing.T) {
	_, plain, _ := setupVaultsCmd(t)
	writeNote(t, plain, ".brain/policy.yaml", "classifications: [secretive]\n")
	got := runBrain(t, "", "vaults", "--vault", "plain")
	if got.code != exitOK || !strings.Contains(got.stdout,
		`policy .brain/policy.yaml: unusable: classification "secretive" is not one of`) {
		t.Errorf("exit %d, stdout %q", got.code, got.stdout)
	}
}

func TestMaskCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://ghp_token@github.com/example-org/brain.git": "https://xxxxx@github.com/example-org/brain.git",
		"https://user:secret@example.com/brain.git":          "https://xxxxx@example.com/brain.git", // suspenders:ignore (a test fixture)
		"https://github.com/example-org/brain.git":           "https://github.com/example-org/brain.git",
		"ssh://git@github.com/example-org/brain.git":         "ssh://git@github.com/example-org/brain.git",
		"ssh://git:pw@github.com/example-org/brain.git":      "ssh://git:xxxxx@github.com/example-org/brain.git", // suspenders:ignore (a test fixture)
		"git@github.com:example-org/brain.git":               "git@github.com:example-org/brain.git",
		"/tmp/remote.git":                                    "/tmp/remote.git",
	} {
		if got := maskCredentials(in); got != want {
			t.Errorf("maskCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}
