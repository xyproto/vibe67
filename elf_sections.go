// Completion: 100% - Platform support complete
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

// ELF section types and flags
const (
	SHT_NULL     = 0
	SHT_PROGBITS = 1
	SHT_SYMTAB   = 2
	SHT_STRTAB   = 3
	SHT_RELA     = 4
	SHT_HASH     = 5
	SHT_DYNAMIC  = 6
	SHT_NOBITS   = 8
	SHT_REL      = 9
	SHT_DYNSYM   = 11

	SHF_WRITE     = 0x1
	SHF_ALLOC     = 0x2
	SHF_EXECINSTR = 0x4

	// Dynamic tags
	DT_NULL     = 0
	DT_NEEDED   = 1
	DT_PLTRELSZ = 2
	DT_PLTGOT   = 3
	DT_HASH     = 4
	DT_STRTAB   = 5
	DT_SYMTAB   = 6
	DT_RELA     = 7
	DT_RELASZ   = 8
	DT_RELAENT  = 9
	DT_STRSZ    = 10
	DT_SYMENT   = 11
	DT_PLTREL   = 20
	DT_DEBUG    = 21
	DT_JMPREL   = 23

	// Relocation types (x86_64)
	R_X86_64_NONE      = 0
	R_X86_64_JUMP_SLOT = 7
	R_X86_64_GLOB_DAT  = 6

	// Relocation types (ARM64 / AArch64)
	R_AARCH64_NONE      = 0
	R_AARCH64_JUMP_SLOT = 1026
	R_AARCH64_GLOB_DAT  = 1025

	// Relocation types (RISC-V)
	R_RISCV_NONE      = 0
	R_RISCV_JUMP_SLOT = 5
	R_RISCV_64        = 2

	// Symbol binding and type
	STB_LOCAL  = 0
	STB_GLOBAL = 1
	STT_NOTYPE = 0
	STT_FUNC   = 2
)

// DynamicSections holds all data needed for dynamic linking
type DynamicSections struct {
	// Symbol table
	dynsym     bytes.Buffer
	dynsymSyms []Symbol

	// String table
	dynstr    bytes.Buffer
	dynstrMap map[string]uint32

	// Hash table (simple)
	hash bytes.Buffer

	// Dynamic section
	dynamic bytes.Buffer

	// Relocations
	rela      bytes.Buffer
	relaCount int

	// PLT and GOT
	plt        bytes.Buffer
	got        bytes.Buffer
	pltEntries []string // function names in PLT

	// Needed libraries
	needed []string

	// Target architecture
	arch Arch
}

type Symbol struct {
	name  uint32 // offset in string table
	info  byte
	other byte
	shndx uint16
	value uint64
	size  uint64
}

func NewDynamicSections(arch Arch) *DynamicSections {
	ds := &DynamicSections{
		dynstrMap:  make(map[string]uint32),
		pltEntries: []string{},
		needed:     []string{},
		arch:       arch,
	}

	// First byte of string table must be null
	ds.dynstr.WriteByte(0)
	ds.dynstrMap[""] = 0

	// First symbol must be null symbol
	ds.addNullSymbol()

	return ds
}

// addString adds a string to dynstr and returns its offset
func (ds *DynamicSections) addString(s string) uint32 {
	if offset, exists := ds.dynstrMap[s]; exists {
		return offset
	}

	offset := uint32(ds.dynstr.Len())
	ds.dynstr.WriteString(s)
	ds.dynstr.WriteByte(0)
	ds.dynstrMap[s] = offset
	return offset
}

// addNullSymbol adds the required null symbol at index 0
func (ds *DynamicSections) addNullSymbol() {
	sym := Symbol{}
	ds.dynsymSyms = append(ds.dynsymSyms, sym)
}

// AddSymbol adds a symbol to the dynamic symbol table
func (ds *DynamicSections) AddSymbol(name string, binding byte, symtype byte) uint32 {
	nameOffset := ds.addString(name)

	sym := Symbol{
		name:  nameOffset,
		info:  (binding << 4) | (symtype & 0xf),
		other: 0,
		shndx: 0, // SHN_UNDEF for external symbols
		value: 0,
		size:  0,
	}

	ds.dynsymSyms = append(ds.dynsymSyms, sym)
	return uint32(len(ds.dynsymSyms) - 1)
}

