package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const (
	// readChunk is the most one PTY read takes.
	readChunk = 32 << 10
	// inputLimit is how many bytes of typed input a session holds for a
	// process that is not reading; a write past it gets ErrInputFull.
	inputLimit = 4 << 20
	// pendingLimit is how many bytes of live output a subscriber may fall
	// behind by before it is dropped. Counted in bytes, not reads, so a
	// program printing a byte at a time does not drop a reader keeping up.
	pendingLimit = 4 << 20
	// drainWait bounds how long an exited process's output may keep
	// arriving. A background child that inherited the terminal can hold it
	// open long after the process itself is gone; the exit is reported
	// anyway.
	drainWait = time.Second
	// killPoll is how often a kill checks whether the job has gone.
	killPoll = 20 * time.Millisecond
	// hupRepeat is how long after the first SIGHUP of a kill the second
	// follows.
	hupRepeat = 100 * time.Millisecond
	// prefillQuiet is how long the output must have been still before a
	// prefill is typed: a shell has printed its prompt by then and handed
	// the terminal to its line editor. Typed earlier, the terminal echoes
	// the text before the prompt and the line editor draws it again.
	prefillQuiet = 250 * time.Millisecond
)

// session is one PTY and the process running in it.
type session struct {
	key         key
	info        Info // the fixed fields: page, session, cmd, cwd, pid, started
	pty         *os.File
	proc        *exec.Cmd
	grace       time.Duration
	idleTimeout time.Duration
	onIdle      func()

	readDone chan struct{} // closed when the PTY read loop returns
	done     chan struct{} // closed once the exit is recorded
	// output is signalled after each read while a prefill waits; nil
	// without one.
	output chan struct{}
	// inputReady is signalled when input is queued for writeLoop.
	inputReady chan struct{}

	mu     sync.Mutex
	ring   *ring
	subs   map[*Subscription]struct{}
	idle   *time.Timer
	waited bool // the process has been reaped; its pid may be reused
	exited bool
	code   int
	// input is typed input writeLoop has yet to write, inputBytes its
	// size and the write in flight, inputErr why writing stopped.
	input      [][]byte
	inputBytes int
	inputErr   error
}

// spawn starts the session process for k under a fresh PTY.
func (m *Manager) spawn(k key, spec Spec) (*session, error) {
	cwd, err := m.resolveCwd(spec.Cwd)
	if err != nil {
		return nil, err
	}
	cols, rows := sizeOr(spec.Cols, DefaultCols), sizeOr(spec.Rows, DefaultRows)
	cmd := exec.Command(m.cfg.Shell, shellArgs(spec.Cmd)...)
	cmd.Dir = cwd
	cmd.Env = withTerm(m.cfg.Env)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("terminal: start %s: %w", k, err)
	}
	s := &session{
		key: k,
		info: Info{
			Page: k.page, Session: k.name, Cmd: spec.Cmd, Cwd: cwd,
			PID: cmd.Process.Pid, Started: m.cfg.Now(),
		},
		pty:         f,
		proc:        cmd,
		grace:       m.cfg.KillGrace,
		idleTimeout: m.cfg.IdleTimeout,
		readDone:    make(chan struct{}),
		done:        make(chan struct{}),
		inputReady:  make(chan struct{}, 1),
		ring:        newRing(m.cfg.RingBytes),
		subs:        map[*Subscription]struct{}{},
	}
	s.onIdle = func() { m.expire(s) }
	if spec.Prefill != "" {
		s.output = make(chan struct{}, 1)
		go s.typePrefill(spec.Prefill, m.cfg.PrefillWait)
	}
	go s.readLoop()
	go s.writeLoop()
	go s.waitLoop()
	return s, nil
}

// typePrefill types text once the output has gone quiet for prefillQuiet
// after the process first printed, or once wait has passed since the
// start, whichever comes first.
func (s *session) typePrefill(text string, wait time.Duration) {
	limit := time.NewTimer(wait)
	defer limit.Stop()
	var quiet <-chan time.Time // nil until the first output
	timer := time.NewTimer(prefillQuiet)
	defer timer.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-s.output:
			timer.Reset(prefillQuiet)
			quiet = timer.C
			continue
		case <-quiet:
		case <-limit.C:
		}
		// An error means the process has ended, leaving nothing to type
		// into.
		_, _ = s.write([]byte(text))
		return
	}
}

// shellArgs runs cmd under an interactive login shell, or that shell
// alone. Interactive because the user's PATH setup (mise, ~/code/bin) lives
// in ~/.zshrc, which a non-interactive shell never reads.
func shellArgs(cmd string) []string {
	if cmd == "" {
		return []string{"-l", "-i"}
	}
	return []string{"-l", "-i", "-c", cmd}
}

