package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

func TestRLECompression(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"single", []byte{42}},
		{"repeated", []byte{0xAA, 0xAA, 0xAA, 0xAA, 0xAA}},
		{"mixed", []byte{0x11, 0x11, 0x22, 0x33, 0x33, 0x33}},
		{"hello", []byte("Hello, World!")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := compressRLE(tt.input)
			decompressed, err := decompressRLE(compressed)
			if err != nil {
				t.Fatalf("decompression failed: %v", err)
			}
			if !bytes.Equal(decompressed, tt.input) {
				t.Errorf("mismatch: got %v, want %v", decompressed, tt.input)
			}
		})
	}
}

func TestDecompressorStub(t *testing.T) {
	t.Skip("decompressor stub not fully implemented yet")
	if testing.Short() {
		t.Skip("skipping decompressor stub test in short mode")
	}

	// Create a simple program: sys_write + sys_exit that prints "OK"
	// sys_write(1, msg, 3)
	// sys_exit(0)
	program := []byte{
		// mov rax, 1 (sys_write)
		0x48, 0xC7, 0xC0, 0x01, 0x00, 0x00, 0x00,
		// mov rdi, 1 (stdout)
		0x48, 0xC7, 0xC7, 0x01, 0x00, 0x00, 0x00,
		// lea rsi, [rip + msg]
		0x48, 0x8D, 0x35, 0x10, 0x00, 0x00, 0x00,
		// mov rdx, 3 (length)
		0x48, 0xC7, 0xC2, 0x03, 0x00, 0x00, 0x00,
		// syscall
		0x0F, 0x05,
		// mov rax, 60 (sys_exit)
		0x48, 0xC7, 0xC0, 0x3C, 0x00, 0x00, 0x00,
		// mov rdi, 0 (exit code)
		0x48, 0xC7, 0xC7, 0x00, 0x00, 0x00, 0x00,
		// syscall
		0x0F, 0x05,
		// msg: "OK\n"
		0x4F, 0x4B, 0x0A,
	}

	// Compress the program
	compressed := compressRLE(program)

	t.Logf("Original size: %d bytes", len(program))
	t.Logf("Compressed size: %d bytes", len(compressed))
	t.Logf("Compression ratio: %.1f%%", 100.0*float64(len(compressed))/float64(len(program)))

	// Build executable with stub
	stub := getDecompressorStubLinuxX64()

	// Patch the RIP-relative offset in the LEA instruction
	// LEA instruction starts at offset 0: 48 8D 35 [offset:4 bytes]
	// RIP points to the instruction AFTER the LEA (offset 7)
	// We want: RIP + offset = address of compressed data
	// compressed data starts at: len(stub)
	// So: 7 + offset = len(stub)
	// Therefore: offset = len(stub) - 7
	stubWithOffset := make([]byte, len(stub))
	copy(stubWithOffset, stub)

	offset := int32(len(stub) - 7)
	stubWithOffset[3] = byte(offset)
	stubWithOffset[4] = byte(offset >> 8)
	stubWithOffset[5] = byte(offset >> 16)
	stubWithOffset[6] = byte(offset >> 24)

	// Combine stub + compressed data
	executable := append(stubWithOffset, compressed...)

	// Write to file with ELF header
	elfFile := createMinimalELF(executable)

	testExe := "/tmp/test_decompress"
	if err := os.WriteFile(testExe, elfFile, 0755); err != nil {
		t.Fatalf("failed to write test executable: %v", err)
	}
	defer os.Remove(testExe)

	// Run it
	cmd := exec.Command(testExe)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execution failed: %v\nOutput: %s", err, output)
	}

	if string(output) != "OK\n" {
		t.Errorf("unexpected output: %q, want %q", output, "OK\n")
	}
}

// Create minimal ELF file with given code
func createMinimalELF(code []byte) []byte {
	// Minimal ELF64 header for Linux x86-64
	const baseAddr = 0x400000
	const headerSize = 0x40
	const phdrSize = 0x38

	elf := make([]byte, 0, headerSize+phdrSize+len(code))

	// ELF header
	elf = append(elf,
		0x7F, 'E', 'L', 'F', // Magic
		2, // 64-bit
		1, // Little-endian
		1, // ELF version
		0, // System V ABI
	)
	elf = append(elf, make([]byte, 8)...) // Padding

	elf = append(elf,
		2, 0, // Executable
		0x3E, 0, // x86-64
		1, 0, 0, 0, // Version
	)

	// Entry point
	entry := uint64(baseAddr + headerSize + phdrSize)
	elf = append(elf,
		byte(entry), byte(entry>>8), byte(entry>>16), byte(entry>>24),
		byte(entry>>32), byte(entry>>40), byte(entry>>48), byte(entry>>56),
	)

	// Program header offset
	phoff := uint64(headerSize)
	elf = append(elf,
		byte(phoff), byte(phoff>>8), byte(phoff>>16), byte(phoff>>24),
		byte(phoff>>32), byte(phoff>>40), byte(phoff>>48), byte(phoff>>56),
	)

	// Section header offset (none)
	elf = append(elf, 0, 0, 0, 0, 0, 0, 0, 0)

	// Flags
	elf = append(elf, 0, 0, 0, 0)

	// Header size
	elf = append(elf, headerSize, 0)

	// Program header size
	elf = append(elf, phdrSize, 0)

	// Program header count
	elf = append(elf, 1, 0)

	// Section header size
	elf = append(elf, 0, 0)

	// Section header count
	elf = append(elf, 0, 0)

	// Section name string table index
	elf = append(elf, 0, 0)

	// Program header
	elf = append(elf,
		1, 0, 0, 0, // PT_LOAD
		7, 0, 0, 0, // PF_R | PF_W | PF_X
	)

	// Offset in file
	offset := uint64(0)
	elf = append(elf,
		byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
		byte(offset>>32), byte(offset>>40), byte(offset>>48), byte(offset>>56),
	)

	// Virtual address
	vaddr := uint64(baseAddr)
	elf = append(elf,
		byte(vaddr), byte(vaddr>>8), byte(vaddr>>16), byte(vaddr>>24),
		byte(vaddr>>32), byte(vaddr>>40), byte(vaddr>>48), byte(vaddr>>56),
	)

	// Physical address
	elf = append(elf,
		byte(vaddr), byte(vaddr>>8), byte(vaddr>>16), byte(vaddr>>24),
		byte(vaddr>>32), byte(vaddr>>40), byte(vaddr>>48), byte(vaddr>>56),
	)

	// File size
	filesize := uint64(headerSize + phdrSize + len(code))
	elf = append(elf,
		byte(filesize), byte(filesize>>8), byte(filesize>>16), byte(filesize>>24),
		byte(filesize>>32), byte(filesize>>40), byte(filesize>>48), byte(filesize>>56),
	)

	// Memory size
	elf = append(elf,
		byte(filesize), byte(filesize>>8), byte(filesize>>16), byte(filesize>>24),
		byte(filesize>>32), byte(filesize>>40), byte(filesize>>48), byte(filesize>>56),
	)

	// Alignment
	align := uint64(0x1000)
	elf = append(elf,
		byte(align), byte(align>>8), byte(align>>16), byte(align>>24),
		byte(align>>32), byte(align>>40), byte(align>>48), byte(align>>56),
	)

	// Append code
	elf = append(elf, code...)

	return elf
}
