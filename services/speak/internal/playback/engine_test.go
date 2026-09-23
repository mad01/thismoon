package playback

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/mad01/thismoon/services/speak/internal/tts"
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

// TestSpeakTextFailsWhenBackendIsDown pins the fix for the silent failure:
// a backend that cannot synthesize the first sentence comes back as a failed
// reply naming the reason, with nothing playing and the lock free.
func TestSpeakTextFailsWhenBackendIsDown(t *testing.T) {
	e := newTestEngine(t)
	e.synth = func(_, _ string) ([]byte, error) {
		return nil, &tts.Error{
			Kind:    tts.KindNetwork,
			Message: "TTS engine not reachable at http://127.0.0.1:8765",
		}
	}

	got := e.SpeakText("One. Two.", "")
	if !got.Failed || got.Session == "" {
		t.Errorf("result = %+v, want a failed reply that keeps the session for resume", got)
	}
	if !strings.HasPrefix(
		got.Message,
		"UNAVAILABLE | TTS down (network): TTS engine not reachable",
	) {
		t.Errorf("message = %q, want UNAVAILABLE with the health summary", got.Message)
	}
	status := e.Status().Message
	for _, want := range []string{"state: stopped", "locked: false", "tts_health: down (network)"} {
		if !strings.Contains(status, want) {
			t.Errorf("status missing %q:\n%s", want, status)
		}
	}
}

// TestResumeRetriesAfterBackendRecovers pins the retry path the UNAVAILABLE
// reply points at: once synthesis works, speak_resume plays the queue.
func TestResumeRetriesAfterBackendRecovers(t *testing.T) {
	e := newTestEngine(t)
	down := true
	e.synth = func(_, _ string) ([]byte, error) {
		if down {
			return nil, &tts.Error{Kind: tts.KindQuota, Message: "rate limited"}
		}
		return []byte("wav"), nil
	}
	failed := e.SpeakText("One. Two.", "")

	down = false
	got := e.Resume(failed.Session)
	if got.Failed || got.Session != failed.Session {
		t.Errorf("resume = %+v, want playback of session %s", got, failed.Session)
	}
	waitIdle(t, e)
	if status := e.Status().Message; !strings.Contains(
		status,
		"last_result: Spoke 2/2 sentence(s).",
	) ||
		!strings.Contains(status, "tts_health: ok") {
		t.Errorf("status = %q, want both sentences spoken and health ok", status)
	}
}

// TestFirstSentenceSynthesizedOnce guards the pre-synthesis from doubling
// the first request (a metered backend would bill it twice).
func TestFirstSentenceSynthesizedOnce(t *testing.T) {
	e := newTestEngine(t)
	var calls []string
	e.synth = func(text, _ string) ([]byte, error) {
		calls = append(calls, text)
		return []byte("wav"), nil
	}
	e.SpeakText("One. Two.", "")
	waitIdle(t, e)
	if len(calls) != 2 || calls[0] != "One." || calls[1] != "Two." {
		t.Errorf("synthesized %q, want each sentence once in order", calls)
	}
}

// TestConcurrentStartsAreSerialized pins the invariant the pre-synthesis
// put at risk: while one start synthesizes its first sentence, a second
// start waits instead of releasing the first one's playback lock.
func TestConcurrentStartsAreSerialized(t *testing.T) {
	e := newTestEngine(t)
	release := make(chan struct{})
	var inFlight, maxInFlight atomic.Int32
	e.synth = func(text, _ string) ([]byte, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		if n > maxInFlight.Load() {
			maxInFlight.Store(n)
		}
		if text == "First." {
			<-release
		}
		return []byte("wav"), nil
	}

	first := make(chan Result)
	go func() { first <- e.SpeakText("First.", "") }()
	for inFlight.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	second := make(chan Result)
	go func() { second <- e.SpeakText("Second.", "") }()
	time.Sleep(50 * time.Millisecond) // give an unserialized start time to race
	close(release)

	for _, res := range []Result{<-first, <-second} {
		if res.Failed || strings.HasPrefix(res.Message, "BUSY") {
			t.Errorf("start = %+v, want both starts to succeed in turn", res)
		}
	}
	waitIdle(t, e)
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("%d first-sentence syntheses overlapped, want starts serialized", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
