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

func TestEditRefusesRevokingAndUnknownStatuses(t *testing.T) {
	const note = "---\ntitle: Live\nstatus: validated\n---\nBody.\n"
	tests := []struct {
		name string
		opts func(t *testing.T) EditOptions
		want string
	}{
		{"patch superseded", patchStatus(lifecycle.Superseded), "brain supersede"},
		{"patch retracted", patchStatus(lifecycle.Retracted), "brain retract"},
		{"patch withdrawn", patchStatus(lifecycle.Withdrawn), "brain withdraw"},
		{"patch unknown", patchStatus("bogus"), "is not one of raw, validated, promoting, disputed"},
		{"patch expired", patchStatus("expired"), "is not one of"},
		{"set superseded", setStatus(lifecycle.Superseded), "brain supersede"},
		{"set unknown", setStatus("Finished"), "is not one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"notes/live.md": note})
			path := filepath.Join(v.Root, "notes", "live.md")
			_, err := v.Edit(path, tt.opts(t))
			if !errors.Is(err, ErrInvalidStatusChange) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want ErrInvalidStatusChange containing %q", err, tt.want)
			}
			if data, _ := os.ReadFile(path); string(data) != note {
				t.Errorf("note changed to %q", data)
			}
		})
	}
}

func TestEditSetsTheLiveStatuses(t *testing.T) {
	for _, status := range EditStatuses {
		t.Run(status, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"inbox/x.md": "---\ntitle: X\nstatus: raw\n---\nBody.\n"})
			path := filepath.Join(v.Root, "inbox", "x.md")
			if _, err := v.Edit(path, patchStatus(status)(t)); err != nil {
				t.Fatal(err)
			}
			if got := readLifecycle(t, v, path).Status; got != status {
				t.Errorf("status = %q, want %q", got, status)
			}
		})
	}
}

func TestEditRefusesATombstone(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/gone.md": "---\ntitle: Gone\n---\nText.\n"})
	path := filepath.Join(v.Root, "notes", "gone.md")
	if _, err := v.Withdraw(path, time.Now()); err != nil {
		t.Fatal(err)
	}
	tomb, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, opts := range map[string]EditOptions{
		"append":    {Append: "back"},
		"validated": patchStatus(lifecycle.Validated)(t),
		"set":       setStatus(lifecycle.Validated)(t),
	} {
		if _, err := v.Edit(path, opts); !errors.Is(err, ErrInvalidStatusChange) ||
			!strings.Contains(err.Error(), "tombstone") {
			t.Errorf("%s: err = %v, want a tombstone refusal", name, err)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != string(tomb) {
		t.Errorf("tombstone changed to %q", data)
	}
}

func patchStatus(status string) func(*testing.T) EditOptions {
	return func(*testing.T) EditOptions {
		return EditOptions{FrontmatterPatch: &markdown.Frontmatter{Status: status}}
	}
}

func setStatus(status string) func(*testing.T) EditOptions {
	return func(t *testing.T) EditOptions {
		return EditOptions{Set: []markdown.Field{mustField(t, "status", status)}}
	}
}
