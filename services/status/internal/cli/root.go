package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/status/internal/server"
)

const (
	defaultPort    = 7426
	defaultWorkdir = "~/.local/share/status"
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
	Long: `status discovers every t-man-managed launchd service, probes it on an
interval (HTTP for services with a --port, launchctl PID check for the rest),
and serves a status-page dashboard with per-service 30-day uptime history at
http://status.this/.`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the status HTTP server and check poller",
	RunE: func(_ *cobra.Command, _ []string) error {
		return server.Serve(server.Options{
			Port:             flagPort,
			Interval:         flagInterval,
			Workdir:          expandTilde(flagWorkdir),
			RoutesPath:       expandTilde(flagRoutes),
			Info:             buildinfo.Get(),
			RestartWindow:    flagRestartWindow,
			RestartThreshold: flagRestartThreshold,
		})
	},
}

func init() {
	serveCmd.Flags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on (env STATUS_PORT)")
	serveCmd.Flags().DurationVar(&flagInterval, "interval", time.Minute,
		"how often to probe services")
	serveCmd.Flags().StringVar(&flagWorkdir, "workdir", resolvedDefaultWorkdir(),
		"directory holding the uptime history (env STATUS_WORKDIR)")
	serveCmd.Flags().StringVar(&flagRoutes, "routes", "~/.config/d-man/routes.toml",
		"d-man routes.toml used to map service ports to .this links")
	serveCmd.Flags().DurationVar(&flagRestartWindow, "restart-window", time.Hour,
		"window for counting launchd respawns per service")
	serveCmd.Flags().IntVar(&flagRestartThreshold, "restart-threshold", 50,
		"respawns within the window that trigger a crash-loop alert")
	rootCmd.AddCommand(serveCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func resolvedDefaultPort() int {
	if v := os.Getenv("STATUS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return defaultPort
}

func resolvedDefaultWorkdir() string {
	if v := os.Getenv("STATUS_WORKDIR"); v != "" {
		return v
	}
	return defaultWorkdir
}

// expandTilde resolves a leading ~ since launchd agents don't run through a
// shell and nothing else expands it.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
