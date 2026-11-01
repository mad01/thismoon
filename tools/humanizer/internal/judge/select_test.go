package judge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
)

// selectEnv is every variable Select and kit/llm consult. Tests pin all of
// them so the host environment cannot leak in.
var selectEnv = []string{
	"HUMANIZER_BACKEND", "HUMANIZER_MODEL",
	"OPENROUTER_API_KEY", "LITELLM_BASE_URL", "LITELLM_API_KEY",
}

// TestSelect covers the humanizer-side policy: the HUMANIZER_* overrides,
// and the claude CLI staying out of the auto order even when it is on PATH.
func TestSelect(t *testing.T) {
	cases := []struct {
		name      string
		force     string
		model     string
		env       map[string]string
		wantName  string
		wantModel string
		wantErr   error
	}{
		{
			name: "HUMANIZER_BACKEND forces openrouter over a litellm url",
			env: map[string]string{
				"HUMANIZER_BACKEND":  "openrouter",
				"OPENROUTER_API_KEY": "k",
				"LITELLM_BASE_URL":   "http://proxy:4000",
			},
			wantName:  llm.OpenRouter,
			wantModel: "anthropic/claude-haiku-5.5",
		},
		{
			name: "HUMANIZER_MODEL overrides the litellm default",
			env: map[string]string{
				"LITELLM_BASE_URL": "http://proxy:4000",
				"HUMANIZER_MODEL":  "claude-haiku-4-5",
			},
			wantName:  llm.LiteLLM,
			wantModel: "claude-haiku-4-5",
		},
		{
			name:  "explicit model wins over HUMANIZER_MODEL",
			model: "anthropic/claude-opus-5",
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"HUMANIZER_MODEL":    "anthropic/claude-sonnet-5",
			},
			wantName:  llm.OpenRouter,
			wantModel: "anthropic/claude-opus-5",
		},
		{
			name:    "claude on PATH is never auto-picked",
			env:     map[string]string{},
			wantErr: llm.ErrNoBackend,
		},
		{
			name:      "claude can be forced",
			force:     "claude",
			env:       map[string]string{},
			wantName:  llm.Claude,
			wantModel: llm.DefaultModel,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range selectEnv {
				t.Setenv(k, tc.env[k])
			}
			dir := t.TempDir()
			bin := filepath.Join(dir, "claude")
			if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			b, err := Select(tc.force, tc.model)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Select() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if b.Name() != tc.wantName || b.Model() != tc.wantModel {
				t.Errorf(
					"Select() = %s %s, want %s %s",
					b.Name(),
					b.Model(),
					tc.wantName,
					tc.wantModel,
				)
			}
		})
	}
}
