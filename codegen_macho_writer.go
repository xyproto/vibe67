// Completion: 100% - Writer module complete
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// codegen_macho_writer.go - Mach-O executable generation for ARM64 macOS
//
// This file handles the generation of Mach-O executables for macOS
// on ARM64 (Apple Silicon) architecture.

// resolveDylibPath resolves a library name to its dylib path on macOS
func (fc *C67Compiler) resolveDylibPath(libName string) string {
	// Common library path mappings for macOS
	switch strings.ToLower(libName) {
	case "sdl3":
		// Try common SDL3 installation paths
		paths := []string{
			"/opt/homebrew/lib/libSDL3.dylib",
			"/usr/local/lib/libSDL3.dylib",
			"/Library/Frameworks/SDL3.framework/SDL3",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		// If not found, use the first path as default (will fail at runtime if missing)
		return paths[0]
	case "sdl2":
		paths := []string{
			"/opt/homebrew/lib/libSDL2.dylib",
			"/usr/local/lib/libSDL2.dylib",
			"/Library/Frameworks/SDL2.framework/SDL2",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return paths[0]
	default:
		// For other libraries, try standard patterns
		// First try Homebrew
		homebrewPath := fmt.Sprintf("/opt/homebrew/lib/lib%s.dylib", libName)
		if _, err := os.Stat(homebrewPath); err == nil {
			return homebrewPath
		}
		// Then try /usr/local
		usrLocalPath := fmt.Sprintf("/usr/local/lib/lib%s.dylib", libName)
		if _, err := os.Stat(usrLocalPath); err == nil {
			return usrLocalPath
		}
		// Default to libSystem for unknown libraries
		return "/usr/lib/libSystem.B.dylib"
	}
}

// Confidence that this function is working: 60%
func (fc *C67Compiler) writeMachOARM64(outputPath string) error {
	// Build neededFunctions list from call patches (actual function calls made)
	// Extract unique function names from callPatches
	neededSet := make(map[string]bool)
	for _, patch := range fc.eb.callPatches {
		// patch.targetName is like "malloc$stub" or "printf$stub"
		// Strip the "$stub" suffix to get the function name
		funcName := strings.TrimSuffix(patch.targetName, "$stub")

		// Skip internal Vibe67 runtime functions (they're defined in the binary)
		if strings.HasPrefix(funcName, "_vibe67_") || strings.HasPrefix(funcName, "vibe67_") || strings.HasPrefix(funcName, "__") {
			continue
		}

		neededSet[funcName] = true
	}

	// Convert set to slice
	neededFuncs := make([]string, 0, len(neededSet))
	for funcName := range neededSet {
		neededFuncs = append(neededFuncs, funcName)
	}

	// Assign to executable builder for Mach-O generation
	fc.eb.neededFunctions = neededFuncs
	if len(neededFuncs) > 0 {
		fc.eb.useDynamicLinking = true
	}

	// Build function-to-library mapping for multi-library support
	fc.eb.functionLibraries = make(map[string]string)
	for _, funcName := range neededFuncs {
		// Check if we have library mapping from C FFI
		if libName, ok := fc.cFunctionLibs[funcName]; ok {
			// Map library name to dylib path
			dylibPath := fc.resolveDylibPath(libName)
			fc.eb.functionLibraries[funcName] = dylibPath
		} else {
			// Default to libSystem for standard functions (malloc, printf, etc.)
			fc.eb.functionLibraries[funcName] = "/usr/lib/libSystem.B.dylib"
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> ARM64 neededFunctions: %v\n", neededFuncs)
	}

	// First, write all rodata symbols to the rodata buffer and assign addresses
	pageSize := uint64(0x4000)      // 16KB page size for ARM64
	baseAddr := uint64(0x100000000) // macOS base address (4GB zero page)

	// Calculate text section address (after Mach-O headers)
	// This must match the calculation in macho.go
	headerSize := uint32(32) // MachHeader64 size

	// Calculate preliminary load commands size (matching macho.go logic exactly)
	// Check if there are any rodata symbols to be written
	rodataSymbols := fc.eb.RodataSection()
	dataSymbols := fc.eb.DataSection()
	hasRodata := len(rodataSymbols) > 0 || len(dataSymbols) > 0
	rodataSize := 0
	if hasRodata {
		rodataSize = 1 // Placeholder value >  0 to indicate rodata/data exists
	}
	numImports := uint32(len(neededFuncs))
	loadCmdsSize := uint32(0)

	// __PAGEZERO segment
	loadCmdsSize += uint32(binary.Size(SegmentCommand64{}))

	// __TEXT segment with sections
	textNSects := uint32(1) // __text
	if fc.eb.useDynamicLinking && numImports > 0 {
		textNSects++ // __stubs
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: textNSects=%d, useDynamicLinking=%v, numImports=%d\n", textNSects, fc.eb.useDynamicLinking, numImports)
	}
	loadCmdsSize += uint32(binary.Size(SegmentCommand64{}) + int(textNSects)*binary.Size(Section64{}))

	// __DATA segment with sections (if needed)
	if rodataSize > 0 || (fc.eb.useDynamicLinking && numImports > 0) {
		dataNSects := uint32(0)
		if rodataSize > 0 {
			dataNSects++
		}
		if fc.eb.useDynamicLinking && numImports > 0 {
			dataNSects++ // __got
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: dataNSects=%d, rodataSize=%d\n", dataNSects, rodataSize)
		}
		loadCmdsSize += uint32(binary.Size(SegmentCommand64{}) + int(dataNSects)*binary.Size(Section64{}))
	}

	// __LINKEDIT segment
	loadCmdsSize += uint32(binary.Size(SegmentCommand64{}))

	// LC_LOAD_DYLINKER
	dylinkerPath := "/usr/lib/dyld\x00"
	dylinkerCmdSize := (uint32(binary.Size(LoadCommand{})+4+len(dylinkerPath)) + 7) &^ 7
	loadCmdsSize += dylinkerCmdSize

	// LC_UUID
	loadCmdsSize += uint32(binary.Size(UUIDCommand{}))

	// LC_BUILD_VERSION
	loadCmdsSize += uint32(binary.Size(BuildVersionCommand{}))

	// LC_MAIN
	loadCmdsSize += uint32(binary.Size(EntryPointCommand{}))

	// LC_SYMTAB
	loadCmdsSize += uint32(binary.Size(SymtabCommand{}))

	// LC_CODE_SIGNATURE
	loadCmdsSize += uint32(binary.Size(LinkEditDataCommand{}))

	// LC_LOAD_DYLIB (default to libSystem)
	dylibPath := "/usr/lib/libSystem.B.dylib\x00"
	dylibCmdSize := (uint32(binary.Size(LoadCommand{})+16+len(dylibPath)) + 7) &^ 7
	loadCmdsSize += dylibCmdSize

	// LC_DYSYMTAB
	loadCmdsSize += uint32(binary.Size(DysymtabCommand{}))

	fileHeaderSize := uint64(headerSize + loadCmdsSize)
	textSectAddr := baseAddr + fileHeaderSize

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG codegen_macho_writer: headerSize=%d, loadCmdsSize=%d, fileHeaderSize=%d, textSectAddr=0x%x\n",
			headerSize, loadCmdsSize, fileHeaderSize, textSectAddr)
	}

	textSize := uint64(fc.eb.text.Len())

	// Calculate stubs size (12 bytes per stub on ARM64, if dynamic linking)
	stubsSize := uint64(0)
	if fc.eb.useDynamicLinking && numImports > 0 {
		stubsSize = uint64(numImports * 12)
	}

	// Calculate __TEXT segment size (must match WriteMachO logic)
	// textSegFileSize = fileHeaderSize + textSize + stubsSize
	textSegFileSize := fileHeaderSize + textSize + stubsSize
	textSegVMSize := (textSegFileSize + pageSize - 1) &^ (pageSize - 1)

	// Calculate rodata address (comes after __TEXT segment)
	// This MUST match WriteMachO's calculation: rodataAddr = textAddr + textSegVMSize
	rodataAddr := baseAddr + textSegVMSize

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: textSegFileSize=0x%x, textSegVMSize=0x%x, rodataAddr=0x%x\n",
			textSegFileSize, textSegVMSize, rodataAddr)
		fmt.Fprintln(os.Stderr, "-> Writing rodata symbols")
	}

	// Get all rodata symbols and write them
	rodataSymbols = fc.eb.RodataSection()
	currentAddr := rodataAddr
	for symbol, value := range rodataSymbols {
		fc.eb.WriteRodata([]byte(value))
		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "   %s at 0x%x (%d bytes)\n", symbol, fc.eb.consts[symbol].addr, len(value))
		}
	}

	rodataSize = fc.eb.rodata.Len()

	// Now write all writable data symbols to the data buffer and assign addresses
	// Data comes after rodata (NOT page-aligned - it's in the same __DATA segment)
	// The __DATA segment file layout is: rodata + writable_data + padding + got
	// So writable data VM address = rodataAddr + rodataSize (no page alignment)
	dataAddr := rodataAddr + uint64(rodataSize)

	if VerboseMode {
		fmt.Fprintln(os.Stderr, "-> Writing data symbols")
	}

	// Get all writable data symbols and write them
	dataSymbols = fc.eb.DataSection()
	currentAddr = dataAddr
	for symbol, value := range dataSymbols {
		fc.eb.WriteData([]byte(value))
		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "   %s at 0x%x (%d bytes)\n", symbol, fc.eb.consts[symbol].addr, len(value))
		}
	}

	dataSize := fc.eb.data.Len()

	// Set addresses for all labels (lambdas and runtime helpers)
	for labelName, offset := range fc.eb.labels {
		labelAddr := textSectAddr + uint64(offset)
		fc.eb.DefineAddr(labelName, labelAddr)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Setting label %s address to 0x%x (offset %d)\n", labelName, labelAddr, offset)
		}
	}

	// Set stub addresses for external functions
	stubsAddr := textSectAddr + uint64(fc.eb.text.Len())
	for i, funcName := range fc.eb.neededFunctions {
		stubName := funcName + "$stub"
		stubAddr := stubsAddr + uint64(i*12)
		fc.eb.DefineAddr(stubName, stubAddr)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Setting stub %s address to 0x%x\n", stubName, stubAddr)
		}
	}

	// Now patch PC-relative relocations in the text
	if VerboseMode && len(fc.eb.pcRelocations) > 0 {
		fmt.Fprintf(os.Stderr, "-> Patching %d PC-relative relocations\n", len(fc.eb.pcRelocations))
	}
	// Note: PatchPCRelocations uses DefineAddr'd addresses from consts, so parameters matter less
	fc.eb.PatchPCRelocations(textSectAddr, rodataAddr, rodataSize)

	// Use the existing Mach-O writer infrastructure
	if err := fc.eb.WriteMachO(); err != nil {
		return fmt.Errorf("failed to write Mach-O: %v", err)
	}

	// Write the executable
	machoBytes := fc.eb.elf.Bytes()

	if err := os.WriteFile(outputPath, machoBytes, 0755); err != nil {
		return fmt.Errorf("failed to write executable: %v", err)
	}

	cmd := exec.Command("ldid", "-S", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: ldid signing failed: %v\n%s\n", err, output)
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Wrote ARM64 Mach-O executable: %s\n", outputPath)
		fmt.Fprintf(os.Stderr, "   Text size: %d bytes\n", fc.eb.text.Len())
		fmt.Fprintf(os.Stderr, "   Rodata size: %d bytes\n", rodataSize)
		fmt.Fprintf(os.Stderr, "   Data size: %d bytes\n", dataSize)
	}

	return nil
}

// writeELFRiscv64 writes a RISC-V64 ELF executable









