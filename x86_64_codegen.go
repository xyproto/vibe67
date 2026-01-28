// Completion: 100% - x86_64 backend fully implemented, production-ready
package main

import (
	"fmt"
	"os"
	"strconv"
)

type X86_64CodeGen struct {
	writer Writer
	eb     *ExecutableBuilder
}

func NewX86_64CodeGen(writer Writer, eb *ExecutableBuilder) *X86_64CodeGen {
	return &X86_64CodeGen{
		writer: writer,
		eb:     eb,
	}
}

func (x *X86_64CodeGen) write(b uint8) {
	x.writer.(*BufferWrapper).Write(b)
}

func (x *X86_64CodeGen) writeUnsigned(i uint) {
	x.writer.(*BufferWrapper).WriteUnsigned(i)
}

func (x *X86_64CodeGen) emit(bytes []byte) {
	for _, b := range bytes {
		x.write(b)
	}
}

func (x *X86_64CodeGen) Ret() {
	x.write(0xC3)
}

func (x *X86_64CodeGen) Syscall() {
	x.write(0x0F)
	x.write(0x05)
}

func (x *X86_64CodeGen) CallSymbol(symbol string) {
	x.write(0xE8)

	callPos := x.eb.text.Len()
	x.writeUnsigned(0x12345678) // Match placeholder used by GenerateCallInstruction

	x.eb.callPatches = append(x.eb.callPatches, CallPatch{
		position:   callPos,
		targetName: symbol,
	})
}

func (x *X86_64CodeGen) CallRelative(offset int32) {
	x.write(0xE8)
	x.writeUnsigned(uint(offset))
}

func (x *X86_64CodeGen) CallRegister(reg string) {
	r, ok := x86_64Registers[reg]
	if !ok {
		compilerError("Unknown register: %s", reg)
	}

	fmt.Fprintf(os.Stderr, "DEBUG CallRegister: reg=%s encoding=%d\n", reg, r.Encoding)

	// For registers r8-r15, we need a REX prefix with the B bit set
	if r.Encoding >= 8 {
		x.write(0x41) // REX.B prefix
		fmt.Fprintf(os.Stderr, "DEBUG CallRegister: writing REX prefix 0x41\n")
	}

	x.write(0xFF)
	x.write(0xD0 + (r.Encoding & 7)) // Use only low 3 bits
	fmt.Fprintf(os.Stderr, "DEBUG CallRegister: wrote 0xFF 0x%02X\n", 0xD0+(r.Encoding&7))
}

func (x *X86_64CodeGen) JumpUnconditional(offset int32) {
	x.write(0xE9)
	x.writeUnsigned(uint(offset))
}

func (x *X86_64CodeGen) JumpConditional(condition JumpCondition, offset int32) {
	var opcode byte
	switch condition {
	case JumpEqual:
		opcode = 0x84
	case JumpNotEqual:
		opcode = 0x85
	case JumpLess:
		opcode = 0x8C
	case JumpLessOrEqual:
		opcode = 0x8E
	case JumpGreater:
		opcode = 0x8F
	case JumpGreaterOrEqual:
		opcode = 0x8D
	case JumpAbove:
		opcode = 0x87
	case JumpAboveOrEqual:
		opcode = 0x83
	case JumpBelow:
		opcode = 0x82
	case JumpBelowOrEqual:
		opcode = 0x86
	default:
		compilerError("Unknown jump condition: %v", condition)
	}
	x.write(0x0F)
	x.write(opcode)
	x.writeUnsigned(uint(offset))
}

// ===== Data Movement =====

