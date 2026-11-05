package vault

import (
	"errors"
	"testing"
)

// newTestSet returns a set of a private personal vault and a shared team
// vault built from the given notes.
func newTestSet(t *testing.T, personal, team map[string]string) *Set {
	t.Helper()
	p := newTestVault(t, personal)
	p.Name, p.Default = "personal", true
	s := newTestVault(t, team)
	s.Name, s.Shared = "team", true
	return NewSet(p, s)
}

func TestSetLocate(t *testing.T) {
	set := newTestSet(
		t,
		map[string]string{
			"notes/a.md":    "---\ntitle: A\n---\n",
			"notes/both.md": "---\ntitle: P\n---\n",
		},
		map[string]string{
			"notes/t.md":    "---\ntitle: T\n---\n",
			"notes/both.md": "---\ntitle: T\n---\n",
		},
	)
	tests := []struct {
		ref     string
		want    string
		wantErr error
	}{
		{ref: "notes/a.md", want: "personal:notes/a.md"},
		{ref: "notes/t.md", want: "team:notes/t.md"},
		{ref: "team:notes/both.md", want: "team:notes/both.md"},
		{ref: "personal:./notes/both.md", want: "personal:notes/both.md"},
		{ref: "notes/both.md", wantErr: ErrAmbiguous},
		{ref: "notes/none.md", wantErr: ErrNotFound},
		{ref: "team:notes/a.md", wantErr: ErrNotFound},
		{ref: "nope:notes/a.md", wantErr: ErrUnknownVault},
		{ref: "personal:../x.md", wantErr: ErrInvalidPath},
		{ref: "notes", wantErr: ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			got, err := set.Locate(tc.ref)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Locate(%q) err = %v, want %v", tc.ref, err, tc.wantErr)
				}
				return
			}
			if err != nil || got.String() != tc.want {
				t.Fatalf("Locate(%q) = %v, %v; want %s", tc.ref, got, err, tc.want)
			}
		})
	}
}

func TestSetDefaultAndFilter(t *testing.T) {
	set := newTestSet(t, nil, nil)
	if v, err := set.Default(); err != nil || v.Name != "personal" {
		t.Errorf("Default = %v, %v", v, err)
	}
	shared, err := set.Filter([]string{"team"})
	if err != nil || len(shared.Vaults()) != 1 {
		t.Fatalf("Filter(team) = %v, %v", shared, err)
	}
	if _, err := shared.Default(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Default of shared vaults = %v, want ErrReadOnly", err)
	}
	if _, err := set.Filter([]string{"nope"}); !errors.Is(err, ErrUnknownVault) {
		t.Errorf("Filter(nope) = %v, want ErrUnknownVault", err)
	}
}

func TestSharedVaultIsReadOnly(t *testing.T) {
	set := newTestSet(t, nil, map[string]string{"inbox/x.md": "---\ntitle: X\n---\nbody\n"})
	team := set.Vaults()[1]
	if _, err := team.Write(WriteOptions{Tier: TierInbox, Title: "New"}); !errors.Is(
		err,
		ErrReadOnly,
	) {
		t.Errorf("Write = %v, want ErrReadOnly", err)
	}
	ref, err := set.Locate("team:inbox/x.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := team.Edit(ref.Abs(), EditOptions{Append: "y"}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Edit = %v, want ErrReadOnly", err)
	}
	if _, err := team.Promote(ref.Abs(), PromoteOptions{}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Promote = %v, want ErrReadOnly", err)
	}
}
