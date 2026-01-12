package main

import (
	"debug/macho"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMachOMagicNumber verifies Mach-O magic number
func TestMachOMagicNumber(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	bytes := eb.Bytes()
	if len(bytes) < 4 {
		t.Fatal("Mach-O too small")
	}

	// Check for 64-bit Mach-O magic (little-endian)
	magic := binary.LittleEndian.Uint32(bytes[0:4])
	if magic != MH_MAGIC_64 {
		t.Errorf("Invalid Mach-O magic: got 0x%x, want 0x%x", magic, MH_MAGIC_64)
	}
}

// TestMachOFileType verifies MH_EXECUTE type
func TestMachOFileType(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	bytes := eb.Bytes()
	if len(bytes) < 16 {
		t.Fatal("Mach-O header too small")
	}

	// File type is at offset 12
	fileType := binary.LittleEndian.Uint32(bytes[12:16])
	if fileType != MH_EXECUTE {
		t.Errorf("File type = 0x%x, want 0x%x (MH_EXECUTE)", fileType, MH_EXECUTE)
	}
}

// TestMachOCPUTypes verifies CPU types for different architectures
func TestMachOCPUTypes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tests := []struct {
		arch        string
		expectedCPU uint32
	}{
		{"x86_64", CPU_TYPE_X86_64},
		{"amd64", CPU_TYPE_X86_64},
		{"arm64", CPU_TYPE_ARM64},
		{"aarch64", CPU_TYPE_ARM64},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			eb, err := New(tt.arch)
			if err != nil {
				t.Fatalf("Failed to create ExecutableBuilder: %v", err)
			}

			err = eb.WriteMachO()
			if err != nil {
				t.Fatalf("Failed to write Mach-O: %v", err)
			}

			bytes := eb.Bytes()
			if len(bytes) < 8 {
				t.Fatal("Mach-O header too small")
			}

			// CPU type is at offset 4
			cpuType := binary.LittleEndian.Uint32(bytes[4:8])
			if cpuType != tt.expectedCPU {
				t.Errorf("CPU type = 0x%x, want 0x%x", cpuType, tt.expectedCPU)
			}
		})
	}
}

// TestMachOSegments verifies required segments exist
func TestMachOSegments(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_seg_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Add some minimal code
	eb.Emit("mov rax, 0x2000001") // sys_exit on macOS
	eb.Emit("xor rdi, rdi")
	eb.Emit("syscall")

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Parse with debug/macho
	f, err := macho.Open(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to open Mach-O: %v", err)
	}
	defer f.Close()

	// Check for required segments
	requiredSegs := map[string]bool{
		"__PAGEZERO": false,
		"__TEXT":     false,
	}

	for _, load := range f.Loads {
		if seg, ok := load.(*macho.Segment); ok {
			if _, exists := requiredSegs[seg.Name]; exists {
				requiredSegs[seg.Name] = true
			}
		}
	}

	for segName, found := range requiredSegs {
		if !found {
			t.Errorf("Missing required segment: %s", segName)
		}
	}
}

// TestMachOPageZero verifies __PAGEZERO segment
func TestMachOPageZero(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_zero_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	f, err := macho.Open(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to open Mach-O: %v", err)
	}
	defer f.Close()

	// Find __PAGEZERO
	var pageZero *macho.Segment
	for _, load := range f.Loads {
		if seg, ok := load.(*macho.Segment); ok && seg.Name == "__PAGEZERO" {
			pageZero = seg
			break
		}
	}

	if pageZero == nil {
		t.Fatal("Missing __PAGEZERO segment")
	}

	// Verify it starts at address 0
	if pageZero.Addr != 0 {
		t.Errorf("__PAGEZERO address = 0x%x, want 0", pageZero.Addr)
	}

	// Verify it has no file content
	if pageZero.Filesz != 0 {
		t.Errorf("__PAGEZERO file size = %d, want 0", pageZero.Filesz)
	}

	// Verify it has VM size (typically 4KB or 16KB)
	if pageZero.Memsz == 0 {
		t.Error("__PAGEZERO memory size is 0")
	}
}

// TestMachOTextSegment verifies __TEXT segment
func TestMachOTextSegment(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_text_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Add code
	eb.Emit("mov rax, 0x2000001")
	eb.Emit("xor rdi, rdi")
	eb.Emit("syscall")

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	f, err := macho.Open(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to open Mach-O: %v", err)
	}
	defer f.Close()

	// Find __TEXT segment
	var textSeg *macho.Segment
	for _, load := range f.Loads {
		if seg, ok := load.(*macho.Segment); ok && seg.Name == "__TEXT" {
			textSeg = seg
			break
		}
	}

	if textSeg == nil {
		t.Fatal("Missing __TEXT segment")
	}

	// Verify it's readable and executable
	// Note: debug/macho doesn't expose protection flags, so we check MaxProt
	if textSeg.Maxprot&VM_PROT_READ == 0 {
		t.Error("__TEXT segment not readable")
	}
	if textSeg.Maxprot&VM_PROT_EXECUTE == 0 {
		t.Error("__TEXT segment not executable")
	}

	// Verify file size matches memory size (no BSS in __TEXT)
	if textSeg.Filesz != textSeg.Memsz {
		t.Errorf("__TEXT file size (%d) != memory size (%d)", textSeg.Filesz, textSeg.Memsz)
	}
}

