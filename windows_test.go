package main

import (
	"os"
	"testing"
)

// TestWindowsCompilation verifies that Windows PE executables can be compiled
// This test only checks compilation, not execution (which would require Wine)
// For actual testing on Windows, use windows.vibe67 manually
func TestWindowsCompilation(t *testing.T) {
	code := `Main = {
println("Hello Windows")
x := 10 + 5
println(x)
c.printf("C FFI works: %d\n", 42)
}`
	// Create temp file
	tmpFile, err := os.CreateTemp("", "windows_test_*.vibe67")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write code: %v", err)
	}
	tmpFile.Close()

	// Compile for Windows
	outputPath := tmpPath + ".exe"
	defer os.Remove(outputPath)

	platform := Platform{
		Arch: ArchX86_64,
		OS:   OSWindows,
	}

	err = CompileC67(tmpPath, outputPath, platform)
	if err != nil {
		t.Errorf("Windows compilation failed: %v", err)
		return
	}

	// Verify PE executable was created
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Errorf("Output file not created: %v", err)
		return
	}

	if info.Size() < 100 {
		t.Errorf("Output file suspiciously small: %d bytes", info.Size())
	}

	// Verify PE header (MZ signature)
	data := make([]byte, 2)
	f, err := os.Open(outputPath)
	if err != nil {
		t.Errorf("Cannot open output file: %v", err)
		return
	}
	defer f.Close()

	n, err := f.Read(data)
	if err != nil || n != 2 {
		t.Errorf("Cannot read PE header: %v", err)
		return
	}

	if data[0] != 'M' || data[1] != 'Z' {
		t.Errorf("Invalid PE header: expected 'MZ', got %c%c", data[0], data[1])
	}
}









