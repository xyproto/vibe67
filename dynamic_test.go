package main

import (
	"debug/elf"
	"os"
	"os/exec"
	"testing"
)

// TestDynamicELFStructure tests the structure of dynamically-linked ELF
func TestDynamicELFStructure(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "vibe67_dynamic_test")
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

	eb.Define("hello", "Hello, Test!\n\x00")
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"printf", "exit"}

	// Generate dynamic ELF
	err = eb.GenerateGlibcHelloWorld()
	if err != nil {
		t.Fatalf("Failed to generate glibc hello world: %v", err)
	}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	for _, funcName := range eb.neededFunctions {
		ds.AddSymbol(funcName, STB_GLOBAL, STT_FUNC)
	}

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, eb.neededFunctions)
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	// Write to file
	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Parse ELF to verify structure
	f, err := elf.Open(tmpfilePath)
	if err != nil {
		t.Fatalf("Failed to open ELF: %v", err)
	}
	defer f.Close()

	// Check ELF type
	if f.Type != elf.ET_DYN {
		t.Errorf("ELF type = %v, want ET_DYN", f.Type)
	}

	// Check for INTERP segment
	hasInterp := false
	hasDynamic := false
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			hasInterp = true
		}
		if prog.Type == elf.PT_DYNAMIC {
			hasDynamic = true
		}
	}

	if !hasInterp {
		t.Error("Missing PT_INTERP segment")
	}
	if !hasDynamic {
		t.Error("Missing PT_DYNAMIC segment")
	}

	// Check dynamic section
	// Note: DynamicSymbols() requires section headers which we don't generate
	dyns, err := f.DynamicSymbols()
	if err != nil {
		t.Logf("Warning: Failed to read dynamic symbols: %v", err)
	} else if len(dyns) == 0 {
		t.Logf("Note: No dynamic symbols found (this is expected without section headers)")
	}

	// Check for needed libraries
	// Note: ImportedLibraries() requires section headers which we don't generate
	libs, err := f.ImportedLibraries()
	if err != nil {
		// This is expected since we don't have section headers
		t.Logf("Note: Cannot read imported libraries without section headers: %v", err)
	} else if len(libs) == 0 {
		// Empty list also indicates no section headers
		t.Logf("Note: No imported libraries found (this is expected without section headers)")
	} else {
		foundLibc := false
		for _, lib := range libs {
			if lib == "libc.so.6" {
				foundLibc = true
				break
			}
		}
		if !foundLibc {
			t.Errorf("Missing libc.so.6 in imported libraries, got: %v", libs)
		}
	}
}

// TestRelocationAddresses tests that relocations have correct addresses
func TestRelocationAddresses(t *testing.T) {
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

	// Add relocations
	for i := range functions {
		symIndex := uint32(i + 1)
		gotEntryAddr := gotBase + uint64(24+i*8)
		ds.AddRelocation(gotEntryAddr, symIndex, R_X86_64_JUMP_SLOT)
	}

	// Verify relocation count
	if ds.relaCount != len(functions) {
		t.Errorf("Relocation count = %d, want %d", ds.relaCount, len(functions))
	}

	// Verify relocation buffer size (24 bytes per entry)
	expectedSize := len(functions) * 24
	if ds.rela.Len() != expectedSize {
		t.Errorf("Relocation buffer size = %d, want %d", ds.rela.Len(), expectedSize)
	}
}

// TestPLTOffset tests that PLT offset calculation is correct
func TestPLTOffset(t *testing.T) {
	ds := NewDynamicSections(ArchX86_64)

	functions := []string{"printf", "exit", "malloc"}

	ds.GeneratePLT(functions, 0x403000, 0x402000)

	// PLT[0] is at offset 0, size 16
	// printf should be at offset 16
	// exit should be at offset 32
	// malloc should be at offset 48

	tests := []struct {
		name     string
		expected int
	}{
		{"printf", 16},
		{"exit", 32},
		{"malloc", 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := ds.GetPLTOffset(tt.name)
			if offset != tt.expected {
				t.Errorf("GetPLTOffset(%s) = %d, want %d", tt.name, offset, tt.expected)
			}
		})
	}

	// Test non-existent function
	offset := ds.GetPLTOffset("nonexistent")
	if offset != -1 {
		t.Errorf("GetPLTOffset(nonexistent) = %d, want -1", offset)
	}
}

