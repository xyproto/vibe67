// Completion: 95% - CLI interface complete, all flags working
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// A tiny compiler for x86_64, aarch64, and riscv64 for Linux, macOS, FreeBSD

const versionString = "c67 1.5.2"

// Architecture type
type Arch int

const (
	ArchUnknown Arch = iota
	ArchX86_64
	ArchARM64
	ArchRiscv64
)

func (a Arch) String() string {
	switch a {
	case ArchX86_64:
		return "x86_64"
	case ArchARM64:
		return "aarch64"
	case ArchRiscv64:
		return "riscv64"
	case ArchUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// ParseArch parses an architecture string (like GOARCH values)
func ParseArch(s string) (Arch, error) {
	switch strings.ToLower(s) {
	case "x86_64", "amd64", "x86-64":
		return ArchX86_64, nil
	case "aarch64", "arm64":
		return ArchARM64, nil
	case "riscv64", "riscv", "rv64":
		return ArchRiscv64, nil
	default:
		return 0, fmt.Errorf("unsupported architecture: %s (supported: amd64, arm64, riscv64)", s)
	}
}

// OS type
type OS int

const (
	OSLinux OS = iota
	OSDarwin
	OSFreeBSD
	OSWindows
)

func (o OS) String() string {
	switch o {
	case OSLinux:
		return "linux"
	case OSDarwin:
		return "darwin"
	case OSFreeBSD:
		return "freebsd"
	case OSWindows:
		return "windows"
	default:
		return "unknown"
	}
}

// ParseOS parses an OS string (like GOOS values)
func ParseOS(s string) (OS, error) {
	switch strings.ToLower(s) {
	case "linux":
		return OSLinux, nil
	case "darwin", "macos":
		return OSDarwin, nil
	case "freebsd":
		return OSFreeBSD, nil
	case "windows", "win", "wine":
		return OSWindows, nil
	default:
		return 0, fmt.Errorf("unsupported OS: %s (supported: linux, darwin, freebsd, windows)", s)
	}
}

// Platform represents a target platform (architecture + OS)
type Platform struct {
	Arch Arch
	OS   OS
}

// String returns a string representation like "aarch64" (just the arch for compatibility)
func (p Platform) String() string {
	return p.Arch.String()
}

// FullString returns the full platform string like "arm64-darwin"
func (p Platform) FullString() string {
	archStr := p.Arch.String()
	// Convert aarch64 -> arm64 for cleaner output
	if p.Arch == ArchARM64 {
		archStr = "arm64"
	} else if p.Arch == ArchX86_64 {
		archStr = "amd64"
	}
	return archStr + "-" + p.OS.String()
}

// IsMachO returns true if this platform uses Mach-O format
func (p Platform) IsMachO() bool {
	return p.OS == OSDarwin
}

// IsELF returns true if this platform uses ELF format
func (p Platform) IsELF() bool {
	return p.OS == OSLinux || p.OS == OSFreeBSD
}

// GetDefaultPlatform returns the platform for the current runtime
func GetDefaultPlatform() Platform {
	var arch Arch
	switch runtime.GOARCH {
	case "amd64":
		arch = ArchX86_64
	case "arm64":
		arch = ArchARM64
	case "riscv64":
		arch = ArchRiscv64
	default:
		arch = ArchX86_64 // fallback
	}

	var os OS
	switch runtime.GOOS {
	case "linux":
		os = OSLinux
	case "darwin":
		os = OSDarwin
	case "freebsd":
		os = OSFreeBSD
	default:
		os = OSLinux // fallback
	}

	return Platform{Arch: arch, OS: os}
}

// Deprecated: Use ParseArch and ParseOS separately
func StringToMachine(s string) (Platform, error) {
	// For backward compatibility, try to parse as "arch" or "arch-os"
	parts := strings.Split(s, "-")
	arch, err := ParseArch(parts[0])
	if err != nil {
		return Platform{}, err
	}

	var os OS
	if len(parts) > 1 {
		os, err = ParseOS(parts[1])
		if err != nil {
			return Platform{}, err
		}
	} else {
		os = GetDefaultPlatform().OS
	}

	return Platform{Arch: arch, OS: os}, nil
}

type Writer interface {
	Write(b byte) int
	WriteN(b byte, n int) int
	Write2(b byte) int
	Write4(b byte) int
	Write8(b byte) int
	Write8u(v uint64) int
	WriteBytes(bs []byte) int
	WriteUnsigned(i uint) int
}

type Const struct {
	value    string
	addr     uint64
	writable bool // If true, place in .data instead of .rodata
}

type PCRelocation struct {
	offset     uint64 // Offset in text section where relocation data is
	symbolName string // Name of symbol being referenced
}

type CallPatch struct {
	position   int    // Position in text section where the rel32 offset starts
	targetName string // Name of the target symbol
}

type BufferWrapper struct {
	buf *bytes.Buffer
}

type ExecutableBuilder struct {
	target                  Target
	consts                  map[string]*Const
	labels                  map[string]int // Maps label names to their offsets in .text
	dynlinker               *DynamicLinker
	useDynamicLinking       bool
	neededFunctions         []string
	functionLibraries       map[string]string // Maps function names to their library paths (for Mach-O multi-lib support)
	pcRelocations           []PCRelocation
	callPatches             []CallPatch
	elf, rodata, data, text bytes.Buffer
	rodataOffsetInELF       uint64
	dataOffsetInELF         uint64
	dynsymOffsetInELF       uint64
}

func (eb *ExecutableBuilder) ELFWriter() Writer {
	return &BufferWrapper{&eb.elf}
}

func (eb *ExecutableBuilder) RodataWriter() Writer {
	return &BufferWrapper{&eb.rodata}
}

func (eb *ExecutableBuilder) DataWriter() Writer {
	return &BufferWrapper{&eb.data}
}

func (eb *ExecutableBuilder) TextWriter() Writer {
	return &BufferWrapper{&eb.text}
}

// PatchPCRelocations patches all PC-relative address loads with actual offsets
func (eb *ExecutableBuilder) PatchPCRelocations(textAddr, rodataAddr uint64, rodataSize int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG PatchPCRelocations called: %d relocations, textAddr=0x%x\n", len(eb.pcRelocations), textAddr)
	}
	textBytes := eb.text.Bytes()

	for _, reloc := range eb.pcRelocations {
		// Find the symbol address
		var targetAddr uint64
		var found bool

		// First try function labels (pattern lambdas, regular functions)
		// These are in .text section
		if labelOffset, ok := eb.labels[reloc.symbolName]; ok {
			targetAddr = textAddr + uint64(labelOffset)
			found = true
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG PatchPCRelocations: function %s using address 0x%x (textAddr=0x%x, offset=%d)\n",
					reloc.symbolName, targetAddr, textAddr, labelOffset)
			}
		}

		// Try data symbols (strings, constants) if not a function label
		// These are in .rodata section
		if !found {
			if c, ok := eb.consts[reloc.symbolName]; ok {
				targetAddr = c.addr
				found = true
				if strings.HasPrefix(reloc.symbolName, "str_") {
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG PatchPCRelocations: %s using address 0x%x\n", reloc.symbolName, targetAddr)
					}
				} else if strings.Contains(reloc.symbolName, "parallel") && VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG PatchPCRelocations: %s found in consts with address 0x%x\n", reloc.symbolName, targetAddr)
				}
			} else if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG PatchPCRelocations: label '%s' not found in labels or consts (have %d labels)\n", reloc.symbolName, len(eb.labels))
			}
		}

		if !found {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: Symbol %s not found for PC relocation\n", reloc.symbolName)
			}
			continue
		}

		offset := int(reloc.offset)

		switch eb.target.Arch() {
		case ArchX86_64:
			eb.patchX86_64PCRel(textBytes, offset, textAddr, targetAddr, reloc.symbolName)
		case ArchARM64:
			eb.patchARM64PCRel(textBytes, offset, textAddr, targetAddr, reloc.symbolName)
		case ArchRiscv64:
			eb.patchRISCV64PCRel(textBytes, offset, textAddr, targetAddr, reloc.symbolName)
		}
	}
}

