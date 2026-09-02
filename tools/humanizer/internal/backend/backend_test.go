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

// selectEnv is every env var Select consults. Tests pin all of them so the
// host environment (which may carry a real key or proxy URL) can't leak in.
var selectEnv = []string{
	"HUMANIZER_BACKEND",
	"HUMANIZER_MODEL",
	"OPENROUTER_API_KEY",
	"LITELLM_BASE_URL",
	"LITELLM_API_KEY",
}

func TestSelect(t *testing.T) {
	cases := []struct {
		name        string
		forceName   string
		forceModel  string
		env         map[string]string
		wantName    string
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
			wantName:  "openrouter",
			wantModel: "anthropic/claude-haiku-4.5",
		},
		{
			name:      "litellm auto-detected from base url",
			env:       map[string]string{"LITELLM_BASE_URL": "http://proxy:4000"},
			wantName:  "litellm",
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name: "litellm wins auto-detection when both are configured",
			env: map[string]string{
				"LITELLM_BASE_URL":   "http://proxy:4000",
				"OPENROUTER_API_KEY": "k",
			},
			wantName:  "litellm",
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name: "HUMANIZER_MODEL overrides default",
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"HUMANIZER_MODEL":    "anthropic/claude-sonnet-5",
			},
			wantName:  "openrouter",
			wantModel: "anthropic/claude-sonnet-5",
		},
		{
			name: "HUMANIZER_MODEL overrides the litellm default",
			env: map[string]string{
				"LITELLM_BASE_URL": "http://proxy:4000",
				"HUMANIZER_MODEL":  "claude-haiku-4-5",
			},
			wantName:  "litellm",
			wantModel: "claude-haiku-4-5",
		},
		{
			name:       "explicit model wins over env",
			forceModel: "anthropic/claude-opus-5",
			env: map[string]string{
				"OPENROUTER_API_KEY": "k",
				"HUMANIZER_MODEL":    "anthropic/claude-sonnet-5",
			},
			wantName:  "openrouter",
			wantModel: "anthropic/claude-opus-5",
		},
		{
			name: "HUMANIZER_BACKEND env forces openrouter over a litellm url",
			env: map[string]string{
				"HUMANIZER_BACKEND":  "openrouter",
				"OPENROUTER_API_KEY": "k",
				"LITELLM_BASE_URL":   "http://proxy:4000",
			},
			wantName:  "openrouter",
			wantModel: "anthropic/claude-haiku-4.5",
		},
		{
			name: "HUMANIZER_BACKEND env forces litellm",
			env: map[string]string{
				"HUMANIZER_BACKEND": "litellm",
				"LITELLM_BASE_URL":  "http://proxy:4000",
			},
			wantName:  "litellm",
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name:      "forced litellm without a key still resolves",
			forceName: "litellm",
			env:       map[string]string{"LITELLM_BASE_URL": "http://proxy:4000"},
			wantName:  "litellm",
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name:        "forced openrouter without key fails",
			forceName:   "openrouter",
			env:         map[string]string{},
			wantErrText: "OPENROUTER_API_KEY",
		},
		{
			name:        "forced litellm without base url fails",
			forceName:   "litellm",
			env:         map[string]string{"LITELLM_API_KEY": "k"},
			wantErrText: "LITELLM_BASE_URL",
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
			for _, k := range selectEnv {
				t.Setenv(k, tc.env[k])
			}
			b, err := Select(tc.forceName, tc.forceModel)
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

// chatCapture records what one fake chat-completions server received.
type chatCapture struct {
	method string
	path   string
	auth   string
	hasKey bool
	body   chatRequest
}

// newChatServer serves one successful chat completion and records the
// request into the returned capture.
func newChatServer(t *testing.T) (*httptest.Server, *chatCapture) {
	t.Helper()
	got := &chatCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		_, got.hasKey = r.Header["Authorization"]
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"verdict":"likely_human"}`}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestChatClientComplete(t *testing.T) {
	cases := []struct {
		name      string
		build     func(base string) (*ChatClient, error)
		wantAuth  string
		wantKey   bool
		wantModel string
	}{
		{
			name: "openrouter sends bearer key and haiku default",
			build: func(base string) (*ChatClient, error) {
				return NewOpenRouter(ChatConfig{APIKey: "test-key", BaseURL: base})
			},
			wantAuth:  "Bearer test-key",
			wantKey:   true,
			wantModel: "anthropic/claude-haiku-4.5",
		},
		{
			name: "litellm sends bearer key and anthropic-native haiku id",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{APIKey: "proxy-key", BaseURL: base})
			},
			wantAuth:  "Bearer proxy-key",
			wantKey:   true,
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name: "litellm without a key omits the Authorization header",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{BaseURL: base})
			},
			wantKey:   false,
			wantModel: "claude-haiku-4-5-20251001",
		},
		{
			name: "litellm trims a pasted /v1 suffix off the base url",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{APIKey: "k", BaseURL: base + "/v1/"})
			},
			wantAuth:  "Bearer k",
			wantKey:   true,
			wantModel: "claude-haiku-4-5-20251001",
		},
	}
	wantMsgs := []chatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newChatServer(t)
			b, err := tc.build(srv.URL)
			if err != nil {
				t.Fatalf("build backend: %v", err)
			}
			out, err := b.Complete(context.Background(), "system prompt", "user text")
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if out != `{"verdict":"likely_human"}` {
				t.Errorf("Complete() = %q", out)
			}
			if got.method != http.MethodPost || got.path != "/v1/chat/completions" {
				t.Errorf("request = %s %s, want POST /v1/chat/completions", got.method, got.path)
			}
			if got.hasKey != tc.wantKey {
				t.Errorf("Authorization header present = %v, want %v", got.hasKey, tc.wantKey)
			}
			if got.auth != tc.wantAuth {
				t.Errorf("Authorization = %q, want %q", got.auth, tc.wantAuth)
			}
			if got.body.Model != tc.wantModel {
				t.Errorf("model = %q, want %q", got.body.Model, tc.wantModel)
			}
			if got.body.Temperature != 0 {
				t.Errorf("temperature = %v, want 0", got.body.Temperature)
			}
			msgs := got.body.Messages
			if len(msgs) != 2 || msgs[0] != wantMsgs[0] || msgs[1] != wantMsgs[1] {
				t.Errorf("messages = %+v, want %+v", msgs, wantMsgs)
			}
		})
	}
}

func TestChatClientCompleteErrors(t *testing.T) {
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
			want: "litellm HTTP 401",
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
			b, err := NewLiteLLM(ChatConfig{APIKey: "k", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewLiteLLM() error = %v", err)
			}
			_, err = b.Complete(context.Background(), "s", "u")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
