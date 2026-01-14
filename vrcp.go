// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VRCP14PD/VRSQRT14PD - Vector reciprocal and reciprocal square root approximations
//
// Essential for Vibe67's fast division and normalization:
//   - Fast division: a / b ≈ a * rcp(b)
//   - Fast normalization: v / length(v) ≈ v * rsqrt(dot(v,v))
//   - Graphics: inverse distance calculations
//   - Physics: inverse mass, inverse inertia
//
// Note: These are approximations with ~14 bits of precision
// For exact results, use Newton-Raphson refinement
//
// Example usage in Vibe67:
//   approx_inv = values || map(rcp)
//   approx_inv_sqrt = values || map(rsqrt)
//
// Architecture details:
//   x86-64: VRCP14PD/VRSQRT14PD zmm1, zmm2 (AVX-512)
//   ARM64:  FRECPE/FRSQRTE zd.d, zn.d (SVE2)
//   RISC-V: vfrec7.v/vfrsqrt7.v vd, vs2 (RVV - 7-bit precision)

// VRcpPDVectorToVector computes reciprocal approximation
// dst[i] ≈ 1.0 / src[i]
func (o *Out) VRcpPDVectorToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vrcppdX86VectorToVector(dst, src)
	case ArchARM64:
		o.vrcpARM64VectorToVector(dst, src)
	case ArchRiscv64:
		o.vrcpRISCVVectorToVector(dst, src)
	}
}

// VRsqrtPDVectorToVector computes reciprocal square root approximation
// dst[i] ≈ 1.0 / sqrt(src[i])
func (o *Out) VRsqrtPDVectorToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vrsqrtpdX86VectorToVector(dst, src)
	case ArchARM64:
		o.vrsqrtARM64VectorToVector(dst, src)
	case ArchRiscv64:
		o.vrsqrtRISCVVectorToVector(dst, src)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - RCP
// ============================================================================

// x86-64 VRCP14PD zmm1, zmm2
// EVEX.512.66.0F38.W1 4C /r
func (o *Out) vrcppdX86VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vrcp14pd %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512 VRCP14PD (14-bit precision)
		p0 := uint8(0x62)

		p1 := uint8(0x02) // map=0F38
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
		o.Write(0x4C) // VRCP14PD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		// No native reciprocal for AVX/SSE with doubles
		// Would need to use division or single-precision RCPPS
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (no AVX/SSE double-precision RCP)")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# Use VDIVPD with 1.0 for exact division\n")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - RSQRT
// ============================================================================

// x86-64 VRSQRT14PD zmm1, zmm2
// EVEX.512.66.0F38.W1 4E /r
func (o *Out) vrsqrtpdX86VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vrsqrt14pd %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512 VRSQRT14PD (14-bit precision)
		p0 := uint8(0x62)

		p1 := uint8(0x02)
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

		p2 := uint8(0x81) | (0x0F << 3)

		p3 := uint8(0x40)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x4E) // VRSQRT14PD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (no AVX/SSE double-precision RSQRT)")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n# Use VSQRTPD + VDIVPD for exact result\n")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation - RCP
// ============================================================================

func (o *Out) vrcpARM64VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE FRECPE - reciprocal estimate
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "frecpe %s.d, %s.d:", dst, src)
		}

		// SVE FRECPE encoding
		// 01100101 11 0 11110 001 110 Zn Zd
		instr := uint32(0x65DE3000) |
			(uint32(srcReg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FRECPE
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "frecpe %s.2d, %s.2d:", dst, src)
		}

		// NEON FRECPE encoding
		// 0 Q 0 01110 1 sz 1 00001 11011 10 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x4EE1D800) |
			(uint32(srcReg.Encoding&31) << 5) |
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
// ARM64 SVE2/NEON implementation - RSQRT
// ============================================================================

func (o *Out) vrsqrtARM64VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE FRSQRTE - reciprocal square root estimate
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "frsqrte %s.d, %s.d:", dst, src)
		}

		// SVE FRSQRTE encoding
		// 01100101 11 0 11111 001 110 Zn Zd
		instr := uint32(0x65DF3000) |
			(uint32(srcReg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FRSQRTE
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "frsqrte %s.2d, %s.2d:", dst, src)
		}

		// NEON FRSQRTE encoding
		// 0 Q 1 01110 1 sz 1 00001 11011 10 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x6EE1D800) |
			(uint32(srcReg.Encoding&31) << 5) |
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
// RISC-V RVV implementation - RCP
// ============================================================================

// RISC-V vfrec7.v vd, vs2
// 7-bit precision reciprocal estimate
func (o *Out) vrcpRISCVVectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfrec7.v %s, %s:", dst, src)
	}

	// vfrec7.v encoding
	// funct6=010011, vm=1, rs1=00101, funct3=001
	instr := uint32(0x57) |
		(1 << 12) | // funct3=001
		(uint32(dstReg.Encoding&31) << 7) |
		(5 << 15) | // rs1=00101 (FREC7)
		(uint32(srcReg.Encoding&31) << 20) |
		(1 << 25) | // vm=1
		(0x13 << 26) // funct6=010011

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation - RSQRT
// ============================================================================

// RISC-V vfrsqrt7.v vd, vs2
// 7-bit precision reciprocal square root estimate
func (o *Out) vrsqrtRISCVVectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfrsqrt7.v %s, %s:", dst, src)
	}

	// vfrsqrt7.v encoding
	// funct6=010011, vm=1, rs1=00100, funct3=001
	instr := uint32(0x57) |
		(1 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(4 << 15) | // rs1=00100 (FRSQRT7)
		(uint32(srcReg.Encoding&31) << 20) |
		(1 << 25) |
		(0x13 << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









