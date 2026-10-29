package local

import (
	"errors"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// readAll reads the log under dir from c in reads of maxBytes until a read
// returns nothing, and returns the edit ids in the order read.
func readAll(t *testing.T, dir string, c Cursor, maxBytes int) ([]string, Cursor) {
	t.Helper()
	var ids []string
	for range 1000 {
		lines, next, err := ReadLog(dir, c, maxBytes)
		if err != nil {
			t.Fatalf("ReadLog(%+v): %v", c, err)
		}
		if len(lines) == 0 {
			if next != c {
				t.Errorf("empty read moved the cursor from %+v to %+v", c, next)
			}
			return ids, next
		}
		for _, l := range lines {
			rec, err := l.Decode()
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			ids = append(ids, rec.(store.Edit).ID)
		}
		c = next
	}
	t.Fatal("ReadLog never reached the end of the log")
	return nil, c
}

func wantEdits(t *testing.T, got []string, from, to int) {
	t.Helper()
	if len(got) != to-from {
		t.Fatalf("read %d edits %q, want %d", len(got), got, to-from)
	}
	for i, id := range got {
		if want := testEdit(from + i).ID; id != want {
			t.Fatalf("edit %d = %s, want %s", i, id, want)
		}
	}
}

func TestReadLogReadsEveryLineOnce(t *testing.T) {
	cases := []struct {
		name     string
		segLimit int64
		maxBytes int
	}{
		{"one segment, one read", 0, 1 << 20},
		{"one segment, a line per read", 0, 1},
		{"rolled segments, one read", 600, 1 << 20},
		{"rolled segments, small reads", 600, 700},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			s := mustOpen(t, dir, c.segLimit)
			t.Cleanup(func() { mustClose(t, s) })
			appendEdits(t, s, 0, 12)

			got, end := readAll(t, dir, Cursor{}, c.maxBytes)
			wantEdits(t, got, 0, 12)

			appendEdits(t, s, 12, 15)
			got, _ = readAll(t, dir, end, c.maxBytes)
			wantEdits(t, got, 12, 15)
		})
	}
}

func TestReadLogLeavesAnAppendInProgress(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	mustClose(t, s)
	appendRaw(t, dir, 1, `{"t":"edit","r":{"id":"e001"`)

	got, end := readAll(t, dir, Cursor{}, 1<<20)
	wantEdits(t, got, 0, 1)

	appendRaw(t, dir, 1, "}}\n")
	got, _ = readAll(t, dir, end, 1<<20)
	wantEdits(t, got, 1, 2)
}

func TestReadLogUnknownKindDecodes(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	mustClose(t, s)
	appendRaw(t, dir, 1, `{"t":"future","r":{"id":"f1"}}`+"\n")

	lines, _, err := ReadLog(dir, Cursor{}, 1<<20)
	if err != nil || len(lines) != 1 {
		t.Fatalf("ReadLog = %d lines, %v; want 1", len(lines), err)
	}
	if _, err := lines[0].Decode(); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("Decode of an unknown kind = %v, want ErrUnknownKind", err)
	}
}
