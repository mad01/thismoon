// Package ticker drives the firing loop: every interval it asks the store for
// due reminders, delivers each via the Notifier, and records the fire (which
// reschedules a recurring reminder or marks a one-shot fired).
package ticker

import (
	"context"
	"log"
	"time"

	kitnotify "github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/reminder/internal/notify"
	"github.com/mad01/thismoon/services/reminder/internal/store"
)

// Store is the subset of *store.Store the ticker needs.
type Store interface {
	DueReminders(now time.Time) []store.Reminder
	Trigger(id string, now time.Time) (store.Reminder, error)
}

// Run loops until ctx is cancelled, firing due reminders every interval. now
// supplies the current time (injected for tests); pass time.Now in production.
func Run(
	ctx context.Context,
	st Store,
	n notify.Notifier,
	interval time.Duration,
	now func() time.Time,
) {
	Cycle(st, n, now)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			Cycle(st, n, now)
		}
	}
}

// Cycle fires every reminder due as of now() exactly once. If delivery fails
// the reminder is left pending so the next cycle retries it — Trigger only runs
// after a successful notification.
func Cycle(st Store, n notify.Notifier, now func() time.Time) {
	for _, r := range st.DueReminders(now()) {
		if err := n.Notify(r.Title, r.Body); err != nil {
			log.Printf("reminder: notify %s (%q) failed, will retry: %v", r.ID, r.Title, err)
			continue
		}
		kitnotify.EmitEvent("reminder", "info", "reminder fired: "+r.Title, r.Body,
			map[string]string{"id": r.ID})
		if _, err := st.Trigger(r.ID, now()); err != nil {
			log.Printf("reminder: trigger %s failed: %v", r.ID, err)
		}
	}
}
