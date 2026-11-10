package timespan

import (
	"strings"
	"testing"
	"time"
)

func TestSince(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		raw     string
		want    time.Time
		wantErr bool
	}{
		{raw: "", want: time.Time{}},
		{raw: "2026-10-24T08:00:00Z", want: time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)},
		{raw: "90m", want: now.Add(-90 * time.Minute)},
		{raw: "24h", want: now.Add(-24 * time.Hour)},
		{raw: "7d", want: now.Add(-7 * 24 * time.Hour)},
		{raw: "0s", wantErr: true},
		{raw: "-1h", wantErr: true},
		{raw: "xd", wantErr: true},
		{raw: "last week", wantErr: true},
		{raw: "01761393600000000000-ab12", wantErr: true}, // an events id cursor
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := Since(tc.raw, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Since(%q) = %v, want an error", tc.raw, got)
				}
				if !strings.HasPrefix(err.Error(), "must be ") ||
					!strings.Contains(err.Error(), tc.raw) {
					t.Errorf("Since(%q) error = %q, want a must-be message naming the value",
						tc.raw, err)
				}
				return
			}
			if err != nil || !got.Equal(tc.want) {
				t.Errorf("Since(%q) = %v, %v; want %v", tc.raw, got, err, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{raw: "14d", want: 14 * 24 * time.Hour},
		{raw: "336h", want: 336 * time.Hour},
		{raw: "45m", want: 45 * time.Minute},
		{raw: "", wantErr: true},
		{raw: "0d", wantErr: true},
		{raw: "-2d", wantErr: true},
		{raw: "2026-10-24T08:00:00Z", wantErr: true},
		{raw: "fortnight", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := Parse(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %v, want an error", tc.raw, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("Parse(%q) = %v, %v; want %v", tc.raw, got, err, tc.want)
			}
		})
	}
}
