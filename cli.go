// Completion: 100% - Utility module complete
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cli.go - User-friendly command-line interface for c67
//
// This file implements a Go-like CLI interface with subcommands:
// - c67 (default: compile current directory or show help)
// - c67 build <file> (compile to executable)
// - c67 run <file> (compile and run immediately)
// - c67 <file.c67> (shorthand for build)
//
// Also supports shebang execution: #!/usr/bin/c67

// CommandContext holds the execution context for a CLI command
type CommandContext struct {
	Args       []string
	Platform   Platform
	Verbose    bool
	Quiet      bool
	OptTimeout float64
	UpdateDeps bool
	SingleFile bool
	OutputPath string
}

// RunCLI is the main entry point for the user-friendly CLI
// It determines which command to run based on arguments
func RunCLI(args []string, platform Platform, verbose, quiet bool, optTimeout float64, updateDeps, singleFile bool, outputPath string) error {
	ctx := &CommandContext{
		Args:       args,
		Platform:   platform,
		Verbose:    verbose,
		Quiet:      quiet,
		OptTimeout: optTimeout,
		UpdateDeps: updateDeps,
		SingleFile: singleFile,
		OutputPath: outputPath,
	}

	// No arguments - show help
	if len(args) == 0 {
		return cmdHelp(ctx)
	}

	// Check for shebang execution
	// If first arg is a .c67 file and it starts with #!, we're in shebang mode
	if len(args) > 0 && strings.HasSuffix(args[0], ".c67") {
		content, err := os.ReadFile(args[0])
		if err == nil && len(content) > 2 && content[0] == '#' && content[1] == '!' {
			// Shebang mode - run the file with remaining args
			return cmdRunShebang(ctx, args[0], args[1:])
		}
	}

	// Parse subcommand
	subcmd := args[0]

	switch subcmd {
	case "build":
		if len(args) < 2 {
			return fmt.Errorf("usage: c67 build <file.c67> [-o output]")
		}
		return cmdBuild(ctx, args[1:])

	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: c67 run <file.c67> [args...]")
		}
		return cmdRun(ctx, args[1:])

	case "test":
		return cmdTest(ctx, args[1:])

	case "help", "--help", "-h":
		return cmdHelp(ctx)

	case "version", "--version", "-V":
		fmt.Println(versionString)
		return nil

	default:
		// Check if it's a .c67 file (shorthand for build)
		if strings.HasSuffix(subcmd, ".c67") {
			return cmdBuild(ctx, args)
		}

		// Check if it's a directory (compile all .c67 files)
		info, err := os.Stat(subcmd)
		if err == nil && info.IsDir() {
			return cmdBuildDir(ctx, subcmd)
		}

		// Unknown command
		return fmt.Errorf("unknown command: %s\n\nRun 'c67 help' for usage information", subcmd)
	}
}

