package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	// readChunk is the most one PTY read takes.
	readChunk = 32 << 10
	// outBuffer is how many chunks a subscriber may fall behind by before it
	// is dropped.
	outBuffer = 256
	// drainWait bounds how long an exited process's output may keep
	// arriving. A background child that inherited the terminal can hold it
	// open long after the process itself is gone; the exit is reported
	// anyway.
	drainWait = time.Second
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

	mu     sync.Mutex
	ring   *ring
	subs   map[*Subscription]struct{}
	idle   *time.Timer
	waited bool // the process has been reaped; its pid may be reused
	exited bool
	code   int
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
		ring:        newRing(m.cfg.RingBytes),
		subs:        map[*Subscription]struct{}{},
	}
	s.onIdle = func() { m.expire(s) }
	go s.readLoop()
	go s.waitLoop()
	return s, nil
}

// shellArgs runs cmd under a login shell, or the login shell alone.
func shellArgs(cmd string) []string {
	if cmd == "" {
		return []string{"-l"}
	}
	return []string{"-l", "-c", cmd}
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
		}
		if err != nil {
			return
		}
	}
}

// publish appends p to the ring and hands it to every subscriber. A
// subscriber whose buffer is full is dropped, so one slow reader never
// holds up the PTY or the others.
func (s *session) publish(p []byte) {
	chunk := bytes.Clone(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring.write(chunk)
	for sub := range s.subs {
		if sub.outClosed {
			continue
		}
		select {
		case sub.out <- chunk:
		default:
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

// finish records the exit, ends every subscriber's output, and releases the
// PTY.
func (s *session) finish(code int) {
	s.mu.Lock()
	s.exited, s.code = true, code
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
	sub := &Subscription{s: s, out: make(chan []byte, outBuffer), replay: s.ring.bytes()}
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

// kill sends SIGHUP to the session's process group, SIGKILL after the grace
// period, and waits for the exit to be recorded.
func (s *session) kill() {
	s.mu.Lock()
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
	s.mu.Unlock()
	s.signal(syscall.SIGHUP)
	select {
	case <-s.done:
		return
	case <-time.After(s.grace):
	}
	s.signal(syscall.SIGKILL)
	<-s.done
}

// signal sends sig to the session's process group, which the PTY start made
// the process its leader of. A reaped process is left alone: its pid may
// already belong to something else.
func (s *session) signal(sig syscall.Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waited {
		return
	}
	// ESRCH means the group is already gone, which is what kill wants.
	_ = syscall.Kill(-s.info.PID, sig)
}

// write sends p to the process's input.
func (s *session) write(p []byte) (int, error) {
	s.mu.Lock()
	exited := s.exited
	s.mu.Unlock()
	if exited {
		return 0, ErrExited
	}
	n, err := s.pty.Write(p)
	if err != nil {
		return n, fmt.Errorf("terminal: write %s: %w", s.key, err)
	}
	return n, nil
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
