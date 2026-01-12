package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// compileTestCodeAllowError compiles code and returns error if compilation fails
func compileTestCodeAllowError(t *testing.T, code string) (string, error) {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()

	// Write source file
	srcFile := filepath.Join(tmpDir, "test.vibe67")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Compile using Go API directly
	exePath := filepath.Join(tmpDir, "test")
	osType, _ := ParseOS(runtime.GOOS)
	archType, _ := ParseArch(runtime.GOARCH)
	platform := Platform{
		OS:   osType,
		Arch: archType,
	}
	err := CompileVibe67WithOptions(srcFile, exePath, platform, 0, false)
	if err != nil {
		return "", err
	}

	return exePath, nil
}

// Confidence that this function is working: 95%
// TestUndefinedFunctionDetection tests that calling undefined functions produces a compile error
func TestUndefinedFunctionDetection(t *testing.T) {
	code := `
main = {
// This should fail because foobar is not defined
result := foobar(42)
println(result)
}
`
	// This should produce a compilation error
	_, err := compileTestCodeAllowError(t, code)
	if err == nil {
		t.Error("Expected compilation error for undefined function, but got none")
	} else if !strings.Contains(err.Error(), "undefined function") && !strings.Contains(err.Error(), "foobar") {
		t.Errorf("Expected error about undefined function 'foobar', got: %v", err)
	}
}

// Confidence that this function is working: 95%
// TestDefinedFunctionWorks tests that defined functions compile successfully
func TestDefinedFunctionWorks(t *testing.T) {
	code := `
// Define a function
double = x -> x * 2

main = {
// Use it
result := double(21)
println(result)
}
`
	output := compileAndRun(t, code)
	if !strings.Contains(output, "42") {
		t.Errorf("Expected output to contain '42', got: %s", output)
	}
}

// Confidence that this function is working: 95%
// TestBuiltinFunctionsWork tests that builtin functions are recognized
func TestBuiltinFunctionsWork(t *testing.T) {
	code := `
// Use builtin functions
println("Hello")
x := sqrt(16.0)
printf("sqrt(16) = %v\n", x)
`
	output := compileAndRun(t, code)
	if !strings.Contains(output, "Hello") {
		t.Errorf("Expected output to contain 'Hello', got: %s", output)
	}
	if !strings.Contains(output, "sqrt(16) = 4") {
		t.Errorf("Expected output to contain 'sqrt(16) = 4', got: %s", output)
	}
}

// Confidence that this function is working: 95%
// TestCFFIFunctionsWork tests that C FFI functions are recognized
func TestCFFIFunctionsWork(t *testing.T) {
	code := `
// Use C FFI functions - should not produce undefined function errors
ptr := c.malloc(64)
println("Allocated memory")
c.free(ptr)
println("Freed memory")
`
	output := compileAndRun(t, code)
	if !strings.Contains(output, "Allocated memory") {
		t.Errorf("Expected output to contain 'Allocated memory', got: %s", output)
	}
	if !strings.Contains(output, "Freed memory") {
		t.Errorf("Expected output to contain 'Freed memory', got: %s", output)
	}
}

// Confidence that this function is working: 90%
// TestImportedCLibraryFunctionsWork tests that imported C library functions are recognized
func TestImportedCLibraryFunctionsWork(t *testing.T) {
	code := `
import sdl3 as sdl

// Use SDL functions - should not produce undefined function errors
result := sdl.SDL_Init(0)
println("SDL initialized")
sdl.SDL_Quit()
println("SDL quit")
`
	// This might fail if SDL3 is not installed, but it should at least compile
	// without "undefined function" errors
	_, err := compileTestCodeAllowError(t, code)
	if err != nil {
		// Check that the error is NOT about undefined functions
		if strings.Contains(err.Error(), "undefined function") {
			t.Errorf("Should not report undefined functions for C library imports, got: %v", err)
		}
		// Other errors (like SDL not installed) are OK for this test
	}
}

// Confidence that this function is working: 95%
// TestMultipleUndefinedFunctions tests that multiple undefined functions are reported
func TestMultipleUndefinedFunctions(t *testing.T) {
	code := `
main = {
// Call multiple undefined functions
x := foo(1)
y := bar(2)
z := baz(3)
println(x + y + z)
}
`
	_, err := compileTestCodeAllowError(t, code)
	if err == nil {
		t.Error("Expected compilation error for undefined functions, but got none")
	} else {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "undefined function") {
			t.Errorf("Expected error about undefined functions, got: %v", err)
		}
		// Check that at least some of the undefined functions are mentioned
		if !strings.Contains(errMsg, "foo") && !strings.Contains(errMsg, "bar") && !strings.Contains(errMsg, "baz") {
			t.Errorf("Expected error to mention undefined functions (foo, bar, baz), got: %v", err)
		}
	}
}

// Confidence that this function is working: 95%
// TestUndefinedFunctionInBlock tests that undefined functions in blocks are caught
func TestUndefinedFunctionInBlock(t *testing.T) {
	code := `
main = {
// Call undefined function in or! block
x := 42 or! {
    undefined_func()
    exitln("error")
}
println(x)
}
`
	_, err := compileTestCodeAllowError(t, code)
	if err == nil {
		t.Error("Expected compilation error for undefined function in block, but got none")
	} else if !strings.Contains(err.Error(), "undefined function") && !strings.Contains(err.Error(), "undefined_func") {
		t.Errorf("Expected error about undefined function 'undefined_func', got: %v", err)
	}
}

// Confidence that this function is working: 95%
// TestUndefinedFunctionInLambda tests that undefined functions in lambdas are caught
func TestUndefinedFunctionInLambda(t *testing.T) {
	code := `
// Call undefined function in lambda
mapper = x -> {
    result := not_defined(x)
    result * 2
}
main = {
y := mapper(5)
println(y)
}
`
	_, err := compileTestCodeAllowError(t, code)
	if err == nil {
		t.Error("Expected compilation error for undefined function in lambda, but got none")
	} else if !strings.Contains(err.Error(), "undefined function") && !strings.Contains(err.Error(), "not_defined") {
		t.Errorf("Expected error about undefined function 'not_defined', got: %v", err)
	}
}

// Confidence that this function is working: 95%
// TestUndefinedFunctionInMatch tests that undefined functions in match expressions are caught
func TestUndefinedFunctionInMatch(t *testing.T) {
	code := `
main = {
// Call undefined function in match expression
x := 5
result = { | x > 10 => big_value() ~> medium_value() }
println(result)
}
`
	_, err := compileTestCodeAllowError(t, code)
	if err == nil {
		t.Error("Expected compilation error for undefined functions in match, but got none")
	} else {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "undefined function") {
			t.Errorf("Expected error about undefined functions, got: %v", err)
		}
		// Check that at least one of the undefined functions is mentioned
		if !strings.Contains(errMsg, "big_value") && !strings.Contains(errMsg, "medium_value") {
			t.Errorf("Expected error to mention undefined functions (big_value, medium_value), got: %v", err)
		}
	}
}
