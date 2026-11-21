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
	orig := [4]string{buildinfo.Version, buildinfo.Commit, buildinfo.Tag, buildinfo.BuildTime}
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = orig[0], orig[1]
		buildinfo.Tag, buildinfo.BuildTime = orig[2], orig[3]
	})
	buildinfo.Version, buildinfo.Commit = version, commit
	buildinfo.Tag, buildinfo.BuildTime = tag, buildTime
}

// execute runs the root command with args and returns what it wrote to
// stdout.
func execute(t *testing.T, args ...string) string {
	t.Helper()
	c := root()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&bytes.Buffer{})
	c.SetArgs(args)
	if err := c.Execute(); err != nil {
		t.Fatalf("cards %s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

func TestVersionPrintsBareToken(t *testing.T) {
	setBuildInfo(t, "abc1234", "abc1234def5678", "cards/v0.1.0", "2026-11-21T10:00:00Z")

	if got := execute(t, "version"); got != "abc1234\n" {
		t.Errorf("cards version = %q, want %q", got, "abc1234\n")
	}
}

// TestVersionJSONShape pins the cross-tool convention: `version -o json`
// emits the shared four-key build metadata object.
func TestVersionJSONShape(t *testing.T) {
	setBuildInfo(t, "abc1234", "abc1234def5678", "cards/v0.1.0", "2026-11-21T10:00:00Z")

	var got map[string]string
	if err := json.Unmarshal([]byte(execute(t, "version", "-o", "json")), &got); err != nil {
		t.Fatalf("cards version -o json is not a JSON object of strings: %v", err)
	}
	want := map[string]string{
		"version":    "abc1234",
		"commit":     "abc1234def5678",
		"tag":        "cards/v0.1.0",
		"build_time": "2026-11-21T10:00:00Z",
	}
	if len(got) != len(want) {
		t.Errorf("cards version -o json keys = %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("cards version -o json %s = %q, want %q", k, got[k], v)
		}
	}
}

// TestDocsCoversProtocol checks that `cards docs` is the protocol reference:
// it shows every message type and both modes.
func TestDocsCoversProtocol(t *testing.T) {
	out := execute(t, "docs")
	for _, typ := range []string{"header", "card", "remove", "snapshot", "clear"} {
		if want := `"type":"` + typ + `"`; !strings.Contains(out, want) {
			t.Errorf("cards docs has no %s example", want)
		}
	}
	for _, mode := range []string{"cards list", "cards feed"} {
		if !strings.Contains(out, mode) {
			t.Errorf("cards docs does not show %q", mode)
		}
	}
}
