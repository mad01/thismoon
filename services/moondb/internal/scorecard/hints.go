package scorecard

import (
	"cmp"
	"path"
	"slices"
	"strings"
)

// hintStats counts the hints a session was shown and joins the assertions
// the kof hints showed to the files the session touched after them. nil
// hints, the events service not answering, give nil.
func hintStats(hints []HintEvent, pins map[string][]Pin, calls []Call) *HintStats {
	if hints == nil {
		return nil
	}
	return &HintStats{
		Shown:  len(hints),
		ByHint: byHint(hints),
		Kof:    kofStats(hints, pins, touched(calls)),
	}
}

// byHint counts the hints by id, most shown first, ties by id.
func byHint(hints []HintEvent) []HintCount {
	counts := map[string]int{}
	for _, h := range hints {
		counts[h.Hint]++
	}
	out := make([]HintCount, 0, len(counts))
	for hint, n := range counts {
		out = append(out, HintCount{Hint: hint, Count: n})
	}
	slices.SortFunc(out, func(a, b HintCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Hint, b.Hint))
	})
	return out
}

// kofStats lists each assertion the hints showed, once, at its first
// showing, with its pins and whether a later call touched them. A hint
// serves the whole session, so a call of any agent counts. The pins come
// from pins (keeper-of-facts), which knows each assertion's own; else from
// the hint's pins tag, whose bare paths the fire's assertions share; an
// assertion neither gives keeps them unknown.
func kofStats(hints []HintEvent, pins map[string][]Pin, touches []touchedPath) KofStats {
	k := KofStats{Assertions: []ShownAssertion{}}
	seen := map[string]bool{}
	for _, h := range hints {
		for _, id := range h.Assertions {
			if seen[id] {
				continue
			}
			seen[id] = true
			a := ShownAssertion{ID: id, Hint: h.Hint, At: h.At, AgentID: h.AgentID}
			ps, known := pins[id]
			if !known {
				ps, known = h.Pins[id]
			}
			if !known {
				k.UnknownPins++
				k.Assertions = append(k.Assertions, a)
				continue
			}
			a.Pins, a.Used = pinUses(ps, h, touches)
			k.PinnedPaths += len(a.Pins)
			for _, p := range a.Pins {
				k.UsedPaths += count(p.Used)
			}
			k.Assertions = append(k.Assertions, a)
		}
	}
	if k.PinnedPaths > 0 {
		rate := float64(k.UsedPaths) / float64(k.PinnedPaths)
		k.HitRate = &rate
	}
	return k
}

// pinUses marks each pinned file used when a call of any agent touched it
// after the hint, and reports whether any was. A file pinned twice, at two
// line ranges, is listed once.
func pinUses(pins []Pin, h HintEvent, touches []touchedPath) ([]PinUse, *bool) {
	out := make([]PinUse, 0, len(pins))
	anyUsed := false
	for _, p := range pins {
		if slices.ContainsFunc(out, func(u PinUse) bool { return u.Path == p.File }) {
			continue
		}
		used := slices.ContainsFunc(touches, func(t touchedPath) bool {
			return t.call.At.After(h.At) && p.touchedBy(t)
		})
		anyUsed = anyUsed || used
		out = append(out, PinUse{Path: p.File, Used: used})
	}
	return out, &anyUsed
}

// touchedBy reports whether a touched file is the pinned one: its path is
// the pin's or ends in it, and, when the pin names its repository, the
// file is in it. A path on disk holds the repository's name as a directory;
// a worktree keeps it too (~/.worktrees/<name>/<branch>/...). A csl_read
// names the repository as owner/name, which must match the pin's exactly.
func (p Pin) touchedBy(t touchedPath) bool {
	file := strings.TrimPrefix(p.File, "./")
	if file == "" || t.path != file && !strings.HasSuffix(t.path, "/"+file) {
		return false
	}
	full := ownerName(p.Repo)
	if full == "" {
		return true
	}
	if t.repo != "" {
		return strings.EqualFold(ownerName(strings.Trim(t.repo, "^$")), full)
	}
	return strings.Contains(t.path, "/"+path.Base(full)+"/")
}

// ownerName is the owner/name of a repository: the last two parts of
// github.com/owner/name, or the name alone when it has one part.
func ownerName(repo string) string {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, "/")
}
