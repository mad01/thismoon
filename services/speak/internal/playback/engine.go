// Package playback is the server-side read-aloud engine behind the speak MCP
// tools. It fetches audio part by part (sentences grouped by package chunk)
// from the active TTS provider and plays it on the machine's speakers with
// afplay, controlling pause/resume by sending SIGSTOP/SIGCONT to the afplay
// child. A single afplay session runs at a time, serialised across processes
// by an flock on playback.lock in the state directory, so two agents can't
// talk over each other. It started as a port of the Python speak_mcp.py
// playback worker.
package playback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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

	"github.com/mad01/thismoon/services/speak/internal/chunk"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

const audioTTL = 24 * time.Hour

// prefetchDepth is how many parts are synthesized ahead of the one playing.
// A remote provider takes seconds per part and answers with the whole clip at
// once, so a single part ahead leaves a gap whenever a short part plays out
// before a long one arrives.
const prefetchDepth = 2

// followingSections sizes the parts of every file section after the first:
// playback is already under way by then, so there is no start to ramp.
var followingSections = chunk.Ramp{chunk.MaxChars}

// State is the coarse playback state reported by Status.
type State string

const (
	StateIdle    State = "idle"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
	// StateStarting is a start synthesizing its first part, which can take
	// minutes on a remote provider.
	StateStarting State = "starting"
)

// Result is a tool's reply: an optional session id plus a human-readable
// message. Failed marks a call that could not start speech, so the MCP layer
// can flag the reply as an error rather than a success that stays silent.
type Result struct {
	Session string `json:"session,omitempty" jsonschema:"playback session id; pass it back to speak_pause/speak_resume/speak_stop"`
	Message string `json:"message"           jsonschema:"human-readable status of the call"`
	Failed  bool   `json:"failed,omitempty"  jsonschema:"true when speech could not start; message says why and nothing is playing"`
}

// Speaker is what playback synthesizes through: the active provider.
type Speaker interface {
	Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error)
}

// Config wires an Engine to its provider and state directory.
type Config struct {
	Speaker      Speaker
	Health       *tts.Health // records every synthesis; speak_status reports it
	DefaultVoice string
	StateDir     string // the resolved --state-dir: audio cache and playback.lock
	// Ping checks a local engine is listening, reporting checked=false for
	// a remote provider. nil means nothing to ping.
	Ping func(ctx context.Context) (checked bool, err error)
}

// Engine owns the single server-side playback session. All mutable fields are
// guarded by mu; cond wakes the worker when playback resumes.
type Engine struct {
	health       *tts.Health
	ping         func(ctx context.Context) (bool, error)
	defaultVoice string
	audioDir     string
	lockPath     string

	// injectable seams for tests (default to the provider and afplay).
	synth      func(ctx context.Context, text, voice string) (tts.Audio, error)
	newPlayCmd func(audioPath string) *exec.Cmd

	// startMu serializes startPlayback and Stop. A start holds the playback
	// lock while it synthesizes the first part, before running is set;
	// without this, a concurrent start or stop would see "not running" and
	// release that lock mid-synthesis, and two sessions could play at once.
	startMu sync.Mutex

	mu   sync.Mutex
	cond *sync.Cond

	sessionID    string
	parts        []string
	voice        string
	currentIndex int
	total        int

	paused  bool
	stopped bool
	running bool
	// starting is set while a start synthesizes its first part: the queue
	// is recorded but no worker runs yet, which is not a stopped session
	// for Resume to restart.
	starting bool

	playCmd *exec.Cmd
	lock    *flock.Flock
	doneCh  chan struct{}
	// cancel ends the session's context: its first synthesis, its
	// prefetches, and the worker's wait for a clip. Stop calls it before
	// taking startMu, so a slow first synthesis cannot hold Stop up.
	cancel     context.CancelFunc
	lastResult string
}

