package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/terminal"
)

// terminalHTTPTimeout bounds one call to serve; a kill waits out the grace
// period between SIGHUP and SIGKILL, so it is well above that.
const terminalHTTPTimeout = 15 * time.Second

var terminalCmd = &cobra.Command{
	Use:   "terminal <on|off|status|list|kill>",
	Short: "Switch the live terminal on or off, and list or kill its sessions",
	Long: `The live terminal lets a deck slide attach to a process present serve runs in
a pseudo-terminal: a login shell, or a command under it. It is off by default.

  on      Turn it on: terminal blocks can start and attach to sessions.
  off     Turn it off: serve kills every running session within seconds and
          terminal blocks show a placeholder.
  status  Print whether it is on.
  list    List the sessions serve holds.
  kill    Kill one session: present terminal kill <page> <session>.

on, off, and status act on the switch file in the workdir, so they work
with serve running or not and need no restart. list and kill ask the local
serve on --port. Only a local serve reads the switch: a shared instance
(serve --shared) has no live terminal at all.`,
}

var terminalOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Turn the live terminal on",
	Args:  cobra.NoArgs,
	RunE:  runTerminalOn,
}

var terminalOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Turn the live terminal off and end its sessions",
	Args:  cobra.NoArgs,
	RunE:  runTerminalOff,
}

var terminalStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print whether the live terminal is on",
	Args:  cobra.NoArgs,
	RunE:  runTerminalStatus,
}

var terminalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the terminal sessions serve holds",
	Args:  cobra.NoArgs,
	RunE:  runTerminalList,
}

var terminalKillCmd = &cobra.Command{
	Use:   "kill <page> <session>",
	Short: "Kill one terminal session",
	Args:  cobra.ExactArgs(2),
	RunE:  runTerminalKill,
}

func init() {
	terminalCmd.AddCommand(
		terminalOnCmd, terminalOffCmd, terminalStatusCmd, terminalListCmd, terminalKillCmd,
	)
	rootCmd.AddCommand(terminalCmd)
}

func runTerminalOn(cmd *cobra.Command, _ []string) error {
	path := terminal.SwitchPath(flagWorkdir)
	if err := terminal.Enable(path); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "live terminal on (%s)\n", path)
	fmt.Fprintln(out, sharedNote)
	return nil
}

// sharedNote is the reminder on prints: the switch is read by a local serve
// alone, and a shared instance has no live terminal to switch.
const sharedNote = "note: this switches the local present serve only; " +
	"a shared instance never runs a live terminal"

func runTerminalOff(cmd *cobra.Command, _ []string) error {
	if err := terminal.Disable(terminal.SwitchPath(flagWorkdir)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"live terminal off; serve ends running sessions within %s\n", terminal.SwitchPoll)
	return nil
}

func runTerminalStatus(cmd *cobra.Command, _ []string) error {
	on, err := terminal.Enabled(terminal.SwitchPath(flagWorkdir))
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "live terminal: %s\n", onOff(on))
	return nil
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// terminalListing is the GET /api/terminal payload serve answers with.
type terminalListing struct {
	Enabled  bool            `json:"enabled"`
	Sessions []terminal.Info `json:"sessions"`
}

func runTerminalList(cmd *cobra.Command, _ []string) error {
	resp, err := callServe(cmd.Context(), http.MethodGet, "/api/terminal")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return serveError(resp)
	}
	var list terminalListing
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return fmt.Errorf("decode the session list: %w", err)
	}
	return printTerminalList(cmd.OutOrStdout(), list)
}

// printTerminalList prints the switch state and one row per session.
func printTerminalList(w io.Writer, list terminalListing) error {
	fmt.Fprintf(w, "live terminal: %s\n", onOff(list.Enabled))
	if len(list.Sessions) == 0 {
		fmt.Fprintln(w, "no sessions")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PAGE\tSESSION\tPID\tSTATE\tATTACHED\tSTARTED\tCMD")
	for _, s := range list.Sessions {
		state := "running"
		if s.Exited {
			state = fmt.Sprintf("exited %d", s.ExitCode)
		}
		cmdText := s.Cmd
		if cmdText == "" {
			cmdText = "(login shell)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%d\t%s\t%s\n", s.Page, s.Session, s.PID, state,
			s.Attached, s.Started.Local().Format("2006-01-02 15:04"), cmdText)
	}
	return tw.Flush()
}

func runTerminalKill(cmd *cobra.Command, args []string) error {
	page, session := args[0], args[1]
	p := "/api/p/" + url.PathEscape(page) + "/terminal/" + url.PathEscape(session)
	resp, err := callServe(cmd.Context(), http.MethodDelete, p)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNoContent:
		fmt.Fprintf(cmd.OutOrStdout(), "%s/%s: killed\n", page, session)
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("no terminal session %s/%s", page, session)
	default:
		return serveError(resp)
	}
}

// localServeURL is the local serve this process's --port points at.
func localServeURL() string {
	return fmt.Sprintf("http://localhost:%d", flagPort)
}

// callServe sends one request without a body to the local serve.
func callServe(ctx context.Context, method, path string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, terminalHTTPTimeout)
	base := localServeURL()
	req, err := http.NewRequestWithContext(ctx, method, base+path, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("present serve at %s: %w", base, err)
	}
	resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnClose releases a request's timeout once its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	defer c.cancel()
	return c.ReadCloser.Close()
}

// serveError turns an unexpected serve answer into an error carrying its
// status and the start of its body.
func serveError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		return fmt.Errorf("present serve answered %s: %w", resp.Status, err)
	}
	return fmt.Errorf("present serve answered %s: %s", resp.Status, strings.TrimSpace(string(body)))
}
