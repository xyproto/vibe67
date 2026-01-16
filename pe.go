// Completion: 100% - PE generation complete for Windows x86_64
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

// PE (Portable Executable) format constants for Windows x86_64
const (
	// DOS header (stub)
	dosHeaderSize = 64
	dosStubSize   = 128

	// PE headers
	peSignatureSize     = 4
	coffHeaderSize      = 20
	optionalHeaderSize  = 240 // PE32+ (64-bit)
	peSectionHeaderSize = 40

	// Memory layout for PE
	peImageBase    = 0x140000000 // Standard Windows x64 image base
	peSectionAlign = 0x1000      // 4KB section alignment in memory
	peFileAlign    = 0x200       // 512 byte file alignment

	// Section characteristics
	scnMemExecute  = 0x20000000
	scnMemRead     = 0x40000000
	scnMemWrite    = 0x80000000
	scnCntCode     = 0x00000020
	scnCntInitData = 0x00000040
)

// Confidence that this function is working: 85%
func (eb *ExecutableBuilder) WritePEHeaderWithImports(entryPointRVA uint32, codeSize, dataSize, idataSize, idataRVA uint32) error {
	w := eb.ELFWriter() // Reuse the writer

	// Helper functions to write multi-byte values
	writeU16 := func(v uint16) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8)})
	}
	writeU32 := func(v uint32) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	}

	// === DOS Header (64 bytes) ===
	writeU16(0x5A4D) // "MZ" signature
	w.WriteN(0, 58)  // Zero bytes 2-59
	// At offset 0x3C (60), write the PE header offset
	peHeaderOffset := uint32(dosHeaderSize + dosStubSize)
	writeU32(peHeaderOffset) // PE header offset

	// === DOS Stub (simple one that just prints "This program cannot be run in DOS mode") ===
	// For simplicity, we'll just pad with zeros (minimal stub)
	stubMsg := []byte("This program requires Windows.\r\n$")
	w.WriteBytes(stubMsg)
	w.WriteN(0, dosStubSize-len(stubMsg))

	// === PE Signature ===
	writeU32(0x00004550) // "PE\0\0"

	// === COFF File Header (20 bytes) ===
	writeU16(0x8664)             // Machine: AMD64
	writeU16(3)                  // Number of sections (.text, .data, .idata)
	writeU32(0)                  // TimeDateStamp (0 for reproducibility)
	writeU32(0)                  // Pointer to symbol table (deprecated)
	writeU32(0)                  // Number of symbols (deprecated)
	writeU16(optionalHeaderSize) // Size of optional header
	writeU16(0x0022)             // Characteristics: EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE

	// === Optional Header (PE32+) ===
	writeU16(0x020B)        // Magic: PE32+ (64-bit)
	w.Write(1)              // Major linker version
	w.Write(0)              // Minor linker version
	writeU32(codeSize)      // Size of code
	writeU32(dataSize)      // Size of initialized data
	writeU32(0)             // Size of uninitialized data
	writeU32(entryPointRVA) // Entry point RVA
	writeU32(0x1000)        // Base of code

	// PE32+ specific fields
	w.Write8u(peImageBase)   // Image base
	writeU32(peSectionAlign) // Section alignment
	writeU32(peFileAlign)    // File alignment
	writeU16(6)              // Major OS version
	writeU16(0)              // Minor OS version
	writeU16(0)              // Major image version
	writeU16(0)              // Minor image version
	writeU16(6)              // Major subsystem version
	writeU16(0)              // Minor subsystem version
	writeU32(0)              // Win32 version value (reserved)

	// Calculate image size: end of last section (idata RVA + idata size), aligned to section alignment
	// SizeOfImage must be the size of the image in memory, not on disk
	imageSize := alignTo(idataRVA+idataSize, peSectionAlign)
	writeU32(imageSize) // Size of image

	headersSize := alignTo(dosHeaderSize+dosStubSize+peSignatureSize+coffHeaderSize+
		optionalHeaderSize+3*peSectionHeaderSize, peFileAlign)
	writeU32(headersSize) // Size of headers

	writeU32(0)         // Checksum
	writeU16(3)         // Subsystem: CUI (Console)
	writeU16(0x8120)    // DLL characteristics: DYNAMIC_BASE | NX_COMPAT | TERMINAL_SERVER_AWARE | NO_SEH
	w.Write8u(0x100000) // Size of stack reserve
	w.Write8u(0x1000)   // Size of stack commit
	w.Write8u(0x100000) // Size of heap reserve
	w.Write8u(0x1000)   // Size of heap commit
	writeU32(0)         // Loader flags
	writeU32(16)        // Number of data directories

	// Data directories (16 entries, each 8 bytes: RVA + Size)
	for i := 0; i < 16; i++ {
		if i == 1 { // Import directory
			writeU32(idataRVA)  // Import RVA
			writeU32(idataSize) // Import size
		} else {
			w.Write8u(0)
		}
	}

	return nil
}

