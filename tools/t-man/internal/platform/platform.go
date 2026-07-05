package platform

import (
	"fmt"
	"runtime"
)

// Type represents the platform type
type Type string

const (
	// Darwin represents macOS platform
	Darwin Type = "darwin"
	// Linux represents Linux platform
	Linux Type = "linux"
	// Windows represents Windows platform
	Windows Type = "windows"
	// Unknown represents an unknown platform
	Unknown Type = "unknown"
)

// Platform provides platform-specific information
type Platform struct {
	Type    Type
	OS      string
	Arch    string
	Version string
}

// Detect returns the current platform information
func Detect() *Platform {
	p := &Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch runtime.GOOS {
	case "darwin":
		p.Type = Darwin
	case "linux":
		p.Type = Linux
	case "windows":
		p.Type = Windows
	default:
		p.Type = Unknown
	}

	return p
}

// IsDarwin returns true if the current platform is macOS
func IsDarwin() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if the current platform is Linux
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// IsWindows returns true if the current platform is Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsSupported returns true if the platform is supported
func IsSupported() bool {
	return IsDarwin() // Currently only macOS is supported
}

// String returns a string representation of the platform
func (p *Platform) String() string {
	return fmt.Sprintf("%s/%s", p.OS, p.Arch)
}

// IsSupported returns true if this platform is supported
func (p *Platform) IsSupported() bool {
	return p.Type == Darwin
}
