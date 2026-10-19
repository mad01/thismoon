package eval

import (
	"slices"
	"testing"
)

// TestUserExclusionsDeniesWhatTheArmsDoNotNeed: an entry whose command
// the arms' allow list needs stays unsandboxed and is reported; every
// other entry becomes a deny rule, in the user's order.
func TestUserExclusionsDeniesWhatTheArmsDoNotNeed(t *testing.T) {
	excluded := []string{"git:*", "pip3:*", "docker compose *", "make:*", "kill:*", "go"}
	deny, unsandboxed := userExclusions(excluded, bashTools())
	if want := []string{"Bash(pip3:*)", "Bash(docker compose *)", "Bash(kill:*)"}; !slices.Equal(
		deny, want,
	) {
		t.Errorf("deny = %q, want %q", deny, want)
	}
	if want := []string{"git:*", "make:*", "go"}; !slices.Equal(unsandboxed, want) {
		t.Errorf("unsandboxed = %q, want %q", unsandboxed, want)
	}
	if deny, unsandboxed := userExclusions(nil, bashTools()); deny != nil || unsandboxed != nil {
		t.Errorf("userExclusions(nil) = %q, %q; want nothing", deny, unsandboxed)
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
