// Completion: 100% - C FFI complete with automatic header parsing and dynamic linking
package main

import (
	"bufio"
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CFunctionParam represents a parameter in a C function signature
type CFunctionParam struct {
	Type string // e.g., "const char*", "int", "SDL_Window*"
	Name string // parameter name (may be empty)
}

// CFunctionSignature represents a parsed C function declaration
type CFunctionSignature struct {
	ReturnType string           // e.g., "int", "SDL_Window*", "void"
	Params     []CFunctionParam // function parameters
}

// CHeaderConstants stores constants and function signatures extracted from C headers
type CHeaderConstants struct {
	Constants map[string]int64               // constant name -> value
	Macros    map[string]string              // macro name -> definition (for simple function-like macros)
	Functions map[string]*CFunctionSignature // function name -> signature
}

// NewCHeaderConstants creates a new constants store
func NewCHeaderConstants() *CHeaderConstants {
	return &CHeaderConstants{
		Constants: make(map[string]int64),
		Macros:    make(map[string]string),
		Functions: make(map[string]*CFunctionSignature),
	}
}

// ExtractConstantsFromLibrary extracts #define constants from a C library's headers
// Uses pkg-config to find include paths and parses the main header file
func ExtractConstantsFromLibrary(libName string) (*CHeaderConstants, error) {
	constants := NewCHeaderConstants()

	// Get include paths from pkg-config
	includePaths, err := getPkgConfigIncludes(libName)
	if err != nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: pkg-config not available for %s: %v\n", libName, err)
		}
		// Fallback to standard paths including local include directory
		includePaths = []string{"./include", "/usr/include", "/usr/local/include"}
	}

	// Try to find and parse the main header file
	headerFile := findMainHeader(libName, includePaths)
	if headerFile == "" {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: could not find header for %s in paths: %v\n", libName, includePaths)
		}
		return constants, nil
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Parsing C header: %s\n", headerFile)
	}

	// Parse the header file for #define constants
	err = parseHeaderFile(headerFile, constants)
	if err != nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: error parsing %s: %v\n", headerFile, err)
		}
		return constants, nil
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Extracted %d constants from %s\n", len(constants.Constants), libName)
	}

	return constants, nil
}

// getPkgConfigIncludes runs pkg-config --cflags and extracts include paths
func getPkgConfigIncludes(libName string) ([]string, error) {
	// Try different pkg-config names for the library
	pkgNames := []string{libName}

	// Add common variants
	switch libName {
	case "sdl3":
		pkgNames = append(pkgNames, "SDL3", "sdl3-dev")
	case "raylib":
		pkgNames = append(pkgNames, "raylib5", "RayLib5")
	}

	var lastErr error
	pkgConfigSucceeded := false
	for _, pkgName := range pkgNames {
		cmd := exec.Command("pkg-config", "--cflags", pkgName)
		output, err := cmd.Output()
		if err != nil {
			lastErr = err
			continue
		}

		// pkg-config succeeded for this package
		pkgConfigSucceeded = true

		// Parse -I flags from output
		var includes []string
		flags := strings.Fields(string(output))
		for _, flag := range flags {
			if strings.HasPrefix(flag, "-I") {
				includePath := strings.TrimPrefix(flag, "-I")
				includes = append(includes, includePath)
			}
		}

		if len(includes) > 0 {
			return includes, nil
		}

		// pkg-config succeeded but no -I flags, use standard paths
		break
	}

	// If pkg-config succeeded but no -I flags found, use standard include paths
	if pkgConfigSucceeded {
		standardPaths := []string{"./include", "/usr/include", "/usr/local/include"}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "No -I flags from pkg-config, using standard paths\n")
		}
		return standardPaths, nil
	}

	return nil, lastErr
}

