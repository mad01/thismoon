package settings

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestValueTable checks each key takes exactly the set the kit accepts.
func TestValueTable(t *testing.T) {
	tests := []struct {
		key    string
		accept []string
		refuse []string
	}{
		{
			key:    "webkit-theme",
			accept: []string{"light", "dark", "system"},
			refuse: []string{"", "Dark", "auto", "default"},
		},
		{
			key:    "webkit-palette-light",
			accept: families,
			refuse: []string{"", "dracula", "Nord", "rose pine"},
		},
		{
			key:    "webkit-palette-dark",
			accept: families,
			refuse: []string{"", "dracula", "TOKYO-NIGHT"},
		},
		{
			key:    "webkit-font",
			accept: []string{"Fira Code", "Inter", "Lexend", "Work Sans"},
			refuse: []string{"", "fira code", "Comic Sans", "Inter "},
		},
		{
			key:    "webkit-size",
			accept: []string{"12", "16", "18", "24"},
			refuse: []string{"", "11", "25", "16.5", "016", "+16", " 16", "1e1", "sixteen"},
		},
		{
			key:    "webkit-fixation",
			accept: []string{"true", "false"},
			refuse: []string{"", "1", "TRUE", "yes"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if !Known(tt.key) {
				t.Fatalf("Known(%q) = false", tt.key)
			}
			for _, v := range tt.accept {
				if !Valid(tt.key, v) {
					t.Errorf("Valid(%q, %q) = false, want true", tt.key, v)
				}
			}
			for _, v := range tt.refuse {
				if Valid(tt.key, v) {
					t.Errorf("Valid(%q, %q) = true, want false", tt.key, v)
				}
			}
		})
	}
	if got, want := len(valueTable), len(tests); got != want {
		t.Errorf("value table has %d keys, test covers %d", got, want)
	}
}

func TestUnknownKeysAreNotKnown(t *testing.T) {
	for _, key := range []string{"", "webkit-ra-speed", "webkit-audio", "webkit-palette-", "theme"} {
		if Known(key) || Valid(key, "light") {
			t.Errorf("key %q is accepted, want it dropped", key)
		}
	}
}

func openTemp(t *testing.T) (*Store, string, *bytes.Buffer) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ui-settings.json")
	var buf bytes.Buffer
	return Open(path, log.New(&buf, "", 0)), path, &buf
}

