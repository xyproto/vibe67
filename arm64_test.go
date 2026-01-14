package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestARM64BasicCompilation tests that ARM64 code can be compiled
// Note: We can't execute the binaries on Linux, but we can verify they compile and have correct structure
func TestARM64BasicCompilation(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantText int // expected minimum text size in bytes
	}{
		{
			name:     "exit_zero",
			code:     "exit(0)",
			wantText: 40, // prologue + exit syscall + epilogue
		},
		{
			name:     "exit_code",
			code:     "exit(42)",
			wantText: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "test_arm64_"+tt.name+".vibe67")
			outFile := filepath.Join(t.TempDir(), "test_arm64_"+tt.name)

			// Write test program
			if err := os.WriteFile(tmpFile, []byte(tt.code), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Compile for ARM64 macOS
			platform := Platform{Arch: ArchARM64, OS: OSDarwin}
			err := CompileC67(tmpFile, outFile, platform)
			if err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			// Verify file exists
			info, err := os.Stat(outFile)
			if err != nil {
				t.Fatalf("Output file not created: %v", err)
			}

			// Verify file is executable
			if info.Mode()&0111 == 0 {
				t.Errorf("Output file is not executable")
			}

			// Verify it's a Mach-O file with ARM64 architecture
			fileInfo, err := IdentifyFile(outFile)
			if err != nil {
				t.Fatalf("Failed to identify file: %v", err)
			}

			t.Logf("File type: %s", fileInfo.String())
			if !fileInfo.IsMachO() {
				t.Errorf("Expected Mach-O file, got: %s", fileInfo.String())
			}
			if !fileInfo.IsARM64() {
				t.Errorf("Expected ARM64 architecture, got: %s", fileInfo.String())
			}

			t.Logf("Successfully compiled %s: %s", tt.name, fileInfo.String())
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}









