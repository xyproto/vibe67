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
				println(b)
			`,
			expectedOutput: "10\n",
			expectCompile:  true,
		},
		{
			name: "universal_type_number_as_map",
			code: `
				n := 42
				// Numbers are maps {0: value}
				// We can add keys if mutable
				n[1] <- 100
				// println uses default formatting which might vary for maps vs numbers
				// so let's be specific
				println(f"{n[0]} {n[1]}")
			`,
			expectedOutput: "42 100\n",
			expectCompile:  true,
		},
		{
			name: "reproduce_global_capture_bug",
			code: `
				state := 10
				outer := (x) -> {
					// This should capture 'state' by reference or use global address
					inner := (y) -> state + x + y
					inner
				}
			
				f := outer(5)
				// 10 + 5 + 3 = 18
				res1 = f(3)
				
				state <- 20
				// 20 + 5 + 3 = 28
				res2 = f(3)
				
				println(f"{res1} {res2}")
			`,
			// If bug is present (capture by value), output will be "18 18"
			// If fixed, output will be "18 28"
			// We assert the "correct" behavior to see if it fails
			expectedOutput: "18 28\n",
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