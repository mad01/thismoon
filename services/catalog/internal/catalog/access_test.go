package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/access"
)

// repo writes files (path -> content) under a fresh repo root with a .git
// directory, and returns the root.
func repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p, content := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestNeedsAccess(t *testing.T) {
	root := repo(t, map[string]string{
		"services/web/service-info.yaml": "",
		"recipes/web/recipe.toml": `[hooks.builds.web_service]
commands = ["t-man add --name web -- $HOME/code/bin/web serve"]
`,
		"tools/mcpish/service-info.yaml": "",
		"recipes/mcpish/recipe.toml":     "[packages.mcpish]\n",
		"recipes/mcpish/mcpish-sandbox.sh": `#!/bin/sh
cd /
exec "$HOME/code/bin/srt-run" "$HOME/.config/srt/mcpish.json" "$HOME/code/bin/mcpish" mcp
`,
		"tools/quiet/service-info.yaml": "",
		"recipes/quiet/recipe.toml": `# A comment that mentions t-man add and srt-run launches nothing.
[packages.quiet]
`,
		"tools/norecipe/service-info.yaml": "",
	})
	tests := []struct {
		path    string
		kind    Kind
		wantWhy string // substring; "" means no access needed
	}{
		{path: "services/web/service-info.yaml", kind: KindComponent, wantWhy: `runs "t-man add"`},
		{
			path:    "tools/mcpish/service-info.yaml",
			kind:    KindComponent,
			wantWhy: `mcpish-sandbox.sh runs "srt-run"`,
		},
		{path: "tools/quiet/service-info.yaml", kind: KindComponent},
		{path: "tools/norecipe/service-info.yaml", kind: KindComponent},
		{path: "services/web/service-info.yaml", kind: KindSystem},
	}
	for _, tt := range tests {
		t.Run(tt.path+"/"+string(tt.kind), func(t *testing.T) {
			e := Entity{Kind: tt.kind, SourcePath: filepath.Join(root, tt.path)}
			why, err := NeedsAccess(e)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantWhy == "" {
				if why != "" {
					t.Fatalf("NeedsAccess() = %q, want none", why)
				}
				return
			}
			if !strings.Contains(why, tt.wantWhy) {
				t.Fatalf("NeedsAccess() = %q, want containing %q", why, tt.wantWhy)
			}
		})
	}
}

func TestRecipeDirStopsAtRepoRoot(t *testing.T) {
	outer := t.TempDir()
	// A recipes dir above the repo root must not be picked up.
	if err := os.MkdirAll(filepath.Join(outer, "recipes", "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if dir, ok := RecipeDir(filepath.Join(inner, "svc", "service-info.yaml")); ok {
		t.Fatalf("RecipeDir() = %q, want none past the repo root", dir)
	}
}

func TestCheckAccess(t *testing.T) {
	needs := func(e Entity) (string, error) {
		if e.Metadata.Name == "web" || e.Metadata.Name == "declared" {
			return "recipe runs t-man add", nil
		}
		return "", nil
	}
	declared := &access.Manifest{Sandbox: access.SandboxNone, Reason: "root daemon"}
	entities := []Entity{
		{
			Kind:       KindComponent,
			Metadata:   Metadata{Name: "web"},
			SourcePath: "/r/web/service-info.yaml",
		},
		{Kind: KindComponent, Metadata: Metadata{Name: "declared"}, Spec: Spec{Access: declared}},
		{Kind: KindComponent, Metadata: Metadata{Name: "cli"}},
	}
	err := CheckAccess(entities, needs)
	if err == nil || !strings.Contains(err.Error(), "1 component(s)") ||
		!strings.Contains(err.Error(), "web (/r/web/service-info.yaml): recipe runs t-man add") {
		t.Fatalf("CheckAccess() = %v", err)
	}
	if err := CheckAccess(entities[1:], needs); err != nil {
		t.Fatalf("CheckAccess() without the offender = %v", err)
	}

	boom := errors.New("boom")
	if err := CheckAccess(entities[:1], func(Entity) (string, error) { return "", boom }); !errors.Is(
		err,
		boom,
	) {
		t.Fatalf("CheckAccess() = %v, want the lookup error", err)
	}
}

func TestParseEntitiesAccess(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid block",
			yaml: `kind: Component
metadata: {name: events}
spec:
  owner: mad01
  system: thismoon
  access:
    sandbox: srt
    listen: 7430
`,
		},
		{
			name: "unknown field in the block",
			yaml: `kind: Component
metadata: {name: events}
spec:
  owner: mad01
  system: thismoon
  access: {sandbox: srt, egres: [x.example.com]}
`,
			wantErr: `unknown field "egres"`,
		},
		{
			name: "invalid block",
			yaml: `kind: Component
metadata: {name: csl}
spec:
  owner: mad01
  system: thismoon
  access: {sandbox: none}
`,
			wantErr: `component "csl": spec.access: access: sandbox none needs a reason`,
		},
		{
			name: "block on a system",
			yaml: `kind: System
metadata: {name: thismoon}
spec:
  owner: mad01
  access: {sandbox: srt}
`,
			wantErr: "spec.access belongs on a Component",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := ParseEntities([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if ents[0].Spec.Access == nil || ents[0].Spec.Access.Listen != 7430 {
					t.Fatalf("access not parsed: %+v", ents[0].Spec)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseEntities() = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
