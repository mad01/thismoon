package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Print the configured host -> backend routes",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, _ []string) error {
	cfg, err := loadRoutes()
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "suffix: %s\n", cfg.Suffix)
	for _, r := range cfg.Routes {
		backend, err := cfg.BackendFor(r)
		if err != nil {
			return err
		}
		if r.Cname != "" {
			fmt.Fprintf(
				w,
				"  %-28s -> %s (cname %s.%s)\n",
				r.Host(cfg.Suffix),
				backend,
				r.Cname,
				cfg.Suffix,
			)
		} else {
			fmt.Fprintf(w, "  %-28s -> %s\n", r.Host(cfg.Suffix), backend)
		}
	}
	return nil
}