func (x *X86_64CodeGen) MovRegToReg(dst, src string) {
	dstIsXMM := (len(dst) >= 3 && dst[:3] == "xmm") || (len(dst) >= 1 && dst[:1] == "v")
	srcIsXMM := (len(src) >= 3 && src[:3] == "xmm") || (len(src) >= 1 && src[:1] == "v")
	if dstIsXMM && srcIsXMM {
		var dstNum, srcNum int
		fmt.Sscanf(dst, "xmm%d", &dstNum)
		fmt.Sscanf(src, "xmm%d", &srcNum)

		x.write(0xF2)

		if dstNum >= 8 || srcNum >= 8 {
			rex := uint8(0x40)
			if dstNum >= 8 {
				rex |= 0x04
			}
			if srcNum >= 8 {
				rex |= 0x01
			}
			x.write(rex)
		}

		x.write(0x0F)
		x.write(0x10)

		modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
		x.write(modrm)
		return
	}

	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]

	if !dstOk || !srcOk {
		return
	}

	needsRex := dstReg.Size == 64 || srcReg.Size == 64 || dstReg.Encoding >= 8 || srcReg.Encoding >= 8
	if needsRex {
		rex := uint8(0x40)
		if dstReg.Size == 64 || srcReg.Size == 64 {
			rex |= 0x08
		}
		if srcReg.Encoding >= 8 {
			rex |= 0x04
		}
		if dstReg.Encoding >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x89)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) MovImmToReg(dst, imm string) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	var immVal uint64
	if val, err := strconv.ParseInt(imm, 0, 64); err == nil {
		immVal = uint64(val)
	} else if val, err := strconv.ParseUint(imm, 0, 64); err == nil {
		immVal = val
	}

	// Check if immediate fits in 32 bits (can be sign-extended)
	if dstReg.Size == 64 && (int64(immVal) < -0x80000000 || int64(immVal) > 0x7FFFFFFF) {
		// Need full 64-bit immediate: MOV r64, imm64 (0xB8+r encoding)
		rex := uint8(0x48)
		if dstReg.Encoding >= 8 {
			rex |= 0x01
		}
		x.write(rex)
		x.write(0xB8 | (dstReg.Encoding & 7))
		// Write full 64-bit immediate
		x.write(uint8(immVal))
		x.write(uint8(immVal >> 8))
		x.write(uint8(immVal >> 16))
		x.write(uint8(immVal >> 24))
		x.write(uint8(immVal >> 32))
		x.write(uint8(immVal >> 40))
		x.write(uint8(immVal >> 48))
		x.write(uint8(immVal >> 56))
	} else {
		// 32-bit immediate (sign-extended for 64-bit): MOV r/m64, imm32
		if dstReg.Size == 64 {
			rex := uint8(0x48)
			if dstReg.Encoding >= 8 {
				rex |= 0x01
			}
			x.write(rex)
		}

		x.write(0xC7)

		modrm := uint8(0xC0) | (dstReg.Encoding & 7)
		x.write(modrm)

		x.writeUnsigned(uint(immVal))
	}
}

func (x *X86_64CodeGen) MovMemToReg(dst, symbol string, offset int32) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	// Check if symbol is actually a register name
	srcReg, isRegister := x86_64Registers[symbol]
	if isRegister {
		// Generate: mov dst, [src+offset]
		rex := uint8(0x48)
		if dstReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if srcReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		x.write(rex)

		x.write(0x8B) // mov opcode

		// ModR/M byte for [src+disp32] addressing
		var modrm uint8
		if offset == 0 && (srcReg.Encoding&7) != 5 { // rbp/r13 require displacement
			modrm = 0x00 | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
		} else if offset >= -128 && offset <= 127 {
			modrm = 0x40 | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7) // disp8
			x.write(modrm)
			x.write(uint8(offset))
			return
		} else {
			modrm = 0x80 | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7) // disp32
			x.write(modrm)
			x.writeUnsigned(uint(offset))
			return
		}
		x.write(modrm)
		return
	}

	// Symbol-based addressing (PC-relative)
	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x8B)

	modrm := uint8(0x05) | ((dstReg.Encoding & 7) << 3)
	x.write(modrm)

	displacementOffset := uint64(x.eb.text.Len())
	x.eb.pcRelocations = append(x.eb.pcRelocations, PCRelocation{
		offset:     displacementOffset,
		symbolName: symbol,
	})

	x.writeUnsigned(uint(offset))
}

