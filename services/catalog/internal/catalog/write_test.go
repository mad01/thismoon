package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderEntity_RoundTrip(t *testing.T) {
	e := Entity{
		Kind:     KindComponent,
		Metadata: Metadata{Name: "present", Description: "briefings", Tags: []string{"go", "cli"}},
		Spec:     Spec{Owner: "mad01", Type: "cli", Lifecycle: "production", System: "dotfiles"},
	}
	data, err := RenderEntity(e)
	if err != nil {
		t.Fatalf("RenderEntity: %v", err)
	}
	got, err := ParseEntities(data)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(got) != 1 || got[0].Metadata.Name != "present" || got[0].Spec.System != "dotfiles" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got[0].APIVersion != DefaultAPIVersion {
		t.Errorf("apiVersion = %q, want default", got[0].APIVersion)
	}
}

func TestWriteServiceInfo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newtool")
	e := Entity{Kind: KindComponent, Metadata: Metadata{Name: "newtool"}, Spec: Spec{Owner: "mad01", System: "dotfiles"}}

	path, err := WriteServiceInfo(dir, e)
	if err != nil {
		t.Fatalf("WriteServiceInfo: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not written: %v", err)
	}

	// Re-scan should surface it.
	ents, err := ScanDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(ents) != 1 || ents[0].Metadata.Name != "newtool" {
		t.Fatalf("scan after write = %+v", ents)
	}

	// Second write must refuse to overwrite.
	if _, err := WriteServiceInfo(dir, e); err == nil {
		t.Error("expected overwrite to be refused, got nil")
	}
}

func TestWriteServiceInfo_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	bad := Entity{Kind: KindComponent, Metadata: Metadata{Name: "x"}, Spec: Spec{Owner: "mad01"}} // no system
	if _, err := WriteServiceInfo(dir, bad); err == nil {
		t.Error("expected validation error for component without system")
	}
}
