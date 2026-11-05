package mcpserver

import (
	"fmt"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/library"
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

// open loads the config and opens every vault it names. Each call reads the
// config afresh, so an edit to it applies to the next tool call.
func (h *handlers) open() (*library.Library, error) {
	cfg, err := h.loadConfig()
	if err != nil {
		return nil, err
	}
	lib, err := library.Open(library.Options{Config: cfg, Embed: h.embed, Health: h.health})
	if err != nil {
		return nil, fmt.Errorf("%w (diagnose with: brain_doctor)", err)
	}
	return lib, nil
}

// vaults opens the library and returns the vault the vault input names, or
// every vault when it is empty.
func (h *handlers) vaults(name string) (*library.Library, *vault.Set, error) {
	lib, err := h.open()
	if err != nil {
		return nil, nil, err
	}
	set, err := lib.Vaults(names(name))
	if err != nil {
		return nil, nil, err
	}
	return lib, set, nil
}

// locate opens the library and resolves a note ref within the named vault,
// or within every vault when name is empty.
func (h *handlers) locate(name, ref string) (*library.Library, vault.Ref, error) {
	lib, err := h.open()
	if err != nil {
		return nil, vault.Ref{}, err
	}
	r, err := lib.Locate(names(name), ref)
	if err != nil {
		return nil, vault.Ref{}, err
	}
	return lib, r, nil
}

func names(name string) []string {
	if name == "" {
		return nil
	}
	return []string{name}
}
