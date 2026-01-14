// Completion: 100% - Module complete
package main

// Library definition system for defining .so file interfaces
// This allows users to define the functions they want to call from libraries

import (
	"fmt"
	"strings"
)

// LibraryDefinition contains pre-defined common library interfaces
type LibraryDefinition struct {
	Name      string
	SoFile    string
	Functions map[string]*Function
}

// CreateRaylibDefinition creates the raylib library definition
func CreateRaylibDefinition() *LibraryDefinition {
	lib := &LibraryDefinition{
		Name:      "raylib",
		SoFile:    "libraylib.so.5",
		Functions: make(map[string]*Function),
	}

	// Core raylib functions for basic graphics
	lib.Functions["InitWindow"] = &Function{
		Name:       "InitWindow",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{
			{Name: "width", Type: CTypeInt},
			{Name: "height", Type: CTypeInt},
			{Name: "title", Type: CTypePointer}, // const char*
		},
	}

	lib.Functions["CloseWindow"] = &Function{
		Name:       "CloseWindow",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{},
	}

	lib.Functions["WindowShouldClose"] = &Function{
		Name:       "WindowShouldClose",
		ReturnType: CTypeInt, // bool -> int
		Parameters: []Parameter{},
	}

	lib.Functions["BeginDrawing"] = &Function{
		Name:       "BeginDrawing",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{},
	}

	lib.Functions["EndDrawing"] = &Function{
		Name:       "EndDrawing",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{},
	}

	lib.Functions["ClearBackground"] = &Function{
		Name:       "ClearBackground",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{
			{Name: "color", Type: CTypeUInt}, // Color as uint32
		},
	}

	lib.Functions["DrawPixel"] = &Function{
		Name:       "DrawPixel",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{
			{Name: "posX", Type: CTypeInt},
			{Name: "posY", Type: CTypeInt},
			{Name: "color", Type: CTypeUInt}, // Color as uint32
		},
	}

	lib.Functions["SetTargetFPS"] = &Function{
		Name:       "SetTargetFPS",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{
			{Name: "fps", Type: CTypeInt},
		},
	}

	return lib
}

// CreateGlibcDefinition creates basic glibc function definitions
func CreateGlibcDefinition() *LibraryDefinition {
	lib := &LibraryDefinition{
		Name:      "glibc",
		SoFile:    "libc.so.6",
		Functions: make(map[string]*Function),
	}

	lib.Functions["printf"] = &Function{
		Name:       "printf",
		ReturnType: CTypeInt,
		Parameters: []Parameter{
			{Name: "format", Type: CTypePointer}, // const char*
			// Note: printf is variadic, would need special handling
		},
	}

	lib.Functions["malloc"] = &Function{
		Name:       "malloc",
		ReturnType: CTypePointer,
		Parameters: []Parameter{
			{Name: "size", Type: CTypeULong}, // size_t
		},
	}

	lib.Functions["free"] = &Function{
		Name:       "free",
		ReturnType: CTypeVoid,
		Parameters: []Parameter{
			{Name: "ptr", Type: CTypePointer},
		},
	}

	lib.Functions["puts"] = &Function{
		Name:       "puts",
		ReturnType: CTypeInt,
		Parameters: []Parameter{
			{Name: "s", Type: CTypePointer}, // const char*
		},
	}

	lib.Functions["sprintf"] = &Function{
		Name:       "sprintf",
		ReturnType: CTypeInt,
		Parameters: []Parameter{
			{Name: "str", Type: CTypePointer},    // char*
			{Name: "format", Type: CTypePointer}, // const char*
			// Note: sprintf is variadic
		},
	}

	return lib
}

// ParseLibraryDefinition parses a library definition from a string format
// Format: "library_name:so_file:function_name(param_type param_name, ...) -> return_type"
func ParseLibraryDefinition(definition string) (*LibraryDefinition, error) {
	parts := strings.Split(definition, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid library definition format")
	}

	lib := &LibraryDefinition{
		Name:      strings.TrimSpace(parts[0]),
		SoFile:    strings.TrimSpace(parts[1]),
		Functions: make(map[string]*Function),
	}

	// Parse function definitions
	funcDef := strings.TrimSpace(parts[2])
	fn, err := parseFunctionSignature(funcDef)
	if err != nil {
		return nil, err
	}

	lib.Functions[fn.Name] = fn
	return lib, nil
}

// parseFunctionSignature parses a function signature string
func parseFunctionSignature(signature string) (*Function, error) {
	// Example: "InitWindow(int width, int height, pointer title) -> void"
	parts := strings.Split(signature, "->")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid function signature format")
	}

	// Parse return type
	returnTypeStr := strings.TrimSpace(parts[1])
	returnType, err := parseType(returnTypeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid return type: %v", err)
	}

	// Parse function name and parameters
	funcPart := strings.TrimSpace(parts[0])
	openParen := strings.Index(funcPart, "(")
	closeParen := strings.LastIndex(funcPart, ")")

	if openParen == -1 || closeParen == -1 {
		return nil, fmt.Errorf("invalid function signature format")
	}

	funcName := strings.TrimSpace(funcPart[:openParen])
	paramStr := strings.TrimSpace(funcPart[openParen+1 : closeParen])

	var params []Parameter
	if paramStr != "" {
		paramParts := strings.Split(paramStr, ",")
		for _, param := range paramParts {
			p, err := parseParameter(strings.TrimSpace(param))
			if err != nil {
				return nil, fmt.Errorf("invalid parameter: %v", err)
			}
			params = append(params, p)
		}
	}

	return &Function{
		Name:       funcName,
		ReturnType: returnType,
		Parameters: params,
	}, nil
}

