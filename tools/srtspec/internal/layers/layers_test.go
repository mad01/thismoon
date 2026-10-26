package layers

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtures is shared with the launch package's tests, so run and explain
// agree on what the expansion layer may do.
const fixtures = "../../../../recipes/speak/testdata/srt-local"

// install lays base.json and an optional layer into a scratch home as label
// svc, the way a machine would have them.
func install(t *testing.T, layer string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "srt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(fixtures, "base.json"), filepath.Join(dir, "svc.json"))
	if layer != "" {
		copyFile(t, filepath.Join(fixtures, layer), filepath.Join(dir, "svc.local.json"))
	}
	return home
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSharedFixtures(t *testing.T) {
	cases, err := filepath.Glob(filepath.Join(fixtures, "*.local.json"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no fixtures under %s: %v", fixtures, err)
	}
	for _, path := range cases {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			home := install(t, name)
			_, err := Resolve(Input{Home: home, Label: "svc"})
			switch {
			case strings.HasPrefix(name, "ok-"):
				if err != nil {
					t.Fatalf("Resolve() = %v, want the layer accepted", err)
				}
			case strings.HasPrefix(name, "refuse-"):
				if err == nil || !strings.Contains(err.Error(), "srt-run refuses to launch svc") {
					t.Fatalf("Resolve() = %v, want a refusal", err)
				}
			default:
				t.Fatalf("fixture %s must start with ok- or refuse-", name)
			}
		})
	}
}

