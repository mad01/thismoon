package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Main(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	path := filepath.Join(home, "config.yaml")
	writeFile(t, path, body)
	return path
}

func TestMissingConfigNamesTheFix(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "absent.yaml")
	code, stdout, stderr := run(t, "--config", cfg, "settings", "check")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "config: file not found: "+cfg) ||
		!strings.Contains(stderr, "AGENT_SYNC_CONFIG") {
		t.Errorf("settings check = %d, %q, %q", code, stdout, stderr)
	}
}

func TestConfigEnvRelocatesTheFile(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "template.json")
	live := filepath.Join(dir, "settings.json")
	writeFile(t, template, `{"model": "opus"}`)
	writeFile(t, live, "{\n  \"model\": \"opus\"\n}\n")
	cfg := writeConfig(t, "settings:\n  template: "+template+"\n  live: "+live+"\n")
	t.Setenv("AGENT_SYNC_CONFIG", cfg)
	code, stdout, stderr := run(t, "settings", "check")
	if code != 0 || stdout != "settings already in sync\n" || stderr != "" {
		t.Errorf("settings check with $AGENT_SYNC_CONFIG = %d, %q, %q", code, stdout, stderr)
	}
}

func TestDoctor(t *testing.T) {
	template := filepath.Join(t.TempDir(), "template.json")
	writeFile(t, template, `{"model": "opus"}`)
	cfg := writeConfig(t, "settings:\n  template: "+template+"\n")
	code, stdout, _ := run(t, "--config", cfg, "doctor")
	for _, line := range []string{
		"ok config\n", "ok settings-config\n", "ok settings-template\n",
		"ok settings-live-writable\n", "all checks passed",
	} {
		if !strings.Contains(stdout, line) {
			t.Errorf("doctor output lacks %q:\n%s", line, stdout)
		}
	}
	if code != 0 {
		t.Errorf("doctor exit = %d, want 0", code)
	}
}

func TestDoctorFailsOnMissingConfig(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "absent.yaml")
	code, stdout, stderr := run(t, "--config", cfg, "doctor")
	if code != 1 || !strings.HasPrefix(stdout, "FAIL config: config: file not found") ||
		!strings.Contains(stderr, "1 of 1 checks failed") {
		t.Errorf("doctor = %d, %q, %q", code, stdout, stderr)
	}
}

func TestDocsRenders(t *testing.T) {
	code, stdout, _ := run(t, "docs")
	if code != 0 || !strings.Contains(stdout, "agent-sync") {
		t.Errorf("docs = %d, %q", code, stdout)
	}
}

func TestVersionIsABareToken(t *testing.T) {
	code, stdout, _ := run(t, "version")
	if code != 0 || strings.Count(stdout, "\n") != 1 || strings.Contains(stdout, " ") {
		t.Errorf("version = %d, %q; want one bare token", code, stdout)
	}
	code, stdout, _ = run(t, "version", "-o", "json")
	for _, key := range []string{`"version"`, `"commit"`, `"tag"`, `"build_time"`} {
		if code != 0 || !strings.Contains(stdout, key) {
			t.Errorf("version -o json = %d, %q; want key %s", code, stdout, key)
		}
	}
}
