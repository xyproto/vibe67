// Completion: 100% - Writer module complete
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// codegen_pe_writer.go - PE executable generation for x86_64 Windows
//
// This file handles the generation of PE (Portable Executable) files
// for Windows systems on x86_64 architecture.

// Confidence that this function is working: 80%
func (fc *C67Compiler) writePE(program *Program, outputPath string) error {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Generating Windows PE executable\n")
	}

	// For Windows PE, we need to handle imports differently than ELF
	// Windows uses import tables instead of PLT/GOT

	// Build library -> functions map for imports
	libraries := make(map[string][]string)

	// Standard C runtime functions (msvcrt.dll)
	msvcrtFuncs := []string{"printf", "fprintf", "exit", "malloc", "free", "realloc", "strlen", "pow", "fflush", "sin", "cos", "sqrt", "fopen", "fclose", "fwrite", "fread", "memcpy", "memset"}

	// Add all functions from usedFunctions, organized by library
	lambdaSet := make(map[string]bool)
	for _, lambda := range fc.lambdaFuncs {
		lambdaSet[lambda.Name] = true
	}

	for funcName := range fc.usedFunctions {
		// Skip lambda functions - they are internal
		if lambdaSet[funcName] {
			continue
		}
		// Skip internal runtime functions
		if strings.HasPrefix(funcName, "_vibe67") || strings.HasPrefix(funcName, "vibe67_") {
			continue
		}
		// Skip Vibe67 built-in functions (they generate internal code)
		if funcName == "println" || funcName == "print" || funcName == "len" || funcName == "push" || funcName == "pop" {
			continue
		}

		// Check if this function belongs to a specific library
		if libName, ok := fc.cFunctionLibs[funcName]; ok {
			// Map library name to DLL name
			dllName := mapLibraryToDLL(libName)
			libraries[dllName] = append(libraries[dllName], funcName)
		} else {
			// Default to msvcrt.dll for C standard library functions
			libraries["msvcrt.dll"] = append(libraries["msvcrt.dll"], funcName)
		}
	}

	// Always include minimal msvcrt.dll functions
	if len(libraries["msvcrt.dll"]) == 0 {
		libraries["msvcrt.dll"] = msvcrtFuncs
	}

	// Sort function names within each library for deterministic output
	for dllName := range libraries {
		funcs := libraries[dllName]
		sort.Strings(funcs)
		libraries[dllName] = funcs
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Windows imports by library:\n")
	for dll, funcs := range libraries {
		fmt.Fprintf(os.Stderr, "DEBUG:   %s: %v\n", dll, funcs)
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Windows imports by library:\n")
		for dll, funcs := range libraries {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", dll, funcs)
		}
	}

	// Write the PE file with proper import tables
	if err := fc.eb.WritePEWithLibraries(outputPath, libraries); err != nil {
		return fmt.Errorf("failed to write PE file: %v", err)
	}

	// Validate generated code
	fc.printCodeValidation()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PE executable written to %s\n", outputPath)
	}

	return nil
}

// mapLibraryToDLL maps a library name (like "sdl3") to its Windows DLL name (like "SDL3.dll")
func mapLibraryToDLL(libName string) string {
	// Common library name mappings
	dllMap := map[string]string{
		"kernel32": "kernel32.dll",
		"sdl3":     "SDL3.dll",
		"sdl2":     "SDL2.dll",
		"raylib":   "raylib.dll",
		"sqlite3":  "sqlite3.dll",
		"opengl":   "opengl32.dll",
		"glu":      "glu32.dll",
		"glfw":     "glfw3.dll",
		"curl":     "libcurl.dll",
		"png":      "libpng.dll",
		"jpeg":     "libjpeg.dll",
		"zlib":     "zlib1.dll",
	}

	if dll, ok := dllMap[libName]; ok {
		return dll
	}

	// Default: add .dll extension
	return libName + ".dll"
}
