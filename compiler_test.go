package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBasicCompilation tests that vibe67 can compile and generate an executable
func TestBasicCompilation(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.Define("hello", "Hello, World!\n\x00")

	// Generate ELF
	eb.WriteELFHeader()

	bytes := eb.Bytes()
	if len(bytes) == 0 {
		t.Fatal("Generated ELF is empty")
	}

	// Check for ELF magic number
	if bytes[0] != 0x7f || bytes[1] != 'E' || bytes[2] != 'L' || bytes[3] != 'F' {
		t.Fatal("Invalid ELF magic number")
	}
}

// TestArchitectures tests that all supported architectures can be initialized
func TestArchitectures(t *testing.T) {
	archs := []string{"x86_64", "amd64", "aarch64", "arm64", "riscv64"}

	for _, arch := range archs {
		t.Run(arch, func(t *testing.T) {
			eb, err := New(arch)
			if err != nil {
				t.Fatalf("Failed to create ExecutableBuilder for %s: %v", arch, err)
			}

			if eb == nil {
				t.Fatalf("ExecutableBuilder is nil for %s", arch)
			}
		})
	}
}

// TestDynamicLinking tests basic dynamic linking setup
func TestDynamicLinking(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Add a library
	glibc := eb.AddLibrary("glibc", "libc.so.6")
	if glibc == nil {
		t.Fatal("Failed to add library")
	}

	// Add a function
	glibc.AddFunction("printf", CTypeInt,
		Parameter{Name: "format", Type: CTypePointer},
	)

	// Import the function
	err = eb.ImportFunction("glibc", "printf")
	if err != nil {
		t.Fatalf("Failed to import function: %v", err)
	}

	// Check that the function is imported
	if len(eb.dynlinker.ImportedFuncs) == 0 {
		t.Fatal("No functions imported")
	}
}

// TestELFSections tests that dynamic sections can be created
func TestELFSections(t *testing.T) {
	ds := NewDynamicSections(ArchX86_64)

	// Add needed library
	ds.AddNeeded("libc.so.6")

	// Add symbols
	ds.AddSymbol("printf", STB_GLOBAL, STT_FUNC)
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	// Build tables
	ds.buildSymbolTable()
	ds.buildHashTable()

	// Check sizes
	if ds.dynsym.Len() == 0 {
		t.Fatal("Symbol table is empty")
	}
	if ds.dynstr.Len() == 0 {
		t.Fatal("String table is empty")
	}
	if ds.hash.Len() == 0 {
		t.Fatal("Hash table is empty")
	}
}

// TestPLTGOT tests PLT and GOT generation
func TestPLTGOT(t *testing.T) {
	ds := NewDynamicSections(ArchX86_64)

	functions := []string{"printf", "exit"}

	// Add symbols
	for _, fn := range functions {
		ds.AddSymbol(fn, STB_GLOBAL, STT_FUNC)
	}

	// Generate PLT and GOT
	pltBase := uint64(0x402000)
	gotBase := uint64(0x403000)
	dynamicAddr := uint64(0x403000)

	ds.GeneratePLT(functions, gotBase, pltBase)
	ds.GenerateGOT(functions, dynamicAddr, pltBase)

	// Check sizes
	// PLT[0] is 16 bytes, plus 16 bytes per function
	expectedPLTSize := 16 + len(functions)*16
	if ds.plt.Len() != expectedPLTSize {
		t.Errorf("PLT size = %d, want %d", ds.plt.Len(), expectedPLTSize)
	}

	// GOT has 3 reserved entries (24 bytes) plus one per function (8 bytes each)
	expectedGOTSize := 24 + len(functions)*8
	if ds.got.Len() != expectedGOTSize {
		t.Errorf("GOT size = %d, want %d", ds.got.Len(), expectedGOTSize)
	}
}

