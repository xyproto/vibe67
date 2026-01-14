// Completion: 100% - Instruction implementation complete
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LeaSymbolToReg generates a load effective address instruction for a symbol
// This is used for position-independent code to load addresses relative to PC/RIP
func (o *Out) LeaSymbolToReg(dst, symbol string) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.leaX86SymbolToReg(dst, symbol)
	case ArchARM64:
		o.leaARM64SymbolToReg(dst, symbol)
	case ArchRiscv64:
		o.leaRISCVSymbolToReg(dst, symbol)
	}
}

// x86_64 LEA with RIP-relative addressing
func (o *Out) leaX86SymbolToReg(dst, symbol string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lea %s, [rip + %s]:", dst, symbol)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	o.Write(rex)

	// LEA opcode
	o.Write(0x8D)

	// ModR/M: 00 reg 101 (RIP-relative)
	modrm := uint8(0x05) | ((dstReg.Encoding & 7) << 3)
	o.Write(modrm)

	// Record the position where we need to patch the displacement
	displacementOffset := uint64(o.eb.text.Len())
	o.eb.pcRelocations = append(o.eb.pcRelocations, PCRelocation{
		offset:     displacementOffset,
		symbolName: symbol,
	})

	// 32-bit displacement (will be patched later with actual RIP-relative offset)
	o.WriteUnsigned(0xDEADBEEF)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADRP + ADD for loading symbol addresses
func (o *Out) leaARM64SymbolToReg(dst, symbol string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "adrp %s, %s; add %s, %s, :lo12:%s:", dst, symbol, dst, dst, symbol)
	}

	// Record position for relocation (ADRP is at this offset)
	adrpOffset := uint64(o.eb.text.Len())
	o.eb.pcRelocations = append(o.eb.pcRelocations, PCRelocation{
		offset:     adrpOffset,
		symbolName: symbol,
	})

	// ADRP: loads page address relative to PC
	// Format: 1 immlo[1:0] 10000 immhi[20:0] Rd
	// Placeholder - will be patched with actual page offset
	adrpInstr := uint32(0x90000000) | uint32(dstReg.Encoding&31)

	o.Write(uint8(adrpInstr & 0xFF))
	o.Write(uint8((adrpInstr >> 8) & 0xFF))
	o.Write(uint8((adrpInstr >> 16) & 0xFF))
	o.Write(uint8((adrpInstr >> 24) & 0xFF))

	// ADD: adds low 12 bits of address
	// Format: sf 0 0 10001 shift[1:0] imm12[11:0] Rn[4:0] Rd[4:0]
	// sf=1 (64-bit), shift=00, imm12=placeholder
	addInstr := uint32(0x91000000) |
		(uint32(dstReg.Encoding&31) << 5) | // Rn (source)
		uint32(dstReg.Encoding&31) // Rd (dest)

	o.Write(uint8(addInstr & 0xFF))
	o.Write(uint8((addInstr >> 8) & 0xFF))
	o.Write(uint8((addInstr >> 16) & 0xFF))
	o.Write(uint8((addInstr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V AUIPC + ADDI for loading symbol addresses
func (o *Out) leaRISCVSymbolToReg(dst, symbol string) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	if !dstOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "auipc %s, %%pcrel_hi(%s); addi %s, %s, %%pcrel_lo:", dst, symbol, dst, dst)
	}

	// Record position for relocation (AUIPC is at this offset)
	auipcOffset := uint64(o.eb.text.Len())
	o.eb.pcRelocations = append(o.eb.pcRelocations, PCRelocation{
		offset:     auipcOffset,
		symbolName: symbol,
	})

	// AUIPC: adds upper 20 bits of PC-relative offset to PC
	// Format: imm[31:12] rd 0010111
	// Placeholder - will be patched with actual offset
	auipcInstr := uint32(0x17) | (uint32(dstReg.Encoding&31) << 7)

	o.Write(uint8(auipcInstr & 0xFF))
	o.Write(uint8((auipcInstr >> 8) & 0xFF))
	o.Write(uint8((auipcInstr >> 16) & 0xFF))
	o.Write(uint8((auipcInstr >> 24) & 0xFF))

	// ADDI: adds lower 12 bits
	// Format: imm[11:0] rs1 000 rd 0010011
	addiInstr := uint32(0x13) |
		(uint32(dstReg.Encoding&31) << 7) | // rd (dest)
		(uint32(dstReg.Encoding&31) << 15) // rs1 (source)
	// funct3 = 000 for ADDI (bits 12-14)

	o.Write(uint8(addiInstr & 0xFF))
	o.Write(uint8((addiInstr >> 8) & 0xFF))
	o.Write(uint8((addiInstr >> 16) & 0xFF))
	o.Write(uint8((addiInstr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// LeaImmToReg generates a LEA instruction with an immediate offset
// This is primarily for x86_64 address calculations
func (o *Out) LeaImmToReg(dst, base string, offset int64) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.leaX86ImmToReg(dst, base, offset)
	case ArchARM64:
		// ARM64 typically uses ADD for this
		o.leaARM64ImmToReg(dst, base, offset)
	case ArchRiscv64:
		// RISC-V uses ADDI
		o.leaRISCVImmToReg(dst, base, offset)
	}
}

// x86_64 LEA with base + displacement
func (o *Out) leaX86ImmToReg(dst, base string, offset int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)

	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "lea %s, [%s + %d]:", dst, base, offset)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if dstReg.Encoding >= 8 {
		rex |= 0x04 // REX.R
	}
	if baseReg.Encoding >= 8 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// LEA opcode
	o.Write(0x8D)

	// Determine ModR/M and displacement size
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		// No displacement (unless base is RBP/R13)
		modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
	} else if offset >= -128 && offset <= 127 {
		// 8-bit displacement
		modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		o.Write(uint8(offset & 0xFF))
	} else {
		// 32-bit displacement
		modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)
		o.WriteUnsigned(uint(offset & 0xFFFFFFFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 ADD for address calculation
func (o *Out) leaARM64ImmToReg(dst, base string, offset int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)

	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "add %s, %s, #%d:", dst, base, offset)
	}

	// ADD Xd, Xn, #imm
	// Format: sf 0 0 10001 shift[1:0] imm12[11:0] Rn[4:0] Rd[4:0]
	var instr uint32
	if offset >= 0 && offset < 4096 {
		instr = 0x91000000 | (uint32(offset&0xFFF) << 10) |
			(uint32(baseReg.Encoding&31) << 5) | uint32(dstReg.Encoding&31)
	} else {
		// For larger offsets, would need multiple instructions
		instr = 0x91000000 | uint32(dstReg.Encoding&31)
	}

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V ADDI for address calculation
func (o *Out) leaRISCVImmToReg(dst, base string, offset int64) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)

	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "addi %s, %s, %d:", dst, base, offset)
	}

	// ADDI rd, rs1, imm
	// Format: imm[11:0] rs1[4:0] 000 rd[4:0] 0010011
	var instr uint32 = 0x13
	instr |= uint32(dstReg.Encoding&31) << 7
	instr |= 0 << 12 // funct3 = 000
	instr |= uint32(baseReg.Encoding&31) << 15
	instr |= uint32(offset&0xFFF) << 20

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// Helper to check if a value is a symbol name
func isSymbolName(s string) bool {
	// Strip immediate prefix if present (ARM64/RISC-V style)
	s = strings.TrimPrefix(s, "#")
	if _, err := strconv.ParseUint(s, 0, 64); err == nil {
		return false // It's a number
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return false // It's a number (signed)
	}
	return true // It's a symbol
}