// AddDefinedSymbol adds a symbol with a defined value (address)
func (ds *DynamicSections) AddDefinedSymbol(name string, binding byte, symtype byte, value uint64) uint32 {
	nameOffset := ds.addString(name)

	sym := Symbol{
		name:  nameOffset,
		info:  (binding << 4) | (symtype & 0xf),
		other: 0,
		shndx: 1, // SHN_ABS (absolute symbol) - no section
		value: value,
		size:  0,
	}

	ds.dynsymSyms = append(ds.dynsymSyms, sym)
	return uint32(len(ds.dynsymSyms) - 1)
}

// buildSymbolTable writes the symbol table
func (ds *DynamicSections) buildSymbolTable() {
	ds.dynsym.Reset()

	for _, sym := range ds.dynsymSyms {
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.name)
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.info)
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.other)
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.shndx)
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.value)
		binary.Write(&ds.dynsym, binary.LittleEndian, sym.size)
	}
}

// UpdateSymbolValue updates a symbol's value (address) by name
// Returns true if symbol was found and updated
func (ds *DynamicSections) UpdateSymbolValue(name string, value uint64) bool {
	// Find the symbol by name - lookup existing string offset
	targetName, exists := ds.dynstrMap[name]
	if !exists {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG UpdateSymbolValue: string '%s' not found in dynstrMap\n", name)
		}
		return false
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG UpdateSymbolValue: looking for symbol '%s' with string offset %d, value=0x%x\n", name, targetName, value)
	}

	for i := range ds.dynsymSyms {
		if ds.dynsymSyms[i].name == targetName {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG UpdateSymbolValue: found symbol at index %d, updating value from 0x%x to 0x%x, shndx from %d to 1\n",
					i, ds.dynsymSyms[i].value, value, ds.dynsymSyms[i].shndx)
			}
			ds.dynsymSyms[i].value = value
			ds.dynsymSyms[i].shndx = 1 // Mark as defined (SHN_ABS)
			return true
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG UpdateSymbolValue: symbol '%s' not found in symbol table\n", name)
	}
	return false
}

// AddNeeded adds a required shared library
func (ds *DynamicSections) AddNeeded(lib string) {
	// Add the string immediately so layout calculations are correct
	ds.addString(lib)
	ds.needed = append(ds.needed, lib)
}

// AddRelocation adds a relocation entry
func (ds *DynamicSections) AddRelocation(offset uint64, symIndex uint32, relType uint32) {
	info := (uint64(symIndex) << 32) | uint64(relType)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "AddRelocation: offset=0x%x, symIndex=%d, relType=%d, info=0x%x\n", offset, symIndex, relType, info)
	}

	binary.Write(&ds.rela, binary.LittleEndian, offset)
	binary.Write(&ds.rela, binary.LittleEndian, info)
	binary.Write(&ds.rela, binary.LittleEndian, uint64(0)) // addend

	ds.relaCount++
}

// buildHashTable creates a simple hash table
func (ds *DynamicSections) buildHashTable() {
	// Simple hash table: nbucket=1, nchain=nsyms
	nbucket := uint32(1)
	nchain := uint32(len(ds.dynsymSyms))

	ds.hash.Reset()
	binary.Write(&ds.hash, binary.LittleEndian, nbucket)
	binary.Write(&ds.hash, binary.LittleEndian, nchain)

	// Bucket (all symbols hash to bucket 0)
	binary.Write(&ds.hash, binary.LittleEndian, uint32(1))

	// Chain (simple linked list)
	for i := uint32(0); i < nchain; i++ {
		if i+1 < nchain {
			binary.Write(&ds.hash, binary.LittleEndian, i+1)
		} else {
			binary.Write(&ds.hash, binary.LittleEndian, uint32(0))
		}
	}
}

