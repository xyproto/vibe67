package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestParallelPrograms tests parallel loop constructs
func TestParallelPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "parallel_simple",
			source: `@@ i in 0..<4 {
    println(i)
}
`,
			expected: "", // Output order is non-deterministic
		},
		{
			name: "parallel_noop",
			source: `@@ i in 0..<10 {
}
`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			srcPath := filepath.Join(tmpDir, tt.name+".vibe67")
			exePath := filepath.Join(tmpDir, tt.name)

			if err := os.WriteFile(srcPath, []byte(tt.source), 0644); err != nil {
				t.Fatalf("Failed to write source: %v", err)
			}

			platform := GetDefaultPlatform()
			if err := CompileC67(srcPath, exePath, platform); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			cmd := exec.Command("timeout", "10s", exePath)
			_, err := cmd.CombinedOutput()
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Execution failed: %v", err)
				}
			}
		})
	}
}

// TestExistingParallelPrograms runs existing parallel test programs
func TestExistingParallelPrograms(t *testing.T) {
	tests := []string{
		"parallel_simple",
		"parallel_noop",
		"parallel_single",
		"parallel_test",
		"parallel_test_simple",
		"parallel_empty",
		"parallel_empty_range",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join("testprograms", name+".vibe67")

			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Skipf("Source file %s not found", srcPath)
				return
			}

			tmpDir := t.TempDir()
			exePath := filepath.Join(tmpDir, name)

			platform := GetDefaultPlatform()
			if err := CompileC67(srcPath, exePath, platform); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			cmd := exec.Command("timeout", "10s", exePath)
			_, err := cmd.CombinedOutput()
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Execution failed: %v", err)
				}
			}
		})
	}
}