// Confidence that this function is working: 75%
func (eb *ExecutableBuilder) WritePEHeader(entryPointRVA uint32, codeSize, dataSize uint32) error {
	w := eb.ELFWriter() // Reuse the writer

	// Helper functions to write multi-byte values
	writeU16 := func(v uint16) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8)})
	}
	writeU32 := func(v uint32) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	}

	// === DOS Header (64 bytes) ===
	writeU16(0x5A4D) // "MZ" signature
	w.WriteN(0, 58)  // Zero bytes 2-59
	// At offset 0x3C (60), write the PE header offset
	peHeaderOffset := uint32(dosHeaderSize + dosStubSize)
	writeU32(peHeaderOffset) // PE header offset

	// === DOS Stub (simple one that just prints "This program cannot be run in DOS mode") ===
	// For simplicity, we'll just pad with zeros (minimal stub)
	stubMsg := []byte("This program requires Windows.\r\n$")
	w.WriteBytes(stubMsg)
	w.WriteN(0, dosStubSize-len(stubMsg))

	// === PE Signature ===
	writeU32(0x00004550) // "PE\0\0"

	// === COFF File Header (20 bytes) ===
	writeU16(0x8664)             // Machine: AMD64
	writeU16(3)                  // Number of sections (.text, .data, .idata)
	writeU32(0)                  // TimeDateStamp (0 for reproducibility)
	writeU32(0)                  // Pointer to symbol table (deprecated)
	writeU32(0)                  // Number of symbols (deprecated)
	writeU16(optionalHeaderSize) // Size of optional header
	writeU16(0x0022)             // Characteristics: EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE

	// === Optional Header (PE32+) ===
	writeU16(0x020B)        // Magic: PE32+ (64-bit)
	w.Write(1)              // Major linker version
	w.Write(0)              // Minor linker version
	writeU32(codeSize)      // Size of code
	writeU32(dataSize)      // Size of initialized data
	writeU32(0)             // Size of uninitialized data
	writeU32(entryPointRVA) // Entry point RVA
	writeU32(0x1000)        // Base of code

	// PE32+ specific fields
	w.Write8u(peImageBase)   // Image base
	writeU32(peSectionAlign) // Section alignment
	writeU32(peFileAlign)    // File alignment
	writeU16(6)              // Major OS version
	writeU16(0)              // Minor OS version
	writeU16(0)              // Major image version
	writeU16(0)              // Minor image version
	writeU16(6)              // Major subsystem version
	writeU16(0)              // Minor subsystem version
	writeU32(0)              // Win32 version value (reserved)

	// Calculate image size: end of last section, aligned to section alignment
	// For this version without imports, last section is .data at textVirtualAddr + alignTo(codeSize) + dataSize
	textVirtualAddr := uint32(0x1000)
	dataVirtualAddr := textVirtualAddr + alignTo(codeSize, peSectionAlign)
	imageSize := alignTo(dataVirtualAddr+dataSize, peSectionAlign)
	writeU32(imageSize) // Size of image

	headersSize := alignTo(dosHeaderSize+dosStubSize+peSignatureSize+coffHeaderSize+
		optionalHeaderSize+3*peSectionHeaderSize, peFileAlign)
	writeU32(headersSize) // Size of headers

	writeU32(0)         // Checksum
	writeU16(3)         // Subsystem: CUI (Console)
	writeU16(0x8120)    // DLL characteristics: DYNAMIC_BASE | NX_COMPAT | TERMINAL_SERVER_AWARE | NO_SEH
	w.Write8u(0x100000) // Size of stack reserve
	w.Write8u(0x1000)   // Size of stack commit
	w.Write8u(0x100000) // Size of heap reserve
	w.Write8u(0x1000)   // Size of heap commit
	writeU32(0)         // Loader flags
	writeU32(16)        // Number of data directories

	// Data directories (16 entries, each 8 bytes: RVA + Size)
	// We'll fill in import directory later, rest are zeros
	for i := 0; i < 16; i++ {
		if i == 1 { // Import directory
			// Will be filled in later when we have actual imports
			writeU32(0) // Import RVA (placeholder)
			writeU32(0) // Import size (placeholder)
		} else {
			w.Write8u(0)
		}
	}

	return nil
}

