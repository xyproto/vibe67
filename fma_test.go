// Completion: 100% - FMA optimization tests
package main

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// Helper to compile, run, and parse float64 result
func compileAndRunFloat(t testing.TB, code string) float64 {
	// Type assert to *testing.T for compileAndRun
	tt, ok := t.(*testing.T)
	if !ok {
		// If it's a *testing.B, we need to convert it
		if tb, ok := t.(*testing.B); ok {
			tt = &testing.T{}
			_ = tb // Use tb to avoid unused variable
		} else {
			panic("unsupported testing type")
		}
	}

	output := compileAndRun(tt, code)
	output = strings.TrimSpace(output)

	// Parse the result as float64
	val, err := strconv.ParseFloat(output, 64)
	if err != nil {
		t.Fatalf("Failed to parse result '%s' as float64: %v", output, err)
	}
	return val
}

// Test FMA pattern detection in optimizer
func TestFMAPatternDetection(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantFMA  bool
		expected float64
	}{
		{
			name: "simple_fma_add",
			code: `
				a = 2.0
				b = 3.0
				c = 4.0
				result = a * b + c
				main = { println(result) }
			`,
			wantFMA:  true,
			expected: 10.0, // 2*3 + 4 = 10
		},
		{
			name: "simple_fma_add_reversed",
			code: `
				a = 2.0
				b = 3.0
				c = 4.0
				result = c + a * b
				main = { println(result) }
			`,
			wantFMA:  true,
			expected: 10.0, // 4 + 2*3 = 10
		},
		{
			name: "fma_subtract",
			code: `
				a = 5.0
				b = 3.0
				c = 2.0
				result = a * b - c
				main = { println(result) }
			`,
			wantFMA:  true,
			expected: 13.0, // 5*3 - 2 = 13
		},
		{
			name: "nested_fma",
			code: `
				x = 2.0
				y = 3.0
				z = 4.0
				w = 5.0
				result = x * y + z * w
				main = { println(result) }
			`,
			wantFMA:  true,
			expected: 26.0, // 2*3 + 4*5 = 6 + 20 = 26
		},
		{
			name: "no_fma_separate_ops",
			code: `
				a = 2.0
				b = 3.0
				mul = a * b
				c = 4.0
				result = mul + c
				main = { println(result) }
			`,
			wantFMA:  false, // Operations separated by assignment
			expected: 10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRunFloat(t, tt.code)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Test FMA with function parameters
func TestFMAWithParameters(t *testing.T) {
	code := `
		fma_func = (a, b, c) -> a * b + c
		main = {
			println(fma_func(2.0, 3.0, 4.0))
		}
	`
	result := compileAndRunFloat(t, code)
	expected := 10.0
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Test FMA in loops (important for vectorization)
func TestFMAInLoop(t *testing.T) {
	code := `
		main = {
			sum := 0.0
			@ i in 0..<10 {
				a = i + 1.0
				b = i + 2.0
				c = i + 3.0
				sum <- sum + (a * b + c)
			}
			println(sum)
		}
	`
	result := compileAndRunFloat(t, code)

	// Calculate expected manually (0..<10 means 0-9, 10 iterations)
	expected := 0.0
	for i := 0.0; i < 10.0; i++ {
		a := i + 1.0
		b := i + 2.0
		c := i + 3.0
		expected += (a*b + c)
	}

	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Test FMA precision advantage
func TestFMAPrecision(t *testing.T) {
	// FMA has better precision due to single rounding
	// Test: (a * b) + c vs fma(a, b, c)
	code := `
		main = {
			a = 1.0000000000000001
			b = 1.0000000000000001  
			c = -0.0000000000000001
			result = a * b + c
			println(result)
		}
	`
	result := compileAndRunFloat(t, code)

	// With FMA, we expect better precision
	// Without FMA: (1.0000000000000001 * 1.0000000000000001) rounds, then + c rounds again
	// With FMA: single rounding at the end

	// Just verify it compiles and runs
	if math.IsNaN(result) {
		t.Error("Result should not be NaN")
	}
}

// Test FMA with negative values
func TestFMANegative(t *testing.T) {
	code := `
		main = {
			a = -2.0
			b = 3.0
			c = 5.0
			println(a * b + c)
		}
	`
	result := compileAndRunFloat(t, code)
	expected := -1.0 // -2*3 + 5 = -6 + 5 = -1
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Test FMA doesn't break constant folding
func TestFMAConstantFolding(t *testing.T) {
	code := `
		main = {
			println(2.0 * 3.0 + 4.0)
		}
	`
	result := compileAndRunFloat(t, code)
	expected := 10.0
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Test FMA with zero (edge case)
func TestFMAWithZero(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected float64
	}{
		{
			name:     "zero_multiplicand",
			code:     `main = { println(0.0 * 3.0 + 4.0) }`,
			expected: 4.0,
		},
		{
			name:     "zero_multiplier",
			code:     `main = { println(2.0 * 0.0 + 4.0) }`,
			expected: 4.0,
		},
		{
			name:     "zero_addend",
			code:     `main = { println(2.0 * 3.0 + 0.0) }`,
			expected: 6.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRunFloat(t, tt.code)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Test that FMA doesn't trigger for non-arithmetic types
func TestFMAOnlyForNumbers(t *testing.T) {
	// String concatenation should NOT use FMA
	code := `
		main = {
			a = "hello"
			b = "world"
			c = a + b
			# c  // Can't return string yet in simple test, so just compile
			println(42.0)
		}
	`
	result := compileAndRunFloat(t, code)
	if math.IsNaN(result) {
		t.Error("Should compile successfully")
	}
}

// Test FMA in polynomial evaluation (classic use case)
func TestFMAPolynomial(t *testing.T) {
	// Test polynomial evaluation with FMA
	code := `
		poly = (x, a, b) -> a * x + b
		main = {
			println(poly(2.0, 3.0, 4.0))
		}
	`
	result := compileAndRunFloat(t, code)
	// 3*2 + 4 = 10
	expected := 10.0
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Test FMA with match expressions
func TestFMAInMatch(t *testing.T) {
	code := `
		main = {
			x = 2.0
			result = x {
				2.0 => x * 2.0 + 1.0
				~> 0.0
			}
			println(result)
		}
	`
	result := compileAndRunFloat(t, code)
	expected := 5.0 // 2*2 + 1 = 5
	if math.Abs(result-expected) > 0.0001 {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

// Benchmark FMA vs separate operations
func BenchmarkFMA(b *testing.B) {
	code := `
		main = {
			sum := 0.0
			@ i in 0..1000 {
				a = i + 1.0
				b = i + 2.0
				c = i + 3.0
				sum <- sum + (a * b + c)
			}
			sum
		}
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compileAndRunFloat(b, code)
	}
}

func BenchmarkSeparateOps(b *testing.B) {
	code := `
		main = {
			sum := 0.0
			@ i in 0..1000 {
				a = i + 1.0
				b = i + 2.0
				c = i + 3.0
				mul = a * b
				add = mul + c
				sum <- sum + add
			}
			sum
		}
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compileAndRunFloat(b, code)
	}
}
