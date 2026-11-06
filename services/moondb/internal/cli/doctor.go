package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(moondb.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir (env vars and config file included), not the
// compile-time defaults. The `doctor` command and the moondb_doctor MCP tool
// share this one set, so an agent with no shell gets the same diagnosis a
// human does.
func doctorChecks(context.Context) []doctor.Check {
	api := apiURL()
	return []doctor.Check{
		{Name: "config", Run: checkConfig},
		{Name: "hooks-wired", Run: hooksWired},
		doctor.ServiceReachable(api),
		{Name: "store-ping", Run: client.New(api).Health},
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(api),
		{Name: "agent-briefs", Run: briefsCheck(client.New(api))},
		{Name: "cut-turns", Run: cutTurnsCheck(client.New(api), time.Now)},
	}
}

// briefsCheck warns about agents an import could still give a spawn
// brief or later messages: they were recorded before moondb kept them.
// The check passes with the count and the command that fills them as its
// note, since the edits and traces of those agents are whole; only the
// brief or the messages are missing. A serve it cannot ask is left to
// service-reachable and version-skew.
func briefsCheck(c *client.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		v, err := c.Doctor(ctx)
		switch {
		case errors.Is(err, store.ErrNotFound):
			return doctor.Skip("serve predates this check; see version-skew")
		case err != nil:
			return doctor.Skip("serve did not answer; see service-reachable")
		case v.ToFill() > 0:
			return doctor.Skip(fmt.Sprintf(
				"%d agents without a spawn brief or later messages; run moondb import --since %s --force",
				v.ToFill(),
				moondb.DefaultImportSince,
			))
		}
		return nil
	}
}

// cutTurnsCheck notes the turns whose stored transcript the old cap cut
// (server.TranscriptCuts): they lost their head, and the trace and cost
// miss the model calls in it. It stats each session's transcript file and
// never reads it. A turn whose file is still there can be repaired, and
// the note names the forced import whose window reaches the oldest such
// file; a turn whose file is gone stays cut and is counted apart. Like
// agent-briefs it passes with the note, since the rest of each record is
// whole, and leaves a serve it cannot ask to the checks that own that.
func cutTurnsCheck(c *client.Client, now func() time.Time) func(context.Context) error {
	return func(ctx context.Context) error {
		v, err := c.Doctor(ctx)
		switch {
		case errors.Is(err, store.ErrNotFound), err == nil && v.TranscriptCuts == nil:
			return doctor.Skip("serve predates this check; see version-skew")
		case err != nil:
			return doctor.Skip("serve did not answer; see service-reachable")
		}
		t := tallyCuts(*v.TranscriptCuts)
		if t.repairable+t.gone == 0 {
			return nil
		}
		return doctor.Skip(t.note(v.TranscriptCuts.Cap, now()))
	}
}

// cutTally counts cut turns by whether their transcript file is still on
// disk, with the oldest last write among the files that are.
type cutTally struct {
	repairable, gone int
	oldest           time.Time
}

// tallyCuts stats each session's transcript file once. Its cut turns are
// repairable while the file exists, and so is its tail turn when the file
// holds more than the cap past the tail's offset. A tail whose file is
// gone cannot be judged and is not counted.
func tallyCuts(cuts server.TranscriptCuts) cutTally {
	var t cutTally
	for _, s := range cuts.Sessions {
		info, err := os.Stat(s.TranscriptPath)
		if err != nil {
			t.gone += len(s.Turns)
			continue
		}
		n := len(s.Turns)
		if s.Tail != nil && info.Size()-s.Tail.Offset > cuts.Cap {
			n++
		}
		if n == 0 {
			continue
		}
		t.repairable += n
		if t.oldest.IsZero() || info.ModTime().Before(t.oldest) {
			t.oldest = info.ModTime()
		}
	}
	return t
}

// note is the check's line for a tally with at least one cut turn.
func (t cutTally) note(capBytes int64, now time.Time) string {
	total := t.repairable + t.gone
	head := fmt.Sprintf("%d %s a transcript cut at the old %d MiB cap",
		total, noun(total, "turn keeps", "turns keep"), capBytes>>20)
	if t.repairable == 0 {
		return head + ", with no transcript file left to repair from"
	}
	note := fmt.Sprintf(
		"%s; %d with the transcript file on disk: with no session active, run moondb import --since %s --force",
		head,
		t.repairable,
		importWindow(t.oldest, now),
	)
	if t.gone > 0 {
		note += fmt.Sprintf("; %d without one %s cut", t.gone, noun(t.gone, "stays", "stay"))
	}
	return note
}

// importWindow is an import --since span that reaches a transcript last
// written at mtime: its age in days rounded up, plus one, so the command
// still reaches the file when it runs later that day.
func importWindow(mtime, now time.Time) string {
	days := int(math.Ceil(now.Sub(mtime).Hours()/24)) + 1
	return fmt.Sprintf("%dd", max(days, 1))
}

// checkConfig fails on a config file that will not parse, and on a
// [prices] table serve would refuse to start with.
func checkConfig(context.Context) error {
	if cfgErr != nil {
		return cfgErr
	}
	_, err := priceOverrides(cfg.Prices)
	return err
}
