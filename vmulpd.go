// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VMULPD - Vector multiplication for packed double-precision (float64) values
// Essential for Vibe67's map operations over collections:
//   - Vectorized scaling: values |> map(x -> x * scale_factor)
//   - Parallel multiplication: [a, b, c, d] * [e, f, g, h]
//   - Matrix operations and dot products
//
// Architecture details:
//   x86-64: VMULPD zmm1, zmm2, zmm3 (AVX-512: 8x float64)
//   ARM64:  FMUL z0.d, p0/m, z1.d, z2.d (SVE2: scalable float64)
//   RISC-V: vfmul.vv v1, v2, v3 (RVV: scalable float64)

// VMulPDVectorToVector performs vector multiplication: dst = src1 * src2
// All three operands are vector registers
func (o *Out) VMulPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vmulpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vmulpdARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vmulpdRISCVVectorToVector(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation
// ============================================================================

// x86-64 VMULPD zmm, zmm, zmm (AVX-512)
// EVEX.NDS.512.66.0F.W1 59 /r
func (o *Out) vmulpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size != 512 && dstReg.Size != 256 && dstReg.Size != 128 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Error: %s is not a vector register\n", dst)
		}
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmulpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VMULPD zmm, zmm, zmm
		// EVEX prefix (same structure as VADDPD but with opcode 0x59)
		p0 := uint8(0x62) // EVEX prefix byte 0

		// P1: [R X B R' 0 0 m m]
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
		p2 := uint8(0x81)                            // W=1, pp=01
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3) // vvvv

		// P3: [z L'L b V' aaa]
		p3 := uint8(0x40) // L'L = 10 for 512-bit
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0x59 (VMULPD)
		o.Write(0x59)

		// ModR/M: 11 dst src2
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX2 VMULPD ymm, ymm, ymm
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2 256-bit)")
		}

		// VEX 3-byte prefix: C4 [RXB m-mmmm] [W vvvv L pp]
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

		vex2 := uint8(0x45) // L=1, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		o.Write(0x59) // VMULPD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		// SSE2 MULPD xmm, xmm
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2 128-bit)")
		}

		o.Write(0x66) // Operand-size prefix

		if (dstReg.Encoding&8) != 0 || (src2Reg.Encoding&8) != 0 {
			rex := uint8(0x40)
			if (dstReg.Encoding & 8) != 0 {
				rex |= 0x04
			}
			if (src2Reg.Encoding & 8) != 0 {
				rex |= 0x01
			}
			o.Write(rex)
		}

		o.Write(0x0F)
		o.Write(0x59) // MULPD opcode

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

// ARM64 FMUL Zd.D, Pg/M, Zn.D, Zm.D (SVE2)
func (o *Out) vmulpdARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE2 FMUL z0.d, p7/m, z1.d, z2.d
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmul %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE FMUL encoding
		// 01100101 11 Zm[4:0] 001 Pg[2:0] Zn[4:0] Zd[4:0]
		// size=11 (double), opc=001 (MUL)
		instr := uint32(0x65000000) |
			(3 << 22) | // size = 11
			(1 << 18) | // opc[0] = 1 for MUL
			(uint32(src2Reg.Encoding&31) << 16) | // Zm
			(7 << 10) | // Pg = p7
			(uint32(src1Reg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FMUL v0.2d, v1.2d, v2.2d
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmul %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}

		// NEON FMUL encoding
		// 0 Q 1 01110 1 sz 1 Rm 11011 1 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x6E601C00) |
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

// RISC-V vfmul.vv vd, vs2, vs1 (RVV)
func (o *Out) vmulpdRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmul.vv %s, %s, %s:", dst, src2, src1)
	}

	// RVV vfmul.vv encoding
	// funct6 vm vs2 vs1 funct3 vd opcode
	// 100100 1 vs2[4:0] vs1[4:0] 001 vd[4:0] 1010111
	// funct6=100100 (VFMUL)

	instr := uint32(0x57) | // opcode
		(1 << 12) | // funct3 = 001 (OPFVV)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(uint32(src1Reg.Encoding&31) << 15) | // vs1
		(uint32(src2Reg.Encoding&31) << 20) | // vs2
		(1 << 25) | // vm = 1 (unmasked)
		(0x24 << 26) // funct6 = 100100

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









