package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// The zero-result path of csl_search: per-term file counts that name the
// term that killed the query, the one relaxation the tool applies on its
// own, and the hint payload that explains an empty page.

// maxDiagnosedTerms caps the per-term counts a zero-hit query triggers. Past
// it the counts cover the first terms only and no relaxation runs, since an
// uncounted term may be the one that matches nothing.
const maxDiagnosedTerms = 6

// termDiagnosis is what the per-term file counts say about a zero-hit query.
type termDiagnosis struct {
	counts  []termCount
	present []string // terms that match at least one file on their own
	dropped []string // terms that match no file, when a relaxation applies
	relaxed string   // the query to rerun without dropped; "" when none
	notes   []string
}

// explainZero turns an empty page into either the one relaxation the tool
// applies on its own or a hint that names the term that killed the query.
// The relaxation runs only when its outcome is unambiguous: some AND terms
// match no file at all while others do, on the first page. The rerun keeps
// every filter and the limit; when it also finds nothing, the hint says so.
func explainZero(
	ctx context.Context,
	b searchBackend,
	in searchInput,
	opts search.SearchOptions,
	repos []finder.Repo,
	indexDir string,
) searchOutput {
	diag := diagnoseTerms(ctx, b, splitTerms(in.Query), opts)
	if diag.relaxed != "" {
		relaxedOpts := opts
		relaxedOpts.Pattern = diag.relaxed
		relaxedOpts.Offset = 0
		matches, err := b.search(ctx, relaxedOpts)
		switch {
		case err != nil:
			diag.notes = append(diag.notes, fmt.Sprintf(
				"dropped %s (0 files) but the rerun as `%s` failed: %v",
				quoteList(diag.dropped), diag.relaxed, err,
			))
		case len(matches) > 0:
			out := buildSearchOutput(opts.OutputMode, opts.Limit, 0, matches)
			out.RelaxedQuery = diag.relaxed
			out.DroppedTerms = diag.dropped
			return out
		default:
			diag.notes = append(diag.notes, fmt.Sprintf(
				"dropped %s (0 files) and reran as `%s`: still no file holds the remaining terms together; search %s for files with any of them",
				quoteList(diag.dropped),
				diag.relaxed,
				strings.Join(diag.present, "|"),
			))
		}
	}
	out := searchOutput{OutputMode: opts.OutputMode, Offset: opts.Offset}
	out.ZeroHint = buildZeroHint(in, opts, repos, indexDir, diag)
	return out
}

// diagnoseTerms counts, for a query with two or more AND terms, the files
// each term matches on its own under the same filters, and decides whether
// the terms matching nothing can be dropped. It never drops when every term
// exists (the fix is the a|b form, which changes the question), when the
// page is not the first, or when the query has more terms than were counted.
func diagnoseTerms(
	ctx context.Context,
	b searchBackend,
	parts queryParts,
	opts search.SearchOptions,
) termDiagnosis {
	var d termDiagnosis
	if len(parts.Terms) < 2 {
		return d
	}
	terms := parts.Terms
	if len(terms) > maxDiagnosedTerms {
		terms = terms[:maxDiagnosedTerms]
	}
	var zero []string
	for _, term := range terms {
		_, n, err := b.count(ctx, termCountOptions(term, parts, opts))
		if err != nil {
			d.notes = append(d.notes, "per-term file counts unavailable: "+err.Error())
			return d
		}
		d.counts = append(d.counts, termCount{Term: term, Files: n})
		if n == 0 {
			zero = append(zero, term)
		} else {
			d.present = append(d.present, term)
		}
	}
	switch {
	case len(parts.Terms) > maxDiagnosedTerms:
		d.notes = append(d.notes, fmt.Sprintf(
			"query has %d AND terms and only the first %d were counted, so nothing was dropped automatically; remove the terms with 0 files and retry with at most 2 terms",
			len(parts.Terms),
			maxDiagnosedTerms,
		))
	case len(zero) == 0:
		d.notes = append(d.notes, fmt.Sprintf(
			"every term matches files on its own but no file contains all %d; search %s for files with any of them, or one term with a narrower file filter",
			len(d.counts),
			strings.Join(d.present, "|"),
		))
	case len(d.present) == 0:
		d.notes = append(
			d.notes,
			"no term matches any file under these filters; check each term's spelling and the filters before retrying",
		)
	case opts.Offset > 0:
		d.notes = append(d.notes, fmt.Sprintf(
			"%s %s no file (0 files); not dropped automatically because offset > 0 — retry with offset 0 or without %s",
			quoteList(zero),
			plural(len(zero), "matches", "match"),
			plural(len(zero), "it", "them"),
		))
	default:
		d.dropped = zero
		d.relaxed = parts.without(zero)
	}
	return d
}

