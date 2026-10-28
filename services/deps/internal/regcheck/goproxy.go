package regcheck

import (
	"bytes"
	"cmp"
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

// goModule looks one Go module up on the proxy. The @v/list request doubles as
// the block check: the proxy refuses every request for a module it considers
// malicious. A module the proxy does not have gets no finding. Otherwise the
// go.mod of the module's highest version says which versions the authors
// retracted, the go.mod the go command reads retractions from.
func (c *Client) goModule(ctx context.Context, path string, versions []string) map[string]Result {
	escPath, err := module.EscapePath(path)
	if err != nil {
		return nil // not a path the proxy can serve
	}
	base := c.endpoints.GoProxy + "/" + escPath
	status, body, err := c.get(ctx, base+"/@v/list", "")
	switch {
	case err != nil:
		return allUnknown(versions, err)
	case status == http.StatusForbidden && bytes.HasPrefix(body, []byte(goSecurityError)):
		return allFound(versions, finding(RuleGoBlocked,
			"The Go module proxy refuses to serve this module because it considers it malicious"))
	case status == http.StatusNotFound || status == http.StatusGone:
		return nil
	case status != http.StatusOK:
		return allUnknown(versions, statusErr(base+"/@v/list", status))
	}
	latest := highestListed(string(body))
	if latest == "" {
		if latest, err = c.goLatest(ctx, base); err != nil {
			return allUnknown(versions, err)
		}
		if latest == "" {
			return nil
		}
	}
	retracts, err := c.goRetractions(ctx, base, latest)
	if err != nil {
		return allUnknown(versions, err)
	}
	out := make(map[string]Result, len(versions))
	for _, v := range versions {
		if summary, ok := retracted(retracts, v, latest); ok {
			out[v] = found(finding(RuleGoRetracted, summary))
		}
	}
	return out
}

// highestListed picks, from a module's @v/list, the version whose go.mod the
// go command reads retractions from: the highest release, else the highest
// prerelease. Like go, it ignores retractions in picking, so a version that
// retracts itself still counts, and it skips a +incompatible version while the
// module has another release. An empty or unreadable list gives "".
func highestListed(list string) string {
	var compatible, incompatible, prerelease string
	for _, v := range strings.Fields(list) {
		switch {
		case !semver.IsValid(v):
		case semver.Prerelease(v) != "":
			prerelease = higher(prerelease, v)
		case strings.HasSuffix(v, "+incompatible"):
			incompatible = higher(incompatible, v)
		default:
			compatible = higher(compatible, v)
		}
	}
	return cmp.Or(compatible, incompatible, prerelease)
}

// higher returns the higher of two versions, either of which may be "".
func higher(have, v string) string {
	if have == "" || semver.Compare(v, have) > 0 {
		return v
	}
	return have
}

// goLatest asks @latest for a module whose @v/list is empty, which the proxy
// answers with a pseudo-version, as the go command does. A module with no
// version at all gives "".
func (c *Client) goLatest(ctx context.Context, base string) (string, error) {
	u := base + "/@latest"
	status, body, err := c.get(ctx, u, "")
	switch {
	case err != nil:
		return "", err
	case status == http.StatusNotFound || status == http.StatusGone:
		return "", nil
	case status != http.StatusOK:
		return "", statusErr(u, status)
	}
	var latest struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(body, &latest); err != nil {
		return "", fmt.Errorf("decode %s: %w", u, err)
	}
	return latest.Version, nil
}

// goRetractions reads the retract directives from the go.mod at version.
func (c *Client) goRetractions(
	ctx context.Context,
	base, version string,
) ([]*modfile.Retract, error) {
	escVer, err := module.EscapeVersion(version)
	if err != nil {
		return nil, fmt.Errorf("highest version %q of %s: %w", version, base, err)
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
