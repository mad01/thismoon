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

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/services/brain"
)

// Config holds the brain vault configuration.
type Config struct {
	// Vault is the absolute path to the Obsidian-style markdown vault.
	Vault string `yaml:"vault"`
	// Semantic enables and tunes the optional vector-search layer over Ollama.
	// When Semantic.Enabled is false, the brain_semantic_search and
	// brain_hybrid_search MCP tools and matching CLI commands return a clear
	// "semantic disabled" error; the literal flow is unaffected.
	Semantic SemanticConfig `yaml:"semantic,omitempty"`
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
	// IndexDir is the on-disk location of the chromem-go collection. Default:
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
// parse, or it leaves out the vault. The CLI exits 2 on it rather than run
// on defaults, because a machine with a config file was configured on
// purpose.
var ErrInvalid = errors.New("invalid config")

// Load reads the config file at path. A missing file wraps fs.ErrNotExist;
// a present file that does not parse or names no vault wraps ErrInvalid.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no config at %s: %w", path, fs.ErrNotExist)
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrInvalid, path, err)
	}
	if cfg.Vault == "" {
		return nil, fmt.Errorf("%w %s: vault is required", ErrInvalid, path)
	}
	if cfg.Vault, err = confdir.Expand(cfg.Vault); err != nil {
		return nil, err
	}
	if err := applySemanticDefaults(&cfg.Semantic); err != nil {
		return nil, err
	}
	return &cfg, nil
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

// ResolvedIndexDir returns the configured semantic index directory, or the
// default under brain's state directory when unset.
func (s SemanticConfig) ResolvedIndexDir() (string, error) {
	if s.IndexDir != "" {
		return s.IndexDir, nil
	}
	return brain.SemanticDir()
}
