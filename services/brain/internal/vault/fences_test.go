package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

func TestChangesRefuseUnclosedFrontmatter(t *testing.T) {
	const note = "---\ntitle: Open\nstatus: raw\nBody with no closing fence.\n"
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	changes := map[string]func(v *Vault, path string) error{
		"edit": func(v *Vault, path string) error {
			_, err := v.Edit(path, EditOptions{Append: "more", Now: now})
			return err
		},
		"flag": func(v *Vault, path string) error {
			_, err := v.Flag(path, "doubtful", now)
			return err
		},
		"promote": func(v *Vault, path string) error {
			_, err := v.Promote(path, PromoteOptions{Now: now})
			return err
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"inbox/open.md": note})
			path := filepath.Join(v.Root, "inbox", "open.md")
			err := change(v, path)
			if !errors.Is(err, ErrInvalidContent) || !strings.Contains(err.Error(), "not closed") {
				t.Fatalf("err = %v, want ErrInvalidContent naming the open fence", err)
			}
			if data, _ := os.ReadFile(path); string(data) != note {
				t.Errorf("note changed to %q", data)
			}
		})
	}
}

func TestEditReadsLooseFences(t *testing.T) {
	notes := map[string]string{
		"byte order mark": "\xEF\xBB\xBF---\ntitle: A\n---\nBody.\n",
		"trailing spaces": "---  \ntitle: A\n--- \nBody.\n",
		"dot fence":       "---\ntitle: A\n...\nBody.\n",
	}
	for name, note := range notes {
		t.Run(name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"notes/a.md": note})
			path := filepath.Join(v.Root, "notes", "a.md")
			if _, err := v.Edit(path, EditOptions{Append: "More."}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, body := markdown.Split(data)
			if strings.Contains(string(body), "title: A") || !strings.Contains(string(raw), "title: A") {
				t.Errorf("edit stacked a second frontmatter block:\n%q", data)
			}
			if !strings.HasSuffix(string(body), "Body.\nMore.\n") {
				t.Errorf("body = %q, want the appended line", body)
			}
		})
	}
}

func TestMigrateIDsSkipsUnclosedFrontmatter(t *testing.T) {
	const note = "---\ntitle: Open\n"
	v := newTestVault(t, map[string]string{"notes/open.md": note})
	report, err := v.MigrateIDs(false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 0 || len(report.Skipped) != 1 ||
		!strings.Contains(report.Skipped[0].Reason, "not closed") {
		t.Errorf("report = %+v, want notes/open.md skipped as not closed", report)
	}
	if data, _ := os.ReadFile(filepath.Join(v.Root, "notes", "open.md")); string(data) != note {
		t.Errorf("note changed to %q", data)
	}
}
