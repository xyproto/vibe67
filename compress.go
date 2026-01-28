package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

type Compressor struct {
	windowSize int
	minMatch   int
}

func NewCompressor() *Compressor {
	return &Compressor{
		windowSize: 32768,
		minMatch:   4,
	}
}

func (c *Compressor) Compress(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	var compressed bytes.Buffer

	binary.Write(&compressed, binary.LittleEndian, uint32(len(data)))

	pos := 0
	for pos < len(data) {
		bestLen := 0
		bestDist := 0

		searchStart := pos - c.windowSize
		if searchStart < 0 {
			searchStart = 0
		}

		for i := searchStart; i < pos; i++ {
			matchLen := 0
			for matchLen < 255 && pos+matchLen < len(data) && data[i+matchLen] == data[pos+matchLen] {
				matchLen++
			}

			if matchLen >= c.minMatch && matchLen > bestLen {
				bestLen = matchLen
				bestDist = pos - i
			}
		}

		if bestLen >= c.minMatch {
			compressed.WriteByte(0xFF)
			binary.Write(&compressed, binary.LittleEndian, uint16(bestDist))
			compressed.WriteByte(byte(bestLen))
			pos += bestLen
		} else {
			literal := data[pos]
			if literal == 0xFF {
				compressed.WriteByte(0xFF)
				compressed.WriteByte(0x00)
				compressed.WriteByte(0x00)
				compressed.WriteByte(0x01)
			} else {
				compressed.WriteByte(literal)
			}
			pos++
		}
	}

	return compressed.Bytes()
}

func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return data, nil
	}

	origSize := binary.LittleEndian.Uint32(data[0:4])
	decompressed := make([]byte, 0, origSize)

	pos := 4
	for pos < len(data) {
		if data[pos] == 0xFF {
			if pos+3 >= len(data) {
				break
			}
			dist := binary.LittleEndian.Uint16(data[pos+1 : pos+3])
			length := int(data[pos+3])

			if dist == 0 && length == 1 {
				decompressed = append(decompressed, 0xFF)
			} else {
				start := len(decompressed) - int(dist)
				for i := 0; i < length; i++ {
					decompressed = append(decompressed, decompressed[start+i])
				}
			}
			pos += 4
		} else {
			decompressed = append(decompressed, data[pos])
			pos++
		}
	}

	return decompressed, nil
}

func generateDecompressorStub(arch string, compressedSize, decompressedSize uint32) []byte {
	switch arch {
	case "amd64":
		return generateX64DecompressorStub(compressedSize, decompressedSize)
	case "arm64":
		return generateARM64DecompressorStub(compressedSize, decompressedSize)
	default:
		return nil
	}
}

