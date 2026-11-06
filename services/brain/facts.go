// Package brain holds the facts the brain binary and its docs share: the
// user-facing defaults, the port brain serve listens on, and where config
// and state live.
package brain

import "github.com/mad01/thismoon/kit/agentdoc"

// Component is brain's name in its config and state directories.
const Component = "brain"

// DefaultPort is the loopback port `brain serve` listens on when neither
// --port nor PortEnv names one, and the port `brain doctor` probes.
const DefaultPort = 7427

// PortEnv names the environment variable that moves brain serve's port.
// --port wins over it, and it wins over DefaultPort.
const PortEnv = "BRAIN_PORT"

// LayoutDirName is the directory inside the state directory where brain
// serve caches graph layouts, one file per layout key. It is the one place
// serve writes.
const LayoutDirName = "layout"

// ServeSandboxLocal is the machine-local srt layer of brain serve's
// sandbox: a vault outside the shipped read paths is added there.
const ServeSandboxLocal = "~/.config/srt/brain-serve.local.json"

// ConfigEnv names the environment variable that relocates the config file.
// --config wins over it, and it wins over the default under the config
// directory.
const ConfigEnv = "BRAIN_CONFIG"

// ConfigFile is the config file's name inside brain's config directory.
const ConfigFile = "config.yaml"

// LegacyVaultName names the one vault a config with the single-path
// `vault:` key describes.
const LegacyVaultName = "default"

// Semantic-layer defaults, applied when the config leaves a key empty.
const (
	DefaultOllamaURL  = "http://localhost:11434"
	DefaultEmbedModel = "nomic-embed-text"
	// DefaultChatModel is the Ollama model the optional reranker scores with.
	DefaultChatModel = "llama3.2:3b"
	// DefaultChunkSize is the target token count of one embedded chunk.
	DefaultChunkSize = 800
	// DefaultChunkOverlap is the token count adjacent chunks share.
	DefaultChunkOverlap = 100
)

// Facts returns the mechanical facts rendered into the MCP instructions and
// the `brain mcp` help.
func Facts() agentdoc.Facts {
	return agentdoc.Facts{
		Name:      Component,
		Bin:       Component,
		Purpose:   "local markdown knowledge base: search, read and edit an Obsidian-style vault",
		StorePath: DefaultStateDir,
		HasDoctor: true,
		MCPNote: "Tools read and write the vault the config names inside this stdio process; " +
			"no service must be running.",
		MCPDoctorTool: "brain_doctor",
	}
}
