package main

import (
	"testing"
)

// TestLeaSymbolToReg tests LEA with RIP-relative addressing
func TestLeaSymbolToReg(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Enable dynamic linking to trigger LEA generation
	eb.useDynamicLinking = true

	// Define a symbol
	eb.Define("message", "Hello\x00")
	eb.DefineAddr("message", 0x1000)

	// Create Out structure
	out := NewOut(eb.target, &BufferWrapper{&eb.text}, eb)

	// Generate LEA instruction: lea rdi, [rip + message]
	out.LeaSymbolToReg("rdi", "message")

	textBytes := eb.text.Bytes()
	if len(textBytes) < 7 {
		t.Fatalf("Text section too small for LEA instruction: got %d bytes", len(textBytes))
	}

	// Check encoding: REX.W + LEA opcode + ModR/M + 4-byte displacement
	// REX.W = 0x48 (64-bit operation)
	if textBytes[0] != 0x48 {
		t.Errorf("Expected REX.W prefix (0x48), got 0x%x", textBytes[0])
	}

	// LEA opcode = 0x8D
	if textBytes[1] != 0x8D {
		t.Errorf("Expected LEA opcode (0x8D), got 0x%x", textBytes[1])
	}

	// ModR/M for RDI + RIP-relative: 0x3D (00 111 101)
	if textBytes[2] != 0x3D {
		t.Errorf("Expected ModR/M (0x3D), got 0x%x", textBytes[2])
	}

	// Placeholder displacement
	if textBytes[3] != 0xEF || textBytes[4] != 0xBE || textBytes[5] != 0xAD || textBytes[6] != 0xDE {
		t.Errorf("Expected placeholder 0xDEADBEEF, got %x %x %x %x",
			textBytes[3], textBytes[4], textBytes[5], textBytes[6])
	}
}

// TestLeaImmToReg tests LEA with base + offset
func TestLeaImmToReg(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, &BufferWrapper{&eb.text}, eb)

	// Test LEA with 8-bit displacement: lea rax, [rsp + 16]
	out.LeaImmToReg("rax", "rsp", 16)

	textBytes := eb.text.Bytes()
	if len(textBytes) < 4 {
		t.Fatalf("Text section too small: got %d bytes", len(textBytes))
	}

	// Check encoding: REX.W + LEA + ModR/M + disp8
	if textBytes[0] != 0x48 {
		t.Errorf("Expected REX.W (0x48), got 0x%x", textBytes[0])
	}

	if textBytes[1] != 0x8D {
		t.Errorf("Expected LEA opcode (0x8D), got 0x%x", textBytes[1])
	}

	// ModR/M for RAX + RSP + disp8: 0x44 (01 000 100)
	if textBytes[2] != 0x44 {
		t.Errorf("Expected ModR/M (0x44), got 0x%x", textBytes[2])
	}

	// Displacement
	if textBytes[3] != 0x10 {
		t.Errorf("Expected displacement (0x10), got 0x%x", textBytes[3])
	}
}

// TestLeaARM64 tests ARM64 ADRP instruction
func TestLeaARM64(t *testing.T) {
	eb, err := New("aarch64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.Define("data", "test\x00")

	out := NewOut(eb.target, &BufferWrapper{&eb.text}, eb)

	// Generate ADRP for loading symbol address
	out.LeaSymbolToReg("x0", "data")

	textBytes := eb.text.Bytes()
	if len(textBytes) < 4 {
		t.Fatalf("Text section too small: got %d bytes", len(textBytes))
	}

	// ADRP encoding should have opcode bits set
	// Verify it's a 32-bit instruction with ADRP pattern
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check ADRP opcode pattern (bits [31,28:24] should be 1_10000)
	if (instr & 0x9F000000) != 0x90000000 {
		t.Errorf("Expected ADRP opcode pattern, got 0x%x", instr)
	}

	// Check destination register (X0 = 0)
	if (instr & 0x1F) != 0 {
		t.Errorf("Expected X0 as destination, got 0x%x", instr&0x1F)
	}
}

// TestLeaRISCV tests RISC-V AUIPC instruction
func TestLeaRISCV(t *testing.T) {
	eb, err := New("riscv64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.Define("label", "value\x00")

	out := NewOut(eb.target, &BufferWrapper{&eb.text}, eb)

	// Generate AUIPC for loading symbol address
	out.LeaSymbolToReg("a0", "label")

	textBytes := eb.text.Bytes()
	if len(textBytes) < 4 {
		t.Fatalf("Text section too small: got %d bytes", len(textBytes))
	}

	// AUIPC encoding
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check AUIPC opcode (bits [6:0] = 0010111 = 0x17)
	if (instr & 0x7F) != 0x17 {
		t.Errorf("Expected AUIPC opcode (0x17), got 0x%x", instr&0x7F)
	}

	// Check destination register (a0 = x10 = 10)
	rd := (instr >> 7) & 0x1F
	if rd != 10 {
		t.Errorf("Expected a0 (x10), got x%d", rd)
	}
}

// TestMovVsLea verifies MOV is used for non-PIE, LEA for PIE
func TestMovVsLea(t *testing.T) {
	// Non-PIE: should use MOV
	eb1, _ := New("x86_64")
	eb1.useDynamicLinking = false
	eb1.Define("sym1", "data\x00")
	eb1.DefineAddr("sym1", 0x2000)

	out1 := NewOut(eb1.target, &BufferWrapper{&eb1.text}, eb1)

	out1.MovInstruction("rax", "sym1")
	mov := eb1.text.Bytes()

	// Should contain MOV opcode (0xC7)
	found := false
	for _, b := range mov {
		if b == 0xC7 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Non-PIE should use MOV instruction (0xC7)")
	}

	// PIE: should use LEA
	eb2, _ := New("x86_64")
	eb2.useDynamicLinking = true
	eb2.Define("sym2", "data\x00")
	eb2.DefineAddr("sym2", 0x2000)

	out2 := NewOut(eb2.target, &BufferWrapper{&eb2.text}, eb2)

	out2.MovInstruction("rax", "sym2")
	lea := eb2.text.Bytes()

	// Should contain LEA opcode (0x8D)
	found = false
	for _, b := range lea {
		if b == 0x8D {
			found = true
			break
		}
	}
	if !found {
		t.Error("PIE should use LEA instruction (0x8D)")
	}
}
