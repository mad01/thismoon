package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// effortLevels are the effort values a ledger.agent_types entry may
// declare: the values Claude Code agent frontmatter accepts.
var effortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// AgentTypeTags is what a subagent-start ledger entry is tagged with when
// the payload's agent_type has a ledger.agent_types entry: the model and
// effort that agent's definition declares. An empty field is a tag left
// off.
type AgentTypeTags struct {
	Model  string `yaml:"model,omitempty"`
	Effort string `yaml:"effort,omitempty"`
}

// AgentTypes is ledger.agent_types resolved: agent type pattern to the
// model and effort its definition declares. A key ending in * matches by
// prefix (MatchAgentType), any other key exactly. The SubagentStart payload
// names the agent type and no model, so the map restates a fact the
// definition already holds; belt never infers one. Like every ledger value it is
// resolved, never rejected. Any shape decodes, a malformed entry or value
// is dropped, and Adjustments names each drop for doctor to warn about. A
// typo here must not fail the file, because a config error denies every
// guarded tool call.
type AgentTypes struct {
	tags  map[string]AgentTypeTags
	warns []string
}

// ResolveAgentTypes reads ledger.agent_types from the generic value a YAML
// or TOML decoder produces for it. nil, an absent or empty key, resolves
// to no entries and no warnings.
func ResolveAgentTypes(raw any) AgentTypes {
	if raw == nil {
		return AgentTypes{}
	}
	entries, ok := asMapping(raw)
	if !ok {
		return AgentTypes{warns: []string{fmt.Sprintf(
			"ledger.agent_types is %s, not a map of agent type to model and effort: "+
				"no subagent-start entry gets a model or effort tag", describe(raw),
		)}}
	}
	a := AgentTypes{tags: make(map[string]AgentTypeTags, len(entries))}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		tags, warns := resolveAgentType(name, entries[name])
		a.warns = append(a.warns, warns...)
		if tags != (AgentTypeTags{}) {
			a.tags[name] = tags
		}
	}
	return a
}

// resolveAgentType reads one entry: a map holding model, effort, or both.
// A value that does not resolve drops its own tag only.
func resolveAgentType(name string, raw any) (AgentTypeTags, []string) {
	if warn := nameProblem(name); warn != "" {
		return AgentTypeTags{}, []string{warn}
	}
	key := "ledger.agent_types." + name
	fields, ok := asMapping(raw)
	if !ok {
		return AgentTypeTags{}, []string{fmt.Sprintf(
			"%s is %s, not a map with model and effort: no tags", key, describe(raw),
		)}
	}
	var tags AgentTypeTags
	var warns []string
	for _, field := range slices.Sorted(maps.Keys(fields)) {
		var warn string
		switch field {
		case "model":
			tags.Model, warn = modelValue(key+".model", fields[field])
		case "effort":
			tags.Effort, warn = effortValue(key+".effort", fields[field])
		default:
			warn = fmt.Sprintf("%s.%s is not a known key (model, effort): ignored", key, field)
		}
		if warn != "" {
			warns = append(warns, warn)
		}
	}
	_, hasModel := fields["model"]
	_, hasEffort := fields["effort"]
	if !hasModel && !hasEffort {
		warns = append(warns, key+" names neither model nor effort: no tags")
	}
	return tags, warns
}

// nameProblem is the warning for an entry whose agent type cannot match,
// or "" for a usable name. A blank name is quoted so that "" and "  " read
// apart; a * before the end is malformed (agentTypePatternProblem).
func nameProblem(name string) string {
	if strings.TrimSpace(name) == "" {
		return fmt.Sprintf(
			"ledger.agent_types has an entry with a blank agent type (%q): dropped",
			name,
		)
	}
	if problem := agentTypePatternProblem(name); problem != "" {
		return fmt.Sprintf("ledger.agent_types.%s %s: dropped", name, problem)
	}
	return ""
}