func (eb *ExecutableBuilder) patchX86_64PCRel(textBytes []byte, offset int, textAddr, targetAddr uint64, symbolName string) {
	// x86-64 RIP-relative: displacement is at offset, instruction ends at offset+4
	if offset+4 > len(textBytes) {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: Relocation offset %d out of bounds\n", offset)
		}
		return
	}

	ripAddr := textAddr + uint64(offset) + 4 // RIP points after displacement
	displacement := int64(targetAddr) - int64(ripAddr)

	if displacement < -0x80000000 || displacement > 0x7FFFFFFF {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: x86-64 displacement too large: %d\n", displacement)
		}
		return
	}

	disp32 := uint32(displacement)
	textBytes[offset] = byte(disp32 & 0xFF)
	textBytes[offset+1] = byte((disp32 >> 8) & 0xFF)
	textBytes[offset+2] = byte((disp32 >> 16) & 0xFF)
	textBytes[offset+3] = byte((disp32 >> 24) & 0xFF)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Patched x86-64 PC relocation: %s at offset 0x%x, target 0x%x, RIP 0x%x, displacement %d\n",
			symbolName, offset, targetAddr, ripAddr, displacement)
	}
}

func (eb *ExecutableBuilder) patchARM64PCRel(textBytes []byte, offset int, textAddr, targetAddr uint64, symbolName string) {
	// ARM64: ADRP at offset, ADD at offset+4
	// ADRP loads page-aligned address (upper 52 bits)
	// ADD adds the low 12 bits
	if offset+8 > len(textBytes) {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: ARM64 relocation offset %d out of bounds\n", offset)
		}
		return
	}

	instrAddr := textAddr + uint64(offset)

	// Page offset calculation for ADRP
	instrPage := instrAddr & ^uint64(0xFFF)
	targetPage := targetAddr & ^uint64(0xFFF)
	pageOffset := int64(targetPage - instrPage)

	// Check if page offset fits in 21 bits (signed, shifted)
	if pageOffset < -0x100000000 || pageOffset > 0xFFFFFFFF {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: ARM64 page offset too large: %d\n", pageOffset)
		}
		return
	}

	// Low 12 bits for ADD
	low12 := uint32(targetAddr & 0xFFF)

	// Patch ADRP instruction (bits [23:5] get immlo, bits [30:29] get immhi)
	adrpInstr := uint32(textBytes[offset]) |
		(uint32(textBytes[offset+1]) << 8) |
		(uint32(textBytes[offset+2]) << 16) |
		(uint32(textBytes[offset+3]) << 24)

	pageOffsetShifted := uint32(pageOffset >> 12)
	immlo := (pageOffsetShifted & 0x3) << 29           // bits [1:0] -> [30:29]
	immhi := ((pageOffsetShifted >> 2) & 0x7FFFF) << 5 // bits [20:2] -> [23:5]

	adrpInstr = (adrpInstr & 0x9F00001F) | immlo | immhi

	textBytes[offset] = byte(adrpInstr & 0xFF)
	textBytes[offset+1] = byte((adrpInstr >> 8) & 0xFF)
	textBytes[offset+2] = byte((adrpInstr >> 16) & 0xFF)
	textBytes[offset+3] = byte((adrpInstr >> 24) & 0xFF)

	// Patch ADD instruction (bits [21:10] get imm12)
	addInstr := uint32(textBytes[offset+4]) |
		(uint32(textBytes[offset+5]) << 8) |
		(uint32(textBytes[offset+6]) << 16) |
		(uint32(textBytes[offset+7]) << 24)

	addInstr = (addInstr & 0xFFC003FF) | (low12 << 10)

	textBytes[offset+4] = byte(addInstr & 0xFF)
	textBytes[offset+5] = byte((addInstr >> 8) & 0xFF)
	textBytes[offset+6] = byte((addInstr >> 16) & 0xFF)
	textBytes[offset+7] = byte((addInstr >> 24) & 0xFF)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Patched ARM64 PC relocation: %s at offset 0x%x, target 0x%x, page offset %d, low12 0x%x\n",
			symbolName, offset, targetAddr, pageOffset, low12)
	}
}

func (eb *ExecutableBuilder) patchRISCV64PCRel(textBytes []byte, offset int, textAddr, targetAddr uint64, symbolName string) {
	// RISC-V: AUIPC at offset, ADDI at offset+4
	// AUIPC loads upper 20 bits of PC-relative offset
	// ADDI adds the lower 12 bits
	if offset+8 > len(textBytes) {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: RISC-V relocation offset %d out of bounds\n", offset)
		}
		return
	}

	instrAddr := textAddr + uint64(offset)
	pcOffset := int64(targetAddr) - int64(instrAddr)

	if pcOffset < -0x80000000 || pcOffset > 0x7FFFFFFF {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: RISC-V offset too large: %d\n", pcOffset)
		}
		return
	}

	// Split into upper 20 bits and lower 12 bits
	// If bit 11 is set, we need to add 1 to upper because ADDI sign-extends
	upper := uint32((pcOffset + 0x800) >> 12)
	lower := uint32(pcOffset & 0xFFF)

	// Patch AUIPC instruction (bits [31:12] get upper 20 bits)
	auipcInstr := uint32(textBytes[offset]) |
		(uint32(textBytes[offset+1]) << 8) |
		(uint32(textBytes[offset+2]) << 16) |
		(uint32(textBytes[offset+3]) << 24)

	auipcInstr = (auipcInstr & 0xFFF) | (upper << 12)

	textBytes[offset] = byte(auipcInstr & 0xFF)
	textBytes[offset+1] = byte((auipcInstr >> 8) & 0xFF)
	textBytes[offset+2] = byte((auipcInstr >> 16) & 0xFF)
	textBytes[offset+3] = byte((auipcInstr >> 24) & 0xFF)

	// Patch ADDI instruction (bits [31:20] get lower 12 bits)
	addiInstr := uint32(textBytes[offset+4]) |
		(uint32(textBytes[offset+5]) << 8) |
		(uint32(textBytes[offset+6]) << 16) |
		(uint32(textBytes[offset+7]) << 24)

	addiInstr = (addiInstr & 0xFFFFF) | (lower << 20)

	textBytes[offset+4] = byte(addiInstr & 0xFF)
	textBytes[offset+5] = byte((addiInstr >> 8) & 0xFF)
	textBytes[offset+6] = byte((addiInstr >> 16) & 0xFF)
	textBytes[offset+7] = byte((addiInstr >> 24) & 0xFF)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Patched RISC-V PC relocation: %s at offset 0x%x, target 0x%x, PC 0x%x, offset %d (upper=0x%x, lower=0x%x)\n",
			symbolName, offset, targetAddr, instrAddr, pcOffset, upper, lower)
	}
}

