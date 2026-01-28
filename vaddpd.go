// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VADDPD - Vector addition for packed double-precision (float64) values
// Essential for Vibe67's map operations over collections:
//   - Vectorized arithmetic: values |> map(x -> x + offset)
//   - Parallel addition: [a, b, c, d] + [e, f, g, h]
//   - Pipeline operations with SIMD acceleration
//
// Architecture details:
//   x86-64: VADDPD zmm1, zmm2, zmm3 (AVX-512: 8x float64)
//   ARM64:  FADD z0.d, p0/m, z1.d, z2.d (SVE2: scalable float64)
//   RISC-V: vfadd.vv v1, v2, v3 (RVV: scalable float64)

// VAddPDVectorToVector performs vector addition: dst = src1 + src2
// All three operands are vector registers
func (o *Out) VAddPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vaddpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vaddpdARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vaddpdRISCVVectorToVector(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation
// ============================================================================

// x86-64 VADDPD zmm, zmm, zmm (AVX-512)
// EVEX.NDS.512.66.0F.W1 58 /r
func (o *Out) vaddpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	// Verify they are ZMM registers (or YMM/XMM for fallback)
	if dstReg.Size != 512 && dstReg.Size != 256 && dstReg.Size != 128 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Error: %s is not a vector register\n", dst)
		}
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vaddpd %s, %s, %s:", dst, src1, src2)
	}

	// EVEX encoding for AVX-512 is complex (4-byte prefix)
	// Format: [EVEX P0] [EVEX P1] [EVEX P2] [EVEX P3] [opcode] [ModR/M]

	// Simplified EVEX prefix for VADDPD zmm1, zmm2, zmm3
	// P0: 0x62 (EVEX marker)
	// P1: Encodes R, X, B, R', map_select
	// P2: Encodes W, vvvv, pp
	// P3: Encodes z, L'L, b, V', aaa

	// For ZMM registers (512-bit)
	if dstReg.Size == 512 {
		// EVEX prefix
		p0 := uint8(0x62) // EVEX prefix byte 0

		// P1: [R X B R' 0 0 m m]
		// R: ~dst[3] (inverted bit 3 of dst encoding)
		// X: 1 (no index)
		// B: ~src2[3] (inverted bit 3 of src2/ModRM.r/m)
		// R': ~dst[4] (inverted bit 4 of dst - for zmm16-31)
		// mm: 01 (0F opcode map)
		p1 := uint8(0x01) // mm = 01 for 0F map
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80 // R
		}
		p1 |= 0x40 // X
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20 // B
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10 // R'
		}

		// P2: [W vvvv 1 pp]
		// W: 1 (64-bit element size for float64)
		// vvvv: ~src1 encoding (inverted bits 0-3)
		// pp: 01 (66 prefix)
		p2 := uint8(0x81)                            // W=1, pp=01
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3) // vvvv

		// P3: [z L'L b V' aaa]
		// z: 0 (no zeroing)
		// L'L: 10 (512-bit vector length)
		// b: 0 (no broadcast)
		// V': ~src1[4] (inverted bit 4 of src1)
		// aaa: 000 (no masking)
		p3 := uint8(0x40) // L'L = 10 for 512-bit
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0x58 (VADDPD)
		o.Write(0x58)

		// ModR/M: 11 dst src2
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX2 VADDPD ymm, ymm, ymm fallback
		// VEX.NDS.256.66.0F.WIG 58 /r
		// Simplified VEX encoding
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2 256-bit)")
		}

		// VEX 3-byte prefix
		// C4 [RXB m-mmmm] [W vvvv L pp] [opcode] [ModR/M]
		o.Write(0xC4) // VEX 3-byte marker

		// Byte 1: ~RXB 00001 (map_select = 1 for 0F)
		vex1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80 // ~R
		}
		vex1 |= 0x40 // ~X
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20 // ~B
		}
		o.Write(vex1)

		// Byte 2: W ~vvvv L pp
		// W=0 (ignored for VADDPD)
		// vvvv=~src1
		// L=1 (256-bit)
		// pp=01 (66 prefix)
		vex2 := uint8(0x45) // L=1, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		// Opcode
		o.Write(0x58)

		// ModR/M
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else { // XMM (128-bit)
		// SSE2 ADDPD xmm, xmm fallback
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2 128-bit)")
		}

		// 66 0F 58 /r
		o.Write(0x66) // Operand-size prefix

		// REX prefix if needed for high registers
		if (dstReg.Encoding&8) != 0 || (src2Reg.Encoding&8) != 0 {
			rex := uint8(0x40)
			if (dstReg.Encoding & 8) != 0 {
				rex |= 0x04 // REX.R
			}
			if (src2Reg.Encoding & 8) != 0 {
				rex |= 0x01 // REX.B
			}
			o.Write(rex)
		}

		o.Write(0x0F)
		o.Write(0x58)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2 implementation
// ============================================================================

// ARM64 FADD Zd.D, Pg/M, Zn.D, Zm.D (SVE2)
// Predicated vector addition with merge
func (o *Out) vaddpdARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	// Check if SVE Z registers or NEON V registers
	if dstReg.Size == 512 {
		// SVE2 FADD z0.d, p0/m, z1.d, z2.d
		// Using predicate p7 as "all true" by convention
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fadd %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE FADD encoding
		// 01100101 11 Zm[4:0] 000 Pg[2:0] Zn[4:0] Zd[4:0]
		// size=11 (double), opc=000
		instr := uint32(0x65000000) |
			(3 << 22) | // size = 11 (64-bit double)
			(uint32(src2Reg.Encoding&31) << 16) | // Zm
			(7 << 10) | // Pg = p7 (all true)
			(uint32(src1Reg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FADD v0.2d, v1.2d, v2.2d (128-bit)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fadd %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}

		// NEON FADD encoding
		// 0 Q 0 01110 1 sz 1 Rm 11010 1 Rn Rd
		// Q=1 (128-bit), sz=1 (double)
		instr := uint32(0x4E601400) |
			(uint32(src2Reg.Encoding&31) << 16) | // Rm
			(uint32(src1Reg.Encoding&31) << 5) | // Rn
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

// RISC-V vfadd.vv vd, vs2, vs1 (RVV)
// Vector-vector floating-point addition
func (o *Out) vaddpdRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfadd.vv %s, %s, %s:", dst, src2, src1)
	}

	// RVV vfadd.vv encoding
	// funct6 vm vs2 vs1 funct3 vd opcode
	// 000000 1 vs2[4:0] vs1[4:0] 001 vd[4:0] 1010111
	// funct6=000000, vm=1 (unmasked), funct3=001 (OPFVV), opcode=1010111 (V)

	instr := uint32(0x57) | // opcode = 1010111
		(1 << 12) | // funct3 = 001 (OPFVV)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(uint32(src1Reg.Encoding&31) << 15) | // vs1
		(uint32(src2Reg.Encoding&31) << 20) | // vs2
		(1 << 25) // vm = 1 (unmasked)
		// funct6 = 000000 (VFADD)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
