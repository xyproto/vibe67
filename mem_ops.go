// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

// Memory Operations for map[uint64]float64 runtime

// MovRegToMem - Store register to memory [base+offset]
func (o *Out) MovRegToMem(src, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movRegToMemX86(src, base, offset)
	}
}

func (o *Out) movRegToMemX86(src, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov [%s+%d], %s: ", base, offset, src)
	}

	srcReg, _ := GetRegister(o.target.Arch(), src)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// REX.W for 64-bit operation
	rex := uint8(0x48)
	if srcReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0x89 - MOV r/m64, r64
	o.Write(0x89)

	// ModR/M byte with displacement
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovMemToReg - Load from memory [base+offset] to register
func (o *Out) MovMemToReg(dst, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movMemToRegX86(dst, base, offset)
	}
}

func (o *Out) movMemToRegX86(dst, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, [%s+%d]: ", dst, base, offset)
	}

	dstReg, _ := GetRegister(o.target.Arch(), dst)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01
	}
	o.Write(rex)

	// 0x8B - MOV r64, r/m64
	o.Write(0x8B)

	// ModR/M
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ShlImmReg - Shift left by immediate
func (o *Out) ShlImmReg(dst string, imm int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.shlImmX86(dst, imm)
	}
}

func (o *Out) shlImmX86(dst string, imm int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "shl %s, %d: ", dst, imm)
	}

	dstReg, _ := GetRegister(o.target.Arch(), dst)

	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x01
	}
	o.Write(rex)

	if imm == 1 {
		// D1 /4 - SHL r/m64, 1
		o.Write(0xD1)
		modrm := uint8(0xE0) | (dstReg.Encoding & 7) // /4 = 100 in reg field
		o.Write(modrm)
	} else {
		// C1 /4 ib - SHL r/m64, imm8
		o.Write(0xC1)
		modrm := uint8(0xE0) | (dstReg.Encoding & 7)
		o.Write(modrm)
		o.Write(uint8(imm))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovByteRegToMem - Store byte from register to memory [base+offset]
func (o *Out) MovByteRegToMem(src, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movByteRegToMemX86(src, base, offset)
	}
}

func (o *Out) movByteRegToMemX86(src, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov byte [%s+%d], %s: ", base, offset, src)
	}

	srcReg, _ := GetRegister(o.target.Arch(), src)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// REX prefix for extended registers (no REX.W for byte operation)
	needREX := srcReg.Encoding >= 8 || baseReg.Encoding >= 8 || srcReg.Encoding >= 4
	if needREX {
		rex := uint8(0x40)
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}

	// 0x88 - MOV r/m8, r8
	o.Write(0x88)

	// ModR/M byte with displacement
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovImmToMem - Store immediate to memory [base+offset]
func (o *Out) MovImmToMem(imm int64, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movImmToMemX86(imm, base, offset)
	}
}

