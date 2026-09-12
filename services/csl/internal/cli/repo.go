package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alpkeskin/gotoon"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/picker"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

var (
	repoListFlag      bool
	repoJSONFlag      bool
	repoToonFlag      bool
	repoSkippedFlag   bool
	repoComponentFlag string
	repoOwnerFlag     string
	repoSystemFlag    string
)

var repoCmd = &cobra.Command{
	Use:   "repo [query]",
	Short: "Interactive git repository finder",
	Long: "Scan configured directories for git repositories and pick one with a fuzzy finder.\n\n" +
		"With no query, opens an interactive picker.\n" +
		"With a query, prints the path of the single matching repo (case-insensitive substring on org/repo).\n" +
		"Errors out non-zero if the query matches zero or multiple repos.\n\n" +
		"--component, --owner, and --system narrow the set to repos whose catalog descriptor\n" +
		"(catalog-info.yaml or service-info.yaml at the root, a Backstage-shaped Component) has a\n" +
		"matching metadata.name, spec.owner, or spec.system (case-insensitive substring; all set\n" +
		"filters must match). A repo whose descriptor is elsewhere points at it with a root\n" +
		".csl-catalog.yaml holding `descriptor: <repo-relative path>`. The filters compose with the\n" +
		"query and with --list; on their own they open the picker over the narrowed set, or print\n" +
		"the path straight away when one repo is left.\n\n" +
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
	repoCmd.Flags().StringVar(&repoComponentFlag, "component", "",
		"only repos whose catalog descriptor names this component (metadata.name)")
	repoCmd.Flags().StringVar(&repoOwnerFlag, "owner", "",
		"only repos whose catalog descriptor has this owner (spec.owner)")
	repoCmd.Flags().StringVar(&repoSystemFlag, "system", "",
		"only repos whose catalog descriptor is in this system (spec.system)")
	rootCmd.AddCommand(repoCmd)
}

