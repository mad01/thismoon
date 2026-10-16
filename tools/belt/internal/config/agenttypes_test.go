package config

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// factoryWorkers is ledger.agent_types as the three factory worker
// definitions declare it.
const factoryWorkers = `ledger:
  agent_types:
    factory-worker-low: {model: sonnet, effort: low}
    factory-worker-medium: {model: sonnet, effort: medium}
    factory-worker-high: {model: opus, effort: high}
`

// TestLedgerAgentTypes pins how ledger.agent_types resolves. A valid entry
// tags what it declares. A malformed entry, key or value is dropped and
// named as an adjustment, and none of them fails the file: a config error
// would deny every guarded tool call.
func TestLedgerAgentTypes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]AgentTypeTags
		warn    string // "" means no adjustment at all
	}{
		{
			name:    "valid factory workers",
			content: factoryWorkers,
			want: map[string]AgentTypeTags{
				"factory-worker-low":    {Model: "sonnet", Effort: "low"},
				"factory-worker-medium": {Model: "sonnet", Effort: "medium"},
				"factory-worker-high":   {Model: "opus", Effort: "high"},
			},
		},
		{
			name: "unknown effort drops the effort tag and keeps the model",
			content: "ledger:\n  agent_types:\n" +
				"    factory-worker-high: {model: opus, effort: extreme}\n",
			want: map[string]AgentTypeTags{"factory-worker-high": {Model: "opus"}},
			warn: `ledger.agent_types.factory-worker-high.effort is "extreme", ` +
				"not one of low, medium, high, xhigh, max: effort tag dropped",
		},
		{
			name: "xhigh and max are valid efforts",
			content: "ledger:\n  agent_types:\n" +
				"    deep-reviewer: {model: opus, effort: xhigh}\n" +
				"    max-reviewer: {model: opus, effort: max}\n",
			want: map[string]AgentTypeTags{
				"deep-reviewer": {Model: "opus", Effort: "xhigh"},
				"max-reviewer":  {Model: "opus", Effort: "max"},
			},
		},
		{
			name: "empty model drops the model tag and keeps the effort",
			content: "ledger:\n  agent_types:\n" +
				"    factory-worker-low: {model: '  ', effort: low}\n",
			want: map[string]AgentTypeTags{"factory-worker-low": {Effort: "low"}},
			warn: "ledger.agent_types.factory-worker-low.model is empty: model tag dropped",
		},
		{
			name:    "non-map entry is dropped",
			content: "ledger:\n  agent_types:\n    factory-worker-high: opus\n",
			want:    map[string]AgentTypeTags{},
			warn: `ledger.agent_types.factory-worker-high is "opus", ` +
				"not a map with model and effort: no tags",
		},
		{
			name:    "non-map section is ignored",
			content: "ledger:\n  agent_types: [factory-worker-high]\n",
			want:    map[string]AgentTypeTags{},
			warn:    "ledger.agent_types is a list, not a map of agent type to model and effort",
		},
		{
			name: "non-string model is dropped",
			content: "ledger:\n  agent_types:\n" +
				"    factory-worker-high: {model: 4.5, effort: high}\n",
			want: map[string]AgentTypeTags{"factory-worker-high": {Effort: "high"}},
			warn: "ledger.agent_types.factory-worker-high.model is 4.5, not a string: " +
				"model tag dropped",
		},
		{
			name:    "a misspelt key is named and tags nothing",
			content: "ledger:\n  agent_types:\n    factory-worker-high: {modle: opus}\n",
			want:    map[string]AgentTypeTags{},
			warn: "ledger.agent_types.factory-worker-high.modle is not a known key " +
				"(model, effort): ignored",
		},
		{
			name:    "a value YAML cannot decode is a warning, not a parse error",
			content: "ledger:\n  agent_types: !!int many\n",
			want:    map[string]AgentTypeTags{},
			warn:    "ledger.agent_types could not be read",
		},
		{
			name:    "an empty key is no entries and no warning",
			content: "ledger:\n  agent_types:\n",
			want:    map[string]AgentTypeTags{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", tt.content)}
			cfg, err := LoadFrom(p)
			if err != nil {
				t.Fatalf("LoadFrom() error = %v, want none: agent_types never fails the file", err)
			}
			checkAgentTypes(t, cfg.Ledger, tt.want, tt.warn)
		})
	}
}

// checkAgentTypes compares the resolved entries and the ledger adjustments.
func checkAgentTypes(t *testing.T, l Ledger, want map[string]AgentTypeTags, warn string) {
	t.Helper()
	got := map[string]AgentTypeTags{}
	for _, name := range l.AgentTypes.Names() {
		got[name] = l.AgentTypes.For(name)
	}
	if !maps.Equal(got, want) {
		t.Errorf("entries = %v, want %v", got, want)
	}
	adjustments := strings.Join(l.Adjustments(), "\n")
	if warn == "" && adjustments != "" {
		t.Errorf("adjustments = %q, want none", adjustments)
	}
	if !strings.Contains(adjustments, warn) {
		t.Errorf("adjustments = %q, want one containing %q", adjustments, warn)
	}
}

// TestLedgerAgentTypesLegacyTOML: the legacy TOML file resolves the same
// way, a malformed value included.
func TestLedgerAgentTypesLegacyTOML(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		BeltYAML: filepath.Join(dir, "config.yaml"),
		BeltTOML: writeFile(t, dir, "config.toml", `
[ledger.agent_types.factory-worker-high]
model = "opus"
effort = "high"

[ledger.agent_types.factory-worker-low]
model = "sonnet"
effort = 3
`),
	}
	cfg, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v, want none", err)
	}
	checkAgentTypes(t, cfg.Ledger, map[string]AgentTypeTags{
		"factory-worker-high": {Model: "opus", Effort: "high"},
		"factory-worker-low":  {Model: "sonnet"},
	}, "ledger.agent_types.factory-worker-low.effort is 3, not a string: effort tag dropped")
}

// TestLedgerAgentTypesLeaveTheRestOfTheSection: a broken agent_types does
// not cost the ledger keys beside it.
func TestLedgerAgentTypesLeaveTheRestOfTheSection(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml",
		"ledger:\n  enabled: false\n  source: factory-ledger\n  agent_types: 7\n")})
	if cfg.Ledger.On() || cfg.Ledger.SourceName() != "factory-ledger" {
		t.Errorf("ledger = on %v, source %q", cfg.Ledger.On(), cfg.Ledger.SourceName())
	}
	if got := cfg.Ledger.Adjustments(); len(got) != 1 ||
		!strings.Contains(got[0], "ledger.agent_types is 7") {
		t.Errorf("adjustments = %v, want the one agent_types warning", got)
	}
}

// TestResolveAgentTypesNamesAreSorted: doctor prints the map in a stable
// order whatever order the file lists it in.
func TestResolveAgentTypesNamesAreSorted(t *testing.T) {
	a := ResolveAgentTypes(map[string]any{
		"zeta":  map[string]any{"effort": "low"},
		"alpha": map[string]any{"model": "opus"},
	})
	if got, want := a.Names(), []string{"alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}
