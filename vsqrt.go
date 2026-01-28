// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VSQRTPD - Vector square root of packed double-precision floats
//
// Essential for Vibe67's numerical operations:
//   - Distance calculations: sqrt(dx² + dy²)
//   - Standard deviation: sqrt(variance)
//   - Normalization: vector / sqrt(dot(vector, vector))
//   - Physics simulations: many formulas require sqrt
//
// Example usage in Vibe67:
//   dist_sq = dxs *+ dxs + (dys *+ dys)
//   distances = dist_sq || sqrt
//
// Architecture details:
//   x86-64: VSQRTPD zmm1, zmm2 (AVX-512/AVX/SSE2)
//   ARM64:  FSQRT zd.d, pg/m, zn.d (SVE2), FSQRT vd.2d, vn.2d (NEON)
//   RISC-V: vfsqrt.v vd, vs2 (RVV)

// VSqrtPDVectorToVector computes square root of all elements
// dst[i] = sqrt(src[i])
func (o *Out) VSqrtPDVectorToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vsqrtpdX86VectorToVector(dst, src)
	case ArchARM64:
		o.vsqrtARM64VectorToVector(dst, src)
	case ArchRiscv64:
		o.vsqrtRISCVVectorToVector(dst, src)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation
// ============================================================================

// x86-64 VSQRTPD zmm1, zmm2
// EVEX.512.66.0F.W1 51 /r
func (o *Out) vsqrtpdX86VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vsqrtpd %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512 VSQRTPD
		p0 := uint8(0x62)

		// P1: map_select = 01 (0F)
		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80 // R
		}
		p1 |= 0x40 // X
		if (srcReg.Encoding & 8) == 0 {
			p1 |= 0x20 // B
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10 // R'
		}

		// P2: W=1, vvvv=1111 (unused for unary op), pp=01
		p2 := uint8(0x81) | (0x0F << 3)

		// P3: L'L=10 (512-bit)
		p3 := uint8(0x40)
		if (srcReg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0x51 (VSQRTPD)
		o.Write(0x51)

		// ModR/M
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX VSQRTPD ymm, ymm
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX)")
		}

		o.Write(0xC4)

		vex1 := uint8(0x01) // map=0F
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0xC5) // W=1, L=1, pp=01, vvvv=1111
		o.Write(vex2)

		o.Write(0x51)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		// SSE2 SQRTPD xmm, xmm
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}

		o.Write(0x66) // prefix
		rex := uint8(0x40)
		if (dstReg.Encoding & 8) != 0 {
			rex |= 0x04
		}
		if (srcReg.Encoding & 8) != 0 {
			rex |= 0x01
		}
		if rex != 0x40 {
			o.Write(rex)
		}
		o.Write(0x0F)
		o.Write(0x51)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation
// ============================================================================

// ARM64 FSQRT zd.d, pg/m, zn.d (SVE2) or FSQRT vd.2d, vn.2d (NEON)
func (o *Out) vsqrtARM64VectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE FSQRT
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fsqrt %s.d, p7/m, %s.d:", dst, src)
		}

		// SVE FSQRT encoding
		// 01100101 11 01 101 101 Pg Zn Zd
		// size=11 (double), opc=01101 (FSQRT)
		instr := uint32(0x651D8000) |
			(7 << 10) | // Pg=p7
			(uint32(srcReg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FSQRT
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fsqrt %s.2d, %s.2d:", dst, src)
		}

		// NEON FSQRT encoding
		// 0 Q 1 01110 1 sz 1 00001 11111 0 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x6EE1F800) |
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

// RISC-V vfsqrt.v vd, vs2
func (o *Out) vsqrtRISCVVectorToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfsqrt.v %s, %s:", dst, src)
	}

	// vfsqrt.v encoding
	// funct6 vm vs2=00000 rs1(sqrt) funct3 vd opcode
	// funct6=100011, funct3=001 (OPFVV), rs1=00000 for sqrt
	instr := uint32(0x57) | // opcode=1010111 (OP-V)
		(1 << 12) | // funct3=001 (OPFVV)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(0 << 15) | // rs1=00000 (SQRT operation)
		(uint32(srcReg.Encoding&31) << 20) | // vs2
		(1 << 25) | // vm=1 (unmasked)
		(0x23 << 26) // funct6=100011 (VFSQRT)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
