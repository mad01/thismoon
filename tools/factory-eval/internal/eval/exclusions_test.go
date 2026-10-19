package eval

import (
	"slices"
	"testing"
)

// TestUserExclusionsDeniesWhatTheArmsDoNotNeed: an entry that overlaps an
// arm allow rule, or names a wrapper word, stays unsandboxed and is
// reported; every other entry becomes a deny rule, in the user's order.
func TestUserExclusionsDeniesWhatTheArmsDoNotNeed(t *testing.T) {
	excluded := []string{
		"git:*", "git *", "git*", "*", "make:*", "go",
		"timeout:*", "nice *",
		"pip3:*", "docker compose *", "docker", "kill:*", "git fetch *", "go generate *",
		"/usr/bin/git *", "git-lfs:*",
	}
	deny, unsandboxed := userExclusions(excluded, bashTools())
	wantDeny := []string{
		"Bash(pip3:*)", "Bash(docker compose *)", "Bash(docker)", "Bash(kill:*)",
		"Bash(git fetch *)", "Bash(go generate *)", "Bash(/usr/bin/git *)", "Bash(git-lfs:*)",
	}
	if !slices.Equal(deny, wantDeny) {
		t.Errorf("deny = %q, want %q", deny, wantDeny)
	}
	wantUnsandboxed := []string{
		"git:*",
		"git *",
		"git*",
		"*",
		"make:*",
		"go",
		"timeout:*",
		"nice *",
	}
	if !slices.Equal(unsandboxed, wantUnsandboxed) {
		t.Errorf("unsandboxed = %q, want %q", unsandboxed, wantUnsandboxed)
	}
	if deny, unsandboxed := userExclusions(nil, bashTools()); deny != nil || unsandboxed != nil {
		t.Errorf("userExclusions(nil) = %q, %q; want nothing", deny, unsandboxed)
	}
	if got := Unsandboxed([]string{"git:*", "pip3:*"}); !slices.Equal(got, []string{"git:*"}) {
		t.Errorf("Unsandboxed() = %q, want git alone", got)
	}
}

func TestLiteralPrefixStopsAtTheWildcard(t *testing.T) {
	for pattern, want := range map[string]string{
		"git:*": "git", "git *": "git ", "git*": "git", "git -C * add *": "git -C ",
		"docker": "docker", "*": "", "make -C * test*": "make -C ",
	} {
		if got := literalPrefix(pattern); got != want {
			t.Errorf("literalPrefix(%q) = %q, want %q", pattern, got, want)
		}
	}
}

func TestArmPermissionsPutExclusionDeniesBeforeTheIsolationRules(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/thismoon"}
	got := armPermissions(ArmSingle, iso, []string{"git:*", "pip3:*"}).Deny
	want := append(disallowedTools(ArmSingle), "Bash(pip3:*)")
	want = append(want, iso.DenyRules()...)
	if !slices.Equal(got, want) {
		t.Errorf(
			"deny =\n%q\nwant the arm's list, the exclusion denies, then the isolation rules\n%q",
			got,
			want,
		)
	}
}

func TestWriteSettingsRefusesADisabledSandbox(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/thismoon"}
	s := Settings{Permissions: armPermissions(ArmSingle, iso, nil)}
	if _, err := WriteSettings(t.TempDir(), s); err == nil {
		t.Error("WriteSettings() with a zero Sandbox = nil error, want a refusal")
	}
}