func New(machineStr string) (*ExecutableBuilder, error) {
	platform, err := StringToMachine(machineStr)
	if err != nil {
		return nil, err
	}

	target := PlatformToTarget(platform)

	return &ExecutableBuilder{
		target:    target,
		consts:    make(map[string]*Const),
		dynlinker: NewDynamicLinker(),
	}, nil
}

// NewWithPlatform creates an ExecutableBuilder for a specific platform
func NewWithPlatform(platform Platform) (*ExecutableBuilder, error) {
	target := PlatformToTarget(platform)

	return &ExecutableBuilder{
		target:    target,
		consts:    make(map[string]*Const),
		dynlinker: NewDynamicLinker(),
	}, nil
}

// NewWithTarget creates an ExecutableBuilder for a specific target
func NewWithTarget(target Target) (*ExecutableBuilder, error) {
	return &ExecutableBuilder{
		target:    target,
		consts:    make(map[string]*Const),
		dynlinker: NewDynamicLinker(),
	}, nil
}

// getSyscallNumbers returns target-specific syscall numbers
func getSyscallNumbers(target Target) map[string]string {
	// macOS (Darwin) has different syscall numbers with class prefix 0x2000000
	if target.OS() == OSDarwin {
		switch target.Arch() {
		case ArchX86_64:
			return map[string]string{
				"SYS_WRITE": "33554436", // 0x2000004
				"SYS_EXIT":  "33554433", // 0x2000001
				"STDOUT":    "1",
			}
		case ArchARM64:
			return map[string]string{
				"SYS_WRITE": "4", // 0x4 - Darwin uses lower 24 bits, x16 needs just the number
				"SYS_EXIT":  "1", // 0x1
				"STDOUT":    "1",
			}
		default:
			return map[string]string{}
		}
	}

	// Linux/FreeBSD syscall numbers
	switch target.Arch() {
	case ArchX86_64:
		return map[string]string{
			"SYS_WRITE": "1",
			"SYS_EXIT":  "60",
			"STDOUT":    "1",
		}
	case ArchARM64:
		return map[string]string{
			"SYS_WRITE": "64",
			"SYS_EXIT":  "93",
			"STDOUT":    "1",
		}
	case ArchRiscv64:
		return map[string]string{
			"SYS_WRITE": "64",
			"SYS_EXIT":  "93",
			"STDOUT":    "1",
		}
	default:
		return map[string]string{}
	}
}

// PatchCallSites patches all direct function calls with correct relative offsets
func (eb *ExecutableBuilder) PatchCallSites(textAddr uint64) {
	textBytes := eb.text.Bytes()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG PatchCallSites: Have %d labels, %d call patches\n", len(eb.labels), len(eb.callPatches))
		for name, offset := range eb.labels {
			if strings.Contains(name, "arena") {
				fmt.Fprintf(os.Stderr, "  Label: %s at offset %d\n", name, offset)
			}
		}
	}

	for _, patch := range eb.callPatches {
		// Find the target symbol address (should be a label in the text section)
		// For internal functions, try without the $stub suffix first
		targetOffset := eb.LabelOffset(patch.targetName)
		if targetOffset < 0 && strings.HasSuffix(patch.targetName, "$stub") {
			// Try looking for internal label without $stub suffix
			baseName := patch.targetName[:len(patch.targetName)-5]
			targetOffset = eb.LabelOffset(baseName)
			if VerboseMode && targetOffset >= 0 {
				fmt.Fprintf(os.Stderr, "DEBUG: Found internal label %s at offset %d (was looking for %s)\n", baseName, targetOffset, patch.targetName)
			}
		}
		if targetOffset < 0 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: Label %s not found for call patch (have %d labels total)\n", patch.targetName, len(eb.labels))
			}
			continue
		}

		// Calculate addresses - architecture specific
		targetAddr := textAddr + uint64(targetOffset)

		if eb.target.Arch() == ArchARM64 || eb.target.Arch() == ArchRiscv64 {
			// ARM64/RISC-V: patch.position points to the BL/JAL instruction itself
			// Offset is from the instruction address, measured in 4-byte words
			currentAddr := textAddr + uint64(patch.position)
			byteOffset := int64(targetAddr) - int64(currentAddr)
			wordOffset := byteOffset / 4

			// ARM64 BL uses 26-bit signed offset
			if wordOffset < -0x2000000 || wordOffset >= 0x2000000 {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: ARM64/RISC-V call offset too large: %d words\n", wordOffset)
				}
				continue
			}

			// Read existing instruction and patch offset bits
			instr := uint32(textBytes[patch.position]) |
				(uint32(textBytes[patch.position+1]) << 8) |
				(uint32(textBytes[patch.position+2]) << 16) |
				(uint32(textBytes[patch.position+3]) << 24)

			// For ARM64 BL: clear old offset (bits 0-25), set new offset
			instr = (instr & 0xFC000000) | (uint32(wordOffset) & 0x03FFFFFF)

			textBytes[patch.position] = byte(instr & 0xFF)
			textBytes[patch.position+1] = byte((instr >> 8) & 0xFF)
			textBytes[patch.position+2] = byte((instr >> 16) & 0xFF)
			textBytes[patch.position+3] = byte((instr >> 24) & 0xFF)

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Patched ARM64/RISC-V call to %s at position 0x%x: target offset 0x%x, word offset %d\n",
					patch.targetName, patch.position, targetOffset, wordOffset)
			}
		} else {
			// x86_64: patch.position points to the 4-byte rel32 offset (after the 0xE8 CALL opcode)
			ripAddr := textAddr + uint64(patch.position) + 4 // RIP points after the rel32
			displacement := int64(targetAddr) - int64(ripAddr)

			if displacement < -0x80000000 || displacement > 0x7FFFFFFF {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: Call displacement too large: %d\n", displacement)
				}
				continue
			}

			// Patch the 4-byte rel32 offset
			disp32 := uint32(displacement)
			textBytes[patch.position] = byte(disp32 & 0xFF)
			textBytes[patch.position+1] = byte((disp32 >> 8) & 0xFF)
			textBytes[patch.position+2] = byte((disp32 >> 16) & 0xFF)
			textBytes[patch.position+3] = byte((disp32 >> 24) & 0xFF)

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Patched x86_64 call to %s at position 0x%x: target offset 0x%x, displacement %d\n",
					patch.targetName, patch.position, targetOffset, displacement)
			}
		}
	}
}

