package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

// stubBuildInfo pins the linked-in build metadata for the duration of a test, so
// the output does not depend on the vcs stamps the test binary happens to carry.
func stubBuildInfo(t *testing.T) buildinfo.Info {
	t.Helper()
	want := buildinfo.Info{
		Version:   "abc1234",
		Commit:    "abc1234000000000000000000000000000000000",
		Tag:       "t-man/v0.1.0",
		BuildTime: "2026-08-13T09:00:00Z",
	}
	orig := buildinfo.Info{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		Tag:       buildinfo.Tag,
		BuildTime: buildinfo.BuildTime,
	}
	buildinfo.Version, buildinfo.Commit = want.Version, want.Commit
	buildinfo.Tag, buildinfo.BuildTime = want.Tag, want.BuildTime
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = orig.Version, orig.Commit
		buildinfo.Tag, buildinfo.BuildTime = orig.Tag, orig.BuildTime
	})
	return want
}

// runVersion executes the version command with the given flags and returns its
// trimmed output.
func runVersion(t *testing.T, args ...string) string {
	t.Helper()
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
	return strings.TrimSpace(buf.String())
}

// TestVersionCmd_JSONShape pins the cross-tool convention: `version -o json`
// must emit exactly the four build metadata keys so a single probe parses any
// sibling tool's build identity uniformly.
func TestVersionCmd_JSONShape(t *testing.T) {
	want := stubBuildInfo(t)

	out := runVersion(t, "-o", "json")
	var got buildinfo.Info
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %q: %v", out, err)
	}
	if got != want {
		t.Errorf("version -o json = %+v, want %+v", got, want)
	}

	var keys map[string]string
	if err := json.Unmarshal([]byte(out), &keys); err != nil {
		t.Fatalf("output is not a string map: %q: %v", out, err)
	}
	for _, k := range []string{"version", "commit", "tag", "build_time"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("key %q missing from %q", k, out)
		}
	}
	if len(keys) != 4 {
		t.Errorf("keys = %v, want exactly the four build metadata keys", keys)
	}
}

func TestVersionCmd_TextIsPlain(t *testing.T) {
	want := stubBuildInfo(t)

	out := runVersion(t, "-o", "text")
	if out != want.Version {
		t.Errorf("version = %q, want the bare token %q", out, want.Version)
	}
}
