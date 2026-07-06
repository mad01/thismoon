package catalog

import (
	"reflect"
	"testing"
)

func fixtureCatalog() *Catalog {
	return NewCatalog([]Entity{
		{Kind: KindSystem, Metadata: Metadata{Name: "dotfiles", Description: "tooling monorepo", Tags: []string{"monorepo"}}, Spec: Spec{Owner: "mad01"}},
		{Kind: KindComponent, Metadata: Metadata{Name: "present", Tags: []string{"go", "cli"}}, Spec: Spec{Owner: "mad01", System: "dotfiles", Type: "cli"}},
		{Kind: KindComponent, Metadata: Metadata{Name: "bionic"}, Spec: Spec{Owner: "mad01", System: "dotfiles", Type: "cli"}},
		{Kind: KindSystem, Metadata: Metadata{Name: "code-search-local"}, Spec: Spec{Owner: "alice"}},
		{Kind: KindComponent, Metadata: Metadata{Name: "csl"}, Spec: Spec{Owner: "alice", System: "code-search-local"}},
	})
}

func names(es []Entity) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Metadata.Name
	}
	return out
}

func TestCatalog_KindAccessors(t *testing.T) {
	c := fixtureCatalog()
	if got := names(c.Systems()); !reflect.DeepEqual(got, []string{"code-search-local", "dotfiles"}) {
		t.Errorf("Systems() = %v", got)
	}
	if got := names(c.Components()); !reflect.DeepEqual(got, []string{"bionic", "csl", "present"}) {
		t.Errorf("Components() = %v", got)
	}
}

func TestCatalog_ComponentsOf(t *testing.T) {
	c := fixtureCatalog()
	if got := names(c.ComponentsOf("dotfiles")); !reflect.DeepEqual(got, []string{"bionic", "present"}) {
		t.Errorf("ComponentsOf(dotfiles) = %v", got)
	}
	if got := names(c.ComponentsOf("code-search-local")); !reflect.DeepEqual(got, []string{"csl"}) {
		t.Errorf("ComponentsOf(code-search-local) = %v", got)
	}
}

func TestCatalog_Lookup(t *testing.T) {
	c := fixtureCatalog()
	if _, ok := c.System("dotfiles"); !ok {
		t.Error("System(dotfiles) not found")
	}
	if _, ok := c.Component("present"); !ok {
		t.Error("Component(present) not found")
	}
	if _, ok := c.System("present"); ok {
		t.Error("System(present) should not exist (it is a Component)")
	}
}

func TestCatalog_Owners(t *testing.T) {
	c := fixtureCatalog()
	if got := c.Owners(); !reflect.DeepEqual(got, []string{"alice", "mad01"}) {
		t.Errorf("Owners() = %v, want [alice mad01]", got)
	}
}

func TestCatalog_Search(t *testing.T) {
	c := fixtureCatalog()
	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"empty matches all", Query{}, []string{"bionic", "code-search-local", "csl", "dotfiles", "present"}},
		{"by owner", Query{Owner: "alice"}, []string{"code-search-local", "csl"}},
		{"by owner case-insensitive", Query{Owner: "MAD01"}, []string{"bionic", "dotfiles", "present"}},
		{"by name substring", Query{Text: "pres"}, []string{"present"}},
		{"by tag", Query{Text: "monorepo"}, []string{"dotfiles"}},
		{"by description", Query{Text: "tooling"}, []string{"dotfiles"}},
		{"kind filter", Query{Kind: KindSystem}, []string{"code-search-local", "dotfiles"}},
		{"owner + kind", Query{Owner: "mad01", Kind: KindComponent}, []string{"bionic", "present"}},
		{"no match", Query{Text: "zzz"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(c.Search(tt.q))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Search(%+v) = %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}
