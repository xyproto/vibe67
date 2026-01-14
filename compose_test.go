package main

import (
	"strings"
	"testing"
)

func TestFunctionComposition(t *testing.T) {
	t.Skip("Function composition <> operator not yet fully implemented - use explicit lambda for now")
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "basic composition",
			source: `
double := x -> x * 2
add_ten := x -> x + 10

// compose creates: x -> double(add_ten(x))
compose := double <> add_ten
result := compose(5)
printf("Result: %f\n", result)
`,
			expected: "Result: 30",
		},
		{
			name: "triple composition",
			source: `
triple := x -> x * 3
double := x -> x * 2
add_five := x -> x + 5

// Right-associative: triple <> double <> add_five
// Evaluates as: x -> triple(double(add_five(x)))
transform := triple <> double <> add_five
value := transform(10)
printf("Value: %f\n", value)
`,
			expected: "Value: 90",
		},
		{
			name: "composition with different operations",
			source: `
square := x -> x * x
increment := x -> x + 1

// x -> square(increment(x))
f := square <> increment
printf("f(4) = %f\n", f(4))
`,
			expected: "f(4) = 25",
		},
		{
			name: "composition order matters",
			source: `
double := x -> x * 2
add_ten := x -> x + 10

// These produce different results
compose1 := double <> add_ten  // x -> double(add_ten(x))
compose2 := add_ten <> double  // x -> add_ten(double(x))

result1 := compose1(5)  // (5 + 10) * 2 = 30
result2 := compose2(5)  // (5 * 2) + 10 = 20

printf("compose1(5) = %f\n", result1)
printf("compose2(5) = %f\n", result2)
`,
			expected: "compose1(5) = 30\ncompose2(5) = 20",
		},
		{
			name: "multiple independent compositions",
			source: `
add_one := x -> x + 1
add_two := x -> x + 2
double := x -> x * 2

f := add_one <> double  // x -> add_one(double(x)) = 2x + 1
g := add_two <> double  // x -> add_two(double(x)) = 2x + 2

printf("f(3) = %f\n", f(3))
printf("g(3) = %f\n", g(3))
`,
			expected: "f(3) = 7\ng(3) = 8",
		},
		{
			name: "composition with arithmetic",
			source: `
negate := x -> -x
abs_value := x -> {
    | x < 0 -> -x
    ~> x
}

// Negate then abs: always gives positive
f := abs_value <> negate
printf("f(-5) = %f\n", f(-5))
printf("f(5) = %f\n", f(5))
`,
			expected: "f(-5) = 5\nf(5) = 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := compileAndRun(t, tt.source)
			output = strings.TrimSpace(output)
			expected := strings.TrimSpace(tt.expected)

			// Check if all expected lines are present
			expectedLines := strings.Split(expected, "\n")
			for _, line := range expectedLines {
				if !strings.Contains(output, line) {
					t.Errorf("Expected output to contain:\n%s\n\nGot:\n%s", expected, output)
					break
				}
			}
		})
	}
}

func TestCompositionRightAssociative(t *testing.T) {
	t.Skip("Function composition <> operator not yet fully implemented")
	source := `
f := x -> x + 1
g := x -> x * 2
h := x -> x - 3

// Right-associative: f <> g <> h means f <> (g <> h)
// Which evaluates as: x -> f(g(h(x)))
composed := f <> g <> h

// Test: h(10) = 7, g(7) = 14, f(14) = 15
result := composed(10)
printf("Result: %f\n", result)
`
	output := compileAndRun(t, source)
	if !strings.Contains(output, "Result: 15") {
		t.Errorf("Expected Result: 15, got: %s", output)
	}
}

func TestCompositionWithVariables(t *testing.T) {
	t.Skip("Function composition <> operator not yet fully implemented")
	source := `
// Store compositions in variables
add_one := x -> x + 1
double := x -> x * 2

step1 := add_one <> double  // x -> (2x) + 1
step2 := double <> add_one  // x -> 2(x + 1)

a := step1(5)  // 2*5 + 1 = 11
b := step2(5)  // 2*(5+1) = 12

printf("step1(5) = %f\n", a)
printf("step2(5) = %f\n", b)
`
	output := compileAndRun(t, source)
	if !strings.Contains(output, "step1(5) = 11") || !strings.Contains(output, "step2(5) = 12") {
		t.Errorf("Composition with variables failed, got: %s", output)
	}
}









