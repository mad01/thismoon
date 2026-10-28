package osv

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

// actionsEcosystem is OSV's ecosystem string for GitHub Actions.
const actionsEcosystem = "GitHub Actions"

// defaultDataURL is the bucket OSV.dev publishes its data exports in.
const defaultDataURL = "https://osv-vulnerabilities.storage.googleapis.com"

// actionsExportPath is the export of every GitHub Actions advisory: a zip of
// one JSON record per advisory, about 100 KB.
const actionsExportPath = "/GitHub%20Actions/all.zip"

// maxExport caps the downloaded export, and maxRecord one record in it.
const (
	maxExport = 64 << 20
	maxRecord = 4 << 20
)

// ActionNamesError reports that OSV's GitHub Actions name list could not be
// read. Check returns it beside complete results: every name lookup used the
// name as written, so an advisory OSV files under another capitalization of
// it, or under a longer name, was missed.
type ActionNamesError struct{ Err error }

func (e *ActionNamesError) Error() string {
	return fmt.Sprintf(
		"OSV's GitHub Actions name list: %v; names were looked up as written, "+
			"so an advisory filed under another capitalization or a longer name is missed",
		e.Err,
	)
}

func (e *ActionNamesError) Unwrap() error { return e.Err }

// githubURL starts the Action names OSV files as a URL.
const githubURL = "https://github.com/"

// actionRepo returns the owner/repo discovery names a GitHub Action by, for
// name as OSV files an advisory under it. A few names are longer: a
// sub-action's path (super-linter/super-linter/slim) or a URL to it
// (https://github.com/pytorch/pytorch/.github/actions/filter-test-configs).
// The github.com prefix and the path past owner/repo are cut. A name with no
// owner/repo it can read for sure (a URL that doesn't start
// https://github.com/, fewer than two segments, an empty owner or repo) comes
// back as it is: missed, never filed under the wrong row.
func actionRepo(name string) string {
	path := name
	if strings.Contains(name, "://") {
		if len(name) < len(githubURL) || !strings.EqualFold(name[:len(githubURL)], githubURL) {
			return name
		}
		path = name[len(githubURL):]
	}
	owner, rest, _ := strings.Cut(path, "/")
	repo, _, _ := strings.Cut(rest, "/")
	if owner == "" || repo == "" {
		return name
	}
	return owner + "/" + repo
}

// actionNames maps a GitHub Action's owner/repo, lowercased, to each name OSV
// files its advisories under: another capitalization, or a longer name that
// actionRepo cuts down to it. OSV matches names case-sensitively while GitHub
// does not, so azure/setup-kubectl finds nothing until it is asked for as
// Azure/setup-kubectl. Owner and repo names are ASCII, so lowercasing is the
// same as strings.EqualFold.
type actionNames map[string][]string

// spellings returns the names to look name up by: name as written, then each
// other name OSV files it under. The written name stays even when OSV has
// others, since the export can lag an advisory filed under it.
func (n actionNames) spellings(name string) []string {
	out := []string{name}
	for _, s := range n[strings.ToLower(name)] {
		if s != name {
			out = append(out, s)
		}
	}
	return out
}

// needsActionNames reports whether any query looks a GitHub Action up by
// name, the only lookup the name list serves.
func needsActionNames(queries []Query) bool {
	return slices.ContainsFunc(queries, func(q Query) bool {
		return q.Commit != "" && q.Ecosystem == actionsEcosystem
	})
}

// fetchActionNames downloads OSV's GitHub Actions export and collects the package
// names its records file advisories under.
func (c *Client) fetchActionNames(ctx context.Context) (actionNames, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.dataURL+actionsExportPath,
		nil,
	)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", req.URL, res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxExport+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", req.URL, err)
	}
	if len(raw) > maxExport {
		return nil, fmt.Errorf("%s is larger than %d bytes", req.URL, maxExport)
	}
	return parseActionNames(raw)
}

// parseActionNames reads the export zip, listing each name under its
// owner/repo. A record it can't read fails the whole list, since a name left
// out of it would be missed without a trace.
func parseActionNames(raw []byte) (actionNames, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	names := actionNames{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		v, err := readRecord(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		for _, a := range v.Affected {
			name, key := a.Package.Name, strings.ToLower(actionRepo(a.Package.Name))
			if a.Package.Ecosystem == actionsEcosystem && !slices.Contains(names[key], name) {
				names[key] = append(names[key], name)
			}
		}
	}
	return names, nil
}

// readRecord decodes one advisory record from the export.
func readRecord(f *zip.File) (*vuln, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	var v vuln
	if err := json.NewDecoder(io.LimitReader(rc, maxRecord)).Decode(&v); err != nil {
		return nil, err
	}
	return &v, nil
}
