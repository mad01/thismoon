package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/ledger"
)

// eventsProbeTimeout bounds the doctor's events-service check, the same
// budget the kof probe gets: a local service answers in milliseconds.
const eventsProbeTimeout = 400 * time.Millisecond

// eventsProbe reports whether the events service answers: the base URL the
// record, watch and archive paths use, and the error when it does not.
// Injected so tests never dial the real port.
type eventsProbe func() (base string, err error)

// liveEventsProbe asks the events service's health endpoint.
func liveEventsProbe() (string, error) {
	base := notify.BaseURL()
	ctx, cancel := context.WithTimeout(context.Background(), eventsProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return base, fmt.Errorf("build health check: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return base, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return base, fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return base, nil
}

// printLifecycle renders the ledger, watch and events-service sections and
// returns the warnings they produce: every ledger or watch value belt uses
// in another form than the file states (those values are clamped, never
// rejected), a watcher that is on but can never wake anyone, and one whose
// max wait outlives the hook that runs it.
func printLifecycle(w io.Writer, cfg config.Config, probe eventsProbe) []string {
	printLedger(w, cfg.Ledger)
	warns := append(cfg.Ledger.Adjustments(), cfg.Watch.Adjustments()...)
	warns = append(warns, printWatch(w, cfg.Watch)...)
	base, err := probe()
	fmt.Fprintln(
		w,
		"\nevents service, where record posts, watch polls, and every deny and hint is archived:",
	)
	if err != nil {
		fmt.Fprintf(
			w,
			"  %s  UNREACHABLE (%v): record entries and deny/hint events are dropped, watch wakes nobody\n",
			base,
			err,
		)
		if cfg.Watch.Enabled {
			warns = append(warns, fmt.Sprintf(
				"watch is enabled but the events service at %s is unreachable: no session will be woken",
				base,
			))
		}
		return warns
	}
	fmt.Fprintf(w, "  %s  reachable\n", base)
	return warns
}

func printLedger(w io.Writer, l config.Ledger) {
	fmt.Fprintln(w, "\nledger, the lifecycle events belt record posts "+
		"(ledger.enabled, ledger.source, ledger.agent_types):")
	fmt.Fprintf(w, "  %s  source: %s  events: %s\n",
		enabledWord(l.On()), l.SourceName(), strings.Join(ledger.Events(), ", "))
	printAgentTypes(w, l.AgentTypes)
}

// printAgentTypes lists the model and effort each agent type's
// subagent-start entries are tagged with: the entries in effect, after
// every malformed one was dropped (the warnings section names those).
func printAgentTypes(w io.Writer, a config.AgentTypes) {
	names := a.Names()
	if len(names) == 0 {
		fmt.Fprintln(w, "  agent types: none, so subagent-start entries carry no model or effort tag")
		return
	}
	fmt.Fprintln(w, "  agent types, tagged on their subagent-start entries:")
	for _, name := range names {
		tags := a.For(name)
		fmt.Fprintf(w, "    %-24s model: %-8s effort: %s\n",
			name, orDash(tags.Model), orDash(tags.Effort))
	}
}

// orDash renders a tag an agent_types entry leaves off.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// printWatch renders the watcher's settings and returns the warning for a
// max wait at or past the documented hook timeout. The wait of a disabled
// watcher cannot outlive anything, so only an enabled one is checked.
func printWatch(w io.Writer, wc config.Watch) []string {
	fmt.Fprintln(w, "\nwatch, the asyncRewake inbox watcher belt watch runs (watch.*):")
	if !wc.Enabled {
		fmt.Fprintln(w, "  DISABLED (the default; set watch.enabled to run it)")
		return nil
	}
	fmt.Fprintf(w, "  enabled  source: %s  poll: %s  max wait: %s\n",
		wc.SourceName(), wc.PollInterval(), wc.MaxWait())
	if wc.MaxWait() < config.WatchHookTimeout {
		return nil
	}
	return []string{fmt.Sprintf(
		"watch max wait is %s: at or past the documented %s hook timeout the watcher is "+
			"killed before it can exit on its own; keep watch.max_wait_s below the hook's timeout",
		wc.MaxWait(), config.WatchHookTimeout,
	)}
}
