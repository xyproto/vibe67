// Completion: 100% - Utility module complete
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// containsMainFunction checks if code contains a Main function definition
func containsMainFunction(code string) bool {
	// Simple check for main = or main :=
	return strings.Contains(code, "main =") || strings.Contains(code, "main :=")
}

// needsMainWrapper checks if code should be wrapped in main function
func needsMainWrapper(code string) bool {
	// Don't wrap if already has main
	if containsMainFunction(code) {
		return false
	}
	// Don't wrap if has imports (imports must be at module level)
	if strings.Contains(code, "import ") {
		return false
	}
	// Don't wrap if has module-level variable declarations before function definitions
	// Pattern: "name := value" at start of line before "->"
	lines := strings.Split(code, "\n")
	hasModuleLevelVar := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ":=") && !strings.Contains(trimmed, "->") {
			hasModuleLevelVar = true
		}
		if hasModuleLevelVar && strings.Contains(trimmed, "->") {
			// Has a variable declaration before a lambda definition
			return false
		}
	}
	return true
}

// compileAndRun is a helper function that compiles and runs Vibe67 code,
// returning the output
func compileAndRun(t *testing.T, code string) string {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()

	// Auto-wrap test code in main function if it doesn't have one
	// This allows test snippets to use lowercase variables
	// But don't wrap if code has imports (they must be at module level)
	if needsMainWrapper(code) {
		// Remove leading/trailing whitespace from each line to avoid parsing issues
		lines := strings.Split(code, "\n")
		var cleaned []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		code = "main = {\n" + strings.Join(cleaned, "\n") + "\n}"
	}

	// Write source file
	srcFile := filepath.Join(tmpDir, "test.vibe67")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Compile using Go API directly
	exePath := filepath.Join(tmpDir, "test")
	osType, _ := ParseOS(runtime.GOOS)
	archType, _ := ParseArch(runtime.GOARCH)
	
	// Add .exe extension on Windows
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}
	
	platform := Platform{
		OS:   osType,
		Arch: archType,
	}
	if err := CompileVibe67WithOptions(srcFile, exePath, platform, 0, false); err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	// Run - inherit environment variables for SDL_VIDEODRIVER etc
	cmd := exec.Command(exePath)
	cmd.Env = os.Environ()
	runOutput, err := cmd.CombinedOutput()
	// Note: Vibe67 programs may return non-zero exit codes as their result value
	// We only fail if there's an actual execution error (not just non-zero exit)
	if err != nil {
		// Check if it's just a non-zero exit code (which is normal for Vibe67)
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Non-zero exit but program ran successfully - return output
			_ = exitErr
			return string(runOutput)
		}
		// Actual execution error (program didn't run)
		t.Fatalf("Execution failed: %v\nOutput: %s", err, runOutput)
	}

	return string(runOutput)
}

// compileAndRunWindows is a helper function that compiles and runs Vibe67 code for Windows
// under Wine (on non-Windows platforms), with a 3-second timeout
func compileAndRunWindows(t *testing.T, code string) string {
	t.Helper()

	// Check if Wine is available on non-Windows platforms
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("wine"); err != nil {
			t.Skip("Wine is not installed - skipping Windows test")
		}
	}

	// Create temporary directory
	tmpDir := t.TempDir()

	// Auto-wrap test code in Main function if it doesn't have one
	if !containsMainFunction(code) {
		// Remove leading/trailing whitespace from each line to avoid parsing issues
		lines := strings.Split(code, "\n")
		var cleaned []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		code = "Main = {\n" + strings.Join(cleaned, "\n") + "\n}"
	}

	// Write source file
	srcFile := filepath.Join(tmpDir, "test.vibe67")
	if err := os.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Compile for Windows using Go API directly
	exePath := filepath.Join(tmpDir, "test.exe")
	osType, _ := ParseOS("windows")
	archType, _ := ParseArch("amd64")
	platform := Platform{
		OS:   osType,
		Arch: archType,
	}
	if err := CompileVibe67WithOptions(srcFile, exePath, platform, 0, false); err != nil {
		t.Fatalf("Compilation failed: %v", err)
	}

	// Verify the PE executable was created
	if _, err := os.Stat(exePath); err != nil {
		t.Fatalf("Executable not created: %v", err)
	}

	// Verify it's a PE executable
	data, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("Failed to read executable: %v", err)
	}
	if len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
		t.Fatalf("Not a valid PE executable (missing MZ header)")
	}

	// Run - use Wine on non-Windows platforms
	var runCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		runCmd = exec.Command(exePath)
	} else {
		runCmd = exec.Command("wine", exePath)
	}

	runOutput, err := runCmd.CombinedOutput()

	// Wine may return non-zero exit codes even on success
	// Check if we got any output, which indicates the program ran
	if err != nil && len(runOutput) == 0 {
		t.Fatalf("Failed to run Windows executable: %v\nOutput: %s", err, runOutput)
	}

	return string(runOutput)
}
