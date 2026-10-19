// Package terminal owns the live terminal sessions present serve runs: named
// pseudo-terminals, one per page and session name, that outlive the browser
// tab showing them. A session spawns on its first Attach and every later
// Attach reuses it. Its output fans out to every subscriber and into a ring
// buffer that a new subscriber is replayed from first, so a reloaded tab
// shows what happened while it was away. A session ends when it is killed,
// when it has had no subscriber for the idle timeout, or when the manager
// closes, and ending it ends every process left in it. A process that
// exits on its own stays listed with its exit code until then.
package terminal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Defaults a zero Config field falls back to.
const (
	// DefaultShell is the shell every session runs. It starts as an
	// interactive login shell, because the launchd agent serve runs under
	// has neither the mise shims nor ~/code/bin on its PATH, and the user's
	// ~/.zshrc, which sets them up, is read by interactive shells only.
	DefaultShell = "/bin/zsh"
	// DefaultIdleTimeout is how long a session may go without a subscriber
	// before it is killed.
	DefaultIdleTimeout = 2 * time.Hour
	// DefaultKillGrace is how long a killed session has between SIGHUP and
	// SIGKILL.
	DefaultKillGrace = 2 * time.Second
	// DefaultRingBytes is how much recent output a session keeps for replay.
	DefaultRingBytes = 1 << 20
	// DefaultCols and DefaultRows size a session spawned without a size.
	DefaultCols = 80
	DefaultRows = 24
	// DefaultPrefillWait bounds how long after the start a Spec's Prefill
	// waits for the output to go quiet.
	DefaultPrefillWait = 5 * time.Second
)

var (
	// ErrNotFound is returned by Kill for a session the manager does not hold.
	ErrNotFound = errors.New("terminal: no such session")
	// ErrClosed is returned by Attach once the manager has closed.
	ErrClosed = errors.New("terminal: manager closed")
	// ErrInvalidSpec is returned for a spec or size the session cannot use.
	ErrInvalidSpec = errors.New("terminal: invalid spec")
	// ErrExited is returned by a write to a session whose process has ended.
	ErrExited = errors.New("terminal: session has exited")
	// ErrInputFull is returned by a write while the session holds as much
	// typed input as it takes and its process is not reading it.
	ErrInputFull = errors.New("terminal: input backed up")
)

// namePattern is the rule a session name follows: short, lowercase, and
// safe in a URL path segment.
var namePattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// ValidName reports whether name is a usable session name: 1 to 32 of
// lowercase letters, digits, and hyphens.
func ValidName(name string) bool {
	return namePattern.MatchString(name)
}

// Spec is how a session is spawned. It is read only on the Attach that
// spawns the session; an Attach to a running session ignores it.
type Spec struct {
	// Cmd runs under the shell as `zsh -l -i -c <Cmd>`; empty starts the
	// interactive login shell itself.
	Cmd string
	// Cwd is the working directory; empty means the home directory, and a
	// leading ~ is expanded.
	Cwd string
	// Prefill is typed into the session once, with no newline, so it waits
	// on the prompt for Enter. It is typed once the process has printed
	// something and gone quiet, which is when a shell's line editor or a
	// TUI is ready to read, and at most Config.PrefillWait after the start
	// for a program that never prints or never stops.
	Prefill string
	// Cols and Rows are the initial size; zero falls back to DefaultCols and
	// DefaultRows.
	Cols uint16
	Rows uint16
}

// Config tunes a Manager. Every zero field takes its default.
type Config struct {
	// Shell is the shell binary, DefaultShell when empty.
	Shell string
	// Env is the environment sessions start with; nil takes os.Environ()
	// without its TERM, which names the terminal serve runs in rather than
	// xterm.js. TERM is xterm-256color unless Env sets one, and MarkerVar
	// is always set to 1 on top.
	Env []string
	// Home resolves an empty or ~ Cwd, os.UserHomeDir() when empty.
	Home string
	// IdleTimeout, KillGrace, RingBytes, and PrefillWait default to the
	// constants above.
	IdleTimeout time.Duration
	KillGrace   time.Duration
	RingBytes   int
	PrefillWait time.Duration
	// Now stamps a session's start, time.Now when nil.
	Now func() time.Time
}

