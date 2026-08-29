package commands

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

var rootCmd = &cobra.Command{
	Use:           "suspenders",
	Short:         "Git secret scanner and pre-commit hook manager",
	Long:          "Suspenders scans git repositories for checked-in secrets, tokens, passwords, and keys. It can install and manage pre-commit hooks across all your repositories.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

// configPath is the --config flag: the config file to use instead of the
// default location. Every command reads it through configFilePath.
var configPath string

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "",
		"config file (default ~/.config/suspenders/config.yaml; overrides $"+config.EnvConfig+")")
}

// configFilePath resolves which config file this invocation uses.
func configFilePath() (string, error) {
	return config.PathFor(configPath)
}

// loadConfig reads that file. A missing one yields the in-memory defaults;
// a present one that cannot be parsed is an error every caller propagates,
// so a broken config fails the command instead of running it unguarded.
func loadConfig() (*config.Config, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}
	return config.LoadFrom(path)
}

// Execute runs the root command. Returns ErrFindingsFound when scan
// detects secrets with --fail-on-findings, which main translates to exit 1.
func Execute() error {
	return rootCmd.Execute()
}

// IsFindingsError reports whether err is the sentinel for detected secrets.
func IsFindingsError(err error) bool {
	return errors.Is(err, scanner.ErrFindingsFound)
}
