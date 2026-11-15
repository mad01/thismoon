// Package run harvests one ended session in three steps, each its own
// process:
//
//   - prepare reads the transcript lines the session's mark has not
//     covered, builds their digest and skips a digest under the size floor.
//     Otherwise it writes the model's user text into the job dir.
//   - complete sends the skill file and that user text to the model and
//     writes the reply and what the call cost.
//   - apply checks the reply, writes each capture with `brain capture`,
//     moves the mark and logs the outcome.
//
// Session is the orchestrator `harvest run` calls. It reads no session text
// and no model output itself: it copies the skill file into the job dir,
// starts the steps through a StepFunc, and removes the job dir at the end.
// A step that fails leaves the mark where it was, so the next end of the
// same session tries those lines again.
//
// Every outcome is one line in the state dir's log: skip, fail or done.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mad01/thismoon/tools/harvest/internal/sandbox"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// The step names: the subcommands of `harvest step`.
const (
	StepPrepare  = "prepare"
	StepComplete = "complete"
	StepApply    = "apply"
)

// Input names one session to harvest: the arguments of `harvest run`.
type Input struct {
	Transcript string // path of the session's JSONL transcript
	Session    string // the session id, which names the mark and the job dir
	Cwd        string // the session's working directory
}

// StepFunc starts one step process and waits for it. sandbox.Run is the
// real one; tests run the step in-process.
type StepFunc func(ctx context.Context, step sandbox.Step) error

// Config is what Session needs besides the input.
type Config struct {
	State *state.Dir
	// Skill is the skill file the model gets as its system prompt.
	Skill string
	// Backend names the backend the complete step will use, without
	// calling it. It decides whether that step runs in the sandbox.
	Backend func() (string, error)
	// Step starts a step.
	Step StepFunc
}

// Session harvests the lines of in.Transcript after the session's mark. A
// skip returns nil; a failure is logged and returned.
func Session(ctx context.Context, cfg Config, in Input) (err error) {
	st := cfg.State
	if err := st.Ensure(); err != nil {
		return err
	}
	job, err := st.Job(in.Session)
	if err != nil {
		return fail(st, in.Session, err)
	}
	if err := job.Reset(); err != nil {
		return fail(st, in.Session, err)
	}
	defer func() {
		if rmErr := job.Remove(); rmErr != nil {
			err = errors.Join(err, fail(st, in.Session, rmErr))
		}
	}()

	if err := copyFile(cfg.Skill, job.System()); err != nil {
		return fail(st, in.Session, fmt.Errorf("skill file: %w", err))
	}
	if err := cfg.Step(ctx, sandbox.Step{
		Name: StepPrepare, Args: []string{in.Transcript, in.Session, in.Cwd},
	}); err != nil {
		return failStep(st, in.Session, StepPrepare, err)
	}
	// prepare writes no user text for a session it skipped, and has logged
	// why.
	ok, err := job.HasUser()
	if err != nil {
		return fail(st, in.Session, err)
	}
	if !ok {
		return nil
	}

	backend, err := cfg.Backend()
	if err != nil {
		return failStep(st, in.Session, StepComplete, err)
	}
	if err := cfg.Step(ctx, sandbox.Step{
		Name: StepComplete, Args: []string{in.Session}, Backend: backend,
	}); err != nil {
		return failStep(st, in.Session, StepComplete, err)
	}
	if err := cfg.Step(ctx, sandbox.Step{Name: StepApply, Args: []string{in.Session}}); err != nil {
		return failStep(st, in.Session, StepApply, err)
	}
	return nil
}

// copyFile copies src to dst, which it creates readable by its owner only.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// failStep logs a step's failure, with the step's name, as the session's
// outcome and returns it.
func failStep(st *state.Dir, session, step string, err error) error {
	return fail(st, session, fmt.Errorf("%s: %w", step, err))
}

// fail logs err as the session's outcome and returns it.
func fail(st *state.Dir, session string, err error) error {
	if logErr := st.Log(fmt.Sprintf("fail %s: %v", session, err)); logErr != nil {
		return errors.Join(err, logErr)
	}
	return err
}
