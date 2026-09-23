// Package config reads speak's provider configuration: one YAML file with a
// block per TTS provider, every block configured side by side, and a
// top-level provider line naming the one in use. Switching providers is a
// one-line edit, or --provider / SPEAK_PROVIDER for a single run.
//
// Only the active block has to be usable. A block that cannot be used as
// written (a missing key, an unknown type) carries a Problem instead of
// failing the load, so doctor can list it and the active provider keeps
// working. What does fail the load is a file that cannot be parsed or an
// active provider that names no block; callers turn that into a config
// failure every surface reports, never a quiet fallback.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	speak "github.com/mad01/thismoon/services/speak"
)

// Type is the implementation a provider block uses. Serialized as a string.
type Type string

const (
	// TypeLocal is the Kokoro engine on this machine (mlx-audio).
	TypeLocal Type = "local"
	// TypeOpenRouter is OpenRouter's speech endpoint.
	TypeOpenRouter Type = "openrouter"
	// TypeOpenAI is OpenAI's speech endpoint.
	TypeOpenAI Type = "openai"
	// TypeLiteLLM is a LiteLLM proxy, which routes to whatever backend it is
	// configured for.
	TypeLiteLLM Type = "litellm"
)

// types lists the known types, in the order error messages name them.
var types = []Type{TypeLocal, TypeOpenRouter, TypeOpenAI, TypeLiteLLM}

// File is the on-disk shape of the config file.
type File struct {
	Provider  string           `yaml:"provider"`
	Providers map[string]Block `yaml:"providers"`
}

// Block is one provider as written in the file. Every field is optional;
// the type's defaults fill the gaps.
type Block struct {
	Type       string   `yaml:"type"`         // defaults to the block's name
	BaseURL    string   `yaml:"base_url"`     // endpoint root, without /v1
	BaseURLEnv string   `yaml:"base_url_env"` // env var holding the base URL
	APIKeyEnv  string   `yaml:"api_key_env"`  // env var holding the API key
	Model      string   `yaml:"model"`
	Voice      string   `yaml:"voice"`  // the default voice
	Voices     []string `yaml:"voices"` // curated voices to offer; empty = discover
}

// Provider is one resolved block: the type's defaults applied, the API key
// read from its environment variable, and Problem set when the block cannot
// be used as written.
type Provider struct {
	Name      string   `json:"name"`
	Type      Type     `json:"type"`
	BaseURL   string   `json:"base_url"`
	APIKeyEnv string   `json:"api_key_env,omitempty"`
	APIKey    string   `json:"-"` // never printed
	Model     string   `json:"model"`
	Voice     string   `json:"voice"`
	Voices    []string `json:"voices,omitempty"` // the curated list; nil = discover
	Format    string   `json:"-"`                // response_format to request
	Remote    bool     `json:"remote"`           // text leaves this machine
	Env       []string `json:"env,omitempty"`    // env vars the block reads (names only)
	Problem   string   `json:"problem,omitempty"`
}

// Config is the whole resolved configuration.
type Config struct {
	Path      string     `json:"path"`
	Found     bool       `json:"found"` // whether the file existed
	Active    string     `json:"active"`
	Providers []Provider `json:"providers"` // sorted by name; includes the active one
}

// Options carries the overrides that outrank the file, each already
// resolved from its flag and environment variable.
type Options struct {
	Path     string              // the config file to read
	Provider string              // --provider / SPEAK_PROVIDER; "" = the file's provider line
	TTSURL   string              // --tts-url / SPEAK_TTS_URL; overrides local blocks' base_url
	Getenv   func(string) string // os.Getenv outside tests
}

// ActiveProvider returns the provider in use. Load guarantees it exists.
func (c Config) ActiveProvider() Provider {
	for _, p := range c.Providers {
		if p.Name == c.Active {
			return p
		}
	}
	return Provider{Name: c.Active, Problem: "not configured"}
}

// Load reads and resolves the configuration. A missing file is not an
// error: speak then runs the one implicit local provider, as it did before
// the file existed.
func Load(opts Options) (Config, error) {
	if opts.Getenv == nil {
		opts.Getenv = os.Getenv
	}
	file, found, err := readFile(opts.Path)
	if err != nil {
		return Config{}, err
	}
	active := firstNonEmpty(opts.Provider, file.Provider, speak.DefaultProvider)
	blocks := file.Providers
	if blocks == nil {
		blocks = map[string]Block{}
	}
	if _, ok := blocks[active]; !ok {
		// A provider named after a type needs no block: --provider
		// openrouter works with nothing but the key in the environment.
		if !slices.Contains(types, Type(active)) {
			return Config{}, fmt.Errorf("config: provider %q has no block in %s (configured: %s)",
				active, opts.Path, strings.Join(sortedNames(blocks), ", "))
		}
		blocks[active] = Block{}
	}
	cfg := Config{Path: opts.Path, Found: found, Active: active}
	for _, name := range sortedNames(blocks) {
		cfg.Providers = append(cfg.Providers, resolve(name, blocks[name], opts))
	}
	return cfg, nil
}