// modelValue is a declared model name: a string, trimmed, not empty.
func modelValue(key string, raw any) (string, string) {
	s, warn := stringValue(key, raw)
	if warn != "" {
		return "", warn + ": model tag dropped"
	}
	if s == "" {
		return "", key + " is empty: model tag dropped"
	}
	return s, ""
}

// effortValue is a declared effort: one of effortLevels after trimming.
func effortValue(key string, raw any) (string, string) {
	s, warn := stringValue(key, raw)
	if warn != "" {
		return "", warn + ": effort tag dropped"
	}
	if !slices.Contains(effortLevels, s) {
		return "", fmt.Sprintf("%s is %q, not one of %s: effort tag dropped",
			key, s, strings.Join(effortLevels, ", "))
	}
	return s, ""
}

// stringValue is raw as a trimmed string, or a warning naming what it is
// instead.
func stringValue(key string, raw any) (string, string) {
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Sprintf("%s is %s, not a string", key, describe(raw))
	}
	return strings.TrimSpace(s), ""
}

// asMapping reads a decoded mapping. YAML decodes a mapping with a
// non-string key into map[any]any; its keys are spelled out as text.
func asMapping(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	}
	return nil, false
}

// describe names a decoded value's shape for a warning.
func describe(v any) string {
	switch v := v.(type) {
	case nil:
		return "empty"
	case string:
		return fmt.Sprintf("%q", v)
	case []any, []map[string]any:
		return "a list"
	case map[string]any, map[any]any:
		return "a map"
	}
	return fmt.Sprint(v)
}

// UnmarshalYAML resolves the key whatever its shape: this key never fails
// the file. A mapping is walked pair by pair rather than decoded whole,
// because yaml.v3 fails a whole decode on one duplicate key or one value
// it cannot read. Walking keeps that damage to the entry it is in.
func (a *AgentTypes) UnmarshalYAML(node *yaml.Node) error {
	*a = resolveAgentTypesNode(dealias(node))
	return nil
}

// resolveAgentTypesNode reads ledger.agent_types from its YAML node. An
// agent type stated twice keeps its first entry, and each entry decodes
// on its own. A node that is not a mapping resolves like any other shape.
func resolveAgentTypesNode(node *yaml.Node) AgentTypes {
	if node.Kind != yaml.MappingNode {
		raw, err := decodeNode(node)
		if err != nil {
			return AgentTypes{warns: []string{fmt.Sprintf(
				"ledger.agent_types could not be read (%s): "+
					"no subagent-start entry gets a model or effort tag", oneLine(err),
			)}}
		}
		return ResolveAgentTypes(raw)
	}
	pairs, warns := uniquePairs(node, "ledger.agent_types")
	a := AgentTypes{tags: make(map[string]AgentTypeTags, len(pairs)), warns: warns}
	for _, p := range pairs {
		tags, warns := resolveAgentTypeNode(p)
		a.warns = append(a.warns, warns...)
		if tags != (AgentTypeTags{}) {
			a.tags[p.name] = tags
		}
	}
	return a
}

// resolveAgentTypeNode reads one entry from its node. A field stated twice
// keeps its first value, and a merge key (<<) inside the entry works as
// YAML defines it. An entry YAML cannot read is dropped on its own. A
// merge key in place of an agent type is dropped too: each agent type is
// listed by name.
func resolveAgentTypeNode(p pair) (AgentTypeTags, []string) {
	if isMergeKey(p.key) {
		return AgentTypeTags{}, []string{fmt.Sprintf(
			"ledger.agent_types has a merge key (<<) at line %d: dropped, "+
				"list each agent type by name", p.key.Line,
		)}
	}
	if warn := nameProblem(p.name); warn != "" {
		return AgentTypeTags{}, []string{warn}
	}
	key := "ledger.agent_types." + p.name
	node := dealias(p.value)
	var warns []string
	if node.Kind == yaml.MappingNode {
		var fields []pair
		fields, warns = uniquePairs(node, key)
		node = withPairs(node, fields)
	}
	raw, err := decodeNode(node)
	if err != nil {
		return AgentTypeTags{}, append(warns,
			fmt.Sprintf("%s could not be read (%s): no tags", key, oneLine(err)))
	}
	tags, more := resolveAgentType(p.name, raw)
	return tags, append(warns, more...)
}

