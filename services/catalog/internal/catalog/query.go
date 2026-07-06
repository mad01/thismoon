package catalog

import "strings"

// Query filters the catalog. A zero Query matches everything. Text is a
// case-insensitive substring matched against name, description and tags; Owner
// is a case-insensitive exact match; Kind, when set, restricts the kind.
type Query struct {
	Text  string
	Owner string
	Kind  Kind
}

// matches reports whether an entity satisfies the query.
func (q Query) matches(e Entity) bool {
	if q.Kind != "" && e.Kind != q.Kind {
		return false
	}
	if q.Owner != "" && !strings.EqualFold(e.Spec.Owner, q.Owner) {
		return false
	}
	if q.Text != "" {
		needle := strings.ToLower(q.Text)
		if !entityMatchesText(e, needle) {
			return false
		}
	}
	return true
}

func entityMatchesText(e Entity, needle string) bool {
	if strings.Contains(strings.ToLower(e.Metadata.Name), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Metadata.Description), needle) {
		return true
	}
	for _, t := range e.Metadata.Tags {
		if strings.Contains(strings.ToLower(t), needle) {
			return true
		}
	}
	return false
}

// Search returns the entities matching the query, sorted by name.
func (c *Catalog) Search(q Query) []Entity {
	var out []Entity
	for _, e := range c.entities {
		if q.matches(e) {
			out = append(out, e)
		}
	}
	return byName(out)
}