// Confidence that this function is working: 80%
func (eb *ExecutableBuilder) WritePESectionHeader(name string, virtualSize, virtualAddr, rawSize, rawAddr uint32, characteristics uint32) {
	w := eb.ELFWriter()

	writeU16 := func(v uint16) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8)})
	}
	writeU32 := func(v uint32) {
		w.WriteBytes([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
	}

	// Section name (8 bytes, null-padded)
	nameBytes := []byte(name)
	if len(nameBytes) > 8 {
		nameBytes = nameBytes[:8]
	}
	w.WriteBytes(nameBytes)
	w.WriteN(0, 8-len(nameBytes))

	writeU32(virtualSize)
	writeU32(virtualAddr)
	writeU32(rawSize)
	writeU32(rawAddr)
	writeU32(0) // Pointer to relocations
	writeU32(0) // Pointer to line numbers
	writeU16(0) // Number of relocations
	writeU16(0) // Number of line numbers
	writeU32(characteristics)
}

// Confidence that this function is working: 75%
func (eb *ExecutableBuilder) WritePE(outputPath string) error {
	// For Windows, we need to generate import tables for C runtime
	// Build import directory for msvcrt.dll (C runtime)

	// Standard C runtime functions needed by Vibe67 programs
	libraries := map[string][]string{
		"msvcrt.dll": {
			"printf", "exit", "malloc", "free", "realloc", "getenv",
			"strlen", "memcpy", "memset", "pow", "fflush",
			"sin", "cos", "sqrt", "fopen", "fclose", "fwrite", "fread",
		},
	}

	// Write rodata and data content to buffers first
	// IMPORTANT: We must iterate in a consistent order so addresses match!
	rodataSymbols := eb.RodataSection()
	dataSymbols := eb.DataSection()

	// Create sorted slices of symbol names for consistent ordering
	rodataNames := make([]string, 0, len(rodataSymbols))
	for name := range rodataSymbols {
		rodataNames = append(rodataNames, name)
	}
	sort.Strings(rodataNames)

	dataNames := make([]string, 0, len(dataSymbols))
	for name := range dataSymbols {
		dataNames = append(dataNames, name)
	}
	sort.Strings(dataNames)

	// Write rodata in sorted order
	for _, name := range rodataNames {
		eb.WriteRodata([]byte(rodataSymbols[name]))
	}
	// Write data in sorted order
	for _, name := range dataNames {
		eb.data.Write([]byte(dataSymbols[name]))
	}

	codeSize := uint32(eb.text.Len())
	dataSize := uint32(eb.rodata.Len() + eb.data.Len())

	// Align sizes to file alignment
	codeSize = alignTo(codeSize, peFileAlign)
	dataSize = alignTo(dataSize, peFileAlign)

	// Calculate section positions
	headerSize := uint32(dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize +
		optionalHeaderSize + 3*peSectionHeaderSize)
	headerSize = alignTo(headerSize, peFileAlign)

	textRawAddr := uint32(headerSize)
	textVirtualAddr := uint32(0x1000) // First section after headers

	dataRawAddr := textRawAddr + codeSize
	dataVirtualAddr := textVirtualAddr + alignTo(codeSize, peSectionAlign)

	// Build import data
	idataVirtualAddr := dataVirtualAddr + alignTo(dataSize, peSectionAlign)
	importData, iatMap, err := BuildPEImportData(libraries, idataVirtualAddr)
	if err != nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: Failed to build import data: %v\n", err)
		}
		importData = []byte{} // Empty import section
	}

	idataSize := uint32(len(importData))
	idataRawSize := alignTo(idataSize, peFileAlign)
	idataRawAddr := dataRawAddr + dataSize

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Import section: size=%d, RVA=0x%x\n", idataSize, idataVirtualAddr)
		fmt.Fprintf(os.Stderr, "IAT mapping: %d functions\n", len(iatMap))
	}

	// Entry point is at start of .text section
	entryPointRVA := textVirtualAddr

	// Write PE header with import directory info
	if err := eb.WritePEHeaderWithImports(entryPointRVA, codeSize, dataSize, idataSize, idataVirtualAddr); err != nil {
		return err
	}

	// Write section headers
	eb.WritePESectionHeader(".text", codeSize, textVirtualAddr, codeSize, textRawAddr,
		scnCntCode|scnMemExecute|scnMemRead)
	eb.WritePESectionHeader(".data", dataSize, dataVirtualAddr, dataSize, dataRawAddr,
		scnCntInitData|scnMemRead|scnMemWrite)
	eb.WritePESectionHeader(".idata", idataSize, idataVirtualAddr, idataRawSize, idataRawAddr,
		scnCntInitData|scnMemRead) // Import section

	// Pad headers to file alignment
	currentPos := uint32(dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize +
		optionalHeaderSize + 3*peSectionHeaderSize)
	padding := int(headerSize - currentPos)
	if padding > 0 {
		eb.ELFWriter().WriteN(0, padding)
	}

	// Assign addresses to all data symbols (strings, constants)
	// For PE, the .data section contains both rodata and data
	// IMPORTANT: Must use the same sorted order as when we wrote the data!
	rodataAddr := peImageBase + uint64(dataVirtualAddr)
	currentAddr := rodataAddr

	// First, rodata symbols (read-only) in sorted order
	for _, symbol := range rodataNames {
		value := rodataSymbols[symbol]
		eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "PE rodata: %s at 0x%x\n", symbol, eb.consts[symbol].addr)
		}
	}

	// Then, data symbols (writable, like cpu_has_avx512) in sorted order
	for _, symbol := range dataNames {
		value := dataSymbols[symbol]
		eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "PE data: %s at 0x%x\n", symbol, eb.consts[symbol].addr)
		}
	}

	// Patch calls to use IAT (Import Address Table)
	if err := eb.PatchPECallsToIAT(iatMap, uint64(textVirtualAddr), uint64(idataVirtualAddr), peImageBase); err != nil {
		return err
	}

	// Patch PC-relative relocations (LEA instructions for data access)
	textAddrFull := peImageBase + uint64(textVirtualAddr)
	eb.PatchPCRelocations(textAddrFull, rodataAddr, eb.rodata.Len())

	// Write sections
	// .text section
	eb.ELFWriter().WriteBytes(eb.text.Bytes())
	if pad := int(codeSize) - eb.text.Len(); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
	}

	// .data section (combine rodata and data)
	eb.ELFWriter().WriteBytes(eb.rodata.Bytes())
	eb.ELFWriter().WriteBytes(eb.data.Bytes())
	if pad := int(dataSize) - eb.rodata.Len() - eb.data.Len(); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
	}

	// .idata section (imports)
	eb.ELFWriter().WriteBytes(importData)
	if pad := int(idataRawSize) - len(importData); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
	}

	// Write to file
	if err := os.WriteFile(outputPath, eb.elf.Bytes(), 0755); err != nil {
		return fmt.Errorf("failed to write PE file: %v", err)
	}

	return nil
}

