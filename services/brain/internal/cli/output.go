package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/library"
	"github.com/mad01/thismoon/services/brain/internal/markdown"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// includeRevokedHelp is the help of every search command's --include-revoked.
const includeRevokedHelp = "include superseded, retracted and withdrawn notes, hidden by default"

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

// open loads the config and opens every vault it names. It also decides
// whether text output qualifies paths with their vault: it does when more
// than one vault is configured.
func (a *app) open() (*library.Library, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	lib, err := library.Open(library.Options{Config: cfg})
	if err != nil {
		return nil, fmt.Errorf("%w (diagnose with: brain doctor)", err)
	}
	a.multiVault = len(cfg.Vaults) > 1
	return lib, nil
}

// openVaults opens the library and returns it with the vaults --vault
// names, or every vault when the flag is absent.
func (a *app) openVaults() (*library.Library, *vault.Set, error) {
	lib, err := a.open()
	if err != nil {
		return nil, nil, err
	}
	set, err := lib.Vaults(a.vaults)
	if err != nil {
		return nil, nil, err
	}
	return lib, set, nil
}

// locate opens the library and resolves a note ref within the --vault
// vaults. A not-found error carries a hint for finding the right path.
func (a *app) locate(ref string) (*library.Library, vault.Ref, error) {
	lib, err := a.open()
	if err != nil {
		return nil, vault.Ref{}, err
	}
	r, err := lib.Locate(a.vaults, ref)
	if err != nil {
		return nil, vault.Ref{}, withNoteHint(err, refPath(ref))
	}
	return lib, r, nil
}

// writeVault returns the one vault --vault names, or "" for the default.
func (a *app) writeVault() (string, error) {
	switch len(a.vaults) {
	case 0:
		return "", nil
	case 1:
		return a.vaults[0], nil
	}
	return "", usageErrorf("name at most one --vault to write to, got %d", len(a.vaults))
}

// show returns how text output prints a note: vault:path when more than one
// vault is configured, the bare path otherwise.
func (a *app) show(vaultName, path string) string {
	if a.multiVault && vaultName != "" {
		return vaultName + ":" + path
	}
	return path
}

// warn prints each warning as one stderr line.
func warn(cmd *cobra.Command, warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s: warning: %s\n", cmd.CommandPath(), w)
	}
}

// parseTierFlag parses an optional tier flag; empty means all tiers.
func parseTierFlag(name string) (vault.Tier, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	return vault.ParseTier(name)
}
