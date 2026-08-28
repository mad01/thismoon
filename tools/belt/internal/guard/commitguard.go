package guard

import (
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/notify"
)

// CommitGuardID identifies the work-hours commit guard.
const CommitGuardID = "commit-guard"

// weekdays is the block_days default: a rule without an explicit day list
// blocks Monday through Friday.
var weekdays = []string{"mon", "tue", "wed", "thu", "fri"}

// dayNames maps time.Weekday onto the three-letter names the config uses.
var dayNames = map[time.Weekday]string{
	time.Monday: "mon", time.Tuesday: "tue", time.Wednesday: "wed",
	time.Thursday: "thu", time.Friday: "fri", time.Saturday: "sat", time.Sunday: "sun",
}

// CommitGuard blocks (or warns about, in the default soft mode) `git commit`
// to configured repos inside a local-time window — the "no personal-repo work
// during work hours" nudge. A rule meant for one machine class lives in that
// class's rendered config (docs/adr/0010);
// always_allow carves out repos needed at any hour, and an active
// override (belt override set <name> --reason "...", timed, 10m default) suppresses the rule
// with a warn event so the exception stays auditable. Unresolved repos,
// malformed windows, and missing config all fail open.
type CommitGuard struct {
	cfg config.Config
	// resolveRepo returns the canonical host/owner/repo of the repo at dir,
	// or "" when it cannot be determined. Injectable for tests.
	resolveRepo func(dir string) string
	// now returns the local wall-clock time. Injectable for tests.
	now func() time.Time
	// overrideActive reports whether the named override file is set.
	// Injectable for tests.
	overrideActive func(name string) bool
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewCommitGuard builds the guard with the real git, clock, and override
// backends.
func NewCommitGuard(cfg config.Config) *CommitGuard {
	return &CommitGuard{
		cfg:            cfg,
		resolveRepo:    func(dir string) string { return canonicalRepo(gitRemoteURL(dir)) },
		now:            time.Now,
		overrideActive: config.OverrideActive,
		emit:           notify.EmitEvent,
	}
}

func (g *CommitGuard) ID() string    { return CommitGuardID }
func (g *CommitGuard) Event() string { return EventBash }

// Check scans every git commit in the command against every commit_guard
// rule and returns the first hard denial; soft rules emit a warn event and
// allow.
func (g *CommitGuard) Check(in Input) *Denial {
	if len(g.cfg.CommitGuards) == 0 {
		return nil
	}
	commits := findGitCommands(in.Command, "commit")
	if len(commits) == 0 {
		return nil
	}
	for _, c := range commits {
		dir := c.dir
		if dir == "" {
			dir = in.Cwd
		}
		repo := g.resolveRepo(dir)
		if repo == "" {
			continue
		}
		for _, rule := range g.cfg.CommitGuards {
			if d := g.checkRule(rule, repo); d != nil {
				return d
			}
		}
	}
	return nil
}

// checkRule applies one rule to one commit's repo. nil means the rule does
// not apply, allows this repo, or fired in soft mode (warn event emitted).
func (g *CommitGuard) checkRule(rule config.CommitGuard, repo string) *Denial {
	if config.RepoMatches(rule.AlwaysAllow, repo) {
		return nil
	}
	if !config.RepoMatches(rule.Repos, repo) {
		return nil
	}
	now := g.now()
	if !inBlockedWindow(rule, now) {
		return nil
	}
	// The override check runs after the window check on purpose: a suppressed
	// would-be block leaves a warn event, so overridden guards stay auditable
	// instead of silently dark.
	if rule.Override != "" && g.overrideActive(rule.Override) {
		g.emit("belt", "warn", "commit-guard overridden ("+rule.Override+")",
			fmt.Sprintf("belt[%s]: override %q suppressed a block on %q inside the %s window. "+
				"Commit proceeding.", CommitGuardID, rule.Override, repo, rule.BlockHours),
			map[string]string{"guard": CommitGuardID, "repo": repo})
		return nil
	}
	when := fmt.Sprintf("%s, %s", now.Format("15:04"), now.Format("Monday"))
	if !rule.Hard() {
		g.emit("belt", "warn", "commit-guard: personal repo during work hours (soft)",
			fmt.Sprintf("belt[%s]: Heads up — personal repo %q during work hours (%s). "+
				"Consider adding a ticket and picking this up tonight. Commit proceeding (soft mode).",
				CommitGuardID, repo, when),
			map[string]string{"guard": CommitGuardID, "repo": repo})
		return nil
	}
	return Reasonf(CommitGuardID,
		"Commit blocked. Personal repo %q during work hours (%s, window %s). "+
			"Add a ticket and work on it this evening instead."+overrideHint(rule.Override),
		repo, when, rule.BlockHours)
}

// overrideHint names the escape hatch in a hard denial, so a legitimate
// exception (vacation) is one command away instead of a config edit.
func overrideHint(override string) string {
	if override == "" {
		return ""
	}
	return fmt.Sprintf(" (override: belt override set %s --reason \"...\" [--for 1h])", override)
}

// inBlockedWindow reports whether t falls inside the rule's blocked window:
// the day must be in block_days (default weekdays) and the time inside
// block_hours. A malformed block_hours fails open — a config typo must not
// block commits.
func inBlockedWindow(rule config.CommitGuard, t time.Time) bool {
	days := rule.BlockDays
	if len(days) == 0 {
		days = weekdays
	}
	today := dayNames[t.Weekday()]
	dayBlocked := false
	for _, d := range days {
		if strings.EqualFold(strings.TrimSpace(d), today) {
			dayBlocked = true
			break
		}
	}
	if !dayBlocked {
		return false
	}
	start, end, ok := parseWindow(rule.BlockHours)
	if !ok {
		return false
	}
	minute := t.Hour()*60 + t.Minute()
	if start <= end {
		return minute >= start && minute < end
	}
	// A window crossing midnight (22:00-06:00) blocks both sides of it.
	return minute >= start || minute < end
}

// parseWindow parses "HH:MM-HH:MM" into minutes-of-day; ok is false for
// anything malformed.
func parseWindow(window string) (start, end int, ok bool) {
	from, to, found := strings.Cut(strings.TrimSpace(window), "-")
	if !found {
		return 0, 0, false
	}
	start, okFrom := parseMinutes(from)
	end, okTo := parseMinutes(to)
	return start, end, okFrom && okTo
}

// parseMinutes parses "HH:MM" into minutes since midnight.
func parseMinutes(s string) (int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
