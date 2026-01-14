// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

// Shift instruction implementations for all architectures
// Used for bit manipulation in unsafe blocks:
//   - Logical shift left: result = value << count
//   - Logical shift right: result = value >> count
//   - Arithmetic shift right: result = value >>a count

// ShlRegByImm generates SHL dst, imm (logical shift left by immediate)
func (o *Out) ShlRegByImm(dst string, imm int64) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shlX86RegByImm(dst, imm)
	case ArchARM64:
		o.shlARM64RegByImm(dst, imm)
	case ArchRiscv64:
		o.shlRISCVRegByImm(dst, imm)
	}
}

// ShlRegByReg generates SHL dst, src (logical shift left by register)
func (o *Out) ShlRegByReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shlX86RegByReg(dst, src)
	case ArchARM64:
		o.shlARM64RegByReg(dst, src)
	case ArchRiscv64:
		o.shlRISCVRegByReg(dst, src)
	}
}

// ShrRegByImm generates SHR dst, imm (logical shift right by immediate)
func (o *Out) ShrRegByImm(dst string, imm int64) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shrX86RegByImm(dst, imm)
	case ArchARM64:
		o.shrARM64RegByImm(dst, imm)
	case ArchRiscv64:
		o.shrRISCVRegByImm(dst, imm)
	}
}

// ShrRegByReg generates SHR dst, src (logical shift right by register)
func (o *Out) ShrRegByReg(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shrX86RegByReg(dst, src)
	case ArchARM64:
		o.shrARM64RegByReg(dst, src)
	case ArchRiscv64:
		o.shrRISCVRegByReg(dst, src)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 SHL by immediate
func (o *Out) shlX86RegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shl %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	if imm == 1 {
		// SHL r/m64, 1 (opcode 0xD1 /4)
		o.Write(0xD1)
		modrm := uint8(0xE0) | (dstReg.Encoding & 7)
		o.Write(modrm)
	} else {
		// SHL r/m64, imm8 (opcode 0xC1 /4)
		o.Write(0xC1)
		modrm := uint8(0xE0) | (dstReg.Encoding & 7)
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 SHL by register (uses CL register)
func (o *Out) shlX86RegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shl %s, %s:", dst, src)
	}

	// x86-64 requires shift count in CL register
	if src != "rcx" {
		o.MovRegToReg("rcx", src)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// SHL r/m64, CL (opcode 0xD3 /4)
	o.Write(0xD3)
	modrm := uint8(0xE0) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 SHR by immediate
func (o *Out) shrX86RegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shr %s, %d:", dst, imm)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	if imm == 1 {
		// SHR r/m64, 1 (opcode 0xD1 /5)
		o.Write(0xD1)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7)
		o.Write(modrm)
	} else {
		// SHR r/m64, imm8 (opcode 0xC1 /5)
		o.Write(0xC1)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7)
		o.Write(modrm)
		o.Write(uint8(imm & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 SHR by register (uses CL register)
func (o *Out) shrX86RegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shr %s, %s:", dst, src)
	}

	// x86-64 requires shift count in CL register
	if src != "rcx" {
		o.MovRegToReg("rcx", src)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// SHR r/m64, CL (opcode 0xD3 /5)
	o.Write(0xD3)
	modrm := uint8(0xE8) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 LSL (logical shift left) by immediate
func (o *Out) shlARM64RegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lsl %s, %s, #%d:", dst, dst, imm)
	}

	// LSL Xd, Xn, #imm (alias for UBFM)
	// Format: sf=1 opc=10 N=1 immr imms Rn Rd
	// LSL is encoded as: UBFM Xd, Xn, #(-imm mod 64), #(63-imm)
	immr := uint32((64 - imm) & 63)
	imms := uint32((63 - imm) & 63)

	instr := uint32(0xD3400000) |
		(immr << 16) |
		(imms << 10) |
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

// ARM64 LSL (logical shift left) by register
func (o *Out) shlARM64RegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lsl %s, %s, %s:", dst, dst, src)
	}

	// LSLV Xd, Xn, Xm (logical shift left variable)
	instr := uint32(0x9AC02000) |
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

// ARM64 LSR (logical shift right) by immediate
func (o *Out) shrARM64RegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lsr %s, %s, #%d:", dst, dst, imm)
	}

	// LSR Xd, Xn, #imm (alias for UBFM)
	// Format: UBFM Xd, Xn, #imm, #63
	instr := uint32(0xD3400000) |
		(uint32(imm&63) << 16) |
		(63 << 10) |
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

// ARM64 LSR (logical shift right) by register
func (o *Out) shrARM64RegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lsr %s, %s, %s:", dst, dst, src)
	}

	// LSRV Xd, Xn, Xm (logical shift right variable)
	instr := uint32(0x9AC02400) |
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

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V SLLI (shift left logical immediate)
func (o *Out) shlRISCVRegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "slli %s, %s, %d:", dst, dst, imm)
	}

	// SLLI: imm[11:6]=000000 imm[5:0] rs1 001 rd 0010011
	instr := uint32(0x13) |
		(1 << 12) | // funct3 = 001
		(uint32(imm&0x3F) << 20) | // shamt (6 bits for 64-bit)
		(uint32(dstReg.Encoding&31) << 15) | // rs1
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V SLL (shift left logical)
func (o *Out) shlRISCVRegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sll %s, %s, %s:", dst, dst, src)
	}

	// SLL: 0000000 rs2 rs1 001 rd 0110011
	instr := uint32(0x33) |
		(1 << 12) | // funct3 = 001
		(uint32(srcReg.Encoding&31) << 20) | // rs2
		(uint32(dstReg.Encoding&31) << 15) | // rs1
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V SRLI (shift right logical immediate)
func (o *Out) shrRISCVRegByImm(dst string, imm int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "srli %s, %s, %d:", dst, dst, imm)
	}

	// SRLI: imm[11:6]=000000 imm[5:0] rs1 101 rd 0010011
	instr := uint32(0x13) |
		(5 << 12) | // funct3 = 101
		(uint32(imm&0x3F) << 20) | // shamt (6 bits for 64-bit)
		(uint32(dstReg.Encoding&31) << 15) | // rs1
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V SRL (shift right logical)
func (o *Out) shrRISCVRegByReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "srl %s, %s, %s:", dst, dst, src)
	}

	// SRL: 0000000 rs2 rs1 101 rd 0110011
	instr := uint32(0x33) |
		(5 << 12) | // funct3 = 101
		(uint32(srcReg.Encoding&31) << 20) | // rs2
		(uint32(dstReg.Encoding&31) << 15) | // rs1
		(uint32(dstReg.Encoding&31) << 7) // rd

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









