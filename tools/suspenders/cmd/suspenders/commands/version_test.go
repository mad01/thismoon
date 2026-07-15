package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/suspenders/internal/cli"
)

// TestVersionCmd_JSONShape pins the cross-tool convention: `version -o json`
// must emit exactly {"version":"<value>"} so a single probe parses any sibling
// tool's build identity uniformly.
func TestVersionCmd_JSONShape(t *testing.T) {
	orig := cli.Version
	cli.Version = "abc1234"
	defer func() { cli.Version = orig }()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "-o", "json"})
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %q: %v", out, err)
	}
	if got["version"] != "abc1234" {
		t.Fatalf(`expected version "abc1234", got %q`, got["version"])
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one key %q, got %v", "version", got)
	}
}

func TestVersionCmd_TextIsPlain(t *testing.T) {
	orig := cli.Version
	cli.Version = "abc1234"
	defer func() { cli.Version = orig }()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version", "-o", "text"})
	defer func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if strings.Contains(out, "{") {
		t.Fatalf("text output should not be JSON: %q", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Fatalf("text output should contain the version: %q", out)
	}
}