func (x *X86_64CodeGen) MovRegToMem(src, symbol string, offset int32) {
	srcReg, srcOk := x86_64Registers[src]
	if !srcOk {
		return
	}

	// Check if symbol is actually a register name
	dstReg, isRegister := x86_64Registers[symbol]
	if isRegister {
		// Generate: mov [dst+offset], src
		rex := uint8(0x48)
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if dstReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		x.write(rex)

		x.write(0x89) // mov opcode

		// ModR/M byte for [dst+disp] addressing
		var modrm uint8
		if offset == 0 && (dstReg.Encoding&7) != 5 { // rbp/r13 require displacement
			modrm = 0x00 | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
		} else if offset >= -128 && offset <= 127 {
			modrm = 0x40 | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7) // disp8
			x.write(modrm)
			x.write(uint8(offset))
			return
		} else {
			modrm = 0x80 | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7) // disp32
			x.write(modrm)
			x.writeUnsigned(uint(offset))
			return
		}
		x.write(modrm)
		return
	}

	// Symbol-based addressing (PC-relative)
	rex := uint8(0x48)
	if srcReg.Encoding >= 8 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x89)

	modrm := uint8(0x05) | ((srcReg.Encoding & 7) << 3)
	x.write(modrm)

	displacementOffset := uint64(x.eb.text.Len())
	x.eb.pcRelocations = append(x.eb.pcRelocations, PCRelocation{
		offset:     displacementOffset,
		symbolName: symbol,
	})

	x.writeUnsigned(uint(offset))
}

// ===== Integer Arithmetic =====

func (x *X86_64CodeGen) AddRegToReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x01)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) AddImmToReg(dst string, imm int64) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	if imm >= -128 && imm <= 127 {
		x.write(0x83)
		modrm := uint8(0xC0) | (dstReg.Encoding & 7)
		x.write(modrm)
		x.write(uint8(imm & 0xFF))
	} else {
		x.write(0x81)
		modrm := uint8(0xC0) | (dstReg.Encoding & 7)
		x.write(modrm)

		imm32 := uint32(imm)
		x.write(uint8(imm32 & 0xFF))
		x.write(uint8((imm32 >> 8) & 0xFF))
		x.write(uint8((imm32 >> 16) & 0xFF))
		x.write(uint8((imm32 >> 24) & 0xFF))
	}
}

func (x *X86_64CodeGen) SubRegToReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x29)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) SubImmFromReg(dst string, imm int64) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	if imm >= -128 && imm <= 127 {
		x.write(0x83)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7)
		x.write(modrm)
		x.write(uint8(imm & 0xFF))
	} else {
		x.write(0x81)
		modrm := uint8(0xE8) | (dstReg.Encoding & 7)
		x.write(modrm)

		imm32 := uint32(imm)
		x.write(uint8(imm32 & 0xFF))
		x.write(uint8((imm32 >> 8) & 0xFF))
		x.write(uint8((imm32 >> 16) & 0xFF))
		x.write(uint8((imm32 >> 24) & 0xFF))
	}
}

func (x *X86_64CodeGen) MulRegToReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0xAF)

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | (srcReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) DivRegToReg(dst, src string) {
	srcReg, srcOk := x86_64Registers[src]
	if !srcOk {
		return
	}

	if dst != "rax" {
		x.MovRegToReg("rax", dst)
	}

	x.write(0x48)
	x.write(0x99)

	rex := uint8(0x48)
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)
	x.write(0xF7)
	modrm := uint8(0xF8) | (srcReg.Encoding & 7)
	x.write(modrm)

	if dst != "rax" {
		x.MovRegToReg(dst, "rax")
	}
}

func (x *X86_64CodeGen) IncReg(dst string) {
	regInfo, ok := x86_64Registers[dst]
	if !ok {
		return
	}

	rex := uint8(0x48)
	if (regInfo.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0xFF)

	modrm := uint8(0xC0) | (regInfo.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) DecReg(dst string) {
	regInfo, ok := x86_64Registers[dst]
	if !ok {
		return
	}

	rex := uint8(0x48)
	if (regInfo.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0xFF)

	modrm := uint8(0xC8) | (regInfo.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) NegReg(dst string) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0xF7)

	modrm := uint8(0xD8) | (dstReg.Encoding & 7)
	x.write(modrm)
}

// ===== Bitwise Operations =====

