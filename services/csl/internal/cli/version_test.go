package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

// pinBuildInfo injects known build metadata for the duration of the test, so
// the assertions don't depend on how the test binary was built.
func pinBuildInfo(t *testing.T) buildinfo.Info {
	t.Helper()
	want := buildinfo.Info{
		Version:   "abc1234",
		Commit:    "abc1234000000000000000000000000000000000",
		Tag:       "csl/v0.0.0",
		BuildTime: "2026-08-13T09:00:00Z",
	}
	orig := buildinfo.Info{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		Tag:       buildinfo.Tag,
		BuildTime: buildinfo.BuildTime,
	}
	buildinfo.Version, buildinfo.Commit, buildinfo.Tag, buildinfo.BuildTime =
		want.Version, want.Commit, want.Tag, want.BuildTime
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Tag, buildinfo.BuildTime =
			orig.Version, orig.Commit, orig.Tag, orig.BuildTime
	})
	return want
}

// runVersion executes the version command with the given flags and returns its
// trimmed output. versionOutput is package-level state cobra writes into and
// does not reset between Execute calls, so a run with no -o would otherwise
// inherit the previous test's format; the reset keeps the tests order-independent.
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
		versionOutput = "text"
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return strings.TrimSpace(buf.String())
}

// TestVersionCmd_JSONShape pins the cross-tool convention: `version -o json`
// must emit the four-key build metadata object every sibling tool serves, so a
// single probe parses any of them uniformly.
func TestVersionCmd_JSONShape(t *testing.T) {
	info := pinBuildInfo(t)
	out := runVersion(t, "-o", "json")

	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %q: %v", out, err)
	}
	want := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"tag":        info.Tag,
		"build_time": info.BuildTime,
	}
	if len(got) != len(want) {
		t.Fatalf("got keys %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%q = %q, want %q", k, got[k], v)
		}
	}
}

// TestVersionCmd_DefaultIsText runs directly after the -o json case on purpose:
// cobra leaves versionOutput holding the last parsed value, so a bare `version`
// here is what catches that leak reaching a caller who passed no format at all.
func TestVersionCmd_DefaultIsText(t *testing.T) {
	info := pinBuildInfo(t)
	out := runVersion(t)

	if out != info.Version {
		t.Fatalf("default output = %q, want the bare token %q", out, info.Version)
	}
}

// TestVersionCmd_TextIsPlain pins the bare token plain output: status and ralph
// parse it as a version, not as JSON.
func TestVersionCmd_TextIsPlain(t *testing.T) {
	info := pinBuildInfo(t)
	out := runVersion(t, "-o", "text")

	if out != info.Version {
		t.Fatalf("text output = %q, want the bare token %q", out, info.Version)
	}
}
