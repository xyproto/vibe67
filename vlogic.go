// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VANDPD/VORPD/VXORPD - Vector bitwise logical operations on packed doubles
//
// Essential for Vibe67's mask manipulation and bit-level operations:
//   - Mask combination: m1 and m2, m1 or m2, m1 xor m2
//   - Bit manipulation: clearing sign bits (abs), flipping bits
//   - Conditional operations: combining predicates
//   - Set operations: union, intersection, symmetric difference
//
// Example usage in Vibe67:
//   m3: mask = m1 and m2           // Intersection
//   m4: mask = m1 or m2            // Union
//   m5: mask = m1 xor m2           // Symmetric difference
//
// Architecture details:
//   x86-64: VANDPD/VORPD/VXORPD zmm1, zmm2, zmm3 (AVX-512/AVX/SSE2)
//   ARM64:  AND/ORR/EOR zd.d, zn.d, zm.d (SVE2)
//   RISC-V: vand.vv/vor.vv/vxor.vv vd, vs2, vs1 (RVV)

// VAndPDVectorToVector computes bitwise AND
// dst[i] = src1[i] & src2[i]
func (o *Out) VAndPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vandpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vandARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vandRISCVVectorToVector(dst, src1, src2)
	}
}

// VOrPDVectorToVector computes bitwise OR
// dst[i] = src1[i] | src2[i]
func (o *Out) VOrPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vorpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vorARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vorRISCVVectorToVector(dst, src1, src2)
	}
}

// VXorPDVectorToVector computes bitwise XOR
// dst[i] = src1[i] ^ src2[i]
func (o *Out) VXorPDVectorToVector(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vxorpdX86VectorToVector(dst, src1, src2)
	case ArchARM64:
		o.vxorARM64VectorToVector(dst, src1, src2)
	case ArchRiscv64:
		o.vxorRISCVVectorToVector(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation - AND
// ============================================================================

func (o *Out) vandpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vandpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VANDPD
		// EVEX.NDS.512.66.0F.W1 54 /r
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
		o.Write(0x54) // VANDPD opcode

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
		o.Write(0x54)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		o.Write(0x0F)
		o.Write(0x54)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation - OR
// ============================================================================

func (o *Out) vorpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vorpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VORPD
		// EVEX.NDS.512.66.0F.W1 56 /r
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
		o.Write(0x56) // VORPD opcode

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
		o.Write(0x56)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		o.Write(0x0F)
		o.Write(0x56)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512/AVX/SSE2 implementation - XOR
// ============================================================================

func (o *Out) vxorpdX86VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vxorpd %s, %s, %s:", dst, src1, src2)
	}

	if dstReg.Size == 512 {
		// AVX-512 VXORPD
		// EVEX.NDS.512.66.0F.W1 57 /r
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
		o.Write(0x57) // VXORPD opcode

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
		o.Write(0x57)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2)")
		}
		o.Write(0x66)
		o.Write(0x0F)
		o.Write(0x57)

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

func (o *Out) vandARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE AND
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "and %s.d, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE AND encoding
		// 00000100 01 1 Zm 0011 00 Zn Zd
		instr := uint32(0x04603000) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON AND
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "and %s.16b, %s.16b, %s.16b:", dst, src1, src2)
		}

		// NEON AND encoding
		// 0 Q 001110 001 Rm 00011 1 Rn Rd
		instr := uint32(0x0E201C00) |
			(1 << 30) | // Q=1
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

func (o *Out) vorARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE ORR
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "orr %s.d, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE ORR encoding
		// 00000100 01 1 Zm 0011 10 Zn Zd
		instr := uint32(0x04603800) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON ORR
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "orr %s.16b, %s.16b, %s.16b:", dst, src1, src2)
		}

		// NEON ORR encoding
		// 0 Q 001110 101 Rm 00011 1 Rn Rd
		instr := uint32(0x0EA01C00) |
			(1 << 30) | // Q=1
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

func (o *Out) vxorARM64VectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if dstReg.Size == 512 {
		// SVE EOR
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "eor %s.d, %s.d, %s.d:", dst, src1, src2)
		}

		// SVE EOR encoding
		// 00000100 01 1 Zm 0011 01 Zn Zd
		instr := uint32(0x04603400) |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON EOR
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "eor %s.16b, %s.16b, %s.16b:", dst, src1, src2)
		}

		// NEON EOR encoding
		// 0 Q 101110 001 Rm 00011 1 Rn Rd
		instr := uint32(0x2E201C00) |
			(1 << 30) | // Q=1
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
// RISC-V RVV implementation
// ============================================================================

func (o *Out) vandRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vand.vv %s, %s, %s:", dst, src1, src2)
	}

	// vand.vv encoding
	// funct6=001001, funct3=000 (OPIVV)
	instr := uint32(0x57) |
		(0 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x09 << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) vorRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vor.vv %s, %s, %s:", dst, src1, src2)
	}

	// vor.vv encoding
	// funct6=001010, funct3=000 (OPIVV)
	instr := uint32(0x57) |
		(0 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x0A << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) vxorRISCVVectorToVector(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vxor.vv %s, %s, %s:", dst, src1, src2)
	}

	// vxor.vv encoding
	// funct6=001011, funct3=000 (OPIVV)
	instr := uint32(0x57) |
		(0 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x0B << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
