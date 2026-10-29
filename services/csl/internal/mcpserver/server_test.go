package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/csl"
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

// TestAlwaysLoadMeta pins which tools ask Claude Code to skip tool-search
// deferral: the four nearly every session calls carry
// _meta["anthropic/alwaysLoad"] = true in tools/list, and every other tool
// leaves the key out so it stays deferred.
func TestAlwaysLoadMeta(t *testing.T) {
	want := []string{"csl_read", "csl_repo_info", "csl_repo_lookup", "csl_search"}
	var got []string
	for _, tool := range mcptest.ListTools(t, New("test", Options{SemanticEnabled: true})) {
		v, ok := tool.Meta["anthropic/alwaysLoad"]
		if !ok {
			continue
		}
		if v != true {
			t.Errorf("%s: _meta[anthropic/alwaysLoad] = %v, want true", tool.Name, v)
		}
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("tools with _meta[anthropic/alwaysLoad] = %v, want %v", got, want)
	}
}

// TestInstructionsNameEveryWriteTool keeps the parallel-calls note honest. The
// instructions say every tool but the ones they name is read-only, so the
// named tools must be exactly those registered without ReadOnlyHint, and the
// note must not push the block past the three-line cap (ADR-0009).
func TestInstructionsNameEveryWriteTool(t *testing.T) {
	instructions := agentdoc.Instructions(csl.Facts())
	if n := len(strings.Split(instructions, "\n")); n > 3 {
		t.Errorf("instructions have %d lines, want at most 3:\n%s", n, instructions)
	}
	_, rest, ok := strings.Cut(instructions, "Every tool but ")
	exceptions, _, found := strings.Cut(rest, " is read-only")
	if !ok || !found {
		t.Fatalf("instructions lack the read-only exception list:\n%s", instructions)
	}
	var named []string
	for _, word := range strings.Fields(exceptions) {
		if name := strings.TrimSuffix(word, ","); strings.HasPrefix(name, "csl_") {
			named = append(named, name)
		}
	}
	var writers []string
	for _, tool := range mcptest.ListTools(t, New("test", Options{SemanticEnabled: true})) {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			writers = append(writers, tool.Name)
		}
	}
	slices.Sort(named)
	slices.Sort(writers)
	if !slices.Equal(named, writers) {
		t.Errorf(
			"instructions name %v as not read-only, want the tools without ReadOnlyHint %v",
			named,
			writers,
		)
	}
}

// TestParallelCallsNote pins which tools carry the call-site nudge to batch
// independent calls, and that each of them really is read-only, since the
// nudge tells the model so.
func TestParallelCallsNote(t *testing.T) {
	want := []string{"csl_hybrid_search", "csl_outline", "csl_read", "csl_search"}
	var got []string
	for _, tool := range mcptest.ListTools(t, New("test", Options{SemanticEnabled: true})) {
		if !strings.Contains(tool.Description, "this tool is read-only and runs concurrently") {
			continue
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: description claims read-only but ReadOnlyHint is false", tool.Name)
		}
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("tools with the parallel-calls note = %v, want %v", got, want)
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
