// Static ELF generation for programs with no external dependencies
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// WriteCompleteStaticELF generates a fully static ELF executable (no dynamic linking)
// Returns (gotBase=0, rodataBase, textAddr, pltBase=0, error)
func (eb *ExecutableBuilder) WriteCompleteStaticELF(ds *DynamicSections) (gotBase, rodataAddr, textAddr, pltBase uint64, err error) {
	eb.elf.Reset()

	rodataSize := eb.rodata.Len()
	codeSize := eb.text.Len()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG [WriteCompleteStaticELF]: codeSize=%d, rodataSize=%d\n", codeSize, rodataSize)
	}

	// Calculate memory layout for static ELF
	// We need: PHDR, LOAD(ro), LOAD(rx), LOAD(rw) = 4 headers
	numProgHeaders := 4
	headersSize := uint64(elfHeaderSize + progHeaderSize*numProgHeaders)

	// Align to page boundary
	alignedHeaders := (headersSize + pageSize - 1) & ^uint64(pageSize-1)

	// Layout sections in memory
	layout := make(map[string]struct {
		offset uint64
		addr   uint64
		size   int
	})

	// Read-only data segment starts after headers
	currentOffset := uint64(alignedHeaders)
	currentAddr := baseAddr + currentOffset

	// No dynamic sections for static binaries!
	// Jump straight to executable segment

	// Align to page for executable segment
	currentOffset = (currentOffset + pageSize - 1) & ^uint64(pageSize-1)
	currentAddr = (currentAddr + pageSize - 1) & ^uint64(pageSize-1)

	// ._start (entry point)
	startSize := 14 // x86-64: 9 bytes xor + 5 bytes jmp
	if eb.target.Arch() == ArchARM64 {
		startSize = 24
	}
	layout["_start"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, startSize}
	entryPoint := currentAddr
	currentOffset += uint64((startSize + 7) & ^7)
	currentAddr += uint64((startSize + 7) & ^7)

	// .text (user code)
	actualCodeSize := uint64(codeSize)
	textReservedSize := ((actualCodeSize + pageSize - 1) & ^uint64(pageSize-1)) + pageSize
	layout["text"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, codeSize}
	textAddr = currentAddr
	currentOffset += textReservedSize
	currentAddr += textReservedSize

	// Align to page for writable segment
	currentOffset = (currentOffset + pageSize - 1) & ^uint64(pageSize-1)
	currentAddr = (currentAddr + pageSize - 1) & ^uint64(pageSize-1)

	// .rodata
	layout["rodata"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, rodataSize}
	rodataAddr = currentAddr
	eb.rodataOffsetInELF = currentOffset
	currentOffset += uint64(rodataSize)
	currentAddr += uint64(rodataSize)

	// .data
	layout["data"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, eb.data.Len()}

	// Write ELF file
	w := eb.ELFWriter()

	// ELF Header
	w.Write(0x7f)
	w.Write(0x45) // E
	w.Write(0x4c) // L
	w.Write(0x46) // F
	w.Write(2)    // 64-bit
	w.Write(1)    // Little-endian
	w.Write(1)    // ELF version
	w.Write(3)    // Linux
	w.WriteN(0, 8)
	w.Write2(2) // ET_EXEC (static executable, not DYN)
	w.Write2(byte(GetELFMachineType(eb.target.Arch())))
	w.Write4(1)

	w.Write8u(entryPoint)
	w.Write8u(elfHeaderSize)
	w.Write8u(0) // no section headers
	w.Write4(0)
	w.Write2(byte(elfHeaderSize))
	w.Write2(byte(progHeaderSize))
	w.Write2(byte(numProgHeaders))
	w.Write2(0)
	w.Write2(0)
	w.Write2(0)

	// Program Headers

	// PT_PHDR
	w.Write4(6) // PT_PHDR
	w.Write4(4) // PF_R
	w.Write8u(elfHeaderSize)
	w.Write8u(baseAddr + elfHeaderSize)
	w.Write8u(baseAddr + elfHeaderSize)
	w.Write8u(uint64(progHeaderSize * numProgHeaders))
	w.Write8u(uint64(progHeaderSize * numProgHeaders))
	w.Write8u(8)

	// PT_LOAD #0 (read-only: ELF header, program headers)
	roStart := uint64(0)
	roEnd := alignedHeaders
	roSize := roEnd - roStart
	w.Write4(1) // PT_LOAD
	w.Write4(4) // PF_R
	w.Write8u(roStart)
	w.Write8u(baseAddr + roStart)
	w.Write8u(baseAddr + roStart)
	w.Write8u(roSize)
	w.Write8u(roSize)
	w.Write8u(pageSize)

	// PT_LOAD #1 (executable: _start, text)
	exStart := layout["_start"].offset
	exEnd := layout["text"].offset + uint64(layout["text"].size)
	exSize := exEnd - exStart
	w.Write4(1) // PT_LOAD
	w.Write4(5) // PF_R | PF_X
	w.Write8u(exStart)
	w.Write8u(baseAddr + exStart)
	w.Write8u(baseAddr + exStart)
	w.Write8u(exSize)
	w.Write8u(exSize)
	w.Write8u(pageSize)

	// PT_LOAD #2 (writable: rodata, data)
	rwStart := layout["rodata"].offset
	rwFileSize := layout["rodata"].offset + uint64(layout["rodata"].size) + uint64(eb.data.Len()) - rwStart
	rwMemSize := rwFileSize
	w.Write4(1) // PT_LOAD
	w.Write4(6) // PF_R | PF_W
	w.Write8u(rwStart)
	w.Write8u(baseAddr + rwStart)
	w.Write8u(baseAddr + rwStart)
	w.Write8u(rwFileSize)
	w.Write8u(rwMemSize)
	w.Write8u(pageSize)

	// Pad to aligned header size
	for i := headersSize; i < alignedHeaders; i++ {
		w.Write(0)
	}

	// Pad to _start offset
	currentPos := eb.elf.Len()
	for i := currentPos; i < int(layout["_start"].offset); i++ {
		w.Write(0)
	}

	// _start function (minimal entry point that clears registers and jumps to user code)
	startAddr := layout["_start"].addr
	textAddrForJump := layout["text"].addr

	if eb.target.Arch() == ArchARM64 {
		// ARM64: Clear registers, call user code, then exit
		w.WriteBytes([]byte{0x00, 0x00, 0x80, 0xd2}) // mov x0, #0
		w.WriteBytes([]byte{0x01, 0x00, 0x80, 0xd2}) // mov x1, #0
		w.WriteBytes([]byte{0x02, 0x00, 0x80, 0xd2}) // mov x2, #0
		
		offset := int64(textAddrForJump) - int64(startAddr+12)
		offsetInstr := uint32((offset >> 2) & 0x3ffffff)
		blInstr := 0x94000000 | offsetInstr
		
		blBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(blBytes, blInstr)
		w.WriteBytes(blBytes)

		w.WriteBytes([]byte{0xe0, 0x03, 0x00, 0x91}) // mov x0, x0 (nop equivalent)
		w.WriteBytes([]byte{0xa8, 0x0b, 0x80, 0xd2}) // mov x8, #93 (exit syscall)
		w.WriteBytes([]byte{0x01, 0x00, 0x00, 0xd4}) // svc #0
	} else {
		// x86-64: Clear rax, rdi, rsi, then jmp to user code
		w.WriteBytes([]byte{0x48, 0x31, 0xc0})       // xor rax, rax
		w.WriteBytes([]byte{0x48, 0x31, 0xff})       // xor rdi, rdi
		w.WriteBytes([]byte{0x48, 0x31, 0xf6})       // xor rsi, rsi
		
		// jmp rel32 to text
		offset := int64(textAddrForJump) - int64(startAddr+9+5)
		w.Write(0xe9)
		offsetBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetBytes, uint32(offset))
		w.WriteBytes(offsetBytes)
	}

	// Pad to next 8-byte boundary
	for i := eb.elf.Len(); i < (eb.elf.Len()+7)&^7; i++ {
		w.Write(0)
	}

	// Pad to text section offset
	currentPos = eb.elf.Len()
	targetTextOffset := int(layout["text"].offset)
	for i := currentPos; i < targetTextOffset; i++ {
		w.Write(0)
	}

	// Update rodata addresses in consts map for static ELF
	rodataAddr = layout["rodata"].addr
	eb.rodataOffsetInELF = layout["rodata"].offset
	
	// Recalculate rodata symbol addresses based on actual layout
	// This mirrors the logic in codegen_elf_writer.go
