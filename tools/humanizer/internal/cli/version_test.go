package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

// setBuildInfo pins the linker-injected build metadata for one test. Get()
// only falls back to the toolchain's vcs stamps for fields left empty, so a
// fully populated fixture keeps the assertions hermetic.
func setBuildInfo(t *testing.T, version, commit, tag, buildTime string) {
	t.Helper()
	orig := [4]string{
		buildinfo.Version,
		buildinfo.Commit,
		buildinfo.Tag,
		buildinfo.BuildTime,
	}
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = orig[0], orig[1]
		buildinfo.Tag, buildinfo.BuildTime = orig[2], orig[3]
	})
	buildinfo.Version, buildinfo.Commit = version, commit
	buildinfo.Tag, buildinfo.BuildTime = tag, buildTime
}

// runVersion executes the version command and returns what it wrote. The
// output flag binds to a package var that cobra does not reset between
// Execute calls, so the helper restores the default first — a real invocation
// is always a fresh process.
func runVersion(t *testing.T, args ...string) string {
	t.Helper()
	versionOutput = "text"
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"version"}, args...))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return buf.String()
}

// TestVersionCmdJSONShape pins the cross-tool convention: `version -o json`
// must emit the shared four-key build metadata object so a single probe parses
// any sibling tool's build identity uniformly.
func TestVersionCmdJSONShape(t *testing.T) {
	setBuildInfo(t, "abc1234", "abc1234def5678", "humanizer/v1.2.3", "2026-08-13T10:00:00Z")

	out := runVersion(t, "-o", "json")

	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %q: %v", out, err)
	}
	want := map[string]string{
		"version":    "abc1234",
		"commit":     "abc1234def5678",
		"tag":        "humanizer/v1.2.3",
		"build_time": "2026-08-13T10:00:00Z",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys %v, want %d", len(got), got, len(want))
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("key %q: got %q, want %q", k, got[k], w)
		}
	}
}

// TestVersionCmdTextIsBareToken pins the other half of the convention: plain
// output is the version and nothing else, so a probe can read the line as a
// version without stripping a tool name off it.
func TestVersionCmdTextIsBareToken(t *testing.T) {
	setBuildInfo(t, "abc1234", "abc1234def5678", "humanizer/v1.2.3", "2026-08-13T10:00:00Z")

	got := strings.TrimSpace(runVersion(t))

	if got != "abc1234" {
		t.Fatalf("text output: got %q, want the bare token %q", got, "abc1234")
	}
}
