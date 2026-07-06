package catalog

import (
	"strings"
	"testing"
)

func TestParseEntities_MultiDoc(t *testing.T) {
	in := `
apiVersion: catalog.mad01/v1alpha1
kind: System
metadata:
  name: dotfiles
  description: Personal dev tooling monorepo
  tags: [monorepo, tooling]
spec:
  owner: mad01
---
apiVersion: catalog.mad01/v1alpha1
kind: Component
metadata:
  name: present
  tags: [go, cli, mcp]
spec:
  type: cli
  lifecycle: production
  owner: mad01
  system: dotfiles
`
	got, err := ParseEntities([]byte(in))
	if err != nil {
		t.Fatalf("ParseEntities: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2", len(got))
	}
	if got[0].Kind != KindSystem || got[0].Metadata.Name != "dotfiles" {
		t.Errorf("entity 0 = %+v, want System/dotfiles", got[0])
	}
	if got[1].Kind != KindComponent || got[1].Spec.System != "dotfiles" {
		t.Errorf("entity 1 = %+v, want Component/system=dotfiles", got[1])
	}
	if len(got[0].Metadata.Tags) != 2 {
		t.Errorf("entity 0 tags = %v, want 2", got[0].Metadata.Tags)
	}
}

func TestParseEntities_DefaultsAPIVersion(t *testing.T) {
	in := `
kind: System
metadata:
  name: standalone
spec:
  owner: mad01
`
	got, err := ParseEntities([]byte(in))
	if err != nil {
		t.Fatalf("ParseEntities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1", len(got))
	}
	if got[0].APIVersion != DefaultAPIVersion {
		t.Errorf("apiVersion = %q, want %q", got[0].APIVersion, DefaultAPIVersion)
	}
}

func TestParseEntities_SkipsBlankDocs(t *testing.T) {
	in := `
kind: System
metadata:
  name: a
spec:
  owner: mad01
---
---
# a comment-only document
`
	got, err := ParseEntities([]byte(in))
	if err != nil {
		t.Fatalf("ParseEntities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1", len(got))
	}
}

func TestParseEntities_Errors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantSub string
	}{
		{
			name:    "missing owner",
			in:      "kind: System\nmetadata:\n  name: a\n",
			wantSub: "missing spec.owner",
		},
		{
			name:    "component without system",
			in:      "kind: Component\nmetadata:\n  name: a\nspec:\n  owner: mad01\n",
			wantSub: "missing spec.system",
		},
		{
			name:    "unknown kind",
			in:      "kind: Widget\nmetadata:\n  name: a\nspec:\n  owner: mad01\n",
			wantSub: "unknown kind",
		},
		{
			name:    "bad name",
			in:      "kind: System\nmetadata:\n  name: 'Not Valid!'\nspec:\n  owner: mad01\n",
			wantSub: "not a valid name",
		},
		{
			name:    "missing name",
			in:      "kind: System\nspec:\n  owner: mad01\n",
			wantSub: "missing metadata.name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseEntities([]byte(tt.in))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}