// withTerm copies env with TERM set for xterm.js.
func withTerm(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TERM=") {
			out = append(out, kv)
		}
	}
	return append(out, "TERM=xterm-256color")
}

func sizeOr(v, def uint16) uint16 {
	if v == 0 {
		return def
	}
	return v
}

// readLoop copies PTY output to the ring and the subscribers until the PTY
// read fails, which it does once every holder of the terminal side is gone.
func (s *session) readLoop() {
	defer close(s.readDone)
	buf := make([]byte, readChunk)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.publish(buf[:n])
			s.signalOutput()
		}
		if err != nil {
			return
		}
	}
}

// signalOutput tells a waiting prefill that output arrived.
func (s *session) signalOutput() {
	if s.output == nil {
		return
	}
	select {
	case s.output <- struct{}{}:
	default:
	}
}

// publish appends p to the ring and queues it for every subscriber. A
// subscriber pendingLimit bytes behind is dropped, so one slow reader
// never holds up the PTY or the others. Both copy p.
func (s *session) publish(p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring.write(p)
	for sub := range s.subs {
		if sub.outClosed {
			continue
		}
		if !sub.queue(p, pendingLimit) {
			delete(s.subs, sub)
			sub.closeOut(false, 0)
			s.idleIfAlone()
		}
	}
}

// waitLoop reaps the process, lets its last output drain, and records the
// exit.
func (s *session) waitLoop() {
	err := s.proc.Wait()
	s.mu.Lock()
	s.waited = true
	s.mu.Unlock()
	select {
	case <-s.readDone:
	case <-time.After(drainWait):
	}
	s.finish(exitCode(err))
}

// exitCode maps a Wait result to a shell-style code: the exit status, or
// 128 plus the signal number for a process a signal ended.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return -1
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ee.ExitCode()
}

// finish records the exit, drops the input not yet written, ends every
// subscriber's output, and releases the PTY.
func (s *session) finish(code int) {
	s.mu.Lock()
	s.exited, s.code = true, code
	s.input, s.inputBytes = nil, 0
	for sub := range s.subs {
		sub.closeOut(true, code)
	}
	s.mu.Unlock()
	close(s.done)
	// The read loop may still be blocked on a terminal a background child
	// holds open; the descriptor is released once that read returns.
	_ = s.pty.Close()
}

// subscribe adds a subscriber that starts from the ring's contents. Taking
// the replay under the lock that publish holds keeps replay and live output
// free of gaps and repeats.
func (s *session) subscribe() *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub := newSubscription(s, s.ring.bytes())
	s.subs[sub] = struct{}{}
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	if s.exited {
		sub.closeOut(true, s.code)
	}
	return sub
}

// unsubscribe removes sub; the last one out starts the idle timer.
func (s *session) unsubscribe(sub *Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	sub.closeOut(false, 0)
	s.idleIfAlone()
}

// idleIfAlone starts the idle timer when no subscriber is left. The caller
// holds s.mu.
func (s *session) idleIfAlone() {
	if len(s.subs) == 0 && s.idle == nil {
		s.idle = time.AfterFunc(s.idleTimeout, s.onIdle)
	}
}

func (s *session) attached() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

func (s *session) describe() Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.info
	info.Exited, info.ExitCode, info.Attached = s.exited, s.code, len(s.subs)
	return info
}

// kill sends SIGHUP to every process group in the session, SIGKILL to
// what is left after the grace period, and waits for the exit to be
// recorded. The PTY start made the shell the leader of a session of its
// own, and every job it starts stays in it, the foreground one, background
// ones, and ones that ignore the hangup and outlive the shell alike; the
// terminal is no help there, since the kernel revokes it once the shell is
// gone. A process that left the session with setsid, as a daemon does, is
// not reached. The session is listed again before each later signal, for a
// job started since. A second SIGHUP follows hupRepeat after the first: a
// shell that takes the first just as it replaces itself with the command
// handles it and loses it with the exec.
func (s *session) kill() {
	s.stopIdle()
	deadline := time.Now().Add(s.grace)
	groups := s.jobGroups(nil)
	s.signal(syscall.SIGHUP, groups)
	if s.endedBy(time.Now().Add(min(hupRepeat, s.grace)), groups) {
		return
	}
	groups = s.jobGroups(groups)
	s.signal(syscall.SIGHUP, groups)
	if s.endedBy(deadline, groups) {
		return
	}
	groups = s.jobGroups(groups)
	s.signal(syscall.SIGKILL, groups)
	<-s.done
}

func (s *session) stopIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
}

