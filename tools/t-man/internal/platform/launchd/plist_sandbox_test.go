package launchd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
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

func TestGeneratePlist_SandboxWrapping(t *testing.T) {
	profile := writeProfile(t, `(version 1)(allow default)`)

	def := &service.Definition{
		Name:           "sandboxed-service",
		Command:        "/usr/bin/true",
		Args:           []string{"--flag", "value"},
		SandboxProfile: profile,
	}

	data, err := GeneratePlist(def, "test-version")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	homeDir, _ := os.UserHomeDir()
	wantArgs := []string{
		SandboxExecPath, "-D", "HOME=" + homeDir, "-f", profile,
		"/usr/bin/true", "--flag", "value",
	}
	if !reflect.DeepEqual(parsed.ProgramArguments, wantArgs) {
		t.Errorf("ProgramArguments = %v, want %v", parsed.ProgramArguments, wantArgs)
	}

	if parsed.TManMetadata.SandboxProfile != profile {
		t.Errorf(
			"metadata SandboxProfile = %q, want %q",
			parsed.TManMetadata.SandboxProfile,
			profile,
		)
	}
	if parsed.TManMetadata.SandboxProfileSHA256 == "" {
		t.Error("metadata SandboxProfileSHA256 should be populated")
	}
}

func TestGeneratePlist_NoSandboxNoWrapping(t *testing.T) {
	def := &service.Definition{
		Name:    "plain-service",
		Command: "/usr/bin/true",
		Args:    []string{"arg1"},
	}

	data, err := GeneratePlist(def, "test-version")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	wantArgs := []string{"/usr/bin/true", "arg1"}
	if !reflect.DeepEqual(parsed.ProgramArguments, wantArgs) {
		t.Errorf("ProgramArguments = %v, want %v", parsed.ProgramArguments, wantArgs)
	}
	if parsed.TManMetadata.SandboxProfile != "" {
		t.Errorf("metadata SandboxProfile = %q, want empty", parsed.TManMetadata.SandboxProfile)
	}
}

// TestGeneratePlist_SandboxRoundTrip pins the idempotency invariant: a
// definition generated into a plist and parsed back must hash identically,
// otherwise every reconcile sees a phantom diff and bounces the service.
func TestGeneratePlist_SandboxRoundTrip(t *testing.T) {
	profile := writeProfile(t, `(version 1)(allow default)(deny network*)`)

	def := &service.Definition{
		Name:            "sandboxed-service",
		Command:         "/usr/bin/true",
		Args:            []string{"--port", "8765"},
		Environment:     map[string]string{"VIRTUAL_ENV": "/tmp/venv"},
		RunAtLoad:       true,
		KeepAlive:       true,
		StandardOutPath: "/tmp/stdout.log",
		StandardErrPath: "/tmp/stderr.log",
		SandboxProfile:  profile,
	}
	if err := def.PopulateSandboxDigest(); err != nil {
		t.Fatalf("PopulateSandboxDigest() error: %v", err)
	}

	data, err := GeneratePlist(def, "test-version")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	m := &Manager{}
	roundTripped := m.plistToDefinition(parsed)

	wantHash, err := def.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	gotHash, err := roundTripped.Hash()
	if err != nil {
		t.Fatalf("round-tripped Hash() error: %v", err)
	}
	if wantHash != gotHash {
		t.Errorf("round-trip hash mismatch: %s != %s\noriginal: %+v\nround-tripped: %+v",
			wantHash, gotHash, def, roundTripped)
	}

	// Editing the profile must change the desired hash vs the stored one
	if err := os.WriteFile(profile, []byte(`(version 1)(deny default)`), 0o644); err != nil {
		t.Fatalf("failed to rewrite profile: %v", err)
	}
	edited := *def
	edited.SandboxProfileSHA256 = ""
	if err := edited.PopulateSandboxDigest(); err != nil {
		t.Fatalf("PopulateSandboxDigest() after edit error: %v", err)
	}
	editedHash, err := edited.Hash()
	if err != nil {
		t.Fatalf("edited Hash() error: %v", err)
	}
	if editedHash == gotHash {
		t.Error("expected hash to change after profile content edit")
	}
}

func TestUnwrapSandboxArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "standard wrapped args",
			in: []string{
				SandboxExecPath,
				"-D",
				"HOME=/Users/x",
				"-f",
				"/p/a.sb",
				"/usr/bin/python",
				"-m",
				"server",
			},
			want: []string{"/usr/bin/python", "-m", "server"},
		},
		{
			name: "not wrapped returns unchanged",
			in:   []string{"/usr/bin/python", "-m", "server"},
			want: []string{"/usr/bin/python", "-m", "server"},
		},
		{
			name: "empty returns unchanged",
			in:   []string{},
			want: []string{},
		},
		{
			name: "wrapped without command returns nil",
			in:   []string{SandboxExecPath, "-D", "HOME=/Users/x", "-f", "/p/a.sb"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapSandboxArgs(tt.in)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unwrapSandboxArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