func generateX64DecompressorStub(compressedSize, decompressedSize uint32) []byte {
	var stub []byte

	// Save registers we'll use
	stub = append(stub, 0x53)       // push rbx
	stub = append(stub, 0x55)       // push rbp
	stub = append(stub, 0x41, 0x54) // push r12
	stub = append(stub, 0x41, 0x55) // push r13

	// mmap(NULL, decompressedSize, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
	stub = append(stub, 0x48, 0x31, 0xFF) // xor rdi, rdi (addr = NULL)
	stub = append(stub, 0x48, 0xC7, 0xC6) // mov rsi, decompressedSize (low 32 bits)
	stub = append(stub, byte(decompressedSize), byte(decompressedSize>>8),
		byte(decompressedSize>>16), byte(decompressedSize>>24))
	stub = append(stub, 0x48, 0xC7, 0xC2, 0x07, 0x00, 0x00, 0x00) // mov rdx, 7 (PROT_R|W|X)
	stub = append(stub, 0x49, 0xC7, 0xC2, 0x22, 0x00, 0x00, 0x00) // mov r10, 0x22 (MAP_PRIVATE|ANON)
	stub = append(stub, 0x49, 0xC7, 0xC0, 0xFF, 0xFF, 0xFF, 0xFF) // mov r8, -1 (fd)
	stub = append(stub, 0x4D, 0x31, 0xC9)                         // xor r9, r9 (offset)
	stub = append(stub, 0x48, 0xC7, 0xC0, 0x09, 0x00, 0x00, 0x00) // mov rax, 9 (sys_mmap)
	stub = append(stub, 0x0F, 0x05)                               // syscall

	// Check for mmap failure
	stub = append(stub, 0x48, 0x85, 0xC0) // test rax, rax
	errorJmpPos := len(stub)
	stub = append(stub, 0x78, 0x00) // js error (will patch)

	// Save mapped address in r13 (our target to jump to)
	stub = append(stub, 0x49, 0x89, 0xC5) // mov r13, rax
	// Also save in rdi (destination for decompression)
	stub = append(stub, 0x48, 0x89, 0xC7) // mov rdi, rax

	// Get source pointer to compressed data
	stub = append(stub, 0x48, 0x8D, 0x35) // lea rsi, [rip+offset]
	leaOffsetPos := len(stub)
	stub = append(stub, 0x00, 0x00, 0x00, 0x00) // Placeholder (will patch)

	// Skip the 4-byte size header
	stub = append(stub, 0x48, 0x83, 0xC6, 0x04) // add rsi, 4

	// Calculate end of decompressed buffer: r12 = rdi + decompressedSize
	stub = append(stub, 0x49, 0x89, 0xFC) // mov r12, rdi
	stub = append(stub, 0x49, 0x81, 0xC4) // add r12, decompressedSize
	stub = append(stub, byte(decompressedSize), byte(decompressedSize>>8),
		byte(decompressedSize>>16), byte(decompressedSize>>24))

	// Main decompress loop
	decompressLoopStart := len(stub)
	// Check if we've filled the output buffer
	stub = append(stub, 0x4C, 0x39, 0xE7) // cmp rdi, r12
	doneJmpPos := len(stub)
	stub = append(stub, 0x73, 0x00) // jae done (will patch)

	// Read next byte from compressed stream
	stub = append(stub, 0xAC)       // lodsb (al = [rsi++])
	stub = append(stub, 0x3C, 0xFF) // cmp al, 0xFF
	literalJmpPos := len(stub)
	stub = append(stub, 0x75, 0x00) // jne literal (will patch)

	// Match case: read distance (2 bytes) and length (1 byte)
	stub = append(stub, 0x66, 0xAD)       // lodsw (ax = dist)
	stub = append(stub, 0x0F, 0xB7, 0xD8) // movzx ebx, ax (ebx = dist)
	stub = append(stub, 0xAC)             // lodsb (al = len)
	stub = append(stub, 0x0F, 0xB6, 0xD0) // movzx edx, al (edx = len)

	// Check for escaped 0xFF (dist==0, len==1)
	stub = append(stub, 0x66, 0x85, 0xDB) // test bx, bx
	copyMatchJmpPos := len(stub)
	stub = append(stub, 0x75, 0x00)       // jnz copy_match (will patch)
	stub = append(stub, 0x83, 0xFA, 0x01) // cmp edx, 1
	copyMatchJmpPos2 := len(stub)
	stub = append(stub, 0x75, 0x00) // jne copy_match (will patch)

	// Escaped 0xFF: write literal 0xFF
	stub = append(stub, 0xC6, 0x07, 0xFF) // mov byte [rdi], 0xFF
	stub = append(stub, 0x48, 0xFF, 0xC7) // inc rdi
	loopBackJmpPos := len(stub)
	stub = append(stub, 0xEB, 0x00) // jmp decompress_loop (will patch)

	// copy_match: copy edx bytes from [rdi-ebx]
	copyMatchLabel := len(stub)
	stub = append(stub, 0x48, 0x89, 0xF8) // mov rax, rdi
	stub = append(stub, 0x48, 0x29, 0xD8) // sub rax, rbx (rax = source)

	// copy_loop:
	copyLoopStart := len(stub)
	stub = append(stub, 0x8A, 0x08)       // mov cl, [rax]
	stub = append(stub, 0x88, 0x0F)       // mov [rdi], cl
	stub = append(stub, 0x48, 0xFF, 0xC0) // inc rax
	stub = append(stub, 0x48, 0xFF, 0xC7) // inc rdi
	stub = append(stub, 0xFF, 0xCA)       // dec edx
	copyLoopJmpPos := len(stub)
	stub = append(stub, 0x75, 0x00) // jnz copy_loop (will patch)
	loopBackJmpPos2 := len(stub)
	stub = append(stub, 0xEB, 0x00) // jmp decompress_loop (will patch)

	// literal: write byte to output
	literalLabel := len(stub)
	stub = append(stub, 0xAA) // stosb ([rdi++] = al)
	loopBackJmpPos3 := len(stub)
	stub = append(stub, 0xEB, 0x00) // jmp decompress_loop (will patch)

	// done: restore registers and jump to decompressed code
	doneLabel := len(stub)
	stub = append(stub, 0x41, 0x5D)       // pop r13
	stub = append(stub, 0x41, 0x5C)       // pop r12
	stub = append(stub, 0x5D)             // pop rbp
	stub = append(stub, 0x5B)             // pop rbx
	stub = append(stub, 0x41, 0xFF, 0xE5) // jmp r13 (jump to decompressed code)

	// error: exit with code 1
	errorLabel := len(stub)
	stub = append(stub, 0x48, 0xC7, 0xC0, 0x3C, 0x00, 0x00, 0x00) // mov rax, 60 (sys_exit)
	stub = append(stub, 0x48, 0xC7, 0xC7, 0x01, 0x00, 0x00, 0x00) // mov rdi, 1
	stub = append(stub, 0x0F, 0x05)                               // syscall

	// Patch all jump offsets
	stub[errorJmpPos+1] = byte(errorLabel - (errorJmpPos + 2))
	stub[doneJmpPos+1] = byte(doneLabel - (doneJmpPos + 2))
	stub[literalJmpPos+1] = byte(literalLabel - (literalJmpPos + 2))
	stub[copyMatchJmpPos+1] = byte(copyMatchLabel - (copyMatchJmpPos + 2))
	stub[copyMatchJmpPos2+1] = byte(copyMatchLabel - (copyMatchJmpPos2 + 2))
	stub[loopBackJmpPos+1] = byte(int(decompressLoopStart) - int(loopBackJmpPos+2))
	stub[copyLoopJmpPos+1] = byte(int(copyLoopStart) - int(copyLoopJmpPos+2))
	stub[loopBackJmpPos2+1] = byte(int(decompressLoopStart) - int(loopBackJmpPos2+2))
	stub[loopBackJmpPos3+1] = byte(int(decompressLoopStart) - int(loopBackJmpPos3+2))

	// Patch LEA offset
	ripAfterLea := leaOffsetPos + 4
	compressedDataOffset := len(stub) - ripAfterLea
	binary.LittleEndian.PutUint32(stub[leaOffsetPos:leaOffsetPos+4], uint32(compressedDataOffset))

	return stub
}

func uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, n)
	return b
}

func generateARM64DecompressorStub(compressedSize, decompressedSize uint32) []byte {
	// TODO: Implement ARM64 decompressor stub
	return []byte{}
}

// WrapWithDecompressor wraps an ELF executable with compression and decompressor stub
func WrapWithDecompressor(originalELF []byte, arch string) ([]byte, error) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: WrapWithDecompressor called for arch=%s, size=%d\n", arch, len(originalELF))
	}

	// NOTE: Compressing the entire ELF doesn't work because the ELF headers,
	// PLT, GOT, and relocations all assume specific virtual addresses.
	// When we decompress to a different mmap'd address, everything breaks.
	//
	// To properly implement compression, we would need to:
	// 1. Extract only the .text section (machine code)
	// 2. Compress that
	// 3. Have the decompressor write it back to the correct virtual address
	// 4. Or implement position-independent decompression
	//
	// For now, disable compression to avoid segfaults.

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Compression disabled for %s (needs position-independent code support)\n", arch)
	}
	return originalELF, nil

	// Keep the old code commented for reference:
	/*
		compressor := NewCompressor()

		// Compress the entire ELF
		compressed := compressor.Compress(originalELF)

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Compressed %d -> %d bytes\n", len(originalELF), len(compressed))
		}

		// Generate decompressor stub
		stub := generateDecompressorStub(arch, uint32(len(compressed)), uint32(len(originalELF)))
		if len(stub) == 0 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: No decompressor stub for arch %s\n", arch)
			}
			// Compression not supported for this arch, return original
			return originalELF, nil
		}

	*/

	// Rest of the function is never reached
}
