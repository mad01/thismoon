package backend

import (
	"errors"
	"os"
)

const (
	liteLLMName = "litellm"
	// liteLLMDefaultModel is the Anthropic-native id for Haiku 4.5. A LiteLLM
	// proxy serves whatever aliases its model_list declares, so a deployment
	// that names the model differently overrides this with HUMANIZER_MODEL.
	liteLLMDefaultModel = "claude-haiku-4-5-20251001"
)

// NewLiteLLM returns a backend for a LiteLLM proxy (self-hosted or a
// company gateway), which serves the same OpenAI-compatible chat-completions
// surface as OpenRouter. BaseURL is required, without the /v1 suffix; APIKey
// is optional because a proxy may run without a master key; Model defaults
// to Haiku 4.5 under its Anthropic id.
func NewLiteLLM(cfg ChatConfig) (*ChatClient, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("backend: LITELLM_BASE_URL is not set")
	}
	if cfg.Model == "" {
		cfg.Model = liteLLMDefaultModel
	}
	return newChatClient(liteLLMName, cfg), nil
}

func newLiteLLMFromEnv(model string) (Backend, error) {
	c, err := NewLiteLLM(ChatConfig{
		APIKey:  os.Getenv("LITELLM_API_KEY"),
		BaseURL: os.Getenv("LITELLM_BASE_URL"),
		Model:   envModel(model),
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
