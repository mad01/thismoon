package audiocache

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// State is where a clip stands. Serialized as a string in speak's document
// status.
type State string

const (
	// StateIdle means the clip is neither stored nor asked for.
	StateIdle State = "idle"
	// StateQueued means the clip waits for a worker.
	StateQueued State = "queued"
	// StateGenerating means a worker is synthesizing the clip.
	StateGenerating State = "generating"
	// StateRetrying means the last synthesis failed in a way the next may
	// not, and another attempt is scheduled; State returns the failure.
	StateRetrying State = "retrying"
	// StateReady means the clip is stored.
	StateReady State = "ready"
	// StateFailed means the clip could not be made; State returns why and
	// after how many attempts.
	StateFailed State = "failed"
)

// maxAttempts is how many syntheses a clip gets before it counts as failed.
// A remote model stalls now and then on a text it reads fine the next time,
// so one failure is not the text's fault.
const maxAttempts = 3

// retryDelays are the waits before the second and third attempts: long
// enough for a provider having a bad minute to recover, short enough that
// the part is likely ready before the reader gets to it.
var retryDelays = [maxAttempts - 1]time.Duration{15 * time.Second, 45 * time.Second}

// defaultRetryDelay is the backoff after failed attempt number attempt.
func defaultRetryDelay(attempt int) time.Duration {
	return retryDelays[min(max(attempt, 1), len(retryDelays))-1]
}

// Synth synthesizes the clip for one part's text.
type Synth func(ctx context.Context, text string) (tts.Audio, error)

// PreparerConfig is what a Preparer synthesizes with and into.
type PreparerConfig struct {
	Store  *Store
	Synth  Synth
	Health *tts.Health // every synthesis outcome is recorded here; required
	// Workers is how many syntheses run at once; below 1 means 1.
	Workers int
	// Timeout bounds one synthesis, a backstop for a provider that never
	// answers; the provider's own client timeout normally ends it first.
	// Zero means no backstop.
	Timeout time.Duration
	// RetryDelay is the backoff after failed attempt number attempt
	// (1-based); nil means 15 seconds, then 45.
	RetryDelay func(attempt int) time.Duration
}

// Preparer synthesizes clips into a Store in the background. Workers start
// when work is queued, up to Workers, and exit when the queues are empty, so
// a Preparer needs no start or stop. Urgent work (a clip someone is waiting
// to play) always goes before background work.
type Preparer struct {
	cfg PreparerConfig

	mu         sync.Mutex
	jobs       map[string]*job // queued, generating, retrying and failed clips by key
	urgent     []*job
	background []*job
	workers    int // running worker goroutines
}

// job is one clip's synthesis. Only urgent jobs have waiters: Fetch moves a
// job to the urgent queue before it waits on it. A job that will be tried
// again is replaced by a retrying one that carries its attempt count, so
// its waiters keep the failure they waited for.
type job struct {
	key, text string
	urgent    bool
	state     State
	attempts  int         // syntheses started, counting from the last manual request
	retry     *time.Timer // requeues a retrying job; nil otherwise
	err       error       // why the job (or, retrying, its last attempt) failed
	audio     tts.Audio   // the synthesized clip, for waiters
	done      chan struct{}
}

// attemptsError is a failure that says how many attempts were made, and
// whether another is on its way.
type attemptsError struct {
	attempts int
	retrying bool
	err      error
}

func (e *attemptsError) Error() string {
	verb, noun := "failed", "attempts"
	if e.retrying {
		verb = "retrying"
	}
	if e.attempts == 1 {
		noun = "attempt"
	}
	return fmt.Sprintf("%s after %d %s: %v", verb, e.attempts, noun, e.err)
}

func (e *attemptsError) Unwrap() error { return e.err }

// NewPreparer returns an idle Preparer.
func NewPreparer(cfg PreparerConfig) *Preparer {
	cfg.Workers = max(cfg.Workers, 1)
	if cfg.RetryDelay == nil {
		cfg.RetryDelay = defaultRetryDelay
	}
	return &Preparer{cfg: cfg, jobs: make(map[string]*job)}
}

// Item is one clip to prepare: the key it is stored under and its text.
type Item struct {
	Key  string
	Text string
}

// Queue asks for items in the background, in the order given and ahead of
// all background work queued before: the newest document is the one being
// read. An item already queued moves up with the rest. One that is
// generating, waited on or waiting to retry is left as it is, and one that
// is stored is only marked used. A failed item starts over with a fresh
// attempt count.
func (p *Preparer) Queue(items []Item) {
	p.mu.Lock()
	defer p.mu.Unlock()
	batch := make([]*job, 0, len(items))
	moved := make(map[*job]bool)
	for _, it := range items {
		j := p.jobs[it.Key]
		switch {
		case j != nil && j.state == StateQueued && !j.urgent:
			if !moved[j] {
				moved[j] = true
				batch = append(batch, j)
			}
		case j != nil && j.pending():
		case p.cfg.Store.Touch(it.Key):
			// Checked under the lock: a worker stores a clip before it
			// drops the job, so a clip neither pending nor stored is not
			// being made.
		default:
			j = p.newJob(it.Key, it.Text, false)
			moved[j] = true
			batch = append(batch, j)
		}
	}
	rest := slices.DeleteFunc(p.background, func(j *job) bool { return moved[j] })
	p.background = append(batch, rest...)
	p.spawn()
}

