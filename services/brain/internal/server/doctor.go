package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

// DoctorView is what GET /api/doctor answers: what serve itself can see,
// which `brain doctor` compares with what the CLI sees. Under a sandbox the
// two differ when a vault or the state directory is outside serve's
// allowed paths.
type DoctorView struct {
	// StateDir is the state directory serve resolved.
	StateDir string `json:"state_dir"`
	// LayoutDirWritable is whether serve can write <state dir>/layout/.
	LayoutDirWritable bool `json:"layout_dir_writable"`
	// Config is the config file serve was pointed at.
	Config string `json:"config"`
	// ConfigError says why serve runs with no vault, when it could not
	// load Config.
	ConfigError string `json:"config_error,omitempty"`
	// Vaults are the configured vaults, readable or not.
	Vaults []DoctorVault `json:"vaults"`
}

// DoctorVault is one configured vault as serve sees it.
type DoctorVault struct {
	Name string `json:"name"`
	// Path is the vault's path as the config names it.
	Path string `json:"path"`
	// Resolved is Path with symlinks resolved, the path a sandbox rule must
	// name; Path when it does not resolve.
	Resolved string `json:"resolved"`
	Readable bool   `json:"readable"`
	// Error says why the vault is not readable; empty when it is.
	Error string `json:"error"`
}

// handleDoctor probes the vaults and the layout directory now, without the
// model, so a vault that cannot be read is named rather than hidden.
func (s *Server) handleDoctor(w http.ResponseWriter, _ *http.Request) {
	view := DoctorView{
		StateDir:          s.stateDir,
		LayoutDirWritable: s.probeLayoutDir() == nil,
		Config:            s.configPath,
		Vaults:            []DoctorVault{},
	}
	if s.configErr != nil {
		view.ConfigError = s.configErr.Error()
	}
	for _, v := range s.cfg.Vaults {
		d := DoctorVault{Name: v.Name, Path: v.Path, Resolved: v.Path}
		if resolved, err := filepath.EvalSymlinks(v.Path); err == nil {
			d.Resolved = resolved
		}
		if err := readable(v.Path); err != nil {
			d.Error = err.Error()
		} else {
			d.Readable = true
		}
		view.Vaults = append(view.Vaults, d)
	}
	writeJSON(w, http.StatusOK, view)
}

// probeLayoutDir creates the layout directory when it is missing, then
// creates and removes a file in it. It writes nowhere else.
func (s *Server) probeLayoutDir() error {
	if err := os.MkdirAll(s.layoutDir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.layoutDir, ".probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	return errors.Join(f.Close(), os.Remove(name))
}
