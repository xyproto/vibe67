package main

// Tiny decompressor stubs for self-extracting executables

// RLE decompressor stub for x86-64 Linux
// Decompresses RIP-relative data to mmap'd memory and jumps to it
func getDecompressorStubLinuxX64() []byte {
	return []byte{
		// Get compressed data address (RIP-relative, patched at link time)
		0x48, 0x8D, 0x35, 0x00, 0x00, 0x00, 0x00, // lea rsi, [rip + offset] ; offset patched later

		// Get decompressed size (first 4 bytes of compressed data)
		0x8B, 0x3E, // mov edi, [rsi]
		0x48, 0x83, 0xC6, 0x04, // add rsi, 4  ; skip size header

		// Save RSI (compressed data pointer)
		0x53,             // push rbx
		0x48, 0x89, 0xF3, // mov rbx, rsi

		// mmap(NULL, size, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
		0x48, 0x31, 0xF6, // xor rsi, rsi  ; addr = NULL
		// rdi already has size
		0xBA, 0x07, 0x00, 0x00, 0x00, // mov edx, 7 (PROT_READ|PROT_WRITE|PROT_EXEC)
		0x41, 0xBA, 0x22, 0x00, 0x00, 0x00, // mov r10d, 0x22 (MAP_PRIVATE|MAP_ANONYMOUS)
		0x41, 0xB8, 0xFF, 0xFF, 0xFF, 0xFF, // mov r8d, -1
		0x45, 0x31, 0xC9, // xor r9d, r9d (offset 0)
		0xB8, 0x09, 0x00, 0x00, 0x00, // mov eax, 9 (sys_mmap)
		0x0F, 0x05, // syscall

		// RAX now contains the mapped memory address
		0x49, 0x89, 0xC4, // mov r12, rax (dest pointer)
		0x49, 0x89, 0xC5, // mov r13, rax (save entry point)
		0x48, 0x89, 0xDE, // mov rsi, rbx (restore compressed data pointer)

		// Decompress RLE data: RSI = source, R12 = dest
		// Format: [count:1][byte:1]... count=0 is terminator
		// decompress_loop:
		0x0F, 0xB6, 0x0E, // movzx ecx, byte [rsi]  ; read count
		0x48, 0xFF, 0xC6, // inc rsi
		0x84, 0xC9, // test cl, cl            ; check for terminator
		0x74, 0x0D, // jz done

		0x0F, 0xB6, 0x06, // movzx eax, byte [rsi]  ; read byte to repeat
		0x48, 0xFF, 0xC6, // inc rsi

		// repeat:
		0x41, 0x88, 0x04, 0x24, // mov [r12], al          ; write byte
		0x49, 0xFF, 0xC4, // inc r12
		0xE2, 0xF8, // loop repeat            ; dec ecx; jnz

		0xEB, 0xE8, // jmp decompress_loop

		// done:
		0x5B, // pop rbx

		// Jump to decompressed code
		0x41, 0xFF, 0xE5, // jmp r13
	}
}

// RLE decompressor stub for x86-64 Windows
func getDecompressorStubWindowsX64() []byte {
	// Similar to Linux but uses VirtualAlloc instead of mmap
	// For now, simplified version
	return []byte{
		// TODO: Implement Windows version with VirtualAlloc
		// For now, just return Linux version as placeholder
	}
}

// Simple RLE compression
// Format: [decompressed_size:4][count:1][byte:1]... with count=0 as terminator
func compressRLE(data []byte) []byte {
	if len(data) == 0 {
		return []byte{0, 0, 0, 0, 0} // Empty: size=0, terminator
	}

	result := make([]byte, 0, len(data)/2) // Estimate

	// Add decompressed size (little-endian)
	size := uint32(len(data))
	result = append(result,
		byte(size),
		byte(size>>8),
		byte(size>>16),
		byte(size>>24),
	)

	i := 0
	for i < len(data) {
		b := data[i]
		count := 1

		// Count consecutive identical bytes
		for i+count < len(data) && data[i+count] == b && count < 255 {
			count++
		}

		result = append(result, byte(count), b)
		i += count
	}

	// Terminator
	result = append(result, 0)

	return result
}

// Decompress RLE data (for testing)
func decompressRLE(data []byte) ([]byte, error) {
	if len(data) < 5 {
		return nil, nil
	}

	// Read decompressed size
	size := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	result := make([]byte, 0, size)

	i := 4
	for i < len(data) {
		count := data[i]
		if count == 0 {
			break // Terminator
		}
		i++
		if i >= len(data) {
			break
		}
		b := data[i]
		i++

		for j := 0; j < int(count); j++ {
			result = append(result, b)
		}
	}

	return result, nil
}
