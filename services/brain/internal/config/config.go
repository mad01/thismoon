// Package config loads the brain configuration from ~/.config/brain/config.yaml.
//
// Ported from kitty-session's internal/repo/config with a single-vault shape.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"

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
// All fields are optional. Defaults are applied in LoadFrom when empty.
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
	// ~/.config/brain/semantic. Tilde is expanded.
	IndexDir string `yaml:"index_dir,omitempty"`
	// ChunkSize is the target token count per chunk. Default: 800.
	ChunkSize int `yaml:"chunk_size,omitempty"`
	// ChunkOverlap is the token overlap between adjacent chunks. Default: 100.
	ChunkOverlap int `yaml:"chunk_overlap,omitempty"`
	// RerankDefault sets the default value of the rerank tool argument when
	// the caller doesn't pass one. Default: false (reranker is opt-in).
	RerankDefault bool `yaml:"rerank_default,omitempty"`
}

// Defaults applied in LoadFrom when fields are empty.
const (
	defaultOllamaURL    = "http://localhost:11434"
	defaultEmbedModel   = "nomic-embed-text"
	defaultChatModel    = "llama3.2:3b"
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
)

// Load reads config.yaml from ~/.config/brain/config.yaml.
func Load() (*Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := LoadFrom(path)
	if err != nil {
		return nil, fmt.Errorf("no config found (checked %s): %w", path, err)
	}
	return cfg, nil
}

// LoadFrom reads config from a specific path.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	cfg.Vault = expandTilde(cfg.Vault)
	if cfg.Vault == "" {
		return nil, fmt.Errorf("parsing %s: vault field is required", path)
	}

	applySemanticDefaults(&cfg.Semantic)

	return &cfg, nil
}

// applySemanticDefaults fills empty SemanticConfig fields with sensible
// defaults. Called from LoadFrom so callers always observe a well-formed
// SemanticConfig regardless of which fields the user specified.
func applySemanticDefaults(s *SemanticConfig) {
	if s.OllamaURL == "" {
		s.OllamaURL = defaultOllamaURL
	}
	if s.EmbedModel == "" {
		s.EmbedModel = defaultEmbedModel
	}
	if s.ChatModel == "" {
		s.ChatModel = defaultChatModel
	}
	if s.ChunkSize <= 0 {
		s.ChunkSize = defaultChunkSize
	}
	if s.ChunkOverlap < 0 {
		s.ChunkOverlap = defaultChunkOverlap
	}
	if s.ChunkOverlap == 0 {
		s.ChunkOverlap = defaultChunkOverlap
	}
	s.IndexDir = expandTilde(s.IndexDir)
}

// DefaultSemanticDir returns the canonical semantic index directory at
// ~/.config/brain/semantic/. Used when SemanticConfig.IndexDir is empty.
func DefaultSemanticDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "brain", "semantic"), nil
}

// SemanticIndexDir returns the configured semantic index directory, or the
// default when unset.
func (s SemanticConfig) ResolvedIndexDir() (string, error) {
	if s.IndexDir != "" {
		return s.IndexDir, nil
	}
	return DefaultSemanticDir()
}

// DefaultPath returns the canonical config path at ~/.config/brain/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "brain", configFileName), nil
}

// DefaultStateDir returns the canonical state directory at ~/.config/brain.
func DefaultStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "brain"), nil
}

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
