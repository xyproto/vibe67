// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
)

func (eb *ExecutableBuilder) CompileDefaultProgram(outputFile string) error {
	eb.Define("hello", "Hello, World!\n\x00")
	// Enable dynamic linking for glibc
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"printf", "exit"}
	if eb.useDynamicLinking && len(eb.neededFunctions) > 0 {
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> .rodata")
		}
		rodataSymbols := eb.RodataSection()
		estimatedRodataAddr := baseAddr + uint64(0x3000+0x100) // baseAddr + typical rodata offset
		currentAddr := estimatedRodataAddr
		for symbol, value := range rodataSymbols {
			eb.WriteRodata([]byte(value))
			eb.DefineAddr(symbol, currentAddr)
			currentAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "%s = %q at ~0x%x (estimated)\n", symbol, value, eb.consts[symbol].addr)
			}
		}
		// Generate text with estimated BSS addresses
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> .text")
		}
		err := eb.GenerateGlibcHelloWorld()
		if err != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Error generating glibc hello world: %v\n", err)
			}
			// Fallback to syscalls
			eb.SysWrite("hello")
			eb.SysExit()
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> ELF generation")
		}
		// Set up complete dynamic sections
		ds := NewDynamicSections(ArchX86_64)
		ds.AddNeeded("libc.so.6")
		// Add symbols
		for _, funcName := range eb.neededFunctions {
			ds.AddSymbol(funcName, STB_GLOBAL, STT_FUNC)
		}
		gotBase, rodataBaseAddr, textAddr, pltBase, err := eb.WriteCompleteDynamicELF(ds, eb.neededFunctions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to write dynamic ELF: %v\n", err)
			os.Exit(1)
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> .rodata (final addresses) and regenerating code")
		}
		currentAddr = rodataBaseAddr
		for symbol, value := range rodataSymbols {
			eb.DefineAddr(symbol, currentAddr)
			currentAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "%s = %q at 0x%x\n", symbol, value, eb.consts[symbol].addr)
			}
		}
		eb.text.Reset()
		err = eb.GenerateGlibcHelloWorld()
		if err != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Error regenerating code: %v\n", err)
			}
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> Patching PLT calls in regenerated code")
		}
		eb.patchPLTCalls(ds, textAddr, pltBase, eb.neededFunctions)
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> Patching RIP-relative relocations in regenerated code")
		}
		rodataSize := eb.rodata.Len()
		eb.PatchPCRelocations(textAddr, rodataBaseAddr, rodataSize)
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> Updating ELF with regenerated code")
		}
		eb.patchTextInELF()
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Final GOT base: 0x%x\n", gotBase)
		}
	} else {
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> .rodata")
		}
		rodataSymbols := eb.RodataSection()
		rodataAddr := baseAddr + headerSize
		currentAddr := uint64(rodataAddr)
		for symbol, value := range rodataSymbols {
			eb.DefineAddr(symbol, currentAddr)
			currentAddr += eb.WriteRodata([]byte(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "%s = %q\n", symbol, value)
			}
		}
		// Handle .data section (writable data like closure objects)
		dataSymbols := eb.DataSection()
		if len(dataSymbols) > 0 {
			if VerboseMode {
				fmt.Fprintln(os.Stderr, "-> .data")
			}
			dataAddr := currentAddr // Comes right after .rodata
			for symbol, value := range dataSymbols {
				eb.DefineAddr(symbol, dataAddr)
				dataAddr += uint64(len(value))
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "%s = %q at 0x%x\n", symbol, value, dataAddr-uint64(len(value)))
				}
			}
			_ = dataAddr // Update for next section (reserved for future use)
		}
		if VerboseMode {
			fmt.Fprintln(os.Stderr, "-> .text")
		}
		if err := eb.GenerateGlibcHelloWorld(); err != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Error generating glibc hello world: %v\n", err)
			}
			eb.SysWrite("hello")
			eb.SysExit()
		}
		// Only write ELF headers for non-Mach-O platforms
		if !eb.target.IsMachO() {
			if len(eb.dynlinker.Libraries) > 0 {
				eb.WriteDynamicELF()
			} else {
				eb.WriteELFHeader()
			}
		}
	}

	// Get the executable bytes
	executableData := eb.Bytes()

	// TODO: Re-enable compression once decompressor stub is fully debugged
	// if !eb.target.IsMachO() && eb.target.OS() != OSWindows {
	// 	archStr := "amd64"
	// 	if eb.target.Arch() == ArchARM64 {
	// 		archStr = "arm64"
	// 	}
	// 	compressed, err := WrapWithDecompressor(executableData, archStr)
	// 	if err == nil && len(compressed) < len(executableData) {
	// 		executableData = compressed
	// 	}
	// }

	// Output the executable file
	if err := os.WriteFile(outputFile, executableData, 0o755); err != nil {
		return err
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Wrote %s\n", outputFile)
	}
	return nil
}
