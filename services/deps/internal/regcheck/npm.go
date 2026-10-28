package regcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// npmPackument is the part of the full npm package document deps reads. Time
// holds a publish time per version, and npm keeps that entry after the version
// is unpublished or replaced by a security holding package, while Versions
// loses it. Time values are raw since its "unpublished" entry is an object.
type npmPackument struct {
	Versions map[string]struct{}        `json:"versions"`
	Time     map[string]json.RawMessage `json:"time"`
	DistTags map[string]string          `json:"dist-tags"`
}

// npmName matches a name the npm registry can hold: an optional @scope/ and
// one URL-safe segment, at most 214 characters (npm's
// validate-npm-package-name). Capitals pass, since packages published before
// npm required lowercase keep theirs.
var npmName = regexp.MustCompile(
	`^(?:@[A-Za-z0-9*~-][A-Za-z0-9*._~-]*/)?[A-Za-z0-9~-][A-Za-z0-9._~-]*$`,
)

// npmMaxName is the longest package name npm accepts.
const npmMaxName = 214

// npm looks each version of one npm package up. A version the registry serves
// is checked for a deprecation notice. A version it does not serve is withdrawn
// when the full package document still lists its publish time. A package or
// version npm has no record of at all (a private package, say) gets no finding,
// and neither does a name npm could not hold, which is never asked about.
func (c *Client) npm(ctx context.Context, name string, versions []string) map[string]Result {
	if len(name) > npmMaxName || !npmName.MatchString(name) {
		return nil
	}
	out := make(map[string]Result, len(versions))
	var (
		doc    *npmPackument // fetched once, for the first version npm does not serve
		docErr error
	)
	for _, v := range versions {
		u := c.endpoints.NPM + "/" + url.PathEscape(name) + "/" + url.PathEscape(v)
		status, body, err := c.get(ctx, u, "")
		switch {
		case err != nil:
			out[v] = Result{Err: err}
		case status == http.StatusOK:
			out[v] = npmDeprecated(u, body)
		case status == http.StatusNotFound:
			if doc == nil && docErr == nil {
				doc, docErr = c.npmPackument(ctx, name)
			}
			if docErr != nil {
				out[v] = Result{Err: docErr}
				continue
			}
			out[v] = npmWithdrawn(doc, name, v)
		default:
			out[v] = Result{Err: statusErr(u, status)}
		}
	}
	return out
}

// npmPackument fetches the full package document. A package npm does not know
// comes back as an empty document, which lists no version.
func (c *Client) npmPackument(ctx context.Context, name string) (*npmPackument, error) {
	u := c.endpoints.NPM + "/" + url.PathEscape(name)
	status, body, err := c.get(ctx, u, "")
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusNotFound:
		return &npmPackument{}, nil
	case status != http.StatusOK:
		return nil, statusErr(u, status)
	}
	var doc npmPackument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", u, err)
	}
	return &doc, nil
}

// npmDeprecated reads the deprecation notice off one version document. npm
// clears a notice by setting it to "", so only a non-empty string counts.
func npmDeprecated(u string, body []byte) Result {
	var doc struct {
		Deprecated json.RawMessage `json:"deprecated"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Result{Err: fmt.Errorf("decode %s: %w", u, err)}
	}
	var notice string
	if json.Unmarshal(doc.Deprecated, &notice) != nil || notice == "" {
		return Result{}
	}
	return found(finding(RuleNPMDeprecated, "Deprecated on npm: "+notice))
}

// npmWithdrawn decides a version npm did not serve. It is withdrawn when the
// package document still records when it was published. When the package's
// latest version is a security holding package (0.0.1-security), npm took the
// package over, which it does to malware, and the summary says so.
func npmWithdrawn(doc *npmPackument, name, version string) Result {
	if _, served := doc.Versions[version]; served {
		return Result{}
	}
	raw, ok := doc.Time[version]
	if !ok {
		return Result{}
	}
	var published string
	_ = json.Unmarshal(raw, &published)
	day, _, _ := strings.Cut(published, "T")
	summary := fmt.Sprintf("npm no longer serves %s@%s", name, version)
	if day != "" {
		summary += ", published " + day
	}
	if strings.HasSuffix(doc.DistTags["latest"], "-security") {
		summary += "; npm replaced the package with a security holding package"
	}
	return found(finding(RuleNPMWithdrawn, summary))
}
