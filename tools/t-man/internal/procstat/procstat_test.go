package procstat

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestParsePSOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    map[int]Stats
		wantErr bool
	}{
		{
			name:   "empty output",
			output: "",
			want:   map[int]Stats{},
		},
		{
			name:   "single process",
			output: "  1234  66560   0.0  02:03\n",
			want: map[int]Stats{
				1234: {
					PID:        1234,
					RSSBytes:   66560 * 1024,
					CPUPercent: 0.0,
					Uptime:     2*time.Minute + 3*time.Second,
				},
			},
		},
		{
			name: "multiple processes with days and hours",
			output: "  10  1024   1.5  3-04:05:06\n" +
				"  20  2048  12.0  01:02:03\n",
			want: map[int]Stats{
				10: {
					PID:        10,
					RSSBytes:   1024 * 1024,
					CPUPercent: 1.5,
					Uptime:     3*24*time.Hour + 4*time.Hour + 5*time.Minute + 6*time.Second,
				},
				20: {
					PID:        20,
					RSSBytes:   2048 * 1024,
					CPUPercent: 12.0,
					Uptime:     time.Hour + 2*time.Minute + 3*time.Second,
				},
			},
		},
		{
			name:   "short row skipped",
			output: "garbage\n",
			want:   map[int]Stats{},
		},
		{
			name:    "bad pid",
			output:  "abc 1024 0.0 01:02\n",
			wantErr: true,
		},
		{
			name:    "bad etime",
			output:  "10 1024 0.0 42\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePSOutput(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePSOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parsePSOutput() returned %d entries, want %d", len(got), len(tt.want))
			}
			for pid, want := range tt.want {
				if got[pid] != want {
					t.Errorf("parsePSOutput()[%d] = %+v, want %+v", pid, got[pid], want)
				}
			}
		})
	}
}

func TestParseEtime(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{input: "00:45", want: 45 * time.Second},
		{input: "12:34", want: 12*time.Minute + 34*time.Second},
		{input: "01:02:03", want: time.Hour + 2*time.Minute + 3*time.Second},
		{input: "2-03:04:05", want: 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		{input: "42", wantErr: true},
		{input: "a:b", wantErr: true},
		{input: "x-01:02:03", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseEtime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseEtime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseEtime(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCollect(t *testing.T) {
	t.Run("no pids skips ps entirely", func(t *testing.T) {
		c := &Collector{
			execCommand: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				t.Fatal("execCommand should not be called with no pids")
				return nil
			},
		}
		got, err := c.Collect(context.Background(), nil)
		if err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Collect() = %v, want empty map", got)
		}
	})

	t.Run("parses ps output", func(t *testing.T) {
		c := &Collector{
			execCommand: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				return exec.Command("echo", "1234 1024 0.5 01:02")
			},
		}
		got, err := c.Collect(context.Background(), []int{1234})
		if err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
		want := Stats{
			PID:        1234,
			RSSBytes:   1024 * 1024,
			CPUPercent: 0.5,
			Uptime:     time.Minute + 2*time.Second,
		}
		if got[1234] != want {
			t.Errorf("Collect()[1234] = %+v, want %+v", got[1234], want)
		}
	})

	t.Run("all pids gone is not an error", func(t *testing.T) {
		c := &Collector{
			execCommand: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				return exec.Command("sh", "-c", "exit 1")
			},
		}
		got, err := c.Collect(context.Background(), []int{99999})
		if err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Collect() = %v, want empty map", got)
		}
	})
}

func TestFormatRSS(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{bytes: 512 * 1024, want: "512K"},
		{bytes: 66560 * 1024, want: "65.0M"},
		{bytes: 8 * 1024 * 1024 * 1024, want: "8.0G"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatRSS(tt.bytes); got != tt.want {
				t.Errorf("FormatRSS(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: 45 * time.Second, want: "45s"},
		{d: 12*time.Minute + 5*time.Second, want: "12m05s"},
		{d: 4*time.Hour + 12*time.Minute, want: "4h12m"},
		{d: 3*24*time.Hour + 4*time.Hour, want: "3d04h"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatUptime(tt.d); got != tt.want {
				t.Errorf("FormatUptime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
