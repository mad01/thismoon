package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// ip returns a pointer to n for building CalendarEntry literals.
func ip(n int) *int { return &n }

func TestScheduleFlagsApply(t *testing.T) {
	tests := []struct {
		name         string
		flags        scheduleFlags
		wantCalendar []service.CalendarEntry
		wantInterval int
		wantErr      string
	}{
		{name: "no flags leaves a long-lived service", flags: scheduleFlags{}},
		{
			name:         "schedule daily",
			flags:        scheduleFlags{clock: "07:30"},
			wantCalendar: []service.CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
		},
		{
			name:  "schedule list with weekday",
			flags: scheduleFlags{clock: "07:30,fri@17:00"},
			wantCalendar: []service.CalendarEntry{
				{Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(5), Hour: ip(17), Minute: ip(0)},
			},
		},
		{
			name:  "calendar entries are repeatable",
			flags: scheduleFlags{calendar: []string{"day=1,hour=6,minute=0", "weekday=1,hour=9,minute=0"}},
			wantCalendar: []service.CalendarEntry{
				{Minute: ip(0), Hour: ip(6), Day: ip(1)},
				{Minute: ip(0), Hour: ip(9), Weekday: ip(1)},
			},
		},
		{
			name:  "weekday set expands to one entry per day",
			flags: scheduleFlags{clock: "weekdays@08:50"},
			wantCalendar: []service.CalendarEntry{
				{Weekday: ip(1), Hour: ip(8), Minute: ip(50)},
				{Weekday: ip(2), Hour: ip(8), Minute: ip(50)},
				{Weekday: ip(3), Hour: ip(8), Minute: ip(50)},
				{Weekday: ip(4), Hour: ip(8), Minute: ip(50)},
				{Weekday: ip(5), Hour: ip(8), Minute: ip(50)},
			},
		},
		{
			name:  "calendar range cross product",
			flags: scheduleFlags{calendar: []string{"hour=9-10,weekday=1-2,minute=0"}},
			wantCalendar: []service.CalendarEntry{
				{Weekday: ip(1), Hour: ip(9), Minute: ip(0)},
				{Weekday: ip(1), Hour: ip(10), Minute: ip(0)},
				{Weekday: ip(2), Hour: ip(9), Minute: ip(0)},
				{Weekday: ip(2), Hour: ip(10), Minute: ip(0)},
			},
		},
		{
			name:  "overlapping flags dedupe",
			flags: scheduleFlags{clock: "mon-fri@07:30", calendar: []string{"weekday=1-5,hour=7,minute=30"}},
			wantCalendar: []service.CalendarEntry{
				{Weekday: ip(1), Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(2), Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(3), Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(4), Hour: ip(7), Minute: ip(30)},
				{Weekday: ip(5), Hour: ip(7), Minute: ip(30)},
			},
		},
		{
			name:    "cap exceeded",
			flags:   scheduleFlags{calendar: []string{"minute=0-59,hour=0-23"}},
			wantErr: "more than the 200 allowed",
		},
		{
			name:  "schedule and calendar combine",
			flags: scheduleFlags{clock: "07:30", calendar: []string{"day=1,hour=6,minute=0"}},
			wantCalendar: []service.CalendarEntry{
				{Hour: ip(7), Minute: ip(30)},
				{Minute: ip(0), Hour: ip(6), Day: ip(1)},
			},
		},
		{name: "every", flags: scheduleFlags{every: "90m"}, wantInterval: 5400},
		{
			name:    "every with schedule rejected",
			flags:   scheduleFlags{every: "1h", clock: "07:30"},
			wantErr: "cannot be combined",
		},
		{
			name:    "every with calendar rejected",
			flags:   scheduleFlags{every: "1h", calendar: []string{"minute=0"}},
			wantErr: "cannot be combined",
		},
		{name: "bad schedule", flags: scheduleFlags{clock: "7am"}, wantErr: "invalid --schedule"},
		{name: "bad calendar", flags: scheduleFlags{calendar: []string{"hour=x"}}, wantErr: "invalid --calendar"},
		{name: "bad every", flags: scheduleFlags{every: "hourly"}, wantErr: "invalid --every"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &service.Definition{Name: "job", Command: "/usr/bin/true"}
			err := tt.flags.apply(def)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("apply() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("apply() error = %v", err)
			}
			want := &service.Definition{
				Calendar:        service.NormalizeCalendar(tt.wantCalendar),
				IntervalSeconds: tt.wantInterval,
			}
			if !reflect.DeepEqual(def.Calendar, want.Calendar) ||
				def.IntervalSeconds != want.IntervalSeconds {
				t.Errorf("apply() = %q, want %q", def.ScheduleString(), want.ScheduleString())
			}
			if def.Scheduled() != (len(tt.wantCalendar) > 0 || tt.wantInterval > 0) {
				t.Errorf("Scheduled() = %v after apply(%+v)", def.Scheduled(), tt.flags)
			}
		})
	}
}

