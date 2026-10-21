package gate

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

func TestGatesPassEmptyAndMatchLowercased(t *testing.T) {
	m := Machine{Host: "laptop", Profiles: map[string]bool{"work": true}}
	for _, c := range []struct {
		hosts []string
		want  bool
	}{
		{nil, true},
		{[]string{}, true},
		{[]string{"LapTop"}, true},
		{[]string{"desk", "laptop"}, true},
		{[]string{"desk"}, false},
	} {
		if got := m.HostAllowed(c.hosts); got != c.want {
			t.Errorf("HostAllowed(%q) = %t, want %t", c.hosts, got, c.want)
		}
	}
	for _, c := range []struct {
		profiles []string
		want     bool
	}{
		{nil, true},
		{[]string{"personal", "WORK"}, true},
		{[]string{"personal"}, false},
	} {
		if got := m.ProfileAllowed(c.profiles); got != c.want {
			t.Errorf("ProfileAllowed(%q) = %t, want %t", c.profiles, got, c.want)
		}
	}
}

// TestValuesIterateLikePython expects [str(v) for v in gate], or no gate for
// a falsy one; the scripts crashed on a value that does not iterate.
func TestValuesIterateLikePython(t *testing.T) {
	obj, err := pyjson.Loads([]byte(`{"a": 1, "b": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		raw  any
		want []string
	}{
		{"missing", nil, nil},
		{"false", false, nil},
		{"zero", 0.0, nil},
		{"empty string", "", nil},
		{"empty list", []any{}, nil},
		{"list", []any{"Work", 1.5, true, nil}, []string{"Work", "1.5", "True", "None"}},
		{"object", obj, []string{"a", "b"}},
		{"string", "ab", []string{"a", "b"}},
	} {
		got, ok := Values(c.raw)
		if !ok || !slices.Equal(got, c.want) {
			t.Errorf("%s: Values = %q, %t; want %q, true", c.name, got, ok, c.want)
		}
	}
	for _, raw := range []any{true, 5.0} {
		if got, ok := Values(raw); ok {
			t.Errorf("Values(%v) = %q, true; want not ok", raw, got)
		}
	}
}

// TestReadProfilesLikePython expects what the scripts' current_profiles
// returned: frozenset(str(p).lower() for p in profiles), or nothing for a
// file that is missing, malformed or has no profiles array.
func TestReadProfilesLikePython(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, toml string
		want       []string
	}{
		{"list", `profiles = ["Work", "lab"]`, []string{"work", "lab"}},
		{"non-strings", `profiles = [1, true, 1.5]`, []string{"1", "true", "1.5"}},
		{"not a list", `profiles = "work"`, nil},
		{"malformed", `profiles = ["work"`, nil},
		{"no key", `other = 1`, nil},
	} {
		path := filepath.Join(dir, c.name+".toml")
		if err := os.WriteFile(path, []byte(c.toml+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		want := map[string]bool{}
		for _, p := range c.want {
			want[p] = true
		}
		if got := readProfiles(path); !maps.Equal(got, want) {
			t.Errorf("%s: readProfiles = %v, want %v", c.name, got, want)
		}
	}
	if got := readProfiles(filepath.Join(dir, "absent.toml")); len(got) != 0 {
		t.Errorf("readProfiles(absent) = %v, want none", got)
	}
}

func TestCurrentLowercasesTheHost(t *testing.T) {
	m, err := Current(filepath.Join(t.TempDir(), "absent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if m.Host != strings.ToLower(host) || len(m.Profiles) != 0 {
		t.Errorf("Current = %+v, want host %q and no profiles", m, strings.ToLower(host))
	}
}
