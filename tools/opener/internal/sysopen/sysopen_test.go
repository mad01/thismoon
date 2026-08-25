package sysopen

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeOpener records the args the runner received and returns err.
func fakeOpener(gotArgs *[]string, err error) *Opener {
	return &Opener{run: func(args ...string) error {
		*gotArgs = append([]string{}, args...)
		return err
	}}
}

func TestURLPassesThrough(t *testing.T) {
	var got []string
	o := fakeOpener(&got, nil)
	opened, err := o.URL("https://example.com/x?y=1")
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	if opened != "https://example.com/x?y=1" {
		t.Errorf("opened = %q, want the URL back", opened)
	}
	if want := []string{"https://example.com/x?y=1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestURLRejectsMissingScheme(t *testing.T) {
	var got []string
	o := fakeOpener(&got, nil)
	for _, raw := range []string{"example.com", "/tmp/x", "-R"} {
		if _, err := o.URL(raw); err == nil {
			t.Errorf("URL(%q) = nil error, want a no-scheme rejection", raw)
		}
	}
	if got != nil {
		t.Errorf("open ran with %v, want no exec on rejected input", got)
	}
}

func TestFileResolvesAndStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string
	o := fakeOpener(&got, nil)
	opened, err := o.File(path)
	if err != nil {
		t.Fatalf("File() error = %v", err)
	}
	if opened != path {
		t.Errorf("opened = %q, want %q", opened, path)
	}
	if want := []string{path}; !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestFileRejectsRelativeAndMissing(t *testing.T) {
	var got []string
	o := fakeOpener(&got, nil)
	if _, err := o.File("doc.txt"); err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Errorf("File(relative) error = %v, want the not-absolute rejection", err)
	}
	missing := filepath.Join(t.TempDir(), "nope.txt")
	if _, err := o.File(missing); err == nil {
		t.Error("File(missing) = nil error, want a stat failure")
	}
	if got != nil {
		t.Errorf("open ran with %v, want no exec on rejected input", got)
	}
}

func TestAppAndWithBuildFlagArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got []string
	o := fakeOpener(&got, nil)

	if _, err := o.App("Safari"); err != nil {
		t.Fatalf("App() error = %v", err)
	}
	if want := []string{"-a", "Safari"}; !reflect.DeepEqual(got, want) {
		t.Errorf("App args = %v, want %v", got, want)
	}

	if _, err := o.With(path, "TextEdit"); err != nil {
		t.Fatalf("With() error = %v", err)
	}
	if want := []string{"-a", "TextEdit", path}; !reflect.DeepEqual(got, want) {
		t.Errorf("With args = %v, want %v", got, want)
	}

	if _, err := o.Reveal(path); err != nil {
		t.Fatalf("Reveal() error = %v", err)
	}
	if want := []string{"-R", path}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reveal args = %v, want %v", got, want)
	}
}

func TestRevealTildeExpands(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	var got []string
	o := fakeOpener(&got, nil)
	opened, err := o.Reveal("~")
	if err != nil {
		t.Fatalf("Reveal(~) error = %v", err)
	}
	if opened != home {
		t.Errorf("opened = %q, want %q", opened, home)
	}
}

func TestErrorsCarryDocsHint(t *testing.T) {
	boom := errors.New("boom")
	var got []string
	o := fakeOpener(&got, boom)
	_, err := o.App("Safari")
	if !errors.Is(err, boom) {
		t.Fatalf("App() error = %v, want it to wrap the runner error", err)
	}
	if !strings.Contains(err.Error(), "opener docs") {
		t.Errorf("error %q does not point at 'opener docs'", err)
	}
}
