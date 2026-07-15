package commands

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

var rootCmd = &cobra.Command{
	Use:           "suspenders",
	Short:         "Git secret scanner and pre-commit hook manager",
	Long:          "Suspenders scans git repositories for checked-in secrets, tokens, passwords, and keys. It can install and manage pre-commit hooks across all your repositories.",
	SilenceErrors: true,
	SilenceUsage:  true,
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
