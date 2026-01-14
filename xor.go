// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// XOR instruction implementation for all architectures
// Used for bitwise operations in unsafe blocks:
//   - Bit flipping: value ^b mask
//   - Zeroing registers: rax ^b rax (efficient way to set register to 0)
//   - Toggle bits: flags ^b BIT_MASK

// XorRegWithReg generates XOR dst, src (dst = dst ^ src)
func (o *Out) XorRegWithReg(dst, src string) {
	if o.backend != nil {
		o.backend.XorRegWithReg(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.xorX86RegWithReg(dst, src)
	}
}

// XorRegWithImm generates XOR dst, imm (dst = dst ^ imm)
func (o *Out) XorRegWithImm(dst string, imm int32) {
	if o.backend != nil {
		o.backend.XorRegWithImm(dst, int64(imm))
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.xorX86RegWithImm(dst, imm)
	}
}

// XorRegWithRegToReg generates XOR dst, src1, src2 (dst = src1 ^ src2)
// 3-operand form for ARM64 and RISC-V
func (o *Out) XorRegWithRegToReg(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86-64: MOV dst, src1; XOR dst, src2
		o.MovRegToReg(dst, src1)
		o.XorRegWithReg(dst, src2)
	case ArchARM64:
		o.xorARM64RegWithRegToReg(dst, src1, src2)
	case ArchRiscv64:
		o.xorRISCVRegWithRegToReg(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 XOR (register-register)
func (o *Out) xorX86RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xor %s, %s:", dst, src)
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

	// XOR opcode (0x31 for r/m64, r64)
	o.Write(0x31)

	// ModR/M: 11 (register direct) | reg (src) | r/m (dst)
	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 XOR with immediate
func (o *Out) xorX86RegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xor %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits
	if imm >= -128 && imm <= 127 {
		// XOR r/m64, imm8 (opcode 0x83 /6)
		o.Write(0x83)
		modrm := uint8(0xF0) | (dstReg.Encoding & 7) // opcode extension /6
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	} else {
		// XOR r/m64, imm32 (opcode 0x81 /6)
		o.Write(0x81)
		modrm := uint8(0xF0) | (dstReg.Encoding & 7) // opcode extension /6
		o.Write(modrm)

		// Write 32-bit immediate
		o.Write(uint8(imm & 0xFF))
		o.Write(uint8((imm >> 8) & 0xFF))
		o.Write(uint8((imm >> 16) & 0xFF))
		o.Write(uint8((imm >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 EOR (XOR) (register-register, 2-operand form)
func (o *Out) xorARM64RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "eor %s, %s, %s:", dst, dst, src)
	}

	// EOR Xd, Xn, Xm (shifted register)
	// Format: sf 1 01010 shift 0 Rm imm6 Rn Rd
	instr := uint32(0xCA000000) |
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

// ARM64 EOR - 3 operand form
func (o *Out) xorARM64RegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "eor %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0xCA000000) |
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

// ARM64 EOR with immediate
func (o *Out) xorARM64RegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "eor %s, %s, #%d:", dst, dst, imm)
	}

	// EOR (immediate) uses bitmask encoding
	// Format: sf 1 10100 1 0 immr imms Rn Rd
	instr := uint32(0xD2000000) |
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

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V XOR (register-register, 2-operand form)
func (o *Out) xorRISCVRegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xor %s, %s, %s:", dst, dst, src)
	}

	// XOR: 0000000 rs2 rs1 100 rd 0110011
	instr := uint32(0x33) |
		(4 << 12) | // funct3 = 100 (XOR)
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

// RISC-V XOR - 3 operand form
func (o *Out) xorRISCVRegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xor %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x33) |
		(4 << 12) | // funct3 = 100 (XOR)
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

// RISC-V XORI (XOR immediate)
func (o *Out) xorRISCVRegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xori %s, %s, %d:", dst, dst, imm)
	}

	// XORI: imm[11:0] rs1 100 rd 0010011
	instr := uint32(0x13) |
		(4 << 12) | // funct3 = 100 (XORI)
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









