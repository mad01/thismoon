// Package playback is the server-side read-aloud engine behind the speak MCP
// tools. It fetches per-sentence WAV audio from the local TTS engine and plays
// it on the machine's speakers with afplay, controlling pause/resume by sending
// SIGSTOP/SIGCONT to the afplay child. A single afplay session runs at a time,
// serialised across processes by an flock on ~/.local/share/speak/playback.lock
// so two agents can't talk over each other. This is a faithful port of the
// Python speak_mcp.py playback worker.
package playback

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"

	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

const audioTTL = 24 * time.Hour

// State is the coarse playback state reported by Status.
type State string

const (
	StateIdle    State = "idle"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
)

// Result is a tool's reply: an optional session id plus a human-readable message.
type Result struct {
	Session string `json:"session,omitempty" jsonschema:"playback session id; pass it back to speak_pause/speak_resume/speak_stop"`
	Message string `json:"message"           jsonschema:"human-readable status of the call"`
}

// Engine owns the single server-side playback session. All mutable fields are
// guarded by mu; cond wakes the worker when playback resumes.
type Engine struct {
	tts          *ttsclient.Client
	defaultVoice string
	audioDir     string
	lockPath     string

	// injectable seams for tests (default to the TTS client and afplay).
	synth      func(text, voice string) ([]byte, error)
	newPlayCmd func(wavPath string) *exec.Cmd

	mu   sync.Mutex
	cond *sync.Cond

	sessionID    string
	sentences    []string
	voice        string
	currentIndex int
	total        int

	paused  bool
	stopped bool
	running bool

	playCmd    *exec.Cmd
	lock       *flock.Flock
	doneCh     chan struct{}
	lastResult string
}

// New builds an Engine that fetches audio from tts and speaks it aloud. It
// reaps stale WAV files left by earlier runs.
func New(tts *ttsclient.Client, defaultVoice string) *Engine {
	base := expandUser(speak.DefaultStateDir)
	e := &Engine{
		tts:          tts,
		defaultVoice: defaultVoice,
		audioDir:     filepath.Join(base, "audio"),
		lockPath:     filepath.Join(base, "playback.lock"),
	}
	e.cond = sync.NewCond(&e.mu)
	e.synth = func(t, v string) ([]byte, error) { return tts.Synthesize(t, v) }
	e.newPlayCmd = func(wav string) *exec.Cmd { return exec.Command("afplay", wav) }
	e.reapOldAudio()
	return e
}

// SpeakText splits text into sentences and starts playback. Returns immediately.
func (e *Engine) SpeakText(text, voice string) Result {
	v := e.resolveVoice(voice)
	sentences := SplitSentences(text)
	if len(sentences) == 0 {
		return Result{Message: "Nothing to speak."}
	}
	return e.startPlayback(sentences, v, 0, "")
}

// SpeakFile reads a markdown file aloud section by section. sections is a
// comma-separated list of 1-based section indices; empty reads all.
func (e *Engine) SpeakFile(path, voice, sections string) Result {
	v := e.resolveVoice(voice)
	path = expandUser(path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return Result{Message: "File not found: " + path}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{Message: "File not found: " + path}
	}
	all := ExtractSections(data)
	if len(all) == 0 {
		return Result{Message: "No speakable content found."}
	}

	var want map[int]bool
	if strings.TrimSpace(sections) != "" {
		want = map[int]bool{}
		for _, part := range strings.Split(sections, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				want[n-1] = true
			}
		}
	}

	var sentences []string
	for idx, sec := range all {
		if want != nil && !want[idx] {
			continue
		}
		sentences = append(sentences, SplitSentences(sec)...)
	}
	if len(sentences) == 0 {
		return Result{Message: "No speakable content after filtering."}
	}
	return e.startPlayback(sentences, v, 0, "")
}

// Pause suspends the current afplay child and releases the playback lock so
// another process can take over. Resume with Resume.
func (e *Engine) Pause(session string) Result {
	e.mu.Lock()
	if mismatch, ok := e.checkSession(session); !ok {
		e.mu.Unlock()
		return mismatch
	}
	if e.paused {
		e.mu.Unlock()
		return Result{Message: "Already paused."}
	}
	if e.playCmd == nil || e.playCmd.Process == nil {
		e.mu.Unlock()
		return Result{Message: "Nothing is playing."}
	}
	_ = e.playCmd.Process.Signal(syscall.SIGSTOP)
	e.paused = true
	pos, total, sid := e.currentIndex+1, e.total, e.sessionID
	e.mu.Unlock()

	e.releaseLock()
	return Result{
		Session: sid,
		Message: fmt.Sprintf("Paused at sentence %d of %d. Playback lock released.", pos, total),
	}
}

