package cli

import (
	"context"
	"os"
	"path/filepath"
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
