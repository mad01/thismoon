package tts

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// WithTimeout bounds one provider request by timeout. It also returns the
// limit that applies, which is the caller's own deadline when that comes
// sooner (a doctor probe allows 10s), so a timeout names the wait that ran
// out.
func WithTimeout(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc, time.Duration) {
	limit := timeout
	if deadline, ok := ctx.Deadline(); ok {
		if left := time.Until(deadline).Round(time.Second); left < limit {
			limit = left
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, limit
}

// TimedOut reports an error from running out of time. A dial that timed out
// is a failure to connect, so it is left to count as unreachable; a caller
// that cancelled is not a timeout either.
func TimedOut(err error) bool {
	if op, ok := errors.AsType[*net.OpError](err); ok && op.Op == "dial" {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	netErr, ok := errors.AsType[net.Error](err)
	return ok && netErr.Timeout()
}

// retriedAttempts is how many attempts a retried request makes: the first
// and one retry.
const retriedAttempts = 2

// WaitToRetry pauses delay before a retry and reports whether to make it.
// It does not when ctx is done, or when ctx's deadline leaves no room for
// the delay plus a whole attempt of timeout: a retry the caller would cut
// short (a doctor or /enginez probe allows 10s) could only fail again.
func WaitToRetry(ctx context.Context, timeout, delay time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay+timeout {
		return false
	}
	pause := time.NewTimer(delay)
	defer pause.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-pause.C:
		return true
	}
}

// Retried is the outcome of a retried request from its two attempts' errors:
// the retry's own, except that when both attempts timed out its reason says
// so, since one stall can be bad luck and two are worth knowing about.
func Retried(first, retry error) error {
	te, ok := errors.AsType[*Error](retry)
	if !ok || !TimedOut(first) || !TimedOut(retry) {
		return retry
	}
	both := *te
	both.Message += fmt.Sprintf(" (%d attempts)", retriedAttempts)
	return &both
}
