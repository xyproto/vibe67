package main

import (
	"testing"
)

// TestCmpX86RegToReg tests x86-64 CMP instruction generation
func TestCmpX86RegToReg(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP rax, rbx
	out.CmpRegToReg("rax", "rbx")

	textBytes := eb.text.Bytes()
	if len(textBytes) != 3 {
		t.Fatalf("Expected 3 bytes, got %d", len(textBytes))
	}

	// REX.W + CMP + ModR/M
	// 48 39 d8 = REX.W (48) + CMP r/m64, r64 (39) + ModR/M (11 011 000)
	expected := []byte{0x48, 0x39, 0xD8}
	for i, b := range expected {
		if textBytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, textBytes[i])
		}
	}
}

// TestCmpX86RegToImm8 tests x86-64 CMP with 8-bit immediate
func TestCmpX86RegToImm8(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP rax, 10
	out.CmpRegToImm("rax", 10)

	textBytes := eb.text.Bytes()
	if len(textBytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(textBytes))
	}

	// 48 83 f8 0a = REX.W + CMP r/m64, imm8 + ModR/M + imm8
	expected := []byte{0x48, 0x83, 0xF8, 0x0A}
	for i, b := range expected {
		if textBytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, textBytes[i])
		}
	}
}

// TestCmpX86RegToImm32 tests x86-64 CMP with 32-bit immediate
func TestCmpX86RegToImm32(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP rax, 1000
	out.CmpRegToImm("rax", 1000)

	textBytes := eb.text.Bytes()
	if len(textBytes) != 7 {
		t.Fatalf("Expected 7 bytes, got %d", len(textBytes))
	}

	// 48 81 f8 e8 03 00 00 = REX.W + CMP r/m64, imm32 + ModR/M + imm32
	if textBytes[0] != 0x48 || textBytes[1] != 0x81 || textBytes[2] != 0xF8 {
		t.Errorf("Wrong opcode bytes: %02X %02X %02X", textBytes[0], textBytes[1], textBytes[2])
	}

	// Check immediate (little-endian 1000 = 0x3E8)
	imm := uint32(textBytes[3]) | (uint32(textBytes[4]) << 8) |
		(uint32(textBytes[5]) << 16) | (uint32(textBytes[6]) << 24)
	if imm != 1000 {
		t.Errorf("Expected immediate 1000, got %d", imm)
	}
}

// TestCmpARM64RegToReg tests ARM64 CMP instruction generation
func TestCmpARM64RegToReg(t *testing.T) {
	eb, err := New("arm64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP x0, x1 (SUBS xzr, x0, x1)
	out.CmpRegToReg("x0", "x1")

	textBytes := eb.text.Bytes()
	if len(textBytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(textBytes))
	}

	// Decode instruction
	instr := uint32(textBytes[0]) |
		(uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) |
		(uint32(textBytes[3]) << 24)

	// Check SUBS opcode (bits 31-21 should be 11101011000)
	if (instr & 0xFFE00000) != 0xEB000000 {
		t.Errorf("Wrong SUBS opcode: 0x%08X", instr)
	}

	// Check registers: Rn=0 (x0), Rm=1 (x1), Rd=31 (xzr)
	rn := (instr >> 5) & 0x1F
	rm := (instr >> 16) & 0x1F
	rd := instr & 0x1F

	if rn != 0 {
		t.Errorf("Expected Rn=0, got %d", rn)
	}
	if rm != 1 {
		t.Errorf("Expected Rm=1, got %d", rm)
	}
	if rd != 31 {
		t.Errorf("Expected Rd=31 (xzr), got %d", rd)
	}
}

// TestCmpARM64RegToImm tests ARM64 CMP with immediate
func TestCmpARM64RegToImm(t *testing.T) {
	eb, err := New("arm64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP x0, #42
	out.CmpRegToImm("x0", 42)

	textBytes := eb.text.Bytes()
	if len(textBytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(textBytes))
	}

	// Decode instruction
	instr := uint32(textBytes[0]) |
		(uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) |
		(uint32(textBytes[3]) << 24)

	// Check immediate
	imm := (instr >> 10) & 0xFFF
	if imm != 42 {
		t.Errorf("Expected immediate 42, got %d", imm)
	}

	// Check Rd=31 (xzr)
	rd := instr & 0x1F
	if rd != 31 {
		t.Errorf("Expected Rd=31 (xzr), got %d", rd)
	}
}

// TestCmpRISCVRegToReg tests RISC-V CMP (SUB) instruction generation
func TestCmpRISCVRegToReg(t *testing.T) {
	eb, err := New("riscv64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Generate: CMP a0, a1 (implemented as SUB t0, a0, a1)
	out.CmpRegToReg("a0", "a1")

	textBytes := eb.text.Bytes()
	if len(textBytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(textBytes))
	}

	// Decode instruction
	instr := uint32(textBytes[0]) |
		(uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) |
		(uint32(textBytes[3]) << 24)

	// Check SUB opcode (0110011 = 0x33)
	if (instr & 0x7F) != 0x33 {
		t.Errorf("Wrong opcode: 0x%02X", instr&0x7F)
	}

	// Check funct3 = 000
	funct3 := (instr >> 12) & 0x7
	if funct3 != 0 {
		t.Errorf("Expected funct3=0, got %d", funct3)
	}

	// Check funct7 = 0100000 (SUB)
	funct7 := (instr >> 25) & 0x7F
	if funct7 != 0x20 {
		t.Errorf("Expected funct7=0x20 (SUB), got 0x%02X", funct7)
	}

	// Check registers: rs1=10 (a0), rs2=11 (a1), rd=5 (t0)
	rs1 := (instr >> 15) & 0x1F
	rs2 := (instr >> 20) & 0x1F
	rd := (instr >> 7) & 0x1F

	if rs1 != 10 {
		t.Errorf("Expected rs1=10 (a0), got %d", rs1)
	}
	if rs2 != 11 {
		t.Errorf("Expected rs2=11 (a1), got %d", rs2)
	}
	if rd != 5 {
		t.Errorf("Expected rd=5 (t0), got %d", rd)
	}
}









