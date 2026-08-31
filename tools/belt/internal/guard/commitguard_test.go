package guard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// commitGuardFixture pins every external input the guard reads: the clock,
// the repo resolver, the override state, and the events sink.
type commitGuardFixture struct {
	guard  *CommitGuard
	warned []string
}

func newCommitGuardFixture(cfg config.Config, repo string, now time.Time, overrides ...string) *commitGuardFixture {
	f := &commitGuardFixture{guard: NewCommitGuard(cfg)}
	f.guard.resolveRepo = func(string) string { return repo }
	f.guard.now = func() time.Time { return now }
	f.guard.overrideActive = func(name string) bool {
		return slices.Contains(overrides, name)
	}
	f.guard.emit = func(_, _, _, message string, _ map[string]string) {
		f.warned = append(f.warned, message)
	}
	return f
}

// localTime builds a local-zone time on a fixed date per weekday: 2026-08-24
// is a Monday.
func localTime(t *testing.T, weekday time.Weekday, hour, minute int) time.Time {
	t.Helper()
	base := time.Date(2026, 8, 24, hour, minute, 0, 0, time.Local) // Monday
	offset := (int(weekday) - int(time.Monday) + 7) % 7
	return base.AddDate(0, 0, offset)
}

func TestCommitGuard(t *testing.T) {
	rule := config.CommitGuard{
		Repos:       []string{"github.com/mad01/*"},
		AlwaysAllow: []string{"github.com/mad01/dotfiles"},
		BlockHours:  "09:00-17:00",
		Override:    "vacation",
		Mode:        "hard",
	}
	softRule := rule
	softRule.Mode = ""

	tests := []struct {
		name      string
		rule      config.CommitGuard
		repo      string
		at        time.Time
		overrides []string
		wantDeny  bool
		wantWarn  bool
	}{
		{"weekday 10:00 hard denies", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0), nil, true, false},
		{"soft mode warns and allows", softRule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0), nil, false, true},
		{"evening allows", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 18, 0), nil, false, false},
		{"before window allows", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 8, 59), nil, false, false},
		{"window end is exclusive", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 17, 0), nil, false, false},
		{"saturday allows", rule, "github.com/mad01/thismoon", localTime(t, time.Saturday, 10, 0), nil, false, false},
		{"sunday allows", rule, "github.com/mad01/thismoon", localTime(t, time.Sunday, 10, 0), nil, false, false},
		{"always_allow repo any time", rule, "github.com/mad01/dotfiles", localTime(t, time.Tuesday, 10, 0), nil, false, false},
		{"unlisted repo allows", rule, "other-host.example/org/repo", localTime(t, time.Tuesday, 10, 0), nil, false, false},
		{"vacation override allows with audit warn", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0), []string{"vacation"}, false, true},
		{"override outside window stays silent", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 18, 0), []string{"vacation"}, false, false},
		{"other override does not help", rule, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0), []string{"sick-day"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{CommitGuards: []config.CommitGuard{tt.rule}}
			f := newCommitGuardFixture(cfg, tt.repo, tt.at, tt.overrides...)
			d := f.guard.Check(Input{Event: EventBash, Command: `git commit -m "x"`, Cwd: "/tmp"})
			if (d != nil) != tt.wantDeny {
				t.Errorf("denial = %v, wantDeny %v", d, tt.wantDeny)
			}
			if (len(f.warned) > 0) != tt.wantWarn {
				t.Errorf("warnings = %v, wantWarn %v", f.warned, tt.wantWarn)
			}
			if d != nil && !strings.Contains(d.Reason, CommitGuardID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
		})
	}
}

