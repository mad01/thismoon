package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mad01/thismoon/tools/t-man/internal/platform/launchd"
	"github.com/mad01/thismoon/tools/t-man/internal/procstat"
	"github.com/spf13/cobra"
)

var topInterval time.Duration

// topCmd represents the top command
var topCmd = &cobra.Command{
	Use:   "top",
	Short: "Live resource view of managed services",
	Long: `Live-updating resource view (PID, RSS, CPU%, uptime) scoped to
t-man-managed services, refreshed on an interval. Press Ctrl-C to quit.`,
	Args: cobra.NoArgs,
	RunE: runTop,
}

func init() {
	topCmd.Flags().DurationVar(&topInterval, "interval", 2*time.Second, "Refresh interval")
	rootCmd.AddCommand(topCmd)
}

func runTop(cmd *cobra.Command, args []string) error {
	if err := checkSudo(); err != nil {
		return err
	}
	if topInterval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	ctx, stop := signal.NotifyContext(getContext(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := launchd.NewManager(GetVersion(), !daemonMode)

	ticker := time.NewTicker(topInterval)
	defer ticker.Stop()

	for {
		rows, err := gatherTopRows(ctx, manager)
		if err != nil {
			if ctx.Err() != nil {
				return nil // interrupted mid-collection
			}
			return err
		}

		// \033[H\033[2J homes the cursor and clears the screen
		fmt.Print("\033[H\033[2J" + formatTopFrame(rows, topInterval, time.Now()))

		select {
		case <-ctx.Done():
			fmt.Println()
			return nil
		case <-ticker.C:
		}
	}
}

// topRow is one service's line in the top view.
type topRow struct {
	name  string
	stats *procstat.Stats // nil when the service has no running process
}

// gatherTopRows lists the managed services and resolves resource stats for
// the running ones.
func gatherTopRows(ctx context.Context, manager *launchd.Manager) ([]topRow, error) {
	services, err := manager.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	stats, err := collectServiceStats(ctx, manager)
	if err != nil {
		return nil, err
	}

	rows := make([]topRow, 0, len(services))
	for _, svc := range services {
		row := topRow{name: svc.Name}
		if st, ok := stats[svc.Name]; ok {
			stCopy := st
			row.stats = &stCopy
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// formatTopFrame renders one refresh of the top view: a summary line, then
// running services sorted by RSS descending, then stopped services by name.
func formatTopFrame(rows []topRow, interval time.Duration, now time.Time) string {
	sorted := make([]topRow, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		switch {
		case a.stats != nil && b.stats == nil:
			return true
		case a.stats == nil && b.stats != nil:
			return false
		case a.stats != nil && b.stats != nil && a.stats.RSSBytes != b.stats.RSSBytes:
			return a.stats.RSSBytes > b.stats.RSSBytes
		default:
			return a.name < b.name
		}
	})

	running := 0
	var totalRSS int64
	for _, r := range rows {
		if r.stats != nil {
			running++
			totalRSS += r.stats.RSSBytes
		}
	}

	var b strings.Builder
	fmt.Fprintf(
		&b,
		"t-man top - %d/%d running - total RSS %s - refresh %s - %s  (Ctrl-C to quit)\n\n",
		running,
		len(rows),
		procstat.FormatRSS(totalRSS),
		interval,
		now.Format("15:04:05"),
	)
	fmt.Fprintf(&b, "%-30s %6s %8s %6s %8s\n", "NAME", "PID", "RSS", "CPU%", "UPTIME")
	b.WriteString(strings.Repeat("-", 62))
	b.WriteByte('\n')

	for _, r := range sorted {
		if r.stats == nil {
			fmt.Fprintf(&b, "%-30s %6s %8s %6s %8s\n", r.name, "-", "-", "-", "-")
			continue
		}
		fmt.Fprintf(
			&b,
			"%-30s %6d %8s %6.1f %8s\n",
			r.name,
			r.stats.PID,
			procstat.FormatRSS(r.stats.RSSBytes),
			r.stats.CPUPercent,
			procstat.FormatUptime(r.stats.Uptime),
		)
	}
	return b.String()
}
