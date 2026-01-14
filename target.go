// Completion: 100% - Utility module complete
package main

import "runtime"

// Target represents a compilation target (architecture + OS)
// This interface abstracts target-specific behavior, using GCC terminology
//
// ARCHITECTURE NOTE: Target = ISA + OS (e.g., arm64-darwin, x86_64-windows)
// - ISA determines registers and instructions
// - OS determines binary format and calling conventions
// Vibe67's unsafe blocks are ISA-based (3 blocks), not target-based (6+ blocks).
// See PLATFORM_ARCHITECTURE.md for the full design rationale.
type Target interface {
	// Architecture and OS (matching Go's GOARCH and GOOS)
	Arch() Arch
	OS() OS

	// String representations
	String() string     // Returns arch string (e.g., "aarch64")
	FullString() string // Returns full target string (e.g., "arm64-darwin")

	// Binary format detection
	IsMachO() bool // Returns true if target uses Mach-O format
	IsELF() bool   // Returns true if target uses ELF format
	IsPE() bool    // Returns true if target uses PE format
}

// TargetImpl is the concrete implementation of Target
type TargetImpl struct {
	arch Arch
	os   OS
}

// NewTarget creates a new Target instance for the given architecture and OS
func NewTarget(arch Arch, os OS) Target {
	return &TargetImpl{
		arch: arch,
		os:   os,
	}
}

// Arch returns the architecture
func (t *TargetImpl) Arch() Arch {
	return t.arch
}

// OS returns the operating system
func (t *TargetImpl) OS() OS {
	return t.os
}

// String returns a string representation like "aarch64" (just the arch for compatibility)
func (t *TargetImpl) String() string {
	return t.arch.String()
}

// FullString returns the full target string like "arm64-darwin"
func (t *TargetImpl) FullString() string {
	archStr := t.arch.String()
	// Convert aarch64 -> arm64 for cleaner output
	if t.arch == ArchARM64 {
		archStr = "arm64"
	} else if t.arch == ArchX86_64 {
		archStr = "amd64"
	}
	return archStr + "-" + t.os.String()
}

// IsMachO returns true if this target uses Mach-O format
func (t *TargetImpl) IsMachO() bool {
	return t.os == OSDarwin
}

// IsELF returns true if this target uses ELF format
func (t *TargetImpl) IsELF() bool {
	return t.os == OSLinux || t.os == OSFreeBSD
}

// IsPE returns true if this target uses PE format
func (t *TargetImpl) IsPE() bool {
	return t.os == OSWindows
}

// GetDefaultTarget returns the target for the current runtime
func GetDefaultTarget() Target {
	var arch Arch
	switch runtime.GOARCH {
	case "amd64":
		arch = ArchX86_64
	case "arm64":
		arch = ArchARM64
	case "riscv64":
		arch = ArchRiscv64
	default:
		arch = ArchX86_64 // fallback
	}

	var os OS
	switch runtime.GOOS {
	case "linux":
		os = OSLinux
	case "darwin":
		os = OSDarwin
	case "freebsd":
		os = OSFreeBSD
	default:
		os = OSLinux // fallback
	}

	return NewTarget(arch, os)
}

// GetELFMachineType returns the ELF machine type constant for a given architecture
func GetELFMachineType(arch Arch) uint16 {
	switch arch {
	case ArchX86_64:
		return 0x3e // AMD x86-64
	case ArchARM64:
		return 0xB7 // ARM64
	case ArchRiscv64:
		return 0xF3 // RISC-V
	default:
		return 0
	}
}

// PlatformToTarget converts a Platform struct to a Target interface
func PlatformToTarget(p Platform) Target {
	return NewTarget(p.Arch, p.OS)
}

// GetVectorWidth returns the SIMD vector width in bits for the target architecture
func (t *TargetImpl) GetVectorWidth() int {
	switch t.arch {
	case ArchX86_64:
		// x86-64 supports multiple widths: SSE (128), AVX (256), AVX-512 (512)
		// Use AVX-512 if explicitly enabled, otherwise use AVX/AVX2
		if EnableAVX512 {
			return 512 // AVX-512: 8 doubles per vector (zmm registers)
		}
		return 256 // AVX/AVX2: 4 doubles per vector (ymm registers)
	case ArchARM64:
		// ARM64 NEON uses 128-bit vectors
		return 128
	case ArchRiscv64:
		// RISC-V vector extension has variable width, use conservative 128-bit
		return 128
	default:
		return 128 // Conservative default
	}
}

// GetVectorWidthBytes returns the SIMD vector width in bytes
func (t *TargetImpl) GetVectorWidthBytes() int {
	return t.GetVectorWidth() / 8
}

// GetVectorLaneCount returns number of float64 elements that fit in a SIMD vector
func (t *TargetImpl) GetVectorLaneCount() int {
	// float64 is 8 bytes (64 bits)
	return t.GetVectorWidth() / 64
}

// SupportsAVX returns true if the architecture supports AVX instructions
func (t *TargetImpl) SupportsAVX() bool {
	return t.arch == ArchX86_64
}

// SupportsNEON returns true if the architecture supports NEON instructions
func (t *TargetImpl) SupportsNEON() bool {
	return t.arch == ArchARM64
}









