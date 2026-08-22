package humanizer

import "github.com/mad01/thismoon/kit/agentdoc"

// DefaultCacheDir is the style-pack extraction target when neither
// HUMANIZER_CACHE_DIR nor XDG_CACHE_HOME is set. The leading ~ is expanded at
// runtime; internal/rules computes the same path from the environment.
const DefaultCacheDir = "~/.cache/humanizer/vale"

// Facts returns the mechanical facts rendered into OperatingDoc, the MCP
// instructions block, and error hints. BaseURL stays empty: humanizer has no
// serve process, every run is a short-lived CLI or stdio MCP process.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      "humanizer",
		Bin:       "humanizer",
		Purpose:   "deterministic AI-writing detection, voice profiling, and watermark scrubbing for prose",
		StorePath: DefaultCacheDir,
		MCPNote:   "Tools run inside this stdio process (no backing service); detection shells out to the vale binary on PATH, so on a detect error call humanizer_status first.",
	}
}
