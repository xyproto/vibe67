package main

import (
	"strings"
	"testing"
)

// TestDivisionByZeroReturnsNaN tests that division by zero returns NaN not exit
func TestDivisionByZeroReturnsNaN(t *testing.T) {
	source := `x := 10 / 0
safe := x or! -999.0
println(safe)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "-999\n") {
		t.Errorf("Expected output to contain: %s, got: %s", "-999\n", result)
	}
}

// TestOrBangWithSuccess tests or! with successful value
func TestOrBangWithSuccess(t *testing.T) {
	source := `result := 10 / 2
safe := result or! 0.0
println(safe)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "5\n") {
		t.Errorf("Expected output to contain: %s, got: %s", "5\n", result)
	}
}

// TestOrBangWithError tests or! with error value (division by zero)
func TestOrBangWithError(t *testing.T) {
	source := `result := 10 / 0
safe := result or! 42.0
println(safe)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "42\n") {
		t.Errorf("Expected output to contain: %s, got: %s", "42\n", result)
	}
}

// TestOrBangChaining tests chained or! operators
func TestOrBangChaining(t *testing.T) {
	source := `x := 10 / 0
y := 20 / 0
z := x or! (y or! 99.0)
println(z)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "99\n") {
		t.Errorf("Expected output to contain: %s, got: %s", "99\n", result)
	}
}

// TestErrorPropertySimple tests .error property doesn't crash
func TestErrorPropertySimple(t *testing.T) {
	source := `x := 5.0
y := x.error
println("ok")
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "ok\n") {
		t.Errorf("Expected output to contain: %s, got: %s", "ok\n", result)
	}
}

// TestErrorPropertyLength tests .error returns a string
func TestErrorPropertyLength(t *testing.T) {
	source := `x := 5.0
code := x.error
len := #code
println(len)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "0\n") { // Empty string for non-error
		t.Errorf("Expected output to contain: %s, got: %s", "0\n", result)
	}
}

// TestErrorPropertyBasic tests .error property on Result types
func TestErrorPropertyBasic(t *testing.T) {
	source := `result := 10 / 0
code := result.error
code {
    "" => println("no error")
    ~> println(f"error: {code}")
}
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "error: dv0") {
		t.Errorf("Expected output to contain 'error: dv0', got: %s", result)
	}
}

// TestErrorFunction tests creating errors with error() function
func TestErrorFunction(t *testing.T) {
	source := `x := error("arg")
code := x.error
println(code)
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "arg") {
		t.Errorf("Expected output to contain 'arg', got: %s", result)
	}
}

// TestErrorPropertyOnSuccess tests .error returns empty string for success
func TestErrorPropertyOnSuccess(t *testing.T) {
	source := `result := 10 / 2
code := result.error
code {
    "" => println("success")
    ~> println(f"error: {code}")
}
`
	result := compileAndRun(t, source)
	if !strings.Contains(result, "success") {
		t.Errorf("Expected output to contain 'success', got: %s", result)
	}
}
