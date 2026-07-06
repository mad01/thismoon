package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAccepts(t *testing.T) {
	good := `# comment
127.0.0.1	localhost
255.255.255.255 broadcasthost
::1             localhost

10.0.0.1 a.b.c alias1 alias2
`
	if err := Validate([]byte(good)); err != nil {
		t.Errorf("Validate rejected good file: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"bad ip":        "999.1.1.1 host\n",
		"hostname only": "127.0.0.1\n",
		"not an ip":     "hello world\n",
		"bad hostname":  "127.0.0.1 bad_host\n",
	}
	for name, body := range cases {
		if err := Validate([]byte(body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestSpliceAppendsThenReplacesIdempotent(t *testing.T) {
	base := "127.0.0.1\tlocalhost\n::1\tlocalhost\n"
	block1 := Render([]string{"csl.this", "p.this"})

	out1 := Splice([]byte(base), block1)
	if !strings.Contains(string(out1), base) {
		t.Errorf("foreign content not preserved:\n%s", out1)
	}
	if !strings.Contains(string(out1), "127.0.0.1\tcsl.this") {
		t.Errorf("managed host missing:\n%s", out1)
	}

	// Re-splicing identical hosts must be a no-op (idempotent).
	out2 := Splice(out1, block1)
	if string(out2) != string(out1) {
		t.Errorf("splice not idempotent:\nA=%q\nB=%q", out1, out2)
	}

	// Replacing with new hosts updates the block but keeps foreign content.
	block2 := Render([]string{"new.this"})
	out3 := Splice(out1, block2)
	if !strings.Contains(string(out3), base) {
		t.Errorf("foreign content lost on replace:\n%s", out3)
	}
	if strings.Contains(string(out3), "csl.this") {
		t.Errorf("old managed host not removed:\n%s", out3)
	}
	if !strings.Contains(string(out3), "new.this") {
		t.Errorf("new managed host missing:\n%s", out3)
	}
}

func TestSpliceResultIsValid(t *testing.T) {
	base := "127.0.0.1\tlocalhost\n"
	out := Splice([]byte(base), Render([]string{"csl.this"}))
	if err := Validate(out); err != nil {
		t.Errorf("spliced result invalid: %v", err)
	}
}

func writeHosts(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncWritesAndBacksUp(t *testing.T) {
	path := writeHosts(t, "127.0.0.1\tlocalhost\n")
	res, err := Sync(path, []string{"csl.this", "present.this"})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !res.Changed {
		t.Error("expected Changed=true")
	}
	if res.Backup == "" {
		t.Error("expected a backup path")
	}
	if _, err := os.Stat(res.Backup); err != nil {
		t.Errorf("backup not written: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "127.0.0.1\tcsl.this") ||
		!strings.Contains(string(got), "localhost") {
		t.Errorf("unexpected result:\n%s", got)
	}
}

func TestSyncIdempotent(t *testing.T) {
	path := writeHosts(t, "127.0.0.1\tlocalhost\n")
	if _, err := Sync(path, []string{"csl.this"}); err != nil {
		t.Fatal(err)
	}
	res, err := Sync(path, []string{"csl.this"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed {
		t.Error("second identical Sync should not change the file")
	}
}

func TestSyncPreservesPreexistingMalformedAndWarns(t *testing.T) {
	// Foreign malformed line present before d-man ran: must be preserved and
	// warned, never cause an abort.
	path := writeHosts(t, "127.0.0.1\tlocalhost\nthis line is broken\n")
	res, err := Sync(path, []string{"csl.this"})
	if err != nil {
		t.Fatalf("Sync aborted on pre-existing foreign breakage: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a warning about the malformed line")
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "this line is broken") {
		t.Error("foreign malformed line was not preserved")
	}
	if !strings.Contains(string(got), "csl.this") {
		t.Error("managed block not written")
	}
}
