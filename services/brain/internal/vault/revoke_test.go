package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

var revokeDay = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

const revocableNote = `---
title: Claim
id: 01JC0000000000000000000000
status: validated
tags: [ai]
---
The benchmark shows a 2x speedup.
`

func TestRevocations(t *testing.T) {
	tests := []struct {
		name     string
		revoke   func(v *Vault, path string) (string, error)
		status   string
		banner   string
		keepBody bool
	}{
		{
			name: "supersede",
			revoke: func(v *Vault, path string) (string, error) {
				return v.Supersede(path, "id:01JD0000000000000000000000", revokeDay)
			},
			status:   lifecycle.Superseded,
			banner:   "SUPERSEDED on 2026-11-05 by id:01JD0000000000000000000000. Cite that note instead.",
			keepBody: true,
		},
		{
			name: "retract",
			revoke: func(v *Vault, path string) (string, error) {
				return v.Retract(path, lifecycle.ReasonError, "misread the chart", revokeDay)
			},
			status:   lifecycle.Retracted,
			banner:   "RETRACTED on 2026-11-05 (error): misread the chart. Do not cite this note.",
			keepBody: true,
		},
		{
			name:   "withdraw",
			revoke: func(v *Vault, path string) (string, error) { return v.Withdraw(path, revokeDay) },
			status: lifecycle.Withdrawn,
			banner: "WITHDRAWN on 2026-11-05. Its content was removed.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"notes/claim.md": revocableNote})
			path := filepath.Join(v.Root, "notes", "claim.md")
			sha, err := tt.revoke(v, path)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if sha != ContentSHA(data) {
				t.Error("returned sha is not the written content's")
			}
			note, err := v.Read("notes/claim.md", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			if note.Frontmatter.Status != tt.status || note.Banner != tt.banner {
				t.Errorf("status %q banner %q, want %q and %q",
					note.Frontmatter.Status, note.Banner, tt.status, tt.banner)
			}
			if note.Frontmatter.ID != "01JC0000000000000000000000" {
				t.Errorf("id = %q, want it kept", note.Frontmatter.ID)
			}
			if hasBody := strings.Contains(string(data), "2x speedup"); hasBody != tt.keepBody {
				t.Errorf("body kept = %v, want %v:\n%s", hasBody, tt.keepBody, data)
			}
			fm, body, err := v.ReadFrontmatter(path)
			if err != nil {
				t.Fatal(err)
			}
			if p := lifecycle.Required(fm, body); len(p) > 0 {
				t.Errorf("revoked note misses required fields: %v", p)
			}
		})
	}
}

func TestWithdrawLeavesOnlyTheTombstone(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/claim.md": revocableNote})
	path := filepath.Join(v.Root, "notes", "claim.md")
	if _, err := v.Withdraw(path, revokeDay); err != nil {
		t.Fatal(err)
	}
	want := "---\nstatus: withdrawn\nid: 01JC0000000000000000000000\nstatus_at: 2026-11-05\n---\n"
	if data, _ := os.ReadFile(path); string(data) != want {
		t.Errorf("tombstone =\n%s\nwant\n%s", data, want)
	}
	if _, err := v.Withdraw(path, revokeDay.AddDate(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != want {
		t.Errorf("a second withdraw changed the tombstone:\n%s", data)
	}
	if _, err := v.Retract(path, lifecycle.ReasonError, "", revokeDay); !errors.Is(
		err,
		ErrInvalidStatusChange,
	) {
		t.Errorf("Retract of a tombstone = %v, want ErrInvalidStatusChange", err)
	}
}

func TestRevocationRefusals(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/claim.md": revocableNote})
	path := filepath.Join(v.Root, "notes", "claim.md")
	if _, err := v.Supersede(path, "id:01JC0000000000000000000000", revokeDay); !errors.Is(
		err,
		ErrInvalidStatusChange,
	) {
		t.Errorf("self-supersede = %v, want ErrInvalidStatusChange", err)
	}
	if _, err := v.Retract(path, "boredom", "", revokeDay); !errors.Is(
		err,
		ErrInvalidStatusChange,
	) {
		t.Errorf("unknown reason = %v, want ErrInvalidStatusChange", err)
	}
	v.Shared = true
	if _, err := v.Withdraw(path, revokeDay); !errors.Is(err, ErrReadOnly) {
		t.Errorf("shared withdraw = %v, want ErrReadOnly", err)
	}
}

func TestWriteAndEditLifecycleDefaults(t *testing.T) {
	v := newTestVault(t, nil)
	raw, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Capture", Now: revokeDay})
	if err != nil {
		t.Fatal(err)
	}
	high, err := v.Write(WriteOptions{
		Tier: TierNotes, Title: "Fast", Now: revokeDay,
		Set: []markdown.Field{mustField(t, "volatility", "high")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readLifecycle(t, v, raw).ExpiresAt; got != "2026-12-05" {
		t.Errorf("raw capture expires_at = %q, want 2026-12-05", got)
	}
	if got := readLifecycle(t, v, high).ReviewBy; got != "2027-02-03" {
		t.Errorf("high volatility review_by = %q, want 2027-02-03", got)
	}
	if _, err := v.Edit(raw, EditOptions{
		FrontmatterPatch: &markdown.Frontmatter{Status: lifecycle.Disputed},
		Now:              revokeDay,
	}); err != nil {
		t.Fatal(err)
	}
	f := readLifecycle(t, v, raw)
	if f.StatusAt != "2026-11-05" || f.ExpiresAt != "" {
		t.Errorf("edit to disputed: status_at %q expires_at %q", f.StatusAt, f.ExpiresAt)
	}
}

func readLifecycle(t *testing.T, v *Vault, path string) lifecycle.Fields {
	t.Helper()
	fm, _, err := v.ReadFrontmatter(path)
	if err != nil {
		t.Fatal(err)
	}
	return lifecycle.Read(fm)
}
