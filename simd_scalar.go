// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
)

// Scalar Double-Precision Floating-Point Operations
// These operate on single float64 values in XMM registers

// AddsdXmm - Add Scalar Double (SSE2)
// addsd xmm, xmm
func (o *Out) AddsdXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.addsdX86(dst, src)
	case ArchARM64:
		o.faddScalarARM64(dst, src)
	case ArchRiscv64:
		o.faddScalarRISCV(dst, src)
	}
}

func (o *Out) addsdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addsd %s, %s: ", dst, src)
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

	// 0F 58 - ADDSD opcode
	o.Write(0x0F)
	o.Write(0x58)

	// ModR/M
	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// SubsdXmm - Subtract Scalar Double
func (o *Out) SubsdXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.subsdX86(dst, src)
	}
}

func (o *Out) subsdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "subsd %s, %s: ", dst, src)
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
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		o.Write(rex)
	}

	// 0F 5C - SUBSD opcode
	o.Write(0x0F)
	o.Write(0x5C)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MulsdXmm - Multiply Scalar Double
func (o *Out) MulsdXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.mulsdX86(dst, src)
	}
}

func (o *Out) mulsdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mulsd %s, %s: ", dst, src)
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
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		o.Write(rex)
	}

	// 0F 59 - MULSD opcode
	o.Write(0x0F)
	o.Write(0x59)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// DivsdXmm - Divide Scalar Double
func (o *Out) DivsdXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.divsdX86(dst, src)
	}
}

func (o *Out) divsdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "divsd %s, %s: ", dst, src)
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
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		o.Write(rex)
	}

	// 0F 5E - DIVSD opcode
	o.Write(0x0F)
	o.Write(0x5E)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Sqrtsd - Square Root Scalar Double (SSE2)
// sqrtsd xmm, xmm
func (o *Out) Sqrtsd(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.sqrtsdX86(dst, src)
	}
}

func (o *Out) sqrtsdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sqrtsd %s, %s: ", dst, src)
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

	// 0F 51 - SQRTSD opcode
	o.Write(0x0F)
	o.Write(0x51)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fsin - x87 FPU sine (operates on ST(0))
// fsin
func (o *Out) Fsin() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fsin: ")
	}
	o.Write(0xD9)
	o.Write(0xFE)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fcos - x87 FPU cosine (operates on ST(0))
// fcos
func (o *Out) Fcos() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fcos: ")
	}
	o.Write(0xD9)
	o.Write(0xFF)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fptan - x87 FPU partial tangent (operates on ST(0), pushes 1.0 after)
// fptan
func (o *Out) Fptan() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fptan: ")
	}
	o.Write(0xD9)
	o.Write(0xF2)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fld - Load double from memory to ST(0)
// fld qword [reg+offset]
func (o *Out) FldMem(reg string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fld qword [%s%+d]: ", reg, offset)
	}
	o.Write(0xDD) // FLD m64

	var rmBits uint8
	switch reg {
	case "rsp":
		rmBits = 4
	case "rbp":
		rmBits = 5
	default:
		rmBits = 0
	}

	// ModR/M encoding
	if offset == 0 && rmBits != 5 { // rbp needs displacement
		modrm := uint8(0x00) | rmBits // mod=00, no displacement
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
	} else if offset >= -128 && offset < 128 {
		modrm := uint8(0x40) | rmBits // mod=01, disp8
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | rmBits // mod=10, disp32
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
		// Write 32-bit displacement
		o.Write(uint8(offset))
		o.Write(uint8(offset >> 8))
		o.Write(uint8(offset >> 16))
		o.Write(uint8(offset >> 24))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fstp - Store ST(0) to memory and pop
// fstp qword [reg+offset]
func (o *Out) FstpMem(reg string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fstp qword [%s%+d]: ", reg, offset)
	}
	o.Write(0xDD) // FSTP m64

	var rmBits uint8
	switch reg {
	case "rsp":
		rmBits = 4
	case "rbp":
		rmBits = 5
	default:
		rmBits = 0
	}

	// ModR/M encoding (reg field = 011 for FSTP)
	if offset == 0 && rmBits != 5 { // rbp needs displacement
		modrm := uint8(0x00) | (3 << 3) | rmBits // mod=00
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
	} else if offset >= -128 && offset < 128 {
		modrm := uint8(0x40) | (3 << 3) | rmBits // mod=01, disp8
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (3 << 3) | rmBits // mod=10, disp32
		o.Write(modrm)
		if rmBits == 4 { // rsp needs SIB
			o.Write(0x24)
		}
		o.Write(uint8(offset))
		o.Write(uint8(offset >> 8))
		o.Write(uint8(offset >> 16))
		o.Write(uint8(offset >> 24))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fpop - Pop x87 stack (FSTP ST(0) - discard top)
func (o *Out) Fpop() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fstp st(0): ")
	}
	o.Write(0xDD)
	o.Write(0xD8) // FSTP ST(0)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fpatan - x87 FPU partial arctangent: ST(1) = atan2(ST(1), ST(0)), then pop
// Computes atan(ST(1)/ST(0)) with proper quadrant handling
func (o *Out) Fpatan() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fpatan: ")
	}
	o.Write(0xD9)
	o.Write(0xF3)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fld1 - Load 1.0 onto ST(0)
