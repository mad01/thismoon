// Package provider builds the active TTS backend from speak's config: the
// client that synthesizes, the voices it offers, and the name and model its
// health is reported under. serve, mcp and doctor each build one the same
// way, so they cannot disagree about which provider, model or voice is in
// use.
//
// A config that cannot produce a working client still yields a Provider,
// one whose every synthesis fails with a config-kind error naming the
// problem. serve and mcp then start and report it on every surface instead
// of refusing to run or quietly falling back to another provider.
package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/gemini"
	"github.com/mad01/thismoon/services/speak/internal/tts"
	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

// synthesizer is a backend client: ttsclient for the OpenAI-style
// endpoints, gemini for the Gemini API.
type synthesizer interface {
	Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error)
}

// Provider is the active TTS backend.
type Provider struct {
	cfg    config.Provider
	synth  synthesizer       // nil when broken
	engine *ttsclient.Client // the local engine, for Ping; nil otherwise
	broken *tts.Error        // set when the config cannot produce a client
	voices Voices
}

// New builds the provider for a resolved config block, discovering its
// voices unless the block curates them. A block with a Problem yields a
// broken Provider that reports it.
func New(ctx context.Context, p config.Provider) *Provider {
	if p.Problem != "" {
		return &Provider{
			cfg:    p,
			broken: configError(p.Name, fmt.Sprintf("provider %q: %s", p.Name, p.Problem)),
			voices: Voices{Default: p.Voice, Source: SourceDefault},
		}
	}
	prov := &Provider{cfg: p, voices: resolveVoices(ctx, p)}
	if p.Type == config.TypeGemini {
		prov.synth = gemini.New(gemini.Config{
			Provider:  p.Name,
			BaseURL:   p.BaseURL,
			APIKey:    p.APIKey,
			APIKeyEnv: p.APIKeyEnv,
			Model:     p.Model,
		})
		return prov
	}
	client := ttsclient.New(ttsclient.Config{
		Provider:  p.Name,
		Local:     p.Type == config.TypeLocal,
		BaseURL:   p.BaseURL,
		APIKey:    p.APIKey,
		APIKeyEnv: p.APIKeyEnv,
		Model:     p.Model,
		Format:    p.Format,
	})
	prov.synth = client
	if p.Type == config.TypeLocal {
		prov.engine = client
	}
	return prov
}

// Broken returns a provider for a config that could not be loaded at all
// (an unparseable file, an active provider with no block). name is the
// provider that was asked for, reported in health.
func Broken(name string, err error) *Provider {
	return &Provider{
		cfg:    config.Provider{Name: name},
		broken: configError(name, err.Error()),
	}
}

func configError(name, msg string) *tts.Error {
	return &tts.Error{Kind: tts.KindConfig, Provider: name, Message: msg}
}

// Name is the config block's name, what health and error bodies report.
func (p *Provider) Name() string { return p.cfg.Name }

// Model is the model the provider synthesizes with.
func (p *Provider) Model() string { return p.cfg.Model }

// Config is the resolved block the provider was built from.
func (p *Provider) Config() config.Provider { return p.cfg }

// Voices is the resolved voice list.
func (p *Provider) Voices() Voices { return p.voices }

// Err is the config problem every synthesis fails with, or nil for a
// working provider.
func (p *Provider) Err() error {
	if p.broken == nil {
		return nil
	}
	return p.broken
}

// NewHealth returns a health tracker reporting under this provider's name
// and model.
func (p *Provider) NewHealth() *tts.Health {
	return tts.NewHealth(p.cfg.Name, p.cfg.Model)
}

// Synthesize speaks req through the provider. An empty voice, or one the
// provider does not offer, becomes the default voice: the read-aloud
// component sends the Kokoro default to every provider. A broken provider
// fails with its config error.
func (p *Provider) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	if p.broken != nil {
		return tts.Audio{}, p.broken
	}
	req.Voice = p.voices.Resolve(req.Voice)
	return p.synth.Synthesize(ctx, req)
}

// ClipID names what decides a clip's sound besides its text and speed: the
// provider, its model, and the voice a request for voice resolves to. speak
// serve's audio cache keys clips by it, so switching provider, model or
// voice never replays a clip made the old way.
func (p *Provider) ClipID(voice string) string {
	return fmt.Sprintf("%q %q %q", p.cfg.Name, p.cfg.Model, p.voices.Resolve(voice))
}

// Ping checks that a local engine is listening. It reports false, nil for a
// remote provider, where there is no process of ours to ping.
func (p *Provider) Ping(ctx context.Context) (checked bool, err error) {
	if p.engine == nil {
		return false, nil
	}
	return true, p.engine.Reachable(ctx)
}

// Voices is a provider's resolved voice list.
type Voices struct {
	Default string   `json:"default"`
	List    []string `json:"voices,omitempty"` // empty = unknown: any voice passes through
	Source  string   `json:"source"`           // config, discovered, catalog, or default
	Note    string   `json:"note,omitempty"`   // why discovery fell back, or a mismatch
}

// Where a voice list came from.
const (
	SourceConfig     = "config"     // the block's voices: list
	SourceDiscovered = "discovered" // asked the provider, or found on disk
	SourceCatalog    = "catalog"    // speak's built-in list for the model family
	SourceDefault    = "default"    // nothing known beyond the default voice
)

// Resolve maps a requested voice to one to send: empty becomes the default,
// and so does a voice the list is known not to contain. With no list, any
// voice passes through for the provider to judge.
func (v Voices) Resolve(voice string) string {
	switch {
	case voice == "":
		return v.Default
	case len(v.List) > 0 && !slices.Contains(v.List, voice):
		return v.Default
	default:
		return voice
	}
}
