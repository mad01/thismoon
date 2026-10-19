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
// exclusions, the user's list is read from the config dir given, the
// managed file's entries come next, then each drop-in's in name order, and
// a broken file fails the batch.
func TestSandboxExclusionsReadsTheUsersAndTheManagedList(t *testing.T) {
	configDir := t.TempDir()
	old := managedDir
	t.Cleanup(func() { managedDir = old })
	managedDir = t.TempDir()
	if got, err := SandboxExclusions(configDir); err != nil || got != nil {
		t.Errorf("SandboxExclusions(no file) = %q, %v; want nil, nil", got, err)
	}
	writeFile(t, filepath.Join(configDir, "settings.json"),
		`{"sandbox":{"enabled":false,"excludedCommands":["git:*","pip3:*"]}}`)
	got, err := SandboxExclusions(configDir)
	if err != nil || !slices.Equal(got, []string{"git:*", "pip3:*"}) {
		t.Errorf("SandboxExclusions() = %q, %v; want the two user entries", got, err)
	}
	writeFile(t, filepath.Join(managedDir, "managed-settings.json"),
		`{"sandbox":{"excludedCommands":["docker:*"]}}`)
	dropIns := filepath.Join(managedDir, "managed-settings.d")
	writeFile(t, filepath.Join(dropIns, "20-team.json"),
		`{"sandbox":{"excludedCommands":["kill:*"]}}`)
	writeFile(t, filepath.Join(dropIns, "10-base.json"),
		`{"sandbox":{"excludedCommands":["timeout:*"]}}`)
	writeFile(t, filepath.Join(dropIns, "notes.txt"), `{"sandbox":`)
	writeFile(t, filepath.Join(dropIns, "old.json", "x.json"), `{"sandbox":`)
	got, err = SandboxExclusions(configDir)
	want := []string{"git:*", "pip3:*", "docker:*", "timeout:*", "kill:*"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("SandboxExclusions() = %q, %v; want the user entries, the managed file's, "+
			"then the drop-ins' in name order: %q", got, err, want)
	}
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "settings.json"),
		`{"sandbox":{"excludedCommands":["kubectl:*"]}}`)
	got, err = SandboxExclusions(other)
	if err != nil || !slices.Equal(got, []string{"kubectl:*", "docker:*", "timeout:*", "kill:*"}) {
		t.Errorf("SandboxExclusions(other dir) = %q, %v; want that dir's list", got, err)
	}
	writeFile(t, filepath.Join(dropIns, "30-broken.json"), `{"sandbox":`)
	if _, err := SandboxExclusions(other); err == nil {
		t.Error("SandboxExclusions(broken drop-in) = nil error, want a parse error")
	}
}
