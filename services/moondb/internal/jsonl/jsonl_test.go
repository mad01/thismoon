package jsonl

import (
	"slices"
	"strings"
	"testing"
)

func TestScanLines(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		max     int
		stop    int // stop after this many lines; 0 reads them all
		lines   []string
		ends    []int64
		skipped int
	}{
		{
			name:  "lines",
			in:    "a\nbb\n",
			max:   10,
			lines: []string{"a\n", "bb\n"},
			ends:  []int64{2, 5},
		},
		{
			name:  "no final newline",
			in:    "a\nbb",
			max:   10,
			lines: []string{"a\n", "bb"},
			ends:  []int64{2, 4},
		},
		{
			name: "long line skipped", in: "a\n0123456789\nb\n", max: 5,
			lines: []string{"a\n", "b\n"}, ends: []int64{2, 15}, skipped: 1,
		},
		{name: "empty", in: "", max: 10},
		{
			name:  "stops when told",
			in:    "a\nb\nc\n",
			max:   10,
			stop:  1,
			lines: []string{"a\n"},
			ends:  []int64{2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			var ends []int64
			skipped, err := Scan(
				strings.NewReader(tc.in),
				tc.max,
				func(l []byte, end int64) bool {
					lines = append(lines, string(l))
					ends = append(ends, end)
					return tc.stop == 0 || len(lines) < tc.stop
				},
			)
			if err != nil || skipped != tc.skipped {
				t.Fatalf("skipped %d err %v, want %d", skipped, err, tc.skipped)
			}
			if !slices.Equal(lines, tc.lines) || !slices.Equal(ends, tc.ends) {
				t.Errorf("lines %q ends %v, want %q %v", lines, ends, tc.lines, tc.ends)
			}
		})
	}
}
