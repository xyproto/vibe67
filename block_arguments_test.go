package main

import (
	"strings"
	"testing"
)

// TestBlockArgumentsCurrentState documents what works and what doesn't
// with passing blocks as arguments
func TestBlockArgumentsCurrentState(t *testing.T) {
	t.Run("pipeline with lambda - not fully implemented", func(t *testing.T) {
		t.Skip("Pipeline operator with lambdas not fully implemented yet")
		source := `
numbers := [1, 2, 3, 4, 5]
doubled := numbers | x -> x * 2
printf("First doubled: %f\n", doubled[0])
`
		output := compileAndRun(t, source)
		if !strings.Contains(output, "First doubled: 2") {
			t.Errorf("Expected pipeline to work, got: %s", output)
		}
	})

	t.Run("lambda stored in variable", func(t *testing.T) {
		source := `
add_ten := x -> x + 10
result := add_ten(5)
printf("Result: %f\n", result)
`
		output := compileAndRun(t, source)
		if !strings.Contains(output, "Result: 15") {
			t.Errorf("Expected lambda call to work, got: %s", output)
		}
	})

	t.Run("multiple lambdas", func(t *testing.T) {
		source := `
double := x -> x * 2
add_five := x -> x + 5
result := add_five(double(10))
printf("Result: %f\n", result)
`
		output := compileAndRun(t, source)
		if !strings.Contains(output, "Result: 25") {
			t.Errorf("Expected composed lambdas to work, got: %s", output)
		}
	})

	// This currently doesn't work - documenting for future implementation
	t.Run("passing lambda as parameter - not yet supported", func(t *testing.T) {
		t.Skip("Calling lambda parameters not yet supported")
		source := `
apply := (f, x) -> f(x)
double := x -> x * 2
result := apply(double, 5)
printf("Result: %f\n", result)
`
		output := compileAndRun(t, source)
		if !strings.Contains(output, "Result: 10") {
			t.Errorf("Expected higher-order function to work, got: %s", output)
		}
	})

	// This currently doesn't work - documenting for future implementation
	t.Run("block literal as argument - not yet supported", func(t *testing.T) {
		t.Skip("Block literals as arguments not yet supported")
		source := `
apply := f -> f()
apply({ println("Hello from block!") })
`
		output := compileAndRun(t, source)
		if !strings.Contains(output, "Hello from block!") {
			t.Errorf("Expected block argument to work, got: %s", output)
		}
	})
}

// TestPipelineOperator - documenting that pipeline with lambdas needs implementation
func TestPipelineOperator(t *testing.T) {
	t.Skip("Pipeline operator with lambdas not yet fully implemented")
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name: "simple map",
			source: `
numbers := [1, 2, 3]
doubled := numbers | x -> x * 2
printf("Result: %f %f %f\n", doubled[0], doubled[1], doubled[2])
`,
			expected: "Result: 2 4 6",
		},
		{
			name: "chained pipelines",
			source: `
numbers := [1, 2, 3]
result := numbers | x -> x * 2 | y -> y + 1
printf("First: %f\n", result[0])
`,
			expected: "First: 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := compileAndRun(t, tt.source)
			output = strings.TrimSpace(output)
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain:\n%s\n\nGot:\n%s", tt.expected, output)
			}
		})
	}
}
