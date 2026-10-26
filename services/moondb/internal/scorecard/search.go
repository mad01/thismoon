package scorecard

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// The csl search tools: their results say how a search went.
const (
	cslSearch   = "csl_search"
	cslSemantic = "csl_semantic_search"
	cslHybrid   = "csl_hybrid_search"
)

// searchTools are the csl search tools.
var searchTools = []string{cslSearch, cslSemantic, cslHybrid}

// isSearch reports whether a tool call is a csl search.
func isSearch(name string) bool {
	return strings.HasPrefix(name, "mcp__") && slices.Contains(searchTools, toolName(name))
}

// searchOutcome is how one csl search came back.
type searchOutcome struct {
	zero, truncated, zeroHint, relaxed, failed, unread bool
}

// The csl result grammar a search outcome relies on, in the default text
// format (services/csl, internal/mcpserver/render_text.go). csl_search:
//
//	no matches                       nothing found; the lines after it, when
//	parsed query: ...                any, are the zero_result_hint
//	relaxed: dropped ...             first line when terms were dropped
//	N files | N lines in M files     the trailer, after the last blank line,
//	truncated: showing N of M ...    with this line when more results existed
//
// csl_semantic_search and csl_hybrid_search end in a trailer of "N hits"
// and are never truncated. Hit lines and snippets come before the
// trailer, so their text is never read as one. The JSON format has the
// same facts as keys: csl_search total, truncated, zero_result_hint and
// relaxed_query; csl_semantic_search available and hits; csl_hybrid_search
// hits. Both omit hits when there are none.
var (
	hitsTrailer = regexp.MustCompile(`^(\d+) hits?$`)
	// jsonZero and jsonTruncated read a csl_search JSON result the
	// transcript keeps only the head of.
	jsonZero      = regexp.MustCompile(`"total":\s*0[,}]`)
	jsonTruncated = regexp.MustCompile(`"truncated":\s*true`)
)

// classifySearch says how a csl search came back from what the transcript
// kept of its result.
func classifySearch(c Call) searchOutcome {
	if c.IsError {
		return searchOutcome{failed: true}
	}
	text := strings.TrimSpace(c.Result)
	if !c.Answered || strings.HasPrefix(text, persistedPrefix) {
		return searchOutcome{unread: true}
	}
	tool := toolName(c.Name)
	if strings.HasPrefix(text, "{") {
		if o, ok := searchJSON(tool, text); ok {
			return o
		}
		o := searchJSONHead(tool, text)
		o.unread = !o.zero
		return o
	}
	var o searchOutcome
	if tool == cslSearch {
		o = searchText(text, c.Cut)
	} else {
		o = hitsText(text, c.Cut)
	}
	o.unread = c.Cut && !o.zero
	return o
}

// searchText reads csl_search's text format. A result cut to its head has
// lost its trailer.
func searchText(text string, cut bool) searchOutcome {
	var o searchOutcome
	lines := strings.Split(text, "\n")
	first := strings.TrimSpace(lines[0])
	switch {
	case first == "no matches":
		o.zero = true
		o.zeroHint = len(lines) > 1 && strings.TrimSpace(strings.Join(lines[1:], "\n")) != ""
		return o
	case strings.HasPrefix(first, "relaxed:"):
		o.relaxed = true
	}
	if cut {
		return o
	}
	for _, line := range lines[trailerStart(lines):] {
		if strings.HasPrefix(strings.TrimSpace(line), "truncated:") {
			o.truncated = true
		}
	}
	return o
}

// trailerStart is the index of the first line after the last blank one.
func trailerStart(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			return i + 1
		}
	}
	return len(lines)
}

// hitsText reads the text format of csl_semantic_search and
// csl_hybrid_search: only its "N hits" trailer.
func hitsText(text string, cut bool) searchOutcome {
	if cut {
		return searchOutcome{}
	}
	lines := strings.Split(text, "\n")
	m := hitsTrailer.FindStringSubmatch(strings.TrimSpace(lines[len(lines)-1]))
	return searchOutcome{zero: m != nil && m[1] == "0"}
}

// searchResult is the JSON format's shape, the keys a scorecard reads.
type searchResult struct {
	Total          *int            `json:"total"`
	Truncated      bool            `json:"truncated"`
	ZeroResultHint json.RawMessage `json:"zero_result_hint"`
	RelaxedQuery   string          `json:"relaxed_query"`
	Available      bool            `json:"available"`
	Hits           json.RawMessage `json:"hits"`
}

// searchJSON reads the JSON format of tool; false when the text is not a
// whole JSON object.
func searchJSON(tool, text string) (searchOutcome, bool) {
	var r searchResult
	if json.Unmarshal([]byte(text), &r) != nil {
		return searchOutcome{}, false
	}
	noHits := !present(r.Hits) || string(r.Hits) == "[]"
	switch tool {
	case cslSemantic:
		// An unavailable index found nothing because it did not search.
		return searchOutcome{zero: r.Available && noHits}, true
	case cslHybrid:
		return searchOutcome{zero: noHits}, true
	}
	return searchOutcome{
		zero:      r.Total != nil && *r.Total == 0,
		truncated: r.Truncated,
		zeroHint:  present(r.ZeroResultHint),
		relaxed:   r.RelaxedQuery != "",
	}, true
}

// searchJSONHead reads the head of a csl_search JSON result: what the keys
// in it say. The head of a semantic or hybrid result holds hits, so it
// found some.
func searchJSONHead(tool, text string) searchOutcome {
	if tool != cslSearch {
		return searchOutcome{}
	}
	return searchOutcome{
		zero:      jsonZero.MatchString(text),
		truncated: jsonTruncated.MatchString(text),
		zeroHint:  strings.Contains(text, `"zero_result_hint"`),
		relaxed:   strings.Contains(text, `"relaxed_query"`),
	}
}

// present reports whether a JSON value is there and not null.
func present(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// searchStats counts the csl searches among calls.
func searchStats(calls []Call) SearchStats {
	var s SearchStats
	for _, c := range calls {
		if !isSearch(c.Name) {
			continue
		}
		o := classifySearch(c)
		s.Calls++
		s.Zero += count(o.zero)
		s.Truncated += count(o.truncated)
		s.ZeroResultHint += count(o.zeroHint)
		s.Relaxed += count(o.relaxed)
		s.Errors += count(o.failed)
		s.Unread += count(o.unread)
	}
	return s
}

// count is 1 for true, 0 for false.
func count(b bool) int {
	if b {
		return 1
	}
	return 0
}
