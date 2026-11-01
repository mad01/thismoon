package llm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selectEnv is every env var Select consults. Tests pin all of them so the
// host environment (which may carry a real key or proxy URL) cannot leak in.
var selectEnv = []string{"OPENROUTER_API_KEY", "LITELLM_BASE_URL", "LITELLM_API_KEY"}

// setPath points PATH at a directory that holds a fake claude when onPath
// is set, and at an empty directory otherwise, so LookPath answers the way
// the case needs.
func setPath(t *testing.T, onPath bool) {
	t.Helper()
	dir := t.TempDir()
	if onPath {
		path := filepath.Join(dir, claudeBin)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestSelect(t *testing.T) {
	cases := []struct {
		name        string
		opts        Options
		env         map[string]string
		claudeOn    bool
		wantName    string
		wantModel   string
		wantErr     error
		wantErrText string
	}{
		{
			name:    "nothing configured",
			wantErr: ErrNoBackend,
		},
		{
			name:     "claude on PATH is not picked unless allowed",
			claudeOn: true,
			wantErr:  ErrNoBackend,
		},
		{
			name:      "claude on PATH is picked when allowed",
			opts:      Options{AllowCLI: true},
			claudeOn:  true,
			wantName:  Claude,
			wantModel: DefaultModel,
		},
		{
			name:      "openrouter auto-detected from key",
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			wantName:  OpenRouter,
			wantModel: "anthropic/claude-haiku-5.5",
		},
		{
			name:      "litellm auto-detected from base url",
			env:       map[string]string{"LITELLM_BASE_URL": "http://proxy:4000"},
			wantName:  LiteLLM,
			wantModel: DefaultModel,
		},
		{
			name: "litellm wins over claude and a key",
			opts: Options{AllowCLI: true},
			env: map[string]string{
				"LITELLM_BASE_URL":   "http://proxy:4000",
				"OPENROUTER_API_KEY": "k",
			},
			claudeOn:  true,
			wantName:  LiteLLM,
			wantModel: DefaultModel,
		},
		{
			name:      "claude wins over a key when allowed",
			opts:      Options{AllowCLI: true},
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			claudeOn:  true,
			wantName:  Claude,
			wantModel: DefaultModel,
		},
		{
			name:      "openrouter is last: a key with no claude on PATH",
			opts:      Options{AllowCLI: true},
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			wantName:  OpenRouter,
			wantModel: "anthropic/claude-haiku-5.5",
		},
		{
			name:      "canonical model is translated for openrouter",
			opts:      Options{Model: "claude-sonnet-5-5"},
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			wantName:  OpenRouter,
			wantModel: "anthropic/claude-sonnet-5.5",
		},
		{
			name:      "provider id passes through on openrouter",
			opts:      Options{Model: "anthropic/claude-sonnet-5"},
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			wantName:  OpenRouter,
			wantModel: "anthropic/claude-sonnet-5",
		},
		{
			name:      "model goes to litellm as given",
			opts:      Options{Model: "claude-haiku-4-5"},
			env:       map[string]string{"LITELLM_BASE_URL": "http://proxy:4000"},
			wantName:  LiteLLM,
			wantModel: "claude-haiku-4-5",
		},
		{
			name: "forced openrouter over a litellm url",
			opts: Options{Backend: OpenRouter},
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"LITELLM_BASE_URL":   "http://proxy:4000",
			},
			wantName:  OpenRouter,
			wantModel: "anthropic/claude-haiku-5.5",
		},
		{
			name:      "forced litellm without a key still resolves",
			opts:      Options{Backend: LiteLLM},
			env:       map[string]string{"LITELLM_BASE_URL": "http://proxy:4000"},
			wantName:  LiteLLM,
			wantModel: DefaultModel,
		},
		{
			name:      "forced claude ignores AllowCLI",
			opts:      Options{Backend: Claude, Model: "claude-opus-5-5"},
			claudeOn:  true,
			wantName:  Claude,
			wantModel: "claude-opus-5-5",
		},
		{
			name:        "forced openrouter without key fails",
			opts:        Options{Backend: OpenRouter},
			wantErrText: "OPENROUTER_API_KEY",
		},
		{
			name:        "forced litellm without base url fails",
			opts:        Options{Backend: LiteLLM},
			env:         map[string]string{"LITELLM_API_KEY": "k"},
			wantErrText: "LITELLM_BASE_URL",
		},
		{
			name:        "forced claude off PATH fails",
			opts:        Options{Backend: Claude},
			wantErrText: "claude CLI",
		},
		{
			name:        "vertex not implemented",
			opts:        Options{Backend: "vertex"},
			wantErrText: "not implemented",
		},
		{
			name:        "unknown backend",
			opts:        Options{Backend: "bogus"},
			wantErrText: "unknown backend",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range selectEnv {
				t.Setenv(k, tc.env[k])
			}
			setPath(t, tc.claudeOn)
			b, err := Select(tc.opts)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Select() error = %v, want %v", err, tc.wantErr)
				}
				if b != nil {
					t.Fatalf("Select() backend = %v, want nil on error", b)
				}
				return
			}
			if tc.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("Select() error = %v, want containing %q", err, tc.wantErrText)
				}
				if b != nil {
					t.Fatalf("Select() backend = %v, want nil on error", b)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if b.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", b.Name(), tc.wantName)
			}
			if b.Model() != tc.wantModel {
				t.Errorf("Model() = %q, want %q", b.Model(), tc.wantModel)
			}
		})
	}
}

// TestNoBackendHintNamesTheCLIOnlyWhenAllowed: the hint tells a caller
// what to set, and must not send a sandboxed caller to a CLI it cannot run.
func TestNoBackendHintNamesTheCLIOnlyWhenAllowed(t *testing.T) {
	for _, k := range selectEnv {
		t.Setenv(k, "")
	}
	setPath(t, false)
	_, err := Select(Options{})
	if err == nil || strings.Contains(err.Error(), "claude") {
		t.Errorf("hint without AllowCLI = %v, want no mention of claude", err)
	}
	_, err = Select(Options{AllowCLI: true})
	if err == nil || !strings.Contains(err.Error(), "put claude on PATH") {
		t.Errorf("hint with AllowCLI = %v, want the PATH advice", err)
	}
}

func TestModelIDs(t *testing.T) {
	tests := []struct {
		canonical  string
		openRouter string
	}{
		{"claude-haiku-5-5", "anthropic/claude-haiku-5.5"},
		{"claude-sonnet-5-5", "anthropic/claude-sonnet-5.5"},
		{"claude-opus-5-5", "anthropic/claude-opus-5.5"},
		{"claude-sonnet-5", "anthropic/claude-sonnet-5"},
		{"claude-haiku-4-5", "anthropic/claude-haiku-4.5"},
	}
	for _, tc := range tests {
		if got := OpenRouterID(tc.canonical); got != tc.openRouter {
			t.Errorf("OpenRouterID(%q) = %q, want %q", tc.canonical, got, tc.openRouter)
		}
		if got := CanonicalID(tc.openRouter); got != tc.canonical {
			t.Errorf("CanonicalID(%q) = %q, want %q", tc.openRouter, got, tc.canonical)
		}
	}
	for _, other := range []string{"anthropic/claude-sonnet-5", "gpt-5", "claude-haiku-5-5-20260401"} {
		if got := OpenRouterID(other); got != other {
			t.Errorf("OpenRouterID(%q) = %q, want it unchanged", other, got)
		}
	}
	if got := CanonicalID("gpt-5"); got != "gpt-5" {
		t.Errorf("CanonicalID(gpt-5) = %q, want it unchanged", got)
	}
}
