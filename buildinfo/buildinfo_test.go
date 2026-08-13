package buildinfo

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

// vcsBuildInfo builds a debug.BuildInfo like the toolchain stamps onto a binary
// built from a git checkout.
func vcsBuildInfo(module, revision, buildTime string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	bi.Main.Version = module
	if revision != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.revision", Value: revision})
	}
	if buildTime != "" {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: "vcs.time", Value: buildTime})
	}
	return bi
}

func TestFallback(t *testing.T) {
	const (
		sha     = "1234567890abcdef1234567890abcdef12345678"
		stamped = "2026-08-13T09:00:00Z"
	)
	tests := []struct {
		name string
		in   Info
		bi   *debug.BuildInfo
		want Info
	}{
		{
			name: "ldflags win over vcs stamps",
			in: Info{
				Version:   "abc1234",
				Commit:    "abc1234000000000000000000000000000000000",
				Tag:       "keep/v0.4.0",
				BuildTime: "2026-01-01T00:00:00Z",
			},
			bi: vcsBuildInfo("v9.9.9", sha, stamped),
			want: Info{
				Version:   "abc1234",
				Commit:    "abc1234000000000000000000000000000000000",
				Tag:       "keep/v0.4.0",
				BuildTime: "2026-01-01T00:00:00Z",
			},
		},
		{
			name: "go install build fills everything from the stamps",
			in:   Info{Version: devVersion},
			bi:   vcsBuildInfo("v0.4.0", sha, stamped),
			want: Info{Version: "1234567", Commit: sha, Tag: "v0.4.0", BuildTime: stamped},
		},
		{
			name: "devel module version is not a tag",
			in:   Info{Version: devVersion},
			bi:   vcsBuildInfo("(devel)", sha, stamped),
			want: Info{Version: "1234567", Commit: sha, Tag: "", BuildTime: stamped},
		},
		{
			name: "a commit-derived pseudo-version is not a tag",
			in:   Info{Version: devVersion},
			bi:   vcsBuildInfo("v0.0.0-20260813151542-218c16b64634+dirty", sha, stamped),
			want: Info{Version: "1234567", Commit: sha, Tag: "", BuildTime: stamped},
		},
		{
			name: "no stamps leaves the dev placeholder",
			in:   Info{Version: devVersion},
			bi:   vcsBuildInfo("(devel)", "", ""),
			want: Info{Version: devVersion},
		},
		{
			name: "a partly injected build keeps its own version",
			in:   Info{Version: "v1.2.3-abc1234"},
			bi:   vcsBuildInfo("(devel)", sha, stamped),
			want: Info{Version: "v1.2.3-abc1234", Commit: sha, BuildTime: stamped},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.fallback(tt.bi); got != tt.want {
				t.Errorf("fallback() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1234567890abcdef", "1234567"},
		{"1234567", "1234567"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := shortSHA(tt.in); got != tt.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestPrettyJSONKeys pins the cross-tool contract: exactly the four keys, always
// present, even when the values are unknown.
func TestPrettyJSONKeys(t *testing.T) {
	out := Info{}.PrettyJSON()
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("PrettyJSON() = %q, want a trailing newline", out)
	}
	if !strings.Contains(out, "\n  \"version\": \"\"") {
		t.Errorf("PrettyJSON() = %q, want 2-space indented fields", out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := []string{"version", "commit", "tag", "build_time"}
	if len(got) != len(want) {
		t.Errorf("keys = %v, want exactly %v", got, want)
	}
	for _, k := range want {
		v, ok := got[k]
		if !ok {
			t.Errorf("key %q missing from %q", k, out)
			continue
		}
		if v != "" {
			t.Errorf("key %q = %v, want the empty string", k, v)
		}
	}
}

func TestPrettyJSONValues(t *testing.T) {
	i := Info{
		Version:   "abc1234",
		Commit:    "abc1234000000000000000000000000000000000",
		Tag:       "keep/v0.4.0",
		BuildTime: "2026-08-13T09:00:00Z",
	}
	var got Info
	if err := json.Unmarshal([]byte(i.PrettyJSON()), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != i {
		t.Errorf("round trip = %+v, want %+v", got, i)
	}
}

// TestGetDefaults guards the zero-configuration path: `go test` links no
// ldflags, so Get falls back to the test binary's own vcs stamps and must still
// produce a non-empty version token.
func TestGetDefaults(t *testing.T) {
	if got := Get().Version; got == "" {
		t.Error("Get().Version is empty, want a version token")
	}
}

func TestIsReleaseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"v0.4.0", true},
		{"v1.2.3-rc1", true},
		{"v0.4.0+dirty", true},
		{"(devel)", false},
		{"", false},
		{"v0.0.0-20260813151542-218c16b64634", false},
		{"v0.0.0-20260813151542-218c16b64634+dirty", false},
		{"v1.2.3-0.20260813151542-218c16b64634", false},
	}
	for _, tt := range tests {
		if got := isReleaseVersion(tt.in); got != tt.want {
			t.Errorf("isReleaseVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
