package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// deadline is how long a test waits on a PTY. Generous on purpose: a process
// launch on a loaded machine can take seconds.
const deadline = 30 * time.Second

// skipWithoutShell skips a test that spawns a session on a machine without
// the default shell: the CI runner ships no zsh.
func skipWithoutShell(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(DefaultShell); err != nil {
		t.Skipf("no %s on this machine: %v", DefaultShell, err)
	}
}

// newManager returns a Manager whose sessions start in a temp home with a
// minimal environment, so no profile of the machine running the test prints
// into the output. The kill grace is short unless a test sets it: a hangup
// sent while the shell is still starting can be lost, and every cleanup
// would otherwise wait out the full default grace.
func newManager(t *testing.T, cfg Config) (*Manager, string) {
	t.Helper()
	skipWithoutShell(t)
	if cfg.KillGrace == 0 {
		cfg.KillGrace = 200 * time.Millisecond
	}
	home := t.TempDir()
	cfg.Home = home
	cfg.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	m := New(cfg)
	t.Cleanup(m.Close)
	return m, home
}

func attach(t *testing.T, m *Manager, name string, spec Spec) *Subscription {
	t.Helper()
	sub, err := m.Attach("page1", name, spec)
	if err != nil {
		t.Fatalf("Attach %s: %v", name, err)
	}
	t.Cleanup(sub.Close)
	return sub
}

// readUntil takes output from sub until want appears, the output ends, or
// the deadline passes, and returns what it took.
func readUntil(t *testing.T, sub *Subscription, want string) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(deadline)
	for !strings.Contains(got.String(), want) {
		select {
		case <-sub.Ready():
		case <-timeout:
			t.Fatalf("no %q within %s; got %q", want, deadline, got.String())
		}
		p, ended := sub.Take()
		got.Write(p)
		if ended {
			break
		}
	}
	return got.String()
}

