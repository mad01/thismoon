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
// synthesis until gate is closed, and fails the texts in fail.
type fakeSynth struct {
	gate    chan struct{}
	started chan string // each text as its synthesis starts
	fail    map[string]error

	mu    sync.Mutex
	order []string
}

func newFakeSynth() *fakeSynth {
	return &fakeSynth{
		gate:    make(chan struct{}),
		started: make(chan string, 100),
		fail:    map[string]error{},
	}
}

func (f *fakeSynth) synth(ctx context.Context, text string) (tts.Audio, error) {
	f.mu.Lock()
	f.order = append(f.order, text)
	err := f.fail[text]
	delete(f.fail, text) // a failure happens once; the retry succeeds
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
// queue: those every other part would hit too. The first part fails; the
// parts queued behind it go back to idle only for those kinds.
func TestPreparerHaltsOnSharedFailures(t *testing.T) {
	cases := []struct {
		kind      tts.Kind
		wantAfter State
	}{
		{tts.KindQuota, StateIdle},
		{tts.KindAuth, StateIdle},
		{tts.KindNetwork, StateIdle},
		{tts.KindUpstream, StateReady},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			f := newFakeSynth()
			f.fail["a"] = &tts.Error{Kind: tc.kind, Provider: "fake", Message: "a failed"}
			p := newTestPreparer(t, f, 1)
			p.Queue(items("a", "b", "c"))
			close(f.gate)
			waitState(t, p, "a", StateFailed)
			for _, text := range []string{"b", "c"} {
				waitState(t, p, text, tc.wantAfter)
			}
			if _, err := p.State(key("a")); err == nil || err.Error() != "a failed" {
				t.Errorf("State(a) reason = %v, want the failure", err)
			}
			if tc.wantAfter == StateIdle && len(f.calls()) != 1 {
				t.Errorf("syntheses = %v, want only the failed one", f.calls())
			}
		})
	}
}

func TestPreparerFetchRetriesFailedPart(t *testing.T) {
	f := newFakeSynth()
	f.fail["a"] = &tts.Error{Kind: tts.KindUpstream, Provider: "fake", Message: "engine hiccup"}
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
