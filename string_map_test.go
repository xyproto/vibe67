package main

import (
	"strings"
	"testing"
)

// TestErrorPropertyDebug tests basic .error property
func TestErrorPropertyDebug(t *testing.T) {
	source := `println("start")
x := 5.0
println("before")
y := x.error
println("after")
`
	result := compileAndRun(t, source)
	// Check that it compiles and runs
	t.Logf("Output: %q", result)
}

// TestBuiltinIsError removed - is_error is not a builtin in Vibe67
// Use or! operator instead for error handling

// TestPropertyAccessOnNumber tests accessing any property on a number
func TestPropertyAccessOnNumber(t *testing.T) {
	source := `x := 5.0
y := x.foo
println("ok")
`
	result := compileAndRun(t, source)
	t.Logf("Output: %q", result)
}

// TestStringOperations tests string handling
func TestStringOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "string_literal",
			source:   `println("Hello, World!")`,
			expected: "Hello, World!\n",
		},
		{
			name: "string_variable",
			source: `msg := "Test"
println(msg)
`,
			expected: "Test\n",
		},
		{
			name: "string_concatenation",
			source: `a := "Hello"
b := " World"
c := a + b
println(c)
`,
			expected: "Hello World\n",
		},
		{
			name: "string_length",
			source: `msg := "Hello"
len := #msg
println(len)
`,
			expected: "5\n",
		},
		{
			name: "empty_string",
			source: `msg := ""
println(msg)
println("done")
`,
			expected: "\ndone\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.source)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected output to contain: %s, got: %s", tt.expected, result)
			}
		})
	}
}

// TestMapOperations tests map/dictionary handling
func TestMapOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "map_create_access",
			source: `m := {0: 10, 1: 20, 2: 30}
println(m[0])
println(m[1])
println(m[2])
`,
			expected: "10\n20\n30\n",
		},
		{
			name: "map_length",
			source: `m := {0: 1, 1: 2, 2: 3}
len := #m
println(len)
`,
			expected: "3\n",
		},
		{
			name: "map_update",
			source: `m := {0: 10}
m[0] <- 20
println(m[0])
`,
			expected: "20\n",
		},
		{
			name: "empty_map",
			source: `m := {}
len := #m
println(len)
`,
			expected: "0\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.source)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected output to contain: %s, got: %s", tt.expected, result)
			}
		})
	}
}

// TestListOperationsComprehensive tests list operations
func TestListOperationsComprehensive(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "list_create_access",
			source: `lst := [10, 20, 30]
println(lst[0])
println(lst[1])
println(lst[2])
`,
			expected: "10\n20\n30\n",
		},
		{
			name: "list_length",
			source: `lst := [1, 2, 3, 4, 5]
len := #lst
println(len)
`,
			expected: "5\n",
		},
		{
			name: "empty_list",
			source: `lst := []
len := #lst
println(len)
`,
			expected: "0\n",
		},
		{
			name: "list_update",
			source: `lst := [10, 20, 30]
lst[1] <- 99
println(lst[0])
println(lst[1])
println(lst[2])
`,
			expected: "10\n99\n30\n",
		},
		// Removed: list_cons test - cons operator (::) has been removed
		// Note: Use ^ for head and _ for tail operators (no head() or tail() functions)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.source)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected output to contain: %s, got: %s", tt.expected, result)
			}
		})
	}
}

// TestPrintfFormatting tests printf with various format specifiers
func TestPrintfFormatting(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "printf_integer",
			source:   `printf("%d\\n", 42)`,
			expected: "42\n",
		},
		{
			name:     "printf_float",
			source:   `printf("%.2f\\n", 3.14159)`,
			expected: "3.14\n",
		},
		{
			name:     "printf_string",
			source:   `printf("%s\\n", "hello")`,
			expected: "hello\n",
		},
		{
			name:     "printf_multiple",
			source:   `printf("%d + %d = %d\\n", 2, 3, 5)`,
			expected: "2 + 3 = 5\n",
		},
		{
			name:     "printf_boolean",
			source:   `printf("%v\\n", 1.0)`,
			expected: "1.0", // %v prints floats with decimal point
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.source)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected output to contain: %s, got: %s", tt.expected, result)
			}
		})
	}
}