func TestFormatNextRun(t *testing.T) {
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		def  service.Definition
		want string
	}{
		{name: "long-lived service", def: service.Definition{}, want: "-"},
		{
			name: "calendar job",
			def:  service.Definition{Calendar: []service.CalendarEntry{{Hour: ip(7), Minute: ip(30)}}},
			want: "2026-10-02 07:30",
		},
		{
			name: "interval job shows its cadence",
			def:  service.Definition{IntervalSeconds: 5400},
			want: "within 1h30m",
		},
		{
			name: "calendar that never matches",
			def:  service.Definition{Calendar: []service.CalendarEntry{{Month: ip(2), Day: ip(31)}}},
			want: "-",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNextRun(&tt.def, now); got != tt.want {
				t.Errorf("formatNextRun() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatExitCode(t *testing.T) {
	tests := []struct{ raw, want string }{
		{"", "-"},
		{"0", "0"},
		{"1", "1"},
		{"78: EX_CONFIG", "78"},
		{"(never exited)", "-"},
		{"-15", "-15"},
	}
	for _, tt := range tests {
		if got := formatExitCode(tt.raw); got != tt.want {
			t.Errorf("formatExitCode(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

// scheduledJob returns a calendar job whose stdout log was last written at
// the given time, inside a temp dir.
func scheduledJob(t *testing.T, lastWrite time.Time) *service.Definition {
	t.Helper()
	dir := t.TempDir()
	stdout := filepath.Join(dir, "stdout.log")
	if err := os.WriteFile(stdout, []byte("ran\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chtimes(stdout, lastWrite, lastWrite); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	return &service.Definition{
		Name:            "digest",
		Command:         "/usr/local/bin/digest",
		Args:            []string{"--yesterday"},
		StandardOutPath: stdout,
		StandardErrPath: filepath.Join(dir, "stderr.log"),
		Calendar:        []service.CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
	}
}

func TestFormatScheduleRow(t *testing.T) {
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.Local)
	lastWrite := time.Date(2026, time.October, 1, 7, 31, 0, 0, time.Local)
	job := scheduledJob(t, lastWrite)
	svc := &service.Definition{
		Name: "present", Command: "/usr/local/bin/present", Args: []string{"serve"},
		RunAtLoad: true, KeepAlive: true,
	}

	tests := []struct {
		name  string
		def   *service.Definition
		state *launchd.RunState
		want  []string
	}{
		{
			name:  "idle job after a clean run",
			def:   job,
			state: &launchd.RunState{Status: "scheduled", LastExitCode: "0"},
			want:  []string{"digest", "scheduled", "2026-10-02 07:30", "2026-10-01 07:31", "   0", "/usr/local/bin/digest --yesterday"},
		},
		{
			name:  "idle job after a failed run",
			def:   job,
			state: &launchd.RunState{Status: "scheduled", LastExitCode: "1"},
			want:  []string{"scheduled", "   1"},
		},
		{
			name:  "job mid-run",
			def:   job,
			state: &launchd.RunState{Status: "running", Running: true, LastExitCode: "0"},
			want:  []string{"running", "2026-10-02 07:30"},
		},
		{
			name:  "launchd unreachable",
			def:   job,
			state: nil,
			want:  []string{"unknown", "2026-10-02 07:30", "   -"},
		},
		{
			name:  "long-lived service shows dashes",
			def:   svc,
			state: &launchd.RunState{Status: "running", Running: true, LastExitCode: "(never exited)"},
			want:  []string{"present", "running", "-                -                   -", "/usr/local/bin/present serve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatScheduleRow(tt.def, tt.state, now)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("row %q lacks %q", got, want)
				}
			}
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("row %q does not end the line", got)
			}
		})
	}
}

func TestScheduleStatusLines(t *testing.T) {
	now := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.Local)
	lastWrite := time.Date(2026, time.October, 1, 7, 31, 0, 0, time.Local)
	job := scheduledJob(t, lastWrite)

	if lines := scheduleStatusLines(&service.Definition{}, nil, now); lines != nil {
		t.Errorf("long-lived service got schedule lines: %v", lines)
	}

	got := scheduleStatusLines(job, &launchd.RunState{Status: "scheduled", LastExitCode: "0"}, now)
	want := []string{
		"Schedule: daily 07:30",
		"Next run: 2026-10-02 07:30",
		"Last run: 2026-10-01 07:31 (newest log write)",
		"Last exit code: 0",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("scheduleStatusLines() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	got = scheduleStatusLines(job, nil, now)
	if got[3] != "Last exit code: unknown" {
		t.Errorf("without launchd, last exit line = %q", got[3])
	}

	interval := &service.Definition{IntervalSeconds: 3600}
	got = scheduleStatusLines(interval, &launchd.RunState{Status: "scheduled"}, now)
	if got[0] != "Schedule: every 1h0m0s" || got[1] != "Next run: within 1h00m" {
		t.Errorf("interval lines = %v", got[:2])
	}
	if got[2] != "Last run: - (newest log write)" || got[3] != "Last exit code: unknown" {
		t.Errorf("interval lines = %v", got[2:])
	}
}

// TestScheduleFlagsApply_SameScheduleHashesSame pins that the hash does not
// depend on which flag or spelling produced the schedule.
func TestScheduleFlagsApply_SameScheduleHashesSame(t *testing.T) {
	ways := []scheduleFlags{
		{clock: "weekdays@07:30"},
		{clock: "mon-fri@07:30"},
		{clock: "fri@07:30,mon@07:30,tue@07:30,wed@07:30,thu@07:30"},
		{calendar: []string{"weekday=1-5,hour=7,minute=30"}},
		{calendar: []string{"weekday=1-3,hour=7,minute=30", "weekday=4-5,hour=7,minute=30"}},
		{clock: "mon-wed@07:30", calendar: []string{"weekday=4-5,hour=7,minute=30"}},
	}
	var first string
	for _, flags := range ways {
		def := &service.Definition{Name: "job", Command: "/usr/bin/true"}
		if err := flags.apply(def); err != nil {
			t.Fatalf("apply(%+v) error = %v", flags, err)
		}
		h, err := def.Hash()
		if err != nil {
			t.Fatalf("Hash() error = %v", err)
		}
		if first == "" {
			first = h
		}
		if h != first {
			t.Errorf("apply(%+v) hashes %s, want %s (schedule %q)", flags, h, first, def.ScheduleString())
		}
		if got := def.ScheduleString(); got != "mon-fri 07:30" {
			t.Errorf("apply(%+v) ScheduleString() = %q, want mon-fri 07:30", flags, got)
		}
	}
}