// readFile parses the config file, rejecting unknown fields so a typo such
// as api_key (a key pasted into the file) fails loudly instead of being
// ignored.
func readFile(path string) (File, bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var file File
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return File{}, true, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return file, true, nil
}

// resolve applies the type's defaults to a block and checks it is usable.
func resolve(name string, b Block, opts Options) Provider {
	p := Provider{
		Name:      name,
		Type:      Type(firstNonEmpty(b.Type, name)),
		APIKeyEnv: b.APIKeyEnv,
		Model:     b.Model,
		Voice:     b.Voice,
		Voices:    b.Voices,
	}
	baseURL := firstNonEmpty(b.BaseURL, envValue(opts.Getenv, b.BaseURLEnv))
	if b.BaseURL == "" && b.BaseURLEnv != "" {
		p.Env = append(p.Env, b.BaseURLEnv)
	}
	switch p.Type {
	case TypeLocal:
		baseURL = firstNonEmpty(opts.TTSURL, baseURL, speak.DefaultTTSURL)
		p.Model = firstNonEmpty(p.Model, speak.DefaultModel)
		p.Voice = firstNonEmpty(p.Voice, speak.DefaultVoice)
		p.Format = "wav"
	case TypeOpenRouter:
		baseURL = firstNonEmpty(baseURL, speak.OpenRouterBaseURL)
		p.APIKeyEnv = firstNonEmpty(p.APIKeyEnv, speak.OpenRouterKeyEnv)
		p.Model = firstNonEmpty(p.Model, speak.OpenRouterModel)
		p.Voice = firstNonEmpty(p.Voice, speak.DefaultVoice)
		p.Format = "pcm" // OpenRouter offers mp3 or pcm, no wav
	case TypeOpenAI:
		baseURL = firstNonEmpty(baseURL, speak.OpenAIBaseURL)
		p.APIKeyEnv = firstNonEmpty(p.APIKeyEnv, speak.OpenAIKeyEnv)
		p.Model = firstNonEmpty(p.Model, speak.OpenAIModel)
		p.Voice = firstNonEmpty(p.Voice, speak.OpenAIVoice)
		p.Format = "wav"
	case TypeLiteLLM:
		if b.BaseURL == "" && b.BaseURLEnv == "" {
			baseURL = envValue(opts.Getenv, speak.LiteLLMBaseURLEnv)
			p.Env = append(p.Env, speak.LiteLLMBaseURLEnv)
		}
		p.APIKeyEnv = firstNonEmpty(p.APIKeyEnv, speak.LiteLLMKeyEnv)
		p.Voice = firstNonEmpty(p.Voice, speak.LiteLLMVoice)
		p.Format = "wav"
	default:
		p.Problem = fmt.Sprintf("unknown type %q (set type: to one of %s)", p.Type, typeList())
		return p
	}
	p.APIKey = envValue(opts.Getenv, p.APIKeyEnv)
	if p.APIKeyEnv != "" {
		p.Env = append(p.Env, p.APIKeyEnv)
	}
	if len(p.Voices) > 0 && b.Voice == "" {
		p.Voice = p.Voices[0]
	}
	if len(p.Voices) > 0 && !slices.Contains(p.Voices, p.Voice) {
		p.Voices = append([]string{p.Voice}, p.Voices...)
	}
	if baseURL == "" {
		p.Problem = noBaseURL(p.Type, b)
		return p
	}
	p.BaseURL, p.Remote, p.Problem = checkBaseURL(baseURL)
	if p.Problem == "" {
		p.Problem = missing(p)
	}
	return p
}

// noBaseURL says where a block's base URL was expected to come from.
func noBaseURL(t Type, b Block) string {
	switch {
	case b.BaseURLEnv != "":
		return b.BaseURLEnv + " is not set"
	case t == TypeLiteLLM:
		return speak.LiteLLMBaseURLEnv + " is not set (or set base_url)"
	default:
		return "no base URL: set base_url, or base_url_env naming a variable that holds it"
	}
}

// missing names what a block of a remote type lacks, or "". A LiteLLM key
// is optional: a proxy without a master key needs none.
func missing(p Provider) string {
	switch {
	case p.Type == TypeLiteLLM && p.Model == "":
		return "model is required: name the model your LiteLLM proxy routes speech to"
	case (p.Type == TypeOpenRouter || p.Type == TypeOpenAI) && p.APIKey == "":
		return p.APIKeyEnv + " is not set"
	}
	return ""
}

// checkBaseURL normalizes a base URL (no trailing slash or /v1: the client
// adds the path) and reports whether it points off this machine.
func checkBaseURL(raw string) (base string, remote bool, problem string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return raw, false, fmt.Sprintf("base_url %q is not an http(s) URL", raw)
	}
	base = strings.TrimSuffix(strings.TrimRight(raw, "/"), "/v1")
	return base, !isLoopback(u.Hostname()), ""
}

// isLoopback reports whether host names this machine.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func envValue(getenv func(string) string, name string) string {
	if name == "" {
		return ""
	}
	return strings.TrimSpace(getenv(name))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func sortedNames(blocks map[string]Block) []string {
	names := make([]string, 0, len(blocks))
	for name := range blocks {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func typeList() string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return strings.Join(names, ", ")
}
