package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath_DirAndFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "service-info.yaml")
	if err := os.WriteFile(file, []byte(`
kind: System
metadata:
  name: demo
spec:
  owner: mad01
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// As a directory.
	ents, err := validatePath(context.Background(), dir)
	if err != nil {
		t.Fatalf("validatePath(dir): %v", err)
	}
	if len(ents) != 1 || ents[0].SourcePath == "" {
		t.Fatalf("dir scan = %+v", ents)
	}

	// As a single file.
	ents, err = validatePath(context.Background(), file)
	if err != nil {
		t.Fatalf("validatePath(file): %v", err)
	}
	if len(ents) != 1 || ents[0].SourcePath != file {
		t.Fatalf("file parse = %+v", ents)
	}
}

func TestValidatePath_Invalid(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "service-info.yaml")
	// Component without a system is invalid.
	if err := os.WriteFile(file, []byte("kind: Component\nmetadata:\n  name: x\nspec:\n  owner: mad01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validatePath(context.Background(), file); err == nil {
		t.Error("expected validation error, got nil")
	}
}

func TestRunValidate_RequiresAccess(t *testing.T) {
	root := t.TempDir()
	write := func(p, content string) {
		t.Helper()
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".git/HEAD", "")
	write("service-info.yaml", "kind: System\nmetadata: {name: demo}\nspec: {owner: mad01}\n")
	write("services/web/service-info.yaml",
		"kind: Component\nmetadata: {name: web}\nspec: {owner: mad01, system: demo}\n")
	write("recipes/web/recipe.toml", "commands = [\"t-man add --name web -- web serve\"]\n")

	validateCmd.SetContext(context.Background())
	err := runValidate(validateCmd, []string{root})
	if err == nil || !strings.Contains(err.Error(), "declare no spec.access") {
		t.Fatalf("runValidate() = %v, want the missing access", err)
	}

	write("services/web/service-info.yaml", `kind: Component
metadata: {name: web}
spec:
  owner: mad01
  system: demo
  access: {sandbox: srt, listen: 8080}
`)
	if err := runValidate(validateCmd, []string{root}); err != nil {
		t.Fatalf("runValidate() with access = %v", err)
	}
}
