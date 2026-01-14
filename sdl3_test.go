package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestSDL3SimpleLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux SDL3 test on non-Linux platform")
	}

	// Check if SDL3 is available
	if _, err := exec.LookPath("pkg-config"); err != nil {
		t.Skip("pkg-config not found, skipping SDL3 test")
	}
	cmd := exec.Command("pkg-config", "--exists", "sdl3")
	if err := cmd.Run(); err != nil {
		t.Skip("SDL3 not installed, skipping test")
	}

	source := `import sdl3 as sdl

println("Initializing SDL3...")
init_result = sdl.SDL_Init(sdl.SDL_INIT_VIDEO)

init_result or! {
    printf("SDL_Init failed: %s\n", sdl.SDL_GetError())
    0
}

printf("SDL_Init successful: %f\n", init_result)

println("Creating window...")
window = sdl.SDL_CreateWindow("SDL3 Test", 640, 480, sdl.SDL_WINDOW_HIDDEN)

window or! {
    printf("Window creation failed: %s\n", sdl.SDL_GetError())
    sdl.SDL_Quit()
    0
}

printf("Window created: %f\n", window)

println("Cleaning up...")
sdl.SDL_DestroyWindow(window)
sdl.SDL_Quit()

println("Done!")
`

	// Set SDL to use dummy video driver for headless testing
	os.Setenv("SDL_VIDEODRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEODRIVER")

	// Compile and run the program
	result := compileAndRun(t, source)

	// Check output
	if !strings.Contains(result, "SDL_Init successful: 1") {
		t.Errorf("Expected SDL_Init to succeed, got: %s", result)
	}
	if !strings.Contains(result, "Window created:") {
		t.Errorf("Expected window creation to succeed, got: %s", result)
	}
	if !strings.Contains(result, "Done!") {
		t.Errorf("Expected program to complete, got: %s", result)
	}
}

func TestSDL3SimpleWindows(t *testing.T) {
	source := `import sdl3 as sdl

println("Initializing SDL3...")
init_result = sdl.SDL_Init(sdl.SDL_INIT_VIDEO)

init_result or! {
    printf("SDL_Init failed: %s\n", sdl.SDL_GetError())
    0
}

println("Creating window...")
window = sdl.SDL_CreateWindow("SDL3 Test", 640, 480, sdl.SDL_WINDOW_HIDDEN)

window or! {
    printf("Window creation failed: %s\n", sdl.SDL_GetError())
    sdl.SDL_Quit()
    0
}

println("Cleaning up...")
sdl.SDL_DestroyWindow(window)
sdl.SDL_Quit()

println("Done!")
`

	// Just verify it compiles for Windows - don't run with Wine
	// (Wine with SDL3 may not work in all environments and SDL3 may not be installed)
	tmpFile, err := os.CreateTemp("", "sdl3_windows_test_*.vibe67")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(source); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write source: %v", err)
	}
	tmpFile.Close()

	// Compile for Windows
	outputPath := tmpPath + ".exe"
	defer os.Remove(outputPath)

	platform := Platform{
		Arch: ArchX86_64,
		OS:   OSWindows,
	}

	err = CompileC67(tmpPath, outputPath, platform)
	if err != nil {
		t.Errorf("Windows SDL3 compilation failed: %v", err)
		return
	}

	// Verify PE executable was created
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Errorf("Output file not created: %v", err)
		return
	}

	if info.Size() < 100 {
		t.Errorf("Output file suspiciously small: %d bytes", info.Size())
	}

	t.Logf("Successfully compiled SDL3 Windows program: %d bytes", info.Size())
}

func TestSDL3Constants(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux SDL3 test on non-Linux platform")
	}

	// Check if SDL3 is available
	if _, err := exec.LookPath("pkg-config"); err != nil {
		t.Skip("pkg-config not found, skipping SDL3 test")
	}
	cmd := exec.Command("pkg-config", "--exists", "sdl3")
	if err := cmd.Run(); err != nil {
		t.Skip("SDL3 not installed, skipping test")
	}

	source := `import sdl3 as sdl

println("Testing SDL3 constants...")
video_flag = sdl.SDL_INIT_VIDEO
printf("SDL_INIT_VIDEO = %.0f\n", video_flag)

resizable = sdl.SDL_WINDOW_RESIZABLE
printf("SDL_WINDOW_RESIZABLE = %.0f\n", resizable)

hidden = sdl.SDL_WINDOW_HIDDEN
printf("SDL_WINDOW_HIDDEN = %.0f\n", hidden)

println("Done!")
`

	// Set SDL to use dummy video driver for headless testing
	os.Setenv("SDL_VIDEODRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEODRIVER")

	result := compileAndRun(t, source)

	// SDL_INIT_VIDEO should be 0x00000020 (32)
	if !strings.Contains(result, "SDL_INIT_VIDEO = 32") {
		t.Errorf("Expected SDL_INIT_VIDEO = 32, got: %s", result)
	}

	// SDL_WINDOW_RESIZABLE should be 0x00000020 (32)
	if !strings.Contains(result, "SDL_WINDOW_RESIZABLE = 32") {
		t.Errorf("Expected SDL_WINDOW_RESIZABLE = 32, got: %s", result)
	}

	// SDL_WINDOW_HIDDEN should be 0x00000008 (8)
	if !strings.Contains(result, "SDL_WINDOW_HIDDEN = 8") {
		t.Errorf("Expected SDL_WINDOW_HIDDEN = 8, got: %s", result)
	}
}

