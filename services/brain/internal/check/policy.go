package check

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// PolicyFile is where a vault keeps its policy, relative to the vault root.
const PolicyFile = ".brain/policy.yaml"

// Levels a rule reports at.
const (
	Error   = "error"
	Warning = "warning"
	Off     = "off"
)

// Rule names, the keys of a policy's rules map.
const (
	RuleSchema          = "schema"
	RuleStatusUnknown   = "status-unknown"
	RuleStatusFields    = "status-fields"
	RuleMissingField    = "missing-field"
	RuleProvenance      = "provenance"
	RuleLinkDangling    = "link-dangling"
	RuleLinkRevoked     = "link-revoked"
	RuleLinkUnvalidated = "link-unvalidated"
	RuleIDDuplicate     = "id-duplicate"
	RuleStemDuplicate   = "stem-duplicate"
	RuleClassification  = "classification"
	RuleSecret          = "secret"
	// RulePolicy reports a policy file that cannot be used. It is always an
	// error and no policy can turn it off.
	RulePolicy = "policy"
)

// DefaultLevels is the lenient default a vault without a policy file gets,
// and the level of every rule a policy does not name. A missing optional
// field is a warning, so a vault whose notes predate the lifecycle fields
// checks clean of errors.
var DefaultLevels = map[string]string{
	RuleSchema:          Error,
	RuleStatusUnknown:   Warning,
	RuleStatusFields:    Error,
	RuleMissingField:    Warning,
	RuleProvenance:      Warning,
	RuleLinkDangling:    Warning,
	RuleLinkRevoked:     Error,
	RuleLinkUnvalidated: Warning,
	RuleIDDuplicate:     Error,
	RuleStemDuplicate:   Warning,
	RuleClassification:  Error,
	RuleSecret:          Error,
}

// Policy is a vault's .brain/policy.yaml: the classifications the vault
// accepts and the level of each rule.
type Policy struct {
	// Classifications are the classification values the vault accepts;
	// empty accepts every one.
	Classifications []string `yaml:"classifications"`
	// Rules overrides DefaultLevels, rule name to error, warning or off.
	Rules map[string]string `yaml:"rules"`
}

// LoadPolicy reads the policy of the vault at root. A vault without the
// file gets the zero Policy, which is the lenient default.
func LoadPolicy(root string) (Policy, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PolicyFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return Policy{}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil && !errors.Is(err, io.EOF) {
		return Policy{}, fmt.Errorf("parse: %w", err)
	}
	return p, p.validate()
}

func (p Policy) validate() error {
	var problems []string
	for rule, level := range p.Rules {
		if _, ok := DefaultLevels[rule]; !ok {
			problems = append(
				problems,
				fmt.Sprintf("unknown rule %q (rules: %s)", rule, ruleNames()),
			)
		}
		if level != Error && level != Warning && level != Off {
			problems = append(
				problems,
				fmt.Sprintf("rule %s: level %q is not error, warning or off", rule, level),
			)
		}
	}
	for _, c := range p.Classifications {
		if !slices.Contains(lifecycle.Classifications, c) {
			problems = append(problems, fmt.Sprintf(
				"classification %q is not one of %s",
				c,
				strings.Join(lifecycle.Classifications, ", "),
			))
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// Level returns the level rule reports at under p.
func (p Policy) Level(rule string) string {
	if rule == RulePolicy {
		return Error
	}
	if level, ok := p.Rules[rule]; ok {
		return level
	}
	return DefaultLevels[rule]
}

// Allows reports whether the vault accepts a note of this classification.
func (p Policy) Allows(classification string) bool {
	return len(p.Classifications) == 0 || slices.Contains(p.Classifications, classification)
}

// allowed lists the classifications p accepts, for messages.
func (p Policy) allowed() string {
	if len(p.Classifications) == 0 {
		return strings.Join(lifecycle.Classifications, ", ")
	}
	return strings.Join(p.Classifications, ", ")
}

func ruleNames() string {
	names := make([]string, 0, len(DefaultLevels))
	for name := range DefaultLevels {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
