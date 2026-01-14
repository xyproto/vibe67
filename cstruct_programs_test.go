package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCStructPrograms tests C struct interop
func TestCStructPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple_cstruct",
			source: `cstruct Point {
    x as float64,
    y as float64
}

println(Point.size)
println(Point.x.offset)
println(Point.y.offset)
`,
			expected: "16\n0\n8\n",
		},
		{
			name: "packed_cstruct",
			source: `cstruct Data packed {
    a as uint8,
    b as uint32,
    c as uint8
}

println(Data.size)
`,
			expected: "6\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInlineVibe67(t, tt.name, tt.source, tt.expected)
		})
	}
}

// TestExistingCStructPrograms runs existing cstruct test programs
func TestExistingCStructPrograms(t *testing.T) {
	tests := []string{
		"cstruct_test",
		"cstruct_syntax_test",
		"cstruct_helpers_test",
		"cstruct_modifiers_test",
		"cstruct_arena_test",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join("testprograms", name+".vibe67")
			resultPath := filepath.Join("testprograms", name+".result")

			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Skipf("Source file %s not found", srcPath)
				return
			}

			var expected string
			if data, err := os.ReadFile(resultPath); err == nil {
				expected = string(data)
			}

			tmpDir := t.TempDir()
			exePath := filepath.Join(tmpDir, name)

			platform := GetDefaultPlatform()
			if err := CompileC67(srcPath, exePath, platform); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			cmd := exec.Command("timeout", "5s", exePath)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Execution failed: %v", err)
				}
			}

			if expected != "" {
				actual := string(output)
				if actual != expected {
					t.Errorf("Output mismatch:\nExpected:\n%s\nActual:\n%s",
						expected, actual)
				}
			}
		})
	}
}









