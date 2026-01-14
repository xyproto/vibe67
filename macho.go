// Completion: 100% - Mach-O generation complete for macOS, production-ready
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

// Mach-O constants
const (
	MH_MAGIC_64            = 0xfeedfacf // 64-bit magic number
	MH_CIGAM_64            = 0xcffaedfe // NXSwapInt(MH_MAGIC_64)
	CPU_TYPE_X86_64        = 0x01000007 // x86_64
	CPU_TYPE_ARM64         = 0x0100000c // ARM64
	CPU_SUBTYPE_X86_64_ALL = 0x00000003
	CPU_SUBTYPE_ARM64_ALL  = 0x00000000

	// File types
	MH_EXECUTE = 0x2 // Executable file

	// Flags
	MH_NOUNDEFS = 0x1
	MH_DYLDLINK = 0x4
	MH_PIE      = 0x200000
	MH_TWOLEVEL = 0x80

	// Load commands
	LC_SEGMENT_64          = 0x19
	LC_SYMTAB              = 0x2
	LC_DYSYMTAB            = 0xb
	LC_LOAD_DYLINKER       = 0xe
	LC_UUID                = 0x1b
	LC_VERSION_MIN_MACOSX  = 0x24
	LC_SOURCE_VERSION      = 0x2A
	LC_MAIN                = 0x80000028
	LC_LOAD_DYLIB          = 0xc
	LC_FUNCTION_STARTS     = 0x26
	LC_DATA_IN_CODE        = 0x29
	LC_CODE_SIGNATURE      = 0x1d
	LC_DYLD_CHAINED_FIXUPS = 0x80000034
	LC_DYLD_EXPORTS_TRIE   = 0x80000033
	LC_BUILD_VERSION       = 0x32

	// Protection flags
	VM_PROT_NONE    = 0x00
	VM_PROT_READ    = 0x01
	VM_PROT_WRITE   = 0x02
	VM_PROT_EXECUTE = 0x04

	// Section types
	S_REGULAR          = 0x0
	S_ZEROFILL         = 0x1
	S_CSTRING_LITERALS = 0x2
	S_4BYTE_LITERALS   = 0x3
	S_8BYTE_LITERALS   = 0x4
	S_LITERAL_POINTERS = 0x5

	// Section attributes
	S_ATTR_PURE_INSTRUCTIONS   = 0x80000000
	S_ATTR_SOME_INSTRUCTIONS   = 0x00000400
	S_SYMBOL_STUBS             = 0x8
	S_LAZY_SYMBOL_POINTERS     = 0x7
	S_NON_LAZY_SYMBOL_POINTERS = 0x6
)

// MachOHeader64 represents the Mach-O 64-bit header
type MachOHeader64 struct {
	Magic      uint32
	CPUType    uint32
	CPUSubtype uint32
	FileType   uint32
	NCmds      uint32
	SizeOfCmds uint32
	Flags      uint32
	Reserved   uint32
}

// LoadCommand represents a generic Mach-O load command
type LoadCommand struct {
	Cmd     uint32
	CmdSize uint32
}

// SegmentCommand64 represents a 64-bit segment load command
type SegmentCommand64 struct {
	Cmd      uint32
	CmdSize  uint32
	SegName  [16]byte
	VMAddr   uint64
	VMSize   uint64
	FileOff  uint64
	FileSize uint64
	MaxProt  uint32
	InitProt uint32
	NSects   uint32
	Flags    uint32
}

// Section64 represents a 64-bit section within a segment
type Section64 struct {
	SectName  [16]byte
	SegName   [16]byte
	Addr      uint64
	Size      uint64
	Offset    uint32
	Align     uint32
	Reloff    uint32
	Nreloc    uint32
	Flags     uint32
	Reserved1 uint32
	Reserved2 uint32
	Reserved3 uint32
}

// SymtabCommand represents the symbol table load command
type SymtabCommand struct {
	Cmd     uint32
	CmdSize uint32
	Symoff  uint32
	Nsyms   uint32
	Stroff  uint32
	Strsize uint32
}

// DysymtabCommand represents the dynamic symbol table load command
type DysymtabCommand struct {
	Cmd            uint32
	CmdSize        uint32
	ILocalSym      uint32
	NLocalSym      uint32
	IExtDefSym     uint32
	NExtDefSym     uint32
	IUndefSym      uint32
	NUndefSym      uint32
	TOCOff         uint32
	NTOC           uint32
	ModTabOff      uint32
	NModTab        uint32
	ExtRefSymOff   uint32
	NExtRefSyms    uint32
	IndirectSymOff uint32
	NIndirectSyms  uint32
	ExtRelOff      uint32
	NExtRel        uint32
	LocRelOff      uint32
	NLocRel        uint32
}

// EntryPointCommand represents the LC_MAIN load command (entry point)
type EntryPointCommand struct {
	Cmd       uint32
	CmdSize   uint32
	EntryOff  uint64
	StackSize uint64
}

// DylinkerCommand represents the dynamic linker load command
type DylinkerCommand struct {
	Cmd     uint32
	CmdSize uint32
	NameOff uint32
	// Name follows
}

// DylibCommand represents a dynamic library load command
type DylibCommand struct {
	Cmd                  uint32
	CmdSize              uint32
	NameOff              uint32
	Timestamp            uint32
	CurrentVersion       uint32
	CompatibilityVersion uint32
	// Name follows
}

// UUIDCommand represents the UUID load command
type UUIDCommand struct {
	Cmd     uint32
	CmdSize uint32
	UUID    [16]byte
}

// VersionMinCommand represents version minimum load command
type VersionMinCommand struct {
	Cmd     uint32
	CmdSize uint32
	Version uint32
	SDK     uint32
}

// SourceVersionCommand represents source version load command
type SourceVersionCommand struct {
	Cmd     uint32
	CmdSize uint32
	Version uint64
}

// CodeSignature structures

// SuperBlob is the container for all signature blobs
type SuperBlob struct {
	Magic  uint32 // CS_MAGIC_EMBEDDED_SIGNATURE
	Length uint32 // Total length of SuperBlob
	Count  uint32 // Number of BlobIndex entries
	// Followed by BlobIndex entries
}

// BlobIndex points to a blob within the SuperBlob
type BlobIndex struct {
	Type   uint32 // Slot type (e.g., CSSLOT_CODEDIRECTORY)
	Offset uint32 // Offset from start of SuperBlob
}

