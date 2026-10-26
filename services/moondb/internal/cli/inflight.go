package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/server"
)

// inflightTitleRunes caps a session title in the SESSIONS column.
const inflightTitleRunes = 40

var (
	inflightWorktree string
	inflightExclude  string
	inflightSince    string
	inflightLimit    int
	inflightJSON     bool
)

var inflightCmd = &cobra.Command{
	Use:   "inflight",
	Short: "List the files other sessions edited in a checkout that no commit has taken yet",
	Long: fmt.Sprintf(`List the files of one checkout with edits no commit has taken yet, newest
edit first, with the sessions behind them. An edit is in flight while
neither it nor a later edit of the same file has a link from a commit made
in this checkout. COMMIT is the commit that took the file's newest
committed edit, a dash when none.

The checkout is the one the current directory sits in unless --worktree
names one. --exclude-session leaves one session's own edits out, the way
a session asks about everyone else's. --since bounds the edits read (an
RFC 3339 time or a span before now: 90m, 24h, 7d; default %dh) and
--limit the files (default %d).

A file a person committed or reverted outside any session stays listed
until the window passes: moondb only links a commit a session saw land.
So does a file whose last edit left no content to match a commit by, even
when a session saw the commit: a delete, a path capture.exclude matches, a
NotebookEdit, or a file over %d MiB. Check git status before acting on one.`,
		int(moondb.InflightWindow.Hours()), moondb.InflightLimit, moondb.MaxChunkBytes>>20),
	Args: cobra.NoArgs,
	RunE: runInflight,
}

func init() {
	inflightCmd.Flags().
		StringVar(&inflightWorktree, "worktree", "", "checkout root (default: the current directory's)")
	inflightCmd.Flags().
		StringVar(&inflightExclude, "exclude-session", "", "session id whose own edits never count")
	inflightCmd.Flags().
		StringVar(&inflightSince, "since", "", "read edits since this time or span (default: the server's)")
	inflightCmd.Flags().
		IntVar(&inflightLimit, "limit", 0, "most files to list (default: the server's)")
	inflightCmd.Flags().BoolVar(&inflightJSON, "json", false, "print the API answer as JSON")
	rootCmd.AddCommand(inflightCmd)
}

func runInflight(cmd *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	worktree, err := inflightCheckout(ctx, inflightWorktree)
	if err != nil {
		return err
	}
	v, err := client.New(apiURL()).Inflight(ctx, client.InflightQuery{
		Worktree:       worktree,
		ExcludeSession: inflightExclude,
		Since:          inflightSince,
		Limit:          inflightLimit,
	})
	if err != nil {
		return err
	}
	if inflightJSON {
		return printJSON(cmd.OutOrStdout(), v)
	}
	printInflight(cmd.OutOrStdout(), v)
	return nil
}

// inflightCheckout is the checkout root to ask about: the flag when set,
// else the root of the checkout the current directory sits in, resolved the
// way record resolves an edit's worktree.
func inflightCheckout(ctx context.Context, flag string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("inflight: current directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, gitLookup)
	defer cancel()
	if root := gitloc.LocationAt(ctx, cwd).Toplevel; root != "" {
		return root, nil
	}
	return "", errors.New("inflight: the current directory is in no git checkout; pass --worktree")
}

// printInflight renders the in-flight files as a table, one row per file
// with its sessions, the latest first.
func printInflight(w io.Writer, v server.InflightView) {
	if len(v.Files) == 0 {
		fmt.Fprintf(w, "no files in %s have edits no commit has taken yet\n", v.Worktree)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PATH\tLAST EDIT\tCOMMIT\tSESSIONS")
	for _, f := range v.Files {
		sessions := make([]string, 0, len(f.Sessions))
		for _, s := range f.Sessions {
			sessions = append(sessions, fmt.Sprintf("%s %s (%d)",
				short(s.ID), firstLine(s.Title, inflightTitleRunes), s.Edits))
		}
		commit := "-"
		if f.LinkedCommit != nil {
			commit = short(*f.LinkedCommit)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			f.Path, when(f.LastEditAt), commit, strings.Join(sessions, ", "))
	}
	_ = tw.Flush()
}