// cmdBuild compiles a C67 source file to an executable
// Confidence that this function is working: 85%
func cmdBuild(ctx *CommandContext, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: c67 build <file.c67> [-o output]")
	}

	// Collect input files (all non-flag arguments)
	inputFiles := []string{}
	outputPath := ""
	
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			outputPath = args[i+1]
			i++ // Skip the output filename
		} else if !strings.HasPrefix(args[i], "-") {
			inputFiles = append(inputFiles, args[i])
		}
	}

	if len(inputFiles) == 0 {
		return fmt.Errorf("no input files specified")
	}

	// Check all files exist
	for _, file := range inputFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", file)
		}
	}

	// If not in args, use context output path (from main -o flag)
	if outputPath == "" && ctx.OutputPath != "" {
		outputPath = ctx.OutputPath
	}

	// Auto-detect Windows target from .exe extension
	if outputPath != "" && strings.HasSuffix(strings.ToLower(outputPath), ".exe") && ctx.Platform.OS != OSWindows {
		ctx.Platform.OS = OSWindows
		if ctx.Verbose {
			fmt.Fprintf(os.Stderr, "Auto-detected Windows target from .exe output filename\n")
		}
	}

	// If still no output path, use first input filename without extension
	if outputPath == "" {
		outputPath = strings.TrimSuffix(filepath.Base(inputFiles[0]), ".c67")
		if ctx.Platform.OS == OSWindows {
			outputPath += ".exe"
		}
	}

	// Enable single-file mode to prevent automatic sibling loading
	oldSingleFlag := SingleFlag
	if !ctx.SingleFile {
		SingleFlag = true
		defer func() { SingleFlag = oldSingleFlag }()
	}

	if ctx.Verbose {
		if len(inputFiles) == 1 {
			fmt.Fprintf(os.Stderr, "Building %s -> %s\n", inputFiles[0], outputPath)
		} else {
			fmt.Fprintf(os.Stderr, "Building %d files -> %s\n", len(inputFiles), outputPath)
		}
	}

	// Compile - if multiple files, concatenate and compile
	var err error
	if len(inputFiles) == 1 {
		err = CompileC67WithOptions(inputFiles[0], outputPath, ctx.Platform, ctx.OptTimeout, ctx.Verbose)
	} else {
		// Multi-file: concatenate sources
		var combinedSource strings.Builder
		for i, file := range inputFiles {
			content, readErr := os.ReadFile(file)
			if readErr != nil {
				return fmt.Errorf("failed to read %s: %v", file, readErr)
			}
			if i > 0 {
				combinedSource.WriteString("\n")
			}
			combinedSource.Write(content)
			if ctx.Verbose {
				fmt.Fprintf(os.Stderr, "  + %s (%d bytes)\n", file, len(content))
			}
		}

		// Write combined source to temp file
		tmpFile, tmpErr := os.CreateTemp("", "c67_multi_*.c67")
		if tmpErr != nil {
			return fmt.Errorf("failed to create temp file: %v", tmpErr)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, writeErr := tmpFile.WriteString(combinedSource.String()); writeErr != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write combined source: %v", writeErr)
		}
		tmpFile.Close()

		// Compile the combined file
		err = CompileC67WithOptions(tmpPath, outputPath, ctx.Platform, ctx.OptTimeout, ctx.Verbose)
	}

	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	if ctx.Verbose {
		fmt.Printf("Built: %s\n", outputPath)
	}

	return nil
}

// cmdRun compiles a C67 source file to /dev/shm and executes it
func cmdRun(ctx *CommandContext, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: c67 run <file.c67> [args...]")
	}

	inputFile := args[0]
	programArgs := args[1:]

	// Create temporary executable in /dev/shm (Linux RAM disk for fast execution)
	// Fall back to temp directory if /dev/shm doesn't exist
	tmpDir := "/dev/shm"
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		tmpDir = os.TempDir()
	}

	// Create unique temporary filename
	baseName := strings.TrimSuffix(filepath.Base(inputFile), ".c67")
	tmpExec := filepath.Join(tmpDir, fmt.Sprintf("c67_run_%s_%d", baseName, os.Getpid()))

	// Enable single-file mode when running a specific file
	oldSingleFlag := SingleFlag
	if !ctx.SingleFile {
		SingleFlag = true
		defer func() { SingleFlag = oldSingleFlag }()
	}

	if ctx.Verbose {
		fmt.Fprintf(os.Stderr, "Compiling %s -> %s (single-file mode)\n", inputFile, tmpExec)
	}

	// Compile
	err := CompileC67WithOptions(inputFile, tmpExec, ctx.Platform, ctx.OptTimeout, ctx.Verbose)
	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	// Ensure cleanup
	defer os.Remove(tmpExec)

	if ctx.Verbose {
		fmt.Fprintf(os.Stderr, "Running %s\n", tmpExec)
	}

	// Execute with arguments
	cmd := exec.Command(tmpExec, programArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Program ran but exited with non-zero status
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("execution failed: %v", err)
	}

	return nil
}

