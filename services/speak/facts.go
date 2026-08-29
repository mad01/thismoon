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

// DefaultStateDir is where the playback engine keeps its on-disk state:
// per-sentence WAV files under audio/ and the cross-process playback.lock.
// It is the pre-XDG location: an install that already has this directory
// keeps using it, while a fresh one lands in the XDG state directory
// (~/.local/state/speak). The leading ~ is expanded at runtime, never at
// build time.
const DefaultStateDir = "~/.local/share/speak"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:          Component,
		Bin:           Component,
		Purpose:       "local text-to-speech that reads text and markdown aloud on this machine's speakers",
		BaseURL:       fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath:     DefaultStateDir,
		HasDoctor:     true,
		MCPNote:       "Tools play audio in this process via the local TTS engine (t-man agent speak-tts); the speak web service is separate and not required.",
		MCPDoctorTool: "speak_doctor",
	}
}
