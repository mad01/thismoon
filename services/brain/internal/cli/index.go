package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/embedding"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

var (
	indexForceFlag          bool
	indexLiteralOnlyFlag    bool
	indexStrictSemanticFlag bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build or refresh the zoekt and (optional) semantic indexes",
	Long: `Build or refresh the indexes over the vault.

The literal (zoekt) index always runs; the semantic (vector) index runs only
when semantic.enabled=true in config.yaml. Both are skipped when the vault's
fingerprint matches the last-indexed state. Use --force to rebuild anyway,
or --literal-only to skip the semantic pass.

Ollama health is probed before the semantic pass. When ollama is unreachable
the semantic pass is skipped with a warning and the literal pass still runs;
brain index exits 0. Pass --strict-semantic to fail the command instead.`,
	RunE: runIndex,
}

func init() {
	indexCmd.Flags().
		BoolVar(&indexForceFlag, "force", false, "force re-index even if fingerprint is unchanged")
	indexCmd.Flags().
		BoolVar(&indexLiteralOnlyFlag, "literal-only", false, "skip the semantic index pass even when enabled")
	indexCmd.Flags().
		BoolVar(&indexStrictSemanticFlag, "strict-semantic", false, "fail when ollama is unreachable (default: skip semantic with a warning)")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("brain index: %w", err)
	}
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return fmt.Errorf("brain index: %w", err)
	}

	fingerprint, files, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return fmt.Errorf("brain index: fingerprint: %w", err)
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return fmt.Errorf("brain index: load state: %w", err)
	}

	literalStale := indexForceFlag || search.IsStale(state, fingerprint)
	semanticEnabled := cfg.Semantic.Enabled && !indexLiteralOnlyFlag
	semanticStale := semanticEnabled && (indexForceFlag || state.IsSemanticStale(fingerprint))

	w := cmd.ErrOrStderr()

	// Probe ollama health before committing to the semantic pass. Without
	// this, brain index exits 1 mid-stream when ollama is down — which
	// breaks `ralph apply` for users whose recipes call `brain index`.
	if semanticStale {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		healthErr := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel).Health(probeCtx)
		cancel()
		skip, err := decideSemantic(healthErr, indexStrictSemanticFlag)
		if err != nil {
			return fmt.Errorf("brain index: %w", err)
		}
		if skip {
			fmt.Fprintf(
				w,
				"warning: ollama unreachable at %s; skipping semantic index (use --strict-semantic to fail)\n",
				cfg.Semantic.OllamaURL,
			)
			semanticStale = false
		}
	}

	if !literalStale && !semanticStale {
		fmt.Fprintln(cmd.OutOrStdout(), "index is up to date")
		return nil
	}

	count := 0
	if literalStale {
		fmt.Fprintf(w, "indexing %s (literal)\n", cfg.Vault)
		err = search.IndexVault(cfg.Vault, indexDir, func(path string) {
			count++
			fmt.Fprintln(w, "  "+path)
		})
		if err != nil {
			return fmt.Errorf("brain index: %w", err)
		}
		state.SetFingerprint(fingerprint, files, time.Now().UTC())
	}

	semChunks := 0
	semFiles := 0
	if semanticStale {
		fmt.Fprintf(
			w,
			"indexing %s (semantic, model=%s)\n",
			cfg.Vault,
			cfg.Semantic.EmbedModel,
		)
		idx, err := openSemanticIndex(cfg)
		if err != nil {
			return fmt.Errorf("brain index: semantic open: %w", err)
		}
		opts := chunk.FromTokens(cfg.Semantic.ChunkSize, cfg.Semantic.ChunkOverlap)
		err = semantic.IndexVault(
			context.Background(),
			idx,
			cfg.Vault,
			opts,
			func(path string, n int) {
				semFiles++
				semChunks += n
				fmt.Fprintf(w, "  %s (%d chunks)\n", path, n)
			},
		)
		if err != nil {
			return fmt.Errorf("brain index: semantic: %w", err)
		}
		state.SetSemanticFingerprint(fingerprint, time.Now().UTC())
	}

	if err := state.Save(indexDir); err != nil {
		fmt.Fprintf(w, "warning: save index state: %v\n", err)
	}

	out := cmd.OutOrStdout()
	if literalStale {
		fmt.Fprintf(out, "indexed %d file(s) (literal)\n", count)
	}
	if semanticStale {
		fmt.Fprintf(out, "indexed %d file(s), %d chunk(s) (semantic)\n", semFiles, semChunks)
	}
	return nil
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

// openSemanticIndex resolves the semantic index dir, builds an Ollama-backed
// embedding function from cfg, and returns a ready-to-query Index.
func openSemanticIndex(cfg *config.Config) (*semantic.Index, error) {
	dir, err := cfg.Semantic.ResolvedIndexDir()
	if err != nil {
		return nil, err
	}
	c := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel)
	return semantic.Open(dir, c.EmbedFunc())
}

// ensureIndexCLI is the subcommand-side equivalent of mcpserver.ensureIndex:
// build or refresh the index only when stale. Progress is silent so search
// output stays clean.
func ensureIndexCLI(cfg *config.Config) (string, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return "", err
	}
	fingerprint, files, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return "", err
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return "", err
	}
	if !search.IsStale(state, fingerprint) {
		return indexDir, nil
	}
	if err := search.IndexVault(cfg.Vault, indexDir, nil); err != nil {
		return "", err
	}
	state.SetFingerprint(fingerprint, files, time.Now().UTC())
	_ = state.Save(indexDir)
	return indexDir, nil
}