// New builds an Engine that fetches audio from cfg.Speaker and speaks it
// aloud, keeping its audio cache and lock under cfg.StateDir. It reaps stale
// audio files left by earlier runs.
func New(cfg Config) *Engine {
	base := expandUser(cfg.StateDir)
	e := &Engine{
		health:       cfg.Health,
		ping:         cfg.Ping,
		defaultVoice: cfg.DefaultVoice,
		audioDir:     filepath.Join(base, "audio"),
		lockPath:     filepath.Join(base, "playback.lock"),
	}
	e.cond = sync.NewCond(&e.mu)
	e.synth = func(ctx context.Context, t, v string) (tts.Audio, error) {
		return cfg.Speaker.Synthesize(ctx, tts.Request{Text: t, Voice: v})
	}
	e.newPlayCmd = func(wav string) *exec.Cmd { return exec.Command("afplay", wav) }
	e.reapOldAudio()
	return e
}

// SpeakText splits text into parts and starts playback once the first part
// is synthesized.
func (e *Engine) SpeakText(text, voice string) Result {
	v := e.resolveVoice(voice)
	parts := chunk.Parts(text, chunk.Live)
	if len(parts) == 0 {
		return Result{Message: "Nothing to speak."}
	}
	return e.startPlayback(parts, v, 0, "")
}

// unreadable explains a file speak_file cannot read. A file that exists but
// is denied (its mode, or macOS privacy protection on the MCP host) is not
// "not found": the reply names the way around it.
func unreadable(path string, err error) Result {
	if errors.Is(err, fs.ErrPermission) {
		return Result{Failed: true, Message: "UNREADABLE | " + path + ": permission denied. " +
			"Read the file yourself and pass its text to speak_text."}
	}
	return Result{Message: "File not found: " + path}
}

// SpeakFile reads a markdown file aloud section by section. sections is a
// comma-separated list of 1-based section indices; empty reads all.
func (e *Engine) SpeakFile(path, voice, sections string) Result {
	v := e.resolveVoice(voice)
	path = expandUser(path)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return unreadable(path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return unreadable(path, err)
	}
	all := ExtractSections(data)
	if len(all) == 0 {
		return Result{Message: "No speakable content found."}
	}
	parts := sectionParts(all, selectedSections(sections))
	if len(parts) == 0 {
		return Result{Message: "No speakable content after filtering."}
	}
	return e.startPlayback(parts, v, 0, "")
}

// selectedSections parses speak_file's comma-separated 1-based section list
// into 0-based indices. nil means every section.
func selectedSections(sections string) map[int]bool {
	if strings.TrimSpace(sections) == "" {
		return nil
	}
	want := map[int]bool{}
	for _, field := range strings.Split(sections, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(field)); err == nil {
			want[n-1] = true
		}
	}
	return want
}

// sectionParts splits the wanted sections (all when want is nil) into parts.
// The first section that has anything to say ramps up from one sentence;
// later ones start at full size. A part never spans two sections.
func sectionParts(all []string, want map[int]bool) []string {
	var parts []string
	ramp := chunk.Live
	for idx, sec := range all {
		if want != nil && !want[idx] {
			continue
		}
		secParts := chunk.Parts(sec, ramp)
		if len(secParts) == 0 {
			continue
		}
		parts = append(parts, secParts...)
		ramp = followingSections
	}
	return parts
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
		Message: fmt.Sprintf("Paused at part %d of %d. Playback lock released.", pos, total),
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
	alive, paused, hasQueue := e.running, e.paused, len(e.parts) > 0
	starting, sid := e.starting, e.sessionID
	from, parts, v := e.currentIndex, e.parts, e.voice
	e.mu.Unlock()

	switch {
	case starting:
		return Result{
			Session: sid,
			Message: "Playback is starting: the first part is still being synthesized.",
		}
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
			Message: fmt.Sprintf("Resumed at part %d of %d.", pos, total),
		}
	case !alive && hasQueue:
		return e.startPlayback(parts, v, from, sid)
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
	// A start holds startMu while it synthesizes the first part, which can
	// take minutes on a remote provider: cancel it rather than wait it out.
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()

	e.startMu.Lock()
	defer e.startMu.Unlock()
	e.fullStop()
	e.mu.Lock()
	savedIndex, savedTotal, sid := e.currentIndex, e.total, e.sessionID
	e.mu.Unlock()
	if savedTotal > 0 {
		return Result{
			Session: sid,
			Message: fmt.Sprintf(
				"Stopped at part %d of %d. Call speak_resume to continue.",
				savedIndex+1,
				savedTotal,
			),
		}
	}
	return Result{Message: "Stopped."}
}

