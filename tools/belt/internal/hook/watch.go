package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/cache"
	"github.com/mad01/thismoon/tools/belt/internal/watch"
)

// RunWatch reads a Stop or SessionStart payload from r and waits for items
// addressed to that session on the events service. When some arrive it
// writes them to stderr and reports true, which the CLI turns into exit 2:
// the asyncRewake signal that shows stderr to the session and wakes it.
// Every other outcome reports false with nothing written: watch disabled
// (the default), a config belt cannot read, a payload with no session id,
// an events service that never answers, or a max wait with no items. The
// watcher is advisory plumbing, not a guard, so it fails open and quiet.
func RunWatch(ctx context.Context, load Load, r io.Reader, stderr io.Writer) bool {
	cfg, err := load()
	if err != nil || !cfg.Watch.Enabled {
		return false
	}
	var p payload
	if err := json.NewDecoder(r).Decode(&p); err != nil || p.SessionID == "" {
		return false
	}
	items := watch.Run(ctx, watch.Config{
		BaseURL:      notify.BaseURL(),
		Source:       cfg.Watch.SourceName(),
		SessionID:    p.SessionID,
		PollInterval: cfg.Watch.PollInterval(),
		MaxWait:      cfg.Watch.MaxWait(),
		CacheDir:     cache.Dir(),
		Owner:        strconv.Itoa(os.Getpid()),
	})
	if len(items) == 0 {
		return false
	}
	fmt.Fprintln(stderr, watch.Render(items))
	return true
}