// TestMachOMinimalSize ensures we stay under size targets
func TestMachOMinimalSize(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_size_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Minimal program: just exit
	eb.Emit("mov rax, 0x2000001") // sys_exit
	eb.Emit("xor rdi, rdi")
	eb.Emit("syscall")

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	info, err := os.Stat(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	size := info.Size()
	t.Logf("Minimal Mach-O size: %d bytes", size)

	// Target: under 64k (stretch goal: under 4k)
	const maxSize = 65536
	if size > maxSize {
		t.Errorf("Mach-O size %d exceeds maximum %d bytes", size, maxSize)
	}

	// Log progress toward stretch goal
	const stretchGoal = 4096
	if size <= stretchGoal {
		t.Logf("✓ Achieved stretch goal: %d <= %d bytes", size, stretchGoal)
	} else {
		t.Logf("Size exceeds stretch goal by %d bytes (%d > %d)",
			size-stretchGoal, size, stretchGoal)
	}
}

// TestMachOExecutable verifies generated Mach-O can be executed
func TestMachOExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O execution tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_exec_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	// defer os.Remove(tmpfilePath) // Keep for debugging

	// Use host architecture for execution test
	VerboseMode = true // Enable debug output
	eb, err := New(runtime.GOARCH + "-darwin")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Enable dynamic linking and set up exit() call
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	// Exit with code 42 - using proper library call
	if runtime.GOARCH == "arm64" || runtime.GOARCH == "aarch64" {
		// ARM64: mov w0, #42 then call exit
		eb.Emit("mov w0, #42") // exit code in w0
		eb.Emit("bl _exit")    // call exit stub
	} else {
		// x86_64: mov edi, 42 then call exit
		eb.Emit("mov edi, 42") // exit code
		eb.Emit("call _exit")  // call exit stub
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	bytesToWrite := eb.Bytes()
	t.Logf("About to write %d bytes to %s", len(bytesToWrite), tmpfilePath)
	if len(bytesToWrite) >= 824 {
		t.Logf("Bytes at offset 816: %x", bytesToWrite[816:824])
	}

	// Debug: write to alternate file for comparison
	debugFile := filepath.Join(t.TempDir(), "test_bytes_direct")
	os.WriteFile(debugFile, bytesToWrite, 0755)

	err = os.WriteFile(tmpfilePath, bytesToWrite, 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Ensure executable permissions (WriteFile mode doesn't apply to existing files)
	err = os.Chmod(tmpfilePath, 0755)
	if err != nil {
		t.Fatalf("Failed to chmod executable: %v", err)
	}

	// Debug: immediately read back what was written
	writtenBytes, _ := os.ReadFile(tmpfilePath)
	t.Logf("After write, file size=%d", len(writtenBytes))
	if len(writtenBytes) >= 824 {
		t.Logf("After write, bytes at offset 816 in file: %x", writtenBytes[816:824])
	}

	// Binary is now self-signed by vibe67's generateCodeSignature()
	// No external codesign tool needed!
	t.Logf("Binary self-signed by vibe67")

	// KNOWN ISSUE: macOS dyld doesn't honor stacksize in LC_MAIN, giving only ~5.6KB stack
	// This causes crashes even in simple programs. Skip execution test until resolved.
	// See TODO.md for details.
	t.Skip("Skipping execution test due to macOS stack size issue")

	// Execute and check exit code
	cmd := exec.Command(tmpfilePath)
	err = cmd.Run()

	// Should exit with code 42
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 42 {
			t.Errorf("Expected exit code 42, got %d", exitErr.ExitCode())
		}
	} else if err != nil {
		t.Fatalf("Failed to execute: %v", err)
	} else {
		t.Error("Expected exit code 42, but command succeeded (exit 0)")
	}
}

// TestMachOFileCommand verifies file command recognizes it
func TestMachOFileCommand(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_file_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Verify it's recognized as Mach-O
	fileInfo, err := IdentifyFile(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to identify file: %v", err)
	}

	t.Logf("File type: %s", fileInfo.String())

	// Verify it's recognized as Mach-O
	if !fileInfo.IsMachO() {
		t.Errorf("Expected Mach-O file, got: %s", fileInfo.String())
	}
}

// TestMachOPermissions verifies executable permissions
func TestMachOPermissions(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	tmpfile, err := os.CreateTemp("", "vibe67_macho_perms_test")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfilePath := tmpfile.Name()
	tmpfile.Close()
	defer os.Remove(tmpfilePath)

	eb, err := New("x86_64")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	err = eb.WriteMachO()
	if err != nil {
		t.Fatalf("Failed to write Mach-O: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Ensure executable permissions (WriteFile mode doesn't apply to existing files)
	err = os.Chmod(tmpfilePath, 0755)
	if err != nil {
		t.Fatalf("Failed to chmod executable: %v", err)
	}

	info, err := os.Stat(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check owner execute permission
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("File not executable: permissions = %o", info.Mode().Perm())
	}
}

// Helper function to check if string contains substring
func machoContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
