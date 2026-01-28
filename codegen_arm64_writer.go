// Completion: 95% - Writer module for ARM64 ELF
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// codegen_arm64_writer.go - ELF executable generation for ARM64 Linux
//
// This file handles the generation of ELF executables for Linux
// on ARM64 architecture.

func (fc *C67Compiler) writeELFARM64(outputPath string) error {
	// Enable dynamic linking for ARM64 ELF
	fc.eb.useDynamicLinking = true

	textBytes := fc.eb.text.Bytes()
	rodataBytes := fc.eb.rodata.Bytes()

	// Build pltFunctions list from all called functions
	pltFunctions := []string{"printf", "exit", "malloc", "free", "realloc", "strlen", "pow", "fflush"}

	// Add all functions from usedFunctions
	pltSet := make(map[string]bool)
	for _, f := range pltFunctions {
		pltSet[f] = true
	}

	// Add functions from eb.neededFunctions (populated by ARM64 codegen)
	for _, funcName := range fc.eb.neededFunctions {
		if !pltSet[funcName] {
			pltFunctions = append(pltFunctions, funcName)
			pltSet[funcName] = true
		}
	}

	// Build set of lambda function names to exclude from PLT
	lambdaSet := make(map[string]bool)
	for _, lambda := range fc.lambdaFuncs {
		lambdaSet[lambda.Name] = true
	}

	for funcName := range fc.usedFunctions {
		// Skip lambda functions - they are internal, not external PLT functions
		if lambdaSet[funcName] {
			continue
		}
		// Skip internal runtime functions
		if strings.HasPrefix(funcName, "_vibe67") || strings.HasPrefix(funcName, "vibe67_") {
			continue
		}
		if !pltSet[funcName] {
			pltFunctions = append(pltFunctions, funcName)
			pltSet[funcName] = true
		}
	}

	// Set up dynamic sections
	ds := NewDynamicSections(fc.eb.target.Arch())
	fc.dynamicSymbols = ds

	// Add NEEDED libraries
	ds.AddNeeded("libc.so.6")

	// Check if pthread functions are used
	if fc.usedFunctions["pthread_create"] || fc.usedFunctions["pthread_join"] {
		ds.AddNeeded("libpthread.so.0")
	}

	// Check if any libm functions are called
	libmFunctions := map[string]bool{
		"sqrt": true, "sin": true, "cos": true, "tan": true,
		"asin": true, "acos": true, "atan": true, "atan2": true,
		"sinh": true, "cosh": true, "tanh": true,
		"log": true, "log10": true, "exp": true, "pow": true,
		"fabs": true, "fmod": true, "ceil": true, "floor": true,
	}
	needsLibm := false
	for funcName := range fc.usedFunctions {
		if libmFunctions[funcName] {
			needsLibm = true
			break
		}
	}
	if needsLibm {
		ds.AddNeeded("libm.so.6")
	}

	// Add C library dependencies from imports
	for libName := range fc.cLibHandles {
		if libName != "linked" && libName != "c" {
			ds.AddNeeded(libName)
		}
	}

	// Add symbols to dynamic sections (STB_GLOBAL = 1, STT_FUNC = 2)
	for _, funcName := range pltFunctions {
		ds.AddSymbol(funcName, 1, 2) // STB_GLOBAL, STT_FUNC
	}

	// Add symbols for lambda functions
	for _, lambda := range fc.lambdaFuncs {
		ds.AddSymbol(lambda.Name, 1, 2) // STB_GLOBAL, STT_FUNC
	}

	// Prepare rodata section (strings, constants) before writing ELF
	// This is crucial - WriteCompleteDynamicELF expects rodata to be in eb.rodata buffer
	rodataSymbols := fc.eb.RodataSection()

	// Create sorted list for deterministic ordering
	var symbolNames []string
	for name := range rodataSymbols {
		symbolNames = append(symbolNames, name)
	}
	sort.Strings(symbolNames)

	// Clear rodata buffer and write sorted symbols
	fc.eb.rodata.Reset()
	estimatedRodataAddr := baseAddr + uint64(0x3000+0x100) // baseAddr + typical rodata offset
	currentAddr := estimatedRodataAddr
	for _, symbol := range symbolNames {
		value := rodataSymbols[symbol]

		// Align string literals to 8-byte boundaries
		if strings.HasPrefix(symbol, "str_") {
			padding := (8 - (currentAddr % 8)) % 8
			if padding > 0 {
				fc.eb.WriteRodata(make([]byte, padding))
				currentAddr += padding
			}
		}

		fc.eb.WriteRodata([]byte(value))
		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Prepared rodata symbol %s at estimated 0x%x (%d bytes): %q\n",
				symbol, currentAddr-uint64(len(value)), len(value), value)
		}
	}

	// Prepare .data section (writable data like itoa buffer)
	dataSymbols := fc.eb.DataSection()
	if len(dataSymbols) > 0 {
		fc.eb.data.Reset()
		dataBaseAddr := currentAddr // Follows .rodata
		dataSymbolNames := make([]string, 0, len(dataSymbols))
		for symbol := range dataSymbols {
			dataSymbolNames = append(dataSymbolNames, symbol)
		}
		sort.Strings(dataSymbolNames)

		for _, symbol := range dataSymbolNames {
			value := dataSymbols[symbol]
			fc.eb.WriteData([]byte(value))
			fc.eb.DefineAddr(symbol, dataBaseAddr)
			dataBaseAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Prepared .data symbol %s at estimated 0x%x (%d bytes)\n",
					symbol, fc.eb.consts[symbol].addr, len(value))
			}
		}
		_ = dataBaseAddr // Mark as used (needed for future logic)
	}

	// Write complete dynamic ELF with PLT/GOT
	// This already patches PC-relative relocations internally
	_, rodataBaseAddr, textAddr, pltBase, err := fc.eb.WriteCompleteDynamicELF(ds, pltFunctions)
	if err != nil {
		return fmt.Errorf("failed to write ARM64 ELF: %v", err)
	}

	// Update rodata addresses with actual addresses from ELF layout
	currentAddr = rodataBaseAddr
	for _, symbol := range symbolNames {
		value := rodataSymbols[symbol]

		// Apply same alignment as when writing
		if strings.HasPrefix(symbol, "str_") {
			padding := (8 - (currentAddr % 8)) % 8
			currentAddr += padding
		}

		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Updated rodata symbol %s to actual address 0x%x\n", symbol, fc.eb.consts[symbol].addr)
		}
	}

	// Update .data addresses with actual addresses from ELF layout
	dataSymbols = fc.eb.DataSection()
	if len(dataSymbols) > 0 {
		dataBaseAddr := currentAddr // Follows rodata
		for symbol, value := range dataSymbols {
			fc.eb.DefineAddr(symbol, dataBaseAddr)
			dataBaseAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Updated .data symbol %s to actual address 0x%x\n",
					symbol, fc.eb.consts[symbol].addr)
			}
		}
	}

	// Re-patch PC relocations with correct rodata/data addresses
	// WriteCompleteDynamicELF patched them with estimated addresses, but now we have actual addresses
	rodataSize := fc.eb.rodata.Len()
	fc.eb.PatchPCRelocations(textAddr, rodataBaseAddr, rodataSize)

	// Patch PLT calls in the generated code (similar to x86_64 path)
	fc.eb.patchPLTCalls(ds, textAddr, pltBase, pltFunctions)

	// Update ELF with patched text
	fc.eb.patchTextInELF()

	// Write the final executable to file
	elfBytes := fc.eb.Bytes()
	if err := os.WriteFile(outputPath, elfBytes, 0755); err != nil {
		return fmt.Errorf("failed to write executable: %v", err)
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Wrote ARM64 dynamic ELF executable: %s\n", outputPath)
		fmt.Fprintf(os.Stderr, "   Text size: %d bytes\n", len(textBytes))
		fmt.Fprintf(os.Stderr, "   Rodata size: %d bytes\n", len(rodataBytes))
		fmt.Fprintf(os.Stderr, "   PLT functions: %d\n", len(pltFunctions))
	}

	return nil
}