// TestExecutableGeneration tests full executable generation
func TestExecutableGeneration(t *testing.T) {
	// Only run if we have write permissions
	tmpfile, err := os.CreateTemp("", "vibe67_test_output")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.Define("hello", "Test\n\x00")
	eb.useDynamicLinking = false

	// Generate static ELF
	eb.WriteELFHeader()

	// Write to file
	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0644)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Set executable permissions
	err = os.Chmod(tmpfilePath, 0755)
	if err != nil {
		t.Fatalf("Failed to set executable permissions: %v", err)
	}

	// Check that it's a valid ELF
	fileInfo, err := IdentifyFile(tmpfilePath)
	if err != nil {
		t.Logf("Failed to identify file: %v", err)
	} else {
		t.Logf("File type: %s", fileInfo.String())
		if !fileInfo.IsELF() {
			t.Errorf("Expected ELF file, got: %s", fileInfo.String())
		}
	}

	// Just verify the file was created with executable permissions
	info, err := os.Stat(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check that owner has execute permission (umask may affect group/other)
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("File not executable: permissions = %o", info.Mode().Perm())
	}
}

// TestCTypeSize tests CType size calculations
func TestCTypeSize(t *testing.T) {
	tests := []struct {
		ctype    CType
		expected int
	}{
		{CTypeVoid, 0},
		{CTypeInt, 4},
		{CTypeUInt, 4},
		{CTypeChar, 1},
		{CTypeFloat, 4},
		{CTypeDouble, 8},
		{CTypeLong, 8},
		{CTypeULong, 8},
		{CTypePointer, 8},
	}

	for _, tt := range tests {
		t.Run(tt.ctype.String(), func(t *testing.T) {
			size := tt.ctype.Size(ArchX86_64)
			if size != tt.expected {
				t.Errorf("Size(%s) = %d, want %d", tt.ctype, size, tt.expected)
			}
		})
	}
}

func testParallelSimpleCompilesOLD(t *testing.T) {
	// FIXED: Lambda epilogue fix resolved crashes on all platforms including ARM64

	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "parallel_simple.bin")

	cmd := exec.Command("go", "run", ".", "-o", output, "testprograms/parallel_simple.vibe67")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to compile parallel_simple.vibe67: %v\n%s", err, string(out))
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("compiled executable missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("compiled executable is empty")
	}

	runCmd := exec.Command(output)
	runOutput, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("parallel_simple executable failed: %v\n%s", err, string(runOutput))
	}
}

// TestCompilationErrors tests that the compiler correctly rejects invalid code
func TestCompilationErrors(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		errorContains string
	}{
		{
			name: "const_immutability",
			code: `// Test immutable variables (should fail)
x = 10
x <- x + 5
println(x)
`,
			errorContains: "cannot update immutable variable 'x'",
		},
		{
			name: "lambda_bad_syntax",
			code: `// This should fail - using => instead of ->
double = x => x * 2
println(double(5))
`,
			errorContains: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file with test code
			tmpFile, err := os.CreateTemp("", "vibe67_error_test_*.vibe67")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			tmpFilePath := tmpFile.Name()
			defer os.Remove(tmpFilePath)

			if _, err := tmpFile.WriteString(tt.code); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write test code: %v", err)
			}
			tmpFile.Close()

			// Create temp output file
			tmpOutput, err := os.CreateTemp("", "vibe67_error_output_*")
			if err != nil {
				t.Fatalf("Failed to create temp output file: %v", err)
			}
			tmpOutputPath := tmpOutput.Name()
			tmpOutput.Close()
			defer os.Remove(tmpOutputPath)

			// Suppress stderr during compilation (these are expected errors)
			oldStderr := os.Stderr
			devNull, err := os.Open(os.DevNull)
			if err == nil {
				os.Stderr = devNull
				defer func() {
					os.Stderr = oldStderr
					devNull.Close()
				}()
			}

			// Try to compile - should fail
			platform := GetDefaultPlatform()
			err = CompileVibe67(tmpFilePath, tmpOutputPath, platform)

			// Restore stderr before checking results
			if devNull != nil {
				os.Stderr = oldStderr
				devNull.Close()
			}

			// Verify it failed
			if err == nil {
				t.Errorf("Expected compilation to fail, but it succeeded")
				return
			}

			// Verify error message contains expected text
			errorMsg := err.Error()
			if errorMsg == "" {
				t.Errorf("Expected error message, got empty string")
				return
			}

			// Check for expected error substring
			if tt.errorContains != "" {
				if !containsSubstring(errorMsg, tt.errorContains) {
					t.Errorf("Expected error to contain %q, got: %s", tt.errorContains, errorMsg)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive check not needed)
func containsSubstring(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
