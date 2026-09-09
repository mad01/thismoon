package semantic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedEmbed records one /api/embed request body for assertions.
type capturedEmbed struct {
	Model     string         `json:"model"`
	Input     []string       `json:"input"`
	KeepAlive string         `json:"keep_alive"`
	Options   map[string]any `json:"options"`
}

// fakeOllama runs an httptest server that answers /api/embed with one
// dim-length vector per input and /api/tags with the given model names.
func fakeOllama(t *testing.T, dim int, tagModels []string, got *[]capturedEmbed) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/embed", func(w http.ResponseWriter, r *http.Request) {
		var req capturedEmbed
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		*got = append(*got, req)
		vecs := make([][]float32, len(req.Input))
		for i := range vecs {
			vecs[i] = make([]float32, dim)
			vecs[i][0] = 1
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vecs})
	})
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		models := make([]map[string]string, len(tagModels))
		for i, m := range tagModels {
			models[i] = map[string]string{"name": m}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestOllamaEmbedderDefaults(t *testing.T) {
	emb := NewOllamaEmbedder("", "", 0)
	if emb.Endpoint != defaultOllamaEndpoint || emb.Model != defaultOllamaModel ||
		emb.Dim() != defaultOllamaDim {
		t.Fatalf("defaults = %q %q %d, want %q %q %d",
			emb.Endpoint, emb.Model, emb.Dim(),
			defaultOllamaEndpoint, defaultOllamaModel, defaultOllamaDim)
	}
}

func TestOllamaEmbedSendsKeepAliveAndNumCtx(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, nil, &got)
	emb := NewOllamaEmbedder(srv.URL, "test-model", 4)

	vecs, err := emb.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vecs))
	}
	if len(got) != 1 {
		t.Fatalf("got %d requests, want 1", len(got))
	}
	req := got[0]
	if req.Model != "test-model" {
		t.Errorf("model = %q, want test-model", req.Model)
	}
	if req.KeepAlive != ollamaKeepAlive {
		t.Errorf("keep_alive = %q, want %q", req.KeepAlive, ollamaKeepAlive)
	}
	if ctxVal, ok := req.Options["num_ctx"].(float64); !ok || int(ctxVal) != ollamaNumCtx {
		t.Errorf("options.num_ctx = %v, want %d", req.Options["num_ctx"], ollamaNumCtx)
	}
	if batchVal, ok := req.Options["num_batch"].(float64); !ok || int(batchVal) != ollamaNumCtx {
		t.Errorf("options.num_batch = %v, want %d", req.Options["num_batch"], ollamaNumCtx)
	}
}

func TestOllamaEmbedQueryPrefixesInstructionForQwen3(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, nil, &got)
	emb := NewOllamaEmbedder(srv.URL, "qwen3-embedding:0.6b", 4)

	if _, err := emb.EmbedQuery(context.Background(), "find retries"); err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(got) != 1 || len(got[0].Input) != 1 {
		t.Fatalf("unexpected requests: %+v", got)
	}
	in := got[0].Input[0]
	if !strings.HasPrefix(in, "Instruct: ") || !strings.HasSuffix(in, "Query: find retries") {
		t.Errorf("query input missing instruction wrapping: %q", in)
	}
}

func TestOllamaEmbedQueryBareForNonInstructionModels(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, nil, &got)
	emb := NewOllamaEmbedder(srv.URL, "unclemusclez/jina-embeddings-v2-base-code:f16", 4)

	if _, err := emb.EmbedQuery(context.Background(), "find retries"); err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if in := got[0].Input[0]; in != "find retries" {
		t.Errorf("query input altered for non-instruction model: %q", in)
	}
}

func TestOllamaEmbedDocumentsAreBare(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, nil, &got)
	emb := NewOllamaEmbedder(srv.URL, "test-model", 4)

	if _, err := emb.Embed(context.Background(), []string{"func main() {}"}); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if in := got[0].Input[0]; in != "func main() {}" {
		t.Errorf("document input altered: %q", in)
	}
}

func TestOllamaUnloadSendsZeroKeepAlive(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, nil, &got)
	emb := NewOllamaEmbedder(srv.URL, "test-model", 4)

	if err := emb.Unload(context.Background()); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	if len(got) != 1 || got[0].KeepAlive != "0" {
		t.Fatalf("unload keep_alive = %+v, want \"0\"", got)
	}
}

func TestOllamaCheckModel(t *testing.T) {
	var got []capturedEmbed
	srv := fakeOllama(t, 4, []string{"other:latest", "test-model:latest"}, &got)

	if err := NewOllamaEmbedder(srv.URL, "test-model", 4).CheckModel(context.Background()); err != nil {
		t.Errorf("CheckModel(pulled model) = %v, want nil", err)
	}
	err := NewOllamaEmbedder(srv.URL, "missing-model", 4).CheckModel(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ollama pull missing-model") {
		t.Errorf("CheckModel(missing model) = %v, want pull hint", err)
	}
}
