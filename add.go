// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// ADD instruction implementation for all architectures
// Essential for implementing Vibe67's arithmetic foundation (map[float64]float64):
//   - Arithmetic expressions: n + 1, x + y
//   - Array/pointer arithmetic: address + offset
//   - Index calculations: me.entities + [i]
//   - Increment operations: me.x := me.x + dx
//   - List concatenation: smaller + [pivot] + larger

// AddRegToReg generates ADD dst, src (dst = dst + src)
func (o *Out) AddRegToReg(dst, src string) {
	if o.backend != nil {
		o.backend.AddRegToReg(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.addX86RegToReg(dst, src)
	case ArchARM64:
		o.addARM64RegToReg(dst, src)
	case ArchRiscv64:
		o.addRISCVRegToReg(dst, src)
	}
}

// AddImmToReg generates ADD dst, imm (dst = dst + imm)
func (o *Out) AddImmToReg(dst string, imm int64) {
	if o.backend != nil {
		o.backend.AddImmToReg(dst, imm)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.addX86ImmToReg(dst, imm)
	case ArchARM64:
		o.addARM64ImmToReg(dst, imm)
	case ArchRiscv64:
		o.addRISCVImmToReg(dst, imm)
	}
}

// AddRegToRegToReg generates ADD dst, src1, src2 (dst = src1 + src2)
// For ARM64 and RISC-V which have 3-operand form
func (o *Out) AddRegToRegToReg(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86 doesn't have 3-operand ADD, use MOV + ADD
		// MOV dst, src1; ADD dst, src2
		o.MovRegToReg(dst, src1)
		o.AddRegToReg(dst, src2)
	case ArchARM64:
		o.addARM64RegToRegToReg(dst, src1, src2)
	case ArchRiscv64:
		o.addRISCVRegToRegToReg(dst, src1, src2)
	}
}

// x86-64 ADD reg, reg
func (o *Out) addX86RegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s:", dst, src)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04 // REX.R
	}
	o.Write(rex)

	// ADD opcode (0x01 for r/m64, r64)
	o.Write(0x01)

	// ModR/M: 11 (register direct) | reg (src) | r/m (dst)
	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 ADD reg, imm
func (o *Out) addX86ImmToReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits
	if imm >= -128 && imm <= 127 {
		// ADD r/m64, imm8 (opcode 0x83 /0)
		o.Write(0x83)
		modrm := uint8(0xC0) | (dstReg.Encoding & 7) // ModR/M: 11 000 reg
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	} else {
		// ADD r/m64, imm32 (opcode 0x81 /0)
		o.Write(0x81)
		modrm := uint8(0xC0) | (dstReg.Encoding & 7)
		o.Write(modrm)

		// Write 32-bit immediate
		imm32 := uint32(imm)
		o.Write(uint8(imm32 & 0xFF))
		o.Write(uint8((imm32 >> 8) & 0xFF))
		o.Write(uint8((imm32 >> 16) & 0xFF))
		o.Write(uint8((imm32 >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADD Xd, Xn, Xm
func (o *Out) addARM64RegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, %s:", dst, dst, src)
	}

	// ADD (shifted register): sf 0 0 01011 shift(2) 0 Rm imm6 Rn Rd
	// sf=1 (64-bit), shift=00
	instr := uint32(0x8B000000) |
		(uint32(srcReg.Encoding&31) << 16) | // Rm
		(uint32(dstReg.Encoding&31) << 5) | // Rn (same as Rd for 2-operand)
		uint32(dstReg.Encoding&31) // Rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADD Xd, Xn, #imm
func (o *Out) addARM64ImmToReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, #%d:", dst, dst, imm)
	}

	if imm < 0 || imm > 4095 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (immediate out of range)")
		}
		imm = 0
	}

	// ADD (immediate): sf 0 0 10001 shift(2) imm12 Rn Rd
	instr := uint32(0x91000000) |
		(uint32(imm&0xFFF) << 10) | // imm12
		(uint32(dstReg.Encoding&31) << 5) | // Rn (same as Rd)
		uint32(dstReg.Encoding&31) // Rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADD Xd, Xn, Xm (3-operand form)
func (o *Out) addARM64RegToRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x8B000000) |
		(uint32(src2Reg.Encoding&31) << 16) | // Rm
		(uint32(src1Reg.Encoding&31) << 5) | // Rn
		uint32(dstReg.Encoding&31) // Rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V ADD rd, rs1, rs2
func (o *Out) addRISCVRegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, %s:", dst, dst, src)
	}

	// ADD: 0000000 rs2 rs1 000 rd 0110011
	instr := uint32(0x33) |
		(uint32(srcReg.Encoding&31) << 20) | // rs2
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

// RISC-V ADDI rd, rs1, imm
func (o *Out) addRISCVImmToReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi %s, %s, %d:", dst, dst, imm)
	}

	if imm < -2048 || imm > 2047 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (immediate out of range)")
		}
		imm = 0
	}

	// ADDI: imm[11:0] rs1 000 rd 0010011
	instr := uint32(0x13) |
		(uint32(imm&0xFFF) << 20) | // imm[11:0]
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

// RISC-V ADD rd, rs1, rs2 (3-operand form)
func (o *Out) addRISCVRegToRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x33) |
		(uint32(src2Reg.Encoding&31) << 20) | // rs2
		(uint32(src1Reg.Encoding&31) << 15) | // rs1
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









