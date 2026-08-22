package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/deps/internal/api"
	"github.com/mad01/thismoon/services/deps/internal/client"
)

// handlers carries the dependencies shared by all deps tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views findings in a browser
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "deps_scan",
		Description: "Discover every external dependency across the registered tool repos (Go modules, npm) and persist the inventory, WITHOUT checking advisories. " +
			"Returns a per-ecosystem count. Use deps_check to additionally check versions against OSV.",
	}, h.handleScan)

	mcp.AddTool(s, &mcp.Tool{
		Name: "deps_check",
		Description: "Discover every dependency and check each version against the OSV.dev supply-chain advisory database. " +
			"Returns the flagged packages with advisory id, severity, and the version that fixes each. This reaches the network and may take a few seconds.",
	}, h.handleCheck)

	mcp.AddTool(s, &mcp.Tool{
		Name: "deps_list_flagged",
		Description: "Return the flagged packages from the most recent check WITHOUT re-scanning (fast, no network). " +
			"Use after deps_check to re-read the findings. Each advisory carries a `key` and a `resolved` flag. " +
			"An empty flagged list with total > 0 means the persisted inventory carries no unresolved advisories (run deps_check to re-verify against OSV); with total == 0 the response carries zero_result_hint, because an empty store means nothing was checked, not that anything is clean.",
	}, h.handleListFlagged)

	mcp.AddTool(s, &mcp.Tool{
		Name: "deps_scan_repo",
		Description: "Rescan a SINGLE repo (by absolute path or basename, e.g. 'dotfiles') against OSV and merge the result, " +
			"without re-sweeping every repo. Use after bumping dependencies in one repo to re-check just it. Returns the full flagged set.",
	}, h.handleScanRepo)

	mcp.AddTool(s, &mcp.Tool{
		Name: "deps_resolve",
		Description: "Acknowledge (resolve) one or more flagged advisories by their `key` (from deps_check / deps_list_flagged). " +
			"A resolved advisory stops alerting and drops out of the active findings until the package version changes or a NEW advisory appears on it. " +
			"Use when the user has accepted a risk or will fix later.",
	}, h.handleResolve)
}

// scanOut is the deps_scan response.
type scanOut struct {
	Total       int            `json:"total"        jsonschema_description:"total dependencies discovered"`
	ByEcosystem map[string]int `json:"by_ecosystem" jsonschema_description:"count per ecosystem (Go, npm, PyPI)"`
	URL         string         `json:"url"          jsonschema_description:"web page where the user can view findings"`
}

func (h *handlers) handleScan(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, scanOut, error) {
	res, err := h.client.Scan()
	if err != nil {
		return nil, scanOut{}, err
	}
	return nil, scanOut{Total: res.Total, ByEcosystem: res.ByEcosystem, URL: h.webURL}, nil
}

// flaggedOut is the deps_check / deps_list_flagged / deps_scan_repo response.
type flaggedOut struct {
	Total        int              `json:"total"         jsonschema_description:"total dependencies checked"`
	FlaggedCount int              `json:"flagged_count" jsonschema_description:"number of dependencies with at least one UNRESOLVED advisory"`
	Flagged      []api.Dependency `json:"flagged"       jsonschema_description:"the flagged dependencies; each advisory has a key and a resolved flag"`
	URL          string           `json:"url"           jsonschema_description:"web page where the user can view findings"`
	ZeroHint     *flaggedZeroHint `json:"zero_result_hint,omitempty" jsonschema:"set by deps_list_flagged only, when the store holds zero dependencies: an empty flagged list then means nothing was checked, not that the dependencies are clean"`
}

// flaggedZeroHint explains a deps_list_flagged response whose store holds no
// dependencies at all — the one case where an empty flagged list must not be
// read as clean. Additive: it appears only when total is zero.
type flaggedZeroHint struct {
	Notes []string `json:"notes" jsonschema:"why the store is empty and what to run"`
}

func (h *handlers) flagged(res client.CheckResult) flaggedOut {
	return flaggedOut{
		Total:        res.Total,
		FlaggedCount: res.FlaggedCount,
		Flagged:      res.Flagged,
		URL:          h.webURL,
	}
}

func (h *handlers) handleCheck(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, flaggedOut, error) {
	res, err := h.client.Check()
	if err != nil {
		return nil, flaggedOut{}, err
	}
	return nil, h.flagged(res), nil
}

func (h *handlers) handleListFlagged(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, flaggedOut, error) {
	res, err := h.client.Flagged()
	if err != nil {
		return nil, flaggedOut{}, err
	}
	out := h.flagged(res)
	if out.Total == 0 {
		out.ZeroHint = flaggedZero(res)
	}
	return nil, out, nil
}

// flaggedZero phrases the empty-store hint. ScannedAt tells "nothing ever
// ran" apart from "a scan completed but discovered zero dependencies" (an
// empty or fully excluded catalog).
func flaggedZero(res client.CheckResult) *flaggedZeroHint {
	if res.ScannedAt.IsZero() {
		return &flaggedZeroHint{Notes: []string{
			"no scan or check has completed on this machine, so an empty flagged list means nothing was verified; run deps_check",
		}}
	}
	return &flaggedZeroHint{Notes: []string{fmt.Sprintf(
		"the last scan at %s discovered zero dependencies; check the catalog registry and the deps config excludes before reading this as clean",
		res.ScannedAt.UTC().Format(time.RFC3339),
	)}}
}

type scanRepoInput struct {
	Repo string `json:"repo" jsonschema_description:"repo to rescan: absolute path or basename (e.g. 'dotfiles')"`
}

func (h *handlers) handleScanRepo(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in scanRepoInput,
) (*mcp.CallToolResult, flaggedOut, error) {
	res, err := h.client.CheckRepo(in.Repo)
	if err != nil {
		return nil, flaggedOut{}, err
	}
	return nil, h.flagged(res), nil
}

type resolveInput struct {
	Keys []string `json:"keys" jsonschema_description:"advisory keys to acknowledge (from deps_check / deps_list_flagged)"`
}

type resolveOut struct {
	Resolved int    `json:"resolved" jsonschema_description:"number of advisories newly acknowledged"`
	URL      string `json:"url"`
}

func (h *handlers) handleResolve(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in resolveInput,
) (*mcp.CallToolResult, resolveOut, error) {
	res, err := h.client.Resolve(in.Keys)
	if err != nil {
		return nil, resolveOut{}, err
	}
	return nil, resolveOut{Resolved: res.Resolved, URL: h.webURL}, nil
}
