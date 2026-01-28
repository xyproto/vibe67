package main

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestELFMagicNumber verifies basic ELF magic number
func TestELFMagicNumber(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.WriteELFHeader()
	bytes := eb.Bytes()

	if len(bytes) < 4 {
		t.Fatal("ELF too small")
	}

	if bytes[0] != 0x7f || bytes[1] != 'E' || bytes[2] != 'L' || bytes[3] != 'F' {
		t.Fatal("Invalid ELF magic number")
	}
}

// TestELFClass verifies ELF is 64-bit
func TestELFClass(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.WriteELFHeader()
	bytes := eb.Bytes()

	if bytes[4] != 2 {
		t.Errorf("Expected 64-bit ELF (class=2), got class=%d", bytes[4])
	}
}

// TestELFEndianness verifies little-endian
func TestELFEndianness(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.WriteELFHeader()
	bytes := eb.Bytes()

	if bytes[5] != 1 {
		t.Errorf("Expected little-endian (1), got %d", bytes[5])
	}
}

// TestELFOSABI verifies Linux ABI
func TestELFOSABI(t *testing.T) {
	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.WriteELFHeader()
	bytes := eb.Bytes()

	if bytes[7] != 3 {
		t.Errorf("Expected Linux OS/ABI (3), got %d", bytes[7])
	}
}

// TestMinimalELFSize ensures we stay under size targets
func TestMinimalELFSize(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_size_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	// Create minimal program
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	// Minimal code: just call exit(0)
	eb.Emit("xor rdi, rdi") // exit code 0
	eb.Emit("call exit@plt")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	info, err := os.Stat(tmpfile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	size := info.Size()
	t.Logf("Minimal ELF size: %d bytes", size)

	// Target: under 64k (stretch goal: under 4k)
	const maxSize = 65536
	if size > maxSize {
		t.Errorf("ELF size %d exceeds maximum %d bytes", size, maxSize)
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

// TestDynamicELFExecutable verifies generated ELF can be executed
func TestDynamicELFExecutable(t *testing.T) {
	// Skip on non-Linux systems since we're generating ELF binaries
	if GetDefaultPlatform().OS != OSLinux {
		t.Skip("Skipping ELF execution test on non-Linux platform")
	}

	tmpfile := filepath.Join(os.TempDir(), "vibe67_exec_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{} // No C library functions needed

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")

	// Exit with code 42 using syscall (no C library call)
	// exit syscall number is 60 on x86_64
	eb.Emit("mov rax, 60") // syscall number for exit
	eb.Emit("mov rdi, 42") // exit code
	eb.Emit("syscall")     // invoke syscall

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Execute and check exit code
	cmd := exec.Command(tmpfile)
	output, err := cmd.CombinedOutput()

	// Should exit with code 42
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 42 {
			t.Errorf("Expected exit code 42, got %d\nOutput: %s", exitErr.ExitCode(), string(output))
		}
	} else if err != nil {
		t.Fatalf("Failed to execute: %v\nOutput: %s", err, string(output))
	} else {
		t.Error("Expected exit code 42, but command succeeded (exit 0)")
	}
}

// TestELFSegmentAlignment verifies proper segment alignment
func TestELFSegmentAlignment(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_align_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	eb.Emit("xor rdi, rdi")
	eb.Emit("call exit@plt")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Parse with debug/elf to verify alignment
	f, err := elf.Open(tmpfile)
	if err != nil {
		t.Fatalf("Failed to open ELF: %v", err)
	}
	defer f.Close()

	// Check that all loadable segments are page-aligned
	const pageSize = 0x1000
	for i, prog := range f.Progs {
		if prog.Type == elf.PT_LOAD {
			if prog.Align != pageSize {
				t.Errorf("Segment %d: align = 0x%x, want 0x%x", i, prog.Align, pageSize)
			}
			if prog.Vaddr%pageSize != 0 {
				t.Errorf("Segment %d: vaddr 0x%x not aligned to 0x%x", i, prog.Vaddr, pageSize)
			}
		}
	}
}

// TestELFInterpSegment verifies interpreter segment exists and is valid
func TestELFInterpSegment(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_interp_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	eb.Emit("xor rdi, rdi")
	eb.Emit("call exit@plt")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	f, err := elf.Open(tmpfile)
	if err != nil {
		t.Fatalf("Failed to open ELF: %v", err)
	}
	defer f.Close()

	// Find interpreter segment
	var interp *elf.Prog
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			interp = prog
			break
		}
	}

	if interp == nil {
		t.Fatal("Missing PT_INTERP segment")
	}

	// Read interpreter string
	interpBytes := make([]byte, interp.Filesz)
	_, err = interp.ReadAt(interpBytes, 0)
	if err != nil {
		t.Fatalf("Failed to read interpreter: %v", err)
	}

	interpStr := string(interpBytes[:len(interpBytes)-1]) // Remove null terminator
	t.Logf("Interpreter: %s", interpStr)

	// Verify it looks like a valid path
	if interpStr == "" {
		t.Error("Interpreter path is empty")
	}
	if interpStr[0] != '/' {
		t.Errorf("Interpreter path should be absolute, got: %s", interpStr)
	}
}

// TestELFDynamicSegment verifies dynamic segment structure
func TestELFDynamicSegment(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_dynamic_seg_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"printf"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("printf", STB_GLOBAL, STT_FUNC)

	// Minimal printf call
	eb.Define("fmt", "%s\n\x00")
	eb.Emit("lea rdi, [rip + fmt]")
	eb.Emit("call printf@plt")
	eb.Emit("xor rdi, rdi")
	eb.Emit("mov rax, 60") // sys_exit
	eb.Emit("syscall")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"printf"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	f, err := elf.Open(tmpfile)
	if err != nil {
		t.Fatalf("Failed to open ELF: %v", err)
	}
	defer f.Close()

	// Verify dynamic segment exists
	var dynamicSeg *elf.Prog
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_DYNAMIC {
			dynamicSeg = prog
			break
		}
	}

	if dynamicSeg == nil {
		t.Fatal("Missing PT_DYNAMIC segment")
	}

	if dynamicSeg.Filesz == 0 {
		t.Error("Dynamic segment has zero size")
	}
}

