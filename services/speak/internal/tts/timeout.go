package tts

import (
	"context"
	"errors"
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
