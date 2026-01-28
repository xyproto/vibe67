// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// DEC instruction - decrement register by 1
// Used for counters and barrier synchronization

func (o *Out) DecReg(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.decX86Reg(reg)
	case ArchARM64:
		o.decARM64Reg(reg)
	case ArchRiscv64:
		o.decRISCVReg(reg)
	}
}

// x86-64 DEC instruction: DEC r64
func (o *Out) decX86Reg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "dec %s: ", reg)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (regInfo.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// DEC opcode (0xFF /1)
	o.Write(0xFF)

	// ModR/M byte: 11 001 reg (register direct mode, opcode extension /1)
	modrm := uint8(0xC8) | (regInfo.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 SUB immediate: SUB Xd, Xn, #1
func (o *Out) decARM64Reg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, #1: ", reg, reg)
	}

	// SUB Xd, Xn, #imm12
	// Format: sf 1 0 10001 shift(2) imm12(12) Rn(5) Rd(5)
	// sf=1 (64-bit), shift=00, imm12=1
	instr := uint32(0xD1000000) | // SUB immediate base
		(1 << 10) | // imm12 = 1
		(uint32(regInfo.Encoding&31) << 5) | // Rn (source)
		(uint32(regInfo.Encoding & 31)) // Rd (dest, same as source)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V ADDI: ADDI rd, rs1, -1
func (o *Out) decRISCVReg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi %s, %s, -1: ", reg, reg)
	}

	// ADDI rd, rs1, imm
	// Format: imm[11:0] rs1 000 rd 0010011
	// -1 in 12-bit two's complement is 0xFFF
	instr := uint32(0x13) |
		(0xFFF << 20) | // imm[11:0] = -1 (0xFFF)
		(uint32(regInfo.Encoding&31) << 15) | // rs1
		(uint32(regInfo.Encoding&31) << 7) // rd (same as rs1)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
