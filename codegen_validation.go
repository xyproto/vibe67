package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

func (fc *C67Compiler) validateGeneratedCode() []string {
	var warnings []string

	// Check 1: Verify all function calls are resolvable
	lambdaSet := make(map[string]bool)
	for _, lambda := range fc.lambdaFuncs {
		lambdaSet[lambda.Name] = true
	}

	// Build set of pattern lambdas
	patternLambdaSet := make(map[string]bool)
	for _, plambda := range fc.patternLambdaFuncs {
		patternLambdaSet[plambda.Name] = true
	}

	// List of common C runtime functions that are always available
	commonCFunctions := map[string]bool{
		"printf": true, "exit": true, "malloc": true, "free": true, "realloc": true,
		"getenv": true, "strlen": true, "memcpy": true, "memset": true, "pow": true,
		"fflush": true, "ExitProcess": true, "GetProcessHeap": true, "HeapAlloc": true,
	}

	// Built-in/intrinsic functions that get compiled into other code
	builtinFunctions := map[string]bool{
		"println": true, "print": true, "len": true, "alloc": true, "main": true,
	}

	for funcName := range fc.usedFunctions {
		// Skip internal vibe67 runtime functions - they'll be generated
		if strings.HasPrefix(funcName, "_vibe67") || strings.HasPrefix(funcName, "vibe67_") {
			continue
		}

		// Skip lambda functions - they are internal
		if lambdaSet[funcName] || patternLambdaSet[funcName] {
			continue
		}

		// Skip forward-declared functions
		if fc.forwardFunctions != nil && fc.forwardFunctions[funcName] {
			continue
		}

		// Skip common C functions
		if commonCFunctions[funcName] {
			continue
		}

		// Skip built-in/intrinsic functions
		if builtinFunctions[funcName] {
			continue
		}

		// Check if it's a known C function
		if _, ok := fc.cFunctionLibs[funcName]; !ok {
			// Check if it's defined as a label
			if fc.eb.LabelOffset(funcName) < 0 {
				// Not in any category - might be unresolved
				warnings = append(warnings, fmt.Sprintf("WARNING: Function '%s' is called but not found (not in lambdas, C FFI, or labels)", funcName))
			}
		}
	}

	// Check 2: Verify arena functions if arenas are used
	if fc.usesArenas {
		arenaFuncs := []string{"_vibe67_init_arenas", "_vibe67_arena_alloc", "_vibe67_arena_reset", "_vibe67_arena_ensure_capacity"}
		for _, funcName := range arenaFuncs {
			// Check if the function is actually defined
			if fc.eb.LabelOffset(funcName) < 0 {
				warnings = append(warnings, fmt.Sprintf("WARNING: Arena function '%s' is not generated but arenas are enabled", funcName))
			}
		}
	}

	// Check 3: Scan generated code for suspicious patterns
	code := fc.eb.text.Bytes()

	// Check for 0xDEADBEEF pattern
	for i := 0; i+3 < len(code); i++ {
		if code[i] == 0xEF && code[i+1] == 0xBE && code[i+2] == 0xAD && code[i+3] == 0xDE {
			warnings = append(warnings, fmt.Sprintf("WARNING: Found 0xDEADBEEF pattern at offset 0x%X - likely unpatched placeholder", i))
		}
	}

	// Check for 0x78563412 pattern (common placeholder)
	for i := 0; i+3 < len(code); i++ {
		if code[i] == 0x12 && code[i+1] == 0x34 && code[i+2] == 0x56 && code[i+3] == 0x78 {
			warnings = append(warnings, fmt.Sprintf("WARNING: Found 0x78563412 pattern at offset 0x%X - likely unpatched placeholder", i))
		}
	}

	// Check 4: Verify stack alignment in prologue
	// Look for push rbp (0x55) at the start
	if len(code) > 0 {
		// Skip initial register clearing (xor instructions) and CPUID
		offset := 0
		for offset < len(code) && offset < 100 {
			if code[offset] == 0x55 { // push rbp
				break
			}
			offset++
		}

		if offset >= 100 {
			warnings = append(warnings, "WARNING: Could not find function prologue (push rbp) in first 100 bytes of code")
		} else {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Found prologue at offset 0x%X\n", offset)
			}
		}
	}

	// Check 5: Verify no undefined CALL instructions
	// Look for FF 15 XX XX XX XX (indirect call) patterns
	for i := 0; i+5 < len(code); i++ {
		if code[i] == 0xFF && code[i+1] == 0x15 {
			// This is a RIP-relative indirect call
			disp := int32(binary.LittleEndian.Uint32(code[i+2 : i+6]))
			targetRVA := uint32(i+6) + uint32(disp)

			// On Windows, this should point to IAT (typically > 0x3000)
			// On Linux, this should be negative or point to GOT
			if fc.eb.target.OS() == OSWindows {
				if disp > 0 && targetRVA < 0x2000 {
					warnings = append(warnings, fmt.Sprintf("WARNING: Suspicious CALL [rip+0x%X] at offset 0x%X (target RVA 0x%X seems too low for IAT)", disp, i, targetRVA))
				}
			}
		}
	}

	// Check 6: Look for unknown functions
	if len(fc.unknownFunctions) > 0 {
		unknownList := make([]string, 0, len(fc.unknownFunctions))
		for funcName := range fc.unknownFunctions {
			unknownList = append(unknownList, funcName)
		}
		warnings = append(warnings, fmt.Sprintf("WARNING: Unknown functions called: %v", unknownList))
	}

	return warnings
}

func (fc *C67Compiler) printCodeValidation() {
	warnings := fc.validateGeneratedCode()

	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\n=== CODE VALIDATION WARNINGS (%d) ===\n", len(warnings))
		for _, warning := range warnings {
			fmt.Fprintf(os.Stderr, "%s\n", warning)
		}
		fmt.Fprintf(os.Stderr, "=====================================\n\n")
	} else if VerboseMode {
		fmt.Fprintf(os.Stderr, "✓ Code validation passed - no warnings\n")
	}
}
