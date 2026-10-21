package gate

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
