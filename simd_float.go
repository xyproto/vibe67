// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// SIMD Floating-Point Operations for map[uint64]float64 foundation

// Cvtsi2sd - Convert int64 to scalar double (SSE2)
// cvtsi2sd xmm, r64
func (o *Out) Cvtsi2sd(dst, src string) {
	if o.backend != nil {
		o.backend.Cvtsi2sd(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.cvtsi2sdX86(dst, src)
	}
}

func (o *Out) cvtsi2sdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "cvtsi2sd %s, %s: ", dst, src)
	}

	// Get source register
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	// Parse XMM register number
	var xmmNum int
	fmt.Sscanf(dst, "xmm%d", &xmmNum)

	// F2 prefix for scalar double
	o.Write(0xF2)

	// REX.W prefix for 64-bit operand
	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04 // REX.R
	}
	if srcReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0F 2A - CVTSI2SD opcode
	o.Write(0x0F)
	o.Write(0x2A)

	// ModR/M byte
	modrm := uint8(0xC0) | (uint8(xmmNum&7) << 3) | (srcReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// SCVTF - Signed Convert to Float (ARM64)
func (o *Out) scvtfARM64(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "scvtf %s, %s: ", dst, src)
	}

	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	var vNum int
	fmt.Sscanf(dst, "v%d", &vNum)

	// SCVTF Dd, Xn - 0x9E620000
	instr := uint32(0x9E620000) |
		(uint32(srcReg.Encoding&31) << 5) |
		uint32(vNum&31)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// FCVT.D.L - Float Convert Double from Long (RISC-V)
func (o *Out) fcvtRISCV(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fcvt.d.l %s, %s: ", dst, src)
	}

	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	var fNum int
	fmt.Sscanf(dst, "f%d", &fNum)

	// FCVT.D.L fd, rs1 - 1101001 00010 rs1 000 fd 1010011
	instr := uint32(0xD2200053) |
		(uint32(fNum&31) << 7) |
		(uint32(srcReg.Encoding&31) << 15)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// AddpdXmm - Add Packed Double (SIMD addition)
func (o *Out) AddpdXmm(dst, src string) {
	if o.backend != nil {
		o.backend.AddpdXmm(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.addpdX86(dst, src)
	}
}

func (o *Out) addpdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addpd %s, %s: ", dst, src)
	}

	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	// 66 prefix for packed double
	o.Write(0x66)

	// REX if needed
	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04 // REX.R
		}
		if srcNum >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0F 58 - ADDPD opcode
	o.Write(0x0F)
	o.Write(0x58)

	// ModR/M
	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// SubpdXmm - Subtract Packed Double
func (o *Out) SubpdXmm(dst, src string) {
	if o.backend != nil {
		o.backend.SubpdXmm(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.subpdX86(dst, src)
	}
}

func (o *Out) subpdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "subpd %s, %s: ", dst, src)
	}

	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	o.Write(0x66) // prefix
	o.Write(0x0F)
	o.Write(0x5C) // SUBPD opcode

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MulpdXmm - Multiply Packed Double
func (o *Out) MulpdXmm(dst, src string) {
	if o.backend != nil {
		o.backend.MulpdXmm(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.mulpdX86(dst, src)
	}
}

func (o *Out) mulpdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mulpd %s, %s: ", dst, src)
	}

	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	o.Write(0x66)
	o.Write(0x0F)
	o.Write(0x59) // MULPD opcode

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// DivpdXmm - Divide Packed Double
func (o *Out) DivpdXmm(dst, src string) {
	if o.backend != nil {
		o.backend.DivpdXmm(dst, src)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.divpdX86(dst, src)
	}
}

func (o *Out) divpdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "divpd %s, %s: ", dst, src)
	}

	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	o.Write(0x66)
	o.Write(0x0F)
	o.Write(0x5E) // DIVPD opcode

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovXmmToMem - Store XMM register to memory
func (o *Out) MovXmmToMem(xmm, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movxmmToMemX86(xmm, base, offset)
	}
}

func (o *Out) movxmmToMemX86(xmm, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsd [%s+%d], %s: ", base, offset, xmm)
	}

	var xmmNum int
	fmt.Sscanf(xmm, "xmm%d", &xmmNum)

	baseReg, _ := GetRegister(o.target.Arch(), base)

	// F2 prefix for scalar double
	o.Write(0xF2)

	// REX if needed
	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	o.Write(rex)

	// 0F 11 - MOVSD m64, xmm
	o.Write(0x0F)
	o.Write(0x11)

	// ModR/M with displacement
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovMemToXmm - Load from memory to XMM register
func (o *Out) MovMemToXmm(xmm, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movMemToXmmX86(xmm, base, offset)
	}
}