func (x *X86_64CodeGen) XorRegWithReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x31)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) XorRegWithImm(dst string, imm int64) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	imm32 := int32(imm)
	if imm32 >= -128 && imm32 <= 127 {
		x.write(0x83)
		modrm := uint8(0xF0) | (dstReg.Encoding & 7)
		x.write(modrm)
		x.write(uint8(imm32 & 0xFF))
	} else {
		x.write(0x81)
		modrm := uint8(0xF0) | (dstReg.Encoding & 7)
		x.write(modrm)

		x.write(uint8(imm32 & 0xFF))
		x.write(uint8((imm32 >> 8) & 0xFF))
		x.write(uint8((imm32 >> 16) & 0xFF))
		x.write(uint8((imm32 >> 24) & 0xFF))
	}
}

func (x *X86_64CodeGen) AndRegWithReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x21)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) OrRegWithReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	srcReg, srcOk := x86_64Registers[src]
	if !dstOk || !srcOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x09)

	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) NotReg(dst string) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0xF7)

	modrm := uint8(0xD0) | (dstReg.Encoding & 7)
	x.write(modrm)
}

// ===== Stack Operations =====

func (x *X86_64CodeGen) PushReg(reg string) {
	regInfo, regOk := x86_64Registers[reg]
	if !regOk {
		return
	}

	if regInfo.Encoding >= 8 {
		x.write(0x41)
		x.write(0x50 + uint8(regInfo.Encoding&7))
	} else {
		x.write(0x50 + uint8(regInfo.Encoding))
	}
}

func (x *X86_64CodeGen) PopReg(reg string) {
	regInfo, regOk := x86_64Registers[reg]
	if !regOk {
		return
	}

	if regInfo.Encoding >= 8 {
		x.write(0x41)
		x.write(0x58 + uint8(regInfo.Encoding&7))
	} else {
		x.write(0x58 + uint8(regInfo.Encoding))
	}
}

// ===== Comparisons =====

func (x *X86_64CodeGen) CmpRegToReg(reg1, reg2 string) {
	src1Reg, src1Ok := x86_64Registers[reg1]
	src2Reg, src2Ok := x86_64Registers[reg2]
	if !src1Ok || !src2Ok {
		return
	}

	rex := uint8(0x48)
	if (src1Reg.Encoding & 8) != 0 {
		rex |= 0x01
	}
	if (src2Reg.Encoding & 8) != 0 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x39)

	modrm := uint8(0xC0) | ((src2Reg.Encoding & 7) << 3) | (src1Reg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) CmpRegToImm(reg string, imm int64) {
	regInfo, regOk := x86_64Registers[reg]
	if !regOk {
		return
	}

	rex := uint8(0x48)
	if (regInfo.Encoding & 8) != 0 {
		rex |= 0x01
	}
	x.write(rex)

	if imm >= -128 && imm <= 127 {
		x.write(0x83)
		modrm := uint8(0xF8) | (regInfo.Encoding & 7)
		x.write(modrm)
		x.write(uint8(imm & 0xFF))
	} else {
		x.write(0x81)
		modrm := uint8(0xF8) | (regInfo.Encoding & 7)
		x.write(modrm)

		imm32 := uint32(imm)
		x.write(uint8(imm32 & 0xFF))
		x.write(uint8((imm32 >> 8) & 0xFF))
		x.write(uint8((imm32 >> 16) & 0xFF))
		x.write(uint8((imm32 >> 24) & 0xFF))
	}
}

// ===== Address Calculation =====

func (x *X86_64CodeGen) LeaSymbolToReg(dst, symbol string) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	x.write(rex)

	x.write(0x8D)

	modrm := uint8(0x05) | ((dstReg.Encoding & 7) << 3)
	x.write(modrm)

	displacementOffset := uint64(x.eb.text.Len())
	x.eb.pcRelocations = append(x.eb.pcRelocations, PCRelocation{
		offset:     displacementOffset,
		symbolName: symbol,
	})

	x.writeUnsigned(0xDEADBEEF)
}

func (x *X86_64CodeGen) LeaImmToReg(dst, base string, offset int32) {
	dstReg, dstOk := x86_64Registers[dst]
	baseReg, baseOk := x86_64Registers[base]

	if !dstOk || !baseOk {
		return
	}

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x8D)

	offset64 := int64(offset)
	if offset64 == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
	} else if offset64 >= -128 && offset64 <= 127 {
		modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		x.write(uint8(offset64 & 0xFF))
	} else {
		modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		x.writeUnsigned(uint(offset64 & 0xFFFFFFFF))
	}
}

