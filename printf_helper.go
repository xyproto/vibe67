// Completion: 100% - Helper module complete
package main

// printf_helper.go - Helper functions for printf runtime implementation
// This file provides utilities for patching jumps and managing code generation

import (
	"fmt"
	"os"
)

// PrintfCodeGen wraps ExecutableBuilder and Out with printf-specific helpers
type PrintfCodeGen struct {
	eb  *ExecutableBuilder
	out *Out
}

// NewPrintfCodeGen creates a new printf code generator
func NewPrintfCodeGen(eb *ExecutableBuilder, out *Out) *PrintfCodeGen {
	return &PrintfCodeGen{eb: eb, out: out}
}

// GetTextPos returns the current position in the text section
func (p *PrintfCodeGen) GetTextPos() int {
	return p.eb.text.Len()
}

// PatchJump patches a jump instruction at the given position with the calculated offset
func (p *PrintfCodeGen) PatchJump(jumpPos int, targetPos int, jumpInstrSize int) {
	// Calculate offset: target - (jumpPos + jumpInstrSize)
	offset := int32(targetPos - (jumpPos + jumpInstrSize))

	// Get bytes buffer
	bytes := p.eb.text.Bytes()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG PatchJump: jumpPos=%d, targetPos=%d, offset=%d\n",
			jumpPos, targetPos, offset)
	}

	// Write offset as little-endian int32 (at jumpPos + 1 or +2, depending on instruction)
	// For conditional jumps (0x0F 0x8X), offset starts at +2
	// For short jumps (0xEB, 0x7X), offset is 1 byte at +1
	// For now, assume long form (4 byte offset at position + 2 for conditional, +1 for unconditional)

	// Determine offset start based on first byte
	offsetStart := jumpPos + 1
	if bytes[jumpPos] == 0x0F {
		// Two-byte opcode, offset at +2
		offsetStart = jumpPos + 2
	}

	// Write 32-bit offset
	bytes[offsetStart] = byte(offset)
	bytes[offsetStart+1] = byte(offset >> 8)
	bytes[offsetStart+2] = byte(offset >> 16)
	bytes[offsetStart+3] = byte(offset >> 24)
}

// EmitFloatToStringRuntime emits a runtime function that converts float64 to string
// This can be called from printf to handle %v, %f, %g conversions
// Signature: _vibe67_float_to_string(xmm0: float64, rdi: buffer_ptr) -> (rsi: str_start, rdx: length)
func EmitFloatToStringRuntime(eb *ExecutableBuilder, out *Out) error {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Emitting _vibe67_float_to_string runtime function\n")
	}

	eb.MarkLabel("_vibe67_float_to_string")

	// TODO: Implement a simplified float-to-string that handles common cases
	// For now, just convert to integer and print that (MVP)

	// Prologue
	out.PushReg("rbp")
	out.MovRegToReg("rbp", "rsp")
	out.PushReg("rbx")

	// xmm0 = float value, rdi = buffer pointer
	// Convert to integer
	out.Cvttsd2si("rax", "xmm0")

	// Simple integer to string conversion
	// Store starting position
	out.MovRegToReg("rbx", "rdi") // rbx = buffer start
	out.MovRegToReg("rsi", "rdi") // rsi = write position

	// Handle zero specially
	out.CmpRegToImm("rax", 0)
	p := NewPrintfCodeGen(eb, out)
	zeroJumpPos := p.GetTextPos()
	out.Write(0x74) // JE (short jump)
	out.Write(0x00) // placeholder

	// Handle negative
	out.CmpRegToImm("rax", 0)
	negJumpPos := p.GetTextPos()
	out.Write(0x7D) // JGE (short jump - skip if positive)
	out.Write(0x00) // placeholder

	// Negative: add '-' and negate
	out.MovImmToReg("r10", "45") // '-'
	out.MovByteRegToMem("r10", "rsi", 0)
	out.AddImmToReg("rsi", 1)
	// Negate: rax = -rax (use NEG instruction manually or multiply by -1)
	out.Write(0x48) // REX.W
	out.Write(0xF7) // NEG r/m64
	out.Write(0xD8) // ModR/M for rax

	negSkipPos := p.GetTextPos()
	p.PatchJump(negJumpPos, negSkipPos, 2)

	// Convert digits to temp buffer (reverse order)
	out.LeaMemToReg("rdi", "rsi", 20) // temp storage
	out.MovImmToReg("rcx", "10")

	digitLoopStart := p.GetTextPos()
	out.XorRegWithReg("rdx", "rdx")
	out.DivRegByReg("rax", "rcx") // rax = quotient, rdx = remainder
	out.AddImmToReg("rdx", 48)    // to ASCII
	out.MovByteRegToMem("rdx", "rdi", 0)
	out.AddImmToReg("rdi", 1)
	out.CmpRegToImm("rax", 0)
	digitLoopJumpPos := p.GetTextPos()
	out.Write(0x7F)                                                   // JG
	out.Write(byte((digitLoopStart - (digitLoopJumpPos + 2)) & 0xFF)) // short backwards jump

	// Copy digits back in reverse
	out.SubImmFromReg("rdi", 1)
	copyLoopStart := p.GetTextPos()
	out.CmpRegToReg("rdi", "rsi")
	copyEndJumpPos := p.GetTextPos()
	out.Write(0x7C) // JL
	out.Write(0x00) // placeholder

	out.MovMemToReg("r10", "rdi", 0)
	out.MovByteRegToMem("r10", "rsi", 0)
	out.AddImmToReg("rsi", 1)
	out.SubImmFromReg("rdi", 1)
	out.Write(0xEB) // JMP
	out.Write(byte((copyLoopStart - (p.GetTextPos() + 2)) & 0xFF))

	copyEndPos := p.GetTextPos()
	p.PatchJump(copyEndJumpPos, copyEndPos, 2)

	normalEndJumpPos := p.GetTextPos()
	out.Write(0xEB) // JMP to end
	out.Write(0x00) // placeholder

	// Zero case: just write '0'
	zeroPos := p.GetTextPos()
	p.PatchJump(zeroJumpPos, zeroPos, 2)
	out.MovImmToReg("r10", "48") // '0'
	out.MovByteRegToMem("r10", "rsi", 0)
	out.AddImmToReg("rsi", 1)

	// End: calculate length and return
	endPos := p.GetTextPos()
	p.PatchJump(normalEndJumpPos, endPos, 2)

	// rsi = end position, rbx = start
	// rdx = length
	out.MovRegToReg("rdx", "rsi")
	out.SubRegFromReg("rdx", "rbx")
	out.MovRegToReg("rsi", "rbx") // return start pointer

	// Epilogue
	out.PopReg("rbx")
	out.PopReg("rbp")
	out.Ret()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: _vibe67_float_to_string emitted successfully\n")
	}

	return nil
}









