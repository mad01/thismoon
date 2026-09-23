package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// load writes content as the config file (none when content is "") and
// loads it with env as the environment.
func load(t *testing.T, content string, opts Options, env map[string]string) (Config, error) {
	t.Helper()
	opts.Path = filepath.Join(t.TempDir(), "config.yaml")
	if content != "" {
		if err := os.WriteFile(opts.Path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	opts.Getenv = func(k string) string { return env[k] }
	return Load(opts)
}

func TestNoFileIsTheImplicitLocalProvider(t *testing.T) {
	cfg, err := load(t, "", Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.ActiveProvider()
	if cfg.Found || cfg.Active != "local" || p.Type != TypeLocal || p.Remote || p.Problem != "" {
		t.Errorf("config = %+v, active %+v; want the implicit ready local provider", cfg, p)
	}
	if p.BaseURL != "http://127.0.0.1:8765" || p.Model != "mlx-community/Kokoro-82M-bf16" ||
		p.Voice != "af_heart" || p.Format != "wav" {
		t.Errorf("local defaults = %+v", p)
	}
}

// TestEveryBlockConfiguredOneSelected is the file shape the design rests on:
// several providers configured side by side, the provider line picking one,
// and --provider overriding it for a run.
func TestEveryBlockConfiguredOneSelected(t *testing.T) {
	file := `
provider: kokoro-remote
providers:
  local: {}
  kokoro-remote:
    type: openrouter
    voice: am_adam
    voices: [af_heart, am_adam]
  proxy:
    type: litellm
    base_url: http://127.0.0.1:4000/v1/
    model: tts-1
`
	env := map[string]string{"OPENROUTER_API_KEY": "sk-or"}
	cfg, err := load(t, file, Options{}, env)
	if err != nil {
		t.Fatal(err)
	}
	if names := providerNames(cfg); !slices.Equal(
		names,
		[]string{"kokoro-remote", "local", "proxy"},
	) {
		t.Errorf("providers = %v", names)
	}
	p := cfg.ActiveProvider()
	if p.Name != "kokoro-remote" || p.Type != TypeOpenRouter || p.APIKey != "sk-or" ||
		p.BaseURL != "https://openrouter.ai/api" || p.Model != "hexgrad/kokoro-82m" ||
		p.Voice != "am_adam" || p.Format != "pcm" || !p.Remote || p.Problem != "" {
		t.Errorf("active = %+v", p)
	}

	cfg, err = load(t, file, Options{Provider: "proxy"}, env)
	if err != nil {
		t.Fatal(err)
	}
	proxy := cfg.ActiveProvider()
	if proxy.Name != "proxy" || proxy.BaseURL != "http://127.0.0.1:4000" || proxy.Remote ||
		proxy.Voice != "alloy" || proxy.Problem != "" {
		t.Errorf("--provider proxy = %+v; want the LiteLLM block, /v1 trimmed, on loopback", proxy)
	}
}

// TestProblemsStayOnTheirBlock pins that an unusable block is reported, not
// fatal: the load succeeds and each block says what it lacks.
func TestProblemsStayOnTheirBlock(t *testing.T) {
	file := `
providers:
  openrouter: {}
  openai: {}
  litellm: {}
  proxy2:
    type: litellm
    base_url_env: PROXY2_URL
    model: tts-1
  typo:
    type: openrouterr
  bad-url:
    type: openai
    base_url: ftp://example.com
`
	cfg, err := load(t, file, Options{}, map[string]string{"OPENAI_API_KEY": "sk"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"local":      "",
		"openrouter": "OPENROUTER_API_KEY is not set",
		"openai":     "",
		"litellm":    "LITELLM_BASE_URL is not set",
		"proxy2":     "PROXY2_URL is not set",
		"typo":       `unknown type "openrouterr"`,
		"bad-url":    "is not an http(s) URL",
	}
	for _, p := range cfg.Providers {
		if w := want[p.Name]; (w == "") != (p.Problem == "") || !strings.Contains(p.Problem, w) {
			t.Errorf("%s problem = %q, want %q", p.Name, p.Problem, w)
		}
	}
}

func TestLiteLLMReadsHumanizersVariables(t *testing.T) {
	cfg, err := load(t, "", Options{Provider: "litellm"}, map[string]string{
		"LITELLM_BASE_URL": "https://litellm.example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.ActiveProvider()
	if p.BaseURL != "https://litellm.example.com" || !p.Remote ||
		p.Problem != "model is required: name the model your LiteLLM proxy routes speech to" {
		t.Errorf("litellm = %+v", p)
	}
}

// TestTTSURLOverridesLocalBlocks keeps --tts-url working as before config
// existed.
func TestTTSURLOverridesLocalBlocks(t *testing.T) {
	cfg, err := load(t, "providers:\n  local:\n    base_url: http://127.0.0.1:1111\n",
		Options{TTSURL: "http://127.0.0.1:2222"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ActiveProvider().BaseURL; got != "http://127.0.0.1:2222" {
		t.Errorf("local base_url = %q, want the --tts-url override", got)
	}
}

// TestCuratedVoicesAlwaysHoldTheDefault: a voice: outside the voices: list
// joins it, and a list without voice: makes its first entry the default.
func TestCuratedVoicesAlwaysHoldTheDefault(t *testing.T) {
	cfg, err := load(t, `
providers:
  local:
    voice: bf_emma
    voices: [af_heart, am_adam]
  other:
    type: local
    voices: [am_adam, af_heart]
`, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cfg.Providers {
		switch p.Name {
		case "local":
			if p.Voice != "bf_emma" ||
				!slices.Equal(p.Voices, []string{"bf_emma", "af_heart", "am_adam"}) {
				t.Errorf("local voices = %s %v", p.Voice, p.Voices)
			}
		case "other":
			if p.Voice != "am_adam" {
				t.Errorf("other default = %s, want the list's first voice", p.Voice)
			}
		}
	}
}

func TestLoadFailsLoudly(t *testing.T) {
	cases := map[string]struct {
		file string
		opts Options
		want string
	}{
		"unparseable": {"providers: [", Options{}, "parse"},
		"key pasted in file": {
			"providers:\n  openai:\n    api_key: sk-leak\n",
			Options{},
			"field api_key not found",
		},
		"unknown active": {
			"provider: nope\nproviders:\n  local: {}\n",
			Options{},
			`provider "nope" has no block`,
		},
		"unknown override": {"", Options{Provider: "nope"}, `provider "nope" has no block`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := load(t, tc.file, tc.opts, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestKeysNeverSerialize guards `speak config -o json`, which prints the
// whole resolved config.
func TestKeysNeverSerialize(t *testing.T) {
	cfg, err := load(
		t,
		"",
		Options{Provider: "openai"},
		map[string]string{"OPENAI_API_KEY": "sk-secret"},
	)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "sk-secret") {
		t.Errorf("config JSON leaks the key: %s", out)
	}
}

// TestEnvNamesWhatTheBlockReads pins the list the spawn wrapper extracts
// from the secrets file: exactly the variables the active block reads.
func TestEnvNamesWhatTheBlockReads(t *testing.T) {
	cfg, err := load(t, `
providers:
  local: {}
  openrouter: {}
  litellm:
    model: tts-1
  proxy:
    type: litellm
    base_url_env: PROXY_URL
    api_key_env: PROXY_KEY
    model: tts-1
`, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"local":      nil,
		"openrouter": {"OPENROUTER_API_KEY"},
		"litellm":    {"LITELLM_BASE_URL", "LITELLM_API_KEY"},
		"proxy":      {"PROXY_URL", "PROXY_KEY"},
	}
	for _, p := range cfg.Providers {
		if !slices.Equal(p.Env, want[p.Name]) {
			t.Errorf("%s env = %v, want %v", p.Name, p.Env, want[p.Name])
		}
	}
}

func providerNames(cfg Config) []string {
	var names []string
	for _, p := range cfg.Providers {
		names = append(names, p.Name)
	}
	return names
}
