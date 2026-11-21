package guard

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// The windows git keeps what a loss leaves behind for by default: an
// unreachable object for 2 weeks (gc.pruneExpire), a reflog entry for 90
// days (gc.reflogExpire) and one no branch holds for 30
// (gc.reflogExpireUnreachable). An expiry sooner than its window makes a
// loss final before git would.
const (
	pruneKeep       = 14 * 24 * time.Hour
	reflogKeep      = 90 * 24 * time.Hour
	unreachableKeep = 30 * 24 * time.Hour
)

// expirySlack is how much sooner than its window an expiry may be and still
// pass: git reads the clock a moment before belt does, so a date written as
// the default lands just inside it.
const expirySlack = time.Minute

// expiryReader judges expiry dates against git's default windows, reading
// each the way git reads it (parse, gitExpiry) within ctx.
type expiryReader struct {
	ctx   context.Context
	parse func(ctx context.Context, date string) (uint64, error)
}

// finalWord reports whether an expiry date as written is sooner than keep
// (final). A word the shell may still rewrite may be any date.
func (x expiryReader) finalWord(raw string, keep time.Duration) bool {
	return mayExpand(raw) || x.final(unquoteWord(raw), keep)
}

// final reports whether an expiry date deletes sooner than git's default
// window keep: now and all, which git reads as everything, a date less than
// keep ago (expirySlack aside) or in the future, and a date git cannot
// read, which it refuses or belt cannot tell. never and false keep
// everything.
func (x expiryReader) final(date string, keep time.Duration) bool {
	switch date {
	case "now", "all":
		return true
	case "never", "false":
		return false
	}
	at, err := x.parse(x.ctx, date)
	if err != nil {
		return true
	}
	limit := time.Now().Add(expirySlack - keep).Unix()
	return limit < 0 || at > uint64(limit)
}

// finalSetting reports whether a setting a git call runs with sets a gc
// expiry key (expiryWindow) sooner than its default (final). A key the
// shell may still rewrite may be any of them, so its value is held to the
// longest window, and a value only the environment or a shell holds may be
// any date.
func (x expiryReader) finalSetting(s callSetting) bool {
	keep, ok := expiryWindow(s.key)
	if s.anyKey {
		keep, ok = reflogKeep, true
	}
	return ok && (s.anyValue || x.final(s.value, keep))
}

// gitExpiry reads an expiry date the way git reads --expire, --prune and
// the gc expiry keys (parse_expiry_date): `git config --type=expiry-date`
// turns it into Unix seconds, with now and all at the top of the range,
// and fails on a date git cannot read. The empty file keeps every config
// file out of the read, and / keeps any repository's config away.
func gitExpiry(ctx context.Context, date string) (uint64, error) {
	out, err := runProofGit(
		ctx, "/", "config", "--file", os.DevNull, "--type=expiry-date", "--default="+date,
		"--get", "belt.expiry",
	)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
}
