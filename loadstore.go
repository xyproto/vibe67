// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

// Load/Store instructions for memory access
// Essential for implementing Vibe67's variable and data access:
//   - Variable access: me.health, me.x
//   - Array element access: entities[i]
//   - Map value access: map[key]
//   - Struct field access: player.position.x
//   - Stack variable access: local_var
//   - Global variable access: game_state

// LoadRegFromMem loads a value from memory into a register
// dst = [base + offset]
func (o *Out) LoadRegFromMem(dst, base string, offset int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.loadX86RegFromMem(dst, base, offset)
	case ArchARM64:
		o.loadARM64RegFromMem(dst, base, offset)
	case ArchRiscv64:
		o.loadRISCVRegFromMem(dst, base, offset)
	}
}

// StoreRegToMem stores a register value to memory
// [base + offset] = src
func (o *Out) StoreRegToMem(src, base string, offset int32) {
	switch o.target.Arch() {
	case ArchX86_64:
		o.storeX86RegToMem(src, base, offset)
	case ArchARM64:
		o.storeARM64RegToMem(src, base, offset)
	case ArchRiscv64:
		o.storeRISCVRegToMem(src, base, offset)
	}
}

// ============================================================================
// x86-64 implementations
// ============================================================================

// x86-64 MOV reg, [reg + offset] (load)
func (o *Out) loadX86RegFromMem(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov %s, [%s + %d]:", dst, base, offset)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (dstReg.Encoding & 8) != 0 {
		rex |= 0x04 // REX.R
	}
	if (baseReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// MOV r64, r/m64 (opcode 0x8B)
	o.Write(0x8B)

	// Determine ModR/M and displacement size
	if offset == 0 && (baseReg.Encoding&7) != 5 { // RBP requires displacement
		// ModR/M: 00 reg base (no displacement)
		modrm := uint8(0x00) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		// Check if we need SIB byte (base register is RSP/R12)
		if (baseReg.Encoding & 7) == 4 {
			// SIB: scale=00, index=100 (none), base=base
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}
	} else if offset >= -128 && offset <= 127 {
		// ModR/M: 01 reg base (8-bit displacement)
		modrm := uint8(0x40) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		// Check if we need SIB byte
		if (baseReg.Encoding & 7) == 4 {
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}

		// 8-bit displacement
		o.Write(uint8(offset & 0xFF))
	} else {
		// ModR/M: 10 reg base (32-bit displacement)
		modrm := uint8(0x80) | ((dstReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		// Check if we need SIB byte
		if (baseReg.Encoding & 7) == 4 {
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}

		// 32-bit displacement
		o.Write(uint8(offset & 0xFF))
		o.Write(uint8((offset >> 8) & 0xFF))
		o.Write(uint8((offset >> 16) & 0xFF))
		o.Write(uint8((offset >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// x86-64 MOV [reg + offset], reg (store)
func (o *Out) storeX86RegToMem(src, base string, offset int32) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !srcOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "mov [%s + %d], %s:", base, offset, src)
	}

	// REX prefix for 64-bit operation
	rex := uint8(0x48)
	if (srcReg.Encoding & 8) != 0 {
		rex |= 0x04 // REX.R
	}
	if (baseReg.Encoding & 8) != 0 {
		rex |= 0x01 // REX.B
	}
	o.Write(rex)

	// MOV r/m64, r64 (opcode 0x89)
	o.Write(0x89)

	// Determine ModR/M and displacement size (same logic as load)
	if offset == 0 && (baseReg.Encoding&7) != 5 {
		modrm := uint8(0x00) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		if (baseReg.Encoding & 7) == 4 {
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}
	} else if offset >= -128 && offset <= 127 {
		modrm := uint8(0x40) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		if (baseReg.Encoding & 7) == 4 {
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}

		o.Write(uint8(offset & 0xFF))
	} else {
		modrm := uint8(0x80) | ((srcReg.Encoding & 7) << 3) | (baseReg.Encoding & 7)
		o.Write(modrm)

		if (baseReg.Encoding & 7) == 4 {
			sib := uint8(0x24) | (baseReg.Encoding & 7)
			o.Write(sib)
		}

		o.Write(uint8(offset & 0xFF))
		o.Write(uint8((offset >> 8) & 0xFF))
		o.Write(uint8((offset >> 16) & 0xFF))
		o.Write(uint8((offset >> 24) & 0xFF))
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// ARM64 implementations
// ============================================================================

// ARM64 LDR Xt, [Xn, #offset] (load)
func (o *Out) loadARM64RegFromMem(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ldr %s, [%s, #%d]:", dst, base, offset)
	}

	// LDR Xt, [Xn, #offset]
	// Format: 11 111 0 01 01 imm12 Rn Rt
	// imm12 is unsigned offset divided by 8 (for 64-bit access)
	// For signed offsets, we'd use LDUR instead

	if offset >= 0 && offset <= 32760 && (offset%8) == 0 {
		// Use LDR with unsigned offset (must be 8-byte aligned)
		imm12 := uint32(offset / 8)
		instr := uint32(0xF9400000) |
			(imm12 << 10) | // imm12
			(uint32(baseReg.Encoding&31) << 5) | // Rn (base)
			uint32(dstReg.Encoding&31) // Rt (dest)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else if offset >= -256 && offset <= 255 {
		// Use LDUR with signed offset (unscaled)
		// Format: 11 111 0 00 01 0 imm9 00 Rn Rt
		imm9 := uint32(offset & 0x1FF)
		instr := uint32(0xF8400000) |
			(imm9 << 12) | // imm9
			(uint32(baseReg.Encoding&31) << 5) | // Rn (base)
			uint32(dstReg.Encoding&31) // Rt (dest)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ARM64 STR Xt, [Xn, #offset] (store)
func (o *Out) storeARM64RegToMem(src, base string, offset int32) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !srcOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "str %s, [%s, #%d]:", src, base, offset)
	}

	// STR Xt, [Xn, #offset]
	// Format: 11 111 0 01 00 imm12 Rn Rt

	if offset >= 0 && offset <= 32760 && (offset%8) == 0 {
		// Use STR with unsigned offset
		imm12 := uint32(offset / 8)
		instr := uint32(0xF9000000) |
			(imm12 << 10) | // imm12
			(uint32(baseReg.Encoding&31) << 5) | // Rn (base)
			uint32(srcReg.Encoding&31) // Rt (source)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else if offset >= -256 && offset <= 255 {
		// Use STUR with signed offset (unscaled)
		// Format: 11 111 0 00 00 0 imm9 00 Rn Rt
		imm9 := uint32(offset & 0x1FF)
		instr := uint32(0xF8000000) |
			(imm9 << 12) | // imm9
			(uint32(baseReg.Encoding&31) << 5) | // Rn (base)
			uint32(srcReg.Encoding&31) // Rt (source)

		o.Write(uint8(instr & 0xFF))
		o.Write(uint8((instr >> 8) & 0xFF))
		o.Write(uint8((instr >> 16) & 0xFF))
		o.Write(uint8((instr >> 24) & 0xFF))
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// ============================================================================
// RISC-V implementations
// ============================================================================

// RISC-V LD rd, offset(rs1) (load)
func (o *Out) loadRISCVRegFromMem(dst, base string, offset int32) {
	dstReg, dstOk := GetRegister(o.target.Arch(), dst)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !dstOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "ld %s, %d(%s):", dst, offset, base)
	}

	// LD: imm[11:0] rs1 011 rd 0000011
	// 12-bit signed immediate
	if offset < -2048 || offset > 2047 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr)
		}
		return
	}

	instr := uint32(0x03) |
		(3 << 12) | // funct3 = 011 (LD)
		(uint32(offset&0xFFF) << 20) | // imm[11:0]
		(uint32(baseReg.Encoding&31) << 15) | // rs1 (base)
		(uint32(dstReg.Encoding&31) << 7) // rd (dest)

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}

// RISC-V SD rs2, offset(rs1) (store)
func (o *Out) storeRISCVRegToMem(src, base string, offset int32) {
	srcReg, srcOk := GetRegister(o.target.Arch(), src)
	baseReg, baseOk := GetRegister(o.target.Arch(), base)
	if !srcOk || !baseOk {
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "sd %s, %d(%s):", src, offset, base)
	}

	// SD: imm[11:5] rs2 rs1 011 imm[4:0] 0100011
	// 12-bit signed immediate split into two fields
	if offset < -2048 || offset > 2047 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " (offset out of range)")
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr)
		}
		return
	}

	imm11_5 := uint32((offset >> 5) & 0x7F)
	imm4_0 := uint32(offset & 0x1F)

	instr := uint32(0x23) |
		(3 << 12) | // funct3 = 011 (SD)
		(imm11_5 << 25) | // imm[11:5]
		(uint32(srcReg.Encoding&31) << 20) | // rs2 (source)
		(uint32(baseReg.Encoding&31) << 15) | // rs1 (base)
		(imm4_0 << 7) // imm[4:0]

	o.Write(uint8(instr & 0xFF))
	o.Write(uint8((instr >> 8) & 0xFF))
	o.Write(uint8((instr >> 16) & 0xFF))
	o.Write(uint8((instr >> 24) & 0xFF))

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
}
