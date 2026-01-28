package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVibe67GameLibraryCompiles tests that vibe67game library compiles
func TestVibe67GameLibraryCompiles(t *testing.T) {
	vibe67gameDir := filepath.Join(os.Getenv("HOME"), "clones", "vibe67game")
	gamePath := filepath.Join(vibe67gameDir, "game.vibe67")

	if _, err := os.Stat(gamePath); os.IsNotExist(err) {
		t.Skip("vibe67game library not found at ~/clones/vibe67game")
	}

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "game_lib")
	cmd := exec.Command("./vibe67", gamePath, "-o", binary)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Compilation output: %s", output)
		t.Fatalf("Failed to compile vibe67game: %v", err)
	}

	t.Log("vibe67game library compiled successfully")
}

// TestVibe67GameTest tests that vibe67 test works in vibe67game directory
func TestVibe67GameTest(t *testing.T) {
	vibe67gameDir := filepath.Join(os.Getenv("HOME"), "clones", "vibe67game")
	testPath := filepath.Join(vibe67gameDir, "vibe67game_test.vibe67")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Skip("vibe67game_test.vibe67 not found")
	}

	// Run vibe67 test in the vibe67game directory
	vibe67Binary := filepath.Join(filepath.Dir(vibe67gameDir), "vibe67", "vibe67")
	cmd := exec.Command(vibe67Binary, "test")
	cmd.Dir = vibe67gameDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Test output: %s", output)
		t.Fatalf("vibe67 test failed: %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "PASS") {
		t.Errorf("Expected PASS in output, got: %s", outputStr)
	}

	t.Log("vibe67game tests passed")
}

// TestVibe67GameSimpleProgram tests that a simple program using vibe67game compiles
func TestVibe67GameSimpleProgram(t *testing.T) {
	cmd := exec.Command("pkg-config", "--exists", "sdl3")
	if err := cmd.Run(); err != nil {
		t.Skip("SDL3 not installed")
	}

	vibe67gameDir := filepath.Join(os.Getenv("HOME"), "clones", "vibe67game")
	gamePath := filepath.Join(vibe67gameDir, "game.vibe67")

	if _, err := os.Stat(gamePath); os.IsNotExist(err) {
		t.Skip("vibe67game library not found")
	}

	// Create a simple program that uses vibe67game
	source := `import "` + gamePath + `"

main = {
    println("Testing vibe67game import")
}
`

	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "test.vibe67")
	if err := os.WriteFile(srcFile, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(tmpDir, "test")
	cmd = exec.Command("./vibe67", srcFile, "-o", binary)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Compilation output: %s", output)
		t.Fatalf("Failed to compile: %v", err)
	}

	// Run it
	cmd = exec.Command(binary)
	output, err = cmd.CombinedOutput()

	if err != nil {
		t.Logf("Run output: %s", output)
		t.Fatalf("Failed to run: %v", err)
	}

	if !strings.Contains(string(output), "Testing vibe67game import") {
		t.Errorf("Expected 'Testing vibe67game import' in output, got: %s", output)
	}

	t.Log("Simple vibe67game program works")
}
