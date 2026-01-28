// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// NEG instruction for arithmetic negation (two's complement)
// Essential for implementing Vibe67's negation operations:
//   - Unary minus: -x, -value
//   - Direction reversal: -velocity
//   - Sign flipping: -balance
//   - Opposite values: -delta
//   - Negating results: -(a + b)

// NegReg generates NEG dst (dst = -dst)
func (o *Out) NegReg(dst string) {
	if o.backend != nil {
		o.backend.NegReg(dst)
		return
	}
	// Fallback for x86_64 (uses methods in this file)
	switch o.target.Arch() {
	case ArchX86_64:
		o.negX86Reg(dst)
	}
}

// NegRegToReg generates NEG dst, src (dst = -src)
// 3-operand form for ARM64 and RISC-V
func (o *Out) NegRegToReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86-64: MOV dst, src; NEG dst
		if dst != src {
			o.MovRegToReg(dst, src)
		}
		o.NegReg(dst)
	case ArchARM64:
		o.negARM64RegToReg(dst, src)
	case ArchRiscv64:
		o.negRISCVRegToReg(dst, src)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 NEG (two's complement negation)
func (o *Out) negX86Reg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "neg %s:", dst)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// NEG r/m64 (opcode 0xF7 /3)
	o.Write(0xF7)

	// ModR/M: 11 011 reg (register direct, opcode extension /3 for NEG)
	modrm := uint8(0xD8) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 NEG (actually SUB Xd, XZR, Xn) - 2 operand form
func (o *Out) negARM64Reg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "neg %s, %s:", dst, dst)
	}

	// NEG Xd, Xn is actually SUB Xd, XZR, Xn
	// Format: sf 1 01011 shift 0 Rm imm6 Rn Rd
	// sf=1 (64-bit), Rn=XZR (31), Rm=src (same as Rd for 2-operand)
	instr := uint32(0xCB000000) |
		(uint32(dstReg.Encoding&31) << 16) | // Rm (source, same as Rd)
		(31 << 5) | // Rn (XZR - zero register)
		uint32(dstReg.Encoding&31) // Rd (dest)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 NEG - 3 operand form
func (o *Out) negARM64RegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "neg %s, %s:", dst, src)
	}

	// NEG Xd, Xn is SUB Xd, XZR, Xn
	instr := uint32(0xCB000000) |
		(uint32(srcReg.Encoding&31) << 16) | // Rm (source)
		(31 << 5) | // Rn (XZR)
		uint32(dstReg.Encoding&31) // Rd (dest)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V NEG (pseudo-instruction: SUB rd, x0, rs) - 2 operand form
func (o *Out) negRISCVReg(dst string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "neg %s, %s:", dst, dst)
	}

	// NEG rd, rs is SUB rd, x0, rs
	// SUB: 0100000 rs2 rs1 000 rd 0110011
	instr := uint32(0x33) |
		(0x20 << 25) | // funct7 = 0100000 (SUB)
		(uint32(dstReg.Encoding&31) << 20) | // rs2 (source, same as rd)
		(0 << 15) | // rs1 (x0 - zero register)
		(uint32(dstReg.Encoding&31) << 7) // rd (dest)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V NEG - 3 operand form
func (o *Out) negRISCVRegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "neg %s, %s:", dst, src)
	}

	// NEG rd, rs is SUB rd, x0, rs
	instr := uint32(0x33) |
		(0x20 << 25) | // funct7 = 0100000 (SUB)
		(uint32(srcReg.Encoding&31) << 20) | // rs2 (source)
		(0 << 15) | // rs1 (x0)
		(uint32(dstReg.Encoding&31) << 7) // rd (dest)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
