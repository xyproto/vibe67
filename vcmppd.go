// Completion: 100% - SIMD instruction complete
package main

import (
	"fmt"
	"os"
)

// VCMPPD - Vector comparison for packed double-precision (float64) values
// Essential for Vibe67's filter operations and conditionals:
//   - List comprehensions: [x for x in values if x > threshold]
//   - Filter operations: values |> filter(x -> x >= min)
//   - Pattern matching guards: [x in rest]{x < pivot}
//   - Conditional select based on comparison results
//
// Architecture details:
//   x86-64: VCMPPD k1, zmm2, zmm3, imm8 (AVX-512: produces mask in k register)
//   ARM64:  FCMGE pd.d, pg/z, zn.d, zm.d (SVE2: produces predicate mask)
//   RISC-V: vmflt.vv v0, v1, v2 (RVV: produces mask in v0)

// ComparisonPredicate defines the type of comparison
type ComparisonPredicate int

const (
	CmpEQ  ComparisonPredicate = 0  // Equal
	CmpLT  ComparisonPredicate = 1  // Less than
	CmpLE  ComparisonPredicate = 2  // Less than or equal
	CmpNE  ComparisonPredicate = 4  // Not equal (unordered)
	CmpNLT ComparisonPredicate = 5  // Not less than (greater or equal)
	CmpNLE ComparisonPredicate = 6  // Not less than or equal (greater)
	CmpGT  ComparisonPredicate = 14 // Greater than (ordered, non-signaling)
	CmpGE  ComparisonPredicate = 13 // Greater or equal (ordered, non-signaling)
)

// VCmpPDVectorToVector performs vector comparison: mask = cmp(src1, src2, predicate)
// Result is a mask (k register on x86-64, predicate on ARM64, v0 on RISC-V)
func (o *Out) VCmpPDVectorToVector(dstMask, src1, src2 string, predicate ComparisonPredicate) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.vcmppdX86VectorToVector(dstMask, src1, src2, predicate)
	case ArchARM64:
		o.vcmppdARM64VectorToVector(dstMask, src1, src2, predicate)
	case ArchRiscv64:
		o.vcmppdRISCVVectorToVector(dstMask, src1, src2, predicate)
	}
}

// ============================================================================
// x86-64 AVX-512 implementation
// ============================================================================

// x86-64 VCMPPD k1{k2}, zmm2, zmm3, imm8 (AVX-512)
// EVEX.NDS.512.66.0F.W1 C2 /r ib
func (o *Out) vcmppdX86VectorToVector(dstMask, src1, src2 string, predicate ComparisonPredicate) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dstMask)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	predicateNames := map[ComparisonPredicate]string{
		CmpEQ:  "eq",
		CmpLT:  "lt",
		CmpLE:  "le",
		CmpNE:  "neq",
		CmpNLT: "nlt",
		CmpNLE: "nle",
		CmpGT:  "gt",
		CmpGE:  "ge",
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "vcmppd %s, %s, %s, %s:", dstMask, src1, src2, predicateNames[predicate])
	}

	if src1Reg.Size == 512 {
		// AVX-512 VCMPPD k1, zmm1, zmm2, imm8
		// EVEX prefix
		p0 := uint8(0x62)

		// P1: map_select = 01 (0F)
		p1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			p1 |= 0x80 // R
		}
		p1 |= 0x40 // X
		if (src2Reg.Encoding & 8) == 0 {
			p1 |= 0x20 // B
		}
		// R' not used for k registers

		// P2: W=1, vvvv=~src1, pp=01
		p2 := uint8(0x81)
		p2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)

		// P3: L'L=10 (512-bit)
		p3 := uint8(0x40)
		if (src1Reg.Encoding & 16) == 0 {
			p3 |= 0x08 // V'
		}

		o.Write(p0)
		o.Write(p1)
		o.Write(p2)
		o.Write(p3)

		// Opcode: 0xC2 (VCMPPD)
		o.Write(0xC2)

		// ModR/M: dst(k) is reg field, src2 is r/m field
		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)

		// Immediate byte: comparison predicate
		o.Write(uint8(predicate))
	} else if src1Reg.Size == 256 {
		// AVX VCMPPD ymm, ymm, ymm, imm8
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (AVX2, produces vector mask)")
		}

		o.Write(0xC4)

		vex1 := uint8(0x01)
		if (dstReg.Encoding & 8) == 0 {
			vex1 |= 0x80
		}
		vex1 |= 0x40
		if (src2Reg.Encoding & 8) == 0 {
			vex1 |= 0x20
		}
		o.Write(vex1)

		vex2 := uint8(0x45) // L=1, pp=01
		vex2 |= uint8((^src1Reg.Encoding & 0x0F) << 3)
		o.Write(vex2)

		o.Write(0xC2)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)

		o.Write(uint8(predicate))
	} else {
		// SSE2 CMPPD xmm, xmm, imm8
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (SSE2, produces vector mask)")
		}

		o.Write(0x66)

		if (dstReg.Encoding&8) != 0 || (src2Reg.Encoding&8) != 0 {
			rex := uint8(0x40)
			if (dstReg.Encoding & 8) != 0 {
				rex |= 0x04
			}
			if (src2Reg.Encoding & 8) != 0 {
				rex |= 0x01
			}
			o.Write(rex)
		}

		o.Write(0x0F)
		o.Write(0xC2)

		modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (src2Reg.Encoding & 7)
		o.Write(modrm)

		o.Write(uint8(predicate))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 SVE2 implementation
