// Completion: 100% - Module complete
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CFFIManager manages C foreign function interface integration
// It combines header file parsing with DLL/shared library introspection
type CFFIManager struct {
	// Parsed data from C headers
	headerConstants *CHeaderConstants

	// Exported functions from DLL/SO files
	dllExports map[string][]ExportedFunction // library name -> exported functions

	// Combined function signatures (from headers + DLL validation)
	functions map[string]*CFunction

	// Type mappings from C to Vibe67
	typeMappings map[string]string
}

// CFunction represents a complete C function with signature and export info
type CFunction struct {
	Name       string
	ReturnType string
	Params     []CFunctionParam
	Library    string // Which DLL/SO exports this function
	Ordinal    uint16 // For Windows PE files
	RVA        uint32 // Relative Virtual Address (Windows)
}

// NewCFFIManager creates a new C FFI manager
func NewCFFIManager() *CFFIManager {
	return &CFFIManager{
		headerConstants: NewCHeaderConstants(),
		dllExports:      make(map[string][]ExportedFunction),
		functions:       make(map[string]*CFunction),
		typeMappings:    getDefaultTypeMappings(),
	}
}

// getDefaultTypeMappings returns the default C-to-Vibe67 type mappings
func getDefaultTypeMappings() map[string]string {
	return map[string]string{
		// Integer types
		"int":           "cint",
		"unsigned int":  "cuint",
		"unsigned":      "cuint",
		"long":          "cint64",
		"unsigned long": "cuint64",
		"int32_t":       "cint",
		"uint32_t":      "cuint",
		"int64_t":       "cint64",
		"uint64_t":      "cuint64",
		"int8_t":        "cint8",
		"uint8_t":       "cuint8",
		"int16_t":       "cint16",
		"uint16_t":      "cuint16",
		"size_t":        "cuint64",
		"ssize_t":       "cint64",

		// Floating point types
		"float":  "cfloat",
		"double": "cdouble",

		// Character and string types
		"char":        "cint8",
		"char*":       "cstring",
		"const char*": "cstring",

		// Pointer types
		"void*":       "cptr",
		"const void*": "cptr",

		// Boolean (if C99/C++ bool is used)
		"bool":  "cint8",
		"_Bool": "cint8",

		// Void
		"void": "void",
	}
}

// ParseHeader parses a C header file and extracts constants and function signatures
func (cm *CFFIManager) ParseHeader(filepath string) error {
	parser := NewCParser()
	constants, err := parser.ParseFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to parse header %s: %v", filepath, err)
	}

	// Merge constants
	for name, value := range constants.Constants {
		cm.headerConstants.Constants[name] = value
	}

	// Merge macros
	for name, value := range constants.Macros {
		cm.headerConstants.Macros[name] = value
	}

	// Merge and upgrade function signatures
	for name, sig := range constants.Functions {
		cm.headerConstants.Functions[name] = sig
		cm.functions[name] = &CFunction{
			Name:       name,
			ReturnType: sig.ReturnType,
			Params:     sig.Params,
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Parsed header %s: %d constants, %d functions\n",
			filepath, len(constants.Constants), len(constants.Functions))
	}

	return nil
}

// ParseDLLExports parses a DLL/PE file and extracts exported function names
func (cm *CFFIManager) ParseDLLExports(libName, filepath string) error {
	exports, err := ParseDLL(filepath)
	if err != nil {
		return fmt.Errorf("failed to parse DLL %s: %v", filepath, err)
	}

	cm.dllExports[libName] = exports

	// Match exports with known function signatures from headers
	for _, export := range exports {
		if sig, ok := cm.headerConstants.Functions[export.Name]; ok {
			// We have both header signature and DLL export
			cm.functions[export.Name] = &CFunction{
				Name:       export.Name,
				ReturnType: sig.ReturnType,
				Params:     sig.Params,
				Library:    libName,
				Ordinal:    export.Ordinal,
				RVA:        export.RVA,
			}
		} else {
			// DLL export without header signature - create minimal entry
			if _, exists := cm.functions[export.Name]; !exists {
				cm.functions[export.Name] = &CFunction{
					Name:       export.Name,
					ReturnType: "void", // Unknown, assume void
					Params:     nil,    // Unknown parameters
					Library:    libName,
					Ordinal:    export.Ordinal,
					RVA:        export.RVA,
				}
			}
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Parsed DLL %s: %d exports, %d matched with headers\n",
			filepath, len(exports), countMatched(exports, cm.headerConstants.Functions))
	}

	return nil
}