func TestSDL3OrBangOperator(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux SDL3 test on non-Linux platform")
	}

	// Check if SDL3 is available
	if _, err := exec.LookPath("pkg-config"); err != nil {
		t.Skip("pkg-config not found, skipping SDL3 test")
	}
	cmd := exec.Command("pkg-config", "--exists", "sdl3")
	if err := cmd.Run(); err != nil {
		t.Skip("SDL3 not installed, skipping test")
	}

	source := `import sdl3 as sdl

println("Testing or! operator...")

result = sdl.SDL_Init(sdl.SDL_INIT_VIDEO) or! {
    println("SDL_Init failed!")
    0
}

printf("SDL_Init result: %f\n", result)

sdl.SDL_Quit()
println("Done!")
`

	// Set SDL to use dummy video driver for headless testing
	os.Setenv("SDL_VIDEODRIVER", "dummy")
	defer os.Unsetenv("SDL_VIDEODRIVER")

	result := compileAndRun(t, source)

	// Should succeed
	if !strings.Contains(result, "SDL_Init result: 1") {
		t.Errorf("Expected SDL_Init to succeed with or!, got: %s", result)
	}
	if strings.Contains(result, "SDL_Init failed!") {
		t.Errorf("Expected or! not to trigger error block, but it did: %s", result)
	}
}

func TestSDL3ExampleCompiles(t *testing.T) {
	// Just verify sdl3example.vibe67 compiles for both targets
	data, err := os.ReadFile("sdl3example.vibe67")
	if err != nil {
		t.Skipf("sdl3example.vibe67 not found: %v", err)
	}

	source := string(data)

	// Test Linux compilation
	if runtime.GOOS == "linux" {
		t.Run("Linux", func(t *testing.T) {
			// Check if SDL3 is available
			if _, err := exec.LookPath("pkg-config"); err != nil {
				t.Skip("pkg-config not found, skipping SDL3 test")
			}
			cmd := exec.Command("pkg-config", "--exists", "sdl3")
			if err := cmd.Run(); err != nil {
				t.Skip("SDL3 not installed, skipping test")
			}

			// Don't run it since it needs the BMP file, just verify it compiles
			tmpDir := t.TempDir()
			srcFile := tmpDir + "/test.vibe67"
			exePath := tmpDir + "/test"

			if err := os.WriteFile(srcFile, []byte(source), 0644); err != nil {
				t.Fatalf("Failed to write source: %v", err)
			}

			osType, _ := ParseOS("linux")
			archType, _ := ParseArch(runtime.GOARCH)
			platform := Platform{
				OS:   osType,
				Arch: archType,
			}
			if err := CompileC67WithOptions(srcFile, exePath, platform, 0, false); err != nil {
				t.Fatalf("Compilation failed: %v", err)
			}

			info, err := os.Stat(exePath)
			if err != nil {
				t.Fatalf("Executable not created: %v", err)
			}
			if info.Size() == 0 {
				t.Errorf("Executable is empty")
			}
			t.Logf("Successfully compiled sdl3example.vibe67 for Linux: %d bytes", info.Size())
		})
	}

	// Test Windows compilation
	t.Run("Windows", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := tmpDir + "/test.vibe67"
		exePath := tmpDir + "/test.exe"

		if err := os.WriteFile(srcFile, []byte(source), 0644); err != nil {
			t.Fatalf("Failed to write source: %v", err)
		}

		osType, _ := ParseOS("windows")
		archType, _ := ParseArch("amd64")
		platform := Platform{
			OS:   osType,
			Arch: archType,
		}
		if err := CompileC67WithOptions(srcFile, exePath, platform, 0, false); err != nil {
			t.Fatalf("Compilation failed: %v", err)
		}

		// Verify it's a valid PE executable
		data, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatalf("Failed to read executable: %v", err)
		}

		if len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
			t.Errorf("Expected PE executable (MZ header)")
		}
		t.Logf("Successfully compiled sdl3example.vibe67 for Windows: %d bytes", len(data))
	})
}









