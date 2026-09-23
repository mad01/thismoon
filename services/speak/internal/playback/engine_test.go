package playback

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/mad01/thismoon/services/speak/internal/chunk"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// testWAV stands in for a synthesized clip.
var testWAV = tts.Audio{Data: []byte("wav"), ContentType: tts.ContentTypeWAV}

// newTestEngine returns an Engine wired to temp paths, with synthesis and
// playback replaced by instant no-ops so tests never touch the network or audio.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(Config{
		Health:       tts.NewHealth("local", "kokoro"),
		DefaultVoice: "af_heart",
		StateDir:     t.TempDir(),
	})
	e.synth = func(_ context.Context, _, _ string) (tts.Audio, error) { return testWAV, nil }
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
	if !strings.Contains(res.Message, "Playing 2 part(s)") {
		t.Errorf("message = %q", res.Message)
	}
	waitIdle(t, e)

	status := e.Status()
	if !strings.Contains(status.Message, "last_result: Spoke 2/2 part(s).") {
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

// TestSpeakFileUnreadable: a file speak may not read fails with the way
// around it, not a misleading "not found".
func TestSpeakFileUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.md")
	writeFile(t, path, "# A\n\nAlpha.\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	got := newTestEngine(t).SpeakFile(path, "", "")
	if !got.Failed || !strings.HasPrefix(got.Message, "UNREADABLE | "+path) ||
		!strings.Contains(got.Message, "speak_text") {
		t.Errorf("res = %+v, want a failed UNREADABLE reply pointing at speak_text", got)
	}
}

func TestSpeakFileSections(t *testing.T) {
	e := newTestEngine(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	writeFile(t, path, "# A\n\nAlpha one. Alpha two.\n\n# B\n\nBravo one.\n")

	// Only section 2 (index 1) → its parts.
	res := e.SpeakFile(path, "", "2")
	if res.Session == "" || !strings.Contains(res.Message, "Playing 1 part(s)") {
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

// TestSpeakTextFailsWhenBackendIsDown pins the fix for the silent failure:
// a backend that cannot synthesize the first part comes back as a failed
// reply naming the reason, with nothing playing and the lock free.
func TestSpeakTextFailsWhenBackendIsDown(t *testing.T) {
	e := newTestEngine(t)
	e.synth = func(_ context.Context, _, _ string) (tts.Audio, error) {
		return tts.Audio{}, &tts.Error{
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
	var down atomic.Bool
	down.Store(true)
	e.synth = func(_ context.Context, _, _ string) (tts.Audio, error) {
		if down.Load() {
			return tts.Audio{}, &tts.Error{Kind: tts.KindQuota, Message: "rate limited"}
		}
		return testWAV, nil
	}
	failed := e.SpeakText("One. Two.", "")

	down.Store(false)
	got := e.Resume(failed.Session)
	if got.Failed || got.Session != failed.Session {
		t.Errorf("resume = %+v, want playback of session %s", got, failed.Session)
	}
	waitIdle(t, e)
	if status := e.Status().Message; !strings.Contains(
		status,
		"last_result: Spoke 2/2 part(s).",
	) ||
		!strings.Contains(status, "tts_health: ok") {
		t.Errorf("status = %q, want both parts spoken and health ok", status)
	}
}

// TestFirstPartSynthesizedOnce guards the pre-synthesis from doubling the
// first request (a metered backend would bill it twice).
func TestFirstPartSynthesizedOnce(t *testing.T) {
	e := newTestEngine(t)
	calls := recordSynth(e)
	e.SpeakText("One. Two.", "")
	waitIdle(t, e)
	if got := calls(); !slices.Equal(got, []string{"One.", "Two."}) {
		t.Errorf("synthesized %q, want each part once in order", got)
	}
}

// TestSpeakTextRampsUpParts pins the Live grouping: the first part is one
// sentence, so the first sound needs one short request, and the sentences
// after it share a part.
func TestSpeakTextRampsUpParts(t *testing.T) {
	e := newTestEngine(t)
	calls := recordSynth(e)
	res := e.SpeakText("One. Two. Three. Four.", "")
	if !strings.Contains(res.Message, "Playing 2 part(s)") {
		t.Errorf("message = %q, want 2 parts", res.Message)
	}
	waitIdle(t, e)
	if got := calls(); !slices.Equal(got, []string{"One.", "Two. Three. Four."}) {
		t.Errorf("synthesized %q, want one sentence, then the rest as one part", got)
	}
}

// TestSectionParts pins how speak_file sizes parts: the first section with
// anything to say ramps up from one sentence, later sections start at full
// size, and no part spans two sections.
func TestSectionParts(t *testing.T) {
	sections := []string{"Alpha one. Alpha two.", "Bravo one. Bravo two."}
	tests := []struct {
		name     string
		sections []string
		want     map[int]bool
		parts    []string
	}{
		{
			"all sections", sections, nil,
			[]string{"Alpha one.", "Alpha two.", "Bravo one. Bravo two."},
		},
		{
			"a later section alone ramps up", sections,
			map[int]bool{1: true},
			[]string{"Bravo one.", "Bravo two."},
		},
		{
			"an unspeakable first section does not take the ramp",
			[]string{"``", sections[1]},
			nil,
			[]string{"Bravo one.", "Bravo two."},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sectionParts(tc.sections, tc.want); !slices.Equal(got, tc.parts) {
				t.Errorf("sectionParts = %q, want %q", got, tc.parts)
			}
		})
	}
}

// TestNextPartsSynthesizeWhilePlaying pins the prefetch: while the first
// part is still playing, the next prefetchDepth parts are synthesizing, and
// no more than that.
func TestNextPartsSynthesizeWhilePlaying(t *testing.T) {
	e := newTestEngine(t)
	release := holdPlayback(t, e)
	started := make(chan string, 4)
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		started <- strings.Fields(text)[0]
		return testWAV, nil
	}

	res := e.SpeakText(partsText("One", "Two", "Three", "Four"), "")
	if !strings.Contains(res.Message, "Playing 4 part(s)") {
		t.Fatalf("message = %q, want 4 parts", res.Message)
	}
	waitForStatus(t, e, "state: playing")
	got := []string{receive(t, started), receive(t, started), receive(t, started)}
	slices.Sort(got[1:]) // the prefetched parts start in either order
	if !slices.Equal(got, []string{"One", "Three", "Two"}) {
		t.Errorf("synthesized %q, want One, then Two and Three", got)
	}
	status := e.Status().Message
	if !strings.Contains(status, "state: playing") ||
		!strings.Contains(status, "position: part 1 of 4") {
		t.Errorf("status = %q, want part 1 still playing", status)
	}
	select {
	case label := <-started:
		t.Errorf("%s synthesized while part 1 plays, want at most %d ahead", label, prefetchDepth)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	waitIdle(t, e)
	if label := receive(t, started); label != "Four" {
		t.Errorf("last synthesis = %s, want Four", label)
	}
	if status := e.Status().Message; !strings.Contains(status, "Spoke 4/4 part(s).") {
		t.Errorf("status = %q, want all four parts spoken", status)
	}
}

// TestStopDoesNotWaitForPrefetch pins that stop is prompt when the next
// part is still synthesizing: a remote provider can take a minute, and the
// user asked for silence now.
func TestStopDoesNotWaitForPrefetch(t *testing.T) {
	e := newTestEngine(t)
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		if strings.HasPrefix(text, "One") {
			return testWAV, nil
		}
		<-stuck
		// Fail once released, so an orphaned synthesis writes no file into
		// the test's temp dir while it is being removed.
		return tts.Audio{}, errors.New("released")
	}

	res := e.SpeakText(partsText("One", "Two", "Three"), "")
	waitForStatus(t, e, "position: part 2 of 3")

	stopped := make(chan Result, 1)
	go func() { stopped <- e.Stop(res.Session) }()
	select {
	case got := <-stopped:
		if !strings.HasPrefix(got.Message, "Stopped at part 2 of 3.") {
			t.Errorf("stop = %q, want it stopped at part 2", got.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop waited for the in-flight synthesis")
	}
	status := e.Status().Message
	if !strings.Contains(status, "state: stopped") || !strings.Contains(status, "locked: false") {
		t.Errorf("status = %q, want stopped with the lock free", status)
	}
}

// TestStopCancelsPrefetches: a stopped session stops paying for parts it
// will never play.
func TestStopCancelsPrefetches(t *testing.T) {
	e := newTestEngine(t)
	cancelled := make(chan string, 4)
	e.synth = func(ctx context.Context, text, _ string) (tts.Audio, error) {
		if strings.HasPrefix(text, "One") {
			return testWAV, nil
		}
		<-ctx.Done()
		cancelled <- strings.Fields(text)[0]
		return tts.Audio{}, ctx.Err()
	}

	res := e.SpeakText(partsText("One", "Two", "Three", "Four", "Five"), "")
	// Waiting on part 2, with parts 3 and 4 prefetched behind it.
	waitForStatus(t, e, "position: part 2 of 5")
	e.Stop(res.Session)
	got := []string{receive(t, cancelled), receive(t, cancelled), receive(t, cancelled)}
	slices.Sort(got)
	if !slices.Equal(got, []string{"Four", "Three", "Two"}) {
		t.Errorf("cancelled %q, want every part in flight", got)
	}
}

// TestStopCancelsASlowFirstPart pins that Stop does not wait out a start
// still synthesizing its first part, which a remote provider can take
// minutes over: the start is cancelled and says so, records no provider
// failure, and leaves its queue for speak_resume.
func TestStopCancelsASlowFirstPart(t *testing.T) {
	e := newTestEngine(t)
	var slow atomic.Bool
	slow.Store(true)
	entered := make(chan struct{}, 1)
	e.synth = func(ctx context.Context, _, _ string) (tts.Audio, error) {
		if !slow.Load() {
			return testWAV, nil
		}
		entered <- struct{}{}
		<-ctx.Done()
		return tts.Audio{}, ctx.Err()
	}

	started := make(chan Result, 1)
	go func() { started <- e.SpeakText("One. Two.", "") }()
	receive(t, entered)
	stopped := make(chan Result, 1)
	go func() { stopped <- e.Stop("") }()

	if start := receive(t, started); start.Failed || start.Message != stoppedBeforeStart {
		t.Errorf("start = %+v, want a plain %q", start, stoppedBeforeStart)
	}
	if stop := receive(t, stopped); !strings.HasPrefix(stop.Message, "Stopped at part 1 of 2.") {
		t.Errorf("stop = %q, want it stopped at part 1", stop.Message)
	}
	status := e.Status().Message
	for _, want := range []string{
		"state: stopped",
		"locked: false",
		"tts_health: unknown",
		"last_result: " + stoppedBeforeStart,
	} {
		if !strings.Contains(status, want) {
			t.Errorf("status missing %q:\n%s", want, status)
		}
	}

	slow.Store(false)
	res := e.Resume("")
	if !strings.Contains(res.Message, "Playing 2 part(s)") {
		t.Errorf("resume = %q, want the queue played", res.Message)
	}
	waitIdle(t, e)
}

// TestResumeWhileStartingLeavesTheStartAlone pins the window while the
// first part synthesizes: status says starting, and speak_resume answers so
// instead of restarting the session and replaying part 1.
func TestResumeWhileStartingLeavesTheStartAlone(t *testing.T) {
	e := newTestEngine(t)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	entered := make(chan struct{}, 1)
	var mu sync.Mutex
	var calls []string
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		mu.Lock()
		calls = append(calls, text)
		first := len(calls) == 1
		mu.Unlock()
		if first {
			entered <- struct{}{}
			<-release
		}
		return testWAV, nil
	}

	started := make(chan Result, 1)
	go func() { started <- e.SpeakText("One. Two.", "") }()
	receive(t, entered)
	if status := e.Status().Message; !strings.Contains(status, "state: starting") {
		t.Errorf("status = %q, want starting while part 1 synthesizes", status)
	}
	resumed := make(chan Result, 1)
	go func() { resumed <- e.Resume("") }()
	if got := receive(t, resumed); !strings.HasPrefix(got.Message, "Playback is starting") {
		t.Errorf("resume = %q, want it to say playback is starting", got.Message)
	}

	unblock()
	if got := receive(t, started); !strings.Contains(got.Message, "Playing 2 part(s)") {
		t.Errorf("start = %q, want it playing", got.Message)
	}
	waitIdle(t, e)
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(calls, []string{"One.", "Two."}) {
		t.Errorf("synthesized %q, want each part once (no restart)", calls)
	}
}

// TestConcurrentClipsGetTheirOwnFiles: prefetched parts are written at the
// same moment, and each must keep its own audio in a file the reaper finds.
func TestConcurrentClipsGetTheirOwnFiles(t *testing.T) {
	e := newTestEngine(t)
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		return tts.Audio{Data: []byte(text), ContentType: tts.ContentTypeWAV}, nil
	}
	const clips = 16
	paths := make([]string, clips)
	var wg sync.WaitGroup
	for i := range clips {
		wg.Go(func() {
			path, err := e.synthToFile(context.Background(), strconv.Itoa(i), "")
			if err != nil {
				t.Error(err)
			}
			paths[i] = path
		})
	}
	wg.Wait()

	for i, path := range paths {
		name := filepath.Base(path)
		if !strings.HasPrefix(name, "speak_") || !isAudioFile(name) {
			t.Errorf("clip %d = %s, want a speak_ audio file", i, name)
		}
		if data, err := os.ReadFile(path); err != nil || string(data) != strconv.Itoa(i) {
			t.Errorf("clip %d holds %q (err %v), want its own audio", i, data, err)
		}
	}
}

// TestPrefetchFailureSurfacesWhenReached: a prefetched part that failed ends
// playback when its turn comes, with the reason in last_result, exactly as a
// failed synthesis did before prefetching.
func TestPrefetchFailureSurfacesWhenReached(t *testing.T) {
	e := newTestEngine(t)
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		if strings.HasPrefix(text, "Two") {
			return tts.Audio{}, &tts.Error{Kind: tts.KindUpstream, Message: "gave up on Two"}
		}
		return testWAV, nil
	}
	e.SpeakText(partsText("One", "Two", "Three"), "")
	waitIdle(t, e)
	status := e.Status().Message
	for _, want := range []string{
		"last_result: TTS error: gave up on Two",
		"position: part 2 of 3",
		"state: stopped",
		"locked: false",
	} {
		if !strings.Contains(status, want) {
			t.Errorf("status missing %q:\n%s", want, status)
		}
	}
}

// TestConcurrentStartsAreSerialized pins the invariant the pre-synthesis
// put at risk: while one start synthesizes its first part, a second
// start waits instead of releasing the first one's playback lock.
func TestConcurrentStartsAreSerialized(t *testing.T) {
	e := newTestEngine(t)
	release := make(chan struct{})
	var inFlight, maxInFlight atomic.Int32
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		if n > maxInFlight.Load() {
			maxInFlight.Store(n)
		}
		if text == "First." {
			<-release
		}
		return testWAV, nil
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
		t.Errorf("%d first-part syntheses overlapped, want starts serialized", got)
	}
}

// recordSynth makes e's synthesis record the text of every call, returning
// a snapshot of the calls so far.
func recordSynth(e *Engine) func() []string {
	var mu sync.Mutex
	var calls []string
	e.synth = func(_ context.Context, text, _ string) (tts.Audio, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, text)
		return testWAV, nil
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(calls)
	}
}

// partsText is one sentence per label, each long enough (over half of
// chunk.MaxChars) to make a part of its own under any ramp, so a test can
// tell parts apart by their first word.
func partsText(labels ...string) string {
	filler := strings.Repeat("x", chunk.MaxChars/2)
	sentences := make([]string, len(labels))
	for i, label := range labels {
		sentences[i] = label + " " + filler + "."
	}
	return strings.Join(sentences, " ")
}

// holdPlayback makes every clip play until release is called, so a test can
// look at the engine while a part is playing.
func holdPlayback(t *testing.T, e *Engine) (release func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// cat plays a clip: it runs until the pipe's write end closes.
	e.newPlayCmd = func(string) *exec.Cmd {
		cmd := exec.Command("cat")
		cmd.Stdin = r
		return cmd
	}
	var once sync.Once
	release = func() { once.Do(func() { _ = w.Close() }) }
	t.Cleanup(func() {
		release()
		waitIdle(t, e)
		_ = r.Close()
	})
	return release
}

// receive returns the next value on ch, failing the test after 2s.
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("nothing received within 2s")
		var zero T
		return zero
	}
}

// waitForStatus polls Status until it contains want, failing after 2s.
func waitForStatus(t *testing.T, e *Engine, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(e.Status().Message, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status never showed %q:\n%s", want, e.Status().Message)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