// buildDynamicSection creates the .dynamic section
func (ds *DynamicSections) buildDynamicSection(addrs map[string]uint64) {
	ds.dynamic.Reset()

	writeDynEntry := func(tag int64, val uint64) {
		binary.Write(&ds.dynamic, binary.LittleEndian, tag)
		binary.Write(&ds.dynamic, binary.LittleEndian, val)
	}

	// Needed libraries
	for _, lib := range ds.needed {
		nameOffset := ds.addString(lib)
		writeDynEntry(DT_NEEDED, uint64(nameOffset))
	}

	// Hash table
	if hashAddr, ok := addrs["hash"]; ok {
		writeDynEntry(DT_HASH, hashAddr)
	}

	// String table
	if strAddr, ok := addrs["dynstr"]; ok {
		writeDynEntry(DT_STRTAB, strAddr)
		writeDynEntry(DT_STRSZ, uint64(ds.dynstr.Len()))
	}

	// Symbol table
	if symAddr, ok := addrs["dynsym"]; ok {
		writeDynEntry(DT_SYMTAB, symAddr)
		writeDynEntry(DT_SYMENT, 24) // sizeof(Elf64_Sym)
	}

	// Relocations
	if relaAddr, ok := addrs["rela"]; ok {
		writeDynEntry(DT_JMPREL, relaAddr)
		writeDynEntry(DT_PLTRELSZ, uint64(ds.relaCount*24)) // sizeof(Elf64_Rela)
		writeDynEntry(DT_PLTREL, 7)                         // DT_RELA
	}

	// GOT
	if gotAddr, ok := addrs["got"]; ok {
		writeDynEntry(DT_PLTGOT, gotAddr)
	}

	// Debug
	writeDynEntry(DT_DEBUG, 0)

	// Terminator
	writeDynEntry(DT_NULL, 0)
}

// updatePLTGOT updates the DT_PLTGOT value in the already-built dynamic section
func (ds *DynamicSections) updatePLTGOT(gotAddr uint64) {
	// Find DT_PLTGOT entry and update it
	// Each dynamic entry is 16 bytes: 8 bytes tag, 8 bytes value
	buf := ds.dynamic.Bytes()

	for i := 0; i < len(buf); i += 16 {
		if i+16 > len(buf) {
			break
		}

		// Read tag (little-endian 64-bit)
		tag := uint64(buf[i]) | uint64(buf[i+1])<<8 | uint64(buf[i+2])<<16 | uint64(buf[i+3])<<24 |
			uint64(buf[i+4])<<32 | uint64(buf[i+5])<<40 | uint64(buf[i+6])<<48 | uint64(buf[i+7])<<56

		if tag == DT_PLTGOT {
			// Update the value (next 8 bytes)
			buf[i+8] = byte(gotAddr)
			buf[i+9] = byte(gotAddr >> 8)
			buf[i+10] = byte(gotAddr >> 16)
			buf[i+11] = byte(gotAddr >> 24)
			buf[i+12] = byte(gotAddr >> 32)
			buf[i+13] = byte(gotAddr >> 40)
			buf[i+14] = byte(gotAddr >> 48)
			buf[i+15] = byte(gotAddr >> 56)

			// Rebuild the buffer
			ds.dynamic.Reset()
			ds.dynamic.Write(buf)
			return
		}
	}
}

// updateRelocationAddress updates a relocation's r_offset field
func (ds *DynamicSections) updateRelocationAddress(oldAddr, newAddr uint64) {
	// Each relocation entry is 24 bytes:
	// - 8 bytes r_offset (address to relocate)
	// - 8 bytes r_info (symbol index and relocation type)
	// - 8 bytes r_addend
	buf := ds.rela.Bytes()

	for i := 0; i < len(buf); i += 24 {
		if i+24 > len(buf) {
			break
		}

		// Read r_offset (little-endian 64-bit)
		offset := uint64(buf[i]) | uint64(buf[i+1])<<8 | uint64(buf[i+2])<<16 | uint64(buf[i+3])<<24 |
			uint64(buf[i+4])<<32 | uint64(buf[i+5])<<40 | uint64(buf[i+6])<<48 | uint64(buf[i+7])<<56

		if offset == oldAddr {
			// Update r_offset
			buf[i] = byte(newAddr)
			buf[i+1] = byte(newAddr >> 8)
			buf[i+2] = byte(newAddr >> 16)
			buf[i+3] = byte(newAddr >> 24)
			buf[i+4] = byte(newAddr >> 32)
			buf[i+5] = byte(newAddr >> 40)
			buf[i+6] = byte(newAddr >> 48)
			buf[i+7] = byte(newAddr >> 56)

			// Rebuild the buffer
			ds.rela.Reset()
			ds.rela.Write(buf)
			return
		}
	}
}









