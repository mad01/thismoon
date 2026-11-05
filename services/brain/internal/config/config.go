// Package config loads the brain configuration file. Which file is the
// caller's choice (--config, BRAIN_CONFIG, or the default under the config
// directory); config.md beside the service documents the keys and the
// parse-failure posture.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/brain"
)

// Config holds the brain vaults and the semantic-layer settings.
type Config struct {
	// Vaults are the configured vaults in config order, which is also the
	// order shared vaults resolve wikilinks in. Load guarantees at least one,
	// unique names and paths, and at most one Default.
	Vaults []Vault
	// Semantic enables and tunes the optional vector-search layer over Ollama.
	// When Semantic.Enabled is false, the brain_semantic_search and
	// brain_hybrid_search MCP tools and matching CLI commands return a clear
	// "semantic disabled" error; the literal flow is unaffected.
	Semantic SemanticConfig
	// CodeRoots are the directories brain drift finds local checkouts in:
	// a code ref's repo host/owner/name is <root>/<host>/<owner>/<name>.
	// Each is cleaned with a leading ~ expanded; empty leaves every code
	// ref unresolved.
	CodeRoots []string
}

// Vault is one configured vault.
type Vault struct {
	// Name is the vault's lowercase slug: the prefix of a qualified ref
	// (name:path) and the name of its index directories.
	Name string `yaml:"name"`
	// Path is the vault root, cleaned, with a leading ~ expanded.
	Path string `yaml:"path"`
	// Default marks the vault writes target when no vault is named.
	Default bool `yaml:"default,omitempty"`
	// Shared marks a team vault: readable and searchable, but write, edit
	// and promote refuse it, because shared notes change through a PR.
	Shared bool `yaml:"shared,omitempty"`
	// AutoCommit, when false, stops brain committing its changes in a vault
	// that is a git repo. Unset means true.
	AutoCommit *bool `yaml:"autocommit,omitempty"`
}

// Commits reports whether brain commits its changes to the vault: true
// unless autocommit is false.
func (v Vault) Commits() bool {
	return v.AutoCommit == nil || *v.AutoCommit
}

// file is the config file as written: either the legacy single vault path
// or a vaults list, plus the semantic block.
type file struct {
	Vault     string         `yaml:"vault"`
	Vaults    []Vault        `yaml:"vaults"`
	Semantic  SemanticConfig `yaml:"semantic,omitempty"`
	CodeRoots []string       `yaml:"code_roots,omitempty"`
}

// SemanticConfig configures the optional semantic search layer (Ollama
// embeddings + chromem-go vector store + RRF hybrid + optional reranker).
//
// All fields are optional. Load fills empty ones from the defaults in the
// brain package.
type SemanticConfig struct {
	// Enabled turns the semantic layer on. When false (default), the tools are
	// still registered but return a typed "disabled" error so agents know.
	Enabled bool `yaml:"enabled,omitempty"`
	// OllamaURL is the base URL of the local Ollama instance.
	// Default: http://localhost:11434.
	OllamaURL string `yaml:"ollama_url,omitempty"`
	// EmbedModel is the Ollama embedding model. Default: nomic-embed-text.
	EmbedModel string `yaml:"embed_model,omitempty"`
	// ChatModel is the Ollama chat model used by the optional reranker.
	// Default: llama3.2:3b.
	ChatModel string `yaml:"chat_model,omitempty"`
	// IndexDir is the parent of every vault's chromem-go directory. Default:
	// the semantic directory under brain's state directory. Tilde is expanded.
	IndexDir string `yaml:"index_dir,omitempty"`
	// ChunkSize is the target token count per chunk. Default: 800.
	ChunkSize int `yaml:"chunk_size,omitempty"`
	// ChunkOverlap is the token overlap between adjacent chunks. Default: 100.
	ChunkOverlap int `yaml:"chunk_overlap,omitempty"`
	// RerankDefault sets the default value of the rerank tool argument when
	// the caller doesn't pass one. Default: false (reranker is opt-in).
	RerankDefault bool `yaml:"rerank_default,omitempty"`
}

// ErrInvalid marks a config file that exists but cannot be used: it does not
// parse, names no vault, or describes its vaults inconsistently. The CLI
// exits 2 on it rather than run on defaults, because a machine with a config
// file was configured on purpose.
var ErrInvalid = errors.New("invalid config")

// vaultName is the shape of a vault name: a lowercase slug.
var vaultName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedNames are the ref prefixes brain gives a meaning of its own
// (id:<ulid>, and the graph's tag: and dangling: node ids), so no vault may
// take them.
var reservedNames = map[string]bool{"id": true, "tag": true, "dangling": true}

