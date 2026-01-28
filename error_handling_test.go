package main

import (
	"strings"
	"testing"
)

func TestOrBangOperatorHandling(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "or! with non-null pointer",
			source: `
x := 42
result := x or! {
    println("Should not print")
    99
}
printf("Result: %f\n", result)
`,
			expected: "Result: 42",
		},
		{
			name: "or! with null pointer",
			source: `
x := 0
result := x or! {
    println("Handling null")
    99
}
printf("Result: %f\n", result)
`,
			expected: "Handling null\nResult: 99",
		},
		{
			name: "or! with function call",
			source: `
get_value := x -> {
    x
}

result := get_value(5) or! {
    println("Error")
    0
}
printf("Result: %f\n", result)
`,
			expected: "Result: 5",
		},
		{
			name: "nested or! operators",
			source: `
x := 10
y := x or! { 20 }
z := y or! { 30 }
printf("z = %f\n", z)
`,
			expected: "z = 10",
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

func TestDeferStatementExecution(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "single defer",
			source: `
println("Start")
defer println("Deferred")
println("End")
`,
			expected: "Start\nEnd\nDeferred",
		},
		{
			name: "multiple defers LIFO",
			source: `
defer println("First defer")
defer println("Second defer")
defer println("Third defer")
println("Main")
`,
			expected: "Main\nThird defer\nSecond defer\nFirst defer",
		},
		{
			name: "defer with function",
			source: `
cleanup := -> {
    println("Cleaning up")
}
defer cleanup()
println("Working")
`,
			expected: "Working\nCleaning up",
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

func TestMatchExpressionsExtended(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple value match",
			source: `
x := 42
result := x {
    42 -> 1
    ~> 0
}
println(result)
`,
			expected: "1",
		},
		{
			name: "value match with default",
			source: `
y := 99
result := y {
    42 -> 1
    ~> 0
}
println(result)
`,
			expected: "0",
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

func TestTypeAnnotationsExtended(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "num annotation",
			source: `
x: num = 42
printf("x = %f\n", x)
`,
			expected: "x = 42",
		},
		{
			name: "str annotation",
			source: `
s: str = "hello"
println(s)
`,
			expected: "hello",
		},
		{
			name: "list annotation",
			source: `
lst: list = [1, 2, 3]
printf("First: %f\n", lst[0])
`,
			expected: "First: 1",
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

func TestBlockTypesExtended(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple lambda",
			source: `
double := x -> x * 2
printf("Result: %f\n", double(5))
`,
			expected: "Result: 10",
		},
		{
			name: "two parameter lambda",
			source: `
add := (a, b) -> a + b
printf("Sum: %f\n", add(3, 4))
`,
			expected: "Sum: 7",
		},
		{
			name: "lambda with expression",
			source: `
compute := x -> x * 2 + 10
printf("Result: %f\n", compute(5))
`,
			expected: "Result: 20",
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
