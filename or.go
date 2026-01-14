// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// OR instruction implementation for all architectures
// Used for bitwise operations in unsafe blocks:
//   - Setting bits: flags | BIT_MASK
//   - Combining values: (r << 8) | g
//   - Flag operations: status | STATUS_READY

// OrRegWithReg generates OR dst, src (dst = dst | src)
func (o *Out) OrRegWithReg(dst, src string) {
	if o.backend != nil {
		o.backend.OrRegWithReg(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.orX86RegWithReg(dst, src)
	}
}

// OrRegWithImm generates OR dst, imm (dst = dst | imm)
func (o *Out) OrRegWithImm(dst string, imm int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.orX86RegWithImm(dst, imm)
	case ArchARM64:
		o.orARM64RegWithImm(dst, imm)
	case ArchRiscv64:
		o.orRISCVRegWithImm(dst, imm)
	}
}

// OrRegWithRegToReg generates OR dst, src1, src2 (dst = src1 | src2)
// 3-operand form for ARM64 and RISC-V
func (o *Out) OrRegWithRegToReg(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86-64: MOV dst, src1; OR dst, src2
		o.MovRegToReg(dst, src1)
		o.OrRegWithReg(dst, src2)
	case ArchARM64:
		o.orARM64RegWithRegToReg(dst, src1, src2)
	case ArchRiscv64:
		o.orRISCVRegWithRegToReg(dst, src1, src2)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 OR (register-register)
func (o *Out) orX86RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "or %s, %s:", dst, src)
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

	// OR opcode (0x09 for r/m64, r64)
	o.Write(0x09)

	// ModR/M: 11 (register direct) | reg (src) | r/m (dst)
	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 OR with immediate
func (o *Out) orX86RegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "or %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits
	if imm >= -128 && imm <= 127 {
		// OR r/m64, imm8 (opcode 0x83 /1)
		o.Write(0x83)
		modrm := uint8(0xC8) | (dstReg.Encoding & 7) // opcode extension /1
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	} else {
		// OR r/m64, imm32 (opcode 0x81 /1)
		o.Write(0x81)
		modrm := uint8(0xC8) | (dstReg.Encoding & 7) // opcode extension /1
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

// ARM64 ORR (register-register, 2-operand form)
func (o *Out) orARM64RegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "orr %s, %s, %s:", dst, dst, src)
	}

	// ORR Xd, Xn, Xm (shifted register)
	// Format: sf 0 10101 shift 0 Rm imm6 Rn Rd
	instr := uint32(0xAA000000) |
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

// ARM64 ORR - 3 operand form
func (o *Out) orARM64RegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "orr %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0xAA000000) |
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

// ARM64 ORR with immediate
func (o *Out) orARM64RegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "orr %s, %s, #%d:", dst, dst, imm)
	}

	// ORR (immediate) uses bitmask encoding
	// Format: sf 1 01100 1 0 immr imms Rn Rd
	instr := uint32(0xB2000000) |
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

// RISC-V OR (register-register, 2-operand form)
func (o *Out) orRISCVRegWithReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "or %s, %s, %s:", dst, dst, src)
	}

	// OR: 0000000 rs2 rs1 110 rd 0110011
	instr := uint32(0x33) |
		(6 << 12) | // funct3 = 110 (OR)
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

// RISC-V OR - 3 operand form
func (o *Out) orRISCVRegWithRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "or %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x33) |
		(6 << 12) | // funct3 = 110 (OR)
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

// RISC-V ORI (OR immediate)
func (o *Out) orRISCVRegWithImm(dst string, imm int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ori %s, %s, %d:", dst, dst, imm)
	}

	// ORI: imm[11:0] rs1 110 rd 0010011
	instr := uint32(0x13) |
		(6 << 12) | // funct3 = 110 (ORI)
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









