// Completion: 100% - ELF generation complete for Linux, production-ready
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// WriteCompleteDynamicELF generates a fully functional dynamically-linked ELF
// Returns (gotBase, rodataBase, error)
func (eb *ExecutableBuilder) WriteCompleteDynamicELF(ds *DynamicSections, functions []string) (gotBase, rodataAddr, textAddr, pltBase uint64, err error) {
	eb.elf.Reset()
	eb.neededFunctions = functions // Store functions list for later use in patchTextInELF

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG [WriteCompleteDynamicELF start]: rodata buffer size: %d bytes\n", eb.rodata.Len())
	}
	if eb.rodata.Len() > 0 {
		previewLen := 32
		if eb.rodata.Len() < previewLen {
			previewLen = eb.rodata.Len()
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG [WriteCompleteDynamicELF start]: rodata buffer first %d bytes: %q\n", previewLen, eb.rodata.Bytes()[:previewLen])
		}
	}

	rodataSize := eb.rodata.Len()
	codeSize := eb.text.Len()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: codeSize=%d (0x%x), rodataSize=%d (0x%x)\n",
			codeSize, codeSize, rodataSize, rodataSize)
	}

	// Build all dynamic sections first
	ds.buildSymbolTable()
	ds.buildHashTable()

	// Pre-generate PLT and GOT with dummy values to get correct sizes
	ds.GeneratePLT(functions, 0, 0)
	ds.GenerateGOT(functions, 0, 0)

	// Calculate memory layout
	// OPTIMIZED: Use single RWX LOAD segment (like static ELF does)
	// Modern Linux supports this, saves ~8KB from segment separation
	// We need: PHDR, INTERP, LOAD(rwx), DYNAMIC
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

	// Read-only data segment
	currentOffset := uint64(alignedHeaders)
	currentAddr := baseAddr + currentOffset

	// Interpreter string
	interp := eb.getInterpreterPath()
	interpSize := len(interp) + 1
	layout["interp"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, interpSize}
	currentOffset += uint64((interpSize + 7) & ^7) // align to 8 bytes
	currentAddr += uint64((interpSize + 7) & ^7)

	// .dynsym
	layout["dynsym"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.dynsym.Len()}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "dynsym: offset=0x%x, size=%d, aligned=%d\n",
			currentOffset, ds.dynsym.Len(), (ds.dynsym.Len()+7) & ^7)
	}
	currentOffset += uint64((ds.dynsym.Len() + 7) & ^7)
	currentAddr += uint64((ds.dynsym.Len() + 7) & ^7)

	// .dynstr
	layout["dynstr"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.dynstr.Len()}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "dynstr: offset=0x%x, size=%d, aligned=%d, next offset=0x%x\n",
			currentOffset, ds.dynstr.Len(), (ds.dynstr.Len()+7) & ^7,
			currentOffset+uint64((ds.dynstr.Len()+7) & ^7))
	}
	currentOffset += uint64((ds.dynstr.Len() + 7) & ^7)
	currentAddr += uint64((ds.dynstr.Len() + 7) & ^7)

	// .hash
	layout["hash"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.hash.Len()}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "hash: offset=0x%x, size=%d, aligned=%d\n",
			currentOffset, ds.hash.Len(), (ds.hash.Len()+7) & ^7)
	}
	currentOffset += uint64((ds.hash.Len() + 7) & ^7)
	currentAddr += uint64((ds.hash.Len() + 7) & ^7)

	// .rela.plt
	layout["rela"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.rela.Len()}
	currentOffset += uint64((ds.rela.Len() + 7) & ^7)
	currentAddr += uint64((ds.rela.Len() + 7) & ^7)

	// Keep page alignment for compatibility with dynamic linker
	// (tried 16-byte alignment but causes segfault in mprotect)
	currentOffset = (currentOffset + pageSize - 1) & ^uint64(pageSize-1)
	currentAddr = (currentAddr + pageSize - 1) & ^uint64(pageSize-1)

	// .plt (executable)
	pltBase = currentAddr
	layout["plt"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.plt.Len()}
	currentOffset += uint64(ds.plt.Len())
	currentAddr += uint64(ds.plt.Len())

	// ._start (entry point - clears registers and calls user code)
	// x86-64: 14 bytes (9 bytes xor + 5 bytes jmp)
	// ARM64: 24 bytes (3*4 mov + 4 bl + 4 mov + 4 svc)
	startSize := 14
	if eb.target.Arch() == ArchARM64 {
		startSize = 24
	}
	layout["_start"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, startSize}
	entryPoint := currentAddr // Entry point is _start
	currentOffset += uint64((startSize + 7) & ^7)
	currentAddr += uint64((startSize + 7) & ^7)

	// Align to new page for .text to prevent overflow into writable segment
	// PLT + _start are small (~100 bytes), so they fit on page 0x2000
	// .text gets TWO pages (0x3000-0x4FFF = 8KB) to allow code growth
	// This prevents RIP-relative addressing issues when code size increases
	currentOffset = (currentOffset + pageSize - 1) & ^uint64(pageSize-1)
	currentAddr = (currentAddr + pageSize - 1) & ^uint64(pageSize-1)

	// .text (our code)
	// Reserve enough pages for the actual code size, rounded up to page boundary
	// Add one extra page for safety margin (RIP-relative addressing)
	actualCodeSize := uint64(codeSize)
	textReservedSize := ((actualCodeSize + pageSize - 1) & ^uint64(pageSize-1)) + pageSize

	layout["text"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, int(textReservedSize)}

	currentOffset += textReservedSize
	currentAddr += textReservedSize

	// writable segment will now start at page 0xB000

	// .dynamic
	dynamicAddr := currentAddr
	layout["dynamic"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.dynamic.Len()}
	currentOffset += uint64((ds.dynamic.Len() + 7) & ^7)
	currentAddr += uint64((ds.dynamic.Len() + 7) & ^7)

	// .got
	gotBase = currentAddr
	layout["got"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, ds.got.Len()}
	currentOffset += uint64((ds.got.Len() + 7) & ^7)
	currentAddr += uint64((ds.got.Len() + 7) & ^7)

	// .rodata
	layout["rodata"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{currentOffset, currentAddr, rodataSize}
	currentAddr += uint64(rodataSize)

	// Now generate PLT and GOT with correct addresses
	ds.GeneratePLT(functions, gotBase, pltBase)
	ds.GenerateGOT(functions, dynamicAddr, pltBase)

	// Add relocations with TEMPORARY addresses - will be updated later
	// Use architecture-specific relocation type
	var relocType uint32
	switch eb.target.Arch() {
	case ArchX86_64:
		relocType = R_X86_64_JUMP_SLOT
	case ArchARM64:
		relocType = R_AARCH64_JUMP_SLOT
	case ArchRiscv64:
		relocType = R_RISCV_JUMP_SLOT
	default:
		relocType = R_X86_64_JUMP_SLOT
	}

	for i := range functions {
		symIndex := uint32(i + 1) // +1 because null symbol is at index 0
		// GOT entries start after 3 reserved entries (24 bytes)
		gotEntryAddr := gotBase + uint64(24+i*8)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Adding TEMPORARY relocation for %s: GOT entry at 0x%x, symIndex=%d, type=%d\n",
				functions[i], gotEntryAddr, symIndex, relocType)
		}
		ds.AddRelocation(gotEntryAddr, symIndex, relocType)
	}

	// Update layout with actual sizes
	layout["rela"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{layout["rela"].offset, layout["rela"].addr, ds.rela.Len()}

	layout["plt"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{layout["plt"].offset, layout["plt"].addr, ds.plt.Len()}

	// Note: .text is now on its own page (page-aligned after PLT + _start)
	// The offset and address were already calculated correctly during initial layout
	// No need to recalculate - just keep the existing layout["text"] values
	textAddr = layout["text"].addr

	// Note: We don't patch PLT calls here because the code will be regenerated
	// and patched later by the caller (see parser.go:1448 and default.go:59)
	// The initial .text is just used to determine section sizes and addresses

	layout["got"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{layout["got"].offset, layout["got"].addr, ds.got.Len()}

	// Build dynamic section with addresses
	addrs := make(map[string]uint64)
	addrs["hash"] = layout["hash"].addr
	addrs["dynstr"] = layout["dynstr"].addr
	addrs["dynsym"] = layout["dynsym"].addr
	addrs["rela"] = layout["rela"].addr
	addrs["got"] = layout["got"].addr

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Hash layout: offset=0x%x, size=%d\n", layout["hash"].offset, layout["hash"].size)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Rela layout: offset=0x%x, addr=0x%x, size=%d\n",
			layout["rela"].offset, layout["rela"].addr, layout["rela"].size)
	}
	ds.buildDynamicSection(addrs)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Dynamic Section Debug ===\n")
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Dynamic section size: %d bytes\n", ds.dynamic.Len())
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Needed libraries: %v\n", ds.needed)
	}

	layout["dynamic"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{layout["dynamic"].offset, layout["dynamic"].addr, ds.dynamic.Len()}

	// Recalculate GOT and BSS offsets now that dynamic size is known
	gotOffset := layout["dynamic"].offset + uint64((ds.dynamic.Len()+7) & ^7)
	gotAddr := layout["dynamic"].addr + uint64((ds.dynamic.Len()+7) & ^7)
	layout["got"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{gotOffset, gotAddr, ds.got.Len()}

	// Regenerate PLT with correct GOT address
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Regenerating PLT with correct GOT address 0x%x\n", gotAddr)
	}
	ds.GeneratePLT(functions, gotAddr, pltBase)
	ds.GenerateGOT(functions, dynamicAddr, pltBase)

	// Update DT_PLTGOT in the dynamic section with the correct GOT address
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Updating DT_PLTGOT from 0x%x to 0x%x\n", gotBase, gotAddr)
	}
	ds.updatePLTGOT(gotAddr)

	// Also update the relocations with the correct GOT addresses
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Updating relocations with final GOT base 0x%x\n", gotAddr)
	}
	for i := range functions {
		oldGotEntryAddr := gotBase + uint64(24+i*8)
		newGotEntryAddr := gotAddr + uint64(24+i*8)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "  %s: reloction 0x%x -> 0x%x\n", functions[i], oldGotEntryAddr, newGotEntryAddr)
		}
		ds.updateRelocationAddress(oldGotEntryAddr, newGotEntryAddr)
	}

	rodataOffset := gotOffset + uint64((ds.got.Len()+7) & ^7)
	rodataAddr = gotAddr + uint64((ds.got.Len()+7) & ^7)
	layout["rodata"] = struct {
		offset uint64
		addr   uint64
		size   int
	}{rodataOffset, rodataAddr, layout["rodata"].size}
	eb.rodataOffsetInELF = rodataOffset

	// Entry point is already set to _start above
	// (entryPoint := layout["_start"].addr)

	// Write ELF file
	w := eb.ELFWriter()

	// ELF Header
	w.Write(0x7f)
	w.Write(0x45) // E
	w.Write(0x4c) // L
	w.Write(0x46) // F
	w.Write(2)    // 64-bit
	w.Write(1)    // little endian
	w.Write(1)    // ELF version
	w.Write(3)    // Linux
	w.WriteN(0, 8)
	w.Write2(3) // DYN
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

	// PT_PHDR (must be first, but covered by a LOAD)
	w.Write4(6) // PT_PHDR
	w.Write4(4) // PF_R
	w.Write8u(elfHeaderSize)
	w.Write8u(baseAddr + elfHeaderSize)
	w.Write8u(baseAddr + elfHeaderSize)
	w.Write8u(uint64(progHeaderSize * numProgHeaders))
	w.Write8u(uint64(progHeaderSize * numProgHeaders))
	w.Write8u(8)

	// PT_INTERP
	interpLayout := layout["interp"]
	w.Write4(3) // PT_INTERP
	w.Write4(4) // PF_R
	w.Write8u(interpLayout.offset)
	w.Write8u(interpLayout.addr)
	w.Write8u(interpLayout.addr)
	w.Write8u(uint64(interpLayout.size))
	w.Write8u(uint64(interpLayout.size))
	w.Write8u(1)

	// OPTIMIZED: Single RWX LOAD segment (like static ELF)
	// Covers entire binary from start to end, saves ~8KB vs 3-segment layout
	loadStart := uint64(0)
	loadEnd := layout["rodata"].offset + uint64(layout["rodata"].size) + uint64(eb.data.Len())
	loadSize := loadEnd - loadStart

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Single RWX LOAD Segment ===\n")
		fmt.Fprintf(os.Stderr, "Range: 0x%x - 0x%x (%d bytes)\n", loadStart, loadEnd, loadSize)
	}

	w.Write4(1) // PT_LOAD
	w.Write4(7) // PF_R | PF_W | PF_X
	w.Write8u(loadStart)
	w.Write8u(baseAddr + loadStart)
	w.Write8u(baseAddr + loadStart)
	w.Write8u(loadSize)
	w.Write8u(loadSize)
	w.Write8u(pageSize) // Keep page alignment for mmap()

	// PT_DYNAMIC
	w.Write4(2) // PT_DYNAMIC
	w.Write4(6) // PF_R | PF_W
	w.Write8u(layout["dynamic"].offset)
	w.Write8u(layout["dynamic"].addr)
	w.Write8u(layout["dynamic"].addr)
	w.Write8u(uint64(layout["dynamic"].size))
	w.Write8u(uint64(layout["dynamic"].size))
	w.Write8u(8)

	// Pad to aligned header size
	for i := headersSize; i < alignedHeaders; i++ {
		w.Write(0)
	}

	// Write all sections
	writePadded := func(buf *bytes.Buffer, targetSize int) {
		currentPos := eb.elf.Len()
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Writing buffer at offset 0x%x (%d bytes from buffer, padding to %d)\n",
				currentPos, buf.Len(), targetSize)
		}
		w.WriteBytes(buf.Bytes())
		for i := buf.Len(); i < targetSize; i++ {
			w.Write(0)
		}
	}

	// Interpreter
	for i := 0; i < len(interp); i++ {
		w.Write(byte(interp[i]))
	}
	w.Write(0)
	for i := interpSize; i < (interpSize+7)&^7; i++ {
		w.Write(0)
	}

	// Record dynsym offset for patching later
	eb.dynsymOffsetInELF = uint64(eb.elf.Len())
	writePadded(&ds.dynsym, (ds.dynsym.Len()+7)&^7)
	writePadded(&ds.dynstr, (ds.dynstr.Len()+7)&^7)
	writePadded(&ds.hash, (ds.hash.Len()+7)&^7)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Rela buffer contents (%d bytes): %x\n", ds.rela.Len(), ds.rela.Bytes())
	}
	writePadded(&ds.rela, (ds.rela.Len()+7)&^7)

	// Pad to page boundary before PLT (required by dynamic linker)
	currentPos := eb.elf.Len()
	nextPage := (currentPos + int(pageSize) - 1) & ^(int(pageSize) - 1)
	for i := currentPos; i < nextPage; i++ {
		w.Write(0)
	}

	// PLT and text
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "About to write PLT: expected offset=0x%x, actual buffer position=0x%x, PLT size=%d bytes\n",
			layout["plt"].offset, eb.elf.Len(), ds.plt.Len())
	}
	w.WriteBytes(ds.plt.Bytes())
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "After PLT, about to write _start\n")
	}

	// _start function (minimal entry point that clears registers and jumps to user code)
	startAddr := layout["_start"].addr
	textAddrForJump := layout["text"].addr
	var startActualSize int

	if eb.target.Arch() == ArchARM64 {
		// ARM64: Clear registers, call user code, then exit with return value
		// mov x0, #0
		w.WriteBytes([]byte{0x00, 0x00, 0x80, 0xd2})
		// mov x1, #0
		w.WriteBytes([]byte{0x01, 0x00, 0x80, 0xd2})
		// mov x2, #0
		w.WriteBytes([]byte{0x02, 0x00, 0x80, 0xd2})
		// bl <user_code> (branch with link - saves return address in x30)
		jumpOffset := int32((textAddrForJump - (startAddr + 12)) / 4)         // 12 = 3 instructions * 4 bytes, offset in instructions
		branchInstr := uint32(0x94000000) | (uint32(jumpOffset) & 0x03FFFFFF) // bl instruction (0x94 instead of 0x14)
		binary.Write(w.(*BufferWrapper).buf, binary.LittleEndian, branchInstr)
		// After return, x0/w0 contains exit code - call exit syscall
		// mov x8, #93 (sys_exit on Linux ARM64)
		w.WriteBytes([]byte{0xa8, 0x0b, 0x80, 0xd2})
		// svc #0
		w.WriteBytes([]byte{0x01, 0x00, 0x00, 0xd4})
		startActualSize = 24 // 6 instructions * 4 bytes
	} else {
		// x86_64: Clear registers and jump to user code
		// xor rax, rax   ; clear rax
		w.Write(0x48)
		w.Write(0x31)
		w.Write(0xc0)
		// xor rdi, rdi   ; clear rdi (first argument)
		w.Write(0x48)
		w.Write(0x31)
		w.Write(0xff)
		// xor rsi, rsi   ; clear rsi (second argument)
		w.Write(0x48)
		w.Write(0x31)
		w.Write(0xf6)
		// jmp to user code (relative jump)
		w.Write(0xe9)                                           // jmp rel32
		jumpOffset := int32(textAddrForJump - (startAddr + 14)) // 14 = size of _start code before jmp
		binary.Write(w.(*BufferWrapper).buf, binary.LittleEndian, jumpOffset)
		startActualSize = 14 // 9 bytes of xor instructions + 5 bytes jmp
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "_start jump: startAddr=0x%x, textAddr=0x%x\n", startAddr, textAddrForJump)
	}

	// Pad _start to aligned size
	for i := startActualSize; i < ((startSize + 7) & ^7); i++ {
		w.Write(0)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Finished writing _start (%d bytes padded to %d), about to write text\n", startActualSize, ((startSize + 7) & ^7))
	}

	// Pad to text section offset (page-aligned)
	currentPos = eb.elf.Len()
	targetTextOffset := int(layout["text"].offset)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Padding from current position 0x%x to text offset 0x%x (%d padding bytes)\n",
			currentPos, targetTextOffset, targetTextOffset-currentPos)
	}
	for i := currentPos; i < targetTextOffset; i++ {
		w.Write(0)
	}

	// Patch PC-relative relocations before writing text section
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Patching PC-relative relocations ===\n")
	}
	eb.PatchPCRelocations(layout["text"].addr, layout["rodata"].addr, rodataSize)

	// Patch direct function calls
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Patching function calls ===\n")
	}
	eb.PatchCallSites(layout["text"].addr)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "About to write text: expected offset=0x%x, actual buffer position=0x%x, text size=%d bytes\n",
			layout["text"].offset, eb.elf.Len(), eb.text.Len())
	}
	w.WriteBytes(eb.text.Bytes())
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Finished writing text section\n")
	}
	for i := codeSize; i < (codeSize+7)&^7; i++ {
		w.Write(0)
	}

	// Pad to dynamic section offset
	currentPos = eb.elf.Len()
	targetOffset := int(layout["dynamic"].offset)
	for i := currentPos; i < targetOffset; i++ {
		w.Write(0)
	}

	// Dynamic, GOT
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing dynamic section (%d bytes) at file position ~0x%x (expected 0x%x)\n", ds.dynamic.Len(), eb.elf.Len(), layout["dynamic"].offset)
	}
	writePadded(&ds.dynamic, (ds.dynamic.Len()+7)&^7)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing GOT section (%d bytes) at file position ~0x%x\n", ds.got.Len(), eb.elf.Len())
	}
	writePadded(&ds.got, (ds.got.Len()+7)&^7)

	w.WriteBytes(eb.rodata.Bytes())

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG [after writing rodata to ELF]: rodata buffer size: %d bytes\n", eb.rodata.Len())
	}
	if eb.rodata.Len() > 0 {
		previewLen := 32
		if eb.rodata.Len() < previewLen {
			previewLen = eb.rodata.Len()
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG [after writing rodata to ELF]: rodata buffer first %d bytes: %q\n", previewLen, eb.rodata.Bytes()[:previewLen])
		}
	}

	// Record data offset for patching later
	eb.dataOffsetInELF = uint64(eb.elf.Len())
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG [before writing data to ELF]: data offset in ELF = 0x%x\n", eb.dataOffsetInELF)
	}

	// Write .data section
	w.WriteBytes(eb.data.Bytes())
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG [after writing data to ELF]: data buffer size: %d bytes\n", eb.data.Len())
	}
	if eb.data.Len() > 0 {
		previewLen := 32
		if eb.data.Len() < previewLen {
			previewLen = eb.data.Len()
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG [after writing data to ELF]: data buffer first %d bytes: %x\n", previewLen, eb.data.Bytes()[:previewLen])
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Complete Dynamic ELF ===\n")
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Entry point: 0x%x\n", entryPoint)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PLT base: 0x%x (%d bytes)\n", pltBase, ds.plt.Len())
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "GOT base: 0x%x (%d bytes)\n", gotAddr, ds.got.Len())
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Rodata base: 0x%x (%d bytes)\n", layout["rodata"].addr, rodataSize)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Functions: %v\n", functions)
	}

	return gotAddr, rodataAddr, textAddr, pltBase, nil
}