func TestPatchMergesAndSurvivesReopen(t *testing.T) {
	s, path, _ := openTemp(t)
	if _, err := s.Patch(map[string]string{"webkit-theme": "dark", "webkit-size": "18"}); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Patch(map[string]string{"webkit-theme": "light", "future-key": "x"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"webkit-theme": "light", "webkit-size": "18"}
	if doc.V != Version || !maps.Equal(doc.Settings, want) {
		t.Errorf("Patch = %+v, want v%d %v", doc, Version, want)
	}

	var buf bytes.Buffer
	reopened := Open(path, log.New(&buf, "", 0)).Document()
	if !maps.Equal(reopened.Settings, want) {
		t.Errorf("after reopen = %v, want %v", reopened.Settings, want)
	}
	if buf.Len() != 0 {
		t.Errorf("reopen logged %q, want nothing", buf.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "\n  \"settings\"") {
		t.Errorf("file is not indented:\n%s", data)
	}
	assertOnlyFile(t, filepath.Dir(path), filepath.Base(path))
}

func TestPatchRefusesBadValueWithoutChange(t *testing.T) {
	s, path, _ := openTemp(t)
	if _, err := s.Patch(map[string]string{"webkit-theme": "dark"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	_, err := s.Patch(map[string]string{"webkit-font": "Inter", "webkit-size": "25"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Patch err = %v, want ErrInvalid", err)
	}
	if got := s.Document().Settings; !maps.Equal(got, map[string]string{"webkit-theme": "dark"}) {
		t.Errorf("settings = %v, want unchanged", got)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Errorf("file changed:\n%s\nwant\n%s", after, before)
	}
}

// A path that is a directory opens empty and can never be saved: the rename
// over it fails, which is how a failed save is forced here even as root.
func TestFailedSaveLeavesMemoryUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ui-settings.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	s := Open(path, log.New(&buf, "", 0))
	if !strings.Contains(buf.String(), "starting empty") {
		t.Errorf("open logged %q, want the unreadable file reported", buf.String())
	}
	if _, err := s.Patch(map[string]string{"webkit-theme": "dark"}); err == nil ||
		errors.Is(err, ErrInvalid) {
		t.Fatalf("Patch err = %v, want a save error", err)
	}
	if got := s.Document().Settings; len(got) != 0 {
		t.Errorf("settings = %v, want empty after a failed save", got)
	}
	assertOnlyFile(t, dir, "ui-settings.json")
}

func TestOpenMissingFileIsEmpty(t *testing.T) {
	s, path, buf := openTemp(t)
	doc := s.Document()
	if doc.V != Version || doc.Settings == nil || len(doc.Settings) != 0 {
		t.Errorf("Document = %#v, want v%d with an empty, non-nil map", doc, Version)
	}
	if buf.Len() != 0 {
		t.Errorf("logged %q, want nothing for a missing file", buf.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Open created %s; it should only read", path)
	}
}

func TestOpenCorruptFileIsEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"not json":      "{not json",
		"wrong version": `{"v":2,"settings":{"webkit-theme":"dark"}}`,
		"wrong type":    `{"v":1,"settings":{"webkit-size":18}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ui-settings.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			s := Open(path, log.New(&buf, "", 0))
			if got := s.Document().Settings; len(got) != 0 {
				t.Errorf("settings = %v, want empty", got)
			}
			if !strings.Contains(buf.String(), "starting empty") {
				t.Errorf("logged %q, want the corrupt file reported", buf.String())
			}
		})
	}
}

func TestOpenDropsRefusedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui-settings.json")
	body := `{"v":1,"settings":{"webkit-theme":"dark","webkit-size":"99","other":"x"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	got := Open(path, log.New(&buf, "", 0)).Document().Settings
	if !maps.Equal(got, map[string]string{"webkit-theme": "dark"}) {
		t.Errorf("settings = %v, want only the valid entry", got)
	}
	if !strings.Contains(buf.String(), "webkit-size") {
		t.Errorf("logged %q, want the dropped entry named", buf.String())
	}
}

func TestDocumentIsACopy(t *testing.T) {
	s, _, _ := openTemp(t)
	if _, err := s.Patch(map[string]string{"webkit-theme": "dark"}); err != nil {
		t.Fatal(err)
	}
	s.Document().Settings["webkit-theme"] = "light"
	if got := s.Document().Settings["webkit-theme"]; got != "dark" {
		t.Errorf("store changed through a returned document: %q", got)
	}
}

// Run with -race: concurrent patches on distinct keys must all land, in
// memory and on disk.
func TestParallelPatches(t *testing.T) {
	s, path, _ := openTemp(t)
	patches := []map[string]string{
		{"webkit-theme": "system"},
		{"webkit-palette-light": "nord"},
		{"webkit-palette-dark": "tokyo-night"},
		{"webkit-font": "Lexend"},
		{"webkit-size": "20"},
		{"webkit-fixation": "true"},
	}
	var wg sync.WaitGroup
	for round := range 5 {
		for _, p := range patches {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.Patch(p); err != nil {
					t.Errorf("round %d: Patch(%v): %v", round, p, err)
				}
				_ = s.Document()
			}()
		}
	}
	wg.Wait()

	want := map[string]string{}
	for _, p := range patches {
		maps.Copy(want, p)
	}
	if got := s.Document().Settings; !maps.Equal(got, want) {
		t.Errorf("memory = %v, want %v", got, want)
	}
	var buf bytes.Buffer
	if got := Open(path, log.New(&buf, "", 0)).Document().Settings; !maps.Equal(got, want) {
		t.Errorf("disk = %v, want %v", got, want)
	}
	assertOnlyFile(t, filepath.Dir(path), filepath.Base(path))
}

// assertOnlyFile fails when dir holds anything but name: a temp file left
// behind by a save.
func assertOnlyFile(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if fmt.Sprint(names) != fmt.Sprint([]string{name}) {
		t.Errorf("%s holds %v, want only %s", dir, names, name)
	}
}
