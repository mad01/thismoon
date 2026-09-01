package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelect(t *testing.T) {
	cases := []struct {
		name        string
		forceName   string
		forceModel  string
		env         map[string]string
		wantModel   string
		wantErr     error
		wantErrText string
	}{
		{
			name:    "nothing configured",
			env:     map[string]string{},
			wantErr: ErrNoBackend,
		},
		{
			name:      "openrouter auto-detected from key",
			env:       map[string]string{"OPENROUTER_API_KEY": "k"},
			wantModel: "anthropic/claude-haiku-4.5",
		},
		{
			name: "HUMANIZER_MODEL overrides default",
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"HUMANIZER_MODEL":    "anthropic/claude-sonnet-5",
			},
			wantModel: "anthropic/claude-sonnet-5",
		},
		{
			name:       "explicit model wins over env",
			forceModel: "anthropic/claude-opus-5",
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"HUMANIZER_MODEL":    "anthropic/claude-sonnet-5",
			},
			wantModel: "anthropic/claude-opus-5",
		},
		{
			name: "HUMANIZER_BACKEND env forces openrouter",
			env: map[string]string{
				"HUMANIZER_BACKEND":  "openrouter",
				"OPENROUTER_API_KEY": "k",
			},
			wantModel: "anthropic/claude-haiku-4.5",
		},
		{
			name:        "forced openrouter without key fails",
			forceName:   "openrouter",
			env:         map[string]string{},
			wantErrText: "OPENROUTER_API_KEY",
		},
		{
			name:        "vertex not implemented",
			forceName:   "vertex",
			env:         map[string]string{},
			wantErrText: "not implemented",
		},
		{
			name:        "anthropic not implemented",
			forceName:   "anthropic",
			env:         map[string]string{},
			wantErrText: "not implemented",
		},
		{
			name:        "unknown backend",
			forceName:   "bogus",
			env:         map[string]string{},
			wantErrText: "unknown backend",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pin every env var Select consults so the host environment
			// (which may carry a real key) can't leak into the case.
			for _, k := range []string{"HUMANIZER_BACKEND", "OPENROUTER_API_KEY", "HUMANIZER_MODEL"} {
				t.Setenv(k, tc.env[k])
			}
			b, err := Select(tc.forceName, tc.forceModel)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Select() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if tc.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("Select() error = %v, want containing %q", err, tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if b.Name() != "openrouter" {
				t.Errorf("Name() = %q, want openrouter", b.Name())
			}
			if b.Model() != tc.wantModel {
				t.Errorf("Model() = %q, want %q", b.Model(), tc.wantModel)
			}
		})
	}
}

func TestOpenRouterComplete(t *testing.T) {
	var got chatRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"verdict":"likely_human"}`}},
			},
		})
	}))
	defer srv.Close()

	b, err := NewOpenRouter(OpenRouterConfig{APIKey: "test-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewOpenRouter() error = %v", err)
	}
	out, err := b.Complete(context.Background(), "system prompt", "user text")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if out != `{"verdict":"likely_human"}` {
		t.Errorf("Complete() = %q", out)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if got.Model != "anthropic/claude-haiku-4.5" {
		t.Errorf("model = %q, want the haiku default", got.Model)
	}
	if got.Temperature != 0 {
		t.Errorf("temperature = %v, want 0", got.Temperature)
	}
	wantMsgs := []chatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user text"},
	}
	if len(got.Messages) != 2 || got.Messages[0] != wantMsgs[0] || got.Messages[1] != wantMsgs[1] {
		t.Errorf("messages = %+v, want %+v", got.Messages, wantMsgs)
	}
}

func TestOpenRouterCompleteErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "http error status surfaces body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
			},
			want: "HTTP 401",
		},
		{
			name: "error object in 200 body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
			},
			want: "model not found",
		},
		{
			name: "empty choices",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[]}`))
			},
			want: "no content",
		},
		{
			name: "redirect refused",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://127.0.0.1:1/steal", http.StatusFound)
			},
			want: "redirect refused",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			b, err := NewOpenRouter(OpenRouterConfig{APIKey: "k", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewOpenRouter() error = %v", err)
			}
			_, err = b.Complete(context.Background(), "s", "u")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
