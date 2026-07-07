package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func scriptDenyConfig(claudeDeny, extra []string) config.Config {
	return config.Config{
		ClaudeDeny: claudeDeny,
		Guards: map[string]config.GuardToggle{
			ScriptDenyListID: {ExtraPatterns: extra},
		},
	}
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScriptDenyList(t *testing.T) {
	dir := t.TempDir()
	deleting := writeScript(t, dir, "deleting.sh",
		"#!/bin/bash\nset -euo pipefail\nkubectl delete pod broken\n")
	clean := writeScript(t, dir, "clean.sh",
		"#!/bin/bash\n# kubectl delete is dangerous, this script does not run it\necho hello\n")
	nuking := writeScript(t, dir, "nuke.py",
		"import subprocess\nsubprocess.run(\"gcloud projects delete my-proj\", shell=True)\n")
	rmScript := writeScript(t, dir, "cleanup.sh", "rm -rf /tmp/scratch\n")

	claudeDeny := []string{"kubectl delete", "gcloud projects delete", "gsutil rm"}
	extra := []string{"rm -rf"}

	tests := []struct {
		name    string
		command string
		cwd     string
		deny    bool
	}{
		{"plain command untouched", "ls -la && echo done", "", false},
		{"blocked words in echo string allowed", `echo "kubectl delete would be bad"`, "", false},
		{"bash script with blocked command", "bash " + deleting, "", true},
		{"sh script with blocked command", "sh " + deleting, "", true},
		{"script with pattern only in comment", "bash " + clean, "", false},
		{"python script shelling out", "python3 " + nuking, "", true},
		{"direct execution", "./cleanup.sh", dir, true},
		{"relative path resolved via cwd", "bash deleting.sh", dir, true},
		{"sourced script", "source " + rmScript, "", true},
		{"dot-sourced script", ". " + rmScript, "", true},
		{"inline -c script", `bash -c "gsutil rm -r gs://bucket/x"`, "", true},
		{"inline -c clean", `bash -c "gsutil ls gs://bucket"`, "", false},
		{"heredoc into bash", "bash <<'EOF'\nrm -rf /tmp/x\nEOF", "", true},
		{"wrapped in sudo and env", "sudo env FOO=bar bash " + deleting, "", true},
		{"missing file allowed", "bash /nonexistent/nope.sh", "", false},
		{"python -m is not a file", "python -m pytest", dir, false},
		{"chained after safe command", "make build && bash " + deleting, "", true},
		{"trailing -c does not skip file scan", "python3 " + nuking + " -c ignored", "", true},
		{"pattern before inline segment allowed", `echo "kubectl delete would be bad" && bash -c "echo hi"`, "", false},
		{"inline -c spanning separators", `bash -c "echo hi; gsutil rm -r gs://bucket/x"`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewScriptDenyList(scriptDenyConfig(claudeDeny, extra))
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)", got, tt.deny, d)
			}
			if d != nil && !strings.Contains(d.Reason, "belt["+ScriptDenyListID+"]") {
				t.Fatalf("reason missing guard prefix: %s", d.Reason)
			}
		})
	}
}

func TestScriptDenyListNoPatterns(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "x.sh", "rm -rf /tmp/x\n")
	g := NewScriptDenyList(config.Config{})
	if d := g.Check(Input{Event: EventBash, Command: "bash " + script}); d != nil {
		t.Fatalf("expected allow with no configured patterns, got %+v", d)
	}
}

func TestScriptDenyListExcludePaths(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "x.sh", "kubectl delete pod x\n")
	cfg := scriptDenyConfig([]string{"kubectl delete"}, nil)
	toggle := cfg.Guards[ScriptDenyListID]
	toggle.ExcludePaths = []string{dir}
	cfg.Guards[ScriptDenyListID] = toggle
	g := NewScriptDenyList(cfg)
	if d := g.Check(Input{Event: EventBash, Command: "bash " + script}); d != nil {
		t.Fatalf("expected exclude_paths to skip the file, got %+v", d)
	}
}

func TestCompilePatternsWordBoundaries(t *testing.T) {
	patterns := compilePatterns([]string{"kubectl delete", "rm -rf", "bq rm"})
	match := func(line string) []string {
		var hits []string
		for _, d := range patterns {
			if d.re.MatchString(line) {
				hits = append(hits, d.pattern)
			}
		}
		return hits
	}
	tests := []struct {
		line string
		want int
	}{
		{"kubectl delete pod x", 1},
		{"  kubectl   delete   pod", 1},
		{"KUBECTL DELETE pod", 1},
		{"kubectl deleted pod", 0},
		{"mykubectl delete pod", 0},
		{"rm -rf /tmp/x", 1},
		{"rm -rfv /tmp/x", 0},
		{"firm -rf x", 0},
		{"bq rm -t ds.table", 1},
		{"bq rmdir", 0},
		{`subprocess.run("kubectl delete pod x")`, 1},
	}
	for _, tt := range tests {
		if got := len(match(tt.line)); got != tt.want {
			t.Errorf("line %q: %d matches, want %d", tt.line, got, tt.want)
		}
	}
}
