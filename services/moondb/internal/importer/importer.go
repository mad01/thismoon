// Package importer brings past sessions into moondb from their transcripts:
// the logic behind `moondb import`. It reads each transcript under the
// projects directory and writes the envelopes `moondb record` would have
// written had the hooks been wired, so serve ingests them with no import
// code of its own. Ids are the hook's (the turn's prompt id, the tool call
// id plus the file's index), so a session imported twice, or imported
// after part of it was captured live, ingests as a no-op for what the
// store already holds.
//
// A transcript records less than the hooks see. There is no read of the
// disk after an edit, so serve has no observed content to compare and
// records no unattributed delta. HEAD at the time of an edit is not
// recorded, so imported edits carry none and no head-moved envelope is
// written: commit links for imported edits come from a later live turn
// whose HEAD moves. Bash commands leave no diff in the transcript and are
// not imported. A file over about 10 KB has no original in the transcript;
// its pre-edit content is taken from the session's file history when the
// history holds the file as the edit found it.
//
// Location is the git placement of the file's directory today: a deleted
// worktree yields no worktree and an absolute path, as the hook records a
// file outside any repository.
package importer

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/jsonl"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Sink receives the envelopes a run writes. *spool.Writer is the one the
// CLI uses.
type Sink interface {
	Write(env spool.Envelope, chunks map[string][]byte) error
}

// Options configures one Run.
type Options struct {
	// Projects is the directory holding one directory of transcripts per
	// project: ~/.claude/projects.
	Projects string
	// FileHistory is the harness's file-history directory, one directory
	// per session: ~/.claude/file-history. Empty skips the history.
	FileHistory string
	// Since skips transcripts last written before it; zero reads them all.
	Since time.Time
	// Known reports whether the store already holds a session; such a
	// session is skipped unless Force. Nil treats every session as new.
	Known func(ctx context.Context, id string) (bool, error)
	// Force imports sessions Known reports, merging by id with what the
	// store holds.
	Force bool
	// Locations lists the locations the store knows, which name the
	// repository of a worktree that is gone. Nil knows none.
	Locations func(ctx context.Context) ([]store.Location, error)
	// Excludes are the globs whose contents are never captured;
	// moondb.DefaultExcludes when nil.
	Excludes []string
	// Sink receives the envelopes. Nil parses, builds and counts without
	// writing: the dry run.
	Sink Sink
}

func (o Options) excludes() []string {
	if o.Excludes == nil {
		return moondb.DefaultExcludes
	}
	return o.Excludes
}

// Counts is what one Run read and wrote.
type Counts struct {
	Transcripts         int `json:"transcripts"`
	SubagentTranscripts int `json:"subagent_transcripts"`
	// UnreadableTranscripts are main transcripts that could not be opened.
	UnreadableTranscripts int     `json:"unreadable_transcripts"`
	SessionsImported      int     `json:"sessions_imported"`
	Skipped               Skipped `json:"sessions_skipped"`
	Turns                 int     `json:"turns"`
	// Edits counts the edits written by tool.
	Edits             map[string]int `json:"edits_by_tool"`
	EditsWithoutPatch int            `json:"edits_without_patch"`
	// Placement counts how the edits' files were keyed.
	Placement Placement `json:"placement"`
	// OriginalsFromHistory counts edits whose pre-edit content came from
	// the file history because the transcript carried none.
	OriginalsFromHistory int `json:"originals_from_file_history"`
	// FailedCalls are file-tool calls the harness answered with an error,
	// UnansweredCalls those it never answered; neither is an edit.
	FailedCalls     int `json:"failed_calls"`
	UnansweredCalls int `json:"unanswered_calls"`
	// BashCalls are shell calls in imported sessions, which are not
	// imported: the transcript records no diff for them.
	BashCalls int `json:"bash_calls_not_imported"`
	// Agents counts the subagents of imported sessions written as Agent
	// records, one agent-close envelope each.
	Agents           int            `json:"agents"`
	Envelopes        int            `json:"envelopes"`
	UndecodableLines int            `json:"undecodable_lines"`
	UnknownTypes     map[string]int `json:"unknown_types"`
	// ElapsedMS is set by the caller, which owns the clock.
	ElapsedMS int64 `json:"elapsed_ms"`
}

