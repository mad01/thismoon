package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes body to a temp config file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// load is Load with the error treated as fatal, for the cases under test that
// are supposed to parse.
func load(t *testing.T, body string) Config {
	t.Helper()
	c, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

// TestRepoExcludedMatchesEveryDocumentedShape covers the three ways the
// reference says a pattern may name a repo: basename, org/repo identity, and a
// deeper path suffix. The suffix cases are the ones that used to match nothing
// at all, because patterns were tested against the absolute path only.
func TestRepoExcludedMatchesEveryDocumentedShape(t *testing.T) {
	c := load(t, `
exclude_repos = ["scratch-repo", "archive-*", "mad01/*", "*/archive/*"]
`)
	const root = "/Users/x/code/src/github.com"

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"basename literal", "/Users/x/code/scratch-repo", true},
		{"basename glob", "/Users/x/code/archive-old", true},
		{"org identity", root + "/mad01/thismoon", true},
		{"org identity, other org", root + "/someone-else/thismoon", false},
		{"path suffix", "/Users/x/code/archive/retired-tool", true},
		{"unmatched repo", root + "/other/tool", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.RepoExcluded(tt.path); got != tt.want {
				t.Errorf("RepoExcluded(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestRepoExcludedHonorsAbsolutePatterns pins the pre-existing behavior the
// suffix matching has to keep: a pattern written as a full path still matches,
// so a config that worked before the semantics changed still works.
func TestRepoExcludedHonorsAbsolutePatterns(t *testing.T) {
	c := load(t, `exclude_repos = ["/Users/x/code/vendored/*"]`)

	if !c.RepoExcluded("/Users/x/code/vendored/thing") {
		t.Error("an absolute pattern no longer matches the path it names")
	}
	if c.RepoExcluded("/Users/x/code/vendored/nested/thing") {
		t.Error("* matched across a /, which would exclude deeper repos by accident")
	}
}

// TestPathExcludedMatchesAtAnyDepth covers exclude_paths: the walker hands it
// repo-relative directory paths, and a pattern naming a directory has to hit
// wherever that directory sits.
func TestPathExcludedMatchesAtAnyDepth(t *testing.T) {
	c := load(t, `exclude_paths = ["third_party", "internal/gen/*"]`)

	for _, rel := range []string{"third_party", "pkg/third_party", "internal/gen/proto"} {
		if !c.PathExcluded(rel) {
			t.Errorf("PathExcluded(%q) = false, want true", rel)
		}
	}
	if c.PathExcluded("internal/store") {
		t.Error("PathExcluded matched a directory no pattern names")
	}
}

// TestLoadRejectsMalformedPattern is the silent-failure guard: a pattern that
// does not compile used to disable its own exclusion without a word.
func TestLoadRejectsMalformedPattern(t *testing.T) {
	_, err := Load(writeConfig(t, `exclude_repos = ["archive-old", "[unclosed"]`))
	if err == nil {
		t.Fatal("Load accepted a malformed glob")
	}
	for _, want := range []string{"exclude_repos", "[unclosed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestLoadMissingFileIsEmptyConfig pins the deliberate posture: no config file
// means no extra exclusions, not a failure.
func TestLoadMissingFileIsEmptyConfig(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatalf("Load(missing): %v", err)
	}
	if c.RepoExcluded("/Users/x/code/anything") || c.PathExcluded("vendor") {
		t.Error("an empty config excluded something")
	}
}

// TestLoadRejectsMalformedTOML keeps the parse error distinguishable from the
// missing-file case, which `deps config` reports differently.
func TestLoadRejectsMalformedTOML(t *testing.T) {
	if _, err := Load(writeConfig(t, "exclude_repos = \n")); err == nil {
		t.Fatal("Load accepted malformed TOML")
	}
}
