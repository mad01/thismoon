package launchd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

func TestGeneratePlist(t *testing.T) {
	// Create a temporary executable for testing
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	tests := []struct {
		name    string
		def     *service.Definition
		version string
		wantErr bool
		errMsg  string
	}{
		{
			name: "minimal valid definition",
			def: &service.Definition{
				Name:    "test.service",
				Command: execPath,
			},
			version: "1.0.0",
			wantErr: false,
		},
		{
			name: "full definition with all fields",
			def: &service.Definition{
				Name:            "com.example.myservice",
				Command:         execPath,
				Args:            []string{"--port", "8080"},
				WorkingDir:      tmpDir,
				Environment:     map[string]string{"ENV": "production", "PORT": "8080"},
				RunAtLoad:       true,
				KeepAlive:       true,
				StandardOutPath: filepath.Join(tmpDir, "stdout.log"),
				StandardErrPath: filepath.Join(tmpDir, "stderr.log"),
			},
			version: "1.2.3",
			wantErr: false,
		},
		{
			name:    "nil definition",
			def:     nil,
			version: "1.0.0",
			wantErr: true,
			errMsg:  "service definition cannot be nil",
		},
		{
			name: "invalid definition - missing name",
			def: &service.Definition{
				Command: execPath,
			},
			version: "1.0.0",
			wantErr: true,
			errMsg:  "invalid service definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GeneratePlist(tt.def, tt.version)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GeneratePlist() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf(
						"GeneratePlist() error = %q, want error containing %q",
						err.Error(),
						tt.errMsg,
					)
				}
				return
			}

			if err != nil {
				t.Errorf("GeneratePlist() unexpected error: %v", err)
				return
			}

			if len(got) == 0 {
				t.Error("GeneratePlist() returned empty data")
			}
		})
	}
}

func TestGeneratePlistXMLFormat(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:        "test.service",
		Command:     execPath,
		Args:        []string{"arg1", "arg2"},
		WorkingDir:  tmpDir,
		Environment: map[string]string{"KEY": "value"},
		RunAtLoad:   true,
		KeepAlive:   true,
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	content := string(data)

	// Verify XML format
	if !strings.HasPrefix(strings.TrimSpace(content), "<?xml") {
		t.Error("Generated plist does not start with XML declaration")
	}

	if !strings.Contains(content, "<!DOCTYPE plist") {
		t.Error("Generated plist missing DOCTYPE declaration")
	}

	// Verify required fields are present
	requiredFields := []string{
		"<key>Label</key>",
		"<key>ProgramArguments</key>",
		"<key>TManMetadata</key>",
		"<key>Hash</key>",
		"<key>ManagedBy</key>",
		"<key>Version</key>",
	}

	for _, field := range requiredFields {
		if !strings.Contains(content, field) {
			t.Errorf("Generated plist missing required field: %s", field)
		}
	}

	// Verify marker comment
	if !strings.Contains(content, MarkerComment) {
		t.Error("Generated plist missing marker comment")
	}
}

func TestGeneratePlistMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:    "test.service",
		Command: execPath,
	}

	version := "2.5.1"
	data, err := GeneratePlist(def, version)
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	// Parse the generated plist
	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	// Verify metadata
	if parsed.TManMetadata.ManagedBy != ManagedByValue {
		t.Errorf(
			"TManMetadata.ManagedBy = %q, want %q",
			parsed.TManMetadata.ManagedBy,
			ManagedByValue,
		)
	}

	if parsed.TManMetadata.Version != version {
		t.Errorf("TManMetadata.Version = %q, want %q", parsed.TManMetadata.Version, version)
	}

	if parsed.TManMetadata.Hash == "" {
		t.Error("TManMetadata.Hash is empty")
	}

	// Verify hash is consistent
	hash, err := def.Hash()
	if err != nil {
		t.Fatalf("Definition.Hash() error: %v", err)
	}
	if parsed.TManMetadata.Hash != hash {
		t.Errorf("TManMetadata.Hash = %q, want %q", parsed.TManMetadata.Hash, hash)
	}
}

func TestParsePlist(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:        "test.service",
		Command:     execPath,
		Args:        []string{"--flag", "value"},
		WorkingDir:  tmpDir,
		Environment: map[string]string{"FOO": "bar"},
		RunAtLoad:   true,
		KeepAlive:   false,
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	// Verify all fields are correctly parsed
	if parsed.Label != def.Name {
		t.Errorf("Label = %q, want %q", parsed.Label, def.Name)
	}

	expectedArgs := []string{def.Command}
	expectedArgs = append(expectedArgs, def.Args...)
	if len(parsed.ProgramArguments) != len(expectedArgs) {
		t.Errorf(
			"ProgramArguments length = %d, want %d",
			len(parsed.ProgramArguments),
			len(expectedArgs),
		)
	}

	if parsed.WorkingDirectory != def.WorkingDir {
		t.Errorf("WorkingDirectory = %q, want %q", parsed.WorkingDirectory, def.WorkingDir)
	}

	if parsed.RunAtLoad != def.RunAtLoad {
		t.Errorf("RunAtLoad = %v, want %v", parsed.RunAtLoad, def.RunAtLoad)
	}

	if parsed.KeepAlive != def.KeepAlive {
		t.Errorf("KeepAlive = %v, want %v", parsed.KeepAlive, def.KeepAlive)
	}
}

func TestParsePlistErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
			errMsg:  "plist data cannot be empty",
		},
		{
			name:    "invalid XML",
			data:    []byte("not valid xml"),
			wantErr: true,
			errMsg:  "failed to decode plist",
		},
		{
			name:    "valid XML but not plist",
			data:    []byte("<?xml version=\"1.0\"?><root></root>"),
			wantErr: true,
			errMsg:  "failed to decode plist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePlist(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Error("ParsePlist() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf(
						"ParsePlist() error = %q, want error containing %q",
						err.Error(),
						tt.errMsg,
					)
				}
			} else {
				if err != nil {
					t.Errorf("ParsePlist() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsManagedByTMan(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:    "test.service",
		Command: execPath,
	}

	// Generate a t-man managed plist
	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	isManaged, err := IsManagedByTMan(data)
	if err != nil {
		t.Fatalf("IsManagedByTMan() error: %v", err)
	}

	if !isManaged {
		t.Error("IsManagedByTMan() = false, want true for t-man generated plist")
	}

	// Test with non-managed plist (manually create one without TManMetadata)
	nonManagedPlist := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.service</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/echo</string>
	</array>
</dict>
</plist>`)

	isManaged, err = IsManagedByTMan(nonManagedPlist)
	if err != nil {
		t.Fatalf("IsManagedByTMan() error: %v", err)
	}

	if isManaged {
		t.Error("IsManagedByTMan() = true, want false for non-managed plist")
	}
}

func TestHasMarkerComment(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:    "test.service",
		Command: execPath,
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	if !HasMarkerComment(data) {
		t.Error("HasMarkerComment() = false, want true for generated plist")
	}

	// Test with data without marker
	dataWithoutMarker := []byte("<?xml version=\"1.0\"?><plist></plist>")
	if HasMarkerComment(dataWithoutMarker) {
		t.Error("HasMarkerComment() = true, want false for data without marker")
	}
}

func TestGeneratePlistConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:    "test.service",
		Command: execPath,
	}

	// Generate plist twice with the same definition
	data1, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() first call error: %v", err)
	}

	data2, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() second call error: %v", err)
	}

	// Parse both
	parsed1, err := ParsePlist(data1)
	if err != nil {
		t.Fatalf("ParsePlist() first error: %v", err)
	}

	parsed2, err := ParsePlist(data2)
	if err != nil {
		t.Fatalf("ParsePlist() second error: %v", err)
	}

	// Verify hashes are identical
	if parsed1.TManMetadata.Hash != parsed2.TManMetadata.Hash {
		t.Errorf("Hash mismatch: %q vs %q", parsed1.TManMetadata.Hash, parsed2.TManMetadata.Hash)
	}
}

func TestGeneratePlist_RoundTrip_AllFieldsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	original := &service.Definition{
		Name:            "com.example.roundtrip",
		Command:         execPath,
		Args:            []string{"--verbose", "--port", "8080", "start"},
		WorkingDir:      tmpDir,
		Environment:     map[string]string{"HOME": "/Users/test", "PATH": "/usr/bin", "EMPTY": ""},
		RunAtLoad:       true,
		KeepAlive:       true,
		StandardOutPath: filepath.Join(tmpDir, "stdout.log"),
		StandardErrPath: filepath.Join(tmpDir, "stderr.log"),
	}

	// Generate plist
	data, err := GeneratePlist(original, "2.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	// Parse it back
	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	// Verify every field
	if parsed.Label != original.Name {
		t.Errorf("Label = %q, want %q", parsed.Label, original.Name)
	}

	expectedArgs := append([]string{original.Command}, original.Args...)
	if len(parsed.ProgramArguments) != len(expectedArgs) {
		t.Fatalf(
			"ProgramArguments length = %d, want %d",
			len(parsed.ProgramArguments),
			len(expectedArgs),
		)
	}
	for i, arg := range expectedArgs {
		if parsed.ProgramArguments[i] != arg {
			t.Errorf("ProgramArguments[%d] = %q, want %q", i, parsed.ProgramArguments[i], arg)
		}
	}

	if parsed.WorkingDirectory != original.WorkingDir {
		t.Errorf("WorkingDirectory = %q, want %q", parsed.WorkingDirectory, original.WorkingDir)
	}

	if parsed.RunAtLoad != original.RunAtLoad {
		t.Errorf("RunAtLoad = %v, want %v", parsed.RunAtLoad, original.RunAtLoad)
	}

	if parsed.KeepAlive != original.KeepAlive {
		t.Errorf("KeepAlive = %v, want %v", parsed.KeepAlive, original.KeepAlive)
	}

	if parsed.StandardOutPath != original.StandardOutPath {
		t.Errorf("StandardOutPath = %q, want %q", parsed.StandardOutPath, original.StandardOutPath)
	}

	if parsed.StandardErrorPath != original.StandardErrPath {
		t.Errorf(
			"StandardErrorPath = %q, want %q",
			parsed.StandardErrorPath,
			original.StandardErrPath,
		)
	}

	// Verify environment variables
	for k, v := range original.Environment {
		got, ok := parsed.EnvironmentVariables[k]
		if !ok {
			t.Errorf("Missing environment variable %q", k)
		} else if got != v {
			t.Errorf("EnvironmentVariables[%q] = %q, want %q", k, got, v)
		}
	}

	// Verify metadata
	if parsed.TManMetadata.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", parsed.TManMetadata.Version, "2.0.0")
	}
	if parsed.TManMetadata.ManagedBy != ManagedByValue {
		t.Errorf("ManagedBy = %q, want %q", parsed.TManMetadata.ManagedBy, ManagedByValue)
	}

	// Verify hash matches definition hash
	originalHash, err := original.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if parsed.TManMetadata.Hash != originalHash {
		t.Errorf("Hash = %q, want %q", parsed.TManMetadata.Hash, originalHash)
	}
}

func TestGeneratePlist_SpecialCharactersInArgs(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	specialArgs := []string{
		"--flag=value with spaces",
		"arg with 'single quotes'",
		`arg with "double quotes"`,
		"arg-with-$dollar",
		"arg\twith\ttabs",
		"arg\nwith\nnewlines",
		"unicode-arg-éèê",
		"",             // empty argument
		"--flag=a=b=c", // multiple equals
	}

	def := &service.Definition{
		Name:    "special-chars-service",
		Command: execPath,
		Args:    specialArgs,
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	// ProgramArguments = command + args
	if len(parsed.ProgramArguments) != len(specialArgs)+1 {
		t.Fatalf(
			"ProgramArguments length = %d, want %d",
			len(parsed.ProgramArguments),
			len(specialArgs)+1,
		)
	}

	for i, arg := range specialArgs {
		got := parsed.ProgramArguments[i+1] // +1 for command
		if got != arg {
			t.Errorf("ProgramArguments[%d] = %q, want %q", i+1, got, arg)
		}
	}
}

func TestGeneratePlist_SpecialEnvironmentValues(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	env := map[string]string{
		"NORMAL":      "value",
		"WITH_SPACE":  "value with spaces",
		"WITH_EQUALS": "key=value",
		"EMPTY":       "",
		"WITH_PATH":   "/usr/local/bin:/usr/bin:/bin",
		"WITH_QUOTE":  `value "with" quotes`,
		"UNICODE":     "éèê",
	}

	def := &service.Definition{
		Name:        "env-special-service",
		Command:     execPath,
		Environment: env,
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	for k, v := range env {
		got, ok := parsed.EnvironmentVariables[k]
		if !ok {
			t.Errorf("Missing environment variable %q after round-trip", k)
		} else if got != v {
			t.Errorf("EnvironmentVariables[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestGeneratePlist_EmptyOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "testexec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	def := &service.Definition{
		Name:    "minimal-service",
		Command: execPath,
		// All optional fields left at zero values
	}

	data, err := GeneratePlist(def, "1.0.0")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}

	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}

	if parsed.WorkingDirectory != "" {
		t.Errorf("WorkingDirectory should be empty, got %q", parsed.WorkingDirectory)
	}
	if parsed.StandardOutPath != "" {
		t.Errorf("StandardOutPath should be empty, got %q", parsed.StandardOutPath)
	}
	if parsed.StandardErrorPath != "" {
		t.Errorf("StandardErrorPath should be empty, got %q", parsed.StandardErrorPath)
	}
	if parsed.RunAtLoad != false {
		t.Error("RunAtLoad should be false")
	}
	if parsed.KeepAlive != false {
		t.Error("KeepAlive should be false")
	}
}

func TestIsServicemanPlist(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "serviceman plist",
			data: []byte(`<?xml version="1.0"?>
<!-- Generated for serviceman -->
<plist version="1.0"><dict></dict></plist>`),
			want: true,
		},
		{
			name: "non-serviceman plist",
			data: []byte(`<?xml version="1.0"?>
<plist version="1.0"><dict></dict></plist>`),
			want: false,
		},
		{
			name: "t-man plist",
			data: []byte(`<?xml version="1.0"?>
<!-- Managed by t-man - DO NOT EDIT MANUALLY -->
<plist version="1.0"><dict></dict></plist>`),
			want: false,
		},
		{
			name: "empty data",
			data: []byte{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsServicemanPlist(tt.data)
			if got != tt.want {
				t.Errorf("IsServicemanPlist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddMarkerComment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool // whether output contains MarkerComment
	}{
		{
			name:  "standard XML",
			input: "<?xml version=\"1.0\"?>\n<plist></plist>",
			want:  true,
		},
		{
			name:  "empty input",
			input: "",
			want:  false,
		},
		{
			name:  "no XML declaration",
			input: "<plist></plist>",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addMarkerComment(tt.input)
			hasMarker := strings.Contains(result, MarkerComment)
			if hasMarker != tt.want {
				t.Errorf(
					"addMarkerComment() contains marker = %v, want %v\nresult: %s",
					hasMarker,
					tt.want,
					result,
				)
			}
		})
	}
}
