package audiocache

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// settleTimeout bounds how long a test waits for background work.
const settleTimeout = 5 * time.Second

// fakeSynth records the order texts are synthesized in, holds every
// synthesis until gate is closed, and answers a text's syntheses with the
// errors listed for it in fail, one per call, then with audio.
type fakeSynth struct {
	gate    chan struct{}
	started chan string // each text as its synthesis starts
	fail    map[string][]error

	mu    sync.Mutex
	order []string
}

func newFakeSynth() *fakeSynth {
	return &fakeSynth{
		gate:    make(chan struct{}),
		started: make(chan string, 100),
		fail:    map[string][]error{},
	}
}

func (f *fakeSynth) synth(ctx context.Context, text string) (tts.Audio, error) {
	f.mu.Lock()
	f.order = append(f.order, text)
	var err error
	if errs := f.fail[text]; len(errs) > 0 {
		err, f.fail[text] = errs[0], errs[1:]
	}
	f.mu.Unlock()
	f.started <- text
	select {
	case <-f.gate:
	case <-ctx.Done():
		return tts.Audio{}, ctx.Err()
	}
	if err != nil {
		return tts.Audio{}, err
	}
	return tts.Audio{Data: tts.WAV([]byte(text), 24000, 1), ContentType: tts.ContentTypeWAV}, nil
}

// failNext makes the next n syntheses of text fail with err.
func (f *fakeSynth) failNext(text string, n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for range n {
		f.fail[text] = append(f.fail[text], err)
	}
}

func (f *fakeSynth) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

func newTestPreparer(t *testing.T, f *fakeSynth, workers int) *Preparer {
	t.Helper()
	return NewPreparer(PreparerConfig{
		Store:   NewStore(t.TempDir()),
		Synth:   f.synth,
		Health:  tts.NewHealth("fake", "model"),
		Workers: workers,
		Timeout: time.Minute,
		// Retry at once, so no test waits out the backoff.
		RetryDelay: func(int) time.Duration { return 0 },
	})
}

// waitState polls until the clip for text reaches want.
func waitState(t *testing.T, p *Preparer, text string, want State) {
	t.Helper()
	deadline := time.Now().Add(settleTimeout)
	for {
		got, _ := p.State(key(text))
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("state of %q = %s, want %s", text, got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func key(text string) string { return Key("fake", 0, text) }

// items makes the queue items for texts, in order.
func items(texts ...string) []Item {
	out := make([]Item, len(texts))
	for i, text := range texts {
		out[i] = Item{Key: key(text), Text: text}
	}
	return out
}

func TestPreparerRunsUrgentBeforeBackground(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 1)
	p.Queue(items("a", "b", "c"))
	<-f.started // the one worker holds "a"

	// Fetch moves "c" to the urgent queue before it waits; a cancelled
	// context makes it return right after, so the test does not race it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Fetch(ctx, key("c"), "c"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch with a cancelled context = %v, want context.Canceled", err)
	}
	close(f.gate)
	for _, text := range []string{"a", "b", "c"} {
		waitState(t, p, text, StateReady)
	}
	if got := f.calls(); len(got) != 3 || got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Errorf("synthesis order = %v, want [a c b] (the fetched part jumps the queue)", got)
	}
}

func TestPreparerFetchSynthesizesOnce(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 3)
	const fetchers = 10
	var wg sync.WaitGroup
	errs := make(chan error, fetchers)
	for range fetchers {
		wg.Go(func() {
			audio, err := p.Fetch(context.Background(), key("once"), "once")
			if err == nil && len(audio.Data) == 0 {
				err = errors.New("empty audio")
			}
			errs <- err
		})
	}
	<-f.started
	close(f.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Fetch: %v", err)
		}
	}
	if got := f.calls(); len(got) != 1 {
		t.Errorf("%d concurrent fetches ran %d syntheses, want 1", fetchers, len(got))
	}
}

