package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPEReader(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("PE reader test only runs on Linux (with Wine) or Windows")
	}

	// Try to find SDL3.dll in the current directory
	dllPath := "SDL3.dll"
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		t.Skip("SDL3.dll not found in current directory")
	}

	t.Logf("Parsing DLL: %s", dllPath)

	pr, err := OpenPE(dllPath)
	if err != nil {
		t.Fatalf("Failed to open PE file: %v", err)
	}
	defer pr.Close()

	t.Logf("Machine type: 0x%04x", pr.coffHdr.Machine)
	t.Logf("Number of sections: %d", pr.coffHdr.NumberOfSections)
	t.Logf("Image base: 0x%x", pr.optHdr.ImageBase)

	// List sections
	t.Log("Sections:")
	for i, section := range pr.sections {
		name := section.GetName()
		t.Logf("  [%d] %s: VirtualSize=0x%x, VirtualAddress=0x%x, SizeOfRawData=0x%x",
			i, name, section.VirtualSize, section.VirtualAddress, section.SizeOfRawData)
	}

	// Get exports
	exports, err := pr.GetExports()
	if err != nil {
		t.Fatalf("Failed to get exports: %v", err)
	}

	t.Logf("Exports: %d functions", len(exports.Functions))
	t.Logf("Export base: %d", exports.Base)

	// Show first 10 exports
	maxShow := 10
	if len(exports.Functions) < maxShow {
		maxShow = len(exports.Functions)
	}

	t.Logf("First %d exported functions:", maxShow)
	for i := 0; i < maxShow; i++ {
		fn := exports.Functions[i]
		t.Logf("  [%d] %s (ordinal %d, RVA 0x%x)", i, fn.Name, fn.Ordinal, fn.RVA)
	}

	// Look for specific SDL3 functions
	sdl3Funcs := []string{"SDL_Init", "SDL_CreateWindow", "SDL_CreateRenderer", "SDL_GetError"}
	for _, funcName := range sdl3Funcs {
		found := false
		for _, fn := range exports.Functions {
			if fn.Name == funcName {
				found = true
				t.Logf("Found %s: ordinal=%d, RVA=0x%x", funcName, fn.Ordinal, fn.RVA)
				break
			}
		}
		if !found {
			t.Logf("Warning: %s not found in exports", funcName)
		}
	}
}

func TestParseDLL(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("DLL parsing test only runs on Linux or Windows")
	}

	dllPath := "SDL3.dll"
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		t.Skip("SDL3.dll not found")
	}

	exports, err := ParseDLL(dllPath)
	if err != nil {
		t.Fatalf("ParseDLL failed: %v", err)
	}

	if len(exports) == 0 {
		t.Error("No exports found")
	}

	t.Logf("Successfully parsed %d exports from %s", len(exports), dllPath)
}

func TestCHeaderParserImproved(t *testing.T) {
	// Create a test header file with various C constructs
	tmpDir := t.TempDir()
	headerPath := filepath.Join(tmpDir, "test.h")

	headerContent := `
#ifndef TEST_H
#define TEST_H

// Simple constants
#define MAX_SIZE 1024
#define MIN_SIZE 0x10
#define FLAG_READ 0x01
#define FLAG_WRITE 0x02
#define FLAGS_ALL (FLAG_READ | FLAG_WRITE)

// Bit shifts
#define BIT_0 (1 << 0)
#define BIT_7 (1 << 7)
#define MASK_HIGH 0xFF00

// Type definitions
typedef struct SDL_Window SDL_Window;
typedef struct SDL_Renderer SDL_Renderer;
typedef unsigned int Uint32;
typedef void* SDL_GLContext;

// Function declarations
extern int SDL_Init(Uint32 flags);
extern SDL_Window* SDL_CreateWindow(const char* title, int w, int h, Uint32 flags);
extern void SDL_DestroyWindow(SDL_Window* window);
extern int SDL_GetError(void);
extern SDL_Renderer* SDL_CreateRenderer(SDL_Window* window, const char* name);

// Complex function with macros
SDL_DECLSPEC void SDLCALL SDL_RenderPresent(SDL_Renderer* renderer);

// Inline function (should be skipped or handled)
static inline int SDL_FOURCC(char a, char b, char c, char d) {
    return (a) | ((b) << 8) | ((c) << 16) | ((d) << 24);
}

#endif // TEST_H
`

	if err := os.WriteFile(headerPath, []byte(headerContent), 0644); err != nil {
		t.Fatalf("Failed to write test header: %v", err)
	}

	parser := NewCParser()
	constants, err := parser.ParseFile(headerPath)
	if err != nil {
		t.Fatalf("Failed to parse header: %v", err)
	}

	// Check constants
	expectedConstants := map[string]int64{
		"MAX_SIZE":   1024,
		"MIN_SIZE":   0x10,
		"FLAG_READ":  0x01,
		"FLAG_WRITE": 0x02,
		"FLAGS_ALL":  0x03,
		"BIT_0":      1,
		"BIT_7":      128,
		"MASK_HIGH":  0xFF00,
	}

	for name, expectedValue := range expectedConstants {
		if value, ok := constants.Constants[name]; !ok {
			t.Errorf("Constant %s not found", name)
		} else if value != expectedValue {
			t.Errorf("Constant %s: got %d, expected %d", name, value, expectedValue)
		}
	}

	// Check functions
	expectedFuncs := []string{
		"SDL_Init",
		"SDL_CreateWindow",
		"SDL_DestroyWindow",
		"SDL_GetError",
		"SDL_CreateRenderer",
		"SDL_RenderPresent",
	}

	for _, funcName := range expectedFuncs {
		if _, ok := constants.Functions[funcName]; !ok {
			t.Errorf("Function %s not found", funcName)
		} else {
			sig := constants.Functions[funcName]
			t.Logf("Function %s: %s (%d params)",
				funcName, sig.ReturnType, len(sig.Params))
		}
	}

	t.Logf("Parsed %d constants and %d functions",
		len(constants.Constants), len(constants.Functions))
}

func TestCTypeMapping(t *testing.T) {
	// Test C type to Vibe67 type mapping
	testCases := []struct {
		cType      string
		vibe67Type string
		shouldMap  bool
	}{
		{"int", "cint", true},
		{"unsigned int", "cuint", true},
		{"char*", "cstring", true},
		{"const char*", "cstring", true},
		{"void*", "cptr", true},
		{"SDL_Window*", "cptr", true},
		{"Uint32", "cuint", true},
		{"int64_t", "cint64", true},
		{"uint64_t", "cuint64", true},
		{"float", "cfloat", true},
		{"double", "cdouble", true},
	}

	for _, tc := range testCases {
		t.Run(tc.cType, func(t *testing.T) {
			// This would test the actual mapping function once implemented
			t.Logf("C type '%s' should map to Vibe67 type '%s'", tc.cType, tc.vibe67Type)
		})
	}
}

func ExampleParseDLL() {
	// Example of parsing a DLL to get exported functions
	exports, err := ParseDLL("SDL3.dll")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found %d exported functions\n", len(exports))

	// Look for a specific function
	for _, fn := range exports {
		if fn.Name == "SDL_Init" {
			fmt.Printf("SDL_Init found at ordinal %d, RVA 0x%x\n", fn.Ordinal, fn.RVA)
			break
		}
	}
}