// TestCommitGuardIgnoresDirectMainRepos pins the narrow reach of the shared
// list (docs/adr/0013): direct_main_repos is about push/commit-policy
// workflow, not work-hours discipline, so a listed repo still hits the
// hours rule. Its own carve-out stays commit_guards[].always_allow.
func TestCommitGuardIgnoresDirectMainRepos(t *testing.T) {
	rule := config.CommitGuard{
		Repos:      []string{"github.com/mad01/*"},
		BlockHours: "09:00-17:00",
		Mode:       "hard",
	}
	cfg := config.Config{
		CommitGuards:    []config.CommitGuard{rule},
		DirectMainRepos: []string{"github.com/mad01/dotfiles"},
	}
	f := newCommitGuardFixture(cfg, "github.com/mad01/dotfiles", localTime(t, time.Tuesday, 10, 0))
	if d := f.guard.Check(Input{Event: EventBash, Command: "git commit -m x", Cwd: "/tmp"}); d == nil {
		t.Error("direct_main_repos must not exempt commit-guard — want a denial")
	}
}

func TestCommitGuardExplicitBlockDays(t *testing.T) {
	rule := config.CommitGuard{
		Repos:      []string{"github.com/mad01/*"},
		BlockHours: "09:00-17:00",
		BlockDays:  []string{"sat", "sun"},
		Mode:       "hard",
	}
	cfg := config.Config{CommitGuards: []config.CommitGuard{rule}}
	saturday := newCommitGuardFixture(cfg, "github.com/mad01/thismoon", localTime(t, time.Saturday, 10, 0))
	if d := saturday.guard.Check(Input{Event: EventBash, Command: "git commit", Cwd: "/tmp"}); d == nil {
		t.Error("explicit sat block day did not deny on Saturday")
	}
	monday := newCommitGuardFixture(cfg, "github.com/mad01/thismoon", localTime(t, time.Monday, 10, 0))
	if d := monday.guard.Check(Input{Event: EventBash, Command: "git commit", Cwd: "/tmp"}); d != nil {
		t.Errorf("Monday denied by a sat/sun rule: %v", d)
	}
}

func TestCommitGuardMalformedWindowFailsOpen(t *testing.T) {
	rule := config.CommitGuard{Repos: []string{"github.com/mad01/*"}, BlockHours: "nine-to-five", Mode: "hard"}
	cfg := config.Config{CommitGuards: []config.CommitGuard{rule}}
	f := newCommitGuardFixture(cfg, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0))
	if d := f.guard.Check(Input{Event: EventBash, Command: "git commit", Cwd: "/tmp"}); d != nil {
		t.Errorf("malformed block_hours should fail open, got %v", d)
	}
}

func TestCommitGuardNonCommitAndNoConfig(t *testing.T) {
	f := newCommitGuardFixture(config.Config{}, "github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0))
	if d := f.guard.Check(Input{Event: EventBash, Command: "git commit", Cwd: "/tmp"}); d != nil {
		t.Errorf("no config should allow, got %v", d)
	}
	rule := config.CommitGuard{Repos: []string{"github.com/mad01/*"}, BlockHours: "09:00-17:00", Mode: "hard"}
	f = newCommitGuardFixture(config.Config{CommitGuards: []config.CommitGuard{rule}},
		"github.com/mad01/thismoon", localTime(t, time.Tuesday, 10, 0))
	if d := f.guard.Check(Input{Event: EventBash, Command: "git status", Cwd: "/tmp"}); d != nil {
		t.Errorf("non-commit command denied: %v", d)
	}
}

func TestInBlockedWindowAcrossMidnight(t *testing.T) {
	rule := config.CommitGuard{BlockHours: "22:00-06:00", BlockDays: []string{"mon"}}
	if !inBlockedWindow(rule, localTime(t, time.Monday, 23, 0)) {
		t.Error("23:00 not inside a 22:00-06:00 window")
	}
	if !inBlockedWindow(rule, localTime(t, time.Monday, 5, 0)) {
		t.Error("05:00 not inside a 22:00-06:00 window")
	}
	if inBlockedWindow(rule, localTime(t, time.Monday, 12, 0)) {
		t.Error("12:00 inside a 22:00-06:00 window")
	}
}
