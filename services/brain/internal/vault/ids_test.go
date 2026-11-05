package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/noteid"
)

const knownID = "01JABCDEFGHJKMNPQRSTVWXYZ0"

func noteID(t *testing.T, v *Vault, rel string) string {
	t.Helper()
	fm, _, err := v.ReadFrontmatter(filepath.Join(v.Root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return fm.ID
}

func TestWriteAssignsAnID(t *testing.T) {
	v := newTestVault(t, nil)
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	path, err := v.Write(WriteOptions{Tier: TierInbox, Title: "Fresh", Body: "x", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if id := noteID(t, v, v.Rel(path)); !noteid.Valid(id) {
		t.Errorf("id = %q, want a ULID", id)
	}

	path, err = v.Write(WriteOptions{
		Tier:  TierInbox,
		Title: "Given",
		Set:   []markdown.Field{mustField(t, "id", knownID)},
		Now:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id := noteID(t, v, v.Rel(path)); id != knownID {
		t.Errorf("id = %q, want the one Set gave", id)
	}
}

func TestEditNeverChangesAnID(t *testing.T) {
	withID := "---\ntitle: A\nid: " + knownID + "\n---\nbody\n"
	tests := []struct {
		name    string
		note    string
		opts    EditOptions
		wantErr error
		wantID  string
	}{
		{
			name:   "body edit keeps it",
			note:   withID,
			opts:   EditOptions{Append: "more"},
			wantID: knownID,
		},
		{
			name: "set to another id",
			note: withID,
			opts: EditOptions{
				Set: []markdown.Field{mustField(t, "id", "01JOTHEROTHEROTHEROTHEROTH")},
			},
			wantErr: ErrImmutableID,
		},
		{
			name:    "unset",
			note:    withID,
			opts:    EditOptions{Unset: []string{"id"}},
			wantErr: ErrImmutableID,
		},
		{
			name: "patch to another id",
			note: withID,
			opts: EditOptions{
				FrontmatterPatch: &markdown.Frontmatter{ID: "01JOTHEROTHEROTHEROTHEROTH"},
			},
			wantErr: ErrImmutableID,
		},
		{
			name:   "a note without one may get one",
			note:   "---\ntitle: A\n---\nbody\n",
			opts:   EditOptions{Set: []markdown.Field{mustField(t, "id", knownID)}},
			wantID: knownID,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestVault(t, map[string]string{"notes/a.md": tc.note})
			full := filepath.Join(v.Root, "notes", "a.md")
			_, err := v.Edit(full, tc.opts)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				data, _ := os.ReadFile(full)
				if string(data) != tc.note {
					t.Errorf("refused edit changed the note:\n%s", data)
				}
				return
			}
			if id := noteID(t, v, "notes/a.md"); id != tc.wantID {
				t.Errorf("id = %q, want %q", id, tc.wantID)
			}
		})
	}
}

func TestPromoteKeepsOrAssignsTheID(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"inbox/kept.md": "---\ntitle: Kept\nid: " + knownID + "\nstatus: raw\n---\nx\n",
		"inbox/bare.md": "---\ntitle: Bare\nstatus: raw\n---\nx\n",
	})
	res, err := v.Promote(
		filepath.Join(v.Root, "inbox", "kept.md"),
		PromoteOptions{NewTitle: "Renamed"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if id := noteID(t, v, v.Rel(res.NewPath)); id != knownID {
		t.Errorf("promoted and renamed note id = %q, want %q", id, knownID)
	}
	res, err = v.Promote(filepath.Join(v.Root, "inbox", "bare.md"), PromoteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if id := noteID(t, v, v.Rel(res.NewPath)); !noteid.Valid(id) {
		t.Errorf("promoted note without an id got %q, want a ULID", id)
	}
}

func TestMigrateIDs(t *testing.T) {
	notes := map[string]string{
		"notes/has.md":    "---\ntitle: Has\nid: " + knownID + "\n---\nbody\n",
		"notes/bare.md":   "---\ntitle: Bare\nupdated: 2026-01-02\nowner: alex\n---\nbody [[has]]\n",
		"inbox/none.md":   "no frontmatter at all\n",
		"notes/broken.md": "---\ntitle: [unclosed\n---\nbody\n",
		"README.md":       "not a note\n",
	}
	v := newTestVault(t, notes)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	dry, err := v.MigrateIDs(true, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := changedPaths(dry); got != "inbox/none.md notes/bare.md" {
		t.Errorf("dry run changed %q", got)
	}
	if len(dry.Skipped) != 1 || dry.Skipped[0].Path != "notes/broken.md" {
		t.Errorf("dry run skipped %+v, want notes/broken.md", dry.Skipped)
	}
	for rel, want := range notes {
		if data, _ := os.ReadFile(filepath.Join(v.Root, rel)); string(data) != want {
			t.Errorf("dry run wrote %s", rel)
		}
	}

	run, err := v.MigrateIDs(false, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := changedPaths(run); got != "inbox/none.md notes/bare.md" {
		t.Errorf("run changed %q", got)
	}
	data, err := os.ReadFile(filepath.Join(v.Root, "notes", "bare.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: Bare\nupdated: 2026-01-02\nowner: alex\nid: " + run.Changed[1].ID + "\n---\nbody [[has]]\n"
	if string(data) != want {
		t.Errorf("migrated note =\n%s\nwant only the id line added:\n%s", data, want)
	}
	if id := noteID(t, v, "notes/has.md"); id != knownID {
		t.Errorf("existing id changed to %q", id)
	}

	again, err := v.MigrateIDs(false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Changed) != 0 {
		t.Errorf("second run changed %+v, want nothing", again.Changed)
	}
}

func changedPaths(r MigrateReport) string {
	paths := make([]string, 0, len(r.Changed))
	for _, c := range r.Changed {
		paths = append(paths, c.Path)
	}
	return strings.Join(paths, " ")
}

func TestMigrateIDsInASharedVault(t *testing.T) {
	set := newTestSet(t, nil, map[string]string{"notes/t.md": "---\ntitle: T\n---\n"})
	team, err := set.Get("team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := team.MigrateIDs(false, time.Time{}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("real run err = %v, want ErrReadOnly", err)
	}
	dry, err := team.MigrateIDs(true, time.Time{})
	if err != nil || len(dry.Changed) != 1 {
		t.Errorf("dry run = %+v, %v; want one change and no error", dry, err)
	}
}

func TestLocateByID(t *testing.T) {
	const dup = "01JDUPDUPDUPDUPDUPDUPDUPDU"
	set := newTestSet(
		t,
		map[string]string{
			"notes/a.md":   "---\ntitle: A\nid: " + knownID + "\n---\n",
			"notes/dup.md": "---\nid: " + dup + "\n---\n",
			"notes/ref.md": "---\ntitle: R\n---\nmentions " + knownID + " in the body\n",
		},
		map[string]string{"notes/dup.md": "---\nid: " + dup + "\n---\n"},
	)
	tests := []struct {
		ref     string
		want    string
		wantErr error
	}{
		{ref: "id:" + knownID, want: "personal:notes/a.md"},
		{ref: "id:01JNOTTHERENOTTHERENOTTHER", wantErr: ErrNotFound},
		{ref: "id:" + dup, wantErr: ErrAmbiguous},
		{ref: "id:", wantErr: ErrInvalidPath},
	}
	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			got, err := set.Locate(tc.ref)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && got.String() != tc.want {
				t.Errorf("Locate = %s, want %s", got, tc.want)
			}
		})
	}

	report, err := set.IDs()
	if err != nil {
		t.Fatal(err)
	}
	if dups := report.DuplicateIDs(); len(dups) != 1 || dups[0] != dup {
		t.Errorf("duplicates = %v, want [%s]", dups, dup)
	}
	if report.Notes != 4 || report.Missing != 1 {
		t.Errorf("notes %d missing %d, want 4 and 1", report.Notes, report.Missing)
	}
}

func TestInsertIDLine(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"keys", "---\ntitle: A\n---\nbody\n", "---\ntitle: A\nid: X\n---\nbody\n"},
		{"empty block", "---\n---\nbody\n", "---\nid: X\n---\nbody\n"},
		{
			"crlf",
			"---\r\ntitle: A\r\n---\r\nbody\r\n",
			"---\r\ntitle: A\r\nid: X\r\n---\r\nbody\r\n",
		},
		{"fence at end of file", "---\ntitle: A\n---", "---\ntitle: A\nid: X\n---"},
		{"no frontmatter", "body\n", "---\nid: X\n---\nbody\n"},
		{"unterminated", "---\ntitle: A\n", "---\nid: X\n---\n---\ntitle: A\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(insertIDLine([]byte(tc.in), "X")); got != tc.want {
				t.Errorf("insertIDLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
