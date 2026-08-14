package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/humanizer/internal/rewrite"
)

var (
	rewriteBackend     string
	rewriteModel       string
	rewriteBaseURL     string
	rewriteAllowRemote bool
	rewriteStrength    string
	rewriteLang        string
	rewriteOrigLang    string
	rewriteTimeout     float64
	rewriteTemperature float64
	rewriteCandidates  int
	rewriteNoLayerA    bool
	rewriteOutput      string
	rewriteJSON        bool
)

var rewriteCmd = &cobra.Command{
	Use:   "rewrite [file]",
	Short: "Layer B: build (or run) a rewrite prompt against statistical watermarks",
	Long: `Best-effort pass at statistical (token-sampling) watermarks that the Layer A
scrub cannot touch. The default backend, print-prompt, returns the rewrite
prompt and calls no model — offline and safe to run anywhere; you (or the
calling agent) then produce the rewrite.

The ollama and openai-compatible backends run a chat model directly. They send
text off-process, so they are refused for non-loopback hosts unless
--allow-remote (or WATERMARKS_REWRITE_ALLOW_REMOTE=1) is set. API keys are read
from WATERMARKS_REWRITE_API_KEY only — never a flag.

Prefer a rewrite model different from the suspected origin model; rewriting with
the origin model can re-stamp the text. Residual risk is lower for short,
predictable text and higher for long, high-entropy prose — no tool can certify a
vendor detector will fail.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRewrite,
}

func init() {
	rewriteCmd.Flags().StringVar(&rewriteBackend, "backend",
		rewrite.Env("WATERMARKS_REWRITE_BACKEND", string(rewrite.PrintPrompt)),
		"print-prompt | ollama | openai-compatible")
	rewriteCmd.Flags().StringVar(&rewriteModel, "model",
		rewrite.Env("WATERMARKS_REWRITE_MODEL", ""), "model name (required for model backends)")
	rewriteCmd.Flags().StringVar(&rewriteBaseURL, "base-url",
		rewrite.Env("WATERMARKS_REWRITE_BASE_URL", "http://127.0.0.1:11434"), "backend base URL")
	rewriteCmd.Flags().BoolVar(&rewriteAllowRemote, "allow-remote", false,
		"allow non-loopback backend hosts (default: deny; content would leave this machine)")
	rewriteCmd.Flags().StringVar(&rewriteStrength, "strength", string(rewrite.Paraphrase),
		"paraphrase | backtranslate | structural | humanize | code")
	rewriteCmd.Flags().StringVar(&rewriteLang, "lang", "French", "pivot language for backtranslate")
	rewriteCmd.Flags().StringVar(&rewriteOrigLang, "original-lang", "English", "original language for backtranslate")
	rewriteCmd.Flags().Float64Var(&rewriteTimeout, "timeout", 120.0, "per-request timeout in seconds")
	rewriteCmd.Flags().Float64Var(&rewriteTemperature, "temperature", 0.9, "sampling temperature for the backend")
	rewriteCmd.Flags().IntVar(&rewriteCandidates, "candidates", 1, "number of rewrite candidates to generate and score")
	rewriteCmd.Flags().BoolVar(&rewriteNoLayerA, "no-layer-a-after", false, "skip the Layer A scrub on model output")
	rewriteCmd.Flags().StringVarP(&rewriteOutput, "output", "o", "", "write result here (default: stdout)")
	rewriteCmd.Flags().BoolVar(&rewriteJSON, "json-stats", false, "emit the info block as JSON on stderr")
	// NOTE: no --api-key flag on purpose — keys on argv leak via `ps` and shell
	// history. Set WATERMARKS_REWRITE_API_KEY instead.
	rootCmd.AddCommand(rewriteCmd)
}

func runRewrite(cmd *cobra.Command, args []string) error {
	text, err := readInput(args)
	if err != nil {
		return err
	}
	allowRemote := rewriteAllowRemote
	if !cmd.Flags().Changed("allow-remote") {
		allowRemote = rewrite.FlagEnv("WATERMARKS_REWRITE_ALLOW_REMOTE")
	}

	result, info, err := rewrite.Rewrite(text, rewrite.Options{
		Backend:      rewrite.Backend(rewriteBackend),
		Model:        rewriteModel,
		BaseURL:      rewriteBaseURL,
		APIKey:       rewrite.Env("WATERMARKS_REWRITE_API_KEY", ""),
		Strength:     rewrite.Strength(rewriteStrength),
		Lang:         rewriteLang,
		OriginalLang: rewriteOrigLang,
		Timeout:      time.Duration(rewriteTimeout * float64(time.Second)),
		LayerAAfter:  !rewriteNoLayerA,
		Temperature:  rewriteTemperature,
		Candidates:   rewriteCandidates,
		AllowRemote:  allowRemote,
	})
	if err != nil {
		return err
	}

	errw := cmd.ErrOrStderr()
	if info.Warning != "" {
		fmt.Fprintln(errw, info.Warning)
	}

	if rewriteOutput != "" {
		if err := os.WriteFile(rewriteOutput, []byte(result), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rewriteOutput, err)
		}
	} else {
		fmt.Fprint(cmd.OutOrStdout(), result)
		if result != "" && result[len(result)-1] != '\n' {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}

	if rewriteJSON {
		enc := json.NewEncoder(errw)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	fmt.Fprintf(errw, "backend=%s strength=%s mode=%s chars %d->%d\n",
		info.Backend, info.Strength, info.Mode, info.InputChars, info.OutputChars)
	return nil
}
