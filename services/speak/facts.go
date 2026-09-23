package speak

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// Component is the name speak registers under in the shared config and state
// directories (kit/confdir); Facts reports the same name.
const Component = "speak"

// DefaultPort is the port `speak serve` listens on when SPEAK_PORT and --port
// are both unset. The server binds to loopback only.
const DefaultPort = 7425

// DefaultTTSURL is the local mlx-audio engine both surfaces synthesize
// against when SPEAK_TTS_URL and --tts-url are unset. The engine is a
// separate process (the t-man agent speak-tts), not part of this binary.
const DefaultTTSURL = "http://127.0.0.1:8765"

// DefaultModel is the model id sent to the local engine. mlx-audio rejects a
// speech request without one (422), and the read-aloud component sends the
// same id.
const DefaultModel = "mlx-community/Kokoro-82M-bf16"

// DefaultVoice is the Kokoro voice used when a caller names none: the MCP
// tools' default and the voice serve's health probe synthesizes with. The
// read-aloud component defaults to the same id.
const DefaultVoice = "af_heart"

// ConfigFileName is speak's provider configuration inside its config
// directory (kit/confdir). ConfigEnv and ProviderEnv back --config and
// --provider. A missing file means the one implicit local provider.
const (
	ConfigFileName = "config.yaml"
	ConfigEnv      = "SPEAK_CONFIG"
	ProviderEnv    = "SPEAK_PROVIDER"
)

// DefaultProvider is the provider block speak uses when neither --provider,
// SPEAK_PROVIDER nor the config file's provider line names one: the local
// Kokoro engine, which sends no text off the machine.
const DefaultProvider = "local"

// Defaults for the remote provider types. A config block of that type (or
// named after it) inherits these unless it sets its own. API keys are only
// ever read from the environment variable a block names.
const (
	OpenRouterBaseURL = "https://openrouter.ai/api"
	OpenRouterKeyEnv  = "OPENROUTER_API_KEY"
	OpenRouterModel   = "hexgrad/kokoro-82m" // the local engine's model, hosted
	OpenAIBaseURL     = "https://api.openai.com"
	OpenAIKeyEnv      = "OPENAI_API_KEY"
	OpenAIModel       = "gpt-4o-mini-tts"
	OpenAIVoice       = "alloy"
	LiteLLMBaseURLEnv = "LITELLM_BASE_URL" // same variables humanizer reads
	LiteLLMKeyEnv     = "LITELLM_API_KEY"
	LiteLLMVoice      = "alloy"
	// The Gemini Developer API. Its key is read from GEMINI_API_KEY, then
	// GOOGLE_API_KEY, the order Google's own SDKs use.
	GeminiBaseURL        = "https://generativelanguage.googleapis.com"
	GeminiKeyEnv         = "GEMINI_API_KEY"
	GeminiFallbackKeyEnv = "GOOGLE_API_KEY"
	GeminiModel          = "gemini-3.1-flash-tts-preview"
	GeminiVoice          = "Kore"
)

// LegacyStateDir is the pre-XDG location of the playback engine's on-disk
// state: per-part WAV files under audio/ and the cross-process
// playback.lock. An install that has played audio there keeps using it,
// while every other install lands in the XDG state directory
// (~/.local/state/speak). The leading ~ is expanded at runtime, never at
// build time.
const LegacyStateDir = "~/.local/share/speak"

// LegacyStateProbe is the artifact that proves playback state lives in
// LegacyStateDir. The directory alone proves nothing: the mlx-audio engine
// this binary talks to installs its virtualenv and logs there, so on a
// machine that has the TTS engine but has never played anything, the
// directory exists while the state does not. The generated-audio cache is
// what playback itself creates.
const LegacyStateProbe = "audio"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:          Component,
		Bin:           Component,
		Purpose:       "local text-to-speech that reads text and markdown aloud on this machine's speakers",
		BaseURL:       fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath:     LegacyStateDir,
		HasDoctor:     true,
		MCPNote:       "Tools play audio in this process through the configured TTS provider (by default the local engine, t-man agent speak-tts); the speak web service is separate and not required.",
		MCPDoctorTool: "speak_doctor",
	}
}
