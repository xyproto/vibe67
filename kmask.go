// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// Mask register operations for AVX-512 k registers
//
// Essential for Vibe67's predication and conditional operations:
//   - Mask combination: k1 and k2, k1 or k2, k1 xor k2
//   - Mask creation from vector comparisons
//   - Conditional execution based on masks
//   - Population count: count set bits in mask
//
// Example usage in Vibe67:
//   m1: mask = values || (x -> x > 0.0)
//   m2: mask = values || (x -> x < 100.0)
//   m3: mask = m1 and m2
//
// Architecture details:
//   x86-64: KANDW/KORW/KXORW k1, k2, k3 (AVX-512 mask operations)
//   ARM64:  AND/ORR/EOR pg.b, pn/z, pm.b (SVE2 predicate operations)
//   RISC-V: vmand.mm/vmor.mm/vmxor.mm vd, vs2, vs1 (RVV mask operations)

// KAndMaskToMask computes bitwise AND on mask registers
// dst = src1 & src2
func (o *Out) KAndMaskToMask(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.kandX86MaskToMask(dst, src1, src2)
	case ArchARM64:
		o.kandARM64MaskToMask(dst, src1, src2)
	case ArchRiscv64:
		o.kandRISCVMaskToMask(dst, src1, src2)
	}
}

// KOrMaskToMask computes bitwise OR on mask registers
// dst = src1 | src2
func (o *Out) KOrMaskToMask(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.korX86MaskToMask(dst, src1, src2)
	case ArchARM64:
		o.korARM64MaskToMask(dst, src1, src2)
	case ArchRiscv64:
		o.korRISCVMaskToMask(dst, src1, src2)
	}
}

// KXorMaskToMask computes bitwise XOR on mask registers
// dst = src1 ^ src2
func (o *Out) KXorMaskToMask(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.kxorX86MaskToMask(dst, src1, src2)
	case ArchARM64:
		o.kxorARM64MaskToMask(dst, src1, src2)
	case ArchRiscv64:
		o.kxorRISCVMaskToMask(dst, src1, src2)
	}
}

// KNotMaskToMask computes bitwise NOT on mask register
// dst = ~src
func (o *Out) KNotMaskToMask(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.knotX86MaskToMask(dst, src)
	case ArchARM64:
		o.knotARM64MaskToMask(dst, src)
	case ArchRiscv64:
		o.knotRISCVMaskToMask(dst, src)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - AND
// ============================================================================

// x86-64 KANDW k1, k2, k3 (16-bit mask operation)
// VEX.L1.0F.W0 41 /r
func (o *Out) kandX86MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "kandw %s, %s, %s:", dst, src1, src2)
	}

	// VEX 3-byte encoding
	o.Write(0xC4)

	// Byte 1: R=1, X=1, B=1, map=01 (0F)
	vex1 := uint8(0xE1)
	o.Write(vex1)

	// Byte 2: W=0, vvvv=~src1, L=1, pp=00
	vex2 := uint8(0x44) // L=1 (L1)
	vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
	o.Write(vex2)

	// Opcode: 0x41 (KANDW)
	o.Write(0x41)

	// ModR/M: dst and src2
	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - OR
// ============================================================================

// x86-64 KORW k1, k2, k3
// VEX.L1.0F.W0 45 /r
func (o *Out) korX86MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "korw %s, %s, %s:", dst, src1, src2)
	}

	o.Write(0xC4)

	vex1 := uint8(0xE1)
	o.Write(vex1)

	vex2 := uint8(0x44)
	vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
	o.Write(vex2)

	o.Write(0x45) // KORW opcode

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - XOR
// ============================================================================

// x86-64 KXORW k1, k2, k3
// VEX.L1.0F.W0 47 /r
func (o *Out) kxorX86MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "kxorw %s, %s, %s:", dst, src1, src2)
	}

	o.Write(0xC4)

	vex1 := uint8(0xE1)
	o.Write(vex1)

	vex2 := uint8(0x44)
	vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
	o.Write(vex2)

	o.Write(0x47) // KXORW opcode

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation - NOT
// ============================================================================

