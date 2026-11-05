package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
)

func TestVersionText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, nil, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	want := buildinfo.Get().Version
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("version text = %q, want the bare token %q", got, want)
	}
}

// TestVersionJSON pins the cross-tool build metadata contract: exactly the four
// keys, carrying the linked-in values.
func TestVersionJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "-o", "json"}, nil, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	var out map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %q (%v)", stdout.String(), err)
	}
	info := buildinfo.Get()
	want := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"tag":        info.Tag,
		"build_time": info.BuildTime,
	}
	if len(out) != len(want) {
		t.Errorf("json = %q, want exactly the keys %v", stdout.String(), want)
	}
	for k, w := range want {
		if out[k] != w {
			t.Errorf("json[%q] = %q, want %q", k, out[k], w)
		}
	}
}
