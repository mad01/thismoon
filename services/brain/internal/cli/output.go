package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Output formats for the persistent -o flag.
const (
	outputText = "text"
	outputJSON = "json"
)

// emit writes v as indented JSON under -o json, otherwise calls text.
func (a *app) emit(w io.Writer, v any, text func(io.Writer) error) error {
	if a.output == outputJSON {
		return writeJSON(w, v)
	}
	return text(w)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// writeFrontmatter prints fm as a fenced YAML block, the way a note stores it.
func writeFrontmatter(w io.Writer, fm markdown.Frontmatter) error {
	yml, err := markdown.Encode(fm)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "---")
	fmt.Fprint(w, string(yml))
	fmt.Fprintln(w, "---")
	return nil
}

// loadConfig loads the config file chosen with --config. Errors point at
// brain doctor, which names the broken piece.
func (a *app) loadConfig() (*config.Config, error) {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return nil, fmt.Errorf("%w (diagnose with: brain doctor)", err)
	}
	return cfg, nil
}

// openVault loads the config and opens its vault.
func (a *app) openVault() (*config.Config, *vault.Vault, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, nil, err
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return nil, nil, fmt.Errorf("%w (diagnose with: brain doctor)", err)
	}
	return cfg, v, nil
}

// parseTierFlag parses an optional tier flag; empty means all tiers.
func parseTierFlag(name string) (vault.Tier, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	return vault.ParseTier(name)
}
