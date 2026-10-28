package regcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// goSecurityError opens the body the Go module proxy sends with its 403 for a
// module it considers malicious. A 403 without it (a sandbox or network proxy
// refusing the host, say) is an unanswered lookup, not a blocked module.
const goSecurityError = "SECURITY ERROR"

// goModule looks one Go module up on the proxy. The @latest request doubles as
// the block check: the proxy refuses every request for a module it considers
// malicious. A module the proxy does not have gets no finding. Otherwise the
// go.mod of the latest version says which versions the authors retracted, as
// it does for the go command.
func (c *Client) goModule(ctx context.Context, path string, versions []string) map[string]Result {
	escPath, err := module.EscapePath(path)
	if err != nil {
		return nil // not a path the proxy can serve
	}
	base := c.endpoints.GoProxy + "/" + escPath
	status, body, err := c.get(ctx, base+"/@latest", "")
	switch {
	case err != nil:
		return allUnknown(versions, err)
	case status == http.StatusForbidden && bytes.HasPrefix(body, []byte(goSecurityError)):
		return allFound(versions, finding(RuleGoBlocked,
			"The Go module proxy refuses to serve this module because it considers it malicious"))
	case status == http.StatusNotFound || status == http.StatusGone:
		return nil
	case status != http.StatusOK:
		return allUnknown(versions, statusErr(base+"/@latest", status))
	}
	var latest struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(body, &latest); err != nil {
		return allUnknown(versions, fmt.Errorf("decode %s/@latest: %w", base, err))
	}
	retracts, err := c.goRetractions(ctx, base, latest.Version)
	if err != nil {
		return allUnknown(versions, err)
	}
	out := make(map[string]Result, len(versions))
	for _, v := range versions {
		if summary, ok := retracted(retracts, v, latest.Version); ok {
			out[v] = found(finding(RuleGoRetracted, summary))
		}
	}
	return out
}

// goRetractions reads the retract directives from the go.mod at version.
func (c *Client) goRetractions(
	ctx context.Context,
	base, version string,
) ([]*modfile.Retract, error) {
	escVer, err := module.EscapeVersion(version)
	if err != nil {
		return nil, fmt.Errorf("latest version %q of %s: %w", version, base, err)
	}
	u := base + "/@v/" + escVer + ".mod"
	status, body, err := c.get(ctx, u, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, statusErr(u, status)
	}
	f, err := modfile.ParseLax(u, body, nil)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", u, err)
	}
	return f.Retract, nil
}

// retracted reports whether a retract directive covers version, which deps
// stores without the leading v, and phrases the finding's summary.
func retracted(retracts []*modfile.Retract, version, latest string) (string, bool) {
	v := "v" + strings.TrimPrefix(version, "v")
	if !semver.IsValid(v) {
		return "", false
	}
	for _, r := range retracts {
		if semver.Compare(r.Low, v) > 0 || semver.Compare(v, r.High) > 0 {
			continue
		}
		summary := "Retracted by the module's authors in its go.mod at " + latest
		if r.Rationale != "" {
			summary += ": " + r.Rationale
		}
		return summary, true
	}
	return "", false
}