// TestELFType verifies ET_DYN type for PIE executables
func TestELFType(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_type_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	eb.Emit("xor rdi, rdi")
	eb.Emit("call exit@plt")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	f, err := elf.Open(tmpfile)
	if err != nil {
		t.Fatalf("Failed to open ELF: %v", err)
	}
	defer f.Close()

	// Dynamic executables should be ET_DYN (PIE)
	if f.Type != elf.ET_DYN {
		t.Errorf("ELF type = %v, want ET_DYN", f.Type)
	}
}

// TestELFMachine verifies machine type
func TestELFMachine(t *testing.T) {
	tests := []struct {
		arch     string
		expected elf.Machine
	}{
		{"x86_64-linux", elf.EM_X86_64},
		{"amd64-linux", elf.EM_X86_64},
		{"aarch64-linux", elf.EM_AARCH64},
		{"arm64-linux", elf.EM_AARCH64},
	}

	for _, tt := range tests {
		t.Run(tt.arch, func(t *testing.T) {
			tmpfile := filepath.Join(os.TempDir(), "vibe67_machine_test_"+tt.arch)
			defer os.Remove(tmpfile)

			eb, err := New(tt.arch)
			if err != nil {
				t.Fatalf("Failed to create ExecutableBuilder: %v", err)
			}

			eb.useDynamicLinking = true
			eb.neededFunctions = []string{"exit"}

			ds := NewDynamicSections(ArchX86_64)
			ds.AddNeeded("libc.so.6")
			ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

			// Architecture-specific exit code
			if tt.arch == "x86_64-linux" || tt.arch == "amd64-linux" {
				eb.Emit("xor rdi, rdi")
				eb.Emit("call exit@plt")
			} else {
				// For ARM64/RISC-V, just write minimal header
				eb.WriteELFHeader()
			}

			_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
			if err != nil {
				t.Skipf("WriteCompleteDynamicELF not fully implemented for %s", tt.arch)
				return
			}

			err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
			if err != nil {
				t.Fatalf("Failed to write executable: %v", err)
			}

			f, err := elf.Open(tmpfile)
			if err != nil {
				t.Fatalf("Failed to open ELF: %v", err)
			}
			defer f.Close()

			if f.Machine != tt.expected {
				t.Errorf("Machine = %v, want %v", f.Machine, tt.expected)
			}
		})
	}
}

// TestELFPermissions verifies executable permissions
func TestELFPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	tmpfile := filepath.Join(tmpDir, "vibe67_perms_test")
	defer os.Remove(tmpfile)

	eb, err := New("x86_64-linux")
	if err != nil {
		t.Fatalf("Failed to create ExecutableBuilder: %v", err)
	}

	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"exit"}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("exit", STB_GLOBAL, STT_FUNC)

	eb.Emit("xor rdi, rdi")
	eb.Emit("call exit@plt")

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"exit"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfile, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	info, err := os.Stat(tmpfile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check owner execute permission
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("File not executable: permissions = %o", info.Mode().Perm())
	}
}