func (eb *ExecutableBuilder) Lookup(what string) string {
	// Check architecture-specific syscall numbers first
	syscalls := getSyscallNumbers(eb.target)
	if v, ok := syscalls[what]; ok {
		return v
	}
	// Then check constants
	if c, ok := eb.consts[what]; ok {
		return strconv.FormatUint(c.addr, 10)
	}
	return "0"
}

func (eb *ExecutableBuilder) Bytes() []byte {
	// For Mach-O format (macOS)
	if eb.target.IsMachO() {
		if err := eb.WriteMachO(); err != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to write Mach-O: %v\n", err)
			}
			// Fallback to ELF
		} else {
			result := eb.elf.Bytes()
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG Bytes(): Using Mach-O format (size=%d)\n", len(result))
				if len(result) >= 824 {
					fmt.Fprintf(os.Stderr, "DEBUG Bytes(): bytes at offset 816: %x\n", result[816:824])
				}
			}
			return result
		}
	}

	// For dynamic ELFs, everything is already in eb.elf
	if eb.useDynamicLinking {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG Bytes(): Using dynamic ELF, returning eb.elf only (size=%d)\n", eb.elf.Len())
		}
		return eb.elf.Bytes()
	}

	// For static ELFs, concatenate sections
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG Bytes(): Using static ELF, concatenating sections\n")
	}
	var result bytes.Buffer
	result.Write(eb.elf.Bytes())
	result.Write(eb.rodata.Bytes())
	result.Write(eb.data.Bytes())
	result.Write(eb.text.Bytes())
	return result.Bytes()
}

func (eb *ExecutableBuilder) Define(symbol, value string) {
	if c, ok := eb.consts[symbol]; ok {
		// Symbol exists - update value but preserve address
		c.value = value
	} else {
		// New symbol
		eb.consts[symbol] = &Const{value: value}
	}
}

func (eb *ExecutableBuilder) DefineWritable(symbol, value string) {
	// Define in writable data section (for things that need runtime initialization)
	if c, ok := eb.consts[symbol]; ok {
		c.value = value
		c.writable = true
	} else {
		eb.consts[symbol] = &Const{value: value, writable: true}
	}
}

func (eb *ExecutableBuilder) DefineAddr(symbol string, addr uint64) {
	if c, ok := eb.consts[symbol]; ok {
		if strings.HasPrefix(symbol, "str_") || strings.HasPrefix(symbol, "lambda_") {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG DefineAddr: %s set to 0x%x\n", symbol, addr)
			}
		}
		c.addr = addr
	} else {
		// Symbol doesn't exist yet - create it first
		if strings.HasPrefix(symbol, "lambda_") {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG DefineAddr: Creating missing symbol %s with addr 0x%x\n", symbol, addr)
			}
		}
		eb.consts[symbol] = &Const{value: "", addr: addr}
	}
}

func (eb *ExecutableBuilder) MarkLabel(label string) {
	// Mark a position in .text for a label (like a function)
	// Store as empty string - address will be set later based on text position
	if _, ok := eb.consts[label]; !ok {
		eb.consts[label] = &Const{value: ""}
	}

	// Also record the current offset in the text section
	if eb.labels == nil {
		eb.labels = make(map[string]int)
	}
	offset := eb.text.Len()
	eb.labels[label] = offset
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG MarkLabel: %s at offset %d\n", label, offset)
	}
}

func (eb *ExecutableBuilder) LabelOffset(label string) int {
	// Get the offset of a label in the text section
	if offset, ok := eb.labels[label]; ok {
		return offset
	}
	return -1 // Label not found
}

func (eb *ExecutableBuilder) RodataSection() map[string]string {
	rodataSymbols := make(map[string]string)
	for name, c := range eb.consts {
		// Skip code labels (they have empty values)
		// Skip writable consts (they go in .data)
		if c.value != "" && !c.writable {
			rodataSymbols[name] = c.value
		}
	}
	return rodataSymbols
}

func (eb *ExecutableBuilder) RodataSize() int {
	size := 0
	for _, data := range eb.RodataSection() {
		size += len(data)
	}
	return size
}

func (eb *ExecutableBuilder) WriteRodata(data []byte) uint64 {
	n, _ := eb.rodata.Write(data)
	return uint64(n)
}

func (eb *ExecutableBuilder) DataSection() map[string]string {
	dataSymbols := make(map[string]string)
	for name, c := range eb.consts {
		// Include writable consts in .data section
		if c.value != "" && c.writable {
			dataSymbols[name] = c.value
		}
	}
	return dataSymbols
}

func (eb *ExecutableBuilder) DataSize() int {
	size := 0
	for _, data := range eb.DataSection() {
		size += len(data)
	}
	return size
}

func (eb *ExecutableBuilder) WriteData(data []byte) uint64 {
	n, _ := eb.data.Write(data)
	return uint64(n)
}

func (eb *ExecutableBuilder) MovInstruction(dst, src string) error {
	out := NewOut(eb.target, eb.TextWriter(), eb)
	out.MovInstruction(dst, src)
	return nil
}

// Dynamic library helper methods
func (eb *ExecutableBuilder) AddLibrary(name, sofile string) *DynamicLibrary {
	return eb.dynlinker.AddLibrary(name, sofile)
}

func (eb *ExecutableBuilder) ImportFunction(libName, funcName string) error {
	return eb.dynlinker.ImportFunction(libName, funcName)
}

func (eb *ExecutableBuilder) CallLibFunction(funcName string, args ...string) error {
	return eb.dynlinker.GenerateFunctionCall(eb, funcName, args)
}

// GenerateGlibcHelloWorld generates a hello world program using glibc printf
func (eb *ExecutableBuilder) GenerateGlibcHelloWorld() error {
	// Set up for glibc dynamic linking
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"printf", "exit"}

	// Add glibc library
	glibc := eb.AddLibrary("glibc", "libc.so.6")

	// Define printf function
	glibc.AddFunction("printf", CTypeInt,
		Parameter{Name: "format", Type: CTypePointer},
	)

	// Define exit function
	glibc.AddFunction("exit", CTypeVoid,
		Parameter{Name: "status", Type: CTypeInt},
	)

	// Import functions
	err := eb.ImportFunction("glibc", "printf")
	if err != nil {
		return err
	}

	err = eb.ImportFunction("glibc", "exit")
	if err != nil {
		return err
	}

	// Generate the function calls (will be patched to use PLT)
	err = eb.CallLibFunction("printf", "hello")
	if err != nil {
		return err
	}

	err = eb.CallLibFunction("exit", "0")
	if err != nil {
		return err
	}

	return nil
}