// Resume continues after Pause (SIGCONT) or restarts from the saved position
// after Stop. Re-acquires the playback lock first.
func (e *Engine) Resume(session string) Result {
	e.mu.Lock()
	if mismatch, ok := e.checkSession(session); !ok {
		e.mu.Unlock()
		return mismatch
	}
	alive, paused, hasQueue := e.running, e.paused, len(e.sentences) > 0
	sid := e.sessionID
	from, sents, v := e.currentIndex, e.sentences, e.voice
	e.mu.Unlock()

	switch {
	case alive && paused:
		if owner, ok := e.acquireLock(sid); !ok {
			return Result{Message: busyResponse(owner)}
		}
		e.mu.Lock()
		if e.playCmd != nil && e.playCmd.Process != nil {
			_ = e.playCmd.Process.Signal(syscall.SIGCONT)
		}
		e.paused = false
		e.cond.Signal()
		pos, total := e.currentIndex+1, e.total
		e.mu.Unlock()
		return Result{
			Session: sid,
			Message: fmt.Sprintf("Resumed at sentence %d of %d.", pos, total),
		}
	case !alive && hasQueue:
		return e.startPlayback(sents, v, from, sid)
	default:
		return Result{Message: "Nothing to resume."}
	}
}

// Stop ends playback but saves the position so Resume can continue.
func (e *Engine) Stop(session string) Result {
	e.mu.Lock()
	if mismatch, ok := e.checkSession(session); !ok {
		e.mu.Unlock()
		return mismatch
	}
	savedIndex, savedTotal, sid := e.currentIndex, e.total, e.sessionID
	e.mu.Unlock()

	e.fullStop()
	if savedTotal > 0 {
		return Result{
			Session: sid,
			Message: fmt.Sprintf(
				"Stopped at sentence %d of %d. Call speak_resume to continue.",
				savedIndex+1,
				savedTotal,
			),
		}
	}
	return Result{Message: "Stopped."}
}

// Voices lists the Kokoro voices the engine ships.
func (e *Engine) Voices() Result {
	return Result{Message: strings.Join([]string{
		"af_heart (default, female)",
		"af_bella (female)",
		"af_nicole (female)",
		"af_sarah (female)",
		"af_sky (female)",
		"am_adam (male)",
		"am_michael (male)",
	}, "\n")}
}

// Status reports engine reachability and the current playback state.
func (e *Engine) Status() Result {
	reachable := e.tts.Reachable()

	e.mu.Lock()
	defer e.mu.Unlock()

	var state State
	switch {
	case e.playCmd != nil && e.playCmd.Process != nil:
		if e.paused {
			state = StatePaused
		} else {
			state = StatePlaying
		}
	case len(e.sentences) > 0 && !e.running:
		state = StateStopped
	default:
		state = StateIdle
	}

	position := "n/a"
	if e.total > 0 {
		position = fmt.Sprintf("sentence %d of %d", e.currentIndex+1, e.total)
	}
	locked := e.lock != nil
	holder := e.readLockOwner()
	if locked {
		holder = fmt.Sprintf("this process (pid=%d)", os.Getpid())
	}
	result := e.lastResult
	if result == "" {
		result = "No recent playback."
	}
	sid := e.sessionID
	if sid == "" {
		sid = "none"
	}

	return Result{Session: e.sessionID, Message: fmt.Sprintf(
		"session: %s\nengine_reachable: %t\nstate: %s\nposition: %s\nlocked: %t\nlock_holder: %s\ndefault_voice: %s\nlast_result: %s",
		sid,
		reachable,
		state,
		position,
		locked,
		holder,
		e.defaultVoice,
		result,
	)}
}

// ── internals ──

// startPlayback stops any current session, acquires the lock, and launches the
// worker from startIndex. Reuses sessionID when resuming, else mints a new one.
func (e *Engine) startPlayback(
	sentences []string,
	voice string,
	startIndex int,
	sessionID string,
) Result {
	e.fullStop()

	if sessionID == "" {
		sessionID = uuid.NewString()[:8]
	}
	if owner, ok := e.acquireLock(sessionID); !ok {
		return Result{Message: busyResponse(owner)}
	}

	e.mu.Lock()
	e.sessionID = sessionID
	e.sentences = sentences
	e.voice = voice
	e.total = len(sentences)
	e.currentIndex = startIndex
	e.paused = false
	e.stopped = false
	e.running = true
	e.lastResult = ""
	e.doneCh = make(chan struct{})
	e.mu.Unlock()

	go e.worker(sentences, voice, startIndex)

	if startIndex > 0 {
		remaining := len(sentences) - startIndex
		return Result{
			Session: sessionID,
			Message: fmt.Sprintf(
				"Resuming from sentence %d of %d (%d remaining).",
				startIndex+1,
				len(sentences),
				remaining,
			),
		}
	}
	return Result{
		Session: sessionID,
		Message: fmt.Sprintf(
			"Playing %d sentence(s). Use speak_pause/speak_resume/speak_stop with this session id.",
			len(sentences),
		),
	}
}

