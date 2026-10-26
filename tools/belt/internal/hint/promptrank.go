package hint

import (
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Prompt ranking for kof-consult: which of a repo's assertions the prompt
// the user just submitted is about. kof has no free-text query a hook budget
// can afford, so belt ranks the repo's assertions itself by token overlap
// with the prompt (docs/adr/0008: relevance gating lives in the hint).
// Everything in this file is pure; kofconsult.go is the I/O shell.

const (
	// minTokenLen drops tokens too short to carry meaning (`go`, `if`, `a`).
	minTokenLen = 3
	// identifierWeight is what one overlap on an identifier-shaped prompt
	// token counts for, against 1 for a plain word. A name like
	// `seen.filter` or `toHintInput` points at specific code; a plain word
	// shared with a statement often does not.
	identifierWeight = 2
	// minPromptOverlap is the least weighted overlap an assertion needs: two
	// plain words, or one identifier, which identifierWeight counts twice.
	minPromptOverlap = 2
	// distinctiveShare is the largest share of the repo's assertions a token
	// may sit in and still name a topic. An assertion needs at least one
	// shared token at or under it, so a pair of words a fifth of the repo
	// carries (`commit` and `message`) does not admit anything on its own.
	distinctiveShare = 0.2
	// minScoreShare keeps the candidates scoring at least this fraction of
	// the best one, so a block holds the prompt's topic and not its tail.
	// Measured on a 254-assertion repo: at 0.4 a prompt about em dashes in
	// a commit message still drew three statements sharing only `commit`
	// and `message`; at 0.6 it draws the two about em dashes.
	minScoreShare = 0.6
	// maxQueryTokens caps the distinct prompt tokens ranked against. A
	// prompt past it is a paste (a file, a log); its opening says what it is,
	// and ranking all of it would let it overlap every assertion.
	maxQueryTokens = 200
)

// urlTokens come with any pasted URL and name no topic.
var urlTokens = []string{"github", "com", "https", "http", "www"}

// promptStopwords are words too common in prompts and statements for an
// overlap on them to say anything, plus path segments most Go repos share.
// Words under minTokenLen never reach the list.
var promptStopwords = func() map[string]bool {
	words := []string{
		"about", "above", "after", "again", "all", "already", "also", "always",
		"and", "another", "any", "are", "aren", "been", "before", "being", "both",
		"but", "can", "could", "couldn", "did", "didn", "does", "doesn", "doing",
		"don", "done", "each", "either", "even", "ever", "every", "for", "from",
		"get", "got", "had", "has", "have", "her", "here", "him", "his", "how",
		"into", "isn", "its", "just", "know", "let", "like", "look", "make",
		"many", "may", "more", "most", "much", "need", "never", "new", "not",
		"now", "okay", "one", "only", "onto", "other", "our", "out", "over",
		"own", "per", "please", "same", "say", "see", "seem", "she", "should",
		"shouldn", "some", "still", "such", "sure", "than", "thank", "thanks",
		"that", "the", "their", "them", "then", "there", "these", "they", "this",
		"those", "too", "two", "under", "upon", "use", "used", "uses", "using",
		"very", "via", "want", "was", "wasn", "way", "were", "what", "when",
		"where", "which", "while", "who", "whom", "why", "will", "with", "won",
		"would", "wouldn", "yes", "yet", "you", "your",
		// path segments that sit in most pin paths of a Go repo
		"cmd", "internal", "pkg", "src",
	}
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}()

// queryToken is one distinct token of the prompt.
type queryToken struct {
	text  string
	ident bool // some word it came from was identifier-shaped
}

// noiseFor is the set of tokens that say nothing within one repo's ranking:
// the repo's own org and name, which every assertion and many statements
// carry, and the URL tokens a link brings in.
func noiseFor(repo string) map[string]bool {
	noise := map[string]bool{}
	for _, t := range urlTokens {
		noise[t] = true
	}
	for _, t := range strings.FieldsFunc(strings.ToLower(repo), isNotAlnum) {
		noise[t] = true
	}
	return noise
}

// promptTokens returns the prompt's first maxQueryTokens distinct tokens,
// sorted so scores sum in the same order on every run. It stops reading at
// the cap, so a pasted file costs no more than its opening.
func promptTokens(prompt string, noise map[string]bool) []queryToken {
	ident := map[string]bool{}
	for word := range strings.FieldsSeq(prompt) {
		word = strings.TrimFunc(word, isNotAlnum)
		// A link is not code the user named: its path segments (`pull`,
		// `blob`) count as plain words.
		shaped := identifierShaped(word) && !strings.Contains(word, "://")
		for _, tok := range tokens(word, noise) {
			if _, ok := ident[tok]; !ok && len(ident) == maxQueryTokens {
				continue
			}
			ident[tok] = ident[tok] || shaped
		}
		if len(ident) == maxQueryTokens {
			break
		}
	}
	out := make([]queryToken, 0, len(ident))
	for t, shaped := range ident {
		out = append(out, queryToken{text: t, ident: shaped})
	}
	slices.SortFunc(out, func(a, b queryToken) int { return strings.Compare(a.text, b.text) })
	return out
}

// identifierShaped reports whether a word, already stripped of surrounding
// punctuation, reads as a code name: it holds `_`, `/` or `.` (a path, a
// file, a qualified name) or a lower-to-upper case change (camelCase).
func identifierShaped(word string) bool {
	if strings.ContainsAny(word, "_/.") {
		return true
	}
	prevLower := false
	for _, r := range word {
		if prevLower && unicode.IsUpper(r) {
			return true
		}
		prevLower = unicode.IsLower(r)
	}
	return false
}

// tokens lowercases text and splits it on anything that is not a letter or
// digit, keeping tokens of minTokenLen runes or more that are neither
// stopwords nor noise.
func tokens(text string, noise map[string]bool) []string {
	var out []string
	for _, t := range strings.FieldsFunc(strings.ToLower(text), isNotAlnum) {
		if utf8.RuneCountInString(t) >= minTokenLen && !promptStopwords[t] && !noise[t] {
			out = append(out, t)
		}
	}
	return out
}

func isNotAlnum(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// assertionTokens is the token set an assertion is matched on: its
// statement, its subject below the repo, and its pins' file paths.
func assertionTokens(repo string, a assertion, noise map[string]bool) map[string]bool {
	set := map[string]bool{}
	add := func(text string) {
		for _, t := range tokens(text, noise) {
			set[t] = true
		}
	}
	add(a.Statement)
	add(strings.TrimPrefix(strings.ToLower(a.Subject), "repo:"+repo))
	for _, p := range a.Pins {
		add(p.File)
	}
	return set
}

// scored is one assertion that passed the gate, with its score.
type scored struct {
	a     assertion
	score float64
}

// rankByPrompt returns the assertions the prompt is about, best first. An
// assertion passes when its weighted overlap with the prompt reaches
// minPromptOverlap and at least one shared token is distinctive (see
// distinctiveShare). Its score sums each shared token's weight times the
// token's inverse frequency across as. Candidates under minScoreShare of
// the best score are dropped. Equal scores put fresh before stale: a prompt,
// unlike a search, is not reading the code a stale pin moved away from.
//
// The score is not divided by statement length. maxQueryTokens already
// bounds what a paste can match, and on measured prompts a length divisor
// lifted short statements that shared only generic words. It does not cap:
// the caller dedupes against the session first and cuts after (seen.take).
func rankByPrompt(prompt, repo string, as []assertion) []assertion {
	noise := noiseFor(repo)
	query := promptTokens(prompt, noise)
	if len(query) == 0 || len(as) == 0 {
		return nil
	}
	docs := make([]map[string]bool, len(as))
	for i, a := range as {
		docs[i] = assertionTokens(repo, a, noise)
	}
	df := documentFrequency(query, docs)
	ceiling := max(1, distinctiveShare*float64(len(as)))
	var out []scored
	for i, a := range as {
		if score, ok := scoreAssertion(query, docs[i], df, len(as), ceiling); ok {
			out = append(out, scored{a: a, score: score})
		}
	}
	return orderAndCut(out)
}

// documentFrequency counts, per query token, the assertions that carry it.
func documentFrequency(query []queryToken, docs []map[string]bool) map[string]int {
	df := map[string]int{}
	for _, doc := range docs {
		for _, q := range query {
			if doc[q.text] {
				df[q.text]++
			}
		}
	}
	return df
}

// scoreAssertion scores one assertion's token set against the query and
// reports whether it passes the gate (see rankByPrompt).
func scoreAssertion(
	query []queryToken,
	doc map[string]bool,
	df map[string]int,
	n int,
	ceiling float64,
) (float64, bool) {
	overlap, score, distinctive := 0, 0.0, false
	for _, q := range query {
		if !doc[q.text] {
			continue
		}
		w := 1
		if q.ident {
			w = identifierWeight
		}
		overlap += w
		score += float64(w) * math.Log(1+float64(n)/float64(df[q.text]))
		distinctive = distinctive || float64(df[q.text]) <= ceiling
	}
	if overlap < minPromptOverlap || !distinctive {
		return 0, false
	}
	return score, true
}

// orderAndCut sorts candidates best first, fresh before stale on equal
// scores and kof's order after that, then drops those under minScoreShare
// of the best score.
func orderAndCut(out []scored) []assertion {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].a.Status != "stale" && out[j].a.Status == "stale"
	})
	var res []assertion
	for _, s := range out {
		if s.score < minScoreShare*out[0].score {
			break
		}
		res = append(res, s.a)
	}
	return res
}
