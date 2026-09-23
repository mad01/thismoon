package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/config"
)

// discoveryTimeout bounds asking a provider for its voices. Discovery runs
// when serve or mcp starts; a slow answer falls back to the catalog rather
// than hold up the start.
const discoveryTimeout = 3 * time.Second

// kokoroEnglish is the Kokoro-82M catalog speak falls back on: its American
// (a) and British (b) English voices. The local engine only has English G2P
// installed and its sandbox blocks fetching another language's, so those are
// the voices that can actually speak.
var kokoroEnglish = []string{
	"af_alloy", "af_aoede", "af_bella", "af_heart", "af_jessica", "af_kore",
	"af_nicole", "af_nova", "af_river", "af_sarah", "af_sky", "am_adam",
	"am_echo", "am_eric", "am_fenrir", "am_liam", "am_michael", "am_onyx",
	"am_puck", "am_santa", "bf_alice", "bf_emma", "bf_isabella", "bf_lily",
	"bm_daniel", "bm_fable", "bm_george", "bm_lewis",
}

// openAIVoices is OpenAI's built-in speech voice set.
var openAIVoices = []string{
	"alloy", "ash", "ballad", "coral", "echo", "fable",
	"nova", "onyx", "sage", "shimmer", "verse",
}

// resolveVoices builds a provider's voice list: the block's curated voices:
// when set, else whatever the provider can tell us, else a catalog for the
// model family, else the default voice alone.
func resolveVoices(ctx context.Context, p config.Provider) Voices {
	v := Voices{Default: p.Voice}
	if len(p.Voices) > 0 {
		v.List, v.Source = p.Voices, SourceConfig
		return v
	}
	var err error
	switch p.Type {
	case config.TypeLocal:
		v.List, err = cachedKokoroVoices(p.Model)
	case config.TypeOpenRouter:
		ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
		defer cancel()
		v.List, err = openRouterVoices(ctx, p.BaseURL, p.Model)
	case config.TypeOpenAI:
		v.List, v.Source = openAIVoices, SourceCatalog
	case config.TypeLiteLLM:
		v.Source = SourceDefault
		v.Note = "LiteLLM does not list voices; set voices: on the block to offer more"
		return v
	}
	switch {
	case v.Source != "":
	case err == nil && len(v.List) > 0:
		v.Source = SourceDiscovered
	case isKokoro(p.Model):
		v.List, v.Source = kokoroEnglish, SourceCatalog
		v.Note = "discovery failed, using the Kokoro catalog: " + errText(err)
	default:
		v.List, v.Source = nil, SourceDefault
		v.Note = "discovery failed: " + errText(err)
	}
	if len(v.List) > 0 && !slices.Contains(v.List, v.Default) {
		v.Note = fmt.Sprintf("default voice %q is not one %s offers", v.Default, p.Model)
	}
	return v
}

// cachedKokoroVoices lists the English voice packs of a Kokoro model in the
// Hugging Face cache, where the local engine loads them from. A pack only
// counts when its blob is present: the cache holds a symlink per file, and
// an interrupted download leaves the link without the blob.
func cachedKokoroVoices(model string) ([]string, error) {
	hub, err := hfHubCache()
	if err != nil {
		return nil, err
	}
	pattern := filepath.Join(hub, "models--"+strings.ReplaceAll(model, "/", "--"),
		"snapshots", "*", "voices", "*.safetensors")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("list voice packs: %w", err)
	}
	var voices []string
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".safetensors")
		if !isEnglishKokoro(name) || slices.Contains(voices, name) {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			voices = append(voices, name)
		}
	}
	if len(voices) == 0 {
		return nil, fmt.Errorf("no English voice packs for %s under %s", model, hub)
	}
	slices.Sort(voices)
	return voices, nil
}

// hfHubCache is where Hugging Face keeps downloaded models: HF_HUB_CACHE,
// else HF_HOME/hub, else ~/.cache/huggingface/hub.
func hfHubCache() (string, error) {
	if dir := os.Getenv("HF_HUB_CACHE"); dir != "" {
		return dir, nil
	}
	if home := os.Getenv("HF_HOME"); home != "" {
		return filepath.Join(home, "hub"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the Hugging Face cache: %w", err)
	}
	return filepath.Join(home, ".cache", "huggingface", "hub"), nil
}

// openRouterVoices asks OpenRouter's public models list which voices model
// supports. The list needs no key.
func openRouterVoices(ctx context.Context, baseURL, model string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/v1/models?output_modalities=speech", nil)
	if err != nil {
		return nil, fmt.Errorf("build models request: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list OpenRouter speech models: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list OpenRouter speech models: %s", res.Status)
	}
	var list struct {
		Data []struct {
			ID              string   `json:"id"`
			SupportedVoices []string `json:"supported_voices"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode OpenRouter models: %w", err)
	}
	for _, m := range list.Data {
		if m.ID == model {
			if len(m.SupportedVoices) == 0 {
				return nil, fmt.Errorf("OpenRouter lists no voices for %s", model)
			}
			return m.SupportedVoices, nil
		}
	}
	return nil, fmt.Errorf("OpenRouter has no speech model %s", model)
}

func isKokoro(model string) bool {
	return strings.Contains(strings.ToLower(model), "kokoro")
}

// isEnglishKokoro reports a Kokoro voice id whose language prefix is
// American (a) or British (b) English: af_, am_, bf_, bm_.
func isEnglishKokoro(name string) bool {
	return len(name) > 3 && (name[0] == 'a' || name[0] == 'b') &&
		(name[1] == 'f' || name[1] == 'm') && name[2] == '_'
}

func errText(err error) string {
	if err == nil {
		return "no voices returned"
	}
	return err.Error()
}
