package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	dman "github.com/mad01/thismoon/services/d-man"
	status "github.com/mad01/thismoon/services/status"
	"github.com/mad01/thismoon/services/status/internal/server"
)

var (
	flagPort             int
	flagInterval         time.Duration
	flagWorkdir          string
	flagRoutes           string
	flagRestartWindow    time.Duration
	flagRestartThreshold int
)

var rootCmd = &cobra.Command{
	Use:   "status",
	Short: "Status page for t-man-managed local services",
	Long: fmt.Sprintf(`status discovers every t-man-managed launchd service, probes it on an
interval (HTTP for services with a --port, launchctl PID check for the rest),
and serves a status-page dashboard with per-service 30-day uptime history at
http://localhost:%d (status.this with d-man).`, status.DefaultPort),
	// A failed probe is a diagnosis, not a usage mistake; main prints the
	// error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the status HTTP server and check poller",
	RunE: func(_ *cobra.Command, _ []string) error {
		return agentdoc.Hint(server.Serve(server.Options{
			Port:             flagPort,
			Interval:         flagInterval,
			Workdir:          flagWorkdir,
			RoutesPath:       flagRoutes,
			Info:             buildinfo.Get(),
			RestartWindow:    flagRestartWindow,
			RestartThreshold: flagRestartThreshold,
		}), status.Facts())
	},
}

func init() {
	// Persistent, not serve-only: `doctor` probes the port and reads the
	// workdir too, so a flag that only `serve` accepted meant
	// `status --port 9999 doctor` was rejected outright and a bare `doctor`
	// silently diagnosed a different target than the running serve.
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("STATUS_PORT", status.DefaultPort),
		"port the HTTP server listens on (env STATUS_PORT)")
	rootCmd.PersistentFlags().DurationVar(&flagInterval, "interval", time.Minute,
		"how often to probe services")
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("STATUS_WORKDIR", status.DefaultWorkdir),
		"directory holding the uptime history (env STATUS_WORKDIR)")
	// d-man owns this path; taking the constant from its package keeps the two
	// from drifting apart the way a copied string literal would.
	rootCmd.PersistentFlags().StringVar(&flagRoutes, "routes", dman.DefaultRoutesPath,
		"d-man routes.toml used to map service ports to .this links")
	rootCmd.PersistentFlags().DurationVar(&flagRestartWindow, "restart-window", time.Hour,
		"window for counting launchd respawns per service")
	rootCmd.PersistentFlags().IntVar(&flagRestartThreshold, "restart-threshold", 50,
		"respawns within the window that trigger a crash-loop alert")
	// Expand a leading ~ before any subcommand runs: launchd agents don't go
	// through a shell, so STATUS_WORKDIR reaches Go with the ~ intact. Both
	// paths are assigned only once both expand, so a failure leaves neither
	// half-resolved.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		workdir, err := confdir.Expand(flagWorkdir)
		if err != nil {
			return err
		}
		routes, err := confdir.Expand(flagRoutes)
		if err != nil {
			return err
		}
		flagWorkdir, flagRoutes = workdir, routes
		return nil
	}
	rootCmd.AddCommand(serveCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
