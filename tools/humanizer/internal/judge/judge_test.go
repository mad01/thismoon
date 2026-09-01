package judge

import (
	"context"
	"strings"
	"testing"
)

// scriptedBackend replays canned replies, one per Complete call.
type scriptedBackend struct {
	replies []string
	calls   int
}

func (s *scriptedBackend) Complete(context.Context, string, string) (string, error) {
	if s.calls >= len(s.replies) {
		return "", context.Canceled
	}
	reply := s.replies[s.calls]
	s.calls++
	return reply, nil
}

func (s *scriptedBackend) Name() string  { return "scripted" }
func (s *scriptedBackend) Model() string { return "test-model" }

const goodReply = `{"verdict":"likely_ai","confidence":0.9,` +
	`"signals":[{"pattern":"buzzword stacking","severity":"warning","excerpt":"robust seamless synergy"}],` +
	`"summary":"reads machine-written"}`

func TestRun(t *testing.T) {
	cases := []struct {
		name        string
		replies     []string
		wantCalls   int
		wantVerdict string
		wantErrText string
	}{
		{
			name:        "clean JSON",
			replies:     []string{goodReply},
			wantCalls:   1,
			wantVerdict: "likely_ai",
		},
		{
			name:        "fenced JSON is stripped",
			replies:     []string{"```json\n" + goodReply + "\n```"},
			wantCalls:   1,
			wantVerdict: "likely_ai",
		},
		{
			name:        "malformed then valid retries once",
			replies:     []string{"sure, here is my analysis:", goodReply},
			wantCalls:   2,
			wantVerdict: "likely_ai",
		},
		{
			name:        "malformed twice fails",
			replies:     []string{"nope", "still nope"},
			wantCalls:   2,
			wantErrText: "twice",
		},
		{
			name:        "invalid verdict value counts as contract break",
			replies:     []string{`{"verdict":"unsure"}`, `{"verdict":"unsure"}`},
			wantCalls:   2,
			wantErrText: "twice",
		},
		{
			name: "out-of-range confidence counts as contract break",
			replies: []string{
				`{"verdict":"mixed","confidence":42}`,
				`{"verdict":"mixed","confidence":42}`,
			},
			wantCalls:   2,
			wantErrText: "twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &scriptedBackend{replies: tc.replies}
			v, err := Run(context.Background(), b, "some passage")
			if b.calls != tc.wantCalls {
				t.Errorf("Complete calls = %d, want %d", b.calls, tc.wantCalls)
			}
			if tc.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("Run() error = %v, want containing %q", err, tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if v.Verdict != tc.wantVerdict {
				t.Errorf("Verdict = %q, want %q", v.Verdict, tc.wantVerdict)
			}
			if v.Backend != "scripted" || v.Model != "test-model" {
				t.Errorf("metadata = %s/%s, want scripted/test-model", v.Backend, v.Model)
			}
		})
	}
}

func TestRunEmptyText(t *testing.T) {
	b := &scriptedBackend{replies: []string{goodReply}}
	if _, err := Run(context.Background(), b, "  \n"); err == nil {
		t.Fatal("Run() with empty text should fail")
	}
	if b.calls != 0 {
		t.Errorf("Complete calls = %d, want 0 (no model call for empty input)", b.calls)
	}
}

func TestParseNormalizesNilSignals(t *testing.T) {
	b := &scriptedBackend{
		replies: []string{`{"verdict":"likely_human","confidence":0.8,"summary":"fine"}`},
	}
	v, err := Run(context.Background(), b, "text")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if v.Signals == nil {
		t.Error("Signals should be an empty slice, not nil, so JSON renders [] not null")
	}
}
