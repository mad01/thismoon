package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteRefusesAFilenameOutsideTheTier(t *testing.T) {
	names := []string{
		"../../escaped",
		"../notes/other",
		"/tmp/escaped",
		"sub/name",
		`sub\name`,
		"..",
		"a..b",
		"line\nbreak",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			v := newTestVault(t, nil)
			_, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Esc", Filename: name})
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("Write(filename %q) err = %v, want ErrInvalidPath", name, err)
			}
			for _, p := range []string{
				filepath.Join(filepath.Dir(v.Root), "escaped.md"),
				filepath.Join(v.Root, "escaped.md"),
				filepath.Join(v.Root, "notes", "other.md"),
			} {
				if _, err := os.Stat(p); err == nil {
					t.Errorf("%s was written", p)
				}
			}
		})
	}
}

func TestWriteTakesAPlainFilename(t *testing.T) {
	v := newTestVault(t, nil)
	path, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Esc", Filename: "custom-name"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(v.Root, "inbox", "custom-name.md"); path != want {
		t.Errorf("path = %s, want %s", path, want)
	}
}
