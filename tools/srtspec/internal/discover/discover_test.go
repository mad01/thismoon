package discover

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tree writes files (path -> content) under a fresh root; a path ending in /
// makes a directory.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, content := range files {
		full := filepath.Join(root, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// comp returns a service-info.yaml for a Component with an access block.
func comp(name, block string) string {
	return "kind: Component\nmetadata: {name: " + name + "}\nspec:\n  owner: mad01\n  access: " + block + "\n"
}

func TestFind(t *testing.T) {
	root := tree(t, map[string]string{
		"services/events/service-info.yaml": comp("events", "{sandbox: srt, listen: 7430}"),
		"services/catalog/service-info.yaml": comp("catalog",
			"{sandbox: srt, label: catalog-web}"),
		"services/csl/service-info.yaml":         comp("csl", "{sandbox: none, reason: ssh}"),
		"tools/opener/service-info.yaml":         "kind: Component\nmetadata: {name: opener}\nspec: {owner: mad01}\n",
		"mytool/service-info.yaml":               comp("mytool", "{sandbox: srt}"),
		"recipes/events/":                        "",
		"recipes/catalog/":                       "",
		"recipes/mytool/":                        "",
		"recipes/claude-mcp/access.yaml":         "sandbox: srt\nlabel: github-mcp\ntls: true\n",
		"recipes/speak-tts/access.yaml":          "sandbox: seatbelt\nreason: Metal\n",
		"recipes/humanizer/recipe.toml":          "",
		"recipes/humanizer/humanizer.txt":        "",
		"recipes/themes/dark-notify/access.yaml": "sandbox: srt\n",
		"recipes/speak/testdata/x/access.yaml":   "sandbox: srt\n",
	})
	targets, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, tg := range targets {
		got[tg.Path] = tg.Output
	}
	want := map[string]string{
		"services/events/service-info.yaml":      "recipes/events/events.srt.json",
		"services/catalog/service-info.yaml":     "recipes/catalog/catalog-web.srt.json",
		"services/csl/service-info.yaml":         "",
		"mytool/service-info.yaml":               "recipes/mytool/mytool.srt.json",
		"recipes/claude-mcp/access.yaml":         "recipes/claude-mcp/github-mcp.srt.json",
		"recipes/speak-tts/access.yaml":          "",
		"recipes/themes/dark-notify/access.yaml": "recipes/themes/dark-notify/dark-notify.srt.json",
	}
	if len(got) != len(want) {
		t.Fatalf("Find() = %v, want %v", got, want)
	}
	for p, out := range want {
		if o, ok := got[p]; !ok || o != out {
			t.Errorf("%s -> %q (found %v), want %q", p, o, ok, out)
		}
	}
	paths := make([]string, 0, len(targets))
	for _, tg := range targets {
		paths = append(paths, tg.Path)
	}
	if !slices.IsSorted(paths) {
		t.Errorf("targets not sorted: %v", paths)
	}
}

func TestFindErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "component without a recipe dir",
			files: map[string]string{
				"services/events/service-info.yaml": comp("events", "{sandbox: srt}"),
			},
			want: "no recipe directory recipes/events",
		},
		{
			name: "two manifests, one output",
			files: map[string]string{
				"services/present/service-info.yaml": comp("present", "{sandbox: srt}"),
				"recipes/present/access.yaml":        "sandbox: srt\n",
			},
			want: "both render recipes/present/present.srt.json",
		},
		{
			name: "invalid manifest names its file",
			files: map[string]string{
				"recipes/x/access.yaml": "sandbox: none\n",
			},
			want: "recipes/x/access.yaml: access: sandbox none needs a reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Find(tree(t, tt.files))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Find() = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOrphans(t *testing.T) {
	root := tree(t, map[string]string{
		"services/events/service-info.yaml":        comp("events", "{sandbox: srt}"),
		"recipes/events/events.srt.json":           "{}",
		"recipes/events/old.srt.json":              "{}",
		"recipes/themes/dark-notify/old.srt.json":  "{}",
		"recipes/events/testdata/fixture.srt.json": "{}",
	})
	targets, err := Find(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Orphans(root, targets)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"recipes/events/old.srt.json", "recipes/themes/dark-notify/old.srt.json"}
	if !slices.Equal(got, want) {
		t.Fatalf("Orphans() = %v, want %v", got, want)
	}
}

func TestLayers(t *testing.T) {
	root := tree(t, map[string]string{
		"recipes/speak/speak-web.local.json":                   "{}",
		"recipes/themes/dark-notify/dark-notify.local.json":    "{}",
		"recipes/speak/testdata/srt-local/ok-widen.local.json": "{}",
		"recipes/speak/speak-web.srt.json":                     "{}",
	})
	got, err := Layers(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"recipes/speak/speak-web.local.json",
		"recipes/themes/dark-notify/dark-notify.local.json",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Layers() = %v, want %v", got, want)
	}
}
