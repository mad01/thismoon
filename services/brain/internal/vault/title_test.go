package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

var controlTitles = []string{
	"two\nlines",
	"carriage\rreturn",
	"red \x1b[31mtext",
	"nul\x00",
	"del\x7f",
}

func TestTitlesRefuseControlCharacters(t *testing.T) {
	const note = "---\ntitle: Clean\nstatus: raw\n---\nBody.\n"
	for _, title := range controlTitles {
		t.Run(title, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"inbox/clean.md": note})
			path := filepath.Join(v.Root, "inbox", "clean.md")
			changes := map[string]func() error{
				"write": func() error {
					_, err := v.Write(WriteOptions{Tier: TierInbox, Title: title})
					return err
				},
				"write --set title": func() error {
					_, err := v.Write(WriteOptions{
						Tier: TierInbox, Title: "Fine",
						Set: []markdown.Field{mustField(t, "title", quoteYAML(title))},
					})
					return err
				},
				"edit --title": func() error {
					_, err := v.Edit(
						path,
						EditOptions{FrontmatterPatch: &markdown.Frontmatter{Title: title}},
					)
					return err
				},
				"edit --set title": func() error {
					_, err := v.Edit(path, EditOptions{
						Set: []markdown.Field{mustField(t, "title", quoteYAML(title))},
					})
					return err
				},
				"promote --title": func() error {
					_, err := v.Promote(path, PromoteOptions{NewTitle: title})
					return err
				},
			}
			for name, change := range changes {
				if err := change(); !errors.Is(err, ErrInvalidContent) {
					t.Errorf("%s: err = %v, want ErrInvalidContent", name, err)
				}
			}
			if data, _ := os.ReadFile(path); string(data) != note {
				t.Errorf("note changed to %q", data)
			}
			entries, _ := os.ReadDir(filepath.Join(v.Root, "inbox"))
			if len(entries) != 1 {
				t.Errorf("inbox holds %d files, want the one note", len(entries))
			}
		})
	}
}

func TestTitleTakesATab(t *testing.T) {
	v := newTestVault(t, nil)
	if _, err := v.Write(WriteOptions{Tier: TierInbox, Title: "tab\there"}); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteRefusesAControlTitleIntoAMOC(t *testing.T) {
	const note = "---\ntitle: \"two\\nlines\"\nstatus: raw\n---\nBody.\n"
	v := newTestVault(t, map[string]string{
		"inbox/bad.md":  note,
		"MOCs/topic.md": "# Topic\n",
	})
	_, err := v.Promote(
		filepath.Join(v.Root, "inbox", "bad.md"),
		PromoteOptions{MOC: "MOCs/topic.md"},
	)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
	if data, _ := os.ReadFile(filepath.Join(v.Root, "MOCs", "topic.md")); string(
		data,
	) != "# Topic\n" {
		t.Errorf("MOC changed to %q", data)
	}
	if _, err := os.Stat(filepath.Join(v.Root, "inbox", "bad.md")); err != nil {
		t.Errorf("inbox note moved: %v", err)
	}
}

func TestPromoteMOCMustBeInTheMOCsTier(t *testing.T) {
	const note = "---\ntitle: Ready\nstatus: raw\n---\nBody.\n"
	outside := []string{
		"../outside.md",
		"MOCs/../notes/other.md",
		"notes/other.md",
		"inbox/ready.md",
		"/tmp/brain-moc-outside.md",
	}
	for _, moc := range outside {
		t.Run(moc, func(t *testing.T) {
			v := newTestVault(t, map[string]string{
				"inbox/ready.md": note,
				"notes/other.md": "---\ntitle: Other\n---\nOther.\n",
			})
			_, err := v.Promote(
				filepath.Join(v.Root, "inbox", "ready.md"),
				PromoteOptions{MOC: moc},
			)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("err = %v, want ErrInvalidPath", err)
			}
			if data, _ := os.ReadFile(filepath.Join(v.Root, "notes", "other.md")); strings.Contains(
				string(data),
				"[[",
			) {
				t.Errorf("notes/other.md got a bullet: %q", data)
			}
			for _, p := range []string{filepath.Join(filepath.Dir(v.Root), "outside.md"), "/tmp/brain-moc-outside.md"} {
				if _, err := os.Stat(p); err == nil {
					t.Errorf("%s was written", p)
				}
			}
			if _, err := os.Stat(filepath.Join(v.Root, "inbox", "ready.md")); err != nil {
				t.Errorf("inbox note moved: %v", err)
			}
		})
	}
}

func TestPromoteMOCInsideTheTier(t *testing.T) {
	const note = "---\ntitle: Ready\nstatus: raw\n---\nBody.\n"
	for name, moc := range map[string]func(root string) string{
		"relative without .md": func(string) string { return "MOCs/topic" },
		"absolute":             func(root string) string { return filepath.Join(root, "MOCs", "topic.md") },
	} {
		t.Run(name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"inbox/ready.md": note})
			res, err := v.Promote(
				filepath.Join(v.Root, "inbox", "ready.md"),
				PromoteOptions{MOC: moc(v.Root)},
			)
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(v.Root, "MOCs", "topic.md")
			if res.MOCAppended != want {
				t.Errorf("MOCAppended = %q, want %q", res.MOCAppended, want)
			}
			if data, _ := os.ReadFile(want); !strings.Contains(string(data), "[[ready|Ready]]") {
				t.Errorf("MOC = %q, want the bullet", data)
			}
		})
	}
}

// quoteYAML returns s as a double-quoted YAML scalar.
func quoteYAML(s string) string {
	r := strings.NewReplacer(
		`\`,
		`\\`,
		`"`,
		`\"`,
		"\n",
		`\n`,
		"\r",
		`\r`,
		"\x1b",
		`\e`,
		"\x00",
		`\0`,
		"\x7f",
		`\x7f`,
	)
	return `"` + r.Replace(s) + `"`
}
