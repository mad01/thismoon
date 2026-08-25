package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func overrideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override",
		Short: "List, set, or clear guard overrides (a set override disables the rules that name it)",
		Long: `Guard overrides are named switches a config rule can point at: a
commit_guards rule with override: vacation stops applying while the vacation
override is set. An override is one empty file under ` + config.OverridesDir() + `;
set and clear manage it, doctor reports what is active.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listOverrides(cmd)
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "set <name>",
			Short: "Activate an override",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				name, err := overrideName(args[0])
				if err != nil {
					return err
				}
				dir := config.OverridesDir()
				if dir == "" {
					return fmt.Errorf("override: cannot resolve home dir")
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("override: %w", err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
					return fmt.Errorf("override: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "override %q set — rules naming it stop applying until belt override clear %s\n", name, name)
				return nil
			},
		},
		&cobra.Command{
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
		},
	)
	return cmd
}

func listOverrides(cmd *cobra.Command) error {
	active := config.ActiveOverrides()
	if len(active) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "no overrides active")
		return nil
	}
	for _, name := range active {
		fmt.Fprintln(cmd.OutOrStdout(), name)
	}
	return nil
}

// overrideName rejects names that would escape the overrides dir or hide as
// dotfiles — the name becomes a filename verbatim.
func overrideName(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid override name %q", name)
	}
	return name, nil
}
