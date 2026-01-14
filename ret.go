// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// RET instruction for returning from functions
// Essential for implementing Vibe67's function returns:
//   - Normal returns: return expression
//   - Early returns: x or return y
//   - Guard returns: me.running or return "game stopped"
//   - Error returns: or! "error message"
//   - Implicit returns from pattern matching

// Ret generates a return instruction
func (o *Out) Ret() {
	if o.backend != nil {
		o.backend.Ret()
		return
	}
	// Fallback for x86_64 (uses methods in this file)
	switch o.target.Arch() {
	case ArchX86_64:
		o.retX86()
	}
}

// RetImm generates a return with immediate pop (x86-64 only)
// Used to clean up stack parameters after return
func (o *Out) RetImm(popBytes uint16) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.retX86Imm(popBytes)
	case ArchARM64:
		// ARM64 doesn't have RET with immediate, just do normal RET
		o.retARM64()
	case ArchRiscv64:
		// RISC-V doesn't have RET with immediate, just do normal RET
		o.retRISCV()
	}
}

// x86-64 RET (near return)
func (o *Out) retX86() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ret:")
	}

	// RET (opcode 0xC3)
	o.Write(0xC3)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 RET imm16 (return and pop imm16 bytes from stack)
func (o *Out) retX86Imm(popBytes uint16) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ret %d:", popBytes)
	}

	// RET imm16 (opcode 0xC2)
	o.Write(0xC2)

	// Write 16-bit immediate (little-endian)
	o.Write(uint8(popBytes & 0xFF))
	o.Write(uint8((popBytes >> 8) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 RET (return using link register)
func (o *Out) retARM64() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ret:")
	}

	// RET (actually BR X30, where X30 is the link register)
	// Encoding: 1101011 0 0 10 11111 000000 Rn 00000
	// Rn = 30 (X30/LR)
	instr := uint32(0xD65F03C0)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V RET (pseudo-instruction for JALR x0, x1, 0)
func (o *Out) retRISCV() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ret:")
	}

	// RET is JALR x0, ra, 0
	// Format: imm[11:0] rs1 000 rd 1100111
	// rd = x0 (zero), rs1 = x1 (ra), imm = 0
	instr := uint32(0x67) |
		(1 << 15) // rs1 = ra (x1)
	// rd = x0 (0), funct3 = 000, imm = 0

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









