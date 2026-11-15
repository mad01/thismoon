// Package cli wires the harvest commands: the SessionEnd hook, the detached
// run it starts and the steps that run starts, the belt hint, and the docs,
// doctor and version commands every thismoon CLI carries.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/tools/harvest"
	"github.com/mad01/thismoon/tools/harvest/internal/hint"
	"github.com/mad01/thismoon/tools/harvest/internal/hook"
	"github.com/mad01/thismoon/tools/harvest/internal/run"
	"github.com/mad01/thismoon/tools/harvest/internal/sandbox"
	"github.com/mad01/thismoon/tools/harvest/internal/settings"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// app holds what the commands reach outside the process for, so tests can
// swap each one.
type app struct {
	getenv func(string) string
	// environ is the whole environment, which the sandbox filters for each
	// step.
	environ func() []string
	now     func() time.Time
	// start runs `harvest <args...>` without waiting; nil starts this
	// binary detached.
	start func(args []string) error
	// runStep starts one step of a run; nil starts it as its own process
	// through the sandbox launcher.
	runStep run.StepFunc
	// selectBackend resolves the model backend: llm.Select outside tests.
	selectBackend func(llm.Options) (llm.Backend, error)
	// brain is the brain binary apply, the hint and doctor run.
	brain string
}

// Execute runs the root command.
func Execute() error { return newApp().root().Execute() }

func newApp() *app {
	return &app{
		getenv:        os.Getenv,
		environ:       os.Environ,
		now:           time.Now,
		selectBackend: llm.Select,
		brain:         "brain",
	}
}

func (a *app) root() *cobra.Command {
	c := &cobra.Command{
		Use:           harvest.Name,
		Short:         "Save what a Claude Code session learned to the brain vault inbox",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(harvest.OperatingDoc, harvest.Facts()),
		a.doctorCmd(),
		a.sessionEndCmd(),
		a.runCmd(),
		a.stepCmd(),
		a.hintCmd(),
	)
	return c
}

func (a *app) sessionEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "session-end",
		Short: "SessionEnd hook: read the payload on stdin and start a harvest in the background",
		Long: `Read the Claude Code SessionEnd payload on stdin and start 'harvest run' for
the session, detached, then return at once. Always exits 0: a skip goes to the
log, and a failure is printed to stderr and logged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := hook.Config{Getenv: a.getenv, Now: a.now, Start: a.startFunc()}
			if err := hook.Handle(cfg, cmd.InOrStdin()); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "harvest session-end:", err)
			}
			return nil
		},
	}
}

// startFunc returns the Start the hook uses. The executable is resolved only
// when a run starts, so a guard that returns early costs nothing.
func (a *app) startFunc() func(args []string) error {
	if a.start != nil {
		return a.start
	}
	return func(args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("find the harvest binary: %w", err)
		}
		return hook.Detached(exe)(args)
	}
}

func (a *app) runCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <transcript> <session-id> <cwd>",
		Short: "Harvest the transcript lines a session added since its last harvest",
		Long: `Harvest one session in three steps, each its own process. prepare builds the
digest of the transcript lines after the session's mark and skips it when it is
under HARVEST_MIN_BYTES. complete sends the skill file and the digest to the
model. apply writes the captures with brain and moves the mark. prepare and
apply run under srt; so does complete, except on the claude CLI backend, whose
login srt cannot reach. HARVEST_SANDBOX=off runs every step without srt.

The session-end hook starts this detached; run it by hand to retry one session.
Every outcome is one line in the log.`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := run.Input{Transcript: args[0], Session: args[1], Cwd: args[2]}
			s, err := settings.Load(a.getenv)
			if err != nil {
				return a.logRunFailure(in.Session, err)
			}
			return run.Session(cmd.Context(), run.Config{
				State:   state.New(s.StateDir, a.now),
				Skill:   s.Skill,
				Backend: a.backendName(s),
				Step:    a.stepFunc(s),
			}, in)
		},
	}
}

// backendName names the backend the complete step will use, the same way
// that step selects it. Selecting makes no call.
func (a *app) backendName(s settings.Settings) func() (string, error) {
	return func() (string, error) {
		b, err := a.selectBackend(backendOptions(s))
		if err != nil {
			return "", err
		}
		return b.Name(), nil
	}
}

// backendOptions are the llm options the settings name. The claude CLI is
// allowed: its step runs outside the sandbox.
func backendOptions(s settings.Settings) llm.Options {
	return llm.Options{Backend: s.Backend, Model: s.Model, Effort: s.Effort, AllowCLI: true}
}

// stepFunc returns how a run starts its steps: a.runStep in tests, else as
// processes of this binary through the sandbox launcher.
func (a *app) stepFunc(s settings.Settings) run.StepFunc {
	if a.runStep != nil {
		return a.runStep
	}
	return func(ctx context.Context, step sandbox.Step) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("find the harvest binary: %w", err)
		}
		return sandbox.Run(ctx, sandbox.Config{
			On:       s.Sandbox,
			Launcher: s.Launcher,
			Self:     self,
			Environ:  a.environ(),
		}, step)
	}
}

// logRunFailure logs a run that could not start, when the state dir itself
// still resolves, and returns err. A session id that is not one is quoted,
// so it cannot break the log line.
func (a *app) logRunFailure(session string, err error) error {
	dir, dirErr := settings.StateDir(a.getenv)
	if dirErr != nil {
		return err
	}
	label := session
	if state.CheckSession(session) != nil {
		label = strconv.Quote(session)
	}
	if logErr := state.New(dir, a.now).Log(fmt.Sprintf("fail %s: %v", label, err)); logErr != nil {
		return errors.Join(err, logErr)
	}
	return err
}

func (a *app) hintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hint",
		Short: "belt custom hint: one line when harvest captures wait in the brain inbox",
		Long: `Print one line when raw notes tagged harvest wait in the brain inbox, at most
once a day, and nothing otherwise. belt relays the line at session start.
Always exits 0; a failure is printed to stderr, which belt does not read.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			line, err := a.hintLine(cmd)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "harvest hint:", err)
				return nil
			}
			if line != "" {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
}

func (a *app) hintLine(cmd *cobra.Command) (string, error) {
	dir, err := settings.StateDir(a.getenv)
	if err != nil {
		return "", err
	}
	return hint.Line(cmd.Context(), hint.Config{
		State: state.New(dir, a.now),
		Brain: a.brain,
		Now:   a.now,
	})
}
