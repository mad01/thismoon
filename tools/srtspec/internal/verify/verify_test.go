package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/access"
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

const eventsInfo = `kind: Component
metadata:
  name: events
spec:
  owner: mad01
  access:
    sandbox: srt
    workdir: ~/.local/share/events
    listen: 7430
    reads: [~/code/bin, workdir]
    writes: [workdir, temp]
`

const (
	githubManifest     = "sandbox: srt\nlabel: github-mcp\ntls: true\n"
	darkNotifyManifest = "sandbox: srt\nlabel: dark-notify\n"
)

// renderInfo renders a component manifest the way generate writes it.
func renderInfo(t *testing.T, yaml string) string {
	t.Helper()
	m, err := access.ParseServiceInfo([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return render(t, m)
}

// renderStandalone renders an access.yaml the way generate writes it.
func renderStandalone(t *testing.T, yaml string) string {
	t.Helper()
	m, err := access.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return render(t, m)
}

func render(t *testing.T, m access.Manifest) string {
	t.Helper()
	b, err := access.Render(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// find returns the result for path, failing the test when there is none.
func find(t *testing.T, results []Result, path string) Result {
	t.Helper()
	for _, r := range results {
		if r.Path == path {
			return r
		}
	}
	t.Fatalf("no result for %s in %+v", path, results)
	return Result{}
}

func TestRunPasses(t *testing.T) {
	root := tree(t, map[string]string{
		"services/events/service-info.yaml": eventsInfo,
		"recipes/events/events.srt.json":    renderInfo(t, eventsInfo),
		"services/csl/service-info.yaml":    "kind: Component\nmetadata: {name: csl}\nspec:\n  owner: mad01\n  access: {sandbox: none, reason: ssh}\n",
		"recipes/claude-mcp/access.yaml":    githubManifest,
		"recipes/claude-mcp/github-mcp.srt.json": renderStandalone(
			t,
			githubManifest,
		),
		"recipes/claude-mcp/github-mcp.local.json": `{"filesystem": {"allowRead": ["~/notes"]}}`,
		"recipes/themes/dark-notify/access.yaml":   darkNotifyManifest,
		"recipes/themes/dark-notify/dark-notify.srt.json": renderStandalone(
			t,
			darkNotifyManifest,
		),
		"recipes/speak-tts/access.yaml":                               "sandbox: seatbelt\nreason: Metal\n",
		"recipes/speak/speak-web.local.json":                          `{"network": {"allowedDomains": ["share.example.net"]}}`,
		"recipes/speak/testdata/srt-local/refuse-deny-key.local.json": `{"filesystem": {"denyRead": []}}`,
	})
	results, err := Run(root, "/home/x")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"recipes/claude-mcp/github-mcp.srt.json":          "renders from recipes/claude-mcp/access.yaml",
		"recipes/themes/dark-notify/dark-notify.srt.json": "renders from recipes/themes/dark-notify/access.yaml",
		"recipes/events/events.srt.json":                  "renders from services/events/service-info.yaml",
		"recipes/claude-mcp/github-mcp.local.json":        "only widens recipes/claude-mcp/github-mcp.srt.json",
		"recipes/speak/speak-web.local.json":              "only widens; no speak-web.srt.json beside it, so carve-backs of its denies are unchecked",
	}
	if len(results) != len(want) {
		t.Fatalf("Run() = %d results %+v, want %d", len(results), results, len(want))
	}
	for path, note := range want {
		r := find(t, results, path)
		if !r.OK() || r.Note != note {
			t.Errorf(
				"%s: OK=%v note=%q reason=%q, want note %q",
				path,
				r.OK(),
				r.Note,
				r.Reason,
				note,
			)
		}
	}
}

func TestRunFindings(t *testing.T) {
	good := map[string]string{
		"services/events/service-info.yaml": eventsInfo,
		"recipes/events/events.srt.json":    renderInfo(t, eventsInfo),
	}
	with := func(extra map[string]string) map[string]string {
		files := map[string]string{}
		for k, v := range good {
			files[k] = v
		}
		for k, v := range extra {
			files[k] = v
		}
		return files
	}
	tests := []struct {
		name  string
		files map[string]string
		path  string
		want  string
	}{
		{
			name:  "settings file missing",
			files: with(map[string]string{"recipes/events/events.srt.json": ""}),
			path:  "recipes/events/events.srt.json",
			want:  "is not a render",
		},
		{
			name: "settings file never rendered",
			files: map[string]string{
				"services/events/service-info.yaml": eventsInfo,
				"recipes/events/":                   "",
			},
			path: "recipes/events/events.srt.json",
			want: "does not exist; services/events/service-info.yaml renders it",
		},
		{
			name: "same meaning, hand-edited layout",
			files: with(
				map[string]string{
					"recipes/events/events.srt.json": renderInfo(t, eventsInfo) + "\n",
				},
			),
			path: "recipes/events/events.srt.json",
			want: "in meaning but not in layout",
		},
		{
			name: "different meaning",
			files: with(
				map[string]string{
					"recipes/events/events.srt.json": `{"network": {"allowedDomains": ["evil.example"]}}`,
				},
			),
			path: "recipes/events/events.srt.json",
			want: "drifted from services/events/service-info.yaml",
		},
		{
			name:  "settings file without a manifest",
			files: with(map[string]string{"recipes/events/old.srt.json": "{}"}),
			path:  "recipes/events/old.srt.json",
			want:  "no manifest renders it",
		},
		{
			name: "layer that narrows",
			files: with(
				map[string]string{
					"recipes/events/events.local.json": `{"filesystem": {"denyRead": []}}`,
				},
			),
			path: "recipes/events/events.local.json",
			want: "srt-run would refuse it: filesystem.denyRead is not allowed",
		},
		{
			name: "layer that carves back a deny of the base beside it",
			files: with(map[string]string{
				"recipes/x/x.srt.json":   `{"filesystem": {"denyRead": ["~/secret"]}}`,
				"recipes/x/x.local.json": `{"filesystem": {"allowRead": ["~/secret"]}}`,
			}),
			path: "recipes/x/x.local.json",
			want: "would remove the deny on ~/secret",
		},
		{
			name:  "layer that is not JSON",
			files: with(map[string]string{"recipes/events/events.local.json": "nope"}),
			path:  "recipes/events/events.local.json",
			want:  "is not a JSON object",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := Run(tree(t, tt.files), "/home/x")
			if err != nil {
				t.Fatal(err)
			}
			r := find(t, results, tt.path)
			if r.OK() || !strings.Contains(r.Reason, tt.want) {
				t.Fatalf("%s: reason %q, want containing %q", tt.path, r.Reason, tt.want)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "nothing to verify",
			files: map[string]string{"recipes/x/recipe.toml": ""},
			want:  "nothing under",
		},
		{
			name:  "invalid manifest",
			files: map[string]string{"recipes/x/access.yaml": "sandbox: none\n"},
			want:  "sandbox none needs a reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(tree(t, tt.files), "/home/x")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() = %v, want containing %q", err, tt.want)
			}
		})
	}
}