// TestPreparerHaltsOnSharedFailures pins which failures stop the background
// queue: those every other part would hit too. The first part fails once,
// is not tried again, and the parts queued behind it go back to idle.
func TestPreparerHaltsOnSharedFailures(t *testing.T) {
	for _, kind := range []tts.Kind{
		tts.KindQuota, tts.KindAuth, tts.KindNetwork, tts.KindConfig, tts.KindModel,
	} {
		t.Run(string(kind), func(t *testing.T) {
			f := newFakeSynth()
			f.failNext("a", 1, &tts.Error{Kind: kind, Provider: "fake", Message: "a failed"})
			p := newTestPreparer(t, f, 1)
			p.Queue(items("a", "b", "c"))
			close(f.gate)
			waitState(t, p, "a", StateFailed)
			for _, text := range []string{"b", "c"} {
				waitState(t, p, text, StateIdle)
			}
			if _, err := p.State(key("a")); err == nil ||
				err.Error() != "failed after 1 attempt: a failed" {
				t.Errorf("State(a) reason = %v, want the failure after 1 attempt", err)
			}
			if got := f.calls(); !slices.Equal(got, []string{"a"}) {
				t.Errorf("syntheses = %v, want only the failed one, not retried", got)
			}
		})
	}
}

// TestPreparerRetriesUpstreamFailures pins the automatic retry: an upstream
// failure (a timeout included) is tried again after a backoff, up to
// maxAttempts in all, and a part queued again after that starts over.
func TestPreparerRetriesUpstreamFailures(t *testing.T) {
	stall := &tts.Error{Kind: tts.KindUpstream, Provider: "fake", Message: "did not answer"}
	cases := []struct {
		name       string
		failures   int
		want       State
		wantReason string
		wantDelays []int // the attempts a backoff was asked for after
	}{
		{"second attempt succeeds", 1, StateReady, "", []int{1}},
		{
			"every attempt fails", maxAttempts, StateFailed,
			"failed after 3 attempts: did not answer",
			[]int{1, 2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeSynth()
			f.failNext("a", tc.failures, stall)
			var mu sync.Mutex
			var delays []int
			p := NewPreparer(PreparerConfig{
				Store: NewStore(t.TempDir()), Synth: f.synth,
				Health: tts.NewHealth("fake", "model"),
				RetryDelay: func(attempt int) time.Duration {
					mu.Lock()
					defer mu.Unlock()
					delays = append(delays, attempt)
					return 0
				},
			})
			p.Queue(items("a", "b"))
			close(f.gate)
			waitState(t, p, "a", tc.want)
			waitState(t, p, "b", StateReady) // the queue carries on past it
			_, err := p.State(key("a"))
			if (tc.wantReason == "" && err != nil) ||
				(tc.wantReason != "" && (err == nil || err.Error() != tc.wantReason)) {
				t.Errorf("State(a) reason = %v, want %q", err, tc.wantReason)
			}
			mu.Lock()
			if !slices.Equal(delays, tc.wantDelays) {
				t.Errorf("backoffs after attempts %v, want %v", delays, tc.wantDelays)
			}
			mu.Unlock()
			if tc.want != StateFailed {
				return
			}
			// Queued again by hand, the part gets a full set of attempts.
			f.failNext("a", maxAttempts-1, stall)
			p.Queue(items("a"))
			waitState(t, p, "a", StateReady)
		})
	}
}

// TestPreparerFetchPromotesRetryingPart pins that a part someone plays does
// not wait out its backoff.
func TestPreparerFetchPromotesRetryingPart(t *testing.T) {
	f := newFakeSynth()
	f.failNext("a", 1, &tts.Error{Kind: tts.KindUpstream, Provider: "fake", Message: "stall"})
	p := NewPreparer(PreparerConfig{
		Store: NewStore(t.TempDir()), Synth: f.synth, Health: tts.NewHealth("fake", "model"),
		RetryDelay: func(int) time.Duration { return time.Hour },
	})
	p.Queue(items("a"))
	close(f.gate)
	waitState(t, p, "a", StateRetrying)
	audio, err := p.Fetch(context.Background(), key("a"), "a")
	if err != nil || len(audio.Data) == 0 {
		t.Fatalf("Fetch of a retrying part = %d bytes, %v; want the clip now", len(audio.Data),
			err)
	}
	if got := f.calls(); len(got) != 2 {
		t.Errorf("syntheses = %v, want the failed one and the fetched retry", got)
	}
}