// findMainHeader tries to find the main header file for a library
func findMainHeader(libName string, includePaths []string) string {
	// Library-specific patterns (no formatting needed)
	specificPatterns := map[string][]string{
		"sdl3": {"SDL3/SDL.h"},
		"SDL3": {"SDL3/SDL.h"},
	}

	// Generic header file patterns (will be formatted with libName)
	genericPatterns := []string{
		"%s.h",       // raylib.h
		"%s/%s.h",    // SDL3/SDL3.h (uppercase lib name)
		"lib%s.h",    // libraylib.h
		"%s/lib%s.h", // raylib/libraylib.h
	}

	for _, includePath := range includePaths {
		// Try library-specific patterns first
		if patterns, ok := specificPatterns[libName]; ok {
			for _, pattern := range patterns {
				headerPath := filepath.Join(includePath, pattern)
				if _, err := os.Stat(headerPath); err == nil {
					return headerPath
				}
			}
		}

		// Try generic patterns
		for _, pattern := range genericPatterns {
			// For single %s patterns
			if strings.Count(pattern, "%s") == 1 {
				headerPath := filepath.Join(includePath, fmt.Sprintf(pattern, libName))
				if _, err := os.Stat(headerPath); err == nil {
					return headerPath
				}
			}

			// For double %s patterns, try both lowercase and uppercase
			if strings.Count(pattern, "%s") == 2 {
				// Try lowercase
				headerPath := filepath.Join(includePath, fmt.Sprintf(pattern, libName, libName))
				if _, err := os.Stat(headerPath); err == nil {
					return headerPath
				}
				// Try uppercase second argument
				headerPath = filepath.Join(includePath, fmt.Sprintf(pattern, libName, strings.ToUpper(libName)))
				if _, err := os.Stat(headerPath); err == nil {
					return headerPath
				}
			}
		}
	}

	return ""
}

// parseHeaderFile parses a C header file and extracts #define constants
func parseHeaderFile(headerPath string, constants *CHeaderConstants) error {
	return parseHeaderFileWithDepth(headerPath, constants, make(map[string]bool), 0)
}

const MaxIncludeDepth = 20 // Maximum #include recursion depth

func parseHeaderFileWithDepth(headerPath string, constants *CHeaderConstants, visited map[string]bool, depth int) error {
	// Safety: prevent infinite recursion
	if depth > MaxIncludeDepth {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: max include depth (%d) reached, stopping recursion\n", MaxIncludeDepth)
		}
		return nil
	}

	// Avoid parsing the same header twice
	if visited[headerPath] {
		return nil
	}
	visited[headerPath] = true

	// Use the new CParser for better parsing
	parser := NewCParser()
	parser.results = constants // Use existing constants map to accumulate results

	parsedResults, err := parser.ParseFile(headerPath)
	if err != nil {
		// Fallback to old regex-based parser if new parser fails
		fmt.Fprintf(os.Stderr, "  CParser failed for %s, using regex fallback: %v\n", headerPath, err)
		return parseHeaderFileRecursive(headerPath, constants, visited)
	}

	// Merge results into constants (parser modifies the results map directly, but be safe)
	for k, v := range parsedResults.Constants {
		constants.Constants[k] = v
	}
	for k, v := range parsedResults.Macros {
		constants.Macros[k] = v
	}
	for k, v := range parsedResults.Functions {
		constants.Functions[k] = v
	}

	if VerboseMode {
		if len(parsedResults.Functions) > 0 {
			fmt.Fprintf(os.Stderr, "  Parsed %d function signatures from %s\n", len(parsedResults.Functions), headerPath)
		} else {
			fmt.Fprintf(os.Stderr, "  Warning: No function signatures found in %s\n", headerPath)
		}
	}

	// Handle #include directives for recursive parsing (CParser doesn't follow includes yet)
	return parseIncludesWithDepth(headerPath, constants, visited, depth)
}

// parseIncludes recursively parses #include directives
func parseIncludes(headerPath string, constants *CHeaderConstants, visited map[string]bool) error {
	return parseIncludesWithDepth(headerPath, constants, visited, 0)
}

