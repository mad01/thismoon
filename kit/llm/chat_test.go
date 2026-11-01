package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatCapture records what one fake chat-completions server received.
type chatCapture struct {
	method string
	path   string
	auth   string
	hasKey bool
	body   chatRequest
	raw    map[string]json.RawMessage
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
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if err := json.Unmarshal(data, &got.body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if err := json.Unmarshal(data, &got.raw); err != nil {
			t.Errorf("decode raw request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake/model",
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"verdict":"likely_human"}`}},
			},
			"usage": map[string]any{
				"prompt_tokens": 12, "completion_tokens": 34,
				"completion_tokens_details": map[string]any{"reasoning_tokens": 5},
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
			name: "openrouter sends bearer key and the translated haiku id",
			build: func(base string) (*ChatClient, error) {
				return NewOpenRouter(ChatConfig{APIKey: "test-key", BaseURL: base})
			},
			wantAuth:  "Bearer test-key",
			wantKey:   true,
			wantModel: "anthropic/claude-haiku-5.5",
		},
		{
			name: "litellm sends bearer key and the canonical haiku id",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{APIKey: "proxy-key", BaseURL: base})
			},
			wantAuth:  "Bearer proxy-key",
			wantKey:   true,
			wantModel: DefaultModel,
		},
		{
			name: "litellm without a key omits the Authorization header",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{BaseURL: base})
			},
			wantKey:   false,
			wantModel: DefaultModel,
		},
		{
			name: "litellm trims a pasted /v1 suffix off the base url",
			build: func(base string) (*ChatClient, error) {
				return NewLiteLLM(ChatConfig{APIKey: "k", BaseURL: base + "/v1/"})
			},
			wantAuth:  "Bearer k",
			wantKey:   true,
			wantModel: DefaultModel,
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
			for _, key := range []string{"temperature", "reasoning_effort"} {
				if _, ok := got.raw[key]; ok {
					t.Errorf(
						"request carries %q by default; the Claude 5.5 generation rejects it",
						key,
					)
				}
			}
			msgs := got.body.Messages
			if len(msgs) != 2 || msgs[0] != wantMsgs[0] || msgs[1] != wantMsgs[1] {
				t.Errorf("messages = %+v, want %+v", msgs, wantMsgs)
			}
		})
	}
}

// TestChatClientOptionalFields covers the knobs an eval sets: a configured
// temperature and effort reach the wire, and the observer sees what the
// provider reported.
func TestChatClientOptionalFields(t *testing.T) {
	srv, got := newChatServer(t)
	var seen []CallInfo
	zero := 0.0
	b, err := NewOpenRouter(ChatConfig{
		APIKey:      "k",
		BaseURL:     srv.URL,
		Temperature: &zero,
		Effort:      "low",
		Observe:     func(ci CallInfo) { seen = append(seen, ci) },
	})
	if err != nil {
		t.Fatalf("NewOpenRouter() error = %v", err)
	}
	if _, err := b.Complete(context.Background(), "s", "u"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if string(got.raw["temperature"]) != "0" {
		t.Errorf("raw temperature = %s, want 0", got.raw["temperature"])
	}
	if got.body.ReasoningEffort != "low" {
		t.Errorf("reasoning_effort = %q, want low", got.body.ReasoningEffort)
	}
	if len(seen) != 1 {
		t.Fatalf("observer calls = %d, want 1", len(seen))
	}
	ci := seen[0]
	if ci.Model != "fake/model" || ci.PromptTokens != 12 || ci.CompletionTokens != 34 ||
		ci.ReasoningTokens != 5 || ci.Status != http.StatusOK {
		t.Errorf("CallInfo = %+v, want fake/model, 12/34 tokens (5 reasoning), status 200", ci)
	}
	if ci.Latency <= 0 {
		t.Errorf("latency = %v, want > 0", ci.Latency)
	}
}

func TestChatClientObservesErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"message":"temperature is deprecated"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	var seen []CallInfo
	b, err := NewLiteLLM(ChatConfig{
		BaseURL: srv.URL,
		Observe: func(ci CallInfo) { seen = append(seen, ci) },
	})
	if err != nil {
		t.Fatalf("NewLiteLLM() error = %v", err)
	}
	_, err = b.Complete(context.Background(), "s", "u")
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("Complete() error = %v, want HTTP 400", err)
	}
	if len(seen) != 1 || seen[0].Status != http.StatusBadRequest {
		t.Fatalf("observer saw %+v, want one call with status 400", seen)
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
