package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionText(t *testing.T) {
	Version = "abc1234"
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionOutput = "text"
	if err := versionCmd.RunE(versionCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "abc1234" {
		t.Fatalf("version text = %q, want abc1234", got)
	}
}

func TestVersionJSON(t *testing.T) {
	Version = "abc1234"
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionOutput = "json"
	if err := versionCmd.RunE(versionCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %q (%v)", buf.String(), err)
	}
	if out["version"] != "abc1234" {
		t.Fatalf("json version = %q, want abc1234", out["version"])
	}
}
