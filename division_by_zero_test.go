package main

import (
	"strings"
	"testing"
)

// TestDivisionByZeroCompleteCoverage tests that all division paths have proper zero checks
func TestDivisionByZeroCompleteCoverage(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple division by zero literal",
			source: `
result := 10 / 0
safe := result or! -999.0
printf("Result: %f\n", safe)
`,
			expected: "-999",
		},
		{
			name: "division by zero variable",
			source: `
divisor := 0
result := 100 / divisor
safe := result or! -1.0
printf("Result: %f\n", safe)
`,
			expected: "-1",
		},
		{
			name: "division in function call",
			source: `
divide := (a, b) -> a / b
result := divide(50, 0)
safe := result or! 42.0
printf("Result: %f\n", safe)
`,
			expected: "42",
		},
		{
			name: "division in expression",
			source: `
x := 10
y := 0
result := (x * 2) / y
safe := result or! -1.0
printf("Result: %f\n", safe)
`,
			expected: "-1",
		},
		{
			name: "chained division operations",
			source: `
a := 100
b := 0
c := 5
// First division by zero should trigger error
result := a / b / c
safe := result or! -999.0
printf("Result: %f\n", safe)
`,
			expected: "-999",
		},
		{
			name: "division by zero in match expression",
			source: `
x := 0
result := x {
    0 -> 10 / x
    ~> 42
}
safe := result or! -1.0
printf("Result: %f\n", safe)
`,
			expected: "-1",
		},
		{
			name: "normal division still works",
			source: `
result := 100 / 4
printf("Result: %f\n", result)
`,
			expected: "25",
		},
		{
			name: "division with or! block",
			source: `
result := 10 / 0 or! {
    println("Caught division by zero")
    99
}
printf("Result: %f\n", result)
`,
			expected: "Caught division by zero\nResult: 99",
		},
		{
			name: "error accessor extracts dv0 code",
			source: `
result := 10 / 0
code := result.error
printf("Error code: %s\n", code)
`,
			expected: "dv0",
		},
		{
			name: "modulo by zero also checked",
			source: `
result := 10 % 0
safe := result or! -1.0
printf("Result: %f\n", safe)
`,
			expected: "-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := compileAndRun(t, tt.source)
			output = strings.TrimSpace(output)
			expected := strings.TrimSpace(tt.expected)
			if !strings.Contains(output, expected) {
				t.Errorf("Expected output to contain:\n%s\n\nGot:\n%s", expected, output)
			}
		})
	}
}

// TestDivisionByZeroDoesNotPanic ensures division by zero returns NaN, not panic
func TestDivisionByZeroDoesNotPanic(t *testing.T) {
	source := `
x := 42 / 0
y := x or! 100
println(y)
`
	// Should not panic, should output 100
	output := compileAndRun(t, source)
	if !strings.Contains(output, "100") {
		t.Errorf("Expected output to contain 100, got: %s", output)
	}
}

// TestDivisionByZeroInUnsafeBlock tests that unsafe blocks may skip checks (documented behavior)
func TestDivisionByZeroInSafeContext(t *testing.T) {
	// Note: This tests SAFE context, not unsafe blocks
	// Unsafe blocks may skip checks by design (needs documentation)
	source := `
safe_divide := (a, b) -> {
    result := a / b
    result or! -1.0
}
println(safe_divide(10, 2))
println(safe_divide(10, 0))
`
	output := compileAndRun(t, source)
	if !strings.Contains(output, "5") {
		t.Errorf("Expected output to contain 5 (normal division), got: %s", output)
	}
	if !strings.Contains(output, "-1") {
		t.Errorf("Expected output to contain -1 (division by zero fallback), got: %s", output)
	}
}
