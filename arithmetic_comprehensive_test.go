package main

import (
	"strings"
	"testing"
)

// TestArithmeticOperations tests all arithmetic operations comprehensively
func TestArithmeticOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "addition",
			source:   "x := 10 + 5\nprintln(x)\n",
			expected: "15\n",
		},
		{
			name:     "subtraction",
			source:   "x := 10 - 3\nprintln(x)\n",
			expected: "7\n",
		},
		{
			name:     "multiplication",
			source:   "x := 6 * 7\nprintln(x)\n",
			expected: "42\n",
		},
		{
			name:     "division",
			source:   "x := 20 / 4\nprintln(x)\n",
			expected: "5\n",
		},
		{
			name:     "power",
			source:   "x := 2 ** 3\nprintln(x)\n",
			expected: "8\n",
		},
		{
			name:     "modulo",
			source:   "x := 10 % 3\nprintln(x)\n",
			expected: "1\n",
		},
		{
			name:     "negative",
			source:   "x := -42\nprintln(x)\n",
			expected: "-42\n",
		},
		{
			name:     "compound_expression",
			source:   "x := (2 + 3) * 4\nprintln(x)\n",
			expected: "20\n",
		},
		{
			name:     "float_division",
			source:   "x := 10 / 3\nprintf(\"%.2f\\n\", x)\n",
			expected: "3.33\n",
		},
		{
			name:     "power_fractional",
			source:   "x := 16 ** 0.5\nprintln(x)\n",
			expected: "4\n",
		},
		{
			name:     "caret_as_power",
			source:   "x := 2 ^ 3\nprintln(x)\n",
			expected: "8\n",
		},
		{
			name:     "caret_precedence",
			source:   "x := 2 + 3 ^ 2\nprintln(x)\n",
			expected: "11\n",
		},
		{
			name:     "caret_vs_double_star",
			source:   "a := 5 ^ 2\nb := 5 ** 2\nprintln(a)\nprintln(b)\n",
			expected: "25\n25\n",
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

// TestComparisonOperations tests all comparison operations
func TestComparisonOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "equal",
			source: `x := 5
y := 5
x == y {
    => println("equal")
    ~> println("not equal")
}
`,
			expected: "equal\n",
		},
		{
			name: "not_equal",
			source: `x := 5
y := 3
x != y {
    => println("not equal")
    ~> println("equal")
}
`,
			expected: "not equal\n",
		},
		{
			name: "less_than",
			source: `x := 3
y := 5
x < y {
    => println("less")
    ~> println("not less")
}
`,
			expected: "less\n",
		},
		{
			name: "greater_than",
			source: `x := 10
y := 5
x > y {
    => println("greater")
    ~> println("not greater")
}
`,
			expected: "greater\n",
		},
		{
			name: "less_or_equal",
			source: `x := 5
y := 5
x <= y {
    => println("le")
    ~> println("not le")
}
`,
			expected: "le\n",
		},
		{
			name: "greater_or_equal",
			source: `x := 7
y := 5
x >= y {
    => println("ge")
    ~> println("not ge")
}
`,
			expected: "ge\n",
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

// TestLogicalOperations tests logical operators
func TestLogicalOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "and_true",
			source: `x := 1.0 and 1.0
println(x)
`,
			expected: "1\n",
		},
		{
			name: "and_false",
			source: `x := 1.0 and 0.0
println(x)
`,
			expected: "0\n",
		},
		{
			name: "or_true",
			source: `x := 0.0 or 1.0
println(x)
`,
			expected: "1\n",
		},
		{
			name: "or_false",
			source: `x := 0.0 or 0.0
println(x)
`,
			expected: "0\n",
		},
		{
			name: "not_true",
			source: `x := not(0.0)
println(x)
`,
			expected: "1\n",
		},
		{
			name: "not_false",
			source: `x := not(1.0)
println(x)
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

// TestBitwiseOperations tests bitwise operators
func TestBitwiseOperations(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "bitwise_and",
			source: `x := 12 &b 10
println(x)
`,
			expected: "8\n",
		},
		{
			name: "bitwise_or",
			source: `x := 12 |b 10
println(x)
`,
			expected: "14\n",
		},
		{
			name: "bitwise_xor",
			source: `x := 12 ^b 10
println(x)
`,
			expected: "6\n",
		},
		{
			name: "bitwise_not",
			source: `x := ~b 0
println(x)
`,
			expected: "-1\n",
		},
		{
			name: "shift_left",
			source: `x := 5 <<b 2
println(x)
`,
			expected: "20\n",
		},
		{
			name: "shift_right",
			source: `x := 20 >>b 2
println(x)
`,
			expected: "5\n",
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