func parseIncludesWithDepth(headerPath string, constants *CHeaderConstants, visited map[string]bool, depth int) error {
	file, err := os.Open(headerPath)
	if err != nil {
		return err
	}
	defer file.Close()

	headerDir := filepath.Dir(headerPath)
	includeRegex := regexp.MustCompile(`^\s*#\s*include\s+[<"](.+)[>"]`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for #include directive
		if includeMatches := includeRegex.FindStringSubmatch(line); len(includeMatches) == 2 {
			includedFile := includeMatches[1]

			// Try to find the included header
			var includedPath string

			// If it's an absolute library include like <SDL3/SDL_init.h>
			// Try standard include paths
			if strings.Contains(includedFile, "/") {
				for _, standardPath := range []string{"./include", "/usr/include", "/usr/local/include"} {
					testPath := filepath.Join(standardPath, includedFile)
					if _, err := os.Stat(testPath); err == nil {
						includedPath = testPath
						break
					}
				}
			} else {
				// Relative include - try relative to current header directory
				relativePath := filepath.Join(headerDir, includedFile)
				if _, err := os.Stat(relativePath); err == nil {
					includedPath = relativePath
				}
			}

			if includedPath != "" {
				// Parse the included header with increased depth
				parseHeaderFileWithDepth(includedPath, constants, visited, depth+1)
			}
		}
	}

	return scanner.Err()
}

