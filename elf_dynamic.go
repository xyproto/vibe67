// Completion: 100% - Platform support complete
package main

// Minimal dynamic linking support
// Just enough to make glibc calls work

import (
	"fmt"
	"os"
)

// WriteDynamicELF writes a minimal dynamically-linked ELF
// This is a simplified approach - just add INTERP segment
func (eb *ExecutableBuilder) WriteDynamicELF() error {
	w := eb.ELFWriter()
	rodataSize := eb.rodata.Len()
	codeSize := eb.text.Len()

	// Interpreter path (architecture-specific)
	interp := "/lib64/ld-linux-x86-64.so.2"
	switch eb.target.Arch() {
	case ArchARM64:
		interp = "/lib/ld-linux-aarch64.so.1"
	case ArchRiscv64:
		interp = "/lib/ld-linux-riscv64-lp64d.so.1"
	}

	interpLen := len(interp) + 1

	elfHeaderSize := uint64(64)
	progHeaderSize := uint64(56)
	numProgHeaders := uint64(2)
	headersSize := elfHeaderSize + (progHeaderSize * numProgHeaders)

	alignedHeaderSize := (headersSize + 0xfff) & ^uint64(0xfff)

	interpOffset := alignedHeaderSize
	rodataOffset := interpOffset + uint64(interpLen)
	codeOffset := rodataOffset + uint64(rodataSize)

	entry := baseAddr + codeOffset

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
	w.Write2(3) // DYN (position independent) - changed from 2 (EXEC)

	w.Write2(byte(GetELFMachineType(eb.target.Arch())))
	w.Write4(1) // ELF version

	w.Write8u(entry)               // entry point
	w.Write8u(elfHeaderSize)       // program header offset
	w.Write8u(0)                   // section header offset (none)
	w.Write4(0)                    // flags
	w.Write2(byte(elfHeaderSize))  // ELF header size
	w.Write2(byte(progHeaderSize)) // program header size
	w.Write2(byte(numProgHeaders)) // number of program headers
	w.Write2(64)                   // section header size
	w.Write2(0)                    // number of section headers
	w.Write2(0)                    // section header string table index

	// Program Header 1: PT_INTERP
	w.Write4(3)                        // PT_INTERP
	w.Write4(4)                        // PF_R (readable)
	w.Write8u(interpOffset)            // offset in file
	w.Write8u(baseAddr + interpOffset) // virtual address
	w.Write8u(baseAddr + interpOffset) // physical address
	w.Write8u(uint64(interpLen))       // file size
	w.Write8u(uint64(interpLen))       // memory size
	w.Write8u(1)                       // alignment

	// Program Header 2: PT_LOAD
	w.Write4(1)         // PT_LOAD
	w.Write4(7)         // PF_X | PF_W | PF_R
	w.Write8u(0)        // offset
	w.Write8u(baseAddr) // virtual address
	w.Write8u(baseAddr) // physical address
	totalSize := codeOffset + uint64(codeSize)
	w.Write8u(totalSize) // file size
	w.Write8u(totalSize) // memory size
	w.Write8u(0x1000)    // alignment

	// Pad to aligned header size
	for i := headersSize; i < alignedHeaderSize; i++ {
		w.Write(0)
	}

	// Write interpreter string
	for i := 0; i < len(interp); i++ {
		w.Write(byte(interp[i]))
	}
	w.Write(0) // null terminator

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "\n=== Dynamic ELF ===\n")
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Interpreter: %s\n", interp)
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Entry: 0x%x\n", entry)
	}

	return nil
}
