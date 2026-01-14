// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// DIV/IDIV instruction implementation for all architectures
// Used for division in unsafe blocks:
//   - Arithmetic: result = a / b
// Note: x86-64 division produces quotient in rax, remainder in rdx

// DivRegByReg generates DIV dst, src (dst = dst / src)
func (o *Out) DivRegByReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.divX86RegByReg(dst, src)
	case ArchARM64:
		o.divARM64RegByReg(dst, src)
	case ArchRiscv64:
		o.divRISCVRegByReg(dst, src)
	}
}

// DivRegByImm generates DIV dst, imm (dst = dst / imm)
func (o *Out) DivRegByImm(dst string, imm int64) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.divX86RegByImm(dst, imm)
	case ArchARM64:
		o.divARM64RegByImm(dst, imm)
	case ArchRiscv64:
		o.divRISCVRegByImm(dst, imm)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 IDIV (signed division)
func (o *Out) divX86RegByReg(dst, src string) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "idiv %s:", src)
	}

	// If dst is not RAX, move it there
	if dst != "rax" {
		o.MovRegToReg("rax", dst)
	}

	// CQO: Sign-extend RAX into RDX:RAX
	o.Write(0x48) // REX.W
	o.Write(0x99) // CQO

	// IDIV r/m64 (opcode 0xF7 /7)
	rex := uint8(0x48)
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0xF7)
	modrm := uint8(0xF8) | (srcReg.Encoding & 7)
	o.Write(modrm)

	// If dst was not RAX, move quotient back
	if dst != "rax" {
		o.MovRegToReg(dst, "rax")
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 division by immediate
func (o *Out) divX86RegByImm(dst string, imm int64) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "idiv %s by %d:", dst, imm)
	}

	o.MovImmToReg("r11", fmt.Sprintf("%d", imm))
	o.divX86RegByReg(dst, "r11")

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 SDIV (signed division)
func (o *Out) divARM64RegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sdiv %s, %s, %s:", dst, dst, src)
	}

	// SDIV Xd, Xn, Xm
	instr := uint32(0x9AC00C00) |
		(uint32(srcReg.Encoding&31) << 16) |
		(uint32(dstReg.Encoding&31) << 5) |
		uint32(dstReg.Encoding&31)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 division by immediate
func (o *Out) divARM64RegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sdiv %s, %s, #%d:", dst, dst, imm)
	}

	// Load immediate into x9
	movz := uint32(0xD2800000) | (uint32(imm&0xFFFF) << 5) | 9
	o.Write(uint8(movz & 0xFF))
	o.Write(uint8((movz >> 8) & 0xFF))
	o.Write(uint8((movz >> 16) & 0xFF))
	o.Write(uint8((movz >> 24) & 0xFF))

	// SDIV dst, dst, x9
	sdiv := uint32(0x9AC00C00) |
		(9 << 16) |
		(uint32(dstReg.Encoding&31) << 5) |
		uint32(dstReg.Encoding&31)

	o.Write(uint8(sdiv & 0xFF))
	o.Write(uint8((sdiv >> 8) & 0xFF))
	o.Write(uint8((sdiv >> 16) & 0xFF))
	o.Write(uint8((sdiv >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V DIV (signed division)
func (o *Out) divRISCVRegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "div %s, %s, %s:", dst, dst, src)
	}

	// DIV: 0000001 rs2 rs1 100 rd 0110011
	instr := uint32(0x33) |
		(1 << 25) |
		(4 << 12) |
		(uint32(srcReg.Encoding&31) << 20) |
		(uint32(dstReg.Encoding&31) << 15) |
		(uint32(dstReg.Encoding&31) << 7)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V division by immediate
func (o *Out) divRISCVRegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "div %s, %s, %d:", dst, dst, imm)
	}

	// Load immediate into t0 (x5)
	addi := uint32(0x13) |
		(0 << 12) |
		(5 << 7) |
		(0 << 15) |
		(uint32(imm&0xFFF) << 20)

	o.Write(uint8(addi & 0xFF))
	o.Write(uint8((addi >> 8) & 0xFF))
	o.Write(uint8((addi >> 16) & 0xFF))
	o.Write(uint8((addi >> 24) & 0xFF))

	// DIV dst, dst, t0
	div := uint32(0x33) |
		(1 << 25) |
		(4 << 12) |
		(5 << 20) |
		(uint32(dstReg.Encoding&31) << 15) |
		(uint32(dstReg.Encoding&31) << 7)

	o.Write(uint8(div & 0xFF))
	o.Write(uint8((div >> 8) & 0xFF))
	o.Write(uint8((div >> 16) & 0xFF))
	o.Write(uint8((div >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









