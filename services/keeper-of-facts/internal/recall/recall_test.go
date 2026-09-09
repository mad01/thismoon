package recall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/store"
)

func testAssertions() []store.Assertion {
	return []store.Assertion{
		{
			ID:        "a1",
			Subject:   "repo:x/y/store",
			Statement: "serve is the single writer",
			Status:    store.StatusFresh,
		},
		{
			ID:        "a2",
			Subject:   "repo:x/y/cli",
			Statement: "flags expand tilde",
			Status:    store.StatusStale,
		},
		{
			ID:        "a3",
			Subject:   "repo:x/y/old",
			Statement: "withdrawn claim",
			Status:    store.StatusRetracted,
		},
	}
}

func fixedRunner(reply string, err error) func(context.Context, string, string) (string, error) {
	return func(context.Context, string, string) (string, error) { return reply, err }
}

func TestRankReturnsJudgedOrder(t *testing.T) {
	j := NewJudgeWithRunner(fixedRunner(`["a2","a1"]`, nil))
	got, err := j.Rank(context.Background(), "q", testAssertions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a2" || got[1].ID != "a1" {
		t.Errorf("Rank = %v, want judge order a2,a1", ids(got))
	}
}

func TestRankNeverShowsRetractedToTheJudge(t *testing.T) {
	var sawPrompt string
	j := NewJudgeWithRunner(func(_ context.Context, _, prompt string) (string, error) {
		sawPrompt = prompt
		return `[]`, nil
	})
	if _, err := j.Rank(context.Background(), "q", testAssertions()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sawPrompt, "a3") || strings.Contains(sawPrompt, "withdrawn claim") {
		t.Errorf("retracted assertion reached the judge:\n%s", sawPrompt)
	}
	if !strings.Contains(sawPrompt, "[stale]") {
		t.Errorf("stale assertion should reach the judge marked, prompt:\n%s", sawPrompt)
	}
}

func TestRankDropsInventedAndDuplicateIDs(t *testing.T) {
	j := NewJudgeWithRunner(fixedRunner(`["a1","made-up","a1","a2"]`, nil))
	got, err := j.Rank(context.Background(), "q", testAssertions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a2" {
		t.Errorf("Rank = %v, want a1,a2 with the invented and duplicate ids dropped", ids(got))
	}
}

func TestRankErrorsNameTheFallback(t *testing.T) {
	tests := []struct {
		name   string
		runner func(context.Context, string, string) (string, error)
	}{
		{name: "exec failure", runner: fixedRunner("", errors.New("boom"))},
		{name: "unparseable reply", runner: fixedRunner("the relevant ones are a1 and a2", nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := NewJudgeWithRunner(tc.runner)
			_, err := j.Rank(context.Background(), "q", testAssertions())
			if err == nil {
				t.Fatal("Rank = nil error, want a failure naming the fallback")
			}
			if !strings.Contains(err.Error(), "kof_query") {
				t.Errorf("error %q does not name the kof_query fallback", err)
			}
		})
	}
}

func TestRankEmptyStoreSkipsTheJudge(t *testing.T) {
	j := NewJudgeWithRunner(func(context.Context, string, string) (string, error) {
		t.Fatal("judge must not run on an empty store")
		return "", nil
	})
	got, err := j.Rank(context.Background(), "q", nil)
	if err != nil || got != nil {
		t.Errorf("Rank on empty store = %v, %v; want nil, nil", got, err)
	}
}

func TestParseIDsToleratesFences(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "bare array", raw: `["a","b"]`, want: []string{"a", "b"}},
		{name: "fenced", raw: "```json\n[\"a\"]\n```", want: []string{"a"}},
		{name: "fenced no lang", raw: "```\n[\"a\"]\n```", want: []string{"a"}},
		{name: "surrounding whitespace", raw: "\n  []\n", want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIDs(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("parseIDs(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseIDs(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPickCapsAtMaxResults(t *testing.T) {
	var as []store.Assertion
	var ranked []string
	for i := range MaxResults + 3 {
		id := string(rune('a' + i))
		as = append(as, store.Assertion{ID: id})
		ranked = append(ranked, id)
	}
	if got := pick(as, ranked); len(got) != MaxResults {
		t.Errorf("pick returned %d, want the cap of %d", len(got), MaxResults)
	}
}

func ids(as []store.Assertion) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}
