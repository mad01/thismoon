package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

const kokoro = "mlx-community/Kokoro-82M-bf16"

// fakeHFCache lays out a Hugging Face cache holding the named voice packs of
// the Kokoro model, and points HF_HUB_CACHE at it. A name ending in "!" is
// a dangling link: the symlink an interrupted download leaves.
func fakeHFCache(t *testing.T, packs ...string) {
	t.Helper()
	hub := t.TempDir()
	t.Setenv("HF_HUB_CACHE", hub)
	t.Setenv("HF_HOME", "")
	dir := filepath.Join(
		hub,
		"models--mlx-community--Kokoro-82M-bf16",
		"snapshots",
		"abc",
		"voices",
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		name, dangling := strings.CutSuffix(pack, "!")
		path := filepath.Join(dir, name+".safetensors")
		var err error
		if dangling {
			err = os.Symlink(filepath.Join(hub, "blobs", "missing"), path)
		} else {
			err = os.WriteFile(path, []byte("x"), 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func local(voice string) config.Provider {
	return config.Provider{
		Name:    "local",
		Type:    config.TypeLocal,
		BaseURL: "http://127.0.0.1:1",
		Model:   kokoro,
		Voice:   voice,
	}
}

// TestLocalVoicesComeFromTheCache pins local discovery: the English packs
// whose blobs are present, sorted; other languages and dangling links are
// left out because the engine cannot speak them.
func TestLocalVoicesComeFromTheCache(t *testing.T) {
	fakeHFCache(t, "am_adam", "af_heart", "jf_alpha", "bf_emma!")
	v := New(context.Background(), local("af_heart")).Voices()
	if v.Source != SourceDiscovered || !slices.Equal(v.List, []string{"af_heart", "am_adam"}) ||
		v.Note != "" {
		t.Errorf("voices = %+v, want af_heart and am_adam discovered", v)
	}
}

func TestLocalFallsBackToTheCatalog(t *testing.T) {
	fakeHFCache(t)
	v := New(context.Background(), local("af_heart")).Voices()
	if v.Source != SourceCatalog || !slices.Equal(v.List, kokoroEnglish) ||
		!strings.Contains(v.Note, "discovery failed") {
		t.Errorf("voices = %+v, want the Kokoro catalog with a note", v)
	}
}

func TestCuratedVoicesWin(t *testing.T) {
	p := local("af_heart")
	p.Voices = []string{"af_heart", "bm_lewis"}
	v := New(context.Background(), p).Voices()
	if v.Source != SourceConfig || !slices.Equal(v.List, p.Voices) {
		t.Errorf("voices = %+v, want the curated list", v)
	}
}

// TestOpenRouterVoicesAreDiscovered pins discovery against OpenRouter's
// models list, and the fallbacks when the model is missing from it.
func TestOpenRouterVoicesAreDiscovered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.URL.Query().Get("output_modalities") != "speech" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"google/gemini-3.1-flash-tts-preview","supported_voices":["Kore","Puck"]},
			{"id":"sesame/csm-1b","supported_voices":[]}]}`))
	}))
	defer srv.Close()

	cases := []struct {
		model, voice string
		wantSource   string
		wantList     []string
		wantNote     string
	}{
		{
			"google/gemini-3.1-flash-tts-preview",
			"Kore",
			SourceDiscovered,
			[]string{"Kore", "Puck"},
			"",
		},
		{
			"google/gemini-3.1-flash-tts-preview",
			"af_heart",
			SourceDiscovered,
			[]string{"Kore", "Puck"},
			`default voice "af_heart" is not one`,
		},
		{"hexgrad/kokoro-82m", "af_heart", SourceCatalog, kokoroEnglish, "has no speech model"},
		{"sesame/csm-1b", "conversational_a", SourceDefault, nil, "lists no voices"},
	}
	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.voice, func(t *testing.T) {
			v := New(context.Background(), config.Provider{
				Name: "openrouter", Type: config.TypeOpenRouter, BaseURL: srv.URL,
				Model: tc.model, Voice: tc.voice,
			}).Voices()
			if v.Source != tc.wantSource || !slices.Equal(v.List, tc.wantList) ||
				!strings.Contains(v.Note, tc.wantNote) {
				t.Errorf(
					"voices = %+v, want %s %v noting %q",
					v,
					tc.wantSource,
					tc.wantList,
					tc.wantNote,
				)
			}
		})
	}
}

func TestCatalogAndUnlistedProviders(t *testing.T) {
	openai := New(
		context.Background(),
		config.Provider{Name: "openai", Type: config.TypeOpenAI, Voice: "alloy"},
	).Voices()
	if openai.Source != SourceCatalog || !slices.Contains(openai.List, "alloy") {
		t.Errorf("openai voices = %+v", openai)
	}
	litellm := New(
		context.Background(),
		config.Provider{Name: "proxy", Type: config.TypeLiteLLM, Voice: "alloy"},
	).Voices()
	if litellm.Source != SourceDefault || len(litellm.List) != 0 ||
		!strings.Contains(litellm.Note, "set voices:") {
		t.Errorf("litellm voices = %+v", litellm)
	}
}

func TestResolve(t *testing.T) {
	known := Voices{Default: "Kore", List: []string{"Kore", "Puck"}}
	unknown := Voices{Default: "alloy"}
	cases := []struct {
		v    Voices
		in   string
		want string
	}{
		{known, "", "Kore"},
		{known, "Puck", "Puck"},
		{known, "af_heart", "Kore"}, // the read-aloud component's Kokoro default
		{unknown, "", "alloy"},
		{unknown, "anything", "anything"}, // no list: the provider judges
	}
	for _, tc := range cases {
		if got := tc.v.Resolve(tc.in); got != tc.want {
			t.Errorf("Resolve(%q) with %v = %q, want %q", tc.in, tc.v.List, got, tc.want)
		}
	}
}

// TestProblemsBecomeConfigFailures pins the no-quiet-fallback rule: a block
// that cannot be used fails every synthesis with its reason, as a config
// error.
func TestProblemsBecomeConfigFailures(t *testing.T) {
	for _, p := range []*Provider{
		New(context.Background(), config.Provider{Name: "openrouter", Problem: "OPENROUTER_API_KEY is not set"}),
		Broken("config", errors.New("config: parse config.yaml: bad indent")),
	} {
		_, err := p.Synthesize(context.Background(), tts.Request{Text: "hi"})
		te, ok := errors.AsType[*tts.Error](err)
		if !ok || te.Kind != tts.KindConfig || p.Err() == nil {
			t.Errorf("%s: err = %v, want a config *tts.Error", p.Name(), err)
		}
	}
}
