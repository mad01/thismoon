package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

func writeLog(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}
	return path
}

func testService() *service.Definition {
	return &service.Definition{
		Name:            "test-service",
		Command:         "/usr/bin/true",
		StandardOutPath: "/logs/stdout.log",
		StandardErrPath: "/logs/stderr.log",
		ExtraLogs:       map[string]string{"sandbox": "/logs/sandbox.log"},
	}
}

func TestResolveLogSources(t *testing.T) {
	svc := testService()

	tests := []struct {
		name       string
		source     string
		stdoutOnly bool
		stderrOnly bool
		wantNames  []string
		wantErr    bool
	}{
		{name: "default is both streams", wantNames: []string{"stdout", "stderr"}},
		{name: "stdout flag", stdoutOnly: true, wantNames: []string{"stdout"}},
		{name: "stderr flag", stderrOnly: true, wantNames: []string{"stderr"}},
		{name: "source stdout", source: "stdout", wantNames: []string{"stdout"}},
		{name: "source stderr", source: "stderr", wantNames: []string{"stderr"}},
		{name: "extra log source", source: "sandbox", wantNames: []string{"sandbox"}},
		{name: "unknown source", source: "nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources, err := resolveLogSources(svc, tt.source, tt.stdoutOnly, tt.stderrOnly)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveLogSources() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			var names []string
			for _, s := range sources {
				names = append(names, s.name)
			}
			if strings.Join(names, ",") != strings.Join(tt.wantNames, ",") {
				t.Errorf("sources = %v, want %v", names, tt.wantNames)
			}
		})
	}
}

func TestResolveLogSources_UnknownSourceListsAvailable(t *testing.T) {
	_, err := resolveLogSources(testService(), "nope", false, false)
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
	for _, want := range []string{"stdout", "stderr", "sandbox"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should list available source %q", err, want)
		}
	}
}

func TestLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := writeLog(t, dir, "out.log", "one\ntwo\nthree\nfour\n")

	lines, err := lastNLines(path, 2)
	if err != nil {
		t.Fatalf("lastNLines() error: %v", err)
	}
	if len(lines) != 2 || lines[0] != "three" || lines[1] != "four" {
		t.Errorf("lastNLines() = %v, want [three four]", lines)
	}

	all, err := lastNLines(path, 100)
	if err != nil {
		t.Fatalf("lastNLines() error: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected all 4 lines when n exceeds file length, got %d", len(all))
	}
}