// plural picks the word form for n items.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// termCountOptions builds the count that measures one term alone: the term,
// the query's fixed atoms, and the tool's filters, wrapped in type:filename
// so zoekt returns one match per file and the total is a file count.
func termCountOptions(
	term string,
	parts queryParts,
	opts search.SearchOptions,
) search.CountOptions {
	single := opts
	single.Pattern = strings.Join(append([]string{"type:filename", term}, parts.Fixed...), " ")
	return search.CountOptions{Pattern: search.BuildQueryString(single)}
}

// buildZeroHint assembles the zero_result_hint payload for a search that ran
// cleanly but matched nothing. Best-effort: any piece that cannot be computed
// is omitted rather than failing the response.
func buildZeroHint(
	in searchInput,
	opts search.SearchOptions,
	repos []finder.Repo,
	indexDir string,
	diag termDiagnosis,
) *searchZeroHint {
	hint := &searchZeroHint{
		ReposDiscovered: len(repos),
		TermCounts:      diag.counts,
		Notes:           zeroNotes(in, diag),
	}

	if info := search.ValidateQuery(search.BuildQueryString(opts)); info.Valid {
		hint.ParsedQuery = info.Parsed
	}

	indexed := make(map[string]struct{})
	if state, err := search.LoadState(indexDir); err == nil {
		hint.ReposIndexed = len(state.Repos)
		var newest, oldest time.Time
		for path, rs := range state.Repos {
			indexed[path] = struct{}{}
			if rs.IndexedAt.IsZero() {
				continue
			}
			if newest.IsZero() || rs.IndexedAt.After(newest) {
				newest = rs.IndexedAt
			}
			if oldest.IsZero() || rs.IndexedAt.Before(oldest) {
				oldest = rs.IndexedAt
			}
		}
		if !newest.IsZero() {
			hint.NewestIndexedAt = newest.Format(time.RFC3339)
			hint.OldestIndexedAt = oldest.Format(time.RFC3339)
		}
	}

	searched, notes := repoFilterHint(in.Repo, repos, indexed)
	hint.ReposSearched = searched
	hint.Notes = append(hint.Notes, notes...)
	return hint
}

// zeroNotes orders the query-level notes: syntax traps, the parameter-name
// trap, what the per-term counts found, then the over-constraint reminders.
// The generic AND-terms reminder yields to the counts when they exist, since
// they say which term is the problem.
func zeroNotes(in searchInput, diag termDiagnosis) []string {
	notes := queryTrapNotes(in.Query)
	if n := paramNameNote(in.Query); n != "" {
		notes = append(notes, n)
	}
	notes = append(notes, diag.notes...)
	if len(diag.counts) == 0 {
		notes = append(notes, andTermsNote(in.Query)...)
	}
	return append(notes, overConstraintNotes(in)...)
}

