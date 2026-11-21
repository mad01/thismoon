package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/cards"
)

// dumbTerm is the TERM value of a terminal that can neither move the cursor
// nor show color, so it cannot draw the UI.
const dumbTerm = "dumb"

// terminal is what doctor probes: the controlling terminal both modes draw
// on, the environment, and the color profile detection the UI runs on that
// terminal. Tests fill it by hand, so no check needs a real terminal.
type terminal struct {
	// open opens the controlling terminal for reading and writing.
	open func() (io.WriteCloser, error)
	// environ is the environment in os.Environ form: KEY=value entries.
	environ []string
	// profile detects the color profile of out under environ.
	profile func(out io.Writer, environ []string) colorprofile.Profile
}

// systemTerminal returns the terminal the modes run on: ttyPath, the process
// environment, and the same detection the UI runs on its output.
func systemTerminal() terminal {
	return terminal{
		open: func() (io.WriteCloser, error) {
			return os.OpenFile(ttyPath, os.O_RDWR, 0)
		},
		environ: os.Environ(),
		profile: colorprofile.Detect,
	}
}

func doctorCmd() *cobra.Command {
	return agentcli.DoctorCommand(cards.Facts(), func(context.Context) []doctor.Check {
		return systemTerminal().checks()
	})
}

// checks returns the doctor checks in order: the terminal opens, TERM names
// a terminal that can draw the UI, and that terminal shows 16 colors or more.
func (t terminal) checks() []doctor.Check {
	termName := "TERM"
	if v, ok := lookupEnv(t.environ, "TERM"); ok && v != "" {
		termName = fmt.Sprintf("TERM (%s)", v)
	}
	return []doctor.Check{
		{Name: "terminal (" + ttyPath + ")", Run: t.checkOpen},
		{Name: termName, Run: t.checkTerm},
		{Name: "colors", Run: t.checkColors},
	}
}

// checkOpen fails when the controlling terminal cannot be opened: both modes
// read keys from it and draw on it.
func (t terminal) checkOpen(context.Context) error {
	tty, err := t.open()
	if err != nil {
		return fmt.Errorf("%w (cards reads keys from the terminal, so run it in a terminal pane)",
			err)
	}
	return tty.Close()
}

// checkTerm fails when TERM is unset, empty or dumb.
func (t terminal) checkTerm(context.Context) error {
	v, ok := lookupEnv(t.environ, "TERM")
	switch {
	case !ok || v == "":
		return errors.New("TERM is not set; start cards from a terminal, which sets it")
	case v == dumbTerm:
		return errors.New("TERM is dumb, which cannot draw the UI; " +
			"set it to your terminal's own value, such as xterm-256color")
	}
	return nil
}

// checkColors fails when the terminal shows fewer than the 16 colors card
// tones are drawn in. With no terminal to detect on it skips: checkOpen
// already fails for that.
func (t terminal) checkColors(context.Context) error {
	tty, err := t.open()
	if err != nil {
		return doctor.Skip("skipped: no terminal to detect colors on")
	}
	p := t.profile(tty, t.environ)
	if err := tty.Close(); err != nil {
		return err
	}
	if p < colorprofile.ANSI {
		return fmt.Errorf("detected %s, want ANSI (16 colors) or more; "+
			"check TERM, and NO_COLOR, which turns color off", p)
	}
	return nil
}

// lookupEnv returns the value of key in environ, the last entry winning as
// it does for the process environment.
func lookupEnv(environ []string, key string) (string, bool) {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], key+"="); ok {
			return v, true
		}
	}
	return "", false
}
