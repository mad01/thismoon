package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// defaultOverrideFor is how long `belt override set` holds without an
// explicit --for: short enough that a forgotten override cannot quietly
// disarm a rule for days.
const defaultOverrideFor = 10 * time.Minute

func overrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override",
		Short: "List, set, extend, or clear timed guard overrides",
		Long: `Guard overrides are named switches a config rule can point at: a
commit_guards rule with override: vacation stops applying while the vacation
override is active. The name is yours to invent, name the exception
(vacation, release-night), not the rule, and several rules may share one
name so a single set stands them all down. The two sides connect by exact
string match and nothing cross-checks them: setting a name no rule
references is a silent no-op, so copy the name from the deny reason rather
than typing it from memory.

An override is timed: set holds it for 10m unless --for says otherwise,
extend pushes the expiry forward, and an expired override deactivates on
its own. Both demand a --reason saying why the guard is stood down; the
reason is archived to the events service (events.this) when it is running.
The switch is one file under ` + config.OverridesDir() + `
carrying its RFC 3339 expiry (an empty file from an older belt counts as
untimed and stays active until cleared). Permanently disabling a guard is a
config decision, not an override: guards.<id>.enabled in the belt config.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listOverrides(cmd)
		},
	}
	cmd.AddCommand(overrideSetCmd(), overrideExtendCmd(), overrideClearCmd())
	return cmd
}

func overrideSetCmd() *cobra.Command {
	var holdFor time.Duration
	var reason string
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Activate an override for a limited time (default 10m)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := overrideName(args[0])
			if err != nil {
				return err
			}
			why, err := overrideReason(reason)
			if err != nil {
				return err
			}
			expiry := time.Now().Add(holdFor)
			if err := writeOverride(name, expiry); err != nil {
				return err
			}
			notify.EmitEventSync("belt", "warn", fmt.Sprintf("override set (%s)", name), why,
				map[string]string{
					"override": name,
					"action":   "set",
					"for":      holdFor.String(),
					"expires":  expiry.Format(time.RFC3339),
				})
			fmt.Fprintf(cmd.OutOrStdout(),
				"override %q set for %s — rules naming it stop applying until %s "+
					"(belt override extend %s pushes it, clear %s ends it early)\n",
				name, holdFor, expiry.Format("15:04"), name, name)
			return nil
		},
	}
	cmd.Flags().DurationVar(&holdFor, "for", defaultOverrideFor,
		"how long the override holds before expiring on its own")
	cmd.Flags().StringVar(&reason, "reason", "",
		"why the guard is being stood down (required, archived to events.this)")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func overrideExtendCmd() *cobra.Command {
	var holdFor time.Duration
	var reason string
	cmd := &cobra.Command{
		Use:   "extend <name>",
		Short: "Push an existing override's expiry forward (default +10m)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := overrideName(args[0])
			if err != nil {
				return err
			}
			why, err := overrideReason(reason)
			if err != nil {
				return err
			}
			o, ok := config.ReadOverride(name)
			if !ok {
				return fmt.Errorf("override: %q is not set (belt override set %s --reason \"...\")", name, name)
			}
			// Extend from the current expiry while it is still ahead, from now
			// otherwise — extending an expired, legacy, or malformed override
			// restarts it as a timed one.
			base := time.Now()
			if o.Expiry.After(base) {
				base = o.Expiry
			}
			expiry := base.Add(holdFor)
			if err := writeOverride(name, expiry); err != nil {
				return err
			}
			notify.EmitEventSync("belt", "warn", fmt.Sprintf("override extended (%s)", name), why,
				map[string]string{
					"override": name,
					"action":   "extend",
					"for":      holdFor.String(),
					"expires":  expiry.Format(time.RFC3339),
				})
			fmt.Fprintf(cmd.OutOrStdout(), "override %q extended — active until %s\n",
				name, expiry.Format("15:04"))
			return nil
		},
	}
	cmd.Flags().DurationVar(&holdFor, "for", defaultOverrideFor,
		"how much time to add past the current expiry (or past now, when already expired)")
	cmd.Flags().StringVar(&reason, "reason", "",
		"why the guard stays stood down (required, archived to events.this)")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func overrideClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear <name>",
		Short: "Deactivate an override",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := overrideName(args[0])
			if err != nil {
				return err
			}
			path := filepath.Join(config.OverridesDir(), name)
			if err := os.Remove(path); err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintf(cmd.OutOrStdout(), "override %q was not set\n", name)
					return nil
				}
				return fmt.Errorf("override: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "override %q cleared\n", name)
			return nil
		},
	}
}

// writeOverride records the override's expiry, creating the overrides dir on
// first use.
func writeOverride(name string, expiry time.Time) error {
	dir := config.OverridesDir()
	if dir == "" {
		return fmt.Errorf("override: cannot resolve home dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("override: %w", err)
	}
	content := expiry.Format(time.RFC3339) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		return fmt.Errorf("override: %w", err)
	}
	return nil
}

func listOverrides(cmd *cobra.Command) error {
	overrides := config.Overrides()
	if len(overrides) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no overrides set")
		return nil
	}
	now := time.Now()
	for _, o := range overrides {
		fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", o.Name, overrideStatus(o, now))
	}
	return nil
}

// overrideStatus renders one override's state for `belt override` and the
// doctor report: active with remaining time, legacy, expired, or malformed.
func overrideStatus(o config.Override, now time.Time) string {
	switch {
	case o.Malformed:
		return fmt.Sprintf("MALFORMED — inactive (re-set with belt override set %s --reason \"...\")", o.Name)
	case o.Legacy:
		return "ACTIVE (untimed legacy file — re-set with --for to make it expire)"
	case o.Active(now):
		return fmt.Sprintf("ACTIVE — expires in %s (%s)",
			o.Remaining(now).Round(time.Second), o.Expiry.Local().Format("15:04"))
	default:
		return fmt.Sprintf("expired %s ago (belt override clear %s to tidy up)",
			now.Sub(o.Expiry).Round(time.Second), o.Name)
	}
}

// overrideName rejects names that would escape the overrides dir or hide as
// dotfiles — the name becomes a filename verbatim.
func overrideName(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid override name %q", name)
	}
	return name, nil
}

// overrideReason rejects blank reasons: the flag is required so every
// stand-down is explained, and a whitespace-only value would defeat that.
func overrideReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", fmt.Errorf("override: --reason must not be blank — say why the guard is being stood down")
	}
	return reason, nil
}