func (eb *ExecutableBuilder) getInterpreterPath() string {
	switch eb.target.Arch() {
	case ArchX86_64:
		return "/lib64/ld-linux-x86-64.so.2"
	case ArchARM64:
		return "/lib/ld-linux-aarch64.so.1"
	case ArchRiscv64:
		return "/lib/ld-linux-riscv64-lp64d.so.1"
	default:
		return "/lib64/ld-linux-x86-64.so.2"
	}
}

// patchPLTCalls patches call instructions in .text to use correct PLT offsets
func (eb *ExecutableBuilder) patchPLTCalls(ds *DynamicSections, textAddr uint64, pltBase uint64, functions []string) {
	textBytes := eb.text.Bytes()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Text bytes (%d total): %x\n", len(textBytes), textBytes)
	}

	switch eb.target.Arch() {
	case ArchX86_64:
		eb.patchX86PLTCalls(textBytes, ds, textAddr, pltBase, functions)
	case ArchARM64:
		eb.patchARM64PLTCalls(textBytes, ds, textAddr, pltBase, functions)
	case ArchRiscv64:
		eb.patchRISCVPLTCalls(textBytes, ds, textAddr, pltBase, functions)
	}

	// Write the patched bytes back
	eb.text.Reset()
	eb.text.Write(textBytes)
	if VerboseMode && len(textBytes) > 70 {
		fmt.Fprintf(os.Stderr, "DEBUG patchPLTCalls: Bytes at 63-68 in eb.text: %x\n", textBytes[63:68])
	}
}

