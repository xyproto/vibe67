package main

import (
	"strings"
	"testing"
)

func TestMainFunction(t *testing.T) {
	code := `
main = {
    println("Hello from main")
    0
}
`
	output := compileAndRun(t, code)
	if !strings.Contains(output, "Hello from main") {
		t.Errorf("Expected 'Hello from main' in output, got: %s", output)
	}
}

func TestMainFunctionReturnValue(t *testing.T) {
	code := `main = 0`
	// This should exit with code 0
	output := compileAndRun(t, code)
	_ = output // main returns 0, no output expected
}