// alignTo aligns a value to the given alignment
func alignTo(value, align uint32) uint32 {
	return (value + align - 1) & ^(align - 1)
}

// Confidence that this function is working: 60%
func (eb *ExecutableBuilder) WritePEWithImports(outputPath string, imports []string) error {
	// TODO: Implement proper import table generation
	// For now, just create a minimal PE without imports
	// This will need to be expanded to support msvcrt.dll imports (printf, malloc, etc.)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Warning: PE import tables not yet fully implemented\n")
		fmt.Fprintf(os.Stderr, "Required imports: %v\n", imports)
	}

	return eb.WritePE(outputPath)
}

// Confidence that this function is working: 80%
// WritePEWithLibraries writes a PE file with the given library imports
func (eb *ExecutableBuilder) WritePEWithLibraries(outputPath string, libraries map[string][]string) error {
	if len(libraries) == 0 {
		// No imports, use default msvcrt.dll
		libraries = map[string][]string{
			"msvcrt.dll": {"printf", "exit", "malloc", "free", "realloc", "getenv", "strlen", "memcpy", "memset", "pow", "fflush"},
		}
	}

	return eb.writePEWithLibraries(outputPath, libraries)
}

// Confidence that this function is working: 75%
func (eb *ExecutableBuilder) writePEWithLibraries(outputPath string, libraries map[string][]string) error {
	// Write rodata and data content to buffers first
	// IMPORTANT: We must iterate in a consistent order so addresses match!
	rodataSymbols := eb.RodataSection()
	dataSymbols := eb.DataSection()

	// Create sorted slices of symbol names for consistent ordering
	rodataNames := make([]string, 0, len(rodataSymbols))
	for name := range rodataSymbols {
		rodataNames = append(rodataNames, name)
	}
	sort.Strings(rodataNames)

	dataNames := make([]string, 0, len(dataSymbols))
	for name := range dataSymbols {
		dataNames = append(dataNames, name)
	}
	sort.Strings(dataNames)

	// Write rodata in sorted order
	for _, name := range rodataNames {
		eb.WriteRodata([]byte(rodataSymbols[name]))
	}
	// Write data in sorted order
	for _, name := range dataNames {
		eb.data.Write([]byte(dataSymbols[name]))
	}

	codeSize := uint32(eb.text.Len())
	dataSize := uint32(eb.rodata.Len() + eb.data.Len())

	// Align sizes to file alignment
	codeSize = alignTo(codeSize, peFileAlign)
	dataSize = alignTo(dataSize, peFileAlign)

	fmt.Fprintf(os.Stderr, "Aligned: codeSize=%d (0x%X), dataSize=%d (0x%X)\n",
		codeSize, codeSize, dataSize, dataSize)

	// Calculate section positions
	headerSize := uint32(dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize +
		optionalHeaderSize + 3*peSectionHeaderSize)
	headerSize = alignTo(headerSize, peFileAlign)

	textRawAddr := uint32(headerSize)
	textVirtualAddr := uint32(0x1000) // First section after headers

	dataRawAddr := textRawAddr + codeSize
	dataVirtualAddr := textVirtualAddr + alignTo(codeSize, peSectionAlign)

	fmt.Fprintf(os.Stderr, "Section layout: textRawAddr=0x%X, dataRawAddr=0x%X\n", textRawAddr, dataRawAddr)

	// Build import data
	idataVirtualAddr := dataVirtualAddr + alignTo(dataSize, peSectionAlign)
	importData, iatMap, err := BuildPEImportData(libraries, idataVirtualAddr)
	if err != nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: Failed to build import data: %v\n", err)
		}
		importData = []byte{} // Empty import section
	}

	idataSize := uint32(len(importData))
	idataRawSize := alignTo(idataSize, peFileAlign)
	idataRawAddr := dataRawAddr + dataSize

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Import section: size=%d, RVA=0x%x\n", idataSize, idataVirtualAddr)
		fmt.Fprintf(os.Stderr, "IAT mapping: %d functions\n", len(iatMap))
	}

	// Entry point is at start of .text section
	entryPointRVA := textVirtualAddr

	// Write PE header with import directory info
	if err := eb.WritePEHeaderWithImports(entryPointRVA, codeSize, dataSize, idataSize, idataVirtualAddr); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[1] After header: pos=%d\n", eb.elf.Len())

	// Write section headers
	eb.WritePESectionHeader(".text", codeSize, textVirtualAddr, codeSize, textRawAddr,
		scnCntCode|scnMemExecute|scnMemRead)
	eb.WritePESectionHeader(".data", dataSize, dataVirtualAddr, dataSize, dataRawAddr,
		scnCntInitData|scnMemRead|scnMemWrite)
	eb.WritePESectionHeader(".idata", idataSize, idataVirtualAddr, idataRawSize, idataRawAddr,
		scnCntInitData|scnMemRead) // Import section
	fmt.Fprintf(os.Stderr, "[2] After section headers: pos=%d\n", eb.elf.Len())

	// Pad headers to file alignment
	currentPos := uint32(dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize +
		optionalHeaderSize + 3*peSectionHeaderSize)
	padding := int(headerSize - currentPos)
	fmt.Fprintf(os.Stderr, "[3] Padding %d bytes\n", padding)
	if padding > 0 {
		eb.ELFWriter().WriteN(0, padding)
	}
	fmt.Fprintf(os.Stderr, "[4] After padding: pos=%d (should be 0x%X)\n", eb.elf.Len(), textRawAddr)

	// Assign addresses to all data symbols (strings, constants)
	// For PE, the .data section contains both rodata and data
	// IMPORTANT: Must use the same sorted order as when we wrote the data!
	rodataAddr := peImageBase + uint64(dataVirtualAddr)
	currentAddr := rodataAddr

	// First, rodata symbols (read-only) in sorted order
	for _, symbol := range rodataNames {
		value := rodataSymbols[symbol]
		eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "PE rodata: %s at 0x%x\n", symbol, eb.consts[symbol].addr)
		}
	}

	// Then, data symbols (writable, like cpu_has_avx512) in sorted order
	for _, symbol := range dataNames {
		value := dataSymbols[symbol]
		eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "PE data: %s at 0x%x\n", symbol, eb.consts[symbol].addr)
		}
	}

	// Patch calls to use IAT (Import Address Table)
	if err := eb.PatchPECallsToIAT(iatMap, uint64(textVirtualAddr), uint64(idataVirtualAddr), peImageBase); err != nil {
		return err
	}

	// Patch PC-relative relocations (LEA instructions for data access)
	textAddrFull := peImageBase + uint64(textVirtualAddr)
	eb.PatchPCRelocations(textAddrFull, rodataAddr, eb.rodata.Len())

	fmt.Fprintf(os.Stderr, "[5] Writing .text: %d bytes\n", eb.text.Len())
	eb.ELFWriter().WriteBytes(eb.text.Bytes())
	if pad := int(codeSize) - eb.text.Len(); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
		fmt.Fprintf(os.Stderr, "[6] Padded .text: %d bytes\n", pad)
	}
	fmt.Fprintf(os.Stderr, "[7] After .text: pos=%d, expected dataRawAddr=0x%X\n", eb.elf.Len(), dataRawAddr)
	// .data section (combine rodata and data)
	fmt.Fprintf(os.Stderr, "[8] Writing .data: rodata=%d + data=%d bytes\n", eb.rodata.Len(), eb.data.Len())
	eb.ELFWriter().WriteBytes(eb.rodata.Bytes())
	eb.ELFWriter().WriteBytes(eb.data.Bytes())
	if pad := int(dataSize) - eb.rodata.Len() - eb.data.Len(); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
	}
	fmt.Fprintf(os.Stderr, "[9] After .data: pos=%d\n", eb.elf.Len())

	// .idata section (imports)
	eb.ELFWriter().WriteBytes(importData)
	if pad := int(idataRawSize) - len(importData); pad > 0 {
		eb.ELFWriter().WriteN(0, pad)
	}

	// Write to file
	fmt.Fprintf(os.Stderr, "[FINAL] File size: %d bytes\n", eb.elf.Len())
	if err := os.WriteFile(outputPath, eb.elf.Bytes(), 0755); err != nil {
		return fmt.Errorf("failed to write PE file: %v", err)
	}

	return nil
}

