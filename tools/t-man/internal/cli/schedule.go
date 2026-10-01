package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/procstat"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// runTimeLayout is how list and status print a run time.
const runTimeLayout = "2006-01-02 15:04"

// scheduleFlags is the raw --schedule, --every, and --calendar input to add.
// Parsing lives in the service package; this only decides how the three
// flags combine.
type scheduleFlags struct {
	clock    string   // --schedule: "07:30" or "mon@07:30,fri@17:00"
	every    string   // --every: a Go duration
	calendar []string // --calendar: one launchd field list per value
}

// apply fills def's schedule fields from the flags. --schedule and
// --calendar both add calendar entries and may be combined; --every is the
// interval form and excludes them, since a definition takes one trigger kind.
// The combined calendar is normalized so the same schedule written two ways
// hashes the same.
func (f scheduleFlags) apply(def *service.Definition) error {
	if f.every != "" && (f.clock != "" || len(f.calendar) > 0) {
		return fmt.Errorf("--every cannot be combined with --schedule or --calendar")
	}
	if f.every != "" {
		seconds, err := service.ParseInterval(f.every)
		if err != nil {
			return fmt.Errorf("invalid --every: %w", err)
		}
		def.IntervalSeconds = seconds
	}
	if f.clock != "" {
		entries, err := service.ParseClockSchedule(f.clock)
		if err != nil {
			return fmt.Errorf("invalid --schedule: %w", err)
		}
		def.Calendar = append(def.Calendar, entries...)
	}
	for _, spec := range f.calendar {
		entries, err := service.ParseCalendarFields(spec)
		if err != nil {
			return fmt.Errorf("invalid --calendar: %w", err)
		}
		def.Calendar = append(def.Calendar, entries...)
	}
	def.Calendar = service.NormalizeCalendar(def.Calendar)
	return nil
}

// formatNextRun renders the NEXT RUN cell: the computed minute for a
// calendar job, the cadence for an interval job (its phase is launchd's),
// and "-" for a long-lived service or an entry that never matches.
func formatNextRun(def *service.Definition, now time.Time) string {
	if def.IntervalSeconds > 0 {
		return "within " + procstat.FormatUptime(def.Interval())
	}
	next, ok := def.NextRun(now)
	if !ok {
		return "-"
	}
	return next.Format(runTimeLayout)
}

// formatLastRun renders the LAST RUN cell from the newest log write, "-"
// when the job has not written a log yet or is not scheduled.
func formatLastRun(def *service.Definition) string {
	if !def.Scheduled() {
		return "-"
	}
	last, ok := def.LastLogWrite()
	if !ok {
		return "-"
	}
	return last.Local().Format(runTimeLayout)
}

// formatExitCode narrows launchd's last exit code to the EXIT cell: the
// number alone ("78: EX_CONFIG" becomes "78"), "-" when launchd reported
// none or the job never exited.
func formatExitCode(raw string) string {
	code, _, _ := strings.Cut(raw, ":")
	code = strings.TrimSpace(code)
	if code == "" || strings.HasPrefix(code, "(") {
		return "-"
	}
	return code
}

// scheduleStatusLines are the extra lines status prints for a scheduled
// job. state may be nil when launchd could not be asked.
func scheduleStatusLines(def *service.Definition, state *launchd.RunState, now time.Time) []string {
	if !def.Scheduled() {
		return nil
	}
	lastExit := "unknown"
	if state != nil && state.LastExitCode != "" {
		lastExit = state.LastExitCode
	}
	return []string{
		"Schedule: " + def.ScheduleString(),
		"Next run: " + formatNextRun(def, now),
		"Last run: " + formatLastRun(def) + " (newest log write)",
		"Last exit code: " + lastExit,
	}
}