func TestTailSources_HeadersAndContent(t *testing.T) {
	dir := t.TempDir()
	out := writeLog(t, dir, "stdout.log", "out line\n")
	errLog := writeLog(t, dir, "stderr.log", "err line\n")

	var buf bytes.Buffer
	p := newSourcePrinter(&buf, true)
	sources := []logSource{{name: "stdout", path: out}, {name: "stderr", path: errLog}}

	if err := tailSources(p, sources, 50); err != nil {
		t.Fatalf("tailSources() error: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"==> stdout <==", "out line", "==> stderr <==", "err line"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestTailSources_SingleSourceNoHeader(t *testing.T) {
	dir := t.TempDir()
	out := writeLog(t, dir, "stdout.log", "just a line\n")

	var buf bytes.Buffer
	p := newSourcePrinter(&buf, false)

	if err := tailSources(p, []logSource{{name: "stdout", path: out}}, 50); err != nil {
		t.Fatalf("tailSources() error: %v", err)
	}
	if strings.Contains(buf.String(), "==>") {
		t.Errorf("single source should not print headers:\n%s", buf.String())
	}
}

func TestTailSources_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	out := writeLog(t, dir, "stdout.log", "out line\n")
	missing := filepath.Join(dir, "stderr.log")

	// one of several missing: skipped
	var buf bytes.Buffer
	p := newSourcePrinter(&buf, true)
	sources := []logSource{{name: "stdout", path: out}, {name: "stderr", path: missing}}
	if err := tailSources(p, sources, 50); err != nil {
		t.Fatalf("tailSources() with one missing file should succeed, got: %v", err)
	}

	// sole source missing: error
	p = newSourcePrinter(&buf, false)
	if err := tailSources(p, []logSource{{name: "stderr", path: missing}}, 50); err == nil {
		t.Error("expected error when the only source is missing")
	}
}

func TestTailStateDrain(t *testing.T) {
	dir := t.TempDir()
	path := writeLog(t, dir, "out.log", "first\n")

	var buf bytes.Buffer
	p := newSourcePrinter(&buf, false)
	st := &tailState{src: logSource{name: "stdout", path: path}}

	// initial drain reads from offset 0
	if err := st.drain(p); err != nil {
		t.Fatalf("drain() error: %v", err)
	}
	if got := buf.String(); got != "first\n" {
		t.Errorf("drain() output = %q, want %q", got, "first\n")
	}

	// appended data, including a partial line held until its newline arrives
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open for append: %v", err)
	}
	if _, err := f.WriteString("second\npart"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
	_ = f.Close()

	buf.Reset()
	if err := st.drain(p); err != nil {
		t.Fatalf("drain() error: %v", err)
	}
	if got := buf.String(); got != "second\n" {
		t.Errorf("drain() output = %q, want %q (partial line buffered)", got, "second\n")
	}

	// truncation resets to the start
	if err := os.WriteFile(path, []byte("fresh\n"), 0o644); err != nil {
		t.Fatalf("failed to truncate: %v", err)
	}
	buf.Reset()
	if err := st.drain(p); err != nil {
		t.Fatalf("drain() error: %v", err)
	}
	if got := buf.String(); got != "fresh\n" {
		t.Errorf("drain() after truncation = %q, want %q", got, "fresh\n")
	}
}

func TestIsSandboxSource(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"sandbox", true},
		{"sandbox-notifications", true},
		{"sandboxy", false},
		{"denials", false},
		{"stdout", false},
	}
	for _, tt := range tests {
		if got := isSandboxSource(tt.name); got != tt.want {
			t.Errorf("isSandboxSource(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSandboxSources(t *testing.T) {
	svcs := []*service.Definition{
		{
			Name: "speak-tts",
			ExtraLogs: map[string]string{
				"sandbox": "/logs/sandbox-denials.log",
				"audit":   "/logs/audit.log", // not sandbox-named, excluded
			},
		},
		{
			Name: "sandbox-watch",
			ExtraLogs: map[string]string{
				"sandbox":               "/logs/sandbox-denials.log", // same path, deduped
				"sandbox-notifications": "/logs/sandbox-notifications.log",
			},
		},
		{Name: "plain"}, // no extra logs
	}

	fleet := sandboxSources(svcs, true)
	if len(fleet) != 2 {
		t.Fatalf("fleet sources = %v, want 2 (deduped by path)", fleet)
	}
	for _, src := range fleet {
		if src.name != src.path {
			t.Errorf(
				"fleet label %q should be the path %q (no home prefix to shorten)",
				src.name,
				src.path,
			)
		}
	}

	single := sandboxSources(svcs[1:2], false)
	if len(single) != 2 {
		t.Fatalf("single-service sources = %v, want 2", single)
	}
	if single[0].name != "sandbox" || single[1].name != "sandbox-notifications" {
		t.Errorf(
			"single-service labels = [%s %s], want source names",
			single[0].name,
			single[1].name,
		)
	}
}

func TestDisplayPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := displayPath(filepath.Join(home, "logs/x.log")); got != "~/logs/x.log" {
		t.Errorf("displayPath(home path) = %q, want ~/logs/x.log", got)
	}
	if got := displayPath("/var/log/x.log"); got != "/var/log/x.log" {
		t.Errorf("displayPath(/var/log/x.log) = %q", got)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	got, err := expandPath("~/logs/x.log")
	if err != nil {
		t.Fatalf("expandPath() error: %v", err)
	}
	if got != filepath.Join(home, "logs/x.log") {
		t.Errorf("expandPath(~/logs/x.log) = %q", got)
	}

	abs, err := expandPath("/var/log/x.log")
	if err != nil {
		t.Fatalf("expandPath() error: %v", err)
	}
	if abs != "/var/log/x.log" {
		t.Errorf("expandPath(/var/log/x.log) = %q", abs)
	}
}