// Update rodata addresses in consts map for static ELF
// The rodata buffer was already assembled with correct offsets during code generation
// We just need to update the base address
rodataAddr = layout["rodata"].addr
eb.rodataOffsetInELF = layout["rodata"].offset

// Find the minimum address in consts (the old base address)
var oldRodataBase uint64 = 0xFFFFFFFFFFFFFFFF
for _, c := range eb.consts {
!c.writable && c.addr < oldRodataBase {
= c.addr
Calculate the offset and update all rodata addresses
if oldRodataBase != 0xFFFFFFFFFFFFFFFF {
t := rodataAddr - oldRodataBase
name, c := range eb.consts {
!c.writable {
+= offsetAdjustment
sts[name] = c
Patch PC-relative relocations before writing text section
	// Patch direct function calls
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Patching function calls\n")
	}
	eb.PatchCallSites(layout["text"].addr)

	// Write text section
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing text at offset 0x%x, size %d\n", eb.elf.Len(), eb.text.Len())
	}
	w.WriteBytes(eb.text.Bytes())
	for i := codeSize; i < (codeSize+7)&^7; i++ {
		w.Write(0)
	}

	// Pad to rodata offset
	currentPos = eb.elf.Len()
	targetOffset := int(layout["rodata"].offset)
	for i := currentPos; i < targetOffset; i++ {
		w.Write(0)
	}

	// Write rodata
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing rodata at offset 0x%x, size %d\n", eb.elf.Len(), eb.rodata.Len())
	}
	w.WriteBytes(eb.rodata.Bytes())

	// Write data
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing data at offset 0x%x, size %d\n", eb.elf.Len(), eb.data.Len())
	}
	w.WriteBytes(eb.data.Bytes())

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Static ELF complete: %d bytes\n", eb.elf.Len())
	}

	return 0, rodataAddr, textAddr, 0, nil
}