// Status reports engine reachability, the health recorded from this
// process's syntheses, and the current playback state.
func (e *Engine) Status() Result {
	reachable := "n/a (remote provider)"
	if e.ping != nil {
		if checked, err := e.ping(context.Background()); checked {
			reachable = strconv.FormatBool(err == nil)
		}
	}
	health := e.health.Snapshot()
	checked := "never"
	if !health.CheckedAt.IsZero() {
		checked = time.Since(health.CheckedAt).Round(time.Second).String() + " ago"
	}

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
	case e.starting:
		state = StateStarting
	case len(e.parts) > 0 && !e.running:
		state = StateStopped
	default:
		state = StateIdle
	}

	position := "n/a"
	if e.total > 0 {
		position = fmt.Sprintf("part %d of %d", e.currentIndex+1, e.total)
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
		"session: %s\nprovider: %s (%s)\nengine_reachable: %s\ntts_health: %s\ntts_checked: %s\nstate: %s\nposition: %s\nlocked: %t\nlock_holder: %s\ndefault_voice: %s\nlast_result: %s",
		sid,
		health.Provider,
		health.Model,
		reachable,
		health.Summary(),
		checked,
		state,
		position,
		locked,
		holder,
		e.defaultVoice,
		result,
	)}
}

// ── internals ──

// startPlayback stops any current session, acquires the lock, synthesizes the
// first part, and launches the worker from startIndex. Reuses sessionID
// when resuming, else mints a new one. Synthesizing before the worker starts
// is what lets a dead backend come back as a failed reply instead of a
// "Playing" that stays silent.
func (e *Engine) startPlayback(
	parts []string,
	voice string,
	startIndex int,
	sessionID string,
) Result {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	e.fullStop()

	if sessionID == "" {
		sessionID = uuid.NewString()[:8]
	}
	if owner, ok := e.acquireLock(sessionID); !ok {
		return Result{Message: busyResponse(owner)}
	}

	// The queue is recorded before the first synthesis, so a Stop naming
	// this session can cancel it, and a start that does not get going is
	// kept as a stopped session that speak_resume starts again.
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.setQueue(sessionID, parts, voice, startIndex)
	e.cancel = cancel
	e.starting = true
	e.mu.Unlock()

	firstClip, err := e.synthToFile(ctx, parts[startIndex], voice)
	e.mu.Lock()
	if err != nil || ctx.Err() != nil {
		e.mu.Unlock()
		return e.notStarted(ctx, sessionID, err)
	}
	e.paused = false
	e.stopped = false
	e.running = true
	e.starting = false
	e.lastResult = ""
	e.doneCh = make(chan struct{})
	e.mu.Unlock()

	fetch := newPrefetcher(e.synthToFile, parts, voice, startIndex, firstClip)
	go e.worker(ctx, fetch, startIndex)
	return startedResult(sessionID, len(parts), startIndex)
}

// stoppedBeforeStart is the reply to a start that Stop cancelled while it
// synthesized the first part.
const stoppedBeforeStart = "Stopped before the first part was ready."

