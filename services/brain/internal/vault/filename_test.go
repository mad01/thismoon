package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestUnicodeTitlesGetTheirOwnFiles(t *testing.T) {
	v := newTestVault(t, nil)
	seen := map[string]string{}
	for _, title := range []string{"日本語のメモ", "中文笔记", "Привет мир"} {
		path, err := v.Write(WriteOptions{Tier: TierInbox, Title: title})
		if err != nil {
			t.Fatalf("Write(%q): %v", title, err)
		}
		if other, ok := seen[path]; ok {
			t.Errorf("%q and %q both went to %s", title, other, path)
		}
		seen[path] = title
	}
}

func TestATitleWithNoLettersIsNamedByItsID(t *testing.T) {
	v := newTestVault(t, nil)
	path, err := v.Write(WriteOptions{Tier: TierInbox, Title: "!!! ???"})
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.ToLower(fm.ID) + ".md"; filepath.Base(path) != want {
		t.Errorf("file = %s, want %s", filepath.Base(path), want)
	}
	res, err := v.Promote(path, PromoteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(v.Root, "notes", strings.ToLower(fm.ID)+".md"); res.NewPath != want {
		t.Errorf("promoted to %s, want %s", res.NewPath, want)
	}
}
