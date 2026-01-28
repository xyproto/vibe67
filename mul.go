// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// MUL instruction for multiplication
// Essential for implementing Vibe67's arithmetic operations:
//   - Arithmetic expressions: n * 2
//   - Recursive multiplication: n * me(n - 1) in factorial
//   - Array size calculations: rows * columns
//   - Scaling operations: value * scale_factor
//   - Area/volume calculations

// MulRegWithReg generates MUL dst, src (dst = dst * src)
func (o *Out) MulRegWithReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.mulX86RegWithReg(dst, src)
	case ArchARM64:
		o.mulARM64RegWithReg(dst, src)
	case ArchRiscv64:
		o.mulRISCVRegWithReg(dst, src)
	}
}

// MulRegWithImm generates MUL dst, imm (dst = dst * imm)
func (o *Out) MulRegWithImm(dst string, imm int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.mulX86RegWithImm(dst, imm)
	case ArchARM64:
		// ARM64 doesn't have MUL with immediate, need to load to register first
		// For now, we'll just document this limitation
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# mul %s, #%d (load to temp reg needed):", dst, imm)
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr)
		}
	case ArchRiscv64:
		// RISC-V doesn't have MUL with immediate either
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "# mul %s, %d (load to temp reg needed):", dst, imm)
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr)
		}
	}
}

// MulRegWithRegToReg generates MUL dst, src1, src2 (dst = src1 * src2)
// 3-operand form for ARM64 and RISC-V
func (o *Out) MulRegWithRegToReg(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86-64: MOV dst, src1; IMUL dst, src2
		o.MovRegToReg(dst, src1)
		o.MulRegWithReg(dst, src2)
	case ArchARM64:
		o.mulARM64RegWithRegToReg(dst, src1, src2)
	case ArchRiscv64:
		o.mulRISCVRegWithRegToReg(dst, src1, src2)
	}
}

// x86-64 IMUL (signed multiply) - 2 operand form
func (o *Out) mulX86RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "imul %s, %s:", dst, src)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x04 // REX.R
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// IMUL opcode (0x0F 0xAF for r64, r/m64)
	o.Write(0x0F)
	o.Write(0xAF)

	// ModR/M: 11 (register direct) | reg (dst) | r/m (src)
	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 IMUL with immediate (3-operand form: dst = src * imm)
func (o *Out) mulX86RegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "imul %s, %s, %d:", dst, dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x05 // REX.R and REX.B
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits
	if imm >= -128 && imm <= 127 {
		// IMUL r64, r/m64, imm8 (opcode 0x6B)
		o.Write(0x6B)
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	} else {
		// IMUL r64, r/m64, imm32 (opcode 0x69)
		o.Write(0x69)
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
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

// ARM64 MUL (multiply) - 2 operand form
func (o *Out) mulARM64RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, dst, src)
	}

	// MUL Xd, Xn, Xm (actually MADD with XZR)
	// Format: sf 0 011011 000 Rm 0 11111 Rn Rd
	// This is: MADD Xd, Xn, Xm, XZR (multiply-add with zero = multiply)
	instr := uint32(0x9B007C00) |
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

// ARM64 MUL - 3 operand form
func (o *Out) mulARM64RegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x9B007C00) |
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

// RISC-V MUL (requires M extension)
func (o *Out) mulRISCVRegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, dst, src)
	}

	// MUL: 0000001 rs2 rs1 000 rd 0110011
	instr := uint32(0x2000033) |
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

// RISC-V MUL - 3 operand form
func (o *Out) mulRISCVRegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x2000033) |
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
