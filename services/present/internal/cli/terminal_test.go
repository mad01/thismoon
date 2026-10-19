package cli

import (
	"bytes"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/store"
	"github.com/mad01/thismoon/services/present/internal/terminal"
)

// terminalWorkdir points the CLI at a fresh temp workdir.
func terminalWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := flagWorkdir
	flagWorkdir = dir
	t.Cleanup(func() { flagWorkdir = prev })
	return dir
}

func runTerminalCmd(
	t *testing.T, cmd *cobra.Command, run func(*cobra.Command, []string) error, args ...string,
) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetContext(t.Context())
	t.Cleanup(func() { cmd.SetOut(nil) })
	err := run(cmd, args)
	return out.String(), err
}

func TestTerminalOnOffStatus(t *testing.T) {
	dir := terminalWorkdir(t)
	status := func() string {
		t.Helper()
		out, err := runTerminalCmd(t, terminalStatusCmd, runTerminalStatus)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		return out
	}

	if got := status(); got != "live terminal: off\n" {
		t.Errorf("fresh status = %q, want off", got)
	}
	out, err := runTerminalCmd(t, terminalOnCmd, runTerminalOn)
	if err != nil {
		t.Fatalf("on: %v", err)
	}
	if !strings.Contains(out, terminal.SwitchPath(dir)) {
		t.Errorf("on printed %q, want the switch path", out)
	}
	if _, err := os.Stat(terminal.SwitchPath(dir)); err != nil {
		t.Errorf("on left no switch file: %v", err)
	}
	if got := status(); got != "live terminal: on\n" {
		t.Errorf("status after on = %q", got)
	}
	if _, err := runTerminalCmd(t, terminalOffCmd, runTerminalOff); err != nil {
		t.Fatalf("off: %v", err)
	}
	if got := status(); got != "live terminal: off\n" {
		t.Errorf("status after off = %q", got)
	}
	if _, err := runTerminalCmd(t, terminalOffCmd, runTerminalOff); err != nil {
		t.Errorf("off twice: %v", err)
	}
}

// terminalServe runs a local server over the CLI's workdir with one page
// and one live session on it, and points --port at it.
func terminalServe(t *testing.T) (*terminal.Manager, string) {
	t.Helper()
	dir := terminalWorkdir(t)
	st, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Create(t.Context(), store.Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	mgr := terminal.New(terminal.Config{
		Home: home, Env: []string{"HOME=" + home, "PATH=/usr/bin:/bin"},
		KillGrace: 500 * time.Millisecond,
	})
	t.Cleanup(mgr.Close)
	sub, err := mgr.Attach(p.ID, "demo", terminal.Spec{Cmd: "sleep 60"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	t.Cleanup(sub.Close)
	srv := server.New(st, server.Options{
		Workdir: dir, Terminals: mgr, TerminalSwitch: terminal.SwitchPath(dir),
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	prev := flagPort
	flagPort = port
	t.Cleanup(func() { flagPort = prev })
	return mgr, p.ID
}

func TestTerminalListAndKillGoThroughServe(t *testing.T) {
	mgr, page := terminalServe(t)

	out, err := runTerminalCmd(t, terminalListCmd, runTerminalList)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"live terminal: off", page, "demo", "running", "sleep 60"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output %q lacks %q", out, want)
		}
	}

	out, err = runTerminalCmd(t, terminalKillCmd, runTerminalKill, page, "demo")
	if err != nil {
		t.Fatalf("kill: %v", err)
	}
	if want := page + "/demo: killed\n"; out != want {
		t.Errorf("kill printed %q, want %q", out, want)
	}
	if got := mgr.List(); len(got) != 0 {
		t.Errorf("sessions after kill = %+v", got)
	}
	_, err = runTerminalCmd(t, terminalKillCmd, runTerminalKill, page, "demo")
	if err == nil || !strings.Contains(err.Error(), "no terminal session") {
		t.Errorf("kill of a gone session: err = %v, want it named missing", err)
	}
}

func TestTerminalListNamesAnUnreachableServe(t *testing.T) {
	terminalWorkdir(t)
	ts := httptest.NewServer(nil)
	u, _ := url.Parse(ts.URL)
	ts.Close()
	port, _ := strconv.Atoi(u.Port())
	prev := flagPort
	flagPort = port
	t.Cleanup(func() { flagPort = prev })

	_, err := runTerminalCmd(t, terminalListCmd, runTerminalList)
	if err == nil || !strings.Contains(err.Error(), "present serve at") {
		t.Errorf("list with serve down: err = %v, want it to name serve", err)
	}
}
