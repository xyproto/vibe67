// Completion: 100% - Module complete
//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
)

// Demo program showing the C FFI manager capabilities
func main() {
	fmt.Println("=== vibe67 C FFI Manager Demo ===\n")

	// Create a new CFFI manager
	cffi := NewCFFIManager()

	// Try to load SDL3 library
	fmt.Println("Loading SDL3 library...")

	// Parse the SDL3 DLL
	dllPath := "SDL3.dll"
	if _, err := os.Stat(dllPath); err == nil {
		fmt.Printf("  Parsing DLL: %s\n", dllPath)
		if err := cffi.ParseDLLExports("SDL3", dllPath); err != nil {
			fmt.Printf("  Error parsing DLL: %v\n", err)
		} else {
			fmt.Printf("  Successfully parsed DLL\n")
		}
	} else {
		fmt.Printf("  SDL3.dll not found, skipping DLL parsing\n")
	}

	// Parse a simple test header
	fmt.Println("\nParsing test header...")
	testHeader := "/tmp/test_sdl3.h"
	testContent := `
#define SDL_INIT_VIDEO 0x00000020
#define SDL_WINDOWPOS_CENTERED 0x2FFF0000

extern int SDL_Init(unsigned int flags);
extern void* SDL_CreateWindow(const char* title, int x, int y, int w, int h, unsigned int flags);
extern void SDL_DestroyWindow(void* window);
extern const char* SDL_GetError(void);
`
	if err := os.WriteFile(testHeader, []byte(testContent), 0644); err != nil {
		fmt.Printf("  Error creating test header: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(testHeader)

	if err := cffi.ParseHeader(testHeader); err != nil {
		fmt.Printf("  Error parsing header: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("  Successfully parsed header")

	// Show constants
	fmt.Println("\nExtracted constants:")
	for name, value := range cffi.headerConstants.Constants {
		fmt.Printf("  %s = 0x%x (%d)\n", name, value, value)
	}

	// Show functions
	fmt.Println("\nExtracted functions:")
	for name, fn := range cffi.functions {
		returnType := cffi.MapCTypeToVibe67(fn.ReturnType)
		fmt.Printf("  %s: ", name)
		if fn.Library != "" {
			fmt.Printf("[%s] ", fn.Library)
		}
		fmt.Printf("%s -> %s (", fn.ReturnType, returnType)
		for i, param := range fn.Params {
			vibe67Type := cffi.MapCTypeToVibe67(param.Type)
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s: %s->%s", param.Name, param.Type, vibe67Type)
		}
		fmt.Println(")")
	}

	// Try to generate Vibe67 bindings
	fmt.Println("\nGenerated Vibe67 bindings:")
	for name := range cffi.functions {
		if binding, err := cffi.GenerateVibe67Binding(name); err == nil {
			fmt.Printf("\n%s\n", binding)
		}
	}

	// Show type mappings
	fmt.Println("\n\nC to Vibe67 type mappings:")
	testTypes := []string{
		"int", "unsigned int", "char*", "const char*", "void*",
		"int64_t", "uint64_t", "float", "double",
	}
	for _, cType := range testTypes {
		vibe67Type := cffi.MapCTypeToVibe67(cType)
		fmt.Printf("  %-20s -> %s\n", cType, vibe67Type)
	}

	fmt.Println("\n=== Demo Complete ===")
}
