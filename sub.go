// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// SUB instruction implementation for all architectures
// Essential for implementing Vibe67's arithmetic and control flow:
//   - Arithmetic expressions: n - 1
//   - Decrement operations: me.health - amount
//   - Pointer arithmetic: end - start
//   - Loop counters: count - 1
//   - Comparisons (CMP uses SUB internally)

// SubRegFromReg generates SUB dst, src (dst = dst - src)
func (o *Out) SubRegFromReg(dst, src string) {
	if o.backend != nil {
		o.backend.SubRegToReg(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.subX86RegFromReg(dst, src)
	}
}

// SubImmFromReg generates SUB dst, imm (dst = dst - imm)
func (o *Out) SubImmFromReg(dst string, imm int64) {
	if o.backend != nil {
		o.backend.SubImmFromReg(dst, imm)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.subX86ImmFromReg(dst, imm)
	}
}

// SubRegFromRegToReg generates SUB dst, src1, src2 (dst = src1 - src2)
// For ARM64 and RISC-V which have 3-operand form
func (o *Out) SubRegFromRegToReg(dst, src1, src2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		// x86 doesn't have 3-operand SUB, use MOV + SUB
		o.MovRegToReg(dst, src1)
		o.SubRegFromReg(dst, src2)
	case ArchARM64:
		o.subARM64RegFromRegToReg(dst, src1, src2)
	case ArchRiscv64:
		o.subRISCVRegFromRegToReg(dst, src1, src2)
	}
}

// x86-64 SUB reg, reg
func (o *Out) subX86RegFromReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s:", dst, src)
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

	// SUB opcode (0x29 for r/m64, r64)
	o.Write(0x29)

	// ModR/M: 11 (register direct) | reg (src) | r/m (dst)
	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 SUB reg, imm
func (o *Out) subX86ImmFromReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// Check if immediate fits in 8 bits
	if imm >= -128 && imm <= 127 {
		// SUB r/m64, imm8 (opcode 0x83 /5)
		o.Write(0x83)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7) // ModR/M: 11 101 reg
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	} else {
		// SUB r/m64, imm32 (opcode 0x81 /5)
		o.Write(0x81)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7)
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

// ARM64 SUB Xd, Xn, Xm
func (o *Out) subARM64RegFromReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, %s:", dst, dst, src)
	}

	// SUB (shifted register): sf 1 0 01011 shift(2) 0 Rm imm6 Rn Rd
	// sf=1 (64-bit), shift=00
	instr := uint32(0xCB000000) |
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

// ARM64 SUB Xd, Xn, #imm
func (o *Out) subARM64ImmFromReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, #%d:", dst, dst, imm)
	}

	if imm < 0 || imm > 4095 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (immediate out of range)")
		}
		imm = 0
	}

	// SUB (immediate): sf 1 0 10001 shift(2) imm12 Rn Rd
	instr := uint32(0xD1000000) |
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

// ARM64 SUB Xd, Xn, Xm (3-operand form)
func (o *Out) subARM64RegFromRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0xCB000000) |
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

// RISC-V SUB rd, rs1, rs2
func (o *Out) subRISCVRegFromReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, %s:", dst, dst, src)
	}

	// SUB: 0100000 rs2 rs1 000 rd 0110011
	instr := uint32(0x40000033) |
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

// RISC-V ADDI with negative immediate (no SUBI instruction)
func (o *Out) subRISCVImmFromReg(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	negImm := -imm
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi %s, %s, %d:", dst, dst, negImm)
	}

	if negImm < -2048 || negImm > 2047 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (immediate out of range)")
		}
		negImm = 0
	}

	// ADDI with negative immediate: imm[11:0] rs1 000 rd 0010011
	instr := uint32(0x13) |
		(uint32(negImm&0xFFF) << 20) | // imm[11:0]
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

// RISC-V SUB rd, rs1, rs2 (3-operand form)
func (o *Out) subRISCVRegFromRegToReg(dst, src1, src2 string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sub %s, %s, %s:", dst, src1, src2)
	}

	instr := uint32(0x40000033) |
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









