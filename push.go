// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// PUSH/POP instructions for stack management
// Critical for implementing Vibe67's function calls and local variables:
//   - Function prologue/epilogue
//   - Preserving registers across calls
//   - Local variable storage
//   - Function parameter passing
//   - Recursive calls: factorial =~ n { ... me(n - 1) }

// PushReg pushes a register value onto the stack
func (o *Out) PushReg(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.pushX86Reg(reg)
	case ArchARM64:
		o.pushARM64Reg(reg)
	case ArchRiscv64:
		o.pushRISCVReg(reg)
	}
}

// PopReg pops a value from the stack into a register
func (o *Out) PopReg(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.popX86Reg(reg)
	case ArchARM64:
		o.popARM64Reg(reg)
	case ArchRiscv64:
		o.popRISCVReg(reg)
	}
}

// x86-64 PUSH reg
func (o *Out) pushX86Reg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "push %s:", reg)
	}

	// PUSH uses compact encoding: 0x50 + reg
	// For extended registers (R8-R15), need REX prefix
	if regInfo.Encoding >= 8 {
		o.Write(0x41) // REX.B
		o.Write(0x50 + uint8(regInfo.Encoding&7))
	} else {
		o.Write(0x50 + uint8(regInfo.Encoding))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 POP reg
func (o *Out) popX86Reg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "pop %s:", reg)
	}

	// POP uses compact encoding: 0x58 + reg
	if regInfo.Encoding >= 8 {
		o.Write(0x41) // REX.B
		o.Write(0x58 + uint8(regInfo.Encoding&7))
	} else {
		o.Write(0x58 + uint8(regInfo.Encoding))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 doesn't have dedicated PUSH/POP, uses STR/LDR with pre/post-indexing
// We'll use STP/LDP (store/load pair) for efficiency
func (o *Out) pushARM64Reg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "str %s, [sp, #-16]!:", reg)
	}

	// STR Xt, [SP, #-16]! (pre-indexed)
	// Format: 11 111 0 00 00 0 imm9 01 Rn Rt
	// imm9 = -16 (0x1F0 in 9-bit signed)
	instr := uint32(0xF81F0FE0) | uint32(regInfo.Encoding&31)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 POP
func (o *Out) popARM64Reg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ldr %s, [sp], #16:", reg)
	}

	// LDR Xt, [SP], #16 (post-indexed)
	// Format: 11 111 0 00 01 0 imm9 01 Rn Rt
	instr := uint32(0xF84107E0) | uint32(regInfo.Encoding&31)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V doesn't have dedicated PUSH/POP, uses SD/LD with stack pointer
func (o *Out) pushRISCVReg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi sp, sp, -8; sd %s, 0(sp):", reg)
	}

	// ADDI sp, sp, -8
	addiInstr := uint32(0x13) |
		(uint32(0xFF8&0xFFF) << 20) | // imm = -8
		(2 << 15) | // rs1 = sp (x2)
		(2 << 7) // rd = sp (x2)

	o.Write(uint8(addiInstr & 0xFF))
	o.Write(uint8((addiInstr >> 8) & 0xFF))
	o.Write(uint8((addiInstr >> 16) & 0xFF))
	o.Write(uint8((addiInstr >> 24) & 0xFF))

	// SD reg, 0(sp)
	// Format: imm[11:5] rs2 rs1 011 imm[4:0] 0100011
	sdInstr := uint32(0x23) |
		(3 << 12) | // funct3 = 011 (SD)
		(uint32(regInfo.Encoding&31) << 20) | // rs2 = source reg
		(2 << 15) // rs1 = sp (x2)
	// imm = 0

	o.Write(uint8(sdInstr & 0xFF))
	o.Write(uint8((sdInstr >> 8) & 0xFF))
	o.Write(uint8((sdInstr >> 16) & 0xFF))
	o.Write(uint8((sdInstr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V POP
func (o *Out) popRISCVReg(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ld %s, 0(sp); addi sp, sp, 8:", reg)
	}

	// LD reg, 0(sp)
	// Format: imm[11:0] rs1 011 rd 0000011
	ldInstr := uint32(0x03) |
		(3 << 12) | // funct3 = 011 (LD)
		(uint32(regInfo.Encoding&31) << 7) | // rd = dest reg
		(2 << 15) // rs1 = sp (x2)
	// imm = 0

	o.Write(uint8(ldInstr & 0xFF))
	o.Write(uint8((ldInstr >> 8) & 0xFF))
	o.Write(uint8((ldInstr >> 16) & 0xFF))
	o.Write(uint8((ldInstr >> 24) & 0xFF))

	// ADDI sp, sp, 8
	addiInstr := uint32(0x13) |
		(8 << 20) | // imm = 8
		(2 << 15) | // rs1 = sp
		(2 << 7) // rd = sp

	o.Write(uint8(addiInstr & 0xFF))
	o.Write(uint8((addiInstr >> 8) & 0xFF))
	o.Write(uint8((addiInstr >> 16) & 0xFF))
	o.Write(uint8((addiInstr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









