// Package state owns harvest's state dir and is the only package that knows
// its layout:
//
//	<dir>/harvest.log           one timestamped line per hook call or run
//	<dir>/sessions/<id>.lines   how many transcript lines a harvest already read
//	<dir>/hint-day              the day the hint last spoke, YYYY-MM-DD
//	<dir>/jobs/<id>/            the files the steps of one harvest hand on
//	<dir>/locks/<id>.lock       the lock a run of the session holds
//
// Marks and the stamp are written to a temp file and renamed into place, so a
// reader never sees half a number. Log entries are appended in one write each.
// A job dir lives only while its harvest runs. Everything is its owner's
// alone: the log names sessions and the job dirs hold their text.
package state

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mad01/thismoon/tools/harvest"
)

const (
	sessionsDir = "sessions"
	jobsDir     = "jobs"
	locksDir    = "locks"
	markSuffix  = ".lines"
	lockSuffix  = ".lock"
	stampFile   = "hint-day"
	// dirMode and fileMode are the modes of what the state dir holds.
	dirMode  fs.FileMode = 0o700
	fileMode fs.FileMode = 0o600
	// logTimeLayout matches `date '+%Y-%m-%dT%H:%M:%S%z'`.
	logTimeLayout = "2006-01-02T15:04:05-0700"
	// tailWindow is how far back from the end of the log LogTail reads. A
	// log line is short, so this holds far more lines than a caller asks for.
	tailWindow = 64 << 10
)

// ErrBadSession means a session id cannot name a mark file: it is empty, it
// would reach outside the sessions directory, or it holds a control
// character, which would break the log line it lands in.
var ErrBadSession = errors.New("state: session id is not a plain file name")

// Dir is one state directory.
type Dir struct {
	path string
	now  func() time.Time
}

// New returns the state dir at path. now stamps the log lines; pass
// time.Now outside tests.
func New(path string, now func() time.Time) *Dir {
	return &Dir{path: path, now: now}
}

// Path is the state dir itself.
func (d *Dir) Path() string { return d.path }

// LogPath is the log file inside the state dir.
func (d *Dir) LogPath() string { return filepath.Join(d.path, harvest.LogFile) }

// Ensure creates the state dir and its sessions directory, readable by their
// owner only, and narrows the mode of either when an earlier harvest made it
// wider.
func (d *Dir) Ensure() error {
	sessions := filepath.Join(d.path, sessionsDir)
	if err := os.MkdirAll(sessions, dirMode); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if err := tighten(d.path, dirMode); err != nil {
		return err
	}
	return tighten(sessions, dirMode)
}

// tighten sets the permission bits of path to perm when they differ. It
// changes nothing when they match, so a confined step, whose state dir the
// unconfined run already narrowed, makes no chmod.
func tighten(path string, perm fs.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if info.Mode().Perm() == perm {
		return nil
	}
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}

// Log appends one timestamped line.
func (d *Dir) Log(msg string) error { return d.LogBlock(msg, "") }

// LogBlock appends a timestamped line followed by block, each of its lines
// indented two spaces: the shape of a done line and the result under it.
func (d *Dir) LogBlock(msg, block string) error {
	var b strings.Builder
	b.WriteString(d.now().Format(logTimeLayout))
	b.WriteByte(' ')
	b.WriteString(msg)
	b.WriteByte('\n')
	if block = strings.TrimRight(block, "\n"); block != "" {
		for line := range strings.SplitSeq(block, "\n") {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if err := os.MkdirAll(d.path, dirMode); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	f, err := os.OpenFile(d.LogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("state: open log: %w", err)
	}
	// A log an earlier harvest created readable to all becomes its owner's.
	if err := tighten(d.LogPath(), fileMode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return fmt.Errorf("state: write log: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("state: close log: %w", err)
	}
	return nil
}

// LogTail returns the last n lines of the log, oldest first. A log that does
// not exist yet has no lines and is not an error.
func (d *Dir) LogTail(n int) ([]string, error) {
	f, err := os.Open(d.LogPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: open log: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("state: stat log: %w", err)
	}
	off := max(0, info.Size()-tailWindow)
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("state: read log: %w", err)
	}
	text := string(buf)
	if off > 0 {
		// The window starts inside a line; drop that partial line.
		_, text, _ = strings.Cut(text, "\n")
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// Mark returns how many transcript lines the last harvest of session read:
// zero when it was never harvested.
func (d *Dir) Mark(session string) (int, error) {
	p, err := d.markPath(session)
	if err != nil {
		return 0, err
	}
	n, err := readCount(p, "line count")
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	return n, err
}

// readCount reads a file that holds one count, as a mark does. what names
// the count for the error.
func readCount(path, what string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("state: read %s: %w", filepath.Base(path), err)
	}
	raw := strings.TrimSpace(string(b))
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("state: %s holds %q, not a %s", path, raw, what)
	}
	return n, nil
}

// SetMark records that a harvest of session read its first lines lines.
func (d *Dir) SetMark(session string, lines int) error {
	p, err := d.markPath(session)
	if err != nil {
		return err
	}
	if err := d.Ensure(); err != nil {
		return err
	}
	return writeAtomic(p, strconv.Itoa(lines)+"\n")
}

// Stamp returns the day the hint last spoke, or "" when it never did.
func (d *Dir) Stamp() (string, error) {
	b, err := os.ReadFile(filepath.Join(d.path, stampFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("state: read stamp: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// SetStamp records day as the day the hint last spoke.
func (d *Dir) SetStamp(day string) error {
	if err := os.MkdirAll(d.path, dirMode); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return writeAtomic(filepath.Join(d.path, stampFile), day+"\n")
}

func (d *Dir) markPath(session string) (string, error) {
	if err := CheckSession(session); err != nil {
		return "", err
	}
	return filepath.Join(d.path, sessionsDir, session+markSuffix), nil
}

// CheckSession refuses a session id that cannot name a file of its own or
// cannot stand in a log line: it wraps ErrBadSession, with the id quoted so
// the message is one line whatever the id holds.
func CheckSession(session string) error {
	if session == "" || session == "." || session == ".." ||
		strings.ContainsRune(session, '/') || strings.ContainsFunc(session, unicode.IsControl) {
		return fmt.Errorf("%w: %q", ErrBadSession, session)
	}
	return nil
}

// writeAtomic writes content to a temp file beside path and renames it over
// path.
func writeAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("state: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("state: %w", err)
	}
	return nil
}
