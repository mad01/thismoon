package clip

import (
	"errors"
	"strings"
	"testing"
)

func TestCopySendsTextToPbcopy(t *testing.T) {
	var gotBin, gotStdin string
	b := &Board{run: func(bin, stdin string) (string, error) {
		gotBin, gotStdin = bin, stdin
		return "", nil
	}}
	if err := b.Copy("hello\nworld\n"); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if gotBin != copyBin {
		t.Errorf("bin = %q, want %q", gotBin, copyBin)
	}
	if gotStdin != "hello\nworld\n" {
		t.Errorf("stdin = %q, want the text verbatim", gotStdin)
	}
}

func TestPasteReturnsPbpasteOutput(t *testing.T) {
	b := &Board{run: func(bin, stdin string) (string, error) {
		if bin != pasteBin {
			t.Errorf("bin = %q, want %q", bin, pasteBin)
		}
		if stdin != "" {
			t.Errorf("stdin = %q, want empty", stdin)
		}
		return "clip contents", nil
	}}
	got, err := b.Paste()
	if err != nil {
		t.Fatalf("Paste() error = %v", err)
	}
	if got != "clip contents" {
		t.Errorf("Paste() = %q, want %q", got, "clip contents")
	}
}

func TestErrorsCarryDocsHint(t *testing.T) {
	boom := errors.New("boom")
	b := &Board{run: func(string, string) (string, error) { return "", boom }}
	err := b.Copy("x")
	if !errors.Is(err, boom) {
		t.Fatalf("Copy() error = %v, want it to wrap the runner error", err)
	}
	if !strings.Contains(err.Error(), "clipboard docs") {
		t.Errorf("error %q does not point at 'clipboard docs'", err)
	}
}
