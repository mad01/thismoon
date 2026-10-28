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
// it was missed.
type ActionNamesError struct{ Err error }

func (e *ActionNamesError) Error() string {
	return fmt.Sprintf(
		"OSV's GitHub Actions name list: %v; names were looked up as written, "+
			"so an advisory filed under another capitalization is missed",
		e.Err,
	)
}

func (e *ActionNamesError) Unwrap() error { return e.Err }

// actionNames maps a GitHub Action name, lowercased, to each spelling OSV
// files advisories under. OSV matches names case-sensitively while GitHub
// does not, so azure/setup-kubectl finds nothing until it is asked for as
// Azure/setup-kubectl. Owner and repo names are ASCII, so lowercasing is the
// same as strings.EqualFold.
type actionNames map[string][]string

// spellings returns the names to look name up by: OSV's spellings of it, or
// name itself when OSV has none, as when the list could not be read.
func (n actionNames) spellings(name string) []string {
	if s := n[strings.ToLower(name)]; len(s) > 0 {
		return s
	}
	return []string{name}
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

// parseActionNames reads the export zip. A record it can't read fails the
// whole list, since a name left out of it would be missed without a trace.
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
			name, key := a.Package.Name, strings.ToLower(a.Package.Name)
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