// Info describes one session for listing.
type Info struct {
	Page     string    `json:"page"`
	Session  string    `json:"session"`
	Cmd      string    `json:"cmd"`
	Cwd      string    `json:"cwd"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Exited   bool      `json:"exited"`
	ExitCode int       `json:"exit_code"`
	Attached int       `json:"attached"`
}

// MarkerVar is set to 1 in every session's environment, so a shell's
// startup files can tell they run under present and skip setup meant for
// a terminal app, such as a theme hook that waits on a reply xterm.js
// never sends.
const MarkerVar = "PRESENT_TERMINAL"

// key names a session: its page and its name on that page.
type key struct{ page, name string }

func (k key) String() string { return k.page + "/" + k.name }

// Manager holds every live session of one serve process.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	sessions map[key]*session
	closed   bool
}

// New returns a Manager with cfg's zero fields set to their defaults.
func New(cfg Config) *Manager {
	if cfg.Shell == "" {
		cfg.Shell = DefaultShell
	}
	if cfg.Env == nil {
		cfg.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
			return strings.HasPrefix(kv, "TERM=")
		})
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.KillGrace <= 0 {
		cfg.KillGrace = DefaultKillGrace
	}
	if cfg.RingBytes <= 0 {
		cfg.RingBytes = DefaultRingBytes
	}
	if cfg.PrefillWait <= 0 {
		cfg.PrefillWait = DefaultPrefillWait
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Manager{cfg: cfg, sessions: map[key]*session{}}
}

// Attach subscribes to the session name on page, spawning it from spec
// when the manager holds none. Joining a running session makes its
// foreground job redraw after the replay, as on a resize: the replay may
// start inside an escape sequence, and a TUI drawn on top of it looks
// broken until it draws again. The caller must Close the subscription.
func (m *Manager) Attach(page, name string, spec Spec) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	k := key{page: page, name: name}
	if s, ok := m.sessions[k]; ok {
		sub := s.subscribe()
		s.redraw()
		return sub, nil
	}
	s, err := m.spawn(k, spec)
	if err != nil {
		return nil, err
	}
	m.sessions[k] = s
	return s.subscribe(), nil
}

// Kill ends the session name on page: SIGHUP to every process group in
// the session its shell leads, SIGKILL after the grace period to whatever
// is left, background jobs and jobs ignoring the hangup included. It
// returns once the process is gone, and the session is no longer listed.
// A process that left the session with setsid, as a daemon does, outlives
// it; nohup alone does not.
func (m *Manager) Kill(page, name string) error {
	k := key{page: page, name: name}
	m.mu.Lock()
	s, ok := m.sessions[k]
	delete(m.sessions, k)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, k)
	}
	s.kill()
	return nil
}

// KillAll kills every session and leaves the manager usable.
func (m *Manager) KillAll() {
	m.mu.Lock()
	all := m.sessions
	m.sessions = map[key]*session{}
	m.mu.Unlock()
	killEach(all)
}

// Close kills every session and refuses every later Attach.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	all := m.sessions
	m.sessions = map[key]*session{}
	m.mu.Unlock()
	killEach(all)
}

// List describes every session the manager holds, by page then name.
func (m *Manager) List() []Info {
	m.mu.Lock()
	out := make([]Info, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s.describe())
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		return out[i].Session < out[j].Session
	})
	return out
}

// expire kills s for idling, unless a subscriber joined it after the timer
// fired or it is no longer the session held under its key.
func (m *Manager) expire(s *session) {
	m.mu.Lock()
	if m.sessions[s.key] != s || s.attached() > 0 {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, s.key)
	m.mu.Unlock()
	s.kill()
}

// resolveCwd turns a spec's Cwd into an absolute directory that exists.
func (m *Manager) resolveCwd(cwd string) (string, error) {
	if cwd == "" || cwd == "~" || strings.HasPrefix(cwd, "~/") {
		home, err := m.home()
		if err != nil {
			return "", err
		}
		cwd = filepath.Join(home, strings.TrimPrefix(cwd, "~"))
	}
	if !filepath.IsAbs(cwd) {
		return "", fmt.Errorf("%w: cwd %q is not absolute", ErrInvalidSpec, cwd)
	}
	st, err := os.Stat(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: cwd: %w", ErrInvalidSpec, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%w: cwd %s is not a directory", ErrInvalidSpec, cwd)
	}
	return cwd, nil
}

func (m *Manager) home() (string, error) {
	if m.cfg.Home != "" {
		return m.cfg.Home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("terminal: home directory: %w", err)
	}
	return home, nil
}

// killEach kills the sessions in parallel and waits for all of them.
func killEach(sessions map[key]*session) {
	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Go(s.kill)
	}
	wg.Wait()
}
