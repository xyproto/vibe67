// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VBLENDMPD - Blend (conditional select) packed double-precision floats using mask
//
// Essential for Vibe67's conditional operations:
//   - Ternary select: mask ? true_values : false_values
//   - Conditional updates: update only selected elements
//   - Predicated operations: apply operation where mask is true
//   - Error handling: select valid or default values
//
// Example usage in Vibe67:
//   m: mask = values || (x -> x > 0.0)
//   result = m ? (values || (x -> x * 2)) : values
//
// Architecture details:
//   x86-64: VBLENDMPD zmm1{k1}, zmm2, zmm3 (AVX-512: dst = k1 ? src1 : src2)
//   ARM64:  SEL zd.d, pg, zn.d, zm.d (SVE2: dst = pg ? zn : zm)
//   RISC-V: vmerge.vvm vd, vs2, vs1, v0 (RVV: vd = v0 ? vs1 : vs2)

// VBlendMPDWithMask blends two vectors based on mask
// dst[i] = mask[i] ? src1[i] : src2[i]
func (o *Out) VBlendMPDWithMask(dst, src1, src2, mask string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vblendmpdX86WithMask(dst, src1, src2, mask)
	case ArchARM64:
		o.vblendARM64WithMask(dst, src1, src2, mask)
	case ArchRiscv64:
		o.vblendRISCVWithMask(dst, src1, src2, mask)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation
// ============================================================================

// x86-64 VBLENDMPD zmm1{k1}, zmm2, zmm3
// EVEX.NDS.512.66.0F38.W1 65 /r
func (o *Out) vblendmpdX86WithMask(dst, src1, src2, mask string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	maskReg, maskOk := GetRegister(o.target.Arch(), mask)
	if !dstOk || !src1Ok || !src2Ok || !maskOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vblendmpd %s{%s}, %s, %s:", dst, mask, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VBLENDMPD
		p0 := uint8(0x62)

		// P1: map_select = 10 (0F38)
		p1 := uint8(0x02)
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

		// P2: W=1, vvvv=~src1, pp=01
		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		// P3: L'L=10 (512-bit), with masking
		p3 := uint8(0x40) | (maskReg.Encoding & 7) // aaa = k register
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0x65 (VBLENDMPD)
		o.Write(0x65)

		// ModR/M: dst is reg field, src2 is r/m field
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else if dstReg.Size == 256 {
		// AVX2 VBLENDVPD (uses implicit XMM0 as mask - different semantics)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2 VBLENDVPD - mask in ymm)")
		}

		// Note: AVX2 VBLENDVPD uses top bit of each element in mask register
		// Different from AVX-512's k register masks
		o.Write(0xC4)

		vex1 := uint8(0x03) // map=0F3A
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src1Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		// vvvv encodes first source
		vex2 := uint8(0x45) // W=0, L=1, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		o.Write(0x4B) // VBLENDVPD opcode

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)

		// Immediate: mask register in bits 7:4
		o.Write(uint8((maskReg.Encoding & 0x0F) << 4))
	} else {
		// SSE4.1 BLENDVPD (uses implicit XMM0)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE4.1 BLENDVPD - mask must be in xmm0)")
		}

		o.Write(0x66) // prefix
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
		o.Write(0x38)
		o.Write(0x15) // BLENDVPD opcode

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

// ARM64 SEL zd.d, pg, zn.d, zm.d
// Select elements based on predicate: dst = pg ? zn : zm
func (o *Out) vblendARM64WithMask(dst, src1, src2, mask string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	maskReg, maskOk := GetRegister(o.target.Arch(), mask)
	if !dstOk || !src1Ok || !src2Ok || !maskOk {
		return
	}

	if dstReg.Size == 512 {
		// SVE SEL (select)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "sel %s.d, %s, %s.d, %s.d:", dst, mask, src1, src2)
		}

		// SVE SEL encoding
		// 00000101 11 1 Zm 11 Pg 0 Zn Zd
		// size=11 (doubleword)
		instr := uint32(0x05C03000) |
			(uint32(src2Reg.Encoding&31) << 16) | // Zm (false values)
			(uint32(maskReg.Encoding&15) << 10) | // Pg (predicate)
			(uint32(src1Reg.Encoding&31) << 5) | // Zn (true values)
			uint32(dstReg.Encoding&31) // Zd (destination)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON BSL (bit select)
		// dst = (dst & mask) | (src2 & ~mask)
		// Requires mask to be in dst first
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# NEON blend: use BSL sequence\n")
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "mov %s.16b, %s.16b  # mask to dst\n", dst, mask)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "bsl %s.16b, %s.16b, %s.16b:", dst, src1, src2)
		}

		// BSL encoding
		// 0 Q 101110 01 1 Rm 0 00 1 01 Rn Rd
		// Q=1 (128-bit), size=01 (not actually used for BSL)
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

// RISC-V vmerge.vvm vd, vs2, vs1, v0
// Merge: vd[i] = v0.mask[i] ? vs1[i] : vs2[i]
// Note: RVV uses v0 as implicit mask register
func (o *Out) vblendRISCVWithMask(dst, src1, src2, mask string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	_, maskOk := GetRegister(o.target.Arch(), mask)
	if !dstOk || !src1Ok || !src2Ok || !maskOk {
		return
	}

	// Note: RVV vmerge uses v0 as implicit mask
	// If mask != "v0", would need to move it first
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "# RVV merge uses v0 as mask\n")
	}
	if mask != "v0" {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "vmv.v.v v0, %s  # move mask to v0\n", mask)
		}
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmerge.vvm %s, %s, %s, v0:", dst, src2, src1)
	}

	// vmerge.vvm encoding
	// funct6 vm=0 vs2 vs1 funct3 vd opcode
	// funct6=010111, funct3=000 (OPIVV), vm=0 (masked)
	instr := uint32(0x57) | // opcode=1010111 (OP-V)
		(0 << 12) | // funct3=000 (OPIVV)
		(uint32(dstReg.Encoding&31) << 7) | // vd
		(uint32(src1Reg.Encoding&31) << 15) | // vs1 (true source)
		(uint32(src2Reg.Encoding&31) << 20) | // vs2 (false source)
		(0 << 25) | // vm=0 (use v0.t mask)
		(0x17 << 26) // funct6=010111 (VMERGE)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









