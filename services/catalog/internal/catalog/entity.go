// Package catalog is the functional core of the systems catalog: pure types,
// parsing, scanning, indexing, querying and rendering of Backstage-aligned
// service-info.yaml entities. All I/O boundaries (filesystem walks, HTTP) live
// in the calling packages; everything here is deterministic and testable.
package catalog

import (
	"errors"
	"fmt"
	"regexp"
)

// DefaultAPIVersion is the apiVersion stamped on entities we write.
const DefaultAPIVersion = "catalog.mad01/v1alpha1"

// Kind enumerates the entity kinds the catalog understands.
type Kind string

const (
	KindSystem    Kind = "System"
	KindComponent Kind = "Component"
)

// Metadata holds the identity of an entity, shaped like Backstage metadata.
type Metadata struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Spec holds the kind-specific fields. Owner applies to all kinds; Type,
// Lifecycle and System apply to Components.
type Spec struct {
	Owner     string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Type      string `yaml:"type,omitempty" json:"type,omitempty"`
	Lifecycle string `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
	System    string `yaml:"system,omitempty" json:"system,omitempty"`
}

// Entity is a single catalog entry (a System or a Component).
type Entity struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       Kind     `yaml:"kind" json:"kind"`
	Metadata   Metadata `yaml:"metadata" json:"metadata"`
	Spec       Spec     `yaml:"spec" json:"spec"`

	// SourcePath records the file the entity was loaded from. It is not part of
	// the serialized form; it is populated by the scanner for traceability.
	SourcePath string `yaml:"-" json:"sourcePath,omitempty"`

	// RepoURL is a browseable link to the entity's source on its git remote
	// (e.g. https://github.com/mad01/dotfiles/tree/HEAD/present). It is derived
	// by the scanner from the repo's git config, never serialized to disk, and
	// empty when the source has no resolvable remote.
	RepoURL string `yaml:"-" json:"repoURL,omitempty"`
}

// nameRe follows Backstage entity-name rules: alphanumeric start/end, with
// '-', '_' and '.' allowed in between.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// Validate reports the first problem with an entity, or nil if it is well
// formed. Rules: known kind, valid name (<=63 chars), an owner, and — for
// Components — a system link.
func (e Entity) Validate() error {
	switch e.Kind {
	case KindSystem, KindComponent:
	case "":
		return errors.New("missing kind (expected System or Component)")
	default:
		return fmt.Errorf("unknown kind %q (expected System or Component)", e.Kind)
	}

	name := e.Metadata.Name
	if name == "" {
		return errors.New("missing metadata.name")
	}
	if len(name) > 63 {
		return fmt.Errorf("metadata.name %q exceeds 63 characters", name)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("metadata.name %q is not a valid name (use alphanumerics, '-', '_', '.')", name)
	}

	if e.Spec.Owner == "" {
		return fmt.Errorf("%s %q: missing spec.owner", e.Kind, name)
	}

	if e.Kind == KindComponent && e.Spec.System == "" {
		return fmt.Errorf("component %q: missing spec.system", name)
	}

	return nil
}