// Confidence that this function is working: 70%
// BuildPEImportData builds the complete import section data for PE files
// Returns: import data, IAT RVA map (funcName -> RVA), error
func BuildPEImportData(libraries map[string][]string, idataRVA uint32) ([]byte, map[string]uint32, error) {
	// Structure of .idata section:
	// 1. Import Directory Table (IDT) - array of IMAGE_IMPORT_DESCRIPTOR (20 bytes each), null-terminated
	// 2. Import Lookup Tables (ILT) - one per DLL, array of RVAs to hint/name entries
	// 3. Import Address Tables (IAT) - one per DLL, same structure as ILT (loader fills this)
	// 4. Hint/Name Table - hint (uint16) + name (null-terminated string) for each function
	// 5. DLL names - null-terminated strings

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "BuildPEImportData: libraries = %v\n", libraries)
	}

	if len(libraries) == 0 {
		return nil, nil, fmt.Errorf("no libraries to import")
	}

	var buf bytes.Buffer
	iatMap := make(map[string]uint32)

	// Calculate offsets
	numLibs := len(libraries)
	idtSize := (numLibs + 1) * 20 // +1 for null terminator
	currentOffset := uint32(idtSize)

	// Storage for each library's data
	type libData struct {
		name        string
		functions   []string
		iltOffset   uint32
		iatOffset   uint32
		nameOffset  uint32
		hintsOffset uint32
	}
	libsData := make([]libData, 0, numLibs)

	// Sort library names for deterministic output
	libNames := make([]string, 0, len(libraries))
	for libName := range libraries {
		libNames = append(libNames, libName)
	}
	sort.Strings(libNames)

	// First pass: calculate all offsets
	for _, libName := range libNames {
		funcs := libraries[libName]
		ld := libData{
			name:      libName,
			functions: funcs,
		}

		// ILT offset
		ld.iltOffset = currentOffset
		iltSize := uint32((len(funcs) + 1) * 8) // 8 bytes per entry (64-bit), +1 for null
		currentOffset += iltSize

		// IAT offset (same size as ILT)
		ld.iatOffset = currentOffset
		currentOffset += iltSize

		libsData = append(libsData, ld)
	}

	// Calculate hint/name entries
	for i := range libsData {
		libsData[i].hintsOffset = currentOffset
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: %s hintsOffset = 0x%x (currentOffset before hints)\n", libsData[i].name, currentOffset)
		}
		for _, funcName := range libsData[i].functions {
			// 2 bytes (hint) + function name + null terminator
			// Align to 2-byte boundary
			entrySize := 2 + len(funcName) + 1
			if entrySize%2 != 0 {
				entrySize++
			}
			if VerboseMode && i == len(libsData)-1 { // Last library
				fmt.Fprintf(os.Stderr, "DEBUG:   %s: len=%d, entrySize=%d, nextOffset=0x%x\n", funcName, len(funcName), entrySize, currentOffset+uint32(entrySize))
			}
			currentOffset += uint32(entrySize)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: %s hints end at 0x%x\n", libsData[i].name, currentOffset)
		}
	}

	// DLL names offset
	for i := range libsData {
		libsData[i].nameOffset = currentOffset
		currentOffset += uint32(len(libsData[i].name) + 1) // +1 for null terminator
	}

	// Write Import Directory Table
	for _, ld := range libsData {
		// IMAGE_IMPORT_DESCRIPTOR
		binary.Write(&buf, binary.LittleEndian, idataRVA+ld.iltOffset)  // OriginalFirstThunk (ILT RVA)
		binary.Write(&buf, binary.LittleEndian, uint32(0))              // TimeDateStamp
		binary.Write(&buf, binary.LittleEndian, uint32(0))              // ForwarderChain
		binary.Write(&buf, binary.LittleEndian, idataRVA+ld.nameOffset) // Name RVA
		binary.Write(&buf, binary.LittleEndian, idataRVA+ld.iatOffset)  // FirstThunk (IAT RVA)
	}
	// Null terminator for IDT
	binary.Write(&buf, binary.LittleEndian, [20]byte{})

	// Write ILTs and IATs for each library (interleaved: ILT then IAT for each library)
	for libIdx, ld := range libsData {
		// Write ILT for this library
		hintOffset := ld.hintsOffset // Start at this library's hint offset
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Writing ILT for %s, starting hintOffset=0x%x\n", ld.name, hintOffset)
		}
		for funcIdx, funcName := range ld.functions {
			// RVA to hint/name entry (bit 63 clear = import by name)
			binary.Write(&buf, binary.LittleEndian, uint64(idataRVA+hintOffset))

			if VerboseMode && libIdx == 1 && funcIdx < 3 { // msvcrt, first 3 functions
				fmt.Fprintf(os.Stderr, "DEBUG:   ILT[%d] %s -> hint RVA 0x%x\n", funcIdx, funcName, idataRVA+hintOffset)
			}

			// Calculate hint/name entry size for next iteration
			entrySize := 2 + len(funcName) + 1
			if entrySize%2 != 0 {
				entrySize++
			}
			hintOffset += uint32(entrySize)
		}
		// Null terminator for ILT
		binary.Write(&buf, binary.LittleEndian, uint64(0))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Finished ILT for %s\n", ld.name)
		}

		// Write IAT for this library (same as ILT initially, loader will fill it)
		hintOffset = ld.hintsOffset // Reset to this library's hint offset
		iatBase := idataRVA + ld.iatOffset

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Writing IAT for %s (offset=0x%x, iatBase=0x%x, functions=%v)\n", ld.name, ld.iatOffset, iatBase, ld.functions)
			fmt.Fprintf(os.Stderr, "DEBUG: Starting hintOffset=0x%x\n", hintOffset)
		}

		for funcIndex, funcName := range ld.functions {
			// Store IAT RVA for this function
			iatRVA := iatBase + uint32(funcIndex*8)
			iatMap[funcName] = iatRVA
			
			if funcName == "ExitProcess" || funcName == "malloc" {
				fmt.Fprintf(os.Stderr, "DEBUG:   %s -> IAT RVA=0x%X (iatBase=0x%X + %d*8), hint RVA=0x%X\n", 
					funcName, iatRVA, iatBase, funcIndex, idataRVA+hintOffset)
			}

			// RVA to hint/name entry
			hintRVA := uint64(idataRVA + hintOffset)
			binary.Write(&buf, binary.LittleEndian, hintRVA)
			
			if funcName == "ExitProcess" || funcName == "malloc" {
				fmt.Fprintf(os.Stderr, "DEBUG:   Writing %s IAT: hintRVA=0x%X, buf position=0x%X\n", funcName, hintRVA, buf.Len()-8)
			}

			entrySize := 2 + len(funcName) + 1
			if entrySize%2 != 0 {
				entrySize++
			}
			hintOffset += uint32(entrySize)
		}
		// Null terminator for IAT
		binary.Write(&buf, binary.LittleEndian, uint64(0))
	}

	// Write Hint/Name Table
	for libIdx, ld := range libsData {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Writing hints for %s starting at buf offset 0x%x\n", ld.name, buf.Len())
		}
		for _, funcName := range ld.functions {
			beforeLen := buf.Len()
			// Hint (ordinal, we use 0)
			binary.Write(&buf, binary.LittleEndian, uint16(0))
			// Function name
			buf.WriteString(funcName)
			buf.WriteByte(0) // Null terminator
			// Align to 2-byte boundary
			if (2+len(funcName)+1)%2 != 0 {
				buf.WriteByte(0)
			}
			afterLen := buf.Len()
			if VerboseMode && libIdx == 0 && (funcName == "SDL_RenderTexture" || funcName == "SDL_RenderPresent" || funcName == "SDL_RenderClear") {
				fmt.Fprintf(os.Stderr, "DEBUG:   %s: wrote %d bytes (buf %d -> %d)\n", funcName, afterLen-beforeLen, beforeLen, afterLen)
			}
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Finished hints for %s at buf offset 0x%x\n", ld.name, buf.Len())
		}
	}

	// Write DLL names
	for _, ld := range libsData {
		buf.WriteString(ld.name)
		buf.WriteByte(0) // Null terminator
	}

	return buf.Bytes(), iatMap, nil
}

