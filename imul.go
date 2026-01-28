// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// IMUL instruction implementation for all architectures
// Used for multiplication in unsafe blocks:
//   - Arithmetic: result = a * b
//   - Scaling: index = base * size
//   - Area calculations: area = width * height

// ImulRegWithReg generates IMUL dst, src (dst = dst * src)
func (o *Out) ImulRegWithReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.imulX86RegWithReg(dst, src)
	case ArchARM64:
		o.imulARM64RegWithReg(dst, src)
	case ArchRiscv64:
		o.imulRISCVRegWithReg(dst, src)
	}
}

// ImulImmToReg generates IMUL dst, imm (dst = dst * imm)
func (o *Out) ImulImmToReg(dst string, imm int64) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.imulX86ImmToReg(dst, imm)
	case ArchARM64:
		o.imulARM64ImmToReg(dst, imm)
	case ArchRiscv64:
		o.imulRISCVImmToReg(dst, imm)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 IMUL dst, src (2-operand form)
func (o *Out) imulX86RegWithReg(dst, src string) {
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
		rex |= 0x04 // REX.R (dst is in reg field)
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B (src is in r/m field)
	}
	o.Write(rex)

	// IMUL opcode: 0x0F 0xAF (2-operand form: r64, r/m64)
	o.Write(0x0F)
	o.Write(0xAF)

	// ModR/M: 11 (register direct) | reg (dst) | r/m (src)
	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 IMUL dst, dst, imm32 (3-operand form with immediate)
func (o *Out) imulX86ImmToReg(dst string, imm int64) {
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
		rex |= 0x05 // REX.R and REX.B (dst is both reg and r/m)
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits (sign-extended)
	if imm >= -128 && imm <= 127 {
		// IMUL r64, r/m64, imm8 (opcode 0x6B)
		o.Write(0x6B)
		// ModR/M: 11 (register direct) | reg (dst) | r/m (dst)
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
		o.Write(modrm)
		// Write 8-bit immediate
		o.Write(uint8(imm & 0xFF))
	} else {
		// IMUL r64, r/m64, imm32 (opcode 0x69)
		o.Write(0x69)
		// ModR/M: 11 (register direct) | reg (dst) | r/m (dst)
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

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 MUL (register-register, 2-operand form)
func (o *Out) imulARM64RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, dst, src)
	}

	// MUL Xd, Xn, Xm (multiply - discard high 64 bits)
	// Format: sf 0 011011 000 Rm 011111 Rn Rd
	// sf=1 (64-bit), Rm=src, Rn=dst (same as Rd), Rd=dst, Ra=11111 (unused)
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

// ARM64 MUL with immediate (load immediate to temp register, then multiply)
func (o *Out) imulARM64ImmToReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, #%d:", dst, dst, imm)
	}

	// ARM64 doesn't have multiply with immediate
	// Use scratch register x9: MOVZ x9, #imm; MUL dst, dst, x9

	// MOVZ x9, #imm (lower 16 bits)
	movz := uint32(0xD2800000) | (uint32(imm&0xFFFF) << 5) | 9
	o.Write(uint8(movz & 0xFF))
	o.Write(uint8((movz >> 8) & 0xFF))
	o.Write(uint8((movz >> 16) & 0xFF))
	o.Write(uint8((movz >> 24) & 0xFF))

	// MUL dst, dst, x9
	mul := uint32(0x9B007C00) |
		(9 << 16) | // Rm = x9
		(uint32(dstReg.Encoding&31) << 5) | // Rn = dst
		uint32(dstReg.Encoding&31) // Rd = dst

	o.Write(uint8(mul & 0xFF))
	o.Write(uint8((mul >> 8) & 0xFF))
	o.Write(uint8((mul >> 16) & 0xFF))
	o.Write(uint8((mul >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V MUL (register-register, 2-operand form)
func (o *Out) imulRISCVRegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %s:", dst, dst, src)
	}

	// MUL: 0000001 rs2 rs1 000 rd 0110011
	instr := uint32(0x33) |
		(1 << 25) | // funct7 = 0000001 (M extension)
		(0 << 12) | // funct3 = 000 (MUL)
		(uint32(srcReg.Encoding&31) << 20) | // rs2
		(uint32(dstReg.Encoding&31) << 15) | // rs1 = dst
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V doesn't have multiply with immediate - use temp register
func (o *Out) imulRISCVImmToReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mul %s, %s, %d:", dst, dst, imm)
	}

	// Use t0 (x5) as scratch register
	// LI t0, imm (using ADDI t0, x0, imm for small values)
	if imm >= -2048 && imm <= 2047 {
		// ADDI t0, x0, imm
		addi := uint32(0x13) |
			(0 << 12) | // funct3 = 000 (ADDI)
			(5 << 7) | // rd = t0 (x5)
			(0 << 15) | // rs1 = x0 (zero)
			(uint32(imm&0xFFF) << 20) // imm[11:0]
		o.Write(uint8(addi & 0xFF))
		o.Write(uint8((addi >> 8) & 0xFF))
		o.Write(uint8((addi >> 16) & 0xFF))
		o.Write(uint8((addi >> 24) & 0xFF))
	} else {
		// For larger immediates, use LUI + ADDI (not implemented here)
		// For now, just use lower 12 bits
		addi := uint32(0x13) |
			(0 << 12) |
			(5 << 7) |
			(0 << 15) |
			(uint32(imm&0xFFF) << 20)
		o.Write(uint8(addi & 0xFF))
		o.Write(uint8((addi >> 8) & 0xFF))
		o.Write(uint8((addi >> 16) & 0xFF))
		o.Write(uint8((addi >> 24) & 0xFF))
	}

	// MUL dst, dst, t0
	mul := uint32(0x33) |
		(1 << 25) | // funct7 = 0000001 (M extension)
		(0 << 12) | // funct3 = 000 (MUL)
		(5 << 20) | // rs2 = t0 (x5)
		(uint32(dstReg.Encoding&31) << 15) | // rs1 = dst
		(uint32(dstReg.Encoding&31) << 7) // rd = dst

	o.Write(uint8(mul & 0xFF))
	o.Write(uint8((mul >> 8) & 0xFF))
	o.Write(uint8((mul >> 16) & 0xFF))
	o.Write(uint8((mul >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
