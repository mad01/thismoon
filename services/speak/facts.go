package speak

import (
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// DefaultPort is the port `speak serve` listens on when SPEAK_PORT and --port
// are both unset. The server binds to loopback only.
const DefaultPort = 7425

// DefaultStateDir is where the playback engine keeps its on-disk state:
// per-sentence WAV files under audio/ and the cross-process playback.lock.
// The leading ~ is expanded at runtime, never at build time.
const DefaultStateDir = "~/.local/share/speak"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "speak",
		Bin:       "speak",
		Purpose:   "local text-to-speech that reads text and markdown aloud on this machine's speakers",
		BaseURL:   fmt.Sprintf("http://localhost:%d", DefaultPort),
		StorePath: DefaultStateDir,
		MCPNote:   "Tools play audio in this process via the local TTS engine (t-man agent speak-tts); the speak web service is separate and not required.",
	}
}
