package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/tools/harvest/internal/sandbox"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

var fixed = time.Date(2026, 11, 14, 9, 30, 0, 0, time.Local)

// fakeBackend stands in for the model; it answers with reply and counts
// its calls.
type fakeBackend struct {
	reply string
	calls int
}

func (f *fakeBackend) Complete(context.Context, string, string) (string, error) {
	f.calls++
	return f.reply, nil
}
func (f *fakeBackend) Name() string  { return "fake" }
func (f *fakeBackend) Model() string { return "fake-model" }

// testApp is an app with a temp HOME and state dir, a skill file, a fake
// model with nothing to keep, a brain that does not exist, and steps that
// run in-process as `harvest step ...` would.
func testApp(t *testing.T, env map[string]string) (*app, string) {
	t.Helper()
	home := t.TempDir()
	stateDir := filepath.Join(t.TempDir(), "state")
	skill := filepath.Join(home, "SKILL.md")
	if err := os.WriteFile(skill, []byte("# harvest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := map[string]string{"HOME": home, "HARVEST_STATE_DIR": stateDir, "HARVEST_SKILL": skill}
	for k, v := range env {
		full[k] = v
	}
	model := &fakeBackend{reply: `{"captures":[],"dropped":[]}`}
	a := &app{
		getenv: func(k string) string { return full[k] },
		environ: func() []string {
			var out []string
			for k, v := range full {
				out = append(out, k+"="+v)
			}
			return out
		},
		now:           func() time.Time { return fixed },
		selectBackend: func(llm.Options) (llm.Backend, error) { return model, nil },
		brain:         filepath.Join(t.TempDir(), "no-such-brain"),
	}
	a.runStep = func(_ context.Context, s sandbox.Step) error {
		// The argv the sandbox starts the step with, so its -- goes
		// through cobra too.
		_, _, err := execute(t, a, "", sandbox.Build(sandbox.Config{}, s).Args...)
		return err
	}
	return a, stateDir
}

func execute(t *testing.T, a *app, stdin string, args ...string) (string, string, error) {
	t.Helper()
	c := a.root()
	var out, errOut bytes.Buffer
	c.SetIn(strings.NewReader(stdin))
	c.SetOut(&out)
	c.SetErr(&errOut)
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), errOut.String(), err
}

// writeTranscript writes a one-line transcript past a 10-byte floor.
func writeTranscript(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session.jsonl")
	line := `{"type":"user","message":{"content":"a question long enough to pass the floor"}}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readLog(t *testing.T, stateDir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(stateDir, "harvest.log"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(b)
}

// TestSessionEndStartsARun runs the started command in-process in place of
// the detached child, so the hook's handoff to `harvest run` and the run's
// steps are covered end to end: a finished harvest shows as a mark.
func TestSessionEndStartsARun(t *testing.T) {
	a, stateDir := testApp(t, map[string]string{"HARVEST_MIN_BYTES": "10"})
	transcript := writeTranscript(t)
	var runErr error
	a.start = func(args []string) error {
		_, _, runErr = execute(t, a, "", args...)
		return nil
	}

	payload := fmt.Sprintf(`{"session_id":"hook-1","transcript_path":%q,"cwd":"/repo"}`, transcript)
	if _, stderr, err := execute(t, a, payload, "session-end"); err != nil || stderr != "" {
		t.Fatalf("session-end = %v, stderr %q", err, stderr)
	}
	if runErr != nil {
		t.Fatalf("the started run failed: %v", runErr)
	}
	n, err := state.New(stateDir, time.Now).Mark("hook-1")
	if err != nil || n != 1 {
		t.Errorf("mark = %d, %v; want 1: the hook did not start a run", n, err)
	}
	log := readLog(t, stateDir)
	if !strings.Contains(log, "done hook-1: model=fake-model digest=") ||
		!strings.Contains(log, "backend=fake sandbox=off\n  captured: none\n") {
		t.Errorf("log lacks the done line:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "jobs", "hook-1")); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Errorf("the job dir outlived the run: %v", err)
	}
}

func TestSessionEndAlwaysExitsZero(t *testing.T) {
	a, _ := testApp(t, map[string]string{"HARVEST_STATE_DIR": "relative"})
	_, stderr, err := execute(t, a, "{}", "session-end")
	if err != nil {
		t.Fatalf("session-end error = %v, want nil whatever happens", err)
	}
	if !strings.Contains(stderr, "must be an absolute path") {
		t.Errorf("stderr = %q, want the cause", stderr)
	}
}

// TestRunFailsClosed uses the real step launcher with the sandbox on and no
// launcher under the temp HOME: the run must stop before any step starts,
// never run one unconfined.
func TestRunFailsClosed(t *testing.T) {
	a, stateDir := testApp(t, map[string]string{"HARVEST_MIN_BYTES": "10"})
	a.runStep = nil
	_, _, err := execute(t, a, "", "run", writeTranscript(t), "s1", "/repo")
	if err == nil {
		t.Fatal("run without the launcher returned nil")
	}
	if log := readLog(t, stateDir); !strings.Contains(
		log,
		"fail s1: prepare: sandbox: launcher: ",
	) {
		t.Errorf("log = %q, want the fail-closed line", log)
	}
	if n, _ := state.New(stateDir, time.Now).Mark("s1"); n != 0 {
		t.Errorf("mark = %d, want none", n)
	}
}

func TestRunLogsBadSettings(t *testing.T) {
	a, stateDir := testApp(t, map[string]string{"HARVEST_MAX_BYTES": "lots"})
	_, _, err := execute(t, a, "", "run", "/t.jsonl", "s1", "/repo")
	if err == nil {
		t.Fatal("run with a bad setting returned nil")
	}
	if log := readLog(t, stateDir); !strings.Contains(
		log,
		`fail s1: settings: HARVEST_MAX_BYTES="lots"`,
	) {
		t.Errorf("log = %q, want the settings failure", log)
	}
}

// TestRunTakesArgumentsThatLookLikeFlags runs `run --` with a session id and
// a cwd that start with -, as the hook starts it; the steps get them after
// their own --.
func TestRunTakesArgumentsThatLookLikeFlags(t *testing.T) {
	a, stateDir := testApp(t, map[string]string{"HARVEST_MIN_BYTES": "10"})
	_, _, err := execute(t, a, "", "run", "--", writeTranscript(t), "-s1", "--cwd")
	if err != nil {
		t.Fatalf("run -- error = %v", err)
	}
	if n, err := state.New(stateDir, time.Now).Mark("-s1"); err != nil || n != 1 {
		t.Errorf("mark = %d, %v; want 1", n, err)
	}
}

func TestRunQuotesABadSessionIDInTheLog(t *testing.T) {
	a, stateDir := testApp(t, map[string]string{"HARVEST_MAX_BYTES": "lots"})
	if _, _, err := execute(t, a, "", "run", "/t.jsonl", "s1\nforged", "/repo"); err == nil {
		t.Fatal("run with a bad setting returned nil")
	}
	if log := readLog(t, stateDir); !strings.Contains(log, `fail "s1\nforged": settings:`) {
		t.Errorf("log = %q, want the id quoted", log)
	}
}

func TestRunNeedsThreeArguments(t *testing.T) {
	a, _ := testApp(t, nil)
	if _, _, err := execute(t, a, "", "run", "/t.jsonl", "s1"); err == nil {
		t.Error("run with two arguments returned nil")
	}
}

func TestStepIsHidden(t *testing.T) {
	a, _ := testApp(t, nil)
	out, _, err := execute(t, a, "", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\n  step ") {
		t.Errorf("--help lists the hidden step group:\n%s", out)
	}
	out, _, err = execute(t, a, "", "step", "--help")
	if err != nil || !strings.Contains(out, "harvest run starts") {
		t.Errorf("step --help = %q, %v; want it to say harvest run starts the steps", out, err)
	}
}

func TestHintStaysQuietOnFailure(t *testing.T) {
	a, _ := testApp(t, nil) // a.brain does not exist
	out, stderr, err := execute(t, a, `{"event":"session-start"}`, "hint")
	if err != nil || out != "" {
		t.Errorf("hint = %q, %v; want no output and exit 0", out, err)
	}
	if !strings.Contains(stderr, "harvest hint:") {
		t.Errorf("stderr = %q, want the cause", stderr)
	}
}

func TestDoctorShowsTheChecksAndTheLog(t *testing.T) {
	a, stateDir := testApp(t, nil)
	if err := state.New(stateDir, a.now).Log("skip s1: nothing new since line 4"); err != nil {
		t.Fatal(err)
	}
	out, _, err := execute(t, a, "", "doctor")
	if err == nil {
		t.Error("doctor passed with no brain and no launcher")
	}
	for _, want := range []string{
		"ok settings\n",
		"ok state dir writable",
		"ok skill file (",
		"FAIL brain on PATH",
		"ok model backend fake, model fake-model\n",
		"FAIL sandbox (launcher ",
		"last log lines (" + filepath.Join(stateDir, "harvest.log") + "):\n",
		"skip s1: nothing new since line 4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
}

func TestDoctorWithTheSandboxOff(t *testing.T) {
	a, _ := testApp(t, map[string]string{"HARVEST_SANDBOX": "off"})
	a.selectBackend = func(llm.Options) (llm.Backend, error) { return nil, llm.ErrNoBackend }
	out, _, _ := execute(t, a, "", "doctor")
	for _, want := range []string{
		"ok sandbox (HARVEST_SANDBOX=off: the steps run without srt)",
		"FAIL model backend: llm: no backend configured",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
}

func TestVersion(t *testing.T) {
	a, _ := testApp(t, nil)
	out, _, err := execute(t, a, "", "version", "-o", "json")
	if err != nil || !strings.Contains(out, `"build_time"`) {
		t.Errorf("version -o json = %q, %v", out, err)
	}
}
