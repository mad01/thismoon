package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/procstat"
)

func TestFormatTopFrame(t *testing.T) {
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, time.UTC)
	rows := []topRow{
		{
			name: "small",
			stats: &procstat.Stats{
				PID:        10,
				RSSBytes:   16 << 20,
				CPUPercent: 0.0,
				Uptime:     90 * time.Second,
			},
		},
		{name: "stopped-svc"},
		{
			name: "big",
			stats: &procstat.Stats{
				PID:        20,
				RSSBytes:   64 << 20,
				CPUPercent: 1.5,
				Uptime:     2 * time.Hour,
			},
		},
	}

	frame := formatTopFrame(rows, 2*time.Second, now)

	for _, want := range []string{"2/3 running", "total RSS 80.0M", "15:04:05", "refresh 2s"} {
		if !strings.Contains(frame, want) {
			t.Errorf("formatTopFrame() missing %q in header:\n%s", want, frame)
		}
	}

	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	// lines: 0 summary, 1 blank, 2 column header, 3 separator, 4.. rows
	if len(lines) != 7 {
		t.Fatalf("formatTopFrame() returned %d lines, want 7:\n%s", len(lines), frame)
	}

	wantOrder := []string{"big", "small", "stopped-svc"}
	for i, name := range wantOrder {
		if !strings.HasPrefix(lines[4+i], name) {
			t.Errorf("row %d = %q, want service %q first", i, lines[4+i], name)
		}
	}

	if !strings.Contains(lines[6], "-") {
		t.Errorf("stopped service row should show dashes, got %q", lines[6])
	}
}

func TestFormatTopFrameEmpty(t *testing.T) {
	frame := formatTopFrame(nil, 2*time.Second, time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(frame, "0/0 running") {
		t.Errorf("formatTopFrame(nil) missing 0/0 summary:\n%s", frame)
	}
}
