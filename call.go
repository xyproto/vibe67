// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

// CALL instruction for function calls
// Essential for implementing Vibe67's function system:
//   - Direct function calls: process_data(validated)
//   - Recursive calls: me(n - 1) in factorial
//   - Method calls: entity.update()
//   - Library function calls: create_user(user_data)
//   - Lambda calls: (x) -> x + 1

// CallRelative generates a relative CALL instruction
// offset is the relative offset to the function (from end of instruction)
func (o *Out) CallRelative(offset int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.callX86Relative(offset)
	case ArchARM64:
		o.callARM64Relative(offset)
	case ArchRiscv64:
		o.callRISCVRelative(offset)
	}
}

// CallRegister generates a CALL to address in register (indirect call)
func (o *Out) CallRegister(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.callX86Register(reg)
	case ArchARM64:
		o.callARM64Register(reg)
	case ArchRiscv64:
		o.callRISCVRegister(reg)
	}
}

// x86-64 CALL relative
func (o *Out) callX86Relative(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "call %d:", offset)
	}

	// CALL rel32 (opcode 0xE8)
	o.Write(0xE8)

	// Write 32-bit offset (little-endian)
	o.Write(uint8(offset & 0xFF))
	o.Write(uint8((offset >> 8) & 0xFF))
	o.Write(uint8((offset >> 16) & 0xFF))
	o.Write(uint8((offset >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 CALL register (indirect)
func (o *Out) callX86Register(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "call %s:", reg)
	}

	// CALL r/m64 (opcode 0xFF /2)
	// Need REX prefix for 64-bit
	rex := uint8(0x48)
	if regInfo.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	o.Write(0xFF)

	// ModR/M: 11 010 reg (register indirect, opcode extension /2)
	modrm := uint8(0xD0) | (regInfo.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 BL (Branch with Link) - relative call
func (o *Out) callARM64Relative(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "bl %d:", offset)
	}

	// BL: 100101 imm26
	// Offset is in instructions (4-byte units), signed 26-bit
	immOffset := offset / 4
	if immOffset < -33554432 || immOffset > 33554431 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
		immOffset = 0
	}

	instr := uint32(0x94000000) | (uint32(immOffset) & 0x03FFFFFF)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 BLR (Branch with Link to Register) - indirect call
func (o *Out) callARM64Register(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "blr %s:", reg)
	}

	// BLR: 1101011 0 0 01 11111 000000 Rn 00000
	instr := uint32(0xD63F0000) | (uint32(regInfo.Encoding&31) << 5)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V JAL (Jump and Link) - relative call
func (o *Out) callRISCVRelative(offset int32) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "jal ra, %d:", offset)
	}

	// JAL: imm[20|10:1|11|19:12] rd 1101111
	// rd = ra (x1) for return address
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

	instr := imm20 | imm19_12 | imm11 | imm10_1 | (1 << 7) | 0x6F // rd=1 (ra)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V JALR (Jump and Link Register) - indirect call
func (o *Out) callRISCVRegister(reg string) {
	regInfo, regOk := GetRegister(o.target.Arch(), reg)
	if !regOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "jalr ra, %s, 0:", reg)
	}

	// JALR: imm[11:0] rs1 000 rd 1100111
	// rd = ra (x1), rs1 = target register, imm = 0
	instr := uint32(0x67) |
		(1 << 7) | // rd = ra (x1)
		(uint32(regInfo.Encoding&31) << 15) // rs1 = target
	// funct3 = 000, imm = 0

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









