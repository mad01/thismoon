package osv

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// affects reports whether an advisory found by package name alone applies to
// q.Version: whether the version falls in one of the record's ECOSYSTEM or
// SEMVER ranges for the package. It fails open, so no advisory is hidden: a
// label that is not a full numeric version (v4, main), a record that didn't
// load, and a range bound it can't compare all keep the advisory.
func (v *vuln) affects(q Query) bool {
	ver, ok := parseVersion(q.Version)
	if !ok {
		return true
	}
	judged := false
	for _, a := range v.Affected {
		if !q.matchesPackage(a.Package.Ecosystem, a.Package.Name) {
			continue
		}
		for _, r := range a.Ranges {
			if r.Type != "ECOSYSTEM" && r.Type != "SEMVER" {
				continue // GIT ranges are the commit lookup's
			}
			in, ok := inRange(r.Events, ver)
			if !ok || in {
				return true
			}
			judged = true
		}
	}
	return !judged
}

// inRange reports whether ver falls in the range the events bound, evaluated
// the way the OSV schema does: events sorted by version, each introduced
// opening the range and each fixed or last_affected closing it. At one
// version, whatever the record's order, a fixed sorts first, then an
// introduced, then a last_affected. A range re-introduced at its own fix
// stays open, and a last_affected keeps the version it shares with an
// introduced. An introduced and a fixed at one version read as affected from
// there on, which keeps the advisory rather than hiding it. ok is false when
// an event's version can't be compared.
func inRange(events []event, ver version) (in, ok bool) {
	type bound struct {
		at    version
		opens bool
		// inclusive marks last_affected, the last version still affected.
		inclusive bool
	}
	bounds := make([]bound, 0, len(events))
	for _, e := range events {
		var (
			b   bound
			raw string
		)
		switch {
		case e.Introduced != "":
			b.opens, raw = true, e.Introduced
		case e.Fixed != "":
			raw = e.Fixed
		case e.LastAffected != "":
			b.inclusive, raw = true, e.LastAffected
		default:
			continue
		}
		if raw != "0" {
			if b.at, ok = parseBound(raw); !ok {
				return false, false
			}
		}
		bounds = append(bounds, b)
	}
	// tie orders the bounds at one version: fixed, introduced, last_affected.
	tie := func(b bound) int {
		switch {
		case b.opens:
			return 1
		case b.inclusive:
			return 2
		default:
			return 0
		}
	}
	slices.SortStableFunc(bounds, func(a, b bound) int {
		return cmp.Or(a.at.compare(b.at), cmp.Compare(tie(a), tie(b)))
	})
	for _, b := range bounds {
		c := ver.compare(b.at)
		switch {
		case b.opens:
			in = in || c >= 0
		case b.inclusive:
			in = in && c <= 0
		default:
			in = in && c < 0
		}
	}
	return in, true
}

// version is a numeric major.minor.patch.
type version [3]uint64

// compare orders v against o: negative, zero or positive.
func (v version) compare(o version) int {
	return slices.CompareFunc(v[:], o[:], cmp.Compare[uint64])
}

// parseVersion reads a full version label, with or without a leading v:
// v45.0.7 or 45.0.7. A bare major (v4), a pre-release, or any other shape is
// not comparable, since its exact version is unknown.
func parseVersion(s string) (version, bool) {
	return parseNumeric(s, 3)
}

// parseBound reads a range bound, which may leave out trailing parts: 41 is
// 41.0.0.
func parseBound(s string) (version, bool) {
	return parseNumeric(s, 1)
}

// parseNumeric reads one to three dot-separated numbers, with or without a
// leading v, requiring at least minParts of them. Missing parts are 0.
func parseNumeric(s string, minParts int) (version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) < minParts || len(parts) > len(version{}) {
		return version{}, false
	}
	var v version
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return version{}, false
		}
		v[i] = n
	}
	return v, true
}
