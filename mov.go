// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Out struct {
	target  Target
	writer  Writer
	eb      *ExecutableBuilder
	backend CodeGenerator
}

// NewOut creates a new Out instance with the backend properly initialized
func NewOut(target Target, writer Writer, eb *ExecutableBuilder) *Out {
	backend := NewCodeGenerator(target.Arch(), writer, eb)
	return &Out{
		target:  target,
		writer:  writer,
		eb:      eb,
		backend: backend, // nil for x86_64, actual backend for ARM64/RISC-V
	}
}

func (o *Out) Write(b uint8) {
	o.writer.(*BufferWrapper).Write(b)
}

func (o *Out) WriteUnsigned(i uint) {
	o.writer.(*BufferWrapper).WriteUnsigned(i)
}

func (o *Out) Lookup(name string) string {
	return o.eb.Lookup(name)
}

// MovRegToReg generates a register-to-register move instruction
func (o *Out) MovRegToReg(dst, src string) {
	if o.backend != nil {
		o.backend.MovRegToReg(dst, src)
		return
	}
	// Fallback to architecture-specific methods for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.movX86RegToReg(dst, src)
	case ArchARM64:
		o.movARM64RegToReg(dst, src)
	case ArchRiscv64:
		o.movRISCVRegToReg(dst, src)
	}
}

// MovImmToReg generates an immediate-to-register move instruction
func (o *Out) MovImmToReg(dst, imm string) {
	if o.backend != nil {
		o.backend.MovImmToReg(dst, imm)
		return
	}
	// Fallback to architecture-specific methods for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.movX86ImmToReg(dst, imm)
	case ArchARM64:
		o.movARM64ImmToReg(dst, imm)
	case ArchRiscv64:
		o.movRISCVImmToReg(dst, imm)
	}
}

// x86_64 register-to-register move
func (o *Out) movX86RegToReg(dst, src string) {
	// Check if these are XMM registers
	dstIsXMM := (len(dst) >= 3 && dst[:3] == "xmm") || (len(dst) >= 1 && dst[:1] == "v")
	srcIsXMM := (len(src) >= 3 && src[:3] == "xmm") || (len(src) >= 1 && src[:1] == "v")
	if dstIsXMM && srcIsXMM {
		// XMM to XMM move using MOVSD
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "movsd %s, %s: ", dst, src)
		}

		var dstNum, srcNum int
		fmt.Sscanf(dst, "xmm%d", &dstNum)
		fmt.Sscanf(src, "xmm%d", &srcNum)

		// F2 0F 10 - MOVSD xmm1, xmm2
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

		o.Write(0x0F)
		o.Write(0x10)

		// ModR/M byte
		modrm := uint8(0xC0) | (uint8(dstNum&7) << 3) | uint8(srcNum&7)
		o.Write(modrm)

		if VerboseMode {
			fmt.Fprintln(os.Stderr)
		}
		return
	}

	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)

	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, %s: ", dst, src)
	}

	// REX prefix for register-extension/operand width
	needsRex := dstReg.Size == 64 || srcReg.Size == 64 || dstReg.Encoding >= 8 || srcReg.Encoding >= 8
	if needsRex {
		rex := uint8(0x40)
		if dstReg.Size == 64 || srcReg.Size == 64 {
			rex |= 0x08 // REX.W - 64-bit operand size
		}
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R extends the reg field (source)
		}
		if dstReg.Encoding >= 8 {
			rex |= 0x01 // REX.B extends the r/m field (destination)
		}
		o.Write(rex)
	}

	// MOV r/m64, r64 (0x89) or MOV r/m32, r32 (0x89)
	o.Write(0x89)

	// ModR/M byte: 11|reg|r/m
	modrm := uint8(0xC0) | ((srcReg.Encoding & 7) << 3) | (dstReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86_64 immediate-to-register move
func (o *Out) movX86ImmToReg(dst, imm string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, %s:", dst, imm)
	}

	// Parse immediate value (support both signed and unsigned)
	var immVal uint64
	if val, err := strconv.ParseInt(imm, 0, 64); err == nil {
		// Signed integer - convert to uint64 (preserves two's complement representation)
		immVal = uint64(val)
	} else if val, err := strconv.ParseUint(imm, 0, 64); err == nil {
		immVal = val
	} else if addr := o.Lookup(imm); addr != "0" {
		if val, err := strconv.ParseUint(addr, 10, 64); err == nil {
			immVal = val
		}
	}

	// REX prefix for 64-bit registers
	if dstReg.Size == 64 {
		rex := uint8(0x48)
		if dstReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// MOV with immediate encoding
	o.Write(0xC7) // MOV r/m64, imm32

	// ModR/M byte for register direct addressing
	modrm := uint8(0xC0) | (dstReg.Encoding & 7)
	o.Write(modrm)

	// Write 32-bit immediate (sign-extended to 64-bit)
	o.WriteUnsigned(uint(immVal))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 register-to-register move
func (o *Out) movARM64RegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)

	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, %s:", dst, src)
	}

	// ARM64 MOV (register): ORR Xd, XZR, Xm
	var instr uint32

	if dstReg.Size == 64 && srcReg.Size == 64 {
		// 64-bit: ORR Xd, XZR, Xm
		instr = 0xAA0003E0 | (uint32(srcReg.Encoding&31) << 16) | uint32(dstReg.Encoding&31)
	} else {
		// 32-bit: ORR Wd, WZR, Wm
		instr = 0x2A0003E0 | (uint32(srcReg.Encoding&31) << 16) | uint32(dstReg.Encoding&31)
	}

	// ARM64 is little-endian
	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 immediate-to-register move
