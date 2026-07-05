package version

import (
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	result := Short()
	if result == "" {
		t.Error("Short() returned empty string")
	}
	if result != Version {
		t.Errorf("Short() = %s, want %s", result, Version)
	}
}

func TestFull(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		gitCommit string
		want      string
	}{
		{
			name:      "with git commit",
			version:   "1.0.0",
			gitCommit: "abc123",
			want:      "1.0.0-abc123",
		},
		{
			name:      "without git commit",
			version:   "1.0.0",
			gitCommit: "unknown",
			want:      "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original values
			origVersion := Version
			origGitCommit := GitCommit
			defer func() {
				Version = origVersion
				GitCommit = origGitCommit
			}()

			Version = tt.version
			GitCommit = tt.gitCommit

			got := Full()
			if got != tt.want {
				t.Errorf("Full() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestInfo(t *testing.T) {
	result := Info()
	if result == "" {
		t.Error("Info() returned empty string")
	}

	// Check that all expected fields are present
	expectedFields := []string{
		"t-man version",
		"Git commit:",
		"Build date:",
		"Go version:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(result, field) {
			t.Errorf("Info() missing expected field: %s", field)
		}
	}

	// Check that version value is present
	if !strings.Contains(result, Version) {
		t.Errorf("Info() missing version value: %s", Version)
	}
}

func TestVersionConstants(t *testing.T) {
	if Version == "" {
		t.Error("Version constant is empty")
	}
	if GitCommit == "" {
		t.Error("GitCommit constant is empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate constant is empty")
	}
	if GoVersion == "" {
		t.Error("GoVersion constant is empty")
	}
}
