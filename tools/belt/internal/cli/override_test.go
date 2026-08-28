package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// runOverride executes `belt override <args...>` against a fresh command
// tree and returns its stdout.
func runOverride(t *testing.T, args ...string) string {
	t.Helper()
	cmd := overrideCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("override %v: %v", args, err)
	}
	return out.String()
}

func TestOverrideSetExtendClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	runOverride(t, "set", "vacation", "--for", "5m", "--reason", "testing the flow")
	o, ok := config.ReadOverride("vacation")
	if !ok {
		t.Fatal("set did not create the override file")
	}
	if o.Legacy || o.Malformed {
		t.Fatalf("set wrote a non-timed override: %+v", o)
	}
	remaining := o.Remaining(time.Now())
	if remaining <= 4*time.Minute || remaining > 5*time.Minute {
		t.Errorf("remaining after set --for 5m = %v, want ~5m", remaining)
	}

	runOverride(t, "extend", "vacation", "--for", "10m", "--reason", "still testing")
	extended, _ := config.ReadOverride("vacation")
	if !extended.Expiry.After(o.Expiry) {
		t.Errorf("extend did not move expiry forward: %v -> %v", o.Expiry, extended.Expiry)
	}

	list := runOverride(t)
	if !strings.Contains(list, "vacation") || !strings.Contains(list, "expires in") {
		t.Errorf("list output missing active override with remaining time: %q", list)
	}

	runOverride(t, "clear", "vacation")
	if _, ok := config.ReadOverride("vacation"); ok {
		t.Error("clear left the override file behind")
	}
	if got := runOverride(t); !strings.Contains(got, "no overrides set") {
		t.Errorf("list after clear = %q, want none", got)
	}
}

func TestOverrideExtendRequiresExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := overrideCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"extend", "nope", "--reason", "testing"})
	if err := cmd.Execute(); err == nil {
		t.Error("extend of an unset override should error")
	}
}

func TestOverrideRequiresReason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := [][]string{
		{"set", "vacation"},                     // flag missing entirely
		{"set", "vacation", "--reason", ""},     // empty
		{"set", "vacation", "--reason", "  \t"}, // whitespace-only
		{"extend", "vacation"},
		{"extend", "vacation", "--reason", " "},
	}
	for _, args := range cases {
		cmd := overrideCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("override %v should error without a non-blank --reason", args)
		}
	}
	if _, ok := config.ReadOverride("vacation"); ok {
		t.Error("a rejected set still wrote the override file")
	}
}

func TestOverrideListStates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := config.OverridesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("legacy", "")
	write("gone", time.Now().Add(-time.Hour).Format(time.RFC3339))
	write("broken", "not-a-timestamp")

	list := runOverride(t)
	for _, want := range []string{"untimed legacy", "expired", "MALFORMED"} {
		if !strings.Contains(list, want) {
			t.Errorf("list output missing %q state: %q", want, list)
		}
	}
}