// notStarted ends a start whose first part is not playing: Stop cancelled
// it, or the provider failed. Either way the queue stays as a stopped
// session, so speak_resume can start it again.
func (e *Engine) notStarted(ctx context.Context, sessionID string, err error) Result {
	stopped := ctx.Err() != nil
	e.releaseLock()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starting = false
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if stopped {
		e.lastResult = stoppedBeforeStart
		return Result{Session: sessionID, Message: stoppedBeforeStart}
	}
	e.lastResult = "TTS error: " + err.Error()
	return Result{
		Session: sessionID,
		Failed:  true,
		Message: unavailableResponse(e.health.Snapshot()),
	}
}

// setQueue records the session's parts and position. mu must be held.
func (e *Engine) setQueue(sessionID string, parts []string, voice string, index int) {
	e.sessionID = sessionID
	e.parts = parts
	e.voice = voice
	e.total = len(parts)
	e.currentIndex = index
}

// startedResult is the reply to a start that is now playing.
func startedResult(sessionID string, total, startIndex int) Result {
	if startIndex > 0 {
		return Result{
			Session: sessionID,
			Message: fmt.Sprintf(
				"Resuming from part %d of %d (%d remaining).",
				startIndex+1,
				total,
				total-startIndex,
			),
		}
	}
	return Result{
		Session: sessionID,
		Message: fmt.Sprintf(
			"Playing %d part(s). Use speak_pause/speak_resume/speak_stop with this session id.",
			total,
		),
	}
}

// clip is one part's synthesis outcome: the audio file, or why there is none.
type clip struct {
	path string
	err  error
}

// prefetcher synthesizes parts ahead of playback, each in its own goroutine,
// and hands the clips over in reading order. Only the worker calls its
// methods.
type prefetcher struct {
	synth func(ctx context.Context, text, voice string) (string, error)
	parts []string
	voice string
	clips []chan clip // clips[i] delivers part i once it is launched
	next  int         // the next part to launch
}

// newPrefetcher starts at part start, whose clip first is already on disk.
func newPrefetcher(
	synth func(ctx context.Context, text, voice string) (string, error),
	parts []string,
	voice string,
	start int,
	first string,
) *prefetcher {
	p := &prefetcher{
		synth: synth,
		parts: parts,
		voice: voice,
		clips: make([]chan clip, len(parts)),
		next:  start + 1,
	}
	p.clips[start] = make(chan clip, 1)
	p.clips[start] <- clip{path: first}
	return p
}

// fill launches the synthesis of every part up to and including last that
// is not launched yet. Cancelling ctx abandons them.
func (p *prefetcher) fill(ctx context.Context, last int) {
	for ; p.next <= last && p.next < len(p.parts); p.next++ {
		// Buffered, so a clip nobody waits for any more (the session was
		// stopped) does not strand its goroutine.
		ch := make(chan clip, 1)
		p.clips[p.next] = ch
		go func(text string) {
			path, err := p.synth(ctx, text, p.voice)
			ch <- clip{path: path, err: err}
		}(p.parts[p.next])
	}
}

// wait returns part i's clip, or ok=false once ctx is cancelled, including
// when the clip and the cancellation arrive together.
func (p *prefetcher) wait(ctx context.Context, i int) (clip, bool) {
	select {
	case c := <-p.clips[i]:
		return c, ctx.Err() == nil
	case <-ctx.Done():
		return clip{}, false
	}
}

// worker plays parts from start, keeping the next prefetchDepth parts
// synthesizing while one plays. A part that failed to synthesize ends the
// session when playback reaches it, like a failure of the first part.
// Cancelling ctx stops it without waiting for a synthesis.
func (e *Engine) worker(ctx context.Context, fetch *prefetcher, start int) {
	spoken := 0
	defer func() { e.finish(spoken, len(fetch.parts)) }()

	for i := start; i < len(fetch.parts); i++ {
		if !e.advance(i) {
			return
		}
		fetch.fill(ctx, i+prefetchDepth)
		c, ok := fetch.wait(ctx, i)
		if !ok {
			return
		}
		if c.err != nil {
			e.setLastResult("TTS error: " + c.err.Error())
			return
		}
		if !e.play(c.path) {
			return
		}
		spoken++
	}
}