// Skipped counts the sessions a run did not import, by reason.
type Skipped struct {
	InStore      int `json:"in_store"`
	OutsideSince int `json:"outside_since"`
	NoEdits      int `json:"no_edits"`
}

// Run imports every transcript under opts.Projects last written at or
// after opts.Since, oldest session first, so the edits of a file reach
// serve in the order they were made. It stops at the first error from
// Locations, Known or the sink; a transcript it cannot read is counted and
// skipped.
func Run(ctx context.Context, opts Options) (Counts, error) {
	counts := Counts{Edits: map[string]int{}, UnknownTypes: map[string]int{}}
	paths, err := filepath.Glob(filepath.Join(opts.Projects, "*", "*.jsonl"))
	if err != nil {
		return counts, fmt.Errorf("importer: list transcripts: %w", err)
	}
	counts.Transcripts = len(paths)
	paths = recent(paths, opts.Since, &counts)
	places := &placer{ctx: ctx}
	if opts.Locations != nil {
		known, err := opts.Locations(ctx)
		if err != nil {
			return counts, fmt.Errorf("importer: list known locations: %w", err)
		}
		places.known = known
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		if err := importOne(ctx, path, opts, places, &counts); err != nil {
			return counts, err
		}
	}
	return counts, nil
}

// recent keeps the transcripts written at or after since, oldest session
// first, and counts the rest as outside the window.
func recent(paths []string, since time.Time, counts *Counts) []string {
	type dated struct {
		path  string
		start time.Time
	}
	var keep []dated
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().Before(since) {
			counts.Skipped.OutsideSince++
			continue
		}
		keep = append(keep, dated{p, firstTime(p, info.ModTime())})
	}
	slices.SortStableFunc(keep, func(a, b dated) int {
		return cmp.Or(a.start.Compare(b.start), cmp.Compare(a.path, b.path))
	})
	out := make([]string, len(keep))
	for i, d := range keep {
		out[i] = d.path
	}
	return out
}

// firstTimeLines bounds how far firstTime reads for a timestamp: the first
// lines of a transcript are short settings entries without one.
const firstTimeLines = 64

// firstTime is the first entry time in a transcript, fallback when the
// first lines carry none.
func firstTime(path string, fallback time.Time) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer f.Close()
	var first time.Time
	n := 0
	_, _ = jsonl.Scan(f, maxLineBytes, func(line []byte, _ int64) bool {
		n++
		var e struct {
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal(bytes.TrimSpace(line), &e) == nil {
			first = entry{Timestamp: e.Timestamp}.at()
		}
		return first.IsZero() && n < firstTimeLines
	})
	if first.IsZero() {
		return fallback
	}
	return first
}

// importOne parses one transcript and, unless a skip rule applies, writes
// its envelopes.
func importOne(
	ctx context.Context,
	path string,
	opts Options,
	places *placer,
	counts *Counts,
) error {
	s, err := parseSession(path)
	if err != nil {
		counts.UnreadableTranscripts++
		return nil
	}
	counts.SubagentTranscripts += s.subagents
	counts.UndecodableLines += s.stats.undecodable
	for t, n := range s.stats.unknown {
		counts.UnknownTypes[t] += n
	}
	if opts.Known != nil && !opts.Force {
		known, err := opts.Known(ctx, s.id)
		if err != nil {
			return fmt.Errorf("importer: look up session %s: %w", s.id, err)
		}
		if known {
			counts.Skipped.InStore++
			return nil
		}
	}
	edits, failed, unanswered := s.edits()
	if len(edits) == 0 {
		counts.Skipped.NoEdits++
		return nil
	}
	em := &emitter{s: s, opts: opts, places: places, counts: counts, sink: opts.Sink}
	if err := em.emit(edits); err != nil {
		return err
	}
	counts.SessionsImported++
	counts.FailedCalls += failed
	counts.UnansweredCalls += unanswered
	counts.BashCalls += s.stats.bash
	return nil
}

// edits returns the calls that changed a file, in the order they were
// made, and how many calls failed or were never answered. A call that
// names no file is no edit, as in the hook.
func (s *session) edits() (out []*call, failed, unanswered int) {
	for _, c := range s.order {
		switch {
		case !c.answered:
			unanswered++
		case c.failed:
			failed++
		case c.target() != "":
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b *call) int { return a.when().Compare(b.when()) })
	return out, failed, unanswered
}
