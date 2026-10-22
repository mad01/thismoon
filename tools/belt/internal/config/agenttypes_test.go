package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// reviewerTiers is ledger.agent_types as three reviewer definitions
// declare it.
const reviewerTiers = `ledger:
  agent_types:
    reviewer-quick: {model: sonnet, effort: low}
    reviewer-standard: {model: sonnet, effort: medium}
    reviewer-deep: {model: opus, effort: high}
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
			name:    "valid reviewer tiers",
			content: reviewerTiers,
			want: map[string]AgentTypeTags{
				"reviewer-quick":    {Model: "sonnet", Effort: "low"},
				"reviewer-standard": {Model: "sonnet", Effort: "medium"},
				"reviewer-deep":     {Model: "opus", Effort: "high"},
			},
		},
		{
			name: "unknown effort drops the effort tag and keeps the model",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-deep: {model: opus, effort: extreme}\n",
			want: map[string]AgentTypeTags{"reviewer-deep": {Model: "opus"}},
			warn: `ledger.agent_types.reviewer-deep.effort is "extreme", ` +
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
				"    reviewer-quick: {model: '  ', effort: low}\n",
			want: map[string]AgentTypeTags{"reviewer-quick": {Effort: "low"}},
			warn: "ledger.agent_types.reviewer-quick.model is empty: model tag dropped",
		},
		{
			name:    "non-map entry is dropped",
			content: "ledger:\n  agent_types:\n    reviewer-deep: opus\n",
			want:    map[string]AgentTypeTags{},
			warn: `ledger.agent_types.reviewer-deep is "opus", ` +
				"not a map with model and effort: no tags",
		},
		{
			name:    "non-map section is ignored",
			content: "ledger:\n  agent_types: [reviewer-deep]\n",
			want:    map[string]AgentTypeTags{},
			warn:    "ledger.agent_types is a list, not a map of agent type to model and effort",
		},
		{
			name: "non-string model is dropped",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-deep: {model: 4.5, effort: high}\n",
			want: map[string]AgentTypeTags{"reviewer-deep": {Effort: "high"}},
			warn: "ledger.agent_types.reviewer-deep.model is 4.5, not a string: " +
				"model tag dropped",
		},
		{
			name:    "a misspelt key is named and tags nothing",
			content: "ledger:\n  agent_types:\n    reviewer-deep: {modle: opus}\n",
			want:    map[string]AgentTypeTags{},
			warn: "ledger.agent_types.reviewer-deep.modle is not a known key " +
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
		{
			name: "a duplicate agent type keeps its first entry and the rest of the map",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-quick: {model: sonnet, effort: low}\n" +
				"    reviewer-deep: {model: opus, effort: high}\n" +
				"    reviewer-quick: {model: haiku, effort: max}\n",
			want: map[string]AgentTypeTags{
				"reviewer-quick": {Model: "sonnet", Effort: "low"},
				"reviewer-deep":  {Model: "opus", Effort: "high"},
			},
			warn: "ledger.agent_types.reviewer-quick is defined twice (lines 3 and 5): " +
				"later one dropped",
		},
		{
			name: "a duplicate field keeps its first value and the rest of the entry",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-deep:\n" +
				"      model: opus\n" +
				"      model: sonnet\n" +
				"      effort: high\n" +
				"    reviewer-quick: {model: sonnet, effort: low}\n",
			want: map[string]AgentTypeTags{
				"reviewer-deep":  {Model: "opus", Effort: "high"},
				"reviewer-quick": {Model: "sonnet", Effort: "low"},
			},
			warn: "ledger.agent_types.reviewer-deep.model is defined twice " +
				"(lines 4 and 5): later one dropped",
		},
		{
			name: "an entry YAML cannot read drops only itself",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-deep: {model: !!int opus, effort: high}\n" +
				"    reviewer-quick: {model: sonnet, effort: low}\n",
			want: map[string]AgentTypeTags{"reviewer-quick": {Model: "sonnet", Effort: "low"}},
			warn: "ledger.agent_types.reviewer-deep could not be read (yaml: ",
		},
		{
			name: "a blank agent type is quoted so the blanks read apart",
			content: "ledger:\n  agent_types:\n" +
				"    '  ': {model: opus}\n" +
				"    reviewer-quick: {model: sonnet}\n",
			want: map[string]AgentTypeTags{"reviewer-quick": {Model: "sonnet"}},
			warn: `ledger.agent_types has an entry with a blank agent type ("  "): dropped`,
		},
		{
			name: "a merge key inside an entry works as YAML defines it",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-quick: &sonnet {model: sonnet, effort: low}\n" +
				"    reviewer-standard: {<<: *sonnet, effort: medium}\n",
			want: map[string]AgentTypeTags{
				"reviewer-quick":    {Model: "sonnet", Effort: "low"},
				"reviewer-standard": {Model: "sonnet", Effort: "medium"},
			},
		},
		{
			name: "a merge key in place of an agent type is named and dropped",
			content: "x-workers: &workers\n" +
				"  reviewer-quick: {model: sonnet}\n" +
				"ledger:\n  agent_types:\n" +
				"    <<: *workers\n" +
				"    reviewer-deep: {model: opus}\n",
			want: map[string]AgentTypeTags{"reviewer-deep": {Model: "opus"}},
			warn: "ledger.agent_types has a merge key (<<) at line 5",
		},
		{
			name: "glob keys load as written",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-deep*: {model: opus, effort: high}\n" +
				"    '*': {model: sonnet}\n",
			want: map[string]AgentTypeTags{
				"reviewer-deep*": {Model: "opus", Effort: "high"},
				"*":              {Model: "sonnet"},
			},
		},
		{
			name: "a * before the end warns and drops that entry",
			content: "ledger:\n  agent_types:\n" +
				"    reviewer-*-deep: {model: opus, effort: high}\n" +
				"    reviewer-quick*: {model: sonnet, effort: low}\n",
			want: map[string]AgentTypeTags{"reviewer-quick*": {Model: "sonnet", Effort: "low"}},
			warn: "ledger.agent_types.reviewer-*-deep has a * before its end " +
				"(only a trailing * is a glob): dropped",
		},
		{
			name: "a key that is not a name is dropped",
			content: "ledger:\n  agent_types:\n" +
				"    [a, b]: {model: opus}\n" +
				"    reviewer-quick: {model: sonnet}\n",
			want: map[string]AgentTypeTags{"reviewer-quick": {Model: "sonnet"}},
			warn: "ledger.agent_types has a key at line 3 that is not a name: dropped",
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
	for _, a := range l.Adjustments() {
		if strings.Contains(a, "\n") {
			t.Errorf("adjustment %q spans lines, want one line per doctor warning", a)
		}
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
[ledger.agent_types.reviewer-deep]
model = "opus"
effort = "high"

[ledger.agent_types.reviewer-quick]
model = "sonnet"
effort = 3

[ledger.agent_types."reviewer-*-deep"]
model = "opus"
`),
	}
	cfg, err := LoadFrom(p)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v, want none", err)
	}
	checkAgentTypes(t, cfg.Ledger, map[string]AgentTypeTags{
		"reviewer-deep":  {Model: "opus", Effort: "high"},
		"reviewer-quick": {Model: "sonnet"},
	}, "ledger.agent_types.reviewer-quick.effort is 3, not a string: effort tag dropped")
	const misplaced = "ledger.agent_types.reviewer-*-deep has a * before its end"
	if adj := strings.Join(cfg.Ledger.Adjustments(), "\n"); !strings.Contains(adj, misplaced) {
		t.Errorf("adjustments = %q, want one containing %q", adj, misplaced)
	}
}

