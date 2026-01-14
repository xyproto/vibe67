// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// INC instruction - increment register by 1
// Used for loop counters and iterator variables

func (o *Out) IncReg(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.incX86Reg(reg)
	case ArchARM64:
		o.incARM64Reg(reg)
	case ArchRiscv64:
		o.incRISCVReg(reg)
	}
}

// x86-64 INC instruction: INC r64
func (o *Out) incX86Reg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "inc %s: ", reg)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (regInfo.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// INC opcode (0xFF /0)
	o.Write(0xFF)

	// ModR/M byte: 11 000 reg (register direct mode, opcode extension /0)
	modrm := uint8(0xC0) | (regInfo.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADD immediate: ADD Xd, Xn, #1
func (o *Out) incARM64Reg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, #1: ", reg, reg)
	}

	// ADD Xd, Xn, #imm12
	// Format: sf 0 0 10001 shift(2) imm12(12) Rn(5) Rd(5)
	// sf=1 (64-bit), shift=00, imm12=1
	instr := uint32(0x91000000) | // ADD immediate base
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

// RISC-V ADDI: ADDI rd, rs1, 1
func (o *Out) incRISCVReg(reg string) {
	regInfo, ok := GetRegister(o.target.Arch(), reg)
	if !ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi %s, %s, 1: ", reg, reg)
	}

	// ADDI rd, rs1, imm
	// Format: imm[11:0] rs1 000 rd 0010011
	instr := uint32(0x13) |
		(1 << 20) | // imm[11:0] = 1
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









