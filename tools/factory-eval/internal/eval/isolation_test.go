package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
)

func TestDenyRulesCoverTheUsersCheckouts(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	got := iso.DenyRules()
	want := []string{
		"Edit(//Users/me/code/**)",
		"Edit(//Users/me/.worktrees/**)",
		"Bash(curl *)",
		"Bash(git -C /Users/me/code*)",
		"Bash(git -C ~/code*)",
		"Bash(git -C /Users/me/.worktrees*)",
		"Bash(git -C ~/.worktrees*)",
		"Bash(make -C /Users/me/code*)",
		"Bash(make -C ~/code*)",
		"Bash(make -C /Users/me/.worktrees*)",
		"Bash(make -C ~/.worktrees*)",
		"Bash(go build *-o /Users/me/code*)",
		"Bash(go build *-o ~/code*)",
		"Bash(go build *-o /Users/me/.worktrees*)",
		"Bash(go build *-o ~/.worktrees*)",
		"Bash(go build -C /Users/me/code*)",
		"Bash(go build -C ~/code*)",
		"Bash(go build -C /Users/me/.worktrees*)",
		"Bash(go build -C ~/.worktrees*)",
		"Bash(go test -C /Users/me/code*)",
		"Bash(go test -C ~/code*)",
		"Bash(go test -C /Users/me/.worktrees*)",
		"Bash(go test -C ~/.worktrees*)",
		"Bash(go vet -C /Users/me/code*)",
		"Bash(go vet -C ~/code*)",
		"Bash(go vet -C /Users/me/.worktrees*)",
		"Bash(go vet -C ~/.worktrees*)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("DenyRules() =\n%q\nwant\n%q", got, want)
	}
	for source, want := range map[string][]string{
		"/srv/thismoon": {
			"Edit(//srv/thismoon/**)", "Bash(git -C /srv/thismoon*)",
			"Bash(make -C /srv/thismoon*)", "Bash(go build *-o /srv/thismoon*)",
			"Bash(go build -C /srv/thismoon*)", "Bash(go test -C /srv/thismoon*)",
			"Bash(go vet -C /srv/thismoon*)",
		},
		"/Users/me/eval-source": {
			"Edit(//Users/me/eval-source/**)",
			"Bash(git -C /Users/me/eval-source*)", "Bash(git -C ~/eval-source*)",
			"Bash(make -C /Users/me/eval-source*)", "Bash(make -C ~/eval-source*)",
			"Bash(go build *-o /Users/me/eval-source*)", "Bash(go build *-o ~/eval-source*)",
			"Bash(go test -C /Users/me/eval-source*)", "Bash(go test -C ~/eval-source*)",
		},
	} {
		rules := Isolation{Home: "/Users/me", Source: source}.DenyRules()
		for _, rule := range want {
			if !slices.Contains(rules, rule) {
				t.Errorf("a source at %s gets no %s rule: %q", source, rule, rules)
			}
		}
	}
}

// TestDenyRulesHoldNoWriteRules guards against dead rules: Claude checks
// the Write tool against Edit rules and warns about Write path rules.
func TestDenyRulesHoldNoWriteRules(t *testing.T) {
	for _, rule := range (Isolation{Home: "/Users/me", Source: "/srv/thismoon"}).DenyRules() {
		if strings.HasPrefix(rule, "Write(") {
			t.Errorf("DenyRules() holds %s, which Claude never consults", rule)
		}
	}
}

func TestCoversRefusesTempDirsTheArmCouldNotEdit(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	for p, want := range map[string]bool{
		"/Users/me/code/tmp":             true,
		"/Users/me/.worktrees":           true,
		"/Users/me/codex":                false,
		"/tmp":                           false,
		"/var/folders/xy/T":              false,
		"/Users/me/.worktrees-elsewhere": false,
	} {
		if got := iso.Covers(p); got != want {
			t.Errorf("Covers(%s) = %v, want %v", p, got, want)
		}
	}
}

// TestSettingsFileCarriesEachArmsLists checks the written file per arm:
// the allow list from the command line, and the command-line deny list
// followed by the isolation rules. Claude Code documents settings-file
// rules as reaching subagents, and never says that of --allowedTools.
func TestSettingsFileCarriesEachArmsLists(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/thismoon"}
	tier := cases.Tier{Model: "sonnet", Effort: "medium", BudgetUSD: 10}
	for _, arm := range []string{ArmSingle, ArmFactory} {
		dir := t.TempDir()
		p, err := WriteSettings(dir, Settings{
			Permissions: armPermissions(arm, iso, nil),
			Sandbox:     armSandbox(dir, "/Users/me/Library/Caches/go-build"),
		})
		if err != nil {
			t.Fatalf("%s: WriteSettings() error = %v", arm, err)
		}
		if p != filepath.Join(dir, SettingsFileName) {
			t.Errorf("%s: path = %s, want it in the run dir", arm, p)
		}
		file := readSettings(t, p)
		got := file.Permissions
		args := armArgs(arm, tier, &checkout.Checkout{Root: dir}, p)
		allowFlag := strings.Split(args[slices.Index(args, "--allowedTools")+1], ",")
		denyFlag := strings.Split(args[slices.Index(args, "--disallowedTools")+1], ",")
		if !slices.Equal(got.Allow, allowFlag) {
			t.Errorf("%s: permissions.allow =\n%q\nwant the --allowedTools list\n%q",
				arm, got.Allow, allowFlag)
		}
		if want := append(denyFlag, iso.DenyRules()...); !slices.Equal(got.Deny, want) {
			t.Errorf("%s: permissions.deny =\n%q\nwant --disallowedTools, then DenyRules()\n%q",
				arm, got.Deny, want)
		}
		for _, tool := range []string{"Agent", "SendMessage"} {
			allowed, denied := slices.Contains(got.Allow, tool), slices.Contains(got.Deny, tool)
			if allowed != (arm == ArmFactory) || denied == allowed {
				t.Errorf("%s: %s allowed=%v denied=%v, want only the factory arm to have it",
					arm, tool, allowed, denied)
			}
		}
		if info, err := os.Stat(p); err != nil || info.Mode().Perm()&0o222 != 0 {
			t.Errorf("%s: settings file mode = %v, %v; want read-only", arm, info.Mode(), err)
		}
		sb := file.Sandbox
		if !sb.Enabled || !sb.FailIfUnavailable || sb.AllowUnsandboxedCommands ||
			sb.AutoAllowBashIfSandboxed {
			t.Errorf("%s: sandbox = %+v, want it on, strict, and with no unsandboxed retry",
				arm, sb)
		}
		if want := []string{dir, "/Users/me/Library/Caches/go-build"}; !slices.Equal(
			sb.Filesystem.AllowWrite, want,
		) {
			t.Errorf("%s: sandbox allowWrite = %q, want the run dir and the go cache %q",
				arm, sb.Filesystem.AllowWrite, want)
		}
	}
}

// readPermissions decodes the permissions block of the settings file at p.
func readPermissions(t *testing.T, p string) Permissions {
	t.Helper()
	return readSettings(t, p).Permissions
}

// readSettings decodes the settings file at p.
func readSettings(t *testing.T, p string) Settings {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("settings file %s = %s: %v", p, raw, err)
	}
	return s
}