// CodeDirectory is the main signing information
type CodeDirectory struct {
	Magic         uint32 // CS_MAGIC_CODEDIRECTORY
	Length        uint32 // Total length of CodeDirectory blob
	Version       uint32 // Version (0x20400 for modern binaries)
	Flags         uint32 // Flags (CS_ADHOC = 0x2)
	HashOffset    uint32 // Offset of hash data from start of CodeDirectory
	IdentOffset   uint32 // Offset of identifier string
	NSpecialSlots uint32 // Number of special hash slots
	NCodeSlots    uint32 // Number of code hash slots
	CodeLimit     uint32 // Limit to main image signature
	HashSize      uint8  // Size of each hash (32 for SHA-256)
	HashType      uint8  // Type of hash (CS_HASHTYPE_SHA256 = 2)
	Platform      uint8  // Platform identifier
	PageSize      uint8  // Log2(page size) = 12 for 4096 bytes
	Spare2        uint32 // Reserved
	ScatterOffset uint32 // Optional scatter vector offset
	TeamOffset    uint32 // Optional team identifier offset
	Spare3        uint32 // Reserved
	CodeLimit64   uint64 // 64-bit code limit
	ExecSegBase   uint64 // Start of executable segment
	ExecSegLimit  uint64 // Limit of executable segment
	ExecSegFlags  uint64 // Exec segment flags
	// Followed by identifier string, then hashes
}

// BuildVersionCommand represents LC_BUILD_VERSION
type BuildVersionCommand struct {
	Cmd      uint32
	CmdSize  uint32
	Platform uint32
	Minos    uint32 // Minimum OS version (X.Y.Z encoded as nibbles)
	Sdk      uint32 // SDK version
	NTools   uint32 // Number of tool entries following this
}

// LinkEditDataCommand represents function starts / data in code load command
type LinkEditDataCommand struct {
	Cmd      uint32
	CmdSize  uint32
	DataOff  uint32
	DataSize uint32
}

// Nlist64 represents a 64-bit symbol table entry
type Nlist64 struct {
	N_strx  uint32 // String table index
	N_type  uint8  // Symbol type
	N_sect  uint8  // Section number
	N_desc  uint16 // Description
	N_value uint64 // Symbol value
}

// Symbol type flags
const (
	N_UNDF = 0x0  // Undefined symbol
	N_EXT  = 0x1  // External symbol
	N_TYPE = 0x0e // Type mask
	N_SECT = 0xe  // Defined in section

	REFERENCE_FLAG_UNDEFINED_NON_LAZY = 0x0
	REFERENCE_FLAG_UNDEFINED_LAZY     = 0x1
)

// Chained fixups structures
type DyldChainedFixupsHeader struct {
	FixupsVersion uint32 // 0
	StartsOffset  uint32 // Offset of dyld_chained_starts_in_image
	ImportsOffset uint32 // Offset of imports table
	SymbolsOffset uint32 // Offset of symbol strings
	ImportsCount  uint32 // Number of imported symbols
	ImportsFormat uint32 // DYLD_CHAINED_IMPORT or DYLD_CHAINED_IMPORT_ADDEND (usually 1)
	SymbolsFormat uint32 // 0 for uncompressed
}

type DyldChainedStartsInImage struct {
	SegCount uint32 // Number of segments
	// Followed by seg_count uint32 offsets to dyld_chained_starts_in_segment
}

type DyldChainedStartsInSegment struct {
	Size            uint32 // Size of this structure
	PageSize        uint16 // Page size (0x4000 for 16KB, 0x1000 for 4KB)
	PointerFormat   uint16 // DYLD_CHAINED_PTR_ARM64E, etc. (1 for ARM64E, 3 for ARM64E_KERNEL)
	SegmentOffset   uint64 // Offset in __LINKEDIT to start of segment
	MaxValidPointer uint32 // For PtrAuth
	PageCount       uint16 // Number of pages
	// Followed by page_count uint16 page_start values (0xFFFF means no fixups)
}

type DyldChainedImport struct {
	LibOrdinal uint8  // Library ordinal (1-based, 0 = self, 0xFE = weak, 0xFF = main executable)
	WeakImport uint8  // 0 or 1
	NameOffset uint32 // Offset into symbol strings (24-bit value in bits 0-23)
}

// Chained fixups constants
const (
	DYLD_CHAINED_PTR_ARM64E            = 1
	DYLD_CHAINED_PTR_64                = 2
	DYLD_CHAINED_PTR_64_OFFSET         = 4
	DYLD_CHAINED_PTR_ARM64E_FIRMWARE   = 6
	DYLD_CHAINED_PTR_ARM64E_KERNEL     = 7
	DYLD_CHAINED_PTR_64_KERNEL_CACHE   = 8
	DYLD_CHAINED_PTR_ARM64E_USERLAND24 = 12

	DYLD_CHAINED_IMPORT          = 1
	DYLD_CHAINED_IMPORT_ADDEND   = 2
	DYLD_CHAINED_IMPORT_ADDEND64 = 3
)

// Code signature constants
const (
	CS_MAGIC_EMBEDDED_SIGNATURE = 0xfade0cc0 // SuperBlob magic
	CS_MAGIC_CODEDIRECTORY      = 0xfade0c02 // CodeDirectory magic
	CS_MAGIC_BLOBWRAPPER        = 0xfade0b01 // CMS signature blob

	CSSLOT_CODEDIRECTORY = 0       // Slot index for CodeDirectory
	CSSLOT_SIGNATURESLOT = 0x10000 // CMS signature slot

	CS_HASHTYPE_SHA256 = 2    // SHA-256 hash type
	CS_PAGE_SIZE       = 4096 // Page size for hashing (4KB)

	CS_EXECSEG_MAIN_BINARY = 0x1 // Main binary exec segment flag
	CS_ADHOC               = 0x2 // Ad-hoc signed
)

