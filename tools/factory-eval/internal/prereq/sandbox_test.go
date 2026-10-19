package prereq

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoBuildCacheWantsAnAbsolutePath(t *testing.T) {
	tests := []struct {
		name   string
		env    Env
		ok     bool
		detail string
	}{
		{"absolute", Env{GoCache: "/Users/me/Library/Caches/go-build"}, true, "/Users/me/Library"},
		{"off", Env{GoCache: "off"}, false, `gave "off"`},
		{"empty", Env{}, false, `gave ""`},
		{
			"error",
			Env{GoCacheErr: errors.New("exec: go: not found")},
			false,
			"go env GOCACHE: exec",
		},
	}
	for _, tt := range tests {
		r := goBuildCache(tt.env)
		if r.OK != tt.ok || !strings.Contains(r.Detail, tt.detail) {
			t.Errorf("%s: goBuildCache() = %+v, want ok=%v and a detail holding %q",
				tt.name, r, tt.ok, tt.detail)
		}
	}
}

// TestSandboxExclusionsReadsTheUsersAndTheManagedList: no file means no
// exclusions, the user's list is read from the config dir, the managed
// file's entries are added, and a broken file fails the batch.
func TestSandboxExclusionsReadsTheUsersAndTheManagedList(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := t.TempDir()
	managed := filepath.Join(t.TempDir(), "managed-settings.json")
	t.Cleanup(func(old string) func() { return func() { managedSettings = old } }(managedSettings))
	managedSettings = managed
	if got, err := SandboxExclusions(home); err != nil || got != nil {
		t.Errorf("SandboxExclusions(no file) = %q, %v; want nil, nil", got, err)
	}
	p := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, p, `{"sandbox":{"enabled":false,"excludedCommands":["git:*","pip3:*"]}}`)
	got, err := SandboxExclusions(home)
	if err != nil || !slices.Equal(got, []string{"git:*", "pip3:*"}) {
		t.Errorf("SandboxExclusions() = %q, %v; want the two user entries", got, err)
	}
	writeFile(t, managed, `{"sandbox":{"excludedCommands":["docker:*"]}}`)
	got, err = SandboxExclusions(home)
	if err != nil || !slices.Equal(got, []string{"git:*", "pip3:*", "docker:*"}) {
		t.Errorf(
			"SandboxExclusions() = %q, %v; want the user entries, then the managed one",
			got,
			err,
		)
	}
	other := t.TempDir()
	writeFile(
		t,
		filepath.Join(other, "settings.json"),
		`{"sandbox":{"excludedCommands":["kubectl:*"]}}`,
	)
	t.Setenv("CLAUDE_CONFIG_DIR", other)
	got, err = SandboxExclusions(home)
	if err != nil || !slices.Equal(got, []string{"kubectl:*", "docker:*"}) {
		t.Errorf(
			"SandboxExclusions() with CLAUDE_CONFIG_DIR = %q, %v; want that dir's list",
			got,
			err,
		)
	}
	writeFile(t, filepath.Join(other, "settings.json"), `{"sandbox":`)
	if _, err := SandboxExclusions(home); err == nil {
		t.Error("SandboxExclusions(broken file) = nil error, want a parse error")
	}
}
