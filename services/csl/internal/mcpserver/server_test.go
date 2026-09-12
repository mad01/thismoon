package mcpserver

import (
	"slices"
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
)

// semanticTools are the two tools gated on semantic.enabled in config.yaml.
var semanticTools = []string{"csl_semantic_search", "csl_hybrid_search"}

// TestToolAnnotations is the annotation gate: every registered tool must state
// whether it writes and whether it reaches outside this machine, so a client
// deciding what to auto-approve reads a hint rather than a tool name.
func TestToolAnnotations(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test", Options{SemanticEnabled: true}))
}

// TestNew_SemanticEnabledRegistersSemanticTools pins the enabled half of the
// gate: with semantic search on, both semantic tools are advertised.
func TestNew_SemanticEnabledRegistersSemanticTools(t *testing.T) {
	names := toolNames(t, Options{SemanticEnabled: true})
	for _, want := range semanticTools {
		if !slices.Contains(names, want) {
			t.Errorf("%s is not registered with semantic enabled; tools: %v", want, names)
		}
	}
}

// TestNew_SemanticDisabledHidesOnlySemanticTools pins the disabled half: the
// two semantic tools disappear and nothing else does, so a machine without an
// embedding backend loses the tools that would answer available=false and
// keeps every other one.
func TestNew_SemanticDisabledHidesOnlySemanticTools(t *testing.T) {
	enabled := toolNames(t, Options{SemanticEnabled: true})
	disabled := toolNames(t, Options{SemanticEnabled: false})

	for _, gated := range semanticTools {
		if slices.Contains(disabled, gated) {
			t.Errorf("%s is registered with semantic disabled", gated)
		}
	}

	want := make([]string, 0, len(enabled))
	for _, name := range enabled {
		if !slices.Contains(semanticTools, name) {
			want = append(want, name)
		}
	}
	if !slices.Equal(disabled, want) {
		t.Errorf("tools with semantic disabled = %v, want %v", disabled, want)
	}
}

// toolNames lists the tools a server built with opts advertises.
func toolNames(t *testing.T, opts Options) []string {
	t.Helper()
	tools := mcptest.ListTools(t, New("test", opts))
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