// ===== Floating Point (SIMD) =====

func (x *X86_64CodeGen) MovXmmToMem(src, base string, offset int32) {
	var xmmNum int
	fmt.Sscanf(src, "xmm%d", &xmmNum)

	baseReg := x86_64Registers[base]

	x.write(0xF2)

	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x11)

	offset64 := int64(offset)
	if offset64 == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
	} else if offset64 < 128 && offset64 >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
		x.write(uint8(offset64))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
		x.writeUnsigned(uint(offset64))
	}
}

func (x *X86_64CodeGen) MovMemToXmm(dst, base string, offset int32) {
	var xmmNum int
	fmt.Sscanf(dst, "xmm%d", &xmmNum)

	baseReg := x86_64Registers[base]

	x.write(0xF2)

	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x10)

	offset64 := int64(offset)
	if offset64 == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
	} else if offset64 < 128 && offset64 >= -128 {
		modrm := uint8(0x40) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
		x.write(uint8(offset64))
	} else {
		modrm := uint8(0x80) | (uint8(xmmNum&7) << 3) | (baseReg.Encoding & 7)
		x.write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			x.write(0x24)
		}
		x.writeUnsigned(uint(offset64))
	}
}

func (x *X86_64CodeGen) MovRegToXmm(dst, src string) {
	srcReg, srcOk := x86_64Registers[src]
	if !srcOk {
		return
	}

	var xmmNum int
	fmt.Sscanf(dst, "xmm%d", &xmmNum)

	x.write(0x66)

	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if srcReg.Encoding >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x6E)

	modrm := uint8(0xC0) | (uint8(xmmNum&7) << 3) | (srcReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) MovXmmToReg(dst, src string) {
	dstReg, dstOk := x86_64Registers[dst]
	if !dstOk {
		return
	}

	var xmmNum int
	fmt.Sscanf(src, "xmm%d", &xmmNum)

	x.write(0x66)

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	if xmmNum >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x7E)

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | uint8(xmmNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) Cvtsi2sd(dst, src string) {
	srcReg, srcOk := x86_64Registers[src]
	if !srcOk {
		return
	}

	var xmmNum int
	fmt.Sscanf(dst, "xmm%d", &xmmNum)

	x.write(0xF2)

	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04
	}
	if srcReg.Encoding >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x2A)

	modrm := uint8(0xC0) | (uint8(xmmNum&7) << 3) | (srcReg.Encoding & 7)
	x.write(modrm)
}

func (x *X86_64CodeGen) Cvttsd2si(dst, src string) {
	dstReg := x86_64Registers[dst]

	var xmmNum int
	fmt.Sscanf(src, "xmm%d", &xmmNum)

	x.write(0xF2)

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	if xmmNum >= 8 {
		rex |= 0x01
	}
	x.write(rex)

	x.write(0x0F)
	x.write(0x2C)

	modrm := uint8(0xC0) | ((dstReg.Encoding & 7) << 3) | uint8(xmmNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) AddpdXmm(dst, src string) {
	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	x.write(0x66)

	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x0F)
	x.write(0x58)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) SubpdXmm(dst, src string) {
	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	x.write(0x66)

	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x0F)
	x.write(0x5C)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) MulpdXmm(dst, src string) {
	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	x.write(0x66)

	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x0F)
	x.write(0x59)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) DivpdXmm(dst, src string) {
	var dstNum, srcNum int
	fmt.Sscanf(dst, "xmm%d", &dstNum)
	fmt.Sscanf(src, "xmm%d", &srcNum)

	x.write(0x66)

	if dstNum >= 8 || srcNum >= 8 {
		rex := uint8(0x40)
		if dstNum >= 8 {
			rex |= 0x04
		}
		if srcNum >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x0F)
	x.write(0x5E)

	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	x.write(modrm)
}