// cmdRunShebang handles shebang execution (#!/usr/bin/c67)
func cmdRunShebang(ctx *CommandContext, scriptPath string, scriptArgs []string) error {
	// In shebang mode, we compile and run immediately
	// This is similar to cmdRun but optimized for shebang use

	tmpDir := "/dev/shm"
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		tmpDir = os.TempDir()
	}

	baseName := strings.TrimSuffix(filepath.Base(scriptPath), ".c67")
	tmpExec := filepath.Join(tmpDir, fmt.Sprintf("c67_shebang_%s_%d", baseName, os.Getpid()))

	// Enable single-file mode for shebang scripts
	oldSingleFlag := SingleFlag
	SingleFlag = true
	defer func() { SingleFlag = oldSingleFlag }()

	// Compile (quietly unless verbose mode)
	err := CompileC67WithOptions(scriptPath, tmpExec, ctx.Platform, ctx.OptTimeout, ctx.Verbose)
	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	defer os.Remove(tmpExec)

	// Execute with script arguments
	cmd := exec.Command(tmpExec, scriptArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("execution failed: %v", err)
	}

	return nil
}

// cmdBuildDir finds the main .c67 file in a directory and compiles it
// (does not compile test files or library files)
func cmdBuildDir(ctx *CommandContext, dirPath string) error {
	matches, err := filepath.Glob(filepath.Join(dirPath, "*.c67"))
	if err != nil {
		return fmt.Errorf("failed to find .c67 files: %v", err)
	}

	// Filter out test files
	var nonTestFiles []string
	for _, file := range matches {
		baseName := filepath.Base(file)
		if !strings.HasPrefix(baseName, "test_") {
			nonTestFiles = append(nonTestFiles, file)
		}
	}

	if len(nonTestFiles) == 0 {
		return fmt.Errorf("no non-test .c67 files found in %s", dirPath)
	}

	// Find the file with main function
	var mainFile string
	for _, file := range nonTestFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		// Check for main function definition (various forms)
		contentStr := string(content)
		if strings.Contains(contentStr, "main = {") ||
			strings.Contains(contentStr, "main={") ||
			strings.Contains(contentStr, "main := ") ||
			strings.Contains(contentStr, "main:=") {
			mainFile = file
			break
		}
	}

	if mainFile == "" {
		return fmt.Errorf("no main function found in .c67 files in %s", dirPath)
	}

	// Compile the main file
	outputPath := strings.TrimSuffix(filepath.Base(mainFile), ".c67")
	if ctx.Platform.OS == OSWindows {
		outputPath += ".exe"
	}

	if ctx.Verbose {
		fmt.Fprintf(os.Stderr, "Building %s -> %s\n", mainFile, outputPath)
	}

	// Don't use single-file mode - allow imports from same directory
	oldSingleFlag := SingleFlag
	SingleFlag = false
	defer func() { SingleFlag = oldSingleFlag }()

	err = CompileC67WithOptions(mainFile, outputPath, ctx.Platform, ctx.OptTimeout, ctx.Verbose)
	if err != nil {
		return fmt.Errorf("compilation of %s failed: %v", mainFile, err)
	}

	if ctx.Verbose {
		fmt.Printf("Built: %s\n", outputPath)
	}

	return nil
}

