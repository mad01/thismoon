package cli

import (
	"io"
	"testing"
)

func TestModeFlagsDaemonSelected(t *testing.T) {
	cases := []struct {
		name    string
		flags   modeFlags
		want    bool
		wantErr bool
	}{
		{"nothing passed", modeFlags{agent: true}, false, false},
		{"--daemon", modeFlags{agent: true, daemon: true, daemonSet: true}, true, false},
		{"--agent", modeFlags{agent: true, agentSet: true}, false, false},
		{"--agent=false", modeFlags{agentSet: true}, true, false},
		{
			"--agent=false --daemon",
			modeFlags{daemon: true, agentSet: true, daemonSet: true},
			true,
			false,
		},
		{
			"--agent --daemon=false",
			modeFlags{agent: true, agentSet: true, daemonSet: true},
			false,
			false,
		},
		{
			"--agent --daemon",
			modeFlags{agent: true, daemon: true, agentSet: true, daemonSet: true},
			false,
			true,
		},
		{"--agent=false --daemon=false", modeFlags{agentSet: true, daemonSet: true}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.flags.daemonSelected()
			if (err != nil) != tc.wantErr {
				t.Fatalf("daemonSelected() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("daemonSelected() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRootAcceptsAgentFalseWithDaemon pins the bug this replaced: the
// mutual-exclusion group was value-blind, so spelling daemon mode out in full
// was rejected as a conflict.
func TestRootAcceptsAgentFalseWithDaemon(t *testing.T) {
	t.Cleanup(func() { agentMode, daemonMode = true, false })

	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"--agent=false", "--daemon", "version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("--agent=false --daemon: %v, want nil error", err)
	}
	if !daemonMode {
		t.Error("daemonMode = false after --agent=false --daemon, want true")
	}
}
