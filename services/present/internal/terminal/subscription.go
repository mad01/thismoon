package terminal

// Subscription is one viewer of a session: the output it has missed, the
// live output after it, and the input and size of the terminal. Close it
// when the viewer goes; a session left with no subscriber starts its idle
// timer.
type Subscription struct {
	s      *session
	replay []byte
	ready  chan struct{} // see Ready

	// Guarded by s.mu.
	pending   []byte // live output not yet taken, at most pendingLimit bytes
	outClosed bool
	exited    bool
	code      int
}

func newSubscription(s *session, replay []byte) *Subscription {
	return &Subscription{s: s, replay: replay, ready: make(chan struct{}, 1)}
}

// Replay returns the session's recent output as it stood when the
// subscription started, up to the ring size. Take carries what comes after
// it.
func (sub *Subscription) Replay() []byte {
	return sub.replay
}

// Ready receives a value whenever live output is waiting for Take, and is
// closed once the output has ended. Signals coalesce: one receive may stand
// for any number of writes, and Take drains them all.
func (sub *Subscription) Ready() <-chan struct{} {
	return sub.ready
}

// Take returns all the live output queued since the last Take, in order,
// and whether the output has ended. Once it has ended nothing more is
// queued: the process exited and all its output is in what Take returned,
// the subscriber fell more than pendingLimit bytes behind and was dropped,
// or the subscription closed. Exit tells the first case from the others.
// The returned bytes belong to the caller.
func (sub *Subscription) Take() (p []byte, ended bool) {
	sub.s.mu.Lock()
	defer sub.s.mu.Unlock()
	p, sub.pending = sub.pending, nil
	return p, sub.outClosed
}

// Exit reports the exit code once the output has ended because the process
// did. exited is false while the process runs, and for a subscription that
// was dropped or closed before the exit reached it.
func (sub *Subscription) Exit() (code int, exited bool) {
	sub.s.mu.Lock()
	defer sub.s.mu.Unlock()
	return sub.code, sub.exited
}

// Write queues p as typed input for the process and returns without
// waiting for the process to read it. It returns ErrExited once the
// process has ended, ErrInputFull while the session holds as much unread
// input as it takes, and the write error once the terminal refused input.
func (sub *Subscription) Write(p []byte) (int, error) {
	return sub.s.write(p)
}

// Resize sets the terminal size. Every subscriber shares one terminal, so
// the last resize wins.
func (sub *Subscription) Resize(cols, rows uint16) error {
	return sub.s.resize(cols, rows)
}

// Close ends the subscription and its output. It is safe to call more than
// once.
func (sub *Subscription) Close() {
	sub.s.unsubscribe(sub)
}

// queue adds p to the pending output and reports false, queueing nothing,
// when that would put the subscriber more than limit bytes behind. The
// caller holds s.mu.
func (sub *Subscription) queue(p []byte, limit int) bool {
	if len(sub.pending)+len(p) > limit {
		return false
	}
	sub.pending = append(sub.pending, p...)
	select {
	case sub.ready <- struct{}{}:
	default: // a signal is already waiting
	}
	return true
}

// closeOut ends the output once, recording whether the exit is why. Output
// still pending stays for Take after an exit, and is discarded otherwise:
// a subscriber dropped or closed has no one left to read it. The caller
// holds s.mu.
func (sub *Subscription) closeOut(exited bool, code int) {
	if sub.outClosed {
		return
	}
	sub.outClosed, sub.exited, sub.code = true, exited, code
	if !exited {
		sub.pending = nil
	}
	close(sub.ready)
}
