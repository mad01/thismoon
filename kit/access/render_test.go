package access

import (
	"strings"
	"testing"
)

func TestRenderGolden(t *testing.T) {
	m := Manifest{
		Sandbox: SandboxSRT,
		Workdir: "~/.local/state/moondb",
		Listen:  7433,
		Reads:   []string{"~/code", "~/.worktrees", SymbolWorkdir},
		Writes:  []string{SymbolWorkdir, SymbolTemp},
		Deny:    []string{"~/code/**/.env", "~/.worktrees/**/*.key"},
		TLS:     true,
		Egress:  []string{EgressAtLaunch, "api.osv.dev", "api.osv.dev"},
	}
	got, err := Render(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "enableWeakerNetworkIsolation": true,
  "filesystem": {
    "allowRead": [
      "~/code",
      "~/.worktrees",
      "~/.local/state/moondb"
    ],
    "allowWrite": [
      "~/.local/state/moondb",
      "/private/tmp",
      "/private/var/folders"
    ],
    "denyRead": [
      "~",
      "~/code/**/.env",
      "~/.worktrees/**/*.key"
    ],
    "denyWrite": []
  },
  "network": {
    "allowLocalBinding": true,
    "allowedDomains": [
      "api.osv.dev"
    ],
    "deniedDomains": []
  }
}
`
	if string(got) != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}
	again, err := Render(m)
	if err != nil || string(again) != string(got) {
		t.Fatalf("second Render() differs or failed: %v", err)
	}
}

func TestRenderDefaults(t *testing.T) {
	got, err := Render(Manifest{Sandbox: SandboxSRT})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "filesystem": {
    "allowRead": [],
    "allowWrite": [
      "/private/tmp",
      "/private/var/folders"
    ],
    "denyRead": [
      "~"
    ],
    "denyWrite": []
  },
  "network": {
    "allowedDomains": [],
    "deniedDomains": []
  }
}
`
	if string(got) != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderLocalPeersEnableLocalBinding(t *testing.T) {
	got, err := Render(Manifest{Sandbox: SandboxSRT, Local: []string{"events"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"allowLocalBinding": true`) {
		t.Fatalf("local peers did not enable allowLocalBinding:\n%s", got)
	}
}

func TestRenderExtra(t *testing.T) {
	base := Manifest{Sandbox: SandboxSRT, Reads: []string{"~/code/bin"}}
	tests := []struct {
		name    string
		extra   map[string]any
		want    []string // substrings of the output
		wantErr string
	}{
		{
			name:  "adds a key the fields cannot say",
			extra: map[string]any{"allowAppleEvents": true},
			want:  []string{`"allowAppleEvents": true`},
		},
		{
			name: "unions into a rendered list",
			extra: map[string]any{
				"network": map[string]any{"allowMachLookup": []any{"com.apple.x"}},
				"filesystem": map[string]any{
					"allowRead": []any{"~/code/bin", "~/notes"},
				},
			},
			want: []string{`"com.apple.x"`, "\"~/code/bin\",\n      \"~/notes\""},
		},
		{
			name: "cannot drop the home deny",
			extra: map[string]any{
				"filesystem": map[string]any{"denyRead": []any{}},
			},
			want: []string{"\"denyRead\": [\n      \"~\"\n    ]"},
		},
		{
			name: "a star host fails the invariants",
			extra: map[string]any{
				"network": map[string]any{"allowedDomains": []any{"*"}},
			},
			wantErr: "allows every host",
		},
		{
			name: "a policy write fails the invariants",
			extra: map[string]any{
				"filesystem": map[string]any{"allowWrite": []any{"~/.config/srt"}},
			},
			wantErr: "reaches ~/.config/srt",
		},
		{
			name:    "type clash",
			extra:   map[string]any{"filesystem": map[string]any{"allowRead": "~/x"}},
			wantErr: "extra.filesystem.allowRead: want a list",
		},
		{
			name:    "object over a scalar",
			extra:   map[string]any{"network": "off"},
			wantErr: "extra.network: want an object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			m.Extra = tt.extra
			got, err := Render(m)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Render() err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(string(got), w) {
					t.Errorf("output lacks %q:\n%s", w, got)
				}
			}
		})
	}
}

func TestRenderRefusesNonSRT(t *testing.T) {
	_, err := Render(Manifest{Sandbox: SandboxNone, Reason: "root daemon"})
	if err == nil || !strings.Contains(err.Error(), "only an srt manifest renders") {
		t.Fatalf("Render() = %v, want refusal", err)
	}
}

func TestRenderExtraFromYAML(t *testing.T) {
	m, err := Parse([]byte(`sandbox: srt
extra:
  allowAppleEvents: true
  network:
    allowUnixSockets: [/private/var/run/x.sock]
`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Render(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`"allowAppleEvents": true`, `"/private/var/run/x.sock"`} {
		if !strings.Contains(string(got), w) {
			t.Errorf("output lacks %q:\n%s", w, got)
		}
	}
}
