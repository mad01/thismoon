// Package hint is the belt custom hint: one line at session start when
// harvest captures wait in the brain inbox, at most once a day, and nothing
// otherwise. A harvest runs with nobody watching, and a capture expires 30
// days after it was written, so this line is how the captures get a review.
//
// belt treats empty output as silence and runs the hint on a 400 ms budget,
// so a hint that cannot answer stays quiet rather than hold up a session.
package hint

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// dayLayout is the stamp's day, as `date '+%Y-%m-%d'` prints it.
const dayLayout = "2006-01-02"

// Config is what Line needs.
type Config struct {
	State *state.Dir
	// Brain is the brain binary; a bare name is looked up on PATH.
	Brain string
	Now   func() time.Time
}

// Line returns the nudge, or "" when there is nothing to say: the hint
// already spoke today, or no raw harvest capture waits. It writes the day's
// stamp only when it speaks. On an error the line is "".
func Line(ctx context.Context, cfg Config) (string, error) {
	today := cfg.Now().Format(dayLayout)
	day, err := cfg.State.Stamp()
	if err != nil {
		return "", err
	}
	if day == today {
		return "", nil
	}
	n, err := waiting(ctx, cfg.Brain)
	if err != nil || n <= 0 {
		return "", err
	}
	if err := cfg.State.SetStamp(today); err != nil {
		return "", err
	}
	return Message(n), nil
}

// Message is the nudge for n waiting captures.
func Message(n int) string {
	noun := "captures"
	if n == 1 {
		noun = "capture"
	}
	return fmt.Sprintf("%d harvest %s from earlier sessions wait in the brain inbox, "+
		"unverified, and each expires 30 days after it was written. "+
		"Tell the user this once, in one line. "+
		"`brain list inbox --tag harvest` lists them, "+
		"`brain promote <note>` keeps one for good, "+
		"and `brain inbox keep <note>` gives one more time.", n, noun)
}

// listing is the part of `brain list -o json` the hint reads.
type listing struct {
	Total json.Number `json:"total"`
}

// waiting asks brain how many raw harvest captures the inbox holds. A
// listing with no total holds none.
func waiting(ctx context.Context, brain string) (int, error) {
	out, err := exec.CommandContext(ctx, brain,
		"list", "inbox", "--tag", "harvest", "--status", "raw", "-o", "json").Output()
	if err != nil {
		return 0, fmt.Errorf("hint: brain list: %w", err)
	}
	var l listing
	if err := json.Unmarshal(out, &l); err != nil {
		return 0, fmt.Errorf("hint: brain list printed no listing: %w", err)
	}
	if l.Total == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(l.Total.String())
	if err != nil {
		return 0, fmt.Errorf("hint: brain list total %q is not a count", l.Total)
	}
	return n, nil
}
