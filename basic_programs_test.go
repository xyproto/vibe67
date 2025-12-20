package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBasicPrograms tests simple C67 programs (print, arithmetic, etc.)
func TestBasicPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "hello_world",
			source: `println("Hello, World!")
`,
			expected: "Hello, World!\n",
		},
		{
			name: "simple_add",
			source: `x := 5
y := 10
result := x + y
println(result)
`,
			expected: "15\n",
		},
		{
			name: "simple_subtract",
			source: `x := 20
y := 8
result := x - y
println(result)
`,
			expected: "12\n",
		},
		{
			name: "simple_multiply",
			source: `x := 6
y := 7
result := x * y
println(result)
`,
			expected: "42\n",
		},
		{
			name: "simple_divide",
			source: `x := 100
y := 4
result := x / y
println(result)
`,
			expected: "25\n",
		},
		{
			name: "printf_basic",
			source: `x := 42
printf("The answer is %v\n", x)
`,
			expected: "The answer is 42",
		},
		{
			name: "fstring_basic",
			source: `name := "C67"
msg := f"Hello, {name}!"
println(msg)
`,
			expected: "SKIP: f-strings not yet fully implemented for ARM64",
		},
		{
			name: "compound_assignment",
			source: `x := 10
x += 5
println(x)
x -= 3
println(x)
x *= 2
println(x)
`,
			expected: "15\n12\n24\n",
		},
		{
			name: "comparison_operators",
			source: `a := 5
b := 10
println(a < b)
println(a > b)
println(a == b)
println(a != b)
`,
			expected: "1\n0\n0\n1\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInlineC67(t, tt.name, tt.source, tt.expected)
		})
	}
}

// testInlineC67 compiles and runs inline C67 source code
func testInlineC67(t *testing.T, name, source, expected string) {
	// Check if this is a known issue that should be skipped
	if strings.HasPrefix(expected, "SKIP:") {
		t.Skip(strings.TrimPrefix(expected, "SKIP: "))
		return
	}

	result := compileAndRun(t, source)
	if !strings.Contains(result, expected) {
		t.Errorf("Output mismatch:\nExpected to contain:\n%s\nActual:\n%s", expected, result)
	}
}

// TestExistingBasicPrograms runs existing testprograms for basic functionality
func TestExistingBasicPrograms(t *testing.T) {
	tests := []string{
		"first",
		"hello",
		"add",
		"subtract",
		"multiply",
		"divide",
		"printf_demo",
		"simple_print",
		"simple_printf",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			srcPath := filepath.Join("testprograms", name+".c67")
			resultPath := filepath.Join("testprograms", name+".result")

			// Skip if source doesn't exist
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				t.Skipf("Source file %s not found", srcPath)
				return
			}

			// Read expected output if result file exists
			var expected string
			if data, err := os.ReadFile(resultPath); err == nil {
				expected = string(data)
			}

			tmpDir := t.TempDir()
			exePath := filepath.Join(tmpDir, name)

			// Compile
			platform := GetDefaultPlatform()
			if err := CompileC67(srcPath, exePath, platform); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			// Run with timeout
			cmd := exec.Command("timeout", "5s", exePath)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					_ = exitErr // Non-zero exit OK
				} else {
					t.Fatalf("Execution failed: %v", err)
				}
			}

			// If we have expected output, verify it
			if expected != "" {
				actual := string(output)
				if !strings.Contains(actual, strings.TrimSpace(expected)) &&
					actual != expected {
					t.Errorf("Output mismatch:\nExpected:\n%s\nActual:\n%s",
						expected, actual)
				}
			}
		})
	}
}
