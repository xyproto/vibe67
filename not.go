// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// NOT instruction implementation for all architectures
// Used for bitwise NOT (one's complement) in unsafe blocks:
//   - Inverting all bits: ~b value
//   - Creating bit masks: ~b 0 gives all 1s

// NotReg generates NOT dst (dst = ~dst) - one's complement
func (o *Out) NotReg(dst string) {
	if o.backend != nil {
		o.backend.NotReg(dst)
		return
	}
	// Fallback for x86_64 (uses methods in this file)
	switch o.target.Arch() {
	case ArchX86_64:
		o.notX86Reg(dst)
	}
}

// ============================================================================
// x86-64 implementation
// ============================================================================

// x86-64 NOT (one's complement negation)
func (o *Out) notX86Reg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "not %s:", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// NOT opcode (0xF7 /2 for r/m64)
	o.Write(0xF7)

	// ModR/M: 11 (register direct) | opcode extension /2 | r/m (dst)
	modrm := uint8(0xD0) | (dstReg.Encoding & 7) // 11 010 xxx
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementation
// ============================================================================

// ARM64 MVN (NOT) - bitwise NOT using MVN (Move NOT)
func (o *Out) notARM64Reg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mvn %s, %s:", dst, dst)
	}

	// MVN Xd, Xm (ORR Xd, XZR, Xm, LSL #0, inverted)
	// This is equivalent to: NOT Xd = ORN Xd, XZR, Xd
	// Format: sf 1 01010 shift 1 Rm imm6 Rn Rd
	// ORN (OR NOT): sf=1, shift=00, N=1 (inverted), Rm=src, Rn=XZR(31), Rd=dst
	instr := uint32(0xAA200000) |
		(uint32(dstReg.Encoding&31) << 16) | // Rm (source, same as dst)
		(uint32(31) << 5) | // Rn = XZR (zero register)
		uint32(dstReg.Encoding&31) // Rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementation
// ============================================================================

// RISC-V NOT (using XORI with -1)
func (o *Out) notRISCVReg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "not %s:", dst)
	}

	// RISC-V doesn't have a direct NOT instruction
	// Use XORI dst, dst, -1 (XOR with all 1s)
	// XORI: imm[11:0] rs1 100 rd 0010011
	instr := uint32(0x13) |
		(4 << 12) | // funct3 = 100 (XORI)
		(uint32(0xFFF) << 20) | // imm = -1 (all 1s in 12 bits)
		(uint32(dstReg.Encoding&31) << 15) | // rs1 (same as rd)
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