// pair is one key and value of a YAML mapping, with the key's text as its
// name.
type pair struct {
	name       string
	key, value *yaml.Node
}

// uniquePairs lists a mapping's pairs in document order. A key stated
// twice keeps its first pair, and each later one is named and dropped. A
// key that is not a scalar has no name to match and is dropped too. The
// first-line map keeps the duplicate check linear in the number of pairs.
func uniquePairs(node *yaml.Node, path string) ([]pair, []string) {
	pairs := make([]pair, 0, len(node.Content)/2)
	firstLine := make(map[string]int, len(node.Content)/2)
	var warns []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, line := node.Content[i], node.Content[i].Line
		name := dealias(key)
		if name.Kind != yaml.ScalarNode {
			warns = append(warns,
				fmt.Sprintf("%s has a key at line %d that is not a name: dropped", path, line))
			continue
		}
		if first, dup := firstLine[name.Value]; dup {
			warns = append(warns, fmt.Sprintf(
				"%s.%s is defined twice (lines %d and %d): later one dropped",
				path, name.Value, first, line,
			))
			continue
		}
		firstLine[name.Value] = line
		pairs = append(pairs, pair{name: name.Value, key: key, value: node.Content[i+1]})
	}
	return pairs, warns
}

// withPairs is a shallow copy of a mapping node that holds only pairs, for
// yaml.v3 to decode once the duplicates are gone.
func withPairs(node *yaml.Node, pairs []pair) *yaml.Node {
	out := *node
	out.Content = make([]*yaml.Node, 0, 2*len(pairs))
	for _, p := range pairs {
		out.Content = append(out.Content, p.key, p.value)
	}
	return &out
}

// isMergeKey reports a YAML merge key (<<).
func isMergeKey(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!merge"
}

// dealias follows an alias to the node its anchor names.
func dealias(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

// decodeNode decodes one node into the generic value resolveAgentType
// reads. The caller names the key in its warning.
func decodeNode(n *yaml.Node) (any, error) {
	var raw any
	if err := n.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// oneLine folds an error yaml.v3 spreads over several lines, one per
// problem, into one line, so each doctor warning stays on its own line.
func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// UnmarshalTOML resolves the key from a legacy TOML file, whatever its
// shape.
func (a *AgentTypes) UnmarshalTOML(raw any) error {
	*a = ResolveAgentTypes(raw)
	return nil
}

// MarshalYAML renders the entries in effect, which is what `belt config`
// prints.
func (a AgentTypes) MarshalYAML() (any, error) { return a.tags, nil }

// IsZero reports that no entry is in effect, so an omitempty key drops out
// of `belt config`.
func (a AgentTypes) IsZero() bool { return len(a.tags) == 0 }

// For is what a subagent of agentType is tagged with: the entry whose key
// MatchAgentType picks (the exact key, else the longest matching glob), or
// the zero value when no key matches.
func (a AgentTypes) For(agentType string) AgentTypeTags {
	key, ok := MatchAgentType(maps.Keys(a.tags), agentType)
	if !ok {
		return AgentTypeTags{}
	}
	return a.tags[key]
}

// Names lists the keys in effect as the file writes them, a trailing *
// included, sorted.
func (a AgentTypes) Names() []string { return slices.Sorted(maps.Keys(a.tags)) }

// Adjustments names every entry, key or value belt dropped from what the
// file states.
func (a AgentTypes) Adjustments() []string { return slices.Clone(a.warns) }