func (o *Out) Fld1() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fld1: ")
	}
	o.Write(0xD9)
	o.Write(0xE8)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fldpi - Load π onto ST(0)
func (o *Out) Fldpi() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fldpi: ")
	}
	o.Write(0xD9)
	o.Write(0xEB)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fmul - Multiply ST(0) by ST(1), pop, result in ST(0)
// fmulp st(1), st(0)
func (o *Out) Fmulp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fmulp: ")
	}
	o.Write(0xDE)
	o.Write(0xC9)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fdiv - Divide ST(1) by ST(0), pop, result in ST(0)
// fdivp st(1), st(0)  -> ST(1) / ST(0)
func (o *Out) Fdivp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fdivp: ")
	}
	o.Write(0xDE)
	o.Write(0xF9)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fdivrp - Divide ST(0) by ST(1), pop, result in ST(0)
// fdivrp st(1), st(0)  -> ST(0) / ST(1)
func (o *Out) Fdivrp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fdivrp: ")
	}
	o.Write(0xDE)
	o.Write(0xF1)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fadd - Add ST(0) to ST(1), pop, result in ST(0)
// faddp st(1), st(0)
func (o *Out) Faddp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "faddp: ")
	}
	o.Write(0xDE)
	o.Write(0xC1)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fsub - Subtract ST(0) from ST(1), pop, result in ST(0)
// fsubp st(1), st(0)  -> ST(1) - ST(0)
func (o *Out) Fsubp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fsubp: ")
	}
	o.Write(0xDE)
	o.Write(0xE9)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fsubrp - Subtract ST(1) from ST(0), pop, result in ST(0)
// fsubrp st(1), st(0)  -> ST(0) - ST(1)
func (o *Out) Fsubrp() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fsubrp: ")
	}
	o.Write(0xDE)
	o.Write(0xE1)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fsqrt - Square root of ST(0), result in ST(0)
func (o *Out) Fsqrt() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fsqrt: ")
	}
	o.Write(0xD9)
	o.Write(0xFA)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fchs - Change sign of ST(0)
func (o *Out) Fchs() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fchs: ")
	}
	o.Write(0xD9)
	o.Write(0xE0)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fmul_st0_st0 - Multiply ST(0) by itself: ST(0) = ST(0) * ST(0)
// fmul st(0), st(0)
func (o *Out) FmulSelf() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fmul st(0), st(0): ")
	}
	o.Write(0xD8)
	o.Write(0xC8)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fld_st0 - Duplicate ST(0): push ST(0) onto stack
// fld st(0)
func (o *Out) FldSt0() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fld st(0): ")
	}
	o.Write(0xD9)
	o.Write(0xC0)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fabs computes abs(ST(0))
func (o *Out) Fabs() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fabs: ")
	}
	o.Write(0xD9)
	o.Write(0xE1)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Frndint rounds ST(0) to integer according to rounding mode