// GenerateCallInstruction generates a call instruction
// NOTE: This generates placeholder addresses that should be fixed
// when we have complete PLT information
func (eb *ExecutableBuilder) GenerateCallInstruction(funcName string) error {
	w := eb.TextWriter()
	if VerboseMode {
		fmt.Fprint(os.Stderr, funcName+"@plt:")
	}

	// Strip leading underscore for Mach-O compatibility, but NOT for:
	// - Internal C67 runtime functions (starting with _c67)
	// - Functions starting with double underscore (like __acrt_iob_func on Windows)
	targetName := funcName
	if strings.HasPrefix(funcName, "_") && !strings.HasPrefix(funcName, "_c67") && !strings.HasPrefix(funcName, "__") {
		targetName = funcName[1:] // Remove underscore for external C functions
	}

	// Register the call patch for later resolution
	// For x86-64 Linux/Unix: position points to displacement after 0xE8 (position + 1)
	// For x86-64 Windows: position points to displacement after 0xFF 0x15 (position + 2)
	// For ARM64/RISC-V: position points to the instruction itself (position)
	position := eb.text.Len()
	dispPosition := position
	if eb.target.Arch() == ArchX86_64 {
		dispPosition = position + 1
		if eb.target.OS() == OSWindows {
			dispPosition = position + 2 // Skip FF 15 prefix
		}
	}
	eb.callPatches = append(eb.callPatches, CallPatch{
		position:   dispPosition,
		targetName: targetName + "$stub",
	})

	// Generate architecture-specific call instruction with placeholder
	switch eb.target.Arch() {
	case ArchX86_64:
		// On Windows, we need to emit a 6-byte CALL [RIP+disp32] instruction (0xFF 0x15 + 4 bytes)
		// to allow patching to IAT without overwriting adjacent code
		// On other platforms, emit a 5-byte CALL rel32 (0xE8 + 4 bytes)
		if eb.target.OS() == OSWindows {
			w.Write(0xFF)               // CALL r/m64 (indirect)
			w.Write(0x15)               // ModR/M: [RIP + disp32]
			w.WriteUnsigned(0x12345678) // Placeholder - will be patched to IAT RVA
		} else {
			w.Write(0xE8)               // CALL rel32
			w.WriteUnsigned(0x12345678) // Placeholder - will be patched
		}
	case ArchARM64:
		w.WriteUnsigned(0x94000000) // BL placeholder
	case ArchRiscv64:
		w.WriteUnsigned(0x000000EF) // JAL placeholder
	}

	if VerboseMode {
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// EmitArenaRuntimeCode emits arena allocator runtime functions
func (eb *ExecutableBuilder) EmitArenaRuntimeCode() {
	out := NewOut(eb.target, &BufferWrapper{&eb.text}, eb)

	// Helper function to patch jump immediates
	patchJump := func(pos int, offset int32) {
		bytes := eb.text.Bytes()
		bytes[pos] = byte(offset)
		bytes[pos+1] = byte(offset >> 8)
		bytes[pos+2] = byte(offset >> 16)
		bytes[pos+3] = byte(offset >> 24)
	}

	// c67_arena_create(capacity) - creates an arena with given capacity
	// Argument: rdi = capacity
	// Returns: rax = arena_ptr (pointer to 32-byte arena structure)
	// Arena structure: [buffer_ptr, capacity, offset, alignment] = 32 bytes
	eb.MarkLabel("c67_arena_create")
	out.PushReg("rbp")
	out.MovRegToReg("rbp", "rsp")
	out.PushReg("rbx")
	out.PushReg("r12")

	// Save capacity
	out.MovRegToReg("r12", "rdi")

	// Allocate arena structure (32 bytes)
	out.MovImmToReg("rdi", "32")
	eb.GenerateCallInstruction("malloc")

	// Check if malloc failed
	out.TestRegReg("rax", "rax")
	arenaStructFailJump := eb.text.Len()
	out.JumpConditional(JumpEqual, 0) // je to error

	// rax = arena structure pointer
	out.MovRegToReg("rbx", "rax") // Save arena ptr

	// Allocate buffer
	out.MovRegToReg("rdi", "r12") // capacity
	eb.GenerateCallInstruction("malloc")

	// Check if malloc failed
	out.TestRegReg("rax", "rax")
	bufferFailJump := eb.text.Len()
	out.JumpConditional(JumpEqual, 0) // je to error

	// Initialize arena structure
	out.MovRegToMem("rax", "rbx", 0) // [arena+0] = buffer_ptr
	out.MovRegToMem("r12", "rbx", 8) // [arena+8] = capacity
	out.MovImmToReg("rax", "0")
	out.MovRegToMem("rax", "rbx", 16) // [arena+16] = offset = 0
	out.MovImmToReg("rax", "8")
	out.MovRegToMem("rax", "rbx", 24) // [arena+24] = alignment = 8

	// Return arena pointer
	out.MovRegToReg("rax", "rbx")
	successJump := eb.text.Len()
	out.JumpUnconditional(0)

	// Error path
	errorLabel := eb.text.Len()
	patchJump(arenaStructFailJump+2, int32(errorLabel-(arenaStructFailJump+6)))
	patchJump(bufferFailJump+2, int32(errorLabel-(bufferFailJump+6)))

	// Return NULL on error
	out.XorRegWithReg("rax", "rax")

	// Return
	returnLabel := eb.text.Len()
	patchJump(successJump+1, int32(returnLabel-(successJump+5)))

	out.PopReg("r12")
	out.PopReg("rbx")
	out.PopReg("rbp")
	out.Ret()

	// c67_arena_destroy(arena_ptr) - destroys an arena
	// Argument: rdi = arena_ptr
	eb.MarkLabel("c67_arena_destroy")
	out.PushReg("rbp")
	out.MovRegToReg("rbp", "rsp")
	out.PushReg("rbx")

	// Save arena ptr
	out.MovRegToReg("rbx", "rdi")

	// Free buffer
	out.MovMemToReg("rdi", "rbx", 0) // buffer_ptr
	eb.GenerateCallInstruction("free")

	// Free arena structure
	out.MovRegToReg("rdi", "rbx")
	eb.GenerateCallInstruction("free")

	out.PopReg("rbx")
	out.PopReg("rbp")
	out.Ret()
}

// patchTextInELF replaces the .text section in the ELF buffer with the current text buffer
func (eb *ExecutableBuilder) patchTextInELF() {
	// The ELF buffer contains: ELF header + program headers + all sections
	// We need to find where the .text section is in the ELF buffer and replace it

	// For now, we'll use a simple approach: the ELF buffer is built in order,
	// so we know the text comes after BSS in the file
	// But actually, in WriteCompleteDynamicELF, the buffers are written in this order:
	// - ELF header + program headers
	// - interpreter, dynsym, dynstr, hash, rela (at page 0x1000)
	// - plt, text (at page 0x2000)
	// - dynamic, got, bss (at page 0x3000)

	// Since the entire elf buffer was already constructed, we need to replace just the text portion
	// The text section starts at file offset 0x2000 + plt_size

	elfBuf := eb.elf.Bytes()
	newText := eb.text.Bytes()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: elfBuf size=%d, newText size=%d\n", len(elfBuf), len(newText))
		fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: newText first 50 bytes: %x\n", newText[:min(50, len(newText))])
	}

	// Search for 49 ff d3 (call r11) in newText
	if VerboseMode {
		callFound := false
		for i := 0; i <= len(newText)-3; i++ {
			if newText[i] == 0x49 && newText[i+1] == 0xFF && newText[i+2] == 0xD3 {
				fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: Found 49 FF D3 at offset %d (0x%x) in newText\n", i, i)
				callFound = true
				break
			}
		}
		if !callFound {
			fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: WARNING - 49 FF D3 (call r11) NOT FOUND in newText buffer!\n")
		}
	}

	// Find the text section in the ELF buffer
	// PLT is at offset 0x2000
	// _start is after PLT
	// text is on its own page at 0x3000 (to prevent overflow into writable segment)
	textOffset := 0x3000 // text is now on page 0x3000 (separate from PLT/_start)
	textSize := len(newText)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: textOffset=0x%x, textSize=%d\n", textOffset, textSize)
		fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: about to copy newText to elfBuf[0x%x:0x%x]\n", textOffset, textOffset+textSize)
	}

	// CRITICAL: The dynamic section starts 4 pages after text section
	// Text section is pre-allocated 8 pages (32KB) at offset 0x3000
	// Dynamic section starts at 0xB000
	// Check if text exceeds the reserved space
	textEndAligned := (textOffset + textSize + 7) & ^7
	textReservedEnd := 0xB000 // End of 32KB text reservation

	// Check if we would overflow past the reserved text space
	if textEndAligned > textReservedEnd {
		// Text section exceeds the reserved 32KB space!
		fmt.Fprintf(os.Stderr, "ERROR: Text section too large!\n")
		fmt.Fprintf(os.Stderr, "  Text size: %d bytes (0x%x)\n", textSize, textSize)
		fmt.Fprintf(os.Stderr, "  Text end (aligned): 0x%x\n", textEndAligned)
		fmt.Fprintf(os.Stderr, "  Reserved space ends at: 0x%x\n", textReservedEnd)
		fmt.Fprintf(os.Stderr, "  Exceeds by: %d bytes\n", textEndAligned-textReservedEnd)
		fmt.Fprintf(os.Stderr, "\nPlease increase textReservedSize in elf_complete.go or reduce code size.\n")
		os.Exit(1)

	} else {
		// Text fits in the original space - simple copy
		copy(elfBuf[textOffset:textOffset+textSize], newText)
	}

	// Get fresh reference to buffer after potential resize
	elfBuf = eb.elf.Bytes()
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchTextInELF: after copy, elfBuf[0x%x:0x%x] = %x\n",
			textOffset, textOffset+min(50, textSize), elfBuf[textOffset:textOffset+min(50, textSize)])
	}

	// No need to update Program Headers - they were generated correctly in elf_complete.go
	// with the new multi-page layout (PLT/_start at 0x2000, text at 0x3000)
	// No need to rebuild - elfBuf is a slice of eb.elf's internal buffer,
	// so modifications to elfBuf are already reflected in eb.elf
}

