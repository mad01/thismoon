package playback

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

// newTestEngine returns an Engine wired to temp paths, with synthesis and
// playback replaced by instant no-ops so tests never touch the network or audio.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(ttsclient.New("http://127.0.0.1:0"), "af_heart", t.TempDir())
	e.synth = func(_, _ string) ([]byte, error) { return []byte("wav"), nil }
	e.newPlayCmd = func(_ string) *exec.Cmd { return exec.Command("true") }
	return e
}

func waitIdle(t *testing.T, e *Engine) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		running := e.running
		e.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("worker did not finish within 2s")
}

func TestSpeakTextEmpty(t *testing.T) {
	if got := newTestEngine(t).SpeakText("   ", ""); got.Message != "Nothing to speak." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestSpeakTextPlaysThrough(t *testing.T) {
	e := newTestEngine(t)
	res := e.SpeakText("One. Two.", "")
	if res.Session == "" {
		t.Fatal("expected a session id")
	}
	if !strings.Contains(res.Message, "Playing 2 sentence(s)") {
		t.Errorf("message = %q", res.Message)
	}
	waitIdle(t, e)

	status := e.Status()
	if !strings.Contains(status.Message, "last_result: Spoke 2/2 sentence(s).") {
		t.Errorf("status = %q", status.Message)
	}
	// Lock is released after a clean finish.
	if !strings.Contains(status.Message, "locked: false") {
		t.Errorf("expected lock released, status = %q", status.Message)
	}
}

func TestSpeakFileNotFound(t *testing.T) {
	got := newTestEngine(t).SpeakFile("/no/such/file.md", "", "")
	if !strings.HasPrefix(got.Message, "File not found:") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestSpeakFileSections(t *testing.T) {
	e := newTestEngine(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFile(t, path, "# A\n\nAlpha one. Alpha two.\n\n# B\n\nBravo one.\n")

	// Only section 2 (index 1) → its sentences.
	res := e.SpeakFile(path, "", "2")
	if res.Session == "" || !strings.Contains(res.Message, "Playing 1 sentence(s)") {
		t.Errorf("res = %+v", res)
	}
	waitIdle(t, e)
}

func TestPauseNothingPlaying(t *testing.T) {
	if got := newTestEngine(t).Pause(""); got.Message != "Nothing is playing." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestResumeNothing(t *testing.T) {
	if got := newTestEngine(t).Resume(""); got.Message != "Nothing to resume." {
		t.Errorf("message = %q", got.Message)
	}
}

func TestSessionMismatch(t *testing.T) {
	e := newTestEngine(t)
	e.sessionID = "abc123"
	e.total = 2
	if got := e.Stop("wrong"); !strings.HasPrefix(got.Message, "Session mismatch:") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestBusyWhenLockHeld(t *testing.T) {
	e := newTestEngine(t)
	// Hold the playback lock from a separate handle in-process.
	other := flock.New(e.lockPath)
	locked, err := other.TryLock()
	if err != nil || !locked {
		t.Fatalf("could not pre-acquire lock: locked=%v err=%v", locked, err)
	}
	t.Cleanup(func() { _ = other.Unlock() })

	got := e.SpeakText("Hello there.", "")
	if !strings.HasPrefix(got.Message, "BUSY |") {
		t.Errorf("expected BUSY, got %q", got.Message)
	}
}

func TestVoices(t *testing.T) {
	got := newTestEngine(t).Voices()
	if !strings.Contains(got.Message, "af_heart") || !strings.Contains(got.Message, "am_michael") {
		t.Errorf("voices = %q", got.Message)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
