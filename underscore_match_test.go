package main

import (
	"strings"
	"testing"
)

// TestUnderscoreDefaultMatch tests _ => as alias for ~>
func TestUnderscoreDefaultMatch(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "underscore_default_value_match",
			source: `x := 7
x {
    0 => println("zero")
    5 => println("five")
    _ => println("other")
}
`,
			expected: "other\n",
		},
		{
			name: "underscore_default_boolean_match",
			source: `x := 5
y := 3
x == y {
    => println("equal")
    _ => println("not equal")
}
`,
			expected: "not equal\n",
		},
		{
			name: "both_syntaxes_equivalent",
			source: `x := 1
x {
    0 => println("zero")
    ~> println("tilde default")
}
x {
    0 => println("zero")
    _ => println("underscore default")
}
`,
			expected: "tilde default\nunderscore default\n",
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