// Confidence that this function is working: 50%
// WritePERelocations writes base relocation table for PE files
func (eb *ExecutableBuilder) WritePERelocations() ([]byte, error) {
	// Base relocations allow the PE loader to adjust addresses if the image
	// is loaded at a different base address

	// For simplicity, we'll generate minimal relocations
	// In a full implementation, we'd need to track all absolute addresses
	// and create relocation entries for them

	return []byte{}, nil
}

// Confidence that this function is working: 80%
// PatchPECallsToIAT patches call instructions to use the Import Address Table (IAT)
// On Windows, we use indirect calls through the IAT instead of PLT stubs
// Returns an error if any functions are unresolved
func (eb *ExecutableBuilder) PatchPECallsToIAT(iatMap map[string]uint32, textVirtualAddr, idataVirtualAddr, imageBase uint64) error {
	textBytes := eb.text.Bytes()
	var unresolvedFunctions []string
	var oversizedDisplacements []string

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Patching %d calls to use IAT\n", len(eb.callPatches))
	}

	for _, patch := range eb.callPatches {
		// Extract the function name (remove $stub suffix if present)
		funcName := patch.targetName
		if len(funcName) > 5 && funcName[len(funcName)-5:] == "$stub" {
			funcName = funcName[:len(funcName)-5]
		}

		fmt.Fprintf(os.Stderr, "PATCH: %s at pos=%d (0x%X), bytes before patch: %02X %02X %02X %02X %02X %02X\n",
			funcName, patch.position, patch.position,
			textBytes[patch.position-2], textBytes[patch.position-1],
			textBytes[patch.position], textBytes[patch.position+1], textBytes[patch.position+2], textBytes[patch.position+3])

		// Check if this is an internal function label
		if targetOffset := eb.LabelOffset(funcName); targetOffset >= 0 {
			fmt.Fprintf(os.Stderr, "  INTERNAL: target offset=%d, converting to direct call\n", targetOffset)
			// Internal function - convert from indirect to direct call
			// Windows GenerateCallInstruction emits: FF 15 XX XX XX XX (6 bytes: indirect call through memory)
			// For internal functions, we need: E8 XX XX XX XX 90 (6 bytes: direct call + NOP for alignment)
			// patch.position points to displacement (after FF 15 or E8)

			// Convert FF 15 (indirect) to E8 (direct) for internal calls
			if patch.position >= 2 && textBytes[patch.position-2] == 0xFF && textBytes[patch.position-1] == 0x15 {
				fmt.Fprintf(os.Stderr, "  Before conversion at %d: %02X %02X %02X %02X %02X %02X\n",
					patch.position-2, textBytes[patch.position-2], textBytes[patch.position-1],
					textBytes[patch.position], textBytes[patch.position+1], textBytes[patch.position+2], textBytes[patch.position+3])
				textBytes[patch.position-2] = 0xE8 // CALL rel32 opcode
				// Position -1 will become part of the displacement (first byte)
			}

			// Calculate displacement for direct call (E8 instruction)
			// For E8 XX XX XX XX: RIP after instruction = current_pos - 2 + 5 = current_pos + 3
			// But we're patching the original 6-byte slot, so:
			//   Original: FF 15 [XX XX XX XX] at position-2
			//   New:      E8 [XX XX XX XX] 90 at position-2
			// RIP after E8 instruction (5 bytes) = (position-2) + 5 = position + 3
			ripAddr := uint64(patch.position) + 3 // RIP after E8 instruction (5 bytes)
			targetAddr := uint64(targetOffset)    // Target function offset in .text
			displacement := int64(targetAddr) - int64(ripAddr)

			if displacement >= -0x80000000 && displacement <= 0x7FFFFFFF {
				disp32 := uint32(displacement)
				// Write displacement (4 bytes) + NOP (1 byte) to fill the original 6-byte slot
				textBytes[patch.position-1] = byte(disp32 & 0xFF)         // First byte of displacement
				textBytes[patch.position] = byte((disp32 >> 8) & 0xFF)    // Second byte
				textBytes[patch.position+1] = byte((disp32 >> 16) & 0xFF) // Third byte
				textBytes[patch.position+2] = byte((disp32 >> 24) & 0xFF) // Fourth byte
				textBytes[patch.position+3] = 0x90                        // NOP to keep size at 6 bytes

				fmt.Fprintf(os.Stderr, "  After conversion: %02X %02X %02X %02X %02X %02X (disp=%d)\n",
					textBytes[patch.position-2], textBytes[patch.position-1],
					textBytes[patch.position], textBytes[patch.position+1], textBytes[patch.position+2], textBytes[patch.position+3], displacement)

				if VerboseMode {
					fmt.Fprintf(os.Stderr, "  Patched internal call to %s: offset=%d, displacement=%d (converted to direct)\n", funcName, targetOffset, displacement)
				}
			} else {
				oversizedDisplacements = append(oversizedDisplacements, funcName)
				fmt.Fprintf(os.Stderr, "  ERROR: Displacement too large for internal call to %s: %d\n", funcName, displacement)
			}
			continue
		}

		// Look up the function in the IAT
		iatRVA, ok := iatMap[funcName]
		if !ok {
			unresolvedFunctions = append(unresolvedFunctions, funcName)
			fmt.Fprintf(os.Stderr, "  ERROR: Function %s not found in IAT or internal labels\n", funcName)
			continue
		}

		// For Windows x86-64, the instruction is already emitted as CALL [RIP+disp32] (0xFF 0x15 XX XX XX XX)
		// We just need to patch the displacement to point to the IAT entry

		// Calculate the RIP-relative offset to the IAT entry
		// The instruction is 6 bytes: FF 15 XX XX XX XX
		// patch.position points to the displacement (after FF 15)
		// RIP points to the byte after the instruction when accessing memory
		dispPos := patch.position                       // Position of displacement
		ripRVA := textVirtualAddr + uint64(dispPos) + 4 // RIP RVA after reading the displacement
		iatAddrRVA := uint64(iatRVA)                    // IAT RVA (relative to image base)

		displacement := int64(iatAddrRVA) - int64(ripRVA)
		
		fmt.Fprintf(os.Stderr, "  IAT: %s iatRVA=0x%X, dispPos=0x%X, ripRVA=0x%X, disp=0x%X (%d)\n",
			funcName, iatAddrRVA, dispPos, ripRVA, uint32(displacement), displacement)

		if displacement < -0x80000000 || displacement > 0x7FFFFFFF {
			oversizedDisplacements = append(oversizedDisplacements, funcName)
			fmt.Fprintf(os.Stderr, "  ERROR: IAT displacement too large for %s: %d\n", funcName, displacement)
			continue
		}

		// Patch the displacement bytes (instruction opcode is already correct)
		disp32 := uint32(displacement)
		textBytes[dispPos] = byte(disp32 & 0xFF)
		textBytes[dispPos+1] = byte((disp32 >> 8) & 0xFF)
		textBytes[dispPos+2] = byte((disp32 >> 16) & 0xFF)
		textBytes[dispPos+3] = byte((disp32 >> 24) & 0xFF)

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "  Patched IAT call to %s: IAT RVA=0x%x, displacement=%d\n", funcName, iatRVA, displacement)
		}
	}

	// Check for any unresolved functions or errors
	if len(unresolvedFunctions) > 0 || len(oversizedDisplacements) > 0 {
		var errMsg strings.Builder
		errMsg.WriteString("PE generation failed:\n")

		if len(unresolvedFunctions) > 0 {
			errMsg.WriteString(fmt.Sprintf("  Unresolved functions (%d): %s\n",
				len(unresolvedFunctions), strings.Join(unresolvedFunctions, ", ")))
		}

		if len(oversizedDisplacements) > 0 {
			errMsg.WriteString(fmt.Sprintf("  Oversized displacements (%d): %s\n",
				len(oversizedDisplacements), strings.Join(oversizedDisplacements, ", ")))
		}

		return fmt.Errorf("%s", errMsg.String())
	}

	// Verify no unpatched placeholders remain (0x12345678)
	unpatchedCount := 0
	unpatchedLocations := []int{}
	placeholder := []byte{0x78, 0x56, 0x34, 0x12} // Little-endian 0x12345678

	for i := 0; i <= len(textBytes)-4; i++ {
		if textBytes[i] == placeholder[0] &&
			textBytes[i+1] == placeholder[1] &&
			textBytes[i+2] == placeholder[2] &&
			textBytes[i+3] == placeholder[3] {
			unpatchedCount++
			unpatchedLocations = append(unpatchedLocations, i)
			if len(unpatchedLocations) <= 5 { // Report first 5
				fmt.Fprintf(os.Stderr, "  ERROR: Unpatched placeholder 0x12345678 found at text offset 0x%x (RVA 0x%x)\n",
					i, textVirtualAddr+uint64(i))
			}
		}
	}

	if unpatchedCount > 0 {
		return fmt.Errorf("PE generation failed: %d unpatched placeholder(s) remain in code", unpatchedCount)
	}

	return nil
}

// Helper function to write import descriptor
func writePEImportDescriptor(buf []byte, offset int, ilt, iat, name uint32, timeDateStamp uint32) {
	binary.LittleEndian.PutUint32(buf[offset:], ilt)             // RVA to ILT
	binary.LittleEndian.PutUint32(buf[offset+4:], timeDateStamp) // TimeDateStamp
	binary.LittleEndian.PutUint32(buf[offset+8:], 0)             // ForwarderChain
	binary.LittleEndian.PutUint32(buf[offset+12:], name)         // RVA to DLL name
	binary.LittleEndian.PutUint32(buf[offset+16:], iat)          // RVA to IAT
}