// repoJSON is one repo in machine-readable output. Component, Owner, and
// System come from the repo's root catalog descriptor and are absent when it
// has none. Reason is set only for --skipped, where it names the setting that
// dropped the repo.
type repoJSON struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Remote    string `json:"remote,omitempty"`
	Host      string `json:"host,omitempty"`
	Component string `json:"component,omitempty"`
	Owner     string `json:"owner,omitempty"`
	System    string `json:"system,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func rowOf(r finder.Repo) repoJSON {
	row := repoJSON{Name: r.Name, Path: r.Path, Remote: r.Remote, Host: r.Host}
	if c := r.Catalog; c != nil {
		row.Component, row.Owner, row.System = c.Name, c.Owner, c.System
	}
	return row
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
	for k, v := range map[string]string{
		"component": r.Component, "owner": r.Owner, "system": r.System, "reason": r.Reason,
	} {
		if v != "" {
			m[k] = v
		}
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
	q := finder.Query{
		Name:      query,
		Component: repoComponentFlag,
		Owner:     repoOwnerFlag,
		System:    repoSystemFlag,
	}

	if repoSkippedFlag {
		return runRepoSkipped(cmd, cfg, repos, dropped, q)
	}

	nonInteractive := repoJSONFlag || repoToonFlag || repoListFlag
	if len(repos) == 0 {
		if err := reportEmptyDiscovery(cmd, cfg, dropped); err != nil || !nonInteractive {
			return err
		}
	}
	repos, err = filterRepos(repos, q)
	if err != nil {
		return err
	}
	if nonInteractive {
		rows := make([]repoJSON, len(repos))
		for i, r := range repos {
			rows[i] = rowOf(r)
		}
		return writeRows(cmd, "repos", rows)
	}
	// A positional query resolves to one repo, the `cd $(csl repo x)`
	// contract. Catalog filters alone narrow the picker instead, since
	// "the platform team's repos" is a set to choose from; with one member
	// there is nothing to choose and it prints straight away, and with none
	// it errors like a query would rather than opening an empty picker.
	if query != "" || (!q.IsZero() && len(repos) <= 1) {
		return printSingleMatch(cmd, repos, q)
	}
	return pickRepo(cmd, repos)
}

// reportEmptyDiscovery handles a discovery that found nothing. A machine with
// no config file has nothing to list yet, which is the state csl starts in
// rather than a failure: the hint goes to stderr and nil comes back, so a
// machine-readable mode still prints its valid empty form on stdout. A
// config that IS present and still yields nothing is a misconfiguration, and
// stays an error.
func reportEmptyDiscovery(cmd *cobra.Command, cfg *config.Config, dropped []finder.Dropped) error {
	if cfg.Loaded {
		return errors.New(cfg.EmptyDiscoveryHint(dropped))
	}
	fmt.Fprintln(cmd.ErrOrStderr(), cfg.EmptyDiscoveryHint(dropped))
	return nil
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
	q finder.Query,
) error {
	if len(repos) == 0 && len(dropped) == 0 {
		if cfg.Loaded {
			return errors.New(cfg.EmptyDiscoveryHint(dropped))
		}
		fmt.Fprintln(cmd.ErrOrStderr(), cfg.EmptyDiscoveryHint(dropped))
	}
	if !q.IsZero() {
		match, err := q.SubstringMatcher()
		if err != nil {
			return err
		}
		dropped = filterBy(dropped, func(d finder.Dropped) finder.Repo { return d.Repo }, match)
	}
	rows := make([]repoJSON, len(dropped))
	for i, d := range dropped {
		rows[i] = rowOfDropped(d)
	}
	return writeRows(cmd, "skipped", rows)
}

// printSingleMatch prints the one repo a query resolved to, or says why it
// did not: the interactive contract `cd $(csl repo <query>)` relies on.
func printSingleMatch(cmd *cobra.Command, repos []finder.Repo, q finder.Query) error {
	switch len(repos) {
	case 1:
		fmt.Fprintln(cmd.OutOrStdout(), repos[0].Path)
		return nil
	case 0:
		return fmt.Errorf("no repos match query %q", q)
	default:
		names := make([]string, len(repos))
		for i, r := range repos {
			names[i] = r.Name
		}
		return fmt.Errorf(
			"multiple repos match query %q:\n  - %s",
			q,
			strings.Join(names, "\n  - "),
		)
	}
}

// pickRepo opens the fuzzy picker over repos and prints the chosen path; an
// aborted picker is not an error.
func pickRepo(cmd *cobra.Command, repos []finder.Repo) error {
	items := make([]picker.Item, len(repos))
	for i, r := range repos {
		items[i] = pickerItem(r)
	}
	idx, err := picker.Pick(items)
	if err != nil {
		if errors.Is(err, picker.ErrAbort) {
			return nil
		}
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), repos[idx].Path)
	return nil
}

// pickerItem is the picker line for a repo: the org/repo name, then the
// catalog identity in its own colors so it reads apart from the name, then
// the path. The component name appears only when it differs from the repo
// name, since repeating "csl" after "mad01/csl" says nothing; the owner and
// system carry a dim label so the line still reads without color.
func pickerItem(r finder.Repo) picker.Item {
	item := picker.Item{{Text: r.Name}}
	if c := r.Catalog; c != nil {
		if c.Name != "" && !strings.EqualFold(c.Name, repoShortName(r.Name)) {
			item = append(
				item,
				picker.Segment{Text: "  "},
				picker.Segment{Text: c.Name, Color: picker.Cyan},
			)
		}
		if c.Owner != "" {
			item = append(item,
				picker.Segment{Text: "  owner:", Color: picker.Dim},
				picker.Segment{Text: c.Owner, Color: picker.Magenta})
		}
		if c.System != "" {
			item = append(item,
				picker.Segment{Text: "  system:", Color: picker.Dim},
				picker.Segment{Text: c.System, Color: picker.Blue})
		}
	}
	return append(item, picker.Segment{Text: " @ " + r.Path, Color: picker.Dim})
}

// repoShortName is the repo part of an org/repo name without a worktree's
// "@branch" suffix: what a catalog name is compared against to decide
// whether it adds anything to the line.
func repoShortName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name, _, _ = strings.Cut(name, "@")
	return name
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

// filterRepos keeps the repos that satisfy q under the command's substring
// rule, and all of them for a zero query.
func filterRepos(repos []finder.Repo, q finder.Query) ([]finder.Repo, error) {
	if q.IsZero() {
		return repos, nil
	}
	match, err := q.SubstringMatcher()
	if err != nil {
		return nil, err
	}
	return filterBy(repos, func(r finder.Repo) finder.Repo { return r }, match), nil
}

// filterBy returns the items whose repo satisfies match: the one filtering
// rule `csl repo <query>` and `--skipped <query>` share.
func filterBy[T any](items []T, repo func(T) finder.Repo, match finder.Matcher) []T {
	out := items[:0:0]
	for _, it := range items {
		if match(repo(it)) {
			out = append(out, it)
		}
	}
	return out
}
