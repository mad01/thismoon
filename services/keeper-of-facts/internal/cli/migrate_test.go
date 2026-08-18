package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrationDirs points $HOME at a temp dir and returns the old and new
// default store paths inside it.
func migrationDirs(t *testing.T) (oldDir, newDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".local", "share", "keep"),
		filepath.Join(home, ".local", "share", "kof")
}

func seedStore(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assertions.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateWorkdirMovesTheOldStore(t *testing.T) {
	oldDir, newDir := migrationDirs(t)
	seedStore(t, oldDir)

	if err := migrateWorkdir(newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "assertions.jsonl")); err != nil {
		t.Errorf("store file did not arrive at the new path: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("old dir still present after migration")
	}
}

func TestMigrateWorkdirNoOldStoreIsANoop(t *testing.T) {
	_, newDir := migrationDirs(t)
	if err := migrateWorkdir(newDir); err != nil {
		t.Fatalf("fresh machine must migrate silently, got %v", err)
	}
}

func TestMigrateWorkdirClearsEmptyNewDir(t *testing.T) {
	oldDir, newDir := migrationDirs(t)
	seedStore(t, oldDir)
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := migrateWorkdir(newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(newDir, "assertions.jsonl")); err != nil {
		t.Errorf("empty new dir was not cleared before migration: %v", err)
	}
}

func TestMigrateWorkdirRefusesTwoRealStores(t *testing.T) {
	oldDir, newDir := migrationDirs(t)
	seedStore(t, oldDir)
	seedStore(t, newDir)

	err := migrateWorkdir(newDir)
	if err == nil {
		t.Fatal("two populated stores must refuse migration, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to guess") {
		t.Errorf("error %q does not explain the refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(oldDir, "assertions.jsonl")); statErr != nil {
		t.Errorf("refusal must leave the old store untouched: %v", statErr)
	}
}

func TestMigrateWorkdirIgnoresCustomWorkdir(t *testing.T) {
	oldDir, _ := migrationDirs(t)
	seedStore(t, oldDir)

	custom := t.TempDir()
	if err := migrateWorkdir(custom); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "assertions.jsonl")); err != nil {
		t.Errorf("a custom workdir must never trigger migration of the defaults: %v", err)
	}
}