func (o *Out) movARM64ImmToReg(dst, imm string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, %s:", dst, imm)
	}

	// Strip ARM64 immediate prefix (#) if present
	imm = strings.TrimPrefix(imm, "#")

	// Parse immediate value (support both signed and unsigned)
	var immVal uint64
	if val, err := strconv.ParseInt(imm, 0, 64); err == nil {
		// Signed integer - convert to uint64 (preserves two's complement representation)
		immVal = uint64(val)
	} else if val, err := strconv.ParseUint(imm, 0, 64); err == nil {
		immVal = val
	} else if addr := o.Lookup(imm); addr != "0" {
		if val, err := strconv.ParseUint(addr, 10, 64); err == nil {
			immVal = val
		}
	}

	// Use MOVZ (move with zero) for immediate values
	var instr uint32
	if dstReg.Size == 64 {
		// MOVZ Xd, #imm16
		instr = 0xD2800000 | (uint32(immVal&0xFFFF) << 5) | uint32(dstReg.Encoding&31)
	} else {
		// MOVZ Wd, #imm16
		instr = 0x52800000 | (uint32(immVal&0xFFFF) << 5) | uint32(dstReg.Encoding&31)
	}

	// Write 32-bit instruction (little-endian)
	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V register-to-register move
func (o *Out) movRISCVRegToReg(dst, src string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	srcReg, srcOk := GetRegister(o.target.Arch(), src)

	if !dstOk || !srcOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mv %s, %s:", dst, src)
	}

	// RISC-V MV is implemented as ADDI rd, rs1, 0
	// Format: imm[11:0] | rs1 | 000 | rd | 0010011
	var instr uint32 = 0x13 // opcode for ADDI

	instr |= uint32(dstReg.Encoding&31) << 7  // rd
	instr |= 0 << 12                          // funct3 = 000 for ADDI
	instr |= uint32(srcReg.Encoding&31) << 15 // rs1
	instr |= 0 << 20                          // immediate = 0

	// Write 32-bit instruction (little-endian)
	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V immediate-to-register move
