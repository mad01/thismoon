package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// The files of a job dir. Each step reads what the one before it wrote:
//
//	system.md      the skill file, the model's system prompt (the run)
//	user.md        the header lines and the <digest> block (prepare)
//	lines          the transcript's line count, the next mark (prepare)
//	digest-bytes   the digest's size, for the done line (prepare)
//	reply.txt      the model's reply, untrusted (complete)
//	call.json      the backend, model, cost and tokens of the call (complete)
const (
	systemFile      = "system.md"
	userFile        = "user.md"
	linesFile       = "lines"
	digestBytesFile = "digest-bytes"
	replyFile       = "reply.txt"
	callFile        = "call.json"
)

// Job is one session's job dir: the files the steps of one harvest hand to
// each other. The run creates it before the first step and removes it after
// the last, whatever the outcome, and holds the session's lock (Lock) all
// that time.
type Job struct {
	path string
}

// Counts is what prepare measured and apply reports: the transcript's line
// count, which becomes the session's mark, and the digest's size in bytes.
type Counts struct {
	Lines       int
	DigestBytes int
}

// Job returns the job dir of session. It refuses a session id that cannot
// name a directory, as Mark does. Nothing is created.
func (d *Dir) Job(session string) (Job, error) {
	if err := CheckSession(session); err != nil {
		return Job{}, err
	}
	return Job{path: filepath.Join(d.path, jobsDir, session)}, nil
}

// Path is the job dir itself.
func (j Job) Path() string { return j.path }

// System is the file that holds the model's system prompt.
func (j Job) System() string { return filepath.Join(j.path, systemFile) }

// User is the file that holds the model's user text.
func (j Job) User() string { return filepath.Join(j.path, userFile) }

// Reply is the file that holds the model's reply.
func (j Job) Reply() string { return filepath.Join(j.path, replyFile) }

// Call is the file that describes the model call.
func (j Job) Call() string { return filepath.Join(j.path, callFile) }

// Reset removes what an earlier run of the same session left behind and
// creates the job dir empty. Only its owner can read it: it holds the text
// of a session.
func (j Job) Reset() error {
	if err := j.Remove(); err != nil {
		return err
	}
	if err := os.MkdirAll(j.path, dirMode); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}

// Remove deletes the job dir and everything in it. A job dir that does not
// exist is not an error.
func (j Job) Remove() error {
	if err := os.RemoveAll(j.path); err != nil {
		return fmt.Errorf("state: remove job: %w", err)
	}
	return nil
}

// HasUser reports whether prepare wrote the user text: whether there is
// anything for the model to read.
func (j Job) HasUser() (bool, error) {
	_, err := os.Stat(j.User())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("state: %w", err)
	}
	return true, nil
}

// SetCounts records what prepare measured.
func (j Job) SetCounts(c Counts) error {
	if err := writeAtomic(filepath.Join(j.path, linesFile), strconv.Itoa(c.Lines)+"\n"); err != nil {
		return err
	}
	return writeAtomic(
		filepath.Join(j.path, digestBytesFile), strconv.Itoa(c.DigestBytes)+"\n",
	)
}

// Counts reads what prepare measured.
func (j Job) Counts() (Counts, error) {
	lines, err := readCount(filepath.Join(j.path, linesFile), "line count")
	if err != nil {
		return Counts{}, err
	}
	size, err := readCount(filepath.Join(j.path, digestBytesFile), "byte count")
	if err != nil {
		return Counts{}, err
	}
	return Counts{Lines: lines, DigestBytes: size}, nil
}