// countMatched counts how many exports have matching header signatures
func countMatched(exports []ExportedFunction, headers map[string]*CFunctionSignature) int {
	count := 0
	for _, exp := range exports {
		if _, ok := headers[exp.Name]; ok {
			count++
		}
	}
	return count
}

// GetFunction returns the function signature for the given name
func (cm *CFFIManager) GetFunction(name string) (*CFunction, bool) {
	fn, ok := cm.functions[name]
	return fn, ok
}

// GetConstant returns the constant value for the given name
func (cm *CFFIManager) GetConstant(name string) (int64, bool) {
	val, ok := cm.headerConstants.Constants[name]
	return val, ok
}

// MapCTypeToVibe67 maps a C type to a Vibe67 type
func (cm *CFFIManager) MapCTypeToVibe67(cType string) string {
	// Normalize the type
	cType = strings.TrimSpace(cType)

	// Direct mapping
	if vibe67Type, ok := cm.typeMappings[cType]; ok {
		return vibe67Type
	}

	// Pointer types
	if strings.HasSuffix(cType, "*") {
		baseType := strings.TrimSpace(strings.TrimSuffix(cType, "*"))
		if strings.HasPrefix(baseType, "const ") {
			baseType = strings.TrimSpace(strings.TrimPrefix(baseType, "const"))
		}

		// Special case: char* is cstring
		if baseType == "char" {
			return "cstring"
		}

		// Everything else is a generic pointer
		return "cptr"
	}

	// Handle SDL types and other library-specific types
	if strings.HasPrefix(cType, "SDL_") || strings.HasPrefix(cType, "Uint") || strings.HasPrefix(cType, "Sint") {
		// SDL types are typically typedef'd integers or pointers
		if strings.HasSuffix(cType, "*") {
			return "cptr"
		}
		// SDL_Bool, Uint32, etc.
		if strings.Contains(strings.ToLower(cType), "32") {
			if strings.Contains(strings.ToLower(cType), "int") {
				return "cuint"
			}
		}
		if strings.Contains(strings.ToLower(cType), "64") {
			if strings.Contains(strings.ToLower(cType), "int") {
				return "cuint64"
			}
		}
		// Default for unknown SDL types
		return "cint"
	}

	// Unknown type - default to cptr for safety
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Warning: unknown C type '%s', defaulting to cptr\n", cType)
	}
	return "cptr"
}

// GenerateVibe67Binding generates a Vibe67 function binding for a C function
func (cm *CFFIManager) GenerateVibe67Binding(funcName string) (string, error) {
	fn, ok := cm.functions[funcName]
	if !ok {
		return "", fmt.Errorf("function %s not found", funcName)
	}

	// Generate Vibe67 function signature
	var paramTypes []string
	var paramNames []string
	for i, param := range fn.Params {
		vibe67Type := cm.MapCTypeToVibe67(param.Type)
		paramTypes = append(paramTypes, vibe67Type)
		if param.Name != "" {
			paramNames = append(paramNames, param.Name)
		} else {
			paramNames = append(paramNames, fmt.Sprintf("arg%d", i))
		}
	}

	returnType := cm.MapCTypeToVibe67(fn.ReturnType)

	// Format: funcName = (param1: type1, param2: type2) -> returnType { cffi("library", "funcName") }
	binding := fmt.Sprintf("%s = (%s) %s {\n    cffi(\"%s\", \"%s\")\n}",
		funcName,
		formatParams(paramNames, paramTypes),
		formatReturnType(returnType),
		fn.Library,
		funcName)

	return binding, nil
}

