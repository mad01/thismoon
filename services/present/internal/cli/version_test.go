package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

func TestVersionText(t *testing.T) {
	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionOutput = "text"
	if err := versionCmd.RunE(versionCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	want := buildinfo.Get().Version
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Fatalf("version text = %q, want the bare token %q", got, want)
	}
}

// TestVersionJSON pins the cross-tool build metadata contract: exactly the four
// keys, carrying the linked-in values.
func TestVersionJSON(t *testing.T) {
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
	info := buildinfo.Get()
	want := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"tag":        info.Tag,
		"build_time": info.BuildTime,
	}
	if len(out) != len(want) {
		t.Errorf("json = %q, want exactly the keys %v", buf.String(), want)
	}
	for k, w := range want {
		if out[k] != w {
			t.Errorf("json[%q] = %q, want %q", k, out[k], w)
		}
	}
}
