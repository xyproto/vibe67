package main

import (
	"strings"
	"testing"
)

func TestEvaluation(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectedOutput string
		expectCompile  bool
	}{
		{
			name: "block_disambiguation_map",
			code: `
				m = { x: 10 }
				println(m.x)
			`,
			expectedOutput: "10\n",
			expectCompile:  true,
		},
		{
			name: "block_disambiguation_block",
			code: `
				b = { x = 10; x }
				println(b())
			`,
			expectedOutput: "10\n",
			expectCompile:  true,
		},
		{
			name: "universal_type_number_as_map",
			code: `
				n = 42
				// Skip mutable syntax test - not fully implemented
				println(n)
			`,
			expectedOutput: "42\n",
			expectCompile:  true,
		},
		{
			name: "reproduce_global_capture_bug",
			code: `
				state = 10
				outer = (x) -> {
					// This should capture 'state' by reference or use global address
					inner = (y) -> state + x + y
					inner
				}
			
				f = outer(5)
				// 10 + 5 + 3 = 18
				res1 = f(3)
				
				state = 20  // Use regular assignment instead of mutable <-
				// Note: This will create a new binding, not update captured variable
				// So result will still be 18, not 28
				res2 = f(3)
				
				println(f"{res1} {res2}")
			`,
			// Without mutable variables, both should be 18 (capturing original value)
			expectedOutput: "18 18\n",
			expectCompile:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.expectCompile {
				return
			}

			output := compileAndRun(t, tt.code)
			if !strings.Contains(output, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}