// cmdTest runs all test_*.c67 and *_test.c67 files in the current directory
func cmdTest(ctx *CommandContext, args []string) error {
	// Determine directory to search (only consider non-flag arguments)
	searchDir := "."
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			searchDir = arg
			break
		}
	}

	// Find all test files: test_*.c67 and *_test.c67
	matchesPrefix, err := filepath.Glob(filepath.Join(searchDir, "test_*.c67"))
	if err != nil {
		return fmt.Errorf("failed to find test files: %v", err)
	}

	matchesSuffix, err := filepath.Glob(filepath.Join(searchDir, "*_test.c67"))
	if err != nil {
		return fmt.Errorf("failed to find test files: %v", err)
	}

	// Combine and deduplicate
	matchMap := make(map[string]bool)
	for _, m := range matchesPrefix {
		matchMap[m] = true
	}
	for _, m := range matchesSuffix {
		matchMap[m] = true
	}

	matches := make([]string, 0, len(matchMap))
	for m := range matchMap {
		matches = append(matches, m)
	}

	if len(matches) == 0 {
		if !ctx.Quiet {
			fmt.Printf("No test files found in %s\n", searchDir)
		}
		return nil
	}

	if ctx.Verbose {
		fmt.Fprintf(os.Stderr, "Found %d test file(s)\n", len(matches))
	}

	// Track test results
	passed := 0
	failed := 0
	failedTests := []string{}

	// Compile all test files together into one test executable with generated main
	for _, testFile := range matches {
		testName := filepath.Base(testFile)

		// Validate test file - ensure no main function
		content, err := os.ReadFile(testFile)
		if err != nil {
			return fmt.Errorf("failed to read test file %s: %v", testFile, err)
		}

		contentStr := string(content)
		if strings.Contains(contentStr, "main = {") ||
			strings.Contains(contentStr, "main={") ||
			strings.Contains(contentStr, "main := ") ||
			strings.Contains(contentStr, "main:=") {
			return fmt.Errorf("test file %s should not contain a main function", testName)
		}

		if !ctx.Quiet {
			fmt.Printf("Running %s... ", testName)
		}

		// Create temporary executable
		tmpDir := "/dev/shm"
		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			tmpDir = os.TempDir()
		}

		baseName := strings.TrimSuffix(testName, ".c67")
		tmpExec := filepath.Join(tmpDir, fmt.Sprintf("c67_test_%s_%d", baseName, os.Getpid()))

		// Generate a test runner in the same directory as the test file for proper imports
		testDir := filepath.Dir(testFile)
		testRunnerPath := filepath.Join(testDir, fmt.Sprintf("_test_runner_%d.c67", os.Getpid()))

		// Parse test file to find test functions
		testFunctions, parseErr := findTestFunctions(testFile)
		if parseErr != nil {
			if !ctx.Quiet {
				fmt.Printf("FAIL (parse error)\n")
			}
			if ctx.Verbose {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", parseErr)
			}
			failed++
			failedTests = append(failedTests, testName)
			continue
		}

		// Generate test runner
		runnerErr := generateTestRunner(testRunnerPath, testFile, testFunctions)
		if runnerErr != nil {
			if !ctx.Quiet {
				fmt.Printf("FAIL (runner generation error)\n")
			}
			if ctx.Verbose {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", runnerErr)
			}
			failed++
			failedTests = append(failedTests, testName)
			continue
		}
		defer os.Remove(testRunnerPath)

		// Enable single-file mode for each test
		oldSingleFlag := SingleFlag
		SingleFlag = false // Allow importing from same directory

		// Compile the test runner
		err = CompileC67WithOptions(testRunnerPath, tmpExec, ctx.Platform, ctx.OptTimeout, false)
		SingleFlag = oldSingleFlag

		if err != nil {
			if !ctx.Quiet {
				fmt.Printf("FAIL (compilation error)\n")
			}
			if ctx.Verbose {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			}
			failed++
			failedTests = append(failedTests, testName)
			continue
		}

		// Run the test
		cmd := exec.Command(tmpExec)
		cmd.Stdin = os.Stdin
		if ctx.Verbose {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}

		err = cmd.Run()
		os.Remove(tmpExec)

		if err != nil {
			if !ctx.Quiet {
				fmt.Printf("FAIL\n")
			}
			if ctx.Verbose {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			}
			failed++
			failedTests = append(failedTests, testName)
		} else {
			if !ctx.Quiet {
				fmt.Printf("PASS\n")
			}
			passed++
		}
	}

	// Print summary
	if !ctx.Quiet {
		fmt.Printf("\n")
		if failed == 0 {
			fmt.Printf("✓ All tests passed (%d/%d)\n", passed, passed+failed)
		} else {
			fmt.Printf("✗ %d test(s) failed, %d passed (%d total)\n", failed, passed, passed+failed)
			fmt.Printf("\nFailed tests:\n")
			for _, name := range failedTests {
				fmt.Printf("  - %s\n", name)
			}
		}
	}

	if failed > 0 {
		os.Exit(1)
	}

	return nil
}

