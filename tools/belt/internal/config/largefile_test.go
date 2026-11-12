package config

import (
	"strings"
	"testing"
)

// TestLargeFileThreshold pins large_file_lines: unset and 0 mean the
// default, a positive value replaces it, and a negative one is a config
// error rather than a threshold that refuses or advises on every read.
func TestLargeFileThreshold(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name, yaml string
		want       int
		wantErr    string
	}{
		{"unset", "guards: {}\n", DefaultLargeFileLines, ""},
		{"zero", "large_file_lines: 0\n", DefaultLargeFileLines, ""},
		{"set", "large_file_lines: 120\n", 120, ""},
		{"negative", "large_file_lines: -5\n", DefaultLargeFileLines, "large_file_lines is -5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFrom(Paths{BeltYAML: writeFile(t, dir, tt.name+".yaml", tt.yaml)})
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("LoadFrom: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("LoadFrom error = %v, want one containing %q", err, tt.wantErr)
			}
			if got := cfg.LargeFileThreshold(); got != tt.want {
				t.Errorf("LargeFileThreshold() = %d, want %d", got, tt.want)
			}
		})
	}
}