// TestPreparerForgetCancelsRetry pins that a forgotten part's pending retry
// never runs.
func TestPreparerForgetCancelsRetry(t *testing.T) {
	const backoff = 200 * time.Millisecond
	f := newFakeSynth()
	f.failNext("a", 1, &tts.Error{Kind: tts.KindUpstream, Provider: "fake", Message: "stall"})
	p := NewPreparer(PreparerConfig{
		Store: NewStore(t.TempDir()), Synth: f.synth, Health: tts.NewHealth("fake", "model"),
		RetryDelay: func(int) time.Duration { return backoff },
	})
	p.Queue(items("a"))
	close(f.gate)
	waitState(t, p, "a", StateRetrying)
	p.Forget([]string{key("a")})
	time.Sleep(2 * backoff)
	if state, _ := p.State(key("a")); state != StateIdle || len(f.calls()) != 1 {
		t.Errorf("after Forget: %s with syntheses %v, want idle and no retry", state, f.calls())
	}
}

// TestPreparerHaltCancelsRetries pins that a shared failure also stops the
// parts waiting to retry: they would hit it too.
func TestPreparerHaltCancelsRetries(t *testing.T) {
	f := newFakeSynth()
	f.failNext("a", 1, &tts.Error{Kind: tts.KindUpstream, Provider: "fake", Message: "stall"})
	f.failNext("b", 1, &tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limit"})
	p := NewPreparer(PreparerConfig{
		Store: NewStore(t.TempDir()), Synth: f.synth, Health: tts.NewHealth("fake", "model"),
		RetryDelay: func(int) time.Duration { return time.Hour },
	})
	p.Queue(items("a", "b"))
	close(f.gate)
	waitState(t, p, "b", StateFailed)
	if state, _ := p.State(key("a")); state != StateIdle {
		t.Errorf("retrying part after a quota failure = %s, want idle", state)
	}
}

func TestPreparerFetchRetriesFailedPart(t *testing.T) {
	f := newFakeSynth()
	f.failNext("a", 1, &tts.Error{Kind: tts.KindQuota, Provider: "fake", Message: "rate limit"})
	health := tts.NewHealth("fake", "model")
	p := NewPreparer(PreparerConfig{
		Store: NewStore(t.TempDir()), Synth: f.synth, Health: health, Workers: 1,
	})
	close(f.gate)
	p.Queue(items("a"))
	waitState(t, p, "a", StateFailed)
	if got := health.Snapshot(); got.Status != tts.StatusDegraded {
		t.Errorf("health after the failure = %+v, want degraded", got)
	}

	audio, err := p.Fetch(context.Background(), key("a"), "a")
	if err != nil || len(audio.Data) == 0 {
		t.Fatalf("Fetch after a failure = %d bytes, %v; want the retried clip", len(audio.Data),
			err)
	}
	if state, _ := p.State(key("a")); state != StateReady || len(f.calls()) != 2 {
		t.Errorf("after the retry: state %s, %d syntheses; want ready after 2", state,
			len(f.calls()))
	}
	if got := health.Snapshot(); got.Status != tts.StatusOK {
		t.Errorf("health after the retry = %+v, want ok", got)
	}
}

func TestPreparerQueueSkipsStoredClips(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 1)
	if err := p.cfg.Store.Put(key("a"), tts.Audio{Data: []byte("RIFF")}); err != nil {
		t.Fatal(err)
	}
	p.Queue(items("a"))
	if state, _ := p.State(key("a")); state != StateReady {
		t.Errorf("state of a stored clip = %s, want ready", state)
	}
	if state, _ := p.State(key("never")); state != StateIdle {
		t.Errorf("state of an unknown clip = %s, want idle", state)
	}
	close(f.gate)
	if got := f.calls(); len(got) != 0 {
		t.Errorf("queueing a stored clip synthesized %v", got)
	}
}

