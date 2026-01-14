package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Confidence that this function is working: 95%
func TestEprint(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		wantStdout string
		wantStderr string
		wantExit   int
	}{
		{
			name:       "eprintln basic",
			code:       `eprintln("error message")`,
			wantStdout: "",
			wantStderr: "error message\n",
			wantExit:   0,
		},
		{
			name:       "eprint without newline",
			code:       `eprint("error: ")`,
			wantStdout: "",
			wantStderr: "error: ",
			wantExit:   0,
		},
		{
			name:       "eprintln number",
			code:       `eprintln(42)`,
			wantStdout: "",
			wantStderr: "42\n",
			wantExit:   0,
		},
		{
			name: "eprint vs println",
			code: `
eprintln("to stderr")
println("to stdout")
`,
			wantStdout: "to stdout\n",
			wantStderr: "to stderr\n",
			wantExit:   0,
		},
		{
			name: "eprintln returns Result",
			code: `
result = eprintln("test")
println("completed")
`,
			wantStdout: "completed\n",
			wantStderr: "test\n",
			wantExit:   0,
		},
		{
			name:       "exitln exits",
			code:       `exitln("fatal error")`,
			wantStdout: "",
			wantStderr: "fatal error\n",
			wantExit:   1,
		},
		{
			name: "exitln does not continue",
			code: `
exitln("error")
println("should not see this")
`,
			wantStdout: "",
			wantStderr: "error\n",
			wantExit:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the code
			binary := compileTestCode(t, tt.code)

			// Run and capture stdout/stderr separately
			cmd := exec.Command(binary)
			stdout, stderr, exitCode := runCommandSeparate(cmd)

			// Check exit code
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d", exitCode, tt.wantExit)
			}

			// Check stdout
			if stdout != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout, tt.wantStdout)
			}

			// Check stderr
			if stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantStderr)
			}
		})
	}
}

// Confidence that this function is working: 95%
func TestEprintFormatted(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		wantStderr string
	}{
		{
			name:       "eprintf simple",
			code:       `eprintf("error: value\n")`,
			wantStderr: "error: value\n",
		},
		{
			name:       "exitf simple",
			code:       `exitf("fatal\n")`,
			wantStderr: "fatal\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary := compileTestCode(t, tt.code)

			cmd := exec.Command(binary)
			_, stderr, _ := runCommandSeparate(cmd)

			if stderr != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr, tt.wantStderr)
			}
		})
	}
}

// compileTestCode compiles Vibe67 code and returns the path to the executable
func compileTestCode(t *testing.T, code string) string {
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
	if err := CompileC67WithOptions(srcFile, exePath, platform, 0, false); err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	return exePath
}

// runCommandSeparate runs a command and returns stdout, stderr, and exit code separately
func runCommandSeparate(cmd *exec.Cmd) (stdout, stderr string, exitCode int) {
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
	} else {
		exitCode = 0
	}

	return
}









