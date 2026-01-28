package main

import (
	"testing"
)

func TestExportParsing(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectMode  string
		expectFuncs []string
	}{
		{
			name: "export all",
			code: `export *
hello = { println("Hello") }`,
			expectMode:  "*",
			expectFuncs: nil,
		},
		{
			name: "export specific functions",
			code: `export hello goodbye
hello = { println("Hello") }
goodbye = { println("Goodbye") }`,
			expectMode:  "",
			expectFuncs: []string{"hello", "goodbye"},
		},
		{
			name: "export with commas",
			code: `export add, sub, mul
add = x -> x + 1
sub = x -> x - 1
mul = x -> x * 2`,
			expectMode:  "",
			expectFuncs: []string{"add", "sub", "mul"},
		},
		{
			name:        "no export statement",
			code:        `hello = { println("Hello") }`,
			expectMode:  "",
			expectFuncs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.code)
			program := parser.ParseProgram()

			if program.ExportMode != tt.expectMode {
				t.Errorf("ExportMode = %q, want %q", program.ExportMode, tt.expectMode)
			}

			if len(program.ExportedFuncs) != len(tt.expectFuncs) {
				t.Errorf("ExportedFuncs length = %d, want %d", len(program.ExportedFuncs), len(tt.expectFuncs))
			} else {
				for i, fn := range tt.expectFuncs {
					if program.ExportedFuncs[i] != fn {
						t.Errorf("ExportedFuncs[%d] = %q, want %q", i, program.ExportedFuncs[i], fn)
					}
				}
			}
		})
	}
}
