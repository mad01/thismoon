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

// AgentTypes is ledger.agent_types resolved: agent type name to the model
// and effort its definition declares. The SubagentStart payload names the
// agent type and no model, so the map restates a fact the definition
// already holds; belt never infers one. Like every ledger value it is
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
	key := "ledger.agent_types." + name
	if strings.TrimSpace(name) == "" {
		return AgentTypeTags{}, []string{
			"ledger.agent_types has an entry with a blank agent type: dropped",
		}
	}
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

// UnmarshalYAML resolves the key whatever its shape. A node that does not
// decode at all becomes a warning too: this key never fails the file.
func (a *AgentTypes) UnmarshalYAML(node *yaml.Node) error {
	var raw any
	if err := node.Decode(&raw); err != nil {
		*a = AgentTypes{warns: []string{fmt.Sprintf(
			"ledger.agent_types could not be read (%v): "+
				"no subagent-start entry gets a model or effort tag", err,
		)}}
		return nil
	}
	*a = ResolveAgentTypes(raw)
	return nil
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

// For is what a subagent of agentType is tagged with: the zero value for a
// type with no entry.
func (a AgentTypes) For(agentType string) AgentTypeTags { return a.tags[agentType] }

// Names lists the agent types with an entry in effect, sorted.
func (a AgentTypes) Names() []string { return slices.Sorted(maps.Keys(a.tags)) }

// Adjustments names every entry, key or value belt dropped from what the
// file states.
func (a AgentTypes) Adjustments() []string { return slices.Clone(a.warns) }
