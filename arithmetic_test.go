package main

import (
	"testing"
)

// Test ADD instruction
func TestAddX86RegToReg(t *testing.T) {
	eb, _ := New("x86_64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.AddRegToReg("rax", "rbx")

	textBytes := eb.text.Bytes()
	// REX.W + ADD + ModR/M = 48 01 d8
	expected := []byte{0x48, 0x01, 0xD8}
	if len(textBytes) != 3 {
		t.Fatalf("Expected 3 bytes, got %d", len(textBytes))
	}
	for i, b := range expected {
		if textBytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, textBytes[i])
		}
	}
}

func TestAddX86ImmToReg(t *testing.T) {
	eb, _ := New("x86_64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.AddImmToReg("rax", 5)

	textBytes := eb.text.Bytes()
	// REX.W + ADD imm8 + ModR/M + imm = 48 83 c0 05
	expected := []byte{0x48, 0x83, 0xC0, 0x05}
	if len(textBytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(textBytes))
	}
	for i, b := range expected {
		if textBytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, textBytes[i])
		}
	}
}

func TestAddARM64RegToReg(t *testing.T) {
	eb, _ := New("arm64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.AddRegToReg("x0", "x1")

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check ADD opcode
	if (instr & 0xFFE00000) != 0x8B000000 {
		t.Errorf("Wrong ADD opcode: 0x%08X", instr)
	}
}

func TestAddRISCVRegToReg(t *testing.T) {
	eb, _ := New("riscv64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.AddRegToReg("a0", "a1")

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check opcode and funct7 for ADD
	if (instr & 0x7F) != 0x33 {
		t.Errorf("Wrong opcode: 0x%02X", instr&0x7F)
	}
	if ((instr >> 25) & 0x7F) != 0x00 {
		t.Errorf("Wrong funct7 for ADD: 0x%02X", (instr>>25)&0x7F)
	}
}

// Test SUB instruction
func TestSubX86RegFromReg(t *testing.T) {
	eb, _ := New("x86_64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.SubRegFromReg("rax", "rbx")

	textBytes := eb.text.Bytes()
	// REX.W + SUB + ModR/M = 48 29 d8
	expected := []byte{0x48, 0x29, 0xD8}
	if len(textBytes) != 3 {
		t.Fatalf("Expected 3 bytes, got %d", len(textBytes))
	}
	for i, b := range expected {
		if textBytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, textBytes[i])
		}
	}
}

func TestSubARM64RegFromReg(t *testing.T) {
	eb, _ := New("arm64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.SubRegFromReg("x0", "x1")

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check SUB opcode
	if (instr & 0xFFE00000) != 0xCB000000 {
		t.Errorf("Wrong SUB opcode: 0x%08X", instr)
	}
}

func TestSubRISCVRegFromReg(t *testing.T) {
	eb, _ := New("riscv64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.SubRegFromReg("a0", "a1")

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check funct7 for SUB (0x20)
	funct7 := (instr >> 25) & 0x7F
	if funct7 != 0x20 {
		t.Errorf("Wrong funct7 for SUB: 0x%02X, expected 0x20", funct7)
	}
}

// Test Jump instructions
func TestJumpX86Equal(t *testing.T) {
	eb, _ := New("x86_64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.JumpConditional(JumpEqual, 100)

	textBytes := eb.text.Bytes()
	if len(textBytes) != 6 {
		t.Fatalf("Expected 6 bytes, got %d", len(textBytes))
	}

	// 0F 84 for JE + 32-bit offset
	if textBytes[0] != 0x0F || textBytes[1] != 0x84 {
		t.Errorf("Wrong JE opcode: %02X %02X", textBytes[0], textBytes[1])
	}

	// Check offset (little-endian)
	offset := int32(uint32(textBytes[2]) | (uint32(textBytes[3]) << 8) |
		(uint32(textBytes[4]) << 16) | (uint32(textBytes[5]) << 24))
	if offset != 100 {
		t.Errorf("Wrong offset: %d, expected 100", offset)
	}
}

func TestJumpX86Unconditional(t *testing.T) {
	eb, _ := New("x86_64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.JumpUnconditional(200)

	textBytes := eb.text.Bytes()
	if len(textBytes) != 5 {
		t.Fatalf("Expected 5 bytes, got %d", len(textBytes))
	}

	// E9 for JMP
	if textBytes[0] != 0xE9 {
		t.Errorf("Wrong JMP opcode: %02X", textBytes[0])
	}

	offset := int32(uint32(textBytes[1]) | (uint32(textBytes[2]) << 8) |
		(uint32(textBytes[3]) << 16) | (uint32(textBytes[4]) << 24))
	if offset != 200 {
		t.Errorf("Wrong offset: %d, expected 200", offset)
	}
}

func TestJumpARM64Equal(t *testing.T) {
	eb, _ := New("arm64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.JumpConditional(JumpEqual, 32) // 8 instructions forward

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check B.cond opcode
	if (instr & 0xFF000000) != 0x54000000 {
		t.Errorf("Wrong B.cond opcode: 0x%08X", instr)
	}

	// Check condition (EQ = 0)
	cond := instr & 0xF
	if cond != 0 {
		t.Errorf("Wrong condition: %d, expected 0 (EQ)", cond)
	}
}

func TestJumpRISCVEqual(t *testing.T) {
	eb, _ := New("riscv64")
	out := NewOut(eb.target, eb.TextWriter(), eb)

	out.JumpConditional(JumpEqual, 8)

	textBytes := eb.text.Bytes()
	instr := uint32(textBytes[0]) | (uint32(textBytes[1]) << 8) |
		(uint32(textBytes[2]) << 16) | (uint32(textBytes[3]) << 24)

	// Check branch opcode (0x63)
	if (instr & 0x7F) != 0x63 {
		t.Errorf("Wrong branch opcode: 0x%02X", instr&0x7F)
	}

	// Check funct3 for BEQ (0)
	funct3 := (instr >> 12) & 0x7
	if funct3 != 0 {
		t.Errorf("Wrong funct3 for BEQ: %d", funct3)
	}
}