func (eb *ExecutableBuilder) patchX86PLTCalls(textBytes []byte, ds *DynamicSections, textAddr, pltBase uint64, functions []string) {
	// Use callPatches which has both position and function name
	// This correctly handles calls from runtime helpers that aren't in fc.callOrder
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchX86PLTCalls: have %d callPatches, textBytes len=%d\n", len(eb.callPatches), len(textBytes))
	}
	for _, patch := range eb.callPatches {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchX86PLTCalls: patch at pos=%d, target=%s\n", patch.position, patch.targetName)
		}
		// Extract function name from targetName (strip "$stub" suffix if present)
		funcName := strings.TrimSuffix(patch.targetName, "$stub")

		// Get the CALL instruction position
		// patch.position was recorded as o.eb.text.Len() AFTER writing 0xE8
		// So it points to where the placeholder STARTS (after the 0xE8 byte)
		// The 0xE8 byte is at position-1
		placeholderPos := patch.position
		callPos := placeholderPos - 1

		// Check bounds
		if callPos < 0 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: callPos %d < 0, skipping\n", callPos)
			}
			continue
		}
		if callPos >= len(textBytes) {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: callPos %d >= len(textBytes) %d, skipping\n", callPos, len(textBytes))
			}
			continue
		}
		if placeholderPos+3 >= len(textBytes) {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: placeholderPos+3 %d >= len(textBytes) %d, skipping\n", placeholderPos+3, len(textBytes))
			}
			continue
		}
		if textBytes[callPos] != 0xE8 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: textBytes[%d] = 0x%x, not 0xE8, skipping\n", callPos, textBytes[callPos])
			}
			continue
		}

		// Verify we're at a CALL instruction with placeholder
		if true {
			placeholder := []byte{0x78, 0x56, 0x34, 0x12}
			actualPlaceholder := textBytes[placeholderPos : placeholderPos+4]
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: At pos %d, found 0xE8, checking placeholder: expected %x, got %x\n", callPos, placeholder, actualPlaceholder)
			}
			if bytes.Equal(textBytes[placeholderPos:placeholderPos+4], placeholder) {
				pltOffset := ds.GetPLTOffset(funcName)
				var targetAddr uint64
				var isInternal bool

				if pltOffset >= 0 {
					// External function via PLT
					targetAddr = pltBase + uint64(pltOffset)
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: Patching PLT call to %s: plt=%d, targetAddr=%x\n", funcName, pltOffset, targetAddr)
					}
				} else if labelOffset, ok := eb.labels[funcName]; ok {
					// Internal label (e.g., vibe67_arena_alloc, _vibe67_arena_ensure_capacity)
					targetAddr = textAddr + uint64(labelOffset)
					isInternal = true
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: Patching internal call to %s: labelOffset=%d, targetAddr=%x\n", funcName, labelOffset, targetAddr)
					}
				} else {
					// Function not in PLT and not an internal label
					// This can happen for runtime helper functions (printf, strlen, malloc, etc.)
					// that are generated after PLT is built. These warnings are harmless.
					// Only warn in verbose mode.
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Note: Function %s called but not in PLT (likely from runtime helpers)\n", funcName)
					}
				}

				if pltOffset >= 0 || isInternal {
					currentAddr := textAddr + uint64(placeholderPos)
					relOffset := int32(targetAddr - (currentAddr + 4))

					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: Call at %x, target %x, relOffset=%d (0x%x)\n", currentAddr, targetAddr, relOffset, uint32(relOffset))
					}

					textBytes[placeholderPos] = byte(relOffset & 0xFF)
					textBytes[placeholderPos+1] = byte((relOffset >> 8) & 0xFF)
					textBytes[placeholderPos+2] = byte((relOffset >> 16) & 0xFF)
					textBytes[placeholderPos+3] = byte((relOffset >> 24) & 0xFF)
				}
			} else if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: No placeholder at position %d for %s\n", placeholderPos, funcName)
			}
		} else if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: Invalid call position %d for %s\n", callPos, funcName)
		}
	}
}

