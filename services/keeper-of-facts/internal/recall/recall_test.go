package recall

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/store"
)

// fakeLister stands in for the serve list API. It records whether recall
// fetched, and with which filters.
type fakeLister struct {
	as      []client.Assertion
	err     error
	called  bool
	filters []string
}

func (f *fakeLister) List(subject, kind, status string) ([]client.Assertion, error) {
	f.called = true
	f.filters = []string{subject, kind, status}
	return f.as, f.err
}

func testAssertions() []client.Assertion {
	return []client.Assertion{
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

// failRunner fails the test if the judge runs at all.
func failRunner(t *testing.T) func(context.Context, string, string) (string, error) {
	return func(context.Context, string, string) (string, error) {
		t.Error("judge ran, want it skipped")
		return "", nil
	}
}

func TestRecallRanks(t *testing.T) {
	tests := []struct {
		name  string
		store []client.Assertion
		reply string
		want  []string
	}{
		{
			name:  "judge order",
			store: testAssertions(),
			reply: `["a2","a1"]`,
			want:  []string{"a2", "a1"},
		},
		{
			name:  "invented and duplicate ids dropped",
			store: testAssertions(),
			reply: `["a1","made-up","a1","a2"]`,
			want:  []string{"a1", "a2"},
		},
		{
			name:  "retracted id named by the judge is not returned",
			store: testAssertions(),
			reply: `["a3","a1"]`,
			want:  []string{"a1"},
		},
		{name: "empty answer", store: testAssertions(), reply: "[]", want: nil},
		{
			name:  "fenced reply",
			store: testAssertions(),
			reply: "```json\n[\"a1\"]\n```",
			want:  []string{"a1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeLister{as: tc.store}
			got, err := NewJudgeWithRunner(fixedRunner(tc.reply, nil)).
				Recall(context.Background(), src, "q")
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(ids(got), tc.want) {
				t.Errorf("Recall = %v, want %v", ids(got), tc.want)
			}
			if !slices.Equal(src.filters, []string{"", "", ""}) {
				t.Errorf("List filters = %q, want the unfiltered store", src.filters)
			}
		})
	}
}

func TestRecallSkipsTheJudgeWithNothingToRank(t *testing.T) {
	tests := []struct {
		name  string
		store []client.Assertion
	}{
		{name: "empty store", store: nil},
		{name: "only retracted", store: testAssertions()[2:]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewJudgeWithRunner(failRunner(t)).
				Recall(context.Background(), &fakeLister{as: tc.store}, "q")
			if err != nil || got != nil {
				t.Errorf("Recall = %v, %v; want nil, nil", got, err)
			}
		})
	}
}

func TestRecallErrors(t *testing.T) {
	listErr := errors.New("kof serve not reachable")
	tests := []struct {
		name      string
		question  string
		listErr   error
		runner    func(context.Context, string, string) (string, error)
		wantIs    error
		wantText  string
		wantFetch bool
	}{
		{
			name:     "empty question",
			question: "  ",
			wantIs:   ErrNoQuestion,
		},
		{
			name:      "serve unreachable",
			question:  "q",
			listErr:   listErr,
			wantIs:    listErr,
			wantFetch: true,
		},
		{
			name:      "judge exec failure",
			question:  "q",
			runner:    fixedRunner("", errors.New("boom")),
			wantText:  "kof_query",
			wantFetch: true,
		},
		{
			name:      "unparseable reply",
			question:  "q",
			runner:    fixedRunner("the relevant ones are a1 and a2", nil),
			wantText:  "kof_query",
			wantFetch: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := tc.runner
			if runner == nil {
				runner = failRunner(t)
			}
			src := &fakeLister{as: testAssertions(), err: tc.listErr}
			_, err := NewJudgeWithRunner(runner).Recall(context.Background(), src, tc.question)
			if err == nil {
				t.Fatal("Recall = nil error, want a failure")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("Recall error %q, want it to wrap %q", err, tc.wantIs)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("Recall error %q does not mention %q", err, tc.wantText)
			}
			if src.called != tc.wantFetch {
				t.Errorf("List called = %v, want %v", src.called, tc.wantFetch)
			}
		})
	}
}

func TestRecallNeverShowsRetractedToTheJudge(t *testing.T) {
	var sawPrompt string
	j := NewJudgeWithRunner(func(_ context.Context, _, prompt string) (string, error) {
		sawPrompt = prompt
		return `[]`, nil
	})
	if _, err := j.Recall(context.Background(), &fakeLister{as: testAssertions()}, "q"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sawPrompt, "a3") || strings.Contains(sawPrompt, "withdrawn claim") {
		t.Errorf("retracted assertion reached the judge:\n%s", sawPrompt)
	}
	if !strings.Contains(sawPrompt, "[stale]") {
		t.Errorf("stale assertion should reach the judge marked, prompt:\n%s", sawPrompt)
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
			if !slices.Equal(got, tc.want) {
				t.Errorf("parseIDs(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestPickCapsAtMaxResults(t *testing.T) {
	var as []client.Assertion
	var ranked []string
	for i := range MaxResults + 3 {
		id := string(rune('a' + i))
		as = append(as, client.Assertion{ID: id})
		ranked = append(ranked, id)
	}
	if got := pick(as, ranked); len(got) != MaxResults {
		t.Errorf("pick returned %d, want the cap of %d", len(got), MaxResults)
	}
}

func TestJudgeEnvDropsSessionLinks(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "session links dropped",
			in: []string{
				"CLAUDECODE=1",
				"CLAUDE_CODE_SESSION_ID=s",
				"CLAUDE_CODE_MESSAGING_SOCKET=/tmp/x.sock",
				"CLAUDE_CODE_MESSAGING_TOKEN=t",
				"CLAUDE_PROJECT_DIR=/repo",
			},
			want: []string{},
		},
		{
			name: "credentials and settings kept",
			in:   []string{"PATH=/bin", "HOME=/h", "ANTHROPIC_API_KEY=k", "CLAUDE_CONFIG_DIR=/c"},
			want: []string{"PATH=/bin", "HOME=/h", "ANTHROPIC_API_KEY=k", "CLAUDE_CONFIG_DIR=/c"},
		},
		{
			name: "match is on the whole name",
			in:   []string{"CLAUDECODE_EXTRA=1", "MY_CLAUDECODE=1", "CLAUDECODE=1"},
			want: []string{"CLAUDECODE_EXTRA=1", "MY_CLAUDECODE=1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := judgeEnv(tc.in); !slices.Equal(got, tc.want) {
				t.Errorf("judgeEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func ids(as []client.Assertion) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}
