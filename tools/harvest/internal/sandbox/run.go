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

// maxErrLine bounds the stderr line a failed step reports, so one long
// message cannot swamp the log line it lands in.
const maxErrLine = 300

// Run starts step as Build describes it and waits for it. A confined step
// whose launcher is missing fails before anything starts:
// a step never falls back to running unconfined. A step that fails reports
// the last line it printed to stderr, which is where a harvest step prints
// its error.
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
		if line := lastLine(stderr.String()); line != "" {
			return errors.New(line)
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

// lastLine is the last non-empty line of s, without the "harvest: " that
// main puts before an error, cut to maxErrLine bytes.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	line := strings.TrimSpace(lines[len(lines)-1])
	line = strings.TrimPrefix(line, harvest.Name+": ")
	if len(line) > maxErrLine {
		n := maxErrLine
		for n > 0 && !utf8.RuneStart(line[n]) {
			n--
		}
		line = line[:n] + "..."
	}
	return line
}
