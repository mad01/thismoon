package judge

import (
	"context"
	"math"
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

// goodReply carries confidence on the 0-100 integer scale the rubric asks
// for (MAD-348); Run publishes it as the 0-1 fraction callers consume.
const goodReply = `{"verdict":"likely_ai","confidence":88,` +
	`"signals":[{"pattern":"buzzword stacking","severity":"warning","excerpt":"robust seamless synergy"}],` +
	`"summary":"reads machine-written"}`

func TestRun(t *testing.T) {
	cases := []struct {
		name           string
		replies        []string
		wantCalls      int
		wantVerdict    string
		wantConfidence float64
		wantErrText    string
	}{
		{
			name:           "clean JSON",
			replies:        []string{goodReply},
			wantCalls:      1,
			wantVerdict:    "likely_ai",
			wantConfidence: 0.88,
		},
		{
			name:           "fenced JSON is stripped",
			replies:        []string{"```json\n" + goodReply + "\n```"},
			wantCalls:      1,
			wantVerdict:    "likely_ai",
			wantConfidence: 0.88,
		},
		{
			name:           "malformed then valid retries once",
			replies:        []string{"sure, here is my analysis:", goodReply},
			wantCalls:      2,
			wantVerdict:    "likely_ai",
			wantConfidence: 0.88,
		},
		{
			name: "a model that answers on the old 0-1 scale still parses",
			replies: []string{
				`{"verdict":"mixed","confidence":0.4,"summary":"half and half"}`,
			},
			wantCalls:      1,
			wantVerdict:    "mixed",
			wantConfidence: 0.4,
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
				`{"verdict":"mixed","confidence":420}`,
				`{"verdict":"mixed","confidence":420}`,
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
			if math.Abs(v.Confidence-tc.wantConfidence) > 1e-9 {
				t.Errorf("Confidence = %v, want %v", v.Confidence, tc.wantConfidence)
			}
			if v.Backend != "scripted" || v.Model != "test-model" {
				t.Errorf("metadata = %s/%s, want scripted/test-model", v.Backend, v.Model)
			}
		})
	}
}

func TestNormalizeConfidence(t *testing.T) {
	cases := []struct {
		name    string
		in      float64
		want    float64
		wantErr bool
	}{
		{name: "integer percent", in: 88, want: 0.88},
		{name: "low integer percent", in: 30, want: 0.30},
		{name: "top of the scale", in: 100, want: 1},
		{name: "legacy fraction", in: 0.65, want: 0.65},
		{name: "zero", in: 0, want: 0},
		{name: "one reads as certainty, not one percent", in: 1, want: 1},
		{name: "above the scale", in: 101, wantErr: true},
		{name: "below the scale", in: -1, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeConfidence(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeConfidence(%v) = %v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeConfidence(%v) error = %v", tc.in, err)
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("normalizeConfidence(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The derived-confidence rubric is what spreads the values (MAD-348): the
// integer scale, the arithmetic the model shows its work in, and the worked
// examples. Dropping any of them silently returns the judge to a constant,
// which no other test would catch.
func TestSystemPromptCarriesTheConfidenceRubric(t *testing.T) {
	want := []string{
		"integer from 0 to 100",
		"confidence_basis",
		"Start at 50",
		"Three worked examples",
	}
	for _, s := range want {
		if !strings.Contains(SystemPrompt, s) {
			t.Errorf("SystemPrompt is missing the calibration text %q", s)
		}
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
		replies: []string{`{"verdict":"likely_human","confidence":70,"summary":"fine"}`},
	}
	v, err := Run(context.Background(), b, "text")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if v.Signals == nil {
		t.Error("Signals should be an empty slice, not nil, so JSON renders [] not null")
	}
}
