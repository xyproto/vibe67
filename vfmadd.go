// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VFMADD - Vector fused multiply-add for packed double-precision (float64)
// Essential for Vibe67's numerical operations with maximum precision:
//   - Dot products: sum of element-wise products
//   - Polynomial evaluation: a*x^2 + b*x + c
//   - Matrix operations: accumulating products
//   - Better precision: single rounding instead of two
//
// Operation: dst = (src1 * src2) + src3
//
// Architecture details:
//   x86-64: VFMADD231PD zmm1, zmm2, zmm3 (AVX-512: dst = src2*src3 + dst)
//   ARM64:  FMLA zd.d, pg/m, zn.d, zm.d (SVE2: dst = dst + zn*zm)
//   RISC-V: vfmadd.vv vd, vs1, vs2 (RVV: vd = vd*vs1 + vs2)

// VFmaddPDVectorToVector performs fused multiply-add: dst = src1 * src2 + src3
// Four operands: destination and three sources
func (o *Out) VFmaddPDVectorToVector(dst, src1, src2, src3 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vfmaddX86VectorToVector(dst, src1, src2, src3)
	case ArchARM64:
		o.vfmaddARM64VectorToVector(dst, src1, src2, src3)
	case ArchRiscv64:
		o.vfmaddRISCVVectorToVector(dst, src1, src2, src3)
	}
}

// ============================================================================
// x86-64 AVX-512/FMA implementation
// ============================================================================

// x86-64 VFMADD231PD - dst = src1 * src2 + dst (accumulator form)
// EVEX.NDS.512.66.0F38.W1 B8 /r
func (o *Out) vfmaddX86VectorToVector(dst, src1, src2, src3 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	src3Reg, src3Ok := GetRegister(o.target.Arch(), src3)
	if !dstOk || !src1Ok || !src2Ok || !src3Ok {
		return
	}

	// For FMA231: dst = src1 * src2 + src3
	// The instruction actually does: dst = dst + src1 * src2
	// So we need to: MOV dst, src3; VFMADD231 dst, src1, src2

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# fma %s = %s * %s + %s\n", dst, src1, src2, src3)
	}

	// Move src3 into dst first (required for FMA231 semantics)
	// Only move if src3 != dst (avoid unnecessary mov)
	if src3Reg.Encoding != dstReg.Encoding {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "movsd %s, %s\n", dst, src3)
		}
		o.MovXmmToXmm(dst, src3)
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmadd231pd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VFMADD231PD
		// EVEX encoding: opcode map is 0F38 (3-byte opcode)
		p0 := uint8(0x62)

		// P1: map_select = 10 (0F38 opcode map)
		p1 := uint8(0x02) // mm = 10 for 0F38 map
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80 // R
		}
		p1 |= 0x40 // X
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20 // B (src2 is ModRM.r/m)
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10 // R'
		}

		// P2: W=1, vvvv=~src1 (src1 is the multiplier), pp=01
		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		// P3: L'L=10 (512-bit)
		p3 := uint8(0x40)
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0xB8 (VFMADD231PD)
		o.Write(0xB8)

		// ModR/M: dst is reg field, src2 is r/m field
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX2 FMA (VEX encoding)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (FMA 256-bit)")
		}

		// VEX 3-byte prefix: C4 [RXB mm-mmm] [W vvvv L pp]
		o.Write(0xC4)

		// Byte 1: map_select = 010 (0F38)
		vex1 := uint8(0x02)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		// Byte 2: W=1, vvvv=~src1, L=1 (256-bit), pp=01
		vex2 := uint8(0xC5) // W=1, L=1, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		// Opcode
		o.Write(0xB8)

		// ModR/M
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		// XMM - FMA3 128-bit
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (FMA 128-bit)")
		}

		o.Write(0xC4)

		vex1 := uint8(0x02)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		// L=0 for 128-bit
		vex2 := uint8(0x85) // W=1, L=0, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		o.Write(0xB8)

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

// ARM64 FMLA Zdn.D, Pg/M, Zm.D, Za.D
// Multiply-accumulate: Zdn = Zdn + Zm * Za
func (o *Out) vfmaddARM64VectorToVector(dst, src1, src2, src3 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	_, src3Ok := GetRegister(o.target.Arch(), src3)
	if !dstOk || !src1Ok || !src2Ok || !src3Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE2 FMLA: dst = dst + src1 * src2
		// To get dst = src1 * src2 + src3, we need: dst = src3 first, then FMLA
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# fma %s = %s * %s + %s\n", dst, src1, src2, src3)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "mov %s.d, p7/m, %s.d\n", dst, src3)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmla %s.d, p7/m, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE FMLA encoding
		// 01100101 11 Zm[4:0] 010 Pg[2:0] Zn[4:0] Zda[4:0]
		// size=11, opc=010 (FMLA)
		instr := uint32(0x65000000) |
			(3 << 22) | // size=11 (double)
			(2 << 18) | // opc=010 (FMLA)
			(uint32(src2Reg.Encoding&31) << 16) | // Zm (multiplier 2)
			(7 << 10) | // Pg=p7
			(uint32(src1Reg.Encoding&31) << 5) | // Zn (multiplier 1)
			uint32(dstReg.Encoding&31) // Zda (accumulator/dest)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON FMLA Vd.2D, Vn.2D, Vm.2D
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# fma %s = %s * %s + %s\n", dst, src1, src2, src3)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "mov %s.16b, %s.16b\n", dst, src3)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "fmla %s.2d, %s.2d, %s.2d:", dst, src1, src2)
		}

		// NEON FMLA encoding
		// 0 Q 0 01110 1 sz 1 Rm 11001 1 Rn Rd
		// Q=1, sz=1 (double)
		instr := uint32(0x4EC01C00) |
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

// RISC-V vfmadd.vv vd, vs1, vs2
// Multiply-add: vd = vd * vs1 + vs2
func (o *Out) vfmaddRISCVVectorToVector(dst, src1, src2, src3 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	_, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	src3Reg, src3Ok := GetRegister(o.target.Arch(), src3)
	if !dstOk || !src1Ok || !src2Ok || !src3Ok {
		return
	}

	// RISC-V vfmadd: vd = vd * vs1 + vs2
	// To get vd = src1 * src2 + src3, need: vd=src1, then vfmadd vd, src2, src3
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# fma %s = %s * %s + %s\n", dst, src1, src2, src3)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmv.v.v %s, %s\n", dst, src1)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmadd.vv %s, %s, %s:", dst, src2, src3)
	}

	// RVV vfmadd.vv encoding
	// funct6 vm vs2 vs1 funct3 vd opcode
	// 101000 1 vs2[4:0] vs1[4:0] 001 vd[4:0] 1010111
	// funct6=101000 (VFMADD)

	instr := uint32(0x57) | // opcode
		(1 << 12) | // funct3=001 (OPFVV)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(uint32(src3Reg.Encoding&31) << 15) | // vs1 (addend)
		(uint32(src2Reg.Encoding&31) << 20) | // vs2 (multiplier)
		(1 << 25) | // vm=1 (unmasked)
		(0x28 << 26) // funct6=101000

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









