// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VZEROUPPER - Zero upper 128 bits of YMM/ZMM registers
//
// Essential for Vibe67's AVX ↔ SSE transition:
//   - Prevents AVX-SSE transition penalties on older CPUs
//   - Clears upper bits of vector registers before function calls
//   - Required when mixing AVX and non-AVX code
//   - Performance optimization for legacy compatibility
//
// Example usage in Vibe67:
//   // Before calling non-AVX function
//   vzeroupper()
//   call_external_function()
//
// Architecture details:
//   x86-64: VZEROUPPER (VEX.128.0F.WIG 77)
//   ARM64:  No equivalent (no AVX-SSE transition issue)
//   RISC-V: No equivalent (no legacy transition issue)
//
// Note: Not needed on modern Zen/Intel Core - kept for compatibility

// VZeroUpper zeros upper bits of vector registers
// Prevents AVX-SSE transition penalty
func (o *Out) VZeroUpper() {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vzeroupperX86()
	case ArchARM64:
		o.vzeroupperARM64()
	case ArchRiscv64:
		o.vzeroupperRISCV()
	}
}

// ============================================================================
// x86-64 VEX implementation
// ============================================================================

// x86-64 VZEROUPPER
// VEX.128.0F.WIG 77
func (o *Out) vzeroupperX86() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vzeroupper:")
	}

	// VEX prefix: C5 F8 77
	// C5 = 2-byte VEX
	// F8 = R=1, vvvv=1111, L=0, pp=00
	// 77 = opcode
	o.Write(0xC5)
	o.Write(0xF8)
	o.Write(0x77)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementation
// ============================================================================

func (o *Out) vzeroupperARM64() {
	// ARM64 SVE/NEON doesn't have AVX-SSE transition penalty
	// No equivalent instruction needed
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# ARM64: no vzeroupper needed (no AVX-SSE transition)\\n")
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementation
// ============================================================================

func (o *Out) vzeroupperRISCV() {
	// RISC-V RVV doesn't have AVX-SSE transition penalty
	// No equivalent instruction needed
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# RISC-V: no vzeroupper needed (no AVX-SSE transition)\\n")
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// VZeroAll zeros all vector registers
// More aggressive than VZEROUPPER - zeros entire ZMM registers
func (o *Out) VZeroAll() {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vzeroallX86()
	case ArchARM64:
		o.vzeroallARM64()
	case ArchRiscv64:
		o.vzeroallRISCV()
	}
}

// ============================================================================
// x86-64 VEX implementation - VZEROALL
// ============================================================================

// x86-64 VZEROALL
// VEX.256.0F.WIG 77
func (o *Out) vzeroallX86() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vzeroall:")
	}

	// VEX prefix: C5 FC 77
	// C5 = 2-byte VEX
	// FC = R=1, vvvv=1111, L=1, pp=00
	// 77 = opcode
	o.Write(0xC5)
	o.Write(0xFC)
	o.Write(0x77)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementation - VZEROALL
// ============================================================================

func (o *Out) vzeroallARM64() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# ARM64: no vzeroall needed\\n")
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementation - VZEROALL
// ============================================================================

func (o *Out) vzeroallRISCV() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# RISC-V: no vzeroall needed\\n")
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
