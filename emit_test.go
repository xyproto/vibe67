package main

import (
	"bytes"
	"testing"
)

// TestEmitSyscall tests syscall emission
func TestEmitSyscall(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.Emit("syscall")
	if err != nil {
		t.Fatalf("Failed to emit syscall: %v", err)
	}

	// x86_64 syscall is 0x0f 0x05
	textBytes := eb.text.Bytes()
	if len(textBytes) < 2 {
		t.Fatal("Text section too small for syscall")
	}

	// Check for syscall instruction at the end
	lastTwo := textBytes[len(textBytes)-2:]
	if lastTwo[0] != 0x0f || lastTwo[1] != 0x05 {
		t.Errorf("Expected syscall (0f 05), got %x %x", lastTwo[0], lastTwo[1])
	}
}

// TestEmitCallInstruction tests call instruction emission
func TestEmitCallInstruction(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.Emit("call printf")
	if err != nil {
		t.Fatalf("Failed to emit call: %v", err)
	}

	// Call instruction is 0xe8 followed by 4-byte offset
	textBytes := eb.text.Bytes()
	if len(textBytes) < 5 {
		t.Fatal("Text section too small for call instruction")
	}

	// First byte should be CALL opcode
	if textBytes[0] != 0xe8 {
		t.Errorf("Expected CALL opcode (0xe8), got 0x%x", textBytes[0])
	}

	// Next 4 bytes should be placeholder (0x12345678 in little-endian)
	if textBytes[1] != 0x78 || textBytes[2] != 0x56 || textBytes[3] != 0x34 || textBytes[4] != 0x12 {
		t.Errorf("Expected placeholder 78 56 34 12, got %x %x %x %x",
			textBytes[1], textBytes[2], textBytes[3], textBytes[4])
	}
}

// TestBufferWrapper tests BufferWrapper write methods
func TestBufferWrapper(t *testing.T) {
	var buf bytes.Buffer
	bw := &BufferWrapper{&buf}

	// Test Write
	bw.Write(0x42)
	if buf.Bytes()[0] != 0x42 {
		t.Errorf("Write failed: expected 0x42, got 0x%x", buf.Bytes()[0])
	}

	// Test Write2
	buf.Reset()
	bw.Write2(0x12)
	bytes := buf.Bytes()
	if len(bytes) != 2 || bytes[0] != 0x12 || bytes[1] != 0x00 {
		t.Errorf("Write2 failed: got %v", bytes)
	}

	// Test Write4
	buf.Reset()
	bw.Write4(0xAB)
	bytes = buf.Bytes()
	if len(bytes) != 4 || bytes[0] != 0xAB {
		t.Errorf("Write4 failed: got %v", bytes)
	}

	// Test Write8
	buf.Reset()
	bw.Write8(0xCD)
	bytes = buf.Bytes()
	if len(bytes) != 8 || bytes[0] != 0xCD {
		t.Errorf("Write8 failed: got %v", bytes)
	}

	// Test WriteUnsigned
	buf.Reset()
	bw.WriteUnsigned(0x12345678)
	bytes = buf.Bytes()
	// Should be little-endian: 78 56 34 12
	if len(bytes) != 4 || bytes[0] != 0x78 || bytes[1] != 0x56 || bytes[2] != 0x34 || bytes[3] != 0x12 {
		t.Errorf("WriteUnsigned failed: got %v", bytes)
	}
}

// TestLookup tests symbol lookup
func TestLookup(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Test syscall number lookup (Linux syscalls)
	writeNum := eb.Lookup("SYS_WRITE")
	if writeNum != "1" {
		t.Errorf("SYS_WRITE = %s, want 1", writeNum)
	}

	exitNum := eb.Lookup("SYS_EXIT")
	if exitNum != "60" {
		t.Errorf("SYS_EXIT = %s, want 60", exitNum)
	}

	// Test constant lookup
	eb.Define("test", "value")
	eb.DefineAddr("test", 0x1234)

	result := eb.Lookup("test")
	if result != "4660" { // 0x1234 in decimal
		t.Errorf("Constant lookup = %s, want 4660", result)
	}

	// Test unknown lookup
	unknown := eb.Lookup("UNKNOWN")
	if unknown != "0" {
		t.Errorf("Unknown lookup = %s, want 0", unknown)
	}
}

// TestARM64Syscall tests ARM64 syscall numbers
func TestARM64Syscall(t *testing.T) {
	eb, err := New("aarch64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// ARM64 Linux uses different syscall numbers than x86_64
	writeNum := eb.Lookup("SYS_WRITE")
	if writeNum != "64" {
		t.Errorf("ARM64 SYS_WRITE = %s, want 64", writeNum)
	}

	exitNum := eb.Lookup("SYS_EXIT")
	if exitNum != "93" {
		t.Errorf("ARM64 SYS_EXIT = %s, want 93", exitNum)
	}
}

// TestPlatformStringConversion tests platform type conversions
func TestPlatformStringConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected Arch
		wantErr  bool
	}{
		{"x86_64", ArchX86_64, false},
		{"amd64", ArchX86_64, false},
		{"aarch64", ArchARM64, false},
		{"arm64", ArchARM64, false},
		{"riscv64", ArchRiscv64, false},
		{"riscv", ArchRiscv64, false},
		{"rv64", ArchRiscv64, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := StringToMachine(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for input %s", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %s: %v", tt.input, err)
				}
				if result.Arch != tt.expected {
					t.Errorf("StringToMachine(%s).Arch = %v, want %v", tt.input, result.Arch, tt.expected)
				}
			}
		})
	}
}

