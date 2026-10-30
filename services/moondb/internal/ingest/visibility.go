package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// visibility hides a session from the listings or shows it again. It sets
// Session.Hidden on a session the store holds and touches nothing else, so
// a later envelope's upsert keeps it. A session the store does not hold is
// logged and dropped rather than created empty: `moondb sessions` resolves
// the id against serve before it writes, so only a store rebuilt since then
// lacks it. An envelope that asks for the state the session is in changes
// nothing, so a replay is harmless.
func (in *Ingester) visibility(ctx context.Context, env spool.Envelope) error {
	if env.Visibility == nil {
		return errNoPayload(env)
	}
	id := env.Session.ID
	cur, err := in.Store.Session(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		in.logf(
			"ingest: visibility %s names session %q, which the store does not hold; dropping it",
			env.ID,
			id,
		)
		return nil
	}
	if err != nil {
		return fmt.Errorf("ingest: load session %s: %w", id, err)
	}
	s := cur.Session
	if s.Hidden == env.Visibility.Hidden {
		return nil
	}
	s.Hidden = env.Visibility.Hidden
	return in.putSession(ctx, s)
}
