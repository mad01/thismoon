package terminal

// Subscription is one viewer of a session: the output it has missed, the
// live output after it, and the input and size of the terminal. Close it
// when the viewer goes; a session left with no subscriber starts its idle
// timer.
type Subscription struct {
	s      *session
	out    chan []byte
	replay []byte

	// Guarded by s.mu.
	outClosed bool
	exited    bool
	code      int
}

// Replay returns the session's recent output as it stood when the
// subscription started, up to the ring size. Output carries what comes
// after it.
func (sub *Subscription) Replay() []byte {
	return sub.replay
}

// Output delivers the live output in order. Chunks are shared between
// subscribers and must not be modified. The channel closes when the process
// has exited and all its output is delivered, when the subscriber fell too
// far behind and was dropped, or when the subscription closed; Exit tells
// the first case from the others.
func (sub *Subscription) Output() <-chan []byte {
	return sub.out
}

// Exit reports the exit code once Output has closed because the process
// ended. exited is false while the process runs, and for a subscription
// that was dropped or closed before the exit reached it.
func (sub *Subscription) Exit() (code int, exited bool) {
	sub.s.mu.Lock()
	defer sub.s.mu.Unlock()
	return sub.code, sub.exited
}

// Write sends p to the process as typed input. It returns ErrExited once
// the process has ended.
func (sub *Subscription) Write(p []byte) (int, error) {
	return sub.s.write(p)
}

// Resize sets the terminal size. Every subscriber shares one terminal, so
// the last resize wins.
func (sub *Subscription) Resize(cols, rows uint16) error {
	return sub.s.resize(cols, rows)
}

// Close ends the subscription and closes Output. It is safe to call more
// than once.
func (sub *Subscription) Close() {
	sub.s.unsubscribe(sub)
}

// closeOut closes the output channel once, recording whether the exit is
// why. The caller holds s.mu.
func (sub *Subscription) closeOut(exited bool, code int) {
	if sub.outClosed {
		return
	}
	sub.outClosed, sub.exited, sub.code = true, exited, code
	close(sub.out)
}
