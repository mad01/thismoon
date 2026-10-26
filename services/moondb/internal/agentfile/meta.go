package agentfile

import (
	"encoding/json"
	"os"
	"strings"
)

// Meta is the part of a subagent's meta file (agent-<id>.meta.json) Read
// uses. The harness writes it when the agent is spawned. AgentType is the
// spawn's name for a named spawn and the agent kind for an unnamed one;
// SpawnedAgentType, when set, is the kind.
type Meta struct {
	AgentType        string `json:"agentType"`
	SpawnedAgentType string `json:"spawnedAgentType"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Model            string `json:"model"`
}

// MetaPath is the meta file beside a transcript file.
func MetaPath(transcript string) string {
	return strings.TrimSuffix(transcript, ".jsonl") + ".meta.json"
}

// ReadMeta reads a meta file; the zero Meta when it is missing or broken.
func ReadMeta(path string) Meta {
	var m Meta
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}

// Kind returns an agent's type and name from its meta file and the
// agent_type a hook payload named ("" without one). The type is the
// spawned agent type when the meta file names it. Otherwise it is the
// meta agentType or the hook's agent_type, whichever is set and is not
// the spawn's name. A named spawn of no known kind is "subagent", and an
// agent with nothing to go on is "internal": the harness ran it for
// itself. The name is the spawn's name when it says more than the type.
func Kind(m Meta, hookType string) (typ, name string) {
	name = m.Name
	switch {
	case m.SpawnedAgentType != "":
		typ = m.SpawnedAgentType
	case m.AgentType != "" && m.AgentType != name:
		typ = m.AgentType
	case hookType != "" && hookType != name:
		typ = hookType
	case name != "":
		typ = "subagent"
	default:
		typ = "internal"
	}
	if name == typ {
		name = ""
	}
	return typ, name
}