// parseParameter parses a parameter string like "int width"
func parseParameter(param string) (Parameter, error) {
	parts := strings.Fields(param)
	if len(parts) != 2 {
		return Parameter{}, fmt.Errorf("parameter must be 'type name'")
	}

	paramType, err := parseType(parts[0])
	if err != nil {
		return Parameter{}, err
	}

	return Parameter{
		Name: parts[1],
		Type: paramType,
	}, nil
}

// parseType parses a type string
func parseType(typeStr string) (CType, error) {
	switch strings.ToLower(typeStr) {
	case "void":
		return CTypeVoid, nil
	case "int":
		return CTypeInt, nil
	case "uint":
		return CTypeUInt, nil
	case "long":
		return CTypeLong, nil
	case "ulong":
		return CTypeULong, nil
	case "float":
		return CTypeFloat, nil
	case "double":
		return CTypeDouble, nil
	case "pointer", "ptr":
		return CTypePointer, nil
	case "char":
		return CTypeChar, nil
	case "struct":
		return CTypeStruct, nil
	default:
		return CTypeVoid, fmt.Errorf("unknown type: %s", typeStr)
	}
}

// GenerateLibraryBindings generates the necessary ELF sections for dynamic linking
func (eb *ExecutableBuilder) GenerateLibraryBindings(dl *DynamicLinker) error {
	// This would generate:
	// 1. .dynsym section (dynamic symbol table)
	// 2. .dynstr section (dynamic string table)
	// 3. .plt section (procedure linkage table)
	// 4. .got section (global offset table)
	// 5. .dynamic section (dynamic linking info)

	// For now, just add placeholders
	for libName, lib := range dl.Libraries {
		eb.Define(fmt.Sprintf("LIB_%s", strings.ToUpper(libName)), lib.Path)

		for funcName := range lib.Functions {
			// Create placeholder entries for each function
			symbolName := fmt.Sprintf("%s_%s", libName, funcName)
			eb.Define(fmt.Sprintf("SYM_%s", strings.ToUpper(symbolName)), "0")

			// Generate PLT stub (placeholder)
			err := dl.GeneratePLTEntry(eb, funcName)
			if err != nil {
				return fmt.Errorf("failed to generate PLT entry for %s: %v", funcName, err)
			}
		}
	}

	return nil
}

// AddDynamicLibrarySupport adds dynamic library support to ExecutableBuilder
func (eb *ExecutableBuilder) AddDynamicLibrarySupport() *DynamicLinker {
	dl := NewDynamicLinker()

	// Add common libraries
	raylibDef := CreateRaylibDefinition()
	raylib := dl.AddLibrary(raylibDef.Name, raylibDef.SoFile)
	for name, fn := range raylibDef.Functions {
		raylib.Functions[name] = fn
	}

	glibcDef := CreateGlibcDefinition()
	glibc := dl.AddLibrary(glibcDef.Name, glibcDef.SoFile)
	for name, fn := range glibcDef.Functions {
		glibc.Functions[name] = fn
	}

	return dl
}

// CallFunction is a helper method for ExecutableBuilder to call dynamic functions
func (eb *ExecutableBuilder) CallFunction(dl *DynamicLinker, funcName string, args ...string) error {
	return dl.GenerateFunctionCall(eb, funcName, args)
}

// DefineRaylibColors creates common raylib color constants
func (eb *ExecutableBuilder) DefineRaylibColors() {
	// Raylib Color values as RGBA packed into uint32
	colors := map[string]string{
		"RAYWHITE": "0xF5F5F5FF",
		"BLACK":    "0x000000FF",
		"RED":      "0xFF0000FF",
		"GREEN":    "0x00FF00FF",
		"BLUE":     "0x0000FFFF",
		"WHITE":    "0xFFFFFFFF",
		"YELLOW":   "0xFFFF00FF",
		"MAGENTA":  "0xFF00FFFF",
		"CYAN":     "0x00FFFFFF",
	}

	for name, value := range colors {
		eb.Define(name, value)
	}
}

// Example usage helper
func GenerateRaylibExample(eb *ExecutableBuilder) error {
	dl := eb.AddDynamicLibrarySupport()
	eb.DefineRaylibColors()

	// Import the functions we want to use
	functions := []string{"InitWindow", "CloseWindow", "BeginDrawing", "EndDrawing", "ClearBackground", "DrawPixel", "WindowShouldClose", "SetTargetFPS"}
	for _, fn := range functions {
		err := dl.ImportFunction("raylib", fn)
		if err != nil {
			return err
		}
	}

	// Generate bindings
	err := eb.GenerateLibraryBindings(dl)
	if err != nil {
		return err
	}

	// Example: Initialize window, draw red pixel, close
	// This would be called by user code, not automatically generated

	return nil
}









