// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// Conditional jump instructions for all architectures
// Critical for implementing Vibe67 language control flow:
//   - Pattern matching: n <= 1 -> 1; ~> n * me(n - 1)
//   - Error handling: x or! "error message"
//   - Guard expressions: x or return y
//   - Loop filtering: @ entity in entities{health > 0}
//   - Default patterns: ~> (catch-all)

// Condition codes for jumps
type JumpCondition int

const (
	JumpEqual          JumpCondition = iota // JE/JZ - equal/zero
	JumpNotEqual                            // JNE/JNZ - not equal/not zero
	JumpGreater                             // JG/JNLE - greater (signed)
	JumpGreaterOrEqual                      // JGE/JNL - greater or equal (signed)
	JumpLess                                // JL/JNGE - less (signed)
	JumpLessOrEqual                         // JLE/JNG - less or equal (signed)
	JumpAbove                               // JA/JNBE - above (unsigned)
	JumpAboveOrEqual                        // JAE/JNB - above or equal (unsigned)
	JumpBelow                               // JB/JNAE - below (unsigned)
	JumpBelowOrEqual                        // JBE/JNA - below or equal (unsigned)
	JumpParity                              // JP - parity/NaN
	JumpNotParity                           // JNP - not parity/not NaN
)

// JumpConditional generates a conditional jump instruction
// offset is the relative offset to jump to (signed, from the end of the instruction)
func (o *Out) JumpConditional(condition JumpCondition, offset int32) {
	if o.backend != nil {
		o.backend.JumpConditional(condition, offset)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.jmpX86Conditional(condition, offset)
	}
}

// JumpUnconditional generates an unconditional jump
func (o *Out) JumpUnconditional(offset int32) {
	if o.backend != nil {
		o.backend.JumpUnconditional(offset)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.jmpX86Unconditional(offset)
	}
}

