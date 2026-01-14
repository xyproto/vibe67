// Completion: 100% - Platform support complete
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// PEReader parses PE/DLL files to extract exported symbols
type PEReader struct {
	file     *os.File
	dosHdr   DOSHeader
	peOffset uint32
	coffHdr  COFFHeader
	optHdr   OptionalHeader64
	sections []SectionHeader
	exports  *ExportDirectory
}

// DOSHeader represents the DOS header at the beginning of a PE file
type DOSHeader struct {
	Magic    uint16 // "MZ"
	PEOffset uint32 // Offset to PE header
}

// COFFHeader represents the COFF file header
type COFFHeader struct {
	Machine              uint16
	NumberOfSections     uint16
	TimeDateStamp        uint32
	PointerToSymbolTable uint32
	NumberOfSymbols      uint32
	SizeOfOptionalHeader uint16
	Characteristics      uint16
}

// OptionalHeader64 represents the PE32+ optional header
type OptionalHeader64 struct {
	Magic                   uint16
	MajorLinkerVersion      uint8
	MinorLinkerVersion      uint8
	SizeOfCode              uint32
	SizeOfInitializedData   uint32
	SizeOfUninitializedData uint32
	AddressOfEntryPoint     uint32
	BaseOfCode              uint32
	ImageBase               uint64
	SectionAlignment        uint32
	FileAlignment           uint32
	MajorOSVersion          uint16
	MinorOSVersion          uint16
	MajorImageVersion       uint16
	MinorImageVersion       uint16
	MajorSubsystemVersion   uint16
	MinorSubsystemVersion   uint16
	Win32VersionValue       uint32
	SizeOfImage             uint32
	SizeOfHeaders           uint32
	CheckSum                uint32
	Subsystem               uint16
	DllCharacteristics      uint16
	SizeOfStackReserve      uint64
	SizeOfStackCommit       uint64
	SizeOfHeapReserve       uint64
	SizeOfHeapCommit        uint64
	LoaderFlags             uint32
	NumberOfRvaAndSizes     uint32
	DataDirectory           [16]DataDirectory
}

// DataDirectory represents a data directory entry
type DataDirectory struct {
	VirtualAddress uint32
	Size           uint32
}

// SectionHeader represents a PE section header
type SectionHeader struct {
	Name                 [8]byte
	VirtualSize          uint32
	VirtualAddress       uint32
	SizeOfRawData        uint32
	PointerToRawData     uint32
	PointerToRelocations uint32
	PointerToLinenumbers uint32
	NumberOfRelocations  uint16
	NumberOfLinenumbers  uint16
	Characteristics      uint32
}

// ExportDirectory represents the export directory table
type ExportDirectory struct {
	Characteristics       uint32
	TimeDateStamp         uint32
	MajorVersion          uint16
	MinorVersion          uint16
	Name                  uint32
	Base                  uint32
	NumberOfFunctions     uint32
	NumberOfNames         uint32
	AddressOfFunctions    uint32
	AddressOfNames        uint32
	AddressOfNameOrdinals uint32
	Functions             []ExportedFunction
}

// ExportedFunction represents an exported function
type ExportedFunction struct {
	Name    string
	Ordinal uint16
	RVA     uint32
}

// OpenPE opens a PE/DLL file for reading
func OpenPE(filepath string) (*PEReader, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PE file: %v", err)
	}

	pr := &PEReader{
		file: file,
	}

	// Read DOS header
	if err := pr.readDOSHeader(); err != nil {
		file.Close()
		return nil, err
	}

	// Read PE headers
	if err := pr.readPEHeaders(); err != nil {
		file.Close()
		return nil, err
	}

	// Read sections
	if err := pr.readSections(); err != nil {
		file.Close()
		return nil, err
	}

	return pr, nil
}

// Close closes the PE file
func (pr *PEReader) Close() error {
	return pr.file.Close()
}