// Forget drops the background work, pending retries and failures recorded
// for keys, so their clips read as idle again: the document that wanted them
// is gone. A clip being generated, or one someone waits on, is left to
// finish.
func (p *Preparer) Forget(keys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dropped := make(map[*job]bool)
	for _, key := range keys {
		j := p.jobs[key]
		if j == nil || j.state == StateGenerating || (j.state == StateQueued && j.urgent) {
			continue
		}
		p.drop(j)
		dropped[j] = true
	}
	p.background = slices.DeleteFunc(p.background, func(j *job) bool { return dropped[j] })
}

// Fetch returns the clip for key, synthesizing it ahead of all background
// work when it is not stored yet and waiting for it until ctx ends. A clip
// whose last synthesis failed is tried again.
func (p *Preparer) Fetch(ctx context.Context, key, text string) (tts.Audio, error) {
	if audio, ok, err := p.cfg.Store.Get(key); err != nil || ok {
		return audio, err
	}
	j := p.urgentJob(key, text)
	if j == nil {
		// Stored between the first look and the lock.
		audio, ok, err := p.cfg.Store.Get(key)
		if err == nil && !ok {
			err = fmt.Errorf("audiocache: clip %s left the cache while fetched", key)
		}
		return audio, err
	}
	select {
	case <-j.done:
		if len(j.audio.Data) > 0 {
			return j.audio, nil
		}
		return tts.Audio{}, j.err
	case <-ctx.Done():
		return tts.Audio{}, ctx.Err()
	}
}

// State reports where the clip for key stands, and for a failed or retrying
// clip why ("failed after 3 attempts: ...", "retrying after 1 attempt: ...").
func (p *Preparer) State(key string) (State, error) {
	p.mu.Lock()
	state, err := StateIdle, error(nil)
	if j := p.jobs[key]; j != nil {
		state, err = j.state, j.err
	}
	p.mu.Unlock()
	switch {
	case state == StateQueued || state == StateGenerating:
		return state, nil
	case state == StateRetrying:
		return state, err
	case p.cfg.Store.Has(key):
		return StateReady, nil
	default:
		return state, err
	}
}

// urgentJob puts the job for key at the back of the urgent queue, reusing a
// queued, generating or retrying one, and returns it; nil means the clip is
// stored.
func (p *Preparer) urgentJob(key, text string) *job {
	p.mu.Lock()
	defer p.mu.Unlock()
	j := p.jobs[key]
	switch {
	case j != nil && j.state == StateGenerating:
	case j != nil && j.state == StateQueued:
		if !j.urgent {
			p.background = slices.DeleteFunc(p.background, func(b *job) bool { return b == j })
			j.urgent = true
			p.urgent = append(p.urgent, j)
		}
	case j != nil && j.state == StateRetrying:
		// Someone is listening: the next attempt runs now, not after the
		// backoff.
		j.retry.Stop()
		j.retry, j.state, j.urgent = nil, StateQueued, true
		p.urgent = append(p.urgent, j)
		p.spawn()
	case p.cfg.Store.Has(key):
		return nil
	default:
		// A failed job stays as it is for whoever waited on it; the retry
		// is a new job.
		j = p.newJob(key, text, true)
		p.urgent = append(p.urgent, j)
		p.spawn()
	}
	return j
}

// newJob records a new queued job for key; the caller puts it in a queue.
// The caller holds p.mu.
func (p *Preparer) newJob(key, text string, urgent bool) *job {
	j := &job{key: key, text: text, urgent: urgent, state: StateQueued, done: make(chan struct{})}
	p.jobs[key] = j
	return j
}

// spawn starts workers for queued jobs, up to Workers in all. The caller
// holds p.mu.
func (p *Preparer) spawn() {
	for queued := len(p.urgent) + len(p.background); queued > 0 &&
		p.workers < p.cfg.Workers; queued-- {
		p.workers++
		go p.work()
	}
}

// work runs jobs until both queues are empty.
func (p *Preparer) work() {
	for j := p.next(); j != nil; j = p.next() {
		p.run(j)
	}
}