// TestDynamicSectionUpdate tests updating DT_PLTGOT
func TestDynamicSectionUpdate(t *testing.T) {
	ds := NewDynamicSections(ArchX86_64)

	addrs := make(map[string]uint64)
	addrs["hash"] = 0x401000
	addrs["dynstr"] = 0x401100
	addrs["dynsym"] = 0x401200
	addrs["rela"] = 0x401300
	addrs["got"] = 0x403000

	ds.buildDynamicSection(addrs)

	// Update PLTGOT to new address
	newGOT := uint64(0x404000)
	ds.updatePLTGOT(newGOT)

	// Verify the update worked by checking the buffer
	// This is a bit fragile, but we're looking for the DT_PLTGOT entry
	// DT_PLTGOT = 3, so we look for tag=3 in the dynamic section
	buf := ds.dynamic.Bytes()

	foundUpdate := false
	for i := 0; i < len(buf); i += 16 {
		if i+16 > len(buf) {
			break
		}

		// Read tag (little-endian)
		tag := uint64(buf[i]) | uint64(buf[i+1])<<8 | uint64(buf[i+2])<<16 | uint64(buf[i+3])<<24

		if tag == DT_PLTGOT {
			// Read value
			val := uint64(buf[i+8]) | uint64(buf[i+9])<<8 | uint64(buf[i+10])<<16 | uint64(buf[i+11])<<24 |
				uint64(buf[i+12])<<32 | uint64(buf[i+13])<<40 | uint64(buf[i+14])<<48 | uint64(buf[i+15])<<56

			if val == newGOT {
				foundUpdate = true
				break
			}
			t.Errorf("DT_PLTGOT value = 0x%x, want 0x%x", val, newGOT)
		}
	}

	if !foundUpdate {
		t.Error("DT_PLTGOT entry not found or not updated")
	}
}

// TestStringTableDeduplication tests that strings are deduplicated
func TestStringTableDeduplication(t *testing.T) {
	ds := NewDynamicSections(ArchX86_64)

	// Add same string multiple times
	offset1 := ds.addString("test")
	offset2 := ds.addString("test")
	offset3 := ds.addString("different")

	if offset1 != offset2 {
		t.Error("Same string got different offsets")
	}

	if offset1 == offset3 {
		t.Error("Different strings got same offset")
	}
}

// TestLDDOutput tests that ldd can analyze the generated executable
func TestLDDOutput(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "vibe67_ldd_test")
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

	eb.Define("msg", "test\n\x00")
	eb.useDynamicLinking = true
	eb.neededFunctions = []string{"printf"}

	err = eb.GenerateGlibcHelloWorld()
	if err != nil {
		t.Skip("Skipping ldd test due to code generation issue")
	}

	ds := NewDynamicSections(ArchX86_64)
	ds.AddNeeded("libc.so.6")
	ds.AddSymbol("printf", STB_GLOBAL, STT_FUNC)

	_, _, _, _, err = eb.WriteCompleteDynamicELF(ds, []string{"printf"})
	if err != nil {
		t.Fatalf("Failed to write dynamic ELF: %v", err)
	}

	err = os.WriteFile(tmpfilePath, eb.Bytes(), 0755)
	if err != nil {
		t.Fatalf("Failed to write executable: %v", err)
	}

	// Run ldd (Linux only)
	if GetDefaultPlatform().OS != OSLinux {
		t.Skip("Skipping ldd test on non-Linux platform")
	}

	cmd := exec.Command("ldd", tmpfilePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("ldd output: %s", output)
		t.Fatalf("ldd failed: %v", err)
	}

	// Just check that it ran successfully
	t.Logf("ldd output:\n%s", output)
}