// readDOSHeader reads the DOS header
func (pr *PEReader) readDOSHeader() error {
	var magic uint16
	if err := binary.Read(pr.file, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("failed to read DOS magic: %v", err)
	}

	if magic != 0x5A4D { // "MZ"
		return fmt.Errorf("invalid DOS magic: 0x%04x (expected 0x5A4D)", magic)
	}

	pr.dosHdr.Magic = magic

	// Seek to PE offset location (at 0x3C)
	if _, err := pr.file.Seek(0x3C, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to PE offset: %v", err)
	}

	if err := binary.Read(pr.file, binary.LittleEndian, &pr.dosHdr.PEOffset); err != nil {
		return fmt.Errorf("failed to read PE offset: %v", err)
	}

	pr.peOffset = pr.dosHdr.PEOffset
	return nil
}

// readPEHeaders reads the PE signature, COFF header, and optional header
func (pr *PEReader) readPEHeaders() error {
	// Seek to PE signature
	if _, err := pr.file.Seek(int64(pr.peOffset), io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to PE signature: %v", err)
	}

	// Read PE signature
	var peSig uint32
	if err := binary.Read(pr.file, binary.LittleEndian, &peSig); err != nil {
		return fmt.Errorf("failed to read PE signature: %v", err)
	}

	if peSig != 0x00004550 { // "PE\0\0"
		return fmt.Errorf("invalid PE signature: 0x%08x", peSig)
	}

	// Read COFF header
	if err := binary.Read(pr.file, binary.LittleEndian, &pr.coffHdr); err != nil {
		return fmt.Errorf("failed to read COFF header: %v", err)
	}

	// Read optional header if present
	if pr.coffHdr.SizeOfOptionalHeader > 0 {
		// Read magic to determine 32-bit or 64-bit
		var magic uint16
		if err := binary.Read(pr.file, binary.LittleEndian, &magic); err != nil {
			return fmt.Errorf("failed to read optional header magic: %v", err)
		}

		// Seek back
		if _, err := pr.file.Seek(-2, io.SeekCurrent); err != nil {
			return fmt.Errorf("failed to seek back: %v", err)
		}

		if magic == 0x020B { // PE32+
			if err := binary.Read(pr.file, binary.LittleEndian, &pr.optHdr); err != nil {
				return fmt.Errorf("failed to read optional header: %v", err)
			}
		} else if magic == 0x010B { // PE32
			return fmt.Errorf("PE32 (32-bit) files not supported, only PE32+ (64-bit)")
		} else {
			return fmt.Errorf("unknown optional header magic: 0x%04x", magic)
		}
	}

	return nil
}

// readSections reads the section headers
func (pr *PEReader) readSections() error {
	// Section headers immediately follow the optional header
	offset := int64(pr.peOffset) + 4 + int64(binary.Size(pr.coffHdr)) + int64(pr.coffHdr.SizeOfOptionalHeader)
	if _, err := pr.file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to section headers: %v", err)
	}

	pr.sections = make([]SectionHeader, pr.coffHdr.NumberOfSections)
	for i := 0; i < int(pr.coffHdr.NumberOfSections); i++ {
		if err := binary.Read(pr.file, binary.LittleEndian, &pr.sections[i]); err != nil {
			return fmt.Errorf("failed to read section %d: %v", i, err)
		}
	}

	return nil
}