// cmdHelp displays usage information
func cmdHelp(ctx *CommandContext) error {
	fmt.Printf(`c67 - The C67 Compiler (Version 1.5.0)

USAGE:
    c67 <command> [arguments]

COMMANDS:
    build <file.c67>      Compile a C67 source file to an executable
    run <file.c67>        Compile and run a C67 program immediately
    test [directory]      Run all test_*.c67 files (default: current directory)
    help                  Show this help message
    version               Show version information

SHORTHAND:
    c67 <file.c67>      Same as 'c67 build <file.c67>'
    c67                  Show this help message (or build if .c67 files found)

FLAGS (can be used with any command):
    -o, --output <file>    Output executable filename (default: input name without .c67)
    -v, --verbose          Verbose mode (show detailed compilation info)
    -q, --quiet            Quiet mode (suppress progress messages)
    --arch <arch>          Target architecture: amd64, arm64, riscv64 (default: amd64)
    --os <os>              Target OS: linux, darwin, freebsd (default: linux)
    --target <platform>    Target platform: amd64-linux, arm64-macos, etc.
    --opt-timeout <secs>   Optimization timeout in seconds (default: 2.0)
    -u, --update-deps      Update dependency repositories from Git
    -s, --single           Compile single file only (don't load siblings)

EXAMPLES:
    # Compile a program
    c67 build hello.c67
    c67 build hello.c67 -o hello

    # Compile and run immediately
    c67 run hello.c67
    c67 run server.c67 --port 8080

    # Shorthand compilation
    c67 hello.c67

    # Run tests
    c67 test
    c67 test ./tests

    # Shebang execution (add #!/usr/bin/c67 to first line of .c67 file)
    chmod +x script.c67
    ./script.c67 arg1 arg2

DOCUMENTATION:
    For language documentation, see LANGUAGESPEC.md
    For help or bug reports: https://github.com/anthropics/c67/issues

`)
	return nil
}

// findTestFunctions parses a test file and finds all functions that start with test or Test
func findTestFunctions(testFile string) ([]string, error) {
	content, err := os.ReadFile(testFile)
	if err != nil {
		return nil, err
	}

	// Parse the file
	parser := NewParserWithFilename(string(content), testFile)
	program := parser.ParseProgram()

	if parser.errors.HasErrors() {
		return nil, fmt.Errorf("parse errors in %s", testFile)
	}

	// Find all function definitions that start with test or Test
	var testFuncs []string
	for _, stmt := range program.Statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			name := assign.Name
			if strings.HasPrefix(name, "test") || strings.HasPrefix(name, "Test") {
				testFuncs = append(testFuncs, name)
			}
		}
	}

	return testFuncs, nil
}

// generateTestRunner creates a test runner file that calls all test functions
// The runner includes the test file content inline and imports the current directory
func generateTestRunner(runnerPath, testFile string, testFunctions []string) error {
	// Read the test file content
	testContent, err := os.ReadFile(testFile)
	if err != nil {
		return err
	}

	var builder strings.Builder

	// Include the test file content directly (inline it)
	// But remove any import statements from the test file
	testLines := strings.Split(string(testContent), "\n")
	for _, line := range testLines {
		trimmed := strings.TrimSpace(line)
		// Skip import statements and empty lines
		if !strings.HasPrefix(trimmed, "import ") && trimmed != "" {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	// Generate main function that calls all test functions
	builder.WriteString("\nmain = {\n")

	for _, testFunc := range testFunctions {
		// Call each test function - they contain or! internally to check assertions
		builder.WriteString(fmt.Sprintf("    %s()\n", testFunc))
	}

	builder.WriteString("    exit(0)\n")
	builder.WriteString("}\n")

	// Write the runner file
	return os.WriteFile(runnerPath, []byte(builder.String()), 0644)
}
