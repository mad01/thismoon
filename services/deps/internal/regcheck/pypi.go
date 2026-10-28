package regcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// simpleJSON asks PyPI's Simple API for its JSON form (PEP 691), which carries
// the project status (PEP 792).
const simpleJSON = "application/vnd.pypi.simple.v1+json"

// pypiProject is the part of the Simple API project page deps reads.
type pypiProject struct {
	ProjectStatus struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"project-status"`
}

// pypiRelease is the part of GET /pypi/<name>/<version>/json deps reads.
type pypiRelease struct {
	Info struct {
		Yanked       bool   `json:"yanked"`
		YankedReason string `json:"yanked_reason"`
	} `json:"info"`
}

// pypi looks one PyPI project up, then each of its versions. A project PyPI
// does not know gets no finding: it is private, or PyPI deleted it outright,
// and OSV's MAL- records cover the malware PyPI deletes. A quarantined project
// makes every version malicious without a release lookup, since PyPI hides a
// quarantined project's files.
func (c *Client) pypi(ctx context.Context, name string, versions []string) map[string]Result {
	u := c.endpoints.PyPI + "/simple/" + url.PathEscape(name) + "/"
	status, body, err := c.get(ctx, u, simpleJSON)
	switch {
	case err != nil:
		return allUnknown(versions, err)
	case status == http.StatusNotFound:
		return nil
	case status != http.StatusOK:
		return allUnknown(versions, statusErr(u, status))
	}
	var project pypiProject
	if err := json.Unmarshal(body, &project); err != nil {
		return allUnknown(versions, fmt.Errorf("decode %s: %w", u, err))
	}
	statusFinding, ok := pypiStatus(project)
	if ok && statusFinding.ID == RulePyPIQuarantined {
		return allFound(versions, statusFinding)
	}
	out := make(map[string]Result, len(versions))
	for _, v := range versions {
		r := c.pypiRelease(ctx, name, v)
		if ok && r.Err == nil {
			r.Findings = append(r.Findings, statusFinding)
		}
		out[v] = r
	}
	return out
}

// pypiStatus maps a PEP 792 project status to its finding. An active project,
// or a status PyPI adds later, has none.
func pypiStatus(p pypiProject) (store.Advisory, bool) {
	rule, summary := "", ""
	switch p.ProjectStatus.Status {
	case "quarantined":
		rule, summary = RulePyPIQuarantined, "PyPI quarantined the project"
	case "archived":
		rule, summary = RulePyPIArchived, "The project is archived on PyPI and expects no more releases"
	case "deprecated":
		rule, summary = RulePyPIDeprecated, "The project is deprecated on PyPI"
	default:
		return store.Advisory{}, false
	}
	if reason := p.ProjectStatus.Reason; reason != "" {
		summary += ": " + reason
	}
	return finding(rule, summary), true
}

// pep440 matches a version PEP 440 accepts, in any spelling it normalizes
// (the regular expression in PEP 440's appendix B).
var pep440 = regexp.MustCompile(`(?i)^v?(?:[0-9]+!)?[0-9]+(?:\.[0-9]+)*` +
	`(?:[-_.]?(?:a|b|c|rc|alpha|beta|pre|preview)[-_.]?[0-9]*)?` +
	`(?:-[0-9]+|[-_.]?(?:post|rev|r)[-_.]?[0-9]*)?` +
	`(?:[-_.]?dev[-_.]?[0-9]*)?` +
	`(?:\+[a-z0-9]+(?:[-_.][a-z0-9]+)*)?$`)

// pypiRelease looks one release up. PyPI matches the version the way pip does,
// so a 404 means the release is gone from a project PyPI still lists, and
// counts as withdrawn. Two kinds of version are never asked about, since PyPI
// cannot serve them: one that is not a PEP 440 version (a misread pin, say),
// and one with a local label (1.0+cu118), which came from another index.
func (c *Client) pypiRelease(ctx context.Context, name, version string) Result {
	if !pep440.MatchString(version) || strings.Contains(version, "+") {
		return Result{}
	}
	u := fmt.Sprintf("%s/pypi/%s/%s/json",
		c.endpoints.PyPI, url.PathEscape(name), url.PathEscape(version))
	status, body, err := c.get(ctx, u, "")
	switch {
	case err != nil:
		return Result{Err: err}
	case status == http.StatusNotFound:
		return found(finding(RulePyPIWithdrawn,
			fmt.Sprintf("PyPI lists %s but no longer serves version %s", name, version)))
	case status != http.StatusOK:
		return Result{Err: statusErr(u, status)}
	}
	var rel pypiRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return Result{Err: fmt.Errorf("decode %s: %w", u, err)}
	}
	if !rel.Info.Yanked {
		return Result{}
	}
	summary := "Yanked on PyPI"
	if rel.Info.YankedReason != "" {
		summary += ": " + rel.Info.YankedReason
	}
	return found(finding(RulePyPIYanked, summary))
}

// withoutPyPIWithdrawn returns fs without a PyPI withdrawn finding, which
// rests on PyPI not serving the version: from a manifest that names another
// index, the version may live there. fs is never written through, since
// queries for the same version share it.
func withoutPyPIWithdrawn(fs []store.Advisory) []store.Advisory {
	isWithdrawn := func(a store.Advisory) bool { return a.ID == RulePyPIWithdrawn }
	if !slices.ContainsFunc(fs, isWithdrawn) {
		return fs
	}
	return slices.DeleteFunc(slices.Clone(fs), isWithdrawn)
}