func (e *Engine) worker(sentences []string, voice string, start int) {
	spoken := 0
	defer func() {
		e.mu.Lock()
		stopped := e.stopped
		e.running = false
		e.playCmd = nil
		if e.lastResult == "" {
			e.lastResult = fmt.Sprintf("Spoke %d/%d sentence(s).", spoken, len(sentences))
		}
		done := e.doneCh
		e.mu.Unlock()
		// On a clean finish we release the lock; on Stop, fullStop owns release.
		if !stopped {
			e.releaseLock()
		}
		if done != nil {
			close(done)
		}
	}()

	for i := start; i < len(sentences); i++ {
		e.mu.Lock()
		for e.paused && !e.stopped {
			e.cond.Wait()
		}
		if e.stopped {
			e.mu.Unlock()
			return
		}
		e.currentIndex = i
		e.mu.Unlock()

		wavPath, err := e.synthToFile(sentences[i], voice)
		if err != nil {
			e.mu.Lock()
			e.lastResult = "TTS error: " + err.Error()
			e.mu.Unlock()
			return
		}

		cmd := e.newPlayCmd(wavPath)
		if err := cmd.Start(); err != nil {
			e.mu.Lock()
			e.lastResult = "playback error: " + err.Error()
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.playCmd = cmd
		e.mu.Unlock()

		_ = cmd.Wait()

		e.mu.Lock()
		stopped := e.stopped
		e.playCmd = nil
		e.mu.Unlock()
		if stopped {
			return
		}
		spoken++
	}
}

// fullStop tears down any running session and blocks until the worker exits.
func (e *Engine) fullStop() {
	e.mu.Lock()
	if !e.running {
		e.paused = false
		e.mu.Unlock()
		e.releaseLock()
		return
	}
	e.stopped = true
	if e.playCmd != nil && e.playCmd.Process != nil {
		if e.paused {
			_ = e.playCmd.Process.Signal(syscall.SIGCONT)
		}
		_ = e.playCmd.Process.Kill()
	}
	e.paused = false
	e.cond.Broadcast()
	done := e.doneCh
	e.mu.Unlock()

	if done != nil {
		<-done
	}
	e.releaseLock()
}

func (e *Engine) synthToFile(text, voice string) (string, error) {
	data, err := e.synth(text, voice)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(e.audioDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(
		e.audioDir,
		fmt.Sprintf("speak_%d_%d.wav", time.Now().UnixMilli(), os.Getpid()),
	)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (e *Engine) resolveVoice(voice string) string {
	if v := strings.TrimSpace(voice); v != "" {
		return v
	}
	return e.defaultVoice
}

// checkSession must be called with mu held. It returns ok=false and a mismatch
// Result when session is non-empty and does not match the active session.
func (e *Engine) checkSession(session string) (Result, bool) {
	if session != "" && session != e.sessionID {
		return Result{
			Message: fmt.Sprintf("Session mismatch: active=%s, requested=%s", e.sessionID, session),
		}, false
	}
	return Result{}, true
}

// ── cross-process lock ──

func (e *Engine) acquireLock(session string) (owner string, ok bool) {
	if err := os.MkdirAll(filepath.Dir(e.lockPath), 0o755); err != nil {
		return "unknown", false
	}
	fl := flock.New(e.lockPath)
	locked, err := fl.TryLock()
	if err != nil || !locked {
		return e.readLockOwner(), false
	}
	e.mu.Lock()
	e.lock = fl
	e.mu.Unlock()
	e.writeLockOwner(session)
	return "", true
}

func (e *Engine) releaseLock() {
	e.mu.Lock()
	fl := e.lock
	e.lock = nil
	e.mu.Unlock()
	if fl != nil {
		_ = fl.Unlock()
	}
}

func (e *Engine) ownerPath() string { return e.lockPath + ".owner" }

func (e *Engine) writeLockOwner(session string) {
	info, _ := json.Marshal(map[string]any{
		"pid":     os.Getpid(),
		"session": session,
		"ts":      time.Now().Unix(),
	})
	_ = os.WriteFile(e.ownerPath(), info, 0o644)
}

func (e *Engine) readLockOwner() string {
	data, err := os.ReadFile(e.ownerPath())
	if err != nil {
		return "unknown"
	}
	var m struct {
		Pid     int    `json:"pid"`
		Session string `json:"session"`
	}
	if json.Unmarshal(data, &m) != nil {
		return "unknown"
	}
	return fmt.Sprintf("pid=%d, session=%s", m.Pid, m.Session)
}

func busyResponse(owner string) string {
	retry := 3.0 + rand.Float64()*4.0
	return fmt.Sprintf("BUSY | Playback locked by %s. Retry in %.1fs.", owner, retry)
}

func (e *Engine) reapOldAudio() {
	entries, err := os.ReadDir(e.audioDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-audioTTL)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".wav") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(e.audioDir, ent.Name()))
		}
	}
}

// expandUser rewrites a leading ~ or ~/ to the user's home directory. When
// the home directory cannot be resolved, the ~ prefix is stripped so the path
// degrades to cwd-relative instead of creating a literal "~" directory.
func expandUser(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		if path == "~" {
			return "."
		}
		return path[2:]
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
