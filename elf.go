// Completion: 100% - Platform support complete
package main

import (
	"fmt"
	"os"
)

const (
	// ELF structure sizes
	elfHeaderSize     = 64 // ELF64 header size
	progHeaderSize    = 56 // Program header entry size (ELF64)
	sectionHeaderSize = 64 // Section header entry size (ELF64)

	// Memory layout
	baseAddr   = 0x400000                       // Virtual base address
	pageSize   = 0x1000                         // 4KB page alignment
	headerSize = elfHeaderSize + progHeaderSize // Total header size for simple executable

	// Program header offset (immediately after ELF header)
	progHeaderOffset = 0x40 // elfHeaderSize

	// Section header offsets
	sectionHeaderEntrySize = 0x40                    // Size of section header entry
	sectionTableAddr       = progHeaderOffset + 0x38 // Section table address
)

func (eb *ExecutableBuilder) WriteELFHeader() error {
	w := eb.ELFWriter()
	rodataSize := eb.rodata.Len()
	codeSize := eb.text.Len()
	
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "WriteELFHeader: rodata=%d bytes, text=%d bytes, data=%d bytes\n",
			rodataSize, codeSize, eb.data.Len())
	}

	// Magic
	w.Write(0x7f)
	w.Write(0x45)  // E
	w.Write(0x4c)  // L
	w.Write(0x46)  // F
	w.Write(2)     // 64-bit
	w.Write(1)     // little endian
	w.Write(1)     // ELF version
	w.Write(3)     // Linux
	w.Write(3)     // ABI version, dynamic linker version
	w.WriteN(0, 7) // zero padding, length of 7
	w.Write2(2)    // object file type: executable

	// Machine type - machine specific
	w.Write2(byte(GetELFMachineType(eb.target.Arch())))

	w.Write4(1) // original ELF version (?)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}

	entry := uint64(baseAddr + headerSize + rodataSize)

	w.Write8u(entry)
	w.Write8(progHeaderOffset)
	w.Write8u(sectionTableAddr)
	w.Write4(0)
	w.Write2(elfHeaderSize)
	w.Write2(progHeaderSize)
	const programHeaderTableEntries = 1
	w.Write2(programHeaderTableEntries)
	w.Write2(sectionHeaderEntrySize)
	const sectionHeaderTableEntries = 0
	w.Write2(sectionHeaderTableEntries)
	const sectionHeaderTableEntryIndex = 0
	w.Write2(sectionHeaderTableEntryIndex)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}

	w.Write4(1)
	w.Write4(7)
	w.Write8u(0)
	w.Write8u(baseAddr)
	w.Write8u(baseAddr)
	fileSize := uint64(headerSize + rodataSize + codeSize)
	w.Write8u(fileSize)
	w.Write8u(fileSize)
	w.Write8u(pageSize)

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}

	return nil
}