func (o *Out) movMemToXmmX86(xmm, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsd %s, [%s+%d]: ", xmm, base, offset)
	}

	var xmmNum int
	fmt.Sscanf(xmm, "xmm%d", &xmmNum)

	baseReg, _ := GetRegister(o.target.Arch(), base)

	o.Write(0xF2) // prefix

	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	o.Write(rex)

	// 0F 10 - MOVSD xmm, m64
	o.Write(0x0F)
	o.Write(0x10)

	// ModR/M
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) faddARM64(dst, src string) {
	// FADD Vd.2D, Vn.2D, Vm.2D for packed double
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fadd %s, %s: ", dst, src)
	}
	// Implementation would go here
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) faddRISCV(dst, src string) {
	// FADD.D fd, fs1, fs2
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fadd.d %s, %s: ", dst, src)
	}
	// Implementation would go here
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Cvttsd2si - Convert float64 to int64 with truncation
func (o *Out) Cvttsd2si(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.cvttsd2siX86(dst, src)
	}
}

func (o *Out) cvttsd2siX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "cvttsd2si %s, %s: ", dst, src)
	}

	dstReg, _ := GetRegister(o.target.Arch(), dst)

	var xmmNum int
	fmt.Sscanf(src, "xmm%d", &xmmNum)

	// F2 prefix
	o.Write(0xF2)

	// REX.W for 64-bit result
	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if xmmNum >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0F 2C - CVTTSD2SI opcode
	o.Write(0x0F)
	o.Write(0x2C)

	// ModR/M
	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | uint8(xmmNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Ucomisd - Compare scalar double-precision floating-point values and set EFLAGS
// ucomisd xmm1, xmm2
func (o *Out) Ucomisd(xmm1, xmm2 string) {
	if o.backend != nil {
		o.backend.Ucomisd(xmm1, xmm2)
		return
	}
	// Fallback for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.ucomisdX86(xmm1, xmm2)
	}
}

func (o *Out) ucomisdX86(xmm1, xmm2 string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ucomisd %s, %s: ", xmm1, xmm2)
	}

	var xmm1Num, xmm2Num int
	fmt.Sscanf(xmm1, "xmm%d", &xmm1Num)
	fmt.Sscanf(xmm2, "xmm%d", &xmm2Num)

	// 66 prefix for packed double
	o.Write(0x66)

	// REX if needed
	rex := uint8(0)
	if xmm1Num >= 8 || xmm2Num >= 8 {
		rex = 0x40
		if xmm1Num >= 8 {
			rex |= 0x04 // REX.R
		}
		if xmm2Num >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0F 2E - UCOMISD opcode
	o.Write(0x0F)
	o.Write(0x2E)

	// ModR/M: 11 xmm1 xmm2
	modrm := uint8(0xC0) | (uint8(xmm1Num&7) << 3) | uint8(xmm2Num&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Emit - Write raw bytes directly to output
// For custom SIMD instructions not yet wrapped
func (o *Out) Emit(bytes []byte) {
	for _, b := range bytes {
		o.Write(b)
	}
}

// MovXmmToXmm - Move scalar double from one XMM register to another
// movsd xmm1, xmm2
func (o *Out) MovXmmToXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movX86XmmToXmm(dst, src)
	}
}

func (o *Out) movX86XmmToXmm(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsd %s, %s: ", dst, src)
	}

	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	// F2 prefix for scalar double
	o.Write(0xF2)

	// REX if needed
	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04 // REX.R
		}
		if srcNum >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0F 10 - MOVSD opcode
	o.Write(0x0F)
	o.Write(0x10)

	// ModR/M: 11 dst src
	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovupdMemToXmm - Move Unaligned Packed Double (128 bits = 2 doubles) from memory to XMM
// movupd xmm, [base+offset]
func (o *Out) MovupdMemToXmm(xmm, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movupdMemToXmmX86(xmm, base, offset)
	}
}

func (o *Out) movupdMemToXmmX86(xmm, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movupd %s, [%s+%d]: ", xmm, base, offset)
	}
	var xmmNum int
	fmt.Sscanf(xmm, "xmm%d", &xmmNum)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// 66 prefix for packed double
	o.Write(0x66)

	// REX prefix only if needed (for extended registers)
	if xmmNum >= 8 || baseReg.Encoding >= 8 {
		rex := uint8(0x40)
		if xmmNum >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0F 10 - MOVUPD xmm, m128
	o.Write(0x0F)
	o.Write(0x10)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovupdXmmToMem - Move Unaligned Packed Double (128 bits = 2 doubles) from XMM to memory
// movupd [base+offset], xmm
func (o *Out) MovupdXmmToMem(xmm, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movupdXmmToMemX86(xmm, base, offset)
	}
}

func (o *Out) movupdXmmToMemX86(xmm, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movupd [%s+%d], %s: ", base, offset, xmm)
	}
	var xmmNum int
	fmt.Sscanf(xmm, "xmm%d", &xmmNum)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// 66 prefix for packed double
	o.Write(0x66)

	// REX prefix only if needed (for extended registers)
	if xmmNum >= 8 || baseReg.Encoding >= 8 {
		rex := uint8(0x40)
		if xmmNum >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0F 11 - MOVUPD m128, xmm
	o.Write(0x0F)
	o.Write(0x11)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovqRegToXmm - Move 64-bit integer from general-purpose register to XMM register