func TestResolveAttributesLayers(t *testing.T) {
	home := install(t, "ok-widen.local.json")
	p, err := Resolve(
		Input{Home: home, Label: "svc", AllowDomains: "share.example.net,api.example.com,"},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Layer{}
	for _, e := range p.Entries {
		got[e.Key+" "+e.Value] = e.Layer
	}
	want := map[string]Layer{
		"filesystem.denyRead ~":                              Base,
		"filesystem.denyRead ~/code/**/.env":                 Base,
		"filesystem.allowRead ~/code":                        Base,
		"filesystem.allowRead ~/notes":                       Local,
		"filesystem.allowWrite ~/.cache/svc":                 Local,
		"filesystem.denyWrite ":                              Base,
		"network.allowedDomains api.example.com":             Local,
		"network.allowedDomains *.example.org":               Local,
		"network.allowedDomains share.example.net":           Env,
		"network.allowLocalBinding true":                     Local,
		"enableWeakerNetworkIsolation true":                  Local,
		"network.allowUnixSockets /private/var/run/svc.sock": Local,
	}
	for k, layer := range want {
		if got[k] != layer {
			t.Errorf("entry %q: layer %q, want %q", k, got[k], layer)
		}
	}
	if _, ok := got["network.allowedDomains "]; ok {
		t.Error("the empty-list placeholder survived an added host")
	}
	if p.LocalPath == "" {
		t.Error("LocalPath empty with a layer present")
	}
}

func TestResolveBaseOnly(t *testing.T) {
	home := install(t, "")
	p, err := Resolve(Input{Home: home, Label: "svc"})
	if err != nil {
		t.Fatal(err)
	}
	if p.LocalPath != "" {
		t.Errorf("LocalPath = %q, want none", p.LocalPath)
	}
	for _, e := range p.Entries {
		if e.Layer != Base {
			t.Errorf("entry %+v not from base", e)
		}
	}
}

func TestResolveRefusals(t *testing.T) {
	tests := []struct {
		name    string
		home    func(t *testing.T) string
		domains string
		wantErr string
	}{
		{
			name:    "missing settings fail closed",
			home:    func(t *testing.T) string { return t.TempDir() },
			wantErr: "srt fails closed",
		},
		{
			name:    "star in SRT_RUN_ALLOW_DOMAINS",
			home:    func(t *testing.T) string { return install(t, "") },
			domains: "ok.example.com,*:443",
			wantErr: "may not hold a bare * entry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(Input{Home: tt.home(t), Label: "svc", AllowDomains: tt.domains})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Resolve() = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCheckLocalNormalizesAbsoluteHomePaths(t *testing.T) {
	base := map[string]any{
		"filesystem": map[string]any{"denyRead": []any{"~", "~/code/**/.env"}},
	}
	tests := []struct {
		name  string
		local map[string]any
		want  string
	}{
		{
			name:  "absolute home",
			local: map[string]any{"filesystem": map[string]any{"allowRead": []any{"/Users/x/"}}},
			want:  "would lift the ~ deny",
		},
		{
			name: "absolute policy dir",
			local: map[string]any{
				"filesystem": map[string]any{"allowWrite": []any{"/Users/x/.config"}},
			},
			want: "reaches ~/.config/srt",
		},
		{
			name: "absolute deny carve-out",
			local: map[string]any{
				"filesystem": map[string]any{"allowRead": []any{"/Users/x/code/**/.env"}},
			},
			want: "would remove the deny on ~/code/**/.env",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckLocal(base, tt.local, "/Users/x")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("CheckLocal() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	home := install(t, "")
	p, err := Resolve(Input{Home: home, Label: "svc", AllowDomains: "share.example.net"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := p.Write(&buf, home); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"srt policy for svc",
		"base   ~/.config/srt/svc.json",
		"local  none",
		"SRT_RUN_ALLOW_DOMAINS=share.example.net",
		"filesystem.denyWrite",
		"(empty)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "share.example.net") || !strings.Contains(out, "env\n") {
		t.Errorf("env host not attributed:\n%s", out)
	}
}

func TestMergeWidens(t *testing.T) {
	base, err := ReadObject(filepath.Join(fixtures, "base.json"))
	if err != nil {
		t.Fatal(err)
	}
	local, err := ReadObject(filepath.Join(fixtures, "ok-widen.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := fmt.Sprint(base)
	got := Merge(base, local, []string{"share.example.net", "api.example.com"})
	if fmt.Sprint(base) != before {
		t.Fatal("Merge modified base")
	}
	want := map[string]any{
		"network.allowedDomains":       `["*.example.org","api.example.com","share.example.net"]`,
		"network.allowMachLookup":      `["com.apple.SystemConfiguration.configd"]`,
		"network.allowUnixSockets":     `["/private/var/run/svc.sock"]`,
		"network.allowLocalBinding":    "true",
		"enableWeakerNetworkIsolation": "true",
		"filesystem.allowRead":         `["~/code","~/notes"]`,
		"filesystem.allowWrite":        `["/private/tmp","/private/var/folders","~/.cache/svc","~/.local/share/svc"]`,
		"filesystem.denyRead":          `["~","~/code/**/.env"]`,
		"filesystem.denyWrite":         `[]`,
	}
	for key, w := range want {
		v, ok := lookup(got, strings.Split(key, ".")...)
		if !ok {
			t.Errorf("%s missing from the merge", key)
			continue
		}
		if display(v) != w {
			t.Errorf("%s = %s, want %s", key, display(v), w)
		}
	}
}

// Merge and Resolve are two views of one policy: every effective entry
// explain shows is in the merged settings, and nothing else is.
func TestMergeMatchesResolve(t *testing.T) {
	home := install(t, "ok-widen.local.json")
	hosts := "share.example.net,api.example.com"
	p, err := Resolve(Input{Home: home, Label: "svc", AllowDomains: hosts})
	if err != nil {
		t.Fatal(err)
	}
	base, _ := ReadObject(filepath.Join(home, ".config/srt/svc.json"))
	local, _ := ReadObject(filepath.Join(home, ".config/srt/svc.local.json"))
	split, _ := Hosts(hosts)
	merged := flatten("", Merge(base, local, split), Base)
	set := func(entries []Entry) map[string]bool {
		m := map[string]bool{}
		for _, e := range entries {
			m[e.Key+"="+e.Value] = true
		}
		return m
	}
	if want, got := set(p.Entries), set(merged); !maps.Equal(want, got) {
		t.Fatalf("merged settings %v\nexplain entries %v", got, want)
	}
}
