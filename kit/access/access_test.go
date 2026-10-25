package access

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	srt := func(edit func(*Manifest)) Manifest {
		m := Manifest{
			Sandbox: SandboxSRT,
			Workdir: "~/.local/share/svc",
			Reads:   []string{"~/code/bin", SymbolWorkdir},
			Writes:  []string{SymbolWorkdir, SymbolTemp},
		}
		edit(&m)
		return m
	}
	tests := []struct {
		name    string
		m       Manifest
		wantErr string // substring; empty means valid
	}{
		{name: "minimal srt", m: srt(func(*Manifest) {})},
		{
			name: "none with reason",
			m:    Manifest{Sandbox: SandboxNone, Reason: "root daemon"},
		},
		{name: "missing sandbox", m: Manifest{}, wantErr: "sandbox is required"},
		{
			name:    "unknown sandbox",
			m:       Manifest{Sandbox: "docker"},
			wantErr: `unknown sandbox "docker"`,
		},
		{
			name:    "none without reason",
			m:       Manifest{Sandbox: SandboxNone},
			wantErr: "sandbox none needs a reason",
		},
		{
			name:    "seatbelt without reason",
			m:       Manifest{Sandbox: SandboxSeatbelt, Reason: "  "},
			wantErr: "sandbox seatbelt needs a reason",
		},
		{
			name:    "label with a space",
			m:       srt(func(m *Manifest) { m.Label = "my svc" }),
			wantErr: "label",
		},
		{
			name:    "relative read",
			m:       srt(func(m *Manifest) { m.Reads = []string{"code/bin"} }),
			wantErr: `reads "code/bin": must start with / or ~/`,
		},
		{
			name:    "home read lifts the deny",
			m:       srt(func(m *Manifest) { m.Reads = []string{"~"} }),
			wantErr: "would lift the ~ read deny",
		},
		{
			name: "workdir symbol without workdir",
			m: srt(func(m *Manifest) {
				m.Workdir = ""
			}),
			wantErr: "uses workdir, but workdir is not set",
		},
		{
			name:    "workdir glob",
			m:       srt(func(m *Manifest) { m.Workdir = "~/.local/share/*" }),
			wantErr: "no globs",
		},
		{
			name:    "listen out of range",
			m:       srt(func(m *Manifest) { m.Listen = 70000 }),
			wantErr: "not a port",
		},
		{
			name:    "local peer with a port",
			m:       srt(func(m *Manifest) { m.Local = []string{"events:7430"} }),
			wantErr: "name a peer service",
		},
		{
			name: "egress hosts, wildcard subdomain, port and the launch token",
			m: srt(func(m *Manifest) {
				m.Egress = []string{"api.osv.dev", "*.hf.co", "example.com:8443", EgressAtLaunch}
			}),
		},
		{
			name:    "egress bare star",
			m:       srt(func(m *Manifest) { m.Egress = []string{"*"} }),
			wantErr: "a bare * allows every host",
		},
		{
			name:    "egress star with port",
			m:       srt(func(m *Manifest) { m.Egress = []string{"*:443"} }),
			wantErr: "a bare * allows every host",
		},
		{
			name:    "egress url",
			m:       srt(func(m *Manifest) { m.Egress = []string{"https://api.osv.dev"} }),
			wantErr: "port must be 1-65535",
		},
		{
			name:    "egress bad port",
			m:       srt(func(m *Manifest) { m.Egress = []string{"example.com:0"} }),
			wantErr: "port must be 1-65535",
		},
		{
			name:    "egress inner wildcard",
			m:       srt(func(m *Manifest) { m.Egress = []string{"api.*.com"} }),
			wantErr: "want host",
		},
		{
			name:    "write to the settings dir",
			m:       srt(func(m *Manifest) { m.Writes = []string{"~/.config/srt"} }),
			wantErr: "reaches ~/.config/srt",
		},
		{
			name:    "write glob over ~/.config",
			m:       srt(func(m *Manifest) { m.Writes = []string{"~/.config/**"} }),
			wantErr: "reaches ~/.config/srt",
		},
		{
			name:    "write to the ralph dir covers the sources cache",
			m:       srt(func(m *Manifest) { m.Writes = []string{"~/.config/ralph"} }),
			wantErr: "reaches ~/.config/ralph/sources",
		},
		{
			name: "write into a code root by glob",
			m: srt(func(m *Manifest) {
				m.Writes = []string{"~/code/**/service-info.yaml"}
			}),
		},
		{
			name: "deny under a reads root",
			m: srt(func(m *Manifest) {
				m.Reads = []string{"~/code"}
				m.Deny = []string{"~/code/**/.env", "~/code/**/*.pem"}
			}),
		},
		{
			name: "deny outside every reads root",
			m: srt(func(m *Manifest) {
				m.Deny = []string{"~/workspace/**/.env"}
			}),
			wantErr: "outside every reads root",
		},
		{
			name: "deny outside home",
			m: srt(func(m *Manifest) {
				m.Deny = []string{"/Volumes/secrets"}
			}),
		},
		{
			name:    "deny relative",
			m:       srt(func(m *Manifest) { m.Deny = []string{"**/.env"} }),
			wantErr: "must start with / or ~/",
		},
		{
			name: "reports every problem",
			m: Manifest{
				Sandbox: SandboxSRT,
				Egress:  []string{"*"},
				Writes:  []string{"~/.config/srt"},
			},
			wantErr: "name the hosts; writes \"~/.config/srt\" reaches",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	_, err := Parse([]byte("sandbox: srt\negres: [api.osv.dev]\n"))
	if err == nil || !strings.Contains(err.Error(), `unknown field "egres"`) {
		t.Fatalf("Parse() = %v, want unknown field error", err)
	}
}

func TestParseServiceInfo(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantLabel string
		wantErr   error
		wantMsg   string
	}{
		{
			name: "component with access",
			yaml: `kind: Component
metadata:
  name: events
spec:
  owner: mad01
  access:
    sandbox: srt
    listen: 7430
`,
			wantLabel: "events",
		},
		{
			name: "label overrides the name",
			yaml: `kind: Component
metadata: {name: catalog}
spec:
  access: {sandbox: srt, label: catalog-web}
`,
			wantLabel: "catalog-web",
		},
		{
			name: "system and component in one file",
			yaml: `kind: System
metadata: {name: tool}
spec: {owner: mad01}
---
kind: Component
metadata: {name: tool-cli}
spec:
  access: {sandbox: none, reason: CLI only}
`,
			wantLabel: "tool-cli",
		},
		{
			name:    "no access block",
			yaml:    "kind: Component\nmetadata: {name: x}\nspec: {owner: mad01}\n",
			wantErr: ErrNoManifest,
		},
		{
			name:    "access on a system",
			yaml:    "kind: System\nmetadata: {name: x}\nspec:\n  access: {sandbox: srt}\n",
			wantMsg: "belongs on a Component",
		},
		{
			name:    "invalid block",
			yaml:    "kind: Component\nmetadata: {name: x}\nspec:\n  access: {sandbox: none}\n",
			wantMsg: "needs a reason",
		},
		{
			name:    "strict inside the block",
			yaml:    "kind: Component\nmetadata: {name: x}\nspec:\n  access: {sandbox: srt, read: [~/code]}\n",
			wantMsg: `unknown field "read"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseServiceInfo([]byte(tt.yaml))
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.wantMsg != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantMsg)
				}
			default:
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				if m.Label != tt.wantLabel {
					t.Fatalf("label = %q, want %q", m.Label, tt.wantLabel)
				}
			}
		})
	}
}

func TestLoadStandaloneDefaultsLabelToDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "github-mcp")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, StandaloneFile)
	if err := os.WriteFile(path, []byte("sandbox: srt\ntls: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Label != "github-mcp" {
		t.Fatalf("label = %q, want github-mcp", m.Label)
	}
}

func TestLiteralPrefix(t *testing.T) {
	tests := map[string]string{
		"~/code":                   "~/code",
		"~/code/":                  "~/code",
		"~/code/**/.env":           "~/code",
		"~/code/**/service-info.y": "~/code",
		"~/.config/*":              "~/.config",
		"/private/tmp":             "/private/tmp",
		"~/a/b?/c":                 "~/a",
	}
	for in, want := range tests {
		if got := LiteralPrefix(in); got != want {
			t.Errorf("LiteralPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
