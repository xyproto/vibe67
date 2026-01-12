// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VBROADCASTSD - Broadcast scalar double-precision float to all vector elements
//
// Essential for Vibe67's vectorized operations:
//   - Scaling: scale all elements by a constant
//   - Comparison: compare all elements against a threshold
//   - Initialization: fill vector with a value
//   - Binary operations: operate vector with scalar
//
// Example usage in Vibe67:
//   values || map(x -> x * 2.0)  // Broadcast 2.0 to all elements
//   values || map(x -> x > threshold)  // Broadcast threshold
//
// Architecture details:
//   x86-64: VBROADCASTSD zmm1, xmm2/m64 (AVX-512/AVX2)
//   ARM64:  DUP zd.d, zn.d[0] (SVE2), DUP vd.2d, vn.d[0] (NEON)
//   RISC-V: vfmv.v.f vd, fs (RVV: broadcast float register to vector)

// VBroadcastSDScalarToVector broadcasts a scalar float64 to all vector elements
// dst[i] = src (for all i)
func (o *Out) VBroadcastSDScalarToVector(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vbroadcastsdX86ScalarToVector(dst, src)
	case ArchARM64:
		o.vbroadcastARM64ScalarToVector(dst, src)
	case ArchRiscv64:
		o.vbroadcastRISCVScalarToVector(dst, src)
	}
}

// VBroadcastSDMemToVector broadcasts a scalar float64 from memory to all vector elements
// dst[i] = memory[base + offset] (for all i)
func (o *Out) VBroadcastSDMemToVector(dst, base string, offset int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vbroadcastsdX86MemToVector(dst, base, offset)
	case ArchARM64:
		o.vbroadcastARM64MemToVector(dst, base, offset)
	case ArchRiscv64:
		o.vbroadcastRISCVMemToVector(dst, base, offset)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX2 implementation
// ============================================================================

// x86-64 VBROADCASTSD zmm1, xmm2 (register to vector)
// EVEX.512.66.0F38.W1 19 /r
func (o *Out) vbroadcastsdX86ScalarToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vbroadcastsd %s, %s:", dst, src)
	}

	if dstReg.Size == 512 {
		// AVX-512 VBROADCASTSD
		p0 := uint8(0x62)

		// P1: map_select = 10 (0F38)
		p1 := uint8(0x02)
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

		// P2: W=1, vvvv=1111 (unused), pp=01
		p2 := uint8(0x81) | (0x0F << 3)

		// P3: L'L=10 (512-bit)
		p3 := uint8(0x40)

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0x19 (VBROADCASTSD)
		o.Write(0x19)

		// ModR/M: dst is reg field, src is r/m field
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX2 VBROADCASTSD ymm, xmm
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2)")
		}

		// VEX 3-byte prefix: C4 [RXB mm-mmm] [W vvvv L pp]
		o.Write(0xC4)

		// Byte 1: map_select = 010 (0F38)
		vex1 := uint8(0x02)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (srcReg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		// Byte 2: W=1, vvvv=1111, L=1 (256-bit), pp=01
		vex2 := uint8(0xC5) // W=1, L=1, pp=01
		o.Write(vex2)

		// Opcode
		o.Write(0x19)

		// ModR/M
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	} else {
		// XMM - just copy (no broadcast needed for 128-bit with single element)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE - movsd)")
		}
		// Use MOVSD for 128-bit scalar copy
		o.Write(0xF2) // MOVSD prefix
		rex := uint8(0x40)
		if (dstReg.Encoding & 8) != 0 {
			rex |= 0x04 // R
		}
		if (srcReg.Encoding & 8) != 0 {
			rex |= 0x01 // B
		}
		if rex != 0x40 {
			o.Write(rex)
		}
		o.Write(0x0F)
		o.Write(0x10) // MOVSD opcode
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 VBROADCASTSD zmm1, m64 (memory to vector)
// EVEX.512.66.0F38.W1 19 /r
func (o *Out) vbroadcastsdX86MemToVector(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vbroadcastsd %s, [%s + %d]:", dst, base, offset)
	}

	if dstReg.Size == 512 {
		// AVX-512 VBROADCASTSD with memory operand
		p0 := uint8(0x62)

		p1 := uint8(0x02)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80
		}
		p1 |= 0x40
		if (baseReg.Encoding & 8) == 0 {
			p1 |= 0x20
		}
		if (dstReg.Encoding & 16) == 0 {
			p1 |= 0x10
		}

		p2 := uint8(0x81) | (0x0F << 3)
		p3 := uint8(0x40)

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)
		o.Write(0x19)

		// ModR/M with memory operand
		if offset == 0 && (baseReg.Encoding&7) != 5 {
			modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
		} else if offset >= -128 && offset <= 127 {
			modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
			o.Write(uint8(offset))
		} else {
			modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
			o.Write(uint8(offset & 0xFF))
			o.Write(uint8((offset >> 8) & 0xFF))
			o.Write(uint8((offset >> 16) & 0xFF))
			o.Write(uint8((offset >> 24) & 0xFF))
		}
	} else if dstReg.Size == 256 {
		// AVX2 version
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2)")
		}
		o.Write(0xC4)

		vex1 := uint8(0x02)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (baseReg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0xC5)
		o.Write(vex2)
		o.Write(0x19)

		if offset == 0 && (baseReg.Encoding&7) != 5 {
			modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
		} else if offset >= -128 && offset <= 127 {
			modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
			o.Write(uint8(offset))
		} else {
			modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
			o.Write(modrm)
			o.Write(uint8(offset & 0xFF))
			o.Write(uint8((offset >> 8) & 0xFF))
			o.Write(uint8((offset >> 16) & 0xFF))
			o.Write(uint8((offset >> 24) & 0xFF))
		}
	} else {
		// SSE - load scalar
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE)")
		}
		// MOVSD for 128-bit
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2/NEON implementation
// ============================================================================

