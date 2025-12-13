// Completion: 100% - Writer module complete
package main

import (
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
		funcName := patch.targetName
		if strings.HasSuffix(funcName, "$stub") {
			funcName = funcName[:len(funcName)-5] // Remove "$stub"
		}

		// Skip internal C67 runtime functions (they're defined in the binary)
		if strings.HasPrefix(funcName, "_c67_") || strings.HasPrefix(funcName, "c67_") {
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
	// This matches the calculation in macho.go
	headerSize := uint32(32) // MachHeader64 size
	// Estimate load commands size (this is a rough estimate, macho.go has the exact calculation)
	// LC_SEGMENT_64 (TEXT), LC_SEGMENT_64 (DATA), LC_SYMTAB, LC_DYSYMTAB, LC_LOAD_DYLIB
	estimatedLoadCmdsSize := uint32(1000) // Conservative estimate
	fileHeaderSize := uint64(headerSize + estimatedLoadCmdsSize)
	textSectAddr := baseAddr + fileHeaderSize

	textSize := uint64(fc.eb.text.Len())
	textSizeAligned := (textSize + pageSize - 1) &^ (pageSize - 1)

	// Calculate rodata address (comes after __TEXT segment)
	rodataAddr := baseAddr + pageSize + textSizeAligned

	if VerboseMode {
		fmt.Fprintln(os.Stderr, "-> Writing rodata symbols")
	}

	// Get all rodata symbols and write them
	rodataSymbols := fc.eb.RodataSection()
	currentAddr := rodataAddr
	for symbol, value := range rodataSymbols {
		fc.eb.WriteRodata([]byte(value))
		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "   %s at 0x%x (%d bytes)\n", symbol, fc.eb.consts[symbol].addr, len(value))
		}
	}

	rodataSize := fc.eb.rodata.Len()

	// Now write all writable data symbols to the data buffer and assign addresses
	// Data comes after rodata
	rodataSizeAligned := uint64((uint64(rodataSize) + pageSize - 1) &^ (pageSize - 1))
	dataAddr := rodataAddr + rodataSizeAligned

	if VerboseMode {
		fmt.Fprintln(os.Stderr, "-> Writing data symbols")
	}

	// Get all writable data symbols and write them
	dataSymbols := fc.eb.DataSection()
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

	// Set lambda function addresses from labels
	for labelName, offset := range fc.eb.labels {
		if strings.HasPrefix(labelName, "lambda_") {
			lambdaAddr := textSectAddr + uint64(offset)
			fc.eb.DefineAddr(labelName, lambdaAddr)
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Setting %s address to 0x%x (offset %d)\n", labelName, lambdaAddr, offset)
			}
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