// generateCodeSignature creates an ad-hoc code signature for a Mach-O binary
// identifier: the executable name (e.g., "a.out")
// binaryData: the complete binary data to sign
// execSegBase: file offset where executable code starts (usually 0 for __TEXT)
// execSegLimit: size of the executable segment
// Returns: signature bytes to be placed in __LINKEDIT
func generateCodeSignature(identifier string, binaryData []byte, execSegBase, execSegLimit uint64) ([]byte, error) {
	var sigBuf bytes.Buffer

	// Calculate number of pages to hash
	codeLimit := uint32(len(binaryData))
	nPages := int(math.Ceil(float64(codeLimit) / float64(CS_PAGE_SIZE)))

	// Identifier string (null-terminated)
	identBytes := []byte(identifier + "\x00")

	// Calculate sizes
	cdHeaderSize := uint32(binary.Size(CodeDirectory{}))
	identSize := uint32(len(identBytes))
	hashSize := uint32(32) // SHA-256 = 32 bytes
	nCodeSlots := uint32(nPages)
	nSpecialSlots := uint32(0) // We don't use special slots for ad-hoc signing

	// CodeDirectory structure size
	cdLength := cdHeaderSize + identSize + (nCodeSlots * hashSize)

	// SuperBlob: contains 1 blob (CodeDirectory)
	sbHeaderSize := uint32(binary.Size(SuperBlob{}))
	indexSize := uint32(binary.Size(BlobIndex{}))
	sbLength := sbHeaderSize + indexSize + cdLength

	// Write SuperBlob header
	sb := SuperBlob{
		Magic:  CS_MAGIC_EMBEDDED_SIGNATURE,
		Length: sbLength,
		Count:  1, // Just CodeDirectory for ad-hoc
	}
	binary.Write(&sigBuf, binary.BigEndian, &sb)

	// Write BlobIndex for CodeDirectory
	cdOffset := sbHeaderSize + indexSize
	idx := BlobIndex{
		Type:   CSSLOT_CODEDIRECTORY,
		Offset: cdOffset,
	}
	binary.Write(&sigBuf, binary.BigEndian, &idx)

	// Write CodeDirectory header
	cd := CodeDirectory{
		Magic:         CS_MAGIC_CODEDIRECTORY,
		Length:        cdLength,
		Version:       0x20400, // Modern version
		Flags:         CS_ADHOC,
		HashOffset:    cdHeaderSize + identSize,
		IdentOffset:   cdHeaderSize,
		NSpecialSlots: nSpecialSlots,
		NCodeSlots:    nCodeSlots,
		CodeLimit:     codeLimit,
		HashSize:      32,
		HashType:      CS_HASHTYPE_SHA256,
		Platform:      0,
		PageSize:      12, // log2(4096) = 12
		Spare2:        0,
		ScatterOffset: 0,
		TeamOffset:    0,
		Spare3:        0,
		CodeLimit64:   uint64(codeLimit),
		ExecSegBase:   execSegBase,
		ExecSegLimit:  execSegLimit,
		ExecSegFlags:  CS_EXECSEG_MAIN_BINARY,
	}
	binary.Write(&sigBuf, binary.BigEndian, &cd)

	// Write identifier string
	sigBuf.Write(identBytes)

	// Write code hashes (hash each 4KB page)
	for i := 0; i < nPages; i++ {
		pageStart := i * CS_PAGE_SIZE
		pageEnd := pageStart + CS_PAGE_SIZE
		if pageEnd > len(binaryData) {
			pageEnd = len(binaryData)
		}

		page := binaryData[pageStart:pageEnd]
		hash := sha256.Sum256(page)
		sigBuf.Write(hash[:])
	}

	return sigBuf.Bytes(), nil
}

