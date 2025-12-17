// Completion: 100% - Writer module complete
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// codegen_elf_writer.go - ELF executable generation for x86_64 Linux/Unix
//
// This file handles the generation of ELF (Executable and Linkable Format)
// executables for Linux/Unix systems on x86_64 architecture.

// Confidence that this function is working: 85%
func (fc *C67Compiler) writeELF(program *Program, outputPath string) error {
	// Enable dynamic linking for ELF (required for WriteCompleteDynamicELF)
	fc.eb.useDynamicLinking = true

	// First pass: Build initial PLT with main program functions
	// (will be rebuilt after runtime helpers are generated)
	pltFunctions := []string{}
	pltSet := make(map[string]bool)

	// Build set of lambda function names to exclude from PLT
	lambdaSet := make(map[string]bool)
	for _, lambda := range fc.lambdaFuncs {
		lambdaSet[lambda.Name] = true
	}

	// Add functions used so far (main program only, not runtime helpers yet)
	for funcName := range fc.usedFunctions {
		if lambdaSet[funcName] {
			continue
		}
		if strings.HasPrefix(funcName, "_c67") || strings.HasPrefix(funcName, "c67_") {
			continue
		}
		if !pltSet[funcName] {
			pltFunctions = append(pltFunctions, funcName)
			pltSet[funcName] = true
		}
	}

	// Note: Runtime helper functions will be tracked but won't be in first-pass PLT
	// This is OK - they'll be resolved as internal labels, not PLT entries

	// Set up dynamic sections
	ds := NewDynamicSections(fc.eb.target.Arch())
	fc.dynamicSymbols = ds

	// Add library dependencies based on functions used in main program
	libcFunctions := map[string]bool{
		"printf": true, "sprintf": true, "snprintf": true, "fprintf": true, "dprintf": true,
		"puts": true, "putchar": true, "fputc": true, "fputs": true, "fflush": true,
		"scanf": true, "sscanf": true, "fscanf": true,
		"fopen": true, "fclose": true, "fread": true, "fwrite": true, "fseek": true, "ftell": true,
		"malloc": true, "calloc": true, "realloc": true, "free": true,
		"memcpy": true, "memset": true, "memmove": true, "memcmp": true,
		"strlen": true, "strcpy": true, "strncpy": true, "strcmp": true, "strncmp": true,
		"strcat": true, "strncat": true, "strchr": true, "strrchr": true, "strstr": true,
		"exit": true, "abort": true, "atexit": true,
		"getenv": true, "setenv": true, "unsetenv": true,
		"time": true, "clock": true, "localtime": true, "gmtime": true,
		"dlopen": true, "dlsym": true, "dlclose": true, "dlerror": true,
	}
	needsLibc := false
	for funcName := range fc.usedFunctions {
		if libcFunctions[funcName] {
			needsLibc = true
			break
		}
	}
	if needsLibc {
		ds.AddNeeded("libc.so.6")
	}

	// Check if pthread functions are used
	if fc.usedFunctions["pthread_create"] || fc.usedFunctions["pthread_join"] {
		ds.AddNeeded("libpthread.so.0")
	}

	// Check if libm functions are used
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
		if libName != "linked" {
			if libName == "c" {
				continue
			}
			libSoName := libName
			if strings.Contains(libSoName, ".so") {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Adding custom C library dependency: %s\n", libSoName)
				}
				ds.AddNeeded(libSoName)
				continue
			}
			if !strings.Contains(libSoName, ".so") {
				cmd := exec.Command("pkg-config", "--libs-only-l", libName)
				if output, err := cmd.Output(); err == nil {
					libs := strings.TrimSpace(string(output))
					if strings.HasPrefix(libs, "-l") {
						libSoName = "lib" + strings.TrimPrefix(libs, "-l") + ".so"
					} else {
						if !strings.HasPrefix(libSoName, "lib") {
							libSoName = "lib" + libSoName
						}
						libSoName += ".so"
					}
				} else {
					if !strings.HasPrefix(libSoName, "lib") {
						libSoName = "lib" + libSoName
					}
					ldconfigCmd := exec.Command("ldconfig", "-p")
					if ldOutput, ldErr := ldconfigCmd.Output(); ldErr == nil {
						lines := strings.Split(string(ldOutput), "\n")
						for _, line := range lines {
							if strings.Contains(line, libSoName) && strings.Contains(line, "=>") {
								parts := strings.Split(line, "=>")
								if len(parts) == 2 {
									actualPath := strings.TrimSpace(parts[1])
									pathParts := strings.Split(actualPath, "/")
									if len(pathParts) > 0 {
										libSoName = pathParts[len(pathParts)-1]
									}
									break
								}
							}
						}
					}
					if !strings.Contains(libSoName, ".so") {
						libSoName += ".so"
					}
				}
			}
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Adding C library dependency: %s\n", libSoName)
			}
			ds.AddNeeded(libSoName)
		}
	}

	// Add PLT symbols
	for _, funcName := range pltFunctions {
		ds.AddSymbol(funcName, STB_GLOBAL, STT_FUNC)
	}

	// Add lambda symbols
	for _, lambda := range fc.lambdaFuncs {
		ds.AddSymbol(lambda.Name, STB_GLOBAL, STT_FUNC)
	}

	// Note: Library dependencies will be determined dynamically based on actual usage

	// Add cache pointer storage to rodata (8 bytes of zeros for each cache)
	if len(fc.memoCaches) > 0 {
		for cacheName := range fc.memoCaches {
			fc.eb.Define(cacheName, "\x00\x00\x00\x00\x00\x00\x00\x00")
		}
	}

	// Check if hot functions are used with WPO disabled
	if len(fc.hotFunctions) > 0 && fc.wpoTimeout == 0 {
		return fmt.Errorf("hot functions require whole-program optimization (do not use --opt-timeout=0)")
	}

	fc.buildHotFunctionTable()
	fc.generateHotFunctionTable()

	rodataSymbols := fc.eb.RodataSection()

	// Create sorted list of symbol names for deterministic ordering
	var symbolNames []string
	for name := range rodataSymbols {
		symbolNames = append(symbolNames, name)
	}
	sort.Strings(symbolNames)

	// DEBUG: Print what symbols we're writing

	// Clear rodata buffer before writing sorted symbols
	// (in case any data was written during code generation)
	fc.eb.rodata.Reset()

	estimatedRodataAddr := uint64(0x403000 + 0x100)
	currentAddr := estimatedRodataAddr
	for _, symbol := range symbolNames {
		value := rodataSymbols[symbol]

		// Align string literals to 8-byte boundaries for proper float64 access
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
	}
	if fc.eb.rodata.Len() > 0 {
		_ = fc.eb.rodata.Len() // Rodata exists but we don't preview it in normal mode
	}

	// Assign addresses to .data section symbols (writable data like closures)
	// Need to sort data symbols for consistent addresses
	dataSymbols := fc.eb.DataSection()
	if len(dataSymbols) > 0 {
		// Clear data buffer before writing sorted symbols
		fc.eb.data.Reset()

		dataBaseAddr := currentAddr // Follows .rodata
		dataSymbolNames := make([]string, 0, len(dataSymbols))
		for symbol := range dataSymbols {
			dataSymbolNames = append(dataSymbolNames, symbol)
		}
		sort.Strings(dataSymbolNames)

		for _, symbol := range dataSymbolNames {
			value := dataSymbols[symbol]
			// Write data to buffer first
			fc.eb.WriteData([]byte(value))
			// Then assign address
			fc.eb.DefineAddr(symbol, dataBaseAddr)
			dataBaseAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Wrote and assigned .data symbol %s to 0x%x (%d bytes)\n", symbol, fc.eb.consts[symbol].addr, len(value))
			}
		}
		_ = dataBaseAddr // Mark as used (needed for future logic)
	}

	// Write complete dynamic ELF with unique PLT functions
	// Note: We pass pltFunctions (unique) for building PLT/GOT structure
	// We'll use fc.callOrder (with duplicates) later for patching actual call sites
	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n=== First compilation callOrder: %v ===\n", fc.callOrder)
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "=== pltFunctions (unique): %v ===\n", pltFunctions)
		}
	}

	// Generate ELF file (static or dynamic based on needs)
	var gotBase, rodataBaseAddr, textAddr, pltBase uint64
	var err error
	
	if fc.runtimeFeatures.needsDynamicLinking() || len(pltFunctions) > 0 {
		// Dynamic ELF with PLT/GOT for external function calls
		if VerboseMode || fc.debug {
			fmt.Fprintf(os.Stderr, "Generating dynamic ELF with %d external functions\n", len(pltFunctions))
		}
		gotBase, rodataBaseAddr, textAddr, pltBase, err = fc.eb.WriteCompleteDynamicELF(ds, pltFunctions)
	} else {
		// Static ELF with no external dependencies
		if VerboseMode || fc.debug {
			fmt.Fprintf(os.Stderr, "Generating static ELF (no external functions)\n")
		}
		gotBase, rodataBaseAddr, textAddr, pltBase, err = fc.eb.WriteCompleteStaticELF(ds)
	}
	
	if err != nil {
		return err
	}

	// Update rodata addresses using same sorted order
	currentAddr = rodataBaseAddr
	for _, symbol := range symbolNames {
		value := rodataSymbols[symbol]

		// Apply same alignment as when writing rodata
		if strings.HasPrefix(symbol, "str_") {
			padding := (8 - (currentAddr % 8)) % 8
			currentAddr += padding
		}

		fc.eb.DefineAddr(symbol, currentAddr)
		currentAddr += uint64(len(value))
	}

	// Update .data addresses similarly (MUST use same sorted order as first pass!)
	dataSymbols = fc.eb.DataSection()
	if len(dataSymbols) > 0 {
		dataBaseAddr := currentAddr // Follows .rodata
		dataSymbolNames := make([]string, 0, len(dataSymbols))
		for symbol := range dataSymbols {
			dataSymbolNames = append(dataSymbolNames, symbol)
		}
		sort.Strings(dataSymbolNames)

		for _, symbol := range dataSymbolNames {
			value := dataSymbols[symbol]
			fc.eb.DefineAddr(symbol, dataBaseAddr)
			dataBaseAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Updated .data symbol %s to 0x%x\n", symbol, fc.eb.consts[symbol].addr)
			}
		}
		currentAddr = dataBaseAddr
	}

	// Regenerate code with correct addresses
	fc.eb.text.Reset()
	// DON'T reset rodata - it already has correct addresses from first pass
	// Resetting rodata causes all symbols to move, breaking PC-relative addressing
	fc.eb.pcRelocations = []PCRelocation{} // Reset PC relocations for recompilation
	fc.eb.callPatches = []CallPatch{}      // Reset call patches for recompilation
	fc.eb.labels = make(map[string]int)    // Reset labels for recompilation
	fc.callOrder = []string{}              // Clear call order for recompilation
	fc.stringCounter = 0                   // Reset string counter for recompilation
	fc.labelCounter = 0                    // Reset label counter for recompilation
	fc.lambdaCounter = 0                   // Reset lambda counter for recompilation
	// DON'T clear lambdaFuncs - we need them for second pass lambda generation
	fc.lambdaOffsets = make(map[string]int) // Reset lambda offsets
	fc.variables = make(map[string]int)     // Reset variables map
	fc.mutableVars = make(map[string]bool)  // Reset mutability tracking
	fc.stackOffset = 0                      // Reset stack offset
	// Set up stack frame
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.SubImmFromReg("rsp", StackSlotSize) // Align stack to 16 bytes
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.XorRegWithReg("rdi", "rdi")
	fc.out.XorRegWithReg("rsi", "rsi")

	// DON'T re-define rodata symbols - they already exist from first pass
	// Re-defining them would change their addresses and break PC-relative references

	// ===== AVX-512 CPU DETECTION (regenerated only if needed) =====
	if fc.runtimeFeatures.needsCPUDetection() {
		fc.out.MovImmToReg("rax", "7")              // CPUID leaf 7
		fc.out.XorRegWithReg("rcx", "rcx")          // subleaf 0
		fc.out.Emit([]byte{0x0f, 0xa2})             // cpuid
		fc.out.Emit([]byte{0xf6, 0xc3, 0x01})       // test bl, 1
		fc.out.Emit([]byte{0x0f, 0xba, 0xe3, 0x10}) // bt ebx, 16
		fc.out.Emit([]byte{0x0f, 0x92, 0xc0})       // setc al
		fc.out.LeaSymbolToReg("rbx", "cpu_has_avx512")
		fc.out.MovByteRegToMem("rax", "rbx", 0) // Write only AL, not full RAX
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.XorRegWithReg("rbx", "rbx")
		fc.out.XorRegWithReg("rcx", "rcx")
	}
	// ===== END AVX-512 DETECTION =====

	// Recompile with correct addresses
	// NOTE: Use the original program parameter (which includes imports),
	// not a reparsed version from source which would lose imported statements

	// Reset compiler state for second pass
	fc.variables = make(map[string]int)
	fc.mutableVars = make(map[string]bool)
	fc.varTypes = make(map[string]string)
	fc.stackOffset = 0
	fc.lambdaFuncs = nil // Clear lambda list so collectSymbols can repopulate it
	fc.lambdaCounter = 0
	fc.labelCounter = 0                                       // Reset label counter for consistent loop labels
	fc.movedVars = make(map[string]bool)                      // Reset moved variables tracking
	fc.scopedMoved = []map[string]bool{make(map[string]bool)} // Reset scoped tracking

	// Re-detect if main() is called at top level for second pass
	fc.mainCalledAtTopLevel = fc.detectMainCallInTopLevel(program.Statements)

	// Collect symbols again (two-pass compilation for second regeneration)
	for _, stmt := range program.Statements {
		if err := fc.collectSymbols(stmt); err != nil {
			return err
		}
	}

	// Reset labelCounter after collectSymbols so compilation uses same labels
	fc.labelCounter = 0

	// DON'T rebuild hot function table - it already exists in rodata from first pass
	// Rebuilding it would change its address and break PC-relative references

	fc.pushDeferScope()

	// Initialize arena system only if needed (malloc'd arenas at runtime)
	if fc.usesArenas {
		fc.initializeMetaArenaAndGlobalArena()
	}

	// Generate code with symbols collected
	for _, stmt := range program.Statements {
		fc.compileStatement(stmt)
	}

	fc.popDeferScope()

	// Jump over lambda functions to reach the main evaluation code
	skipLambdasJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Will be patched
	skipLambdasEnd := fc.eb.text.Len()

	// Generate lambda functions here (before exit, but jumped over)
	fc.generateLambdaFunctions()

	// Patch the jump to skip over lambdas
	skipLambdasTarget := fc.eb.text.Len()
	fc.patchJumpImmediate(skipLambdasJump+1, int32(skipLambdasTarget-skipLambdasEnd))

	// Evaluate main (if it exists) to get the exit code
	_, exists := fc.variables["main"]
	if exists {
		// main exists - check if it's a lambda/function or a direct value
		if fc.lambdaVars["main"] {
			// main is a lambda/function
			// Only auto-call if main() was NOT explicitly called at top level
			if !fc.mainCalledAtTopLevel {
				// Auto-call main with no arguments
				fc.compileExpression(&CallExpr{Function: "main", Args: []Expression{}})
			} else {
				// main() was already called - use exit code 0
				fc.out.XorRegWithReg("xmm0", "xmm0")
			}
		} else {
			// main is a direct value - just load it
			fc.compileExpression(&IdentExpr{Name: "main"})
		}
		// Result is in xmm0 (float64)
		// Convert float64 result in xmm0 to int32 in rdi (for exit code)
		fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xf8})
	} else {
		// No main - use exit code 0
		fc.out.XorRegWithReg("rdi", "rdi")
	}

	// Always add implicit exit at the end of the program
	// Use syscall exit on Linux (no libc dependency for syscall-based printf)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Using syscall exit (no libc)\n")
	}
	fc.out.MovImmToReg("rax", "60") // syscall number for exit
	// exit code is already in rdi (first syscall argument)
	fc.eb.Emit("syscall") // invoke syscall directly

	// Generate pattern lambda functions
	fc.generatePatternLambdaFunctions()

	// Generate runtime helper functions AFTER lambda generation
	fc.generateRuntimeHelpers()

	// Collect rodata symbols again (lambda/runtime functions may have created new ones)
	rodataSymbols = fc.eb.RodataSection()

	// Find any NEW symbols that weren't in the original list
	var newSymbols []string
	for symbol := range rodataSymbols {
		found := false
		for _, existingSym := range symbolNames {
			if symbol == existingSym {
				found = true
				break
			}
		}
		if !found {
			newSymbols = append(newSymbols, symbol)
		}
	}

	if len(newSymbols) > 0 {
		sort.Strings(newSymbols)

		// Append new symbols to rodata and assign addresses
		for _, symbol := range newSymbols {
			value := rodataSymbols[symbol]
			fc.eb.WriteRodata([]byte(value))
			fc.eb.DefineAddr(symbol, currentAddr)
			currentAddr += uint64(len(value))
			symbolNames = append(symbolNames, symbol)
		}
	}

	// Handle new .data symbols similarly
	dataSymbols = fc.eb.DataSection()
	newDataSymbols := []string{}
	for symbol := range dataSymbols {
		// Check if already assigned
		if _, ok := fc.eb.consts[symbol]; ok && fc.eb.consts[symbol].addr != 0 {
			continue
		}
		newDataSymbols = append(newDataSymbols, symbol)
	}
	if len(newDataSymbols) > 0 {
		sort.Strings(newDataSymbols)
		for _, symbol := range newDataSymbols {
			value := dataSymbols[symbol]
			fc.eb.DefineAddr(symbol, currentAddr)
			// Write the actual data to the .data buffer
			fc.eb.WriteData([]byte(value))
			currentAddr += uint64(len(value))
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Assigned new .data symbol %s to 0x%x, wrote %d bytes\n", symbol, fc.eb.consts[symbol].addr, len(value))
			}
		}
	}

	// Set lambda function addresses
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Setting lambda function addresses, have %d lambdas\n", len(fc.lambdaOffsets))
	}
	for lambdaName, offset := range fc.lambdaOffsets {
		lambdaAddr := textAddr + uint64(offset)
		fc.eb.DefineAddr(lambdaName, lambdaAddr)

		// Update the symbol value in the dynamic symbol table
		if fc.dynamicSymbols != nil {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Calling UpdateSymbolValue for lambda '%s' at address 0x%x\n", lambdaName, lambdaAddr)
			}
			success := fc.dynamicSymbols.UpdateSymbolValue(lambdaName, lambdaAddr)
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: UpdateSymbolValue returned %v\n", success)
			}
		} else if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: fc.dynamicSymbols is nil, cannot update symbol\n")
		}
	}

	// Rebuild and repatch the symbol table with updated lambda addresses
	if fc.dynamicSymbols != nil {
		fc.dynamicSymbols.buildSymbolTable()
		fc.eb.patchDynsymInELF(fc.dynamicSymbols)
	}

	// Patch PLT calls using callOrder (actual sequence of calls)
	// patchPLTCalls will look up each function name in the PLT to get its offset
	// This handles duplicate calls (e.g., two calls to exit) correctly
	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n=== Second compilation callOrder: %v ===\n", fc.callOrder)
		}
	}
	fc.eb.patchPLTCalls(ds, textAddr, pltBase, fc.callOrder)

	// Patch PC-relative relocations
	rodataSize := fc.eb.rodata.Len()
	fc.eb.PatchPCRelocations(textAddr, rodataBaseAddr, rodataSize)

	// Patch function calls in regenerated code
	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "\n=== Patching function calls (regenerated code) ===\n")
		}
	}
	// Patch hot function pointer table
	fc.patchHotFunctionTable()

	// Update ELF with regenerated code (copies eb.text into ELF buffer)
	fc.eb.patchTextInELF()
	fc.eb.patchRodataInELF()
	// Note: data section is already written during WriteCompleteDynamicELF, no patching needed

	// Output the executable file
	elfBytes := fc.eb.Bytes()

	if CompressFlag {
		archStr := "amd64"
		if fc.eb.target.Arch() == ArchARM64 {
			archStr = "arm64"
		}
		compressed, compressErr := WrapWithDecompressor(elfBytes, archStr)
		if compressErr == nil && len(compressed) < len(elfBytes) {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Compressed %d -> %d bytes (%.1f%%)\n", len(elfBytes), len(compressed), float64(len(compressed))*100/float64(len(elfBytes)))
			}
			elfBytes = compressed
		} else if VerboseMode {
			if compressErr != nil {
				fmt.Fprintf(os.Stderr, "Compression failed: %v\n", compressErr)
			} else {
				fmt.Fprintf(os.Stderr, "Compression didn't reduce size: %d -> %d\n", len(elfBytes), len(compressed))
			}
		}
	}

	if err := os.WriteFile(outputPath, elfBytes, 0o755); err != nil {
		return err
	}

	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Final GOT base: 0x%x\n", gotBase)
		}
	}
	return nil
}

// Confidence that this function is working: 50%
// writePE generates a Windows PE (Portable Executable) file for x86_64
