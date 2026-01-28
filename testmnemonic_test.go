package main

import (
	"bytes"
	"testing"
)

// TestTestX86RegWithReg verifies TEST instruction encoding for x86_64
func TestTestX86RegWithReg(t *testing.T) {
	target := NewTarget(ArchX86_64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// TEST rax, rax - most common zero check
	// Expected: 48 85 C0 (REX.W TEST rax, rax)
	out.TestRegWithReg("rax", "rax")
	result := buf.Bytes()
	expected := []byte{0x48, 0x85, 0xC0}
	if !bytes.Equal(result, expected) {
		t.Errorf("TEST rax, rax: got %x, want %x", result, expected)
	}
}

func TestTestX86RegWithRegR8(t *testing.T) {
	target := NewTarget(ArchX86_64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// TEST r8, r8 - extended register
	// Expected: 4D 85 C0 (REX.W+R+B TEST r8, r8)
	out.TestRegWithReg("r8", "r8")
	result := buf.Bytes()
	expected := []byte{0x4D, 0x85, 0xC0}
	if !bytes.Equal(result, expected) {
		t.Errorf("TEST r8, r8: got %x, want %x", result, expected)
	}
}

func TestTestX86RegWithImm(t *testing.T) {
	target := NewTarget(ArchX86_64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// TEST rax, 0xFF - special encoding for RAX
	// Expected: 48 A9 FF 00 00 00
	out.TestRegWithImm("rax", 0xFF)
	result := buf.Bytes()
	expected := []byte{0x48, 0xA9, 0xFF, 0x00, 0x00, 0x00}
	if !bytes.Equal(result, expected) {
		t.Errorf("TEST rax, 0xFF: got %x, want %x", result, expected)
	}
}

func TestTestX86OtherRegWithImm(t *testing.T) {
	target := NewTarget(ArchX86_64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// TEST rbx, 0x1 - non-RAX register
	// Expected: 48 F7 C3 01 00 00 00
	out.TestRegWithImm("rbx", 0x1)
	result := buf.Bytes()
	expected := []byte{0x48, 0xF7, 0xC3, 0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(result, expected) {
		t.Errorf("TEST rbx, 0x1: got %x, want %x", result, expected)
	}
}

func TestTestARM64RegWithReg(t *testing.T) {
	target := NewTarget(ArchARM64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// TST x0, x0 (ANDS xzr, x0, x0)
	// Expected: 1F 00 00 EA
	out.TestRegWithReg("x0", "x0")
	result := buf.Bytes()
	expected := []byte{0x1F, 0x00, 0x00, 0xEA}
	if !bytes.Equal(result, expected) {
		t.Errorf("TST x0, x0: got %x, want %x", result, expected)
	}
}

func TestTestRISCVRegWithReg(t *testing.T) {
	target := NewTarget(ArchRiscv64, OSLinux)
	var buf bytes.Buffer
	bw := &BufferWrapper{buf: &buf}
	eb := &ExecutableBuilder{text: buf, target: target}
	out := NewOut(target, bw, eb)

	// AND x0, x10, x11 (test x10 with x11, result discarded)
	out.TestRegWithReg("x10", "x11")
	result := buf.Bytes()
	// Verify it's a valid RISC-V AND instruction
	if len(result) != 4 {
		t.Errorf("RISC-V TEST should be 4 bytes, got %d", len(result))
	}
}

// Benchmark TEST vs CMP for zero checking
func BenchmarkTestVsCmp(b *testing.B) {
	target := NewTarget(ArchX86_64, OSLinux)

	b.Run("TEST_rax_rax", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			bw := &BufferWrapper{buf: &buf}
			eb := &ExecutableBuilder{text: buf, target: target}
			out := NewOut(target, bw, eb)
			out.TestRegWithReg("rax", "rax")
		}
	})

	b.Run("CMP_rax_0", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			bw := &BufferWrapper{buf: &buf}
			eb := &ExecutableBuilder{text: buf, target: target}
			out := NewOut(target, bw, eb)
			out.CmpRegToImm("rax", 0)
		}
	})
}
