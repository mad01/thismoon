package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/prs/internal/client"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

// handlers carries the dependencies shared by all prs tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views the PR list in a browser
	checks func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "prs_list",
		Description: "List the open pull requests across every locally checked-out repo, from the local cache, " +
			"the way to see what needs attention without calling GitHub yourself. " +
			"Only open, non-draft PRs are tracked; closed, merged, and draft PRs never appear. " +
			"Filter by `repo` (exact org/name), `author` (exact login), and/or `review` " +
			"(EXACTLY APPROVED or CHANGES_REQUESTED; a PR with no standing review matches neither). " +
			"`sort` is newest (default) or oldest by the PR's created time. " +
			"The response also carries the distinct repos and authors in the cache (the valid filter values) " +
			"and the cache status including per-repo fetch errors. " +
			"The cache refreshes on the serve poll interval; call prs_refresh first when you need the state right now.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleList)

	mcp.AddTool(s, &mcp.Tool{
		Name: "prs_refresh",
		Description: "Force one synchronous poll cycle: re-discover the locally checked-out repos, fetch every repo's " +
			"open PRs from its GitHub host, and update the cache. Returns how many repos were polled, how many " +
			"open PRs were found, and how many repos failed to fetch. " +
			"Takes seconds to a minute depending on repo count. Use it before prs_list when staleness matters, " +
			"not on every call.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(true),
		},
	}, h.handleRefresh)

	mcp.AddTool(s, &mcp.Tool{
		Name: "prs_status",
		Description: "Report the service status: when the cache was last polled, how many repos and open PRs it holds, " +
			"which hosts they came from, every repo whose last fetch failed (with the error), and the config in " +
			"effect (scan dirs, excludes, host allowlist, poll interval). " +
			"The first stop when prs_list looks wrong: a missing repo is usually an exclude, a host allowlist, " +
			"or a fetch error visible here.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleStatus)

	mcp.AddTool(s, &mcp.Tool{
		Name: "prs_doctor",
		Description: "Diagnose prs itself: is serve reachable, is the cache readable, is the running build the " +
			"installed one, does the config parse and name directories to scan, is the gh CLI available. " +
			"Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing. " +
			"Call this when another prs tool errors or returns an empty list: it answers whether the fault is " +
			"the service, the config, or genuinely no open PRs.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleDoctor)
}

// ── list ──

type listInput struct {
	Repo   string `json:"repo,omitempty"   jsonschema:"optional exact org/name filter, e.g. mad01/thismoon"`
	Author string `json:"author,omitempty" jsonschema:"optional exact GitHub login filter"`
	Review string `json:"review,omitempty" jsonschema:"optional review filter: EXACTLY APPROVED or CHANGES_REQUESTED"`
	Sort   string `json:"sort,omitempty"   jsonschema:"sort order by created time: newest (default) | oldest"`
}

type listOutput struct {
	PRs     []store.PR   `json:"prs"`
	Repos   []string     `json:"repos"   jsonschema:"distinct repos with open PRs (the valid repo filter values)"`
	Authors []string     `json:"authors" jsonschema:"distinct PR authors (the valid author filter values)"`
	Status  store.Status `json:"status"`
	URL     string       `json:"url"     jsonschema:"web page where the user can view the PR list"`
}

func (h *handlers) handleList(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	res, err := h.client.List(client.ListFilter{
		Repo:   in.Repo,
		Author: in.Author,
		Review: in.Review,
		Sort:   in.Sort,
	})
	if err != nil {
		return nil, listOutput{}, err
	}
	return nil, listOutput{
		PRs:     res.PRs,
		Repos:   res.Facets.Repos,
		Authors: res.Facets.Authors,
		Status:  res.Status,
		URL:     h.webURL,
	}, nil
}

// ── refresh ──

type refreshInput struct{}

type refreshOutput struct {
	client.RefreshSummary
	URL string `json:"url"`
}

func (h *handlers) handleRefresh(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ refreshInput,
) (*mcp.CallToolResult, refreshOutput, error) {
	sum, err := h.client.Refresh()
	if err != nil {
		return nil, refreshOutput{}, err
	}
	return nil, refreshOutput{RefreshSummary: sum, URL: h.webURL}, nil
}

// ── status ──

type statusInput struct{}

type statusOutput struct {
	client.ServiceStatus
	URL string `json:"url"`
}

func (h *handlers) handleStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ statusInput,
) (*mcp.CallToolResult, statusOutput, error) {
	st, err := h.client.Status()
	if err != nil {
		return nil, statusOutput{}, err
	}
	return nil, statusOutput{ServiceStatus: st, URL: h.webURL}, nil
}

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `prs doctor` and returns the report.
// A failing check is a result, not a tool error: the caller asked what is
// wrong, and an error would hide the answer behind a transport failure.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}
