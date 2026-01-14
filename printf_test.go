package main

import (
	"testing"
)

// TestPrintfWithStringLiteral tests printf with string literals
func TestPrintfWithStringLiteral(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "simple string",
			code:     `printf("Hello, World!\n")`,
			expected: "Hello, World!\n",
		},
		{
			name:     "string with %s format",
			code:     `printf("Test: %s\n", "hello")`,
			expected: "Test: hello\n",
		},
		{
			name:     "number with %g format",
			code:     `printf("Number: %.15g\n", 42)`,
			expected: "Number: 42.000000000000000\n",
		},
		{
			name:     "boolean with %b format",
			code:     `printf("Bool: %b\n", 1)`,
			expected: "Bool: yes\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compileAndRun(t, tt.code)
			if got != tt.expected {
				t.Errorf("Output mismatch:\nGot:      %q\nExpected: %q", got, tt.expected)
			}
		})
	}
}