// TestPlatformToString tests platform to string conversion
func TestPlatformToString(t *testing.T) {
	tests := []struct {
		platform Platform
		expected string
	}{
		{Platform{Arch: ArchX86_64, OS: OSLinux}, "x86_64"},
		{Platform{Arch: ArchARM64, OS: OSLinux}, "aarch64"},
		{Platform{Arch: ArchRiscv64, OS: OSLinux}, "riscv64"},
	}

	for _, tt := range tests {
		result := tt.platform.String()
		if result != tt.expected {
			t.Errorf("Platform{Arch: %v}.String() = %s, want %s", tt.platform.Arch, result, tt.expected)
		}
	}
}

func TestEmitVectorMnemonics(t *testing.T) {
	cases := []struct {
		name string
		asm  string
	}{
		{name: "VMOVUPDLoad", asm: "vmovupd zmm0, [rdi + 32]"},
		{name: "VMOVUPDStore", asm: "vmovupd [rsi-16], zmm1"},
		{name: "VBROADCASTSDReg", asm: "vbroadcastsd zmm2, xmm3"},
		{name: "VBROADCASTSDMem", asm: "vbroadcastsd zmm4, [rax + 8]"},
		{name: "VADDPD", asm: "vaddpd zmm5, zmm6, zmm7"},
		{name: "VMULPD", asm: "vmulpd zmm8, zmm9, zmm10"},
		{name: "VSUBPD", asm: "vsubpd zmm11, zmm12, zmm13"},
		{name: "VDIVPD", asm: "vdivpd zmm14, zmm15, zmm16"},
		{name: "VCMPPD", asm: "vcmppd k1, zmm17, zmm18, gt"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			eb, err := New("x86_64")
			if err != nil {
				t.Fatalf("New x86_64 builder failed: %v", err)
			}

			before := eb.text.Len()
			if err := eb.Emit(tt.asm); err != nil {
				t.Fatalf("Emit(%q) returned error: %v", tt.asm, err)
			}
			after := eb.text.Len()
			if after <= before {
				t.Fatalf("Emit(%q) did not emit any bytes", tt.asm)
			}
		})
	}
}

func TestEmitVCMPPDInvalidPredicate(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("New x86_64 builder failed: %v", err)
	}

	if err := eb.Emit("vcmppd k1, zmm0, zmm1, invalid"); err == nil {
		t.Fatalf("Emit with invalid predicate succeeded")
	}
}

// TestRodataSection tests Rodata section handling
func TestRodataSection(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.Define("str1", "Hello\x00")
	eb.Define("str2", "World!\x00")

	rodata := eb.RodataSection()

	if len(rodata) != 2 {
		t.Errorf("Rodata section size = %d, want 2", len(rodata))
	}

	if rodata["str1"] != "Hello\x00" {
		t.Errorf("str1 = %q, want %q", rodata["str1"], "Hello\x00")
	}

	if rodata["str2"] != "World!\x00" {
		t.Errorf("str2 = %q, want %q", rodata["str2"], "World!\x00")
	}

	expectedSize := len("Hello\x00") + len("World!\x00")
	actualSize := eb.RodataSize()
	if actualSize != expectedSize {
		t.Errorf("RodataSize() = %d, want %d", actualSize, expectedSize)
	}
}

// TestWriteRodata tests Rodata section writing
func TestWriteRodata(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	data := []byte("test data\x00")
	n := eb.WriteRodata(data)

	if n != uint64(len(data)) {
		t.Errorf("WriteRodata returned %d, want %d", n, len(data))
	}

	rodataBytes := eb.rodata.Bytes()
	if !bytes.Equal(rodataBytes, data) {
		t.Errorf("Rodata content = %v, want %v", rodataBytes, data)
	}
}

// TestPrependBytes tests prepending bytes to text section
func TestPrependBytes(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Write some initial data
	eb.text.Write([]byte{0x01, 0x02, 0x03})

	// Prepend data
	eb.PrependBytes([]byte{0xAA, 0xBB})

	textBytes := eb.text.Bytes()
	expected := []byte{0xAA, 0xBB, 0x01, 0x02, 0x03}

	if !bytes.Equal(textBytes, expected) {
		t.Errorf("Text after prepend = %v, want %v", textBytes, expected)
	}
}

// TestEmitNoAssembly tests error handling for empty assembly
func TestEmitNoAssembly(t *testing.T) {
	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.Emit("")
	if err != errNoAssembly {
		t.Errorf("Emit(\"\") error = %v, want %v", err, errNoAssembly)
	}

	err = eb.Emit("   ")
	if err != errNoAssembly {
		t.Errorf("Emit(\"   \") error = %v, want %v", err, errNoAssembly)
	}
}
