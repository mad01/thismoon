package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

var (
	doctorJSONFlag   bool
	doctorRepairFlag bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check search index health",
	Long: `Check the health of the search index.

Reports issues (stale, missing, or dirty indexes), lists healthy repos, and
prints the build metadata of the csl binary doing the checking — the same
version, commit, tag and build time 'csl version -o json' reports, so a
diagnosis names the build it came from.
Use --json for machine-readable output.
Use --repair to fix a corrupt state file (backs up the old file and resets state).`,
	RunE: runDoctor,
}

// doctorDocument is the --json document: the index report with the build
// metadata of the binary that produced it nested under "build". DoctorReport is
// embedded, so its own keys stay at the top level where consumers expect them.
type doctorDocument struct {
	search.DoctorReport
	Build buildinfo.Info `json:"build"`
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSONFlag, "json", false, "output as JSON")
	doctorCmd.Flags().
		BoolVar(&doctorRepairFlag, "repair", false, "fix corrupt state file and continue")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
	if err != nil {
		return err
	}

	state, stateErr := search.LoadState(indexDir)
	if stateErr != nil && !doctorRepairFlag {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "State file corrupt: %v\n", stateErr)
		fmt.Fprintln(
			w,
			"Run 'csl doctor --repair' to back up the corrupt file and reset state.",
		)
		return stateErr
	}
	if stateErr != nil {
		// --repair: LoadState already backed up the corrupt file; start fresh.
		state = search.EmptyState()
		if err := state.Save(indexDir); err != nil {
			return fmt.Errorf("failed to write repaired state: %w", err)
		}
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"Repaired: reset state file (corrupt backup saved).\n\n",
		)
	}

	staleness, err := search.CheckStaleness(repos, state)
	if err != nil {
		return err
	}

	indexSize, _ := search.IndexDirSize(indexDir)

	// Validate shard integrity.
	shards, corrupted, _ := search.ValidateShards(indexDir)

	report := search.DoctorReport{
		IndexDir:       indexDir,
		IndexSizeBytes: indexSize,
		TotalRepos:     len(repos),
	}

	// Classify stale repos into issues.
	for _, r := range staleness.Stale {
		rs, indexed := state.GetRepo(r.Path)

		issue := search.RepoIssue{
			Repo: r.Name,
			Path: r.Path,
		}

		if !indexed {
			issue.Type = "missing"
			issue.Message = "not indexed"
		} else {
			fp, ok := staleness.Current[r.Path]
			if ok && fp.Dirty {
				issue.Type = "dirty"
				issue.Dirty = true
				mod, untracked, _ := search.DirtyInfo(r.Path)
				issue.ModifiedFiles = mod
				issue.UntrackedFiles = untracked
				issue.Message = fmt.Sprintf("%d modified, %d untracked", mod, untracked)
			} else {
				issue.Type = "stale"
				if !rs.IndexedAt.IsZero() {
					issue.IndexAge = time.Since(rs.IndexedAt).Truncate(time.Second).String()
				}
				issue.Message = "index out of date"
			}
		}

		report.Issues = append(report.Issues, issue)
	}

	// Classify fresh repos as healthy.
	for _, r := range staleness.Fresh {
		rs, _ := state.GetRepo(r.Path)
		age := "-"
		if !rs.IndexedAt.IsZero() {
			age = time.Since(rs.IndexedAt).Truncate(time.Second).String()
		}
		report.Healthy = append(report.Healthy, search.RepoHealth{
			Repo:     r.Name,
			Path:     r.Path,
			IndexAge: age,
		})
	}

	w := cmd.OutOrStdout()
	build := buildinfo.Get()

	if doctorJSONFlag {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(doctorDocument{DoctorReport: report, Build: build})
	}

	// Daemon status
	daemonStatus := "stopped"
	if daemon.IsRunning(daemon.DefaultPIDPath()) {
		daemonStatus = "running"
	}

	// Shard health
	corruptedCount := len(corrupted)
	shardCount := len(shards)

	fmt.Fprintf(w, "Index directory: %s\n", report.IndexDir)
	fmt.Fprintf(w, "Index size:      %s\n", formatBytes(report.IndexSizeBytes))
	fmt.Fprintf(w, "Total repos:     %d\n", report.TotalRepos)
	fmt.Fprintf(
		w,
		"Index shards:    %d (%d healthy, %d corrupted)\n",
		shardCount,
		shardCount-corruptedCount,
		corruptedCount,
	)
	fmt.Fprintf(w, "Search daemon:   %s\n", daemonStatus)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Build:")
	fmt.Fprintf(w, "  version:       %s\n", orDash(build.Version))
	fmt.Fprintf(w, "  commit:        %s\n", orDash(build.Commit))
	fmt.Fprintf(w, "  tag:           %s\n", orDash(build.Tag))
	fmt.Fprintf(w, "  build time:    %s\n", orDash(build.BuildTime))
	fmt.Fprintln(w)

	if corruptedCount > 0 {
		fmt.Fprintf(w, "Corrupted shards (%d):\n", corruptedCount)
		for _, s := range shards {
			if !s.OK {
				fmt.Fprintf(w, "  %s — %s\n", s.Path, s.Error)
			}
		}
		fmt.Fprintln(
			w,
			"  Run 'csl index --repair' to remove corrupted shards, then 'csl index' to rebuild.",
		)
		fmt.Fprintln(w)
	}

	if len(report.Issues) > 0 {
		fmt.Fprintf(w, "Issues (%d):\n", len(report.Issues))
		for _, issue := range report.Issues {
			fmt.Fprintf(w, "  %-8s %-40s %s\n", issue.Type, issue.Repo, issue.Message)
		}
		fmt.Fprintln(w)
	}

	if len(report.Healthy) > 0 {
		fmt.Fprintf(w, "Healthy (%d):\n", len(report.Healthy))
		for _, h := range report.Healthy {
			fmt.Fprintf(w, "  %-40s indexed %s ago\n", h.Repo, h.IndexAge)
		}
	}

	if len(report.Issues) == 0 && len(report.Healthy) > 0 {
		fmt.Fprintln(w, "\nAll repos are healthy.")
	}

	if len(report.Issues) == 0 && len(report.Healthy) == 0 {
		fmt.Fprintln(w, "No repos found. Run 'csl index' to build the index.")
	}

	return nil
}

// orDash renders an unknown build metadata field, which buildinfo reports as
// the empty string, as a dash so the column never looks truncated.
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
