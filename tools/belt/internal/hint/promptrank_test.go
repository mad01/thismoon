package hint

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestPromptTokens(t *testing.T) {
	got := promptTokens(
		"Fix `seen.filter` in kofconsult.go, then toHintInput and the HTTP API!",
		nil,
	)
	want := []queryToken{
		{text: "api"},
		{text: "filter", ident: true},
		{text: "fix"},
		{text: "http"},
		{text: "kofconsult", ident: true},
		{text: "seen", ident: true},
		{text: "tohintinput", ident: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("promptTokens = %+v,\nwant %+v", got, want)
	}
}

// A token is identifier-shaped when any word it came from was, so a name
// written once as code and once as prose still counts as a name.
func TestPromptTokensKeepIdentifierShapeFromAnyWord(t *testing.T) {
	got := promptTokens("the ledger is odd; see internal/ledger", nil)
	if !slices.Contains(got, queryToken{text: "ledger", ident: true}) {
		t.Errorf("promptTokens = %+v, want ledger marked identifier-shaped", got)
	}
}

func TestIdentifierShaped(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"seen.filter", true},
		{"tools/belt", true},
		{"exclude_repos", true},
		{"toHintInput", true},
		{"HintFields", true},
		{"kofconsult.go", true},
		{"ledger", false},
		{"API", false},
		{"Ledger", false},
		{"kof-consult", false}, // hyphens are prose too ("well-known")
	}
	for _, tt := range tests {
		if got := identifierShaped(tt.word); got != tt.want {
			t.Errorf("identifierShaped(%q) = %v, want %v", tt.word, got, tt.want)
		}
	}
}

func TestRankByPromptGate(t *testing.T) {
	as := []assertion{{
		ID:      "x",
		Subject: "repo:mad01/thismoon/tools/belt",
		Statement: "the mad01/thismoon ledger posts lifecycle entries with the session " +
			"tags (https://github.com/mad01/thismoon/pull/7)",
		Pins: []pin{{File: "tools/belt/internal/ledger/post.go"}},
	}}
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"one plain overlap", "why are lifecycle hooks slow today", false},
		{"two plain overlaps", "check the ledger lifecycle wiring", true},
		{"one identifier overlap", "open ledger.go and explain it", true},
		{"identifier the assertion lacks", "what does postLifecycle do in the code", false},
		{"overlap through the pin path", "what calls post.go from cli", true},
		{"stopwords only", "what about all of this and that", false},
		{"repo name only", "work in mad01/thismoon please, thanks", false},
		{"a link to the repo", "review https://github.com/mad01/thismoon/pull/512 please", false},
		{"path segment shared by most repos", "move cmd/internal/pkg around", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := len(rankByPrompt(tt.prompt, "mad01/thismoon", as)) == 1
			if got != tt.want {
				t.Errorf("rankByPrompt(%q) kept = %v, want %v", tt.prompt, got, tt.want)
			}
		})
	}
}

// TestRankByPromptIdentifierWeight: the same word passes the gate on its
// own when written as code and does not when written as prose.
func TestRankByPromptIdentifierWeight(t *testing.T) {
	as := []assertion{{ID: "x", Subject: "repo:o/r", Statement: "seenfilter dedupes per session"}}
	if got := rankByPrompt("check what seenFilter does now", "o/r", as); len(got) != 1 {
		t.Error("a camelCase name matching one statement word should pass the gate")
	}
	if got := rankByPrompt("check what seenfilter does now", "o/r", as); len(got) != 0 {
		t.Error("the same word as prose is one plain overlap and should not pass")
	}
}

// claim is an assertion fixture in repo o/r.
func claim(id, status, statement string) assertion {
	return assertion{ID: id, Subject: "repo:o/r", Statement: statement, Status: status}
}

// withFiller pads as with unrelated assertions to n, so the share of the set
// a fixture word sits in is what the test says it is.
func withFiller(n int, as ...assertion) []assertion {
	for i := len(as); i < n; i++ {
		as = append(
			as,
			claim(fmt.Sprintf("filler-%d", i), "fresh", fmt.Sprintf("unrelated%d claim", i)),
		)
	}
	return as
}

func ids(as []assertion) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

// TestRankByPromptNeedsADistinctiveToken: two words that half the repo's
// assertions carry overlap enough to pass the count, but name no topic, so
// they admit nothing. The assertion sharing rare words is the one kept.
func TestRankByPromptNeedsADistinctiveToken(t *testing.T) {
	as := withFiller(10,
		claim("common-1", "fresh", "commit message rules for guards"),
		claim("common-2", "fresh", "commit message lint runs late"),
		claim("common-3", "fresh", "commit message bodies stay short"),
		claim("common-4", "fresh", "commit message subjects are checked"),
		claim("common-5", "fresh", "commit message trailers are dropped"),
		claim("rare", "fresh", "humanizer flags dashes in prose"),
	)
	got := ids(rankByPrompt("why does the humanizer flag dashes in my commit message", "o/r", as))
	if !slices.Equal(got, []string{"rare"}) {
		t.Errorf("rankByPrompt kept %v, want only the assertion sharing rare words", got)
	}
}