// next takes the next job, urgent first, or retires the worker when there
// is none.
func (p *Preparer) next() *job {
	p.mu.Lock()
	defer p.mu.Unlock()
	var j *job
	switch {
	case len(p.urgent) > 0:
		j, p.urgent = p.urgent[0], p.urgent[1:]
	case len(p.background) > 0:
		j, p.background = p.background[0], p.background[1:]
	default:
		p.workers--
		return nil
	}
	j.state = StateGenerating
	j.attempts++
	return j
}

// run synthesizes and stores one clip.
func (p *Preparer) run(j *job) {
	audio, err := p.synthesize(j.text)
	p.cfg.Health.Record(err)
	var storeErr error
	if err == nil {
		storeErr = p.cfg.Store.Put(j.key, audio)
	}
	p.finish(j, audio, err, storeErr)
}

// synthesize runs one synthesis under the Timeout backstop. The context is
// the Preparer's own, not a waiter's: a clip whose listener left is still
// worth storing.
func (p *Preparer) synthesize(text string) (tts.Audio, error) {
	ctx := context.Background()
	if p.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.Timeout)
		defer cancel()
	}
	return p.cfg.Synth(ctx, text)
}

// finish records a job's outcome and wakes its waiters. A clip that was
// synthesized but could not be stored still goes to them.
func (p *Preparer) finish(j *job, audio tts.Audio, synthErr, storeErr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer close(j.done)
	switch {
	case synthErr != nil:
		j.state, j.err = StateFailed, &attemptsError{attempts: j.attempts, err: synthErr}
		switch {
		case haltsBackground(synthErr):
			p.halt()
		case j.attempts >= maxAttempts && tts.TimedOut(synthErr):
			// A part out of attempts on stalls says the provider is
			// stalling, not that this text is hard: each queued part would
			// spend its attempts the same way.
			p.halt()
		case j.attempts < maxAttempts && p.jobs[j.key] == j:
			p.scheduleRetry(j, synthErr)
		}
		return
	case storeErr != nil:
		j.state, j.err = StateFailed, storeErr
	default:
		j.state = StateReady
		if p.jobs[j.key] == j {
			delete(p.jobs, j.key) // the store answers for it from here
		}
	}
	j.audio = audio
}

// scheduleRetry replaces job j, which failed with err, by a retrying one
// that goes back to the front of the background queue after the backoff.
// The caller holds p.mu.
func (p *Preparer) scheduleRetry(j *job, err error) {
	next := &job{
		key:      j.key,
		text:     j.text,
		state:    StateRetrying,
		attempts: j.attempts,
		err:      &attemptsError{attempts: j.attempts, retrying: true, err: err},
		done:     make(chan struct{}),
	}
	p.jobs[j.key] = next
	delay := p.cfg.RetryDelay(j.attempts)
	if delay <= 0 {
		p.requeueLocked(next)
		return
	}
	next.retry = time.AfterFunc(delay, func() { p.requeue(next) })
}

// requeue puts a retrying job whose backoff ended at the front of the
// background queue: it is earlier in its document than what waits there. A
// job forgotten or promoted in the meantime is left alone.
func (p *Preparer) requeue(j *job) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.jobs[j.key] != j || j.state != StateRetrying {
		return
	}
	p.requeueLocked(j)
}

// requeueLocked is requeue for a caller that holds p.mu.
func (p *Preparer) requeueLocked(j *job) {
	j.retry, j.state = nil, StateQueued
	p.background = append([]*job{j}, p.background...)
	p.spawn()
}

// drop forgets job j, stopping its pending retry. The caller holds p.mu.
func (p *Preparer) drop(j *job) {
	if j.retry != nil {
		j.retry.Stop()
		j.retry = nil
	}
	if p.jobs[j.key] == j {
		delete(p.jobs, j.key)
	}
}

// halt returns every background job, and every one waiting to retry, to
// idle: the failure says each of them would fail the same way, and a rate
// limit or a bad key should not spend a request per part finding that out.
// Urgent jobs still run, since someone is waiting on each. The caller holds
// p.mu.
func (p *Preparer) halt() {
	for _, j := range p.background {
		p.drop(j)
	}
	p.background = nil
	for _, j := range p.jobs {
		if j.state == StateRetrying {
			p.drop(j)
		}
	}
}

func (j *job) pending() bool {
	return j.state == StateQueued || j.state == StateGenerating || j.state == StateRetrying
}

// haltsBackground reports a failure every other synthesis would share: bad
// credentials, a rate limit, an unreachable provider, a config problem or a
// missing model. Trying again would not help either, so these are not
// retried. An upstream failure (a timeout included) can be particular to one
// text or one moment, so the queue carries on past it and retries it; only a
// part that runs out of attempts on a timeout halts the queue (see finish).
func haltsBackground(err error) bool {
	failure, ok := errors.AsType[*tts.Error](err)
	if !ok {
		return false
	}
	switch failure.Kind {
	case tts.KindAuth, tts.KindQuota, tts.KindNetwork, tts.KindConfig, tts.KindModel:
		return true
	default:
		return false
	}
}