// x86-64 conditional jump implementation
func (o *Out) jmpX86Conditional(condition JumpCondition, offset int32) {
	var opcode uint8
	var name string

	switch condition {
	case JumpEqual:
		opcode = 0x84
		name = "je"
	case JumpNotEqual:
		opcode = 0x85
		name = "jne"
	case JumpGreater:
		opcode = 0x8F
		name = "jg"
	case JumpGreaterOrEqual:
		opcode = 0x8D
		name = "jge"
	case JumpLess:
		opcode = 0x8C
		name = "jl"
	case JumpLessOrEqual:
		opcode = 0x8E
		name = "jle"
	case JumpAbove:
		opcode = 0x87
		name = "ja"
	case JumpAboveOrEqual:
		opcode = 0x83
		name = "jae"
	case JumpBelow:
		opcode = 0x82
		name = "jb"
	case JumpBelowOrEqual:
		opcode = 0x86
		name = "jbe"
	case JumpParity:
		opcode = 0x8A
		name = "jp"
	case JumpNotParity:
		opcode = 0x8B
		name = "jnp"
	default:
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "%s %d:", name, offset)
	}

	// Use near jump (32-bit offset) with 0x0F prefix
	o.Write(0x0F)
	o.Write(opcode)

	// Write 32-bit offset (little-endian)
	o.Write(uint8(offset & 0xFF))
	o.Write(uint8((offset >> 8) & 0xFF))
	o.Write(uint8((offset >> 16) & 0xFF))
	o.Write(uint8((offset >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 unconditional jump
func (o *Out) jmpX86Unconditional(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "jmp %d:", offset)
	}

	// Use near jump (32-bit offset)
	o.Write(0xE9)

	// Write 32-bit offset (little-endian)
	o.Write(uint8(offset & 0xFF))
	o.Write(uint8((offset >> 8) & 0xFF))
	o.Write(uint8((offset >> 16) & 0xFF))
	o.Write(uint8((offset >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 conditional jump (B.cond)
func (o *Out) jmpARM64Conditional(condition JumpCondition, offset int32) {
	var cond uint32
	var name string

	switch condition {
	case JumpEqual:
		cond = 0x0 // EQ
		name = "b.eq"
	case JumpNotEqual:
		cond = 0x1 // NE
		name = "b.ne"
	case JumpGreater:
		cond = 0xC // GT
		name = "b.gt"
	case JumpGreaterOrEqual:
		cond = 0xA // GE
		name = "b.ge"
	case JumpLess:
		cond = 0xB // LT
		name = "b.lt"
	case JumpLessOrEqual:
		cond = 0xD // LE
		name = "b.le"
	case JumpAbove:
		cond = 0x8 // HI (unsigned higher)
		name = "b.hi"
	case JumpAboveOrEqual:
		cond = 0x2 // CS/HS (unsigned higher or same)
		name = "b.hs"
	case JumpBelow:
		cond = 0x3 // CC/LO (unsigned lower)
		name = "b.lo"
	case JumpBelowOrEqual:
		cond = 0x9 // LS (unsigned lower or same)
		name = "b.ls"
	default:
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "%s %d:", name, offset)
	}

	// B.cond: 01010100 imm19 0 cond
	// Offset is in instructions (4-byte units), signed 19-bit
	immOffset := offset / 4
	if immOffset < -262144 || immOffset > 262143 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
		immOffset = 0
	}

	instr := uint32(0x54000000) |
		((uint32(immOffset) & 0x7FFFF) << 5) |
		cond

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 unconditional jump (B)
func (o *Out) jmpARM64Unconditional(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "b %d:", offset)
	}

	// B: 000101 imm26
	// Offset is in instructions (4-byte units), signed 26-bit
	immOffset := offset / 4
	if immOffset < -33554432 || immOffset > 33554431 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
		immOffset = 0
	}

	instr := uint32(0x14000000) | (uint32(immOffset) & 0x03FFFFFF)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V conditional jump (BEQ, BNE, BLT, BGE, etc.)
func (o *Out) jmpRISCVConditional(condition JumpCondition, offset int32) {
	var funct3 uint32
	var name string
	var rs1, rs2 uint32 = 5, 0 // t0 and zero

	switch condition {
	case JumpEqual: // BEQ t0, zero
		funct3 = 0x0
		name = "beq"
	case JumpNotEqual: // BNE t0, zero
		funct3 = 0x1
		name = "bne"
	case JumpLess: // BLT t0, zero
		funct3 = 0x4
		name = "blt"
	case JumpGreaterOrEqual: // BGE t0, zero
		funct3 = 0x5
		name = "bge"
	case JumpBelow: // BLTU t0, zero (unsigned)
		funct3 = 0x6
		name = "bltu"
	case JumpAboveOrEqual: // BGEU t0, zero (unsigned)
		funct3 = 0x7
		name = "bgeu"
	case JumpGreater: // Use BGE zero, t0 (reverse operands)
		funct3 = 0x5
		name = "blt" // Actually BLT zero, t0
		rs1, rs2 = 0, 5
	case JumpLessOrEqual: // Use BLT zero, t0 or BGE t0, zero
		funct3 = 0x5
		name = "bge" // Actually BGE zero, t0
		rs1, rs2 = 0, 5
	default:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# unsupported condition")
		}
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "%s t0, zero, %d:", name, offset)
	}

	// Branch format: imm[12|10:5] rs2 rs1 funct3 imm[4:1|11] 1100011
	if offset < -4096 || offset > 4094 || (offset&1) != 0 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range or misaligned)")
		}
		offset = 0
	}

	imm12 := (uint32(offset>>12) & 1) << 31
	imm10_5 := (uint32(offset>>5) & 0x3F) << 25
	imm4_1 := (uint32(offset>>1) & 0xF) << 8
	imm11 := (uint32(offset>>11) & 1) << 7

	instr := imm12 | imm10_5 | (rs2 << 20) | (rs1 << 15) |
		(funct3 << 12) | imm4_1 | imm11 | 0x63

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V unconditional jump (JAL with x0 as destination)
func (o *Out) jmpRISCVUnconditional(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "jal zero, %d:", offset)
	}

	// JAL: imm[20|10:1|11|19:12] rd 1101111
	if offset < -1048576 || offset > 1048574 || (offset&1) != 0 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range or misaligned)")
		}
		offset = 0
	}

	imm20 := (uint32(offset>>20) & 1) << 31
	imm10_1 := (uint32(offset>>1) & 0x3FF) << 21
	imm11 := (uint32(offset>>11) & 1) << 20
	imm19_12 := (uint32(offset>>12) & 0xFF) << 12

	instr := imm20 | imm19_12 | imm11 | imm10_1 | 0x6F // rd=0 (x0/zero)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