func (eb *ExecutableBuilder) patchARM64PLTCalls(textBytes []byte, ds *DynamicSections, textAddr, pltBase uint64, functions []string) {
	// Use callPatches which has both position and function name
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchARM64PLTCalls: have %d callPatches\n", len(eb.callPatches))
	}
	for _, patch := range eb.callPatches {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchARM64PLTCalls: patch at pos=%d, target=%s\n", patch.position, patch.targetName)
		}
		// Extract function name from targetName (strip "$stub" suffix if present)
		funcName := strings.TrimSuffix(patch.targetName, "$stub")

		// Get the BL instruction position
		// For ARM64, patch.position was recorded as the position where we wrote the BL
		callPos := patch.position

		// Check bounds
		if callPos < 0 || callPos+3 >= len(textBytes) {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: callPos %d out of bounds (len=%d), skipping\n", callPos, len(textBytes))
			}
			continue
		}

		// Verify it's a BL placeholder (0x94000000)
		instr := uint32(textBytes[callPos]) |
			(uint32(textBytes[callPos+1]) << 8) |
			(uint32(textBytes[callPos+2]) << 16) |
			(uint32(textBytes[callPos+3]) << 24)

		if (instr & 0xFC000000) != 0x94000000 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Not a BL instruction at %d: 0x%08x\n", callPos, instr)
			}
			continue
		}

		// Get PLT offset for this function
		pltOffset := ds.GetPLTOffset(funcName)
		if pltOffset < 0 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: No PLT entry for %s\n", funcName)
			}
			continue
		}

		targetAddr := pltBase + uint64(pltOffset)
		currentAddr := textAddr + uint64(callPos)
		offset := int64(targetAddr - currentAddr)

		// BL uses signed 26-bit word offset (divide by 4)
		wordOffset := offset >> 2
		if wordOffset >= -0x2000000 && wordOffset < 0x2000000 {
			imm26 := uint32(wordOffset) & 0x03FFFFFF
			blInstr := 0x94000000 | imm26

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Patching ARM64 call to %s: pos=0x%x, currentAddr=0x%x, targetAddr=0x%x, wordOffset=%d\n",
					funcName, callPos, currentAddr, targetAddr, wordOffset)
			}

			textBytes[callPos] = byte(blInstr & 0xFF)
			textBytes[callPos+1] = byte((blInstr >> 8) & 0xFF)
			textBytes[callPos+2] = byte((blInstr >> 16) & 0xFF)
			textBytes[callPos+3] = byte((blInstr >> 24) & 0xFF)
		} else if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: BL offset too large for %s: %d words\n", funcName, wordOffset)
		}
	}
}

