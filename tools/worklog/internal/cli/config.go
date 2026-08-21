package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/worklog/internal/config"
	"github.com/mad01/thismoon/tools/worklog/internal/scan"
)

// configReference documents every setting worklog reads, shipped in the binary
// so a machine with no config file still explains what one would contain.
const configReference = `# ~/.config/worklog/config.yaml — every key optional.
# A set list REPLACES the default shown beside it; it does not extend it.

scan:                        # ticket-firewall strings for 'worklog scan',
                             # which classifies each session as personal or
                             # internal and keeps the two worlds' ticket ids
                             # from co-mingling.
  linear_prefixes:           # TEAM-NN key prefixes routed to the personal
    - MAD                    # (Linear) world. Any other key is read as
                             # internal (Jira).
  personal_path_markers:     # a session cwd containing one of these is
    - github.com/mad01/      # personal
  internal_path_markers:     # a session cwd containing one of these is
    - /workspace/            # internal
  checkout_roots:            # GOPATH-style checkout roots. The path segment
    - /code/src/             # right after a root is read as the git host, and
                             # a non-github.com host counts as internal — the
                             # split is derived, never enumerated.
  repo_path_markers:         # a cwd matching none of these reports no repo
    - /code/                 # (a tmp dir, say)
    - /workspace/

remote:                      # git upstream for the store (~/code/worklog).
                             # No upstreams = local-only git, the default.
  push: true                 # auto commit+push after each write; omitting the
                             # key means true once an upstream resolves.
  upstreams:                 # machine profile (from ralph's config.local.toml)
    personal: git@github.com:you/worklog-personal.git   # -> remote URL. The
    work: git@github.com:you/worklog-work.git           # machine's first
                             # profile with an entry wins, so one shared file
                             # sends each machine's store to its own repo.
                             # worklog keeps origin pointed at the resolved
                             # URL, clones it when the store dir is missing,
                             # and 'worklog sync' pulls+pushes on demand.`

func configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the current effective config",
		Long: `Print which config file worklog reads and the settings in effect after the
built-in defaults are applied.

The file is $WORKLOG_CONFIG when that variable is set, otherwise
~/.config/worklog/config.yaml. It is optional and so is every key in it: a
missing file, an unreadable one, or a malformed one all leave worklog running
on the defaults, and the header line above the output says which of those
happened.

` + configReference + `

Pair it with doctor: doctor shows the state worklog resolved, config shows
which file and key to change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, loadErr := config.LoadFile()
			fmt.Fprintf(out, "config file: %s (%s)\n\n", config.Path(), configStatus(loadErr))

			b, err := yaml.Marshal(effectiveConfig(cfg))
			if err != nil {
				return fmt.Errorf("marshaling effective config: %w", err)
			}
			_, err = out.Write(b)
			return err
		},
	}
}

// effectiveConfig resolves cfg against the defaults the scan package applies,
// so the printed YAML shows the values worklog runs on rather than the blanks
// the file left behind. The remote section passes through as written: its only
// default (push=true) is resolved per machine, which the header of `worklog
// config` is not the place to flatten.
func effectiveConfig(cfg config.Config) config.Config {
	return config.Config{
		Scan:   config.Scan(scan.Config(cfg.Scan).WithDefaults()),
		Remote: cfg.Remote,
	}
}

// configStatus describes a config load for the header line. A broken file is
// not fatal here — the command names the problem and prints the defaults that
// worklog fell back to.
func configStatus(err error) string {
	switch {
	case err == nil:
		return "loaded"
	case errors.Is(err, fs.ErrNotExist):
		return "missing, defaults in use"
	default:
		return fmt.Sprintf("parse error: %v", err)
	}
}
