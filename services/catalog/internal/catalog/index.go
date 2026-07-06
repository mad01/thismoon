package catalog

import "sort"

// Catalog is an in-memory, queryable index of entities. It is immutable once
// built; refreshing means building a new Catalog from a fresh scan.
type Catalog struct {
	entities   []Entity
	systems    map[string]Entity
	components map[string]Entity
}

// NewCatalog indexes the given entities by kind and name. When two entities of
// the same kind share a name, the last one wins. The input order is preserved
// for All()/Systems()/Components() after a stable sort by name.
func NewCatalog(entities []Entity) *Catalog {
	c := &Catalog{
		entities:   append([]Entity(nil), entities...),
		systems:    make(map[string]Entity),
		components: make(map[string]Entity),
	}
	for _, e := range entities {
		switch e.Kind {
		case KindSystem:
			c.systems[e.Metadata.Name] = e
		case KindComponent:
			c.components[e.Metadata.Name] = e
		}
	}
	return c
}

// byName returns entities sorted by name.
func byName(in []Entity) []Entity {
	out := append([]Entity(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Name < out[j].Metadata.Name
	})
	return out
}

// All returns every entity, sorted by name.
func (c *Catalog) All() []Entity { return byName(c.entities) }

// Systems returns all System entities, sorted by name.
func (c *Catalog) Systems() []Entity {
	out := make([]Entity, 0, len(c.systems))
	for _, e := range c.systems {
		out = append(out, e)
	}
	return byName(out)
}

// Components returns all Component entities, sorted by name.
func (c *Catalog) Components() []Entity {
	out := make([]Entity, 0, len(c.components))
	for _, e := range c.components {
		out = append(out, e)
	}
	return byName(out)
}

// System looks up a System by name.
func (c *Catalog) System(name string) (Entity, bool) {
	e, ok := c.systems[name]
	return e, ok
}

// Component looks up a Component by name.
func (c *Catalog) Component(name string) (Entity, bool) {
	e, ok := c.components[name]
	return e, ok
}

// ComponentsOf returns the Components whose spec.system is the given system,
// sorted by name.
func (c *Catalog) ComponentsOf(system string) []Entity {
	var out []Entity
	for _, e := range c.components {
		if e.Spec.System == system {
			out = append(out, e)
		}
	}
	return byName(out)
}

// Owners returns the distinct, sorted set of owners across all entities.
func (c *Catalog) Owners() []string {
	seen := make(map[string]struct{})
	for _, e := range c.entities {
		if e.Spec.Owner != "" {
			seen[e.Spec.Owner] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for o := range seen {
		out = append(out, o)
	}
	sort.Strings(out)
	return out
}
