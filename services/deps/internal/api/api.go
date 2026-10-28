// Package api defines the JSON wire shapes shared between the serve HTTP API and
// its clients (the CLI and the MCP server). It enriches stored dependencies with
// two derived, non-persisted fields the UI needs: each advisory's stable Key
// (to resolve it) and whether it is currently Resolved.
package api

import (
	"sort"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// Advisory is a store.Advisory plus its resolve key and resolved status.
type Advisory struct {
	ID           string     `json:"id"`
	Kind         store.Kind `json:"kind"                    jsonschema:"vuln (a known vulnerability), malicious (a known malicious version: listed first, alerts even when not compiled in) or signal (a warning sign that never alerts or counts as active)"`
	Summary      string     `json:"summary,omitempty"`
	Severity     string     `json:"severity,omitempty"`
	FixedVersion string     `json:"fixed_version,omitempty"`
	Key          string     `json:"key"`      // pass back to POST /api/resolve
	Resolved     bool       `json:"resolved"` // user has acknowledged this occurrence
}

// Dependency is a store.Dependency with enriched advisories.
type Dependency struct {
	Ecosystem  string     `json:"ecosystem"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	Repo       string     `json:"repo"`
	Direct     bool       `json:"direct"`
	Imported   bool       `json:"imported"` // contributes compiled code (false = graph-only transitive)
	Advisories []Advisory `json:"advisories,omitempty"`
}

// Active reports whether the dependency needs the user's attention: it has at
// least one unresolved finding whose kind Alerts. A vulnerability on a
// graph-only (not imported) dep never counts, since it isn't a real exposure
// and can't be fixed by a version bump; a malicious finding always counts; a
// signal never does.
func (d Dependency) Active() bool {
	for _, a := range d.Advisories {
		if !a.Resolved && a.Kind.Alerts(d.Imported) {
			return true
		}
	}
	return false
}

// Malicious reports whether the dependency has an unresolved malicious
// finding.
func (d Dependency) Malicious() bool {
	for _, a := range d.Advisories {
		if !a.Resolved && a.Kind == store.KindMalicious {
			return true
		}
	}
	return false
}

// Enrich converts stored deps to API deps, computing each advisory's key and
// resolved status via isResolved.
func Enrich(deps []store.Dependency, isResolved func(key string) bool) []Dependency {
	out := make([]Dependency, 0, len(deps))
	for _, d := range deps {
		ad := Dependency{
			Ecosystem: d.Ecosystem,
			Name:      d.Name,
			Version:   d.Version,
			Repo:      d.Repo,
			Direct:    d.Direct,
			Imported:  d.Imported,
		}
		for _, a := range d.Advisories {
			key := store.FlagKey(d.Ecosystem, d.Name, d.Version, a.ID)
			ad.Advisories = append(ad.Advisories, Advisory{
				ID:           a.ID,
				Kind:         a.Kind,
				Summary:      a.Summary,
				Severity:     a.Severity,
				FixedVersion: a.FixedVersion,
				Key:          key,
				Resolved:     isResolved(key),
			})
		}
		out = append(out, ad)
	}
	return out
}

// Flagged returns only the deps that have at least one advisory. Deps with an
// unresolved malicious finding come first, so a reader that stops at the top
// of the list still sees them; the order is otherwise kept.
func Flagged(deps []Dependency) []Dependency {
	var out []Dependency
	for _, d := range deps {
		if len(d.Advisories) > 0 {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Malicious() && !out[j].Malicious()
	})
	return out
}

// ActiveCount counts the deps that are Active.
func ActiveCount(deps []Dependency) int {
	n := 0
	for _, d := range deps {
		if d.Active() {
			n++
		}
	}
	return n
}
