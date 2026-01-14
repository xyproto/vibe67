// Completion: 100% - Utility module complete
package main

// CodeGenerator is the interface that all architecture backends must implement
type CodeGenerator interface {
	// Move operations
	MovRegToReg(dst, src string)
	MovImmToReg(dst, imm string)
	MovRegToXmm(dst, src string)

	// Arithmetic operations
	AddRegToReg(dst, src string)
	AddImmToReg(dst string, imm int64)
	SubRegToReg(dst, src string)
	SubImmFromReg(dst string, imm int64)
	MulRegToReg(dst, src string)
	DivRegToReg(dst, src string)
	NegReg(dst string)

	// Logical operations
	AndRegWithReg(dst, src string)
	OrRegWithReg(dst, src string)
	XorRegWithReg(dst, src string)
	XorRegWithImm(dst string, imm int64)
	NotReg(dst string)

	// Comparison and jumps
	CmpRegToReg(src1, src2 string)
	CmpRegToImm(reg string, imm int64)
	JumpConditional(condition JumpCondition, offset int32)
	JumpUnconditional(offset int32)

	// SIMD operations
	AddpdXmm(dst, src string)
	SubpdXmm(dst, src string)
	MulpdXmm(dst, src string)
	DivpdXmm(dst, src string)
	Ucomisd(dst, src string)
	Cvtsi2sd(dst, src string)

	// System operations
	Ret()
	Syscall()
}

// NewCodeGenerator creates a code generator backend for the given architecture
// Returns nil for x86_64 since it uses legacy methods in Out
func NewCodeGenerator(arch Arch, writer Writer, eb *ExecutableBuilder) CodeGenerator {
	switch arch {
	case ArchX86_64:
		return nil // x86_64 uses legacy methods in mov.go, add.go, etc.
	case ArchARM64:
		return NewARM64Backend(writer, eb)
	case ArchRiscv64:
		return NewRISCV64Backend(writer, eb)
	default:
		return nil
	}
}









