package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alpkeskin/gotoon"
	fuzzyfinder "github.com/ktr0731/go-fuzzyfinder"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

var (
	repoListFlag    bool
	repoJSONFlag    bool
	repoToonFlag    bool
	repoSkippedFlag bool
)

var repoCmd = &cobra.Command{
	Use:   "repo [query]",
	Short: "Interactive git repository finder",
	Long: "Scan configured directories for git repositories and pick one with a fuzzy finder.\n\n" +
		"With no query, opens an interactive picker.\n" +
		"With a query, prints the path of the single matching repo (case-insensitive substring on org/repo).\n" +
		"Errors out non-zero if the query matches zero or multiple repos.\n\n" +
		"--skipped lists the repos discovery dropped instead, one per line as " +
		"name, path and reason, so a repo missing from search says which setting hid it.",
	Args: cobra.MaximumNArgs(1),
	RunE: runRepo,
}

func init() {
	repoCmd.Flags().BoolVar(&repoListFlag, "list", false, "list all repos (non-interactive)")
	repoCmd.Flags().BoolVar(&repoJSONFlag, "json", false, "output as JSON (implies --list)")
	repoCmd.Flags().
		BoolVar(&repoToonFlag, "toon", false, "output as TOON for LLMs (implies --list)")
	repoCmd.Flags().
		BoolVar(&repoSkippedFlag, "skipped", false,
			"list the repos discovery dropped, and why (implies --list)")
	rootCmd.AddCommand(repoCmd)
}

// repoJSON is one repo in machine-readable output. Reason is set only for
// --skipped, where it names the setting that dropped the repo.
type repoJSON struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Remote string `json:"remote,omitempty"`
	Host   string `json:"host,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func rowOf(r finder.Repo) repoJSON {
	return repoJSON{Name: r.Name, Path: r.Path, Remote: r.Remote, Host: r.Host}
}

func rowOfDropped(d finder.Dropped) repoJSON {
	row := rowOf(d.Repo)
	row.Reason = d.Reason()
	return row
}

// toonFields is the row as gotoon wants it: a plain map, since gotoon keys on
// the raw json tag ("remote,omitempty") and does not flatten structs.
func (r repoJSON) toonFields() map[string]any {
	m := map[string]any{"name": r.Name, "path": r.Path, "remote": r.Remote, "host": r.Host}
	if r.Reason != "" {
		m["reason"] = r.Reason
	}
	return m
}

func runRepo(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	repos, dropped, err := cfg.DiscoverReposReport()
	if err != nil {
		return err
	}
	var query string
	if len(args) == 1 {
		query = args[0]
	}

	if repoSkippedFlag {
		return runRepoSkipped(cmd, cfg, repos, dropped, query)
	}

	nonInteractive := repoJSONFlag || repoToonFlag || repoListFlag
	if len(repos) == 0 {
		// A machine with no config file has nothing to list yet, which is the
		// state csl starts in rather than a failure: the machine-readable
		// modes print their empty form and the hint goes to stderr, so a
		// consumer parsing stdout still gets valid output. A config that IS
		// present and still yields nothing is a misconfiguration, and stays
		// an error.
		if cfg.Loaded {
			return errors.New(cfg.EmptyDiscoveryHint(dropped))
		}
		fmt.Fprintln(cmd.ErrOrStderr(), cfg.EmptyDiscoveryHint(dropped))
		if !nonInteractive {
			return nil
		}
	}
	if query != "" {
		repos = filterByName(repos, func(r finder.Repo) string { return r.Name }, query)
	}
	if !nonInteractive && query != "" {
		return printSingleMatch(cmd, repos, query)
	}
	if nonInteractive {
		rows := make([]repoJSON, len(repos))
		for i, r := range repos {
			rows[i] = rowOf(r)
		}
		return writeRows(cmd, "repos", rows)
	}
	return pickRepo(cmd, repos)
}

// runRepoSkipped answers the opposite question from the rest of the command:
// not "where is this repo" but "why is it missing". It runs ahead of the
// empty-list handling because the machine where every repo was dropped is the
// one that most needs the answer; a machine that discovered nothing at all
// gets the same hint --list prints, so the documented "run this when a repo is
// missing" never comes back silent.
func runRepoSkipped(
	cmd *cobra.Command,
	cfg *config.Config,
	repos []finder.Repo,
	dropped []finder.Dropped,
	query string,
) error {
	if len(repos) == 0 && len(dropped) == 0 {
		if cfg.Loaded {
			return errors.New(cfg.EmptyDiscoveryHint(dropped))
		}
		fmt.Fprintln(cmd.ErrOrStderr(), cfg.EmptyDiscoveryHint(dropped))
	}
	if query != "" {
		dropped = filterByName(dropped, func(d finder.Dropped) string { return d.Repo.Name }, query)
	}
	rows := make([]repoJSON, len(dropped))
	for i, d := range dropped {
		rows[i] = rowOfDropped(d)
	}
	return writeRows(cmd, "skipped", rows)
}

// printSingleMatch prints the one repo a query resolved to, or says why it
// did not: the interactive contract `cd $(csl repo <query>)` relies on.
func printSingleMatch(cmd *cobra.Command, repos []finder.Repo, query string) error {
	switch len(repos) {
	case 1:
		fmt.Fprintln(cmd.OutOrStdout(), repos[0].Path)
		return nil
	case 0:
		return fmt.Errorf("no repos match query %q", query)
	default:
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.Name
		}
		return fmt.Errorf(
			"multiple repos match query %q:\n  - %s",
			query,
			strings.Join(names, "\n  - "),
		)
	}
}

// pickRepo opens the fuzzy finder over repos and prints the chosen path; an
// aborted picker is not an error.
func pickRepo(cmd *cobra.Command, repos []finder.Repo) error {
	idx, err := fuzzyfinder.Find(repos, func(i int) string {
		return repos[i].Name + " @ " + repos[i].Path
	})
	if err != nil {
		if errors.Is(err, fuzzyfinder.ErrAbort) {
			return nil
		}
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), repos[idx].Path)
	return nil
}

// writeRows renders rows in whichever machine-readable form the flags asked
// for: a JSON array, TOON under toonKey, or tab-separated lines (name, path,
// and the reason when there is one) that pipe into cut and awk. Both the repo
// list and the skipped list go through here, so the two shapes cannot drift.
func writeRows(cmd *cobra.Command, toonKey string, rows []repoJSON) error {
	w := cmd.OutOrStdout()
	switch {
	case repoJSONFlag:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case repoToonFlag:
		items := make([]map[string]any, len(rows))
		for i, r := range rows {
			items[i] = r.toonFields()
		}
		encoded, err := gotoon.Encode(map[string]any{toonKey: items})
		if err != nil {
			return err
		}
		fmt.Fprintln(w, encoded)
		return nil
	}
	for _, r := range rows {
		if r.Reason != "" {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Path, r.Reason)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\n", r.Name, r.Path)
	}
	return nil
}

// filterByName returns the items whose name contains query, case-insensitive:
// the one matching rule `csl repo <query>` and `--skipped <query>` share.
func filterByName[T any](items []T, name func(T) string, query string) []T {
	q := strings.ToLower(query)
	out := items[:0:0]
	for _, it := range items {
		if strings.Contains(strings.ToLower(name(it)), q) {
			out = append(out, it)
		}
	}
	return out
}
