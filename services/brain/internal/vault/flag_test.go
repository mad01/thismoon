package vault

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

func TestFlagDisputesTheNote(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/claim.md": revocableNote})
	path := filepath.Join(v.Root, "notes", "claim.md")
	if _, err := v.Flag(path, "the benchmark ran on one machine", revokeDay); err != nil {
		t.Fatal(err)
	}
	note, err := v.Read("notes/claim.md", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "DISPUTED on 2026-11-05: the benchmark ran on one machine. Check the claims before citing them."
	if note.Frontmatter.Status != lifecycle.Disputed || note.Banner != want {
		t.Errorf(
			"status %q banner %q, want disputed and %q",
			note.Frontmatter.Status,
			note.Banner,
			want,
		)
	}

	if _, err := v.Flag(path, "rerun pending", revokeDay.AddDate(0, 0, 2)); err != nil {
		t.Fatal(err)
	}
	f := readLifecycle(t, v, path)
	if f.StatusReason != "rerun pending" || f.StatusAt != "2026-11-05" {
		t.Errorf("second flag: reason %q status_at %q, want the new reason and the first day",
			f.StatusReason, f.StatusAt)
	}
}

func TestFlagRefusals(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/claim.md": revocableNote})
	path := filepath.Join(v.Root, "notes", "claim.md")
	if _, err := v.Flag(path, " ", revokeDay); !errors.Is(err, ErrInvalidStatusChange) {
		t.Errorf("empty reason = %v, want ErrInvalidStatusChange", err)
	}
	if _, err := v.Retract(path, lifecycle.ReasonError, "", revokeDay); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Flag(path, "late", revokeDay); !errors.Is(err, ErrInvalidStatusChange) {
		t.Errorf("flag of a retracted note = %v, want ErrInvalidStatusChange", err)
	}
	v.Shared = true
	_, err := v.Flag(path, "late", revokeDay)
	if !errors.Is(err, ErrReadOnly) || !strings.Contains(err.Error(), "brain flag opens one") {
		t.Errorf("shared flag = %v, want ErrReadOnly pointing at an issue", err)
	}
}
