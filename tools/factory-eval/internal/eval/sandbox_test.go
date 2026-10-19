package eval

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestArmSandboxWritesOnlyTheRootAndTheGoCache(t *testing.T) {
	sb := armSandbox("/tmp/factory-eval-1", "/Users/me/Library/Caches/go-build")
	if want := []string{"/tmp/factory-eval-1", "/Users/me/Library/Caches/go-build"}; !slices.Equal(
		sb.Filesystem.AllowWrite, want,
	) {
		t.Errorf("allowWrite = %q, want %q", sb.Filesystem.AllowWrite, want)
	}
	if got := armSandbox("/tmp/factory-eval-1", "").Filesystem.AllowWrite; !slices.Equal(
		got, []string{"/tmp/factory-eval-1"},
	) {
		t.Errorf("allowWrite with no go cache = %q, want the root alone", got)
	}
	if !sb.Enabled || !sb.FailIfUnavailable || sb.AutoAllowBashIfSandboxed ||
		sb.AllowUnsandboxedCommands {
		t.Errorf("sandbox flags = %+v, want enabled and fail-if-unavailable only", sb)
	}
	if n := sb.Network; !n.StrictAllowlist || !n.AllowLocalBinding || n.AllowAllUnixSockets {
		t.Errorf("network = %+v, want a strict allowlist, local binding, no unix sockets", n)
	}
}

// TestSandboxWritesEveryKeyExplicitly checks the JSON shape: a false that
// is left out would let the user's settings supply a true, and a missing
// excludedCommands would let their exclusions through.
func TestSandboxWritesEveryKeyExplicitly(t *testing.T) {
	raw, err := json.Marshal(armSandbox("/tmp/r", ""))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"enabled":                  "true",
		"autoAllowBashIfSandboxed": "false",
		"allowUnsandboxedCommands": "false",
		"failIfUnavailable":        "true",
		"excludedCommands":         "[]",
	} {
		if got := string(m[key]); got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}
	var net map[string]bool
	if err := json.Unmarshal(m["network"], &net); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]bool{
		"strictAllowlist": true, "allowLocalBinding": true, "allowAllUnixSockets": false,
	} {
		if got, ok := net[key]; !ok || got != want {
			t.Errorf("network.%s = %v (present %v), want %v", key, got, ok, want)
		}
	}
}
