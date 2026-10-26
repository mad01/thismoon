package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the root command with args and returns stdout and the error.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := root()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
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

func TestRenderAndCheck(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "service-info.yaml")
	write(t, manifest, eventsInfo)
	settings := filepath.Join(dir, "events.json")

	if _, err := run(t, "render", manifest, "-o", settings); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "check", manifest, settings)
	if err != nil || !strings.Contains(out, "ok: ") {
		t.Fatalf("check after render: %v\n%s", err, out)
	}

	write(t, settings, `{"network": {"allowedDomains": ["evil.example"], "deniedDomains": []},
"filesystem": {"denyRead": ["~"], "allowRead": [], "allowWrite": [], "denyWrite": []}}`)
	out, err = run(t, "check", manifest, settings)
	if err == nil || !strings.Contains(err.Error(), "drifted") {
		t.Fatalf("check on drift = %v", err)
	}
	if !strings.Contains(out, `-      "evil.example"`) {
		t.Errorf("diff does not show the drift:\n%s", out)
	}
}

func TestRenderNoManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service-info.yaml")
	write(t, path, "kind: Component\nmetadata: {name: x}\nspec: {owner: mad01}\n")
	_, err := run(t, "render", path)
	if err == nil || !strings.Contains(err.Error(), "declares no spec.access") {
		t.Fatalf("render = %v", err)
	}
}

func TestGenerate(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "services/events/service-info.yaml"), eventsInfo)
	write(t, filepath.Join(root, "recipes/events/recipe.toml"), "")
	out := filepath.Join(root, "recipes/events/events.srt.json")

	got, err := run(t, "generate", root)
	if err != nil || !strings.Contains(got, "wrote   recipes/events/events.srt.json") {
		t.Fatalf("first generate: %v\n%s", err, got)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err = run(t, "generate", root)
	if err != nil || !strings.Contains(got, "ok      recipes/events/events.srt.json") {
		t.Fatalf("second generate: %v\n%s", err, got)
	}
	second, _ := os.ReadFile(out)
	if !bytes.Equal(first, second) {
		t.Fatal("generate is not deterministic")
	}

	write(t, filepath.Join(root, "recipes/events/stale.srt.json"), "{}")
	if _, err := run(t, "generate", root); err == nil ||
		!strings.Contains(err.Error(), "stale.srt.json") {
		t.Fatalf("generate with an orphan = %v", err)
	}
}

func TestExplain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SRT_RUN_ALLOW_DOMAINS", "share.example.net")
	write(t, filepath.Join(home, ".config/srt/svc.json"), `{
  "network": {"allowedDomains": [], "deniedDomains": []},
  "filesystem": {"denyRead": ["~"], "allowRead": [], "allowWrite": [], "denyWrite": []}
}`)
	write(t, filepath.Join(home, ".config/srt/svc.local.json"),
		`{"filesystem": {"allowRead": ["~/notes"]}}`)

	out, err := run(t, "explain", "svc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"local  ~/.config/srt/svc.local.json", "~/notes", "share.example.net"} {
		if !strings.Contains(out, want) {
			t.Errorf("explain output lacks %q:\n%s", want, out)
		}
	}

	write(t, filepath.Join(home, ".config/srt/svc.local.json"), `{"allowPty": true}`)
	if _, err := run(t, "explain", "svc"); err == nil ||
		!strings.Contains(err.Error(), "refuses to launch") {
		t.Fatalf("explain with a narrowing layer = %v", err)
	}
	if _, err := run(t, "explain", "../svc"); err == nil {
		t.Fatal("explain accepted a path as a label")
	}
}

func TestVerify(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "services/events/service-info.yaml"), eventsInfo)
	write(t, filepath.Join(root, "recipes/events/recipe.toml"), "")
	if _, err := run(t, "generate", root); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "verify", root)
	want := "ok      recipes/events/events.srt.json (renders from services/events/service-info.yaml)"
	if err != nil || !strings.Contains(out, want) {
		t.Fatalf("verify after generate: %v\n%s", err, out)
	}

	settings := filepath.Join(root, "recipes/events/events.srt.json")
	write(t, settings, "{}")
	write(t, filepath.Join(root, "recipes/events/events.local.json"), `{"allowPty": true}`)
	out, err = run(t, "verify", root)
	if err == nil || !strings.Contains(err.Error(), "2 of 2 sandbox checks") {
		t.Fatalf("verify on a broken tree = %v\n%s", err, out)
	}
	for _, line := range []string{
		"FAIL    recipes/events/events.srt.json: drifted from services/events/service-info.yaml",
		"FAIL    recipes/events/events.local.json: srt-run would refuse it",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("verify output lacks %q:\n%s", line, out)
		}
	}
	if data, _ := os.ReadFile(settings); string(data) != "{}" {
		t.Error("verify rewrote the settings file; only generate may write")
	}
}
