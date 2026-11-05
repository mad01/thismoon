package lifecycle

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

var day = time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)

func parseFM(t *testing.T, yml string) markdown.Frontmatter {
	t.Helper()
	fm, _, err := markdown.ParseFrontmatter([]byte("---\n" + yml + "---\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return fm
}

func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name, yml   string
		key, want   string
		absentKey   string
		wantDefault bool
	}{
		{
			name: "raw expires in 30 days",
			yml:  "status: raw\n",
			key:  KeyExpiresAt,
			want: "2026-12-05",
		},
		{
			name: "validated low by default",
			yml:  "status: validated\n",
			key:  KeyReviewBy,
			want: "2027-11-05",
		},
		{
			name: "validated high volatility",
			yml:  "status: validated\nvolatility: high\n",
			key:  KeyReviewBy, want: "2027-02-03",
		},
		{
			name: "explicit review_by kept",
			yml:  "status: validated\nreview_by: 2026-12-01\n",
			key:  KeyReviewBy, want: "2026-12-01",
		},
		{name: "raw gets no review_by", yml: "status: raw\n", key: KeyReviewBy, want: ""},
		{name: "disputed gets nothing", yml: "status: disputed\n", key: KeyExpiresAt, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := parseFM(t, tt.yml)
			ApplyDefaults(&fm, day)
			if got := fieldValue(Read(fm), tt.key); got != tt.want {
				t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestTransitionLeavingRaw(t *testing.T) {
	fm := parseFM(t, "status: raw\nexpires_at: 2026-12-01\n")
	fm.Status = Validated
	Transition(&fm, Raw, day)
	f := Read(fm)
	if f.ExpiresAt != "" {
		t.Errorf("expires_at = %q, want it dropped", f.ExpiresAt)
	}
	if f.StatusAt != "2026-11-05" || f.ReviewBy != "2027-11-05" {
		t.Errorf(
			"status_at %q review_by %q, want 2026-11-05 and 2027-11-05",
			f.StatusAt,
			f.ReviewBy,
		)
	}
	out, err := markdown.Render(fm, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "status_at: 2026-11-05\n") {
		t.Errorf("dates should render unquoted:\n%s", out)
	}
}

func TestTransitionSameStatusIsNoop(t *testing.T) {
	fm := parseFM(t, "status: validated\n")
	Transition(&fm, "Validated", day)
	if f := Read(fm); f.StatusAt != "" || f.ReviewBy != "" {
		t.Errorf("no change expected, got status_at %q review_by %q", f.StatusAt, f.ReviewBy)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name, yml string
		wantKeys  []string
	}{
		{name: "clean legacy note", yml: "title: x\nstatus: validated\nowner: alex\n"},
		{
			name: "clean full note",
			yml: "status: validated\nreview_by: 2027-01-01\nvolatility: low\n" +
				"classification: internal\ncaptured_by: agent:abc\ndistinct_from: [id:01J]\n" +
				"sources:\n  - https://example.com\n  - url: https://example.org\n" +
				"    content_hash: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n" +
				"    ref: thismoon:services/brain/facts.go@0cfaa7a2#Facts\n",
		},
		{name: "stored expired", yml: "status: expired\n", wantKeys: []string{KeyStatus}},
		{name: "bad date", yml: "review_by: next year\n", wantKeys: []string{KeyReviewBy}},
		{
			name:     "bad enum",
			yml:      "volatility: medium\nclassification: secret\n",
			wantKeys: []string{KeyClassification, KeyVolatility},
		},
		{
			name:     "bad captured_by",
			yml:      "captured_by: \"agent:\"\n",
			wantKeys: []string{KeyCapturedBy},
		},
		{name: "list for a scalar", yml: "steward: [a, b]\n", wantKeys: []string{KeySteward}},
		{name: "sources not a list", yml: "sources: https://x\n", wantKeys: []string{KeySources}},
		{
			name:     "bad source ref and hash",
			yml:      "sources:\n  - ref: just-a-path\n    content_hash: nothex\n",
			wantKeys: []string{"sources[0].content_hash", "sources[0].ref"},
		},
		{
			name: "note refs as sources",
			yml: "sources:\n  - ref: id:01JC0000000000000000000000\n" +
				"  - ref: team:notes/x.md\n  - ref: notes/y.md\n",
		},
		{
			name:     "a bad id ref",
			yml:      "sources:\n  - ref: id:nope\n",
			wantKeys: []string{"sources[0].ref"},
		},
		{
			name:     "validity window reversed",
			yml:      "valid_from: 2026-02-01\ninvalid_at: 2026-01-01\n",
			wantKeys: []string{KeyInvalidAt},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keys []string
			for _, p := range Validate(parseFM(t, tt.yml)) {
				keys = append(keys, p.Key)
			}
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("problem keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}

func TestRequired(t *testing.T) {
	tests := []struct {
		name, yml, body string
		wantKeys        []string
	}{
		{name: "validated needs nothing", yml: "status: validated\n"},
		{
			name:     "superseded needs superseded_by",
			yml:      "status: superseded\n",
			wantKeys: []string{KeySupersededBy},
		},
		{
			name:     "retracted needs reason and date",
			yml:      "status: retracted\n",
			wantKeys: []string{KeyRetractedReason, KeyRetractedAt},
		},
		{name: "tombstone", yml: "id: 01J\nstatus: withdrawn\nstatus_at: 2026-11-05\n"},
		{
			name:     "withdrawn with leftovers",
			yml:      "id: 01J\ntitle: x\nstatus: withdrawn\n",
			body:     "still here\n",
			wantKeys: []string{"title", "body"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keys []string
			for _, p := range Required(parseFM(t, tt.yml), []byte(tt.body)) {
				keys = append(keys, p.Key)
			}
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("problem keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}

func TestRevocationsSatisfyRequired(t *testing.T) {
	sup := parseFM(t, "title: x\nstatus: validated\n")
	Supersede(&sup, "id:01JREPLACEMENT", day)
	ret := parseFM(t, "title: x\nstatus: validated\n")
	if err := Retract(&ret, ReasonUnverifiable, "source vanished", day); err != nil {
		t.Fatal(err)
	}
	for name, fm := range map[string]markdown.Frontmatter{
		"supersede": sup, "retract": ret, "withdraw": Tombstone("01J", day),
	} {
		if p := Required(fm, nil); len(p) > 0 {
			t.Errorf("%s leaves %v", name, p)
		}
		if p := Validate(fm); len(p) > 0 {
			t.Errorf("%s writes invalid fields %v", name, p)
		}
	}
	if err := Retract(&ret, "boredom", "", day); err == nil {
		t.Error("Retract accepted an unknown reason")
	}
}

func TestBanner(t *testing.T) {
	ret := parseFM(t, "status: validated\n")
	_ = Retract(&ret, ReasonError, "the benchmark was misread", day)
	want := "RETRACTED on 2026-11-05 (error): the benchmark was misread. Do not cite this note."
	if got := Banner(Read(ret)); got != want {
		t.Errorf("Banner = %q, want %q", got, want)
	}
	if got := Banner(Read(parseFM(t, "status: validated\n"))); got != "" {
		t.Errorf("validated note banner = %q, want none", got)
	}
}

func TestStanding(t *testing.T) {
	past := Read(parseFM(t, "review_by: 2026-11-04\n"))
	today := Read(parseFM(t, "review_by: 2026-11-05\n"))
	tests := []struct {
		status string
		f      Fields
		want   string
	}{
		{Validated, Fields{}, ""},
		{Validated, today, ""},
		{Validated, past, Expired},
		{"Retracted", Fields{}, Retracted},
		{Raw, Fields{}, Raw},
		{Raw, Fields{ExpiresAt: "2026-11-05"}, Raw},
		{Raw, Fields{ExpiresAt: "2026-11-04"}, Expired},
	}
	for _, tt := range tests {
		if got := Standing(tt.status, tt.f, day); got != tt.want {
			t.Errorf("Standing(%q, %+v) = %q, want %q", tt.status, tt.f, got, tt.want)
		}
	}
	if !Revoked("Superseded") || Revoked(Disputed) {
		t.Error("Revoked misclassifies superseded or disputed")
	}
}

func fieldValue(f Fields, key string) string {
	switch key {
	case KeyExpiresAt:
		return f.ExpiresAt
	case KeyReviewBy:
		return f.ReviewBy
	}
	return stringValue(f, key)
}