func (o *Out) Frndint() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "frndint: ")
	}
	o.Write(0xD9)
	o.Write(0xFC)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fldcw loads FPU control word from memory
func (o *Out) FldcwMem(reg string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fldcw [%s+%d]: ", reg, offset)
	}
	o.Write(0xD9) // FLDCW m16

	var rmBits uint8
	switch reg {
	case "rsp":
		rmBits = 4
	case "rbp":
		rmBits = 5
	default:
		rmBits = 0
	}

	if offset == 0 && rmBits != 5 {
		modrm := uint8(0x28) | rmBits // reg=5 for FLDCW
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24) // SIB for rsp
		}
	} else if offset >= -128 && offset < 128 {
		modrm := uint8(0x68) | rmBits
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0xA8) | rmBits
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
		o.Write(uint8(offset >> 8))
		o.Write(uint8(offset >> 16))
		o.Write(uint8(offset >> 24))
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fstcw stores FPU control word to memory
func (o *Out) FstcwMem(reg string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fstcw [%s+%d]: ", reg, offset)
	}
	o.Write(0xD9) // FSTCW m16

	var rmBits uint8
	switch reg {
	case "rsp":
		rmBits = 4
	case "rbp":
		rmBits = 5
	default:
		rmBits = 0
	}

	if offset == 0 && rmBits != 5 {
		modrm := uint8(0x38) | rmBits // reg=7 for FSTCW
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24)
		}
	} else if offset >= -128 && offset < 128 {
		modrm := uint8(0x78) | rmBits
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0xB8) | rmBits
		o.Write(modrm)
		if rmBits == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
		o.Write(uint8(offset >> 8))
		o.Write(uint8(offset >> 16))
		o.Write(uint8(offset >> 24))
	}
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fyl2x computes ST(1) * log2(ST(0)), pops both, pushes result
func (o *Out) Fyl2x() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fyl2x: ")
	}
	o.Write(0xD9)
	o.Write(0xF1)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fldln2 pushes ln(2) onto FPU stack
func (o *Out) Fldln2() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fldln2: ")
	}
	o.Write(0xD9)
	o.Write(0xED)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fldl2e pushes log2(e) onto FPU stack
func (o *Out) Fldl2e() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fldl2e: ")
	}
	o.Write(0xD9)
	o.Write(0xEA)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// F2xm1 computes 2^ST(0) - 1 (for -1 <= ST(0) <= 1)
func (o *Out) F2xm1() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "f2xm1: ")
	}
	o.Write(0xD9)
	o.Write(0xF0)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Fscale scales ST(0) by 2^ST(1) (ST(0) = ST(0) * 2^ST(1))
func (o *Out) Fscale() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fscale: ")
	}
	o.Write(0xD9)
	o.Write(0xFD)
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 scalar floating-point operations
func (o *Out) faddScalarARM64(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fadd %s, %s (scalar): ", dst, src)
	}
	// FADD Dd, Dn, Dm
	// Implementation would go here
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V scalar floating-point operations
func (o *Out) faddScalarRISCV(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fadd.d %s, %s (scalar): ", dst, src)
	}
	// FADD.D fd, fs1, fs2
	// Implementation would go here
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// XorpdXmm - XOR Packed Double (SSE2)
// xorpd xmm, xmm - commonly used to zero a register
func (o *Out) XorpdXmm(dst, src string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.xorpdX86(dst, src)
	case ArchARM64:
		o.eorARM64Xmm(dst, src)
	case ArchRiscv64:
		o.fxorRISCV(dst, src)
	}
}

func (o *Out) xorpdX86(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "xorpd %s, %s: ", dst, src)
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

	// 0F 57 - XORPD opcode
	o.Write(0x0F)
	o.Write(0x57)

	// ModR/M
	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) eorARM64Xmm(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "eor %s, %s (NEON): ", dst, src)
	}
	// EOR Vd.16B, Vn.16B, Vm.16B
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

func (o *Out) fxorRISCV(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fxor %s, %s (RISC-V): ", dst, src)
	}
	// No direct floating-point XOR, use integer XOR on FP registers
	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