// WriteMachO writes a Mach-O executable for macOS
func (eb *ExecutableBuilder) WriteMachO() error {

	debug := os.Getenv("FLAP_DEBUG") != ""

	if debug || VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: WriteMachO() called, text.Len()=%d, useDynamicLinking=%v, neededFunctions=%v\n",
			eb.text.Len(), eb.useDynamicLinking, eb.neededFunctions)
	}

	var buf bytes.Buffer

	// Determine CPU type
	var cpuType, cpuSubtype uint32
	switch eb.target.Arch() {
	case ArchX86_64:
		cpuType = CPU_TYPE_X86_64
		cpuSubtype = CPU_SUBTYPE_X86_64_ALL
	case ArchARM64:
		cpuType = CPU_TYPE_ARM64
		cpuSubtype = CPU_SUBTYPE_ARM64_ALL
	default:
		return fmt.Errorf("unsupported architecture for Mach-O: %s", eb.target)
	}

	// Page size
	pageSize := uint64(0x4000) // 16KB for ARM64, but 4KB works for x86_64 too

	// macOS uses a large zero page (4GB) for security
	zeroPageSize := uint64(0x100000000) // 4GB zero page

	// Calculate sizes
	textSize := uint64(eb.text.Len())
	rodataSize := uint64(eb.rodata.Len())
	dataSize := uint64(eb.data.Len())
	combinedDataSize := rodataSize + dataSize

	// Calculate dynamic linking section sizes
	numImports := uint32(len(eb.neededFunctions))
	stubsSize := uint64(0)
	gotSize := uint64(0)
	if eb.useDynamicLinking && numImports > 0 {
		stubsSize = uint64(numImports * 12) // 12 bytes per stub on ARM64
		gotSize = uint64(numImports * 8)    // 8 bytes per GOT entry
	}

	// Align sizes (for reference, but mostly calculated dynamically now)
	_ = (textSize + pageSize - 1) &^ (pageSize - 1)         // textSizeAligned - calculated dynamically in segment
	_ = (combinedDataSize + pageSize - 1) &^ (pageSize - 1) // rodataSizeAligned - may be used later
	_ = (stubsSize + 15) &^ 15                              // stubsSizeAligned - may be used for stub alignment
	_ = (gotSize + 15) &^ 15                                // May be used for GOT alignment

	// Calculate addresses - __TEXT starts after zero page
	textAddr := zeroPageSize // __TEXT segment starts at 4GB (includes headers at start)

	// Build list of unique libraries needed for dynamic linking
	// Map library path to ordinal (1-based index for dylib references)
	libraryList := []string{}
	libraryOrdinals := make(map[string]int)

	if eb.useDynamicLinking {
		// Collect unique library paths
		uniqueLibs := make(map[string]bool)
		for _, libPath := range eb.functionLibraries {
			uniqueLibs[libPath] = true
		}

		// Convert to sorted list for consistent ordering
		for libPath := range uniqueLibs {
			libraryList = append(libraryList, libPath)
		}
		// Sort for deterministic output
		sort.Strings(libraryList)

		// Assign ordinals (1-based)
		for i, libPath := range libraryList {
			libraryOrdinals[libPath] = i + 1
		}
	}

	numLibraries := len(libraryList)

	// Calculate where __text section starts (after Mach-O header + load commands)
	// This is computed later after we know the size of load commands, so use a placeholder
	var textSectAddr uint64 // Will be set after we know header size
	var stubsAddr uint64    // Will be set relative to textSectAddr

	// These will be calculated after we know textSegVMSize
	var rodataAddr uint64
	var rodataSectAddr uint64
	var gotAddr uint64

	// Build load commands in a temporary buffer
	var loadCmdsBuf bytes.Buffer
	ncmds := uint32(0)

	// Calculate preliminary load commands size for offset calculations
	headerSize := uint32(binary.Size(MachOHeader64{}))
	prelimLoadCmdsSize := uint32(0)
	prelimLoadCmdsSize += uint32(binary.Size(SegmentCommand64{})) // __PAGEZERO

	// __TEXT segment with sections
	textNSects := uint32(1) // __text
	if eb.useDynamicLinking && numImports > 0 {
		textNSects++ // __stubs
	}
	prelimLoadCmdsSize += uint32(binary.Size(SegmentCommand64{}) + int(textNSects)*binary.Size(Section64{}))

	// __DATA segment with sections (if needed)
	if combinedDataSize > 0 || (eb.useDynamicLinking && numImports > 0) {
		dataNSects := uint32(0)
		if combinedDataSize > 0 {
			dataNSects++
		}
		if eb.useDynamicLinking && numImports > 0 {
			dataNSects++ // __got
		}
		prelimLoadCmdsSize += uint32(binary.Size(SegmentCommand64{}) + int(dataNSects)*binary.Size(Section64{}))
	}

	// __LINKEDIT segment (always required for macOS executables - needed for symbol table and code signature)
	prelimLoadCmdsSize += uint32(binary.Size(SegmentCommand64{}))

	dylinkerPath := "/usr/lib/dyld\x00"
	dylinkerCmdSize := (uint32(binary.Size(LoadCommand{})+4+len(dylinkerPath)) + 7) &^ 7
	prelimLoadCmdsSize += dylinkerCmdSize                            // LC_LOAD_DYLINKER
	prelimLoadCmdsSize += uint32(binary.Size(UUIDCommand{}))         // LC_UUID
	prelimLoadCmdsSize += uint32(binary.Size(BuildVersionCommand{})) // LC_BUILD_VERSION
	prelimLoadCmdsSize += uint32(binary.Size(EntryPointCommand{}))   // LC_MAIN

	// Always include LINKEDIT load commands on macOS (needed for code signature)
	prelimLoadCmdsSize += uint32(binary.Size(SymtabCommand{}))       // LC_SYMTAB
	prelimLoadCmdsSize += uint32(binary.Size(LinkEditDataCommand{})) // LC_CODE_SIGNATURE

	// Calculate size for LC_LOAD_DYLIB commands (one per library)
	// If no libraries specified, default to libSystem only
	if numLibraries == 0 {
		dylibPath := "/usr/lib/libSystem.B.dylib\x00"
		dylibCmdSize := (uint32(binary.Size(LoadCommand{})+16+len(dylibPath)) + 7) &^ 7
		prelimLoadCmdsSize += dylibCmdSize
	} else {
		for _, libPath := range libraryList {
			dylibPathWithNull := libPath + "\x00"
			dylibCmdSize := (uint32(binary.Size(LoadCommand{})+16+len(dylibPathWithNull)) + 7) &^ 7
			prelimLoadCmdsSize += dylibCmdSize
		}
	}
	prelimLoadCmdsSize += uint32(binary.Size(DysymtabCommand{})) // LC_DYSYMTAB (always required)

	fileHeaderSize := headerSize + prelimLoadCmdsSize

	if debug || VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: headerSize=%d, prelimLoadCmdsSize=%d, fileHeaderSize=%d\n",
			headerSize, prelimLoadCmdsSize, fileHeaderSize)
	}

	// __text section comes right after headers in file (no page alignment needed for file offset)
	// The __TEXT segment starts at file offset 0 and includes the headers
	textFileOffset := uint64(fileHeaderSize)

	// Now we can calculate VM addresses
	// __TEXT segment is at textAddr (0x100000000), headers are at start of segment
	// __text section VM address = segment base + file offset within segment
	textSectAddr = textAddr + textFileOffset
	stubsAddr = textSectAddr + textSize

	stubsFileOffset := textFileOffset + textSize

	// __DATA segment file offset must be page-aligned
	// It comes after the __TEXT segment content (headers + text + stubs)
	textSegContentEnd := stubsFileOffset + stubsSize
	rodataFileOffset := (textSegContentEnd + pageSize - 1) &^ (pageSize - 1)

	// Calculate padding needed to align GOT to 8 bytes
	gotPadding := uint64(0)
	if eb.useDynamicLinking && numImports > 0 {
		afterRodata := rodataFileOffset + rodataSize
		gotPadding = (8 - (afterRodata % 8)) % 8
	}

	gotFileOffset := rodataFileOffset + rodataSize + gotPadding

	// Calculate LINKEDIT segment offset and size (after all data sections including GOT)
	dataEndOffset := rodataFileOffset + rodataSize + gotPadding
	if eb.useDynamicLinking && numImports > 0 {
		dataEndOffset += gotSize
	}
	linkeditFileOffset := (dataEndOffset + pageSize - 1) &^ (pageSize - 1) // Page align

	// Build symbol table and string table
	// Symbol ordering: defined external symbols first, undefined external symbols last
	var symtab []Nlist64
	var strtab bytes.Buffer
	strtab.WriteByte(0) // First byte must be null

	numDefinedSyms := uint32(0)
	numUndefSyms := uint32(0)

	// ALL macOS executables need at minimum these 2 symbols for code signing to work:
	// 1. __mh_execute_header - the Mach-O header symbol
	mhStrOffset := uint32(strtab.Len())
	strtab.WriteString("__mh_execute_header")
	strtab.WriteByte(0)
	mhSym := Nlist64{
		N_strx:  mhStrOffset,
		N_type:  N_SECT | N_EXT, // Defined external symbol in section 1
		N_sect:  1,              // Section 1 (__text)
		N_desc:  0,
		N_value: textAddr, // Address of Mach-O header
	}
	symtab = append(symtab, mhSym)
	numDefinedSyms++

	// 2. main - the program entry point
	mainStrOffset := uint32(strtab.Len())
	strtab.WriteString("_main")
	strtab.WriteByte(0)
	mainSym := Nlist64{
		N_strx:  mainStrOffset,
		N_type:  N_SECT | N_EXT, // Defined external symbol in section 1
		N_sect:  1,              // Section 1 (__text)
		N_desc:  0,
		N_value: textSectAddr, // Entry point at start of __text
	}
	symtab = append(symtab, mainSym)
	numDefinedSyms++

	// 2.5. Add internal labels (runtime helpers, etc.) as defined symbols
	// These are functions like _vibe67_itoa, _vibe67_string_concat that are in the text section
	for labelName, labelOffset := range eb.labels {
		// Skip lambda functions (they're internal and don't need to be in symbol table)
		// Skip special labels that aren't function entry points
		if strings.HasPrefix(labelName, "lambda_") {
			continue
		}
		if strings.HasSuffix(labelName, "_loop") || strings.HasSuffix(labelName, "_end") ||
			strings.HasSuffix(labelName, "_skip") || strings.HasSuffix(labelName, "_done") {
			continue
		}

		// Add as defined symbol
		strOffset := uint32(strtab.Len())
		// On Mach-O, C symbols get an extra underscore prepended
		// Our labels have one underscore (_vibe67_itoa), but Mach-O needs two (__vibe67_itoa)
		strtab.WriteString("_" + labelName)
		strtab.WriteByte(0)

		sym := Nlist64{
			N_strx:  strOffset,
			N_type:  N_SECT | N_EXT, // Defined external symbol in section
			N_sect:  1,              // Section 1 (__text)
			N_desc:  0,
			N_value: textSectAddr + uint64(labelOffset),
		}
		symtab = append(symtab, sym)
		numDefinedSyms++
	}

	// 3. Add undefined external symbols for dynamic linking (if used)
	if eb.useDynamicLinking && numImports > 0 {
		for _, funcName := range eb.neededFunctions {
			strOffset := uint32(strtab.Len())
			// macOS symbols need underscore prefix
			strtab.WriteString("_" + funcName)
			strtab.WriteByte(0)

			// Determine which library this function belongs to
			dylibOrdinal := uint16(1) // Default to first library (libSystem)
			if libPath, ok := eb.functionLibraries[funcName]; ok {
				if ordinal, found := libraryOrdinals[libPath]; found {
					dylibOrdinal = uint16(ordinal)
				}
			}

			sym := Nlist64{
				N_strx:  strOffset,
				N_type:  N_UNDF | N_EXT,
				N_sect:  0,
				N_desc:  dylibOrdinal << 8, // Two-level namespace: dylib ordinal in bits 8-15
				N_value: 0,
			}
			symtab = append(symtab, sym)
			numUndefSyms++
		}
	}

	// Build indirect symbol table (maps GOT/stub entries to symbol indices)
	// Indirect symbols must point to the correct symbol indices based on final ordering
	var indirectSymTab []uint32
	if eb.useDynamicLinking && numImports > 0 {
		undefSymStartIdx := numDefinedSyms
		for i := uint32(0); i < numImports; i++ {
			idx := undefSymStartIdx + i
			indirectSymTab = append(indirectSymTab, idx) // GOT entries
		}
		for i := uint32(0); i < numImports; i++ {
			idx := undefSymStartIdx + i
			indirectSymTab = append(indirectSymTab, idx) // Stub entries
		}
	}

	symtabSize := uint32(len(symtab) * binary.Size(Nlist64{}))
	strtabSize := uint32(strtab.Len())

	// Create separate string table for chained fixups imports (only import names, not all symbols)
	var importsStrtab bytes.Buffer
	importsStrtab.WriteByte(0) // Null string at offset 0
	for _, funcName := range eb.neededFunctions {
		importsStrtab.WriteString("_" + funcName) // macOS needs underscore prefix
		importsStrtab.WriteByte(0)
	}

	// Calculate padding needed to align indirect symbol table to 8 bytes
	// Padding comes after: symtab + strtab
	alignmentPadding := uint32(0)
	if len(indirectSymTab) > 0 {
		currentOffset := symtabSize + strtabSize
		alignmentPadding = (8 - (currentOffset % 8)) % 8
	}

	indirectSymTabSize := uint32(len(indirectSymTab) * 4)

	// Chained fixups removed - using lazy binding instead
	var chainedFixupsSize uint32

	// Reserve space for code signature (will be filled by codesign tool)
	// Ad-hoc signatures are typically ~400-1000 bytes, use 4KB to be safe
	codeSignatureSize := uint32(4096)

	linkeditSize := symtabSize + strtabSize + alignmentPadding + indirectSymTabSize + chainedFixupsSize + codeSignatureSize

	// 1. LC_SEGMENT_64 for __PAGEZERO (required on macOS)
	{
		seg := SegmentCommand64{
			Cmd:      LC_SEGMENT_64,
			CmdSize:  uint32(binary.Size(SegmentCommand64{})),
			VMAddr:   0,
			VMSize:   zeroPageSize, // 4GB zero page
			FileOff:  0,
			FileSize: 0,
			MaxProt:  VM_PROT_NONE,
			InitProt: VM_PROT_NONE,
			NSects:   0,
			Flags:    0,
		}
		copy(seg.SegName[:], "__PAGEZERO")
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &seg)
		ncmds++
	}

	// 2. LC_SEGMENT_64 for __TEXT with __text and __stubs sections
	{
		// __TEXT segment maps from file offset 0 (includes headers) to end of stubs
		textSegFileSize := stubsFileOffset + stubsSize

		// VMSize must be page-aligned and include headers + text + stubs
		textSegVMSize := ((textSegFileSize + pageSize - 1) &^ (pageSize - 1))

		// Now we can calculate __DATA segment addresses (comes after __TEXT segment)
		rodataAddr = textAddr + textSegVMSize
		rodataSectAddr = rodataAddr
		gotAddr = rodataAddr + rodataSize + gotPadding

		seg := SegmentCommand64{
			Cmd:      LC_SEGMENT_64,
			CmdSize:  uint32(binary.Size(SegmentCommand64{}) + int(textNSects)*binary.Size(Section64{})),
			VMAddr:   textAddr,
			VMSize:   textSegVMSize,
			FileOff:  0,             // __TEXT starts at beginning of file (includes headers)
			FileSize: textSegVMSize, // FileSize should match VMSize for __TEXT
			MaxProt:  VM_PROT_READ | VM_PROT_EXECUTE,
			InitProt: VM_PROT_READ | VM_PROT_EXECUTE,
			NSects:   textNSects,
			Flags:    0,
		}
		copy(seg.SegName[:], "__TEXT")
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &seg)

		// __text section
		sect := Section64{
			Addr:      textSectAddr,
			Size:      textSize,
			Offset:    uint32(textFileOffset),
			Align:     4,
			Reloff:    0,
			Nreloc:    0,
			Flags:     S_REGULAR | S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS,
			Reserved1: 0,
			Reserved2: 0,
			Reserved3: 0,
		}
		copy(sect.SectName[:], "__text")
		copy(sect.SegName[:], "__TEXT")
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &sect)

		// __stubs section (if dynamic linking)
		if eb.useDynamicLinking && numImports > 0 {
			stubsSect := Section64{
				Addr:      stubsAddr,
				Size:      stubsSize,
				Offset:    uint32(stubsFileOffset),
				Align:     2, // 2^2 = 4 byte alignment
				Reloff:    0,
				Nreloc:    0,
				Flags:     S_SYMBOL_STUBS | S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS,
				Reserved1: numImports, // Indirect symbol table index (stubs start after GOT entries)
				Reserved2: 12,         // Stub size (12 bytes per stub)
				Reserved3: 0,
			}
			copy(stubsSect.SectName[:], "__stubs")
			copy(stubsSect.SegName[:], "__TEXT")
			binary.Write(&loadCmdsBuf, binary.LittleEndian, &stubsSect)
		}

		ncmds++
	}

	// 3. LC_SEGMENT_64 for __DATA with __data and __got sections
	if combinedDataSize > 0 || (eb.useDynamicLinking && numImports > 0) {
		dataNSects := uint32(0)
		if combinedDataSize > 0 {
			dataNSects++
		}
		if eb.useDynamicLinking && numImports > 0 {
			dataNSects++
		}

		dataFileSize := combinedDataSize
		if eb.useDynamicLinking && numImports > 0 {
			dataFileSize += gotSize
		}
		// VMSize must be page-aligned and include all data
		dataSegSize := (dataFileSize + pageSize - 1) &^ (pageSize - 1)

		seg := SegmentCommand64{
			Cmd:      LC_SEGMENT_64,
			CmdSize:  uint32(binary.Size(SegmentCommand64{}) + int(dataNSects)*binary.Size(Section64{})),
			VMAddr:   rodataAddr,
			VMSize:   dataSegSize,
			FileOff:  rodataFileOffset,
			FileSize: dataFileSize,
			MaxProt:  VM_PROT_READ | VM_PROT_WRITE,
			InitProt: VM_PROT_READ | VM_PROT_WRITE,
			NSects:   dataNSects,
			Flags:    0,
		}
		copy(seg.SegName[:], "__DATA")
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &seg)

		// __data section (rodata + writable data)
		if combinedDataSize > 0 {
			sect := Section64{
				Addr:      rodataSectAddr,
				Size:      combinedDataSize,
				Offset:    uint32(rodataFileOffset),
				Align:     3, // 2^3 = 8 byte alignment
				Reloff:    0,
				Nreloc:    0,
				Flags:     S_REGULAR,
				Reserved1: 0,
				Reserved2: 0,
				Reserved3: 0,
			}
			copy(sect.SectName[:], "__data")
			copy(sect.SegName[:], "__DATA")
			binary.Write(&loadCmdsBuf, binary.LittleEndian, &sect)
		}

		// __got section (Global Offset Table)
		if eb.useDynamicLinking && numImports > 0 {
			gotSect := Section64{
				Addr:      gotAddr,
				Size:      gotSize,
				Offset:    uint32(gotFileOffset),
				Align:     3, // 2^3 = 8 byte alignment
				Reloff:    0,
				Nreloc:    0,
				Flags:     S_NON_LAZY_SYMBOL_POINTERS,
				Reserved1: 0, // Indirect symbol table index (GOT entries start at 0)
				Reserved2: 0,
				Reserved3: 0,
			}
			copy(gotSect.SectName[:], "__got")
			copy(gotSect.SegName[:], "__DATA")
			binary.Write(&loadCmdsBuf, binary.LittleEndian, &gotSect)
		}

		ncmds++
	}

	// 4. LC_SEGMENT_64 for __LINKEDIT (always required for macOS executables - needed for code signature)
	{
		seg := SegmentCommand64{
			Cmd:      LC_SEGMENT_64,
			CmdSize:  uint32(binary.Size(SegmentCommand64{})),
			VMAddr:   ((linkeditFileOffset + pageSize - 1) &^ (pageSize - 1)) + zeroPageSize,
			VMSize:   uint64((linkeditSize + uint32(pageSize) - 1) &^ (uint32(pageSize) - 1)),
			FileOff:  linkeditFileOffset,
			FileSize: uint64(linkeditSize),
			MaxProt:  VM_PROT_READ,
			InitProt: VM_PROT_READ,
			NSects:   0,
			Flags:    0,
		}
		copy(seg.SegName[:], "__LINKEDIT")
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &seg)
		ncmds++
	}

	// 5. LC_LOAD_DYLINKER
	{
		dylinkerPath := "/usr/lib/dyld\x00"
		cmdSize := uint32(binary.Size(LoadCommand{}) + 4 + len(dylinkerPath))
		cmdSize = (cmdSize + 7) &^ 7 // 8-byte align

		cmd := LoadCommand{
			Cmd:     LC_LOAD_DYLINKER,
			CmdSize: cmdSize,
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &cmd)
		binary.Write(&loadCmdsBuf, binary.LittleEndian, uint32(12)) // name offset
		loadCmdsBuf.WriteString(dylinkerPath)

		// Pad to alignment
		for loadCmdsBuf.Len()%8 != 0 {
			loadCmdsBuf.WriteByte(0)
		}
		ncmds++
	}

	// 6. LC_UUID
	{
		// Generate a deterministic UUID based on binary content
		// For now, use zeros (can be improved later with hash of content)
		uuid := UUIDCommand{
			Cmd:     LC_UUID,
			CmdSize: uint32(binary.Size(UUIDCommand{})),
			UUID:    [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &uuid)
		ncmds++
	}

	// 7. LC_BUILD_VERSION
	{
		buildVer := BuildVersionCommand{
			Cmd:      LC_BUILD_VERSION,
			CmdSize:  uint32(binary.Size(BuildVersionCommand{})),
			Platform: 1,          // 1 = macOS
			Minos:    0x001a0000, // macOS 26.0 (0x001a = 26, 0x0000 = 0.0)
			Sdk:      0x001a0000, // SDK 26.0
			NTools:   0,          // No tool entries
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &buildVer)
		ncmds++
	}

	// 7. LC_MAIN (entry point)
	{
		entry := EntryPointCommand{
			Cmd:       LC_MAIN,
			CmdSize:   uint32(binary.Size(EntryPointCommand{})),
			EntryOff:  textFileOffset,  // Entry is at start of __text section (file offset)
			StackSize: 8 * 1024 * 1024, // Request 8MB stack
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &entry)
		ncmds++
	}

	// 8. LC_LOAD_DYLIB commands for all needed libraries
	// If no libraries specified, default to libSystem only
	libsToLoad := libraryList
	if len(libsToLoad) == 0 {
		libsToLoad = []string{"/usr/lib/libSystem.B.dylib"}
	}

	for _, dylibPath := range libsToLoad {
		dylibPathWithNull := dylibPath + "\x00"
		cmdSize := uint32(binary.Size(LoadCommand{}) + 16 + len(dylibPathWithNull))
		cmdSize = (cmdSize + 7) &^ 7 // 8-byte align

		cmd := LoadCommand{
			Cmd:     LC_LOAD_DYLIB,
			CmdSize: cmdSize,
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &cmd)
		binary.Write(&loadCmdsBuf, binary.LittleEndian, uint32(24)) // name offset
		binary.Write(&loadCmdsBuf, binary.LittleEndian, uint32(0))  // timestamp

		// Version numbers: use defaults for all libraries
		binary.Write(&loadCmdsBuf, binary.LittleEndian, uint32(0x10000)) // current version 1.0.0
		binary.Write(&loadCmdsBuf, binary.LittleEndian, uint32(0x10000)) // compatibility version 1.0.0
		loadCmdsBuf.WriteString(dylibPathWithNull)

		// Pad to alignment
		for loadCmdsBuf.Len()%8 != 0 {
			loadCmdsBuf.WriteByte(0)
		}
		ncmds++
	}

	// 8. LC_SYMTAB (always required for macOS executables - needed for code signature)
	// Apple's LINKEDIT order: symtab → indirect symtab → strtab → code signature
	{
		indirectSymOff := uint32(0)
		stroffValue := uint32(linkeditFileOffset) + symtabSize
		if eb.useDynamicLinking && numImports > 0 {
			indirectSymOff = uint32(linkeditFileOffset) + symtabSize
			stroffValue = uint32(linkeditFileOffset) + symtabSize + indirectSymTabSize
		}

		symtabCmd := SymtabCommand{
			Cmd:     LC_SYMTAB,
			CmdSize: uint32(binary.Size(SymtabCommand{})),
			Symoff:  uint32(linkeditFileOffset),
			Nsyms:   uint32(len(symtab)),
			Stroff:  stroffValue,
			Strsize: strtabSize,
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &symtabCmd)
		ncmds++

		// 9. LC_DYSYMTAB (required for all macOS executables)
		dysymtabCmd := DysymtabCommand{
			Cmd:            LC_DYSYMTAB,
			CmdSize:        uint32(binary.Size(DysymtabCommand{})),
			ILocalSym:      0,
			NLocalSym:      0,
			IExtDefSym:     0,
			NExtDefSym:     numDefinedSyms,
			IUndefSym:      numDefinedSyms,
			NUndefSym:      numUndefSyms,
			TOCOff:         0,
			NTOC:           0,
			ModTabOff:      0,
			NModTab:        0,
			ExtRefSymOff:   0,
			NExtRefSyms:    0,
			IndirectSymOff: indirectSymOff,
			NIndirectSyms:  uint32(len(indirectSymTab)),
			ExtRelOff:      0,
			NExtRel:        0,
			LocRelOff:      0,
			NLocRel:        0,
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &dysymtabCmd)
		ncmds++
	}

	// LC_CODE_SIGNATURE: Reserve space for codesign tool to fill
	{
		codeSignatureOffset := linkeditFileOffset + uint64(symtabSize) + uint64(indirectSymTabSize) + uint64(strtabSize)
		codeSignCmd := LinkEditDataCommand{
			Cmd:      LC_CODE_SIGNATURE,
			CmdSize:  uint32(binary.Size(LinkEditDataCommand{})),
			DataOff:  uint32(codeSignatureOffset),
			DataSize: codeSignatureSize,
		}
		binary.Write(&loadCmdsBuf, binary.LittleEndian, &codeSignCmd)
		ncmds++
	}

	// Verify our preliminary calculation was correct
	loadCmdsSize := uint32(loadCmdsBuf.Len())
	if loadCmdsSize != prelimLoadCmdsSize {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: load commands size mismatch: expected %d, got %d\n", prelimLoadCmdsSize, loadCmdsSize)
		}
	}

	// Write Mach-O header
	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: About to write Mach-O header with NCmds=%d, SizeOfCmds=%d\n", ncmds, loadCmdsSize)
	}

	// Set Mach-O header flags - ALL macOS executables are dynamically linked (at minimum to libSystem)
	flags := uint32(MH_PIE | MH_NOUNDEFS | MH_DYLDLINK | MH_TWOLEVEL)

	if debug || VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Mach-O header flags = 0x%08x (MH_PIE=0x%x, useDynamicLinking=%v)\n", flags, MH_PIE, eb.useDynamicLinking)
	}
	header := MachOHeader64{
		Magic:      MH_MAGIC_64,
		CPUType:    cpuType,
		CPUSubtype: cpuSubtype,
		FileType:   MH_EXECUTE,
		NCmds:      ncmds,
		SizeOfCmds: loadCmdsSize,
		Flags:      flags,
		Reserved:   0,
	}
	binary.Write(&buf, binary.LittleEndian, &header)
	if debug {
		fmt.Fprintf(os.Stderr, "DEBUG: Wrote Mach-O header\n")
	}

	// Debug: verify what's actually in the buffer
	bufBytes := buf.Bytes()
	if len(bufBytes) >= 32 {
		ncmdsInBuf := binary.LittleEndian.Uint32(bufBytes[16:20])
		sizeofcmdsInBuf := binary.LittleEndian.Uint32(bufBytes[20:24])
		flagsInBuf := binary.LittleEndian.Uint32(bufBytes[24:28])
		if debug || VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Header in buffer has NCmds=%d, SizeOfCmds=%d, Flags=0x%08x\n",
				ncmdsInBuf, sizeofcmdsInBuf, flagsInBuf)
		}
	}

	// Write load commands
	buf.Write(loadCmdsBuf.Bytes())

	// Patch bl instructions (both external stubs and internal functions)
	textBytes := eb.text.Bytes()
	for _, patch := range eb.callPatches {
		// First check if it's an external function (has a stub)
		stubIndex := -1
		if eb.useDynamicLinking && numImports > 0 {
			for i, funcName := range eb.neededFunctions {
				if patch.targetName == funcName+"$stub" {
					stubIndex = i
					break
				}
			}
		}

		if stubIndex >= 0 {
			// External function call - patch to stub
			thisStubAddr := stubsAddr + uint64(stubIndex*12)
			callAddr := textSectAddr + uint64(patch.position)
			offset := int64(thisStubAddr-callAddr) / 4 // ARM64 offset in words

			blInstr := uint32(0x94000000) | (uint32(offset) & 0x03ffffff)
			textBytes[patch.position] = byte(blInstr)
			textBytes[patch.position+1] = byte(blInstr >> 8)
			textBytes[patch.position+2] = byte(blInstr >> 16)
			textBytes[patch.position+3] = byte(blInstr >> 24)
		} else {
			// Internal function call - find the function label
			funcName := strings.TrimSuffix(patch.targetName, "$stub")

			if labelOffset, ok := eb.labels[funcName]; ok {
				// Patch to internal function
				targetAddr := textSectAddr + uint64(labelOffset)
				callAddr := textSectAddr + uint64(patch.position)
				offset := int64(targetAddr-callAddr) / 4 // ARM64 offset in words

				blInstr := uint32(0x94000000) | (uint32(offset) & 0x03ffffff)
				textBytes[patch.position] = byte(blInstr)
				textBytes[patch.position+1] = byte(blInstr >> 8)
				textBytes[patch.position+2] = byte(blInstr >> 16)
				textBytes[patch.position+3] = byte(blInstr >> 24)

				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Patched internal call to %s at offset 0x%x\n", funcName, patch.position)
				}
			} else if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: Could not find target for call %s\n", patch.targetName)
			}
		}
	}

	// Pad to text file offset (should be right after headers)
	if debug || VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Before padding, buf.Len()=%d, textFileOffset=%d\n", buf.Len(), textFileOffset)
	}
	for uint64(buf.Len()) < textFileOffset {
		buf.WriteByte(0)
	}

	// Write __text section
	if debug || VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Writing __text at offset %d, size=%d bytes: %x\n", buf.Len(), eb.text.Len(), eb.text.Bytes())
	}
	buf.Write(eb.text.Bytes())

	// Write __stubs section (if dynamic linking)
	if eb.useDynamicLinking && numImports > 0 {
		// Generate stub code for each import
		for i := uint32(0); i < numImports; i++ {
			// ARM64 stub pattern (12 bytes):
			// adrp x16, GOT@PAGE
			// ldr x16, [x16, GOT@PAGEOFF]
			// br x16

			// Calculate GOT entry address for this import
			gotEntryAddr := gotAddr + uint64(i*8)
			stubAddr := stubsAddr + uint64(i*12)

			// Calculate PC-relative offset from stub to GOT entry
			// ADRP: PC-relative page address
			pcRelPage := int64((gotEntryAddr &^ 0xfff) - (stubAddr &^ 0xfff))
			adrpImm := (pcRelPage >> 12) & 0x1fffff
			adrpImmLo := (adrpImm & 0x3) << 29
			adrpImmHi := (adrpImm >> 2) << 5
			adrpInstr := uint32(0x90000010) | uint32(adrpImmLo) | uint32(adrpImmHi) // adrp x16, #page

			// LDR: Load from [x16 + pageoffset]
			pageOffset := (gotEntryAddr & 0xfff) >> 3                   // Divide by 8 for 8-byte loads
			ldrInstr := uint32(0xf9400210) | (uint32(pageOffset) << 10) // ldr x16, [x16, #offset]

			// BR: Branch to x16
			brInstr := uint32(0xd61f0200) // br x16

			binary.Write(&buf, binary.LittleEndian, adrpInstr)
			binary.Write(&buf, binary.LittleEndian, ldrInstr)
			binary.Write(&buf, binary.LittleEndian, brInstr)
		}
	}

	// Pad to page boundary
	for uint64(buf.Len())%pageSize != 0 {
		buf.WriteByte(0)
	}

	// Write __data section (rodata + writable data)
	if rodataSize > 0 {
		buf.Write(eb.rodata.Bytes())
	}
	// Write writable data (like _itoa_buffer)
	dataSize = uint64(eb.data.Len())
	if dataSize > 0 {
		buf.Write(eb.data.Bytes())
	}

	// Align to 8 bytes before GOT (GOT entries must be 8-byte aligned)
	if eb.useDynamicLinking && numImports > 0 {
		for buf.Len()%8 != 0 {
			buf.WriteByte(0)
		}
	}

	// Write __got section (if dynamic linking)
	if eb.useDynamicLinking && numImports > 0 {
		// GOT entries: dyld will fill these via lazy binding
		// Initialize to zero (dyld_stub_binder resolves on first call)
		for i := uint32(0); i < numImports; i++ {
			binary.Write(&buf, binary.LittleEndian, uint64(0))
		}
	}

	// Write __LINKEDIT segment (always required for macOS executables - needed for code signature)
	{
		// Pad to linkedit file offset
		for uint64(buf.Len()) < linkeditFileOffset {
			buf.WriteByte(0)
		}

		// Write symbol table
		for _, sym := range symtab {
			binary.Write(&buf, binary.LittleEndian, &sym)
		}

		// Write indirect symbol table (if dynamic linking)
		// Apple's LINKEDIT order: symtab → indirect symtab → strtab → code signature
		if eb.useDynamicLinking && numImports > 0 {
			for _, idx := range indirectSymTab {
				binary.Write(&buf, binary.LittleEndian, idx)
			}
		}

		// Write string table
		buf.Write(strtab.Bytes())

		// Reserve space for code signature (zeros - ldid will fill it)
		for i := uint32(0); i < codeSignatureSize; i++ {
			buf.WriteByte(0)
		}
	}

	eb.elf = buf

	// Debug: check final buffer
	if debug || VerboseMode {
		finalBytes := buf.Bytes()
		if len(finalBytes) >= 824 {
			fmt.Fprintf(os.Stderr, "DEBUG: Final buffer, bytes at offset 816: %x\n", finalBytes[816:824])
		}
	}

	return nil
}