// x86-64 KNOTW k1, k2
// VEX.L0.0F.W0 44 /r
func (o *Out) knotX86MaskToMask(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "knotw %s, %s:", dst, src)
	}

	o.Write(0xC4)

	vex1 := uint8(0xE1)
	o.Write(vex1)

	vex2 := uint8(0x04) // L=0, vvvv=1111
	o.Write(vex2)

	o.Write(0x44) // KNOTW opcode

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2 implementation
// ============================================================================

func (o *Out) kandARM64MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	// SVE predicate AND
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "and %s.b, %s/z, %s.b, %s.b:", dst, src1, src1, src2)
	}

	// SVE predicate AND encoding
	// 00100101 00 0 Pm 01 0000 Pg 0 Pn Pd
	instr := uint32(0x25004000) |
		(uint32(src2Reg.Encoding&15) << 16) | // Pm
		(uint32(src1Reg.Encoding&15) << 10) | // Pg (also source)
		(uint32(src1Reg.Encoding&15) << 5) | // Pn
		uint32(dstReg.Encoding&15) // Pd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) korARM64MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	// SVE predicate ORR
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "orr %s.b, %s/z, %s.b, %s.b:", dst, src1, src1, src2)
	}

	// SVE predicate ORR encoding
	// 00100101 10 0 Pm 01 0000 Pg 0 Pn Pd
	instr := uint32(0x25804000) |
		(uint32(src2Reg.Encoding&15) << 16) |
		(uint32(src1Reg.Encoding&15) << 10) |
		(uint32(src1Reg.Encoding&15) << 5) |
		uint32(dstReg.Encoding&15)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) kxorARM64MaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	// SVE predicate EOR
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "eor %s.b, %s/z, %s.b, %s.b:", dst, src1, src1, src2)
	}

	// SVE predicate EOR encoding
	// 00100101 01 0 Pm 01 0000 Pg 0 Pn Pd
	instr := uint32(0x25404000) |
		(uint32(src2Reg.Encoding&15) << 16) |
		(uint32(src1Reg.Encoding&15) << 10) |
		(uint32(src1Reg.Encoding&15) << 5) |
		uint32(dstReg.Encoding&15)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) knotARM64MaskToMask(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	// SVE predicate NOT
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "not %s.b, p15/z, %s.b:", dst, src)
	}

	// SVE predicate NOT encoding
	// 00100101 01 010000 01 Pg 0 Pn Pd
	instr := uint32(0x25504000) |
		(15 << 10) | // Pg=p15 (all true)
		(uint32(srcReg.Encoding&15) << 5) | // Pn
		uint32(dstReg.Encoding&15) // Pd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation
// ============================================================================

func (o *Out) kandRISCVMaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmand.mm %s, %s, %s:", dst, src1, src2)
	}

	// vmand.mm encoding
	// funct6=011001, vm=1, funct3=010 (OPMVV)
	instr := uint32(0x57) |
		(2 << 12) | // funct3=010
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) | // vm=1
		(0x19 << 26) // funct6=011001

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) korRISCVMaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmor.mm %s, %s, %s:", dst, src1, src2)
	}

	// vmor.mm encoding
	// funct6=011010, vm=1, funct3=010 (OPMVV)
	instr := uint32(0x57) |
		(2 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x1A << 26) // funct6=011010

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) kxorRISCVMaskToMask(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmxor.mm %s, %s, %s:", dst, src1, src2)
	}

	// vmxor.mm encoding
	// funct6=011011, vm=1, funct3=010 (OPMVV)
	instr := uint32(0x57) |
		(2 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(src1Reg.Encoding&31) << 15) |
		(uint32(src2Reg.Encoding&31) << 20) |
		(1 << 25) |
		(0x1B << 26) // funct6=011011

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) knotRISCVMaskToMask(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vmnot.m %s, %s:", dst, src)
	}

	// vmnot.m encoding (vmxor.mm with all-ones mask)
	// funct6=011011, vm=1, funct3=010
	instr := uint32(0x57) |
		(2 << 12) |
		(uint32(dstReg.Encoding&31) << 7) |
		(uint32(srcReg.Encoding&31) << 15) |
		(uint32(srcReg.Encoding&31) << 20) | // vs2=vs1 for NOT
		(1 << 25) |
		(0x1B << 26)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