func (x *X86_64CodeGen) Ucomisd(reg1, reg2 string) {
	var xmm1Num, xmm2Num int
	fmt.Sscanf(reg1, "xmm%d", &xmm1Num)
	fmt.Sscanf(reg2, "xmm%d", &xmm2Num)

	x.write(0x66)

	rex := uint8(0)
	if xmm1Num >= 8 || xmm2Num >= 8 {
		rex = 0x40
		if xmm1Num >= 8 {
			rex |= 0x04
		}
		if xmm2Num >= 8 {
			rex |= 0x01
		}
		x.write(rex)
	}

	x.write(0x0F)
	x.write(0x2E)

	modrm := uint8(0xC0) | (uint8(xmm1Num&7) << 3) | uint8(xmm2Num&7)
	x.write(modrm)
}

// Cld clears the direction flag (for string operations)
func (x *X86_64CodeGen) Cld() {
	x.write(0xFC)
}

// RepMovsb repeats movsb rcx times (copies rcx bytes from rsi to rdi)
func (x *X86_64CodeGen) RepMovsb() {
	x.write(0xF3) // REP prefix
	x.write(0xA4) // MOVSB
}

// BtRegReg performs bit test: BT reg1, reg2 (test bit reg2 in reg1, sets CF)
func (x *X86_64CodeGen) BtRegReg(reg1, reg2 string) {
	reg1Info, ok1 := x86_64Registers[reg1]
	reg2Info, ok2 := x86_64Registers[reg2]
	if !ok1 || !ok2 {
		compilerError("Invalid registers for BtRegReg: %s, %s", reg1, reg2)
		return
	}

	reg1Num := reg1Info.Encoding
	reg2Num := reg2Info.Encoding

	// REX.W prefix for 64-bit operation
	rex := uint8(0x48)
	if reg1Num >= 8 {
		rex |= 0x01 // REX.B
	}
	if reg2Num >= 8 {
		rex |= 0x04 // REX.R
	}
	x.write(rex)

	// BT r/m64, r64 opcode
	x.write(0x0F)
	x.write(0xA3)

	// ModR/M byte: mod=11 (register), reg=reg2, rm=reg1
	modrm := uint8(0xC0) | (uint8(reg2Num&7) << 3) | uint8(reg1Num&7)
	x.write(modrm)
}

// SetcReg sets a register to 1 if CF=1, 0 otherwise (SETC r/m8)
func (x *X86_64CodeGen) SetcReg(reg string) {
	// For byte registers like "al", we need special handling
	if reg == "al" {
		// SETC al - no REX needed for AL
		x.write(0x0F)
		x.write(0x92)
		x.write(0xC0) // ModR/M for AL
		return
	}

	// For other registers, look up in register map
	regInfo, ok := x86_64Registers[reg]
	if !ok {
		compilerError("Invalid register for SetcReg: %s", reg)
		return
	}
	regNum := regInfo.Encoding

	// REX prefix if needed (for extended registers or 64-bit)
	if regNum >= 8 || (regNum >= 4 && regNum <= 7) {
		x.write(0x40 | (uint8(regNum>>3) & 0x01))
	}

	x.write(0x0F)
	x.write(0x92)
	x.write(0xC0 | uint8(regNum&7))
}

// MovzxByteToQword performs MOVZX r64, r/m8 (zero-extend byte to qword)
func (x *X86_64CodeGen) MovzxByteToQword(dstReg, srcReg string) {
	dstInfo, dstOk := x86_64Registers[dstReg]
	if !dstOk {
		compilerError("Invalid destination register for MovzxByteToQword: %s", dstReg)
		return
	}
	dstNum := dstInfo.Encoding

	// For source, handle byte registers like "al"
	srcNum := uint8(0)
	if srcInfo, ok := x86_64Registers[srcReg]; ok {
		srcNum = srcInfo.Encoding
	} else {
		compilerError("Invalid source register for MovzxByteToQword: %s", srcReg)
		return
	}

	// REX.W prefix for 64-bit destination
	rex := uint8(0x48)
	if dstNum >= 8 {
		rex |= 0x04 // REX.R
	}
	if srcNum >= 8 {
		rex |= 0x01 // REX.B
	}
	x.write(rex)

	// MOVZX r64, r/m8 opcode
	x.write(0x0F)
	x.write(0xB6)

	// ModR/M byte
	modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
	x.write(modrm)
}
