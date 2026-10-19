package terminal

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// deadline is how long a test waits on a PTY. Generous on purpose: a process
// launch on a loaded machine can take seconds.
const deadline = 30 * time.Second

// newManager returns a Manager whose sessions start in a temp home with a
// minimal environment, so no profile of the machine running the test prints
// into the output. The kill grace is short unless a test sets it: a hangup
// sent while the shell is still starting can be lost, and every cleanup
// would otherwise wait out the full default grace.
func newManager(t *testing.T, cfg Config) (*Manager, string) {
	t.Helper()
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

// readUntil collects output from sub until want appears, Output closes, or
// the deadline passes, and returns what it read.
func readUntil(t *testing.T, sub *Subscription, want string) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(deadline)
	for !strings.Contains(got.String(), want) {
		select {
		case chunk, ok := <-sub.Output():
			if !ok {
				return got.String()
			}
			got.Write(chunk)
		case <-timeout:
			t.Fatalf("no %q within %s; got %q", want, deadline, got.String())
		}
	}
	return got.String()
}

// readAll collects output until Output closes.
func readAll(t *testing.T, sub *Subscription) string {
	t.Helper()
	var got bytes.Buffer
	timeout := time.After(deadline)
	for {
		select {
		case chunk, ok := <-sub.Output():
			if !ok {
				return got.String()
			}
			got.Write(chunk)
		case <-timeout:
			t.Fatalf("output still open after %s; got %q", deadline, got.String())
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
	const total = 4_000_000
	cmd := "sleep 0.2; head -c 4000000 /dev/zero | tr '\\0' a"
	slow := attach(t, m, "flood", Spec{Cmd: cmd})
	fast := attach(t, m, "flood", Spec{})

	got := readAll(t, fast)
	if len(got) != total {
		t.Errorf("fast subscriber read %d bytes, want %d", len(got), total)
	}
	if _, exited := fast.Exit(); !exited {
		t.Error("fast subscriber should see the exit")
	}
	readAll(t, slow)
	if _, exited := slow.Exit(); exited {
		t.Error("slow subscriber was not dropped: it reports the exit")
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
	// SIGHUP normally ends it; a hangup that lands while the shell is still
	// starting can be lost, and then SIGKILL after the grace does.
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

// A session must find the user's tools from the bare environment the
// launchd agent gives serve: the PATH setup lives in ~/.zshrc, which only
// an interactive shell reads. It runs the real login shell of whoever runs
// the test, so it skips on a machine without present installed (CI).
func TestScrubbedEnvFindsPresentThroughTheUsersShell(t *testing.T) {
	if _, err := exec.LookPath("present"); err != nil {
		t.Skip("present is not installed on this machine")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Config{Home: home, Env: []string{"HOME=" + home, "TERM=dumb"}})
	t.Cleanup(m.Close)
	sub := attach(t, m, "path", Spec{Cmd: "command -v present && echo FOUND"})

	out := readAll(t, sub)
	code, exited := sub.Exit()
	if !exited || code != 0 || !strings.Contains(out, "FOUND") {
		t.Errorf("command -v present = %q, exit %d %v; want FOUND and exit 0", out, code, exited)
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
