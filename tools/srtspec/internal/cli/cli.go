// Package cli wires the srtspec commands: render, check and generate turn
// access manifests into srt settings files, verify checks a repo's rendered
// files and expansion layers without writing, run launches a process under
// srt with the machine-local layers merged in, and explain shows the policy
// such a launch gets.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/access"
	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/tools/srtspec"
	"github.com/mad01/thismoon/tools/srtspec/internal/discover"
	"github.com/mad01/thismoon/tools/srtspec/internal/drift"
	"github.com/mad01/thismoon/tools/srtspec/internal/layers"
)

// Execute runs the root command.
func Execute() error { return root().Execute() }

func root() *cobra.Command {
	c := &cobra.Command{
		Use:           "srtspec",
		Short:         "Render access manifests to srt settings and explain the effective policy",
		Version:       buildinfo.Get().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	c.AddCommand(
		versionCmd(),
		agentcli.DocsCommand(srtspec.OperatingDoc, srtspec.Facts()),
		renderCmd(),
		checkCmd(),
		generateCmd(),
		verifyCmd(),
		runCmd(),
		explainCmd(),
	)
	return c
}

func renderCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "render <manifest>",
		Short: "Render one manifest to srt settings JSON",
		Long: `Render one access manifest to the srt settings JSON the kernel enforces.

The manifest is a service-info.yaml whose Component declares spec.access, or
a standalone access.yaml. Without -o the JSON goes to stdout.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			settings, err := renderFile(args[0])
			if err != nil {
				return err
			}
			if out == "" {
				_, err := cmd.OutOrStdout().Write(settings)
				return err
			}
			_, err = writeIfChanged(out, settings)
			return err
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "write the JSON to this file instead of stdout")
	return c
}

func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check <manifest> <settings.json>",
		Short: "Fail with a diff when a settings file drifted from its manifest",
		Long: `Compare a settings file to what its manifest renders, by meaning.

Key order, indentation, and the order of string lists do not count. On drift
the command prints a unified diff (committed file to render) and exits 1.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifest, settingsPath := args[0], args[1]
			rendered, err := renderFile(manifest)
			if err != nil {
				return err
			}
			committed, err := os.ReadFile(settingsPath)
			if err != nil {
				return err
			}
			diff, err := drift.Diff(settingsPath, committed, manifest, rendered)
			if err != nil {
				return err
			}
			if diff != "" {
				fmt.Fprint(cmd.OutOrStdout(), diff)
				return fmt.Errorf("%s drifted from %s; run srtspec render", settingsPath, manifest)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s matches %s\n", settingsPath, manifest)
			return nil
		},
	}
}

func generateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate [root]",
		Short: "Render every manifest in a repo to its recipe's settings file",
		Long: `Render every access manifest under a repo root (default .) and write each
to its recipe's <label>.srt.json.

A component's service-info.yaml (services/<name>, tools/<name>, or <name>)
renders to recipes/<name>/<label>.srt.json; a recipe's access.yaml renders
beside it. A manifest whose sandbox is not srt renders nothing. Any
recipes/*/*.srt.json no manifest renders is an error: the manifest is the
source of truth.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return generate(cmd, root)
		},
	}
}

func generate(cmd *cobra.Command, root string) error {
	targets, err := discover.Find(root)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	for _, t := range targets {
		if t.Output == "" {
			fmt.Fprintf(w, "skip    %s (sandbox %s)\n", t.Path, t.Manifest.Sandbox)
			continue
		}
		settings, err := access.Render(t.Manifest)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Path, err)
		}
		changed, err := writeIfChanged(filepath.Join(root, t.Output), settings)
		if err != nil {
			return err
		}
		verb := "ok     "
		if changed {
			verb = "wrote  "
		}
		fmt.Fprintf(w, "%s %s (from %s)\n", verb, t.Output, t.Path)
	}
	orphans, err := discover.Orphans(root, targets)
	if err != nil {
		return err
	}
	if len(orphans) > 0 {
		return fmt.Errorf(
			"no manifest renders %v; declare spec.access or an access.yaml, or delete the file",
			orphans,
		)
	}
	return nil
}

func explainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <label>",
		Short: "Show the effective policy srt-run would launch a label under",
		Long: `Show the effective srt policy for a label, the way srt-run merges it at
launch: ~/.config/srt/<label>.json, then ~/.config/srt/<label>.local.json if
present, then SRT_RUN_ALLOW_DOMAINS if set. Each row names the layer it came
from. Where run would refuse to launch (a missing settings file, a local
layer that does more than widen, a bare * host), explain says why and exits 1.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]
			if !access.ValidLabel(label) {
				return fmt.Errorf("label %q must match [A-Za-z0-9._-]+", label)
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			p, err := layers.Resolve(layers.Input{
				Home:         home,
				Label:        label,
				AllowDomains: os.Getenv("SRT_RUN_ALLOW_DOMAINS"),
			})
			if err != nil {
				return err
			}
			return p.Write(cmd.OutOrStdout(), home)
		},
	}
}

func renderFile(path string) ([]byte, error) {
	m, err := access.Load(path)
	if errors.Is(err, access.ErrNoManifest) {
		return nil, fmt.Errorf("%s declares no spec.access", path)
	}
	if err != nil {
		return nil, err
	}
	settings, err := access.Render(m)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return settings, nil
}

// writeIfChanged writes data to path unless it already holds exactly that,
// so a no-op generate leaves timestamps alone. It reports whether it wrote.
func writeIfChanged(path string, data []byte) (bool, error) {
	cur, err := os.ReadFile(path)
	if err == nil && bytes.Equal(cur, data) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