// TestLedgerAgentTypesForGlobs: a named spawn reports its name as its agent
// type, so a reviewer spawned as reviewer-deep-<slug> is tagged
// through the reviewer-deep* key. An exact key beats a glob, the
// longest glob wins, and a plain key does not match a slugged name.
func TestLedgerAgentTypesForGlobs(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", `ledger:
  agent_types:
    reviewer*: {model: haiku, effort: low}
    reviewer-deep*: {model: opus, effort: high}
    reviewer-deep-csl-prune: {model: opus, effort: max}
    docs-writer: {model: opus, effort: xhigh}
`)})
	tests := []struct {
		agentType string
		want      AgentTypeTags
	}{
		{"reviewer-deep-ledger-tags", AgentTypeTags{Model: "opus", Effort: "high"}},
		{"reviewer-deep-csl-prune", AgentTypeTags{Model: "opus", Effort: "max"}},
		{"reviewer-quick-docs", AgentTypeTags{Model: "haiku", Effort: "low"}},
		{"docs-writer", AgentTypeTags{Model: "opus", Effort: "xhigh"}},
		{"docs-writer-belt", AgentTypeTags{}},
		{"Explore", AgentTypeTags{}},
	}
	for _, tt := range tests {
		if got := cfg.Ledger.AgentTypes.For(tt.agentType); got != tt.want {
			t.Errorf("For(%q) = %v, want %v", tt.agentType, got, tt.want)
		}
	}
}

// TestLedgerAgentTypesLeaveTheRestOfTheSection: a broken agent_types does
// not cost the ledger keys beside it.
func TestLedgerAgentTypesLeaveTheRestOfTheSection(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml",
		"ledger:\n  enabled: false\n  source: custom-ledger\n  agent_types: 7\n")})
	if cfg.Ledger.On() || cfg.Ledger.SourceName() != "custom-ledger" {
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

// TestLedgerAgentTypesLargeMap: a map far past any real config still
// loads in full, a late duplicate included, since walking the pairs keeps
// the duplicate check linear.
func TestLedgerAgentTypesLargeMap(t *testing.T) {
	const entries = 10000
	var b strings.Builder
	b.WriteString("ledger:\n  agent_types:\n")
	for i := range entries {
		fmt.Fprintf(&b, "    worker-%d: {model: opus, effort: high}\n", i)
	}
	b.WriteString("    worker-0: {model: sonnet, effort: low}\n")
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", b.String())})
	if got := len(cfg.Ledger.AgentTypes.Names()); got != entries {
		t.Errorf("len(Names()) = %d, want %d", got, entries)
	}
	if got := cfg.Ledger.AgentTypes.For("worker-0"); got != (AgentTypeTags{Model: "opus", Effort: "high"}) {
		t.Errorf("For(worker-0) = %v, want the first entry", got)
	}
	if got := cfg.Ledger.Adjustments(); len(got) != 1 {
		t.Errorf("adjustments = %v, want the one duplicate", got)
	}
}
