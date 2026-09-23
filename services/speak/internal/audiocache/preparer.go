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
	// StateReady means the clip is stored.
	StateReady State = "ready"
	// StateFailed means the last synthesis of the clip failed; State
	// returns why.
	StateFailed State = "failed"
)

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
}

// Preparer synthesizes clips into a Store in the background. Workers start
// when work is queued, up to Workers, and exit when the queues are empty, so
// a Preparer needs no start or stop. Urgent work (a clip someone is waiting
// to play) always goes before background work.
type Preparer struct {
	cfg PreparerConfig

	mu         sync.Mutex
	jobs       map[string]*job // queued, generating and failed clips by key
	urgent     []*job
	background []*job
	workers    int // running worker goroutines
}

// job is one clip's synthesis. Only urgent jobs have waiters: Fetch moves a
// job to the urgent queue before it waits on it.
type job struct {
	key, text string
	urgent    bool
	state     State
	err       error     // why the job failed
	audio     tts.Audio // the synthesized clip, for waiters
	done      chan struct{}
}

// NewPreparer returns an idle Preparer.
func NewPreparer(cfg PreparerConfig) *Preparer {
	cfg.Workers = max(cfg.Workers, 1)
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
// generating or waited on is left as it is, and one that is stored is only
// marked used.
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

// Forget drops the background work and the failures recorded for keys, so
// their clips read as idle again: the document that wanted them is gone. A
// clip being generated, or one someone waits on, is left to finish.
func (p *Preparer) Forget(keys []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	dropped := make(map[*job]bool)
	for _, key := range keys {
		j := p.jobs[key]
		if j == nil || j.state == StateGenerating || (j.state == StateQueued && j.urgent) {
			continue
		}
		delete(p.jobs, key)
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

// State reports where the clip for key stands, and for a failed clip why.
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
	case p.cfg.Store.Has(key):
		return StateReady, nil
	default:
		return state, err
	}
}

// urgentJob puts the job for key at the back of the urgent queue, reusing a
// queued or generating one, and returns it; nil means the clip is stored.
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
		j.state, j.err = StateFailed, synthErr
		if haltsBackground(synthErr) {
			p.halt()
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

// halt returns every background job to idle: the failure says each of them
// would fail the same way, and a rate limit or a bad key should not spend a
// request per part finding that out. Urgent jobs still run, since someone is
// waiting on each. The caller holds p.mu.
func (p *Preparer) halt() {
	for _, j := range p.background {
		if p.jobs[j.key] == j {
			delete(p.jobs, j.key)
		}
	}
	p.background = nil
}

func (j *job) pending() bool {
	return j.state == StateQueued || j.state == StateGenerating
}

// haltsBackground reports a failure every other synthesis would share: bad
// credentials, a rate limit, an unreachable provider, a config problem or a
// missing model. An upstream failure can be particular to one text, so the
// queue carries on past it.
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
