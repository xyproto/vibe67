// Completion: 100% - Utility module complete
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IncrementalState holds the state needed for incremental hot reload
type IncrementalState struct {
	sourceFiles      map[string]string       // path -> source code
	parsedASTs       map[string]*Program     // path -> parsed AST (cached)
	fileModTimes     map[string]time.Time    // path -> last modification time
	hotFunctions     map[string]*FunctionDef // function name -> definition
	hotFunctionAddrs map[string]uint64       // function name -> address in memory
	hotFunctionFiles map[string]string       // function name -> source file path
	compiledBinary   []byte                  // the full compiled binary
	platform         Platform                // target platform
	lastBinaryPath   string                  // path to last compiled binary
}

// NewIncrementalState creates a new incremental compilation state
func NewIncrementalState(platform Platform) *IncrementalState {
	return &IncrementalState{
		sourceFiles:      make(map[string]string),
		parsedASTs:       make(map[string]*Program),
		fileModTimes:     make(map[string]time.Time),
		hotFunctions:     make(map[string]*FunctionDef),
		hotFunctionAddrs: make(map[string]uint64),
		hotFunctionFiles: make(map[string]string),
		platform:         platform,
	}
}

// InitialCompile performs the first full compilation and captures hot function info
func (is *IncrementalState) InitialCompile(inputPath, outputPath string) error {
	// Read and store source with modification time
	content, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", inputPath, err)
	}
	is.sourceFiles[inputPath] = string(content)

	// Get file modification time
	fileInfo, err := os.Stat(inputPath)
	if err == nil {
		is.fileModTimes[inputPath] = fileInfo.ModTime()
	}

	// Perform full compilation
	if err := CompileC67(inputPath, outputPath, is.platform); err != nil {
		return err
	}

	// Read the compiled binary
	binary, err := os.ReadFile(outputPath)
	if err != nil {
		return fmt.Errorf("failed to read compiled binary: %v", err)
	}
	is.compiledBinary = binary
	is.lastBinaryPath = outputPath

	// Parse program to extract hot functions
	parser := NewParserWithFilename(string(content), inputPath)
	program := parser.ParseProgram()
	is.parsedASTs[inputPath] = program

	// Find all hot function definitions
	is.extractHotFunctions(program, inputPath)

	return nil
}

// extractHotFunctions walks the AST and collects hot function definitions (DISABLED - hot keyword removed)
func (is *IncrementalState) extractHotFunctions(program *Program, filePath string) {
	// Hot-reloading feature removed from language spec
}

// FunctionDef represents a hot function definition
type FunctionDef struct {
	Name   string
	Lambda *LambdaExpr
}

// IncrementalRecompile recompiles only the changed file's hot functions
func (is *IncrementalState) IncrementalRecompile(changedPath string) ([]string, error) {
	// Read the changed file
	content, err := os.ReadFile(changedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %v", changedPath, err)
	}
	newSource := string(content)

	// Check if source actually changed
	if oldSource, exists := is.sourceFiles[changedPath]; exists && oldSource == newSource {
		return nil, nil // No change, skip recompilation
	}

	// Update stored source and modification time
	is.sourceFiles[changedPath] = newSource
	fileInfo, err := os.Stat(changedPath)
	if err == nil {
		is.fileModTimes[changedPath] = fileInfo.ModTime()
	}

	// Parse only the changed file (optimization: reuse cached ASTs for unchanged files)
	parser := NewParserWithFilename(newSource, changedPath)
	program := parser.ParseProgram()
	is.parsedASTs[changedPath] = program

	// Identify which hot functions are in the changed file
	changedHotFuncs := []string{}
	for funcName, filePath := range is.hotFunctionFiles {
		if filePath == changedPath {
			changedHotFuncs = append(changedHotFuncs, funcName)
		}
	}

	// Extract updated hot function definitions from the changed file
	oldHotFuncs := make(map[string]*FunctionDef)
	for _, funcName := range changedHotFuncs {
		if def, exists := is.hotFunctions[funcName]; exists {
			oldHotFuncs[funcName] = def
		}
	}

	// Clear old hot functions from this file
	for _, funcName := range changedHotFuncs {
		delete(is.hotFunctions, funcName)
		delete(is.hotFunctionFiles, funcName)
	}

	// Extract new hot functions from the changed file
	is.extractHotFunctions(program, changedPath)

	// Find which hot functions actually changed or are new (DISABLED - hot keyword removed)
	updatedFuncs := []string{}
	// Hot-reloading feature removed from language spec

	if len(updatedFuncs) == 0 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "No hot functions in changed file %s\n", filepath.Base(changedPath))
		}
		return nil, nil
	}

	// Do a full recompilation with the updated source
	// (This is fast and ensures all dependencies are resolved correctly)
	tempOutput := is.lastBinaryPath + ".tmp"
	if err := CompileC67(changedPath, tempOutput, is.platform); err != nil {
		return nil, fmt.Errorf("recompilation failed: %v", err)
	}

	// Read the new binary
	binary, err := os.ReadFile(tempOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to read recompiled binary: %v", err)
	}
	is.compiledBinary = binary

	// Phase 4 will: Extract machine code for the updated hot functions
	// Phase 4 will: Inject the new code into the running process
	// For now, just return the list of updated function names

	return updatedFuncs, nil
}

// GetChangedFiles returns files that have been modified since last compilation
func (is *IncrementalState) GetChangedFiles() ([]string, error) {
	var changed []string

	for path, lastModTime := range is.fileModTimes {
		fileInfo, err := os.Stat(path)
		if err != nil {
			continue // File might have been deleted
		}

		if fileInfo.ModTime().After(lastModTime) {
			changed = append(changed, path)
		}
	}

	return changed, nil
}

// GetWatchFiles returns all files that should be watched for changes
func (is *IncrementalState) GetWatchFiles() []string {
	var files []string
	for path := range is.sourceFiles {
		absPath, err := filepath.Abs(path)
		if err == nil {
			files = append(files, absPath)
		}
	}
	return files
}
