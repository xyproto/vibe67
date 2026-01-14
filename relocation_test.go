package main

import (
	"testing"
)

// TestPCRelocationPatchingX86 tests that PC relocations are correctly patched for x86-64
func TestPCRelocationPatchingX86(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Define a symbol
	eb.Define("test_symbol", "test data")
	eb.DefineAddr("test_symbol", 0x404000)

	// Generate LEA instruction that references the symbol
	out := NewOut(eb.target, eb.TextWriter(), eb)
	out.LeaSymbolToReg("rdi", "test_symbol")

	// Verify relocation was recorded
	if len(eb.pcRelocations) != 1 {
		t.Fatalf("Expected 1 PC relocation, got %d", len(eb.pcRelocations))
	}

	// Patch relocations
	textAddr := uint64(0x402000)
	eb.PatchPCRelocations(textAddr, 0, 0)

	// Check that the displacement was patched (not 0xDEADBEEF anymore)
	textBytes := eb.text.Bytes()
	if len(textBytes) < 7 {
		t.Fatalf("Text section too small: %d bytes", len(textBytes))
	}

	// Read the 4-byte displacement (last 4 bytes of LEA instruction)
	disp := uint32(textBytes[3]) |
		(uint32(textBytes[4]) << 8) |
		(uint32(textBytes[5]) << 16) |
		(uint32(textBytes[6]) << 24)

	// Should not be the placeholder
	if disp == 0xDEADBEEF {
		t.Errorf("Displacement still contains placeholder 0xDEADBEEF")
	}

	// Verify it's a valid displacement
	// RIP = textAddr + 3 (offset of displacement) + 4 (displacement size)
	ripAddr := textAddr + 7
	targetAddr := uint64(0x404000)
	expectedDisp := int32(int64(targetAddr) - int64(ripAddr))

	if disp != uint32(expectedDisp) {
		t.Errorf("Displacement mismatch: got 0x%x, expected 0x%x", disp, uint32(expectedDisp))
	}
}

// TestPCRelocationPatchingARM64 tests that PC relocations are correctly patched for ARM64
func TestPCRelocationPatchingARM64(t *testing.T) {
	eb, err := New("arm64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Define a symbol
	eb.Define("test_symbol", "test data")
	eb.DefineAddr("test_symbol", 0x404123) // Non-page-aligned to test low 12 bits

	// Generate ADRP + ADD instructions
	out := NewOut(eb.target, eb.TextWriter(), eb)
	out.LeaSymbolToReg("x0", "test_symbol")

	// Verify relocation was recorded
	if len(eb.pcRelocations) != 1 {
		t.Fatalf("Expected 1 PC relocation, got %d", len(eb.pcRelocations))
	}

	// Patch relocations
	textAddr := uint64(0x402000)
	eb.PatchPCRelocations(textAddr, 0, 0)

	// Check that instructions were patched
	textBytes := eb.text.Bytes()
	if len(textBytes) < 8 {
		t.Fatalf("Text section too small: %d bytes", len(textBytes))
	}

	// Read ADRP instruction
	adrpInstr := uint32(textBytes[0]) |
		(uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) |
		(uint32(textBytes[3]) << 24)

	// Read ADD instruction
	addInstr := uint32(textBytes[4]) |
		(uint32(textBytes[5]) << 8) |
		(uint32(textBytes[6]) << 16) |
		(uint32(textBytes[7]) << 24)

	// Verify ADRP has non-zero immediate
	immlo := (adrpInstr >> 29) & 0x3
	immhi := (adrpInstr >> 5) & 0x7FFFF
	if immlo == 0 && immhi == 0 {
		t.Errorf("ADRP immediate still zero after patching")
	}

	// Verify ADD has correct low 12 bits (0x123)
	imm12 := (addInstr >> 10) & 0xFFF
	expectedLow12 := uint32(0x123)
	if imm12 != expectedLow12 {
		t.Errorf("ADD imm12 mismatch: got 0x%x, expected 0x%x", imm12, expectedLow12)
	}
}

// TestPCRelocationPatchingRISCV tests that PC relocations are correctly patched for RISC-V
func TestPCRelocationPatchingRISCV(t *testing.T) {
	eb, err := New("riscv64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Define a symbol
	eb.Define("test_symbol", "test data")
	eb.DefineAddr("test_symbol", 0x404000)

	// Generate AUIPC + ADDI instructions
	out := NewOut(eb.target, eb.TextWriter(), eb)
	out.LeaSymbolToReg("a0", "test_symbol")

	// Verify relocation was recorded
	if len(eb.pcRelocations) != 1 {
		t.Fatalf("Expected 1 PC relocation, got %d", len(eb.pcRelocations))
	}

	// Patch relocations
	textAddr := uint64(0x402000)
	eb.PatchPCRelocations(textAddr, 0, 0)

	// Check that instructions were patched
	textBytes := eb.text.Bytes()
	if len(textBytes) < 8 {
		t.Fatalf("Text section too small: %d bytes", len(textBytes))
	}

	// Read AUIPC instruction
	auipcInstr := uint32(textBytes[0]) |
		(uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) |
		(uint32(textBytes[3]) << 24)

	// Read ADDI instruction
	addiInstr := uint32(textBytes[4]) |
		(uint32(textBytes[5]) << 8) |
		(uint32(textBytes[6]) << 16) |
		(uint32(textBytes[7]) << 24)

	// Verify AUIPC has non-zero upper immediate
	upper := (auipcInstr >> 12) & 0xFFFFF
	if upper == 0 {
		t.Errorf("AUIPC immediate still zero after patching")
	}

	// Calculate expected values
	pcOffset := int64(0x404000 - textAddr)
	expectedUpper := uint32((pcOffset+0x800)>>12) & 0xFFFFF
	expectedLower := uint32(pcOffset & 0xFFF)

	if upper != expectedUpper {
		t.Errorf("AUIPC upper mismatch: got 0x%x, expected 0x%x", upper, expectedUpper)
	}

	// Verify ADDI has correct lower 12 bits
	lower := (addiInstr >> 20) & 0xFFF
	if lower != expectedLower {
		t.Errorf("ADDI lower mismatch: got 0x%x, expected 0x%x", lower, expectedLower)
	}
}