// Load reads the config file at path. A missing file wraps fs.ErrNotExist;
// a present file that does not parse or describes no usable vault wraps
// ErrInvalid.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no config at %s: %w", path, fs.ErrNotExist)
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw file
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrInvalid, path, err)
	}
	vaults, err := parseVaults(raw)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrInvalid, path, err)
	}
	roots, err := expandPaths(raw.CodeRoots)
	if err != nil {
		return nil, fmt.Errorf("%w %s: code_roots: %w", ErrInvalid, path, err)
	}
	cfg := &Config{Vaults: vaults, Semantic: raw.Semantic, CodeRoots: roots}
	if err := applySemanticDefaults(&cfg.Semantic); err != nil {
		return nil, err
	}
	return cfg, nil
}

// expandPaths cleans each path with a leading ~ expanded. An empty entry
// is an error.
func expandPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return nil, errors.New("an entry is empty")
		}
		expanded, err := confdir.Expand(p)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.Clean(expanded))
	}
	return out, nil
}

// parseVaults turns the legacy vault key or the vaults list into validated
// vaults with the default resolved.
func parseVaults(raw file) ([]Vault, error) {
	vaults := raw.Vaults
	switch {
	case raw.Vault != "" && len(raw.Vaults) > 0:
		return nil, errors.New("set vault or vaults, not both")
	case raw.Vault != "":
		vaults = []Vault{{Name: brain.LegacyVaultName, Path: raw.Vault}}
	case len(vaults) == 0:
		return nil, errors.New("vault is required (a path, or a vaults list)")
	}
	names := make(map[string]bool, len(vaults))
	paths := make(map[string]string, len(vaults))
	for i := range vaults {
		v := &vaults[i]
		if err := checkName(v.Name, names); err != nil {
			return nil, err
		}
		names[v.Name] = true
		if strings.TrimSpace(v.Path) == "" {
			return nil, fmt.Errorf("vault %s: path is required", v.Name)
		}
		expanded, err := confdir.Expand(v.Path)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", v.Name, err)
		}
		v.Path = filepath.Clean(expanded)
		if other, ok := paths[v.Path]; ok {
			return nil, fmt.Errorf("vaults %s and %s share the path %s", other, v.Name, v.Path)
		}
		paths[v.Path] = v.Name
	}
	return vaults, resolveDefault(vaults)
}

func checkName(name string, seen map[string]bool) error {
	switch {
	case name == "":
		return errors.New("every vault needs a name")
	case !vaultName.MatchString(name):
		return fmt.Errorf(
			"vault name %q must be a lowercase slug (a-z, 0-9, single hyphens)",
			name,
		)
	case reservedNames[name]:
		return fmt.Errorf("vault name %q is reserved", name)
	case seen[name]:
		return fmt.Errorf("vault name %q is used twice", name)
	}
	return nil
}

// resolveDefault checks the default marks and, when none is set, makes the
// first non-shared vault the default. A config of shared vaults alone has no
// default and refuses every write.
func resolveDefault(vaults []Vault) error {
	marked := -1
	for i, v := range vaults {
		if !v.Default {
			continue
		}
		if v.Shared {
			return fmt.Errorf("vault %s is shared and cannot be the default", v.Name)
		}
		if marked >= 0 {
			return fmt.Errorf("vaults %s and %s are both default", vaults[marked].Name, v.Name)
		}
		marked = i
	}
	if marked >= 0 {
		return nil
	}
	for i := range vaults {
		if !vaults[i].Shared {
			vaults[i].Default = true
			return nil
		}
	}
	return nil
}

// applySemanticDefaults fills empty SemanticConfig fields from the brain
// package defaults, so callers always see a well-formed SemanticConfig.
func applySemanticDefaults(s *SemanticConfig) error {
	if s.OllamaURL == "" {
		s.OllamaURL = brain.DefaultOllamaURL
	}
	if s.EmbedModel == "" {
		s.EmbedModel = brain.DefaultEmbedModel
	}
	if s.ChatModel == "" {
		s.ChatModel = brain.DefaultChatModel
	}
	if s.ChunkSize <= 0 {
		s.ChunkSize = brain.DefaultChunkSize
	}
	if s.ChunkOverlap <= 0 {
		s.ChunkOverlap = brain.DefaultChunkOverlap
	}
	var err error
	s.IndexDir, err = confdir.Expand(s.IndexDir)
	return err
}

// ResolvedIndexDir returns the semantic index directory of the named vault:
// <index_dir>/<vault> when index_dir is set, else the default under brain's
// state directory.
func (s SemanticConfig) ResolvedIndexDir(vault string) (string, error) {
	if s.IndexDir != "" {
		return filepath.Join(s.IndexDir, vault), nil
	}
	return brain.SemanticDir(vault)
}
