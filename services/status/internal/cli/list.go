package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	status "github.com/mad01/thismoon/services/status"
	"github.com/mad01/thismoon/services/status/internal/server"
)

// Output formats list takes.
const (
	outputText = "text"
	outputJSON = "json"
)

// listTimeout bounds the one request list makes. serve answers from memory,
// so anything slower is a hung process, not a busy one.
const listTimeout = 5 * time.Second

var listOutput string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List every service the running status serve probes, and whether it is up",
	Long: `List every t-man-managed service with its last probe result, read from the
running status serve (GET /api/status on --port). One line per service: label,
up or down, the probe's detail, and "stale binary" when the running build lags
the installed one.

With -o json, prints {generated_at, down, services: [...]}, each service with
label, up, detail, port, link, drift, version and installed; the uptime days
the dashboard draws are left out. Exits 1 when any service is down, after
printing the list, so a script or an agent can branch on the exit code alone.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
		return runList(cmd.OutOrStdout(), baseURL, listOutput)
	},
}

func init() {
	listCmd.Flags().
		StringVarP(&listOutput, "output", "o", outputText, "Output format: text or json")
	rootCmd.AddCommand(listCmd)
}

// listing is the fleet view list prints: the snapshot without the per-day
// uptime strips, which only the dashboard draws.
type listing struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Down        int           `json:"down"`
	Services    []listService `json:"services"`
}

// listService is one service as list reports it.
type listService struct {
	Label     string `json:"label"`
	Up        bool   `json:"up"`
	Detail    string `json:"detail,omitempty"`
	Port      int    `json:"port,omitempty"`
	Link      string `json:"link,omitempty"`
	Drift     bool   `json:"drift,omitempty"`
	Version   string `json:"version,omitempty"`
	Installed string `json:"installed,omitempty"`
}

// runList fetches the snapshot from serve at baseURL and prints it as
// output. It returns an error naming the down services when any is down.
func runList(w io.Writer, baseURL, output string) error {
	if output != outputText && output != outputJSON {
		return fmt.Errorf("--output must be text or json, got %q", output)
	}
	snap, err := fetchSnapshot(baseURL)
	if err != nil {
		return err
	}
	if snap.GeneratedAt.IsZero() {
		return fmt.Errorf("status serve at %s has not finished its first check yet; "+
			"run status list again in a few seconds", baseURL)
	}
	l := toListing(snap)
	if output == outputJSON {
		err = writeJSON(w, l)
	} else {
		err = writeText(w, l)
	}
	if err != nil {
		return err
	}
	return downErr(l)
}

// fetchSnapshot reads GET /api/status. A serve that does not answer gets the
// doctor hint, since status doctor tells a stopped serve from a moved port.
func fetchSnapshot(baseURL string) (server.Snapshot, error) {
	client := &http.Client{Timeout: listTimeout}
	res, err := client.Get(baseURL + "/api/status")
	if err != nil {
		return server.Snapshot{}, agentdoc.Hint(
			fmt.Errorf("status serve not reachable at %s: %w", baseURL, err), status.Facts())
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return server.Snapshot{}, agentdoc.Hint(
			fmt.Errorf("status serve at %s answered %s", baseURL, res.Status), status.Facts())
	}
	var snap server.Snapshot
	if err := json.NewDecoder(res.Body).Decode(&snap); err != nil {
		return server.Snapshot{}, fmt.Errorf("decode %s/api/status: %w", baseURL, err)
	}
	return snap, nil
}

// toListing drops the uptime strips and recounts down from the services, so
// the count and the names in the exit error always agree.
func toListing(snap server.Snapshot) listing {
	l := listing{
		GeneratedAt: snap.GeneratedAt,
		Services:    make([]listService, 0, len(snap.Services)),
	}
	for _, s := range snap.Services {
		l.Services = append(l.Services, listService{
			Label: s.Label, Up: s.Up, Detail: s.Detail, Port: s.Port, Link: s.Link,
			Drift: s.Drift, Version: s.Version, Installed: s.Installed,
		})
		if !s.Up {
			l.Down++
		}
	}
	return l
}

func writeJSON(w io.Writer, l listing) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(l)
}

func writeText(w io.Writer, l listing) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVICE\tSTATE\tDETAIL")
	for _, s := range l.Services {
		state := "up"
		if !s.Up {
			state = "DOWN"
		}
		if s.Drift {
			state += ", stale binary"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Label, state, s.Detail)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "%d of %d down, checked %s\n",
		l.Down, len(l.Services), l.GeneratedAt.Local().Format("15:04:05"))
	return err
}

// downErr names the down services, the error that makes list exit 1.
func downErr(l listing) error {
	if l.Down == 0 {
		return nil
	}
	var names []string
	for _, s := range l.Services {
		if !s.Up {
			names = append(names, s.Label)
		}
	}
	return fmt.Errorf("%d of %d services down: %s; t-man restart <label> restarts one",
		l.Down, len(l.Services), strings.Join(names, ", "))
}