// GetExports parses and returns the export directory
func (pr *PEReader) GetExports() (*ExportDirectory, error) {
	if pr.exports != nil {
		return pr.exports, nil
	}

	// Get export directory from data directory (index 0)
	exportDir := pr.optHdr.DataDirectory[0]
	if exportDir.Size == 0 {
		return nil, fmt.Errorf("no export directory")
	}

	// Find section containing export directory
	section := pr.rvaToSection(exportDir.VirtualAddress)
	if section == nil {
		return nil, fmt.Errorf("export directory RVA not found in any section")
	}

	// Convert RVA to file offset
	fileOffset := pr.rvaToFileOffset(exportDir.VirtualAddress)
	if fileOffset == 0 {
		return nil, fmt.Errorf("failed to convert export RVA to file offset")
	}

	// Seek to export directory
	if _, err := pr.file.Seek(int64(fileOffset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to export directory: %v", err)
	}

	// Read export directory table
	var expDir ExportDirectory
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.Characteristics); err != nil {
		return nil, fmt.Errorf("failed to read export characteristics: %v", err)
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.TimeDateStamp); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.MajorVersion); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.MinorVersion); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.Name); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.Base); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.NumberOfFunctions); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.NumberOfNames); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.AddressOfFunctions); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.AddressOfNames); err != nil {
		return nil, err
	}
	if err := binary.Read(pr.file, binary.LittleEndian, &expDir.AddressOfNameOrdinals); err != nil {
		return nil, err
	}

	// Read export address table (function RVAs)
	funcAddrs := make([]uint32, expDir.NumberOfFunctions)
	if err := pr.readRVAArray(expDir.AddressOfFunctions, funcAddrs); err != nil {
		return nil, fmt.Errorf("failed to read function addresses: %v", err)
	}

	// Read name pointer table
	nameRVAs := make([]uint32, expDir.NumberOfNames)
	if err := pr.readRVAArray(expDir.AddressOfNames, nameRVAs); err != nil {
		return nil, fmt.Errorf("failed to read name RVAs: %v", err)
	}

	// Read name ordinals
	nameOrdinals := make([]uint16, expDir.NumberOfNames)
	ordOffset := pr.rvaToFileOffset(expDir.AddressOfNameOrdinals)
	if _, err := pr.file.Seek(int64(ordOffset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to name ordinals: %v", err)
	}
	for i := range nameOrdinals {
		if err := binary.Read(pr.file, binary.LittleEndian, &nameOrdinals[i]); err != nil {
			return nil, fmt.Errorf("failed to read name ordinal %d: %v", i, err)
		}
	}

	// Read function names and build export list
	expDir.Functions = make([]ExportedFunction, 0, expDir.NumberOfNames)
	for i := uint32(0); i < expDir.NumberOfNames; i++ {
		name, err := pr.readStringAtRVA(nameRVAs[i])
		if err != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: failed to read export name %d: %v\n", i, err)
			}
			continue
		}

		ordinal := nameOrdinals[i]
		if ordinal >= uint16(expDir.NumberOfFunctions) {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: invalid ordinal %d for function %s\n", ordinal, name)
			}
			continue
		}

		rva := funcAddrs[ordinal]
		expDir.Functions = append(expDir.Functions, ExportedFunction{
			Name:    name,
			Ordinal: ordinal + uint16(expDir.Base),
			RVA:     rva,
		})
	}

	pr.exports = &expDir
	return &expDir, nil
}

// readRVAArray reads an array of RVAs from the given RVA
func (pr *PEReader) readRVAArray(rva uint32, out []uint32) error {
	offset := pr.rvaToFileOffset(rva)
	if _, err := pr.file.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}
	return binary.Read(pr.file, binary.LittleEndian, out)
}

// readStringAtRVA reads a null-terminated string at the given RVA
func (pr *PEReader) readStringAtRVA(rva uint32) (string, error) {
	offset := pr.rvaToFileOffset(rva)
	if _, err := pr.file.Seek(int64(offset), io.SeekStart); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	b := make([]byte, 1)
	for {
		if _, err := pr.file.Read(b); err != nil {
			return "", err
		}
		if b[0] == 0 {
			break
		}
		buf.WriteByte(b[0])
	}

	return buf.String(), nil
}

// rvaToSection finds the section containing the given RVA
func (pr *PEReader) rvaToSection(rva uint32) *SectionHeader {
	for i := range pr.sections {
		section := &pr.sections[i]
		if rva >= section.VirtualAddress && rva < section.VirtualAddress+section.VirtualSize {
			return section
		}
	}
	return nil
}

// rvaToFileOffset converts an RVA to a file offset
func (pr *PEReader) rvaToFileOffset(rva uint32) uint32 {
	section := pr.rvaToSection(rva)
	if section == nil {
		return 0
	}
	return rva - section.VirtualAddress + section.PointerToRawData
}

// GetSectionName returns the name of a section
func (sh *SectionHeader) GetName() string {
	// Section names are 8 bytes, null-terminated or space-padded
	name := string(sh.Name[:])
	if idx := strings.IndexByte(name, 0); idx != -1 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

// ParseDLL is a convenience function to parse a DLL and return exported functions
func ParseDLL(filepath string) ([]ExportedFunction, error) {
	pr, err := OpenPE(filepath)
	if err != nil {
		return nil, err
	}
	defer pr.Close()

	exports, err := pr.GetExports()
	if err != nil {
		return nil, err
	}

	return exports.Functions, nil
}









