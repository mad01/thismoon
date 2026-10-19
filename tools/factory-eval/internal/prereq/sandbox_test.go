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

func TestSandboxExclusionsReadsTheUsersList(t *testing.T) {
	home := t.TempDir()
	if got, err := SandboxExclusions(home); err != nil || got != nil {
		t.Errorf("SandboxExclusions(no file) = %q, %v; want nil, nil", got, err)
	}
	p := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, p, `{"sandbox":{"enabled":false,"excludedCommands":["git:*","pip3:*"]}}`)
	got, err := SandboxExclusions(home)
	if err != nil || !slices.Equal(got, []string{"git:*", "pip3:*"}) {
		t.Errorf("SandboxExclusions() = %q, %v; want the two entries", got, err)
	}
	writeFile(t, p, `{"sandbox":`)
	if _, err := SandboxExclusions(home); err == nil {
		t.Error("SandboxExclusions(broken file) = nil error, want a parse error")
	}
}