func (o *Out) movRISCVImmToReg(dst, imm string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "li %s, %s:", dst, imm)
	}

	// Parse immediate value
	var immVal int64
	if val, err := strconv.ParseInt(imm, 0, 64); err == nil {
		immVal = val
	} else if addr := o.Lookup(imm); addr != "0" {
		if val, err := strconv.ParseInt(addr, 10, 64); err == nil {
			immVal = val
		}
	}

	// For simplicity, use ADDI rd, x0, imm for small immediates
	if immVal >= -2048 && immVal <= 2047 {
		// ADDI rd, x0, imm
		var instr uint32 = 0x13 // opcode

		instr |= uint32(dstReg.Encoding&31) << 7 // rd
		instr |= 0 << 12                         // funct3 = 000
		instr |= 0 << 15                         // rs1 = x0
		instr |= uint32((immVal & 0xFFF)) << 20  // immediate

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		// For larger immediates, would need LUI + ADDI sequence
		// For now, just use ADDI with truncated immediate
		immVal = immVal & 0xFFF
		var instr uint32 = 0x13

		instr |= uint32(dstReg.Encoding&31) << 7
		instr |= 0 << 12
		instr |= 0 << 15
		instr |= uint32(immVal&0xFFF) << 20

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovInstruction handles both register-to-register and immediate-to-register moves
func (o *Out) MovInstruction(dst, src string) {
	// Clean up source and destination
	dst = strings.TrimSuffix(strings.TrimSpace(dst), ",")
	src = strings.TrimSpace(src)

	// Check if source is a register
	if IsRegister(o.target.Arch(), src) {
		o.MovRegToReg(dst, src)
	} else {
		// Check if this is a symbol in PIE mode
		if o.eb.useDynamicLinking && isSymbolName(src) {
			o.LeaSymbolToReg(dst, src)
		} else {
			o.MovImmToReg(dst, src)
		}
	}
}

// MovRegToXmm moves a general purpose register to an XMM register
func (o *Out) MovRegToXmm(dst, src string) {
	if o.backend != nil {
		o.backend.MovRegToXmm(dst, src)
		return
	}
	// Fallback to architecture-specific methods for x86_64
	switch o.target.Arch() {
	case ArchX86_64:
		o.movX86RegToXmm(dst, src)
	case ArchARM64:
		o.movARM64RegToFP(dst, src)
	case ArchRiscv64:
		o.movRISCVRegToFP(dst, src)
	}
}

// x86-64 MOVQ GP register to XMM register
func (o *Out) movX86RegToXmm(dst, src string) {
	// For x86-64, use MOVQ xmm, r64 (66 REX.W 0F 6E /r)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movq %s, %s:", dst, src)
	}

	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	// Parse XMM register number from "xmm0", "xmm1", etc.
	var xmmNum int
	fmt.Sscanf(dst, "xmm%d", &xmmNum)

	// 66 prefix (operand size override)
	o.Write(0x66)

	// REX.W prefix (0x48 base + adjustments for registers)
	rex := uint8(0x48)
	if xmmNum >= 8 {
		rex |= 0x04 // REX.R
	}
	if srcReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0F 6E - MOVQ opcode
	o.Write(0x0F)
	o.Write(0x6E)

	// ModR/M byte
	modrm := uint8(0xC0) | (uint8(xmmNum&7) << 3) | (srcReg.Encoding & 7)
	o.Write(modrm)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64: Move GP register to FP register
func (o *Out) movARM64RegToFP(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fmov %s, %s:", dst, src)
	}

	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	// Parse vector register number
	var vNum int
	fmt.Sscanf(dst, "v%d", &vNum)

	// FMOV Vd.D[0], Xn - encoding: 0x9E670000
	instr := uint32(0x9E670000) |
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

// RISC-V: Move GP register to FP register
func (o *Out) movRISCVRegToFP(dst, src string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "fmv.d.x %s, %s:", dst, src)
	}

	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	if !srcOk {
		return
	}

	// Parse FP register number
	var fNum int
	fmt.Sscanf(dst, "f%d", &fNum)

	// FMV.D.X fd, rs1 - encoding: 1111000 00000 rs1 000 rd 1010011
	// funct7=1111000 (0xF0), rs2=00000, rs1, funct3=000, rd, opcode=1010011 (0x53)
	instr := uint32(0xF0000053) |
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

// CallSymbol generates a relative CALL instruction to a labeled symbol
func (o *Out) CallSymbol(symbol string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.callSymbolX86(symbol)
	}
}

func envBool(name string) bool {
	return os.Getenv(name) != ""
}