// advance waits out a pause and moves the position to part i. It reports
// false when the session was stopped.
func (e *Engine) advance(i int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for e.paused && !e.stopped {
		e.cond.Wait()
	}
	if e.stopped {
		return false
	}
	e.currentIndex = i
	return true
}

// play runs the player on one clip until it ends. It reports false when the
// player could not start or the session was stopped.
func (e *Engine) play(path string) bool {
	cmd := e.newPlayCmd(path)
	if err := cmd.Start(); err != nil {
		e.setLastResult("playback error: " + err.Error())
		return false
	}
	e.mu.Lock()
	e.playCmd = cmd
	stopped := e.stopped
	e.mu.Unlock()
	if stopped {
		// fullStop ran before playCmd was set, so it had nothing to kill.
		_ = cmd.Process.Kill()
	}

	_ = cmd.Wait()

	e.mu.Lock()
	stopped = e.stopped
	e.playCmd = nil
	e.mu.Unlock()
	return !stopped
}

// finish records how the worker ended and releases what it held.
func (e *Engine) finish(spoken, total int) {
	e.mu.Lock()
	stopped := e.stopped
	e.running = false
	e.playCmd = nil
	if e.lastResult == "" {
		e.lastResult = fmt.Sprintf("Spoke %d/%d part(s).", spoken, total)
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
}

func (e *Engine) setLastResult(msg string) {
	e.mu.Lock()
	e.lastResult = msg
	e.mu.Unlock()
}

// fullStop tears down any running session and blocks until the worker exits.
// It never waits for a synthesis: cancelling the session's context abandons
// the prefetches, and the worker gives up on a clip still in flight.
func (e *Engine) fullStop() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
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

// synthToFile synthesizes one part to an audio file under the audio
// directory, recording the outcome in the engine's health. A synthesis cut
// short by cancelling ctx is not recorded: it says nothing about the
// provider.
func (e *Engine) synthToFile(ctx context.Context, text, voice string) (string, error) {
	audio, err := e.synth(ctx, text, voice)
	if err != nil && ctx.Err() != nil {
		return "", fmt.Errorf("synthesis cancelled: %w", ctx.Err())
	}
	e.health.Record(err)
	if err != nil {
		return "", err
	}
	return e.writeClip(audio)
}

// writeClip saves audio under the audio directory with the extension its
// format needs, which afplay goes by. Prefetched parts are written
// concurrently, so every clip gets a file of its own.
func (e *Engine) writeClip(audio tts.Audio) (string, error) {
	if err := os.MkdirAll(e.audioDir, 0o755); err != nil {
		return "", fmt.Errorf("create audio dir: %w", err)
	}
	f, err := os.CreateTemp(e.audioDir, "speak_*"+audio.Ext())
	if err != nil {
		return "", fmt.Errorf("create clip: %w", err)
	}
	_, err = f.Write(audio.Data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("write clip: %w", err)
	}
	return f.Name(), nil
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

// unavailableResponse is the reply when the first part could not be
// synthesized: the health state names why, in the same "WORD | detail" shape
// as busyResponse.
func unavailableResponse(state tts.State) string {
	return fmt.Sprintf(
		"UNAVAILABLE | TTS %s. Nothing is playing. Call speak_doctor to diagnose, "+
			"then speak_resume to retry.",
		state.Summary(),
	)
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
		if ent.IsDir() || !isAudioFile(ent.Name()) {
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

// isAudioFile reports a clip synthToFile writes: WAV, or MP3 from a
// provider that answers MP3.
func isAudioFile(name string) bool {
	return strings.HasSuffix(name, ".wav") || strings.HasSuffix(name, ".mp3")
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
