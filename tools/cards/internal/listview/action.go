package listview

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"syscall"
)

// ProcessStarter starts run actions as real processes.
type ProcessStarter struct{}

// Start runs argv directly, with no shell, in a session of its own, so it
// outlives the UI and never reads the terminal. Its output is discarded. A
// goroutine waits for it, which reaps it when it exits.
func (ProcessStarter) Start(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %q: %w", argv[0], err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// JSONEmitter writes each emit action as one JSON line. Construct with
// NewJSONEmitter.
type JSONEmitter struct {
	w io.Writer
}

// NewJSONEmitter returns a JSONEmitter that writes to w.
func NewJSONEmitter(w io.Writer) *JSONEmitter { return &JSONEmitter{w: w} }

// emitLine is the JSON line an emit action writes.
type emitLine struct {
	Action string `json:"action"`
	ID     string `json:"id"`
}

// Emit writes {"action": action, "id": id} and a newline in a single write,
// so a file opened for append gets whole lines.
func (e *JSONEmitter) Emit(action, id string) error {
	b, err := json.Marshal(emitLine{Action: action, ID: id})
	if err != nil {
		return fmt.Errorf("emit %s: %w", action, err)
	}
	if _, err := e.w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("emit %s: %w", action, err)
	}
	return nil
}
