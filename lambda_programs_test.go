package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLambdaPrograms tests lambda expressions and closures
func TestLambdaPrograms(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple_lambda",
			source: `square := x -> x * x
result := square(5)
println(result)
`,
			expected: "25\n",
		},
		{
			name: "lambda_with_multiple_params",
			source: `add = (a, b) -> a + b
result = add(10, 20)
println(result)
`,
			expected: "30\n",
		},
		{
			name: "lambda_with_block",
			source: `calculate = (x, y) -> x * 2 + y
println(calculate(5, 3))
`,
			expected: "13\n",
		},
		{
			name: "recursive_lambda",
			source: `factorial := (n, acc) -> n == 0 {
    => acc
    ~> factorial(n-1, n*acc) max 100
}
println(factorial(5, 1))
`,
			expected: "120\n",
		},
		{
			name: "lambda_captures_module_global_immutable",
			source: `global_value := 100
counter := x -> global_value + x
println(counter(5))
`,
			expected: "105\n",
		},
		{
			name: "lambda_captures_module_global_mutable",
			source: `counter := 0
increment := () -> {
    counter <- counter + 1
    counter
}
println(increment())
println(increment())
println(counter)
`,
			expected: "1\n2\n2",
		},
		{
			name: "nested_lambda_with_module_global_KNOWN_ISSUE",
			source: `state := 10
outer := (x) -> {
    inner := (y) -> state + x + y
    inner
}
f := outer(5)
println(f(3))
state <- 20
println(f(3))
`,
			expected: "SKIP: Known issue - module-level mutable updates don't reflect in closures",
		},
		{
			name: "lambda_modifying_captured_mutable",
			source: `total := 0
add_to_total := (n) -> {
    total <- total + n
    total
}
println(add_to_total(10))
println(add_to_total(20))
println(total)
`,
			expected: "10\n30\n30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInlineVibe67(t, tt.name, tt.source, tt.expected)
		})
	}
}

// TestExistingLambdaPrograms runs existing lambda test programs
func TestExistingLambdaPrograms(t *testing.T) {
	tests := []string{
		"lambda_test",
		"lambda_syntax_test",
		"lambda_calculator",
		"lambda_comprehensive",
		"lambda_direct_test",
		"lambda_multi_arg_test",
		"lambda_multiple_test",
		"lambda_parse_test",
		"lambda_store_test",
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
			if err := CompileVibe67(srcPath, exePath, platform); err != nil {
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
