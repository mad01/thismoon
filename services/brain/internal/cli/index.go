package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// indexFlags are the brain index options.
type indexFlags struct {
	force          bool
	literalOnly    bool
	strictSemantic bool
}

func newIndexCmd(a *app) *cobra.Command {
	var flags indexFlags
	c := &cobra.Command{
		Use:   "index",
		Short: "Build or refresh the zoekt and (optional) semantic indexes",
		Long: `Build or refresh the indexes over the vault.

Each vault (every vault unless --vault narrows it) has its own indexes. The
literal (zoekt) index always runs; the semantic (vector) index runs only
when semantic.enabled=true in config.yaml. Both are skipped when the vault's
fingerprint matches the last-indexed state. Use --force to rebuild anyway,
or --literal-only to skip the semantic pass. A build holds a per-vault lock,
so concurrent brain processes wait instead of building the same index.

Ollama health is probed before the semantic pass. When ollama is unreachable
the semantic pass is skipped with a warning and the literal pass still runs;
brain index exits 0. Pass --strict-semantic to fail the command instead.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error { return a.runIndex(cmd, flags) },
	}
	c.Flags().
		BoolVar(&flags.force, "force", false, "force re-index even if fingerprint is unchanged")
	c.Flags().
		BoolVar(&flags.literalOnly, "literal-only", false, "skip the semantic index pass even when enabled")
	c.Flags().
		BoolVar(&flags.strictSemantic, "strict-semantic", false, "fail when ollama is unreachable (default: skip semantic with a warning)")
	return c
}

func (a *app) runIndex(cmd *cobra.Command, flags indexFlags) error {
	lib, set, err := a.openVaults()
	if err != nil {
		return err
	}
	ctx := context.Background()
	cfg := lib.Config()
	w := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()

	built := false
	for _, v := range set.Vaults() {
		res, err := lib.RefreshLiteral(v, flags.force, progressLines(w, v.Name+" (literal)"))
		if err != nil {
			return err
		}
		if res.Built {
			built = true
			fmt.Fprintf(out, "indexed %d file(s) in %s (literal)\n", res.Notes, v.Name)
		}
	}

	semanticOn, err := a.semanticPass(ctx, cmd, lib, set, flags)
	if err != nil {
		return err
	}
	if semanticOn {
		for _, v := range set.Vaults() {
			header := fmt.Sprintf("%s (semantic, model=%s)", v.Name, cfg.Semantic.EmbedModel)
			lines := progressLines(w, header)
			res, err := lib.RefreshSemantic(ctx, v, flags.force, func(path string, n int) {
				lines(fmt.Sprintf("%s (%d chunks)", path, n))
			})
			if err != nil {
				return fmt.Errorf("semantic: %w", err)
			}
			if res.Built {
				built = true
				fmt.Fprintf(out, "indexed %d file(s), %d chunk(s) in %s (semantic)\n",
					res.Notes, res.Chunks, v.Name)
			}
		}
	}
	if !built {
		fmt.Fprintln(out, "index is up to date")
	}
	return nil
}

// semanticPass reports whether brain index runs the semantic pass. It does
// when the layer is on, some vault needs it, and Ollama answers; an
// unreachable Ollama skips it with a warning, or fails under
// --strict-semantic. Without the probe brain index would exit 1 mid-stream
// when Ollama is down, which breaks provisioning runs that call it.
func (a *app) semanticPass(
	ctx context.Context,
	cmd *cobra.Command,
	lib *library.Library,
	set *vault.Set,
	flags indexFlags,
) (bool, error) {
	cfg := lib.Config()
	if !cfg.Semantic.Enabled || flags.literalOnly {
		return false, nil
	}
	needed := flags.force
	for _, v := range set.Vaults() {
		if needed {
			break
		}
		stale, err := lib.SemanticStale(v)
		if err != nil {
			return false, err
		}
		needed = stale
	}
	if !needed {
		return false, nil
	}
	skip, err := decideSemantic(lib.Health(ctx), flags.strictSemantic)
	if err != nil {
		return false, err
	}
	if skip {
		fmt.Fprintf(
			cmd.ErrOrStderr(),
			"warning: ollama unreachable at %s; skipping semantic index (use --strict-semantic to fail)\n",
			cfg.Semantic.OllamaURL,
		)
	}
	return !skip, nil
}

// progressLines returns a progress callback that prints "indexing <header>"
// before its first line, so a vault that needs no build prints nothing.
func progressLines(w io.Writer, header string) func(line string) {
	started := false
	return func(line string) {
		if !started {
			fmt.Fprintf(w, "indexing %s\n", header)
			started = true
		}
		fmt.Fprintln(w, "  "+line)
	}
}

// decideSemantic translates an ollama health probe into a (skip, err) pair.
// Healthy ollama: don't skip, no error. Unreachable ollama: skip when
// strict is false, return an error when strict is true. Pure logic so it
// can be unit tested without a live HTTP server or filesystem.
func decideSemantic(healthErr error, strict bool) (skip bool, err error) {
	if healthErr == nil {
		return false, nil
	}
	if strict {
		return false, fmt.Errorf("ollama health: %w", healthErr)
	}
	return true, nil
}
