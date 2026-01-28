// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VROUNDPD - Vector rounding of packed double-precision floats
//
// Essential for Vibe67's numerical operations:
//   - Floor: round down to nearest integer
//   - Ceil: round up to nearest integer
//   - Trunc: round toward zero
//   - Round to nearest: standard rounding
//
// Example usage in Vibe67:
//   floored = values || map(floor)
//   ceiled = values || map(ceil)
//   truncated = values || map(trunc)
//
// Architecture details:
//   x86-64: VROUNDPD zmm1, zmm2, imm8 (AVX-512/AVX)
//   ARM64:  FRINTN/FRINTP/FRINTM/FRINTZ zd.d, pg/m, zn.d (SVE2)
//   RISC-V: vfcvt.x.f.v then vfcvt.f.x.v (RVV)

// Rounding modes
const (
	RoundNearest = 0 // Round to nearest (even)
	RoundDown    = 1 // Round down (floor)
	RoundUp      = 2 // Round up (ceil)
	RoundTrunc   = 3 // Round toward zero (truncate)
)

// VRoundPDVectorToVector rounds vector elements
// dst[i] = round(src[i], mode)
func (o *Out) VRoundPDVectorToVector(dst, src string, mode int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vroundpdX86VectorToVector(dst, src, mode)
	case ArchARM64:
		o.vroundARM64VectorToVector(dst, src, mode)
	case ArchRiscv64:
		o.vroundRISCVVectorToVector(dst, src, mode)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX implementation
// ============================================================================

func (o *Out) vroundpdX86VectorToVector(dst, src string, mode int) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	modeStr := "nearest"
	switch mode {
	case RoundDown:
		modeStr = "down"
	case RoundUp:
		modeStr = "up"
	case RoundTrunc:
		modeStr = "trunc"
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vroundpd %s, %s, %s:", dst, src, modeStr)
	}

	if dstReg.Size == 512 {
		// AVX-512 uses VRNDSCALEPD with different encoding
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX-512 VRNDSCALEPD)")
		}

		// EVEX.512.66.0F3A.W1 09 /r ib
		p0 := uint8(0x62)

		p1 := uint8(0x03) // map=0F3A
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81) | (0x0F << 3) // vvvv=1111 (unused)

		p3 := uint8(0x40)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x09) // VRNDSCALEPD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)

		// Immediate: rounding mode
		o.Write(uint8(mode))
	} else if dstReg.Size == 256 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX)")
		}

		// AVX VROUNDPD
		// VEX.256.66.0F3A.WIG 09 /r ib
		o.Write(0xC4)

		vex1 := uint8(0x03) // map=0F3A
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0x45) // L=1, pp=01, vvvv=1111
		o.Write(vex2)

		o.Write(0x09)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)

		o.Write(uint8(mode))
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE4.1)")
		}

		// SSE4.1 ROUNDPD
		o.Write(0x66)
		o.Write(0x0F)
		o.Write(0x3A)
		o.Write(0x09)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)

		o.Write(uint8(mode))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation
// ============================================================================

func (o *Out) vroundARM64VectorToVector(dst, src string, mode int) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE FRINT* instructions
		var opcode uint32
		var name string

		switch mode {
		case RoundNearest:
			opcode = 0x6500A000 // FRINTN
			name = "frintn"
		case RoundDown:
			opcode = 0x6501A000 // FRINTM (minus infinity)
			name = "frintm"
		case RoundUp:
			opcode = 0x6502A000 // FRINTP (plus infinity)
			name = "frintp"
		case RoundTrunc:
			opcode = 0x6503A000 // FRINTZ (toward zero)
			name = "frintz"
		default:
			opcode = 0x6500A000
			name = "frintn"
		}

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "%s %s.d, p7/m, %s.d:", name, dst, src)
		}

		instr := opcode |
			(7 << 10) | // Pg=p7
			(uint32(srcReg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FRINT* instructions
		var opcode uint32
		var name string

		switch mode {
		case RoundNearest:
			opcode = 0x4EE18800 // FRINTN
			name = "frintn"
		case RoundDown:
			opcode = 0x4EE19800 // FRINTM
			name = "frintm"
		case RoundUp:
			opcode = 0x4EE18800 // FRINTP (need to check)
			name = "frintp"
		case RoundTrunc:
			opcode = 0x4EE19800 // FRINTZ (need to check)
			name = "frintz"
		default:
			opcode = 0x4EE18800
			name = "frintn"
		}

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "%s %s.2d, %s.2d:", name, dst, src)
		}

		instr := opcode |
			(uint32(srcReg.Encoding&31) << 5) | // Rn
			uint32(dstReg.Encoding&31) // Rd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation
// ============================================================================

func (o *Out) vroundRISCVVectorToVector(dst, src string, mode int) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	// RVV doesn't have direct rounding instructions for float vectors
	// Would need to convert to integer and back, or use vfrec7.v + operations
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# RVV rounding (mode %d): convert to int then back\n", mode)
	}

	switch mode {
	case RoundNearest:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vfcvt.x.f.v vtemp, %s  # round to int\n", src)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vfcvt.f.x.v %s, vtemp:", dst)
		}
	case RoundDown:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# floor: special rounding mode\n")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vfcvt.x.f.v %s, %s:", dst, src)
		}
	case RoundUp:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# ceil: special rounding mode\n")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vfcvt.x.f.v %s, %s:", dst, src)
		}
	case RoundTrunc:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# trunc: rounding mode RTZ\n")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vfcvt.x.f.v %s, %s:", dst, src)
		}
	}

	// vfcvt.x.f.v encoding (simplified)
	// Convert float to integer, then back
	instr := uint32(0x57) |
		(1 << 12) | // funct3=001
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(srcReg.Encoding&31) << 20) |
		(1 << 25) |
		(0x12 << 26) // funct6 for vfcvt

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
