package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/tools/harvest"
	"github.com/mad01/thismoon/tools/harvest/internal/sandbox"
	"github.com/mad01/thismoon/tools/harvest/internal/settings"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// doctorLogLines is how much of the log doctor shows under the checks:
// enough for the last few sessions' outcomes.
const doctorLogLines = 10

// doctorCmd runs the checks, then prints the end of the log: a harvest runs
// with nobody watching, so the log is where its outcomes are.
func (a *app) doctorCmd() *cobra.Command {
	c := agentcli.DoctorCommand(harvest.Facts(), a.doctorChecks)
	runChecks := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		err := runChecks(cmd, args)
		a.printLogTail(cmd.OutOrStdout())
		return err
	}
	return c
}

// doctorChecks checks the settings first: no other check can run without
// the paths they name. No check calls the model.
func (a *app) doctorChecks(context.Context) []doctor.Check {
	s, err := settings.Load(a.getenv)
	checks := []doctor.Check{{Name: "settings", Run: func(context.Context) error {
		return err
	}}}
	if err != nil {
		return checks
	}
	st := state.New(s.StateDir, a.now)
	return append(
		checks,
		doctor.Check{
			Name: "state dir writable (" + s.StateDir + ")",
			Run: func(context.Context) error {
				return writable(st)
			},
		},
		doctor.Check{Name: "skill file (" + s.Skill + ")", Run: func(context.Context) error {
			f, err := os.Open(s.Skill)
			if err != nil {
				return err
			}
			return f.Close()
		}},
		doctor.Check{Name: "brain on PATH", Run: func(context.Context) error {
			_, err := exec.LookPath(a.brain)
			return err
		}},
		a.backendCheck(s),
		sandboxCheck(s),
	)
}

// backendCheck names the backend and model the complete step would use. It
// selects the backend, which reads the environment and PATH, and calls
// nothing.
func (a *app) backendCheck(s settings.Settings) doctor.Check {
	b, err := a.selectBackend(backendOptions(s))
	if err != nil {
		return doctor.Check{Name: "model backend", Run: func(context.Context) error { return err }}
	}
	name := fmt.Sprintf("model backend %s, model %s", b.Name(), b.Model())
	if b.Name() == llm.Claude {
		name += " (the model step runs outside the sandbox)"
	}
	return doctor.Check{Name: name, Run: func(context.Context) error { return nil }}
}

// sandboxCheck checks the launcher the steps start through, or says the
// sandbox is off.
func sandboxCheck(s settings.Settings) doctor.Check {
	if !s.Sandbox {
		return doctor.Check{Name: "sandbox", Run: func(context.Context) error {
			return doctor.Skip(settings.EnvSandbox + "=off: the steps run without srt")
		}}
	}
	cfg := sandbox.Config{On: true, Launcher: s.Launcher}
	return doctor.Check{
		Name: "sandbox (launcher " + s.Launcher + ")",
		Run:  func(context.Context) error { return sandbox.Check(cfg) },
	}
}

// writable creates the state dir and proves a file can be written there.
func writable(st *state.Dir) error {
	if err := st.Ensure(); err != nil {
		return err
	}
	f, err := os.CreateTemp(st.Path(), ".doctor-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

func (a *app) printLogTail(w io.Writer) {
	dir, err := settings.StateDir(a.getenv)
	if err != nil {
		return
	}
	st := state.New(dir, a.now)
	lines, err := st.LogTail(doctorLogLines)
	switch {
	case err != nil:
		fmt.Fprintf(w, "log %s: %v\n", st.LogPath(), err)
	case len(lines) == 0:
		fmt.Fprintf(w, "no log yet at %s\n", st.LogPath())
	default:
		fmt.Fprintf(w, "last log lines (%s):\n", st.LogPath())
		for _, l := range lines {
			fmt.Fprintf(w, "  %s\n", l)
		}
	}
}
