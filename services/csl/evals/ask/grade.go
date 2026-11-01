package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Grade is how one answer scored against its case.
type Grade struct {
	Found     int      `json:"found"`
	Total     int      `json:"total"`
	Missing   []string `json:"missing,omitempty"`
	Forbidden []string `json:"forbidden,omitempty"`
	Chars     int      `json:"chars"`
	Budget    int      `json:"budget"`
	BudgetOK  bool     `json:"budget_ok"`
	// LineClaims counts the path:line and "line N" references in the
	// answer that stand beside an identifier. LineClaimsWrong are those
	// whose identifier is not on that line in any corpus file.
	LineClaims      int `json:"line_claims"`
	LineClaimsWrong int `json:"line_claims_wrong"`
	// WrongClaims describes each wrong claim: the line it named, the
	// bullet that made it, and what the line reads.
	WrongClaims []string `json:"wrong_claims,omitempty"`
	Score       float64  `json:"score"`
	Pass        bool     `json:"pass"`
}

// grade scores an answer: the share of expected facts present, any
// forbidden match, the budget, and the line claims. A row passes only with
// every fact, no forbidden match, and an answer inside the budget.
func grade(c Case, corpus Corpus, answer string) Grade {
	g := Grade{Total: len(c.expected), Chars: len(answer), Budget: c.MaxAnswerChars}
	for i, re := range c.expected {
		if re.MatchString(answer) {
			g.Found++
		} else {
			g.Missing = append(g.Missing, c.Expected[i])
		}
	}
	for i, re := range c.forbidden {
		if re.MatchString(answer) {
			g.Forbidden = append(g.Forbidden, c.Forbidden[i])
		}
	}
	g.BudgetOK = g.Chars <= g.Budget
	g.LineClaims, g.LineClaimsWrong, g.WrongClaims = checkLineClaims(corpus, answer)
	if g.Total > 0 {
		g.Score = float64(g.Found) / float64(g.Total)
	}
	g.Pass = g.Found == g.Total && len(g.Forbidden) == 0 && g.BudgetOK
	return g
}

var (
	// lineClaimRE finds "line 42", "lines 42-50", "file.go:42" and
	// "file.go:42-50", with a hyphen, en dash or em dash in a range, since
	// the larger models write ranges with an en dash. The extension before
	// the colon keeps clock times and ratios out.
	lineClaimRE = regexp.MustCompile(
		`(?i)\blines?\s+(\d{1,6})(?:\s*[-\x{2013}\x{2014}]\s*(\d{1,6}))?\b` +
			`|\.[a-z]{1,5}:(\d{1,6})(?:[-\x{2013}\x{2014}](\d{1,6}))?\b`,
	)
	identRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
	// pathTokenRE drops the path tokens around a claim, so a directory
	// name is not mistaken for an identifier the line must carry.
	pathTokenRE = regexp.MustCompile(`\S*/\S*|\S+\.[a-z]{1,5}:\d+`)
	// identStop are the words a bullet uses around a claim that say
	// nothing about the line itself. The list stays short on purpose: a
	// word such as "path" or "kind" is often the very token the line
	// carries, and dropping it turned right claims into wrong ones.
	identStop = map[string]bool{
		"and": true, "are": true, "also": true, "but": true, "defined": true,
		"defines": true, "for": true, "from": true, "func": true, "function": true,
		"has": true, "have": true, "its": true, "line": true, "lines": true,
		"method": true, "returns": true, "return": true, "see": true, "that": true,
		"the": true, "then": true, "this": true, "when": true, "where": true,
		"which": true, "with": true,
	}
)

// checkLineClaims verifies every line reference in the answer against the
// corpus. A claim is right when one identifier from its bullet appears,
// case-insensitively, on that line, inside the cited range, or in the few
// lines above a range (claimContext) in some corpus file; a bullet with no
// identifier beside the number is not a claim. The check is a heuristic for the report rather than a gate: it
// counts the claims a worker makes and how many are wrong, and errs
// toward calling a claim right when the bullet names anything the range
// holds.
func checkLineClaims(corpus Corpus, answer string) (claims, wrong int, detail []string) {
	for bullet := range strings.SplitSeq(answer, "\n") {
		matches := lineClaimRE.FindAllStringSubmatch(bullet, -1)
		if len(matches) == 0 {
			continue
		}
		idents := bulletIdents(pathTokenRE.ReplaceAllString(bullet, " "))
		if len(idents) == 0 {
			continue
		}
		for _, m := range matches {
			from, to := claimRange(m)
			if from <= 0 {
				continue
			}
			claims++
			if !rangeMentions(corpus, from, to, idents) {
				wrong++
				detail = append(detail, fmt.Sprintf(
					"%d named by %q; line reads %q",
					from,
					truncate(bullet, 90),
					truncate(strings.Join(corpus.lineText(from), " / "), 90),
				))
			}
		}
	}
	return claims, wrong, detail
}

// claimRange reads the start and end of one lineClaimRE match. A single
// line is a range of one; an end before the start, or a span past 200
// lines, is read as the start alone, since neither is a citation.
func claimRange(m []string) (from, to int) {
	start, end := m[1], m[2]
	if start == "" {
		start, end = m[3], m[4]
	}
	from, err := strconv.Atoi(start)
	if err != nil || from <= 0 {
		return 0, 0
	}
	to = from
	if end != "" {
		if n, err := strconv.Atoi(end); err == nil && n >= from && n-from <= 200 {
			to = n
		}
	}
	return from, to
}