// readAll takes output until it ends.
func readAll(t *testing.T, sub *Subscription) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(deadline)
	for {
		select {
		case <-sub.Ready():
		case <-timeout:
			t.Fatalf("output still open after %s; got %q", deadline, got.String())
		}
		p, ended := sub.Take()
		got.Write(p)
		if ended {
			return got.String()
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(deadline)
	for !cond() {
		if time.Now().After(end) {
			t.Fatalf("%s: not within %s", what, deadline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCommandOutputThenExitCode(t *testing.T) {
	m, _ := newManager(t, Config{})
	sub := attach(t, m, "demo", Spec{Cmd: "printf hello; exit 3"})

	if out := readAll(t, sub); !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want it to contain hello", out)
	}
	code, exited := sub.Exit()
	if !exited || code != 3 {
		t.Errorf("Exit() = %d, %v; want 3, true", code, exited)
	}
	list := m.List()
	if len(list) != 1 || !list[0].Exited || list[0].ExitCode != 3 {
		t.Fatalf("List() = %+v, want the exited session with code 3", list)
	}
	if list[0].Page != "page1" || list[0].Session != "demo" || list[0].PID == 0 {
		t.Errorf("List() entry = %+v", list[0])
	}
}

func TestLateSubscriberIsReplayedThenSharesLiveOutput(t *testing.T) {
	m, _ := newManager(t, Config{})
	first := attach(t, m, "demo", Spec{Cmd: "printf ready; cat"})
	readUntil(t, first, "ready")

	second := attach(t, m, "demo", Spec{Cmd: "ignored for a running session"})
	if !strings.Contains(string(second.Replay()), "ready") {
		t.Errorf("replay = %q, want the earlier output", second.Replay())
	}
	if _, err := second.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	readUntil(t, first, "ping")
	readUntil(t, second, "ping")
	if got := m.List(); len(got) != 1 || got[0].Attached != 2 {
		t.Errorf("List() = %+v, want one session with two subscribers", got)
	}
}

// A subscriber joining a running session gets a redraw after its replay:
// the foreground job sees SIGWINCH. The first one, which spawns it, does
// not.
func TestJoiningARunningSessionRedrawsIt(t *testing.T) {
	m, _ := newManager(t, Config{})
	// zsh runs no WINCH trap of a -c script, so a perl handler reports it.
	cmd := `perl -e '$SIG{WINCH} = sub { print "winch\n" }; $| = 1; print "ready\n"; sleep 1 while 1'`
	first := attach(t, m, "demo", Spec{Cmd: cmd})
	readUntil(t, first, "ready")
	time.Sleep(200 * time.Millisecond)
	if p, _ := first.Take(); strings.Contains(string(p), "winch") {
		t.Fatalf("output %q: the spawning Attach must not redraw", p)
	}

	second := attach(t, m, "demo", Spec{})
	readUntil(t, second, "winch")
}

func TestReplayKeepsOnlyTheNewestRingBytes(t *testing.T) {
	m, _ := newManager(t, Config{RingBytes: 8})
	first := attach(t, m, "demo", Spec{Cmd: "printf 0123456789abcdef; cat"})
	readUntil(t, first, "abcdef")

	late := attach(t, m, "demo", Spec{})
	if got := string(late.Replay()); got != "89abcdef" {
		t.Errorf("replay = %q, want the newest 8 bytes", got)
	}
}

func TestSlowSubscriberIsDroppedOthersKeepReceiving(t *testing.T) {
	m, _ := newManager(t, Config{})
	const total = 3 * pendingLimit
	cmd := fmt.Sprintf("sleep 0.2; head -c %d /dev/zero | tr '\\0' a", total)
	slow := attach(t, m, "flood", Spec{Cmd: cmd})
	fast := attach(t, m, "flood", Spec{})

	got := readAll(t, fast)
	if len(got) != total {
		t.Errorf("fast subscriber read %d bytes, want %d", len(got), total)
	}
	if _, exited := fast.Exit(); !exited {
		t.Error("fast subscriber should see the exit")
	}
	if out := readAll(t, slow); len(out) != 0 {
		t.Errorf("dropped subscriber took %d bytes, want its pending output discarded", len(out))
	}
	if _, exited := slow.Exit(); exited {
		t.Error("slow subscriber was not dropped: it reports the exit")
	}
}

// A program printing a byte at a time puts many small reads behind a
// subscriber; only the bytes count toward dropping it, so one that takes
// its output late still gets all of it and the exit.
func TestManySmallWritesDoNotDropASubscriber(t *testing.T) {
	m, _ := newManager(t, Config{})
	const total = 20000
	cmd := fmt.Sprintf("for i in {1..%d}; do printf x; done", total)
	sub := attach(t, m, "drip", Spec{Cmd: cmd})
	waitFor(t, "the process to exit", func() bool {
		list := m.List()
		return len(list) == 1 && list[0].Exited
	})

	if got := readAll(t, sub); strings.Count(got, "x") != total {
		t.Errorf("read %d x, want %d", strings.Count(got, "x"), total)
	}
	if _, exited := sub.Exit(); !exited {
		t.Error("the subscriber was dropped; want it to see the exit")
	}
}

// Input to a process that is not reading queues instead of holding up the
// caller, is refused past inputLimit, and leaves resize and kill working.
func TestInputToAProcessNotReadingQueuesAndKillStillWorks(t *testing.T) {
	m, _ := newManager(t, Config{})
	sub := attach(t, m, "demo", Spec{Cmd: "stty raw -echo; printf up; sleep 60"})
	readUntil(t, sub, "up")

	chunk := bytes.Repeat([]byte("x"), 64<<10)
	start := time.Now()
	var err error
	for written := 0; err == nil && written <= 2*inputLimit; written += len(chunk) {
		_, err = sub.Write(chunk)
	}
	if !errors.Is(err, ErrInputFull) {
		t.Fatalf("writing past the limit: err = %v, want ErrInputFull", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("queueing the input took %s, want no wait on the process", took)
	}
	if err := sub.Resize(100, 30); err != nil {
		t.Errorf("Resize with input stuck: %v", err)
	}
	if err := m.Kill("page1", "demo"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	readAll(t, sub)
	if _, err := sub.Write([]byte("x")); !errors.Is(err, ErrExited) {
		t.Errorf("Write after the exit: err = %v, want ErrExited", err)
	}
}

func TestKillEndsTheSessionAndUnlistsIt(t *testing.T) {
	m, _ := newManager(t, Config{KillGrace: 500 * time.Millisecond})
	sub := attach(t, m, "demo", Spec{Cmd: "printf up; sleep 60"})
	readUntil(t, sub, "up")

	if err := m.Kill("page1", "demo"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	readAll(t, sub)
	// SIGHUP ends it, the second one when the shell lost the first to the
	// exec of sleep; under heavy load SIGKILL after the grace may.
	code, exited := sub.Exit()
	hup, kill := 128+int(syscall.SIGHUP), 128+int(syscall.SIGKILL)
	if !exited || (code != hup && code != kill) {
		t.Errorf("Exit() = %d, %v; want %d or %d, true", code, exited, hup, kill)
	}
	if got := m.List(); len(got) != 0 {
		t.Errorf("List() after kill = %+v, want none", got)
	}
	if err := m.Kill("page1", "demo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Kill: err = %v, want ErrNotFound", err)
	}
}

// An interactive shell runs a command as a job in its own process group, and
// a job may ignore the hangup the shell passes on as it exits. Kill must
// still leave nothing running.
func TestKillReachesAJobThatIgnoresHangup(t *testing.T) {
	m, _ := newManager(t, Config{KillGrace: 500 * time.Millisecond})
	// The trailing true keeps zsh from exec'ing the job in its own place.
	sub := attach(t, m, "demo", Spec{Cmd: `sh -c 'trap "" HUP; echo pid=$$; exec sleep 60'; true`})
	out := readUntil(t, sub, "\n")
	match := regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("no pid in %q", out)
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Kill("page1", "demo"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	waitFor(t, "the job to be gone", func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	})
}

// userShellOptIn is the variable that opts in to the test running the
// user's own login shell.
const userShellOptIn = "PRESENT_TEST_USER_SHELL"

// A session must find the user's tools from the bare environment the
// launchd agent gives serve: the PATH setup lives in ~/.zshrc, which only
// an interactive shell reads. It runs the real login shell and startup
// files of whoever runs the test, which can take seconds, so it runs only
// with PRESENT_TEST_USER_SHELL=1, and skips on a machine without present
// installed (CI):
//
//	PRESENT_TEST_USER_SHELL=1 go test -run UsersShell ./internal/terminal/
func TestScrubbedEnvFindsPresentThroughTheUsersShell(t *testing.T) {
	if os.Getenv(userShellOptIn) != "1" {
		t.Skip("runs the user's own login shell; set " + userShellOptIn + "=1 to run it")
	}
	if _, err := exec.LookPath("present"); err != nil {
		t.Skip("present is not installed on this machine")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Config{Home: home, Env: []string{"HOME=" + home}})
	t.Cleanup(m.Close)
	sub := attach(t, m, "path", Spec{Cmd: "command -v present && echo FOUND"})

	out := readAll(t, sub)
	code, exited := sub.Exit()
	if !exited || code != 0 || !strings.Contains(out, "FOUND") {
		t.Errorf("command -v present = %q, exit %d %v; want FOUND and exit 0", out, code, exited)
	}
}

// finished matches a line holding only FOUND: what echo FOUND prints once it
// runs, which the typed text itself never contains.
var finished = regexp.MustCompile(`\nFOUND\r?\n`)

// A prefill waits on the shell's prompt: typed once, not run until Enter.
func TestPrefillWaitsOnTheShellPromptForEnter(t *testing.T) {
	m, _ := newManager(t, Config{})
	first := attach(t, m, "demo", Spec{Prefill: "echo FOUND"})
	readUntil(t, first, "echo FOUND")
	time.Sleep(100 * time.Millisecond)

	late := attach(t, m, "demo", Spec{})
	replay := string(late.Replay())
	if !strings.Contains(replay, "echo FOUND") || finished.MatchString(replay) {
		t.Fatalf("replay = %q, want the prefill typed and not run", replay)
	}
	if _, err := late.Write([]byte("\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out := readUntil(t, late, "FOUND\r\n"); !finished.MatchString(out) {
		t.Errorf("output after Enter = %q, want FOUND printed", out)
	}
}

// A command reading its input gets the prefill too: after its prompt goes
// quiet, or after PrefillWait when it prints nothing at all.
func TestPrefillReachesACommandReadingInput(t *testing.T) {
	cases := []struct {
		name  string
		cmd   string
		wait  time.Duration
		quiet bool // typed by the quiet output, before the wait is up
	}{
		{"prompting first", "printf 'name? '; read x; echo got:$x", DefaultPrefillWait, true},
		{"silent", "read x; echo got:$x", 300 * time.Millisecond, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := newManager(t, Config{PrefillWait: c.wait})
			start := time.Now()
			sub := attach(t, m, "demo", Spec{Cmd: c.cmd, Prefill: "hello"})
			readUntil(t, sub, "hello")
			if waited := time.Since(start); (waited < c.wait) != c.quiet {
				t.Errorf("prefill typed after %s; want before the %s wait: %v",
					waited, c.wait, c.quiet)
			}
			if _, err := sub.Write([]byte("\r")); err != nil {
				t.Fatalf("Write: %v", err)
			}
			readUntil(t, sub, "got:hello")
		})
	}
}

func TestSessionEnv(t *testing.T) {
	const term, mark = "TERM=xterm-256color", "PRESENT_TERMINAL=1"
	cases := []struct {
		name string
		env  []string
		want []string
	}{
		{"no TERM", []string{"HOME=/h"}, []string{"HOME=/h", term, mark}},
		{"TERM set", []string{"TERM=vt100", "HOME=/h"}, []string{"TERM=vt100", "HOME=/h", mark}},
		{"marker inherited", []string{"PRESENT_TERMINAL=0"}, []string{term, mark}},
	}
	for _, c := range cases {
		if got := sessionEnv(c.env); !slices.Equal(got, c.want) {
			t.Errorf("%s: sessionEnv(%q) = %q, want %q", c.name, c.env, got, c.want)
		}
	}
}

// A session sees the marker, keeps a TERM the caller set, and gets
// xterm-256color when it set none, never the TERM of the terminal serve
// itself runs in.
func TestSessionSeesTheMarkerAndItsTerm(t *testing.T) {
	skipWithoutShell(t)
	const report = `printf '%s|%s' "$TERM" "$PRESENT_TERMINAL"`
	home := t.TempDir()
	t.Setenv("HOME", home) // keep the default env off the user's startup files
	t.Setenv("ZDOTDIR", home)
	t.Setenv("TERM", "xterm-kitty")
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"default env", nil, "xterm-256color|1"},
		{"caller TERM", []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=vt100"}, "vt100|1"},
	}
	for _, c := range cases {
		m := New(Config{Home: home, Env: c.env, KillGrace: 200 * time.Millisecond})
		t.Cleanup(m.Close)
		out := readAll(t, attach(t, m, "env", Spec{Cmd: report}))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: session printed %q, want %q", c.name, out, c.want)
		}
	}
}

func TestIdleTimeoutKillsOnlyUnattendedSessions(t *testing.T) {
	m, _ := newManager(t, Config{IdleTimeout: 50 * time.Millisecond})
	watched := attach(t, m, "watched", Spec{Cmd: "sleep 60"})
	alone := attach(t, m, "alone", Spec{Cmd: "sleep 60"})
	alone.Close()

	waitFor(t, "idle session killed", func() bool { return len(m.List()) == 1 })
	time.Sleep(200 * time.Millisecond)
	if got := m.List(); len(got) != 1 || got[0].Session != "watched" {
		t.Errorf("List() = %+v, want only the attached session", got)
	}
	if _, exited := watched.Exit(); exited {
		t.Error("the attached session must keep running")
	}
}

func TestSizeAndResizeReachTheProcess(t *testing.T) {
	m, _ := newManager(t, Config{})
	sub := attach(t, m, "demo", Spec{Cmd: "stty size; read x; stty size", Cols: 100, Rows: 30})
	readUntil(t, sub, "30 100")

	if err := sub.Resize(120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if _, err := sub.Write([]byte("\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	readUntil(t, sub, "40 120")
	if err := sub.Resize(0, 40); !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("Resize(0, 40): err = %v, want ErrInvalidSpec", err)
	}
}

func TestCwdExpandsHomeAndRefusesBadPaths(t *testing.T) {
	m, home := newManager(t, Config{})
	sub := attach(t, m, "demo", Spec{Cmd: "pwd -P", Cwd: "~"})
	want, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	if out := readAll(t, sub); !strings.Contains(out, want) {
		t.Errorf("pwd = %q, want %q", out, want)
	}

	for _, cwd := range []string{"relative/dir", "~/missing", "/etc/hosts"} {
		if _, err := m.Attach("page1", "bad", Spec{Cwd: cwd}); !errors.Is(err, ErrInvalidSpec) {
			t.Errorf("cwd %q: err = %v, want ErrInvalidSpec", cwd, err)
		}
	}
}

func TestCloseKillsEverythingAndRefusesAttach(t *testing.T) {
	m, _ := newManager(t, Config{})
	sub := attach(t, m, "demo", Spec{Cmd: "sleep 60"})

	m.Close()
	readAll(t, sub)
	if _, exited := sub.Exit(); !exited {
		t.Error("Close must end the running session")
	}
	if _, err := m.Attach("page1", "demo", Spec{}); !errors.Is(err, ErrClosed) {
		t.Errorf("Attach after Close: err = %v, want ErrClosed", err)
	}
}

func TestValidName(t *testing.T) {
	cases := map[string]bool{
		"demo": true, "a-1": true, strings.Repeat("a", 32): true,
		"": false, "Demo": false, "a_b": false, "a/b": false, strings.Repeat("a", 33): false,
	}
	for name, want := range cases {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}
