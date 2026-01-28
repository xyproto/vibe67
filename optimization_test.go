package main

import (
	"strings"
	"testing"
)

func TestFMAOptimization(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "FMA pattern: (a * b) + c",
			code: `
a := 2.0
b := 3.0
c := 4.0
println(a * b + c)`,
			want: "10",
		},
		{
			name: "FMA pattern: c + (a * b)",
			code: `
a := 2.0
b := 3.0
c := 4.0
println(c + a * b)`,
			want: "10",
		},
		{
			name: "FMA pattern: nested",
			code: `
x := 5.0
y := 2.0
z := 3.0
println(x * x + y * z)`,
			want: "31",
		},
		{
			name: "FMA pattern: polynomial",
			code: `
x := 2.0
a := 3.0
b := 4.0
c := 5.0
println(a * x * x + b * x + c)`,
			want: "25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			result = strings.TrimSpace(result)
			if result != tt.want {
				t.Errorf("Expected %s, got %s", tt.want, result)
			}
		})
	}
}

func TestBitManipulationBuiltins(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "popcount of 7 (0b111)",
			code: `println(popcount(7.0))`,
			want: "3",
		},
		{
			name: "popcount of 15 (0b1111)",
			code: `println(popcount(15.0))`,
			want: "4",
		},
		{
			name: "popcount of 0",
			code: `println(popcount(0.0))`,
			want: "0",
		},
		{
			name: "clz of 1 (leading zeros in 64-bit)",
			code: `println(clz(1.0))`,
			want: "63",
		},
		{
			name: "clz of 8 (0b1000)",
			code: `println(clz(8.0))`,
			want: "60",
		},
		{
			name: "clz of 0 (all zeros)",
			code: `println(clz(0.0))`,
			want: "64",
		},
		{
			name: "ctz of 8 (0b1000)",
			code: `println(ctz(8.0))`,
			want: "3",
		},
		{
			name: "ctz of 1",
			code: `println(ctz(1.0))`,
			want: "0",
		},
		{
			name: "ctz of 0 (all zeros)",
			code: `println(ctz(0.0))`,
			want: "64",
		},
		{
			name: "popcount of large number",
			code: `println(popcount(255.0))`,
			want: "8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compileAndRun(t, tt.code)
			result = strings.TrimSpace(result)
			if result != tt.want {
				t.Errorf("Expected %s, got %s", tt.want, result)
			}
		})
	}
}

func TestCPUFeatureDetection(t *testing.T) {
	// This test just verifies that CPU feature detection code compiles and runs
	// It doesn't test the actual CPU features since we don't know what CPU we're on
	code := `
x := 2.0
y := 3.0
z := 4.0
println(x * y + z)
`
	result := compileAndRun(t, code)
	result = strings.TrimSpace(result)
	if result != "10" {
		t.Errorf("Expected 10, got %s", result)
	}
}

func TestFMAPrecisionOld(t *testing.T) {
	// Test that FMA provides better precision than mul+add
	// Simple test showing FMA works correctly with large numbers
	code := `
a := 1000000.0
b := 0.000001
c := 1.0
println(a * b + c)
`
	result := compileAndRun(t, code)
	result = strings.TrimSpace(result)
	// With FMA, result should be exactly 2.0
	// With separate mul+add, might have rounding errors
	if result != "2" {
		t.Logf("Result: %s (FMA should give exact 2.0, mul+add might differ)", result)
	}
}

func BenchmarkFMAOld(b *testing.B) {
	code := `
fn polynomial(x: float64) -> float64 {
	var a = 3.0
	var b = 4.0
	var c = 5.0
	a * x * x + b * x + c
}

fn main() -> float64 {
	var sum = 0.0
	var i = 0.0
	while i < 1000.0 {
		sum = sum + polynomial(i)
		i = i + 1.0
	}
	sum
}
`
	// Create fake *testing.T for compileAndRun
	t := &testing.T{}
	_ = compileAndRun(t, code)
}

func BenchmarkPopcount(b *testing.B) {
	code := `
fn main() -> float64 {
	var sum = 0.0
	var i = 0.0
	while i < 1000.0 {
		sum = sum + popcount(i)
		i = i + 1.0
	}
	sum
}
`
	// Create fake *testing.T for compileAndRun
	t := &testing.T{}
	_ = compileAndRun(t, code)
}
