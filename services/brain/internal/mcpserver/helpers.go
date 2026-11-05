package mcpserver

import (
	"fmt"
	"time"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// loadConfig reads the config file the server was started with.
func (h *handlers) loadConfig() (*config.Config, error) {
	cfg, err := config.Load(h.configPath)
	if err != nil {
		return nil, fmt.Errorf("load brain config: %w (diagnose with: brain_doctor)", err)
	}
	return cfg, nil
}

// loadVault reads the brain config and opens the vault. Returned together so
// handlers that both inspect config (e.g. for paths) and call vault operations
// don't duplicate the Load/Open dance.
func (h *handlers) loadVault() (*config.Config, *vault.Vault, error) {
	cfg, err := h.loadConfig()
	if err != nil {
		return nil, nil, err
	}
	v, err := vault.Open(cfg.Vault)
	if err != nil {
		return nil, nil, fmt.Errorf("open vault: %w", err)
	}
	return cfg, v, nil
}

// ensureIndex makes sure the zoekt index in brain's state directory is
// current before a search/count call. It compares the vault fingerprint to the
// stored state and re-indexes if stale (or if no index exists yet). Progress
// is intentionally silent — MCP handlers shouldn't spam stderr mid-RPC.
func ensureIndex(cfg *config.Config) (string, error) {
	indexDir, err := brain.IndexDir()
	if err != nil {
		return "", fmt.Errorf("resolve index dir: %w", err)
	}
	fingerprint, _, err := search.ComputeFingerprint(cfg.Vault)
	if err != nil {
		return "", fmt.Errorf("fingerprint vault: %w", err)
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		return "", fmt.Errorf("load index state: %w", err)
	}
	if !search.IsStale(state, fingerprint) {
		return indexDir, nil
	}
	if err := search.IndexVault(cfg.Vault, indexDir, nil); err != nil {
		return "", fmt.Errorf("index vault: %w", err)
	}
	_, files, err := search.ComputeFingerprint(cfg.Vault)
	if err == nil {
		state.SetFingerprint(fingerprint, files, time.Now().UTC())
		_ = state.Save(indexDir)
	}
	return indexDir, nil
}
