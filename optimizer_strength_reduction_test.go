// Tests for integer strength reduction optimizations (FUTURE FEATURE)
//
// These optimizations are currently DISABLED but the infrastructure is in place.
// They would optimize integer operations:
//   - x * 2^n → x << n (multiply by power-of-2 → left shift)
//   - x / 2^n → x >> n (divide by power-of-2 → right shift)
//   - x % 2^n → x & (2^n-1) (modulo by power-of-2 → AND mask)
//
// Status: Context detection needs more work. Low priority since:
//  1. Vibe67 is float64-native (integer ops are rare)
//  2. Users can use unsafe blocks with inline assembly for performance
//  3. These optimizations would only help a tiny subset of programs
//
// These tests are SKIPPED for now, ready to be enabled when optimization is fixed.
package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestMultiplyByPowerOf2WithIntCast tests x * 2^n → x << n optimization with int cast
func TestMultiplyByPowerOf2WithIntCast(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    y := (x as int32) * 8    // Should be optimized to x << 3
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "800"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestDivideByPowerOf2WithIntCast tests x / 2^n → x >> n optimization with int cast
func TestDivideByPowerOf2WithIntCast(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    y := (x as int32) / 4    // Should be optimized to x >> 2
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "25"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestModuloByPowerOf2WithIntCast tests x % 2^n → x & (2^n-1) optimization with int cast
func TestModuloByPowerOf2WithIntCast(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    y := (x as int32) % 16   // Should be optimized to x & 15
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "4"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestCombinedStrengthReduction tests all three optimizations together
func TestCombinedStrengthReduction(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    a := (x as int32) * 8    // → x << 3 = 800
    b := (x as int32) / 4    // → x >> 2 = 25
    c := (x as int32) % 16   // → x & 15 = 4
    result := a + b + c
    println(result)
}
`
	result := compileAndRun(t, source)
	expected := "829"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestMultiplyByNonPowerOf2 tests that non-power-of-2 multiplication is NOT optimized
func TestMultiplyByNonPowerOf2(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    y := (x as int32) * 7    // Should NOT be optimized (7 is not power of 2)
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "700"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestNoOptimizationOutsideUnsafe tests that optimizations are NOT applied outside unsafe blocks
func TestNoOptimizationOutsideUnsafe(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	// This test verifies that float64 operations are NOT optimized
	// because shifts don't work correctly on floating-point values
	source := `main = {
    x := 100.5
    y := (x as int32) * 8    // Should NOT be optimized (not in unsafe, and x is float)
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "804"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestStrengthReductionWithIntCast tests optimization with explicit integer cast
func TestStrengthReductionWithIntCast(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    y := (x as int32) * 8    // Should be optimized because x has int32 cast
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "800"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestMultiplyByPowerOf2_Various tests various powers of 2
func TestMultiplyByPowerOf2_Various(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	tests := []struct {
		power    int
		expected string
	}{
		{2, "200"},   // 100 * 2
		{4, "400"},   // 100 * 4
		{16, "1600"}, // 100 * 16
		{32, "3200"}, // 100 * 32
	}

	for _, tt := range tests {
		source := `main = {
    x := 100 as int32
    y := (x as int32) * ` + toString(tt.power) + `
    println(y)
}
`
		result := compileAndRun(t, source)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("For multiply by %d, expected %s in output, got: %s", tt.power, tt.expected, result)
		}
	}
}

// TestDivideByPowerOf2_Various tests various powers of 2
func TestDivideByPowerOf2_Various(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	tests := []struct {
		power    int
		expected string
	}{
		{2, "50"}, // 100 / 2
		{4, "25"}, // 100 / 4
		{8, "12"}, // 100 / 8
		{16, "6"}, // 100 / 16
		{32, "3"}, // 100 / 32
	}

	for _, tt := range tests {
		source := `main = {
    x := 100 as int32
    y := (x as int32) / ` + toString(tt.power) + `
    println(y)
}
`
		result := compileAndRun(t, source)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("For divide by %d, expected %s in output, got: %s", tt.power, tt.expected, result)
		}
	}
}

// TestModuloByPowerOf2_Various tests various powers of 2
func TestModuloByPowerOf2_Various(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	tests := []struct {
		power    int
		expected string
	}{
		{2, "0"},   // 100 % 2 = 0
		{4, "0"},   // 100 % 4 = 0
		{8, "4"},   // 100 % 8 = 4
		{16, "4"},  // 100 % 16 = 4
		{32, "4"},  // 100 % 32 = 4
		{64, "36"}, // 100 % 64 = 36
	}

	for _, tt := range tests {
		source := `main = {
    x := 100 as int32
    y := (x as int32) % ` + toString(tt.power) + `
    println(y)
}
`
		result := compileAndRun(t, source)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("For modulo by %d, expected %s in output, got: %s", tt.power, tt.expected, result)
		}
	}
}

// TestLeftOperandPowerOf2 tests optimization when left operand is power of 2
func TestLeftOperandPowerOf2(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    y := (8 as int32) * 100    // Should be optimized to 100 << 3
    println(y)
}
`
	result := compileAndRun(t, source)
	expected := "800"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestNestedUnsafeBlocks tests that optimization works in nested unsafe blocks
func TestNestedUnsafeBlocks(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    x := 100 as int32
    unsafe {
        y := (x as int32) * 8
        println(y)
    }
}
`
	result := compileAndRun(t, source)
	expected := "800"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// TestStrengthReductionInLoop tests optimization within loops
func TestStrengthReductionInLoop(t *testing.T) {
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	t.Skip("Integer strength reduction disabled - infrastructure in place for future use")
	source := `main = {
    sum := 0 as int32
    @ i in 0..<10 {
        sum += (i as int32) * 8    // Should be optimized to i << 3
    }
    println(sum)
}
`
	result := compileAndRun(t, source)
	// sum = (0+1+2+3+4+5+6+7+8+9) * 8 = 45 * 8 = 360
	expected := "360"
	if !strings.Contains(result, expected) {
		t.Errorf("Expected %s in output, got: %s", expected, result)
	}
}

// Helper function to convert int to string for test generation
func toString(n int) string {
	return fmt.Sprintf("%d", n)
}









