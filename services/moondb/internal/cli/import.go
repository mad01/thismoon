package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/importer"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var (
	importSince       string
	importProjects    string
	importFileHistory string
	importDryRun      bool
	importForce       bool
	importJSON        bool
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import past sessions into the store from their transcripts",
	Long: `Read the Claude Code session transcripts written within --since and spool
what the hooks would have recorded: one turn per prompt, one edit per Edit,
Write, MultiEdit and NotebookEdit call. serve ingests the spool as it does the
hooks' envelopes, so it must be running for the import to reach the store.

A session the store already holds was captured live and is skipped; --force
imports it anyway. A forced import only fills: an edit the store holds stays
as it is, a session or turn keeps every field it has and gets only its empty
fields filled, and a turn still open is closed. An earlier imported prompt
still moves a start time back. Run --force only while no session is active,
because it closes a session's in-flight turn until the live Stop arrives.
Sessions without a file edit are skipped.

A transcript records less than the hooks see: no disk read after an edit (so
no unattributed delta), no HEAD (so no commit link until a later live turn
moves HEAD), and no diff for Bash commands, which are not imported. A file
over about 10 KB has no original in the transcript; its content before the
edit comes from the session's file history when the history holds it.

--since takes an RFC 3339 time or a span before now: 90m, 24h, 30d.
--dry-run reads and counts everything and writes nothing. With serve down it
goes on without the store, so it skips no session as already held and knows
no repos of deleted worktrees.`,
	Args: cobra.NoArgs,
	RunE: runImport,
}

func init() {
	f := importCmd.Flags()
	f.StringVar(&importSince, "since", moondb.DefaultImportSince,
		"read transcripts written after this time or span")
	f.StringVar(&importProjects, "projects", moondb.DefaultProjectsDir,
		"the harness's transcript directory, one subdirectory per project")
	f.StringVar(&importFileHistory, "file-history", moondb.DefaultFileHistoryDir,
		"the harness's file-history directory, one subdirectory per session")
	f.BoolVar(&importDryRun, "dry-run", false, "parse and count without writing the spool")
	f.BoolVar(&importForce, "force", false, "import sessions the store already holds")
	f.BoolVar(&importJSON, "json", false, "print the counts as JSON")
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	opts, err := importOptions(ctx, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	start := time.Now()
	counts, err := importer.Run(ctx, opts)
	counts.ElapsedMS = time.Since(start).Milliseconds()
	if err != nil {
		return err
	}
	if importJSON {
		return printJSON(cmd.OutOrStdout(), counts)
	}
	printImport(cmd.OutOrStdout(), counts, importDryRun)
	return nil
}

// importOptions resolves the flags: the window, the harness directories,
// the store lookups that skip live-captured sessions and name the repos of
// deleted worktrees, and the spool unless this is a dry run. A dry run goes
// on without the lookups when serve does not answer, and says so on warn.
func importOptions(ctx context.Context, warn io.Writer) (importer.Options, error) {
	since, err := server.ParseSince(importSince, time.Now())
	if err != nil {
		return importer.Options{}, err
	}
	projects, err := confdir.Expand(importProjects)
	if err != nil {
		return importer.Options{}, err
	}
	history, err := confdir.Expand(importFileHistory)
	if err != nil {
		return importer.Options{}, err
	}
	c := client.New(apiURL())
	opts := importer.Options{
		Projects:    projects,
		FileHistory: history,
		Since:       since,
		Force:       importForce,
		Excludes:    cfg.Capture.excludes(),
	}
	if importDryRun && c.Health(ctx) != nil {
		fmt.Fprintf(warn, "serve at %s did not answer: skip information and known repos "+
			"unavailable, so no session counts as already held\n", apiURL())
		return opts, nil
	}
	opts.Known = sessionKnown(c)
	opts.Locations = knownLocations(c)
	if !importDryRun {
		opts.Sink = &spool.Writer{
			Dir:     filepath.Join(flagWorkdir, moondb.SpoolDirName),
			Objects: filepath.Join(flagWorkdir, moondb.ObjectsDirName),
		}
	}
	return opts, nil
}

// sessionKnown reports whether the store holds a session.
func sessionKnown(c *client.Client) func(context.Context, string) (bool, error) {
	return func(ctx context.Context, id string) (bool, error) {
		_, err := c.Session(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return err == nil, err
	}
}

// everyEdit is an Edits limit past any store's size: knownLocations needs
// every edit's location.
const everyEdit = 1 << 30

// knownLocations lists the distinct locations of the edits the store holds
// that name a repository.
func knownLocations(c *client.Client) func(context.Context) ([]store.Location, error) {
	return func(ctx context.Context) ([]store.Location, error) {
		edits, err := c.Edits(ctx, client.EditQuery{Limit: everyEdit})
		if err != nil {
			return nil, err
		}
		seen := map[store.Location]bool{}
		var known []store.Location
		for _, e := range edits {
			l := store.Location{Repo: e.Repo, Worktree: e.Worktree, Canonical: e.Canonical}
			if l.Repo != "" && !seen[l] {
				seen[l] = true
				known = append(known, l)
			}
		}
		return known, nil
	}
}

// printImport renders the counts as a two-column table.
func printImport(w io.Writer, c importer.Counts, dryRun bool) {
	if dryRun {
		fmt.Fprintln(w, "dry run: nothing written")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	rows := [][2]string{
		{"transcripts", fmt.Sprintf("%d (%d subagent, %d unreadable)",
			c.Transcripts, c.SubagentTranscripts, c.UnreadableTranscripts)},
		{"sessions imported", fmtCount(c.SessionsImported)},
		{"sessions skipped", fmt.Sprintf("%d in store, %d outside --since, %d without edits",
			c.Skipped.InStore, c.Skipped.OutsideSince, c.Skipped.NoEdits)},
		{"turns", fmtCount(c.Turns)},
		{"edits", tally(c.Edits)},
		{"edits without a patch", fmtCount(c.EditsWithoutPatch)},
		{"originals from file history", fmtCount(c.OriginalsFromHistory)},
		{"files placed", fmt.Sprintf(
			"%d live, %d by ancestor, %d by worktree layout, %d by cwd, %d norepo",
			c.Placement.Live, c.Placement.Ancestor, c.Placement.Layout, c.Placement.Cwd,
			c.Placement.NoRepo)},
		{"placed without a repo", fmtCount(c.Placement.WithoutRepo)},
		{"calls not imported", fmt.Sprintf("%d failed, %d unanswered, %d bash",
			c.FailedCalls, c.UnansweredCalls, c.BashCalls)},
		{"envelopes", fmtCount(c.Envelopes)},
		{"undecodable lines", fmtCount(c.UndecodableLines)},
		{"unknown entry types", tally(c.UnknownTypes)},
		{"elapsed", (time.Duration(c.ElapsedMS) * time.Millisecond).String()},
	}
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\n", r[0], r[1])
	}
	_ = tw.Flush()
	if !dryRun && c.Envelopes > 0 {
		fmt.Fprintln(
			w,
			"spooled; moondb serve ingests the spool (moondb doctor checks it is running)",
		)
	}
}

// tally renders a count per name, sorted by name; "none" when empty.
func tally(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = fmt.Sprintf("%s %d", name, m[name])
	}
	return strings.Join(parts, ", ")
}
