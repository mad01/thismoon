package catalog

import (
	"strings"
	"testing"
)

func TestCheckUnique_OK(t *testing.T) {
	ents := []Entity{
		{Kind: KindSystem, Metadata: Metadata{Name: "dotfiles"}, Spec: Spec{Owner: "mad01"}},
		{Kind: KindComponent, Metadata: Metadata{Name: "present"}, Spec: Spec{Owner: "mad01", System: "dotfiles"}},
	}
	if err := CheckUnique(ents); err != nil {
		t.Errorf("CheckUnique = %v, want nil", err)
	}
}

func TestCheckUnique_SystemComponentCollision(t *testing.T) {
	// A System and a Component sharing a name must collide — single namespace.
	ents := []Entity{
		{Kind: KindSystem, Metadata: Metadata{Name: "present"}, Spec: Spec{Owner: "mad01"}, SourcePath: "a/service-info.yaml"},
		{Kind: KindComponent, Metadata: Metadata{Name: "present"}, Spec: Spec{Owner: "mad01", System: "x"}, SourcePath: "b/service-info.yaml"},
	}
	err := CheckUnique(ents)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	for _, want := range []string{"present", "a/service-info.yaml", "b/service-info.yaml", "System", "Component"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %s", want, err.Error())
		}
	}
}

func TestCheckUnique_DuplicateComponents(t *testing.T) {
	ents := []Entity{
		{Kind: KindComponent, Metadata: Metadata{Name: "csl"}, Spec: Spec{Owner: "mad01", System: "a"}},
		{Kind: KindComponent, Metadata: Metadata{Name: "csl"}, Spec: Spec{Owner: "mad01", System: "b"}},
	}
	if err := CheckUnique(ents); err == nil {
		t.Error("expected duplicate-component error, got nil")
	}
}
