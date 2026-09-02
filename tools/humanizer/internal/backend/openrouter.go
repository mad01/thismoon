package backend

import (
	"errors"
	"os"
)

const (
	openRouterName           = "openrouter"
	openRouterDefaultModel   = "anthropic/claude-haiku-4.5"
	openRouterDefaultBaseURL = "https://openrouter.ai/api"
)

// NewOpenRouter returns a backend for the OpenRouter chat-completions API,
// routing to anthropic/claude-haiku-4.5 by default. It fails when APIKey is
// empty; Model and BaseURL default.
func NewOpenRouter(cfg ChatConfig) (*ChatClient, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("backend: OPENROUTER_API_KEY is not set")
	}
	if cfg.Model == "" {
		cfg.Model = openRouterDefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = openRouterDefaultBaseURL
	}
	return newChatClient(openRouterName, cfg), nil
}

func newOpenRouterFromEnv(model string) (Backend, error) {
	c, err := NewOpenRouter(ChatConfig{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  envModel(model),
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