// jobGroups adds to known the process groups with a process in the
// session. Where the processes cannot be listed it adds the terminal's
// foreground group, which a reaped shell's terminal no longer reports.
func (s *session) jobGroups(known []int) []int {
	groups, ok := sessionGroups(s.info.PID)
	if !ok {
		if fg, err := foregroundGroup(s.pty); err == nil && fg > 0 {
			groups = []int{fg}
		}
	}
	for _, g := range groups {
		if !slices.Contains(known, g) {
			known = append(known, g)
		}
	}
	return known
}

// endedBy reports whether, by deadline, the exit was recorded and no
// process is left in the session, or in groups where the session's
// processes cannot be listed.
func (s *session) endedBy(deadline time.Time, groups []int) bool {
	timeout := time.NewTimer(time.Until(deadline))
	defer timeout.Stop()
	select {
	case <-s.done:
	case <-timeout.C:
		return false
	}
	poll := time.NewTicker(killPoll)
	defer poll.Stop()
	for !s.sessionGone(groups) {
		select {
		case <-poll.C:
		case <-timeout.C:
			return false
		}
	}
	return true
}

// sessionGone reports whether the session has no live process left, or,
// where its processes cannot be listed, whether every one of groups is
// empty.
func (s *session) sessionGone(groups []int) bool {
	if left, ok := sessionGroups(s.info.PID); ok {
		return len(left) == 0
	}
	for _, g := range groups {
		if !groupGone(g) {
			return false
		}
	}
	return true
}

// groupGone reports whether process group pgid has no process left.
func groupGone(pgid int) bool {
	return errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
}

// signal sends sig to the shell's own group until the shell is reaped, and
// to every group in groups. A reaped shell's pid may already belong to
// something else, so it is left alone unless it was listed: a group that
// still had a process when it was listed keeps its id, since the kernel
// hands out no pid in use as a group or session id.
func (s *session) signal(sig syscall.Signal, groups []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// ESRCH means a group is already gone, which is what kill wants.
	if !s.waited {
		_ = syscall.Kill(-s.info.PID, sig)
	}
	for _, g := range groups {
		if g > 0 && (g != s.info.PID || s.waited) {
			_ = syscall.Kill(-g, sig)
		}
	}
}

// foregroundGroup asks the terminal behind f for its foreground process
// group. An interactive shell runs each job in a group of its own, so the
// shell's group alone misses the command the user is looking at.
func foregroundGroup(f *os.File) (int, error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("terminal: pty: %w", err)
	}
	var pgid int
	var ioctlErr error
	err = rc.Control(func(fd uintptr) {
		pgid, ioctlErr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	})
	if err != nil {
		return 0, fmt.Errorf("terminal: pty: %w", err)
	}
	if ioctlErr != nil {
		return 0, fmt.Errorf("terminal: foreground group: %w", ioctlErr)
	}
	return pgid, nil
}

// write queues a copy of p for the process's input and returns without
// waiting for it to be written: a process that is not reading must not
// hold up the caller, which also carries resizes, kills, and the close.
// It fails with ErrExited once the process has ended, with the error that
// stopped writeLoop, and with ErrInputFull past inputLimit.
func (s *session) write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.exited:
		return 0, ErrExited
	case s.inputErr != nil:
		return 0, s.inputErr
	case s.inputBytes+len(p) > inputLimit:
		return 0, fmt.Errorf("%w: %s", ErrInputFull, s.key)
	case len(p) == 0:
		return 0, nil
	}
	s.input = append(s.input, bytes.Clone(p))
	s.inputBytes += len(p)
	select {
	case s.inputReady <- struct{}{}:
	default: // writeLoop already has a signal waiting
	}
	return len(p), nil
}

// writeLoop writes queued input to the PTY in order until the exit is
// recorded or a write fails. A write blocks while the process is not
// reading, and like the read loop it returns once every holder of the
// terminal side is gone.
func (s *session) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.inputReady:
		}
		for p := s.nextInput(); p != nil; p = s.nextInput() {
			_, err := s.pty.Write(p)
			if !s.inputWritten(len(p), err) {
				return
			}
		}
	}
}

// nextInput takes the oldest queued input, nil when there is none.
func (s *session) nextInput() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.input) == 0 {
		return nil
	}
	p := s.input[0]
	s.input[0] = nil
	s.input = s.input[1:]
	return p
}

// inputWritten accounts for n bytes written, or records err and drops the
// queue. It reports whether writing goes on.
func (s *session) inputWritten(n int, err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputBytes -= n
	if err == nil {
		return true
	}
	s.inputErr = fmt.Errorf("terminal: write %s: %w", s.key, err)
	s.input, s.inputBytes = nil, 0
	return false
}

// resize sets the terminal size the process sees.
func (s *session) resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("%w: size %dx%d", ErrInvalidSpec, cols, rows)
	}
	if err := pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return fmt.Errorf("terminal: resize %s: %w", s.key, err)
	}
	return nil
}