// repoFilterHint reports how many discovered repos the repo filter matches
// AND the index actually covers — zoekt cannot return hits from a repo that
// is discovered on disk but not yet indexed. It mirrors the case-insensitive
// matching the repo tools use; the whitespace case is called out instead of
// diagnosed, because the search itself space-splits the filter into separate
// zoekt terms and no repo-name count describes what actually ran.
func repoFilterHint(
	repoFilter string,
	repos []finder.Repo,
	indexed map[string]struct{},
) (int, []string) {
	countIndexed := func(rs []finder.Repo) (n int) {
		for _, r := range rs {
			if _, ok := indexed[r.Path]; ok {
				n++
			}
		}
		return n
	}

	if repoFilter == "" {
		return countIndexed(repos), nil
	}
	if strings.ContainsAny(repoFilter, " \t") {
		return 0, []string{fmt.Sprintf(
			"repo filter %q contains whitespace; the search splits it into separate zoekt terms, so it is not matched as one repo name — use a regex without spaces",
			repoFilter,
		)}
	}

	re, err := finder.CompileMatcher(repoFilter)
	if err != nil {
		return 0, nil
	}
	var matched []finder.Repo
	for _, r := range repos {
		if re.MatchString(r.Name) {
			matched = append(matched, r)
		}
	}
	if len(matched) == 0 {
		return 0, []string{fmt.Sprintf(
			"repo filter %q matched none of the %d locally discovered repos; check the name with csl_repo_lookup — if the repo is not checked out locally, csl cannot see it, so search it where it is hosted instead of retrying here",
			repoFilter,
			len(repos),
		)}
	}
	searched := countIndexed(matched)
	if unindexed := len(matched) - searched; unindexed > 0 {
		return searched, []string{fmt.Sprintf(
			"%d of the %d repos matching the filter are not in the search index yet and are invisible to search; run csl_repo_reindex on them",
			unindexed,
			len(matched),
		)}
	}
	return searched, nil
}

// queryTrapNotes flags known zoekt syntax traps present in the raw query that
// commonly explain a surprising zero-hit result. Quoted phrases are stripped
// first: an ' OR ' inside a "quoted literal" is content, not an operator.
func queryTrapNotes(query string) []string {
	query = stripQuoted(query)
	var notes []string
	if strings.Contains(query, " | ") {
		notes = append(notes,
			"'a | b' parses as three AND terms, not OR; write a|b with no spaces")
	}
	if strings.Contains(query, " OR ") {
		notes = append(notes,
			"uppercase OR is a literal search term; use | with no spaces or lowercase 'or'")
	}
	return notes
}

// andTermsNote flags the query shape that most often explains a zero-hit
// search: 3+ AND terms that must all land in one file.
func andTermsNote(query string) []string {
	n := len(splitTerms(query).Terms)
	if n < 3 {
		return nil
	}
	return []string{fmt.Sprintf(
		"query has %d AND terms that must ALL appear in the same file — retry with 1-2 key terms, or join alternatives as a|b (no spaces)",
		n,
	)}
}

// overConstraintNotes flags the other shapes that often explain a zero-hit
// search: a verbatim-only quoted phrase and a stacked file filter. Sessions
// loosen these one guess at a time over long refinement chains; naming them
// up front is what shortens the chain.
func overConstraintNotes(in searchInput) []string {
	var notes []string
	for _, span := range quotedSpans(in.Query) {
		if strings.ContainsAny(span, " \t") {
			notes = append(
				notes,
				"a \"quoted phrase\" matches only that exact text verbatim — drop the quotes to match the words as separate AND terms",
			)
			break
		}
	}
	if in.File != "" {
		notes = append(
			notes,
			"the file filter is the most common over-constraint — retry without it before loosening the query",
		)
	}
	return notes
}

// quotedSpans returns the contents of every complete double-quoted span in
// the query, in order. An unclosed quote yields no span for its tail.
func quotedSpans(s string) []string {
	var spans []string
	for {
		i := strings.Index(s, `"`)
		if i < 0 {
			return spans
		}
		s = s[i+1:]
		j := strings.Index(s, `"`)
		if j < 0 {
			return spans
		}
		spans = append(spans, s[:j])
		s = s[j+1:]
	}
}

// stripQuoted removes double-quoted spans from a query so trap sniffing does
// not fire on operators that appear inside a quoted literal phrase.
func stripQuoted(s string) string {
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			b.WriteRune(r)
		}
	}
	return b.String()
}