// claimContext is how many lines above a cited range the checker also
// reads. A range names a block, and the declaration that names the block
// sits just above its first line. A single-line claim stays exact.
const claimContext = 10

func rangeMentions(corpus Corpus, from, to int, idents []string) bool {
	start := from
	if to > from {
		start = max(1, from-claimContext)
	}
	for n := start; n <= to; n++ {
		if lineMentions(corpus.lineText(n), idents) {
			return true
		}
	}
	return false
}

func bulletIdents(bullet string) []string {
	var out []string
	for _, id := range identRE.FindAllString(bullet, -1) {
		if identStop[strings.ToLower(id)] {
			continue
		}
		out = append(out, id)
	}
	return out
}

func lineMentions(lines, idents []string) bool {
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, id := range idents {
			if strings.Contains(lower, strings.ToLower(id)) {
				return true
			}
		}
	}
	return false
}

// ArmStats is the per-arm headline.
type ArmStats struct {
	Arm             Arm
	Rows            int
	Errors          int
	PassRate        float64
	MeanScore       float64
	ForbiddenRate   float64
	BudgetFailRate  float64
	LineClaims      int
	LineClaimsWrong int
	// Compression is the mean of answer chars over corpus chars: what the
	// caller's context pays instead of the files.
	Compression   float64
	LatencyP50    int64
	MeanPromptTok float64
	MeanOutputTok float64
	TotalCost     float64
}

// maxWrongClaimsShown caps the wrong-claim detail a case carries into the
// summary; the traces hold the rest.
const maxWrongClaimsShown = 6

// CaseStats is one case on one arm across its reps.
type CaseStats struct {
	ID          string
	Reps        int
	Passes      int
	MeanScore   float64
	MinScore    float64
	Missing     []string
	Forbidden   []string
	WrongClaims []string
	Compression float64
}

func summarize(arm Arm, rows []Row, errs []ErrRow) (ArmStats, map[string]CaseStats) {
	st := ArmStats{Arm: arm}
	byCase := map[string][]Row{}
	for _, r := range rows {
		if r.Variant != arm.ID {
			continue
		}
		st.Rows++
		byCase[r.ID] = append(byCase[r.ID], r)
	}
	for _, e := range errs {
		if e.Variant == arm.ID {
			st.Errors++
		}
	}
	cases := map[string]CaseStats{}
	for id, rs := range byCase {
		cases[id] = caseStats(id, rs)
	}
	fillRates(&st, byCase)
	return st, cases
}

func caseStats(id string, rs []Row) CaseStats {
	cs := CaseStats{ID: id, Reps: len(rs), MinScore: 1}
	var comp float64
	for _, r := range rs {
		if r.Grade.Pass {
			cs.Passes++
		}
		cs.MeanScore += r.Grade.Score
		cs.MinScore = min(cs.MinScore, r.Grade.Score)
		for _, m := range r.Grade.Missing {
			if !slices.Contains(cs.Missing, m) {
				cs.Missing = append(cs.Missing, m)
			}
		}
		for _, f := range r.Grade.Forbidden {
			if !slices.Contains(cs.Forbidden, f) {
				cs.Forbidden = append(cs.Forbidden, f)
			}
		}
		for _, wc := range r.Grade.WrongClaims {
			if len(cs.WrongClaims) < maxWrongClaimsShown && !slices.Contains(cs.WrongClaims, wc) {
				cs.WrongClaims = append(cs.WrongClaims, wc)
			}
		}
		if r.CorpusChars > 0 {
			comp += float64(r.Grade.Chars) / float64(r.CorpusChars)
		}
	}
	if len(rs) > 0 {
		cs.MeanScore /= float64(len(rs))
		cs.Compression = comp / float64(len(rs))
	}
	return cs
}

func fillRates(st *ArmStats, byCase map[string][]Row) {
	var (
		passes, forbidden, overBudget int
		score, comp, inTok, outTok    float64
		latencies                     []int64
	)
	for _, rs := range byCase {
		for _, r := range rs {
			if r.Grade.Pass {
				passes++
			}
			if len(r.Grade.Forbidden) > 0 {
				forbidden++
			}
			if !r.Grade.BudgetOK {
				overBudget++
			}
			score += r.Grade.Score
			if r.CorpusChars > 0 {
				comp += float64(r.Grade.Chars) / float64(r.CorpusChars)
			}
			st.LineClaims += r.Grade.LineClaims
			st.LineClaimsWrong += r.Grade.LineClaimsWrong
			inTok += float64(r.PromptTokens)
			outTok += float64(r.CompletionTokens)
			st.TotalCost += r.CostUSD
			latencies = append(latencies, r.LatencyMS)
		}
	}
	n := float64(st.Rows)
	if st.Rows == 0 {
		return
	}
	st.PassRate = float64(passes) / n
	st.MeanScore = score / n
	st.ForbiddenRate = float64(forbidden) / n
	st.BudgetFailRate = float64(overBudget) / n
	st.Compression = comp / n
	st.MeanPromptTok = inTok / n
	st.MeanOutputTok = outTok / n
	st.LatencyP50 = percentile(latencies, 0.5)
}

func percentile(xs []int64, p float64) int64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := slices.Clone(xs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	i := int(p * float64(len(sorted)-1))
	return sorted[i]
}

func sortRows(rows []Row) {
	slices.SortFunc(rows, func(a, b Row) int {
		if c := strings.Compare(a.Variant, b.Variant); c != 0 {
			return c
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return a.Rep - b.Rep
	})
}