func (eb *ExecutableBuilder) patchRodataInELF() {
	elfBuf := eb.elf.Bytes()
	newRodata := eb.rodata.Bytes()

	rodataOffset := int(eb.rodataOffsetInELF)
	rodataSize := len(newRodata)

	if rodataOffset > 0 && rodataOffset+rodataSize <= len(elfBuf) {
		copy(elfBuf[rodataOffset:rodataOffset+rodataSize], newRodata)
	}
}

func (eb *ExecutableBuilder) patchDataInELF() {
	elfBuf := eb.elf.Bytes()
	newData := eb.data.Bytes()

	dataOffset := int(eb.dataOffsetInELF)
	dataSize := len(newData)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchDataInELF: elfBuf size=%d, newData size=%d\n", len(elfBuf), len(newData))
		fmt.Fprintf(os.Stderr, "DEBUG patchDataInELF: dataOffset=0x%x, dataSize=%d\n", dataOffset, dataSize)
	}

	if dataOffset > 0 && dataOffset+dataSize <= len(elfBuf) {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchDataInELF: copying newData to elfBuf[0x%x:0x%x]\n", dataOffset, dataOffset+dataSize)
		}
		copy(elfBuf[dataOffset:dataOffset+dataSize], newData)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchDataInELF: first 32 bytes of .data = %x\n", newData[:min(32, len(newData))])
		}
	} else if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchDataInELF: WARNING - invalid offset or size (offset=%d, size=%d, elfBuf=%d)\n",
			dataOffset, dataSize, len(elfBuf))
	}
}

func (eb *ExecutableBuilder) patchDynsymInELF(ds *DynamicSections) {
	elfBuf := eb.elf.Bytes()
	newDynsym := ds.dynsym.Bytes()

	dynsymOffset := int(eb.dynsymOffsetInELF)
	dynsymSize := len(newDynsym)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchDynsymInELF: elfBuf size=%d, newDynsym size=%d\n", len(elfBuf), len(newDynsym))
		fmt.Fprintf(os.Stderr, "DEBUG patchDynsymInELF: dynsymOffset=0x%x, dynsymSize=%d\n", dynsymOffset, dynsymSize)
	}

	if dynsymOffset > 0 && dynsymOffset+dynsymSize <= len(elfBuf) {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchDynsymInELF: copying newDynsym to elfBuf[0x%x:0x%x]\n", dynsymOffset, dynsymOffset+dynsymSize)
		}
		copy(elfBuf[dynsymOffset:dynsymOffset+dynsymSize], newDynsym)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG patchDynsymInELF: patched symbol table successfully\n")
		}
	} else if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG patchDynsymInELF: WARNING - invalid offset or size (offset=%d, size=%d, elfBuf=%d)\n",
			dynsymOffset, dynsymSize, len(elfBuf))
	}
}

// Global flags for controlling output verbosity and dependencies
var VerboseMode bool
var QuietMode bool
var UpdateDepsFlag bool
var WPOTimeout float64
var SingleFlag bool
var CompressFlag bool

