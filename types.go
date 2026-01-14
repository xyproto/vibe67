// Completion: 100% - Type system complete with C FFI integration
package main

// Vibe67Type represents a type in the Vibe67 type system
type Vibe67Type struct {
	Kind     TypeKind    // The category of type
	CType    string      // For Foreign types, the C type string (e.g., "char*", "SDL_Window*")
	ElemType *Vibe67Type // For container types, the element type
}

// TypeKind represents the category of a type
type TypeKind int

const (
	TypeUnknown  TypeKind = iota
	TypeNumber            // Vibe67's native float64 type
	TypeString            // Vibe67's native string (map-based)
	TypeList              // Vibe67's native list (map-based)
	TypeMap               // Vibe67's native map
	TypeCString           // C char* (null-terminated string)
	TypeCInt              // C int, int32_t, etc.
	TypeCLong             // C long, int64_t
	TypeCFloat            // C float
	TypeCDouble           // C double
	TypeCBool             // C bool, _Bool
	TypeCPointer          // Generic C pointer (void*, SDL_Window*, etc.)
	TypeCVoid             // C void (for return types)
)

// String returns a human-readable representation of the type
func (t *Vibe67Type) String() string {
	switch t.Kind {
	case TypeUnknown:
		return "unknown"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeList:
		if t.ElemType != nil {
			return "list[" + t.ElemType.String() + "]"
		}
		return "list"
	case TypeMap:
		return "map"
	case TypeCString:
		return "cstring"
	case TypeCInt:
		return "cint"
	case TypeCLong:
		return "clong"
	case TypeCFloat:
		return "cfloat"
	case TypeCDouble:
		return "cdouble"
	case TypeCBool:
		return "cbool"
	case TypeCPointer:
		return "cpointer:" + t.CType
	case TypeCVoid:
		return "void"
	default:
		return "unknown"
	}
}

// IsNative returns true if this is a native Vibe67 type
func (t *Vibe67Type) IsNative() bool {
	switch t.Kind {
	case TypeNumber, TypeString, TypeList, TypeMap:
		return true
	default:
		return false
	}
}

// IsForeign returns true if this is a C foreign type
func (t *Vibe67Type) IsForeign() bool {
	return !t.IsNative() && t.Kind != TypeUnknown
}

// IsPointer returns true if this represents a pointer type
func (t *Vibe67Type) IsPointer() bool {
	return t.Kind == TypeCString || t.Kind == TypeCPointer
}

// NeedsConversionToC returns true if this type needs conversion when passing to C
func (t *Vibe67Type) NeedsConversionToC() bool {
	// Vibe67 strings need conversion to C strings
	return t.Kind == TypeString
}

// NeedsConversionFromC returns true if this type needs conversion when receiving from C
func (t *Vibe67Type) NeedsConversionFromC() bool {
	// Currently no conversions needed from C to Vibe67
	// (C strings stay as cstrings until explicitly converted)
	return false
}

// ParseCType converts a C type string to a Vibe67Type
func ParseCType(ctype string) *Vibe67Type {
	// Remove const, volatile, etc.
	ctype = removeTypeQualifiers(ctype)

	// Check for pointer types
	if len(ctype) > 0 && ctype[len(ctype)-1] == '*' {
		baseType := ctype[:len(ctype)-1]
		baseType = removeTypeQualifiers(baseType)

		if baseType == "char" {
			return &Vibe67Type{Kind: TypeCString, CType: ctype}
		}
		return &Vibe67Type{Kind: TypeCPointer, CType: ctype}
	}

	// Check for basic types
	switch ctype {
	case "void":
		return &Vibe67Type{Kind: TypeCVoid}
	case "int", "int32_t", "unsigned", "unsigned int", "uint32_t":
		return &Vibe67Type{Kind: TypeCInt, CType: ctype}
	case "long", "int64_t", "uint64_t":
		return &Vibe67Type{Kind: TypeCLong, CType: ctype}
	case "float":
		return &Vibe67Type{Kind: TypeCFloat, CType: ctype}
	case "double":
		return &Vibe67Type{Kind: TypeCDouble, CType: ctype}
	case "bool", "_Bool":
		return &Vibe67Type{Kind: TypeCBool, CType: ctype}
	default:
		// Unknown C type - treat as pointer
		return &Vibe67Type{Kind: TypeCPointer, CType: ctype}
	}
}

// removeTypeQualifiers strips const, volatile, etc. from a type string
func removeTypeQualifiers(ctype string) string {
	// Simple implementation - just trim spaces
	// Could be more sophisticated if needed
	result := ""
	words := splitTypeWords(ctype)
	for _, word := range words {
		if word != "const" && word != "volatile" && word != "restrict" {
			if result != "" {
				result += " "
			}
			result += word
		}
	}
	return result
}

// splitTypeWords splits a C type into words
func splitTypeWords(s string) []string {
	var words []string
	var current string
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

// Native type constructors
var (
	TypeNumberValue  = &Vibe67Type{Kind: TypeNumber}
	TypeStringValue  = &Vibe67Type{Kind: TypeString}
	TypeListValue    = &Vibe67Type{Kind: TypeList}
	TypeMapValue     = &Vibe67Type{Kind: TypeMap}
	TypeCStringValue = &Vibe67Type{Kind: TypeCString, CType: "char*"}
	TypeUnknownValue = &Vibe67Type{Kind: TypeUnknown}
)