// TestPreparerNewestQueueGoesFirst pins the order across two uploads: the
// later document's parts run before the earlier one's that still wait, each
// in reading order, and a part both hold moves up with the later one.
func TestPreparerNewestQueueGoesFirst(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 1)
	p.Queue(items("a1", "a2", "a3", "shared"))
	<-f.started // the one worker holds a1
	p.Queue(items("b1", "shared", "b2"))
	close(f.gate)
	for _, text := range []string{"a1", "a2", "a3", "shared", "b1", "b2"} {
		waitState(t, p, text, StateReady)
	}
	want := []string{"a1", "b1", "shared", "b2", "a2", "a3"}
	if got := f.calls(); !slices.Equal(got, want) {
		t.Errorf("synthesis order = %v, want %v", got, want)
	}
}

// TestPreparerForgetDropsQueuedParts pins eviction: forgotten parts that
// wait go back to idle and are never synthesized, while one already
// generating finishes into the store.
func TestPreparerForgetDropsQueuedParts(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 1)
	p.Queue(items("a", "b", "c"))
	<-f.started // the one worker holds a
	p.Forget([]string{key("a"), key("b")})
	if state, _ := p.State(key("b")); state != StateIdle {
		t.Errorf("forgotten queued part = %s, want idle", state)
	}
	close(f.gate)
	waitState(t, p, "a", StateReady)
	waitState(t, p, "c", StateReady)
	if got := f.calls(); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("syntheses = %v, want [a c] (b forgotten, a left to finish)", got)
	}
}

// TestPreparerPendingReportsWorkInFlight pins what the document registry
// evicts by: a generating or queued part counts as work in flight; an
// unknown, forgotten or stored one does not.
func TestPreparerPendingReportsWorkInFlight(t *testing.T) {
	f := newFakeSynth()
	p := newTestPreparer(t, f, 1)
	if p.Pending([]string{key("a")}) {
		t.Error("an unknown part counts as pending")
	}
	p.Queue(items("a", "b"))
	<-f.started // the one worker holds a
	if !p.Pending([]string{key("a")}) || !p.Pending([]string{key("x"), key("b")}) {
		t.Error("a generating part or a queued one among others does not count as pending")
	}
	p.Forget([]string{key("b")})
	if p.Pending([]string{key("b")}) {
		t.Error("a forgotten part counts as pending")
	}
	close(f.gate)
	waitState(t, p, "a", StateReady)
	if p.Pending([]string{key("a")}) {
		t.Error("a stored part counts as pending")
	}
}

// TestPreparerHaltsWhenOutOfAttemptsOnTimeouts pins the cost guard: a part
// that spends every attempt on a timeout says the provider is stalling, so
// the parts queued behind it go back to idle instead of each spending its
// own attempts. Running out on another upstream failure does not.
func TestPreparerHaltsWhenOutOfAttemptsOnTimeouts(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantAfter State
	}{
		{"timeouts", &tts.Error{
			Kind: tts.KindUpstream, Provider: "fake", Message: "fake did not answer within 1m30s",
			Err: context.DeadlineExceeded,
		}, StateIdle},
		{"other upstream failures", &tts.Error{
			Kind: tts.KindUpstream, Provider: "fake", Message: "engine returned 500",
		}, StateReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeSynth()
			f.failNext("a", maxAttempts, tc.err)
			p := newTestPreparer(t, f, 1) // no backoff: a's retries go first
			p.Queue(items("a", "b", "c"))
			close(f.gate)
			waitState(t, p, "a", StateFailed)
			for _, text := range []string{"b", "c"} {
				waitState(t, p, text, tc.wantAfter)
			}
		})
	}
}