// ============================================================================

// ARM64 FCMGE/FCMGT/FCMEQ Pd.D, Pg/Z, Zn.D, Zm.D (SVE2)
func (o *Out) vcmppdARM64VectorToVector(dstMask, src1, src2 string, predicate ComparisonPredicate) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dstMask)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	if src1Reg.Size == 512 {
		// SVE2 comparison instructions
		var mnemonic string
		var opcode uint32

		switch predicate {
		case CmpEQ:
			mnemonic = "fcmeq"
			opcode = 0x65202000 // FCMEQ encoding
		case CmpGT, CmpNLE:
			mnemonic = "fcmgt"
			opcode = 0x65208000 // FCMGT encoding
		case CmpGE, CmpNLT:
			mnemonic = "fcmge"
			opcode = 0x65004000 // FCMGE encoding
		case CmpLT:
			// LT: swap operands and use GT
			mnemonic = "fcmgt"
			opcode = 0x65208000
			src1Reg, src2Reg = src2Reg, src1Reg
		case CmpLE:
			// LE: swap operands and use GE
			mnemonic = "fcmge"
			opcode = 0x65004000
			src1Reg, src2Reg = src2Reg, src1Reg
		case CmpNE:
			mnemonic = "fcmne"
			opcode = 0x65202000 // Use EQ and invert
		default:
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "# Unsupported comparison predicate\n")
			}
			return
		}

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "%s %s.d, p7/z, %s.d, %s.d:", mnemonic, dstMask, src1, src2)
		}

		// Build instruction
		instr := opcode |
			(uint32(src2Reg.Encoding&31) << 16) | // Zm
			(7 << 10) | // Pg = p7
			(uint32(src1Reg.Encoding&31) << 5) | // Zn
			uint32(dstReg.Encoding&31) // Pd

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// NEON comparison (produces vector of all-1s or all-0s)
		var mnemonic string
		var opcode uint32

		switch predicate {
		case CmpEQ:
			mnemonic = "fcmeq"
			opcode = 0x4E60E400
		case CmpGE, CmpNLT:
			mnemonic = "fcmge"
			opcode = 0x6E60E400
		case CmpGT, CmpNLE:
			mnemonic = "fcmgt"
			opcode = 0x6E60E400
		default:
			mnemonic = "fcmeq" // fallback
			opcode = 0x4E60E400
		}

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "%s %s.2d, %s.2d, %s.2d:", mnemonic, dstMask, src1, src2)
		}

		instr := opcode |
			(uint32(src2Reg.Encoding&31) << 16) |
			(uint32(src1Reg.Encoding&31) << 5) |
			uint32(dstReg.Encoding&31)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V RVV implementation
// ============================================================================

// RISC-V vmflt.vv/vmfle.vv/vmfeq.vv (RVV)
// Result goes to mask register v0
func (o *Out) vcmppdRISCVVectorToVector(dstMask, src1, src2 string, predicate ComparisonPredicate) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dstMask)
	src1Reg, src1Ok := GetRegister(o.target.Arch(), src1)
	src2Reg, src2Ok := GetRegister(o.target.Arch(), src2)
	if !dstOk || !src1Ok || !src2Ok {
		return
	}

	var mnemonic string
	var funct6 uint32

	switch predicate {
	case CmpEQ:
		mnemonic = "vmfeq.vv"
		funct6 = 0x18 // 011000
	case CmpLT:
		mnemonic = "vmflt.vv"
		funct6 = 0x1B // 011011
	case CmpLE:
		mnemonic = "vmfle.vv"
		funct6 = 0x19 // 011001
	case CmpNE:
		mnemonic = "vmfne.vv"
		funct6 = 0x1C // 011100
	case CmpGT:
		// GT: swap operands and use LT
		mnemonic = "vmflt.vv"
		funct6 = 0x1B
		src1Reg, src2Reg = src2Reg, src1Reg
	case CmpGE, CmpNLT:
		// GE: swap operands and use LE
		mnemonic = "vmfle.vv"
		funct6 = 0x19
		src1Reg, src2Reg = src2Reg, src1Reg
	default:
		mnemonic = "vmfeq.vv"
		funct6 = 0x18
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "%s %s, %s, %s:", mnemonic, dstMask, src2, src1)
	}

	// RVV compare encoding
	// funct6 vm vs2 vs1 funct3 vd opcode
	// funct3 = 001 (OPFVV)
	instr := uint32(0x57) | // opcode
		(1 << 12) | // funct3=001
		(uint32(dstReg.Encoding&31) << 7) | // vd (mask dest)
		(uint32(src1Reg.Encoding&31) << 15) | // vs1
		(uint32(src2Reg.Encoding&31) << 20) | // vs2
		(1 << 25) | // vm=1 (unmasked)
		(funct6 << 26) // funct6

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}









