package mcpserver

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/provider"
)

// newTestServer builds the real MCP server over a provider with a curated
// voice list, so no test reads the Hugging Face cache or the network.
// StateDir is a temp directory: New builds a playback engine, which reaps
// stale audio under it on construction.
func newTestServer(t *testing.T, p config.Provider) *mcp.Server {
	t.Helper()
	s, err := New("test", Config{
		Provider: provider.New(context.Background(), p),
		StateDir: t.TempDir(),
		Checks:   func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s
}

var localProvider = config.Provider{
	Name:    "local",
	Type:    config.TypeLocal,
	BaseURL: "http://127.0.0.1:0",
	Model:   "mlx-community/Kokoro-82M-bf16",
	Voice:   "af_heart",
	Voices:  []string{"af_heart", "am_adam"},
	Format:  "wav",
}

// TestToolAnnotationsFollowTheContract checks the tool list against the
// shared annotation gate, so a tool added without its read-only/
// destructive/open-world hints fails here rather than shipping with the SDK
// defaults.
func TestToolAnnotationsFollowTheContract(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, newTestServer(t, localProvider))
}

// TestVoiceParameterListsTheProvidersVoices pins what the MCP tools show an
// agent: the voice parameter of speak_text and speak_file is exactly the
// provider's voice list, with the default named.
func TestVoiceParameterListsTheProvidersVoices(t *testing.T) {
	for _, tool := range mcptest.ListTools(t, newTestServer(t, localProvider)) {
		if tool.Name != "speak_text" && tool.Name != "speak_file" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]struct {
				Enum        []string `json:"enum"`
				Description string   `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		voice := schema.Properties["voice"]
		if !slices.Equal(voice.Enum, []string{"af_heart", "am_adam"}) ||
			!strings.Contains(voice.Description, "default af_heart") {
			t.Errorf(
				"%s voice = %+v, want the provider's two voices with af_heart as default",
				tool.Name,
				voice,
			)
		}
	}
}

// TestRemoteProviderIsOpenWorld pins the hint a client uses to warn that
// text leaves the machine.
func TestRemoteProviderIsOpenWorld(t *testing.T) {
	remote := localProvider
	remote.Name, remote.Type, remote.Remote = "openrouter", config.TypeOpenRouter, true
	for _, tool := range mcptest.ListTools(t, newTestServer(t, remote)) {
		if tool.Name != "speak_text" {
			continue
		}
		if hint := tool.Annotations.OpenWorldHint; hint == nil || !*hint {
			t.Errorf("speak_text OpenWorldHint = %v, want true for a remote provider", hint)
		}
	}
}

func TestDescribeVoicesMarksTheDefault(t *testing.T) {
	got := describeVoices(provider.New(context.Background(), localProvider))
	for _, want := range []string{"provider: local", "source: config", "af_heart (default)", "am_adam"} {
		if !strings.Contains(got, want) {
			t.Errorf("speak_voices reply missing %q:\n%s", want, got)
		}
	}
}
