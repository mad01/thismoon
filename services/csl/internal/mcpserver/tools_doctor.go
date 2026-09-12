package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl/internal/selfcheck"
)

// doctorInput is the typed input for the csl_doctor tool: the checks take no
// parameters beyond the response format, and the repair path stays on the CLI
// where a human confirms it.
type doctorInput struct {
	formatParam
}

// doctorCheck is one check's outcome in the csl_doctor result.
type doctorCheck struct {
	Name   string `json:"name"             jsonschema:"the check that ran"`
	Status string `json:"status"           jsonschema:"ok, skipped, or fail; skipped is a pass with something to report, e.g. no config file yet"`
	Detail string `json:"detail,omitempty" jsonschema:"what failed and what to run next; empty when the check passed"`
}

// doctorOutput is the typed output of the csl_doctor tool.
type doctorOutput struct {
	OK     bool          `json:"ok"             jsonschema:"true when every check passed"`
	Note   string        `json:"note,omitempty" jsonschema:"set when csl is running with no config file, naming the path to create; the checks still pass, csl just has nothing configured to index"`
	Checks []doctorCheck `json:"checks"         jsonschema:"one entry per check, in the order csl doctor runs them"`
}

func registerDoctorTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_doctor",
		Description: "Diagnose csl itself: config, index state, shard integrity, the search server, and the web UI. " +
			"Read-only: it repairs nothing. Call it when a csl tool errors or returns nothing you expect, " +
			"before concluding that a repo or a query is at fault. " +
			"The web-ui checks can fail while search works fine; they only mean `csl web` is down or out of date. " +
			"For git health of the repos csl indexes (dirty trees, unpushed work), use csl_repo_health instead.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, handleDoctor, renderDoctorText)
}

func handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctorOutput, error) {
	// repair=false: a tool call is not the place to rewrite state files.
	report := doctor.Collect(ctx, selfcheck.Checks(false))
	out := doctorOutput{OK: report.OK, Checks: make([]doctorCheck, 0, len(report.Checks))}
	for _, c := range report.Checks {
		out.Checks = append(
			out.Checks,
			doctorCheck{Name: c.Name, Status: c.Status, Detail: c.Detail},
		)
	}
	return nil, out, nil
}
