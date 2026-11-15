package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/harvest/internal/run"
	"github.com/mad01/thismoon/tools/harvest/internal/settings"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// envSrtRunLabel is the marker srt-run sets on every process it confines.
// apply reads it to report whether it ran in the sandbox.
const envSrtRunLabel = "SRT_RUN"

// stepCmd is the hidden group of the three step processes `harvest run`
// starts. A step reads its settings from the environment, as the run does,
// and prints an error on stderr for the run to log.
func (a *app) stepCmd() *cobra.Command {
	c := &cobra.Command{
		Use:    "step",
		Short:  "The steps of one harvest; harvest run starts them",
		Hidden: true,
		Long: `The three steps of one harvest, each its own process. harvest run starts
them, under srt unless HARVEST_SANDBOX=off, and logs a step that fails; run
'harvest run' by hand, not these.

  prepare   build the digest and write the model's user text to the job dir
  complete  send the skill file and the user text to the model
  apply     write the captures with brain, move the mark, log the done line`,
	}
	c.AddCommand(a.prepareCmd(), a.completeCmd(), a.applyCmd())
	return c
}

func (a *app) prepareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   run.StepPrepare + " <transcript> <session-id> <cwd>",
		Short: "Build the digest of the lines after the mark, or log a skip",
		Args:  cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := settings.Load(a.getenv)
			if err != nil {
				return err
			}
			return run.Prepare(run.PrepareConfig{
				State:    state.New(s.StateDir, a.now),
				MinBytes: s.MinBytes,
				MaxBytes: s.MaxBytes,
				Now:      a.now,
			}, run.Input{Transcript: args[0], Session: args[1], Cwd: args[2]})
		},
	}
}

func (a *app) completeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   run.StepComplete + " <session-id>",
		Short: "Send the job's system prompt and user text to the model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := settings.Load(a.getenv)
			if err != nil {
				return err
			}
			return run.Complete(cmd.Context(), run.CompleteConfig{
				State:   state.New(s.StateDir, a.now),
				Select:  a.selectBackend,
				Options: backendOptions(s),
			}, args[0])
		},
	}
}

func (a *app) applyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   run.StepApply + " <session-id>",
		Short: "Write the reply's captures with brain and move the mark",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := settings.Load(a.getenv)
			if err != nil {
				return err
			}
			return run.Apply(cmd.Context(), run.ApplyConfig{
				State:     state.New(s.StateDir, a.now),
				Brain:     a.brain,
				Sandboxed: a.getenv(envSrtRunLabel) != "",
			}, args[0])
		},
	}
}