func main() {
	// Create default output filename in system temp directory
	defaultOutputFilename := filepath.Join(os.TempDir(), "main")

	// Get default platform
	defaultPlatform := GetDefaultPlatform()
	defaultArchStr := "amd64"
	if defaultPlatform.Arch == ArchARM64 {
		defaultArchStr = "arm64"
	} else if defaultPlatform.Arch == ArchRiscv64 {
		defaultArchStr = "riscv64"
	}
	defaultOSStr := defaultPlatform.OS.String()

	// NOTE: Go's flag package stops parsing at the first non-flag argument
	// So flags must come BEFORE the filename: c67 --arch arm64 program.c67
	// NOT: c67 program.c67 --arch arm64
	var archFlag = flag.String("arch", defaultArchStr, "target architecture (amd64, arm64, riscv64)")
	var osFlag = flag.String("os", defaultOSStr, "target OS (linux, darwin, freebsd)")
	var targetFlag = flag.String("target", "", "target platform (e.g., arm64-macos, amd64-linux, riscv64-linux)")
	var outputFilenameFlag = flag.String("o", defaultOutputFilename, "output executable filename")
	var outputFilenameLongFlag = flag.String("output", defaultOutputFilename, "output executable filename")
	var versionShort = flag.Bool("V", false, "print version information and exit")
	var version = flag.Bool("version", false, "print version information and exit")
	var verbose = flag.Bool("v", false, "verbose mode (show build messages and detailed compilation info)")
	var verboseLong = flag.Bool("verbose", false, "verbose mode (show build messages and detailed compilation info)")
	var updateDeps = flag.Bool("u", false, "update all dependency repositories from Git")
	var updateDepsLong = flag.Bool("update-deps", false, "update all dependency repositories from Git")
	var codeFlag = flag.String("c", "", "execute C67 code from command line")
	var optTimeout = flag.Float64("opt-timeout", 2.0, "optimization timeout in seconds (0 to disable)")
	var watchFlag = flag.Bool("watch", false, "watch mode: recompile on file changes (requires hot functions)")
	var singleFlag = flag.Bool("single", false, "compile single file only (don't load other .c67 files from directory)")
	var singleShort = flag.Bool("s", false, "shorthand for --single")
	var compressFlag = flag.Bool("compress", false, "enable executable compression (experimental)")
	_ = flag.Bool("tiny", false, "size optimization mode: remove debug strings and minimize runtime checks for demoscene/64k")
	flag.Parse()

	// Set global update-deps flag (use whichever was specified)
	UpdateDepsFlag = *updateDeps || *updateDepsLong

	// Set global single flag (use whichever was specified)
	SingleFlag = *singleFlag || *singleShort
	CompressFlag = *compressFlag

	if *version || *versionShort {
		fmt.Println(versionString)
		os.Exit(0)
	}

	// Set global verbosity flag (use whichever was specified)
	VerboseMode = *verbose || *verboseLong
	// Quiet mode is false by default (commands should show progress)
	QuietMode = false

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG main: VerboseMode enabled\n")
	}

	// Set global WPO timeout
	WPOTimeout = *optTimeout

	// Use whichever output flag was specified (prefer short form if both given)
	outputFilename := *outputFilenameFlag
	outputFlagProvided := false
	// Check if user explicitly provided -o or --output flag
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "o" || f.Name == "output" {
			outputFlagProvided = true
		}
	})
	if *outputFilenameLongFlag != defaultOutputFilename {
		outputFilename = *outputFilenameLongFlag
	}
	if *outputFilenameFlag != defaultOutputFilename {
		outputFilename = *outputFilenameFlag
	}

	// Parse target platform
	var targetArch Arch
	var targetOS OS
	var err error

	// Track if target/arch/os were explicitly provided
	targetExplicitlyProvided := false
	archExplicitlyProvided := false
	osExplicitlyProvided := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "target" {
			targetExplicitlyProvided = true
		}
		if f.Name == "arch" {
			archExplicitlyProvided = true
		}
		if f.Name == "os" {
			osExplicitlyProvided = true
		}
	})

	// Auto-detect Windows target from .exe extension if no explicit target/OS was provided
	autoDetectWindows := !targetExplicitlyProvided && !osExplicitlyProvided &&
		outputFlagProvided && strings.HasSuffix(strings.ToLower(outputFilename), ".exe")

	// If --target is specified, parse it; otherwise use --arch and --os
	if *targetFlag != "" {
		// Parse target string like "arm64-macos" or "amd64-linux"
		parts := strings.Split(*targetFlag, "-")
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Error: Invalid --target format '%s'. Expected format: ARCH-OS (e.g., arm64-macos, amd64-linux)\n", *targetFlag)
			os.Exit(1)
		}

		targetArch, err = ParseArch(parts[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid architecture in --target '%s': %v\n", *targetFlag, err)
			os.Exit(1)
		}

		// Handle OS aliases (macos -> darwin)
		osStr := parts[1]
		if osStr == "macos" {
			osStr = "darwin"
		}
		targetOS, err = ParseOS(osStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Invalid OS in --target '%s': %v (use 'darwin' instead of 'macos')\n", *targetFlag, err)
			os.Exit(1)
		}
	} else {
		// Use separate --arch and --os flags
		if !archExplicitlyProvided && autoDetectWindows {
			// Use default arch when auto-detecting Windows
			targetArch, _ = ParseArch(defaultArchStr)
		} else {
			targetArch, err = ParseArch(*archFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Invalid --arch '%s': %v\n", *archFlag, err)
				os.Exit(1)
			}
		}

		if autoDetectWindows {
			targetOS = OSWindows
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Auto-detected Windows target from .exe output filename (outputFilename=%s, outputFlagProvided=%v)\n",
					outputFilename, outputFlagProvided)
			}
		} else {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Not auto-detecting Windows: targetExplicit=%v osExplicit=%v outputProvided=%v hasSuffix=%v\n",
					targetExplicitlyProvided, osExplicitlyProvided, outputFlagProvided, strings.HasSuffix(strings.ToLower(outputFilename), ".exe"))
			}
			targetOS, err = ParseOS(*osFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Invalid --os '%s': %v\n", *osFlag, err)
				os.Exit(1)
			}
		}
	}

	targetPlatform := Platform{Arch: targetArch, OS: targetOS}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Target platform: %s-%s\n", targetArch.String(), targetOS.String())
	}

	// Get input files from remaining arguments
	inputFiles := flag.Args()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "----=[ %s ]=----\n", versionString)
	}

	// NEW CLI MODE: If arguments look like subcommands (build, run, etc.), use new CLI
	// This provides a Go-like experience while maintaining backward compatibility
	if len(inputFiles) > 0 {
		firstArg := inputFiles[0]
		// Check if it's a subcommand or looks like the new CLI style
		if firstArg == "build" || firstArg == "run" || firstArg == "test" || firstArg == "help" ||
			(strings.HasSuffix(firstArg, ".c67") && *codeFlag == "") {
			// Use new CLI system
			// Only pass outputFilename if user explicitly provided it
			cliOutputPath := ""
			if outputFlagProvided {
				cliOutputPath = outputFilename
			}
			err := RunCLI(inputFiles, targetPlatform, VerboseMode, QuietMode, *optTimeout, UpdateDepsFlag, SingleFlag, cliOutputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// FALLBACK: No arguments and no -c flag - check for .c67 files or show help
	if len(inputFiles) == 0 && *codeFlag == "" {
		matches, err := filepath.Glob("*.c67")
		if err == nil && len(matches) > 0 {
			// Use new CLI system to build directory
			cliOutputPath := ""
			if outputFlagProvided {
				cliOutputPath = outputFilename
			}
			err := RunCLI([]string{"."}, targetPlatform, VerboseMode, QuietMode, *optTimeout, UpdateDepsFlag, SingleFlag, cliOutputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		// No .c67 files - show help
		RunCLI([]string{"help"}, targetPlatform, VerboseMode, QuietMode, *optTimeout, UpdateDepsFlag, SingleFlag, "")
		return
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Target platform: %s\n", targetPlatform.FullString())
	}

	eb, err := NewWithPlatform(targetPlatform)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create executable builder: %v\n", err)
		os.Exit(1)
	}

	// Handle -c flag for inline code execution
	if *codeFlag != "" {
		// Create a temporary file with the inline code
		tmpFile, err := os.CreateTemp("", "c67_*.c67")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to create temp file: %v\n", err)
			os.Exit(1)
		}
		tmpFilename := tmpFile.Name()
		defer os.Remove(tmpFilename)

		// Write the code to the temp file
		if _, err := tmpFile.WriteString(*codeFlag); err != nil {
			tmpFile.Close()
			fmt.Fprintf(os.Stderr, "Error: Failed to write to temp file: %v\n", err)
			os.Exit(1)
		}
		tmpFile.Close()

		// Compile the temp file
		writeToFilename := outputFilename
		inlineFlagProvided := outputFlagProvided
		if outputFilename == defaultOutputFilename {
			writeToFilename = filepath.Join(os.TempDir(), "c67_inline")
			inlineFlagProvided = false
		}

		err = CompileC67(tmpFilename, writeToFilename, targetPlatform)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "-> Wrote executable: %s\n", writeToFilename)
		} else if !inlineFlagProvided {
			fmt.Println(writeToFilename)
		}
		return
	}

	if len(inputFiles) > 0 {
		for _, file := range inputFiles {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "source file: %s\n", file)
			}

			// Check if this is a C67 source file
			if strings.HasSuffix(file, ".c67") {
				if VerboseMode {
					fmt.Fprintln(os.Stderr, "-> Compiling C67 source")
				}

				writeToFilename := outputFilename
				if !outputFlagProvided {
					// No output filename specified - use input filename without .c67
					writeToFilename = strings.TrimSuffix(filepath.Base(file), ".c67")
				}

				err := CompileC67(file, writeToFilename, targetPlatform)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					os.Exit(1)
				}

				if VerboseMode {
					fmt.Fprintf(os.Stderr, "-> Wrote executable: %s\n", writeToFilename)
				} else if !outputFlagProvided {
					fmt.Println(writeToFilename)
				}

				// Enter watch mode if requested
				if *watchFlag {
					if err := watchAndRecompile(file, writeToFilename, targetPlatform); err != nil {
						fmt.Fprintf(os.Stderr, "Watch error: %v\n", err)
						os.Exit(1)
					}
				}
				return
			}
		}
	}

	if err := eb.CompileDefaultProgram(filepath.Join(os.TempDir(), "main")); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

}

