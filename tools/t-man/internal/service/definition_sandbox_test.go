package service

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProfile creates a temp .sb file and returns its path.
func writeProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sb")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}
	return path
}

func TestDefinitionHash_SandboxProfilePath(t *testing.T) {
	base := Definition{
		Name:    "test-service",
		Command: "/usr/bin/echo",
	}

	withProfile := base
	withProfile.SandboxProfile = "/etc/profiles/a.sb"

	h1, err := base.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	h2, err := withProfile.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	if h1 == h2 {
		t.Error("expected different hashes when sandbox profile path differs")
	}
}

func TestDefinitionHash_SandboxProfileContent(t *testing.T) {
	def1 := Definition{
		Name:                 "test-service",
		Command:              "/usr/bin/echo",
		SandboxProfile:       "/etc/profiles/a.sb",
		SandboxProfileSHA256: "digest-v1",
	}
	def2 := def1
	def2.SandboxProfileSHA256 = "digest-v2"

	h1, err := def1.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	h2, err := def2.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	if h1 == h2 {
		t.Error("expected different hashes when sandbox profile content digest differs")
	}
}

func TestPopulateSandboxDigest(t *testing.T) {
	profile := writeProfile(t, `(version 1)(allow default)`)

	def := Definition{
		Name:           "test-service",
		Command:        "/usr/bin/echo",
		SandboxProfile: profile,
	}

	if err := def.PopulateSandboxDigest(); err != nil {
		t.Fatalf("PopulateSandboxDigest() error: %v", err)
	}
	if def.SandboxProfileSHA256 == "" {
		t.Fatal("expected digest to be populated")
	}
	first := def.SandboxProfileSHA256

	// Editing the file changes the digest
	if err := os.WriteFile(profile, []byte(`(version 1)(deny default)`), 0o644); err != nil {
		t.Fatalf("failed to rewrite profile: %v", err)
	}
	if err := def.PopulateSandboxDigest(); err != nil {
		t.Fatalf("PopulateSandboxDigest() after edit error: %v", err)
	}
	if def.SandboxProfileSHA256 == first {
		t.Error("expected digest to change when profile content changes")
	}
}

func TestPopulateSandboxDigest_NoProfile(t *testing.T) {
	def := Definition{Name: "test-service", Command: "/usr/bin/echo"}
	if err := def.PopulateSandboxDigest(); err != nil {
		t.Fatalf("expected no-op without profile, got: %v", err)
	}
	if def.SandboxProfileSHA256 != "" {
		t.Error("expected empty digest without profile")
	}
}

func TestPopulateSandboxDigest_MissingFile(t *testing.T) {
	def := Definition{
		Name:           "test-service",
		Command:        "/usr/bin/echo",
		SandboxProfile: "/nonexistent/profile.sb",
	}
	if err := def.PopulateSandboxDigest(); err == nil {
		t.Error("expected error for missing profile file")
	}
}

func TestDefinitionValidate_SandboxProfile(t *testing.T) {
	profile := writeProfile(t, `(version 1)(allow default)`)

	tests := []struct {
		name    string
		profile string
		wantErr bool
	}{
		{name: "valid profile file", profile: profile, wantErr: false},
		{name: "no profile is fine", profile: "", wantErr: false},
		{name: "relative path rejected", profile: "relative/path.sb", wantErr: true},
		{name: "missing file rejected", profile: "/nonexistent/profile.sb", wantErr: true},
		{name: "directory rejected", profile: filepath.Dir(profile), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{
				Name:           "test-service",
				Command:        "/usr/bin/true",
				SandboxProfile: tt.profile,
			}
			err := def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
