package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/worklog/internal/config"
	"github.com/mad01/thismoon/tools/worklog/internal/scan"
)

// configReference documents every setting worklog reads, shipped in the binary
// so a machine with no config file still explains what one would contain.
const configReference = `# worklog config.yaml: every key optional.
# A set list REPLACES the default shown beside it; it doesn't extend it.

scan:                        # ticket-firewall strings for 'worklog scan',
                             # which classifies each session as personal or
                             # internal and keeps the two worlds' ticket ids
                             # from co-mingling. The three classification
                             # lists have no built-in values: with none of
                             # them set every session comes back as "unknown"
                             # and all of its ticket refs are surfaced.
  linear_prefixes:           # TEAM-NN key prefixes routed to the personal
    - MAD                    # (Linear) world. Any other key is read as
                             # internal (Jira).
  personal_path_markers:     # a session cwd containing one of these is
    - github.com/you/        # personal
  internal_path_markers:     # a session cwd containing one of these is
    - /workspace/            # internal
  checkout_roots:            # GOPATH-style checkout roots. The path segment
    - /code/src/             # right after a root is read as the git host, and
                             # a non-github.com host counts as internal: the
                             # split is derived, never enumerated.
  repo_path_markers:         # a cwd matching none of these reports no repo
    - /code/                 # (a tmp dir, say)
    - /workspace/

remote:                      # git upstream for the store (~/code/worklog).
                             # No url = local-only git, the default.
  url: git@github.com:you/worklog-private.git
  push: true                 # auto commit+push after each write; omitting the
                             # key means true once a url is set. worklog keeps
                             # origin pointed at the url, clones it when the
                             # store dir is missing, and 'worklog sync'
                             # pulls+pushes on demand.`

func (a *app) configCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the current effective config",
		Long: `Print which config file worklog reads and the settings in effect after the
built-in defaults are applied.

The file is --config when that flag is passed, $WORKLOG_CONFIG when that
variable is set, otherwise config.yaml in worklog's directory under
$XDG_CONFIG_HOME or ~/.config. It is optional and so is every key in it: with
no file at all worklog runs on the defaults, and the header line above the
output says so. A file that exists but can't be read or parsed is an error
everywhere else in worklog; this command names the problem instead and prints
the defaults, since explaining a broken config is what it is for.

` + configReference + `

Pair it with docs: docs prints the operating doc (runtime behavior, failure
modes, first moves), config shows which file and key to change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			path, err := config.Path(a.configPath)
			if err != nil {
				return err
			}
			cfg, loadErr := config.Load(path)
			fmt.Fprintf(out, "config file: %s (%s)\n\n", path, configStatus(path, loadErr))

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
// default (push=true) applies once a url is set, which the header of `worklog
// config` is not the place to flatten.
func effectiveConfig(cfg config.Config) config.Config {
	return config.Config{
		Scan:   config.Scan(scan.Config(cfg.Scan).WithDefaults()),
		Remote: cfg.Remote,
	}
}

// configStatus describes a config load for the header line. A broken file is
// not fatal here — the command names the problem and prints the defaults
// worklog would otherwise refuse to run on.
func configStatus(path string, err error) string {
	switch {
	case err != nil:
		return fmt.Sprintf("error: %v", err)
	case !config.Exists(path):
		return "missing, defaults in use"
	default:
		return "loaded"
	}
}