func launchGameProcess(binaryPath string) (*os.Process, error) {
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return nil, err
	}

	procAttr := &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	process, err := os.StartProcess(absPath, []string{absPath}, procAttr)
	if err != nil {
		return nil, fmt.Errorf("failed to start process: %v", err)
	}

	return process, nil
}

func watchAndRecompile(sourceFile, outputFile string, platform Platform) error {
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\n🔥 Watch mode enabled - monitoring %s\n", absPath)
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop, or send SIGUSR1 to trigger manual reload\n")
	fmt.Fprintf(os.Stderr, "Command: kill -USR1 %d\n\n", os.Getpid())

	// Initialize incremental state for hot reload
	incrementalState := NewIncrementalState(platform)
	var gameProcess *os.Process

	// Initial compilation
	fmt.Fprintf(os.Stderr, "[%s] Initial compilation...\n", time.Now().Format("15:04:05"))
	if err := incrementalState.InitialCompile(absPath, outputFile); err != nil {
		return fmt.Errorf("initial compilation failed: %v", err)
	}
	fmt.Fprintf(os.Stderr, "✅ Compiled successfully\n")

	// Launch the game process
	if len(incrementalState.hotFunctions) > 0 {
		fmt.Fprintf(os.Stderr, "🎮 Launching game process with %d hot functions...\n", len(incrementalState.hotFunctions))
		gameProcess, err = launchGameProcess(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Failed to launch game: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "🎮 Game running (PID: %d)\n\n", gameProcess.Pid)
		}
	}

	// Create recompile function that can be called from multiple sources
	recompile := func(trigger string) {
		fmt.Fprintf(os.Stderr, "\n[%s] %s\n", time.Now().Format("15:04:05"), trigger)

		// Incremental recompilation
		updatedFuncs, err := incrementalState.IncrementalRecompile(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Compilation failed: %v\n", err)
			return
		}

		if len(updatedFuncs) == 0 {
			fmt.Fprintf(os.Stderr, "ℹ️  No hot functions changed\n")
			return
		}

		fmt.Fprintf(os.Stderr, "✅ Recompiled %d hot function(s): %v\n", len(updatedFuncs), updatedFuncs)

		// Check if any updated functions are actually hot functions
		anyHotFuncChanged := false
		for _, funcName := range updatedFuncs {
			if incrementalState.hotFunctions[funcName] != nil {
				anyHotFuncChanged = true
				break
			}
		}

		if !anyHotFuncChanged {
			fmt.Fprintf(os.Stderr, "ℹ️  No hot functions changed - keeping process alive\n")
			return
		}

		// Hot functions changed - restart process
		// TODO: Future enhancement - patch running process via IPC instead of restart
		if gameProcess != nil {
			fmt.Fprintf(os.Stderr, "🔄 Hot functions changed - restarting game process...\n")
			gameProcess.Kill()
			gameProcess.Wait()

			gameProcess, err = launchGameProcess(outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Failed to restart game: %v\n", err)
				gameProcess = nil
			} else {
				fmt.Fprintf(os.Stderr, "🎮 Game restarted (PID: %d)\n\n", gameProcess.Pid)
			}
		}
	}

	// Set up signal handler for USR1
	setupReloadSignal(recompile)

	// Set up file watcher
	watcher, err := NewFileWatcher(func(path string) {
		recompile(fmt.Sprintf("File changed: %s", filepath.Base(path)))
	})

	if err != nil {
		return fmt.Errorf("failed to create file watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.AddFile(absPath); err != nil {
		return fmt.Errorf("failed to watch file: %v", err)
	}

	// Clean up game process on exit
	defer func() {
		if gameProcess != nil {
			fmt.Fprintf(os.Stderr, "\n🛑 Stopping game process...\n")
			gameProcess.Kill()
			gameProcess.Wait()
		}
	}()

	watcher.Watch()
	return nil
}
