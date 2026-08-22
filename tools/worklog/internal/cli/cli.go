// Package cli wires the worklog commands onto the store.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/tools/worklog/internal/config"
	"github.com/mad01/thismoon/tools/worklog/internal/mcpserver"
	"github.com/mad01/thismoon/tools/worklog/internal/scan"
	"github.com/mad01/thismoon/tools/worklog/internal/store"
)

// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "worklog",
		Short:         "Resumable, ticket/topic-keyed cross-session work state",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		docsCmd(),
		newCmd(),
		checkpointCmd(),
		listCmd(),
		showCmd(),
		searchCmd(),
		statusCmd(),
		pathCmd(),
		syncCmd(),
		scanCmd(),
		configCmd(),
		mcpCmd(),
	)
	return c
}

// newStore returns the store wired with the upstream resolved from the config
// for this machine's profile, cloning it first when the store directory is
// missing (fresh machine). A failed bootstrap clone degrades to the local-only
// store with a warning — worklog must keep working offline.
func newStore() *store.Store {
	s := store.New("")
	s.Remote = resolveRemote()
	if err := s.EnsureCloned(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	return s
}

// resolveRemote maps the config's profile-keyed upstreams onto this machine.
func resolveRemote() store.Remote {
	rc := config.Load().Remote
	url := rc.ResolveUpstream(config.MachineProfiles())
	return store.Remote{URL: url, Push: url != "" && rc.PushEnabled()}
}

// warnPush downgrades a push failure to a stderr warning: the write and local
// commit already succeeded, so the command itself did not fail.
func warnPush(err error) error {
	if errors.Is(err, store.ErrPush) {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return nil
	}
	return err
}

func syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull the store's upstream (fast-forward only) and push local commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			if err := s.Sync(); err != nil {
				return err
			}
			fmt.Println("synced", s.Remote.URL)
			return nil
		},
	}
}

func scanCmd() *cobra.Command {
	var since string
	c := &cobra.Command{
		Use:   "scan",
		Short: "Digest recent Claude session transcripts as JSON (for /worklog-backfill)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			sessions, err := scan.Scan("", d, time.Now(), scan.Config(config.Load().Scan))
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(sessions)
		},
	}
	c.Flags().StringVar(&since, "since", "14d", "window: Nd (days) or a Go duration like 336h")
	return c
}

// parseSince accepts "14d" / "30d" (days) or any Go duration string.
func parseSince(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the worklog MCP stdio server for Claude Code",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.New(buildinfo.Get().Version).
				Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}

func cwd() string {
	d, _ := os.Getwd()
	return d
}

func newCmd() *cobra.Command {
	var ticket, topic string
	c := &cobra.Command{
		Use:   "new <key>",
		Short: "Create a new work item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			key := store.Sanitize(args[0])
			if s.Exists(key) {
				return fmt.Errorf("item %q already exists", key)
			}
			repo := store.DetectRepo(cwd())
			it, err := s.Checkpoint(key, store.CheckpointInput{
				Ticket: ticket, Topic: topic, Repo: repo, Cwd: cwd(),
			})
			if err = warnPush(err); err != nil {
				return err
			}
			fmt.Printf("created %s\n", it.FM.Key)
			return nil
		},
	}
	c.Flags().StringVar(&ticket, "ticket", "", "ticket id (e.g. ABC-1234)")
	c.Flags().StringVar(&topic, "topic", "", "free-text topic")
	return c
}

func checkpointCmd() *cobra.Command {
	var ticket, topic, where, note, repo string
	c := &cobra.Command{
		Use:   "checkpoint <key>",
		Short: "Append a checkpoint to a work item (creates it if missing)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			key := store.Sanitize(args[0])
			if where == "" && note == "" {
				return fmt.Errorf("nothing to record: pass --where and/or --note")
			}
			if repo == "" {
				repo = store.DetectRepo(cwd())
			}
			it, err := s.Checkpoint(key, store.CheckpointInput{
				Ticket: ticket, Topic: topic, Where: where, Note: note, Repo: repo, Cwd: cwd(),
			})
			if err = warnPush(err); err != nil {
				return err
			}
			scope := repo
			if scope == "" {
				scope = "no repo"
			}
			fmt.Printf("checkpointed %s (%s)\n", it.FM.Key, scope)
			return nil
		},
	}
	c.Flags().StringVar(&ticket, "ticket", "", "ticket id (set on first creation)")
	c.Flags().StringVar(&topic, "topic", "", "free-text topic (set on first creation)")
	c.Flags().StringVar(&where, "where", "", "replace the \"Where I am\" snapshot")
	c.Flags().StringVar(&note, "note", "", "append a log entry")
	c.Flags().StringVar(&repo, "repo", "", "repo this touched (default: auto-detect from cwd)")
	return c
}

func listCmd() *cobra.Command {
	var status, repo string
	c := &cobra.Command{
		Use:   "list",
		Short: "List work items (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			items, err := s.List(status, repo)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("no items")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			// Writes to the tabwriter are buffered; the real error surfaces at Flush.
			fmt.Fprintln(w, "KEY\tSTATUS\tUPDATED\tREPOS")
			for _, it := range items {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					it.FM.Key, it.FM.Status,
					it.FM.Updated.Local().Format("2006-01-02 15:04"),
					strings.Join(it.FM.Repos, ","))
			}
			return w.Flush()
		},
	}
	c.Flags().StringVar(&status, "status", "", "filter by status (active|paused|done)")
	c.Flags().StringVar(&repo, "repo", "", "filter to items touching this repo")
	return c
}

func showCmd() *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:   "show <key>",
		Short: "Print a work item's CONTEXT.md (or a repo note with --repo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			key := store.Sanitize(args[0])
			if repo != "" {
				note, err := s.RepoNote(key, repo)
				if err != nil {
					return fmt.Errorf("no notes for repo %q on %q", repo, key)
				}
				fmt.Print(note)
				return nil
			}
			it, err := s.Load(key)
			if err != nil {
				return fmt.Errorf("no item %q", key)
			}
			b, err := it.Render()
			if err != nil {
				return err
			}
			fmt.Print(string(b))
			return nil
		},
	}
	c.Flags().StringVar(&repo, "repo", "", "print this repo's note file instead")
	return c
}

func searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search items by key and content (active first)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			hits, err := s.Search(strings.Join(args, " "))
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				fmt.Println("no matches")
				return nil
			}
			for _, it := range hits {
				fmt.Printf("%-24s %-7s %s\n", it.FM.Key, it.FM.Status,
					it.FM.Updated.Local().Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <key> <active|paused|done>",
		Short: "Set a work item's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st := args[1]
			if st != "active" && st != "paused" && st != "done" {
				return fmt.Errorf("status must be active, paused, or done")
			}
			it, err := newStore().SetStatus(store.Sanitize(args[0]), st)
			if err = warnPush(err); err != nil {
				return err
			}
			fmt.Printf("%s -> %s\n", it.FM.Key, it.FM.Status)
			return nil
		},
	}
}

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path [key]",
		Short: "Print the store root, or an item's directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := newStore()
			if len(args) == 0 {
				fmt.Println(s.Root)
				return nil
			}
			fmt.Println(s.ItemDir(store.Sanitize(args[0])))
			return nil
		},
	}
}
