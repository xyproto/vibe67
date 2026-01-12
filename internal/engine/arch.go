// Completion: 100% - Utility module complete
package engine

import (
	"fmt"
	"strings"
)

// Architecture type
type Arch int

const (
	ArchUnknown Arch = iota
	ArchX86_64
	ArchARM64
	ArchRiscv64
)

func (a Arch) String() string {
	switch a {
	case ArchX86_64:
		return "x86_64"
	case ArchARM64:
		return "aarch64"
	case ArchRiscv64:
		return "riscv64"
	case ArchUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseArch parses an architecture string (like GOARCH values)
func ParseArch(s string) (Arch, error) {
	switch strings.ToLower(s) {
	case "x86_64", "amd64", "x86-64":
		return ArchX86_64, nil
	case "aarch64", "arm64":
		return ArchARM64, nil
	case "riscv64", "riscv", "rv64":
		return ArchRiscv64, nil
	default:
		return 0, fmt.Errorf("unsupported architecture: %s (supported: amd64, arm64, riscv64)", s)
	}
}

// OS type
type OS int

const (
	OSLinux OS = iota
	OSDarwin
	OSFreeBSD
	OSWindows
)

func (o OS) String() string {
	switch o {
	case OSLinux:
		return "linux"
	case OSDarwin:
		return "darwin"
	case OSFreeBSD:
		return "freebsd"
	case OSWindows:
		return "windows"
	default:
		return "unknown"
	}
}

// ParseOS parses an OS string (like GOOS values)
func ParseOS(s string) (OS, error) {
	switch strings.ToLower(s) {
	case "linux":
		return OSLinux, nil
	case "darwin", "macos":
		return OSDarwin, nil
	case "freebsd":
		return OSFreeBSD, nil
	case "windows", "win", "wine":
		return OSWindows, nil
	default:
		return 0, fmt.Errorf("unsupported OS: %s (supported: linux, darwin, freebsd, windows)", s)
	}
}

// Platform represents a target platform (architecture + OS)
type Platform struct {
	Arch Arch
	OS   OS
}

// String returns a human-readable platform string
func (p Platform) String() string {
	return fmt.Sprintf("%s-%s", p.Arch, p.OS)
}

// FullString returns a detailed platform string
func (p Platform) FullString() string {
	return fmt.Sprintf("%s on %s", p.Arch, p.OS)
}
