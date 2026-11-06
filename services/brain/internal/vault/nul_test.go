package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// nulBody hides fixtureKey behind a NUL byte, which makes the secret scan
// treat the note as binary.
const nulBody = "x\x00y\nkey " + fixtureKey + "\n"

func TestWritePathsRefuseANULByte(t *testing.T) {
	const clean = "---\ntitle: Clean\nstatus: raw\n---\nBody.\n"
	v := newTestVault(t, map[string]string{
		"inbox/clean.md": clean,
		"inbox/held.md":  "---\ntitle: Held\nstatus: raw\n---\n" + nulBody,
	})
	cleanPath := filepath.Join(v.Root, "inbox", "clean.md")
	changes := map[string]func() error{
		"write": func() error {
			_, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Nul", Body: nulBody})
			return err
		},
		"edit append": func() error {
			_, err := v.Edit(cleanPath, EditOptions{Append: nulBody})
			return err
		},
		"edit body": func() error {
			_, err := v.Edit(cleanPath, EditOptions{Body: nulBody})
			return err
		},
		"promote": func() error {
			_, err := v.Promote(filepath.Join(v.Root, "inbox", "held.md"), PromoteOptions{Now: time.Now()})
			return err
		},
	}
	for name, change := range changes {
		err := change()
		if !errors.Is(err, ErrInvalidContent) || !strings.Contains(err.Error(), "holds a NUL byte at line") {
			t.Errorf("%s: err = %v, want ErrInvalidContent naming the NUL", name, err)
		}
	}
	if data, _ := os.ReadFile(cleanPath); string(data) != clean {
		t.Errorf("inbox/clean.md changed to %q", data)
	}
	for _, p := range []string{"inbox/nul.md", "notes/held.md"} {
		if _, err := os.Stat(filepath.Join(v.Root, p)); err == nil {
			t.Errorf("%s was written", p)
		}
	}
}

func TestSecretsReportsANULByte(t *testing.T) {
	v := newTestVault(t, nil)
	found, err := v.Secrets("inbox/x.md", []byte("---\ntitle: X\n---\n"+nulBody))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 || found[0].Rule.ID != "nul-byte" || found[0].Line != 4 || found[0].Column != 2 {
		t.Fatalf("findings = %+v, want nul-byte at line 4, column 2 first", found)
	}
}
