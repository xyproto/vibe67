// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VMINPD/VMAXPD - Vector minimum/maximum of packed double-precision floats
//
// Essential for Vibe67's numerical operations:
//   - Reductions: find min/max in array
//   - Clamping: clamp values to range [min, max]
//   - Comparisons: element-wise min/max selection
//   - Statistics: finding extrema
//
// Example usage in Vibe67:
//   maximum = values ||> max    // reduction
//   clamped = values || map(x -> min(max(x, lower), upper))
//
// Architecture details:
//   x86-64: VMINPD/VMAXPD zmm1, zmm2, zmm3 (AVX-512/AVX/SSE2)
//   ARM64:  FMIN/FMAX zd.d, pg/m, zn.d, zm.d (SVE2)
//   RISC-V: vfmin.vv/vfmax.vv vd, vs2, vs1 (RVV)

// VMinPDVectorToVector computes element-wise minimum
// dst[i] = min(src1[i], src2[i])
func (o *Out) VMinPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vminpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vminARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vminRISCVVectorToVector(dst, src1, src2)
	}
}

// VMaxPDVectorToVector computes element-wise maximum
// dst[i] = max(src1[i], src2[i])
func (o *Out) VMaxPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vmaxpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vmaxARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vmaxRISCVVectorToVector(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation - MIN
// ============================================================================

// x86-64 VMINPD zmm1, zmm2, zmm3
// EVEX.NDS.512.66.0F.W1 5D /r
func (o *Out) vminpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vminpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VMINPD
		p0 := uint8(0x62)

		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		p3 := uint8(0x40)
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x5D)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX)")
		}
		o.Write(0xC4)

		vex1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0xC5)
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)
		o.Write(0x5D)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		rex := uint8(0x40)
		if (dstReg.Encoding & 8) != 0 {
			rex |= 0x04
		}
		if (src2Reg.Encoding & 8) != 0 {
			rex |= 0x01
		}
		if rex != 0x40 {
			o.Write(rex)
		}
		o.Write(0x0F)
		o.Write(0x5D)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation - MAX
// ============================================================================

// x86-64 VMAXPD zmm1, zmm2, zmm3
// EVEX.NDS.512.66.0F.W1 5F /r
func (o *Out) vmaxpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmaxpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VMAXPD
		p0 := uint8(0x62)

		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		p3 := uint8(0x40)
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x5F) // VMAXPD opcode (only difference from VMINPD)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX)")
		}
		o.Write(0xC4)

		vex1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0xC5)
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)
		o.Write(0x5F)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		rex := uint8(0x40)
		if (dstReg.Encoding & 8) != 0 {
			rex |= 0x04
		}
		if (src2Reg.Encoding & 8) != 0 {
			rex |= 0x01
		}
		if rex != 0x40 {
			o.Write(rex)
		}
		o.Write(0x0F)
		o.Write(0x5F)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation - MIN
// ============================================================================

func (o *Out) vminARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE FMIN
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmin %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE FMIN encoding
		// 01100101 11 0 Zm 100 Pg 0 Zn Zd
		instr := uint32(0x65006000) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(7 << 10) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FMIN
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmin %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}

		// NEON FMIN encoding
		// 0 Q 1 01110 1 sz 1 Rm 11 110 1 Rn Rd
		instr := uint32(0x6EE0F400) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

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
// ARM64 SVE2/NEON implementation - MAX
// ============================================================================

func (o *Out) vmaxARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE FMAX
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmax %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE FMAX encoding
		// 01100101 11 0 Zm 011 Pg 0 Zn Zd
		instr := uint32(0x65004000) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(7 << 10) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FMAX
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmax %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}

		// NEON FMAX encoding
		// 0 Q 1 01110 1 sz 1 Rm 11 111 1 Rn Rd
		instr := uint32(0x6EE0FC00) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

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
// RISC-V RVV implementation - MIN
// ============================================================================

func (o *Out) vminRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmin.vv %s, %s, %s:", dst, src1, src2)
	}

	// vfmin.vv encoding
	// funct6 vm vs2 vs1 funct3 vd opcode
	// funct6=000100, funct3=001 (OPFVV)
	instr := uint32(0x57) |
		(1 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x04 << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation - MAX
// ============================================================================

func (o *Out) vmaxRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmax.vv %s, %s, %s:", dst, src1, src2)
	}

	// vfmax.vv encoding
	// funct6=000110, funct3=001 (OPFVV)
	instr := uint32(0x57) |
		(1 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x06 << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









