package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/tools/harvest"
)

// maxErrLine bounds the error a failed step reports, so one long message
// cannot swamp the log line it lands in.
const maxErrLine = 300

// Run starts step as Build describes it and waits for it. A confined step
// whose launcher is missing fails before anything starts:
// a step never falls back to running unconfined. A step that fails reports
// the error it printed to stderr, as stepError reads it.
func Run(ctx context.Context, cfg Config, step Step) error {
	if cfg.Confined(step) {
		if err := Check(cfg); err != nil {
			return err
		}
	}
	c := Build(cfg, step)
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := stepError(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return fmt.Errorf("sandbox: %s: %w", c.Path, err)
	}
	return nil
}

// Check reports whether the sandbox can start a step: the launcher is an
// executable file. The launcher itself fails when srt-run or the settings
// file is missing, and that failure is the step's.
func Check(cfg Config) error {
	info, err := os.Stat(cfg.Launcher)
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0) {
		err = fmt.Errorf("%s is not an executable file", cfg.Launcher)
	}
	if err != nil {
		return fmt.Errorf("sandbox: launcher: %w (the harvest recipe links it; "+
			"HARVEST_SANDBOX=off runs the steps without the sandbox)", err)
	}
	return nil
}

// stepError is the error a failed step printed on stderr, as one line. main
// prints an error as "harvest: <err>", and an error can span lines: kit/llm
// puts the body of a failed HTTP call in it. So it is the text from the last
// line that starts with that prefix through the end, without the prefix. A
// stderr with no such line, from the launcher or srt, gives its last
// non-empty line. Either way runs of whitespace become one space and the
// text is cut to maxErrLine bytes, so the start, which names the cause,
// stays.
func stepError(stderr string) string {
	const prefix = harvest.Name + ": "
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	start := len(lines) - 1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], prefix) {
			start = i
			break
		}
	}
	msg := strings.Join(strings.Fields(strings.Join(lines[start:], " ")), " ")
	msg = strings.TrimPrefix(msg, prefix)
	if len(msg) > maxErrLine {
		n := maxErrLine
		for n > 0 && !utf8.RuneStart(msg[n]) {
			n--
		}
		msg = msg[:n] + "..."
	}
	return msg
}