func (o *Out) movImmToMemX86(imm int64, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov qword [%s+%d], %d: ", base, offset, imm)
	}

	baseReg, _ := GetRegister(o.target.Arch(), base)

	// REX.W for 64-bit operation
	rex := uint8(0x48)
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0xC7 - MOV r/m64, imm32 (sign-extended to 64-bit)
	o.Write(0xC7)

	// ModR/M byte with /0 (reg field = 000)
	if offset == 0 && (baseReg.Encoding&7) != 5 { // rbp/r13 needs displacement
		modrm := uint8(0x00) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 { // rsp/r12 needs SIB
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	// Write 32-bit immediate (sign-extended to 64-bit)
	o.Write(uint8(imm & 0xFF))
	o.Write(uint8((imm >> 8) & 0xFF))
	o.Write(uint8((imm >> 16) & 0xFF))
	o.Write(uint8((imm >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// LeaMemToReg - Load effective address [base+offset] to register
func (o *Out) LeaMemToReg(dst, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.leaMemToRegX86(dst, base, offset)
	}
}

func (o *Out) leaMemToRegX86(dst, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lea %s, [%s+%d]: ", dst, base, offset)
	}

	dstReg, _ := GetRegister(o.target.Arch(), dst)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	rex := uint8(0x48) // REX.W for 64-bit
	if dstReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// 0x8D - LEA r64, m
	o.Write(0x8D)

	// ModR/M
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU8MemToReg emits a zero-extended byte load (MOVZX r64, byte [base+offset])
func (o *Out) MovU8MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU8MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movU8MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movzx %s, byte [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOVZX r64, r/m8 - opcode 0x0F 0xB6
	rex := uint8(0x48) // REX.W for 64-bit destination
	if destReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0x0F)
	o.Write(0xB6)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovI8MemToReg emits a sign-extended byte load (MOVSX r64, byte [base+offset])
func (o *Out) MovI8MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movI8MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movI8MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsx %s, byte [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOVSX r64, r/m8 - opcode 0x0F 0xBE
	rex := uint8(0x48) // REX.W for 64-bit destination
	if destReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0x0F)
	o.Write(0xBE)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU16MemToReg emits a zero-extended word load (MOVZX r64, word [base+offset])
func (o *Out) MovU16MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU16MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movU16MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movzx %s, word [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOVZX r64, r/m16 - opcode 0x0F 0xB7
	rex := uint8(0x48) // REX.W for 64-bit destination
	if destReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0x0F)
	o.Write(0xB7)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovI16MemToReg emits a sign-extended word load (MOVSX r64, word [base+offset])
func (o *Out) MovI16MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movI16MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movI16MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsx %s, word [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOVSX r64, r/m16 - opcode 0x0F 0xBF
	rex := uint8(0x48) // REX.W for 64-bit destination
	if destReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0x0F)
	o.Write(0xBF)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU32MemToReg emits a zero-extended dword load (MOV r32, [base+offset])
// Note: On x86-64, 32-bit operations automatically zero-extend to 64-bit
func (o *Out) MovU32MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU32MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movU32MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, dword [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOV r32, r/m32 - opcode 0x8B (no REX.W, auto zero-extends)
	// Only emit REX if we need extended registers
	needsRex := destReg.Encoding >= 8 || baseReg.Encoding >= 8
	if needsRex {
		rex := uint8(0x40) // REX prefix without W bit
		if destReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}
	o.Write(0x8B)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovI32MemToReg emits a sign-extended dword load (MOVSXD r64, [base+offset])
func (o *Out) MovI32MemToReg(dest, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movI32MemToRegX86(dest, base, offset)
	}
}

func (o *Out) movI32MemToRegX86(dest, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "movsxd %s, dword [%s", dest, base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "]: ")
	}

	destReg, _ := GetRegister(o.target.Arch(), dest)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOVSXD r64, r/m32 - opcode 0x63 with REX.W
	rex := uint8(0x48) // REX.W for 64-bit destination
	if destReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)
	o.Write(0x63)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((destReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU8RegToMem emits a byte store (MOV byte [base+offset], src)
func (o *Out) MovU8RegToMem(src, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU8RegToMemX86(src, base, offset)
	}
}

func (o *Out) movU8RegToMemX86(src, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov byte [%s", base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "], %s: ", src)
	}

	srcReg, _ := GetRegister(o.target.Arch(), src)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOV r/m8, r8 - opcode 0x88
	rex := uint8(0x40) // REX prefix (needed for accessing low byte of extended registers)
	needsRex := srcReg.Encoding >= 8 || baseReg.Encoding >= 8
	if needsRex {
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}
	o.Write(0x88)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU16RegToMem emits a word store (MOV word [base+offset], src)
func (o *Out) MovU16RegToMem(src, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU16RegToMemX86(src, base, offset)
	}
}

func (o *Out) movU16RegToMemX86(src, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov word [%s", base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "], %s: ", src)
	}

	srcReg, _ := GetRegister(o.target.Arch(), src)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOV r/m16, r16 - requires 0x66 prefix and opcode 0x89
	o.Write(0x66) // 16-bit operand size prefix

	needsRex := srcReg.Encoding >= 8 || baseReg.Encoding >= 8
	if needsRex {
		rex := uint8(0x40)
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}
	o.Write(0x89)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// MovU32RegToMem emits a dword store (MOV dword [base+offset], src)
func (o *Out) MovU32RegToMem(src, base string, offset int) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.movU32RegToMemX86(src, base, offset)
	}
}

func (o *Out) movU32RegToMemX86(src, base string, offset int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov dword [%s", base)
		if offset != 0 {
			if offset > 0 {
				fmt.Fprintf(os.Stderr, "+%d", offset)
			} else {
				fmt.Fprintf(os.Stderr, "%d", offset)
			}
		}
		fmt.Fprintf(os.Stderr, "], %s: ", src)
	}

	srcReg, _ := GetRegister(o.target.Arch(), src)
	baseReg, _ := GetRegister(o.target.Arch(), base)

	// MOV r/m32, r32 - opcode 0x89 (no REX.W)
	needsRex := srcReg.Encoding >= 8 || baseReg.Encoding >= 8
	if needsRex {
		rex := uint8(0x40)
		if srcReg.Encoding >= 8 {
			rex |= 0x04 // REX.R
		}
		if baseReg.Encoding >= 8 {
			rex |= 0x01 // REX.B
		}
		o.Write(rex)
	}
	o.Write(0x89)

	// ModR/M byte
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24) // SIB for RSP
		}
	} else if offset < 128 && offset >= -128 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.Write(uint8(offset))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		if (baseReg.Encoding & 7) == 4 {
			o.Write(0x24)
		}
		o.WriteUnsigned(uint(offset))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
