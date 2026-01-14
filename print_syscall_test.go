package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPrintSyscall tests print/println/printf using syscalls on Linux
func TestPrintSyscall(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Syscall tests only run on Linux x86_64")
	}

	tests := []struct {
		name       string
		code       string
		wantStdout string
	}{
		{
			name:       "print string literal",
			code:       `print("Hello")`,
			wantStdout: "Hello",
		},
		{
			name:       "println string literal",
			code:       `println("Hello")`,
			wantStdout: "Hello\n",
		},
		{
			name: "print multiple calls",
			code: `
print("Hello")
print(" ")
print("World")
`,
			wantStdout: "Hello World",
		},
		{
			name: "println multiple calls",
			code: `
println("Line 1")
println("Line 2")
println("Line 3")
`,
			wantStdout: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name: "print string variable",
			code: `
msg := "Hello World"
print(msg)
`,
			wantStdout: "Hello World",
		},
		{
			name: "println string variable",
			code: `
msg := "Hello World"
println(msg)
`,
			wantStdout: "Hello World\n",
		},
		{
			name: "print number",
			code: `
x := 42
println(x)
`,
			wantStdout: "42\n",
		},
		{
			name: "println with variables before",
			code: `
x := 10
y := 20
z := x + y
println(z)
`,
			wantStdout: "30\n",
		},
		{
			name: "println with variables after",
			code: `
println(42)
x := 100
y := x + 1
println(y)
`,
			wantStdout: "42\n101\n",
		},
		{
			name: "print f-string",
			code: `
name := "Alice"
age := 30
println(f"Hello, {name}! You are {age} years old.")
`,
			wantStdout: "Hello, Alice! You are 30 years old.\n",
		},
		{
			name: "print f-string with expressions",
			code: `
a := 5
b := 7
println(f"Sum: {a + b}, Product: {a * b}")
`,
			wantStdout: "Sum: 12, Product: 35\n",
		},
		{
			name: "print in lambda",
			code: `
greet := { println("Hello from lambda") }
greet()
`,
			wantStdout: "Hello from lambda\n",
		},
		{
			name: "print with function call before",
			code: `
double := x -> x * 2
result := double(5)
println(result)
`,
			wantStdout: "10\n",
		},
		{
			name: "print with function call after",
			code: `
println(42)
triple := x -> x * 3
println(triple(10))
`,
			wantStdout: "42\n30\n",
		},

		{
			name: "println empty",
			code: `
println()
`,
			wantStdout: "\n",
		},
		{
			name: "mixed print and println",
			code: `
print("Hello")
print(" ")
println("World!")
print("Next")
println(" line")
`,
			wantStdout: "Hello World!\nNext line\n",
		},
		{
			name: "print with arithmetic",
			code: `
println(10 + 5)
println(20 - 3)
println(4 * 5)
println(100 / 10)
`,
			wantStdout: "15\n17\n20\n10\n",
		},
		{
			name: "print in loop",
			code: `
@ i in 0..2 {
    println(i)
}
`,
			wantStdout: "0\n1\n2\n",
		},
		{
			name: "print f-string in loop",
			code: `
@ i in 0..2 {
    println(f"Count: {i}")
}
`,
			wantStdout: "Count: 0\nCount: 1\nCount: 2\n",
		},
		{
			name: "print with complex expressions",
			code: `
x := 5
y := 10
println(f"x={x}, y={y}, x+y={x+y}, x*y={x*y}")
`,
			wantStdout: "x=5, y=10, x+y=15, x*y=50\n",
		},
		{
			name: "print string concatenation via f-string",
			code: `
first := "Hello"
second := "World"
println(f"{first} {second}!")
`,
			wantStdout: "Hello World!\n",
		},
		{
			name: "print with comparison result",
			code: `
x := 10
y := 20
println(x < y)
println(x > y)
`,
			wantStdout: "1\n0\n",
		},

		{
			name: "print multiple values on same line",
			code: `
print("Value: ")
print(42)
println(" done")
`,
			wantStdout: "Value: 42 done\n",
		},
		{
			name: "println with multiple arguments",
			code: `
println("Hello", "World")
println(1, 2, 3)
println("x:", 10, "y:", 20)
`,
			wantStdout: "Hello World\n1 2 3\nx: 10 y: 20\n",
		},
		{
			name: "println with mixed types",
			code: `
x := 42
y := "answer"
println(y, "is", x)
`,
			wantStdout: "answer is 42\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile code
			binary := compileTestBinary(t, tt.code)

			// Run and capture output
			cmd := exec.Command(binary)
			stdout, stderr, exitCode := runCommand(cmd)

			if exitCode != 0 {
				t.Errorf("Unexpected exit code %d, stderr: %s", exitCode, stderr)
			}

			if stdout != tt.wantStdout {
				t.Errorf("stdout mismatch:\nwant: %q\ngot:  %q", tt.wantStdout, stdout)
			}
		})
	}
}

// TestPrintfSyscall tests printf implementation
// Note: printf expects a string literal format (C-style), not f-strings
// For f-strings, use println instead which handles them natively
func TestPrintfSyscall(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Syscall tests only run on Linux x86_64")
	}

	tests := []struct {
		name       string
		code       string
		wantStdout string
	}{
		{
			name:       "printf with format %v",
			code:       `printf("%v\n", 42)`,
			wantStdout: "42.000000\n",
		},
		{
			name: "printf with multiple %v",
			code: `
x := 10
y := 20
printf("x=%v, y=%v\n", x, y)
`,
			wantStdout: "x=10.000000, y=20.000000\n",
		},
		{
			name: "println with f-string (preferred method)",
			code: `
x := 100
y := 200
println(f"x={x}, y={y}")
`,
			wantStdout: "x=100, y=200\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := compileTestBinary(t, tt.code)
			cmd := exec.Command(binary)
			stdout, stderr, exitCode := runCommand(cmd)

			if exitCode != 0 {
				t.Errorf("Unexpected exit code %d, stderr: %s", exitCode, stderr)
			}

			if stdout != tt.wantStdout {
				t.Errorf("stdout mismatch:\nwant: %q\ngot:  %q", tt.wantStdout, stdout)
			}
		})
	}
}

// Helper: compile test code and return binary path
func compileTestBinary(t *testing.T, code string) string {
	t.Helper()

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.vibe67")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatalf("Failed to write source: %v", err)
	}

	binPath := filepath.Join(tmpDir, "test")
	osType, _ := ParseOS(runtime.GOOS)
	archType, _ := ParseArch(runtime.GOARCH)
	platform := Platform{
		OS:   osType,
		Arch: archType,
	}
	if err := CompileC67WithOptions(srcFile, binPath, platform, 0, false); err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	return binPath
}

// Helper: run command and capture stdout/stderr/exit code
func runCommand(cmd *exec.Cmd) (stdout, stderr string, exitCode int) {
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return
}









