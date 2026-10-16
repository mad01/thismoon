// Package dryrun holds the stand-ins a --dry-run uses instead of model
// calls. The agent prints the exact argv it would run and leaves every
// case branch at the pinned commit, as a session that edited nothing
// would. The judge prints its argv once and returns a fixture verdict.
// Both return a result message shaped like the real one, so the parser,
// the checks and the results file all run for real.
package dryrun

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
)

const agentReport = `{"type":"result","subtype":"success","is_error":false,` +
	`"result":"dry run: no model call","session_id":"","total_cost_usd":0,` +
	`"duration_ms":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0},` +
	`"modelUsage":{},"permission_denials":[],"structured_output":{"cases":[]}}`

const judgeReport = `{"type":"result","subtype":"success","is_error":false,` +
	`"total_cost_usd":0,"structured_output":{"score":0,` +
	`"reasons":["dry run: fixture verdict, no model call"]}}`

// Agent stands in for an arm session.
type Agent struct {
	Cases []cases.Case
	Out   io.Writer
}

// Run prints the invocation and creates each case branch at HEAD, the
// pinned commit, without editing anything.
func (a Agent) Run(ctx context.Context, inv claude.Invocation) (claude.Output, error) {
	fmt.Fprintf(
		a.Out,
		"  dry run, would run in %s:\n    claude %s\n    (prompt on stdin, %d bytes)\n",
		inv.Dir,
		Quote(inv.Args),
		len(inv.Stdin),
	)
	for _, c := range a.Cases {
		cmd := exec.CommandContext(ctx, "git", "branch", c.Branch, "HEAD")
		cmd.Dir = inv.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return claude.Output{}, fmt.Errorf("dryrun: git branch %s: %w: %s", c.Branch, err, out)
		}
	}
	return claude.Output{Stdout: []byte(agentReport)}, nil
}

// Judge stands in for the judge call.
type Judge struct {
	Out     io.Writer
	printed *bool
}

// NewJudge returns a judge stand-in that prints the judge argv once.
func NewJudge(out io.Writer) Judge { return Judge{Out: out, printed: new(bool)} }

// Run prints the judge argv on its first call and returns the fixture.
func (j Judge) Run(_ context.Context, inv claude.Invocation) (claude.Output, error) {
	if !*j.printed {
		fmt.Fprintf(j.Out, "  dry run, the judge would run:\n    claude %s\n    "+
			"(prompt on stdin, %d bytes for the first case)\n", Quote(inv.Args), len(inv.Stdin))
		*j.printed = true
	}
	return claude.Output{Stdout: []byte(judgeReport)}, nil
}

// Quote renders argv the way a shell would read it back.
func Quote(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n\"'`$\\*(){}[]<>|&;!?#~") {
			quoted[i] = a
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
