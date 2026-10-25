package local

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const tinySegment = 1024

func segmentSize(t *testing.T, dir string, n int) int64 {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, logDirName, segmentName(n)))
	if err != nil {
		t.Fatalf("stat segment %d: %v", n, err)
	}
	return info.Size()
}

func appendRaw(t *testing.T, dir string, n int, data string) {
	t.Helper()
	path := filepath.Join(dir, logDirName, segmentName(n))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatalf("append: %v", err)
	}
}

func TestSegmentsRoll(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, tinySegment)
	appendEdits(t, s, 0, 40)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	nums, err := listSegments(filepath.Join(dir, logDirName))
	if err != nil {
		t.Fatalf("listSegments: %v", err)
	}
	if len(nums) < 2 {
		t.Fatalf("segments = %v after 40 edits at %d bytes each, want several", nums, tinySegment)
	}
	for i, n := range nums {
		if n != i+1 {
			t.Errorf("segment numbers = %v, want 1..%d", nums, len(nums))
		}
		if size := segmentSize(t, dir, n); size > tinySegment {
			t.Errorf("segment %d is %d bytes, over the %d limit", n, size, tinySegment)
		}
	}

	s = mustOpen(t, dir, 0)
	t.Cleanup(func() { mustClose(t, s) })
	if n := editCount(t, s); n != 40 {
		t.Errorf("edits after reopen = %d, want 40", n)
	}
	appendEdits(t, s, 40, 41)
	if again, _ := listSegments(filepath.Join(dir, logDirName)); len(again) != len(nums) {
		t.Errorf("segments after one more edit at the default limit = %v, want %v", again, nums)
	}
}

func TestTornTailIsDropped(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 2)
	mustClose(t, s)
	good := segmentSize(t, dir, 1)
	appendRaw(t, dir, 1, `{"t":"edit","r":{"id":"e9","sess`)
	logged := captureLog(t)

	s = mustOpen(t, dir, 0)
	if len(*logged) != 1 || !strings.Contains((*logged)[0], "torn") {
		t.Errorf("logged %q, want one torn-line warning", *logged)
	}
	if n := editCount(t, s); n != 2 {
		t.Errorf("edits after repair = %d, want 2", n)
	}
	if size := segmentSize(t, dir, 1); size != good {
		t.Errorf("segment after repair is %d bytes, want the %d before the torn line", size, good)
	}
	appendEdits(t, s, 2, 3)
	mustClose(t, s)

	*logged = nil
	s = mustOpen(t, dir, 0)
	t.Cleanup(func() { mustClose(t, s) })
	if n := editCount(t, s); n != 3 || len(*logged) != 0 {
		t.Errorf(
			"after an append past the repair: %d edits, logged %q; want 3 and nothing",
			n,
			*logged,
		)
	}
}

func TestUnterminatedTailIsKept(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	mustClose(t, s)
	path := filepath.Join(dir, logDirName, segmentName(1))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	line := strings.Replace(string(raw), `"e000"`, `"e001"`, 1)
	appendRaw(t, dir, 1, strings.TrimSuffix(line, "\n"))

	s = mustOpen(t, dir, 0)
	if n := editCount(t, s); n != 2 {
		t.Errorf("edits with an unterminated last line = %d, want 2", n)
	}
	appendEdits(t, s, 2, 3)
	mustClose(t, s)
	s = mustOpen(t, dir, 0)
	t.Cleanup(func() { mustClose(t, s) })
	if n := editCount(t, s); n != 3 {
		t.Errorf("edits after appending past it = %d, want 3", n)
	}
}

func TestDuplicateLineReplaysOnce(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	mustClose(t, s)
	raw, err := os.ReadFile(filepath.Join(dir, logDirName, segmentName(1)))
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	appendRaw(t, dir, 1, strings.Replace(string(raw), `"x.go"`, `"y.go"`, 1))

	s = mustOpen(t, dir, 0)
	t.Cleanup(func() { mustClose(t, s) })
	edits, err := s.Edits(t.Context(), store.EditQuery{})
	if err != nil {
		t.Fatalf("Edits: %v", err)
	}
	if len(edits) != 1 || edits[0].Path != "x.go" {
		t.Errorf("edits after a duplicate line = %+v, want e000 once with its first path", edits)
	}
}

func TestBadLineFailsOpen(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, dir string)
	}{
		{"garbage mid segment", func(t *testing.T, dir string) {
			appendRaw(t, dir, 1, "not json\n")
			appendRaw(t, dir, 1, `{"t":"edit","r":{"id":"late"}}`+"\n")
		}},
		{"garbage last line with newline", func(t *testing.T, dir string) {
			appendRaw(t, dir, 1, "{\"t\":\"edit\"\n")
		}},
		{"torn tail of an earlier segment", func(t *testing.T, dir string) {
			appendRaw(t, dir, 1, `{"t":"edit","r":{"id":"e9`)
			if err := os.WriteFile(filepath.Join(dir, logDirName, segmentName(2)), nil, filePerm); err != nil {
				t.Fatal(err)
			}
		}},
		{"record that does not decode", func(t *testing.T, dir string) {
			appendRaw(t, dir, 1, `{"t":"edit","r":{"id":5}}`+"\n")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			s := mustOpen(t, dir, 0)
			appendEdits(t, s, 0, 1)
			mustClose(t, s)
			c.damage(t, dir)
			if s, err := Open(dir); err == nil {
				mustClose(t, s)
				t.Fatal("Open succeeded on a damaged log, want an error")
			}
		})
	}
}

func TestUnknownKindIsSkipped(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	mustClose(t, s)
	appendRaw(t, dir, 1, `{"t":"future","r":{"id":"f1"}}`+"\n")
	logged := captureLog(t)

	s = mustOpen(t, dir, 0)
	t.Cleanup(func() { mustClose(t, s) })
	if n := editCount(t, s); n != 1 {
		t.Errorf("edits = %d, want 1", n)
	}
	if len(*logged) != 1 || !strings.Contains((*logged)[0], `"future"`) {
		t.Errorf("logged %q, want one warning naming the kind", *logged)
	}
}

func TestLogLineShape(t *testing.T) {
	dir := t.TempDir()
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	mustClose(t, s)
	raw, err := os.ReadFile(filepath.Join(dir, logDirName, segmentName(1)))
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte(`{"t":"edit","r":{"id":"e000",`)) ||
		!bytes.HasSuffix(raw, []byte("}\n")) {
		t.Errorf("segment = %s, want one {\"t\":\"edit\",\"r\":{...}} line", raw)
	}
}
