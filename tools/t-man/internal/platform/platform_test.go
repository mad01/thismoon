package platform

import (
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	p := Detect()

	if p == nil {
		t.Fatal("Detect() returned nil")
	}

	if p.OS == "" {
		t.Error("Detect() returned empty OS")
	}

	if p.Arch == "" {
		t.Error("Detect() returned empty Arch")
	}

	if p.Type == Unknown {
		t.Error("Detect() returned Unknown type")
	}

	// Verify OS matches runtime
	if p.OS != runtime.GOOS {
		t.Errorf("Detect() OS = %s, want %s", p.OS, runtime.GOOS)
	}

	// Verify Arch matches runtime
	if p.Arch != runtime.GOARCH {
		t.Errorf("Detect() Arch = %s, want %s", p.Arch, runtime.GOARCH)
	}
}

func TestPlatformType(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		wantType Type
	}{
		{"macOS", "darwin", Darwin},
		{"Linux", "linux", Linux},
		{"Windows", "windows", Windows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates the type mapping logic
			// Since we can't change runtime.GOOS in tests easily,
			// we just verify the constants are defined correctly
			if tt.wantType == "" {
				t.Errorf("Type constant for %s is empty", tt.name)
			}
		})
	}
}

func TestIsDarwin(t *testing.T) {
	result := IsDarwin()
	expected := runtime.GOOS == "darwin"

	if result != expected {
		t.Errorf("IsDarwin() = %v, want %v", result, expected)
	}
}

func TestIsLinux(t *testing.T) {
	result := IsLinux()
	expected := runtime.GOOS == "linux"

	if result != expected {
		t.Errorf("IsLinux() = %v, want %v", result, expected)
	}
}

func TestIsWindows(t *testing.T) {
	result := IsWindows()
	expected := runtime.GOOS == "windows"

	if result != expected {
		t.Errorf("IsWindows() = %v, want %v", result, expected)
	}
}

func TestIsSupported(t *testing.T) {
	result := IsSupported()
	expected := runtime.GOOS == "darwin"

	if result != expected {
		t.Errorf(
			"IsSupported() = %v, want %v (only macOS is currently supported)",
			result,
			expected,
		)
	}
}

func TestPlatformString(t *testing.T) {
	p := Detect()
	result := p.String()

	if result == "" {
		t.Error("Platform.String() returned empty string")
	}

	expected := runtime.GOOS + "/" + runtime.GOARCH
	if result != expected {
		t.Errorf("Platform.String() = %s, want %s", result, expected)
	}
}

func TestPlatformIsSupported(t *testing.T) {
	tests := []struct {
		name      string
		platform  *Platform
		supported bool
	}{
		{
			name: "Darwin platform",
			platform: &Platform{
				Type: Darwin,
				OS:   "darwin",
				Arch: "amd64",
			},
			supported: true,
		},
		{
			name: "Linux platform",
			platform: &Platform{
				Type: Linux,
				OS:   "linux",
				Arch: "amd64",
			},
			supported: false,
		},
		{
			name: "Windows platform",
			platform: &Platform{
				Type: Windows,
				OS:   "windows",
				Arch: "amd64",
			},
			supported: false,
		},
		{
			name: "Unknown platform",
			platform: &Platform{
				Type: Unknown,
				OS:   "unknown",
				Arch: "unknown",
			},
			supported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.platform.IsSupported()
			if result != tt.supported {
				t.Errorf("Platform.IsSupported() = %v, want %v", result, tt.supported)
			}
		})
	}
}

func TestTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		typeVal  Type
		expected string
	}{
		{"Darwin", Darwin, "darwin"},
		{"Linux", Linux, "linux"},
		{"Windows", Windows, "windows"},
		{"Unknown", Unknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.typeVal) != tt.expected {
				t.Errorf("Type %s = %s, want %s", tt.name, tt.typeVal, tt.expected)
			}
		})
	}
}