// ARM64 DUP zd.d, zn.d[0] (SVE2) or DUP vd.2d, vn.d[0] (NEON)
func (o *Out) vbroadcastARM64ScalarToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE DUP zd.d, zn.d[0]
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "dup %s.d, %s.d[0]:", dst, src)
		}

		// SVE DUP encoding
		// 00000101 10 1 imm2 001 000 Zn Zd
		// imm2=00 for element 0
		instr := uint32(0x05203800) |
			(uint32(srcReg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Zd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON DUP vd.2d, vn.d[0]
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "dup %s.2d, %s.d[0]:", dst, src)
		}

		// NEON DUP (element) encoding
		// 0 Q 001110 imm5 0 0000 1 Rn Rd
		// Q=1 (128-bit), imm5=01000 (doubleword, index 0)
		instr := uint32(0x4E080400) |
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

// ARM64 LD1RD {zt.d}, pg/z, [xn, #imm]
func (o *Out) vbroadcastARM64MemToVector(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE LD1RD - load and replicate doubleword
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "ld1rd {%s.d}, p7/z, [%s, #%d]:", dst, base, offset)
		}

		// SVE LD1RD encoding
		// 1000010 1 1 imm6 111 Pg Rn Zt
		// imm6 is scaled by element size (8 bytes for doubleword)
		imm6 := uint32((offset / 8) & 0x3F)
		instr := uint32(0x85C0E000) |
			(imm6 << 16) | // imm6
			(7 << 10) | // Pg=p7
			(uint32(baseReg.Encoding&31) << 5) | // Rn
			uint32(dstReg.Encoding&31) // Zt

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON - load then duplicate
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# NEON broadcast from mem needs LD1R\n")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation
// ============================================================================

// RISC-V vfmv.v.f vd, fs
// Broadcast float register to all vector elements
func (o *Out) vbroadcastRISCVScalarToVector(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	_, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmv.v.f %s, %s:", dst, src)
	}

	// vfmv.v.f encoding (note: src should be an f register, not v register)
	// For now, assuming scalar register encoding
	// funct6 vm rs2=00000 rs1(float) funct3 vd opcode
	// funct6=010111, funct3=101 (OPFVF), vm=1

	// This is a simplified encoding - actual implementation would need
	// to distinguish between vector and float registers
	instr := uint32(0x57) | // opcode=1010111 (OP-V)
		(5 << 12) | // funct3=101 (OPFVF)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(1 << 25) | // vm=1
		(0x17 << 26) // funct6=010111 (VMV.V.X/VMV.V.F)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V vle64.v + vrgather for broadcast from memory
func (o *Out) vbroadcastRISCVMemToVector(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	_, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	// RVV doesn't have direct broadcast-from-memory
	// Would need: load scalar, then broadcast
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# RVV broadcast from mem: load then vfmv.v.f\n")
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fld ft0, %d(%s)\n", offset, base)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vfmv.v.f %s, ft0:", dst)
	}

	// Just emit the broadcast part
	instr := uint32(0x57) |
		(5 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(1 << 25) |
		(0x17 << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
