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
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/tools/worklog"
	"github.com/mad01/thismoon/tools/worklog/internal/config"
	"github.com/mad01/thismoon/tools/worklog/internal/mcpserver"
	"github.com/mad01/thismoon/tools/worklog/internal/scan"
	"github.com/mad01/thismoon/tools/worklog/internal/store"
)

// Execute runs the root command.
func Execute() error { return root().Execute() }

// app carries what the root command resolves once for every subcommand: where
// the config file lives. Subcommands hang off it so nothing reads a package
// -level flag variable.
type app struct{ configPath string }

func root() *cobra.Command {
	a := &app{}
	c := &cobra.Command{
		Use:           "worklog",
		Short:         "Resumable, ticket/topic-keyed cross-session work state",
		Long:          rootLong(),
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.PersistentFlags().StringVar(&a.configPath, "config",
		envdefault.String("WORKLOG_CONFIG", ""),
		"config file (default "+defaultConfigPath()+")")
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(worklog.OperatingDoc, worklog.Facts()),
		a.newCmd(),
		a.checkpointCmd(),
		a.listCmd(),
		a.showCmd(),
		a.searchCmd(),
		a.statusCmd(),
		a.pathCmd(),
		a.syncCmd(),
		a.scanCmd(),
		a.configCmd(),
		a.mcpCmd(),
	)
	return c
}

// rootLong names the two directories worklog reads and writes, so `worklog
// --help` answers "where does this keep things" without a source dive.
func rootLong() string {
	return fmt.Sprintf(`worklog keeps the state of a long, cross-repo task somewhere a later
session can find it, keyed by ticket id or topic rather than by working
directory.

  store   %s (override with $WORKLOG_DIR)
  config  %s (override with --config or $WORKLOG_CONFIG)

'worklog config' prints the settings in effect; 'worklog docs' prints the
operating doc.`, worklog.DefaultRoot, defaultConfigPath())
}

// defaultConfigPath is the compiled default for help text. A home directory
// that will not resolve is a real error everywhere it matters, but help text
// is not one of those places, so it falls back to the conventional spelling.
func defaultConfigPath() string {
	p, err := config.Path("")
	if err != nil {
		return "~/.config/" + config.Component + "/" + config.FileName
	}
	return p
}

// config loads the config file this invocation was pointed at.
func (a *app) config() (config.Config, error) {
	path, err := config.Path(a.configPath)
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(path)
}

// store returns the store wired with the configured upstream, cloning it
// first when the store directory is missing (fresh machine). A failed
// bootstrap clone degrades to the local-only store with a warning — worklog
// must keep working offline.
func (a *app) store() (*store.Store, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, err
	}
	s, err := store.NewDefault()
	if err != nil {
		return nil, err
	}
	s.Remote = remote(cfg.Remote)
	if err := s.EnsureCloned(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	return s, nil
}

// remote maps the config's remote section onto the store's, warning when the
// file still keys its upstream by machine profile: that form is no longer
// read, and the store would go local-only without saying so.
func remote(rc config.Remote) store.Remote {
	if rc.RetiredUpstreams() {
		fmt.Fprintln(
			os.Stderr,
			"warning: remote.upstreams is no longer read; set remote.url to the upstream for this machine",
		)
	}
	return store.Remote{URL: rc.URL, Push: rc.URL != "" && rc.PushEnabled()}
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

func (a *app) syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Pull the store's upstream (fast-forward only) and push local commits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
			if err := s.Sync(); err != nil {
				return err
			}
			fmt.Println("synced", s.Remote.URL)
			return nil
		},
	}
}

func (a *app) scanCmd() *cobra.Command {
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
			cfg, err := a.config()
			if err != nil {
				return err
			}
			sessions, err := scan.Scan("", d, time.Now(), scan.Config(cfg.Scan))
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

func (a *app) mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the worklog MCP stdio server for Claude Code",
		Long: `Start the worklog MCP stdio server. The process reads the same config file
the CLI does, so --config and $WORKLOG_CONFIG apply to the registered command.

` + agentdoc.RegistrationSnippet(worklog.Facts()),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path(a.configPath)
			if err != nil {
				return err
			}
			return mcpserver.New(buildinfo.Get().Version, path).
				Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}

func cwd() string {
	d, _ := os.Getwd()
	return d
}

func (a *app) newCmd() *cobra.Command {
	var ticket, topic string
	c := &cobra.Command{
		Use:   "new <key>",
		Short: "Create a new work item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
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

func (a *app) checkpointCmd() *cobra.Command {
	var ticket, topic, where, note, repo string
	c := &cobra.Command{
		Use:   "checkpoint <key>",
		Short: "Append a checkpoint to a work item (creates it if missing)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
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

func (a *app) listCmd() *cobra.Command {
	var status, repo string
	c := &cobra.Command{
		Use:   "list",
		Short: "List work items (newest first)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
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

func (a *app) showCmd() *cobra.Command {
	var repo string
	c := &cobra.Command{
		Use:   "show <key>",
		Short: "Print a work item's CONTEXT.md (or a repo note with --repo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
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

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search items by key and content (active first)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
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

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <key> <active|paused|done>",
		Short: "Set a work item's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st := args[1]
			if st != "active" && st != "paused" && st != "done" {
				return fmt.Errorf("status must be active, paused, or done")
			}
			s, err := a.store()
			if err != nil {
				return err
			}
			it, err := s.SetStatus(store.Sanitize(args[0]), st)
			if err = warnPush(err); err != nil {
				return err
			}
			fmt.Printf("%s -> %s\n", it.FM.Key, it.FM.Status)
			return nil
		},
	}
}

func (a *app) pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path [key]",
		Short: "Print the store root, or an item's directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.store()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				fmt.Println(s.Root)
				return nil
			}
			fmt.Println(s.ItemDir(store.Sanitize(args[0])))
			return nil
		},
	}
}
