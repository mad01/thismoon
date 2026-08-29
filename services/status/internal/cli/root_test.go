package cli

import "testing"

// The six service settings used to be registered on `serve` alone, so
// `status --port 9999 doctor` was rejected as an unknown flag and a bare
// `doctor` probed the compiled-in port no matter what serve was told. They
// are persistent now, and every subcommand must resolve them identically.
func TestServiceFlagsReachEverySubcommand(t *testing.T) {
	flags := []string{
		"port", "workdir", "routes", "interval", "restart-window", "restart-threshold",
	}
	for _, name := range []string{"serve", "doctor", "docs", "version"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		if cmd.Name() != name {
			t.Fatalf("looking up %q resolved to %q", name, cmd.Name())
		}
		for _, flag := range flags {
			if cmd.InheritedFlags().Lookup(flag) == nil {
				t.Errorf("%s does not accept --%s", name, flag)
			}
		}
	}
}