func parseHeaderFileRecursive(headerPath string, constants *CHeaderConstants, visited map[string]bool) error {
	// Avoid parsing the same header twice
	if visited[headerPath] {
		return nil
	}
	visited[headerPath] = true

	file, err := os.Open(headerPath)
	if err != nil {
		return err
	}
	defer file.Close()

	headerDir := filepath.Dir(headerPath)

	// Regular expressions for parsing #define and #include
	// Match: #define NAME VALUE
	defineRegex := regexp.MustCompile(`^\s*#\s*define\s+([A-Z_][A-Z0-9_]*)\s+(.+)$`)

	// Match function-like macro: #define NAME(param) body
	macroRegex := regexp.MustCompile(`^\s*#\s*define\s+([A-Z_][A-Z0-9_]*)\(([^)]*)\)\s+(.+)$`)

	// Match: #include "file.h" or #include <file.h>
	includeRegex := regexp.MustCompile(`^\s*#\s*include\s+[<"](.+)[>"]`)

	// Match C function declaration: extern type function_name(params);
	// Handles SDL-style macros: extern SDL_DECLSPEC type SDLCALL function_name(params);
	// Also handles simple declarations: extern type function_name(params);
	funcRegex := regexp.MustCompile(`^\s*(?:extern\s+)?(?:[A-Z_]+\s+)?([A-Za-z_][A-Za-z0-9_*\s]+?)\s+(?:[A-Z_]+\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*;`)

	// Match hex numbers: 0x1234, 0X1234
	hexRegex := regexp.MustCompile(`^0[xX]([0-9a-fA-F]+)`)

	// Match decimal numbers: 123, -456
	decimalRegex := regexp.MustCompile(`^-?\d+`)

	// Match binary: 0b1010
	binaryRegex := regexp.MustCompile(`^0[bB]([01]+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip comment-only lines
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Remove inline comments (we'll handle them later when parsing the value)
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}

		// Check for #include directive
		if includeMatches := includeRegex.FindStringSubmatch(line); len(includeMatches) == 2 {
			includedFile := includeMatches[1]

			// Try to find the included header
			var includedPath string

			// If it's an absolute library include like <SDL3/SDL_init.h>
			// Try standard include paths
			if strings.Contains(includedFile, "/") {
				for _, standardPath := range []string{"./include", "/usr/include", "/usr/local/include"} {
					testPath := filepath.Join(standardPath, includedFile)
					if _, err := os.Stat(testPath); err == nil {
						includedPath = testPath
						break
					}
				}
			} else {
				// Relative include - try relative to current header directory
				relativePath := filepath.Join(headerDir, includedFile)
				if _, err := os.Stat(relativePath); err == nil {
					includedPath = relativePath
				}
			}

			if includedPath != "" {
				// Recursively parse the included header
				parseHeaderFileRecursive(includedPath, constants, visited)
			}
			continue
		}

		// Check for C function declaration
		if funcMatches := funcRegex.FindStringSubmatch(line); len(funcMatches) == 4 {
			returnType := strings.TrimSpace(funcMatches[1])
			funcName := funcMatches[2]
			paramsStr := strings.TrimSpace(funcMatches[3])

			// Parse parameters
			sig := &CFunctionSignature{
				ReturnType: returnType,
				Params:     parseFunctionParams(paramsStr),
			}

			constants.Functions[funcName] = sig

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Function: %s %s(...)\n", returnType, funcName)
			}
			continue
		}

		// Check for function-like macro
		if macroMatches := macroRegex.FindStringSubmatch(line); len(macroMatches) == 4 {
			macroName := macroMatches[1]
			params := strings.TrimSpace(macroMatches[2])
			body := strings.TrimSpace(macroMatches[3])

			// Store the macro for later expansion
			constants.Macros[macroName] = body

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Macro: %s(%s) = %s\n", macroName, params, body)
			}
			continue
		}

		matches := defineRegex.FindStringSubmatch(line)
		if len(matches) != 3 {
			// Debug: show lines that don't match
			if VerboseMode && strings.HasPrefix(strings.TrimSpace(line), "#define SDL_INIT") {
				fmt.Fprintf(os.Stderr, "  Regex didn't match: %s\n", line)
			}
			continue
		}

		name := matches[1]
		valueStr := strings.TrimSpace(matches[2])

		// Remove inline comments
		if idx := strings.Index(valueStr, "//"); idx != -1 {
			valueStr = strings.TrimSpace(valueStr[:idx])
		}
		if idx := strings.Index(valueStr, "/*"); idx != -1 {
			valueStr = strings.TrimSpace(valueStr[:idx])
		}

		// Remove C type suffixes: u, l, ul, ll, ull, etc.
		valueStr = regexp.MustCompile(`[uUlL]+$`).ReplaceAllString(valueStr, "")

		// Try to expand macros in the value
		valueStr = expandSimpleMacros(valueStr, constants.Macros)

		// Try to parse the value
		var value int64
		var parsed bool

		// Try hex
		if hexMatches := hexRegex.FindStringSubmatch(valueStr); len(hexMatches) > 1 {
			if v, err := strconv.ParseInt(hexMatches[1], 16, 64); err == nil {
				value = v
				parsed = true
			}
		}

		// Try binary
		if !parsed {
			if binMatches := binaryRegex.FindStringSubmatch(valueStr); len(binMatches) > 1 {
				if v, err := strconv.ParseInt(binMatches[1], 2, 64); err == nil {
					value = v
					parsed = true
				}
			}
		}

		// Try decimal
		if !parsed {
			if decMatches := decimalRegex.FindString(valueStr); decMatches != "" {
				if v, err := strconv.ParseInt(decMatches, 10, 64); err == nil {
					value = v
					parsed = true
				}
			}
		}

		// Try to resolve other constants (e.g., SDL_INIT_VIDEO might reference another constant)
		if !parsed {
			// Check if it's a reference to another constant
			if existingValue, ok := constants.Constants[valueStr]; ok {
				value = existingValue
				parsed = true
			}
		}

		// Try simple expressions like (1 << 5) or (0x00000020)
		if !parsed {
			value, parsed = evalSimpleExpression(valueStr, constants)
		}

		if parsed {
			constants.Constants[name] = value
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Constant: %s = %d (0x%x)\n", name, value, value)
			}
		} else {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Failed to parse: %s = %s\n", name, valueStr)
			}
		}
	}

	return scanner.Err()
}

// evalSimpleExpression evaluates simple C expressions like (1 << 5) or (0x20)
func evalSimpleExpression(expr string, constants *CHeaderConstants) (int64, bool) {
	// Remove parentheses and whitespace
	expr = strings.TrimSpace(expr)
	expr = strings.Trim(expr, "()")
	expr = strings.TrimSpace(expr)

	// Handle bitwise shift: N << M
	if strings.Contains(expr, "<<") {
		parts := strings.Split(expr, "<<")
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])

			leftVal, leftOk := parseValue(left, constants)
			rightVal, rightOk := parseValue(right, constants)

			if leftOk && rightOk {
				return leftVal << rightVal, true
			}
		}
	}

	// Handle bitwise OR: N | M
	if strings.Contains(expr, "|") {
		parts := strings.Split(expr, "|")
		if len(parts) >= 2 {
			result := int64(0)
			allOk := true
			for _, part := range parts {
				val, ok := parseValue(strings.TrimSpace(part), constants)
				if !ok {
					allOk = false
					break
				}
				result |= val
			}
			if allOk {
				return result, true
			}
		}
	}

	// Try direct parse
	return parseValue(expr, constants)
}

// parseValue parses a single value (number or constant reference)
func parseValue(s string, constants *CHeaderConstants) (int64, bool) {
	s = strings.TrimSpace(s)

	// Try hex
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		if v, err := strconv.ParseInt(s[2:], 16, 64); err == nil {
			return v, true
		}
	}

	// Try decimal
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}

	// Try constant reference
	if val, ok := constants.Constants[s]; ok {
		return val, true
	}

	return 0, false
}

// expandSimpleMacros expands simple function-like macros in a value string
// For example: SDL_UINT64_C(0x0000000000000002) -> 0x0000000000000002ULL
func expandSimpleMacros(value string, macros map[string]string) string {
	// Match macro invocations: NAME(args)
	macroCallRegex := regexp.MustCompile(`([A-Z_][A-Z0-9_]*)\(([^)]*)\)`)

	result := value
	for {
		matches := macroCallRegex.FindStringSubmatch(result)
		if len(matches) != 3 {
			break // No more macros to expand
		}

		macroName := matches[1]
		arg := strings.TrimSpace(matches[2])

		// Check if we have this macro
		if macroBody, ok := macros[macroName]; ok {
			// For simple macros like SDL_UINT64_C(c) -> c##ULL
			// Just append ULL to the argument
			if strings.Contains(macroBody, "##") {
				// Token pasting: c##ULL -> cULL
				parts := strings.Split(macroBody, "##")
				if len(parts) == 2 && strings.Contains(parts[0], "c") {
					// Replace 'c' with the argument and append the suffix
					expanded := arg + strings.TrimSpace(parts[1])
					result = macroCallRegex.ReplaceAllLiteralString(result, expanded)
					continue
				}
			}

			// For other simple macros, just replace with the body
			// (This is a very simplified macro expansion)
			result = macroCallRegex.ReplaceAllLiteralString(result, macroBody)
		} else {
			// Unknown macro - give up
			break
		}
	}

	return result
}

// parseFunctionParams parses C function parameter list
// Examples: "int x, const char* str" -> [{Type:"int", Name:"x"}, {Type:"const char*", Name:"str"}]
func parseFunctionParams(paramsStr string) []CFunctionParam {
	if paramsStr == "" || paramsStr == "void" {
		return nil
	}

	var params []CFunctionParam
	paramParts := strings.Split(paramsStr, ",")

	for _, param := range paramParts {
		param = strings.TrimSpace(param)
		if param == "" || param == "void" {
			continue
		}

		// Split parameter into type and name
		// Handle cases like: "int x", "const char* str", "SDL_Window *window"
		parts := strings.Fields(param)
		if len(parts) == 0 {
			continue
		}

		var paramType, paramName string
		if len(parts) == 1 {
			// Just a type, no parameter name (e.g., "int")
			paramType = parts[0]
		} else {
			// Last part is the name, rest is the type
			paramName = parts[len(parts)-1]
			// Remove any * from the name (e.g., "*window" -> "window")
			paramName = strings.TrimLeft(paramName, "*")

			// Type is everything except the last part
			paramType = strings.Join(parts[:len(parts)-1], " ")

			// If the last type part doesn't end with *, and the name started with *,
			// add it to the type
			if !strings.HasSuffix(paramType, "*") && strings.HasPrefix(parts[len(parts)-1], "*") {
				paramType += "*"
			}
		}

		params = append(params, CFunctionParam{
			Type: paramType,
			Name: paramName,
		})
	}

	return params
}

// isPointerType checks if a C type is a pointer type
func isPointerType(cType string) bool {
	return strings.Contains(cType, "*") || strings.HasSuffix(cType, "Ptr")
}

// ExtractSymbolsFromSo extracts exported function symbols from a .so file using Go's debug/elf
func ExtractSymbolsFromSo(soPath string) ([]string, error) {
	// Open ELF file
	elfFile, err := elf.Open(soPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file: %v", err)
	}
	defer elfFile.Close()

	// Get dynamic symbols
	symbols, err := elfFile.DynamicSymbols()
	if err != nil {
		return nil, fmt.Errorf("failed to read dynamic symbols: %v", err)
	}

	var funcSymbols []string
	for _, sym := range symbols {
		// Only include function symbols (STT_FUNC)
		// elf.STT_FUNC = 2
		if elf.ST_TYPE(sym.Info) == elf.STT_FUNC {
			funcSymbols = append(funcSymbols, sym.Name)
		}
	}

	return funcSymbols, nil
}

// DiscoverFunctionSignatures attempts to discover function signatures for a library
// using multiple strategies in order of preference:
// 1. pkg-config for library information and include paths
// 2. Parse header files for function declarations
// 3. Extract DWARF debug information
// 4. Symbol table analysis (names only, no type info)
func DiscoverFunctionSignatures(libraryName string, soPath string) (map[string]*CFunctionSignature, error) {
	signatures := make(map[string]*CFunctionSignature)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Discovering signatures for %s (%s)\n", libraryName, soPath)
	}

	// Strategy 1: Try pkg-config for include paths
	pkgConfigPaths := tryPkgConfig(libraryName)
	if len(pkgConfigPaths) > 0 && VerboseMode {
		fmt.Fprintf(os.Stderr, "  pkg-config found include paths: %v\n", pkgConfigPaths)
	}

	// Strategy 2: Try to find and parse header files
	headerSigs := tryParseHeaders(libraryName, pkgConfigPaths)
	for name, sig := range headerSigs {
		signatures[name] = sig
	}
	if len(headerSigs) > 0 && VerboseMode {
		fmt.Fprintf(os.Stderr, "  Parsed %d signatures from headers\n", len(headerSigs))
	}

	// Strategy 3: Try DWARF debug info
	dwarfSigs, err := ExtractFunctionSignatures(soPath)
	if err == nil && len(dwarfSigs) > 0 {
		for name, sig := range dwarfSigs {
			if _, exists := signatures[name]; !exists {
				signatures[name] = sig
			}
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "  DWARF extracted %d signatures\n", len(dwarfSigs))
		}
	}

	// Strategy 4: Extract symbol names (no type info)
	// This at least tells us which functions exist
	symbolNames, err := ExtractSymbolsFromSo(soPath)
	if err == nil && VerboseMode {
		fmt.Fprintf(os.Stderr, "  Symbol table has %d function names\n", len(symbolNames))
	}

	if len(signatures) == 0 && VerboseMode {
		fmt.Fprintf(os.Stderr, "  Warning: No type signatures discovered. FFI calls will need explicit casts.\n")
	}

	return signatures, nil
}

// tryPkgConfig attempts to get include paths from pkg-config
func tryPkgConfig(libraryName string) []string {
	cmd := exec.Command("pkg-config", "--cflags-only-I", libraryName)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var paths []string
	fields := strings.Fields(string(output))
	for _, field := range fields {
		if strings.HasPrefix(field, "-I") {
			path := strings.TrimPrefix(field, "-I")
			paths = append(paths, path)
		}
	}
	return paths
}

// tryParseHeaders attempts to parse C header files for function declarations
// Returns discovered function signatures
func tryParseHeaders(libraryName string, additionalPaths []string) map[string]*CFunctionSignature {
	signatures := make(map[string]*CFunctionSignature)

	// Map library names to common header files
	headerMap := map[string][]string{
		"c":       {"math.h", "stdlib.h", "string.h", "stdio.h"},
		"m":       {"math.h"},
		"sdl3":    {"SDL3/SDL.h"},
		"sdl2":    {"SDL2/SDL.h"},
		"GL":      {"GL/gl.h"},
		"GLU":     {"GL/glu.h"},
		"pthread": {"pthread.h"},
	}

	headers, ok := headerMap[libraryName]
	if !ok {
		// Try guessing header name from library name
		// e.g., "sdl3" -> "SDL3/SDL.h" or "libfoo" -> "foo.h"
		guessedHeader := strings.TrimPrefix(libraryName, "lib") + ".h"
		headers = []string{guessedHeader}
	}

	// Try to parse each header
	for _, header := range headers {
		sigs := parseHeaderForFunctions(header, additionalPaths)
		for name, sig := range sigs {
			if _, exists := signatures[name]; !exists {
				signatures[name] = sig
			}
		}
	}

	return signatures
}

// parseHeaderForFunctions uses gcc -E to preprocess a header, then extracts function signatures
func parseHeaderForFunctions(headerName string, includePaths []string) map[string]*CFunctionSignature {
	// Build gcc command to preprocess header
	args := []string{"-E", "-dD", "-x", "c", "-"}
	for _, path := range includePaths {
		args = append(args, "-I"+path)
	}

	// Create a minimal C file that includes the header
	includeCode := fmt.Sprintf("#include <%s>\n", headerName)

	cmd := exec.Command("gcc", args...)
	cmd.Stdin = strings.NewReader(includeCode)
	output, err := cmd.Output()
	if err != nil {
		// Header not found or gcc failed
		return make(map[string]*CFunctionSignature)
	}

	// Parse the preprocessed output for function declarations
	return extractFunctionDeclarations(string(output))
}

// extractFunctionDeclarations parses preprocessed C code for function declarations
func extractFunctionDeclarations(preprocessed string) map[string]*CFunctionSignature {
	signatures := make(map[string]*CFunctionSignature)

	// Pattern: extern EVERYTHING_BEFORE_PAREN (PARAMS) ... ;
	funcDeclPattern := regexp.MustCompile(`\bextern\s+(.+?)\s*\(([^)]*)\)[^;]*;`)

	matches := funcDeclPattern.FindAllStringSubmatch(preprocessed, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		declPart := strings.TrimSpace(match[1])
		paramsStr := match[2]

		// Split declPart into return type and function name
		parts := strings.Fields(declPart)
		if len(parts) == 0 {
			continue
		}

		// Extract function name and return type
		var funcName, returnType string
		lastPart := parts[len(parts)-1]

		if strings.Contains(lastPart, "*") && !strings.HasPrefix(lastPart, "*") {
			// Pattern: type*funcname
			starIdx := strings.Index(lastPart, "*")
			returnType = strings.Join(parts[:len(parts)-1], " ") + " " + lastPart[:starIdx+1]
			funcName = lastPart[starIdx+1:]
		} else if strings.HasPrefix(lastPart, "*") {
			// Pattern: *funcname
			returnType = strings.Join(parts[:len(parts)-1], " ") + " *"
			funcName = strings.TrimLeft(lastPart, "*")
		} else {
			// Normal case: last word is function name
			funcName = lastPart
			if len(parts) > 1 {
				returnType = strings.Join(parts[:len(parts)-1], " ")
			} else {
				returnType = "void"
			}
		}

		returnType = strings.TrimSpace(returnType)
		funcName = strings.TrimSpace(funcName)

		// Skip internal/private functions (starting with __)
		if strings.HasPrefix(funcName, "__") && !strings.HasPrefix(funcName, "SDL_") {
			continue
		}

		// Parse parameters
		var params []CFunctionParam
		if strings.TrimSpace(paramsStr) != "" && paramsStr != "void" {
			paramParts := strings.Split(paramsStr, ",")
			for _, paramStr := range paramParts {
				paramStr = strings.TrimSpace(paramStr)
				if paramStr == "" {
					continue
				}

				// Extract type (everything before parameter name)
				// Handle cases: "double x", "const char *str", "int", "void*"
				parts := strings.Fields(paramStr)
				if len(parts) == 0 {
					continue
				}

				var paramType string
				// Check if last part looks like a parameter name (starts with __ or lowercase)
				lastPart := parts[len(parts)-1]
				if len(parts) > 1 && (strings.HasPrefix(lastPart, "__") || (len(lastPart) > 0 && lastPart[0] >= 'a' && lastPart[0] <= 'z' && !strings.Contains(lastPart, "*"))) {
					// Has parameter name - take all but last as type
					paramType = strings.Join(parts[:len(parts)-1], " ")
				} else {
					// No parameter name or last part is the type
					paramType = strings.Join(parts, " ")
				}

				params = append(params, CFunctionParam{Type: paramType})
			}
		}

		signatures[funcName] = &CFunctionSignature{
			ReturnType: returnType,
			Params:     params,
		}
	}

	return signatures
}

// ExtractFunctionSignatures extracts function signatures from DWARF debug info in a .so file
// Returns a map of function name -> CFunctionSignature
func ExtractFunctionSignatures(soPath string) (map[string]*CFunctionSignature, error) {
	// Open ELF file
	elfFile, err := elf.Open(soPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file: %v", err)
	}
	defer elfFile.Close()

	// Get DWARF debug info
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		// No DWARF info available - not an error, just return empty map
		return make(map[string]*CFunctionSignature), nil
	}

	signatures := make(map[string]*CFunctionSignature)
	reader := dwarfData.Reader()

	// Iterate through all DWARF entries
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF: %v", err)
		}
		if entry == nil {
			break
		}

		// Look for subprogram entries (functions)
		if entry.Tag == dwarf.TagSubprogram {
			name, sig := parseFunctionEntry(entry, reader, dwarfData)
			if name != "" && sig != nil {
				signatures[name] = sig
			}
		}
	}

	return signatures, nil
}

// parseFunctionEntry parses a DWARF subprogram entry to extract function signature
// Returns (name, signature)
func parseFunctionEntry(entry *dwarf.Entry, reader *dwarf.Reader, data *dwarf.Data) (string, *CFunctionSignature) {
	sig := &CFunctionSignature{}
	var funcName string

	// Get function name
	if nameAttr := entry.Val(dwarf.AttrName); nameAttr != nil {
		funcName = nameAttr.(string)
	}

	// Get return type
	if typeAttr := entry.Val(dwarf.AttrType); typeAttr != nil {
		if typeOffset, ok := typeAttr.(dwarf.Offset); ok {
			sig.ReturnType = resolveTypeName(typeOffset, data)
		}
	} else {
		sig.ReturnType = "void"
	}

	// Parse parameters - read all immediate children
	if !entry.Children {
		return funcName, sig
	}

	for {
		childEntry, err := reader.Next()
		if err != nil || childEntry == nil {
			break
		}

		// Tag == 0 means end of children at this level
		if childEntry.Tag == 0 {
			break
		}

		// Look for formal parameter entries
		if childEntry.Tag == dwarf.TagFormalParameter {
			param := CFunctionParam{}

			if nameAttr := childEntry.Val(dwarf.AttrName); nameAttr != nil {
				param.Name = nameAttr.(string)
			}

			if typeAttr := childEntry.Val(dwarf.AttrType); typeAttr != nil {
				if typeOffset, ok := typeAttr.(dwarf.Offset); ok {
					param.Type = resolveTypeName(typeOffset, data)
				}
			}

			sig.Params = append(sig.Params, param)
		}

		// Skip children of this entry (if any)
		if childEntry.Children {
			reader.SkipChildren()
		}
	}

	return funcName, sig
}

// resolveTypeName resolves a DWARF type offset to a simplified type name
func resolveTypeName(offset dwarf.Offset, data *dwarf.Data) string {
	reader := data.Reader()
	reader.Seek(offset)

	entry, err := reader.Next()
	if err != nil || entry == nil {
		return "unknown"
	}

	// Handle base types
	if entry.Tag == dwarf.TagBaseType {
		if nameAttr := entry.Val(dwarf.AttrName); nameAttr != nil {
			typeName := nameAttr.(string)

			// Map C types to simplified categories
			switch {
			case strings.Contains(typeName, "float"):
				return "float"
			case strings.Contains(typeName, "double"):
				return "double"
			case strings.Contains(typeName, "int") || strings.Contains(typeName, "long") ||
				strings.Contains(typeName, "short") || strings.Contains(typeName, "char"):
				return "int"
			default:
				return typeName
			}
		}
	}

	// Handle pointer types
	if entry.Tag == dwarf.TagPointerType {
		return "pointer"
	}

	// Handle typedef
	if entry.Tag == dwarf.TagTypedef {
		if typeAttr := entry.Val(dwarf.AttrType); typeAttr != nil {
			if typeOffset, ok := typeAttr.(dwarf.Offset); ok {
				return resolveTypeName(typeOffset, data)
			}
		}
	}

	// Handle const types
	if entry.Tag == dwarf.TagConstType {
		if typeAttr := entry.Val(dwarf.AttrType); typeAttr != nil {
			if typeOffset, ok := typeAttr.(dwarf.Offset); ok {
				return resolveTypeName(typeOffset, data)
			}
		}
	}

	return "unknown"
}