// formatParams formats parameter list as "name1: type1, name2: type2"
func formatParams(names, types []string) string {
	if len(names) == 0 {
		return ""
	}
	var parts []string
	for i := range names {
		parts = append(parts, fmt.Sprintf("%s: %s", names[i], types[i]))
	}
	return strings.Join(parts, ", ")
}

// formatReturnType formats return type as "-> type" or empty for void
func formatReturnType(returnType string) string {
	if returnType == "void" {
		return ""
	}
	return "-> " + returnType
}

// LoadLibrary loads a library by parsing both its header and DLL/SO file
func (cm *CFFIManager) LoadLibrary(libName string, headerPath string, dllPath string) error {
	// Parse header if provided
	if headerPath != "" {
		if err := cm.ParseHeader(headerPath); err != nil {
			return fmt.Errorf("failed to parse header for %s: %v", libName, err)
		}
	}

	// Parse DLL if provided and it exists
	if dllPath != "" {
		if _, err := os.Stat(dllPath); err == nil {
			if err := cm.ParseDLLExports(libName, dllPath); err != nil {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: failed to parse DLL for %s: %v\n", libName, err)
				}
				// Continue without DLL info
			}
		} else if VerboseMode {
			fmt.Fprintf(os.Stderr, "Warning: DLL file not found: %s\n", dllPath)
		}
	}

	return nil
}

// AutoLoadLibrary attempts to automatically load a library by searching common locations
func (cm *CFFIManager) AutoLoadLibrary(libName string) error {
	// Try to find header
	headerPaths := []string{
		fmt.Sprintf("%s.h", libName),
		filepath.Join("./include", fmt.Sprintf("%s.h", libName)),
		filepath.Join("/usr/include", fmt.Sprintf("%s.h", libName)),
		filepath.Join("/usr/local/include", fmt.Sprintf("%s.h", libName)),
	}

	// Try uppercase library name patterns (e.g., SDL3/SDL.h)
	upperLib := strings.ToUpper(libName)
	headerPaths = append(headerPaths,
		filepath.Join(upperLib, fmt.Sprintf("%s.h", upperLib)),
		filepath.Join(".", "include", upperLib, fmt.Sprintf("%s.h", upperLib)),
		filepath.Join("/usr/include", upperLib, fmt.Sprintf("%s.h", upperLib)),
		filepath.Join("/usr/local/include", upperLib, fmt.Sprintf("%s.h", upperLib)),
	)

	var headerPath string
	for _, path := range headerPaths {
		if _, err := os.Stat(path); err == nil {
			headerPath = path
			break
		}
	}

	// Try to find DLL/SO
	dllPaths := []string{
		fmt.Sprintf("%s.dll", libName),
		fmt.Sprintf("lib%s.so", libName),
		fmt.Sprintf("/usr/lib/lib%s.so", libName),
		fmt.Sprintf("/usr/local/lib/lib%s.so", libName),
	}

	var dllPath string
	for _, path := range dllPaths {
		if _, err := os.Stat(path); err == nil {
			dllPath = path
			break
		}
	}

	if headerPath == "" && dllPath == "" {
		return fmt.Errorf("could not find header or library file for %s", libName)
	}

	return cm.LoadLibrary(libName, headerPath, dllPath)
}

// GetAllFunctions returns all known functions
func (cm *CFFIManager) GetAllFunctions() map[string]*CFunction {
	return cm.functions
}

// GetLibraryFunctions returns all functions exported by a specific library
func (cm *CFFIManager) GetLibraryFunctions(libName string) []*CFunction {
	var funcs []*CFunction
	for _, fn := range cm.functions {
		if fn.Library == libName {
			funcs = append(funcs, fn)
		}
	}
	return funcs
}

// ValidateFunction checks if a function exists and has a complete signature
func (cm *CFFIManager) ValidateFunction(name string) error {
	fn, ok := cm.functions[name]
	if !ok {
		return fmt.Errorf("function %s not found", name)
	}

	if fn.Library == "" {
		return fmt.Errorf("function %s has no associated library", name)
	}

	if fn.ReturnType == "" {
		return fmt.Errorf("function %s has no return type", name)
	}

	return nil
}