// TestRankByPromptDropsTheTail: a candidate that passes the gate but scores
// under minScoreShare of the best is the prompt's tail, not its topic.
func TestRankByPromptDropsTheTail(t *testing.T) {
	as := withFiller(10,
		claim("top", "fresh", "watcher cursor survives restarts"),
		claim("tail", "fresh", "ledger entries post late"),
		claim("tail-twin", "fresh", "ledger entries carry tags"),
	)
	got := ids(
		rankByPrompt("why the watcher cursor survives restarts, like ledger entries", "o/r", as),
	)
	if !slices.Equal(got, []string{"top"}) {
		t.Errorf("rankByPrompt kept %v, want only the top candidate", got)
	}
}

// TestRankByPromptPutsFreshBeforeStaleOnTies: equal scores order fresh
// first, against kof's order; a stale assertion that scores higher keeps
// its place.
func TestRankByPromptPutsFreshBeforeStaleOnTies(t *testing.T) {
	as := withFiller(20,
		claim("stale-tie", "stale", "watcher cursor survives restarts"),
		claim("fresh-tie", "fresh", "watcher cursor survives restarts"),
		claim("stale-best", "stale", "watcher cursor survives restarts quietly"),
	)
	got := ids(rankByPrompt("why the watcher cursor survives restarts quietly", "o/r", as))
	if want := []string{"stale-best", "fresh-tie", "stale-tie"}; !slices.Equal(got, want) {
		t.Errorf("rankByPrompt order = %v, want %v", got, want)
	}
}

// TestRankByPromptIgnoresTheRepoAndLinks: the repo's own org and name and a
// link's host tokens sit in many statements and name no topic, and a link's
// path segments are plain words, not names written as code.
func TestRankByPromptIgnoresTheRepoAndLinks(t *testing.T) {
	as := withFiller(10,
		claim("repo", "fresh", "mad01/thismoon ships from github.com"),
		claim("link", "fresh", "see https://github.com/mad01/thismoon/pull/9 for the change"),
	)
	for _, prompt := range []string{
		"review https://github.com/mad01/thismoon/pull/512 please",
		"what ships in mad01/thismoon from www.github.com",
	} {
		if got := rankByPrompt(prompt, "mad01/thismoon", as); len(got) != 0 {
			t.Errorf("rankByPrompt(%q) kept %v, want nothing", prompt, ids(got))
		}
	}
	got := promptTokens(
		"see https://github.com/mad01/thismoon/pull/512",
		noiseFor("mad01/thismoon"),
	)
	if want := []queryToken{{text: "512"}, {text: "pull"}}; !slices.Equal(got, want) {
		t.Errorf("promptTokens on a link = %+v, want %+v", got, want)
	}
}

// TestRankByPromptHoldsOnALongPaste: a pasted file is read up to
// maxQueryTokens distinct tokens and no further, so its opening decides
// what it matches. Every fixture word sits deep in the paste, and only the
// assertion its opening names is kept.
func TestRankByPromptHoldsOnALongPaste(t *testing.T) {
	var as []assertion
	var late strings.Builder
	for i := range 20 {
		word := fmt.Sprintf("topic%02d", i)
		as = append(as, claim(word, "fresh", word+" handles "+word+"Store.Load"))
		fmt.Fprintf(&late, "func %s() { %sStore.Load() }\n", word, word)
	}
	var paste strings.Builder
	paste.WriteString("// topic00 handles topic00Store.Load\n")
	for i := range 2 * maxQueryTokens {
		fmt.Fprintf(&paste, "x := fill.Value%04d(ctx)\n", i)
	}
	for range 500 { // about 1 MB of code after the cap
		paste.WriteString(late.String())
	}
	if n := len(promptTokens(paste.String(), nil)); n != maxQueryTokens {
		t.Errorf("promptTokens read %d distinct tokens, want the cap of %d", n, maxQueryTokens)
	}
	got := ids(rankByPrompt(paste.String(), "o/r", as))
	if !slices.Equal(got, []string{"topic00"}) {
		t.Errorf("rankByPrompt on a long paste kept %v, want only topic00", got)
	}
}

func TestRankByPromptEmpty(t *testing.T) {
	as := []assertion{{ID: "x", Subject: "repo:o/r", Statement: "ledger lifecycle"}}
	if got := rankByPrompt("the and of it", "o/r", as); got != nil {
		t.Errorf("a prompt with no tokens ranked %v, want nil", got)
	}
	if got := rankByPrompt("ledger lifecycle", "o/r", nil); got != nil {
		t.Errorf("an empty assertion set ranked %v, want nil", got)
	}
}