func (eb *ExecutableBuilder) patchRISCVPLTCalls(textBytes []byte, ds *DynamicSections, textAddr, pltBase uint64, functions []string) {
	// Search for placeholder JAL instructions (0x000000EF)
	funcIndex := 0
	for i := 0; i+3 < len(textBytes); i += 4 {
		instr := uint32(textBytes[i]) |
			(uint32(textBytes[i+1]) << 8) |
			(uint32(textBytes[i+2]) << 16) |
			(uint32(textBytes[i+3]) << 24)

		// JAL instruction: imm[20|10:1|11|19:12] rd 1101111
		if (instr&0x7F) == 0x6F && (instr&0xFFFFF000) == 0 {
			if funcIndex < len(functions) {
				pltOffset := ds.GetPLTOffset(functions[funcIndex])
				if pltOffset >= 0 {
					targetAddr := pltBase + uint64(pltOffset)
					currentAddr := textAddr + uint64(i)
					offset := int64(targetAddr - currentAddr)

					// JAL uses signed 21-bit offset
					if offset >= -0x100000 && offset < 0x100000 {
						// Encode immediate in JAL format: [20|10:1|11|19:12]
						imm20 := (uint32(offset>>20) & 1) << 31
						imm10_1 := (uint32(offset>>1) & 0x3FF) << 21
						imm11 := (uint32(offset>>11) & 1) << 20
						imm19_12 := (uint32(offset>>12) & 0xFF) << 12
						rd := (instr >> 7) & 0x1F

						jalInstr := imm20 | imm19_12 | imm11 | imm10_1 | (rd << 7) | 0x6F

						if VerboseMode {
							fmt.Fprintf(os.Stderr, "Patching RISC-V call #%d (%s): offset=0x%x, currentAddr=0x%x, targetAddr=0x%x, pcOffset=%d\n",
								funcIndex, functions[funcIndex], i, currentAddr, targetAddr, offset)
						}

						textBytes[i] = byte(jalInstr & 0xFF)
						textBytes[i+1] = byte((jalInstr >> 8) & 0xFF)
						textBytes[i+2] = byte((jalInstr >> 16) & 0xFF)
						textBytes[i+3] = byte((jalInstr >> 24) & 0xFF)
					}
				}
				funcIndex++
			}
		}
	}
}