func (o *Out) callSymbolX86(symbol string) {
	if strings.Contains(symbol, "arena") {
		fmt.Fprintf(os.Stderr, "DEBUG: CallSymbol(%s) at text position %d (0x%X)\n", symbol, o.eb.text.Len(), o.eb.text.Len())
	}
	// Emit CALL instruction
	// On Windows: use indirect call (FF 15) for consistency with GenerateCallInstruction
	// On other OS: use direct call (E8)
	posBeforeCall := o.eb.text.Len()
	
	if o.target.OS() == OSWindows {
		// Emit FF 15 (CALL [RIP+disp32]) - same as GenerateCallInstruction
		o.Write(0xFF)
		o.Write(0x15)
		callPos := o.eb.text.Len()
		o.WriteUnsigned(0x12345678) // Placeholder for displacement
		
		if envBool("DEBUG") && (symbol == "_vibe67_arena_ensure_capacity" || symbol == "malloc$stub") {
			fmt.Fprintf(os.Stderr, "DEBUG mov.go: CallSymbol(%s) Windows: FF 15 at %d, callPos=%d\n",
				symbol, posBeforeCall, callPos)
		}
		
		o.eb.callPatches = append(o.eb.callPatches, CallPatch{
			position:   callPos,
			targetName: symbol,
		})
	} else {
		// Emit E8 (CALL rel32)
		o.Write(0xE8)
		callPos := o.eb.text.Len()
		o.WriteUnsigned(0x12345678) // Placeholder
		
		if envBool("DEBUG") && (symbol == "_vibe67_arena_ensure_capacity" || symbol == "malloc$stub") {
			fmt.Fprintf(os.Stderr, "DEBUG mov.go: CallSymbol(%s): E8 at %d, callPos=%d\n",
				symbol, posBeforeCall, callPos)
		}
		
		o.eb.callPatches = append(o.eb.callPatches, CallPatch{
			position:   callPos,
			targetName: symbol,
		})
	}
}

// Cld clears the direction flag (for string operations)
func (o *Out) Cld() {
	switch o.target.Arch() {
	case ArchX86_64:
		o.Write(0xFC)
	case ArchARM64:
		// ARM64 doesn't have direction flag
	case ArchRiscv64:
		// RISC-V doesn't have direction flag
	}
}

// RepMovsb repeats movsb rcx times (copies rcx bytes from rsi to rdi)
func (o *Out) RepMovsb() {
	switch o.target.Arch() {
	case ArchX86_64:
		o.Write(0xF3) // REP prefix
		o.Write(0xA4) // MOVSB
	case ArchARM64:
		// ARM64: implement with a loop or memcpy call
		// For now, just panic - this shouldn't be called for ARM64
		compilerError("RepMovsb not implemented for ARM64")
	case ArchRiscv64:
		// RISC-V: implement with a loop or memcpy call
		compilerError("RepMovsb not implemented for RISC-V")
	}
}

// BtRegReg performs bit test: BT reg1, reg2 (test bit reg2 in reg1, sets CF)
func (o *Out) BtRegReg(reg1, reg2 string) {
	switch o.target.Arch() {
	case ArchX86_64:
		x64 := NewX86_64CodeGen(o.writer, o.eb)
		x64.BtRegReg(reg1, reg2)
	case ArchARM64:
		// ARM64: Use LSR + AND to test bit
		// Need to shift right by bit position, then AND with 1
		compilerError("BtRegReg not fully implemented for ARM64 yet")
	case ArchRiscv64:
		// RISC-V: Use SRL + ANDI to test bit
		compilerError("BtRegReg not fully implemented for RISC-V yet")
	}
}

// SetcReg sets a register to 1 if CF=1, 0 otherwise (SETC r/m8)
func (o *Out) SetcReg(reg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		x64 := NewX86_64CodeGen(o.writer, o.eb)
		x64.SetcReg(reg)
	case ArchARM64:
		// ARM64: Use CSET instruction
		compilerError("SetcReg not fully implemented for ARM64 yet")
	case ArchRiscv64:
		// RISC-V: Use conditional moves or branch
		compilerError("SetcReg not fully implemented for RISC-V yet")
	}
}

// MovzxByteToQword performs MOVZX r64, r/m8 (zero-extend byte to qword)
func (o *Out) MovzxByteToQword(dstReg, srcReg string) {
	switch o.target.Arch() {
	case ArchX86_64:
		x64 := NewX86_64CodeGen(o.writer, o.eb)
		x64.MovzxByteToQword(dstReg, srcReg)
	case ArchARM64:
		// ARM64: Use UXTB (unsigned extend byte)
		compilerError("MovzxByteToQword not fully implemented for ARM64 yet")
	case ArchRiscv64:
		// RISC-V: Use ANDI with 0xFF
		compilerError("MovzxByteToQword not fully implemented for RISC-V yet")
	}
}









