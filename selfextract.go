package main

import (
	"bytes"
	"encoding/binary"
)

// Tiny RLE compressor for self-extracting executables
// Format: [count:u8][byte:u8]... where count > 0 means literal, count = 0 means end
func compressTinyRLE(data []byte) []byte {
	if len(data) == 0 {
		return []byte{0} // Just end marker
	}

	var result bytes.Buffer
	i := 0

	for i < len(data) {
		// Find run of same byte
		runStart := i
		runByte := data[i]
		for i < len(data) && data[i] == runByte && i-runStart < 255 {
			i++
		}
		runLen := i - runStart

		if runLen >= 3 {
			// Encode as run: count | 0x80, byte
			result.WriteByte(byte(runLen) | 0x80)
			result.WriteByte(runByte)
		} else {
			// Encode as literal
			result.WriteByte(byte(runLen))
			for j := 0; j < runLen; j++ {
				result.WriteByte(data[runStart+j])
			}
		}
	}

	result.WriteByte(0) // End marker
	return result.Bytes()
}

// Generate x86-64 decompressor stub
// This stub decompresses data and jumps to it
func generateDecompressorStub_x64(compressedSize, decompressedSize uint64) []byte {
	var stub bytes.Buffer

	// The stub will:
	// 1. mmap() memory for decompressed code
	// 2. Decompress inline data to that memory
	// 3. Jump to decompressed code

	// mmap(NULL, size, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
	// syscall 9 on Linux x86-64

	// mov rax, 9 (mmap)
	stub.Write([]byte{0x48, 0xc7, 0xc0, 0x09, 0x00, 0x00, 0x00})

	// xor rdi, rdi (addr = NULL)
	stub.Write([]byte{0x48, 0x31, 0xff})

	// mov rsi, decompressedSize
	stub.Write([]byte{0x48, 0xbe})
	binary.Write(&stub, binary.LittleEndian, decompressedSize)

	// mov rdx, 7 (PROT_READ|PROT_WRITE|PROT_EXEC)
	stub.Write([]byte{0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00})

	// mov r10, 0x22 (MAP_PRIVATE|MAP_ANONYMOUS)
	stub.Write([]byte{0x49, 0xc7, 0xc2, 0x22, 0x00, 0x00, 0x00})

	// mov r8, -1 (fd)
	stub.Write([]byte{0x49, 0xc7, 0xc0, 0xff, 0xff, 0xff, 0xff})

	// xor r9, r9 (offset = 0)
	stub.Write([]byte{0x4d, 0x31, 0xc9})

	// syscall
	stub.Write([]byte{0x0f, 0x05})

	// rax now contains the mapped memory address
	// Save it in r15
	stub.Write([]byte{0x49, 0x89, 0xc7}) // mov r15, rax

	// Now decompress: RSI = source (compressed data), RDI = dest (r15), RCX = compressed size
	// lea rsi, [rip + compressed_data]
	stub.Write([]byte{0x48, 0x8d, 0x35})
	// Offset will be: size of remaining stub
	remainingStubSize := uint32(7 + 8 + 3 + 2) // Approximate
	binary.Write(&stub, binary.LittleEndian, remainingStubSize)

	// mov rdi, r15 (destination)
	stub.Write([]byte{0x4c, 0x89, 0xff})

	// mov rcx, compressedSize
	stub.Write([]byte{0x48, 0xb9})
	binary.Write(&stub, binary.LittleEndian, compressedSize)

	// call decompress (inline)
	stub.Write([]byte{0xe8, 0x00, 0x00, 0x00, 0x00}) // Will be patched

	// jmp r15 (jump to decompressed code)
	stub.Write([]byte{0x41, 0xff, 0xe7})

	// Inline decompressor (tiny RLE decoder)
	decompStart := len(stub.Bytes())
	decompressor := generateTinyRLEDecompressor_x64()
	stub.Write(decompressor)

	// Patch the call offset
	stubBytes := stub.Bytes()
	callOffset := int32(decompStart - (len(stubBytes) - len(decompressor) - 5 + 5))
	binary.LittleEndian.PutUint32(stubBytes[len(stubBytes)-len(decompressor)-4:], uint32(callOffset))

	return stubBytes
}

func generateTinyRLEDecompressor_x64() []byte {
	// Tiny RLE decompressor
	// Input: RSI = source, RDI = dest, RCX = source size
	// Format: [count:u8] then if high bit set: [byte:u8] (run), else: count literal bytes

	var code bytes.Buffer

	// .loop:
	loopStart := 0

	// test rcx, rcx; jz .done
	code.Write([]byte{0x48, 0x85, 0xc9})
	code.Write([]byte{0x74, 0x00}) // Will be patched
	doneJumpPos := code.Len() - 1

	// movzx eax, byte [rsi]; inc rsi; dec rcx
	code.Write([]byte{0x0f, 0xb6, 0x06, 0x48, 0xff, 0xc6, 0x48, 0xff, 0xc9})

	// test al, 0x80; jz .literal
	code.Write([]byte{0xa8, 0x80})
	code.Write([]byte{0x74, 0x00}) // Will be patched
	literalJumpPos := code.Len() - 1

	// Run: and eax, 0x7f; movzx ebx, byte [rsi]; inc rsi; dec rcx
	code.Write([]byte{0x83, 0xe0, 0x7f})
	code.Write([]byte{0x0f, 0xb6, 0x1e, 0x48, 0xff, 0xc6, 0x48, 0xff, 0xc9})

	// .copy_run: test eax, eax; jz .loop
	copyRunStart := code.Len() - loopStart
	code.Write([]byte{0x85, 0xc0})
	code.Write([]byte{0x74, byte(-int8(copyRunStart + 4))}) // Jump back to loop

	// mov [rdi], bl; inc rdi; dec eax; jmp .copy_run
	code.Write([]byte{0x88, 0x1f, 0x48, 0xff, 0xc7, 0xff, 0xc8})
	code.Write([]byte{0xeb, byte(-int8(code.Len() - copyRunStart + 2))})

	// .literal: test eax, eax; jz .loop
	literalStart := code.Len() - loopStart
	codeBytes := code.Bytes()
	codeBytes[literalJumpPos] = byte(literalStart - literalJumpPos - 1)
	code = *bytes.NewBuffer(codeBytes)

	code.Write([]byte{0x85, 0xc0})
	code.Write([]byte{0x74, byte(-int8(literalStart + 4))})

	// .copy_literal: test eax, eax; jz .loop; test rcx, rcx; jz .done
	copyLiteralStart := code.Len() - loopStart
	code.Write([]byte{0x85, 0xc0})
	code.Write([]byte{0x74, byte(-int8(copyLiteralStart + 4))})
	code.Write([]byte{0x48, 0x85, 0xc9})
	code.Write([]byte{0x74, 0x00}) // Will be patched to done

	// movsx bl, byte [rsi]; mov [rdi]; inc rsi; inc rdi; dec rcx; dec eax; jmp .copy_literal
	code.Write([]byte{0x8a, 0x1e, 0x88, 0x1f})
	code.Write([]byte{0x48, 0xff, 0xc6, 0x48, 0xff, 0xc7, 0x48, 0xff, 0xc9, 0xff, 0xc8})
	code.Write([]byte{0xeb, byte(-int8(code.Len() - copyLiteralStart + 2))})

	// .done: ret
	doneStart := code.Len() - loopStart
	codeBytes = code.Bytes()
	codeBytes[doneJumpPos] = byte(doneStart - doneJumpPos - 1)
	code = *bytes.NewBuffer(codeBytes)

	code.Write([]byte{0xc3})

	return code.Bytes()
}









