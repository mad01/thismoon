package vault

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestWriteRaceHasOneWinner is two writers creating one note at once: one
// creates it, the other gets ErrExists, and the note is the winner's whole.
// The exclusive create decides it with or without the write lock.
func TestWriteRaceHasOneWinner(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "no lock", true: "write lock"}[locked], func(t *testing.T) {
			v := newTestVault(t, nil)
			if locked {
				v.WriteLock = filepath.Join(t.TempDir(), "locks", "v.write.lock")
			}
			raceWrites(t, v)
		})
	}
}

func raceWrites(t *testing.T, v *Vault) {
	t.Helper()
	bodies := []string{"first writer\n", "second writer\n"}
	errs := make([]error, len(bodies))
	var wg sync.WaitGroup
	for i, body := range bodies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = v.Write(WriteOptions{Tier: TierInbox, Title: "Race", Body: body})
		}()
	}
	wg.Wait()
	winner := -1
	for i, err := range errs {
		switch {
		case err == nil:
			if winner >= 0 {
				t.Fatalf("both writers created the note")
			}
			winner = i
		case !errors.Is(err, ErrExists):
			t.Errorf("writer %d: %v, want ErrExists", i, err)
		}
	}
	if winner < 0 {
		t.Fatal("no writer created the note")
	}
	_, body, err := v.ReadFrontmatter(filepath.Join(v.Root, "inbox", "race.md"))
	if err != nil || string(body) != bodies[winner] {
		t.Errorf("note body = %q, %v; want the winner's %q", body, err, bodies[winner])
	}
	entries, err := os.ReadDir(filepath.Join(v.Root, "inbox"))
	if err != nil || len(entries) != 1 {
		t.Errorf("inbox holds %v, want the note alone", entries)
	}
}

func TestEditIfMatch(t *testing.T) {
	v := newTestVault(t, map[string]string{"notes/a.md": "---\ntitle: A\n---\nbody\n"})
	full := filepath.Join(v.Root, "notes", "a.md")
	read, err := v.Read("notes/a.md", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := v.Edit(full, EditOptions{Append: "first", IfMatch: read.SHA})
	if err != nil {
		t.Fatalf("edit with the read sha: %v", err)
	}
	if sha == read.SHA {
		t.Error("edit returned the old sha")
	}
	before, _ := os.ReadFile(full)
	if _, err := v.Edit(full, EditOptions{Append: "stale", IfMatch: read.SHA}); !errors.Is(
		err,
		ErrConflict,
	) {
		t.Fatalf("edit with a stale sha: %v, want ErrConflict", err)
	}
	if after, _ := os.ReadFile(full); string(after) != string(before) {
		t.Error("a refused edit changed the note")
	}
	if _, err := v.Edit(full, EditOptions{Append: "second", IfMatch: sha}); err != nil {
		t.Errorf("edit with the sha the last edit returned: %v", err)
	}
}

// TestCreateCheckedRunsTheCheckUnderTheLock is two writers of one title
// under different file names: the second writer's check waits for the
// first note, sees it, and writes nothing.
func TestCreateCheckedRunsTheCheckUnderTheLock(t *testing.T) {
	v := newTestVault(t, nil)
	v.WriteLock = filepath.Join(t.TempDir(), "locks", "v.write.lock")
	draft := func(name string) Draft {
		d, err := v.Draft(WriteOptions{Tier: TierInbox, Title: "Same", Filename: name})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	first, second := draft("one"), draft("two")
	sameTitle := func() error {
		if found, err := NewSet(v).SameTitle("Same"); err != nil || len(found) > 0 {
			return errors.Join(ErrPossibleDuplicate, err)
		}
		return nil
	}
	var secondErr error
	done := make(chan struct{})
	_, err := v.CreateChecked(first, func() error {
		go func() {
			defer close(done)
			_, secondErr = v.CreateChecked(second, sameTitle)
		}()
		time.Sleep(50 * time.Millisecond) // the second writer reaches the lock meanwhile
		return sameTitle()
	})
	<-done
	if err != nil {
		t.Fatalf("first writer: %v", err)
	}
	if !errors.Is(secondErr, ErrPossibleDuplicate) {
		t.Errorf("second writer: %v, want ErrPossibleDuplicate", secondErr)
	}
	if _, err := os.Stat(filepath.Join(v.Root, "inbox", "two.md")); err == nil {
		t.Error("the second note landed")
	}
}
