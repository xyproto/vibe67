// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

// LockXaddMemReg emits LOCK XADD [base+offset], reg
// Atomically exchanges and adds
// Result: reg gets old value from memory, memory gets incremented by reg
func (o *Out) LockXaddMemReg(base string, offset int, reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.lockXaddMemRegX86(base, offset, reg)
	case ArchARM64:
		compilerError("LOCK XADD not supported on ARM64 (use LDADD instead)")
	case ArchRiscv64:
		compilerError("LOCK XADD not supported on RISC-V (use AMO instructions instead)")
	}
}

func (o *Out) lockXaddMemRegX86(base string, offset int, reg string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lock xadd [%s+%d], %s: ", base, offset, reg)
	}

	baseReg, _ := GetRegister(o.target.Arch(), base)
	srcReg, _ := GetRegister(o.target.Arch(), reg)

	// LOCK prefix
	o.Write(0xF0)

	// REX prefix for 64-bit operation
	// REX.W = 1 for 64-bit operand size
	rex := uint8(0x48) // Base REX with REX.W set
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B for base register
	}
	if srcReg.Encoding >= 8 {
		rex |= 0x04 // REX.R for source register
	}
	o.Write(rex)

	// XADD opcode: 0x0F 0xC1
	o.Write(0x0F)
	o.Write(0xC1)

	// ModR/M byte
	baseEncoding := baseReg.Encoding & 7
	srcEncoding := srcReg.Encoding & 7

	if offset == 0 && baseEncoding != 5 { // Direct addressing (except rbp/r13)
		modrm := uint8(0x00) | (srcEncoding << 3) | baseEncoding
		o.Write(modrm)
		if baseEncoding == 4 { // rsp/r12 needs SIB
			o.Write(0x24) // SIB: scale=0, index=rsp, base=rsp
		}
	} else if offset >= -128 && offset <= 127 { // 8-bit displacement
		modrm := uint8(0x40) | (srcEncoding << 3) | baseEncoding
		o.Write(modrm)
		if baseEncoding == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(int8(offset)))
	} else { // 32-bit displacement
		modrm := uint8(0x80) | (srcEncoding << 3) | baseEncoding
		o.Write(modrm)
		if baseEncoding == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		// Write 32-bit offset in little-endian
		o.Write(uint8(offset))
		o.Write(uint8(offset >> 8))
		o.Write(uint8(offset >> 16))
		o.Write(uint8(offset >> 24))
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "OK\n")
	}
}









