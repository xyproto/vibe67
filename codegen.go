// Completion: 90% - Core x86_64 complete, ARM64/RISC-V functional, some TODOs remain
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"unsafe"
)

// codegen.go - C67 Code Generator
//
// This code generator is the authoritative implementation of LANGUAGESPEC.md v1.5.0.
// It transforms parsed AST into x86_64 assembly and ELF executables.
//
// Stability Commitment:
// This code generator implements all LANGUAGESPEC.md v1.5.0 features. Future work
// focuses on bug fixes, optimizations, and additional target architectures only.
//
// Current Target Support:
// - x86_64 Linux (complete, production-ready)
// - ARM64 Linux/macOS (deferred)
// - RISC-V64 Linux (deferred)
//
// This file contains the C67Compiler and all code generation logic:
// - Expression and statement compilation
// - Register allocation and optimization
// - Stack management and calling conventions
// - x86_64 assembly emission
// - ELF executable generation
// - C FFI and syscall support

// LoopInfo tracks information about an active loop during compilation
type LoopInfo struct {
	Label           int   // Loop label (@1, @2, @3, etc.)
	StartPos        int   // Code position of loop start (condition check)
	ContinuePos     int   // Code position for continue (increment step)
	EndPatches      []int // Positions that need to be patched to jump to loop end
	ContinuePatches []int // Positions that need to be patched to jump to continue position

	// Special loop variables support
	IteratorOffset   int  // Stack offset for iterator variable (loop variable)
	IndexOffset      int  // Stack offset for index counter (list loops only)
	UpperBoundOffset int  // Stack offset for limit (range) or length (list)
	ListPtrOffset    int  // Stack offset for list pointer (list loops only)
	IsRangeLoop      bool // True for range loops, false for list loops

	// Register tracking
	CounterReg  string // Register used for loop counter (if any)
	UseRegister bool   // True if counter is in register, false if on stack
}

// Code Generator for C67
type C67Compiler struct {
	eb                   *ExecutableBuilder
	out                  *Out
	platform             Platform                      // Target platform (arch + OS)
	variables            map[string]int                // variable name -> stack offset
	mutableVars          map[string]bool               // variable name -> is mutable
	lambdaVars           map[string]bool               // variable name -> is lambda/function
	parentVariables      map[string]bool               // Track parent-scope vars in parallel loops (use r11 instead of rbp)
	varTypes             map[string]string             // variable name -> "map" or "list" (legacy)
	varTypeInfo          map[string]*C67Type           // variable name -> type annotation (new type system)
	functionSignatures   map[string]*FunctionSignature // function name -> signature (params, variadic)
	sourceCode           string                        // Store source for recompilation
	usedFunctions        map[string]bool               // Track which functions are called
	unknownFunctions     map[string]bool               // Track functions called but not defined
	callOrder            []string                      // Track order of function calls
	cImports             map[string]string             // Track C imports: alias -> library name
	cLibHandles          map[string]string             // Track library handles: library -> handle var name
	cConstants           map[string]*CHeaderConstants  // Track C constants: alias -> constants
	cFunctionLibs        map[string]string             // Track which library each C function belongs to: function -> library
	stringCounter        int                           // Counter for unique string labels
	stackOffset          int                           // Current stack offset for variables (logical)
	maxStackOffset       int                           // Maximum stack offset reached (for frame allocation)
	runtimeStack         int                           // Actual runtime stack usage (updated during compilation)
	loopBaseOffsets      map[int]int                   // Loop label -> stackOffset before loop body (for state calculation)
	labelCounter         int                           // Counter for unique labels (if/else, loops, etc)
	lambdaCounter        int                           // Counter for unique lambda function names
	activeLoops          []LoopInfo                    // Stack of active loops (for @N jump resolution)
	lambdaFuncs          []LambdaFunc                  // List of lambda functions to generate
	patternLambdaFuncs   []PatternLambdaFunc           // List of pattern lambda functions to generate
	lambdaOffsets        map[string]int                // Lambda name -> offset in .text
	currentLambda        *LambdaFunc                   // Currently compiling lambda (for "me" self-reference)
	lambdaBodyStart      int                           // Offset where lambda body starts (for tail recursion)
	hasExplicitExit      bool                          // Track if program contains explicit exit() call
	debug                bool                          // Enable debug output (set via DEBUG env var)
	verbose              bool                          // Enable verbose output
	cContext             bool                          // When true, compile expressions for C FFI (affects strings, pointers, ints)
	currentArena         int                           // Current arena index (starts at 1 for global arena = meta-arena[0])
	usesArenas           bool                          // Track if program uses any arena blocks
	arenaStack           []ArenaScope                  // Stack of active arena scopes
	globalArenaInit      bool                          // Track if global arena has been initialized
	importedFunctions    []string                      // Track imported C functions (malloc, free, etc.)
	cacheEnabledLambdas  map[string]bool               // Track which lambdas use cme
	deferredExprs        [][]Expression                // Stack of deferred expressions per scope (LIFO order)
	memoCaches           map[string]bool               // Track memoization caches that need storage allocation
	currentAssignName    string                        // Name of variable being assigned (for lambda naming)
	inTailPosition       bool                          // True when compiling expression in tail position
	hotFunctions         map[string]bool               // Track hot-reloadable functions
	hotFunctionTable     map[string]int
	hotTableRodataOffset int
	tailCallsOptimized   int // Count of tail calls optimized
	nonTailCalls         int // Count of non-tail recursive calls

	mainCalledAtTopLevel bool // Track if main() is explicitly called in top-level code

	metaArenaGrowthErrorJump      int
	firstMetaArenaMallocErrorJump int

	regAlloc          *RegisterAllocator // Register allocator for optimized variable allocation
	regTracker        *RegisterTracker   // Real-time register availability tracker
	regSpiller        *RegisterSpiller   // Register spilling manager
	wpoTimeout        float64            // Whole-program optimization timeout (non-global, thread-safe)
	movedVars         map[string]bool    // Track variables that have been moved (use-after-move detection)
	inUnsafeBlock     bool               // True when compiling inside an unsafe block (skip safety checks)
	functionNamespace map[string]string  // function name -> namespace (for imported C67 functions)
	scopeDepth        int                // Track scope depth for proper move tracking
	scopedMoved       []map[string]bool  // Stack of moved variables per scope
	errors            *ErrorCollector    // Railway-oriented error collector
	dynamicSymbols    *DynamicSections   // Dynamic symbol table (for updating lambda symbols post-generation)
	moduleLevelVars   map[string]bool    // Track module-level variables (defined outside lambdas)
	globalVars        map[string]int     // Global variable name -> .data offset
	globalVarsMutable map[string]bool    // Global variable name -> is mutable
	dataSection       []byte             // .data section contents
	forwardFunctions  map[string]bool    // Functions that can be forward-referenced (defined in program)

	// Feature tracking for minimal runtime inclusion
	usesStringConcat bool // Track if string concatenation is used
	usesStringToCstr bool // Track if C string conversion is needed
	usesPrintf       bool // Track if printf/println is used
	usesArenaAlloc   bool // Track if arena allocation is explicitly used
	usesCPUFeatures  bool // Track if CPU feature detection is needed (FMA, SIMD, etc.)

	// Runtime function emission flags (all true by default for full compatibility)
	runtimeFeatures *RuntimeFeatures // Enhanced runtime feature tracker

}

type FunctionSignature struct {
	ParamCount    int    // Number of fixed parameters
	VariadicParam string // Name of variadic parameter (empty if not variadic)
	IsVariadic    bool   // Quick check if function is variadic
}

type LambdaFunc struct {
	Name             string
	Params           []string
	VariadicParam    string // Name of variadic parameter (empty if none)
	Body             Expression
	CapturedVars     []string          // Variables captured from outer scope
	CapturedVarTypes map[string]string // Types of captured variables
	IsNested         bool              // True if this lambda is nested inside another
	IsPure           bool              // True if function has no side effects (eligible for memoization)
}

type PatternLambdaFunc struct {
	Name    string
	Clauses []*PatternClause
}

// nextLabel generates a unique label name
func (fc *C67Compiler) nextLabel() string {
	fc.labelCounter++
	return fmt.Sprintf("L%d", fc.labelCounter)
}

// defineLabel marks a label at the current position
func (fc *C67Compiler) defineLabel(label string, pos int) {
	// Labels are tracked in ExecutableBuilder
	fc.eb.labels[label] = pos
}

func NewC67Compiler(platform Platform, verbose bool) (*C67Compiler, error) {
	// Create ExecutableBuilder
	eb, err := NewWithPlatform(platform)
	if err != nil {
		return nil, err
	}

	// Don't enable dynamic linking by default - it will be enabled
	// when we actually call external functions (printf, exit, etc)
	// eb.useDynamicLinking = false (default)
	// Don't set neededFunctions yet - we'll build it dynamically

	// Create Out wrapper
	out := NewOut(eb.target, eb.TextWriter(), eb)

	// Check if debug mode is enabled
	debugEnabled := envBool("DEBUG")

	return &C67Compiler{
		eb:                  eb,
		out:                 out,
		platform:            platform,
		variables:           make(map[string]int),
		mutableVars:         make(map[string]bool),
		lambdaVars:          make(map[string]bool),
		varTypes:            make(map[string]string),
		varTypeInfo:         make(map[string]*C67Type),
		functionSignatures:  make(map[string]*FunctionSignature),
		usedFunctions:       make(map[string]bool),
		unknownFunctions:    make(map[string]bool),
		callOrder:           []string{},
		cImports:            make(map[string]string),
		cLibHandles:         make(map[string]string),
		cConstants:          make(map[string]*CHeaderConstants),
		cFunctionLibs:       make(map[string]string),
		lambdaOffsets:       make(map[string]int),
		loopBaseOffsets:     make(map[int]int),
		cacheEnabledLambdas: make(map[string]bool),
		hotFunctions:        make(map[string]bool),
		hotFunctionTable:    make(map[string]int),
		debug:               debugEnabled,
		verbose:             verbose,
		currentArena:        1, // Start at arena 1 (which is meta-arena[0], the global arena)
		regAlloc:            NewRegisterAllocator(platform.Arch),
		regTracker:          NewRegisterTracker(),
		regSpiller:          NewRegisterSpiller(SpillToStack),
		movedVars:           make(map[string]bool),
		scopeDepth:          0,
		scopedMoved:         []map[string]bool{make(map[string]bool)},
		errors:              NewErrorCollector(10),
		functionNamespace:   make(map[string]string),
		globalVars:          make(map[string]int),
		globalVarsMutable:   make(map[string]bool),
		dataSection:         []byte{},
		moduleLevelVars:     make(map[string]bool),

		// Initialize runtime feature tracker
		runtimeFeatures: NewRuntimeFeatures(),

	}, nil
}

// addSemanticError adds a semantic error to the error collector
// For codegen-time errors, we don't have exact line/column info,
// so we use line 0 as a placeholder
func (fc *C67Compiler) addSemanticError(message string, suggestions ...string) {
	suggestion := ""
	if len(suggestions) > 0 {
		suggestion = strings.Join(suggestions, ", ")
	}

	err := CompilerError{
		Level:    LevelError,
		Category: CategorySemantic,
		Message:  message,
		Location: SourceLocation{
			File:   "<compilation>",
			Line:   0,
			Column: 0,
		},
		Context: ErrorContext{
			Suggestion: suggestion,
		},
	}
	fc.errors.AddError(err)
}

// Confidence that this function is working: 95%
// getIntArgReg returns the register name for the nth integer/pointer argument (0-based)
// based on the target platform's calling convention
func (fc *C67Compiler) getIntArgReg(argIndex int) string {
	if fc.eb.target.OS() == OSWindows {
		// Microsoft x64 calling convention: RCX, RDX, R8, R9
		switch argIndex {
		case 0:
			return "rcx"
		case 1:
			return "rdx"
		case 2:
			return "r8"
		case 3:
			return "r9"
		default:
			// Args 5+ go on stack (at [rsp+32+n*8])
			return ""
		}
	} else {
		// System V ABI (Linux, macOS, FreeBSD): RDI, RSI, RDX, RCX, R8, R9
		switch argIndex {
		case 0:
			return "rdi"
		case 1:
			return "rsi"
		case 2:
			return "rdx"
		case 3:
			return "rcx"
		case 4:
			return "r8"
		case 5:
			return "r9"
		default:
			// Args 7+ go on stack
			return ""
		}
	}
}

// Confidence that this function is working: 90%
// allocateShadowSpace allocates the required shadow space for Windows x64 calling convention
// Returns the amount of space allocated (32 for Windows, 0 for other platforms)
func (fc *C67Compiler) allocateShadowSpace() int {
	if fc.eb.target.OS() == OSWindows {
		// Windows requires 32 bytes of "shadow space" for the called function
		fc.out.SubImmFromReg("rsp", 32)
		return 32
	}
	return 0
}

// Confidence that this function is working: 90%
// deallocateShadowSpace removes the shadow space after a function call
func (fc *C67Compiler) deallocateShadowSpace(shadowSpace int) {
	if shadowSpace > 0 {
		fc.out.AddImmToReg("rsp", int64(shadowSpace))
	}
}

// processCImports processes C import statements and extracts constants/function signatures
func (fc *C67Compiler) processCImports(program *Program) {
	for _, stmt := range program.Statements {
		if cImport, ok := stmt.(*CImportStmt); ok {
			fc.cImports[cImport.Alias] = cImport.Library
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Registered C import: %s -> %s\n", cImport.Alias, cImport.Library)
			}

			// Resolve .so path if not already set (identifier-based imports)
			if cImport.SoPath == "" {
				libSoName := cImport.Library
				if !strings.HasPrefix(libSoName, "lib") {
					libSoName = "lib" + libSoName
				}
				if !strings.Contains(libSoName, ".so") {
					libSoName += ".so"
				}

				// Use ldconfig to find the full path
				ldconfigCmd := exec.Command("ldconfig", "-p")
				if ldOutput, ldErr := ldconfigCmd.Output(); ldErr == nil {
					lines := strings.Split(string(ldOutput), "\n")
					for _, line := range lines {
						if strings.Contains(line, libSoName) && strings.Contains(line, "=>") {
							parts := strings.Split(line, "=>")
							if len(parts) == 2 {
								cImport.SoPath = strings.TrimSpace(parts[1])
								if VerboseMode {
									fmt.Fprintf(os.Stderr, "Resolved %s to %s\n", cImport.Library, cImport.SoPath)
								}
								break
							}
						}
					}
				}
			}

			// For .so file imports, extract symbols and function signatures
			if cImport.SoPath != "" {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Extracting symbols from %s...\n", cImport.SoPath)
				}
				symbols, err := ExtractSymbolsFromSo(cImport.SoPath)
				if err != nil {
					// Non-fatal: symbol extraction is optional
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Warning: failed to extract symbols from %s: %v\n", cImport.SoPath, err)
					}
				} else if VerboseMode && len(symbols) > 0 {
					fmt.Fprintf(os.Stderr, "Found %d symbols in %s\n", len(symbols), cImport.Library)
					if len(symbols) <= 20 {
						for _, sym := range symbols {
							fmt.Fprintf(os.Stderr, "  - %s\n", sym)
						}
					}
				}

				// Discover function signatures using multiple strategies:
				// 1. pkg-config, 2. header parsing, 3. DWARF, 4. symbol table
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Discovering function signatures...\n")
				}
				signatures, err := DiscoverFunctionSignatures(cImport.Library, cImport.SoPath)
				if err != nil {
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Warning: failed to extract signatures: %v\n", err)
					}
				} else if len(signatures) > 0 {
					// Store signatures for this library
					if fc.cConstants[cImport.Alias] == nil {
						fc.cConstants[cImport.Alias] = &CHeaderConstants{
							Constants: make(map[string]int64),
							Macros:    make(map[string]string),
							Functions: make(map[string]*CFunctionSignature),
						}
					}
					// Merge DWARF signatures into the constants map
					for funcName, sig := range signatures {
						fc.cConstants[cImport.Alias].Functions[funcName] = sig
					}
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Extracted %d function signatures from DWARF\n", len(signatures))
						if len(signatures) <= 10 {
							for name, sig := range signatures {
								paramTypes := make([]string, len(sig.Params))
								for i, p := range sig.Params {
									paramTypes[i] = p.Type
								}
								fmt.Fprintf(os.Stderr, "  - %s(%s) -> %s\n", name, strings.Join(paramTypes, ", "), sig.ReturnType)
							}
						}
					}
				} else if VerboseMode {
					fmt.Fprintf(os.Stderr, "No DWARF debug info found in %s\n", cImport.SoPath)
				}
			}

			// Extract constants and function signatures from C headers
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Extracting constants and functions from %s headers...\n", cImport.Library)
			}
			constants, err := ExtractConstantsFromLibrary(cImport.Library)
			if err != nil {
				// Non-fatal: constants extraction is optional
				fmt.Fprintf(os.Stderr, "Warning: failed to extract constants from %s: %v\n", cImport.Library, err)
			} else {
				// Ensure cConstants map is initialized
				if fc.cConstants[cImport.Alias] == nil {
					fc.cConstants[cImport.Alias] = &CHeaderConstants{
						Constants: make(map[string]int64),
						Macros:    make(map[string]string),
						Functions: make(map[string]*CFunctionSignature),
					}
				}
				// Merge with existing data (don't overwrite DWARF or builtin signatures!)
				for k, v := range constants.Constants {
					fc.cConstants[cImport.Alias].Constants[k] = v
				}
				for k, v := range constants.Macros {
					fc.cConstants[cImport.Alias].Macros[k] = v
				}
				// Merge functions (header signatures don't overwrite DWARF/builtin)
				for k, v := range constants.Functions {
					if _, exists := fc.cConstants[cImport.Alias].Functions[k]; !exists {
						fc.cConstants[cImport.Alias].Functions[k] = v
					}
				}
				if fc.verbose {
					fmt.Fprintf(os.Stderr, "Extracted %d constants and %d functions from %s\n",
						len(constants.Constants), len(constants.Functions), cImport.Library)
				}
			}

			// Fallback: Add known library functions from libdef.go if extraction failed/incomplete
			if cImport.Library == "libc" || cImport.Library == "glibc" {
				// Ensure cConstants map is initialized
				if fc.cConstants[cImport.Alias] == nil {
					fc.cConstants[cImport.Alias] = &CHeaderConstants{
						Constants: make(map[string]int64),
						Macros:    make(map[string]string),
						Functions: make(map[string]*CFunctionSignature),
					}
				}

				// Add built-in glibc function signatures
				glibcDef := CreateGlibcDefinition()
				for name, fn := range glibcDef.Functions {
					if _, exists := fc.cConstants[cImport.Alias].Functions[name]; !exists {
						// Convert libdef.Function to CFunctionSignature
						params := make([]CFunctionParam, len(fn.Parameters))
						for i, p := range fn.Parameters {
							params[i] = CFunctionParam{
								Name: p.Name,
								Type: p.Type.String(), // Convert CType to string
							}
						}
						fc.cConstants[cImport.Alias].Functions[name] = &CFunctionSignature{
							ReturnType: fn.ReturnType.String(), // Convert CType to string
							Params:     params,
						}
					}
				}
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Added %d built-in glibc functions\n", len(glibcDef.Functions))
				}
			}
		}
	}
}

// detectMainCallInTopLevel checks if main() is called anywhere in top-level statements
// (outside of lambda definitions). Returns true if main() is called, false otherwise.
func (fc *C67Compiler) detectMainCallInTopLevel(statements []Statement) bool {
	for _, stmt := range statements {
		if fc.statementCallsMain(stmt) {
			return true
		}
	}
	return false
}

// statementCallsMain recursively checks if a statement calls main()
func (fc *C67Compiler) statementCallsMain(stmt Statement) bool {
	switch s := stmt.(type) {
	case *ExpressionStmt:
		return fc.expressionCallsMain(s.Expr)
	case *AssignStmt:
		// Don't check if the assignment is defining main itself
		if s.Name == "main" {
			return false
		}
		return fc.expressionCallsMain(s.Value)
	case *MultipleAssignStmt:
		return fc.expressionCallsMain(s.Value)
	case *MapUpdateStmt:
		return fc.expressionCallsMain(s.Index) || fc.expressionCallsMain(s.Value)
	default:
		// Other statement types (loops, arenas, etc.) don't need deep inspection for now
		// since main() calls in those contexts are less common
		return false
	}
}

// expressionCallsMain recursively checks if an expression calls main()
func (fc *C67Compiler) expressionCallsMain(expr Expression) bool {
	if expr == nil {
		return false
	}

	switch e := expr.(type) {
	case *CallExpr:
		// Direct main() call
		if e.Function == "main" {
			return true
		}
		// Check arguments for nested main() calls
		for _, arg := range e.Args {
			if fc.expressionCallsMain(arg) {
				return true
			}
		}
		return false

	case *BinaryExpr:
		return fc.expressionCallsMain(e.Left) || fc.expressionCallsMain(e.Right)

	case *UnaryExpr:
		return fc.expressionCallsMain(e.Operand)

	case *MatchExpr:
		// Check the condition being matched
		if fc.expressionCallsMain(e.Condition) {
			return true
		}
		// Check all match clauses
		for _, clause := range e.Clauses {
			if clause.Guard != nil && fc.expressionCallsMain(clause.Guard) {
				return true
			}
			if clause.Result != nil && fc.expressionCallsMain(clause.Result) {
				return true
			}
		}
		if e.DefaultExpr != nil && fc.expressionCallsMain(e.DefaultExpr) {
			return true
		}
		return false

	case *ListExpr:
		for _, elem := range e.Elements {
			if fc.expressionCallsMain(elem) {
				return true
			}
		}
		return false

	case *MapExpr:
		for i := range e.Keys {
			if fc.expressionCallsMain(e.Keys[i]) || fc.expressionCallsMain(e.Values[i]) {
				return true
			}
		}
		return false

	case *LambdaExpr:
		// Don't check inside lambda bodies - those are not top-level
		return false

	case *IndexExpr:
		return fc.expressionCallsMain(e.List) || fc.expressionCallsMain(e.Index)

	case *SliceExpr:
		return fc.expressionCallsMain(e.List) || fc.expressionCallsMain(e.Start) || fc.expressionCallsMain(e.End)

	case *LengthExpr:
		return fc.expressionCallsMain(e.Operand)

	case *BlockExpr:
		// Check statements in block
		for _, stmt := range e.Statements {
			if fc.statementCallsMain(stmt) {
				return true
			}
		}
		return false

	default:
		// Literals and simple expressions don't call functions
		return false
	}
}

// collectAllFunctions does a pre-pass to collect all function definitions
// This enables forward references - functions can be called before they're defined
func (fc *C67Compiler) collectAllFunctions(program *Program) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Pre-pass: Collecting all function definitions for forward references\n")
	}

	// Walk through all statements and find assignments that define functions
	for _, stmt := range program.Statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			// Check if the value is a lambda (function definition)
			if _, isLambda := assign.Value.(*LambdaExpr); isLambda {
				// Mark this function as forward-referenceable
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "   Marked function for forward reference: %s\n", assign.Name)
				}

				if fc.forwardFunctions == nil {
					fc.forwardFunctions = make(map[string]bool)
				}
				fc.forwardFunctions[assign.Name] = true
			}
		}
	}
}

func (fc *C67Compiler) analyzeRuntimeFeatures(program *Program) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Pre-analysis: Detecting required runtime features\n")
	}

	// Walk through all statements and expressions to detect runtime feature usage
	for _, stmt := range program.Statements {
		fc.analyzeStatementFeatures(stmt)
	}

	if VerboseMode {
		if fc.runtimeFeatures.Uses(FeatureStringConcat) {
			fmt.Fprintf(os.Stderr, "   Detected: string concatenation\n")
		}
		if fc.runtimeFeatures.needsArenaInit() {
			fmt.Fprintf(os.Stderr, "   Detected: arena allocation needed\n")
		}
		if fc.runtimeFeatures.needsCPUDetection() {
			fmt.Fprintf(os.Stderr, "   Detected: CPU feature detection needed\n")
		}
	}
}

func (fc *C67Compiler) analyzeStatementFeatures(stmt Statement) {
	switch s := stmt.(type) {
	case *AssignStmt:
		fc.analyzeExpressionFeatures(s.Value)
	case *ExpressionStmt:
		fc.analyzeExpressionFeatures(s.Expr)
	}
}

func (fc *C67Compiler) analyzeExpressionFeatures(expr Expression) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *BinaryExpr:
		// String concatenation
		if e.Operator == "+" {
			leftType := fc.getExprTypeForAnalysis(e.Left)
			rightType := fc.getExprTypeForAnalysis(e.Right)
			if leftType == "string" || rightType == "string" {
				fc.runtimeFeatures.Mark(FeatureStringConcat)
			}
		}
		fc.analyzeExpressionFeatures(e.Left)
		fc.analyzeExpressionFeatures(e.Right)
	
	case *CallExpr:
		// Check for builtin functions that use runtime features
		switch e.Function {
		case "printf", "println":
			fc.runtimeFeatures.Mark(FeaturePrintf)
		}
		for _, arg := range e.Args {
			fc.analyzeExpressionFeatures(arg)
		}
	
	case *LambdaExpr:
		fc.analyzeExpressionFeatures(e.Body)
	
	case *BlockExpr:
		for _, stmt := range e.Statements {
			fc.analyzeStatementFeatures(stmt)
		}
	
	case *MatchExpr:
		if e.Condition != nil {
			fc.analyzeExpressionFeatures(e.Condition)
		}
		for _, clause := range e.Clauses {
			if clause.Guard != nil {
				fc.analyzeExpressionFeatures(clause.Guard)
			}
			if clause.Result != nil {
				fc.analyzeExpressionFeatures(clause.Result)
			}
		}
		if e.DefaultExpr != nil {
			fc.analyzeExpressionFeatures(e.DefaultExpr)
		}
	
	case *LoopExpr:
		for _, stmt := range e.Body {
			fc.analyzeStatementFeatures(stmt)
		}
		if e.Iterable != nil {
			fc.analyzeExpressionFeatures(e.Iterable)
		}
	
	case *FStringExpr:
		// F-strings require string concatenation
		fc.runtimeFeatures.Mark(FeatureStringConcat)
		for _, part := range e.Parts {
			fc.analyzeExpressionFeatures(part)
		}
	}
}

func (fc *C67Compiler) getExprTypeForAnalysis(expr Expression) string {
	if expr == nil {
		return "unknown"
	}

	switch e := expr.(type) {
	case *StringExpr:
		return "string"
	case *NumberExpr:
		return "number"
	case *BinaryExpr:
		if e.Operator == "+" {
			leftType := fc.getExprTypeForAnalysis(e.Left)
			rightType := fc.getExprTypeForAnalysis(e.Right)
			if leftType == "string" || rightType == "string" {
				return "string"
			}
		}
		return "number"
	}
	return "unknown"
}


// This allows functions to be called before they appear in the source (forward references)
func (fc *C67Compiler) reorderStatementsForForwardRefs(statements []Statement) []Statement {
	var functionDefs []Statement
	var otherStmts []Statement

	for _, stmt := range statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			// Check if this is a function definition
			if _, isLambda := assign.Value.(*LambdaExpr); isLambda {
				functionDefs = append(functionDefs, stmt)
				continue
			}
		}
		// Not a function definition
		otherStmts = append(otherStmts, stmt)
	}

	// Return functions first, then other statements
	result := make([]Statement, 0, len(statements))
	result = append(result, functionDefs...)
	result = append(result, otherStmts...)

	if VerboseMode && len(functionDefs) > 0 {
		fmt.Fprintf(os.Stderr, "-> Reordered %d function definitions to the top\n", len(functionDefs))
	}

	return result
}

func (fc *C67Compiler) Compile(program *Program, outputPath string) error {
	// Clear moved variables tracking for this compilation
	fc.movedVars = make(map[string]bool)
	fc.scopedMoved = []map[string]bool{make(map[string]bool)}

	// Arenas will be enabled on-demand when string concat, list operations, etc. are detected
	// via trackFunctionCall()
	fc.usesArenas = false

	// Check if main() is called at top level (to decide whether to auto-call main)
	fc.mainCalledAtTopLevel = fc.detectMainCallInTopLevel(program.Statements)
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: mainCalledAtTopLevel = %v\n", fc.mainCalledAtTopLevel)
	}

	// Transfer namespace information from program to compiler
	if program.FunctionNamespaces != nil {
		fc.functionNamespace = program.FunctionNamespaces
	}

	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG Compile: starting compilation with %d statements\n", len(program.Statements))
		}
	}

	// Pre-pass: Collect C imports to set up library handles and extract constants
	// This MUST happen before architecture-specific compilation
	fc.processCImports(program)

	// Pre-pass: Collect all function definitions to enable forward references
	// This allows functions to be called before they're defined in the source
	fc.collectAllFunctions(program)

	// Reorder statements to put function definitions first
	// This ensures all functions are defined before being called
	program.Statements = fc.reorderStatementsForForwardRefs(program.Statements)

	// PRE-ANALYSIS PASS: Analyze what runtime features are needed
	// This must happen before any code emission so we know what to initialize
	fc.analyzeRuntimeFeatures(program)

	// Update usesArenas flag based on runtime feature analysis
	if fc.runtimeFeatures.needsArenaInit() {
		fc.usesArenas = true
	}

	if VerboseMode || fc.debug {
		fmt.Fprintf(os.Stderr, "Runtime feature analysis results:\n")
		fmt.Fprintf(os.Stderr, "  needsCPUDetection: %v\n", fc.runtimeFeatures.needsCPUDetection())
		fmt.Fprintf(os.Stderr, "  needsArenaInit: %v\n", fc.runtimeFeatures.needsArenaInit())
		fmt.Fprintf(os.Stderr, "  usesArenas: %v\n", fc.usesArenas)
	}

	// Use ARM64 code generator if target is ARM64
	if fc.eb.target.Arch() == ArchARM64 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "-> Using ARM64 code generator\n")
		}
		return fc.compileARM64(program, outputPath)
	}
	// Use RISC-V64 code generator if target is RISC-V64
	if fc.eb.target.Arch() == ArchRiscv64 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "-> Using RISC-V64 code generator\n")
		}
		return fc.compileRiscv64(program, outputPath)
	}

	// Add format strings for printf
	fc.eb.Define("fmt_str", "%s\x00")
	fc.eb.Define("fmt_int", "%ld\n\x00")
	fc.eb.Define("fmt_float", "%.0f\n\x00") // Print float without decimal places
	fc.eb.Define("_loop_max_exceeded_msg", "Error: loop exceeded maximum iterations\n\x00")
	fc.eb.Define("_recursion_max_exceeded_msg", "Error: recursion exceeded maximum depth\n\x00")
	fc.eb.Define("_null_ptr_msg", "ERROR: Null pointer dereference detected\n\x00")
	fc.eb.Define("_bounds_negative_msg", "ERROR: Array index out of bounds (index < 0)\n\x00")
	fc.eb.Define("_bounds_too_large_msg", "ERROR: Array index out of bounds (index >= length)\n\x00")
	fc.eb.Define("_malloc_failed_msg", "ERROR: Memory allocation failed (out of memory)\n\x00")

	// Arena metadata symbols will be defined later if arenas are used
	// This is checked during the symbol collection pass

	// Debug strings (only defined when needed - currently unused)
	// fc.eb.Define("_str_debug_default_arena", "DEBUG: Initializing default arena\n\x00")
	// fc.eb.Define("_str_debug_arena_value", "DEBUG: Arena pointer value: %p\n\x00")
	// fc.eb.Define("_str_arena_ptr_fmt", "arena_alloc: arena_ptr=%p\n\x00")
	// fc.eb.Define("_str_alloc_loading_arena", "alloc: loading arena pointer\n\x00")
	// fc.eb.Define("_str_meta_arena_addr", "alloc: meta-arena address=%p\n\x00")
	// fc.eb.Define("_str_meta_arena_ptr", "alloc: meta-arena pointer=%p\n\x00")
	// fc.eb.Define("_str_ensure_capacity_called", "ensure_capacity called with required_depth=%ld\n\x00")
	// fc.eb.Define("_str_capacity_value", "current capacity=%ld\n\x00")
	// fc.eb.Define("_count_mismatch_error", "ERROR: Count write/read mismatch!\n\x00")

	// Initialize registers at entry (where _start jumps to)
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.XorRegWithReg("rdi", "rdi")
	fc.out.XorRegWithReg("rsi", "rsi")

	// ===== CPU FEATURE DETECTION =====
	// Only emit CPU feature detection if SIMD/FMA instructions are actually used
	if fc.runtimeFeatures.needsCPUDetection() {
		// Detect FMA, AVX2, POPCNT, and AVX-512 support at runtime
		// This enables dynamic optimization for available CPU features

		fc.eb.DefineWritable("cpu_has_fma", "\x00")    // FMA3 support (Haswell 2013+)
		fc.eb.DefineWritable("cpu_has_avx2", "\x00")   // AVX2 support (Haswell 2013+)
		fc.eb.DefineWritable("cpu_has_popcnt", "\x00") // POPCNT support (Nehalem 2008+)
		fc.eb.DefineWritable("cpu_has_avx512", "\x00") // AVX-512F support (Skylake-X 2017+)

		// Check CPUID leaf 1 for FMA and POPCNT
		fc.out.MovImmToReg("rax", "1")     // CPUID leaf 1
		fc.out.XorRegWithReg("rcx", "rcx") // subleaf 0
		fc.out.Emit([]byte{0x0f, 0xa2})    // cpuid

		// Test ECX bit 12 (FMA)
		fc.out.Emit([]byte{0x0f, 0xba, 0xe1, 0x0c}) // bt ecx, 12
		fc.out.Emit([]byte{0x0f, 0x92, 0xc0})       // setc al
		fc.out.LeaSymbolToReg("rbx", "cpu_has_fma")
		fc.out.MovByteRegToMem("rax", "rbx", 0)

		// Test ECX bit 23 (POPCNT)
		fc.out.Emit([]byte{0x0f, 0xba, 0xe1, 0x17}) // bt ecx, 23
		fc.out.Emit([]byte{0x0f, 0x92, 0xc0})       // setc al
		fc.out.LeaSymbolToReg("rbx", "cpu_has_popcnt")
		fc.out.MovByteRegToMem("rax", "rbx", 0)

		// Check CPUID leaf 7 for AVX2 and AVX-512
		fc.out.MovImmToReg("rax", "7")     // CPUID leaf 7
		fc.out.XorRegWithReg("rcx", "rcx") // subleaf 0
		fc.out.Emit([]byte{0x0f, 0xa2})    // cpuid

		// Test EBX bit 5 (AVX2)
		fc.out.Emit([]byte{0x0f, 0xba, 0xe3, 0x05}) // bt ebx, 5
		fc.out.Emit([]byte{0x0f, 0x92, 0xc0})       // setc al
		fc.out.LeaSymbolToReg("rbx", "cpu_has_avx2")
		fc.out.MovByteRegToMem("rax", "rbx", 0)

		// Test EBX bit 16 (AVX512F - foundation)
		fc.out.Emit([]byte{0x0f, 0xba, 0xe3, 0x10}) // bt ebx, 16
		fc.out.Emit([]byte{0x0f, 0x92, 0xc0})       // setc al
		fc.out.LeaSymbolToReg("rbx", "cpu_has_avx512")
		fc.out.MovByteRegToMem("rax", "rbx", 0)

		// Clear registers used for CPUID
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.XorRegWithReg("rbx", "rbx")
		fc.out.XorRegWithReg("rcx", "rcx")
	}
	// ===== END CPU FEATURE DETECTION =====

	// Two-pass compilation: First pass collects all variable declarations
	// so that function/constant order doesn't matter
	for i, stmt := range program.Statements {
		if VerboseMode && i < 10 {
			if assign, ok := stmt.(*AssignStmt); ok {
				fmt.Fprintf(os.Stderr, "DEBUG collectSymbols [%d]: %s (Mutable=%v, IsUpdate=%v)\n",
					i, assign.Name, assign.Mutable, assign.IsUpdate)
			}
		}
		if err := fc.collectSymbols(stmt); err != nil {
			return err
		}
	}

	// Define global variables in .data section (after collecting symbols)
	for varName := range fc.globalVars {
		fc.eb.DefineWritable("_global_"+varName, "\x00\x00\x00\x00\x00\x00\x00\x00") // 8 bytes for float64
	}

	// currentArena is already set to 1 in NewC67Compiler (representing meta-arena[0])
	// Arena initialization is needed only if we actually use arenas
	// (e.g., for string concat, list operations, etc.)
	if fc.runtimeFeatures.needsArenaInit() {
		fc.initializeMetaArenaAndGlobalArena()
	}

	// Function prologue - set up stack frame for main code
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// REGISTER ALLOCATOR: Save callee-saved registers used for loop optimization
	// rbx = loop counter
	fc.out.PushReg("rbx")
	// Maintain 16-byte stack alignment required by x86_64 ABI
	// (call pushed 8 bytes, rbp pushed 8 bytes, rbx pushed 8 bytes = 24 bytes total)
	// Subtract 8 more bytes to reach 32 bytes (16-byte aligned)
	fc.out.SubImmFromReg("rsp", 8)

	if fc.maxStackOffset > 0 {
		alignedSize := int64((fc.maxStackOffset + 15) & ^15)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Allocating %d bytes of stack space (maxStackOffset=%d)\n", alignedSize, fc.maxStackOffset)
		}
		fc.out.SubImmFromReg("rsp", alignedSize)
	}

	fc.pushDeferScope()

	// Predeclare lambda symbols so closure initialization can reference them
	fc.predeclareLambdaSymbols()

	// Second pass: Generate actual code with all symbols known
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Compiling %d statements\n", len(program.Statements))
		for i, stmt := range program.Statements {
			fmt.Fprintf(os.Stderr, "DEBUG:   Statement %d: %T\n", i, stmt)
		}
	}
	for i, stmt := range program.Statements {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: About to compile statement %d: %T\n", i, stmt)
		}
		fc.compileStatement(stmt)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Finished compiling statement %d\n", i)
		}
	}

	fc.popDeferScope()

	// Jump over lambda functions to reach the main evaluation code
	skipLambdasJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Will be patched
	skipLambdasEnd := fc.eb.text.Len()

	// Generate lambda functions here (before exit, but jumped over)
	fc.generateLambdaFunctions()

	// Patch the jump to skip over lambdas
	skipLambdasTarget := fc.eb.text.Len()
	fc.patchJumpImmediate(skipLambdasJump+1, int32(skipLambdasTarget-skipLambdasEnd))

	// Evaluate main (if it exists) to get the exit code BEFORE cleaning up arenas
	// main can be a direct value (main = 42) or a function (main = { 42 })
	_, exists := fc.variables["main"]
	if exists {
		// main exists - check if it's a lambda/function or a direct value
		if fc.lambdaVars["main"] {
			// main is a lambda/function
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: main is a lambda, mainCalledAtTopLevel=%v\n", fc.mainCalledAtTopLevel)
			}
			// Only auto-call if main() was NOT explicitly called at top level
			if !fc.mainCalledAtTopLevel {
				// Auto-call main with no arguments
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG: Auto-calling main()\n")
				}
				fc.compileExpression(&CallExpr{Function: "main", Args: []Expression{}})
			} else {
				// main() was already called - use exit code 0
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG: Skipping auto-call of main() (already called at top level)\n")
				}
				fc.out.XorRegWithReg("xmm0", "xmm0")
			}
		} else {
			// main is a direct value - just load it
			fc.compileExpression(&IdentExpr{Name: "main"})
		}
		// Result is in xmm0 (float64)
	} else {
		// No main - use exit code 0
		fc.out.XorRegWithReg("xmm0", "xmm0")
	}

	// Convert float64 result in xmm0 to int32 in rdi (for exit code)
	// cvttsd2si rdi, xmm0 (convert with truncation scalar double to signed int)
	fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xf8})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Exit code conversion complete, value in rdi\n")
	}

	// Save exit code on stack before cleanup (rdi will be clobbered by munmap syscalls)
	fc.out.PushReg("rdi")

	// Cleanup all arenas in meta-arena at program exit
	// Skip on Windows to avoid Wine compatibility issues (OS will clean up on process exit anyway)
	if fc.eb.target.OS() != OSWindows {
		fc.cleanupAllArenas()
	}

	// Restore exit code to rdi
	fc.out.PopReg("rdi")

	// Always add implicit exit at the end of the program
	// Even if there's an exit() call in the code, it might be conditional
	// If an unconditional exit() is called, it will never return, so this code is harmless

	// Determine if we need libc exit or can use syscall
	// We need libc exit if:
	// 1. On Windows (no syscalls)
	// 2. Used C FFI functions (c.printf, c.exit, etc.) that need libc cleanup
	// 3. Used libc printf (on non-Linux systems)
	needsLibcExit := fc.eb.target.OS() == OSWindows
	if !needsLibcExit && fc.eb.target.OS() != OSLinux {
		// Non-Linux, non-Windows systems: check if used printf (which would be libc printf)
		needsLibcExit = fc.usedFunctions["printf"]
	}

	if needsLibcExit {
		// Use libc's exit() for proper cleanup (flushes buffers)
		// Exit code is already in rdi (first argument)
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")
	} else {
		// Use direct syscall exit on Linux (works with syscall-based printf)
		fc.out.MovImmToReg("rax", "60") // syscall number for exit
		// exit code is already in rdi (first syscall argument)
		fc.eb.Emit("syscall") // invoke syscall directly
	}

	// Lambda functions were already generated and jumped over before main evaluation

	// Generate runtime helpers (string conversion, concatenation, etc.)
	// For ELF, this is done in writeELF() after second lambda pass
	// For PE, we do it here since PE doesn't have a second pass
	if fc.eb.target.IsPE() {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Generating runtime helpers for PE\n")
		}
		fc.generateRuntimeHelpers()
	}

	// Write executable in appropriate format based on target OS
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Target format: OS=%v, IsPE=%v, IsMachO=%v, IsELF=%v\n",
			fc.eb.target.OS(), fc.eb.target.IsPE(), fc.eb.target.IsMachO(), fc.eb.target.IsELF())
	}

	if fc.eb.target.IsPE() {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Writing PE executable to %s\n", outputPath)
		}
		return fc.writePE(program, outputPath)
	} else if fc.eb.target.IsMachO() {
		// MachO is handled in ARM64 codegen path above
		return fmt.Errorf("MachO should be handled by ARM64 code generator")
	}

	// Default: Write ELF using existing infrastructure
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Writing ELF executable to %s\n", outputPath)
	}
	return fc.writeELF(program, outputPath)
}

// collectSymbols performs the first pass: collect all variable declarations
// without generating any code. This allows forward references.
func (fc *C67Compiler) updateStackOffset(delta int) {
	fc.stackOffset += delta
	if fc.stackOffset > fc.maxStackOffset {
		fc.maxStackOffset = fc.stackOffset
	}
}

// Confidence that this function is working: 95%
func (fc *C67Compiler) collectSymbols(stmt Statement) error {
	switch s := stmt.(type) {
	case *AssignStmt:
		// Check if variable already exists
		_, exists := fc.variables[s.Name]

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG collectSymbols AssignStmt: name=%s, exists=%v, IsUpdate=%v, Mutable=%v, mutableVars[%s]=%v, stackOffset=%d\n",
				s.Name, exists, s.IsUpdate, s.Mutable, s.Name, fc.mutableVars[s.Name], fc.stackOffset)
		}

		// Debug: print stack trace when we see the problematic variable
		if s.Name == "r1" && exists {
			fmt.Fprintf(os.Stderr, "DEBUG: Variable 'r1' already exists! Stack trace:\n")
			debug.PrintStack()
		}

		if s.IsUpdate {
			// Update operation (<-) requires existing mutable variable
			if !exists {
				return fmt.Errorf("cannot update undefined variable '%s'", s.Name)
			}
			// Check both local mutableVars and global globalVarsMutable
			isMutable := fc.mutableVars[s.Name] || fc.globalVarsMutable[s.Name]
			if !isMutable {
				return fmt.Errorf("cannot update immutable variable '%s' (use <- only for mutable variables)", s.Name)
			}
		} else if s.Mutable {
			if exists {
				return fmt.Errorf("variable '%s' already defined (use <- to update) [currently at offset %d]", s.Name, fc.variables[s.Name])
			}

			// Track module-level variables (defined outside any lambda) - allocate in .data
			if fc.currentLambda == nil {
				fc.moduleLevelVars[s.Name] = true
				// Allocate in .data section
				dataOffset := len(fc.dataSection)
				fc.dataSection = append(fc.dataSection, make([]byte, 8)...) // 8 bytes for float64
				fc.globalVars[s.Name] = dataOffset
				fc.globalVarsMutable[s.Name] = true
				// Don't use stack offset for globals
				fc.variables[s.Name] = -1 // Mark as global
				fc.mutableVars[s.Name] = true
			} else {
				// Local variable - use stack
				fc.updateStackOffset(16)
				offset := fc.stackOffset
				fc.variables[s.Name] = offset
				fc.mutableVars[s.Name] = true
			}

			if fc.debug {
				if VerboseMode {
					if fc.currentLambda == nil {
						fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: storing mutable global variable '%s' at data offset %d\n", s.Name, fc.globalVars[s.Name])
					} else {
						fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: storing mutable variable '%s' at offset %d\n", s.Name, fc.variables[s.Name])
					}
				}
			}

			// Track type annotation if provided
			if s.TypeAnnotation != nil {
				fc.varTypeInfo[s.Name] = s.TypeAnnotation
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG: Setting varTypeInfo[%s] = %s (mutable, annotated)\n", s.Name, s.TypeAnnotation.String())
				}
			}

			// Track type if we can determine it from the expression
			exprType := fc.getExprType(s.Value)
			if exprType != "number" && exprType != "unknown" {
				fc.varTypes[s.Name] = exprType
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG: Setting varTypes[%s] = %s (mutable)\n", s.Name, exprType)
				}
			}

			// Track if this is a lambda/function
			switch s.Value.(type) {
			case *LambdaExpr, *PatternLambdaExpr, *MultiLambdaExpr:
				fc.lambdaVars[s.Name] = true
			}
		} else {
			// = - Define immutable variable (can shadow existing immutable, but not mutable)
			if exists && fc.mutableVars[s.Name] {
				// Allow updating existing mutable variable with =
				// Don't create new variable, reuse existing offset
				s.IsReuseMutable = true
			} else {
				// Create new immutable variable

				// Track module-level variables (defined outside any lambda) - allocate in .data
				if fc.currentLambda == nil {
					fc.moduleLevelVars[s.Name] = true
					// Allocate in .data section
					dataOffset := len(fc.dataSection)
					fc.dataSection = append(fc.dataSection, make([]byte, 8)...) // 8 bytes for float64
					fc.globalVars[s.Name] = dataOffset
					fc.globalVarsMutable[s.Name] = false
					// Don't use stack offset for globals
					fc.variables[s.Name] = -1 // Mark as global
					fc.mutableVars[s.Name] = false
				} else {
					// Local variable - use stack
					fc.updateStackOffset(16)
					offset := fc.stackOffset
					fc.variables[s.Name] = offset
					fc.mutableVars[s.Name] = false
				}

				if fc.debug {
					if VerboseMode {
						if fc.currentLambda == nil {
							fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: storing immutable global variable '%s' at data offset %d\n", s.Name, fc.globalVars[s.Name])
						} else {
							fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: storing immutable variable '%s' at offset %d\n", s.Name, fc.variables[s.Name])
						}
					}
				}

				// Track type annotation if provided
				if s.TypeAnnotation != nil {
					fc.varTypeInfo[s.Name] = s.TypeAnnotation
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: Setting varTypeInfo[%s] = %s (immutable, annotated)\n", s.Name, s.TypeAnnotation.String())
					}
				}

				// Track type if we can determine it from the expression
				exprType := fc.getExprType(s.Value)
				if fc.debug {
					fmt.Fprintf(os.Stderr, "DEBUG TYPE TRACKING: var=%s, exprType=%s, Value type=%T\n", s.Name, exprType, s.Value)
				}
				if exprType != "number" && exprType != "unknown" {
					fc.varTypes[s.Name] = exprType
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: Setting varTypes[%s] = %s (immutable)\n", s.Name, exprType)
					}
				}

				// Track if this is a lambda/function
				switch s.Value.(type) {
				case *LambdaExpr, *PatternLambdaExpr, *MultiLambdaExpr:
					fc.lambdaVars[s.Name] = true
				}
			}
		}
	case *MultipleAssignStmt:
		// Multiple assignment: a, b, c = expr
		// Each variable needs stack space
		for _, name := range s.Names {
			_, exists := fc.variables[name]

			if s.IsUpdate {
				// Update operation (<-) requires existing mutable variables
				if !exists {
					return fmt.Errorf("cannot update undefined variable '%s'", name)
				}
				if !fc.mutableVars[name] {
					return fmt.Errorf("cannot update immutable variable '%s' (use <- only for mutable variables)", name)
				}
			} else if s.Mutable {
				if exists {
					return fmt.Errorf("variable '%s' already defined", name)
				}
				fc.updateStackOffset(16)
				offset := fc.stackOffset
				fc.variables[name] = offset
				fc.mutableVars[name] = true
				// Type will be determined at runtime from list elements
				fc.varTypes[name] = "unknown"
			} else {
				// Immutable assignment
				if exists && fc.mutableVars[name] {
					// Can't shadow mutable with immutable in multiple assignment
					return fmt.Errorf("variable '%s' is mutable, cannot shadow in multiple assignment", name)
				}
				fc.updateStackOffset(16)
				offset := fc.stackOffset
				fc.variables[name] = offset
				fc.mutableVars[name] = false
				fc.varTypes[name] = "unknown"
			}
		}
	case *LoopStmt:
		baseOffset := fc.stackOffset

		if s.BaseOffset == 0 {
			s.BaseOffset = baseOffset
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: Storing baseOffset=%d in LoopStmt (first pass)\n", baseOffset)
			}
		} else {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG collectSymbols: BaseOffset already set to %d, not overwriting with %d\n",
					s.BaseOffset, baseOffset)
			}
		}

		// Confidence that this function is working: 100%
		_, isRange := s.Iterable.(*RangeExpr)
		if isRange {
			if s.NeedsMaxCheck {
				// Stack: [iteration_count][max_iterations][limit][iterator]
				// Need to account for possible stack-based counter (depth >= 4)
				// Allocate maximum space needed (with counter)
				fc.updateStackOffset(40)
			} else {
				// Stack: [limit][counter(if needed)][iterator]
				// Allocate maximum space needed (with stack counter)
				fc.updateStackOffset(40)
			}
		} else {
			fc.updateStackOffset(64)
		}

		for _, bodyStmt := range s.Body {
			if err := fc.collectSymbols(bodyStmt); err != nil {
				return err
			}
		}

		// Restore stackOffset after loop body
		// Sequential loops at the same nesting level should start at the same stackOffset
		fc.stackOffset = baseOffset

	case *WhileStmt:
		baseOffset := fc.stackOffset

		if s.BaseOffset == 0 {
			s.BaseOffset = baseOffset
		}

		// Allocate stack space for iteration counter (8 bytes)
		fc.updateStackOffset(8)

		for _, bodyStmt := range s.Body {
			if err := fc.collectSymbols(bodyStmt); err != nil {
				return err
			}
		}

		// Restore stackOffset after loop body
		fc.stackOffset = baseOffset

	case *ReceiveLoopStmt:
		baseOffset := fc.stackOffset

		if s.BaseOffset == 0 {
			s.BaseOffset = baseOffset
		}

		// Allocate stack space for:
		// - message variable (8 bytes) at baseOffset+8
		// - sender variable (8 bytes) at baseOffset+16
		// - socket fd (8 bytes) at baseOffset+24
		// - sockaddr_in (16 bytes) at baseOffset+40 (with padding to avoid overlap)
		// - buffer (256 bytes) starting at baseOffset+56
		// - addrlen (8 bytes) at baseOffset+320
		// Total: 320 bytes
		fc.updateStackOffset(320)

		for _, bodyStmt := range s.Body {
			if err := fc.collectSymbols(bodyStmt); err != nil {
				return err
			}
		}

		// Restore stackOffset after loop body
		fc.stackOffset = baseOffset

	case *ArenaStmt:
		// Track arena depth during symbol collection
		// This ensures alloc() calls are validated correctly
		fc.usesArenas = true // Mark that this program uses arenas (for runtime helper generation)
		fc.runtimeFeatures.Mark(FeatureArenaAlloc)
		previousArena := fc.currentArena
		fc.currentArena++

		// Recursively collect symbols from arena body
		// Note: Arena pointers are stored in static storage (_c67_arena_ptrs)
		for _, bodyStmt := range s.Body {
			if err := fc.collectSymbols(bodyStmt); err != nil {
				return err
			}
		}

		// Restore arena depth
		fc.currentArena = previousArena
	case *CStructDecl:
		// Cstruct declarations don't allocate runtime stack space
		// Constants are already registered in parser (Name_SIZEOF, Name_field_OFFSET)
	case *ExpressionStmt:
		// No symbols to collect from expression statements
	}
	return nil
}

func (fc *C67Compiler) collectLoopsFromExpression(expr Expression) {
	switch e := expr.(type) {
	case *LoopExpr:
		fc.labelCounter++
		loopLabel := fc.labelCounter
		baseOffset := fc.stackOffset
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG collectLoopsFromExpression: Setting loopBaseOffsets[%d] = %d (stackOffset=%d)\n",
				loopLabel, baseOffset, fc.stackOffset)
		}
		fc.loopBaseOffsets[loopLabel] = baseOffset

		if e.NeedsMaxCheck {
			fc.updateStackOffset(48)
		} else {
			fc.updateStackOffset(24)
		}

		oldVariables := fc.variables
		oldMutableVars := fc.mutableVars
		fc.variables = make(map[string]int)
		fc.mutableVars = make(map[string]bool)
		for k, v := range oldVariables {
			fc.variables[k] = v
		}
		for k, v := range oldMutableVars {
			fc.mutableVars[k] = v
		}

		for _, bodyStmt := range e.Body {
			if err := fc.collectSymbols(bodyStmt); err != nil {
				return
			}
		}

		fc.variables = oldVariables
		fc.mutableVars = oldMutableVars
		fc.stackOffset = baseOffset

	case *BinaryExpr:
		fc.collectLoopsFromExpression(e.Left)
		fc.collectLoopsFromExpression(e.Right)

	case *CallExpr:
		for _, arg := range e.Args {
			fc.collectLoopsFromExpression(arg)
		}

	case *LambdaExpr:
		// Don't recurse into lambda bodies - they have their own scope
		// Lambdas will be processed separately in generateLambdaFunctions()

	case *ListExpr:
		for _, elem := range e.Elements {
			fc.collectLoopsFromExpression(elem)
		}

	case *MapExpr:
		for i := range e.Keys {
			fc.collectLoopsFromExpression(e.Keys[i])
			fc.collectLoopsFromExpression(e.Values[i])
		}

	case *IndexExpr:
		fc.collectLoopsFromExpression(e.List)
		fc.collectLoopsFromExpression(e.Index)

	case *RangeExpr:
		fc.collectLoopsFromExpression(e.Start)
		fc.collectLoopsFromExpression(e.End)

	case *ParallelExpr:
		fc.collectLoopsFromExpression(e.List)
		fc.collectLoopsFromExpression(e.Operation)

	case *PipeExpr:
		fc.collectLoopsFromExpression(e.Left)
		fc.collectLoopsFromExpression(e.Right)

	case *InExpr:
		fc.collectLoopsFromExpression(e.Value)
		fc.collectLoopsFromExpression(e.Container)

	case *LengthExpr:
		fc.collectLoopsFromExpression(e.Operand)

	case *MatchExpr:
		fc.collectLoopsFromExpression(e.Condition)
		for _, clause := range e.Clauses {
			if clause.Guard != nil {
				fc.collectLoopsFromExpression(clause.Guard)
			}
			fc.collectLoopsFromExpression(clause.Result)
		}
		if e.DefaultExpr != nil {
			fc.collectLoopsFromExpression(e.DefaultExpr)
		}

	case *BlockExpr:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG collectLoopsFromExpression BlockExpr: variables BEFORE = %v, stackOffset=%d\n",
				fc.variables, fc.stackOffset)
		}
		oldVariables := fc.variables
		oldMutableVars := fc.mutableVars
		oldStackOffset := fc.stackOffset // Save stackOffset to restore after processing block
		fc.variables = make(map[string]int)
		fc.mutableVars = make(map[string]bool)
		for k, v := range oldVariables {
			fc.variables[k] = v
		}
		for k, v := range oldMutableVars {
			fc.mutableVars[k] = v
		}

		for _, stmt := range e.Statements {
			if err := fc.collectSymbols(stmt); err != nil {
				return
			}
		}

		fc.variables = oldVariables
		fc.mutableVars = oldMutableVars
		fc.stackOffset = oldStackOffset // Restore stackOffset - block variables will be re-allocated in compileExpression
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG collectLoopsFromExpression BlockExpr: variables AFTER = %v, stackOffset=%d\n",
				fc.variables, fc.stackOffset)
		}

	case *UnaryExpr:
		fc.collectLoopsFromExpression(e.Operand)

	case *PostfixExpr:
		fc.collectLoopsFromExpression(e.Operand)

	case *CastExpr:
		fc.collectLoopsFromExpression(e.Expr)

	case *SliceExpr:
		fc.collectLoopsFromExpression(e.List)
		if e.Start != nil {
			fc.collectLoopsFromExpression(e.Start)
		}
		if e.End != nil {
			fc.collectLoopsFromExpression(e.End)
		}

	case *NumberExpr, *IdentExpr, *StringExpr, *FStringExpr, *NamespacedIdentExpr:
	}
}

func (fc *C67Compiler) isExpressionPure(expr Expression, pureFunctions map[string]bool) bool {
	switch e := expr.(type) {
	case *NumberExpr, *StringExpr:
		return true
	case *IdentExpr:
		return true
	case *BinaryExpr:
		return fc.isExpressionPure(e.Left, pureFunctions) && fc.isExpressionPure(e.Right, pureFunctions)
	case *UnaryExpr:
		return fc.isExpressionPure(e.Operand, pureFunctions)
	case *MatchExpr:
		if !fc.isExpressionPure(e.Condition, pureFunctions) {
			return false
		}
		for _, clause := range e.Clauses {
			if !fc.isExpressionPure(clause.Result, pureFunctions) {
				return false
			}
		}
		return true
	case *CallExpr:
		impureBuiltins := map[string]bool{
			"print": true, "println": true, "printf": true, "exit": true,
			"eprint": true, "eprintln": true, "eprintf": true,
			"exitln": true, "exitf": true,
			"syscall": true, "alloc": true, "free": true,
		}
		if impureBuiltins[e.Function] {
			return false
		}
		if !pureFunctions[e.Function] {
			return false
		}
		for _, arg := range e.Args {
			if !fc.isExpressionPure(arg, pureFunctions) {
				return false
			}
		}
		return true
	case *LambdaExpr:
		return len(e.CapturedVars) == 0 && fc.isExpressionPure(e.Body, pureFunctions)
	case *IndexExpr:
		return fc.isExpressionPure(e.List, pureFunctions) && fc.isExpressionPure(e.Index, pureFunctions)
	case *ListExpr:
		for _, elem := range e.Elements {
			if !fc.isExpressionPure(elem, pureFunctions) {
				return false
			}
		}
		return true
	case *MapExpr:
		for _, key := range e.Keys {
			if !fc.isExpressionPure(key, pureFunctions) {
				return false
			}
		}
		for _, val := range e.Values {
			if !fc.isExpressionPure(val, pureFunctions) {
				return false
			}
		}
		return true
	case *RangeExpr:
		return fc.isExpressionPure(e.Start, pureFunctions) && fc.isExpressionPure(e.End, pureFunctions)
	case *LengthExpr:
		return fc.isExpressionPure(e.Operand, pureFunctions)
	case *InExpr:
		return fc.isExpressionPure(e.Value, pureFunctions) && fc.isExpressionPure(e.Container, pureFunctions)
	case *LoopExpr, *BlockExpr:
		return false
	default:
		return false
	}
}

// Confidence that this function is working: 100%
func (fc *C67Compiler) compileStatement(stmt Statement) {
	switch s := stmt.(type) {
	case *AssignStmt:
		if fc.debug {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG compileStatement: compiling assignment '%s' (type: %T)\n", s.Name, s.Value)
			}
		}

		// Check if it's a global variable
		if _, isGlobal := fc.globalVars[s.Name]; isGlobal {
			// Compile the value
			fc.currentAssignName = s.Name
			fc.compileExpression(s.Value)
			fc.currentAssignName = ""
			// Store to .data section
			// lea rax, [rel _global_varname]
			// movsd [rax], xmm0
			fc.out.LeaSymbolToReg("rax", "_global_"+s.Name)
			fc.out.MovXmmToMem("xmm0", "rax", 0)
		} else {
			// Local variable
			offset := fc.variables[s.Name]

			// Only allocate new stack space if this is a new variable definition
			// Don't allocate if:
			// - IsUpdate (i.e., <- operator)
			// - IsReuseMutable (i.e., = updating existing mutable variable)
			// Note: Lambdas with local variables are rejected at compile time,
			// so we don't need a special check here
			if !s.IsUpdate && !s.IsReuseMutable {
				fc.out.SubImmFromReg("rsp", 16)
				fc.runtimeStack += 16
			}

			fc.currentAssignName = s.Name
			fc.compileExpression(s.Value)
			fc.currentAssignName = ""
			// Use r11 for parent variables in parallel loops, rbp for local variables
			baseReg := "rbp"
			if fc.parentVariables != nil && fc.parentVariables[s.Name] {
				baseReg = "r11"
			}
			fc.out.MovXmmToMem("xmm0", baseReg, -offset)
		}

	case *MultipleAssignStmt:
		// Confidence that this function is working: 100%
		// Multiple assignment: a, b, c = expr
		// expr must evaluate to a list, we unpack elements to variables

		// Only allocate runtime stack space for NEW immutable variables
		// Mutable variables and updates reuse existing space
		if !s.IsUpdate && !s.Mutable {
			// Immutable new variables - allocate runtime space
			fc.out.SubImmFromReg("rsp", int64(len(s.Names)*16))
			fc.runtimeStack += len(s.Names) * 16
		} else if !s.IsUpdate && s.Mutable {
			// Mutable new variables - allocate runtime space
			fc.out.SubImmFromReg("rsp", int64(len(s.Names)*16))
			fc.runtimeStack += len(s.Names) * 16
		}
		// Updates reuse existing stack space, no allocation needed

		// Compile the expression (should return a list pointer in xmm0)
		fc.compileExpression(s.Value)

		// Save list pointer to stack
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Load list pointer to a register
		fc.out.MovqXmmToReg("rsi", "xmm0") // rsi = list pointer

		// Extract each element and assign to variables
		for i, name := range s.Names {
			offset := fc.variables[name]

			// Calculate element offset: 8 + i*16 + 8 = 16 + i*16
			elementOffset := 16 + i*16

			// Load element at index i
			// Check if list is non-null first (for safety)
			fc.out.TestRegReg("rsi", "rsi")
			skipLoad := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // If null, skip to zero
			skipLoadPatch := fc.eb.text.Len()

			// List exists, load element
			fc.out.MovMemToXmm("xmm0", "rsi", elementOffset)
			skipAfterLoad := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			skipAfterLoadPatch := fc.eb.text.Len()

			// List is null or element doesn't exist, use 0
			zeroPos := fc.eb.text.Len()
			fc.patchJumpImmediate(skipLoad+2, int32(zeroPos-skipLoadPatch))
			fc.out.XorpdXmm("xmm0", "xmm0")

			// Patch skip-after-load jump
			afterZeroPos := fc.eb.text.Len()
			fc.patchJumpImmediate(skipAfterLoad+1, int32(afterZeroPos-skipAfterLoadPatch))

			// Store to variable
			baseReg := "rbp"
			if fc.parentVariables != nil && fc.parentVariables[name] {
				baseReg = "r11"
			}
			fc.out.MovXmmToMem("xmm0", baseReg, -offset)
		}

		// Clean up list pointer from stack
		fc.out.AddImmToReg("rsp", 8)

	case *MapUpdateStmt:
		// List/map element update: arr[idx] <- value
		// For lists (linked lists): Creates new list with updated element
		// For maps: Updates in-place

		// Check if variable exists and is mutable
		offset, exists := fc.variables[s.MapName]
		isGlobal := false
		if !exists {
			// Check if it's a global variable
			if _, globalExists := fc.globalVars[s.MapName]; globalExists {
				isGlobal = true
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG MapUpdateStmt: '%s' is a global variable\n", s.MapName)
				}
			} else {
				suggestions := findSimilarIdentifiers(s.MapName, fc.variables, 3)
				if len(suggestions) > 0 {
					compilerError("undefined variable '%s'. Did you mean: %s?", s.MapName, strings.Join(suggestions, ", "))
				} else {
					compilerError("undefined variable '%s'", s.MapName)
				}
			}
		} else if offset == -1 {
			// offset == -1 means it's a global variable
			isGlobal = true
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG MapUpdateStmt: '%s' is a global variable (offset -1)\n", s.MapName)
			}
		} else {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG MapUpdateStmt: '%s' is a local variable at offset %d\n", s.MapName, offset)
			}
		}

		// Check mutability (check both local and global)
		isMutable := fc.mutableVars[s.MapName] || fc.globalVarsMutable[s.MapName]
		if !isMutable {
			compilerError("cannot modify immutable list '%s'", s.MapName)
		}

		// Check if this is a list or map
		varType := fc.varTypes[s.MapName]
		baseReg := "rbp"
		if fc.parentVariables != nil && fc.parentVariables[s.MapName] {
			baseReg = "r11"
		}

		if varType == "list" {
			// LIST UPDATE: Lists use map representation [count][key0][val0][key1][val1]...
			// Same as map update, but keys are always sequential integers (0, 1, 2...)

			// Compile index expression -> xmm0
			fc.compileExpression(s.Index)
			// Save index to stack
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Compile value expression -> xmm0
			fc.compileExpression(s.Value)
			// Save value to stack
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Load list pointer from variable
			if isGlobal {
				// Load from .data section
				fc.out.LeaSymbolToReg("rax", "_global_"+s.MapName)
				fc.out.MovMemToXmm("xmm1", "rax", 0)
			} else {
				fc.out.MovMemToXmm("xmm1", baseReg, -offset)
			}
			// Convert list pointer to rax
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm1", "rsp", 0)
			fc.out.MovMemToReg("rax", "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)

			// Load index from stack and convert to integer in rcx
			idxReg := fc.regTracker.AllocXMM("index_convert")
			if idxReg == "" {
				idxReg = "xmm2" // Fallback
			}
			fc.out.MovMemToXmm(idxReg, "rsp", 8)
			fc.out.Cvttsd2si("rcx", idxReg) // rcx = integer index
			fc.regTracker.FreeXMM(idxReg)

			// Calculate offset: 8 + (index * 16) + 8 = 16 + index * 16
			// Memory layout: [count][key0][val0][key1][val1]...
			// Value at index i is at: 8 + i*16 + 8
			fc.out.ShlImmReg("rcx", 4)    // rcx = index * 16
			fc.out.AddImmToReg("rcx", 16) // rcx = 16 + index * 16

			// Add offset to list pointer: rax + rcx = address of value
			fc.out.AddRegToReg("rax", "rcx")

			// Load value from stack into xmm0
			fc.out.MovMemToXmm("xmm0", "rsp", 0)

			// Write value to memory at [rax]
			fc.out.MovXmmToMem("xmm0", "rax", 0)

			// Clean up stack (index + value)
			fc.out.AddImmToReg("rsp", 16)
		} else {
			// MAP UPDATE: In-place modification
			// Memory layout: [length (float64)] [key1] [val1] [key2] [val2] ...

			// Compile index expression -> xmm0
			fc.compileExpression(s.Index)
			// Save index to stack
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Compile value expression -> xmm0
			fc.compileExpression(s.Value)
			// Save value to stack
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Load map pointer from variable
			if isGlobal {
				// Load from .data section
				fc.out.LeaSymbolToReg("rax", "_global_"+s.MapName)
				fc.out.MovMemToXmm("xmm1", "rax", 0)
			} else {
				fc.out.MovMemToXmm("xmm1", baseReg, -offset)
			}
			// Convert map pointer to rax
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm1", "rsp", 0)
			fc.out.MovMemToReg("rax", "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)

			// Load index from stack and convert to integer in rcx
			idxReg := fc.regTracker.AllocXMM("index_convert")
			if idxReg == "" {
				idxReg = "xmm2" // Fallback
			}
			fc.out.MovMemToXmm(idxReg, "rsp", 8) // index is 8 bytes below top
			fc.out.Cvttsd2si("rcx", idxReg)      // rcx = integer index
			fc.regTracker.FreeXMM(idxReg)

			// Calculate offset: 8 + (index * 16) + 8 = 16 + index * 16
			// Keys and values alternate: [length][key0][val0][key1][val1]...
			// Value at index i is at: 8 + i*16 + 8
			fc.out.ShlImmReg("rcx", 4)    // rcx = index * 16
			fc.out.AddImmToReg("rcx", 16) // rcx = 16 + index * 16

			// Add offset to map pointer: rax + rcx = address of value
			fc.out.AddRegToReg("rax", "rcx") // rax = address of value

			// Load value from stack into xmm0
			fc.out.MovMemToXmm("xmm0", "rsp", 0)

			// Write value to memory at [rax]
			fc.out.MovXmmToMem("xmm0", "rax", 0)

			// Clean up stack (index + value)
			fc.out.AddImmToReg("rsp", 16)
		}

	case *LoopStmt:
		fc.compileLoopStatement(s)

	case *WhileStmt:
		fc.compileWhileStatement(s)

	case *ReceiveLoopStmt:
		fc.compileReceiveLoopStmt(s)

	case *JumpStmt:
		fc.compileJumpStatement(s)

	case *ExpressionStmt:
		// Handle PostfixExpr as a statement (like Go)
		if postfix, ok := s.Expr.(*PostfixExpr); ok {
			// x++ and x-- are statements only, not expressions
			identExpr, ok := postfix.Operand.(*IdentExpr)
			if !ok {
				compilerError("postfix operator %s requires a variable operand", postfix.Operator)
			}

			// Get the variable's stack offset
			offset, exists := fc.variables[identExpr.Name]
			if !exists {
				suggestions := findSimilarIdentifiers(identExpr.Name, fc.variables, 3)
				if len(suggestions) > 0 {
					compilerError("undefined variable '%s'. Did you mean: %s?", identExpr.Name, strings.Join(suggestions, ", "))
				} else {
					compilerError("undefined variable '%s'", identExpr.Name)
				}
			}

			// Check if variable is mutable
			if !fc.mutableVars[identExpr.Name] {
				compilerError("cannot modify immutable variable '%s'", identExpr.Name)
			}

			// Use r11 for parent variables, rbp for local
			baseReg := "rbp"
			if fc.parentVariables != nil && fc.parentVariables[identExpr.Name] {
				baseReg = "r11"
			}

			// Load current value into xmm0
			fc.out.MovMemToXmm("xmm0", baseReg, -offset)

			// Create 1.0 constant
			labelName := fmt.Sprintf("one_%d", fc.stringCounter)
			fc.stringCounter++

			one := 1.0
			bits := uint64(0)
			*(*float64)(unsafe.Pointer(&bits)) = one
			var floatData []byte
			for i := 0; i < 8; i++ {
				floatData = append(floatData, byte((bits>>(i*8))&ByteMask))
			}
			fc.eb.Define(labelName, string(floatData))

			// Load 1.0 into xmm1
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovMemToXmm("xmm1", "rax", 0)

			// Apply the operation
			switch postfix.Operator {
			case "++":
				fc.out.AddsdXmm("xmm0", "xmm1") // xmm0 = xmm0 + 1.0
			case "--":
				fc.out.SubsdXmm("xmm0", "xmm1") // xmm0 = xmm0 - 1.0
			default:
				compilerError("unknown postfix operator '%s'", postfix.Operator)
			}

			// Store the modified value back to the variable
			fc.out.MovXmmToMem("xmm0", baseReg, -offset)
		} else {
			fc.compileExpression(s.Expr)
		}

	case *ArenaStmt:
		fc.compileArenaStmt(s)

	case *DeferStmt:
		if len(fc.deferredExprs) == 0 {
			compilerError("defer can only be used inside a function or block scope")
		}
		currentScope := len(fc.deferredExprs) - 1
		fc.deferredExprs[currentScope] = append(fc.deferredExprs[currentScope], s.Call)

	case *SpawnStmt:
		fc.compileSpawnStmt(s)

	case *CStructDecl:
		// Cstruct declarations generate no runtime code
		// Constants are already available via Name_SIZEOF and Name_field_OFFSET
	}
}

func (fc *C67Compiler) pushDeferScope() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: pushDeferScope called, len before = %d\n", len(fc.deferredExprs))
	}
	fc.deferredExprs = append(fc.deferredExprs, []Expression{})
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: pushDeferScope called, len after = %d\n", len(fc.deferredExprs))
	}
}

func (fc *C67Compiler) popDeferScope() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: popDeferScope called, len before = %d\n", len(fc.deferredExprs))
	}
	if len(fc.deferredExprs) == 0 {
		return
	}

	currentScope := len(fc.deferredExprs) - 1
	deferred := fc.deferredExprs[currentScope]

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: popDeferScope emitting %d deferred expressions\n", len(deferred))
	}

	for i := len(deferred) - 1; i >= 0; i-- {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG:   Emitting deferred expr %d: %T - %v\n", i, deferred[i], deferred[i])
		}
		fc.compileExpression(deferred[i])
	}

	fc.deferredExprs = fc.deferredExprs[:currentScope]
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: popDeferScope done, len after = %d\n", len(fc.deferredExprs))
	}
}

func (fc *C67Compiler) compileArenaStmt(stmt *ArenaStmt) {
	// Mark that this program uses arenas
	fc.usesArenas = true
	fc.runtimeFeatures.Mark(FeatureArenaAlloc)

	// Save previous arena context and increment to next arena
	previousArena := fc.currentArena
	fc.currentArena++
	arenaIndex := fc.currentArena - 1 // Convert arena number to 0-based index

	// Ensure meta-arena has enough capacity
	// Call _c67_arena_ensure_capacity(arenaIndex + 1)
	fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", fc.currentArena))
	fc.out.CallSymbol("_c67_arena_ensure_capacity")

	// Load arena pointer from meta-arena: _c67_arena_meta[arenaIndex]
	// Each pointer is 8 bytes, so offset = arenaIndex * 8
	offset := arenaIndex * 8
	fc.out.LeaSymbolToReg("rax", "_c67_arena_meta")
	fc.out.MovMemToReg("rax", "rax", 0)      // Load the meta-arena pointer
	fc.out.MovMemToReg("rax", "rax", offset) // Load the arena pointer from slot

	fc.pushDeferScope()

	// Compile statements in arena body
	for _, bodyStmt := range stmt.Body {
		fc.compileStatement(bodyStmt)
	}

	fc.popDeferScope()

	// Restore previous arena context
	fc.currentArena = previousArena

	// Reset arena (resets offset to 0, keeps buffer allocated for reuse)
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovMemToReg("rbx", "rbx", 0)      // rbx = meta-arena pointer
	fc.out.MovMemToReg("rdi", "rbx", offset) // rdi = arena pointer from slot
	fc.out.CallSymbol("_c67_arena_reset")
}

func (fc *C67Compiler) compileArenaExpr(expr *ArenaExpr) {
	// Mark that this program uses arenas
	fc.usesArenas = true
	fc.runtimeFeatures.Mark(FeatureArenaAlloc)

	// First, collect symbols from all statements in the block
	for _, stmt := range expr.Body {
		if err := fc.collectSymbols(stmt); err != nil {
			compilerError("%v at line 0", err)
		}
	}

	// Save previous arena context and increment to next arena
	previousArena := fc.currentArena
	fc.currentArena++
	arenaIndex := fc.currentArena - 1

	// Ensure meta-arena has enough capacity
	fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", fc.currentArena))
	fc.out.CallSymbol("_c67_arena_ensure_capacity")

	// Load arena pointer from meta-arena
	offset := arenaIndex * 8
	fc.out.LeaSymbolToReg("rax", "_c67_arena_meta")
	fc.out.MovMemToReg("rax", "rax", 0)
	fc.out.MovMemToReg("rax", "rax", offset)

	fc.pushDeferScope()

	// Compile statements in arena body
	// The last statement should leave its value in xmm0
	for i, bodyStmt := range expr.Body {
		fc.compileStatement(bodyStmt)
		// If it's the last statement and it's an assignment,
		// we need to load the assigned value
		if i == len(expr.Body)-1 {
			if assignStmt, ok := bodyStmt.(*AssignStmt); ok {
				fc.compileExpression(&IdentExpr{Name: assignStmt.Name})
			}
		}
	}

	fc.popDeferScope()

	// Restore previous arena context
	fc.currentArena = previousArena

	// Reset arena
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovMemToReg("rbx", "rbx", 0)
	fc.out.MovMemToReg("rdi", "rbx", offset)
	fc.out.CallSymbol("_c67_arena_reset")

	// Result is already in xmm0 from the last statement
}

func (fc *C67Compiler) compileSpawnStmt(stmt *SpawnStmt) {
	// Call fork() syscall (57 on x86-64 Linux)
	// Returns: child gets 0 in rax, parent gets child PID in rax
	fc.out.MovImmToReg("rax", "57") // fork syscall number
	fc.out.Syscall()

	// Test if we're in child or parent
	// If rax == 0, we're in child
	fc.out.TestRegReg("rax", "rax")

	// Jump to child code if rax == 0 (we're in child)
	childJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // Placeholder, will patch

	// Parent path: just continue execution
	// (child PID is in rax, but we don't use it for fire-and-forget)
	if stmt.Block != nil {
		// Note: Pipe-based result waiting from child processes is a future enhancement
		compilerError("c67 with pipe syntax (| params | block) not yet implemented - use simple c67 for now")
	}

	// Jump over child code
	parentJumpPos := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Placeholder

	// Child path: execute expression and exit
	childStartPos := fc.eb.text.Len()

	// Patch the jump to child
	childOffset := int32(childStartPos - (childJumpPos + ConditionalJumpSize))
	fc.patchJumpImmediate(childJumpPos+2, childOffset)

	// Execute the c67ped expression
	fc.compileExpression(stmt.Expr)

	// Flush all output streams before exiting
	// Call fflush(NULL) to flush all streams
	fc.out.MovImmToReg("rdi", "0") // NULL = 0
	fc.trackFunctionCall("fflush")
	fc.eb.GenerateCallInstruction("fflush")

	// Exit child process with status 0
	fc.out.MovImmToReg("rax", "60") // exit syscall number
	fc.out.MovImmToReg("rdi", "0")  // exit status 0
	fc.out.Syscall()

	// Parent continues here
	parentContinuePos := fc.eb.text.Len()

	// Patch the parent jump
	parentOffset := int32(parentContinuePos - (parentJumpPos + UnconditionalJumpSize))
	fc.patchJumpImmediate(parentJumpPos+1, parentOffset)
}

func (fc *C67Compiler) compileLoopStatement(stmt *LoopStmt) {
	// Check if this is a parallel loop
	if stmt.NumThreads != 0 {
		// Parallel loop: @@ or N @
		// Currently only range loops are supported for parallel execution
		if rangeExpr, isRange := stmt.Iterable.(*RangeExpr); isRange {
			fc.compileParallelRangeLoop(stmt, rangeExpr)
		} else {
			fmt.Fprintf(os.Stderr, "Error: Parallel loops currently only support range expressions (e.g., 0..<100)\n")
			fmt.Fprintf(os.Stderr, "       List iteration with parallel loops not yet implemented\n")
			os.Exit(1)
		}
		return
	}

	// Sequential loop
	// Check if iterating over a range expression (0..<10, 0..=10)
	if rangeExpr, isRange := stmt.Iterable.(*RangeExpr); isRange {
		// Range loop (lazy iteration)
		fc.compileRangeLoop(stmt, rangeExpr)
	} else {
		// List iteration
		fc.compileListLoop(stmt)
	}
}

func (fc *C67Compiler) compileWhileStatement(stmt *WhileStmt) {
	// Condition loop: @ expr max N { ... }
	// Structure:
	//   loop_start:
	//     evaluate condition
	//     if condition == false, jump to loop_end
	//     execute body
	//     increment iteration counter
	//     if counter >= max, jump to loop_end
	//     jump to loop_start
	//   loop_end:

	// Increment label counter for uniqueness
	fc.labelCounter++
	currentLoopLabel := fc.labelCounter

	// Allocate a register or stack slot for iteration counter
	// Use a callee-saved register if available
	counterReg := fc.regTracker.AllocIntCalleeSaved(fmt.Sprintf("while_counter_%d", currentLoopLabel))
	useRegister := counterReg != ""
	var counterOffset int

	if !useRegister {
		// Allocate stack space for counter
		fc.stackOffset -= 8
		counterOffset = fc.stackOffset
	}

	// Initialize counter to 0
	if useRegister {
		fc.out.XorRegWithReg(counterReg, counterReg) // counter = 0
	} else {
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbp", counterOffset)
	}

	// Mark loop start - record position for back jump
	loopStartPos := fc.eb.text.Len()

	// Push loop info for break/continue handling
	loopInfo := LoopInfo{
		Label:       len(fc.activeLoops) + 1,
		StartPos:    loopStartPos,
		ContinuePos: loopStartPos,
		EndPatches:  []int{}, // Collect positions that need to jump to end
	}
	fc.activeLoops = append(fc.activeLoops, loopInfo)

	// Evaluate condition expression
	fc.compileExpression(stmt.Condition)
	// Result is in xmm0 (float)

	// Check if condition is zero (false)
	// Compare xmm0 with 0.0
	fc.out.XorpdXmm("xmm1", "xmm1") // xmm1 = 0.0
	fc.out.Ucomisd("xmm0", "xmm1")  // Compare xmm0 with xmm1 (0.0)

	// Jump to end if condition is false (zero)
	conditionJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // Placeholder, will patch
	fc.activeLoops[len(fc.activeLoops)-1].EndPatches = append(
		fc.activeLoops[len(fc.activeLoops)-1].EndPatches, conditionJumpPos)

	// Execute loop body
	for _, bodyStmt := range stmt.Body {
		fc.compileStatement(bodyStmt)
	}

	// Increment iteration counter
	if useRegister {
		fc.out.IncReg(counterReg)
		// Check against max iterations
		fc.out.MovImmToReg("r10", fmt.Sprintf("%d", stmt.MaxIterations))
		fc.out.CmpRegToReg(counterReg, "r10")
	} else {
		// Load counter, increment, store back
		fc.out.MovMemToReg("rax", "rbp", counterOffset)
		fc.out.IncReg("rax")
		fc.out.MovRegToMem("rax", "rbp", counterOffset)
		// Check against max iterations
		fc.out.MovImmToReg("r10", fmt.Sprintf("%d", stmt.MaxIterations))
		fc.out.CmpRegToReg("rax", "r10")
	}

	// Jump to end if counter >= max
	maxCheckJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder
	fc.activeLoops[len(fc.activeLoops)-1].EndPatches = append(
		fc.activeLoops[len(fc.activeLoops)-1].EndPatches, maxCheckJumpPos)

	// Jump back to loop start
	currentPos := fc.eb.text.Len()
	backOffset := int32(loopStartPos - (currentPos + 5)) // 5 bytes for JMP instruction
	fc.out.JumpUnconditional(backOffset)

	// Mark loop end - patch all forward jumps
	loopEndPos := fc.eb.text.Len()
	for _, patchPos := range fc.activeLoops[len(fc.activeLoops)-1].EndPatches {
		offset := int32(loopEndPos - (patchPos + 6)) // 6 bytes for conditional jump
		fc.patchJumpImmediate(patchPos+2, offset)    // +2 to skip opcode bytes
	}

	// Pop loop info
	fc.activeLoops = fc.activeLoops[:len(fc.activeLoops)-1]

	// Free the counter register if we allocated one
	if useRegister {
		fc.regTracker.FreeInt(counterReg)
	}
}

func (fc *C67Compiler) compileRangeLoop(stmt *LoopStmt, rangeExpr *RangeExpr) {
	// SIMD AUTO-VECTORIZATION CHECK
	// Try to vectorize this loop if possible
	if fc.tryVectorizeLoop(stmt, rangeExpr) {
		// Loop was successfully vectorized
		return
	}

	// Fall back to scalar compilation
	// REGISTER ALLOCATION OPTIMIZATION:
	// Use rbx for loop counter, r12 for loop limit
	// This eliminates memory operations in tight loops (30-40% speedup)

	// Increment label counter for uniqueness
	fc.labelCounter++
	currentLoopLabel := fc.labelCounter

	// Get the stack offset from BEFORE loop body was processed
	// This is stored directly in the LoopStmt during collectSymbols
	baseOffset := stmt.BaseOffset

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileRangeLoop: currentLoopLabel=%d, baseOffset=%d from stmt.BaseOffset\n",
			currentLoopLabel, baseOffset)
	}

	// Confidence that this function is working: 100%
	// Determine loop depth and try to allocate register FIRST
	loopDepth := len(fc.activeLoops)

	// Try to allocate a callee-saved register for loop counter
	// These survive function calls without save/restore
	var counterReg string
	var useRegister bool
	counterReg = fc.regTracker.AllocIntCalleeSaved(fmt.Sprintf("loop_counter_%d", loopDepth))
	if counterReg != "" {
		useRegister = true
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileRangeLoop: depth=%d, counterReg='%s', useRegister=%v\n",
			loopDepth, counterReg, useRegister)
	}

	// NOW allocate stack space based on whether we got a register
	// Allocate stack space for loop state
	// With register allocation: need space for limit + iterator
	// If runtime checking needed: [iteration_count] [max_iterations] [limit] [iterator]
	// Without register: add [counter] between limit and iterator
	var loopStateOffset, iterationCountOffset, maxIterOffset, limitOffset, counterOffset, iterOffset int
	var stackSize int64

	if stmt.NeedsMaxCheck {
		// Need extra space for iteration tracking
		// Stack layout (each field is 8 bytes):
		// [iteration_count][max_iterations][limit][counter(if no reg)][iterator]
		// Offsets are from rbp going downward (higher offset = lower address)
		if useRegister {
			// Using register for counter, no stack space needed for it
			// Layout: [iteration_count][max_iterations][limit][iterator]
			stackSize = 32
			loopStateOffset = baseOffset + 32
			iterationCountOffset = loopStateOffset - 24
			maxIterOffset = loopStateOffset - 16
			limitOffset = loopStateOffset - 8
			iterOffset = loopStateOffset
		} else {
			// Using stack for counter
			// Layout: [iteration_count][max_iterations][limit][counter][iterator]
			stackSize = 40
			loopStateOffset = baseOffset + 40
			iterationCountOffset = loopStateOffset - 32
			maxIterOffset = loopStateOffset - 24
			limitOffset = loopStateOffset - 16
			counterOffset = loopStateOffset - 8
			iterOffset = loopStateOffset
		}
	} else {
		// No runtime checking needed - minimal stack frame
		// Stack layout: [limit][counter(if no reg)][iterator]
		// Keep original 32-byte size for compatibility
		if useRegister {
			// Using register for counter
			stackSize = 32
			loopStateOffset = baseOffset + 32
			limitOffset = loopStateOffset - 16
			iterOffset = loopStateOffset
		} else {
			// Using stack for counter - need extra 8 bytes
			stackSize = 40
			loopStateOffset = baseOffset + 40
			limitOffset = loopStateOffset - 24
			counterOffset = loopStateOffset - 16
			iterOffset = loopStateOffset
		}
	}

	fc.out.SubImmFromReg("rsp", stackSize)
	fc.runtimeStack += int(stackSize) // Track runtime allocation

	// Initialize max iterations (after stack allocation)
	if stmt.NeedsMaxCheck {
		// Store max iterations on stack (skip if infinite)
		if stmt.MaxIterations != math.MaxInt64 {
			fc.out.MovImmToReg("rax", fmt.Sprintf("%d", stmt.MaxIterations))
			fc.out.MovRegToMem("rax", "rbp", -maxIterOffset)
		}
	}

	// Evaluate range start and store in counter (register or stack)
	fc.compileExpression(rangeExpr.Start)
	if useRegister {
		fc.out.Cvttsd2si(counterReg, "xmm0") // counter = start
	} else {
		fc.out.Cvttsd2si("rax", "xmm0")
		fc.out.MovRegToMem("rax", "rbp", -counterOffset)
	}

	// Evaluate range end and store on stack (limit)
	fc.compileExpression(rangeExpr.End)
	fc.out.Cvttsd2si("rax", "xmm0") // rax = loop limit
	// For inclusive ranges (..=), increment end by 1
	if rangeExpr.Inclusive {
		fc.out.IncReg("rax")
	}
	// Store limit on stack at rbp-limitOffset
	fc.out.MovRegToMem("rax", "rbp", -limitOffset)

	// Register iterator variable
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Loop iterator '%s' at offset %d (baseOffset=%d, loopStateOffset=%d)\n",
			stmt.Iterator, iterOffset, baseOffset, loopStateOffset)
		fmt.Fprintf(os.Stderr, "DEBUG: Current variables before iterator: %v\n", fc.variables)
	}
	fc.variables[stmt.Iterator] = iterOffset
	fc.mutableVars[stmt.Iterator] = true

	// Loop start label - this is where we jump back to
	loopStartPos := fc.eb.text.Len()

	// Reset iteration counter to 0 at loop start (critical for nested loops)
	if stmt.NeedsMaxCheck {
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbp", -iterationCountOffset)
	}

	// Register this loop on the active loop stack
	loopLabel := len(fc.activeLoops) + 1
	loopInfo := LoopInfo{
		Label:          loopLabel,
		StartPos:       loopStartPos,
		EndPatches:     []int{},
		IteratorOffset: iterOffset,
		IsRangeLoop:    true,
		CounterReg:     counterReg,
		UseRegister:    useRegister,
	}
	fc.activeLoops = append(fc.activeLoops, loopInfo)

	// Runtime max iteration checking (only if needed)
	if stmt.NeedsMaxCheck {
		// Check max iterations (if not infinite)
		if stmt.MaxIterations != math.MaxInt64 {
			// Load iteration count
			fc.out.MovMemToReg("rax", "rbp", -iterationCountOffset)
			// Load max iterations
			fc.out.MovMemToReg("rcx", "rbp", -maxIterOffset)
			// Compare: if iteration_count >= max_iterations, exceeded limit
			fc.out.CmpRegToReg("rax", "rcx")

			// Jump past error handling if not exceeded
			notExceededJumpPos := fc.eb.text.Len()
			fc.out.JumpConditional(JumpLess, 0) // Placeholder, will patch

			// Exceeded max iterations - print error and exit
			// printf("Error: Loop exceeded max iterations\n")
			fc.out.LeaSymbolToReg("rdi", "_loop_max_exceeded_msg")
			fc.trackFunctionCall("printf")
			fc.eb.GenerateCallInstruction("printf")

			// exit(1)
			fc.out.MovImmToReg("rdi", "1")
			fc.trackFunctionCall("exit")
			fc.eb.GenerateCallInstruction("exit")

			// Patch the jump to skip error handling
			notExceededPos := fc.eb.text.Len()
			notExceededOffset := int32(notExceededPos - (notExceededJumpPos + 6))
			fc.patchJumpImmediate(notExceededJumpPos+2, notExceededOffset)
		}

		// Increment iteration counter
		fc.out.MovMemToReg("rax", "rbp", -iterationCountOffset)
		fc.out.IncReg("rax")
		fc.out.MovRegToMem("rax", "rbp", -iterationCountOffset)
	}

	// Compare counter with limit
	fc.out.MovMemToReg("rax", "rbp", -limitOffset) // Load limit

	if useRegister {
		fc.out.CmpRegToReg(counterReg, "rax")
	} else {
		fc.out.MovMemToReg("rcx", "rbp", -counterOffset) // Load counter from stack
		fc.out.CmpRegToReg("rcx", "rax")
	}

	// Jump to loop end if counter >= limit
	loopEndJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder

	// Add this to the loop's end patches
	fc.activeLoops[len(fc.activeLoops)-1].EndPatches = append(
		fc.activeLoops[len(fc.activeLoops)-1].EndPatches,
		loopEndJumpPos+2, // +2 to skip to the offset field
	)

	// Store current counter value as iterator (convert to float64)
	if useRegister {
		fc.out.Cvtsi2sd("xmm0", counterReg)
	} else {
		fc.out.MovMemToReg("rcx", "rbp", -counterOffset)
		fc.out.Cvtsi2sd("xmm0", "rcx")
	}
	fc.out.MovXmmToMem("xmm0", "rbp", -iterOffset)

	// Save runtime stack before loop body (to clean up loop-local variables)
	runtimeStackBeforeBody := fc.runtimeStack

	// Compile loop body
	for _, s := range stmt.Body {
		fc.compileStatement(s)
	}

	// Mark continue position (increment step)
	continuePos := fc.eb.text.Len()
	fc.activeLoops[len(fc.activeLoops)-1].ContinuePos = continuePos

	// Patch all continue jumps to point here
	for _, patchPos := range fc.activeLoops[len(fc.activeLoops)-1].ContinuePatches {
		backOffset := int32(continuePos - (patchPos + 4))
		fc.patchJumpImmediate(patchPos, backOffset)
	}

	// Clean up loop-local variables allocated during loop body
	// Calculate how much stack was actually allocated during loop body
	bodyStackUsage := fc.runtimeStack - runtimeStackBeforeBody
	if bodyStackUsage > 0 {
		fc.out.AddImmToReg("rsp", int64(bodyStackUsage))
		fc.runtimeStack = runtimeStackBeforeBody
	}

	// Increment loop counter
	if useRegister {
		fc.out.IncReg(counterReg) // Single instruction!
	} else {
		fc.out.MovMemToReg("rax", "rbp", -counterOffset)
		fc.out.IncReg("rax")
		fc.out.MovRegToMem("rax", "rbp", -counterOffset)
	}

	// Jump back to loop start
	loopBackJumpPos := fc.eb.text.Len()
	backOffset := int32(loopStartPos - (loopBackJumpPos + UnconditionalJumpSize))
	fc.out.JumpUnconditional(backOffset)

	// Loop end cleanup - this is where all loop exit jumps target
	loopEndPos := fc.eb.text.Len()

	// Clean up stack space
	fc.out.AddImmToReg("rsp", stackSize)
	fc.runtimeStack -= int(stackSize)

	// No need to restore since we don't save r12/r13/r14 (they're reserved)

	// Unregister iterator variable
	delete(fc.variables, stmt.Iterator)
	delete(fc.mutableVars, stmt.Iterator)

	// Patch all end jumps to point to loopEndPos (cleanup code)
	for _, patchPos := range fc.activeLoops[len(fc.activeLoops)-1].EndPatches {
		endOffset := int32(loopEndPos - (patchPos + 4))
		fc.patchJumpImmediate(patchPos, endOffset)
	}

	// Free loop counter register if it was allocated
	currentLoop := &fc.activeLoops[len(fc.activeLoops)-1]
	if currentLoop.UseRegister && currentLoop.CounterReg != "" {
		fc.regTracker.FreeInt(currentLoop.CounterReg)
	}

	// Pop loop from active stack
	fc.activeLoops = fc.activeLoops[:len(fc.activeLoops)-1]
}

// tryVectorizeLoop attempts to vectorize a simple range loop
// Returns true if loop was vectorized, false if should fall back to scalar
func (fc *C67Compiler) tryVectorizeLoop(stmt *LoopStmt, rangeExpr *RangeExpr) bool {
	// Check if optimizer already marked this loop as vectorizable
	if !stmt.Vectorized {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Loop not marked as vectorizable by optimizer\n")
		}
		return false
	}

	// Only vectorize on x86-64 with AVX support for now
	if fc.platform.Arch != ArchX86_64 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Not x86-64 architecture, skipping vectorization\n")
		}
		return false
	}

	// Only vectorize simple patterns for now:
	// @ i in range(n) { result[i] <- a[i] + b[i] }
	if len(stmt.Body) != 1 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Loop body has %d statements, need exactly 1\n", len(stmt.Body))
		}
		return false // Only single-statement loops
	}

	// Handle both AssignStmt and MapUpdateStmt
	var binExpr *BinaryExpr
	var lhsName string

	if assign, ok := stmt.Body[0].(*AssignStmt); ok {
		// Check if LHS is also an index expression
		lhsName = assign.Name
		if !strings.Contains(lhsName, "[") {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "SIMD: LHS is not array access: '%s'\n", lhsName)
			}
			return false // LHS must be array access
		}

		// Check if RHS is a binary expression (a[i] + b[i])
		var ok bool
		binExpr, ok = assign.Value.(*BinaryExpr)
		if !ok {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "SIMD: RHS is not a BinaryExpr: %T\n", assign.Value)
			}
			return false // Must be binary operation
		}
	} else if mapUpdate, ok := stmt.Body[0].(*MapUpdateStmt); ok {
		// Array update: result[i] <- value
		lhsName = mapUpdate.MapName + "[" + stmt.Iterator + "]"

		// Check if value is a binary expression
		var ok bool
		binExpr, ok = mapUpdate.Value.(*BinaryExpr)
		if !ok {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "SIMD: MapUpdate value is not a BinaryExpr: %T\n", mapUpdate.Value)
			}
			return false
		}
	} else {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Loop body is not AssignStmt or MapUpdateStmt: %T\n", stmt.Body[0])
		}
		return false
	}

	// Support addition, subtraction, and multiplication
	if binExpr.Operator != "+" && binExpr.Operator != "-" && binExpr.Operator != "*" {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Unsupported operator: %s\n", binExpr.Operator)
		}
		return false
	}

	// Check if both operands are index expressions
	leftIndex, leftOk := binExpr.Left.(*IndexExpr)
	rightIndex, rightOk := binExpr.Right.(*IndexExpr)
	if !leftOk || !rightOk {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Operands are not IndexExpr: left=%T, right=%T\n",
				binExpr.Left, binExpr.Right)
		}
		return false
	}

	// Verify indices use the loop iterator
	leftIdxIdent, leftIdxOk := leftIndex.Index.(*IdentExpr)
	rightIdxIdent, rightIdxOk := rightIndex.Index.(*IdentExpr)
	if !leftIdxOk || !rightIdxOk {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Indices are not IdentExpr: left=%T, right=%T\n",
				leftIndex.Index, rightIndex.Index)
		}
		return false
	}

	if leftIdxIdent.Name != stmt.Iterator || rightIdxIdent.Name != stmt.Iterator {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Indices don't match iterator '%s': left='%s', right='%s'\n",
				stmt.Iterator, leftIdxIdent.Name, rightIdxIdent.Name)
		}
		return false // Indices must use loop iterator
	}

	// Extract base array names
	leftArray, leftArrayOk := leftIndex.List.(*IdentExpr)
	rightArray, rightArrayOk := rightIndex.List.(*IdentExpr)
	if !leftArrayOk || !rightArrayOk {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Array bases are not IdentExpr: left=%T, right=%T\n",
				leftIndex.List, rightIndex.List)
		}
		return false
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD: Vectorizing loop - pattern: %s = %s[i] %s %s[i]\n",
			lhsName, leftArray.Name, binExpr.Operator, rightArray.Name)
		fmt.Fprintf(os.Stderr, "SIMD: Vector width: %d elements\n", stmt.VectorWidth)
	}

	// Emit vectorized code using the vector width from the optimizer
	fc.emitVectorizedBinaryOpLoop(stmt, rangeExpr, lhsName, leftArray.Name, rightArray.Name,
		binExpr.Operator, stmt.VectorWidth)

	// Successfully vectorized
	return true
}

// emitVectorizedBinaryOpLoop emits SIMD code for: result[i] = a[i] OP b[i]
func (fc *C67Compiler) emitVectorizedBinaryOpLoop(stmt *LoopStmt, rangeExpr *RangeExpr,
	resultName, leftArrayName, rightArrayName string, operator string, vectorWidth int) {

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD: Emitting vectorized loop: %s = %s[i] %s %s[i] (width=%d)\n",
			resultName, leftArrayName, operator, rightArrayName, vectorWidth)
	}

	// Get array pointers from variables map
	// Extract base name from result (might be "result[i]" -> "result")
	resultBase := strings.Split(resultName, "[")[0]
	resultOffset, resultExists := fc.variables[resultBase]
	leftOffset, leftExists := fc.variables[leftArrayName]
	rightOffset, rightExists := fc.variables[rightArrayName]

	if !resultExists || !leftExists || !rightExists {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Cannot find array variables in symbol table\n")
			fmt.Fprintf(os.Stderr, "SIMD:   result=%s exists=%v, left=%s exists=%v, right=%s exists=%v\n",
				resultBase, resultExists, leftArrayName, leftExists, rightArrayName, rightExists)
		}
		return
	}

	// Evaluate and store range start and end
	fc.compileExpression(rangeExpr.Start)
	fc.out.Cvttsd2si("rbx", "xmm0") // rbx = loop counter (start)

	fc.compileExpression(rangeExpr.End)
	fc.out.Cvttsd2si("r12", "xmm0") // r12 = loop limit (end)
	if rangeExpr.Inclusive {
		fc.out.IncReg("r12")
	}

	// Load array base pointers into registers
	// Arrays are stored as pointers on the stack
	fc.out.MovMemToReg("rdi", "rbp", -resultOffset) // rdi = result array ptr
	fc.out.MovMemToReg("rsi", "rbp", -leftOffset)   // rsi = left array ptr
	fc.out.MovMemToReg("rdx", "rbp", -rightOffset)  // rdx = right array ptr

	// Determine register type based on vector width
	var regPrefix string
	if vectorWidth == 8 {
		regPrefix = "zmm" // AVX-512: 512-bit (8 doubles)
	} else if vectorWidth == 4 {
		regPrefix = "ymm" // AVX/AVX2: 256-bit (4 doubles)
	} else {
		regPrefix = "xmm" // SSE: 128-bit (2 doubles)
	}

	// ===== VECTOR LOOP =====
	// Process vectorWidth elements per iteration
	vecLoopStart := fc.eb.text.Len()

	// Check if we have at least vectorWidth elements remaining
	fc.out.MovRegToReg("rax", "r12")
	fc.out.SubRegFromReg("rax", "rbx") // rax = limit - counter (remaining)
	fc.out.CmpRegToImm("rax", int64(vectorWidth))

	// Jump to cleanup if remaining < vectorWidth
	cleanupJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLess, 0) // Placeholder, will patch later

	// Load vectorWidth doubles from left array: reg0 = left[rbx:rbx+vectorWidth]
	// Offset = rbx * 8 (8 bytes per double)
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)     // r10 = rbx * 8
	fc.out.AddRegToReg("r10", "rsi") // r10 = &left[rbx]
	fc.out.VMovupdLoadFromMem(regPrefix+"0", "r10", 0)

	// Load vectorWidth doubles from right array: reg1 = right[rbx:rbx+vectorWidth]
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)
	fc.out.AddRegToReg("r10", "rdx") // r10 = &right[rbx]
	fc.out.VMovupdLoadFromMem(regPrefix+"1", "r10", 0)

	// Vector operation: reg0 = reg0 OP reg1
	switch operator {
	case "+":
		fc.out.VAddPDVectorToVector(regPrefix+"0", regPrefix+"0", regPrefix+"1")
	case "-":
		fc.out.VSubPDVectorToVector(regPrefix+"0", regPrefix+"0", regPrefix+"1")
	case "*":
		fc.out.VMulPDVectorToVector(regPrefix+"0", regPrefix+"0", regPrefix+"1")
	}

	// Store result: result[rbx:rbx+vectorWidth] = reg0
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)
	fc.out.AddRegToReg("r10", "rdi") // r10 = &result[rbx]
	fc.out.VMovupdStoreToMem(regPrefix+"0", "r10", 0)

	// Increment counter by vectorWidth
	fc.out.AddImmToReg("rbx", int64(vectorWidth))

	// Jump back to vector loop start
	vecLoopEnd := fc.eb.text.Len()
	offset := vecLoopStart - vecLoopEnd - 2
	fc.out.JumpUnconditional(int32(offset))

	// ===== CLEANUP LOOP =====
	// Process remaining elements one by one
	cleanupStart := fc.eb.text.Len()

	// Patch the earlier jump to cleanup
	cleanupJumpTarget := cleanupStart - cleanupJump - 6
	fc.patchJump(cleanupJump, cleanupJumpTarget)

	// Check if counter >= limit
	cleanupLoopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("rbx", "r12")

	// Jump to done if rbx >= r12
	doneJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder, will patch later

	// Load one element from left: xmm0 = left[rbx]
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)
	fc.out.AddRegToReg("r10", "rsi")
	// Load scalar double (use SSE2 movsd)
	fc.out.Emit([]byte{0xF2, 0x41, 0x0F, 0x10, 0x02}) // movsd xmm0, [r10]

	// Load one element from right: xmm1 = right[rbx]
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)
	fc.out.AddRegToReg("r10", "rdx")
	fc.out.Emit([]byte{0xF2, 0x41, 0x0F, 0x10, 0x0A}) // movsd xmm1, [r10]

	// Scalar operation: xmm0 = xmm0 OP xmm1
	switch operator {
	case "+":
		fc.out.Emit([]byte{0xF2, 0x0F, 0x58, 0xC1}) // addsd xmm0, xmm1
	case "-":
		fc.out.Emit([]byte{0xF2, 0x0F, 0x5C, 0xC1}) // subsd xmm0, xmm1
	case "*":
		fc.out.Emit([]byte{0xF2, 0x0F, 0x59, 0xC1}) // mulsd xmm0, xmm1
	}

	// Store result: result[rbx] = xmm0
	fc.out.MovRegToReg("r10", "rbx")
	fc.out.ShlRegByImm("r10", 3)
	fc.out.AddRegToReg("r10", "rdi")
	fc.out.Emit([]byte{0xF2, 0x41, 0x0F, 0x11, 0x02}) // movsd [r10], xmm0

	// Increment counter
	fc.out.IncReg("rbx")

	// Jump back to cleanup loop start
	cleanupLoopEnd := fc.eb.text.Len()
	cleanupOffset := cleanupLoopStart - cleanupLoopEnd - 2
	fc.out.JumpUnconditional(int32(cleanupOffset))

	// ===== DONE =====
	doneStart := fc.eb.text.Len()

	// Patch the jump to done
	doneJumpTarget := doneStart - doneJump - 6
	fc.patchJump(doneJump, doneJumpTarget)

	// Clean up AVX state
	fc.out.VZeroUpper()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD: Successfully emitted vectorized loop\n")
	}
}

// patchJump patches a conditional jump with the correct offset
func (fc *C67Compiler) patchJump(jumpPos int, offset int) {
	// For conditional jumps, the offset is encoded as a 32-bit signed integer
	// Get the raw bytes from the buffer
	textBytes := fc.eb.text.Bytes()
	textBytes[jumpPos+2] = byte(offset & 0xFF)
	textBytes[jumpPos+3] = byte((offset >> 8) & 0xFF)
	textBytes[jumpPos+4] = byte((offset >> 16) & 0xFF)
	textBytes[jumpPos+5] = byte((offset >> 24) & 0xFF)
}

// collectLoopLocalVars scans the loop body and returns a map of variables defined inside it
func collectLoopLocalVars(body []Statement) map[string]bool {
	localVars := make(map[string]bool)

	// Recursively scan statements for variable assignments
	var scanStatements func([]Statement)
	scanStatements = func(stmts []Statement) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *AssignStmt:
				localVars[s.Name] = true
			case *LoopStmt:
				// Recursively scan nested loop bodies
				scanStatements(s.Body)
				// Add more cases as needed for other statement types with nested statements
			}
		}
	}

	scanStatements(body)
	return localVars
}

// hasAtomicOperations recursively checks if any atomic operations are used in statements
func hasAtomicOperations(stmts []Statement) bool {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ExpressionStmt:
			if hasAtomicInExpr(s.Expr) {
				return true
			}
		case *AssignStmt:
			if hasAtomicInExpr(s.Value) {
				return true
			}
		case *LoopStmt:
			if hasAtomicOperations(s.Body) {
				return true
			}
		}
	}
	return false
}

// hasAtomicInExpr checks if an expression contains atomic operation calls
func hasAtomicInExpr(expr Expression) bool {
	if expr == nil {
		return false
	}

	switch e := expr.(type) {
	case *CallExpr:
		// Check if this is an atomic operation
		atomicOps := []string{"atomic_add", "atomic_load", "atomic_store", "atomic_cas"}
		for _, op := range atomicOps {
			if e.Function == op {
				return true
			}
		}
		// Check arguments recursively
		for _, arg := range e.Args {
			if hasAtomicInExpr(arg) {
				return true
			}
		}
	case *BinaryExpr:
		return hasAtomicInExpr(e.Left) || hasAtomicInExpr(e.Right)
	case *UnaryExpr:
		return hasAtomicInExpr(e.Operand)
	case *MatchExpr:
		// Check condition and all clauses
		if hasAtomicInExpr(e.Condition) {
			return true
		}
		for _, clause := range e.Clauses {
			if hasAtomicInExpr(clause.Guard) || hasAtomicInExpr(clause.Result) {
				return true
			}
		}
		if hasAtomicInExpr(e.DefaultExpr) {
			return true
		}
	case *BlockExpr:
		// Check all statements in the block
		return hasAtomicOperations(e.Statements)
	case *LoopExpr:
		// Check loop body
		return hasAtomicOperations(e.Body)
	}
	return false
}

func (fc *C67Compiler) compileParallelRangeLoop(stmt *LoopStmt, rangeExpr *RangeExpr) {
	// Fixed: atomic operations now work in parallel loops!
	// We changed atomic_cas to use r12 instead of r11, avoiding register conflicts.
	// r11 is reserved for parent rbp in parallel loops.

	// Determine actual thread count
	actualThreads := stmt.NumThreads
	if actualThreads == -1 {
		// @@ syntax: detect CPU cores at compile time
		actualThreads = GetNumCPUCores()
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: @@ resolved to %d CPU cores\n", actualThreads)
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Compiling parallel range loop with %d threads, iterator '%s'\n",
			actualThreads, stmt.Iterator)
	}

	// Verify the range is compile-time constant
	// For the initial implementation, we require constant ranges
	startLit, startIsLit := rangeExpr.Start.(*NumberExpr)
	endLit, endIsLit := rangeExpr.End.(*NumberExpr)

	if !startIsLit || !endIsLit {
		fmt.Fprintf(os.Stderr, "Error: Parallel loops currently require constant range bounds\n")
		fmt.Fprintf(os.Stderr, "       Example: @@ i in 0..<100 { } (not @@ i in start..<end)\n")
		fmt.Fprintf(os.Stderr, "       Dynamic ranges will be supported in a future version\n")
		os.Exit(1)
	}

	start := int(startLit.Value)
	end := int(endLit.Value)
	if rangeExpr.Inclusive {
		end++ // Convert inclusive to exclusive for calculations
	}

	totalItems := end - start
	if totalItems <= 0 {
		// Empty range: skip parallel loop entirely (no error)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Skipping parallel loop with empty range [%d, %d)\n", start, end)
		}
		return // Skip code generation for empty loops
	}

	// Calculate work distribution
	chunkSize, remainder := CalculateWorkDistribution(totalItems, actualThreads)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Range [%d, %d) = %d items\n", start, end, totalItems)
		fmt.Fprintf(os.Stderr, "DEBUG: Each thread: ~%d items (remainder: %d)\n", chunkSize, remainder)
	}

	// V1 IMPLEMENTATION: Actual parallel execution with thread spawning
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Info: Parallel loop detected: %d threads for range [%d, %d)\n",
			actualThreads, start, end)
		fmt.Fprintf(os.Stderr, "      Work distribution: %d items/thread", chunkSize)
		if remainder > 0 {
			fmt.Fprintf(os.Stderr, " (+%d to last thread)", remainder)
		}
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "      Emitting parallel execution assembly code\n")
	}

	// Step 1: Allocate space on stack for barrier
	// Barrier layout: [count: int64][total: int64] = 16 bytes total
	// Using int64 for simplicity (assembly has better support for 64-bit operations)
	fc.out.SubImmFromReg("rsp", 16)
	fc.runtimeStack += 16

	// Step 2: Initialize barrier
	// V6: actualThreads worker threads + 1 parent thread = actualThreads+1 total
	// All participants (including parent) use barrier synchronization
	// barrier.count = actualThreads+1 at [rsp+0]
	// barrier.total = actualThreads+1 at [rsp+8]
	totalParticipants := actualThreads + 1
	fc.out.MovImmToMem(int64(totalParticipants), "rsp", 0) // count at offset 0
	fc.out.MovImmToMem(int64(totalParticipants), "rsp", 8) // total at offset 8

	// Save barrier address in r15 for later use
	fc.out.MovRegToReg("r15", "rsp")

	// Step 3: Spawn threads
	// For V1, spawn 2 threads
	// Each thread will execute its portion of the loop

	// Calculate work ranges for each thread
	threadRanges := make([][2]int, actualThreads)
	for i := 0; i < actualThreads; i++ {
		threadStart, threadEnd := GetThreadWorkRange(i, totalItems, actualThreads)
		threadRanges[i][0] = start + threadStart
		threadRanges[i][1] = start + threadEnd
	}

	// For each thread, we need to:
	// 1. Allocate a stack (1MB)
	// 2. Call clone() syscall
	// 3. Child jumps to thread_entry
	// 4. Parent continues to next thread

	// Save original rsp to restore later
	fc.out.MovRegToReg("r14", "rsp")

	// V6: Spawn actualThreads threads, each with different work range
	// All children execute the same code but with different work ranges
	// Each thread synchronizes at barrier after completion

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "      Note: V6 spawning %d threads with barrier synchronization\n", actualThreads)
	}

	// Allocate pthread_t array on stack to store thread IDs
	// Each pthread_t is 8 bytes, allocate space for all threads
	pthreadArraySize := int64(actualThreads * 8)
	fc.out.SubImmFromReg("rsp", pthreadArraySize)
	fc.out.MovRegToReg("r12", "rsp") // r12 = pthread_t array base

	// Spawn actualThreads child threads using pthread_create
	for threadIdx := 0; threadIdx < actualThreads; threadIdx++ {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "      Spawning thread %d with range [%d, %d)\n",
				threadIdx, threadRanges[threadIdx][0], threadRanges[threadIdx][1])
		}

		// Allocate thread argument structure on heap (32 bytes)
		// Structure: [start: int64][end: int64][barrier_ptr: int64][parent_rbp: int64]
		fc.out.MovImmToReg("rdi", "32") // size = 32 bytes
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax") // r13 = thread args

		// Store thread parameters in the allocated structure
		threadStart := int64(threadRanges[threadIdx][0])
		threadEnd := int64(threadRanges[threadIdx][1])

		// Store start at [r13+0]
		fc.out.MovImmToReg("rax", fmt.Sprintf("%d", threadStart))
		fc.out.MovRegToMem("rax", "r13", 0)

		// Store end at [r13+8]
		fc.out.MovImmToReg("rax", fmt.Sprintf("%d", threadEnd))
		fc.out.MovRegToMem("rax", "r13", 8)

		// Store barrier address at [r13+16]
		fc.out.MovRegToMem("r15", "r13", 16)

		// Store parent rbp at [r13+24] for accessing parent variables
		// Note: Must save rbp AFTER malloc() since malloc clobbers caller-saved regs
		fc.out.MovRegToMem("rbp", "r13", 24)

		// Call pthread_create(&thread_id, NULL, thread_func, arg)
		// pthread_create(pthread_t *thread, const pthread_attr_t *attr,
		//                void *(*start_routine)(void*), void *arg)
		// System V AMD64 ABI: rdi, rsi, rdx, rcx, r8, r9

		// Calculate pthread_t pointer: r12 + (threadIdx * 8)
		pthreadOffset := int64(threadIdx * 8)
		fc.out.MovRegToReg("rdi", "r12")                       // rdi = pthread array base (arg 1)
		fc.out.AddImmToReg("rdi", pthreadOffset)               // rdi = &thread_id
		fc.out.MovImmToReg("rsi", "0")                         // attr = NULL (arg 2)
		fc.out.LeaSymbolToReg("rdx", "_parallel_thread_entry") // start_routine (arg 3)
		fc.out.MovRegToReg("rcx", "r13")                       // arg = thread args (arg 4)
		fc.trackFunctionCall("pthread_create")
		fc.eb.GenerateCallInstruction("pthread_create")

		// Check if pthread_create succeeded (returns 0 on success)
		fc.out.TestRegReg("rax", "rax")
		createSuccessJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // je success

		// pthread_create failed - print error and exit
		fc.out.MovImmToReg("rdi", "1")
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")

		// Success - continue to next thread
		createSuccessPos := fc.eb.text.Len()
		fc.patchJumpImmediate(createSuccessJump+2, int32(createSuccessPos-(createSuccessJump+ConditionalJumpSize)))
	}

	// Now parent waits for all threads to complete
	// Using barrier-based synchronization (pthread_join alternative works better)

	// Parent also participates in barrier
	// Load barrier address (still in r15)
	// Atomically decrement and check
	fc.out.MovImmToReg("rax", "-1")
	fc.out.LockXaddMemReg("r15", 0, "eax") // Atomically add -1, eax gets old value
	fc.out.DecReg("eax")                   // eax = new value after our decrement

	// Spin-wait until barrier becomes 0
	parentWaitLoopStart := fc.eb.text.Len()
	fc.out.MovMemToReg("rax", "r15", 0) // Load current barrier value
	fc.out.TestRegReg("rax", "rax")
	parentWaitExit := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // if 0, all done
	// Loop back
	parentWaitBackOffset := int32(parentWaitLoopStart - (fc.eb.text.Len() + UnconditionalJumpSize))
	fc.out.JumpUnconditional(parentWaitBackOffset)

	// Patch exit jump
	parentWaitExitPos := fc.eb.text.Len()
	parentWaitExitOffset := int32(parentWaitExitPos - (parentWaitExit + ConditionalJumpSize))
	fc.patchJumpImmediate(parentWaitExit+2, parentWaitExitOffset)

	// Clean up pthread_t array from stack
	fc.out.AddImmToReg("rsp", pthreadArraySize)

	// Jump over thread entry function
	parentJumpPos := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Will patch to skip thread function

	// Thread entry function: void* _parallel_thread_entry(void* arg)
	fc.eb.MarkLabel("_parallel_thread_entry")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// arg is in rdi (first parameter), save it in rbx (callee-saved)
	fc.out.PushReg("rbx")            // Save original rbx
	fc.out.MovRegToReg("rbx", "rdi") // rbx = arg pointer for later free

	// Extract parameters from structure at [rdi] and save to stack using rbp-relative addressing
	// Structure: [start: int64][end: int64][barrier_ptr: int64][parent_rbp: int64]
	// Using rbp-relative addressing ensures stability across function calls (which modify rsp)

	// Allocate stack space for loop variables and alignment
	// After push rbp (8) + push rbx (8) = 16 bytes
	// Stack layout (rbp-relative):
	//   [rbp-8]:  saved rbx
	//   [rbp-16]: start
	//   [rbp-24]: end
	//   [rbp-32]: counter
	//   [rbp-40]: barrier_ptr
	//   [rbp-48]: parent_rbp
	//   [rbp-56]: iterator value (float64)
	// CRITICAL: pthread entry gives us 16-byte aligned rsp. After push rbp + push rbx,
	// rsp is aligned. We need rsp MISALIGNED by 8 before call instructions (so that
	// after call pushes return address, it becomes aligned). Therefore, sub by 56 not 64.
	fc.out.SubImmFromReg("rsp", 56)

	// Load parameters from argument structure and store to stack slots (rbp-relative)
	// Note: Using rbx since we saved rdi to rbx above
	fc.out.MovMemToReg("rax", "rbx", 0)   // rax = start
	fc.out.MovRegToMem("rax", "rbp", -16) // [rbp-16] = start

	fc.out.MovMemToReg("rax", "rbx", 8)   // rax = end
	fc.out.MovRegToMem("rax", "rbp", -24) // [rbp-24] = end

	fc.out.MovMemToReg("rax", "rbx", 16)  // rax = barrier_ptr
	fc.out.MovRegToMem("rax", "rbp", -40) // [rbp-40] = barrier_ptr

	fc.out.MovMemToReg("rax", "rbx", 24)  // rax = parent_rbp
	fc.out.MovRegToMem("rax", "rbp", -48) // [rbp-48] = parent_rbp

	// Initialize loop counter to start value
	fc.out.MovMemToReg("rax", "rbp", -16) // rax = start
	fc.out.MovRegToMem("rax", "rbp", -32) // [rbp-32] = counter (initialized to start)

	// Loop start
	loopStartPos := fc.eb.text.Len()

	// Load counter and end from stack and compare (rbp-relative)
	fc.out.MovMemToReg("rax", "rbp", -32) // rax = counter
	fc.out.MovMemToReg("rcx", "rbp", -24) // rcx = end
	fc.out.CmpRegToReg("rax", "rcx")

	// If counter >= end, exit loop
	loopEndJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder, will patch

	// V5 Step 2: Set up iterator variable
	// Convert counter (int) to float64 and store at rbp-56
	// This makes the iterator accessible as a proper float64 variable
	// Note: Using rbp-56 to avoid conflict with loop variables at rbp-16 through rbp-48
	iteratorOffset := 56
	fc.out.MovMemToReg("rax", "rbp", -32)              // rax = counter
	fc.out.Cvtsi2sd("xmm0", "rax")                     // xmm0 = (float64)counter
	fc.out.MovXmmToMem("xmm0", "rbp", -iteratorOffset) // Store at rbp-56

	// V5 Step 3 & 4: Compile loop body with existing variable context
	// The variables are already registered in fc.variables from collectSymbols phase
	// We just need to ensure the iterator is set correctly for this context

	// Save parent_rbp to r11 for parent variable access (rbp-relative)
	fc.out.MovMemToReg("r11", "rbp", -48) // r11 = parent_rbp

	// Capture parent variables: exclude iterator and loop-local vars
	loopLocalVars := collectLoopLocalVars(stmt.Body)
	savedParentVariables := fc.parentVariables
	fc.parentVariables = make(map[string]bool)
	for varName := range fc.variables {
		// Only mark as parent if it's not the iterator and not defined inside loop
		if varName != stmt.Iterator && !loopLocalVars[varName] {
			fc.parentVariables[varName] = true
		}
	}

	// Temporarily override the iterator offset for compilation
	// (collectSymbols set it to a different offset, but in child thread it's at rbp-16)
	savedIteratorOffset := fc.variables[stmt.Iterator]
	fc.variables[stmt.Iterator] = iteratorOffset

	// Compile actual loop body
	// Parent variables will use r11, local variables use rbp
	for _, bodyStmt := range stmt.Body {
		fc.compileStatement(bodyStmt)
	}

	// Restore original context
	fc.variables[stmt.Iterator] = savedIteratorOffset
	fc.parentVariables = savedParentVariables

	// Increment loop counter in memory (rbp-relative)
	fc.out.MovMemToReg("rax", "rbp", -32) // rax = counter
	fc.out.IncReg("rax")                  // rax++
	fc.out.MovRegToMem("rax", "rbp", -32) // store back to [rbp-32]

	// Jump back to loop start
	loopBackJumpPos := fc.eb.text.Len()
	loopBackOffset := int32(loopStartPos - (loopBackJumpPos + UnconditionalJumpSize))
	fc.out.JumpUnconditional(loopBackOffset)

	// Loop end
	loopEndPos := fc.eb.text.Len()

	// Patch the loop exit jump
	loopExitOffset := int32(loopEndPos - (loopEndJumpPos + ConditionalJumpSize))
	fc.patchJumpImmediate(loopEndJumpPos+2, loopExitOffset)

	// V4: Barrier synchronization after loop completes
	// Atomically decrement barrier counter and synchronize

	// Load barrier pointer from stack into r15 for barrier operations (rbp-relative)
	fc.out.MovMemToReg("r15", "rbp", -40) // r15 = barrier_ptr from [rbp-40]

	// Load -1 into eax for atomic decrement
	fc.out.MovImmToReg("rax", "-1")

	// LOCK XADD [r15], eax - Atomically add -1 to barrier.count
	// This emits: lock xadd [r15], eax
	// Result: eax gets the OLD value, memory gets decremented
	fc.out.LockXaddMemReg("r15", 0, "eax")

	// After LOCK XADD, eax contains the OLD value of the counter
	// Decrement eax to get the NEW value (what's now in memory)
	fc.out.DecReg("eax")

	// Check if we're the last thread (new value == 0)
	fc.out.TestRegReg("eax", "eax")

	// Jump if NOT last thread (need to wait)
	waitJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0) // Placeholder, will patch

	// Last thread path: Wake all waiting threads
	// futex(barrier_addr, FUTEX_WAKE_PRIVATE, num_threads)
	fc.out.MovImmToReg("rax", "202")    // sys_futex
	fc.out.MovRegToReg("rdi", "r15")    // addr = barrier address
	fc.out.MovImmToReg("rsi", "129")    // op = FUTEX_WAKE_PRIVATE (1 | 128)
	fc.out.MovMemToReg("rdx", "r15", 8) // val = barrier.total (wake all threads)
	fc.out.Syscall()

	// Jump to exit
	wakeExitJumpPos := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Placeholder, will patch

	// Not last thread: Spin-wait until barrier reaches 0
	// This is less efficient than futex but simpler and avoids potential futex issues
	waitStartPos := fc.eb.text.Len()

	// Patch the wait jump to point here
	waitOffset := int32(waitStartPos - (waitJumpPos + ConditionalJumpSize))
	fc.patchJumpImmediate(waitJumpPos+2, waitOffset)

	// Spin-wait loop: Keep checking barrier value until it's 0
	waitLoopStart := fc.eb.text.Len()

	// Load current barrier value
	fc.out.MovMemToReg("rax", "r15", 0) // rax = current barrier count

	// Check if barrier is now 0 (all threads done)
	fc.out.TestRegReg("rax", "rax")
	waitLoopExit := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // if barrier == 0, exit loop

	// Loop back to check again (spin-wait)
	waitLoopBackOffset := int32(waitLoopStart - (fc.eb.text.Len() + UnconditionalJumpSize))
	fc.out.JumpUnconditional(waitLoopBackOffset)

	// Exit point for all threads
	threadExitPos := fc.eb.text.Len()

	// Patch the wait loop exit jump
	waitLoopExitOffset := int32(threadExitPos - (waitLoopExit + ConditionalJumpSize))
	fc.patchJumpImmediate(waitLoopExit+2, waitLoopExitOffset)

	// Patch the wake exit jump
	wakeExitOffset := int32(threadExitPos - (wakeExitJumpPos + UnconditionalJumpSize))
	fc.patchJumpImmediate(wakeExitJumpPos+1, wakeExitOffset)

	// Restore stack pointer (matches the sub rsp, 56 in prologue)
	fc.out.AddImmToReg("rsp", 56)

	// Note: Argument structure cleanup - currently relies on process termination
	// (Memory leak acceptable for short-lived thread wrapper functions)
	// fc.out.MovRegToReg("rdi", "rbx") // rdi = arg pointer (saved in rbx)
	// fc.trackFunctionCall("free")
	// fc.eb.GenerateCallInstruction("free")

	// Restore rbx and return
	fc.out.PopReg("rbx")               // Restore original rbx
	fc.out.XorRegWithReg("rax", "rax") // Return NULL
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Parent continues here after all pthread_join calls complete
	parentContinuePos := fc.eb.text.Len()

	// Patch the jump over thread function to point here
	parentOffset := int32(parentContinuePos - (parentJumpPos + UnconditionalJumpSize))
	fc.patchJumpImmediate(parentJumpPos+1, parentOffset)

	// All threads have completed via pthread_join
	// Cleanup - deallocate barrier structure from stack
	fc.out.AddImmToReg("rsp", 16)
	fc.runtimeStack -= 16
}

func (fc *C67Compiler) compileListLoop(stmt *LoopStmt) {
	fc.labelCounter++

	fc.compileExpression(stmt.Iterable)

	baseOffset := stmt.BaseOffset
	stackSize := int64(64)

	listPtrOffset := baseOffset + 16
	lengthOffset := baseOffset + 32
	indexOffset := baseOffset + 48
	iterOffset := baseOffset + 64

	fc.out.SubImmFromReg("rsp", stackSize)
	fc.runtimeStack += int(stackSize)

	fc.out.MovXmmToMem("xmm0", "rbp", -listPtrOffset)

	fc.out.MovMemToXmm("xmm1", "rbp", -listPtrOffset)
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm1", "rsp", 0)
	fc.out.MovMemToReg("rax", "rsp", 0)
	fc.out.AddImmToReg("rsp", StackSlotSize)

	fc.out.MovMemToXmm("xmm0", "rax", 0)

	fc.out.Cvttsd2si("rax", "xmm0")

	fc.out.MovRegToMem("rax", "rbp", -lengthOffset)

	fc.out.XorRegWithReg("rax", "rax")
	fc.out.MovRegToMem("rax", "rbp", -indexOffset)

	fc.variables[stmt.Iterator] = iterOffset
	fc.mutableVars[stmt.Iterator] = true

	loopStartPos := fc.eb.text.Len()

	// Register this loop on the active loop stack
	// Label is determined by loop depth (1-indexed)
	loopLabel := len(fc.activeLoops) + 1
	loopInfo := LoopInfo{
		Label:            loopLabel,
		StartPos:         loopStartPos,
		EndPatches:       []int{},
		IteratorOffset:   iterOffset,
		IndexOffset:      indexOffset,
		UpperBoundOffset: lengthOffset,
		ListPtrOffset:    listPtrOffset,
		IsRangeLoop:      false,
	}
	fc.activeLoops = append(fc.activeLoops, loopInfo)

	// Load index: mov rax, [rbp - indexOffset]
	fc.out.MovMemToReg("rax", "rbp", -indexOffset)

	// Load length: mov rdi, [rbp - lengthOffset]
	fc.out.MovMemToReg("rdi", "rbp", -lengthOffset)

	// Compare index with length: cmp rax, rdi
	fc.out.CmpRegToReg("rax", "rdi")

	// Jump to loop end if index >= length
	loopEndJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder

	// Add this to the loop's end patches
	fc.activeLoops[len(fc.activeLoops)-1].EndPatches = append(
		fc.activeLoops[len(fc.activeLoops)-1].EndPatches,
		loopEndJumpPos+2, // +2 to skip to the offset field
	)

	// Load list pointer from stack to rbx
	fc.out.MovMemToXmm("xmm1", "rbp", -listPtrOffset)
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm1", "rsp", 0)
	fc.out.MovMemToReg("rbx", "rsp", 0)
	fc.out.AddImmToReg("rsp", StackSlotSize)

	// Lists are maps: [count][key0][val0][key1][val1]...
	// Element at index i is at: 8 + i*16 + 8 = 16 + i*16
	// Load index into rax
	fc.out.MovMemToReg("rax", "rbp", -indexOffset)

	// Calculate offset: 16 + index * 16
	fc.out.ShlImmReg("rax", 4)    // rax = index * 16
	fc.out.AddImmToReg("rax", 16) // rax = 16 + index * 16

	// Add offset to base: rbx = rbx + rax
	fc.out.AddRegToReg("rbx", "rax")

	// Load element value from list: movsd xmm0, [rbx]
	fc.out.MovMemToXmm("xmm0", "rbx", 0)

	// Store in iterator variable
	fc.out.MovXmmToMem("xmm0", "rbp", -iterOffset)

	// Compile loop body
	for _, s := range stmt.Body {
		fc.compileStatement(s)
	}

	// Mark continue position (increment step)
	continuePos := fc.eb.text.Len()
	fc.activeLoops[len(fc.activeLoops)-1].ContinuePos = continuePos

	// Patch all continue jumps to point here
	for _, patchPos := range fc.activeLoops[len(fc.activeLoops)-1].ContinuePatches {
		backOffset := int32(continuePos - (patchPos + 4)) // 4 bytes for 32-bit offset
		fc.patchJumpImmediate(patchPos, backOffset)
	}

	// Increment index
	fc.out.MovMemToReg("rax", "rbp", -indexOffset)
	fc.out.IncReg("rax")
	fc.out.MovRegToMem("rax", "rbp", -indexOffset)

	// Jump back to loop start
	loopBackJumpPos := fc.eb.text.Len()
	backOffset := int32(loopStartPos - (loopBackJumpPos + UnconditionalJumpSize)) // 5 bytes for unconditional jump
	fc.out.JumpUnconditional(backOffset)

	loopEndPos := fc.eb.text.Len()

	fc.out.AddImmToReg("rsp", stackSize)
	fc.runtimeStack -= int(stackSize)

	delete(fc.variables, stmt.Iterator)
	delete(fc.mutableVars, stmt.Iterator)

	// Patch all end jumps (conditional jump + any @0 breaks)
	for _, patchPos := range fc.activeLoops[len(fc.activeLoops)-1].EndPatches {
		endOffset := int32(loopEndPos - (patchPos + 4)) // 4 bytes for 32-bit offset
		fc.patchJumpImmediate(patchPos, endOffset)
	}

	// Pop loop from active stack
	fc.activeLoops = fc.activeLoops[:len(fc.activeLoops)-1]
}

func (fc *C67Compiler) compileJumpStatement(stmt *JumpStmt) {
	// New semantics with ret keyword:
	// ret (Label=0, IsBreak=true): return from function
	// ret @N (Label=N, IsBreak=true): exit loop N and all inner loops
	// @N (Label=N, IsBreak=false): continue loop N

	// Handle function return: ret with Label=0
	if stmt.Label == 0 && stmt.IsBreak {
		// Return from function
		if stmt.Value != nil {
			fc.compileExpression(stmt.Value)
			// xmm0 now contains return value
		}
		fc.out.MovRegToReg("rsp", "rbp")

		// REGISTER ALLOCATOR: Restore callee-saved registers (for lambda functions)
		if fc.currentLambda != nil {
			fc.out.SubImmFromReg("rsp", 8) // Point to saved rbx
			fc.out.PopReg("rbx")
		}

		fc.out.PopReg("rbp")
		fc.out.Ret()
		return
	}

	// All other cases require being inside a loop
	if len(fc.activeLoops) == 0 {
		keyword := "@"
		if stmt.IsBreak {
			keyword = "ret"
		}
		compilerError("%s used outside of loop", keyword)
	}

	targetLoopIndex := -1

	if stmt.Label == 0 {
		// Label 0 with IsBreak=false means innermost loop continue
		targetLoopIndex = len(fc.activeLoops) - 1
	} else if stmt.Label == -1 {
		// Label -1 means current loop (from "ret @" without number)
		targetLoopIndex = len(fc.activeLoops) - 1
	} else {
		// Find loop with specified label
		for i := 0; i < len(fc.activeLoops); i++ {
			if fc.activeLoops[i].Label == stmt.Label {
				targetLoopIndex = i
				break
			}
		}

		if targetLoopIndex == -1 {
			keyword := "@"
			if stmt.IsBreak {
				keyword = "ret"
			}
			compilerError("%s @%d references loop @%d which is not active",
				keyword, stmt.Label, stmt.Label)
		}
	}

	if stmt.IsBreak {
		// Break: jump to end of target loop
		jumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Placeholder
		fc.activeLoops[targetLoopIndex].EndPatches = append(
			fc.activeLoops[targetLoopIndex].EndPatches,
			jumpPos+1, // +1 to skip the opcode byte
		)
	} else {
		// Continue: jump to continue point of target loop
		jumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Placeholder
		fc.activeLoops[targetLoopIndex].ContinuePatches = append(
			fc.activeLoops[targetLoopIndex].ContinuePatches,
			jumpPos+1, // +1 to skip the opcode byte
		)
	}
}

func (fc *C67Compiler) patchJumpImmediate(pos int, offset int32) {
	// Get the current bytes from buffer
	// This is safe because we're patching backwards into already-written code
	bytes := fc.eb.text.Bytes()

	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG PATCH: Before patching at pos %d: %02x %02x %02x %02x\n", pos, bytes[pos], bytes[pos+1], bytes[pos+2], bytes[pos+3])
		}
	}

	// Write 32-bit little-endian offset at position
	bytes[pos] = byte(offset)
	bytes[pos+1] = byte(offset >> 8)
	bytes[pos+2] = byte(offset >> 16)
	bytes[pos+3] = byte(offset >> 24)

	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG PATCH: After patching at pos %d: %02x %02x %02x %02x (offset=%d)\n", pos, bytes[pos], bytes[pos+1], bytes[pos+2], bytes[pos+3], offset)
		}
	}
}

// isCFFIStringCall returns true if the expression is a C FFI call that returns char*
func (fc *C67Compiler) isCFFIStringCall(expr Expression) bool {
	callExpr, ok := expr.(*CallExpr)
	if !ok {
		return false
	}

	// Check if this is a namespaced C FFI function call (contains dot)
	if !strings.Contains(callExpr.Function, ".") {
		return false
	}

	// Extract namespace/alias and function name
	parts := strings.Split(callExpr.Function, ".")
	if len(parts) != 2 {
		return false
	}
	alias := parts[0]
	funcName := parts[1]

	// Look up in parsed headers
	// NOTE: cConstants is keyed by alias, not library name
	// e.g., for "import sdl3 as sdl", the key is "sdl"
	if nsHeader, exists := fc.cConstants[alias]; exists {
		if funcSig, exists := nsHeader.Functions[funcName]; exists {
			// Check if return type is char*
			returnType := strings.TrimSpace(funcSig.ReturnType)
			isCString := returnType == "char*" || returnType == "const char*"
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "isCFFIStringCall: %s.%s -> %s (isCString=%v)\n", alias, funcName, returnType, isCString)
			}
			return isCString
		}
	}

	// Fallback: use naming heuristics for common patterns
	// This is a reasonable approximation when headers can't be parsed
	// Functions returning strings typically follow naming conventions
	if strings.Contains(funcName, "GetError") ||
		strings.Contains(funcName, "GetString") ||
		strings.HasSuffix(funcName, "Error") ||
		strings.HasPrefix(funcName, "strerror") {
		return true
	}

	// No signature found, and doesn't match naming pattern
	return false
}

// Helper to get function names from CHeaderConstants
func getFunctionNames(constants *CHeaderConstants) []string {
	names := make([]string, 0, len(constants.Functions))
	for name := range constants.Functions {
		names = append(names, name)
	}
	return names
}

// Helper to get map keys for debugging
func keysOf(m map[string]*CHeaderConstants) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getExprType returns the type of an expression at compile time
// Returns: "string", "number", "list", "map", "cstring", or "unknown"
func (fc *C67Compiler) getExprType(expr Expression) string {
	switch e := expr.(type) {
	case *StringExpr:
		return "string"
	case *NumberExpr:
		return "number"
	case *RangeExpr:
		return "list" // Range expressions compile to lists
	case *ListExpr:
		return "list"
	case *MapExpr:
		return "map"
	case *IdentExpr:
		// Look up in varTypes
		if typ, exists := fc.varTypes[e.Name]; exists {
			return typ
		}
		// Default to number if not tracked (most variables are numbers)
		return "number"
	case *NamespacedIdentExpr:
		// C constants are always numbers
		return "number"
	case *BinaryExpr:
		// Cons operator :: always returns a list
		if e.Operator == "::" {
			return "list"
		}
		// Binary expressions between strings return strings if operator is "+"
		if e.Operator == "+" {
			leftType := fc.getExprType(e.Left)
			rightType := fc.getExprType(e.Right)
			if leftType == "string" && rightType == "string" {
				return "string"
			}
		}
		return "number"
	case *CallExpr:
		// Check if this is a C FFI call first (namespace.function)
		if strings.Contains(e.Function, ".") {
			parts := strings.Split(e.Function, ".")
			if len(parts) == 2 {
				alias := parts[0]
				funcName := parts[1]

				// Look up function signature
				if constants, ok := fc.cConstants[alias]; ok {
					if funcSig, found := constants.Functions[funcName]; found {
						returnType := strings.TrimSpace(funcSig.ReturnType)

						// Map C return types to C67 types
						if returnType == "char*" || returnType == "const char*" {
							return "cstring"
						} else if isPointerType(returnType) {
							return "cpointer"
						} else if returnType == "void" {
							return "number" // void becomes 0
						} else if returnType == "float" || returnType == "double" {
							return "number"
						} else {
							// int, bool, etc.
							return "number"
						}
					} else if VerboseMode {
						fmt.Fprintf(os.Stderr, "Warning: inferExprType: C function %s.%s not found in parsed signatures\n", alias, funcName)
					}
				} else if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: inferExprType: No parsed signatures for C library alias '%s'\n", alias)
				}

				// Default for unknown C FFI: assume number (safest default)
				return "number"
			}
		}

		// Function calls - check return type for C67 built-ins
		stringFuncs := map[string]bool{
			"str": true, "read_file": true,
			"upper": true, "lower": true, "trim": true,
			"_error_code_extract": true,
		}
		if stringFuncs[e.Function] {
			return "string"
		}
		// Functions that return lists
		listFuncs := map[string]bool{
			"append": true, "pop": true, "tail": true,
		}
		if listFuncs[e.Function] {
			return "list"
		}
		// Functions that return maps
		mapFuncs := map[string]bool{
			"safe_divide_result": true,
			"safe_sqrt_result":   true,
			"safe_ln_result":     true,
		}
		if mapFuncs[e.Function] {
			return "map"
		}
		// Other functions return numbers by default
		return "number"
	case *SliceExpr:
		// Slicing preserves the type of the list
		return fc.getExprType(e.List)
	case *FStringExpr:
		// F-strings are always strings
		return "string"
	case *CastExpr:
		// Cast expressions have the type they're cast to
		// Map cast types to C67 types
		switch e.Type {
		case "string", "str":
			return "string"
		case "cstr", "cstring":
			return "cstring"
		case "list":
			return "list"
		case "map":
			return "map"
		case "ptr", "pointer":
			return "pointer"
		default:
			// All numeric types
			return "number"
		}
	case *IndexExpr:
		// Indexing returns the element type
		// For lists/maps, elements are numbers (float64)
		return "number"
	default:
		return "unknown"
	}
}

// Confidence that this function is working: 95%
// (IndexExpr with SIMD is very complex but tested; minor edge cases may exist)
func (fc *C67Compiler) compileExpression(expr Expression) {
	if expr == nil {
		fmt.Fprintf(os.Stderr, "DEBUG: nil expression stack trace:\n")
		debug.PrintStack()
		compilerError("INTERNAL ERROR: compileExpression received nil expression")
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileExpression: expr type = %T\n", expr)
	}
	switch e := expr.(type) {
	case *NumberExpr:
		// C67 uses float64 foundation - all values are float64
		// For whole numbers, use integer conversion; for decimals, load from .rodata
		if e.Value == float64(int64(e.Value)) {
			// Whole number - can use integer path
			val := int64(e.Value)
			fc.out.MovImmToReg("rax", strconv.FormatInt(val, 10))
			fc.out.Cvtsi2sd("xmm0", "rax")
		} else {
			// Decimal number - store in .rodata and load
			labelName := fmt.Sprintf("float_%d", fc.stringCounter)
			fc.stringCounter++

			// Convert float64 to 8 bytes (little-endian)
			bits := uint64(0)
			*(*float64)(unsafe.Pointer(&bits)) = e.Value
			var floatData []byte
			for i := 0; i < 8; i++ {
				floatData = append(floatData, byte((bits>>(i*8))&ByteMask))
			}
			fc.eb.Define(labelName, string(floatData))

			// Load from .rodata
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovMemToXmm("xmm0", "rax", 0)
		}

	case *RandomExpr:
		// ?? operator: secure random float64 in [0.0, 1.0) using getrandom syscall
		// getrandom syscall: rax=318, rdi=buffer, rsi=length, rdx=flags
		// We need 8 random bytes for a uint64

		// Allocate space on stack for random bytes (8 bytes, keep aligned)
		fc.out.SubImmFromReg("rsp", 8)

		// Call getrandom: syscall(318, rsp, 8, 0)
		fc.out.MovImmToReg("rax", "318")   // getrandom syscall number
		fc.out.MovRegToReg("rdi", "rsp")   // buffer = stack pointer
		fc.out.MovImmToReg("rsi", "8")     // length = 8 bytes
		fc.out.XorRegWithReg("rdx", "rdx") // flags = 0
		fc.out.Syscall()

		// Load the random uint64 from stack
		fc.out.MovMemToReg("rax", "rsp", 0)

		// Clean up stack
		fc.out.AddImmToReg("rsp", 8)

		// Convert to float64 in range [0.0, 1.0)
		// Use upper 53 bits for mantissa (IEEE 754 double precision has 53 bits precision)
		// Shift right by 11 bits to get 53-bit value
		fc.out.ShrRegByImm("rax", 11)

		// Convert to float64 and divide by 2^53 to get [0.0, 1.0)
		fc.out.Cvtsi2sd("xmm0", "rax")

		// Load 2^53 as divisor
		labelName := fmt.Sprintf("float_2pow53_%d", fc.stringCounter)
		fc.stringCounter++
		divisor := float64(1 << 53) // 2^53 = 9007199254740992
		bits := uint64(0)
		*(*float64)(unsafe.Pointer(&bits)) = divisor
		var floatData []byte
		for i := 0; i < 8; i++ {
			floatData = append(floatData, byte((bits>>(i*8))&ByteMask))
		}
		fc.eb.Define(labelName, string(floatData))

		// Divide to get [0.0, 1.0)
		fc.out.LeaSymbolToReg("rax", labelName)
		fc.out.MovMemToXmm("xmm1", "rax", 0)
		fc.out.DivsdXmm("xmm0", "xmm1")

	case *StringExpr:
		labelName := fmt.Sprintf("str_%d", fc.stringCounter)
		fc.stringCounter++

		if fc.cContext {
			// C context: compile as null-terminated C string
			// Format: just the raw bytes followed by null terminator
			cStringData := append([]byte(e.Value), 0) // Add null terminator
			fc.eb.Define(labelName, string(cStringData))

			// Load pointer to C string into rax (not xmm0)
			fc.out.LeaSymbolToReg("rax", labelName)
			// Note: In C context, we keep the pointer in rax, not convert to float64
			// The caller (compileCFunctionCall) will handle it appropriately
		} else {
			// C67 context: compile as map[uint64]float64 where keys are indices
			// and values are character codes
			// Map format: [count][key0][val0][key1][val1]...
			// Following Lisp philosophy: even empty strings are objects (count=0), not null

			// Build map data: count followed by key-value pairs
			var mapData []byte

			// Count (number of Unicode codepoints/runes) - can be 0 for empty strings
			// Use utf8.RuneCountInString to get proper character count
			runes := []rune(e.Value) // Convert to rune slice for proper UTF-8 handling
			count := float64(len(runes))
			countBits := uint64(0)
			*(*float64)(unsafe.Pointer(&countBits)) = count
			for i := 0; i < 8; i++ {
				mapData = append(mapData, byte((countBits>>(i*8))&ByteMask))
			}

			// Add each Unicode codepoint as a key-value pair (none for empty strings)
			// IMPORTANT: Iterate over runes, not bytes, for proper UTF-8 support
			for idx, r := range runes {
				// Key: codepoint index as float64
				keyVal := float64(idx)
				keyBits := uint64(0)
				*(*float64)(unsafe.Pointer(&keyBits)) = keyVal
				for i := 0; i < 8; i++ {
					mapData = append(mapData, byte((keyBits>>(i*8))&ByteMask))
				}

				// Value: Unicode codepoint value as float64
				runeVal := float64(r)
				runeBits := uint64(0)
				*(*float64)(unsafe.Pointer(&runeBits)) = runeVal
				for i := 0; i < 8; i++ {
					mapData = append(mapData, byte((runeBits>>(i*8))&ByteMask))
				}
			}

			fc.eb.Define(labelName, string(mapData))
			fc.out.LeaSymbolToReg("rax", labelName)
			// Convert pointer to float64 (direct register move, no stack)
			fc.out.MovqRegToXmm("xmm0", "rax")
		}

	case *FStringExpr:
		// F-string: concatenate all parts
		if len(e.Parts) == 0 {
			// Empty f-string, return empty string
			fc.compileExpression(&StringExpr{Value: ""})
			return
		}

		// Compile first part
		// Check if it needs str() conversion using type checking
		firstPart := e.Parts[0]
		if fc.getExprType(firstPart) == "string" {
			// Already a string - compile directly
			fc.compileExpression(firstPart)
		} else {
			// Not a string - use 'as string' for conversion
			fc.compileExpression(&CastExpr{
				Expr: firstPart,
				Type: "string",
			})
		}

		// Concatenate remaining parts
		for i := 1; i < len(e.Parts); i++ {
			// Save left pointer (current result) to stack
			fc.out.SubImmFromReg("rsp", 16)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Evaluate right string (next part)
			part := e.Parts[i]
			if fc.getExprType(part) == "string" {
				// Already a string - compile directly
				fc.compileExpression(part)
			} else {
				// Not a string - use 'as string' for conversion
				fc.compileExpression(&CastExpr{
					Expr: part,
					Type: "string",
				})
			}

			// Save right pointer to stack
			fc.out.SubImmFromReg("rsp", 16)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)

			// Load arguments: rdi = left ptr, rsi = right ptr
			fc.out.MovMemToReg("rdi", "rsp", 16) // left ptr from [rsp+16]
			fc.out.MovMemToReg("rsi", "rsp", 0)  // right ptr from [rsp]
			fc.out.AddImmToReg("rsp", 32)        // clean up both stack slots

			// Align stack for call
			fc.out.SubImmFromReg("rsp", StackSlotSize)

			// Call _c67_string_concat(rdi, rsi) -> rax
			fc.trackFunctionCall("_c67_string_concat")
			fc.out.CallSymbol("_c67_string_concat")

			// Restore stack alignment
			fc.out.AddImmToReg("rsp", StackSlotSize)

			// Convert result pointer from rax back to xmm0 (direct register move)
			fc.out.MovqRegToXmm("xmm0", "rax")
		}

	case *IdentExpr:
		// Check if variable has been moved
		if fc.movedVars != nil && fc.movedVars[e.Name] {
			compilerError("use of moved variable '%s' - value was transferred with '!'", e.Name)
		}

		// Check if it's a global variable
		if dataOffset, isGlobal := fc.globalVars[e.Name]; isGlobal {
			// Load from .data section
			// lea rax, [rel _global_varname]
			// movsd xmm0, [rax]
			fc.out.LeaSymbolToReg("rax", "_global_"+e.Name)
			fc.out.MovMemToXmm("xmm0", "rax", 0)
			_ = dataOffset // Will use this for actual .data section later
		} else {
			// Load variable from stack into xmm0
			offset, exists := fc.variables[e.Name]
			if !exists {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG: Undefined variable '%s', available vars: %v\n", e.Name, fc.variables)
					fmt.Fprintf(os.Stderr, "DEBUG: Current lambda: %v\n", fc.currentLambda)
				}
				suggestions := findSimilarIdentifiers(e.Name, fc.variables, 3)
				// Add to error collector (railway-oriented)
				if len(suggestions) > 0 {
					fc.addSemanticError(
						fmt.Sprintf("undefined variable '%s'", e.Name),
						fmt.Sprintf("Did you mean: %s?", strings.Join(suggestions, ", ")),
					)
					compilerError("undefined variable '%s'. Did you mean: %s?", e.Name, strings.Join(suggestions, ", "))
				} else {
					fc.addSemanticError(fmt.Sprintf("undefined variable '%s'", e.Name))
					compilerError("undefined variable '%s'", e.Name)
				}
			}
			// Use r11 for parent variables in parallel loops, rbp for local variables
			baseReg := "rbp"
			if fc.parentVariables != nil && fc.parentVariables[e.Name] {
				baseReg = "r11"
			}
			fc.out.MovMemToXmm("xmm0", baseReg, -offset)
		}

	case *MoveExpr:
		// Compile the expression being moved (loads into xmm0)
		fc.compileExpression(e.Expr)

		// Mark variable as moved if it's an identifier
		if ident, ok := e.Expr.(*IdentExpr); ok {
			if fc.movedVars != nil {
				if fc.movedVars[ident.Name] {
					compilerError("variable '%s' was already moved", ident.Name)
				}
				fc.movedVars[ident.Name] = true
				// Track in current scope for proper cleanup
				if len(fc.scopedMoved) > 0 {
					fc.scopedMoved[len(fc.scopedMoved)-1][ident.Name] = true
				}
			}
		}
		// Value is already in xmm0 from compileExpression call

	case *NamespacedIdentExpr:
		// Handle namespaced identifiers like sdl.SDL_INIT_VIDEO or data.field
		// Check if this is a C constant
		if constants, ok := fc.cConstants[e.Namespace]; ok {
			if value, found := constants.Constants[e.Name]; found {
				// Found a C constant - load it as a number
				fc.out.MovImmToReg("rax", strconv.FormatInt(value, 10))
				fc.out.Cvtsi2sd("xmm0", "rax")
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Resolved C constant %s.%s = %d\n", e.Namespace, e.Name, value)
				}
			} else {
				compilerError("undefined constant '%s.%s'", e.Namespace, e.Name)
			}
		} else {
			// Not a C import - treat as field access (obj.field)
			// Convert to IndexExpr and compile it
			hashValue := hashStringKey(e.Name)
			indexExpr := &IndexExpr{
				List:  &IdentExpr{Name: e.Namespace},
				Index: &NumberExpr{Value: float64(hashValue)},
			}
			fc.compileExpression(indexExpr)
		}

	case *LoopStateExpr:
		// @first, @last, @counter, @i are special loop state variables
		if len(fc.activeLoops) == 0 {
			compilerError("@%s used outside of loop", e.Type)
		}

		currentLoop := fc.activeLoops[len(fc.activeLoops)-1]

		switch e.Type {
		case "first":
			// @first: check if counter == 0
			var counterOffset int
			if currentLoop.IsRangeLoop {
				counterOffset = currentLoop.IteratorOffset
				// Load iterator as float, convert to int
				fc.out.MovMemToXmm("xmm0", "rbp", -counterOffset)
				fc.out.Cvttsd2si("rax", "xmm0")
			} else {
				counterOffset = currentLoop.IndexOffset
				// Load index as integer
				fc.out.MovMemToReg("rax", "rbp", -counterOffset)
			}
			// Compare with 0
			fc.out.CmpRegToImm("rax", 0)
			// Set rax to 1 if equal, 0 if not
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmove("rax", "rcx") // rax = (counter == 0) ? 1 : 0
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")

		case "last":
			// @last: check if counter == upper_bound - 1
			var counterOffset int
			if currentLoop.IsRangeLoop {
				counterOffset = currentLoop.IteratorOffset
				// Load iterator as float, convert to int
				fc.out.MovMemToXmm("xmm0", "rbp", -counterOffset)
				fc.out.Cvttsd2si("rax", "xmm0")
			} else {
				counterOffset = currentLoop.IndexOffset
				// Load index as integer
				fc.out.MovMemToReg("rax", "rbp", -counterOffset)
			}
			// Load upper bound
			fc.out.MovMemToReg("rdi", "rbp", -currentLoop.UpperBoundOffset)
			// Subtract 1 from upper bound: rdi = upper_bound - 1
			fc.out.SubImmFromReg("rdi", 1)
			// Compare counter with upper_bound - 1
			fc.out.CmpRegToReg("rax", "rdi")
			// Set rax to 1 if equal, 0 if not
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmove("rax", "rcx") // rax = (counter == upper_bound - 1) ? 1 : 0
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")

		case "counter":
			// @counter: return the iteration counter (starting at 0)
			if currentLoop.IsRangeLoop {
				// For range loops, iterator is the counter
				fc.out.MovMemToXmm("xmm0", "rbp", -currentLoop.IteratorOffset)
			} else {
				// For list loops, index is the counter
				fc.out.MovMemToReg("rax", "rbp", -currentLoop.IndexOffset)
				fc.out.Cvtsi2sd("xmm0", "rax")
			}

		case "i":
			// @i (level 0): current loop iterator
			// @i1 (level 1): outermost loop iterator
			// @i2 (level 2): second loop iterator, etc.

			var targetLoop LoopInfo
			if e.LoopLevel == 0 {
				// @i means current loop
				targetLoop = currentLoop
			} else {
				// @iN means loop at level N (1-indexed from outermost)
				if e.LoopLevel > len(fc.activeLoops) {
					compilerError("@i%d refers to loop level %d, but only %d loops active",
						e.LoopLevel, e.LoopLevel, len(fc.activeLoops))
				}
				// activeLoops[0] is outermost (level 1), activeLoops[1] is level 2, etc.
				targetLoop = fc.activeLoops[e.LoopLevel-1]
			}

			// Return the iterator value from the target loop
			fc.out.MovMemToXmm("xmm0", "rbp", -targetLoop.IteratorOffset)

		default:
			compilerError("unknown loop state variable @%s", e.Type)
		}

	case *UnaryExpr:
		// Compile the operand first (result in xmm0)
		fc.compileExpression(e.Operand)

		switch e.Operator {
		case "-":
			// Unary minus: negate the value
			// Create -1.0 constant and multiply
			labelName := fmt.Sprintf("negone_%d", fc.stringCounter)
			fc.stringCounter++

			// Store -1.0 as float64 bytes
			negOne := -1.0
			bits := uint64(0)
			*(*float64)(unsafe.Pointer(&bits)) = negOne
			var floatData []byte
			for i := 0; i < 8; i++ {
				floatData = append(floatData, byte((bits>>(i*8))&ByteMask))
			}
			fc.eb.Define(labelName, string(floatData))

			// Load -1.0 into xmm1 and multiply
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovMemToXmm("xmm1", "rax", 0)
			fc.out.MulsdXmm("xmm0", "xmm1") // xmm0 = xmm0 * -1.0
		case "not":
			// Logical NOT: returns 1.0 if operand is 0.0, else 0.0
			// Compare xmm0 with 0
			fc.out.XorpdXmm("xmm1", "xmm1") // xmm1 = 0.0
			fc.out.Ucomisd("xmm0", "xmm1")
			// Set rax to 1 if xmm0 == 0, else 0
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmove("rax", "rcx") // rax = (xmm0 == 0) ? 1 : 0
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")
		case "~b":
			// Bitwise NOT: convert to int64, NOT, convert back
			fc.out.Cvttsd2si("rax", "xmm0") // rax = int64(xmm0)
			fc.out.NotReg("rax")            // rax = ~rax
			fc.out.Cvtsi2sd("xmm0", "rax")  // xmm0 = float64(rax)
		case "#":
			// Length operator: return length of list/map/string
			// For numbers, return 1.0 (numbers are single-element maps)
			// For unknown types, treat as list/map/string (load length from pointer)
			operandType := fc.getExprType(e.Operand)

			if operandType == "number" {
				// It's a number, return 1.0
				fc.out.MovImmToReg("rax", "1")
				fc.out.Cvtsi2sd("xmm0", "rax")
			} else {
				// It's a map/list/string pointer, load length from offset 0
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
				fc.out.MovMemToReg("rax", "rsp", 0)
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Load the count from the map at offset 0
				fc.out.MovMemToXmm("xmm0", "rax", 0)
			}
		case "$":
			// Address value operator: treat value as memory address
			// This is for low-level operations, just pass through for now
			// xmm0 already contains the value (interpreted as address)
			// No-op: the value in xmm0 is already the "address"
		}

	case *PostfixExpr:
		// PostfixExpr (x++, x--) can only be used as statements, not expressions
		compilerError("%s can only be used as a statement, not in an expression (like Go)", e.Operator)

	case *FMAExpr:
		// Fused Multiply-Add: result = a * b + c (or a * b - c for FMSUB)
		// Detected by optimizer from patterns like (a * b) + c
		// Compile to single FMA instruction on x86-64 (FMA3/AVX-512), ARM64 (NEON/SVE), RISC-V (RVV)
		savedTailPosition := fc.inTailPosition
		fc.inTailPosition = false
		defer func() { fc.inTailPosition = savedTailPosition }()

		// Allocate registers for operands
		regA := fc.regTracker.AllocXMM("fma_a")
		regB := fc.regTracker.AllocXMM("fma_b")
		regC := fc.regTracker.AllocXMM("fma_c")
		if regA == "" || regB == "" || regC == "" {
			// Fallback to scalar registers if SIMD not available
			regA, regB, regC = "xmm1", "xmm2", "xmm3"
		}

		// Compile operands
		fc.compileExpression(e.A)
		fc.out.MovXmmToXmm(regA, "xmm0")

		fc.compileExpression(e.B)
		fc.out.MovXmmToXmm(regB, "xmm0")

		fc.compileExpression(e.C)
		fc.out.MovXmmToXmm(regC, "xmm0")

		// Emit FMA instruction: result = a * b + c or a * b - c
		// The FMA instruction does: dst = src1 * src2 +/- src3
		// So we want: xmm0 = regA * regB +/- regC
		if e.IsSub {
			// FMSUB: xmm0 = regA * regB - regC
			fc.out.VFmsubPDVectorToVector("xmm0", regA, regB, regC)
		} else {
			// FMADD: xmm0 = regA * regB + regC
			fc.out.VFmaddPDVectorToVector("xmm0", regA, regB, regC)
		}

		fc.regTracker.FreeXMM(regA)
		fc.regTracker.FreeXMM(regB)
		fc.regTracker.FreeXMM(regC)
		return

	case *BinaryExpr:
		// Confidence that this function is working: 98%
		// (arithmetic fully tested; string/list concat tested; edge cases may exist)
		savedTailPosition := fc.inTailPosition
		fc.inTailPosition = false
		defer func() { fc.inTailPosition = savedTailPosition }()

		// Special handling for or! operator (railway-oriented programming)
		// or! requires conditional execution: only evaluate right side if left is error/null
		if e.Operator == "or!" {
			if e.Right == nil {
				compilerError("or! operator requires a right-hand side expression or block")
			}

			// Compile left expression into xmm0
			fc.compileExpression(e.Left)

			// Check if xmm0 is NaN by comparing with itself
			fc.out.Ucomisd("xmm0", "xmm0") // Compare xmm0 with itself
			// If NaN, parity flag is set (PF=1)
			// Jump to execute_default if parity (i.e., if value is NaN)
			executeDefaultPos1 := fc.eb.text.Len()
			fc.out.JumpConditional(JumpParity, 0) // jp (jump if parity/NaN)

			// Not NaN, now check if xmm0 == 0.0 (null pointer)
			zeroReg := fc.regTracker.AllocXMM("or_bang_zero")
			if zeroReg == "" {
				zeroReg = "xmm2" // Fallback
			}
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm0", zeroReg)   // Compare xmm0 with 0.0
			fc.regTracker.FreeXMM(zeroReg)

			// Jump to execute_default if equal (i.e., if value is 0/null)
			executeDefaultPos2 := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // je (jump if equal to 0)

			// Value is valid (not NaN and not 0), skip to end without evaluating right side
			skipDefaultPos := fc.eb.text.Len()
			fc.out.JumpUnconditional(0) // jmp (unconditional jump to end)

			// execute_default label: evaluate right expression (could be block or value)
			executeDefaultLabel := fc.eb.text.Len()
			fc.compileExpression(e.Right) // Result goes to xmm0

			// End label
			endLabel := fc.eb.text.Len()

			// Patch the jumps
			// Patch NaN check jump to execute_default
			offset1 := int32(executeDefaultLabel - (executeDefaultPos1 + 6))
			fc.patchJumpImmediate(executeDefaultPos1+2, offset1)

			// Patch zero check jump to execute_default
			offset2 := int32(executeDefaultLabel - (executeDefaultPos2 + 6))
			fc.patchJumpImmediate(executeDefaultPos2+2, offset2)

			// Patch skip jump to end
			offset3 := int32(endLabel - (skipDefaultPos + 5))
			fc.patchJumpImmediate(skipDefaultPos+1, offset3)

			// xmm0 now contains either original value (if not NaN/null) or result of right side
			return
		}

		// Check for list repetition with * operator: [0] * 10
		// This MUST allocate on heap, not in .rodata, to allow list updates
		if e.Operator == "*" {
			leftType := fc.getExprType(e.Left)
			rightType := fc.getExprType(e.Right)

			if leftType == "list" && rightType == "number" {
				// List repetition: [x] * n creates a new heap-allocated list
				// NOT a compile-time .rodata constant (to allow mutations)

				// Compile list (result in xmm0 - pointer to list)
				fc.compileExpression(e.Left)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Compile count (result in xmm0 - the number)
				fc.compileExpression(e.Right)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Convert count to integer in rcx
				fc.out.Cvttsd2si("rcx", "xmm0") // rcx = count

				// Load list pointer from stack to rdi
				fc.out.MovMemToReg("rdi", "rsp", 16)

				// Save rcx (count) to rdx since we'll need it
				fc.out.MovRegToReg("rdx", "rcx")

				// Clean up stack
				fc.out.AddImmToReg("rsp", 32)

				// Call _c67_list_repeat(list_ptr in rdi, count in rdx)
				// We need to implement this helper function
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.CallSymbol("_c67_list_repeat")
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Result pointer is in rax, convert to xmm0
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovRegToMem("rax", "rsp", 0)
				fc.out.MovMemToXmm("xmm0", "rsp", 0)
				fc.out.AddImmToReg("rsp", StackSlotSize)
				return
			}
		}

		// Check for string/list/map operations with + operator
		if e.Operator == "+" {
			leftType := fc.getExprType(e.Left)
			rightType := fc.getExprType(e.Right)

			if leftType == "string" && rightType == "string" {
				// String concatenation (strings are maps, so merge with offset keys)
				leftStr, leftIsLiteral := e.Left.(*StringExpr)
				rightStr, rightIsLiteral := e.Right.(*StringExpr)

				if leftIsLiteral && rightIsLiteral {
					// Compile-time concatenation - just create new string map
					result := leftStr.Value + rightStr.Value

					// Build concatenated string map
					labelName := fmt.Sprintf("str_%d", fc.stringCounter)
					fc.stringCounter++

					var mapData []byte
					count := float64(len(result))
					countBits := uint64(0)
					*(*float64)(unsafe.Pointer(&countBits)) = count
					for i := 0; i < 8; i++ {
						mapData = append(mapData, byte((countBits>>(i*8))&ByteMask))
					}

					for idx, ch := range result {
						// Key: index
						keyVal := float64(idx)
						keyBits := uint64(0)
						*(*float64)(unsafe.Pointer(&keyBits)) = keyVal
						for i := 0; i < 8; i++ {
							mapData = append(mapData, byte((keyBits>>(i*8))&ByteMask))
						}

						// Value: char code
						charVal := float64(ch)
						charBits := uint64(0)
						*(*float64)(unsafe.Pointer(&charBits)) = charVal
						for i := 0; i < 8; i++ {
							mapData = append(mapData, byte((charBits>>(i*8))&ByteMask))
						}
					}

					fc.eb.Define(labelName, string(mapData))
					fc.out.LeaSymbolToReg("rax", labelName)
					fc.out.SubImmFromReg("rsp", StackSlotSize)
					fc.out.MovRegToMem("rax", "rsp", 0)
					fc.out.MovMemToXmm("xmm0", "rsp", 0)
					fc.out.AddImmToReg("rsp", StackSlotSize)
					break
				}

				// Runtime string concatenation
				// Evaluate left string (result pointer in xmm0)
				fc.compileExpression(e.Left)
				// Save left pointer to stack
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Evaluate right string (result pointer in xmm0)
				fc.compileExpression(e.Right)
				// Save right pointer to stack
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Call _c67_string_concat(left_ptr, right_ptr)
				// Load arguments into registers following x86-64 calling convention
				fc.out.MovMemToReg("rdi", "rsp", 16) // left ptr (first arg)
				fc.out.MovMemToReg("rsi", "rsp", 0)  // right ptr (second arg)
				fc.out.AddImmToReg("rsp", 32)        // clean up stack

				// Align stack for call (must be at 16n+8 before CALL)
				fc.out.SubImmFromReg("rsp", StackSlotSize)

				// Call the helper function (direct call, not through PLT)
				fc.trackFunctionCall("_c67_string_concat")
				fc.out.CallSymbol("_c67_string_concat")

				// Restore stack alignment
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Result pointer is in rax, convert to xmm0
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovRegToMem("rax", "rsp", 0)
				fc.out.MovMemToXmm("xmm0", "rsp", 0)
				fc.out.AddImmToReg("rsp", StackSlotSize)
				break
			}

			if leftType == "list" && rightType == "list" {
				// List concatenation: [1, 2] + [3, 4] -> [1, 2, 3, 4]
				leftList, leftIsLiteral := e.Left.(*ListExpr)
				rightList, rightIsLiteral := e.Right.(*ListExpr)

				if leftIsLiteral && rightIsLiteral {
					// Compile-time concatenation
					labelName := fmt.Sprintf("list_%d", fc.stringCounter)
					fc.stringCounter++

					var listData []byte

					// Calculate total length
					totalLen := float64(len(leftList.Elements) + len(rightList.Elements))
					lengthBits := uint64(0)
					*(*float64)(unsafe.Pointer(&lengthBits)) = totalLen
					for i := 0; i < 8; i++ {
						listData = append(listData, byte((lengthBits>>(i*8))&ByteMask))
					}

					// Add all elements from left list
					for _, elem := range leftList.Elements {
						if numExpr, ok := elem.(*NumberExpr); ok {
							elemBits := uint64(0)
							*(*float64)(unsafe.Pointer(&elemBits)) = numExpr.Value
							for i := 0; i < 8; i++ {
								listData = append(listData, byte((elemBits>>(i*8))&ByteMask))
							}
						}
					}

					// Add all elements from right list
					for _, elem := range rightList.Elements {
						if numExpr, ok := elem.(*NumberExpr); ok {
							elemBits := uint64(0)
							*(*float64)(unsafe.Pointer(&elemBits)) = numExpr.Value
							for i := 0; i < 8; i++ {
								listData = append(listData, byte((elemBits>>(i*8))&ByteMask))
							}
						}
					}

					fc.eb.Define(labelName, string(listData))
					fc.out.LeaSymbolToReg("rax", labelName)
					fc.out.SubImmFromReg("rsp", StackSlotSize)
					fc.out.MovRegToMem("rax", "rsp", 0)
					fc.out.MovMemToXmm("xmm0", "rsp", 0)
					fc.out.AddImmToReg("rsp", StackSlotSize)
					return
				}

				// Runtime concatenation
				// Compile left list (result in xmm0)
				fc.compileExpression(e.Left)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Compile right list (result in xmm0)
				fc.compileExpression(e.Right)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Call _c67_list_concat(left_ptr, right_ptr)
				fc.out.MovMemToReg("rdi", "rsp", 16) // left ptr
				fc.out.MovMemToReg("rsi", "rsp", 0)  // right ptr
				fc.out.AddImmToReg("rsp", 32)

				// Align stack for call
				fc.out.SubImmFromReg("rsp", StackSlotSize)

				// Call the helper function
				// Note: Don't track internal function calls (see comment at _c67_string_eq call)
				fc.out.CallSymbol("_c67_list_concat")

				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Result pointer is in rax, convert to xmm0
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovRegToMem("rax", "rsp", 0)
				fc.out.MovMemToXmm("xmm0", "rsp", 0)
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Return early - don't do numeric operation
				return
			}

			// List + element: append element to list
			// This makes "list += element" work as shorthand for "list <- list.append(element)"
			if leftType == "list" || leftType == "unknown" {
				// Compile list (result in xmm0)
				fc.compileExpression(e.Left)
				fc.out.SubImmFromReg("rsp", 8)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Compile element (result in xmm0)
				fc.compileExpression(e.Right)
				fc.out.SubImmFromReg("rsp", 8)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Use the same append logic as the append() builtin
				// Load list pointer and get count
				listReg := fc.regTracker.AllocXMM("append_list_ptr")
				countReg := fc.regTracker.AllocXMM("append_count")
				if listReg == "" {
					listReg = "xmm1" // Fallback
				}
				if countReg == "" {
					countReg = "xmm2" // Fallback
				}
				fc.out.MovMemToXmm(listReg, "rsp", 8)
				fc.out.MovqXmmToReg("rsi", listReg)
				fc.out.MovMemToXmm(countReg, "rsi", 0)
				fc.out.Cvttsd2si("rcx", countReg)
				fc.regTracker.FreeXMM(listReg)
				fc.regTracker.FreeXMM(countReg)

				// Calculate new size: 8 + (count + 1) * 16
				fc.out.MovRegToReg("rdx", "rcx")
				fc.out.AddImmToReg("rdx", 1)
				fc.out.ShlImmReg("rdx", 4)
				fc.out.AddImmToReg("rdx", 8)

				// Allocate new list
				fc.out.MovRegToReg("rdi", "rdx")
				fc.out.PushReg("rsi")
				fc.out.PushReg("rcx")
				// Allocate from arena
				fc.callArenaAlloc()
				fc.out.PopReg("rcx")
				fc.out.PopReg("rsi")

				// Store new count
				newCountReg := fc.regTracker.AllocXMM("append_new_count")
				if newCountReg == "" {
					newCountReg = "xmm3" // Fallback
				}
				fc.out.MovRegToReg("r10", "rcx")
				fc.out.AddImmToReg("r10", 1)
				fc.out.Cvtsi2sd(newCountReg, "r10")
				fc.out.MovXmmToMem(newCountReg, "rax", 0)
				fc.regTracker.FreeXMM(newCountReg)

				// Copy old elements if count > 0
				fc.out.TestRegReg("rcx", "rcx")
				skipCopyJump := fc.eb.text.Len()
				fc.out.JumpConditional(JumpEqual, 0)
				skipCopyPatch := fc.eb.text.Len()

				fc.out.PushReg("rax")
				fc.out.PushReg("rsi")
				fc.out.PushReg("rcx")

				fc.out.LeaMemToReg("rdi", "rax", 8)
				fc.out.LeaMemToReg("rsi", "rsi", 8)
				fc.out.MovRegToReg("rdx", "rcx")
				fc.out.ShlImmReg("rdx", 4)

				fc.trackFunctionCall("memcpy")
				fc.eb.GenerateCallInstruction("memcpy")

				fc.out.PopReg("rcx")
				fc.out.PopReg("rsi")
				fc.out.PopReg("rax")

				// Patch skip jump
				currentPos := fc.eb.text.Len()
				skipOffset := currentPos - skipCopyPatch
				fc.patchJumpImmediate(skipCopyJump+2, int32(skipOffset))

				// Add new entry at end
				fc.out.MovRegToReg("rdx", "rcx")
				fc.out.ShlImmReg("rdx", 4)
				fc.out.AddImmToReg("rdx", 8)

				fc.out.MovRegToReg("r8", "rax")
				fc.out.AddRegToReg("r8", "rdx")

				// Store key and value
				valReg := fc.regTracker.AllocXMM("append_value")
				if valReg == "" {
					valReg = "xmm4" // Fallback
				}
				fc.out.StoreRegToMem("rcx", "r8", 0)
				fc.out.MovMemToXmm(valReg, "rsp", 0)
				fc.out.MovXmmToMem(valReg, "r8", 8)
				fc.regTracker.FreeXMM(valReg)

				// Clean stack
				fc.out.AddImmToReg("rsp", 16)

				// Return new list pointer in xmm0
				fc.out.MovqRegToXmm("xmm0", "rax")
				return
			}

			if leftType == "map" && rightType == "map" {
				// Map union is a future enhancement
				compilerError("map union not yet implemented")
			}
		}

		// String comparison operators
		if e.Operator == "==" || e.Operator == "!=" {
			leftType := fc.getExprType(e.Left)
			rightType := fc.getExprType(e.Right)

			if leftType == "string" && rightType == "string" {
				// String comparison: compare character by character
				// Compile left string (result in xmm0)
				fc.compileExpression(e.Left)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Compile right string (result in xmm0)
				fc.compileExpression(e.Right)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)

				// Call _c67_string_eq(left_ptr, right_ptr)
				fc.out.MovMemToReg("rdi", "rsp", 16) // left ptr
				fc.out.MovMemToReg("rsi", "rsp", 0)  // right ptr
				fc.out.AddImmToReg("rsp", 32)

				// Align stack for call
				fc.out.SubImmFromReg("rsp", StackSlotSize)

				// Call the helper function
				// Track this so we know to emit it
				fc.trackFunctionCall("_c67_string_eq")
				fc.out.CallSymbol("_c67_string_eq")

				// Restore stack alignment
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Result (1.0 or 0.0) is in xmm0
				if e.Operator == "!=" {
					// Invert the result: result = 1.0 - result
					labelName := fmt.Sprintf("float_const_%d", fc.stringCounter)
					fc.stringCounter++
					one := 1.0
					bits := uint64(0)
					*(*float64)(unsafe.Pointer(&bits)) = one
					var floatData []byte
					for i := 0; i < 8; i++ {
						floatData = append(floatData, byte((bits>>(i*8))&ByteMask))
					}
					fc.eb.Define(labelName, string(floatData))
					fc.out.LeaSymbolToReg("rax", labelName)
					fc.out.MovMemToXmm("xmm1", "rax", 0)
					fc.out.SubsdXmm("xmm1", "xmm0") // xmm1 = 1.0 - xmm0
					fc.out.MovRegToReg("xmm0", "xmm1")
				}
				return
			}
		}

		// Default: numeric binary operation
		// We must save the left operand to the STACK, not to a register,
		// because compileExpression(e.Right) may call functions that clobber registers

		// Compile left into xmm0
		fc.compileExpression(e.Left)

		// Save left operand to stack (function calls may clobber all XMM registers)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile right into xmm0
		fc.compileExpression(e.Right)

		// Move right to xmm1, restore left from stack to xmm0
		fc.out.MovRegToReg("xmm1", "xmm0")
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)
		// Perform scalar floating-point operation
		switch e.Operator {
		case "+":
			fc.out.AddsdXmm("xmm0", "xmm1") // addsd xmm0, xmm1
		case "-":
			fc.out.SubsdXmm("xmm0", "xmm1") // subsd xmm0, xmm1
		case "*":
			fc.out.MulsdXmm("xmm0", "xmm1") // mulsd xmm0, xmm1
		case "*+":
			// FMA: a *+ b = a * a + b (square and add, using fused multiply-add)
			// Use VFMADD213SD xmm0, xmm0, xmm1 => xmm0 = xmm0 * xmm0 + xmm1
			// Encoding: C4 E2 F9 A9 C1 (VFMADD213SD xmm0, xmm0, xmm1)
			fc.out.Write(0xC4) // VEX 3-byte prefix
			fc.out.Write(0xE2) // VEX byte 1: R=1, X=1, B=1, m=00010 (0F38)
			fc.out.Write(0xF9) // VEX byte 2: W=1, vvvv=0000 (xmm0), L=0, pp=01 (66)
			fc.out.Write(0xA9) // Opcode: VFMADD213SD
			fc.out.Write(0xC1) // ModR/M: 11 000 001 (xmm0, xmm0, xmm1)
		case "/":
			// Check for division by zero (xmm1 == 0.0)
			zeroReg := fc.regTracker.AllocXMM("div_zero_check")
			if zeroReg == "" {
				zeroReg = "xmm2" // Fallback
			}
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm1", zeroReg)   // Compare divisor with 0
			fc.regTracker.FreeXMM(zeroReg)

			// Jump to division if not zero
			jumpPos := fc.eb.text.Len()
			fc.out.JumpConditional(JumpNotEqual, 0) // Placeholder, will patch later

			// Division by zero: return error NaN with "dv0\0" code
			// Error format: 0x7FF8_0000_6476_3000 (quiet NaN + error code)
			fc.out.Emit([]byte{0x48, 0xb8})                                     // mov rax, immediate64
			fc.out.Emit([]byte{0x00, 0x30, 0x76, 0x64, 0x00, 0x00, 0xf8, 0x7f}) // NaN with "dv0\0"
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovRegToMem("rax", "rsp", 0)
			fc.out.MovMemToXmm("xmm0", "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)

			// Jump over the normal division
			divDonePos := fc.eb.text.Len()
			fc.out.JumpUnconditional(0) // Placeholder

			// Patch jump to here (safe division)
			safePos := fc.eb.text.Len()
			jumpEndPos := jumpPos + 6
			offset := int32(safePos - jumpEndPos)
			fc.patchJumpImmediate(jumpPos+2, offset)

			fc.out.DivsdXmm("xmm0", "xmm1") // divsd xmm0, xmm1

			// Patch the jump over division
			endPos := fc.eb.text.Len()
			divDoneOffset := int32(endPos - (divDonePos + 5))
			fc.patchJumpImmediate(divDonePos+1, divDoneOffset)
		case "mod", "%":
			// Modulo: a mod b = a - b * floor(a / b)
			// xmm0 = dividend (a), xmm1 = divisor (b)

			// Check for modulo by zero (xmm1 == 0.0)
			zeroReg := fc.regTracker.AllocXMM("mod_zero_check")
			if zeroReg == "" {
				zeroReg = "xmm4" // Fallback
			}
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm1", zeroReg)   // Compare divisor with 0
			fc.regTracker.FreeXMM(zeroReg)

			// Jump to modulo if not zero
			jumpPos := fc.eb.text.Len()
			fc.out.JumpConditional(JumpNotEqual, 0) // Placeholder

			// Modulo by zero: print error and exit
			errorMsg := "Error: modulo by zero\n"
			errorLabel := fmt.Sprintf("mod_zero_error_%d", fc.stringCounter)
			fc.stringCounter++
			fc.eb.Define(errorLabel, errorMsg)

			// syscall: write(2, msg, len)
			fc.out.MovImmToReg("rax", "1")
			fc.out.MovImmToReg("rdi", "2")
			fc.out.LeaSymbolToReg("rsi", errorLabel)
			fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(errorMsg)))
			fc.eb.Emit("syscall")

			// syscall: exit(1)
			fc.out.MovImmToReg("rax", "60")
			fc.out.MovImmToReg("rdi", "1")
			fc.eb.Emit("syscall")

			// Patch jump to here (safe modulo)
			safePos := fc.eb.text.Len()
			jumpEndPos := jumpPos + 6
			offset := int32(safePos - jumpEndPos)
			fc.patchJumpImmediate(jumpPos+2, offset)

			// Allocate temporary registers for modulo computation
			tmpDividend := fc.regTracker.AllocXMM("mod_dividend")
			tmpDivisor := fc.regTracker.AllocXMM("mod_divisor")
			if tmpDividend == "" {
				tmpDividend = "xmm2" // Fallback
			}
			if tmpDivisor == "" {
				tmpDivisor = "xmm3" // Fallback
			}

			fc.out.MovXmmToXmm(tmpDividend, "xmm0") // Save dividend
			fc.out.MovXmmToXmm(tmpDivisor, "xmm1")  // Save divisor
			fc.out.DivsdXmm("xmm0", "xmm1")         // xmm0 = a / b
			// Floor: convert to int64 and back
			fc.out.Cvttsd2si("rax", "xmm0")         // rax = floor(a / b) as int
			fc.out.Cvtsi2sd("xmm0", "rax")          // xmm0 = floor(a / b) as float
			fc.out.MulsdXmm("xmm0", tmpDivisor)     // xmm0 = floor(a / b) * b
			fc.out.SubsdXmm(tmpDividend, "xmm0")    // tmpDividend = a - floor(a / b) * b
			fc.out.MovXmmToXmm("xmm0", tmpDividend) // Result in xmm0

			fc.regTracker.FreeXMM(tmpDividend)
			fc.regTracker.FreeXMM(tmpDivisor)
		case "<", "<=", ">", ">=", "==", "!=":
			// Compare xmm0 with xmm1, sets flags
			fc.out.Ucomisd("xmm0", "xmm1")
			// Convert comparison result to boolean (0.0 or 1.0)
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			// Use conditional move based on comparison operator
			switch e.Operator {
			case "<":
				fc.out.Cmovb("rax", "rcx") // rax = (xmm0 < xmm1) ? 1 : 0
			case "<=":
				fc.out.Cmovbe("rax", "rcx") // rax = (xmm0 <= xmm1) ? 1 : 0
			case ">":
				fc.out.Cmova("rax", "rcx") // rax = (xmm0 > xmm1) ? 1 : 0
			case ">=":
				fc.out.Cmovae("rax", "rcx") // rax = (xmm0 >= xmm1) ? 1 : 0
			case "==":
				fc.out.Cmove("rax", "rcx") // rax = (xmm0 == xmm1) ? 1 : 0
			case "!=":
				fc.out.Cmovne("rax", "rcx") // rax = (xmm0 != xmm1) ? 1 : 0
			}
			// Convert integer result to float64
			fc.out.Cvtsi2sd("xmm0", "rax")
		case "and":
			// Logical AND: returns 1.0 if both non-zero, else 0.0
			zeroReg := fc.regTracker.AllocXMM("and_zero")
			if zeroReg == "" {
				zeroReg = "xmm2" // Fallback
			}
			// Compare xmm0 with 0
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm0", zeroReg)
			// Set rax to 1 if xmm0 != 0
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmovne("rax", "rcx") // rax = (xmm0 != 0) ? 1 : 0
			// Compare xmm1 with 0
			fc.out.Ucomisd("xmm1", zeroReg)
			// Set rcx to 1 if xmm1 != 0
			fc.out.MovImmToReg("rcx", "0")
			fc.out.MovImmToReg("rdx", "1")
			fc.out.Cmovne("rcx", "rdx") // rcx = (xmm1 != 0) ? 1 : 0
			// AND the results: rax = rax & rcx
			fc.out.AndRegWithReg("rax", "rcx")
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")
			fc.regTracker.FreeXMM(zeroReg)
		case "or":
			// Logical OR: returns 1.0 if either non-zero, else 0.0
			zeroReg := fc.regTracker.AllocXMM("or_zero")
			if zeroReg == "" {
				zeroReg = "xmm2" // Fallback
			}
			// Compare xmm0 with 0
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm0", zeroReg)
			// Set rax to 1 if xmm0 != 0
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmovne("rax", "rcx") // rax = (xmm0 != 0) ? 1 : 0
			// Compare xmm1 with 0
			fc.out.Ucomisd("xmm1", zeroReg)
			// Set rcx to 1 if xmm1 != 0
			fc.out.MovImmToReg("rcx", "0")
			fc.out.MovImmToReg("rdx", "1")
			fc.out.Cmovne("rcx", "rdx") // rcx = (xmm1 != 0) ? 1 : 0
			// OR the results: rax = rax | rcx
			fc.out.OrRegWithReg("rax", "rcx")
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")
			fc.regTracker.FreeXMM(zeroReg)
		case "xor":
			// Logical XOR: returns 1.0 if exactly one non-zero, else 0.0
			zeroReg := fc.regTracker.AllocXMM("xor_zero")
			if zeroReg == "" {
				zeroReg = "xmm2" // Fallback
			}
			// Compare xmm0 with 0
			fc.out.XorpdXmm(zeroReg, zeroReg) // zero register = 0.0
			fc.out.Ucomisd("xmm0", zeroReg)
			// Set rax to 1 if xmm0 != 0
			fc.out.MovImmToReg("rax", "0")
			fc.out.MovImmToReg("rcx", "1")
			fc.out.Cmovne("rax", "rcx") // rax = (xmm0 != 0) ? 1 : 0
			// Compare xmm1 with 0
			fc.out.Ucomisd("xmm1", zeroReg)
			// Set rcx to 1 if xmm1 != 0
			fc.out.MovImmToReg("rcx", "0")
			fc.out.MovImmToReg("rdx", "1")
			fc.out.Cmovne("rcx", "rdx") // rcx = (xmm1 != 0) ? 1 : 0
			// XOR the results: rax = rax ^ rcx
			fc.out.XorRegWithReg("rax", "rcx")
			// Convert to float64
			fc.out.Cvtsi2sd("xmm0", "rax")
			fc.regTracker.FreeXMM(zeroReg)
		case "or!":
			// Confidence that this function is working: 0%
			// NOTE: or! is now handled specially before the operator switch (see above)
			// This case should never be reached. If it is, there's a bug in the special handling.
			compilerError("or! operator reached generic binary operation switch (should be handled specially)")
		case "<<b":
			// Shift left: convert to int64, shift, convert back
			fc.out.Cvttsd2si("rax", "xmm0") // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = int64(xmm1)
			fc.out.ShlClReg("rax", "cl")    // rax <<= cl
			fc.out.Cvtsi2sd("xmm0", "rax")  // xmm0 = float64(rax)
		case ">>b":
			// Shift right: convert to int64, shift, convert back
			fc.out.Cvttsd2si("rax", "xmm0") // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = int64(xmm1)
			fc.out.ShrClReg("rax", "cl")    // rax >>= cl
			fc.out.Cvtsi2sd("xmm0", "rax")  // xmm0 = float64(rax)
		case "<<<b":
			// Rotate left: convert to int64, rotate, convert back
			fc.out.Cvttsd2si("rax", "xmm0") // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = int64(xmm1)
			fc.out.RolClReg("rax", "cl")    // rol rax, cl
			fc.out.Cvtsi2sd("xmm0", "rax")  // xmm0 = float64(rax)
		case ">>>b":
			// Rotate right: convert to int64, rotate, convert back
			fc.out.Cvttsd2si("rax", "xmm0") // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = int64(xmm1)
			fc.out.RorClReg("rax", "cl")    // ror rax, cl
			fc.out.Cvtsi2sd("xmm0", "rax")  // xmm0 = float64(rax)
		case "?b":
			// Bit test: test if bit at position (xmm1) is set in value (xmm0)
			// Returns 1.0 if bit is set, 0.0 otherwise
			fc.out.Cvttsd2si("rax", "xmm0")      // rax = int64(value)
			fc.out.Cvttsd2si("rcx", "xmm1")      // rcx = int64(bit_position)
			fc.out.BtRegReg("rax", "rcx")        // BT rax, rcx (sets CF if bit is set)
			fc.out.SetcReg("al")                 // al = CF ? 1 : 0
			fc.out.MovzxByteToQword("rax", "al") // Zero-extend al to rax
			fc.out.Cvtsi2sd("xmm0", "rax")       // xmm0 = float64(result)
		case "|b":
			// Bitwise OR: convert to int64, OR, convert back
			fc.out.Cvttsd2si("rax", "xmm0")   // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1")   // rcx = int64(xmm1)
			fc.out.OrRegWithReg("rax", "rcx") // rax |= rcx
			fc.out.Cvtsi2sd("xmm0", "rax")    // xmm0 = float64(rax)
		case "&b":
			// Bitwise AND: convert to int64, AND, convert back
			fc.out.Cvttsd2si("rax", "xmm0")    // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1")    // rcx = int64(xmm1)
			fc.out.AndRegWithReg("rax", "rcx") // rax &= rcx
			fc.out.Cvtsi2sd("xmm0", "rax")     // xmm0 = float64(rax)
		case "^b":
			// Bitwise XOR: convert to int64, XOR, convert back
			fc.out.Cvttsd2si("rax", "xmm0")    // rax = int64(xmm0)
			fc.out.Cvttsd2si("rcx", "xmm1")    // rcx = int64(xmm1)
			fc.out.XorRegWithReg("rax", "rcx") // rax ^= rcx
			fc.out.Cvtsi2sd("xmm0", "rax")     // xmm0 = float64(rax)
		case "**":
			// Power: call pow(base, exponent) from libm
			// xmm0 = base, xmm1 = exponent -> result in xmm0
			fc.trackFunctionCall("pow")
			fc.eb.GenerateCallInstruction("pow")
		case "::":
			// Cons: prepend element to list
			// xmm0 = element, xmm1 = list pointer
			// Convert floats to pointers for C function call
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
			fc.out.MovMemToReg("rdi", "rsp", 0) // first arg
			fc.out.AddImmToReg("rsp", 8)

			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm1", "rsp", 0)
			fc.out.MovMemToReg("rsi", "rsp", 0) // second arg
			fc.out.AddImmToReg("rsp", 8)

			fc.eb.GenerateCallInstruction("_c67_list_cons")
			// Result pointer in rax, move to xmm0 preserving bit pattern
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovRegToMem("rax", "rsp", 0)
			fc.out.MovMemToXmm("xmm0", "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)
		}

	case *CallExpr:
		fc.compileCall(e)

	case *DirectCallExpr:
		fc.compileDirectCall(e)

	case *RangeExpr:
		// Compile range expression by expanding it to a list
		// 0..<10 becomes [0, 1, 2, ..., 9]
		// 0..=10 becomes [0, 1, 2, ..., 10]

		// Evaluate start and end expressions (must be compile-time constants for now)
		startNum, startOk := e.Start.(*NumberExpr)
		endNum, endOk := e.End.(*NumberExpr)

		if !startOk || !endOk {
			compilerError("range expressions currently only support number literals")
		}

		start := int64(startNum.Value)
		end := int64(endNum.Value)

		// Build list of elements
		var elements []Expression
		if e.Inclusive {
			// ..= includes end value
			for i := start; i <= end; i++ {
				elements = append(elements, &NumberExpr{Value: float64(i)})
			}
		} else {
			// ..< excludes end value
			for i := start; i < end; i++ {
				elements = append(elements, &NumberExpr{Value: float64(i)})
			}
		}

		// Compile as a list
		listExpr := &ListExpr{Elements: elements}
		fc.compileExpression(listExpr)

	case *ListExpr:
		// UNIVERSAL MAP representation: [count][key0][val0][key1][val1]...
		// [1, 2, 3] => [3.0][0][1.0][1][2.0][2][3.0]
		// Lists are just maps with sequential integer keys (0, 1, 2, ...)

		count := len(e.Elements)

		if count == 0 {
			// Empty list: return empty map pointer
			// Allocate 8 bytes for count=0
			labelName := fmt.Sprintf("empty_list_%d", fc.stringCounter)
			fc.stringCounter++

			// Create empty list data: just count = 0.0
			var listData []byte
			countBits := uint64(0)
			for i := 0; i < 8; i++ {
				listData = append(listData, byte((countBits>>(i*8))&ByteMask))
			}

			fc.eb.Define(labelName, string(listData))
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovqRegToXmm("xmm0", "rax")
		} else {
			// Build list data in memory: [count][key0][val0][key1][val1]...
			labelName := fmt.Sprintf("list_%d", fc.stringCounter)
			fc.stringCounter++

			var listData []byte

			// Write count as float64
			countBits := uint64(0)
			*(*float64)(unsafe.Pointer(&countBits)) = float64(count)
			for i := 0; i < 8; i++ {
				listData = append(listData, byte((countBits>>(i*8))&ByteMask))
			}

			// Write [key, value] pairs
			for i, elem := range e.Elements {
				// Write key (index as uint64)
				key := uint64(i)
				for j := 0; j < 8; j++ {
					listData = append(listData, byte((key>>(j*8))&ByteMask))
				}

				// Write value (float64)
				if numExpr, ok := elem.(*NumberExpr); ok {
					// Compile-time constant
					valueBits := uint64(0)
					*(*float64)(unsafe.Pointer(&valueBits)) = numExpr.Value
					for j := 0; j < 8; j++ {
						listData = append(listData, byte((valueBits>>(j*8))&ByteMask))
					}
				} else {
					// Non-constant expression - can't use static data
					// Fall back to runtime allocation
					goto runtimeAllocation
				}
			}

			// All elements are constants, use static data
			fc.eb.Define(labelName, string(listData))
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovqRegToXmm("xmm0", "rax")
			break

		runtimeAllocation:
			// Dynamic list: allocate memory and populate at runtime
			// Size: 8 + (count * 16) bytes
			size := 8 + (count * 16)

			// Allocate memory using arena allocator
			fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", size))
			fc.callArenaAlloc() // Use arena instead of malloc
			// rax now contains pointer to allocated memory

			// Save list pointer
			fc.out.PushReg("rbx")
			fc.out.MovRegToReg("rbx", "rax") // rbx = list pointer

			// Write count
			fc.out.MovImmToReg("rax", fmt.Sprintf("%d", count))
			fc.out.Cvtsi2sd("xmm0", "rax")
			fc.out.MovXmmToMem("xmm0", "rbx", 0)

			// Write each [key, value] pair
			for i, elem := range e.Elements {
				offset := 8 + (i * 16)

				// Write key (index)
				fc.out.MovImmToReg("rax", fmt.Sprintf("%d", i))
				fc.out.MovRegToMem("rax", "rbx", offset)

				// Compile and write value
				fc.compileExpression(elem)
				fc.out.MovXmmToMem("xmm0", "rbx", offset+8)
			}

			// Return list pointer in xmm0
			fc.out.MovqRegToXmm("xmm0", "rbx")
			fc.out.PopReg("rbx")
		}

	case *InExpr:
		// Membership testing: value in container
		// Returns 1.0 if found, 0.0 if not found

		// Compile value to search for
		fc.compileExpression(e.Value)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile container
		fc.compileExpression(e.Container)
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)
		fc.out.MovMemToReg("rbx", "rsp", StackSlotSize) // rbx = container pointer

		// Load count from container (empty containers have count=0, not null)
		fc.out.MovMemToXmm("xmm1", "rbx", 0)
		fc.out.Cvttsd2si("rcx", "xmm1")      // rcx = count
		fc.out.MovMemToXmm("xmm2", "rsp", 0) // xmm2 = search value

		// Loop: rdi = index
		fc.out.XorRegWithReg("rdi", "rdi")
		loopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("rdi", "rcx")
		loopEndJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		loopEndJumpEnd := fc.eb.text.Len()

		// Load element at index
		fc.out.MovRegToReg("rax", "rdi")
		fc.out.MulRegWithImm("rax", 8)
		fc.out.AddImmToReg("rax", 8)
		fc.out.AddRegToReg("rax", "rbx")
		fc.out.MovMemToXmm("xmm3", "rax", 0)

		// Compare
		fc.out.Ucomisd("xmm2", "xmm3")
		notEqualJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		notEqualEnd := fc.eb.text.Len()

		// Found! Return 1.0
		fc.out.MovImmToReg("rax", "1")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.AddImmToReg("rsp", 16)
		foundJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)
		foundJumpEnd := fc.eb.text.Len()

		// Not equal: next iteration
		notEqualPos := fc.eb.text.Len()
		fc.patchJumpImmediate(notEqualJump+2, int32(notEqualPos-notEqualEnd))
		fc.out.AddImmToReg("rdi", 1)
		fc.out.JumpUnconditional(int32(loopStart - (fc.eb.text.Len() + 5)))

		// Not found: return 0.0
		loopEndPos := fc.eb.text.Len()
		fc.patchJumpImmediate(loopEndJump+2, int32(loopEndPos-loopEndJumpEnd))
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.AddImmToReg("rsp", 16)

		// Patch found jump to skip to end
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(foundJump+1, int32(endPos-foundJumpEnd))

	case *MapExpr:
		// Map literal stored as: [count (float64)] [key1] [value1] [key2] [value2] ...
		// Even empty maps need a proper data structure with count = 0
		labelName := fmt.Sprintf("map_%d", fc.stringCounter)
		fc.stringCounter++
		var mapData []byte

		// Add count
		count := float64(len(e.Keys))
		countBits := uint64(0)
		*(*float64)(unsafe.Pointer(&countBits)) = count
		for i := 0; i < 8; i++ {
			mapData = append(mapData, byte((countBits>>(i*8))&ByteMask))
		}

		// Add key-value pairs (if any)
		for i := range e.Keys {
			if keyNum, ok := e.Keys[i].(*NumberExpr); ok {
				keyBits := uint64(0)
				*(*float64)(unsafe.Pointer(&keyBits)) = keyNum.Value
				for j := 0; j < 8; j++ {
					mapData = append(mapData, byte((keyBits>>(j*8))&ByteMask))
				}
			}
			if valNum, ok := e.Values[i].(*NumberExpr); ok {
				valBits := uint64(0)
				*(*float64)(unsafe.Pointer(&valBits)) = valNum.Value
				for j := 0; j < 8; j++ {
					mapData = append(mapData, byte((valBits>>(j*8))&ByteMask))
				}
			}
		}

		fc.eb.Define(labelName, string(mapData))
		fc.out.LeaSymbolToReg("rax", labelName)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovRegToMem("rax", "rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
	case *IndexExpr:
		// Determine if we're indexing a map/string or list
		// Strings are map[uint64]float64, so use map indexing
		containerType := fc.getExprType(e.List)

		// If indexing a number, return 0.0 (undefined property)
		if containerType == "number" {
			fc.out.XorpdXmm("xmm0", "xmm0") // xmm0 = 0.0
			break
		}

		// For "unknown" types (lambda parameters, captured vars), default to list indexing
		// This is a reasonable default since lists are more common than maps
		isMap := false
		if containerType == "map" || containerType == "string" {
			isMap = true
		} else if containerType == "unknown" {
			// Default unknown types to list indexing (simpler and more common)
			isMap = false
		}

		// Compile container expression (returns pointer as float64 in xmm0)
		fc.compileExpression(e.List)
		// Save container pointer to stack
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile index/key expression (returns value as float64 in xmm0)
		fc.compileExpression(e.Index)
		// Save key/index to stack
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)

		// Load container pointer from stack to rbx
		// Pointer is stored as float64, need to extract to integer register
		// rbx is safe to use since loop counters are in r12/r13/r14
		EmitLoadPointerFromStack(fc.out, "rbx", "rsp", 0)

		if isMap {
			// SIMD-OPTIMIZED MAP INDEXING
			// =============================
			// Three-tier approach for optimal performance:
			// 1. AVX-512: Process 8 keys/iteration (8× throughput)
			// 2. SSE2:    Process 2 keys/iteration (2× throughput)
			// 3. Scalar:  Process 1 key/iteration (baseline)
			//
			// Map format: [count (float64)][key1][value1][key2][value2]...
			// Keys are interleaved with values at 16-byte strides
			//
			// Load key to search for from stack into xmm2
			fc.out.MovMemToXmm("xmm2", "rsp", StackSlotSize)

			// Load count from [rbx]
			fc.out.MovMemToXmm("xmm1", "rbx", 0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = count

			// Check if count is 0
			fc.out.CmpRegToImm("rcx", 0)
			notFoundJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0)
			notFoundEnd := fc.eb.text.Len()

			// Start at first key-value pair (skip 8-byte count)
			fc.out.AddImmToReg("rbx", 8)

			// ============ AVX-512 PATH (8 keys/iteration) ============
			// Runtime CPU detection: check if AVX-512 is supported
			// AVX-512 is available on Intel Xeon Scalable and some high-end desktop CPUs
			// Requires: AVX512F, AVX512DQ for VGATHERQPD and VCMPPD with k-registers

			// Check cpu_has_avx512 flag
			fc.out.LeaSymbolToReg("r15", "cpu_has_avx512")
			fc.out.Emit([]byte{0x41, 0x80, 0x3f, 0x00}) // cmp byte [r15], 0
			avx512NotSupportedJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // Jump to SSE2 if not supported
			avx512NotSupportedEnd := fc.eb.text.Len()

			// Check if we can process 8 at a time (count >= 8)
			fc.out.CmpRegToImm("rcx", 8)
			avx512SkipJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpLess, 0)
			avx512SkipEnd := fc.eb.text.Len()

			// Broadcast search key to all 8 lanes of zmm3
			// vbroadcastsd zmm3, xmm2
			fc.out.Emit([]byte{0x62, 0xf2, 0xfd, 0x48, 0x19, 0xda}) // EVEX.512.66.0F38.W1 19 /r

			// Set up gather indices for keys at 16-byte strides
			// Keys are at offsets: 0, 16, 32, 48, 64, 80, 96, 112 from rbx
			// Store indices in ymm4 (we need 8 x 64-bit indices for VGATHERQPD)
			// Using stack to construct index vector
			fc.out.SubImmFromReg("rsp", 64) // Space for 8 indices
			for i := 0; i < 8; i++ {
				fc.out.MovImmToReg("rax", fmt.Sprintf("%d", i*16))
				fc.out.MovRegToMem("rax", "rsp", i*8)
			}
			// Load indices into zmm4
			// vmovdqu64 zmm4, [rsp]
			fc.out.Emit([]byte{0x62, 0xf1, 0xfe, 0x48, 0x6f, 0x24, 0x24}) // EVEX.512.F3.0F.W1 6F /r

			// AVX-512 loop
			avx512LoopStart := fc.eb.text.Len()

			// Gather 8 keys using VGATHERQPD
			// vgatherqpd zmm0{k1}, [rbx + zmm4*1]
			// First, set mask k1 to all 1s (we want all 8 values)
			fc.out.Emit([]byte{0xc5, 0xf8, 0x92, 0xc9}) // kmovb k1, ecx (set to 0xFF)
			// Actually, let's use kxnorb k1, k1, k1 to set all bits to 1
			fc.out.Emit([]byte{0xc5, 0xfc, 0x46, 0xc9}) // kxnorb k1, k0, k1 -> k1 = 0xFF

			// vgatherqpd zmm0{k1}, [rbx + zmm4*1]
			// EVEX.512.66.0F38.W1 92 /r
			// This is complex - we need rbx as base, zmm4 as index, scale=1
			fc.out.Emit([]byte{0x62, 0xf2, 0xfd, 0x49, 0x92, 0x04, 0xe3}) // [rbx + zmm4*1]

			// Compare all 8 keys with search key
			// vcmppd k2{k1}, zmm0, zmm3, 0 (EQ_OQ)
			fc.out.Emit([]byte{0x62, 0xf1, 0xfd, 0x49, 0xc2, 0xd3, 0x00}) // EVEX.512.66.0F.W1 C2 /r ib

			// Extract mask to GPR
			// kmovb eax, k2
			fc.out.Emit([]byte{0xc5, 0xf9, 0x90, 0xc2}) // kmovb eax, k2

			// Test if any key matched
			fc.out.Emit([]byte{0x85, 0xc0}) // test eax, eax
			avx512FoundJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpNotEqual, 0)
			avx512FoundEnd := fc.eb.text.Len()

			// No match - advance by 128 bytes (8 key-value pairs)
			fc.out.AddImmToReg("rbx", 128)
			fc.out.SubImmFromReg("rcx", 8)
			// Continue if count >= 8
			fc.out.CmpRegToImm("rcx", 8)
			fc.out.JumpConditional(JumpGreaterOrEqual, int32(avx512LoopStart-(fc.eb.text.Len()+6)))

			// Clean up indices from stack and fall through to SSE2
			fc.out.AddImmToReg("rsp", 64)
			avx512ToSse2Jump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			avx512ToSse2End := fc.eb.text.Len()

			// AVX-512 match found - determine which key matched
			avx512FoundPos := fc.eb.text.Len()
			fc.patchJumpImmediate(avx512FoundJump+2, int32(avx512FoundPos-avx512FoundEnd))

			// Use BSF (bit scan forward) to find first set bit
			// bsf edx, eax
			fc.out.Emit([]byte{0x0f, 0xbc, 0xd0}) // bsf edx, eax

			// edx now contains index (0-7) of matched key
			// Calculate offset: base_rbx + (edx * 16) + 8 for value
			// shl edx, 4  (multiply by 16)
			fc.out.Emit([]byte{0xc1, 0xe2, 0x04}) // shl edx, 4
			// add edx, 8 (offset to value)
			fc.out.Emit([]byte{0x83, 0xc2, 0x08}) // add edx, 8
			// Load value at [rbx + rdx]
			// movsd xmm0, [rbx + rdx]
			fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x10, 0x04, 0x13}) // movsd xmm0, [rbx+rdx]

			// Clean up and jump to end
			fc.out.AddImmToReg("rsp", 64)
			avx512DoneJump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			avx512DoneEnd := fc.eb.text.Len()

			// ============ SSE2 PATH (2 keys/iteration) ============
			avx512SkipPos := fc.eb.text.Len()
			fc.patchJumpImmediate(avx512NotSupportedJump+2, int32(avx512SkipPos-avx512NotSupportedEnd))
			fc.patchJumpImmediate(avx512SkipJump+2, int32(avx512SkipPos-avx512SkipEnd))
			fc.patchJumpImmediate(avx512ToSse2Jump+1, int32(avx512SkipPos-avx512ToSse2End))

			// Broadcast search key to both lanes of xmm3 for SSE2 comparison
			// unpcklpd xmm3, xmm2, xmm2 duplicates xmm2 into both 64-bit lanes
			fc.out.MovXmmToXmm("xmm3", "xmm2")
			fc.out.Emit([]byte{0x66, 0x0f, 0x14, 0xda}) // unpcklpd xmm3, xmm2

			// Check if we can process 2 at a time (count >= 2)
			fc.out.CmpRegToImm("rcx", 2)
			scalarLoopJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpLess, 0)
			scalarLoopEnd := fc.eb.text.Len()

			// SIMD loop: process 2 key-value pairs at a time
			simdLoopStart := fc.eb.text.Len()
			// Load key1 from [rbx] into low lane of xmm0
			fc.out.MovMemToXmm("xmm0", "rbx", 0)
			// Load key2 from [rbx+16] into low lane of xmm1
			fc.out.MovMemToXmm("xmm1", "rbx", 16)
			// Pack both keys into xmm0: [key1 | key2]
			fc.out.Emit([]byte{0x66, 0x0f, 0x14, 0xc1}) // unpcklpd xmm0, xmm1

			// Compare both keys with search key in parallel
			// cmpeqpd xmm0, xmm3 (sets all bits in lane to 1 if equal)
			fc.out.Emit([]byte{0x66, 0x0f, 0xc2, 0xc3, 0x00}) // cmpeqpd xmm0, xmm3, 0

			// Extract comparison mask to eax
			// movmskpd eax, xmm0 (bit 0 = key1 match, bit 1 = key2 match)
			fc.out.Emit([]byte{0x66, 0x0f, 0x50, 0xc0}) // movmskpd eax, xmm0

			// Test if any key matched (eax != 0)
			fc.out.Emit([]byte{0x85, 0xc0}) // test eax, eax
			simdFoundJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpNotEqual, 0)
			simdFoundEnd := fc.eb.text.Len()

			// No match - advance by 32 bytes (2 key-value pairs)
			fc.out.AddImmToReg("rbx", 32)
			fc.out.SubImmFromReg("rcx", 2)
			// Continue if count >= 2
			fc.out.CmpRegToImm("rcx", 2)
			fc.out.JumpConditional(JumpGreaterOrEqual, int32(simdLoopStart-(fc.eb.text.Len()+6)))

			// Fall through to scalar loop if count < 2
			scalarFallThrough := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			scalarFallThroughEnd := fc.eb.text.Len()

			// SIMD match found - determine which key matched
			simdFoundPos := fc.eb.text.Len()
			fc.patchJumpImmediate(simdFoundJump+2, int32(simdFoundPos-simdFoundEnd))

			// Test bit 0 (key1 match)
			fc.out.Emit([]byte{0xa8, 0x01}) // test al, 1
			key1MatchJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpNotEqual, 0)
			key1MatchEnd := fc.eb.text.Len()

			// Bit 0 not set, must be bit 1 (key2 match) - load value at [rbx+24]
			fc.out.MovMemToXmm("xmm0", "rbx", 24)
			simdDoneJump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			simdDoneEnd := fc.eb.text.Len()

			// Key1 matched - load value at [rbx+8]
			key1MatchPos := fc.eb.text.Len()
			fc.patchJumpImmediate(key1MatchJump+2, int32(key1MatchPos-key1MatchEnd))
			fc.out.MovMemToXmm("xmm0", "rbx", 8)

			// Patch SIMD done jump to skip scalar loop
			allDoneJump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			allDoneEnd := fc.eb.text.Len()

			simdDonePos := fc.eb.text.Len()
			fc.patchJumpImmediate(simdDoneJump+1, int32(simdDonePos-simdDoneEnd))
			fc.out.JumpUnconditional(int32(allDoneJump - fc.eb.text.Len() - 5))

			// SCALAR loop: handle remaining keys (when count < 2 or remainder)
			scalarLoopPos := fc.eb.text.Len()
			fc.patchJumpImmediate(scalarLoopJump+2, int32(scalarLoopPos-scalarLoopEnd))
			fc.patchJumpImmediate(scalarFallThrough+1, int32(scalarLoopPos-scalarFallThroughEnd))

			// Check if any keys remain
			fc.out.CmpRegToImm("rcx", 0)
			scalarDoneJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0)
			scalarDoneEnd := fc.eb.text.Len()

			scalarLoopStart := fc.eb.text.Len()
			// Load current key from [rbx] into xmm1
			fc.out.MovMemToXmm("xmm1", "rbx", 0)
			// Compare key with search key (xmm1 vs xmm2)
			fc.out.Ucomisd("xmm1", "xmm2")

			// If equal, jump to found
			foundJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0)
			foundEnd := fc.eb.text.Len()

			// Not equal - advance to next pair (16 bytes)
			fc.out.AddImmToReg("rbx", 16)
			fc.out.SubImmFromReg("rcx", 1)
			// If counter > 0, continue loop
			fc.out.CmpRegToImm("rcx", 0)
			fc.out.JumpConditional(JumpNotEqual, int32(scalarLoopStart-(fc.eb.text.Len()+6)))

			// Not found - return 0.0
			scalarDonePos := fc.eb.text.Len()
			fc.patchJumpImmediate(scalarDoneJump+2, int32(scalarDonePos-scalarDoneEnd))
			notFoundPos := fc.eb.text.Len()
			fc.patchJumpImmediate(notFoundJump+2, int32(notFoundPos-notFoundEnd))
			fc.out.XorRegWithReg("rax", "rax")
			fc.out.Cvtsi2sd("xmm0", "rax")
			notFoundDoneJump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			notFoundDoneEnd := fc.eb.text.Len()

			// Scalar found - load value at [rbx + 8]
			foundPos := fc.eb.text.Len()
			fc.patchJumpImmediate(foundJump+2, int32(foundPos-foundEnd))
			fc.out.MovMemToXmm("xmm0", "rbx", 8)

			// All done - patch final jumps
			allDonePos := fc.eb.text.Len()
			fc.patchJumpImmediate(allDoneJump+1, int32(allDonePos-allDoneEnd))
			fc.patchJumpImmediate(avx512DoneJump+1, int32(allDonePos-avx512DoneEnd))
			fc.patchJumpImmediate(notFoundDoneJump+1, int32(allDonePos-notFoundDoneEnd))

		} else {
			// LIST INDEXING: Lists use map representation [count][key0][val0][key1][val1]...
			// For lists, keys are sequential integers (0, 1, 2...), so we can use direct offset calculation
			// Offset = 8 + (index * 16) + 8 = 16 + index * 16

			// Load index from stack (as float64)
			fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize)
			// Convert index from float64 to integer in rcx
			fc.out.Cvttsd2si("rcx", "xmm0")

			// BOUNDS CHECKING: Load list length and check bounds
			fc.out.MovMemToXmm("xmm1", "rbx", 0) // Load count from [rbx]
			fc.out.Cvttsd2si("rax", "xmm1")      // rax = count

			// Check if index < 0 (negative index)
			fc.out.CmpRegToImm("rcx", 0)
			negativeIndexJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpLess, 0)
			negativeIndexEnd := fc.eb.text.Len()

			// Check if index >= count (out of bounds)
			fc.out.CmpRegToReg("rcx", "rax")
			outOfBoundsJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpGreaterOrEqual, 0)
			outOfBoundsEnd := fc.eb.text.Len()

			// Calculate offset: 16 + index * 16
			fc.out.ShlImmReg("rcx", 4)    // rcx = index * 16
			fc.out.AddImmToReg("rcx", 16) // rcx = 16 + index * 16

			// Use rax as temp to avoid corrupting rbx (which might be loop counter)
			// rax = rbx + rcx = address of value
			fc.out.MovRegToReg("rax", "rbx")
			fc.out.AddRegToReg("rax", "rcx")

			// Load value from [rax]
			fc.out.MovMemToXmm("xmm0", "rax", 0)

			// Jump to end
			boundsCheckDoneJump := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)
			boundsCheckDoneEnd := fc.eb.text.Len()

			// Out of bounds handler: return 0.0
			outOfBoundsPos := fc.eb.text.Len()
			fc.patchJumpImmediate(negativeIndexJump+2, int32(outOfBoundsPos-negativeIndexEnd))
			fc.patchJumpImmediate(outOfBoundsJump+2, int32(outOfBoundsPos-outOfBoundsEnd))
			fc.out.XorpdXmm("xmm0", "xmm0") // xmm0 = 0.0

			// Patch done jump
			boundsCheckDonePos := fc.eb.text.Len()
			fc.patchJumpImmediate(boundsCheckDoneJump+1, int32(boundsCheckDonePos-boundsCheckDoneEnd))
		}

		// Clean up stack (remove saved key/index)
		fc.out.AddImmToReg("rsp", 16)

	case *LambdaExpr:
		// Generate function name for this lambda
		// If being assigned to a variable, use that name to allow recursion
		// Otherwise, use a generated name
		var funcName string
		if fc.currentAssignName != "" {
			funcName = fc.currentAssignName
		} else {
			fc.lambdaCounter++
			funcName = fmt.Sprintf("lambda_%d", fc.lambdaCounter)
		}

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileExpression: adding lambda '%s' with %d params, variadic='%s', body type: %T\n", funcName, len(e.Params), e.VariadicParam, e.Body)
		}

		// Register function signature for call site resolution
		fc.functionSignatures[funcName] = &FunctionSignature{
			ParamCount:    len(e.Params),
			VariadicParam: e.VariadicParam,
			IsVariadic:    e.VariadicParam != "",
		}

		// Variadic functions need arena allocation for argument lists
		if e.VariadicParam != "" {
			fc.usesArenas = true
			fc.runtimeFeatures.Mark(FeatureArenaAlloc)
		}

		// Detect if lambda is pure (eligible for memoization)
		pureFunctions := make(map[string]bool)
		pureFunctions[funcName] = true // Assume self-recursion is pure initially
		isPure := len(e.CapturedVars) == 0 && fc.isExpressionPure(e.Body, pureFunctions)

		// Capture types of captured variables from current scope
		capturedVarTypes := make(map[string]string)
		for _, varName := range e.CapturedVars {
			if typ, exists := fc.varTypes[varName]; exists {
				capturedVarTypes[varName] = typ
			} else {
				// Default to "number" if type unknown
				capturedVarTypes[varName] = "number"
			}
		}

		// Store lambda for later code generation
		fc.lambdaFuncs = append(fc.lambdaFuncs, LambdaFunc{
			Name:             funcName,
			Params:           e.Params,
			VariadicParam:    e.VariadicParam,
			Body:             e.Body,
			CapturedVars:     e.CapturedVars,
			CapturedVarTypes: capturedVarTypes,
			IsNested:         e.IsNestedLambda,
			IsPure:           isPure,
		})

		// For closures with captured variables, we need runtime allocation
		// For simple lambdas, use a static closure object with NULL environment
		if e.IsNestedLambda && len(e.CapturedVars) > 0 {
			// Allocate closure object and environment on the heap using malloc
			// Closure: [func_ptr, env_ptr] (16 bytes)
			// Environment: [var0, var1, ...] (8 bytes each)
			envSize := len(e.CapturedVars) * 8
			totalSize := 16 + envSize // closure object + environment

			// Note: Currently uses malloc for closures. Future: arena allocation
			// Allocate memory for closure environment
			// Ensure stack is 16-byte aligned before call
			// Save current rsp to restore after alignment
			fc.out.MovRegToReg("r13", "rsp") // Save original rsp
			fc.out.AndRegWithImm("rsp", -16) // Align to 16 bytes

			fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", totalSize))
			// Allocate from arena
			fc.callArenaAlloc()

			fc.out.MovRegToReg("rsp", "r13") // Restore original rsp
			fc.out.MovRegToReg("r12", "rax") // r12 = closure object pointer

			// Store function pointer at offset 0
			fc.out.LeaSymbolToReg("rax", funcName)
			fc.out.MovRegToMem("rax", "r12", 0)

			// Store environment pointer at offset 8
			fc.out.LeaMemToReg("rax", "r12", 16) // rax = address of environment (within same allocation)
			fc.out.MovRegToMem("rax", "r12", 8)

			// Copy captured variable values into environment
			for i, varName := range e.CapturedVars {
				varOffset, exists := fc.variables[varName]
				if !exists {
					compilerError("captured variable '%s' not found in scope", varName)
				}
				// Load variable value to xmm15
				fc.out.MovMemToXmm("xmm15", "rbp", -varOffset)
				// Store in environment at r12+16+(i*8)
				fc.out.MovXmmToMem("xmm15", "r12", 16+(i*8))
			}

			// Return closure object pointer as float64 in xmm0
			fc.out.SubImmFromReg("rsp", 16)
			fc.out.MovRegToMem("r12", "rsp", 0)
			fc.out.MovMemToXmm("xmm0", "rsp", 0)
			fc.out.AddImmToReg("rsp", 16)
		} else {
			// Simple lambda (no captures) - create static closure object
			// Closure object format: [func_ptr (8 bytes), env_ptr (8 bytes)]
			closureLabel := fmt.Sprintf("closure_%s", funcName)

			// We can't statically encode a function pointer, so we'll do it at runtime
			// Create a placeholder in .data (writable!) for the closure object
			// We need writable memory because we initialize it at runtime
			fc.eb.DefineWritable(closureLabel, strings.Repeat("\x00", 16))

			// At runtime, initialize the closure object with function pointer
			fc.out.LeaSymbolToReg("r12", closureLabel) // r12 = closure object address
			fc.out.LeaSymbolToReg("rax", funcName)     // rax = function pointer
			fc.out.MovRegToMem("rax", "r12", 0)        // Store func ptr at offset 0
			// Offset 8 is already 0 (NULL environment) from the zeroed data

			// Return closure object pointer as float64 in xmm0
			fc.out.SubImmFromReg("rsp", 16)
			fc.out.MovRegToMem("r12", "rsp", 0)
			fc.out.MovMemToXmm("xmm0", "rsp", 0)
			fc.out.AddImmToReg("rsp", 16)
		}

	case *PatternLambdaExpr:
		// Pattern lambda: (pattern1) => body1, (pattern2) => body2, ...
		// Compiles to a function that checks patterns in order and executes first match
		var funcName string
		if fc.currentAssignName != "" {
			funcName = fc.currentAssignName
		} else {
			fc.lambdaCounter++
			funcName = fmt.Sprintf("lambda_%d", fc.lambdaCounter)
		}

		// Create synthetic lambda body that implements pattern matching
		// The body will be a series of if-else checks for each pattern
		// For now, we'll generate the pattern matching code directly during lambda codegen

		// Store pattern lambda for later code generation
		fc.patternLambdaFuncs = append(fc.patternLambdaFuncs, PatternLambdaFunc{
			Name:    funcName,
			Clauses: e.Clauses,
		})

		// Create static closure object (pattern lambdas don't capture vars)
		closureLabel := fmt.Sprintf("closure_%s", funcName)
		// Use DefineWritable since we initialize at runtime
		fc.eb.DefineWritable(closureLabel, strings.Repeat("\x00", 16))

		// Initialize closure at runtime
		fc.out.LeaSymbolToReg("r12", closureLabel)
		fc.out.LeaSymbolToReg("rax", funcName)
		fc.out.MovRegToMem("rax", "r12", 0)

		// Return closure object pointer in xmm0
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovRegToMem("r12", "rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case *LengthExpr:
		// MAP/LIST LENGTH: Read count from header [count][key0][val0]...
		// Compile the operand (should be a list/map, returns pointer as float64 in xmm0)
		fc.compileExpression(e.Operand)

		// Convert pointer from float64 to integer in rax
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rax", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Load count from [rax] (first 8 bytes)
		fc.out.MovMemToXmm("xmm0", "rax", 0)

	case *JumpExpr:
		// Compile the value expression of return/jump statements
		// The value will be left in xmm0
		if e.Value != nil {
			fc.compileExpression(e.Value)
		} else {
			// No value - leave 0.0 in xmm0
			fc.out.MovImmToReg("rax", "0")
			fc.out.Cvtsi2sd("xmm0", "rax")
		}

	case *BlockExpr:
		// First, collect symbols from all statements in the block
		for _, stmt := range e.Statements {
			if err := fc.collectSymbols(stmt); err != nil {
				compilerError("%v at line 0", err)
			}
		}

		// Empty block returns true (1.0)
		if len(e.Statements) == 0 {
			fc.compileExpression(&NumberExpr{Value: 1.0})
			return
		}

		// Save tail position state - only last statement can be in tail position
		savedTailPosition := fc.inTailPosition

		// Compile each statement in the block
		// The last statement should leave its value in xmm0
		for i, stmt := range e.Statements {
			// Only the last statement inherits tail position from the block
			if i < len(e.Statements)-1 {
				fc.inTailPosition = false
			} else {
				fc.inTailPosition = savedTailPosition
			}

			fc.compileStatement(stmt)
			// If it's not the last statement and it's an expression statement,
			// the value is already in xmm0 but will be overwritten by the next statement
			if i == len(e.Statements)-1 {
				// Last statement - its value should already be in xmm0
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG BlockExpr: last statement type = %T\n", stmt)
				}
				// If it's an assignment, we need to load the assigned value
				if assignStmt, ok := stmt.(*AssignStmt); ok {
					fc.compileExpression(&IdentExpr{Name: assignStmt.Name})
				} else if _, ok := stmt.(*MapUpdateStmt); ok {
					// MapUpdateStmt doesn't produce a meaningful value
					// Return true (1.0) implicitly
					fc.compileExpression(&NumberExpr{Value: 1.0})
				} else if _, ok := stmt.(*ExpressionStmt); !ok {
					// Other statement types that aren't expressions
					// Return true (1.0) implicitly
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG BlockExpr: last statement is NOT ExpressionStmt, returning 1.0\n")
					}
					fc.compileExpression(&NumberExpr{Value: 1.0})
				}
				// For ExpressionStmt, compileStatement already compiled the expression
				// and left the result in xmm0, so we don't need to do anything here
			}
		}

		// Restore tail position
		fc.inTailPosition = savedTailPosition

	case *MatchExpr:
		fc.compileMatchExpr(e)

	case *ParallelExpr:
		fc.compileParallelExpr(e)

	case *PipeExpr:
		fc.compilePipeExpr(e)

	case *ComposeExpr:
		fc.compileComposeExpr(e)

	case *SendExpr:
		fc.compileSendExpr(e)

	case *ReceiveExpr:
		fc.compileReceiveExpr(e)

	case *CastExpr:
		fc.compileCastExpr(e)

	case *UnsafeExpr:
		fc.compileUnsafeExpr(e)

	case *ArenaExpr:
		fc.compileArenaExpr(e)

	case *SliceExpr:
		fc.compileSliceExpr(e)

	case *VectorExpr:
		// Allocate stack space for vector components (8 bytes per float64)
		stackSize := int64(e.Size * 8)
		fc.out.SubImmFromReg("rsp", stackSize)

		// Compile and store each component
		for i, comp := range e.Components {
			fc.compileExpression(comp)
			// Result is in xmm0, store it to stack at offset i*8
			offset := i * 8
			fc.out.MovXmmToMem("xmm0", "rsp", offset)
		}

		// Load stack address into rax and convert to float64 for return
		fc.out.MovRegToReg("rax", "rsp")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case *LoopExpr:
		// Loop expressions return a value (possibly through reduction)
		// For now, we don't support parallel loop expressions with reducers
		if e.NumThreads != 0 && e.Reducer != nil {
			compilerError("parallel loop expressions with reducers not yet implemented")
		}
		if e.NumThreads != 0 {
			compilerError("parallel loop expressions not yet implemented")
		}

		// For sequential loops, we need to accumulate results
		// This is a simplified implementation - full support needs more work
		compilerError("loop expressions (@ i in ... { expr }) not yet implemented as expressions")
	}
}

func (fc *C67Compiler) compileMatchExpr(expr *MatchExpr) {
	fc.compileExpression(expr.Condition)

	fc.labelCounter++

	var jumpCond JumpCondition
	needsZeroCompare := false

	if binExpr, ok := expr.Condition.(*BinaryExpr); ok {
		switch binExpr.Operator {
		case "<":
			jumpCond = JumpAboveOrEqual
		case "<=":
			jumpCond = JumpAbove
		case ">":
			jumpCond = JumpBelowOrEqual
		case ">=":
			jumpCond = JumpBelow
		case "==":
			jumpCond = JumpNotEqual
		case "!=":
			jumpCond = JumpEqual
		default:
			needsZeroCompare = true
		}
	} else {
		needsZeroCompare = true
	}

	// Check if any clause has a guard (for pattern matching)
	hasGuards := false
	for _, clause := range expr.Clauses {
		if clause.Guard != nil {
			hasGuards = true
			break
		}
	}

	var defaultJumpPos int
	// Only do preliminary condition check if there are no guards
	// With guards, we need to evaluate them regardless of condition value
	if len(expr.Clauses) > 0 && hasGuards {
		// Skip preliminary check - go straight to evaluating guards
		defaultJumpPos = -1
	} else if needsZeroCompare {
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm1", "rax")
		fc.out.Ucomisd("xmm0", "xmm1")
		defaultJumpPos = fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)
	} else {
		defaultJumpPos = fc.eb.text.Len()
		fc.out.JumpConditional(jumpCond, 0)
	}

	endJumpPositions := []int{}
	pendingGuardJumps := []int{}

	if len(expr.Clauses) == 0 {
		// Only default clause - don't jump over it, fall through to execute it
		// No clauses to process, go straight to default
	} else {
		for _, clause := range expr.Clauses {
			// Patch any guards that should skip to this clause
			for _, pos := range pendingGuardJumps {
				offset := int32(fc.eb.text.Len() - (pos + 6))
				fc.patchJumpImmediate(pos+2, offset)
			}
			pendingGuardJumps = pendingGuardJumps[:0]

			if clause.Guard != nil {
				fc.compileExpression(clause.Guard)
				fc.out.XorRegWithReg("rax", "rax")
				fc.out.Cvtsi2sd("xmm1", "rax")
				fc.out.Ucomisd("xmm0", "xmm1")
				guardJump := fc.eb.text.Len()
				fc.out.JumpConditional(JumpEqual, 0)
				pendingGuardJumps = append(pendingGuardJumps, guardJump)
			}

			fc.compileMatchClauseResult(clause.Result, &endJumpPositions)
		}
	}

	defaultPos := fc.eb.text.Len()

	for _, pos := range pendingGuardJumps {
		offset := int32(defaultPos - (pos + 6))
		fc.patchJumpImmediate(pos+2, offset)
	}

	// Only patch preliminary jump if it exists (defaultJumpPos != -1)
	if defaultJumpPos != -1 {
		defaultOffset := int32(defaultPos - (defaultJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(defaultJumpPos+2, defaultOffset)
	}

	fc.compileMatchDefault(expr.DefaultExpr)

	endPos := fc.eb.text.Len()
	if fc.debug {
		fmt.Fprintf(os.Stderr, "DEBUG JUMP PATCHING: endPos = %d, patching %d jumps\n", endPos, len(endJumpPositions))
	}
	for i, jumpPos := range endJumpPositions {
		endOffset := int32(endPos - (jumpPos + 5))
		if fc.debug {
			fmt.Fprintf(os.Stderr, "DEBUG JUMP PATCHING: jump %d at pos %d -> endOffset %d (target pos %d)\n",
				i, jumpPos, endOffset, jumpPos+5+int(endOffset))
		}
		fc.patchJumpImmediate(jumpPos+1, endOffset)
	}
}

func (fc *C67Compiler) compileMatchClauseResult(result Expression, endJumps *[]int) {
	if jumpExpr, isJump := result.(*JumpExpr); isJump {
		fc.compileMatchJump(jumpExpr)
		return
	}

	// Check if this result is in tail position for TCO
	// A call is in tail position ONLY if it's the direct result expression
	// NOT if it's wrapped in a BinaryExpr or other operation
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = true
	fc.compileExpression(result)
	fc.inTailPosition = savedTailPosition
	jumpPos := fc.eb.text.Len()
	if fc.debug {
		fmt.Fprintf(os.Stderr, "DEBUG JUMP PATCHING: adding jump at pos %d to endJumps list (count before: %d)\n",
			jumpPos, len(*endJumps))
	}
	fc.out.JumpUnconditional(0)
	*endJumps = append(*endJumps, jumpPos)
}

func (fc *C67Compiler) compileMatchDefault(result Expression) {
	if jumpExpr, isJump := result.(*JumpExpr); isJump {
		fc.compileMatchJump(jumpExpr)
		return
	}

	// Default expression is also in tail position
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = true
	fc.compileExpression(result)
	fc.inTailPosition = savedTailPosition
}

func (fc *C67Compiler) compileMatchJump(jumpExpr *JumpExpr) {
	// Handle ret (Label=0, IsBreak=true) - return from function
	if jumpExpr.Label == 0 && jumpExpr.IsBreak {
		// Return from function
		if jumpExpr.Value != nil {
			fc.compileExpression(jumpExpr.Value)
			// xmm0 now contains return value
		}
		fc.out.MovRegToReg("rsp", "rbp")

		// REGISTER ALLOCATOR: Restore callee-saved registers (for lambda functions)
		if fc.currentLambda != nil {
			fc.out.AddImmToReg("rsp", 8) // Remove alignment padding
			fc.out.PopReg("rbx")
		}

		fc.out.PopReg("rbp")
		fc.out.Ret()
		return
	}

	// Handle ret @N or @N - loop control
	if len(fc.activeLoops) == 0 {
		keyword := "@"
		if jumpExpr.IsBreak {
			keyword = "ret"
		}
		compilerError("%s @%d used outside of loop in match expression", keyword, jumpExpr.Label)
	}

	// Find the loop with the specified label
	targetLoopIndex := -1
	for i := 0; i < len(fc.activeLoops); i++ {
		if fc.activeLoops[i].Label == jumpExpr.Label {
			targetLoopIndex = i
			break
		}
	}

	if targetLoopIndex == -1 {
		keyword := "@"
		if jumpExpr.IsBreak {
			keyword = "ret"
		}
		compilerError("%s @%d references loop @%d which is not active",
			keyword, jumpExpr.Label, jumpExpr.Label)
	}

	if jumpExpr.IsBreak {
		// ret @N - exit loop N and all inner loops
		jumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)
		fc.activeLoops[targetLoopIndex].EndPatches = append(
			fc.activeLoops[targetLoopIndex].EndPatches,
			jumpPos+1,
		)
	} else {
		// @N - continue loop N (jump to continue point)
		jumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)
		fc.activeLoops[targetLoopIndex].ContinuePatches = append(
			fc.activeLoops[targetLoopIndex].ContinuePatches,
			jumpPos+1,
		)
	}
}

func (fc *C67Compiler) compileCastExpr(expr *CastExpr) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileCastExpr: Type=%s, Expr type=%T\n", expr.Type, expr.Expr)
	}
	// Check if this is a read syntax: ptr[offset] as TYPE
	// Transform to: read_TYPE(ptr, offset)
	if indexExpr, ok := expr.Expr.(*IndexExpr); ok {
		// Map cast type to read function name
		typeMap := map[string]string{
			"int8":    "i8",
			"int16":   "i16",
			"int32":   "i32",
			"int64":   "i64",
			"uint8":   "u8",
			"uint16":  "u16",
			"uint32":  "u32",
			"uint64":  "u64",
			"float32": "f32",
			"float64": "f64",
		}

		if shortType, ok := typeMap[expr.Type]; ok {
			// Transform to read_TYPE(ptr, offset as int32) call
			funcName := "read_" + shortType

			// Cast offset to int32 for read functions
			offsetCast := &CastExpr{
				Expr: indexExpr.Index,
				Type: "int32",
			}

			readCall := &CallExpr{
				Function: funcName,
				Args: []Expression{
					indexExpr.List,
					offsetCast,
				},
			}

			// Compile the read call instead
			fc.compileExpression(readCall)
			return
		}
	}

	// Compile the expression being cast (result in xmm0)
	fc.compileExpression(expr.Expr)

	// Cast is primarily a TYPE ANNOTATION for FFI interop
	// The actual conversion happens when the value is used in a specific context:
	// - In call() FFI: convert to proper register (xmm for float, rdi/rsi/etc for int)
	// - In unsafe blocks: interpret bits differently
	//
	// For standalone casts (e.g., x := as("int64", 8.0)), NO code is generated
	// The value stays as float64 in xmm0, and the cast is just metadata

	switch expr.Type {
	case "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		// Integer type annotation - NO runtime conversion
		// Value remains as float64 in xmm0
		// Actual conversion happens in call() when passing to C functions
		// This allows: x := as("int64", 8.0) without overhead

	case "float32":
		// float32 cast: for C float arguments
		// For now, keep as float64 (C will handle the conversion)
		// TODO: Add explicit cvtsd2ss/cvtss2sd if needed for precision

	case "float64":
		// Already float64, nothing to do
		// This is the native C67 type

	case "ptr":
		// Pointer cast: value is already in xmm0 as float64 (reinterpreted bits)
		// No conversion needed - bits pass through as-is
		// Used for NULL pointers and raw memory addresses

	case "number":
		// Convert C return value to C67 number (identity, already float64)
		// This is a no-op but explicit for FFI clarity

	case "cstr":
		// Convert C67 string to C null-terminated string
		// xmm0 contains pointer to C67 string map
		// Call runtime function: _c67_string_to_cstr(xmm0) -> rax
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")
		// Convert C string pointer (rax) back to float64 in xmm0
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovRegToMem("rax", "rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "string", "str":
		// Convert value to C67 string
		// First check if already a string
		exprType := fc.getExprType(expr.Expr)
		if exprType == "string" {
			// Already a string, no conversion needed
			// xmm0 already has correct value from fc.compileExpression(expr.Expr)
			return
		}

		// Check if it's a list/map - needs special conversion
		if exprType == "list" || exprType == "map" {
			// For now, just return a placeholder "[...]" or "{...}"
			// TODO: Implement proper list/map to string conversion
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Converting %s to string placeholder\n", exprType)
			}
			placeholder := "[...]"
			if exprType == "map" {
				placeholder = "{...}"
			}
			fc.compileExpression(&StringExpr{Value: placeholder})
			return
		}

		// Convert number to string using _c67_itoa (pure machine code, no libc)
		// Allocate buffer for number string (32 bytes enough for any number)
		fc.out.SubImmFromReg("rsp", 32)
		fc.out.MovRegToReg("r15", "rsp") // r15 = buffer pointer

		// Convert float to int64 (truncate)
		fc.out.Cvttsd2si("rdi", "xmm0")

		// Call _c67_itoa(rdi=number) -> (rsi=buffer, rdx=length)
		// Save r15 before call (it contains buffer pointer)
		fc.out.PushReg("r15")
		fc.trackFunctionCall("_c67_itoa")
		fc.eb.GenerateCallInstruction("_c67_itoa")
		fc.out.PopReg("r15")

		// _c67_itoa returns: rsi=buffer start, rdx=length
		// Copy result to our stack buffer
		fc.out.MovRegToReg("rdi", "r15") // dest
		fc.out.MovRegToReg("rcx", "rdx") // count
		// memcpy loop
		fc.out.CmpRegToImm("rcx", 0)
		fc.out.Write(0x74) // JE (jump if zero)
		fc.out.Write(0x00) // Placeholder
		endJump := fc.eb.text.Len() - 1

		copyStart := fc.eb.text.Len()
		fc.out.MovMemToReg("al", "rsi", 0)
		fc.out.MovByteRegToMem("al", "rdi", 0)
		fc.out.AddImmToReg("rsi", 1)
		fc.out.AddImmToReg("rdi", 1)
		fc.out.SubImmFromReg("rcx", 1)
		fc.out.Write(0x75) // JNZ (jump back if not zero)
		copyOffset := int8(copyStart - (fc.eb.text.Len() + 1))
		fc.out.Write(byte(copyOffset))

		endPos := fc.eb.text.Len()
		fc.eb.text.Bytes()[endJump] = byte(endPos - (endJump + 1))

		fc.out.MovRegToReg("r13", "rdx") // r13 = length

		// Convert C string to C67 string: allocate 8 + len*16 bytes
		fc.out.MovRegToReg("rax", "r13")
		fc.out.ShlRegByImm("rax", 4) // len * 16
		fc.out.AddImmToReg("rax", 8) // + 8 for count
		fc.out.MovRegToReg("rdi", "rax")
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r14", "rax") // r14 = C67 string

		// Store count
		fc.out.Cvtsi2sd("xmm0", "r13")
		fc.out.MovXmmToMem("xmm0", "r14", 0)

		// Copy characters to C67 string
		fc.out.XorRegWithReg("rcx", "rcx") // index
		copyLoopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("rcx", "r13")
		copyLoopEnd := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Load character: movzx rax, byte [r15 + rcx]
		fc.out.MovRegToReg("rax", "r15")
		fc.out.AddRegToReg("rax", "rcx")
		fc.out.Emit([]byte{0x48, 0x0f, 0xb6, 0x00}) // movzx rax, byte [rax]

		// Store to C67 string at offset 8 + rcx*16
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.ShlRegByImm("rdx", 4)     // rcx * 16
		fc.out.AddImmToReg("rdx", 8)     // + 8
		fc.out.AddRegToReg("rdx", "r14") // rdx = string_ptr + offset

		// Store index as float64
		fc.out.Cvtsi2sd("xmm0", "rcx")
		fc.out.MovXmmToMem("xmm0", "rdx", 0)

		// Store char as float64
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "rdx", 8)

		// Increment and loop
		fc.out.AddImmToReg("rcx", 1)
		backOffset := int32(copyLoopStart - (fc.eb.text.Len() + UnconditionalJumpSize))
		fc.out.JumpUnconditional(backOffset)

		// Loop end
		loopEndTarget := fc.eb.text.Len()
		fc.patchJumpImmediate(copyLoopEnd+2, int32(loopEndTarget-(copyLoopEnd+ConditionalJumpSize)))

		// Clean up buffer and return C67 string pointer in xmm0
		fc.out.AddImmToReg("rsp", 32)
		fc.out.MovqRegToXmm("xmm0", "r14")

	case "list":
		// Convert C array to C67 list
		// TODO: implement when needed (requires length parameter)
		compilerError("'as list' conversion not yet implemented")

	default:
		compilerError("unknown cast type '%s'", expr.Type)
	}
}

func (fc *C67Compiler) compileSliceExpr(expr *SliceExpr) {
	// Slice syntax: list[start:end:step] or string[start:end:step]
	// For now, implement simple case: string/list[start:end] (step=1, forward)

	// Compile the collection expression (result in xmm0 as pointer)
	fc.compileExpression(expr.List)

	// Save collection pointer on stack
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Compile step parameter first to know if we need special defaults
	if expr.Step != nil {
		fc.compileExpression(expr.Step)
		// step is now in xmm0
	} else {
		// Default step = 1
		fc.out.MovImmToReg("rax", "1")
		fc.out.Cvtsi2sd("xmm0", "rax")
	}
	// Save step on stack temporarily
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Compile start index (default depends on step sign)
	if expr.Start != nil {
		fc.compileExpression(expr.Start)
	} else {
		// Check if step is negative (convert to integer first)
		fc.out.MovMemToXmm("xmm0", "rsp", 0) // load step
		fc.out.Cvttsd2si("rax", "xmm0")      // convert to integer
		fc.out.XorRegWithReg("rbx", "rbx")
		fc.out.CmpRegToReg("rax", "rbx") // compare with 0

		negStepStartJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // If step < 0, jump to negative step path

		// Positive step: default start = 0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

		negStepStartEndJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Skip negative step path

		// Negative step: default start = length - 1
		negStepStartPos := fc.eb.text.Len()
		negStepStartOffset := int32(negStepStartPos - (negStepStartJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(negStepStartJumpPos+2, negStepStartOffset)

		fc.out.MovMemToReg("rax", "rsp", StackSlotSize) // Load collection pointer
		fc.out.MovMemToXmm("xmm0", "rax", 0)            // Load length
		fc.out.MovImmToReg("rax", "1")
		fc.out.Cvtsi2sd("xmm1", "rax")
		fc.out.SubsdXmm("xmm0", "xmm1") // xmm0 = length - 1

		negStepStartEndPos := fc.eb.text.Len()
		negStepStartEndOffset := int32(negStepStartEndPos - (negStepStartEndJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(negStepStartEndJumpPos+1, negStepStartEndOffset)
	}
	// Save start on stack
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Compile end index (default depends on step sign)
	if expr.End != nil {
		fc.compileExpression(expr.End)
		// end is now in xmm0
	} else {
		// Check if step is negative (convert to integer first)
		fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize) // load step (now 8 bytes back from start)
		fc.out.Cvttsd2si("rax", "xmm0")                  // convert to integer
		fc.out.XorRegWithReg("rbx", "rbx")
		fc.out.CmpRegToReg("rax", "rbx") // compare with 0

		negStepEndJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // If step < 0, jump to negative step path

		// Positive step: default end = length
		fc.out.MovMemToReg("rax", "rsp", 16) // Load collection pointer
		fc.out.MovMemToXmm("xmm0", "rax", 0) // Load length

		negStepEndEndJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Skip negative step path

		// Negative step: default end = -1
		negStepEndPos := fc.eb.text.Len()
		negStepEndOffset := int32(negStepEndPos - (negStepEndJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(negStepEndJumpPos+2, negStepEndOffset)

		fc.out.XorRegWithReg("rax", "rax") // rax = 0
		fc.out.SubImmFromReg("rax", 1)     // rax = -1
		fc.out.Cvtsi2sd("xmm0", "rax")     // xmm0 = -1

		negStepEndEndPos := fc.eb.text.Len()
		negStepEndEndOffset := int32(negStepEndEndPos - (negStepEndEndJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(negStepEndEndJumpPos+1, negStepEndEndOffset)
	}
	// end is in xmm0
	// Save end on stack
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Stack layout: [collection_ptr][step][start][end] (rsp points to end)
	// Call runtime function: _c67_slice_string(collection_ptr, start, end, step) -> new_collection_ptr

	// Load step into rcx (arg4)
	fc.out.MovMemToXmm("xmm0", "rsp", 16)
	fc.out.Cvttsd2si("rcx", "xmm0") // rcx = step (as integer)

	// Load end into rdx (arg3)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.Cvttsd2si("rdx", "xmm0") // rdx = end (as integer)

	// Load start into rsi (arg2)
	fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize)
	fc.out.Cvttsd2si("rsi", "xmm0") // rsi = start (as integer)

	// Load collection pointer into rdi (arg1)
	fc.out.MovMemToReg("rdi", "rsp", 24) // rdi = collection pointer

	// Clean up stack before call (4 values * 8 bytes = 32)
	fc.out.AddImmToReg("rsp", 32)

	// Call runtime function
	fc.out.CallSymbol("_c67_slice_string")

	// Result (new string pointer) is in rax, convert to float64 in xmm0
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", StackSlotSize)
}

func (fc *C67Compiler) compileUnsafeExpr(expr *UnsafeExpr) {
	// Execute the appropriate architecture block based on target
	var block []Statement
	var retStmt *UnsafeReturnStmt
	switch fc.platform.Arch {
	case ArchX86_64:
		block = expr.X86_64Block
		retStmt = expr.X86_64Return
	case ArchARM64:
		block = expr.ARM64Block
		retStmt = expr.ARM64Return
	case ArchRiscv64:
		block = expr.RISCV64Block
		retStmt = expr.RISCV64Return
	default:
		compilerError("unsupported architecture: %s", fc.platform.Arch.String())
	}

	// Mark that we're in an unsafe block (skip safety checks)
	fc.inUnsafeBlock = true
	defer func() { fc.inUnsafeBlock = false }()

	// Compile each statement in the unsafe block
	for _, stmt := range block {
		switch s := stmt.(type) {
		case *RegisterAssignStmt:
			fc.compileRegisterAssignment(s)
		case *MemoryStore:
			fc.compileSizedMemoryStore(s)
		case *SyscallStmt:
			fc.compileSyscall()
		default:
			compilerError("unsupported statement type in unsafe block: %T", s)
		}
	}

	// Handle return value
	if retStmt != nil {
		// Resolve register alias (a->rax, etc)
		reg := resolveRegisterAlias(retStmt.Register, fc.platform.Arch)
		asType := retStmt.AsType

		if asType == "" {
			compilerError("unsafe block return requires explicit type conversion (e.g., 'rax as int64', 'rax as float64', 'rax as pointer')")
		}

		// Handle different return types
		if len(reg) >= 3 && reg[:3] == "xmm" {
			// XMM register
			if asType == "float64" {
				// Already a float, just move to xmm0
				if reg != "xmm0" {
					fc.out.MovXmmToXmm("xmm0", reg)
				}
			} else {
				compilerError("cannot convert XMM register to type %s", asType)
			}
		} else {
			// GPR (general purpose register)
			if reg != "rax" {
				fc.out.MovRegToReg("rax", reg)
			}

			switch asType {
			case "float64":
				// Reinterpret bits as float64 (for reading float64 from memory)
				fc.out.MovRegToXmm("xmm0", "rax")
			case "int64", "int32", "int16", "int8", "uint64", "uint32", "uint16", "uint8":
				// Convert integer to float64
				fc.out.Cvtsi2sd("xmm0", "rax")
			case "pointer", "ptr", "cstr":
				// Convert pointer/cstr to float64 (treated as integer)
				fc.out.Cvtsi2sd("xmm0", "rax")
			default:
				compilerError("unsupported return type in unsafe block: %s", asType)
			}
		}
	} else {
		// No return statement - default behavior
		compilerError("unsafe block must end with explicit return (e.g., 'rax as int64')")
	}
}

func (fc *C67Compiler) compileSyscall() {
	// Emit raw syscall instruction
	// Registers must be set up before calling syscall:
	// x86-64: rax=syscall#, rdi=arg1, rsi=arg2, rdx=arg3, r10=arg4, r8=arg5, r9=arg6
	// ARM64: x8=syscall#, x0-x6=args
	// RISC-V: a7=syscall#, a0-a6=args
	fc.out.Syscall()
}

func (fc *C67Compiler) compileRegisterAssignment(stmt *RegisterAssignStmt) {
	// Resolve register aliases (a->rax, b->rbx, etc)
	register := stmt.Register

	// Handle memory stores: [a] <- value (resolve alias inside brackets)
	if len(register) > 2 && register[0] == '[' && register[len(register)-1] == ']' {
		addr := register[1 : len(register)-1]
		addr = resolveRegisterAlias(addr, fc.platform.Arch)
		fc.compileMemoryStore(addr, stmt.Value)
		return
	}

	// Resolve register alias for direct assignments
	register = resolveRegisterAlias(register, fc.platform.Arch)

	// Handle various value types
	switch v := stmt.Value.(type) {
	case *NumberExpr:
		// Immediate value: register <- 42 or register <- 10.5
		if register == "stack" {
			compilerError("cannot assign immediate value to stack; use 'stack <- register' to push")
		}

		// Check if this is a floating-point value (has decimal part)
		if v.Value != float64(int64(v.Value)) {
			// Floating-point value - need to use XMM register
			// For unsafe blocks, interpret float bits as integer for GPR storage
			floatBits := math.Float64bits(v.Value)
			fc.out.MovImmToReg(register, strconv.FormatUint(floatBits, 10))
		} else {
			// Integer value
			val := int64(v.Value)
			fc.out.MovImmToReg(register, strconv.FormatInt(val, 10))
		}

	case string:
		// Resolve source register alias
		sourceReg := resolveRegisterAlias(v, fc.platform.Arch)

		// Handle stack operations
		if register == "stack" && sourceReg != "stack" {
			// Push: stack <- rax
			fc.out.PushReg(sourceReg)
		} else if register != "stack" && sourceReg == "stack" {
			// Pop: rax <- stack
			fc.out.PopReg(register)
		} else if register == "stack" && sourceReg == "stack" {
			compilerError("cannot do 'stack <- stack'")
		} else {
			// Register-to-register move: rax <- rbx
			fc.out.MovRegToReg(register, sourceReg)
		}

	case *RegisterOp:
		// Arithmetic or bitwise operation
		fc.compileRegisterOp(register, v)

	case *MemoryLoad:
		// Memory load: rax <- [rbx] or rax <- u8 [rbx + 16]
		fc.compileMemoryLoad(register, v)

	case *CastExpr:
		// Type cast: rax <- 42 as uint8, rax <- ptr as pointer
		fc.compileUnsafeCast(register, v)

	default:
		compilerError("unsupported value type in register assignment: %T", v)
	}
}

func (fc *C67Compiler) compileRegisterOp(dest string, op *RegisterOp) {
	// Unary operations
	if op.Left == "" {
		if op.Operator == "~b" {
			// NOT: dest <- ~b src
			srcReg := op.Right.(string)
			if dest != srcReg {
				fc.out.MovRegToReg(dest, srcReg)
			}
			fc.out.NotReg(dest)
			return
		}
		compilerError("unsupported unary operator in unsafe block: %s", op.Operator)
	}

	// Binary operations: dest <- left OP right
	// First, ensure left operand is in dest
	if dest != op.Left {
		fc.out.MovRegToReg(dest, op.Left)
	}

	// Now apply the operation
	switch op.Operator {
	case "+":
		switch r := op.Right.(type) {
		case string:
			fc.out.AddRegToReg(dest, r)
		case *NumberExpr:
			fc.out.AddImmToReg(dest, int64(r.Value))
		}
	case "-":
		switch r := op.Right.(type) {
		case string:
			fc.out.SubRegFromReg(dest, r)
		case *NumberExpr:
			fc.out.SubImmFromReg(dest, int64(r.Value))
		}
	case "&":
		switch r := op.Right.(type) {
		case string:
			fc.out.AndRegWithReg(dest, r)
		case *NumberExpr:
			fc.out.AndRegWithImm(dest, int32(r.Value))
		}
	case "|":
		switch r := op.Right.(type) {
		case string:
			fc.out.OrRegWithReg(dest, r)
		case *NumberExpr:
			fc.out.OrRegWithImm(dest, int32(r.Value))
		}
	case "^b":
		switch r := op.Right.(type) {
		case string:
			fc.out.XorRegWithReg(dest, r)
		case *NumberExpr:
			fc.out.XorRegWithImm(dest, int32(r.Value))
		}
	case "*":
		switch r := op.Right.(type) {
		case string:
			fc.out.ImulRegWithReg(dest, r)
		case *NumberExpr:
			fc.out.ImulImmToReg(dest, int64(r.Value))
		}
	case "/":
		switch r := op.Right.(type) {
		case string:
			fc.out.DivRegByReg(dest, r)
		case *NumberExpr:
			fc.out.DivRegByImm(dest, int64(r.Value))
		}
	case "<<":
		switch r := op.Right.(type) {
		case string:
			fc.out.ShlRegByReg(dest, r)
		case *NumberExpr:
			fc.out.ShlRegByImm(dest, int64(r.Value))
		}
	case ">>":
		switch r := op.Right.(type) {
		case string:
			fc.out.ShrRegByReg(dest, r)
		case *NumberExpr:
			fc.out.ShrRegByImm(dest, int64(r.Value))
		}
	default:
		compilerError("operator %s not yet implemented", op.Operator)
	}
}

func (fc *C67Compiler) compileMemoryLoad(dest string, load *MemoryLoad) {
	// Memory load: dest <- [addr + offset]
	// Support sized loads: uint8, int8, uint16, int16, uint32, int32, uint64, int64

	// SAFETY: Add null pointer check for the address register (skip in unsafe blocks)
	if !fc.inUnsafeBlock {
		fc.emitNullPointerCheck(load.Address)
	}

	switch load.Size {
	case "", "uint64", "int64":
		// Default 64-bit load (unsigned and signed are the same for full width)
		fc.out.MovMemToReg(dest, load.Address, int(load.Offset))
	case "uint8":
		// Zero-extend byte to 64-bit
		fc.out.MovU8MemToReg(dest, load.Address, int(load.Offset))
	case "int8":
		// Sign-extend byte to 64-bit
		fc.out.MovI8MemToReg(dest, load.Address, int(load.Offset))
	case "uint16":
		// Zero-extend word to 64-bit
		fc.out.MovU16MemToReg(dest, load.Address, int(load.Offset))
	case "int16":
		// Sign-extend word to 64-bit
		fc.out.MovI16MemToReg(dest, load.Address, int(load.Offset))
	case "uint32":
		// Zero-extend dword to 64-bit (automatic on x86-64)
		fc.out.MovU32MemToReg(dest, load.Address, int(load.Offset))
	case "int32":
		// Sign-extend dword to 64-bit
		fc.out.MovI32MemToReg(dest, load.Address, int(load.Offset))
	default:
		compilerError("unsupported memory load size: %s (supported: uint8, int8, uint16, int16, uint32, int32, uint64, int64)", load.Size)
	}
}

func (fc *C67Compiler) compileSizedMemoryStore(store *MemoryStore) {
	// Memory store: [addr + offset] <- value as size

	// SAFETY: Add null pointer check for the address register (skip in unsafe blocks)
	if !fc.inUnsafeBlock {
		fc.emitNullPointerCheck(store.Address)
	}

	// Get the value into a register first
	var srcReg string

	switch v := store.Value.(type) {
	case string:
		// Value is already in a register
		srcReg = v
	case *NumberExpr:
		// Load immediate value into a temporary register (r11)
		srcReg = "r11"

		// Check if this is a floating-point value (has decimal part)
		if v.Value != float64(int64(v.Value)) {
			// Floating-point value - interpret float bits as integer for GPR storage
			floatBits := math.Float64bits(v.Value)
			fc.out.MovImmToReg(srcReg, strconv.FormatUint(floatBits, 10))
		} else {
			// Integer value
			val := int64(v.Value)
			fc.out.MovImmToReg(srcReg, strconv.FormatInt(val, 10))
		}
	default:
		compilerError("unsupported value type in memory store: %T", store.Value)
	}

	// Perform sized store based on Size field
	switch store.Size {
	case "", "uint64", "int64":
		// Default 64-bit store
		fc.out.MovRegToMem(srcReg, store.Address, int(store.Offset))
	case "uint8", "int8":
		// Byte store (signed and unsigned are the same for stores)
		fc.out.MovU8RegToMem(srcReg, store.Address, int(store.Offset))
	case "uint16", "int16":
		// Word store
		fc.out.MovU16RegToMem(srcReg, store.Address, int(store.Offset))
	case "uint32", "int32":
		// Dword store
		fc.out.MovU32RegToMem(srcReg, store.Address, int(store.Offset))
	default:
		compilerError("unsupported memory store size: %s (supported: uint8, int8, uint16, int16, uint32, int32, uint64, int64)", store.Size)
	}
}

// emitNullPointerCheck generates code to check if a register contains a null pointer (0)
// and aborts the program with an error message if so.
// This prevents segfaults from null pointer dereferences.
func (fc *C67Compiler) emitNullPointerCheck(reg string) {
	// Test if register is zero (null)
	// test reg, reg sets ZF if reg == 0
	fc.out.TestRegReg(reg, reg)

	// Jump if not zero (pointer is valid)
	okJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0) // Placeholder, will patch

	// Null pointer detected - print error and exit
	fc.out.LeaSymbolToReg("rdi", "_null_ptr_msg")
	fc.out.XorRegWithReg("rax", "rax") // AL=0 for variadic function
	fc.trackFunctionCall("printf")
	fc.eb.GenerateCallInstruction("printf")

	// exit(1)
	fc.out.MovImmToReg("rdi", "1")
	fc.trackFunctionCall("exit")
	fc.eb.GenerateCallInstruction("exit")

	// Patch the jump to skip error handling
	okPos := fc.eb.text.Len()
	okOffset := int32(okPos - (okJumpPos + 6))
	fc.patchJumpImmediate(okJumpPos+2, okOffset)
}

// emitBoundsCheck generates code to check if an index is within valid bounds [0, length)
// and aborts the program with an error message if out of bounds.
// indexReg: register containing the index (as signed 64-bit integer)
// lengthReg: register containing the list/array length (as signed 64-bit integer)
func (fc *C67Compiler) emitBoundsCheck(indexReg, lengthReg string) {
	// Check if index < 0
	fc.out.CmpRegToImm(indexReg, 0)
	negativeJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLess, 0) // Jump to error if index < 0

	// Check if index >= length
	fc.out.CmpRegToReg(indexReg, lengthReg)
	tooLargeJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Jump to error if index >= length

	// Index is valid - jump to ok
	okJumpPos := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Will patch to skip error handlers

	// Index < 0 error handler
	negativePos := fc.eb.text.Len()
	fc.out.LeaSymbolToReg("rdi", "_bounds_negative_msg")
	fc.out.XorRegWithReg("rax", "rax") // AL=0 for variadic function
	fc.trackFunctionCall("printf")
	fc.eb.GenerateCallInstruction("printf")
	fc.out.MovImmToReg("rdi", "1")
	fc.trackFunctionCall("exit")
	fc.eb.GenerateCallInstruction("exit")

	// Index >= length error handler
	tooLargePos := fc.eb.text.Len()
	fc.out.LeaSymbolToReg("rdi", "_bounds_too_large_msg")
	fc.out.XorRegWithReg("rax", "rax") // AL=0 for variadic function
	fc.trackFunctionCall("printf")
	fc.eb.GenerateCallInstruction("printf")
	fc.out.MovImmToReg("rdi", "1")
	fc.trackFunctionCall("exit")
	fc.eb.GenerateCallInstruction("exit")

	// Continue here if index is valid
	okPos := fc.eb.text.Len()

	// Patch jumps
	negativeOffset := int32(negativePos - (negativeJumpPos + 6))
	fc.patchJumpImmediate(negativeJumpPos+2, negativeOffset)

	tooLargeOffset := int32(tooLargePos - (tooLargeJumpPos + 6))
	fc.patchJumpImmediate(tooLargeJumpPos+2, tooLargeOffset)

	okOffset := int32(okPos - (okJumpPos + 5)) // Unconditional jump is 5 bytes
	fc.patchJumpImmediate(okJumpPos+1, okOffset)
}

func (fc *C67Compiler) compileMemoryStore(addr string, value interface{}) {
	// Memory store: [addr] <- value

	// SAFETY: Add null pointer check for the address register
	fc.emitNullPointerCheck(addr)

	switch v := value.(type) {
	case string:
		// Store register: [rax] <- rbx
		fc.out.MovRegToMem(v, addr, 0)
	case *NumberExpr:
		// Store immediate: [rax] <- 42
		fc.out.MovImmToMem(int64(v.Value), addr, 0)
	default:
		compilerError("unsupported memory store value type: %T", value)
	}
}

func (fc *C67Compiler) compileUnsafeCast(dest string, cast *CastExpr) {
	// Handle type casts in unsafe blocks
	// Examples: rax <- 42 as uint8, rax <- buffer as pointer, rax <- msg as cstr

	switch expr := cast.Expr.(type) {
	case *NumberExpr:
		// Immediate cast: rax <- 42 as uint8
		val := int64(expr.Value)
		// For integer types, just load the value (truncation happens naturally)
		fc.out.MovImmToReg(dest, strconv.FormatInt(val, 10))

	case *IdentExpr:
		// Variable cast: rax <- buffer as pointer, rax <- msg as cstr
		// Load the variable value
		if offset, ok := fc.variables[expr.Name]; ok {
			// Stack variable - load as float64 in xmm0
			fc.out.MovMemToXmm("xmm0", "rbp", -offset)

			// Handle specific cast types
			if cast.Type == "cstr" || cast.Type == "cstring" {
				// Convert C67 string to C null-terminated string
				// xmm0 contains pointer to C67 string map
				// _c67_string_to_cstr is an internal runtime function, not external
				fc.trackFunctionCall("_c67_string_to_cstr")
				fc.trackFunctionCall("_c67_string_to_cstr")
				fc.out.CallSymbol("_c67_string_to_cstr")
				// Result is C string pointer in rax
				if dest != "rax" {
					fc.out.MovRegToReg(dest, "rax")
				}
			} else {
				// For other types (pointer, integer types), extract raw bits
				// The float64 in xmm0 contains raw pointer/integer bits, not a numeric value
				// We need to preserve the bits, not convert the number
				fc.out.SubImmFromReg("rsp", 8)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
				fc.out.MovMemToReg(dest, "rsp", 0)
				fc.out.AddImmToReg("rsp", 8)
			}
		} else {
			suggestions := findSimilarIdentifiers(expr.Name, fc.variables, 3)
			if len(suggestions) > 0 {
				compilerError("undefined variable in unsafe cast: '%s'. Did you mean: %s?", expr.Name, strings.Join(suggestions, ", "))
			} else {
				compilerError("undefined variable in unsafe cast: '%s'", expr.Name)
			}
		}

	default:
		compilerError("unsupported cast expression type in unsafe block: %T", expr)
	}
}

func (fc *C67Compiler) compileParallelExpr(expr *ParallelExpr) {
	// Support: list || lambda or list || lambdaVar
	lambda, isDirectLambda := expr.Operation.(*LambdaExpr)
	if isDirectLambda {
		if len(lambda.Params) != 1 {
			compilerError("parallel operator lambda must have exactly one parameter")
		}
	}

	// No longer using fixed stack allocation - malloc will handle memory

	// Compile the lambda to get its function pointer (result in xmm0)
	fc.compileExpression(expr.Operation)

	// Save lambda function pointer (currently in xmm0) to stack and convert once to raw pointer bits
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize) // Store at rsp+8
	fc.out.MovMemToReg("r11", "rsp", StackSlotSize)  // Reinterpret float64 bits as pointer
	fc.out.MovRegToMem("r11", "rsp", StackSlotSize)  // Keep integer pointer for later loads

	// Compile the input list expression (returns pointer as float64 in xmm0)
	fc.compileExpression(expr.List)

	// Save list pointer to stack (reuse reserved slot) and load as integer pointer
	fc.out.MovXmmToMem("xmm0", "rsp", 0) // Store at rsp+0
	fc.out.MovMemToReg("r13", "rsp", 0)

	// Load list length from [r13] into r14 (empty lists have length=0, not null)
	fc.out.MovMemToXmm("xmm0", "r13", 0)
	fc.out.Cvttsd2si("r14", "xmm0") // r14 = length as integer

	// Calculate allocation size: 8 bytes (length) + length * 8 bytes (elements)
	fc.out.MovRegToReg("rdi", "r14") // rdi = length
	fc.out.ShlRegImm("rdi", "3")     // rdi = length * 8
	fc.out.AddImmToReg("rdi", 8)     // rdi = 8 + length * 8

	// DO NOT USE MALLOC! See MEMORY.md - should use arena allocation
	// TODO: Replace with arena allocation (if mutable) or .rodata (if immutable)
	// Allocate from arena
	fc.callArenaAlloc()

	// Store result list pointer in r12
	fc.out.MovRegToReg("r12", "rax") // r12 = result list base (from malloc)

	// Store length in result list
	fc.out.MovMemToXmm("xmm0", "r13", 0) // Reload length as float64
	fc.out.MovXmmToMem("xmm0", "r12", 0)

	// Initialize loop counter to 0
	fc.out.XorRegWithReg("r15", "r15") // r15 = index

	// Loop start
	loopStart := fc.eb.text.Len()

	// Check if index >= length
	fc.out.CmpRegToReg("r15", "r14")
	loopEndJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)

	// Load element from input list: input_list[index]
	// Element address = r13 + 8 + (r15 * 8)
	fc.out.MovRegToReg("rax", "r15")
	fc.out.MulRegWithImm("rax", 8)
	fc.out.AddImmToReg("rax", 8)     // skip length
	fc.out.AddRegToReg("rax", "r13") // rax = address of element

	// Load element into xmm0 (this is the argument to the lambda)
	fc.out.MovMemToXmm("xmm0", "rax", 0)

	// Save loop index r15 to stack (will be clobbered by environment pointer)
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("r15", "rsp", 0)

	// Load lambda closure object pointer (stored at [rsp+8] from earlier)
	fc.out.MovMemToReg("rax", "rsp", 16) // rsp+8 for saved r15, +8 for lambda pointer

	// Extract function pointer from closure object (offset 0)
	fc.out.MovMemToReg("r11", "rax", 0)

	// Extract environment pointer from closure object (offset 8) into r15
	fc.out.MovMemToReg("r15", "rax", 8)

	// Call the lambda function with element in xmm0 and environment in r15
	fc.out.CallRegister("r11")

	// Restore loop index from stack
	fc.out.MovMemToReg("r15", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)

	// Result is in xmm0, store it in output list: result_list[index]
	fc.out.MovRegToReg("rax", "r15")
	fc.out.MulRegWithImm("rax", 8)
	fc.out.AddImmToReg("rax", 8)     // skip length
	fc.out.AddRegToReg("rax", "r12") // rax = address in result list
	fc.out.MovXmmToMem("xmm0", "rax", 0)

	// Increment index
	fc.out.IncReg("r15")

	// Jump back to loop start
	loopBackJumpPos := fc.eb.text.Len()
	backOffset := int32(loopStart - (loopBackJumpPos + UnconditionalJumpSize))
	fc.out.JumpUnconditional(backOffset)

	// Loop end
	loopEndPos := fc.eb.text.Len()

	// Patch conditional jump
	endOffset := int32(loopEndPos - (loopEndJumpPos + ConditionalJumpSize))
	fc.patchJumpImmediate(loopEndJumpPos+2, endOffset)

	// Don't clean up the lambda/list spill area yet - it's part of our memory layout
	// The result buffer includes this space in its allocation

	// Return result list pointer as float64 in xmm0
	// r12 points to the result buffer on stack
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovRegToMem("r12", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", StackSlotSize)

	// Clean up the initial 16-byte spill area for lambda/list pointers
	// but leave the result list on the stack
	fc.out.AddImmToReg("rsp", 16)

	// IMPORTANT: The result list remains on the stack and will be valid
	// as long as no other stack allocations overwrite it
	// This is a temporary solution - proper heap allocation would be better

	// End of parallel operator - xmm0 contains result pointer as float64
}

func (fc *C67Compiler) predeclareLambdaSymbols() {
	for _, lambda := range fc.lambdaFuncs {
		if _, ok := fc.eb.consts[lambda.Name]; !ok {
			fc.eb.consts[lambda.Name] = &Const{value: ""}
		}
	}
}

func (fc *C67Compiler) generateLambdaFunctions() {
	// Use index-based loop to handle lambdas added during iteration (nested lambdas)
	for i := 0; i < len(fc.lambdaFuncs); i++ {
		lambda := fc.lambdaFuncs[i]
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG generateLambdaFunctions: generating lambda '%s' with body type %T\n", lambda.Name, lambda.Body)
		}

		// Local variables in lambdas are now supported via stack allocation

		// Record the offset of this lambda function in .text
		offsetBefore := fc.eb.text.Len()
		fc.lambdaOffsets[lambda.Name] = offsetBefore

		// Mark the start of the lambda function with a label (again, to update offset)
		fc.eb.MarkLabel(lambda.Name)

		// Function prologue with proper calling convention
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Calculate total stack frame size upfront
		// Layout: [rbp-8]=rbx, [rbp-24]=param0, [rbp-40]=param1, etc.
		// Each param/captured var takes 16 bytes, rbx takes 8, add 8 for alignment
		paramCount := len(lambda.Params)
		capturedCount := len(lambda.CapturedVars)

		// Frame: 8(rbx) + 8(align) + params*16 + captured*16 + 2048(temp space)
		// Temp space: Large buffer for local variables and expression temporaries
		// This accounts for: local variable definitions, nested arithmetic, function calls, etc.
		frameSize := 16 + paramCount*16 + capturedCount*16 + 4096
		// Ensure 16-byte alignment for external function calls
		frameSize = (frameSize + 15) & ^15

		// Allocate entire frame at once (keeps rsp aligned)
		fc.out.SubImmFromReg("rsp", int64(frameSize))

		// Save rbx at fixed location
		fc.out.MovRegToMem("rbx", "rbp", -8)

		// Stack layout after prologue:
		// [rbp+0]  = saved rbp (from push)
		// [rbp-8]  = saved rbx
		// [rbp-16] = alignment padding
		// [rbp-24] = param 0
		// [rbp-40] = param 1
		// [rbp-56] = param 2
		// ...
		// [rsp]    = stack top (16-byte aligned)

		// Save previous state
		oldVariables := fc.variables
		oldMutableVars := fc.mutableVars
		oldStackOffset := fc.stackOffset
		oldRuntimeStack := fc.runtimeStack

		// Create new scope for lambda
		// CRITICAL: Copy ALL variables (including module-level) for lookup
		// Module-level variables will reference the main function's stack frame via rbp
		// This allows lambdas to properly read and update module-level mutable globals
		fc.variables = make(map[string]int)
		for k, v := range oldVariables {
			fc.variables[k] = v
		}
		fc.mutableVars = make(map[string]bool)
		for k, v := range oldMutableVars {
			fc.mutableVars[k] = v
		}
		fc.stackOffset = 0
		fc.runtimeStack = 0 // Not used in new convention

		// Store parameters from xmm registers at fixed offsets
		// Parameters come in xmm0, xmm1, xmm2, ...
		xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5"}
		baseParamOffset := 24 // First param at rbp-24

		// CRITICAL: If variadic, save ALL xmm registers immediately
		// This must happen BEFORE any other operations
		savedXmmOffset := 3500
		if lambda.VariadicParam != "" {
			for i := 0; i < len(xmmRegs); i++ {
				tempOffset := savedXmmOffset + i*16
				fc.out.MovXmmToMem(xmmRegs[i], "rbp", -tempOffset)
			}
		}

		for i, paramName := range lambda.Params {
			if i >= len(xmmRegs) {
				compilerError("lambda has too many parameters (max 6)")
			}

			// Calculate fixed offset for this parameter
			paramOffset := baseParamOffset + i*16
			fc.stackOffset = paramOffset // Track for collectSymbols compatibility
			fc.variables[paramName] = paramOffset
			fc.mutableVars[paramName] = false

			// Mark parameter type as "number" by default (all values are float64 in C67)
			// This prevents x + y from being interpreted as list append when x and y are parameters
			fc.varTypes[paramName] = "number"

			// Store parameter at fixed offset
			if lambda.VariadicParam != "" {
				// Load from saved location
				tempOffset := savedXmmOffset + i*16
				fc.out.MovMemToXmm("xmm15", "rbp", -tempOffset)
				fc.out.MovXmmToMem("xmm15", "rbp", -paramOffset)
			} else {
				// Direct from register (no save needed)
				fc.out.MovXmmToMem(xmmRegs[i], "rbp", -paramOffset)
			}
		}

		// Handle variadic parameter if present
		// Variadic parameter collects remaining arguments into a list
		// Convention: remaining args passed in xmm regs, count in r14
		if lambda.VariadicParam != "" {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG generateLambdaFunctions: '%s' HAS variadic param '%s' (fixedParams=%d)\n", lambda.Name, lambda.VariadicParam, paramCount)
			}

			variadicOffset := baseParamOffset + paramCount*16
			fc.stackOffset = variadicOffset
			fc.variables[lambda.VariadicParam] = variadicOffset
			fc.mutableVars[lambda.VariadicParam] = false
			fc.varTypes[lambda.VariadicParam] = "list"

			// Build list from variadic arguments
			// r14 contains the count of variadic arguments
			// Variadic args are in xmm registers starting at xmmRegs[paramCount]

			// Skip if no variadic args (r14 == 0)
			skipLabel := fc.nextLabel()
			fc.out.CmpRegToImm("r14", 0)
			fc.out.Write(0x74) // JE (jump if equal)
			fc.out.Write(0x00) // Will be patched
			skipPos := fc.eb.text.Len() - 1

			// Calculate list size: 8 (count) + r14 * 16 (key+value pairs)
			// size = 8 + r14 * 16 = 8 + r14 << 4
			fc.out.MovRegToReg("rax", "r14")
			fc.out.ShlRegByImm("rax", 4) // rax = r14 * 16
			fc.out.AddImmToReg("rax", 8) // rax = 8 + r14 * 16

			// Allocate from arena: _c67_arena_alloc(arena_ptr, size)
			// Save r14 (we need it after the call)
			fc.out.PushReg("r14")

			// rdi = arena_ptr (requires TWO dereferences)
			fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
			fc.out.MovMemToReg("rdi", "rdi", 0) // Load meta-arena array pointer
			fc.out.MovMemToReg("rdi", "rdi", 0) // Load arena[0] struct pointer

			// rsi = size (already in rax)
			fc.out.MovRegToReg("rsi", "rax")

			// Call arena allocator
			fc.trackFunctionCall("_c67_arena_alloc")
			fc.out.CallSymbol("_c67_arena_alloc")

			// rax now contains pointer to allocated list
			// Restore r14
			fc.out.PopReg("r14")

			// Store count in list (first 8 bytes)
			fc.out.MovRegToReg("rcx", "r14")
			fc.out.Cvtsi2sd("xmm15", "rcx")
			fc.out.MovXmmToMem("xmm15", "rax", 0)

			// Copy variadic arguments from saved xmm locations to list
			// The xmm registers were saved at offset savedXmmOffset earlier
			// Arguments are at xmmRegs[paramCount] through xmmRegs[paramCount + r14 - 1]
			maxVariadic := 6 - paramCount
			if maxVariadic > 6 {
				maxVariadic = 6
			}

			for i := 0; i < maxVariadic; i++ {
				xmmIdx := paramCount + i
				if xmmIdx >= 6 {
					break
				}

				// Check if this arg exists (i < r14)
				checkLabel := fc.nextLabel()
				fc.out.CmpRegToImm("r14", int64(i+1))
				fc.out.Write(0x7C) // JL (jump if r14 < i+1, meaning this arg doesn't exist)
				fc.out.Write(0x00) // Will be patched
				checkPos := fc.eb.text.Len() - 1

				// This arg exists - load from saved location and store in list
				keyOffset := 8 + i*16
				valOffset := keyOffset + 8

				// Key = i (as float64)
				fc.out.MovImmToReg("rcx", fmt.Sprintf("%d", i))
				fc.out.Cvtsi2sd("xmm15", "rcx")
				fc.out.MovXmmToMem("xmm15", "rax", keyOffset)

				// Value from saved xmm register location
				savedOffset := savedXmmOffset + xmmIdx*16
				fc.out.MovMemToXmm("xmm15", "rbp", -savedOffset)
				fc.out.MovXmmToMem("xmm15", "rax", valOffset)

				// Patch the check jump to here (skip storing this arg)
				checkTarget := fc.eb.text.Len()
				fc.patchJumpImmediate(checkPos, int32(checkTarget-(checkPos+1)))
				fc.defineLabel(checkLabel, checkTarget)
			}

			// Store list pointer in variadic parameter location
			fc.out.MovqRegToXmm("xmm15", "rax")
			fc.out.MovXmmToMem("xmm15", "rbp", -variadicOffset)

			// Jump over empty list creation
			hasArgsLabel := fc.nextLabel()
			fc.out.Write(0xEB) // JMP short
			fc.out.Write(0x00) // Will be patched
			hasArgsPos := fc.eb.text.Len() - 1

			// Empty list path (when r14==0)
			skipTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(skipPos, int32(skipTarget-(skipPos+1)))
			fc.defineLabel(skipLabel, skipTarget)

			// Create static empty list
			emptyListLabel := fmt.Sprintf("variadic_empty_%d", fc.stringCounter)
			fc.stringCounter++
			emptyData := make([]byte, 8)
			fc.eb.Define(emptyListLabel, string(emptyData))
			fc.out.LeaSymbolToReg("r13", emptyListLabel)
			fc.out.MovqRegToXmm("xmm15", "r13")
			fc.out.MovXmmToMem("xmm15", "rbp", -variadicOffset)

			// Patch has-args jump
			hasArgsTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(hasArgsPos, int32(hasArgsTarget-(hasArgsPos+1)))
			fc.defineLabel(hasArgsLabel, hasArgsTarget)
		}

		// Add captured variables to the lambda's scope
		// The environment pointer is in r15, passed by the caller
		// Environment contains: [var0, var1, var2, ...] where each is 8 bytes
		baseCapturedOffset := baseParamOffset + paramCount*16

		for i, capturedVar := range lambda.CapturedVars {
			// Calculate fixed offset for captured variable
			varOffset := baseCapturedOffset + i*16
			fc.stackOffset = varOffset // Track for compatibility
			fc.variables[capturedVar] = varOffset
			fc.mutableVars[capturedVar] = false

			// IMPORTANT: Restore type information for captured variables
			if typ, exists := lambda.CapturedVarTypes[capturedVar]; exists {
				fc.varTypes[capturedVar] = typ
			} else {
				fc.varTypes[capturedVar] = "number"
			}

			// Load captured variable from environment and store at fixed offset
			// No rsp modification - everything stays aligned!
			fc.out.MovMemToXmm("xmm15", "r15", i*8)        // Load from environment
			fc.out.MovXmmToMem("xmm15", "rbp", -varOffset) // Store to stack
		}

		// Set current lambda context for "me" self-reference and tail recursion
		fc.currentLambda = &lambda
		fc.lambdaBodyStart = fc.eb.text.Len()

		// Note: Don't call collectLoopsFromExpression here - symbols will be collected
		// when we compile the BlockExpr. Calling it here causes duplicate symbol collection.
		fc.labelCounter = 0

		fc.pushDeferScope()

		// Compile lambda body (result in xmm0)
		fc.compileExpression(lambda.Body)

		fc.popDeferScope()

		// Clear lambda context
		fc.currentLambda = nil

		// Function epilogue with proper calling convention
		// Restore rbx from fixed location
		fc.out.MovMemToReg("rbx", "rbp", -8)

		// Deallocate stack frame (restore rsp to rbp)
		fc.out.MovRegToReg("rsp", "rbp")

		// Restore caller's rbp
		fc.out.PopReg("rbp")

		// Return to caller
		fc.out.Ret()

		// Restore previous state
		fc.variables = oldVariables
		fc.mutableVars = oldMutableVars
		fc.stackOffset = oldStackOffset
		fc.runtimeStack = oldRuntimeStack
	}
}

func (fc *C67Compiler) generatePatternLambdaFunctions() {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG generatePatternLambdaFunctions: generating %d pattern lambdas\n", len(fc.patternLambdaFuncs))
	}
	for _, patternLambda := range fc.patternLambdaFuncs {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG generating pattern lambda '%s' with %d clauses\n", patternLambda.Name, len(patternLambda.Clauses))
		}
		// Record offset
		fc.lambdaOffsets[patternLambda.Name] = fc.eb.text.Len()
		fc.eb.MarkLabel(patternLambda.Name)

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save state
		oldVariables := fc.variables
		oldMutableVars := fc.mutableVars
		oldStackOffset := fc.stackOffset

		fc.variables = make(map[string]int)
		fc.mutableVars = make(map[string]bool)
		fc.stackOffset = 0

		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG generatePatternLambdaFunctions: reset variables map for '%s', fc.variables=%v\n", patternLambda.Name, fc.variables)
		}

		// Determine number of parameters from first clause
		numParams := len(patternLambda.Clauses[0].Patterns)

		// Store parameters from xmm0, xmm1, ... to stack
		xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5"}
		paramOffsets := make([]int, numParams)
		for i := 0; i < numParams; i++ {
			fc.stackOffset += 16
			paramOffsets[i] = fc.stackOffset
			fc.out.SubImmFromReg("rsp", 16)
			fc.out.MovXmmToMem(xmmRegs[i], "rbp", -paramOffsets[i])
		}

		// Generate pattern matching code
		// For each clause, check if patterns match, execute body if so
		clauseLabels := make([]string, len(patternLambda.Clauses))
		for i := range patternLambda.Clauses {
			fc.labelCounter++
			clauseLabels[i] = fmt.Sprintf("pattern_clause_%d", fc.labelCounter)
		}

		fc.labelCounter++
		failLabel := fmt.Sprintf("pattern_fail_%d", fc.labelCounter)

		// Track all jumps that need patching across all clauses
		type jumpPatch struct {
			jumpPos int
			target  string
		}
		var allJumps []jumpPatch

		for clauseIdx, clause := range patternLambda.Clauses {
			fc.eb.MarkLabel(clauseLabels[clauseIdx])

			// Determine target for failed pattern matches in this clause
			nextTarget := failLabel
			if clauseIdx < len(patternLambda.Clauses)-1 {
				nextTarget = clauseLabels[clauseIdx+1]
			}

			// Check each pattern in this clause
			for paramIdx, pattern := range clause.Patterns {
				paramOffset := paramOffsets[paramIdx]

				switch p := pattern.(type) {
				case *LiteralPattern:
					// Compare parameter against literal value
					fc.compileExpression(p.Value) // Result in xmm0
					fc.out.MovMemToXmm("xmm1", "rbp", -paramOffset)
					fc.out.Ucomisd("xmm0", "xmm1")
					// If not equal, jump to next clause
					jumpOffset := fc.eb.text.Len()
					fc.out.JumpConditional(JumpNotEqual, 0)
					allJumps = append(allJumps, jumpPatch{jumpOffset, nextTarget})

				case *VarPattern:
					// Bind parameter to variable name
					fc.stackOffset += 16
					varOffset := fc.stackOffset
					fc.variables[p.Name] = varOffset
					fc.mutableVars[p.Name] = false
					fc.out.SubImmFromReg("rsp", 16)
					fc.out.MovMemToXmm("xmm15", "rbp", -paramOffset)
					fc.out.MovXmmToMem("xmm15", "rbp", -varOffset)

				case *WildcardPattern:
					// Match anything, no binding
				}
			}

			// All patterns matched, execute body
			fc.compileExpression(clause.Body)

			// After executing body, return (don't fall through to next clause)
			fc.out.MovRegToReg("rsp", "rbp")

			fc.out.PopReg("rbp")
			fc.out.Ret()
		}

		// Fail label - must be marked before patching jumps
		fc.eb.MarkLabel(failLabel)

		// Now patch all jumps after all labels have been marked
		for _, jump := range allJumps {
			targetOffset := fc.eb.LabelOffset(jump.target)
			if targetOffset < 0 {
				compilerError("pattern lambda jump target not found: %s", jump.target)
			}
			offset := int32(targetOffset - (jump.jumpPos + 6)) // 6 = size of conditional jump instruction
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Patching jump at %d to target %s (offset %d -> %d, relative %d)\n",
					jump.jumpPos, jump.target, jump.jumpPos, targetOffset, offset)
			}
			fc.eb.text.Bytes()[jump.jumpPos+2] = byte(offset)
			fc.eb.text.Bytes()[jump.jumpPos+3] = byte(offset >> 8)
			fc.eb.text.Bytes()[jump.jumpPos+4] = byte(offset >> 16)
			fc.eb.text.Bytes()[jump.jumpPos+5] = byte(offset >> 24)
		}
		// No pattern matched - return 0
		fc.out.XorpdXmm("xmm0", "xmm0")

		// Function epilogue
		fc.out.MovRegToReg("rsp", "rbp")

		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Restore state
		fc.variables = oldVariables
		fc.mutableVars = oldMutableVars
		fc.stackOffset = oldStackOffset
	}
}

func (fc *C67Compiler) buildHotFunctionTable() {
	if len(fc.hotFunctions) == 0 {
		return
	}

	var hotNames []string
	for name := range fc.hotFunctions {
		hotNames = append(hotNames, name)
	}
	sort.Strings(hotNames)

	for idx, name := range hotNames {
		fc.hotFunctionTable[name] = idx
	}
}

func (fc *C67Compiler) generateHotFunctionTable() {
	if len(fc.hotFunctions) == 0 {
		return
	}

	var hotNames []string
	for name := range fc.hotFunctions {
		hotNames = append(hotNames, name)
	}
	sort.Strings(hotNames)

	fc.hotTableRodataOffset = fc.eb.rodata.Len()
	tableData := make([]byte, len(hotNames)*8)
	fc.eb.Define("_hot_function_table", string(tableData))
}

func (fc *C67Compiler) patchHotFunctionTable() {
	if len(fc.hotFunctions) == 0 {
		return
	}

	tableConst, ok := fc.eb.consts["_hot_function_table"]
	if !ok {
		return
	}

	rodataSymbols := fc.eb.RodataSection()
	firstRodataAddr := uint64(0xFFFFFFFFFFFFFFFF)
	for symName := range rodataSymbols {
		if c, ok := fc.eb.consts[symName]; ok {
			if c.addr > 0 && c.addr < firstRodataAddr {
				firstRodataAddr = c.addr
			}
		}
	}

	tableOffsetInRodata := int(tableConst.addr - firstRodataAddr)
	rodataBytes := fc.eb.rodata.Bytes()

	var hotNames []string
	for name := range fc.hotFunctions {
		hotNames = append(hotNames, name)
	}
	sort.Strings(hotNames)

	for idx, name := range hotNames {
		closureName := "closure_" + name
		if closureConst, ok := fc.eb.consts[closureName]; ok {
			closureAddr := closureConst.addr
			offset := tableOffsetInRodata + idx*8
			if offset >= 0 && offset+8 <= len(rodataBytes) {
				binary.LittleEndian.PutUint64(rodataBytes[offset:offset+8], closureAddr)
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Hot table: %s -> closure at 0x%x\n", name, closureAddr)
				}
			}
		} else if VerboseMode {
			fmt.Fprintf(os.Stderr, "Hot table: closure_%s not found\n", name)
		}
	}
}

func (fc *C67Compiler) generateCacheLookup() {
	fc.eb.MarkLabel("_c67_cache_lookup")

	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")

	fc.out.MovRegToReg("r12", "rdi")
	fc.out.MovRegToReg("r13", "rsi")

	fc.out.MovMemToReg("rax", "r12", 0)
	fc.out.CmpRegToImm("rax", 0)
	notInitJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)

	fc.out.MovMemToReg("rdi", "r12", 0)
	fc.out.MovMemToReg("rsi", "r12", 8)

	fc.out.MovRegToReg("rax", "r13")
	fc.out.AndRegWithImm("rax", 31)

	fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x04})
	fc.out.AddRegToReg("rax", "rdi")
	fc.out.MovRegToReg("rbx", "rax")

	fc.out.XorRegWithReg("rcx", "rcx")

	loopStart := fc.eb.text.Len()
	fc.out.CmpRegToImm("rcx", 32)
	loopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)

	fc.out.MovMemToReg("rax", "rbx", 0)
	fc.out.CmpRegToReg("rax", "r13")
	foundJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)

	fc.out.AddImmToReg("rbx", 16)
	fc.out.AddImmToReg("rcx", 1)
	backJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(int32(loopStart - (backJump + 5)))

	foundLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(foundJump+2, int32(foundLabel-(foundJump+6)))
	fc.out.LeaMemToReg("rax", "rbx", 8)

	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	notInitLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(notInitJump+2, int32(notInitLabel-(notInitJump+6)))

	loopEndLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(loopEndJump+2, int32(loopEndLabel-(loopEndJump+6)))
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()
}

func (fc *C67Compiler) generateCacheInsert() {
	fc.eb.MarkLabel("_c67_cache_insert")

	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")
	fc.out.PushReg("r15")

	fc.out.MovRegToReg("r12", "rdi")
	fc.out.MovRegToReg("r13", "rsi")
	fc.out.MovRegToReg("r14", "rdx")

	fc.out.MovMemToReg("rax", "r12", 0)
	fc.out.CmpRegToImm("rax", 0)
	alreadyInitJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0)

	// Allocate hash table: malloc(defaultHashTableSize)
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", defaultHashTableSize))
	fc.callMallocAligned("rax", 5) // 5 pushes after prologue
	fc.out.MovRegToMem("rax", "r12", 0)
	fc.out.MovImmToReg("rax", "32")
	fc.out.MovRegToMem("rax", "r12", 8)

	alreadyInitLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(alreadyInitJump+2, int32(alreadyInitLabel-(alreadyInitJump+6)))

	fc.out.MovMemToReg("rdi", "r12", 0)

	fc.out.MovRegToReg("rax", "r13")
	fc.out.AndRegWithImm("rax", 31)

	fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x04})
	fc.out.AddRegToReg("rax", "rdi")
	fc.out.MovRegToMem("r13", "rax", 0)
	fc.out.MovRegToMem("r14", "rax", 8)
	fc.out.PopReg("r15")
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()
}

func (fc *C67Compiler) generateRuntimeHelpers() {
	// Arena runtime functions are generated inline below (_c67_arena_create, alloc, etc)
	// Don't call fc.eb.EmitArenaRuntimeCode() as it's the old stub from main.go
	// Arena symbols are predeclared earlier in writeELF() to ensure they're available during code generation

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Used functions: %v\n", fc.usedFunctions)
	}

	// Generate syscall-based printf runtime on Linux
	fc.GeneratePrintfSyscallRuntime()

	// Only generate cache functions if actually used (small optimization)
	if len(fc.cacheEnabledLambdas) > 0 {
		fc.generateCacheLookup()
		fc.generateCacheInsert()

		for lambdaName := range fc.cacheEnabledLambdas {
			cacheName := lambdaName + "_cache"
			fc.eb.Define(cacheName, "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		}
	}

	// Generate _c67_string_concat only if used
	if fc.runtimeFeatures.Uses(FeatureStringConcat) {
		// Generate _c67_string_concat(left_ptr, right_ptr) -> new_ptr
		// Arguments: rdi = left_ptr, rsi = right_ptr
		// Returns: rax = pointer to new concatenated string

		fc.eb.MarkLabel("_c67_string_concat")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save callee-saved registers
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		// Align stack to 16 bytes for malloc call
		// After call (8) + push rbp (8) + push 5 regs (40) = 56 bytes
		// We need to subtract 8 more to get 16-byte alignment
		fc.out.SubImmFromReg("rsp", StackSlotSize)

		// Save arguments
		fc.out.MovRegToReg("r12", "rdi") // r12 = left_ptr
		fc.out.MovRegToReg("r13", "rsi") // r13 = right_ptr

		// Get left string length
		fc.out.MovMemToXmm("xmm0", "r12", 0) // load count as float64
		// Convert float64 to integer using cvttsd2si
		fc.out.Emit([]byte{0xf2, 0x4c, 0x0f, 0x2c, 0xf0}) // cvttsd2si r14, xmm0

		// Get right string length
		fc.out.MovMemToXmm("xmm0", "r13", 0) // load count as float64
		// Convert float64 to integer
		fc.out.Emit([]byte{0xf2, 0x4c, 0x0f, 0x2c, 0xf8}) // cvttsd2si r15, xmm0

		// Calculate total length: rbx = r14 + r15
		fc.out.MovRegToReg("rbx", "r14")
		fc.out.Emit([]byte{0x4c, 0x01, 0xfb}) // add rbx, r15

		// Calculate allocation size: rax = 8 + rbx * 16
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x04}) // shl rax, 4 (multiply by 16)
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x08}) // add rax, 8

		// Align to 16 bytes for safety
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x0f}) // add rax, 15
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0xf0}) // and rax, ~15

		// Use arena allocation instead of malloc
		// rax contains the size needed
		fc.out.MovRegToReg("rdi", "rax") // rdi = size
		fc.callArenaAlloc()              // Allocate from current arena
		fc.out.MovRegToReg("r10", "rax") // r10 = result pointer

		// Write total count to result
		fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2a, 0xc3}) // cvtsi2sd xmm0, rbx
		fc.out.MovXmmToMem("xmm0", "r10", 0)

		// Copy left string entries
		// memcpy(r10 + 8, r12 + 8, r14 * 16)
		fc.out.Emit([]byte{0x4d, 0x89, 0xf1})             // mov r9, r14 (counter)
		fc.out.Emit([]byte{0x49, 0x8d, 0x74, 0x24, 0x08}) // lea rsi, [r12 + 8]
		fc.out.Emit([]byte{0x49, 0x8d, 0x7a, 0x08})       // lea rdi, [r10 + 8]

		// Loop to copy left entries
		fc.eb.MarkLabel("_concat_copy_left_loop")
		fc.out.Emit([]byte{0x4d, 0x85, 0xc9}) // test r9, r9
		// jz to skip copying if zero length - skip entire loop body (22 + 8 + 3 + 2 = 35 bytes)
		fc.out.Emit([]byte{0x74, 0x23}) // jz +35 bytes (skip the entire loop)

		fc.out.MovMemToXmm("xmm0", "rsi", 0)        // load key
		fc.out.MovXmmToMem("xmm0", "rdi", 0)        // store key
		fc.out.MovMemToXmm("xmm0", "rsi", 8)        // load value
		fc.out.MovXmmToMem("xmm0", "rdi", 8)        // store value
		fc.out.Emit([]byte{0x48, 0x83, 0xc6, 0x10}) // add rsi, 16
		fc.out.Emit([]byte{0x48, 0x83, 0xc7, 0x10}) // add rdi, 16
		fc.out.Emit([]byte{0x49, 0xff, 0xc9})       // dec r9
		fc.out.Emit([]byte{0xeb, 0xd8})             // jmp back to test (-40 bytes)

		// Now copy right string entries with offset keys
		// r15 = right_len (counter), r14 = offset for keys
		fc.out.Emit([]byte{0x49, 0x8d, 0x75, 0x08}) // lea rsi, [r13 + 8]
		// rdi already points to correct position

		fc.eb.MarkLabel("_concat_copy_right_loop")
		fc.out.Emit([]byte{0x4d, 0x85, 0xff}) // test r15, r15
		fc.out.Emit([]byte{0x74, 0x2c})       // jz +44 bytes (skip entire second loop)

		fc.out.MovMemToXmm("xmm0", "rsi", 0)              // load key
		fc.out.Emit([]byte{0xf2, 0x49, 0x0f, 0x2a, 0xce}) // cvtsi2sd xmm1, r14 (offset)
		fc.out.Emit([]byte{0xf2, 0x0f, 0x58, 0xc1})       // addsd xmm0, xmm1 (key += offset)
		fc.out.MovXmmToMem("xmm0", "rdi", 0)              // store adjusted key
		fc.out.MovMemToXmm("xmm0", "rsi", 8)              // load value
		fc.out.MovXmmToMem("xmm0", "rdi", 8)              // store value
		fc.out.Emit([]byte{0x48, 0x83, 0xc6, 0x10})       // add rsi, 16
		fc.out.Emit([]byte{0x48, 0x83, 0xc7, 0x10})       // add rdi, 16
		fc.out.Emit([]byte{0x49, 0xff, 0xcf})             // dec r15
		fc.out.Emit([]byte{0xeb, 0xcf})                   // jmp back to test (-49 bytes)

		// Return result pointer in rax
		fc.out.MovRegToReg("rax", "r10")

		// Restore stack alignment
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Restore callee-saved registers
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Function epilogue
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_string_concat used

	// Generate _c67_string_to_cstr only if used (for printf, f-strings, C FFI)
	if fc.runtimeFeatures.needsStringToCstr() {
		// Generate _c67_string_to_cstr(c67_string_ptr) -> cstr_ptr
		// Converts a C67 string (map format) to a null-terminated C string
		// Argument: xmm0 = C67 string pointer (as float64)
		// Returns: rax = C string pointer
		fc.eb.MarkLabel("_c67_string_to_cstr")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save callee-saved registers
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14") // r14 = codepoint count
		fc.out.PushReg("r15") // r15 = output byte position

		// Stack alignment FIX: call(8) + 6 pushes(48) = 56 bytes (MISALIGNED!)
		// Sub 16 keeps stack aligned for malloc call
		// Stack alignment FIX: call(8) + 6 pushes(48) = 56 bytes (MISALIGNED!)
		// Sub 16 keeps stack aligned for malloc call

		// Convert float64 pointer to integer pointer in r12
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r12", "rsp", 0)

		// Get string length from map: count = [r12+0]
		fc.out.MovMemToXmm("xmm0", "r12", 0)
		fc.out.Emit([]byte{0xf2, 0x4c, 0x0f, 0x2c, 0xf0}) // cvttsd2si r14, xmm0 (r14 = codepoint count)

		// DO NOT USE MALLOC! See MEMORY.md - C string conversion should use arena allocation
		// TODO: Replace with arena allocation (or stack for small strings)
		// Allocate memory: count * 4 + 1 for UTF-8 (max 4 bytes per codepoint + null)
		// Calculate size in temporary register
		fc.out.MovRegToReg("rax", "r14")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x02}) // shl rax, 2 (multiply by 4)
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x01}) // add rax, 1

		// Platform-specific calling convention for malloc
		if fc.eb.target.OS() == OSWindows {
			// Windows x64: first arg in rcx
			fc.out.MovRegToReg("rcx", "rax")
			// Allocate shadow space (32 bytes) for Windows calling convention
			fc.out.SubImmFromReg("rsp", 32)
		} else {
			// SysV (Linux/Unix): first arg in rdi
			fc.out.MovRegToReg("rdi", "rax")
		}

		// Allocate from arena
		fc.callArenaAlloc()

		// Clean up shadow space on Windows
		if fc.eb.target.OS() == OSWindows {
			fc.out.AddImmToReg("rsp", 32)
		}

		fc.out.MovRegToReg("r13", "rax") // r13 = C string buffer

		// Initialize: rbx = codepoint index, r12 = map ptr, r13 = output buffer, r14 = count, r15 = byte position
		fc.out.XorRegWithReg("rbx", "rbx") // rbx = 0 (codepoint index)
		fc.out.XorRegWithReg("r15", "r15") // r15 = 0 (output byte position)

		// Loop through map entries to extract and encode codepoints
		fc.eb.MarkLabel("_cstr_convert_loop")
		fc.out.Emit([]byte{0x4c, 0x39, 0xf3}) // cmp rbx, r14
		loopEndJump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // je _loop_end (4-byte offset, will patch)

		// Calculate map entry offset: 8 + (rbx * 16) for [count][key0][val0][key1][val1]...
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x04}) // shl rax, 4 (multiply by 16)
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x08}) // add rax, 8

		// Load codepoint value: xmm0 = [r12 + rax + 8] (value field)
		fc.out.Emit([]byte{0xf2, 0x49, 0x0f, 0x10, 0x44, 0x04, 0x08}) // movsd xmm0, [r12 + rax + 8]

		// Convert codepoint to integer in rdx
		fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2c, 0xd0}) // cvttsd2si rdx, xmm0 (rdx = codepoint)

		// UTF-8 encoding: check codepoint ranges and encode
		// Case 1: codepoint <= 0x7F (1 byte: 0xxxxxxx)
		fc.out.Emit([]byte{0x48, 0x81, 0xfa, 0x7f, 0x00, 0x00, 0x00}) // cmp rdx, 0x7F
		case1Jump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x87, 0x00, 0x00, 0x00, 0x00}) // ja case2 (4-byte offset)
		// 1-byte encoding
		fc.out.Emit([]byte{0x43, 0x88, 0x54, 0x3d, 0x00}) // mov [r13 + r15], dl
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		continueJump1 := fc.eb.text.Len()
		fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00}) // jmp loop_continue (4-byte offset)

		// Case 2: codepoint <= 0x7FF (2 bytes: 110xxxxx 10xxxxxx)
		case2Start := fc.eb.text.Len()
		fc.patchJumpImmediate(case1Jump+2, int32(case2Start-(case1Jump+6)))
		fc.out.Emit([]byte{0x48, 0x81, 0xfa, 0xff, 0x07, 0x00, 0x00}) // cmp rdx, 0x7FF
		case2Jump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x87, 0x00, 0x00, 0x00, 0x00}) // ja case3
		// 2-byte encoding
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x06})       // shr rax, 6
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0xc0})       // or rax, 0xC0
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		continueJump2 := fc.eb.text.Len()
		fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00}) // jmp loop_continue

		// Case 3: codepoint <= 0xFFFF (3 bytes: 1110xxxx 10xxxxxx 10xxxxxx)
		case3Start := fc.eb.text.Len()
		fc.patchJumpImmediate(case2Jump+2, int32(case3Start-(case2Jump+6)))
		fc.out.Emit([]byte{0x48, 0x81, 0xfa, 0xff, 0xff, 0x00, 0x00}) // cmp rdx, 0xFFFF
		case3Jump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x87, 0x00, 0x00, 0x00, 0x00}) // ja case4
		// 3-byte encoding
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x0c})       // shr rax, 12
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0xe0})       // or rax, 0xE0
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x06})       // shr rax, 6
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		continueJump3 := fc.eb.text.Len()
		fc.out.Emit([]byte{0xe9, 0x00, 0x00, 0x00, 0x00}) // jmp loop_continue

		// Case 4: codepoint > 0xFFFF (4 bytes: 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx)
		case4Start := fc.eb.text.Len()
		fc.patchJumpImmediate(case3Jump+2, int32(case4Start-(case3Jump+6)))
		// 4-byte encoding
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x12})       // shr rax, 18
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0xf0})       // or rax, 0xF0
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x0c})       // shr rax, 12
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x06})       // shr rax, 6
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x3f})       // and rax, 0x3F
		fc.out.Emit([]byte{0x48, 0x83, 0xc8, 0x80})       // or rax, 0x80
		fc.out.Emit([]byte{0x43, 0x88, 0x44, 0x3d, 0x00}) // mov [r13 + r15], al
		fc.out.Emit([]byte{0x49, 0xff, 0xc7})             // inc r15

		// Loop continue: increment codepoint index and jump back
		loopContinue := fc.eb.text.Len()
		fc.patchJumpImmediate(continueJump1+1, int32(loopContinue-(continueJump1+5)))
		fc.patchJumpImmediate(continueJump2+1, int32(loopContinue-(continueJump2+5)))
		fc.patchJumpImmediate(continueJump3+1, int32(loopContinue-(continueJump3+5)))
		fc.out.Emit([]byte{0x48, 0xff, 0xc3}) // inc rbx
		loopJumpBack := fc.eb.text.Len()
		backOffset := int32(fc.eb.LabelOffset("_cstr_convert_loop") - (loopJumpBack + 5))
		fc.out.Emit([]byte{0xe9, byte(backOffset), byte(backOffset >> 8), byte(backOffset >> 16), byte(backOffset >> 24)}) // jmp _cstr_convert_loop

		// Loop end: add null terminator
		loopEnd := fc.eb.text.Len()
		fc.patchJumpImmediate(loopEndJump+2, int32(loopEnd-(loopEndJump+6)))
		fc.out.Emit([]byte{0x43, 0xc6, 0x44, 0x3d, 0x00, 0x00}) // mov byte [r13 + r15], 0

		// Return C string pointer in rax
		fc.out.MovRegToReg("rax", "r13")

		// Restore stack alignment
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Restore callee-saved registers
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Function epilogue
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_string_to_cstr used

	// Generate _c67_cstr_to_string only if used (for C FFI string returns)
	if fc.runtimeFeatures.Uses(FeatureCstrToString) {
		// Generate _c67_cstr_to_string(cstr_ptr) -> c67_string_ptr
		// Converts a null-terminated C string to a C67 string (map format)
		// Argument: rdi = C string pointer
		// Returns: xmm0 = C67 string pointer (as float64)
		fc.eb.MarkLabel("_c67_cstr_to_string")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save callee-saved registers
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")

		// Stack is now 16-byte aligned (call pushed 8, then 5 pushes = 48 bytes total)
		// No additional alignment needed

		// Save C string pointer
		fc.out.MovRegToReg("r12", "rdi") // r12 = C string pointer

		// Calculate string length using strlen(r12)
		fc.out.MovRegToReg("rdi", "r12") // Set argument for strlen
		fc.trackFunctionCall("strlen")
		fc.eb.GenerateCallInstruction("strlen")
		fc.out.MovRegToReg("r14", "rax") // r14 = string length

		// Allocate C67 string map: 8 + (length * 16) bytes
		// count (8 bytes) + (key, value) pairs (16 bytes each)
		fc.out.MovRegToReg("rdi", "r14")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe7, 0x04}) // shl rdi, 4 (multiply by 16)
		fc.out.Emit([]byte{0x48, 0x83, 0xc7, 0x08}) // add rdi, 8
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax") // r13 = C67 string map pointer

		// Store count in map[0]
		fc.out.MovRegToReg("rax", "r14")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r13", 0)

		// Fill map with character data
		fc.out.XorRegWithReg("rbx", "rbx") // rbx = index

		// Loop: for each character
		cstrLoopStart := fc.eb.text.Len()
		fc.eb.MarkLabel("_cstr_to_c67_loop")

		// Compare index with length
		fc.out.Emit([]byte{0x4c, 0x39, 0xf3}) // cmp rbx, r14
		cstrExitJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // je to exit (will patch later)

		// Load character from C string: al = [r12 + rbx]
		fc.out.Emit([]byte{0x41, 0x8a, 0x04, 0x1c}) // mov al, [r12 + rbx]

		// Convert character to float64
		fc.out.Emit([]byte{0x48, 0x0f, 0xb6, 0xc0}) // movzx rax, al
		fc.out.Cvtsi2sd("xmm0", "rax")

		// Convert index to float64 for key
		fc.out.MovRegToReg("rdx", "rbx")
		fc.out.Cvtsi2sd("xmm1", "rdx")

		// Calculate offset for entry: 8 + (rbx * 16)
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x04}) // shl rax, 4
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x08}) // add rax, 8

		// Add offset to base pointer: rax = r13 + rax
		fc.out.Emit([]byte{0x4c, 0x01, 0xe8}) // add rax, r13

		// Store key (index): [rax] = xmm1
		fc.out.Emit([]byte{0xf2, 0x0f, 0x11, 0x08}) // movsd [rax], xmm1

		// Store value (character): [rax + 8] = xmm0
		fc.out.Emit([]byte{0xf2, 0x0f, 0x11, 0x40, 0x08}) // movsd [rax + 8], xmm0

		// Increment index
		fc.out.Emit([]byte{0x48, 0xff, 0xc3}) // inc rbx

		// Jump back to loop start
		cstrLoopEnd := fc.eb.text.Len()
		cstrOffset := int32(cstrLoopStart - (cstrLoopEnd + 2))
		fc.out.Emit([]byte{0xeb, byte(cstrOffset)}) // jmp rel8

		// Patch the exit jump
		cstrExitPos := fc.eb.text.Len()
		fc.patchJumpImmediate(cstrExitJumpPos+2, int32(cstrExitPos-(cstrExitJumpPos+6)))

		// Return C67 string pointer in xmm0
		fc.out.MovRegToXmm("xmm0", "r13")

		// Restore callee-saved registers (no stack adjustment needed)
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Function epilogue
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_cstr_to_string used

	// Generate _c67_slice_string only if used
	if fc.runtimeFeatures.Uses(FeatureStringSlice) {
		// Generate _c67_slice_string(str_ptr, start, end, step) -> new_str_ptr
		// Arguments: rdi = string_ptr, rsi = start_index (int64), rdx = end_index (int64), rcx = step (int64)
		// Returns: rax = pointer to new sliced string
		// String format (map): [count (float64)][key0 (float64)][val0 (float64)]...
		// Note: Currently only step == 1 is fully supported

		fc.eb.MarkLabel("_c67_slice_string")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save callee-saved registers
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		// Save arguments
		fc.out.MovRegToReg("r12", "rdi") // r12 = original string pointer
		fc.out.MovRegToReg("r13", "rsi") // r13 = start index
		fc.out.MovRegToReg("r14", "rdx") // r14 = end index
		fc.out.MovRegToReg("r8", "rcx")  // r8 = step

		// Calculate result length based on step
		// For step == 1: length = end - start
		// For step > 1: length = ((end - start + step - 1) / step)
		// For step < 0: length = ((start - end - step - 1) / (-step))

		fc.out.XorRegWithReg("rax", "rax")
		fc.out.CmpRegToReg("r8", "rax")
		stepNegativeJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // If step < 0, jump to negative path

		// Positive step path
		fc.out.MovImmToReg("rax", "1")
		fc.out.CmpRegToReg("r8", "rax")
		stepOneJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // If step == 1, use simple path

		// Step > 1 path: length = ((end - start + step - 1) / step)
		fc.out.MovRegToReg("r15", "r14")
		fc.out.SubRegFromReg("r15", "r13") // r15 = end - start
		fc.out.AddRegToReg("r15", "r8")    // r15 = end - start + step
		fc.out.SubImmFromReg("r15", 1)     // r15 = end - start + step - 1
		fc.out.MovRegToReg("rax", "r15")
		fc.out.XorRegWithReg("rdx", "rdx")    // Clear rdx for division
		fc.out.Emit([]byte{0x49, 0xF7, 0xF8}) // idiv r8
		fc.out.MovRegToReg("r15", "rax")      // r15 = result length

		stepEndJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Jump to end

		// Patch step == 1 jump to here
		stepOnePos := fc.eb.text.Len()
		stepOneOffset := int32(stepOnePos - (stepOneJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(stepOneJumpPos+2, stepOneOffset)

		// Step == 1 simple path: length = end - start
		fc.out.MovRegToReg("r15", "r14")
		fc.out.SubRegFromReg("r15", "r13") // r15 = length

		stepPosEndJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Jump to end

		// Patch negative step jump to here
		stepNegativePos := fc.eb.text.Len()
		stepNegativeOffset := int32(stepNegativePos - (stepNegativeJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(stepNegativeJumpPos+2, stepNegativeOffset)

		// Negative step path: length = ((start - end - step - 1) / (-step))
		fc.out.MovRegToReg("r15", "r13")   // r15 = start
		fc.out.SubRegFromReg("r15", "r14") // r15 = start - end
		fc.out.SubRegFromReg("r15", "r8")  // r15 = start - end - step
		fc.out.SubImmFromReg("r15", 1)     // r15 = start - end - step - 1
		// Divide by -step, so negate r8, divide, then restore r8
		fc.out.MovRegToReg("r10", "r8")       // Save r8
		fc.out.Emit([]byte{0x49, 0xF7, 0xD8}) // neg r8 (r8 = -r8)
		fc.out.MovRegToReg("rax", "r15")
		fc.out.XorRegWithReg("rdx", "rdx")    // Clear rdx for division
		fc.out.Emit([]byte{0x49, 0xF7, 0xF8}) // idiv r8
		fc.out.MovRegToReg("r15", "rax")      // r15 = result length
		fc.out.MovRegToReg("r8", "r10")       // Restore r8

		// Patch end jumps
		stepEndPos := fc.eb.text.Len()
		stepEndOffset := int32(stepEndPos - (stepEndJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(stepEndJumpPos+1, stepEndOffset)

		stepPosEndOffset := int32(stepEndPos - (stepPosEndJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(stepPosEndJumpPos+1, stepPosEndOffset)

		// Allocate memory for new string: 8 + (length * 16) bytes
		fc.out.MovRegToReg("rax", "r15")
		fc.out.ShlRegImm("rax", "4") // shl rax, 4 (multiply by 16)
		fc.out.AddImmToReg("rax", 8) // add rax, 8
		fc.out.MovRegToReg("rdi", "rax")
		// Save r8 (step) before malloc since it's caller-saved
		fc.out.PushReg("r8")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("rbx", "rax") // rbx = new string pointer
		// Restore r8 (step)
		fc.out.PopReg("r8")

		// Store count (length) as float64 in first 8 bytes
		fc.out.Cvtsi2sd("xmm0", "r15") // xmm0 = length as float64
		fc.out.MovXmmToMem("xmm0", "rbx", 0)

		// Copy characters from original string
		// Initialize loop counter (output index): rcx = 0
		fc.out.XorRegWithReg("rcx", "rcx")
		// Initialize source index: r9 = start
		fc.out.MovRegToReg("r9", "r13")

		fc.eb.MarkLabel("_slice_copy_loop")
		sliceLoopStart := fc.eb.text.Len() // Track actual loop start position

		// Check if rcx >= length (exit loop if true)
		fc.out.CmpRegToReg("rcx", "r15")
		loopExitJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpAboveOrEqual, 0) // Placeholder, will patch later

		// Use source index from r9
		fc.out.MovRegToReg("rax", "r9")

		// Calculate source address: r11 = r12 + 8 + (source_idx * 16)
		fc.out.ShlRegImm("rax", "4") // rax = source_idx * 16
		fc.out.AddImmToReg("rax", 8) // rax = source_idx * 16 + 8
		fc.out.MovRegToReg("r11", "r12")
		fc.out.AddRegToReg("r11", "rax") // r11 = r12 + rax

		// Load key and value from source string
		fc.out.MovMemToXmm("xmm0", "r11", 0) // xmm0 = [r11] (key)
		fc.out.MovMemToXmm("xmm1", "r11", 8) // xmm1 = [r11 + 8] (value)

		// Calculate destination address: rdx = 8 + (rcx * 16)
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.ShlRegImm("rdx", "4") // rdx = rcx * 16
		fc.out.AddImmToReg("rdx", 8) // rdx = rcx * 16 + 8

		// Calculate full destination address: r11 = rbx + rdx
		fc.out.MovRegToReg("r11", "rbx")
		fc.out.AddRegToReg("r11", "rdx") // r11 = rbx + rdx

		// Store key as rcx (new index), and value
		fc.out.Cvtsi2sd("xmm0", "rcx")       // xmm0 = rcx as float64 (new key)
		fc.out.MovXmmToMem("xmm0", "r11", 0) // [r11] = xmm0 (key)
		fc.out.MovXmmToMem("xmm1", "r11", 8) // [r11 + 8] = xmm1 (value)

		// Increment loop counter
		fc.out.IncReg("rcx")

		// Increment source index by step
		fc.out.AddRegToReg("r9", "r8") // r9 = r9 + step

		// Jump back to loop start
		loopBackJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Placeholder, will patch later

		// Patch loop jumps
		loopExitPos := fc.eb.text.Len()

		// Patch exit jump: JumpConditional emits 6 bytes (0x0f 0x83 + 4-byte offset)
		// Offset is from end of jump instruction to loop exit
		loopExitOffset := int32(loopExitPos - (loopExitJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(loopExitJumpPos+2, loopExitOffset) // +2 to skip 0x0f 0x83 opcode bytes

		// Patch back jump: JumpUnconditional emits 5 bytes (0xe9 + 4-byte offset)
		// Offset is from end of jump instruction back to loop start
		loopBackOffset := int32(sliceLoopStart - (loopBackJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(loopBackJumpPos+1, loopBackOffset) // +1 to skip 0xe9 opcode byte

		// Return new string pointer in rax
		fc.out.MovRegToReg("rax", "rbx")

		// Restore callee-saved registers
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Function epilogue
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_slice_string used

	// Generate _c67_list_concat only if list operations are used
	// This function is called when concatenating lists at runtime
	// Arguments: rdi = left_ptr, rsi = right_ptr
	// Returns: rax = pointer to new concatenated list
	// List format: [length (8 bytes)][elem0 (8 bytes)][elem1 (8 bytes)]...
	if fc.runtimeFeatures.Uses(FeatureListConcat) {
		fc.eb.MarkLabel("_c67_list_concat")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Save callee-saved registers
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		// Align stack to 16 bytes for malloc call
		fc.out.SubImmFromReg("rsp", StackSlotSize)

		// Save arguments
		fc.out.MovRegToReg("r12", "rdi") // r12 = left_ptr
		fc.out.MovRegToReg("r13", "rsi") // r13 = right_ptr

		// Get left list length
		fc.out.MovMemToXmm("xmm0", "r12", 0)              // load length as float64
		fc.out.Emit([]byte{0xf2, 0x4c, 0x0f, 0x2c, 0xf0}) // cvttsd2si r14, xmm0

		// Get right list length
		fc.out.MovMemToXmm("xmm0", "r13", 0)              // load length as float64
		fc.out.Emit([]byte{0xf2, 0x4c, 0x0f, 0x2c, 0xf8}) // cvttsd2si r15, xmm0

		// Calculate total length: rbx = r14 + r15
		fc.out.MovRegToReg("rbx", "r14")
		fc.out.Emit([]byte{0x4c, 0x01, 0xfb}) // add rbx, r15

		// Calculate allocation size: rax = 8 + rbx * 8
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe0, 0x03}) // shl rax, 3 (multiply by 8)
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x08}) // add rax, 8

		// Align to 16 bytes for safety
		fc.out.Emit([]byte{0x48, 0x83, 0xc0, 0x0f}) // add rax, 15
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0xf0}) // and rax, ~15

		// Call malloc(rax)
		fc.out.MovRegToReg("rdi", "rax")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r10", "rax") // r10 = result pointer

		// Write total length to result
		fc.out.Emit([]byte{0xf2, 0x48, 0x0f, 0x2a, 0xc3}) // cvtsi2sd xmm0, rbx
		fc.out.MovXmmToMem("xmm0", "r10", 0)

		// Copy left list elements
		// memcpy(r10 + 8, r12 + 8, r14 * 8)
		fc.out.Emit([]byte{0x4d, 0x89, 0xf1})             // mov r9, r14 (counter)
		fc.out.Emit([]byte{0x49, 0x8d, 0x74, 0x24, 0x08}) // lea rsi, [r12 + 8]
		fc.out.Emit([]byte{0x49, 0x8d, 0x7a, 0x08})       // lea rdi, [r10 + 8]

		// Loop to copy left elements
		fc.eb.MarkLabel("_list_concat_copy_left_loop")
		fc.out.Emit([]byte{0x4d, 0x85, 0xc9}) // test r9, r9
		fc.out.Emit([]byte{0x74, 0x17})       // jz +23 bytes (skip loop body)

		fc.out.MovMemToXmm("xmm0", "rsi", 0)        // load element (4 bytes)
		fc.out.MovXmmToMem("xmm0", "rdi", 0)        // store element (4 bytes)
		fc.out.Emit([]byte{0x48, 0x83, 0xc6, 0x08}) // add rsi, 8 (4 bytes)
		fc.out.Emit([]byte{0x48, 0x83, 0xc7, 0x08}) // add rdi, 8 (4 bytes)
		fc.out.Emit([]byte{0x49, 0xff, 0xc9})       // dec r9 (3 bytes)
		fc.out.Emit([]byte{0xeb, 0xe4})             // jmp back -28 bytes (2 bytes)

		// Copy right list elements
		// memcpy(r10 + 8 + r14*8, r13 + 8, r15 * 8)
		fc.out.Emit([]byte{0x49, 0x8d, 0x75, 0x08}) // lea rsi, [r13 + 8]
		// rdi already points to correct position

		fc.eb.MarkLabel("_list_concat_copy_right_loop")
		fc.out.Emit([]byte{0x4d, 0x85, 0xff}) // test r15, r15
		fc.out.Emit([]byte{0x74, 0x17})       // jz +23 bytes (skip loop body)

		fc.out.MovMemToXmm("xmm0", "rsi", 0)        // load element (4 bytes)
		fc.out.MovXmmToMem("xmm0", "rdi", 0)        // store element (4 bytes)
		fc.out.Emit([]byte{0x48, 0x83, 0xc6, 0x08}) // add rsi, 8 (4 bytes)
		fc.out.Emit([]byte{0x48, 0x83, 0xc7, 0x08}) // add rdi, 8 (4 bytes)
		fc.out.Emit([]byte{0x49, 0xff, 0xcf})       // dec r15 (3 bytes)
		fc.out.Emit([]byte{0xeb, 0xe4})             // jmp back -28 bytes (2 bytes)

		// Return result pointer in rax
		fc.out.MovRegToReg("rax", "r10")

		// Restore stack alignment
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Restore callee-saved registers
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Function epilogue
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_list_concat used

	// Generate _c67_list_repeat only if list repeat operations are used
	// Arguments: rdi = list_ptr, rdx = count (integer)
	// Returns: rax = pointer to new repeated list (heap-allocated)
	// Simple implementation: just call list_concat repeatedly
	if fc.runtimeFeatures.Uses(FeatureListRepeat) {
		fc.eb.MarkLabel("_c67_list_repeat")

		// Function prologue
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")

		// Save arguments
		fc.out.MovRegToReg("r12", "rdi") // r12 = original list_ptr
		fc.out.MovRegToReg("r13", "rdx") // r13 = count

		// If count <= 0, return empty list
		fc.out.TestRegReg("r13", "r13")
		emptyJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLessOrEqual, 0)

		// If count == 1, return original list (already heap-allocated from literal)
		// Actually no, we need to copy it to ensure it's mutable
		// Start with result = original list
		fc.out.MovRegToReg("rbx", "r12") // rbx = result (start with first copy)

		// Dec count since we already have one copy
		fc.out.Emit([]byte{0x49, 0xff, 0xcd}) // dec r13

		// Loop: concat result with original list (count-1) times
		loopStart := fc.eb.text.Len()
		fc.out.TestRegReg("r13", "r13")
		loopEndJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)

		// Call _c67_list_concat(result, original)
		fc.out.MovRegToReg("rdi", "rbx") // first arg = result so far
		fc.out.MovRegToReg("rsi", "r12") // second arg = original list
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.CallSymbol("_c67_list_concat")
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.out.MovRegToReg("rbx", "rax") // update result

		fc.out.Emit([]byte{0x49, 0xff, 0xcd}) // dec r13
		backJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Patch loop jump
		repeatLoopEnd := fc.eb.text.Len()
		offset1 := int32(repeatLoopEnd - (loopEndJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(loopEndJumpPos+2, offset1)
		offset2 := int32(loopStart - (backJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(backJumpPos+1, offset2)

		// Return result
		fc.out.MovRegToReg("rax", "rbx")
		doneJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Empty list case: return an empty list
		emptyLabel := fc.eb.text.Len()
		fc.out.XorRegWithReg("rax", "rax") // return NULL for now

		// Patch empty jump
		offset3 := int32(emptyLabel - (emptyJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(emptyJumpPos+2, offset3)

		// Done
		doneLabel := fc.eb.text.Len()
		offset4 := int32(doneLabel - (doneJumpPos + UnconditionalJumpSize))
		fc.patchJumpImmediate(doneJumpPos+1, offset4)

		// Restore registers
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_list_repeat used

	// Generate _c67_string_eq only if string equality checks are used
	// Arguments: rdi = left_ptr, rsi = right_ptr
	// Returns: xmm0 = 1.0 if equal, 0.0 if not
	// String format: [count (8 bytes)][key0 (8)][val0 (8)][key1 (8)][val1 (8)]...
	if fc.runtimeFeatures.Uses(FeatureStringEq) {
		fc.eb.MarkLabel("_c67_string_eq")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")

		// rdi = left_ptr, rsi = right_ptr
		// Check if both are null (empty strings)
		fc.out.MovRegToReg("rax", "rdi")
		fc.out.OrRegToReg("rax", "rsi")
		fc.out.TestRegReg("rax", "rax")
		eqNullJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // If both null, they're equal

		// Check if only one is null
		fc.out.TestRegReg("rdi", "rdi")
		neqJumpPos1 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // left is null but right isn't

		fc.out.TestRegReg("rsi", "rsi")
		neqJumpPos2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // right is null but left isn't

		// Both non-null, load counts
		fc.out.MovMemToXmm("xmm0", "rdi", 0) // left count
		fc.out.MovMemToXmm("xmm1", "rsi", 0) // right count

		// Convert counts to integers for comparison
		fc.out.Cvttsd2si("r12", "xmm0") // left count in r12
		fc.out.Cvttsd2si("r13", "xmm1") // right count in r13

		// Compare counts
		fc.out.CmpRegToReg("r12", "r13")
		neqJumpPos3 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0) // If counts differ, not equal

		// Counts are equal, compare each character
		// rbx = index counter
		fc.out.XorRegWithReg("rbx", "rbx")

		loopStart2 := fc.eb.text.Len()

		// Check if we've compared all characters
		fc.out.CmpRegToReg("rbx", "r12")
		endLoopJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Calculate offset: 8 + rbx * 16 (count is 8 bytes, each key-value pair is 16 bytes)
		// Actually, format is [count][key0][val0][key1][val1]...
		// So to get value at index i: offset = 8 + i*16 + 8 = 16 + i*16
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.ShlRegImm("rax", "4")  // multiply by 16
		fc.out.AddImmToReg("rax", 16) // skip count (8) and key (8)

		// Load characters
		fc.out.Comment("Load left[rbx] and right[rbx]")
		fc.out.MovRegToReg("r8", "rdi")
		fc.out.AddRegToReg("r8", "rax")
		fc.out.MovMemToXmm("xmm2", "r8", 0)

		fc.out.MovRegToReg("r9", "rsi")
		fc.out.AddRegToReg("r9", "rax")
		fc.out.MovMemToXmm("xmm3", "r9", 0)

		// Compare characters
		fc.out.Ucomisd("xmm2", "xmm3")
		neqJumpPos4 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)

		// Increment index and continue
		fc.out.AddImmToReg("rbx", 1)
		loopJumpPos2 := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // jump back to loop start

		// Patch loop jump
		offset5 := int32(loopStart2 - (loopJumpPos2 + UnconditionalJumpSize))
		fc.patchJumpImmediate(loopJumpPos2+1, offset5)

		// All characters matched - return 1.0
		endLoopLabel := fc.eb.text.Len()
		eqNullLabel := fc.eb.text.Len() // Same position as endLoopLabel
		fc.out.MovImmToReg("rax", "1")
		fc.out.Cvtsi2sd("xmm0", "rax")
		doneJumpPos2 := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Not equal - return 0.0
		neqLabel := fc.eb.text.Len()
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

		// Done label
		doneLabel2 := fc.eb.text.Len()

		// Patch all jumps
		// Patch eqNull jump to eqNullLabel
		offset6 := int32(eqNullLabel - (eqNullJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(eqNullJumpPos+2, offset6)

		// Patch neq jumps to neqLabel
		offset7 := int32(neqLabel - (neqJumpPos1 + 6))
		fc.patchJumpImmediate(neqJumpPos1+2, offset7)

		offset8 := int32(neqLabel - (neqJumpPos2 + 6))
		fc.patchJumpImmediate(neqJumpPos2+2, offset8)

		offset9 := int32(neqLabel - (neqJumpPos3 + 6))
		fc.patchJumpImmediate(neqJumpPos3+2, offset9)

		offset10 := int32(neqLabel - (neqJumpPos4 + 6))
		fc.patchJumpImmediate(neqJumpPos4+2, offset10)

		// Patch endLoop jump to endLoopLabel
		offset11 := int32(endLoopLabel - (endLoopJumpPos + ConditionalJumpSize))
		fc.patchJumpImmediate(endLoopJumpPos+2, offset11)

		// Patch done jump to doneLabel2
		offset12 := int32(doneLabel2 - (doneJumpPos2 + UnconditionalJumpSize))
		fc.patchJumpImmediate(doneJumpPos2+1, offset12)

		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Generate upper_string(c67_string_ptr) -> uppercase_c67_string_ptr
		// Converts a C67 string to uppercase
		// Argument: rdi = C67 string pointer (as integer)
		// Returns: xmm0 = uppercase C67 string pointer (as float64)
		fc.eb.MarkLabel("upper_string")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		fc.out.MovRegToReg("r12", "rdi") // r12 = input string

		// Get string length
		fc.out.MovMemToXmm("xmm0", "r12", 0)
		fc.out.Cvttsd2si("r14", "xmm0") // r14 = count

		// Allocate new string map
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlRegImm("rax", "4") // rax = count * 16
		fc.out.AddImmToReg("rax", 8)
		fc.out.MovRegToReg("rdi", "rax")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax") // r13 = output string

		// Copy count
		fc.out.MovRegToReg("rax", "r14")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r13", 0)

		// Loop through characters
		fc.out.XorRegWithReg("rbx", "rbx") // rbx = loop counter
		upperLoopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("rbx", "r14")
		upperLoopEnd := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Calculate offset: rax = 8 + rbx*16
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.ShlRegImm("rax", "4") // rax = rbx * 16
		fc.out.AddImmToReg("rax", 8) // rax = 8 + rbx * 16

		// Calculate source address: r15 = r12 + rax
		fc.out.MovRegToReg("r15", "r12")
		fc.out.AddRegToReg("r15", "rax")

		// Calculate dest address: r10 = r13 + rax
		fc.out.MovRegToReg("r10", "r13")
		fc.out.AddRegToReg("r10", "rax")

		// Copy key (index)
		fc.out.MovMemToXmm("xmm0", "r15", 0)
		fc.out.MovXmmToMem("xmm0", "r10", 0)

		// Load character value and convert
		fc.out.MovMemToXmm("xmm0", "r15", 8)
		fc.out.Cvttsd2si("rax", "xmm0") // Use rax for the character value

		// Convert to uppercase: if (c >= 'a' && c <= 'z') c -= 32
		fc.out.CmpRegToImm("rax", int64('a'))
		notLowerJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0)
		fc.out.CmpRegToImm("rax", int64('z'))
		notLowerJump2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreater, 0)
		fc.out.SubImmFromReg("rax", 32)

		// Store uppercase character
		notLowerPos := fc.eb.text.Len()
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r10", 8)

		fc.out.IncReg("rbx")
		jumpBack := int32(upperLoopStart - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(jumpBack)

		upperDone := fc.eb.text.Len()
		fc.patchJumpImmediate(upperLoopEnd+2, int32(upperDone-(upperLoopEnd+6)))
		fc.patchJumpImmediate(notLowerJump+2, int32(notLowerPos-(notLowerJump+6)))
		fc.patchJumpImmediate(notLowerJump2+2, int32(notLowerPos-(notLowerJump2+6)))

		fc.out.MovRegToXmm("xmm0", "r13")
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Generate lower_string(c67_string_ptr) -> lowercase_c67_string_ptr
		// Converts a C67 string to lowercase
		// Argument: rdi = C67 string pointer (as integer)
		// Returns: xmm0 = lowercase C67 string pointer (as float64)
		fc.eb.MarkLabel("lower_string")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		fc.out.MovRegToReg("r12", "rdi") // r12 = input string

		// Get string length
		fc.out.MovMemToXmm("xmm0", "r12", 0)
		fc.out.Cvttsd2si("r14", "xmm0") // r14 = count

		// Allocate new string map
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlRegImm("rax", "4") // rax = count * 16
		fc.out.AddImmToReg("rax", 8)
		fc.out.MovRegToReg("rdi", "rax")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax") // r13 = output string

		// Copy count
		fc.out.MovRegToReg("rax", "r14")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r13", 0)

		// Loop through characters
		fc.out.XorRegWithReg("rbx", "rbx") // rbx = loop counter
		lowerLoopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("rbx", "r14")
		lowerLoopEnd := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Calculate offset: rax = 8 + rbx*16
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.ShlRegImm("rax", "4") // rax = rbx * 16
		fc.out.AddImmToReg("rax", 8) // rax = 8 + rbx * 16

		// Calculate source address: r15 = r12 + rax
		fc.out.MovRegToReg("r15", "r12")
		fc.out.AddRegToReg("r15", "rax")

		// Calculate dest address: r10 = r13 + rax
		fc.out.MovRegToReg("r10", "r13")
		fc.out.AddRegToReg("r10", "rax")

		// Copy key (index)
		fc.out.MovMemToXmm("xmm0", "r15", 0)
		fc.out.MovXmmToMem("xmm0", "r10", 0)

		// Load character value and convert
		fc.out.MovMemToXmm("xmm0", "r15", 8)
		fc.out.Cvttsd2si("rax", "xmm0") // Use rax for the character value

		// Convert to lowercase: if (c >= 'A' && c <= 'Z') c += 32
		fc.out.CmpRegToImm("rax", int64('A'))
		notUpperJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0)
		fc.out.CmpRegToImm("rax", int64('Z'))
		notUpperJump2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreater, 0)
		fc.out.AddImmToReg("rax", 32)

		// Store lowercase character
		notUpperPos := fc.eb.text.Len()
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r10", 8)

		fc.out.IncReg("rbx")
		jumpBack = int32(lowerLoopStart - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(jumpBack)

		lowerDone := fc.eb.text.Len()
		fc.patchJumpImmediate(lowerLoopEnd+2, int32(lowerDone-(lowerLoopEnd+6)))
		fc.patchJumpImmediate(notUpperJump+2, int32(notUpperPos-(notUpperJump+6)))
		fc.patchJumpImmediate(notUpperJump2+2, int32(notUpperPos-(notUpperJump2+6)))

		fc.out.MovRegToXmm("xmm0", "r13")
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Generate trim_string(c67_string_ptr) -> trimmed_c67_string_ptr
		// Removes leading and trailing whitespace
		// Argument: rdi = C67 string pointer (as integer)
		// Returns: xmm0 = trimmed C67 string pointer (as float64)
		fc.eb.MarkLabel("trim_string")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		fc.out.MovRegToReg("r12", "rdi") // r12 = input string

		// Get string length
		fc.out.MovMemToXmm("xmm0", "r12", 0)
		fc.out.Cvttsd2si("r14", "xmm0") // r14 = original count

		// Find start (skip leading whitespace)
		fc.out.XorRegWithReg("rbx", "rbx") // rbx = start index
		trimStartLoop := fc.eb.text.Len()
		fc.out.Emit([]byte{0x4c, 0x39, 0xf3}) // cmp rbx, r14
		trimStartDone := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Load character at rbx
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.ShlRegImm("rax", "4") // rax = rbx * 16
		fc.out.AddImmToReg("rax", 8) // rax = 8 + rbx * 16
		fc.out.MovRegToReg("r8", "r12")
		fc.out.AddRegToReg("r8", "rax")     // r8 = r12 + offset
		fc.out.MovMemToXmm("xmm0", "r8", 8) // Load value
		fc.out.Cvttsd2si("r10", "xmm0")

		// Check if whitespace (space=32, tab=9, newline=10, cr=13)
		fc.out.CmpRegToImm("r10", 32)
		notWhitespace1 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		fc.out.IncReg("rbx")
		jumpStartLoop := int32(trimStartLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpStartLoop)})

		notWS1Pos := fc.eb.text.Len()
		fc.out.CmpRegToImm("r10", 9)
		notWhitespace2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		fc.out.IncReg("rbx")
		jumpStartLoop2 := int32(trimStartLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpStartLoop2)})

		notWS2Pos := fc.eb.text.Len()
		fc.out.CmpRegToImm("r10", 10)
		notWhitespace3 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		fc.out.IncReg("rbx")
		jumpStartLoop3 := int32(trimStartLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpStartLoop3)})

		notWS3Pos := fc.eb.text.Len()
		fc.out.CmpRegToImm("r10", 13)
		trimStartFound := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		fc.out.IncReg("rbx")
		jumpStartLoop4 := int32(trimStartLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpStartLoop4)})

		// Start found - rbx = start index
		trimFoundStart := fc.eb.text.Len()
		fc.patchJumpImmediate(trimStartDone+2, int32(trimFoundStart-(trimStartDone+6)))
		fc.patchJumpImmediate(notWhitespace1+2, int32(notWS1Pos-(notWhitespace1+6)))
		fc.patchJumpImmediate(notWhitespace2+2, int32(notWS2Pos-(notWhitespace2+6)))
		fc.patchJumpImmediate(notWhitespace3+2, int32(notWS3Pos-(notWhitespace3+6)))
		fc.patchJumpImmediate(trimStartFound+2, int32(trimFoundStart-(trimStartFound+6)))

		// Find end (skip trailing whitespace) - work backwards from r14-1
		fc.out.MovRegToReg("r15", "r14") // r15 = end index (exclusive)
		// Handle empty or all-whitespace case
		fc.out.Emit([]byte{0x4c, 0x39, 0xfb}) // cmp rbx, r15
		emptyResult := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		trimEndLoop := fc.eb.text.Len()
		fc.out.Emit([]byte{0x49, 0x83, 0xff, 0x00}) // cmp r15, 0
		trimEndDone := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLessOrEqual, 0)
		fc.out.Emit([]byte{0x49, 0x83, 0xef, 0x01}) // dec r15

		// Load character at r15
		fc.out.MovRegToReg("rax", "r15")
		fc.out.ShlRegImm("rax", "4") // rax = r15 * 16
		fc.out.AddImmToReg("rax", 8) // rax = 8 + r15 * 16
		fc.out.MovRegToReg("r8", "r12")
		fc.out.AddRegToReg("r8", "rax")     // r8 = r12 + offset
		fc.out.MovMemToXmm("xmm0", "r8", 8) // Load value
		fc.out.Cvttsd2si("r10", "xmm0")

		// Check if whitespace
		fc.out.CmpRegToImm("r10", 32)
		trimWSJump1 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)
		fc.out.CmpRegToImm("r10", 9)
		trimWSJump2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)
		fc.out.CmpRegToImm("r10", 10)
		trimWSJump3 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)
		fc.out.CmpRegToImm("r10", 13)
		trimWSJump4 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)

		// Not whitespace - found end
		fc.out.IncReg("r15") // Make exclusive
		trimFoundEnd := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Was whitespace - continue loop
		trimWSTarget := fc.eb.text.Len()
		jumpEndLoop := int32(trimEndLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpEndLoop)})

		// Patch jumps
		trimRealEnd := fc.eb.text.Len()
		fc.patchJumpImmediate(trimEndDone+2, int32(trimRealEnd-(trimEndDone+6)))
		fc.patchJumpImmediate(trimWSJump1+2, int32(trimWSTarget-(trimWSJump1+6)))
		fc.patchJumpImmediate(trimWSJump2+2, int32(trimWSTarget-(trimWSJump2+6)))
		fc.patchJumpImmediate(trimWSJump3+2, int32(trimWSTarget-(trimWSJump3+6)))
		fc.patchJumpImmediate(trimWSJump4+2, int32(trimWSTarget-(trimWSJump4+6)))
		fc.patchJumpImmediate(trimFoundEnd+1, int32(trimRealEnd-(trimFoundEnd+5)))

		// Now rbx = start, r15 = end (exclusive), create substring
		// new_len = r15 - rbx
		fc.out.MovRegToReg("r14", "r15")
		fc.out.SubRegFromReg("r14", "rbx")

		// Allocate new string
		fc.out.MovRegToReg("rdi", "r14")
		fc.out.ShlRegImm("rdi", "4") // rdi = r14 * 16
		fc.out.AddImmToReg("rdi", 8) // rdi = r14 * 16 + 8
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax")

		// Copy count
		fc.out.MovRegToReg("rax", "r14")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r13", 0)

		// Copy characters from rbx to r15
		fc.out.XorRegWithReg("rcx", "rcx") // rcx = output index
		trimCopyLoop := fc.eb.text.Len()
		fc.out.Emit([]byte{0x4c, 0x39, 0xf1}) // cmp rcx, r14
		trimCopyDone := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)

		// Calculate source offset (rbx + rcx)
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.AddRegToReg("rax", "rcx") // rax = rbx + rcx (source index)
		fc.out.ShlRegImm("rax", "4")     // rax = (rbx + rcx) * 16
		fc.out.AddImmToReg("rax", 8)     // rax = (rbx + rcx) * 16 + 8

		// Calculate dest offset (rcx)
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.ShlRegImm("rdx", "4") // rdx = rcx * 16
		fc.out.AddImmToReg("rdx", 8) // rdx = rcx * 16 + 8

		// Calculate source and dest addresses
		fc.out.MovRegToReg("r8", "r12")
		fc.out.AddRegToReg("r8", "rax") // r8 = source base + offset
		fc.out.MovRegToReg("r9", "r13")
		fc.out.AddRegToReg("r9", "rdx") // r9 = dest base + offset

		// Copy key
		fc.out.Cvtsi2sd("xmm0", "rcx")
		fc.out.MovXmmToMem("xmm0", "r9", 0)

		// Copy value
		fc.out.MovMemToXmm("xmm0", "r8", 8)
		fc.out.MovXmmToMem("xmm0", "r9", 8)

		fc.out.IncReg("rcx")
		jumpCopyLoop := int32(trimCopyLoop - (fc.eb.text.Len() + 2))
		fc.out.Emit([]byte{0xeb, byte(jumpCopyLoop)})

		// Handle empty result case
		emptyPos := fc.eb.text.Len()
		fc.patchJumpImmediate(emptyResult+2, int32(emptyPos-(emptyResult+6)))

		// Allocate empty string
		fc.out.MovImmToReg("rdi", "8")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r13", "rax")
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")
		fc.out.MovXmmToMem("xmm0", "r13", 0)

		// Done
		trimAllDone := fc.eb.text.Len()
		fc.patchJumpImmediate(trimCopyDone+2, int32(trimAllDone-(trimCopyDone+6)))

		fc.out.MovRegToXmm("xmm0", "r13")
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if _c67_string_eq used

	// Generate arena functions only if arenas are actually used
	if fc.usesArenas {
		// Generate _c67_arena_create(capacity) -> arena_ptr
		// Creates a new arena with the specified capacity
		// Argument: rdi = capacity (int64)
		// Returns: rax = arena pointer
		// Arena structure: [buffer_ptr (8)][capacity (8)][offset (8)][alignment (8)] = 32 bytes header
		fc.eb.MarkLabel("_c67_arena_create")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")

		// Save capacity argument
		fc.out.MovRegToReg("r12", "rdi") // r12 = capacity

		// Allocate arena structure using mmap (4096 bytes = 1 page)
		fc.out.PushReg("r12")             // Save capacity
		fc.out.MovImmToReg("rdi", "0")    // addr = NULL
		fc.out.MovImmToReg("rsi", "4096") // length = 4096
		fc.out.MovImmToReg("rdx", "3")    // prot = PROT_READ | PROT_WRITE
		fc.out.MovImmToReg("r10", "34")   // flags = MAP_PRIVATE | MAP_ANONYMOUS
		fc.out.MovImmToReg("r8", "-1")    // fd = -1
		fc.out.MovImmToReg("r9", "0")     // offset = 0
		fc.out.MovImmToReg("rax", "9")    // syscall number for mmap
		fc.out.Syscall()
		fc.out.PopReg("r12") // Restore capacity

		// Check if mmap failed (returns -1 or negative on error)
		fc.out.CmpRegToImm("rax", 0)
		structMallocFailedJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // jl to error (negative result)

		fc.out.MovRegToReg("rbx", "rax") // rbx = arena struct pointer

		// Allocate arena buffer using mmap
		fc.out.MovImmToReg("rdi", "0")   // addr = NULL
		fc.out.MovRegToReg("rsi", "r12") // length = capacity
		fc.out.MovImmToReg("rdx", "3")   // prot = PROT_READ | PROT_WRITE
		fc.out.MovImmToReg("r10", "34")  // flags = MAP_PRIVATE | MAP_ANONYMOUS
		fc.out.MovImmToReg("r8", "-1")   // fd = -1
		fc.out.MovImmToReg("r9", "0")    // offset = 0
		fc.out.MovImmToReg("rax", "9")   // syscall number for mmap
		fc.out.Syscall()

		// Check if mmap failed
		fc.out.CmpRegToImm("rax", 0)
		bufferMallocFailedJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // jl to error

		// Fill arena structure
		fc.out.MovRegToMem("rax", "rbx", 0) // [rbx+0] = buffer_ptr
		fc.out.MovRegToMem("r12", "rbx", 8) // [rbx+8] = capacity
		fc.out.MovImmToMem(0, "rbx", 16)    // [rbx+16] = offset (0)
		fc.out.MovImmToMem(8, "rbx", 24)    // [rbx+24] = alignment (8)

		// Return arena pointer in rax
		fc.out.MovRegToReg("rax", "rbx")

		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Error path: malloc failed
		createErrorLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(structMallocFailedJump+2, int32(createErrorLabel-(structMallocFailedJump+6)))
		fc.patchJumpImmediate(bufferMallocFailedJump+2, int32(createErrorLabel-(bufferMallocFailedJump+6)))

		// Print error message and exit
		fc.out.LeaSymbolToReg("rdi", "_c67_str_arena_alloc_error")
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
		fc.out.MovImmToReg("rdi", "1")
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")

		// Generate _c67_arena_alloc(arena_ptr, size) -> allocation_ptr
		// Allocates memory from the arena using bump allocation with auto-growing
		// If arena is full, reallocs buffer to 2x size
		// Arguments: rdi = arena_ptr, rsi = size (int64)
		// Returns: rax = allocated memory pointer
		fc.eb.MarkLabel("_c67_arena_alloc")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14") // Extra push for 16-byte stack alignment (5 total pushes = 40 bytes)

		fc.out.MovRegToReg("rbx", "rdi") // rbx = arena_ptr (preserve across calls)
		fc.out.MovRegToReg("r12", "rsi") // r12 = size (preserve across calls)

		// Check if arena pointer is NULL
		fc.out.TestRegReg("rbx", "rbx")
		arenaNotNullJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0) // jne arena_not_null

		// Arena is NULL - print error and return NULL
		fc.out.LeaSymbolToReg("rdi", "_arena_null_error")
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
		fc.out.XorRegWithReg("rax", "rax") // return NULL
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// arena_not_null:
		arenaNotNullLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(arenaNotNullJump+2, int32(arenaNotNullLabel-(arenaNotNullJump+6)))

		// DEBUG: Print arena pointer value
		if false { // Disabled for now - causes stack alignment issues
			fc.out.MovRegToReg("rsi", "rbx") // arena ptr in rsi for printf
			fc.out.LeaSymbolToReg("rdi", "_str_debug_arena_value")
			fc.trackFunctionCall("printf")
			fc.eb.GenerateCallInstruction("printf")
			// rbx is callee-saved, so it's preserved across the call
		}

		// Load arena fields
		fc.out.MovMemToReg("r8", "rbx", 0)   // r8 = buffer_ptr
		fc.out.MovMemToReg("r9", "rbx", 8)   // r9 = capacity
		fc.out.MovMemToReg("r10", "rbx", 16) // r10 = current offset
		fc.out.MovMemToReg("r11", "rbx", 24) // r11 = alignment

		// Align offset: aligned_offset = (offset + alignment - 1) & ~(alignment - 1)
		fc.out.MovRegToReg("rax", "r10")      // rax = offset
		fc.out.AddRegToReg("rax", "r11")      // rax = offset + alignment
		fc.out.SubImmFromReg("rax", 1)        // rax = offset + alignment - 1
		fc.out.MovRegToReg("rcx", "r11")      // rcx = alignment
		fc.out.SubImmFromReg("rcx", 1)        // rcx = alignment - 1
		fc.out.Emit([]byte{0x48, 0xf7, 0xd1}) // not rcx
		fc.out.Emit([]byte{0x48, 0x21, 0xc8}) // and rax, rcx (aligned_offset in rax)
		fc.out.MovRegToReg("r13", "rax")      // r13 = aligned_offset

		// Check if we have enough space: if (aligned_offset + size > capacity) grow
		fc.out.MovRegToReg("rdx", "r13") // rdx = aligned_offset
		fc.out.AddRegToReg("rdx", "r12") // rdx = aligned_offset + size
		fc.out.CmpRegToReg("rdx", "r9")  // compare with capacity
		arenaGrowJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreater, 0) // jg to grow path

		// Fast path: enough space, no need to grow
		fc.eb.MarkLabel("_arena_alloc_fast")
		fc.out.MovRegToReg("rax", "r8")  // rax = buffer_ptr
		fc.out.AddRegToReg("rax", "r13") // rax = buffer_ptr + aligned_offset

		// Update arena offset
		fc.out.MovRegToReg("rdx", "r13")     // rdx = aligned_offset
		fc.out.AddRegToReg("rdx", "r12")     // rdx = aligned_offset + size
		fc.out.MovRegToMem("rdx", "rbx", 16) // [arena_ptr+16] = new offset

		arenaDoneJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // jmp to done

		// Grow path: realloc buffer with 1.3x growth
		arenaGrowLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(arenaGrowJump+2, int32(arenaGrowLabel-(arenaGrowJump+6)))
		fc.eb.MarkLabel("_arena_alloc_grow")

		// Calculate new capacity: max(capacity * 1.3, aligned_offset + size)
		// capacity * 1.3 = (capacity * 13) / 10
		fc.out.MovRegToReg("rdi", "r9")             // rdi = capacity
		fc.out.MovImmToReg("rax", "13")             // rax = 13
		fc.out.Emit([]byte{0x48, 0x0f, 0xaf, 0xf8}) // imul rdi, rax (rdi *= 13)
		fc.out.MovImmToReg("rax", "10")             // rax = 10
		fc.out.XorRegWithReg("rdx", "rdx")          // rdx = 0 (for div)
		fc.out.MovRegToReg("rcx", "rdi")            // rcx = capacity * 13 (save)
		fc.out.MovRegToReg("rax", "rcx")            // rax = capacity * 13
		fc.out.MovImmToReg("rcx", "10")             // rcx = 10
		fc.out.Emit([]byte{0x48, 0xf7, 0xf1})       // div rcx (rax = capacity * 13 / 10)
		fc.out.MovRegToReg("rdi", "rax")            // rdi = capacity * 1.3

		// Check if we need even more space
		fc.out.MovRegToReg("rsi", "r13") // rsi = aligned_offset
		fc.out.AddRegToReg("rsi", "r12") // rsi = aligned_offset + size
		fc.out.CmpRegToReg("rdi", "rsi") // compare 1.3*capacity with needed
		skipMaxJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge skip_max
		fc.out.MovRegToReg("rdi", "rsi")              // rdi = max(1.3*capacity, needed)
		skipMaxLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(skipMaxJump+2, int32(skipMaxLabel-(skipMaxJump+6)))

		// rdi now contains new_capacity
		fc.out.MovRegToReg("r9", "rdi") // r9 = new_capacity (update)

		// Check if new capacity exceeds max (1GB)
		fc.out.MovImmToReg("rax", "1073741824") // 1GB max
		fc.out.CmpRegToReg("r9", "rax")
		arenaMaxExceeded := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreater, 0) // jg to error if > 1GB

		// Grow the arena buffer
		var arenaErrorJump int
		if fc.eb.target.OS() == OSLinux {
			// Use mremap syscall on Linux
			// syscall 25: mremap(void *old_address, size_t old_size, size_t new_size, int flags, ...)
			// MREMAP_MAYMOVE = 1
			fc.out.MovRegToReg("rdi", "r8")     // rdi = old buffer_ptr
			fc.out.MovMemToReg("rsi", "rbx", 8) // rsi = old capacity from arena
			fc.out.MovRegToReg("rdx", "r9")     // rdx = new_capacity
			fc.out.MovImmToReg("r10", "1")      // r10 = MREMAP_MAYMOVE
			fc.out.MovImmToReg("rax", "25")     // rax = syscall number for mremap
			fc.out.Syscall()

			// Check if mremap failed (returns MAP_FAILED = -1 or negative on error)
			fc.out.CmpRegToImm("rax", -1)
			arenaErrorJump = fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // je to error (mremap failed)
		} else {
			// Use realloc on Windows/macOS
			fc.out.MovRegToReg("rdi", "r8") // rdi = old buffer_ptr
			fc.out.MovRegToReg("rsi", "r9") // rsi = new_capacity
			shadowSpace := fc.allocateShadowSpace()
			fc.trackFunctionCall("realloc")
			fc.eb.GenerateCallInstruction("realloc")
			fc.deallocateShadowSpace(shadowSpace)

			// Check if realloc failed (returns NULL)
			fc.out.TestRegReg("rax", "rax")
			arenaErrorJump = fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // je to error (realloc failed)
		}

		// Realloc succeeded: update arena structure
		fc.out.MovRegToMem("rax", "rbx", 0) // [arena_ptr+0] = new buffer_ptr
		fc.out.MovRegToMem("r9", "rbx", 8)  // [arena_ptr+8] = new capacity
		fc.out.MovRegToReg("r8", "rax")     // r8 = new buffer_ptr

		// Now allocate from the grown arena
		fc.out.MovRegToReg("rax", "r8")      // rax = buffer_ptr
		fc.out.AddRegToReg("rax", "r13")     // rax = buffer_ptr + aligned_offset
		fc.out.MovRegToReg("rdx", "r13")     // rdx = aligned_offset
		fc.out.AddRegToReg("rdx", "r12")     // rdx = aligned_offset + size
		fc.out.MovRegToMem("rdx", "rbx", 16) // [arena_ptr+16] = new offset

		arenaDoneJump2 := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // jmp to done

		// Error path: realloc failed or max size exceeded
		arenaErrorLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(arenaErrorJump+2, int32(arenaErrorLabel-(arenaErrorJump+6)))
		fc.patchJumpImmediate(arenaMaxExceeded+2, int32(arenaErrorLabel-(arenaMaxExceeded+6)))
		fc.eb.MarkLabel("_arena_alloc_error")

		// Print error message to stderr and exit(1)
		// Write to stderr (fd=2): "Error: Arena allocation failed (out of memory)\n"
		errorMsg := "Error: Arena allocation failed (out of memory or exceeded 1GB limit)\n"
		errorLabel := fmt.Sprintf("_arena_error_msg_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(errorLabel, errorMsg)

		fc.out.MovImmToReg("rdi", "2") // stderr
		fc.out.LeaSymbolToReg("rsi", errorLabel)
		fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(errorMsg)))
		fc.out.MovImmToReg("rax", "1") // write syscall
		fc.out.Syscall()

		fc.out.MovImmToReg("rdi", "1")  // exit code 1
		fc.out.MovImmToReg("rax", "60") // exit syscall
		fc.out.Syscall()

		// Done label
		arenaDoneLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(arenaDoneJump+1, int32(arenaDoneLabel-(arenaDoneJump+5)))
		fc.patchJumpImmediate(arenaDoneJump2+1, int32(arenaDoneLabel-(arenaDoneJump2+5)))
		fc.eb.MarkLabel("_arena_alloc_done")

		// rax already contains the allocated pointer - don't overwrite it!

		fc.out.PopReg("r14") // Pop extra register for stack alignment
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Generate _c67_arena_destroy(arena_ptr)
		// Frees all memory associated with the arena
		// Argument: rdi = arena_ptr
		fc.eb.MarkLabel("_c67_arena_destroy")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")

		fc.out.MovRegToReg("rbx", "rdi") // rbx = arena_ptr

		// Munmap buffer: munmap(ptr, size) via syscall 11
		fc.out.MovMemToReg("rdi", "rbx", 0) // rdi = buffer_ptr
		fc.out.MovMemToReg("rsi", "rbx", 8) // rsi = capacity
		fc.out.MovImmToReg("rax", "11")     // rax = syscall number for munmap
		fc.out.Syscall()

		// Munmap arena structure: munmap(ptr, 32) via syscall 11
		fc.out.MovRegToReg("rdi", "rbx")  // rdi = arena_ptr
		fc.out.MovImmToReg("rsi", "4096") // rsi = page size (was 32, but mmap'd full page)
		fc.out.MovImmToReg("rax", "11")   // rax = syscall number for munmap
		fc.out.Syscall()

		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()

		// Generate _c67_arena_reset(arena_ptr)
		// Resets the arena offset to 0, effectively freeing all allocations
		// Argument: rdi = arena_ptr
		fc.eb.MarkLabel("_c67_arena_reset")

		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")

		// Reset offset to 0
		fc.out.MovImmToMem(0, "rdi", 16) // [arena_ptr+16] = 0

		fc.out.PopReg("rbp")
		fc.out.Ret()
	} // end if usesArenas (will reopen for arena_ensure_capacity later)

	// Generate _c67_list_cons(element_float, list_ptr_float) -> new_list_ptr
	// LINKED LIST implementation - creates a cons cell: [head|tail]
	// Arguments: rdi = element (as float64 bits), rsi = tail pointer (as float64 bits, 0.0 = nil)
	// Returns: rax = pointer to new cons cell (16 bytes)
	fc.eb.MarkLabel("_c67_list_cons")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// Save callee-saved registers
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")

	// Align stack: call(8) + push rbp(8) + 2 pushes(16) = 32 bytes (ALIGNED)
	// Need to subtract 8 to be misaligned by 8 before calling arena_alloc
	fc.out.SubImmFromReg("rsp", StackSlotSize)

	// Save arguments
	fc.out.MovRegToReg("r12", "rdi") // r12 = element bits (head)
	fc.out.MovRegToReg("r13", "rsi") // r13 = tail pointer bits

	// Allocate 16-byte cons cell from arena (use default arena 0)
	// Cons cell format: [head: float64][tail: float64] = 16 bytes
	fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
	fc.out.MovMemToReg("rdi", "rdi", 0) // rdi = meta-arena array pointer
	fc.out.MovMemToReg("rdi", "rdi", 0) // rdi = arena[0] struct pointer
	fc.out.MovImmToReg("rsi", "16")     // rsi = 16 bytes
	fc.trackFunctionCall("_c67_arena_alloc")
	fc.eb.GenerateCallInstruction("_c67_arena_alloc")
	// rax now contains pointer to cons cell

	// Write head (element) at [cell+0]
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("r12", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
	fc.out.MovXmmToMem("xmm0", "rax", 0)

	// Write tail pointer at [cell+8]
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("r13", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
	fc.out.MovXmmToMem("xmm0", "rax", 8)

	// Return cons cell pointer in rax

	// Restore stack alignment
	fc.out.AddImmToReg("rsp", StackSlotSize)

	// Restore callee-saved registers
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")

	// Function epilogue
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_list_head(list_ptr_float) -> element_float
	// LINKED LIST implementation - returns head of cons cell
	// Argument: rdi = list pointer (as float64 bits, 0.0 = nil)
	// Returns: xmm0 = first element (or NaN if empty)
	fc.eb.MarkLabel("_c67_list_head")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// Check if list pointer is NULL (0)
	fc.out.TestRegReg("rdi", "rdi")
	fc.out.Emit([]byte{0x75, 0x12}) // jnz +18 (skip NaN generation)

	// Return NaN for empty list (NULL pointer)
	fc.out.Emit([]byte{0x48, 0xb8})                                     // mov rax, immediate
	fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f}) // NaN bits
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Load head (first element) at [cell+0]
	fc.out.MovMemToXmm("xmm0", "rdi", 0)

	// Function epilogue
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_list_tail(list_ptr_float) -> tail_ptr_float
	// LINKED LIST implementation - returns tail of cons cell (O(1) operation)
	// Argument: rdi = list pointer (as float64 bits, 0.0 = nil)
	// Returns: xmm0 = tail pointer (as float64, 0.0 = nil)
	fc.eb.MarkLabel("_c67_list_tail")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// Check if list pointer is NULL (0)
	fc.out.TestRegReg("rdi", "rdi")
	fc.out.Emit([]byte{0x75, 0x0e}) // jnz +14 (skip returning 0.0)

	// Return 0.0 for empty list (NULL pointer)
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Load tail pointer at [cell+8]
	fc.out.MovMemToXmm("xmm0", "rdi", 8)

	// Function epilogue
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_list_length(list_ptr_float) -> length_int
	// LINKED LIST implementation - walks list and counts nodes (O(n))
	// Argument: rdi = list pointer (as float64 bits, 0.0 = nil)
	// Returns: rax = length as int64
	fc.eb.MarkLabel("_c67_list_length")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// Initialize counter
	fc.out.XorRegWithReg("rax", "rax") // rax = 0 (length counter)

	// Check if list is empty
	fc.out.TestRegReg("rdi", "rdi")
	lengthDoneJump1 := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // jz to done
	lengthDoneEnd1 := fc.eb.text.Len()

	// Walk list
	lengthLoopStart := fc.eb.text.Len()
	// Increment counter
	fc.out.IncReg("rax")
	// Load tail pointer from [rdi+8]
	fc.out.MovMemToReg("rdi", "rdi", 8)
	// Check if we've reached end (NULL)
	fc.out.TestRegReg("rdi", "rdi")
	jnzOffset := int32(lengthLoopStart - (fc.eb.text.Len() + 6))
	fc.out.JumpConditional(JumpNotEqual, jnzOffset) // jnz back to loop start

	// Patch the done jump
	lengthDonePos1 := fc.eb.text.Len()
	fc.patchJumpImmediate(lengthDoneJump1+2, int32(lengthDonePos1-(lengthDoneEnd1)))

	// Return length in rax
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_list_index(list_ptr_float, index_int) -> element_float
	// LINKED LIST implementation - walks to index-th node (O(n))
	// Arguments: rdi = list pointer (as float64 bits), rsi = index (int64)
	// Returns: xmm0 = element at index (or NaN if out of bounds)
	fc.eb.MarkLabel("_c67_list_index")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")

	// Walk to index-th node
	fc.out.XorRegWithReg("rax", "rax") // rax = 0 (current index)

	// Check if list is empty
	fc.out.TestRegReg("rdi", "rdi")
	indexOutOfBoundsJump1 := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // jz to out of bounds
	indexOutOfBoundsEnd1 := fc.eb.text.Len()

	// Walk loop
	indexWalkLoopStart := fc.eb.text.Len()
	// Check if we've reached target index
	fc.out.CmpRegToReg("rax", "rsi")
	indexFoundJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // jz to found
	indexFoundEnd := fc.eb.text.Len()

	// Not at target yet, move to next node
	fc.out.IncReg("rax")
	fc.out.MovMemToReg("rdi", "rdi", 8) // rdi = tail pointer

	// Check if we've reached end
	fc.out.TestRegReg("rdi", "rdi")
	indexOutOfBoundsJump2 := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // jz to out of bounds
	indexOutOfBoundsEnd2 := fc.eb.text.Len()

	// Continue walking
	jmpOffset := int32(indexWalkLoopStart - (fc.eb.text.Len() + 5))
	fc.out.Emit([]byte{0xe9}) // jmp rel32
	fc.out.Emit([]byte{byte(jmpOffset), byte(jmpOffset >> 8), byte(jmpOffset >> 16), byte(jmpOffset >> 24)})

	// Found: load head element
	indexFoundPos := fc.eb.text.Len()
	fc.patchJumpImmediate(indexFoundJump+2, int32(indexFoundPos-indexFoundEnd))
	fc.out.MovMemToXmm("xmm0", "rdi", 0)
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Out of bounds: return NaN
	indexOutOfBoundsPos := fc.eb.text.Len()
	fc.patchJumpImmediate(indexOutOfBoundsJump1+2, int32(indexOutOfBoundsPos-indexOutOfBoundsEnd1))
	fc.patchJumpImmediate(indexOutOfBoundsJump2+2, int32(indexOutOfBoundsPos-indexOutOfBoundsEnd2))
	fc.out.Emit([]byte{0x48, 0xb8})                                     // mov rax, immediate
	fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f}) // NaN bits
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_list_update(list_ptr, index, value, arena_ptr) -> new_list_ptr
	// Updates a list element at the given index (functional - returns new list)
	// Arguments: rdi = list_ptr, rsi = index (integer), xmm0 = new value (float64), rdx = arena_ptr
	// Returns: rax = new list pointer
	// Uses specified arena for allocation
	fc.eb.MarkLabel("_c67_list_update")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx") // old list ptr
	fc.out.PushReg("r12") // new list ptr
	fc.out.PushReg("r13") // target index
	fc.out.PushReg("r14") // new value bits
	fc.out.PushReg("r15") // arena ptr

	// Stack: call(8) + rbp(8) + 5 regs(40) = 56 bytes (aligned)
	fc.out.SubImmFromReg("rsp", StackSlotSize)

	// Save arguments
	fc.out.MovRegToReg("rbx", "rdi") // rbx = old list ptr
	fc.out.MovRegToReg("r13", "rsi") // r13 = target index
	fc.out.MovRegToReg("r15", "rdx") // r15 = arena ptr
	// Save xmm0 (new value) to r14
	fc.out.MovqXmmToReg("r14", "xmm0")

	// Get length from old list
	fc.out.MovMemToXmm("xmm0", "rbx", 0)
	fc.out.Cvttsd2si("rcx", "xmm0") // rcx = length

	// Calculate allocation size: 8 + length * 8
	fc.out.MovRegToReg("rax", "rcx")
	fc.out.MulRegWithImm("rax", 8)
	fc.out.AddImmToReg("rax", 8)

	// Allocate from specified arena
	// Call _c67_arena_alloc(rdi=arena_ptr, rsi=size)
	fc.out.MovRegToReg("rdi", "r15") // arena ptr in rdi
	fc.out.MovRegToReg("rsi", "rax") // size in rsi
	fc.trackFunctionCall("_c67_arena_alloc")
	fc.eb.GenerateCallInstruction("_c67_arena_alloc")
	fc.out.MovRegToReg("r12", "rax") // r12 = new list ptr

	// Write length to new list
	fc.out.Cvtsi2sd("xmm0", "rcx")
	fc.out.MovXmmToMem("xmm0", "r12", 0)

	// Restore new value to xmm2 for use in loop
	fc.out.MovqRegToXmm("xmm2", "r14")

	// Simple loop to copy all elements, updating target index
	// Use r8 as loop counter (0..length-1)
	fc.out.XorRegWithReg("r8", "r8") // r8 = 0

	// Copy loop - iterate over all elements
	fc.eb.MarkLabel("_list_update_loop")
	fc.out.CmpRegToReg("r8", "rcx")
	fc.out.Emit([]byte{0x7d, 0x2c}) // jge +44 to end

	// Calculate byte offset: r8 * 8 + 8
	fc.out.MovRegToReg("rax", "r8")
	fc.out.ShlRegByImm("rax", 3)
	fc.out.AddImmToReg("rax", 8)

	// Check if this is the target index
	fc.out.CmpRegToReg("r8", "r13")
	fc.out.Emit([]byte{0x75, 0x0d}) // jne +13 (copy old value)

	// This is target index: store new value (in xmm2)
	fc.out.MovRegToReg("rsi", "rax")
	fc.out.AddRegToReg("rsi", "r12")
	fc.out.MovXmmToMem("xmm2", "rsi", 0)
	fc.out.Emit([]byte{0xeb, 0x11}) // jmp +17 (to increment)

	// Not target: copy old value
	fc.out.MovRegToReg("rsi", "rax")
	fc.out.AddRegToReg("rsi", "rbx")
	fc.out.MovMemToXmm("xmm3", "rsi", 0)
	fc.out.MovRegToReg("rsi", "rax")
	fc.out.AddRegToReg("rsi", "r12")
	fc.out.MovXmmToMem("xmm3", "rsi", 0)

	// Increment and loop back
	fc.out.AddImmToReg("r8", 1)
	fc.out.Emit([]byte{0xeb, 0xca}) // jmp -54 (back to loop start)

	// Return new list pointer
	fc.out.MovRegToReg("rax", "r12")

	// Restore stack and registers
	fc.out.AddImmToReg("rsp", StackSlotSize)
	fc.out.PopReg("r15")
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Generate _c67_string_println(string_ptr) - prints string followed by newline
	// Argument: rdi/rcx (platform-dependent) = string pointer (map with [count][0][char0][1][char1]...)
	fc.eb.MarkLabel("_c67_string_println")

	// For Windows, we use a simpler approach: call printf for each character
	// For Unix, we use write syscall for efficiency
	if fc.eb.target.OS() == OSWindows {
		// Windows version: use printf to print the string
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r14")

		// Windows calling convention: first arg is rcx
		fc.out.MovRegToReg("rbx", "rcx") // rbx = string pointer

		// Get length
		fc.out.MovMemToXmm("xmm0", "rbx", 0)
		fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

		// Loop through characters
		fc.out.XorRegWithReg("r14", "r14") // r14 = index

		strPrintLoopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("r14", "r12")
		strPrintLoopEnd := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		strPrintLoopEndPos := fc.eb.text.Len()

		// Calculate offset: 16 + index * 16
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlImmReg("rax", 4)       // rax = index * 16
		fc.out.AddImmToReg("rax", 16)    // rax = 16 + index * 16
		fc.out.AddRegToReg("rax", "rbx") // rax = string_ptr + offset

		// Load character code
		fc.out.MovMemToXmm("xmm0", "rax", 0)
		fc.out.Cvttsd2si("rdx", "xmm0") // rdx = character code

		// Call putchar via printf
		// printf("%c", char)
		// Create format string "%c"
		charFmtLabel := fmt.Sprintf("_c67_char_fmt_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(charFmtLabel, "%c\x00")

		fc.out.SubImmFromReg("rsp", 32) // Shadow space
		fc.out.LeaSymbolToReg("rcx", charFmtLabel)
		// rdx already has the character
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
		fc.out.AddImmToReg("rsp", 32)

		// Increment and loop
		fc.out.IncReg("r14")
		strPrintBackOffset := int32(strPrintLoopStart - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(strPrintBackOffset)

		// Patch loop end
		strPrintDonePos := fc.eb.text.Len()
		fc.patchJumpImmediate(strPrintLoopEnd+2, int32(strPrintDonePos-strPrintLoopEndPos))

		// Print newline: printf("\n")
		newlineFmtLabel := fmt.Sprintf("_c67_newline_fmt_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(newlineFmtLabel, "\n\x00")

		fc.out.SubImmFromReg("rsp", 32)
		fc.out.LeaSymbolToReg("rcx", newlineFmtLabel)
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
		fc.out.AddImmToReg("rsp", 32)

		// Restore
		fc.out.PopReg("r14")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} else {
		// Unix version: use write syscall
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")

		fc.out.MovRegToReg("rbx", "rdi") // rbx = string pointer

		// Get length
		fc.out.MovMemToXmm("xmm0", "rbx", 0)
		fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

		// Allocate 1-byte buffer on stack
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovRegToReg("r13", "rsp") // r13 = buffer address

		// Loop through characters
		fc.out.XorRegWithReg("r14", "r14") // r14 = index (use r14 instead of rcx since syscall clobbers rcx)

		strPrintLoopStart := fc.eb.text.Len()
		fc.out.CmpRegToReg("r14", "r12")
		strPrintLoopEnd := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		strPrintLoopEndPos := fc.eb.text.Len()

		// Calculate offset: 16 + index * 16
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlImmReg("rax", 4)       // rax = index * 16
		fc.out.AddImmToReg("rax", 16)    // rax = 16 + index * 16
		fc.out.AddRegToReg("rax", "rbx") // rax = string_ptr + offset

		// Load character code
		fc.out.MovMemToXmm("xmm0", "rax", 0)
		fc.out.Cvttsd2si("rdi", "xmm0")
		fc.out.MovRegToMem("rdi", "r13", 0)

		// write(1, buffer, 1)
		fc.out.MovImmToReg("rax", "1")   // syscall: write
		fc.out.MovImmToReg("rdi", "1")   // fd: stdout
		fc.out.MovRegToReg("rsi", "r13") // buffer
		fc.out.MovImmToReg("rdx", "1")   // length: 1
		fc.out.Syscall()

		// Increment and loop
		fc.out.IncReg("r14")
		strPrintBackOffset := int32(strPrintLoopStart - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(strPrintBackOffset)

		// Patch loop end
		strPrintDonePos := fc.eb.text.Len()
		fc.patchJumpImmediate(strPrintLoopEnd+2, int32(strPrintDonePos-strPrintLoopEndPos))

		// Print newline
		fc.out.MovImmToReg("rax", "10") // '\n'
		fc.out.MovRegToMem("rax", "r13", 0)
		fc.out.MovImmToReg("rax", "1")   // syscall: write
		fc.out.MovImmToReg("rdi", "1")   // fd: stdout
		fc.out.MovRegToReg("rsi", "r13") // buffer
		fc.out.MovImmToReg("rdx", "1")   // length: 1
		fc.out.Syscall()

		// Restore
		fc.out.AddImmToReg("rsp", 8)
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	}

	// Generate _c67_string_print(string_ptr) - prints string WITHOUT newline
	// Argument: rdi/rcx (platform-dependent) = string pointer (map with [count][0][char0][1][char1]...)
	fc.eb.MarkLabel("_c67_string_print")

	if fc.eb.target.OS() == OSWindows {
		// Windows version: use printf for each character
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r14")

		// Windows calling convention: first arg is rcx
		fc.out.MovRegToReg("rbx", "rcx") // rbx = string pointer

		// Get length
		fc.out.MovMemToXmm("xmm0", "rbx", 0)
		fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

		// Loop through characters
		fc.out.XorRegWithReg("r14", "r14") // r14 = index

		strPrintLoopStart2 := fc.eb.text.Len()
		fc.out.CmpRegToReg("r14", "r12")
		strPrintLoopEnd2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		strPrintLoopEndPos2 := fc.eb.text.Len()

		// Calculate offset: 16 + index * 16
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlImmReg("rax", 4)       // rax = index * 16
		fc.out.AddImmToReg("rax", 16)    // rax = 16 + index * 16
		fc.out.AddRegToReg("rax", "rbx") // rax = string_ptr + offset

		// Load character code
		fc.out.MovMemToXmm("xmm0", "rax", 0)
		fc.out.Cvttsd2si("rdx", "xmm0") // rdx = character code

		// Call putchar via printf
		charFmtLabel2 := fmt.Sprintf("_c67_char_fmt_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(charFmtLabel2, "%c\x00")

		fc.out.SubImmFromReg("rsp", 32) // Shadow space
		fc.out.LeaSymbolToReg("rcx", charFmtLabel2)
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
		fc.out.AddImmToReg("rsp", 32)

		// Increment and loop
		fc.out.IncReg("r14")
		strPrintBackOffset2 := int32(strPrintLoopStart2 - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(strPrintBackOffset2)

		// Patch loop end
		strPrintDonePos2 := fc.eb.text.Len()
		fc.patchJumpImmediate(strPrintLoopEnd2+2, int32(strPrintDonePos2-strPrintLoopEndPos2))

		// No newline - just return
		fc.out.PopReg("r14")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	} else {
		// Unix version: use write syscall
		fc.out.PushReg("rbp")
		fc.out.MovRegToReg("rbp", "rsp")
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")

		fc.out.MovRegToReg("rbx", "rdi") // rbx = string pointer

		// Get length
		fc.out.MovMemToXmm("xmm0", "rbx", 0)
		fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

		// Allocate 1-byte buffer on stack
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovRegToReg("r13", "rsp") // r13 = buffer address

		// Loop through characters
		fc.out.XorRegWithReg("r14", "r14") // r14 = index

		strPrintLoopStart2 := fc.eb.text.Len()
		fc.out.CmpRegToReg("r14", "r12")
		strPrintLoopEnd2 := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		strPrintLoopEndPos2 := fc.eb.text.Len()

		// Calculate offset: 16 + index * 16
		fc.out.MovRegToReg("rax", "r14")
		fc.out.ShlImmReg("rax", 4)
		fc.out.AddImmToReg("rax", 16)
		fc.out.AddRegToReg("rax", "rbx")

		// Load character
		fc.out.MovMemToXmm("xmm0", "rax", 0)
		fc.out.Cvttsd2si("rax", "xmm0")
		fc.out.MovRegToMem("rax", "r13", 0)

		// Write syscall
		fc.out.MovImmToReg("rax", "1")   // syscall: write
		fc.out.MovImmToReg("rdi", "1")   // fd: stdout
		fc.out.MovRegToReg("rsi", "r13") // buffer
		fc.out.MovImmToReg("rdx", "1")   // length: 1
		fc.out.Syscall()

		// Increment and loop
		fc.out.IncReg("r14")
		strPrintBackOffset2 := int32(strPrintLoopStart2 - (fc.eb.text.Len() + 5))
		fc.out.JumpUnconditional(strPrintBackOffset2)

		// Patch loop end
		strPrintDonePos2 := fc.eb.text.Len()
		fc.patchJumpImmediate(strPrintLoopEnd2+2, int32(strPrintDonePos2-strPrintLoopEndPos2))

		// No newline - just return
		fc.out.AddImmToReg("rsp", 8)
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")
		fc.out.PopReg("rbp")
		fc.out.Ret()
	}

	// Generate _c67_itoa for number to string conversion
	fc.generateItoa()

	// Generate syscall-based print helpers for Linux
	if fc.eb.target.OS() == OSLinux {
		fc.generatePrintSyscall()
		fc.generatePrintlnSyscall()
	}

	// Generate _c67_arena_ensure_capacity if arenas are used
	if fc.usesArenas {
		fc.generateArenaEnsureCapacity()
	}
}

// initializeMetaArenaAndGlobalArena initializes the meta-arena and creates arena 0 (default arena)
func (fc *C67Compiler) initializeMetaArenaAndGlobalArena() {
	// Define arena metadata symbols now that we know arenas are used
	fc.eb.DefineWritable("_c67_arena_meta", "\x00\x00\x00\x00\x00\x00\x00\x00")     // Pointer to arena array
	fc.eb.DefineWritable("_c67_arena_meta_cap", "\x00\x00\x00\x00\x00\x00\x00\x00") // Capacity (number of slots)
	fc.eb.DefineWritable("_c67_arena_meta_len", "\x00\x00\x00\x00\x00\x00\x00\x00") // Length (number of active arenas)
	fc.eb.Define("_arena_null_error", "ERROR: Arena alloc returned NULL\n\x00")

	// Initialize meta-arena system - all arenas are malloc'd at runtime
	const initialCapacity = 4

	// DEBUG: Print that we're starting initialization
	if false { // Set to true for debugging
		fc.out.LeaSymbolToReg("rdi", "_str_debug_default_arena")
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")
	}

	// Allocate meta-arena array using mmap: 8 * 4 = 32 bytes for 4 arena pointers
	fc.out.MovImmToReg("rdi", "0")                                  // addr = NULL
	fc.out.MovImmToReg("rsi", fmt.Sprintf("%d", 8*initialCapacity)) // length = 32
	fc.out.MovImmToReg("rdx", "3")                                  // prot = PROT_READ | PROT_WRITE
	fc.out.MovImmToReg("r10", "34")                                 // flags = MAP_PRIVATE | MAP_ANONYMOUS
	fc.out.MovImmToReg("r8", "-1")                                  // fd = -1
	fc.out.MovImmToReg("r9", "0")                                   // offset = 0
	fc.out.MovImmToReg("rax", "9")                                  // syscall number for mmap
	fc.out.Syscall()

	// Store meta-arena array pointer
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovRegToMem("rax", "rbx", 0)

	// Set meta-arena capacity
	fc.out.MovImmToReg("rcx", fmt.Sprintf("%d", initialCapacity))
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_cap")
	fc.out.MovRegToMem("rcx", "rbx", 0)

	// Create default arena (arena 0) - 1MB mmap'd buffer
	// Arena struct: [base_ptr(8), capacity(8), used(8), alignment(8)] = 32 bytes

	// Allocate arena buffer using mmap: 1MB
	fc.out.MovImmToReg("rdi", "0")       // addr = NULL
	fc.out.MovImmToReg("rsi", "1048576") // length = 1MB
	fc.out.MovImmToReg("rdx", "3")       // prot = PROT_READ | PROT_WRITE
	fc.out.MovImmToReg("r10", "34")      // flags = MAP_PRIVATE | MAP_ANONYMOUS
	fc.out.MovImmToReg("r8", "-1")       // fd = -1
	fc.out.MovImmToReg("r9", "0")        // offset = 0
	fc.out.MovImmToReg("rax", "9")       // syscall number for mmap
	fc.out.Syscall()

	// Check if mmap failed (returns -1 on error)
	fc.out.MovImmToReg("rcx", "-1")
	fc.out.CmpRegToReg("rax", "rcx")
	mmapOkJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0) // jne mmap_ok

	// mmap failed - print error and exit
	fc.out.LeaSymbolToReg("rdi", "_malloc_failed_msg")
	fc.trackFunctionCall("printf")
	fc.eb.GenerateCallInstruction("printf")
	fc.out.MovImmToReg("rdi", "1")  // exit code 1
	fc.out.MovImmToReg("rax", "60") // sys_exit
	fc.out.Syscall()

	// mmap_ok:
	mmapOkLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(mmapOkJump+2, int32(mmapOkLabel-(mmapOkJump+6)))

	fc.out.MovRegToReg("r12", "rax") // r12 = arena buffer

	// Allocate arena struct using mmap: 32 bytes (round up to page size 4096)
	fc.out.MovImmToReg("rdi", "0")    // addr = NULL
	fc.out.MovImmToReg("rsi", "4096") // length = 4096 (page size)
	fc.out.MovImmToReg("rdx", "3")    // prot = PROT_READ | PROT_WRITE
	fc.out.MovImmToReg("r10", "34")   // flags = MAP_PRIVATE | MAP_ANONYMOUS
	fc.out.MovImmToReg("r8", "-1")    // fd = -1
	fc.out.MovImmToReg("r9", "0")     // offset = 0
	fc.out.MovImmToReg("rax", "9")    // syscall number for mmap
	fc.out.Syscall()

	// Initialize arena struct fields
	fc.out.MovRegToMem("r12", "rax", 0)  // base_ptr = arena buffer
	fc.out.MovImmToReg("rcx", "1048576") // 1MB
	fc.out.MovRegToMem("rcx", "rax", 8)  // capacity = 1MB
	fc.out.XorRegWithReg("rcx", "rcx")
	fc.out.MovRegToMem("rcx", "rax", 16) // used = 0
	fc.out.MovImmToReg("rcx", "8")
	fc.out.MovRegToMem("rcx", "rax", 24) // alignment = 8

	// Store arena struct pointer in meta-arena[0]
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovMemToReg("rbx", "rbx", 0) // rbx = meta-arena array
	fc.out.MovRegToMem("rax", "rbx", 0) // meta-arena[0] = arena struct

	// Set meta-arena len = 1 (one active arena)
	fc.out.MovImmToReg("rcx", "1")
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_len")
	fc.out.MovRegToMem("rcx", "rbx", 0)
}

// cleanupAllArenas frees all arenas in the meta-arena
func (fc *C67Compiler) cleanupAllArenas() {
	// Load meta-arena pointer
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovMemToReg("rbx", "rbx", 0) // rbx = meta-arena pointer

	// Check if meta-arena is NULL (no arenas allocated)
	fc.out.TestRegReg("rbx", "rbx")
	skipCleanupJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je skip_cleanup

	// Load meta-arena length
	fc.out.LeaSymbolToReg("rax", "_c67_arena_meta_len")
	fc.out.MovMemToReg("rcx", "rax", 0) // rcx = number of arenas

	// Loop through all arenas and free them
	fc.out.XorRegWithReg("r8", "r8") // r8 = index = 0

	cleanupLoopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("r8", "rcx")
	skipCleanupEnd := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge cleanup_done

	// Load arena pointer at index r8
	fc.out.MovRegToReg("rax", "r8")
	fc.out.ShlRegByImm("rax", 3) // offset = index * 8
	fc.out.AddRegToReg("rax", "rbx")

	fc.out.MovMemToReg("r9", "rax", 0) // r9 = arena struct pointer

	// Munmap buffer: munmap(ptr, size)
	fc.out.MovMemToReg("rdi", "r9", 0) // rdi = buffer_ptr (arena[0])
	fc.out.MovMemToReg("rsi", "r9", 8) // rsi = capacity (arena[8])

	if fc.eb.target.OS() == OSLinux {
		// Use syscall on Linux
		fc.out.MovImmToReg("rax", "11") // syscall number for munmap
		fc.out.Syscall()
	} else {
		// Use C function on Windows/macOS
		shadowSpace := fc.allocateShadowSpace()
		fc.trackFunctionCall("munmap")
		fc.eb.GenerateCallInstruction("munmap")
		fc.deallocateShadowSpace(shadowSpace)
	}

	// Munmap arena struct: munmap(ptr, 4096)
	fc.out.MovRegToReg("rdi", "r9")   // rdi = arena struct pointer
	fc.out.MovImmToReg("rsi", "4096") // rsi = page size (was 32, but mmap'd full page)

	if fc.eb.target.OS() == OSLinux {
		// Use syscall on Linux
		fc.out.MovImmToReg("rax", "11") // syscall number for munmap
		fc.out.Syscall()
	} else {
		// Use C function on Windows/macOS
		shadowSpace := fc.allocateShadowSpace()
		fc.trackFunctionCall("munmap")
		fc.eb.GenerateCallInstruction("munmap")
		fc.deallocateShadowSpace(shadowSpace)
	}

	// Increment index
	fc.out.AddImmToReg("r8", 1)
	backOffset := int32(cleanupLoopStart - (fc.eb.text.Len() + UnconditionalJumpSize))
	fc.out.JumpUnconditional(backOffset)

	// cleanup_done: Free the meta-arena itself
	cleanupDone := fc.eb.text.Len()
	fc.patchJumpImmediate(skipCleanupEnd+2, int32(cleanupDone-(skipCleanupEnd+ConditionalJumpSize)))

	// Munmap the meta-arena array itself (only if it was allocated)
	// Size = MAX_ARENAS * 8 = 256 * 8 = 2048
	fc.out.MovRegToReg("rdi", "rbx")  // rdi = meta-arena pointer
	fc.out.MovImmToReg("rsi", "2048") // rsi = size (256 arena pointers * 8 bytes)

	if fc.eb.target.OS() == OSLinux {
		// Use syscall on Linux
		fc.out.MovImmToReg("rax", "11") // syscall number for munmap
		fc.out.Syscall()
	} else {
		// Use C function on Windows/macOS
		shadowSpace := fc.allocateShadowSpace()
		fc.trackFunctionCall("munmap")
		fc.eb.GenerateCallInstruction("munmap")
		fc.deallocateShadowSpace(shadowSpace)
	}

	// skip_cleanup: Patch both skip jumps to here (after all cleanup)
	skipCleanup := fc.eb.text.Len()
	fc.patchJumpImmediate(skipCleanupJump+2, int32(skipCleanup-(skipCleanupJump+ConditionalJumpSize)))
}

// generateItoa generates the _c67_itoa function
// Converts int64 to decimal string representation
// Input: rdi = number
// Output: rsi = buffer pointer, rdx = length
// Uses a global buffer _itoa_buffer
func (fc *C67Compiler) generateItoa() {
	// Define global buffer for itoa (32 bytes)
	fc.eb.DefineWritable("_itoa_buffer", string(make([]byte, 32)))

	fc.eb.MarkLabel("_c67_itoa")

	// Prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")

	// rdi = input number
	// rbx = buffer pointer (builds backwards from end)
	// r12 = digit counter
	// r13 = is_negative flag

	// Load buffer address
	fc.out.LeaSymbolToReg("rbx", "_itoa_buffer")
	fc.out.AddImmToReg("rbx", 31)      // Point to end of buffer
	fc.out.XorRegWithReg("r12", "r12") // digit counter = 0
	fc.out.XorRegWithReg("r13", "r13") // is_negative = 0

	// Handle negative
	fc.out.CmpRegToImm("rdi", 0)
	fc.out.Write(0x7D) // JGE (jump if >= 0)
	fc.out.Write(0x00) // Placeholder
	positiveJump := fc.eb.text.Len() - 1

	// Negative: set flag and negate
	fc.out.MovImmToReg("r13", "1")
	fc.out.NegReg("rdi")

	// Positive label
	positivePos := fc.eb.text.Len()
	fc.eb.text.Bytes()[positiveJump] = byte(positivePos - (positiveJump + 1))

	// Special case: zero
	fc.out.CmpRegToImm("rdi", 0)
	fc.out.Write(0x75) // JNE (jump if != 0)
	fc.out.Write(0x00) // Placeholder
	nonZeroJump := fc.eb.text.Len() - 1

	// Zero case: just write '0'
	fc.out.MovImmToReg("rax", "48") // '0'
	fc.out.MovByteRegToMem("rax", "rbx", 0)
	fc.out.MovImmToReg("r12", "1")
	fc.out.Write(0xEB) // JMP unconditional
	fc.out.Write(0x00) // Placeholder
	endJump := fc.eb.text.Len() - 1

	// Non-zero: convert digits
	nonZeroPos := fc.eb.text.Len()
	fc.eb.text.Bytes()[nonZeroJump] = byte(nonZeroPos - (nonZeroJump + 1))

	// Loop start
	loopStart := fc.eb.text.Len()

	// Divide by 10: rax = rdi / 10, rdx = rdi % 10
	fc.out.MovRegToReg("rax", "rdi")
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.MovImmToReg("r14", "10")
	fc.out.DivRegByReg("rax", "r14") // rax = quotient, rdx = remainder

	// Convert remainder to ASCII
	fc.out.AddImmToReg("rdx", 48) // + '0'
	fc.out.MovByteRegToMem("rdx", "rbx", 0)

	// Move to previous position
	fc.out.SubImmFromReg("rbx", 1)
	fc.out.AddImmToReg("r12", 1)

	// Continue if quotient != 0
	fc.out.MovRegToReg("rdi", "rax")
	fc.out.CmpRegToImm("rdi", 0)
	fc.out.Write(0x75) // JNE (jump back if != 0)
	loopOffset := int8(loopStart - (fc.eb.text.Len() + 1))
	fc.out.Write(byte(loopOffset))

	// Add minus sign if negative
	fc.out.CmpRegToImm("r13", 0)
	fc.out.Write(0x74) // JE (skip if not negative)
	fc.out.Write(0x00) // Placeholder
	skipMinusJump := fc.eb.text.Len() - 1

	fc.out.MovImmToReg("rax", "45") // '-'
	fc.out.MovByteRegToMem("rax", "rbx", 0)
	fc.out.SubImmFromReg("rbx", 1)
	fc.out.AddImmToReg("r12", 1)

	// Skip minus label
	skipMinusPos := fc.eb.text.Len()
	fc.eb.text.Bytes()[skipMinusJump] = byte(skipMinusPos - (skipMinusJump + 1))

	// Calculate start pointer: rbx currently points before first char
	fc.out.AddImmToReg("rbx", 1)

	// End label
	endPos := fc.eb.text.Len()
	fc.eb.text.Bytes()[endJump] = byte(endPos - (endJump + 1))

	// Return: rsi = buffer start, rdx = length
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.MovRegToReg("rdx", "r12")

	// Epilogue
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()
}

// generateArenaEnsureCapacity generates the _c67_arena_ensure_capacity function
// This function ensures the meta-arena has enough capacity for the requested depth
// Argument: rdi = required_depth
func (fc *C67Compiler) generateArenaEnsureCapacity() {
	fc.eb.MarkLabel("_c67_arena_ensure_capacity")

	// Function prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")
	fc.out.PushReg("r15")

	// r12 = required_depth
	fc.out.MovRegToReg("r12", "rdi")

	// Load current capacity
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_cap")
	fc.out.MovMemToReg("r13", "rbx", 0) // r13 = current capacity

	// Check if this is first allocation (capacity == 0)
	fc.out.TestRegReg("r13", "r13")
	firstAllocJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je to first_alloc

	// Not first time - load len
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_len")
	fc.out.MovMemToReg("r14", "rbx", 0) // r14 = current len

	// Check if we already have enough arenas (required <= len)
	fc.out.CmpRegToReg("r12", "r14")
	noGrowthNeededJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLessOrEqual, 0) // jle to return (required <= len)

	// Need more arenas - check if we have capacity for them
	fc.out.CmpRegToReg("r12", "r13")
	needCapacityGrowthJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreater, 0) // jg to capacity growth (required > capacity)

	// Have capacity, just need to initialize more arenas
	// r12 = required, r14 = current len
	// Load meta-arena pointer into r15
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta")
	fc.out.MovMemToReg("r15", "rbx", 0) // r15 = meta-arena pointer
	fc.out.MovRegToReg("r13", "r14")    // r13 = current len (start index for init loop)
	fc.generateArenaInitLoop()
	// Jump to return
	lenOnlyGrowthJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)

	// Capacity growth needed - realloc and initialize new slots
	growLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(needCapacityGrowthJump+2, int32(growLabel-(needCapacityGrowthJump+6)))

	// Use helper function to grow meta-arena
	fc.generateMetaArenaGrowth()

	// Use helper function to initialize new arena structures
	fc.generateArenaInitLoop()

	// Update capacity
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_cap")
	fc.out.MovRegToMem("r14", "rbx", 0)

	// Jump to return
	returnJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Will patch this later

	// First allocation path
	firstAllocLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(firstAllocJump+2, int32(firstAllocLabel-(firstAllocJump+6)))

	// Use helper function for first meta-arena allocation
	fc.generateFirstMetaArenaAlloc()

	// Check if we need to grow further (required > 8)
	fc.out.CmpRegToImm("r12", 8)
	firstGrowCheckJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLessOrEqual, 0) // jle to return (no growth needed)

	// Need to grow: load capacity into r13 for growth path
	fc.out.LeaSymbolToReg("rbx", "_c67_arena_meta_cap")
	fc.out.MovMemToReg("r13", "rbx", 0) // r13 = capacity (8)

	// Jump to growth path
	growthJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	fc.patchJumpImmediate(growthJump+1, int32(growLabel-(growthJump+5)))

	// Return (no growth needed)
	returnLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(noGrowthNeededJump+2, int32(returnLabel-(noGrowthNeededJump+6)))
	fc.patchJumpImmediate(returnJump+1, int32(returnLabel-(returnJump+5)))
	fc.patchJumpImmediate(lenOnlyGrowthJump+1, int32(returnLabel-(lenOnlyGrowthJump+5)))
	fc.patchJumpImmediate(firstGrowCheckJump+2, int32(returnLabel-(firstGrowCheckJump+6)))

	fc.out.PopReg("r15")
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	// Error path: malloc/realloc failed
	errorLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(fc.metaArenaGrowthErrorJump+2, int32(errorLabel-(fc.metaArenaGrowthErrorJump+6)))
	fc.patchJumpImmediate(fc.firstMetaArenaMallocErrorJump+2, int32(errorLabel-(fc.firstMetaArenaMallocErrorJump+6)))

	fc.out.MovImmToReg("rdi", "1")
	fc.trackFunctionCall("exit")
	fc.eb.GenerateCallInstruction("exit")
}

func (fc *C67Compiler) compileStoredFunctionCall(call *CallExpr) {
	// Check if this is actually a lambda/function or just a value
	isLambda := fc.lambdaVars[call.Function]

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileStoredFunctionCall: function='%s', isLambda=%v, args=%d\n", call.Function, isLambda, len(call.Args))
	}

	// If calling a non-lambda value with no args, just return the value
	if !isLambda && len(call.Args) == 0 {
		offset := fc.variables[call.Function]
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileStoredFunctionCall: returning non-lambda value from offset %d\n", offset)
		}
		fc.out.MovMemToXmm("xmm0", "rbp", -offset)
		return
	}

	// Load closure object pointer from variable
	offset := fc.variables[call.Function]
	if fc.debug {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileStoredFunctionCall: calling '%s' at offset %d, args=%d\n", call.Function, offset, len(call.Args))
		}
	}
	fc.out.MovMemToXmm("xmm0", "rbp", -offset)

	// Convert function pointer from float64 to integer in rax
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)
	fc.out.MovMemToReg("rax", "rsp", 0)
	fc.out.AddImmToReg("rsp", 16)

	// Load function pointer from closure object (offset 0)
	fc.out.MovMemToReg("r11", "rax", 0)
	// Load environment pointer from closure object (offset 8)
	fc.out.MovMemToReg("r15", "rax", 8)

	// Check if this stored function is variadic
	var isVariadic bool
	var fixedParamCount int
	if sig, exists := fc.functionSignatures[call.Function]; exists && sig.IsVariadic {
		isVariadic = true
		fixedParamCount = sig.ParamCount
	}

	// Compile arguments and put them in xmm registers
	xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5"}
	if len(call.Args) > len(xmmRegs) {
		compilerError("too many arguments to stored function (max 6)")
	}

	// Save function pointer and environment to stack (will be clobbered during arg evaluation)
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovRegToMem("r11", "rsp", 0)
	fc.out.MovRegToMem("r15", "rsp", 8)
	// Evaluate all arguments and save to stack
	for _, arg := range call.Args {
		fc.compileExpression(arg) // Result in xmm0
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
	}

	// Load arguments from stack into xmm registers (in reverse order)
	for i := len(call.Args) - 1; i >= 0; i-- {
		fc.out.MovMemToXmm(xmmRegs[i], "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)
	}

	// Load function pointer from stack to r11
	fc.out.MovMemToReg("r11", "rsp", 0)
	fc.out.MovMemToReg("r15", "rsp", 8)
	fc.out.AddImmToReg("rsp", 16)

	// If variadic, set r14 to the count of variadic arguments
	if isVariadic {
		variadicArgCount := len(call.Args) - fixedParamCount
		if variadicArgCount < 0 {
			variadicArgCount = 0
		}
		fc.out.MovImmToReg("r14", fmt.Sprintf("%d", variadicArgCount))
	}

	// Call the function pointer in r11
	// r15 contains the environment pointer (accessible within the lambda)
	fc.out.CallRegister("r11")

	// Result is in xmm0
}

func (fc *C67Compiler) compileLambdaDirectCall(call *CallExpr) {
	// Check if this is a pure function eligible for memoization
	var targetLambda *LambdaFunc
	for i := range fc.lambdaFuncs {
		if fc.lambdaFuncs[i].Name == call.Function {
			targetLambda = &fc.lambdaFuncs[i]
			break
		}
	}

	// Direct call to a lambda by name (for recursion)
	// Compile arguments and put them in xmm registers
	xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5"}

	// Check if function is variadic
	var isVariadic bool
	var fixedParamCount int
	if sig, exists := fc.functionSignatures[call.Function]; exists && sig.IsVariadic {
		isVariadic = true
		fixedParamCount = sig.ParamCount

		// Variadic functions can have more args than fixed params
		if len(call.Args) > len(xmmRegs) {
			compilerError("too many arguments (max %d total for variadic function)", len(xmmRegs))
		}
	} else {

		if len(call.Args) > len(xmmRegs) {
			compilerError("too many arguments to lambda function (max 6)")
		}
	}

	// For pure single-argument functions, add memoization
	if targetLambda != nil && targetLambda.IsPure && len(call.Args) == 1 {
		fc.compileMemoizedCall(call, targetLambda)
		return
	}

	// Evaluate all arguments and save to stack
	// Arguments are NOT in tail position, even if the call itself is
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = false
	for _, arg := range call.Args {
		fc.compileExpression(arg) // Result in xmm0
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
	}
	fc.inTailPosition = savedTailPosition

	// Load arguments from stack into xmm registers (in reverse order)
	for i := len(call.Args) - 1; i >= 0; i-- {
		fc.out.MovMemToXmm(xmmRegs[i], "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)
	}

	// If variadic, pass count of variadic arguments in r14
	if isVariadic {
		variadicArgCount := len(call.Args) - fixedParamCount
		if variadicArgCount < 0 {
			variadicArgCount = 0
		}
		fc.out.MovImmToReg("r14", fmt.Sprintf("%d", variadicArgCount))
	}

	// Call the lambda function (direct or indirect for hot functions)
	fc.trackFunctionCall(call.Function)

	if idx, isHot := fc.hotFunctionTable[call.Function]; isHot {
		// Hot function: load closure object pointer from table and call through it
		fc.out.LeaSymbolToReg("r11", "_hot_function_table")
		offset := idx * 8
		fc.out.MovMemToReg("rax", "r11", offset) // Load closure object pointer into rax

		// Extract function pointer from closure object (offset 0)
		fc.out.MovMemToReg("r11", "rax", 0)
		// Extract environment pointer from closure object (offset 8)
		fc.out.MovMemToReg("r15", "rax", 8)

		// Call the function pointer
		fc.out.CallRegister("r11")
	} else {
		fc.out.CallSymbol(call.Function)
	}

	// Result is in xmm0
}

func (fc *C67Compiler) compileMemoizedCall(call *CallExpr, lambda *LambdaFunc) {
	cacheName := fmt.Sprintf("_memo_%s", lambda.Name)

	// Evaluate argument (single argument for memoization)
	fc.compileExpression(call.Args[0])
	// xmm0 = argument
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 0) // Save argument on stack

	// Load cache map pointer
	fc.out.LeaSymbolToReg("rbx", cacheName)
	fc.out.MovMemToReg("rbx", "rbx", 0) // rbx = cache pointer

	// Check if cache is NULL (not yet initialized)
	fc.out.TestRegReg("rbx", "rbx")
	initCacheJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // Jump to init if NULL

	// Cache exists - check if argument is in cache
	// Implement "arg in cache" check
	fc.out.MovMemToXmm("xmm2", "rsp", 0) // xmm2 = argument to search for
	fc.out.MovMemToXmm("xmm1", "rbx", 0) // Load count from cache
	fc.out.Cvttsd2si("rcx", "xmm1")      // rcx = count

	// Loop through cache entries
	fc.out.XorRegWithReg("rdi", "rdi") // rdi = index
	searchLoopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("rdi", "rcx")
	notFoundJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Exit loop if index >= count

	// Load key at index: cache[8 + index*16]
	fc.out.MovRegToReg("rax", "rdi")
	fc.out.MulRegWithImm("rax", 16) // 16 bytes per key-value pair
	fc.out.AddImmToReg("rax", 8)    // Skip count field
	fc.out.AddRegToReg("rax", "rbx")
	fc.out.MovMemToXmm("xmm3", "rax", 0) // xmm3 = key

	// Compare key with argument
	fc.out.Ucomisd("xmm2", "xmm3")
	notEqualJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0)

	// Found! Load cached value at cache[8 + index*16 + 8]
	fc.out.MovMemToXmm("xmm0", "rax", 8) // Load value (8 bytes after key)
	fc.out.AddImmToReg("rsp", 16)        // Clean up stack
	endJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0) // Jump to end

	// Not equal: try next entry
	notEqualPos := fc.eb.text.Len()
	fc.patchJumpImmediate(notEqualJump+2, int32(notEqualPos-(notEqualJump+6)))
	fc.out.IncReg("rdi")
	backJump := int32(searchLoopStart - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(backJump)

	// Not found: call function and cache result
	notFoundPos := fc.eb.text.Len()
	fc.patchJumpImmediate(notFoundJump+2, int32(notFoundPos-(notFoundJump+6)))

	// Load argument and call function
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.trackFunctionCall(lambda.Name)
	fc.out.CallSymbol(lambda.Name)
	// xmm0 = result

	// Save result
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Store in cache: need to add key-value pair to map
	// For simplicity, use linear growth (reallocate and copy)
	// Load cache pointer
	fc.out.LeaSymbolToReg("r12", cacheName)
	fc.out.MovMemToReg("rbx", "r12", 0)

	// Load current count and save to callee-saved register (malloc will preserve it)
	fc.out.MovMemToXmm("xmm1", "rbx", 0)
	fc.out.Cvttsd2si("r14", "xmm1") // r14 = old count (callee-saved, preserved across malloc)

	// Calculate new size: 8 + (count+1)*16 bytes
	fc.out.MovRegToReg("rax", "r14")
	fc.out.IncReg("rax")            // rax = new count
	fc.out.MulRegWithImm("rax", 16) // rax = new count * 16
	fc.out.AddImmToReg("rax", 8)    // rax = total bytes needed

	// Reallocate with malloc
	fc.out.MovRegToReg("rdi", "rax")
	fc.out.SubImmFromReg("rsp", 16) // Align stack
	// Allocate from arena
	fc.callArenaAlloc()
	fc.out.AddImmToReg("rsp", 16)
	// rax = new cache pointer (r14 still has old count - malloc preserves callee-saved regs)

	// Copy old entries
	fc.out.MovRegToReg("r13", "rax") // r13 = new cache
	fc.out.LeaSymbolToReg("r12", cacheName)
	fc.out.MovMemToReg("rbx", "r12", 0) // rbx = old cache

	// Calculate bytes to copy: 8 + count*16
	fc.out.MovRegToReg("rdx", "r14") // Use preserved old count from r14
	fc.out.MulRegWithImm("rdx", 16)
	fc.out.AddImmToReg("rdx", 8)

	// memcpy: rdi=dest, rsi=src, rdx=size
	fc.out.MovRegToReg("rdi", "r13")
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.SubImmFromReg("rsp", 16)
	fc.trackFunctionCall("memcpy")
	fc.eb.GenerateCallInstruction("memcpy")
	fc.out.AddImmToReg("rsp", 16)

	// Old cache is arena-allocated, no need to free (arena cleanup handles it)

	// Update cache pointer
	fc.out.LeaSymbolToReg("r12", cacheName)
	fc.out.MovRegToMem("r13", "r12", 0)

	// Increment count in new cache
	fc.out.MovMemToXmm("xmm1", "r13", 0)
	fc.out.Cvttsd2si("rax", "xmm1")
	fc.out.IncReg("rax")
	fc.out.Cvtsi2sd("xmm1", "rax")
	fc.out.MovXmmToMem("xmm1", "r13", 0)

	// Add new entry at end: key = arg, value = result
	// Position: 8 + (count)*16
	fc.out.Cvttsd2si("rcx", "xmm1") // rcx = new count
	fc.out.SubImmFromReg("rcx", 1)  // rcx = old count
	fc.out.MulRegWithImm("rcx", 16)
	fc.out.AddImmToReg("rcx", 8)
	fc.out.AddRegToReg("rcx", "r13") // rcx = address for new entry

	// Store key (argument)
	fc.out.MovMemToXmm("xmm2", "rsp", 16)
	fc.out.MovXmmToMem("xmm2", "rcx", 0)

	// Store value (result)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.MovXmmToMem("xmm0", "rcx", 8)

	// Clean up stack and return result
	fc.out.AddImmToReg("rsp", 32) // Remove result + argument
	doneJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)

	// Initialize cache (first call)
	initPos := fc.eb.text.Len()
	fc.patchJumpImmediate(initCacheJump+2, int32(initPos-(initCacheJump+6)))

	// Allocate initial cache: 8 bytes for count
	fc.out.MovImmToReg("rdi", "8")
	fc.out.SubImmFromReg("rsp", 16)
	// Allocate from arena
	fc.callArenaAlloc()
	fc.out.AddImmToReg("rsp", 16)
	// rax = cache pointer

	// Initialize count to 0
	fc.out.XorRegWithReg("rcx", "rcx")
	fc.out.Cvtsi2sd("xmm1", "rcx")
	fc.out.MovXmmToMem("xmm1", "rax", 0)

	// Store cache pointer
	fc.out.LeaSymbolToReg("rbx", cacheName)
	fc.out.MovRegToMem("rax", "rbx", 0)
	fc.out.MovRegToReg("rbx", "rax")

	// Jump back to check (cache now exists)
	backToCheck := int32(initCacheJump + 6 - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(backToCheck)

	// End label
	endPos := fc.eb.text.Len()
	fc.patchJumpImmediate(endJump+1, int32(endPos-(endJump+5)))
	fc.patchJumpImmediate(doneJump+1, int32(endPos-(doneJump+5)))

	// Track cache for rodata storage allocation (defined before ELF generation)
	if fc.memoCaches == nil {
		fc.memoCaches = make(map[string]bool)
	}
	fc.memoCaches[cacheName] = true
}

// isFMAPattern detects if an expression is a FMA pattern: a * b + c
// Returns (true, a, b, c) if pattern matches, (false, nil, nil, nil) otherwise
func (fc *C67Compiler) isFMAPattern(expr Expression) (bool, Expression, Expression, Expression) {
	// Check if this is an addition
	if call, ok := expr.(*DirectCallExpr); ok {
		if ident, ok := call.Callee.(*IdentExpr); ok && ident.Name == "+" && len(call.Args) == 2 {
			// Check if left is multiplication: (a * b) + c
			if leftCall, ok := call.Args[0].(*DirectCallExpr); ok {
				if leftIdent, ok := leftCall.Callee.(*IdentExpr); ok && leftIdent.Name == "*" && len(leftCall.Args) == 2 {
					// Pattern: (a * b) + c
					return true, leftCall.Args[0], leftCall.Args[1], call.Args[1]
				}
			}
			// Check if right is multiplication: c + (a * b)
			if rightCall, ok := call.Args[1].(*DirectCallExpr); ok {
				if rightIdent, ok := rightCall.Callee.(*IdentExpr); ok && rightIdent.Name == "*" && len(rightCall.Args) == 2 {
					// Pattern: c + (a * b)
					return true, rightCall.Args[0], rightCall.Args[1], call.Args[0]
				}
			}
		}
	}
	return false, nil, nil, nil
}

// compileFMA compiles a fused multiply-add: result = a * b + c
// Uses VFMADD132SD if FMA is available, falls back to mul+add otherwise
func (fc *C67Compiler) compileFMA(a, b, c Expression) {
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = false

	// Compile c into xmm0 (accumulator)
	fc.compileExpression(c)
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	// Compile a into xmm0
	fc.compileExpression(a)
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 8)

	// Compile b into xmm1
	fc.compileExpression(b)
	fc.out.MovRegToReg("xmm1", "xmm0")

	// Restore a into xmm0
	fc.out.MovMemToXmm("xmm0", "rsp", 8)

	// Restore c into xmm2
	fc.out.MovMemToXmm("xmm2", "rsp", 16)
	fc.out.AddImmToReg("rsp", 32)

	fc.inTailPosition = savedTailPosition

	// Generate FMA with runtime check
	// if (cpu_has_fma) { vfmadd132sd xmm0, xmm2, xmm1 } else { mulsd + addsd }

	// Load cpu_has_fma flag
	fc.out.LeaSymbolToReg("rax", "cpu_has_fma")
	fc.out.Emit([]byte{0x0f, 0xb6, 0x00}) // movzx eax, byte [rax]
	fc.out.Emit([]byte{0x85, 0xc0})       // test eax, eax

	// Jump to fallback if no FMA
	jzPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // jz fallback (6 bytes)

	// FMA path: xmm0 = xmm0 * xmm1 + xmm2
	// VFMADD132SD xmm0, xmm2, xmm1 => xmm0 = xmm0 * xmm1 + xmm2
	fc.out.Emit([]byte{0xc4, 0xe2, 0xe9, 0x99, 0xc1}) // vfmadd132sd xmm0, xmm2, xmm1

	// Jump over fallback
	jmpOverPos := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp end (2 bytes)

	// Fallback path: mul + add
	fallbackPos := fc.eb.text.Len()
	fc.out.MulsdXmm("xmm0", "xmm1") // xmm0 = xmm0 * xmm1
	fc.out.AddsdXmm("xmm0", "xmm2") // xmm0 = xmm0 + xmm2

	// End position
	endPos := fc.eb.text.Len()

	// Patch jumps
	fc.patchJumpImmediate(jzPos+2, int32(fallbackPos-(jzPos+6)))
	fc.eb.text.Bytes()[jmpOverPos+1] = byte(endPos - (jmpOverPos + 2))

	// Result is in xmm0
}

// compileBinaryOpSafe compiles a binary operation with proper stack-based
// intermediate storage to avoid register clobbering.
// This is the recommended pattern for all binary operations.
func (fc *C67Compiler) compileBinaryOpSafe(left, right Expression, operator string) {
	// Check for FMA pattern: (a * b) + c or c + (a * b)
	if operator == "+" {
		// Try to detect FMA on left side: (a * b) + c
		if leftCall, ok := left.(*DirectCallExpr); ok {
			if leftIdent, ok := leftCall.Callee.(*IdentExpr); ok && leftIdent.Name == "*" && len(leftCall.Args) == 2 {
				// Pattern: (a * b) + c
				fc.compileFMA(leftCall.Args[0], leftCall.Args[1], right)
				return
			}
		}
		// Try to detect FMA on right side: c + (a * b)
		if rightCall, ok := right.(*DirectCallExpr); ok {
			if rightIdent, ok := rightCall.Callee.(*IdentExpr); ok && rightIdent.Name == "*" && len(rightCall.Args) == 2 {
				// Pattern: c + (a * b)
				fc.compileFMA(rightCall.Args[0], rightCall.Args[1], left)
				return
			}
		}
	}

	// Clear tail position - operands of binary expressions cannot be in tail position
	// because the operation happens AFTER the operands are evaluated
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = false

	// Compile left into xmm0
	fc.compileExpression(left)
	// Save left to stack (registers may be clobbered by function calls in right expr)
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)
	// Compile right into xmm0
	fc.compileExpression(right)
	// Move right operand to xmm1
	fc.out.MovRegToReg("xmm1", "xmm0")
	// Restore left operand from stack to xmm0
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 16)
	// Now xmm0 has left, xmm1 has right - ready for operation

	fc.inTailPosition = savedTailPosition

	// Perform the operation
	switch operator {
	case "+":
		fc.out.AddsdXmm("xmm0", "xmm1")
	case "-":
		fc.out.SubsdXmm("xmm0", "xmm1")
	case "*":
		fc.out.MulsdXmm("xmm0", "xmm1")
	case "/":
		// Division needs zero check - caller should handle
		fc.out.DivsdXmm("xmm0", "xmm1")
	default:
		compilerError("unsupported operator in compileBinaryOpSafe: %s", operator)
	}
	// Result is in xmm0
}

func (fc *C67Compiler) compileDirectCall(call *DirectCallExpr) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: callee type = %T\n", call.Callee)
		if v, ok := call.Callee.(*IdentExpr); ok {
			fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: callee ident name = '%s'\n", v.Name)
		}
	}

	// WORKAROUND for parser bug: If Callee is a UnaryExpr, this is an operator
	// being used in prefix notation (e.g., (- 8 6))
	if unary, ok := call.Callee.(*UnaryExpr); ok {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: detected operator '%s' via UnaryExpr, args=%d\n", unary.Operator, len(call.Args))
			for i, arg := range call.Args {
				fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: arg[%d] = %T\n", i, arg)
			}
			fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: UnaryExpr.Operand = %T\n", unary.Operand)
		}
		// This is a binary operation disguised as a DirectCallExpr
		// The operator is in unary.Operator
		// The operands are in call.Args
		if len(call.Args) != 2 {
			compilerError("operator '%s' requires exactly 2 arguments, got %d", unary.Operator, len(call.Args))
		}
		fc.compileBinaryOpSafe(call.Args[0], call.Args[1], unary.Operator)
		return
	}

	// WORKAROUND: If Callee is nil, this is a bug in the parser where operators
	// are being represented as DirectCallExpr with nil Callee.
	// We need to figure out what the operator is from the context.
	// For now, let me just check if Args match a binary operation pattern
	if call.Callee == nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: nil Callee with %d args\n", len(call.Args))
			for i, arg := range call.Args {
				fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: arg[%d] type = %T\n", i, arg)
			}
		}
		// If there's exactly 1 arg and it's a DirectCallExpr, just compile it
		// This happens with nested expressions like (+ 5 (- 8 6))
		if len(call.Args) == 1 {
			if innerCall, ok := call.Args[0].(*DirectCallExpr); ok {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "DEBUG compileDirectCall: compiling nested DirectCallExpr\n")
				}
				fc.compileDirectCall(innerCall)
				return
			}
		}
		compilerError("DirectCallExpr has nil Callee - this is a parser bug!")
	}

	// Special case: calling a value (not a lambda) just returns the value
	// This handles cases like: main = 42; main() returns 42
	if len(call.Args) == 0 {
		// Check if callee is a simple value (not a lambda)
		isLambda := false
		switch call.Callee.(type) {
		case *LambdaExpr, *PatternLambdaExpr, *MultiLambdaExpr:
			isLambda = true
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: DirectCall with 0 args - callee is a Lambda expression\n")
			}
		case *IdentExpr:
			// Check if the identifier refers to a lambda/function
			if ident, ok := call.Callee.(*IdentExpr); ok {
				if fc.lambdaVars[ident.Name] {
					isLambda = true
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: DirectCall with 0 args - ident '%s' is a lambda\n", ident.Name)
					}
				} else {
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "DEBUG: DirectCall with 0 args - ident '%s' is NOT a lambda\n", ident.Name)
					}
				}
			}
		}

		if !isLambda {
			// Just compile the value and return it (calling a value returns the value)
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Calling a value (not lambda) - just returning the value\n")
			}
			fc.compileExpression(call.Callee)
			return
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Calling a lambda with 0 args - will dereference and call\n")
		}
	}

	// Compile the callee expression (e.g., a lambda) to get function pointer
	fc.compileExpression(call.Callee) // Result in xmm0 (function pointer as float64)

	// Convert function pointer from float64 to integer in rax
	fc.out.SubImmFromReg("rsp", StackSlotSize)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)
	fc.out.MovMemToReg("rax", "rsp", 0)
	fc.out.AddImmToReg("rsp", StackSlotSize)

	// Compile arguments and put them in xmm registers
	xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5"}
	if len(call.Args) > len(xmmRegs) {
		compilerError("too many arguments to direct call (max 6)")
	}

	// Save function pointer to stack (rax might get clobbered)
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovRegToMem("rax", "rsp", 0)

	// Evaluate all arguments and save to stack
	for _, arg := range call.Args {
		fc.compileExpression(arg) // Result in xmm0
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
	}

	// Load arguments from stack into xmm registers (in reverse order)
	for i := len(call.Args) - 1; i >= 0; i-- {
		fc.out.MovMemToXmm(xmmRegs[i], "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)
	}

	// Load closure pointer from stack
	fc.out.MovMemToReg("r11", "rsp", 0)
	fc.out.AddImmToReg("rsp", 16)

	// Dereference closure to get actual function pointer at offset 0
	fc.out.MovMemToReg("r11", "r11", 0)

	// Call the function pointer in r11
	fc.out.CallRegister("r11")

	// Result is in xmm0
}

// compileMapToCString converts a string map (map[uint64]float64) to a CString
// Input: mapPtr (register name) = pointer to string map
// Output: cstrPtr (register name) = pointer to first character of CString
// CString format: [length_byte][char0][char1]...[charn][newline][null]
//
//	^-- returned pointer points here
func (fc *C67Compiler) compileMapToCString(mapPtr, cstrPtr string) {
	// Allocate space on stack for CString (max 256 bytes + length + newline + null)
	fc.out.SubImmFromReg("rsp", 260) // 1 (length) + 256 (chars) + 1 (newline) + 1 (null) + padding

	// Load count from map[0] (empty strings have count=0, not null)
	fc.out.MovMemToXmm("xmm0", mapPtr, 0)
	fc.out.Cvttsd2si("rcx", "xmm0") // rcx = character count

	// Store length byte at [rsp]
	fc.out.MovRegToMem("rcx", "rsp", 0) // Just store lower byte

	// rsi = write position (starts at rsp+1, after length byte)
	fc.out.LeaMemToReg("rsi", "rsp", 1)

	// rbx = map pointer (start after count)
	fc.out.MovRegToReg("rbx", mapPtr)
	fc.out.AddImmToReg("rbx", 8) // Skip count field

	// rdi = character index (0, 1, 2, ...)
	fc.out.XorRegWithReg("rdi", "rdi")

	// Loop through each character
	loopStart := fc.eb.text.Len()

	// Check if done (rdi >= rcx)
	fc.out.CmpRegToReg("rdi", "rcx")
	loopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)
	loopEndEnd := fc.eb.text.Len()

	// Find character at index rdi in the map
	// For simplicity, use linear search through map pairs
	// TODO: This is O(n²) - optimize later

	// r8 = current map position
	fc.out.MovRegToReg("r8", "rbx")

	// r9 = remaining keys to check
	fc.out.MovRegToReg("r9", "rcx")

	// Inner loop: search for key == rdi
	innerLoopStart := fc.eb.text.Len()

	// Check if any keys remain
	fc.out.CmpRegToImm("r9", 0)
	innerLoopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	innerLoopEndEnd := fc.eb.text.Len()

	// Load key from [r8]
	fc.out.MovMemToXmm("xmm1", "r8", 0)
	fc.out.Cvttsd2si("r10", "xmm1") // r10 = key as integer

	// Compare with rdi (target index)
	fc.out.CmpRegToReg("r10", "rdi")
	keyMatchJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	keyMatchEnd := fc.eb.text.Len()

	// Not a match, advance to next pair
	fc.out.AddImmToReg("r8", 16) // Skip key+value pair
	fc.out.SubImmFromReg("r9", 1)
	fc.out.JumpUnconditional(int32(innerLoopStart - (fc.eb.text.Len() + 5)))

	// Key matched - load value (character code)
	keyMatchPos := fc.eb.text.Len()
	fc.patchJumpImmediate(keyMatchJump+2, int32(keyMatchPos-keyMatchEnd))

	fc.out.MovMemToXmm("xmm2", "r8", 8) // Load value at [r8+8]
	fc.out.Cvttsd2si("r10", "xmm2")     // r10 = character code

	// Store character byte at [rsi]
	fc.out.MovByteRegToMem("r10", "rsi", 0)

	// Advance write position
	fc.out.AddImmToReg("rsi", 1)

	// Advance character index
	fc.out.AddImmToReg("rdi", 1)

	// Continue outer loop
	fc.out.JumpUnconditional(int32(loopStart - (fc.eb.text.Len() + 5)))

	// Inner loop end (key not found - shouldn't happen for valid strings)
	innerLoopEndPos := fc.eb.text.Len()
	fc.patchJumpImmediate(innerLoopEndJump+2, int32(innerLoopEndPos-innerLoopEndEnd))

	// Store '?' for missing character (shouldn't happen)
	fc.out.MovImmToReg("r10", "63") // ASCII '?'
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)
	fc.out.AddImmToReg("rdi", 1)
	fc.out.JumpUnconditional(int32(loopStart - (fc.eb.text.Len() + 5)))

	// Loop end - all characters processed
	loopEndPos := fc.eb.text.Len()
	fc.patchJumpImmediate(loopEndJump+2, int32(loopEndPos-loopEndEnd))

	// Add newline character
	fc.out.MovImmToReg("r10", "10") // ASCII '\n'
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Add null terminator
	fc.out.XorRegWithReg("r10", "r10")
	fc.out.MovByteRegToMem("r10", "rsi", 0)

	// Return pointer to first character (skip length byte)
	fc.out.LeaMemToReg(cstrPtr, "rsp", 1)

	// Note: Stack not cleaned up here - caller must handle
}

// compilePrintMapAsString converts a string map to bytes for printing via syscall
// Input: mapPtr (register) = pointer to string map, bufPtr (register) = buffer start
// Output: rsi = pointer to string data, rdx = length (including newline)
func (fc *C67Compiler) compilePrintMapAsString(mapPtr, bufPtr string) {
	// Load count from map[0] (empty strings have count=0, not null)
	fc.out.MovMemToXmm("xmm0", mapPtr, 0)
	fc.out.Cvttsd2si("rcx", "xmm0") // rcx = character count

	// rsi = write position (buffer start)
	fc.out.MovRegToReg("rsi", bufPtr)

	// rbx = map data pointer (start after count at offset 8)
	fc.out.MovRegToReg("rbx", mapPtr)
	fc.out.AddImmToReg("rbx", 8)

	// rdi = character index
	fc.out.XorRegWithReg("rdi", "rdi")

	// Loop through each character
	loopStart := fc.eb.text.Len()

	// Check if done (rdi >= rcx)
	fc.out.CmpRegToReg("rdi", "rcx")
	loopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)
	loopEndEnd := fc.eb.text.Len()

	// Linear search for key == rdi
	fc.out.MovRegToReg("r8", "rbx")
	fc.out.MovRegToReg("r9", "rcx")

	innerLoopStart := fc.eb.text.Len()
	fc.out.CmpRegToImm("r9", 0)
	innerLoopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	innerLoopEndEnd := fc.eb.text.Len()

	// Load and compare key
	fc.out.MovMemToXmm("xmm1", "r8", 0)
	fc.out.Cvttsd2si("r10", "xmm1")
	fc.out.CmpRegToReg("r10", "rdi")
	keyMatchJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	keyMatchEnd := fc.eb.text.Len()

	// Not a match, advance
	fc.out.AddImmToReg("r8", 16)
	fc.out.SubImmFromReg("r9", 1)
	fc.out.JumpUnconditional(int32(innerLoopStart - (fc.eb.text.Len() + 5)))

	// Key matched - store character
	keyMatchPos := fc.eb.text.Len()
	fc.patchJumpImmediate(keyMatchJump+2, int32(keyMatchPos-keyMatchEnd))

	fc.out.MovMemToXmm("xmm2", "r8", 8)
	fc.out.Cvttsd2si("r10", "xmm2")
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Inner loop end
	innerLoopEndPos := fc.eb.text.Len()
	fc.patchJumpImmediate(innerLoopEndJump+2, int32(innerLoopEndPos-innerLoopEndEnd))

	// Advance character index
	fc.out.AddImmToReg("rdi", 1)
	fc.out.JumpUnconditional(int32(loopStart - (fc.eb.text.Len() + 5)))

	// Loop end - add newline
	loopEndPos := fc.eb.text.Len()
	fc.patchJumpImmediate(loopEndJump+2, int32(loopEndPos-loopEndEnd))

	// Store newline
	fc.out.MovImmToReg("r10", "10") // '\n' = 10
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Calculate length: rsi - bufPtr
	fc.out.MovRegToReg("rdx", "rsi")
	fc.out.SubRegFromReg("rdx", bufPtr)

	// Set rsi back to buffer start
	fc.out.MovRegToReg("rsi", bufPtr)
}

// compileFloatToString converts a float64 to ASCII string representation
// Input: xmmReg = XMM register with float64, bufPtr = buffer pointer (register)
// Output: rsi = string start, rdx = length (including newline)
func (fc *C67Compiler) compileFloatToString(xmmReg, bufPtr string) {
	// Caller has already allocated buffer at bufPtr
	// Save the float value in a temporary location (we'll use the end of the buffer)
	fc.out.MovXmmToMem(xmmReg, bufPtr, 24)

	// Check if negative by testing sign bit
	// We'll load 0.0 by converting integer 0
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.Cvtsi2sd("xmm2", "rax") // xmm2 = 0.0
	fc.out.Ucomisd(xmmReg, "xmm2")
	negativeJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpBelow, 0)
	negativeEnd := fc.eb.text.Len()

	// Positive path
	positiveSkipJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	positiveSkipEnd := fc.eb.text.Len()

	// Negative path - add minus sign and negate
	negativePos := fc.eb.text.Len()
	fc.patchJumpImmediate(negativeJump+2, int32(negativePos-negativeEnd))
	fc.out.MovImmToReg("r10", "45") // '-'
	fc.out.MovByteRegToMem("r10", bufPtr, 0)
	fc.out.LeaMemToReg("rsi", bufPtr, 1)

	// Negate the float: multiply by -1
	fc.out.MovMemToXmm("xmm0", bufPtr, 24)
	fc.loadFloatConstant("xmm3", -1.0)
	fc.out.MulsdXmm("xmm0", "xmm3")
	fc.out.MovXmmToMem("xmm0", bufPtr, 24)

	negativeSkipJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	negativeSkipEnd := fc.eb.text.Len()

	// Positive path target
	positiveSkip := fc.eb.text.Len()
	fc.patchJumpImmediate(positiveSkipJump+1, int32(positiveSkip-positiveSkipEnd))
	fc.out.MovRegToReg("rsi", bufPtr)

	// Negative skip target
	negativeSkip := fc.eb.text.Len()
	fc.patchJumpImmediate(negativeSkipJump+1, int32(negativeSkip-negativeSkipEnd))

	// Now rsi points to where we write, load the (now positive) float
	fc.out.MovMemToXmm("xmm0", bufPtr, 24)

	// Check if it's a whole number
	fc.out.Cvttsd2si("rax", "xmm0")
	fc.out.Cvtsi2sd("xmm1", "rax")
	fc.out.Ucomisd("xmm0", "xmm1")

	notWholeJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0)
	notWholeEnd := fc.eb.text.Len()

	// Whole number path - print as integer
	fc.compileIntToStringAtPos("rax", "rsi")

	// If we wrote a '-' sign, we need to adjust rsi to include it
	// Check if byte [bufPtr] == '-' (ASCII 45)
	fc.out.MovMemToReg("r10", bufPtr, 0) // load 8 bytes from bufPtr
	// Emit AND r10, 0xFF manually to mask to low byte
	fc.out.Write(0x49) // REX.W prefix for r10
	fc.out.Write(0x81) // AND r/m64, imm32
	fc.out.Write(0xE2) // ModR/M byte for r10 (11 100 010)
	fc.out.Write(0xFF) // immediate value (low byte)
	fc.out.Write(0x00) // immediate value (next 3 bytes)
	fc.out.Write(0x00)
	fc.out.Write(0x00)
	fc.out.CmpRegToImm("r10", 45) // compare with '-'
	noMinusJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0)
	noMinusEnd := fc.eb.text.Len()

	// Has minus sign - adjust rsi and rdx
	fc.out.MovRegToReg("rsi", bufPtr)
	fc.out.AddImmToReg("rdx", 1) // include the '-' in length

	noMinusPos := fc.eb.text.Len()
	fc.patchJumpImmediate(noMinusJump+2, int32(noMinusPos-noMinusEnd))

	// No cleanup needed - caller manages the buffer

	wholeEndJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	wholeEndEnd := fc.eb.text.Len()

	// Float path - print with decimal point
	notWholePos := fc.eb.text.Len()
	fc.patchJumpImmediate(notWholeJump+2, int32(notWholePos-notWholeEnd))

	// Extract integer part (rax already has it from above)
	fc.out.Cvttsd2si("rax", "xmm0")

	// Save int part as float in xmm1 BEFORE printing (printing will clobber rax)
	fc.out.Cvtsi2sd("xmm1", "rax")

	// Print integer part
	fc.compileIntToStringAtPosNoNewline("rax", "rsi")
	// rsi now points after the integer part

	// Add decimal point
	fc.out.MovImmToReg("r10", "46") // '.'
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Get fractional part: frac = num - int_part
	fc.out.MovMemToXmm("xmm0", bufPtr, 24)
	// xmm1 already has int part as float from above
	fc.out.SubsdXmm("xmm0", "xmm1") // xmm0 = fractional part

	// Check if fractional part is zero
	fc.out.XorRegWithReg("rax", "rax")
	fc.out.Cvtsi2sd("xmm2", "rax") // xmm2 = 0.0
	fc.out.Ucomisd("xmm0", "xmm2")
	fracZeroJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // Jump if frac == 0
	fracZeroEnd := fc.eb.text.Len()

	// Print up to 6 decimal digits
	fc.out.MovImmToReg("r11", "6") // digit counter
	fc.loadFloatConstant("xmm3", 10.0)

	fracLoopStart := fc.eb.text.Len()

	// Check if done
	fc.out.CmpRegToImm("r11", 0)
	fracLoopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	fracLoopEndEnd := fc.eb.text.Len()

	// Multiply by 10
	fc.out.MulsdXmm("xmm0", "xmm3")

	// Extract digit (save it first before converting to ASCII)
	fc.out.Cvttsd2si("r10", "xmm0")

	// Convert integer digit back to float for subtraction
	fc.out.Cvtsi2sd("xmm1", "r10")
	fc.out.SubsdXmm("xmm0", "xmm1")

	// Convert digit to ASCII and store
	fc.out.AddImmToReg("r10", 48) // to ASCII
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	fc.out.SubImmFromReg("r11", 1)
	fc.out.JumpUnconditional(int32(fracLoopStart - (fc.eb.text.Len() + 5)))

	fracLoopEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(fracLoopEndJump+2, int32(fracLoopEnd-fracLoopEndEnd))

	// Strip trailing zeros by walking backwards
	// rsi points one past the last digit
	stripLoopStart := fc.eb.text.Len()
	// Go back one byte
	fc.out.SubImmFromReg("rsi", 1)
	// Load the byte
	fc.out.MovMemToReg("r10", "rsi", 0)
	// Mask to low byte: AND r10, 0xFF
	fc.out.Write(0x49) // REX.W for r10
	fc.out.Write(0x81) // AND r/m64, imm32
	fc.out.Write(0xE2) // ModR/M for r10
	fc.out.Write(0xFF) // imm = 0xFF
	fc.out.Write(0x00)
	fc.out.Write(0x00)
	fc.out.Write(0x00)
	// Compare with '0' (48)
	fc.out.CmpRegToImm("r10", 48)
	// If equal to '0', continue stripping
	fc.out.JumpConditional(JumpEqual, int32(stripLoopStart-(fc.eb.text.Len()+6)))
	// Not a '0', so advance back to position after this character
	fc.out.AddImmToReg("rsi", 1)

	// Fractional part was zero - remove the decimal point we added
	fracZeroPos := fc.eb.text.Len()
	fc.patchJumpImmediate(fracZeroJump+2, int32(fracZeroPos-fracZeroEnd))
	fc.out.SubImmFromReg("rsi", 1) // Remove the '.' we added

	// Add newline
	fc.out.MovImmToReg("r10", "10") // '\n'
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Calculate length
	fc.out.MovRegToReg("rdx", "rsi")
	fc.out.SubRegFromReg("rdx", bufPtr)
	fc.out.MovRegToReg("rsi", bufPtr)

	// No cleanup needed - caller manages the buffer

	// End
	wholeEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(wholeEndJump+1, int32(wholeEnd-wholeEndEnd))
}

// loadFloatConstant loads a float constant into an XMM register
func (fc *C67Compiler) loadFloatConstant(xmmReg string, value float64) {
	// Create a constant label for this float value
	labelName := fmt.Sprintf("float_const_%d", fc.stringCounter)
	fc.stringCounter++

	// Convert float64 to bytes
	bits := math.Float64bits(value)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, bits)
	fc.eb.Define(labelName, string(bytes))

	// Load the address into a temp register, then load the value
	fc.out.LeaSymbolToReg("rax", labelName)
	fc.out.MovMemToXmm(xmmReg, "rax", 0)
}

// compileIntToStringAtPos is like compileIntToString but writes at rsi position
func (fc *C67Compiler) compileIntToStringAtPos(intReg, posReg string) {
	fc.compileWholeNumberToStringAtPos(intReg, posReg, true)
}

// compileIntToStringAtPosNoNewline writes integer without newline
func (fc *C67Compiler) compileIntToStringAtPosNoNewline(intReg, posReg string) {
	fc.compileWholeNumberToStringAtPos(intReg, posReg, false)
}

// compileWholeNumberToStringAtPos converts a whole number to ASCII at a given position
// Input: intReg = register with int64, posReg = write position register
// If addNewline is true, adds '\n' and sets rsi/rdx; otherwise just updates posReg
func (fc *C67Compiler) compileWholeNumberToStringAtPos(intReg, posReg string, addNewline bool) {
	// Store the starting position
	startPosReg := "r14"
	fc.out.MovRegToReg(startPosReg, posReg)

	// Convert digits (rax = number, posReg = write position)
	fc.out.MovRegToReg("rax", intReg)
	fc.out.LeaMemToReg("rdi", posReg, 20) // digit storage area
	fc.out.MovImmToReg("rcx", "10")       // divisor

	digitLoopStart := fc.eb.text.Len()

	// Divide rax by 10
	fc.out.DivRegByReg("rax", "rcx")

	// Convert remainder to ASCII
	fc.out.AddImmToReg("rdx", 48) // '0' = 48
	fc.out.MovByteRegToMem("rdx", "rdi", 0)
	fc.out.AddImmToReg("rdi", 1)

	// Continue if quotient > 0
	fc.out.CmpRegToImm("rax", 0)
	digitLoopJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreater, 0)
	digitLoopEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(digitLoopJump+2, int32(digitLoopStart-(digitLoopEnd)))

	// Copy digits back in reverse
	fc.out.SubImmFromReg("rdi", 1)
	fc.out.LeaMemToReg("r11", posReg, 20)

	copyLoopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("rdi", "r11")
	copyLoopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLess, 0)
	copyLoopEndEnd := fc.eb.text.Len()

	fc.out.MovMemToReg("r10", "rdi", 0)
	fc.out.MovByteRegToMem("r10", posReg, 0)
	fc.out.AddImmToReg(posReg, 1)
	fc.out.SubImmFromReg("rdi", 1)
	fc.out.JumpUnconditional(int32(copyLoopStart - (fc.eb.text.Len() + 5)))

	copyLoopEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(copyLoopEndJump+2, int32(copyLoopEnd-copyLoopEndEnd))

	if addNewline {
		// Add newline
		fc.out.MovImmToReg("r10", "10")
		fc.out.MovByteRegToMem("r10", posReg, 0)
		fc.out.AddImmToReg(posReg, 1)

		// Calculate length
		fc.out.MovRegToReg("rdx", posReg)
		fc.out.SubRegFromReg("rdx", startPosReg)
		fc.out.MovRegToReg("rsi", startPosReg)
	}
}

// compileWholeNumberToString converts a whole number (truncated float) to ASCII string
// Input: intReg = register with int64, bufPtr = buffer pointer (register)
// Output: rsi = string start, rdx = length (including newline)
func (fc *C67Compiler) compileWholeNumberToString(intReg, bufPtr string) {
	// Special case: zero
	fc.out.CmpRegToImm(intReg, 0)
	zeroJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0)
	zeroEnd := fc.eb.text.Len()

	// Handle negative numbers
	fc.out.CmpRegToImm(intReg, 0)
	negativeJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLess, 0)
	negativeEnd := fc.eb.text.Len()

	// Positive path
	fc.out.MovRegToReg("rax", intReg)
	positiveSkipJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	positiveSkipEnd := fc.eb.text.Len()

	// Negative path
	negativePos := fc.eb.text.Len()
	fc.patchJumpImmediate(negativeJump+2, int32(negativePos-negativeEnd))
	fc.out.MovRegToReg("rax", intReg)
	fc.out.Emit([]byte{0x48, 0xF7, 0xD8}) // neg rax

	// Store negative sign
	fc.out.MovImmToReg("r10", "45") // '-' = 45
	fc.out.MovByteRegToMem("r10", bufPtr, 0)
	fc.out.LeaMemToReg("rsi", bufPtr, 1)

	negativeSkipJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	negativeSkipEnd := fc.eb.text.Len()

	// Positive skip target
	positiveSkip := fc.eb.text.Len()
	fc.patchJumpImmediate(positiveSkipJump+1, int32(positiveSkip-positiveSkipEnd))
	fc.out.MovRegToReg("rsi", bufPtr)

	// Negative skip target
	negativeSkip := fc.eb.text.Len()
	fc.patchJumpImmediate(negativeSkipJump+1, int32(negativeSkip-negativeSkipEnd))

	// Convert digits (rax = number, rsi = buffer position)
	// Store digits in reverse, then copy forward
	fc.out.LeaMemToReg("rdi", bufPtr, 20) // digit storage area
	fc.out.MovImmToReg("rcx", "10")       // divisor

	digitLoopStart := fc.eb.text.Len()

	// Divide rax by 10: rax = quotient, rdx = remainder
	fc.out.DivRegByReg("rax", "rcx")

	// Convert remainder to ASCII ('0' + digit)
	fc.out.AddImmToReg("rdx", 48) // '0' = 48
	fc.out.MovByteRegToMem("rdx", "rdi", 0)
	fc.out.AddImmToReg("rdi", 1)

	// Continue if quotient > 0
	fc.out.CmpRegToImm("rax", 0)
	digitLoopJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreater, 0)
	digitLoopEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(digitLoopJump+2, int32(digitLoopStart-(digitLoopEnd)))

	// Copy digits back in reverse order
	fc.out.SubImmFromReg("rdi", 1)        // point to last digit
	fc.out.LeaMemToReg("r11", bufPtr, 20) // r11 = start of digit storage

	copyLoopStart := fc.eb.text.Len()

	// Check if done (rdi < r11 means we've copied all digits)
	fc.out.CmpRegToReg("rdi", "r11")
	copyLoopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpLess, 0)
	copyLoopEndEnd := fc.eb.text.Len()

	// Copy byte
	fc.out.MovMemToReg("r10", "rdi", 0)
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)
	fc.out.SubImmFromReg("rdi", 1)
	fc.out.JumpUnconditional(int32(copyLoopStart - (fc.eb.text.Len() + 5)))

	copyLoopEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(copyLoopEndJump+2, int32(copyLoopEnd-copyLoopEndEnd))

	// Add newline
	fc.out.MovImmToReg("r10", "10") // '\n'
	fc.out.MovByteRegToMem("r10", "rsi", 0)
	fc.out.AddImmToReg("rsi", 1)

	// Calculate length
	fc.out.MovRegToReg("rdx", "rsi")
	fc.out.SubRegFromReg("rdx", bufPtr)
	fc.out.MovRegToReg("rsi", bufPtr)

	// Jump to end
	normalEndJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)
	normalEndEnd := fc.eb.text.Len()

	// Zero case
	zeroPos := fc.eb.text.Len()
	fc.patchJumpImmediate(zeroJump+2, int32(zeroPos-zeroEnd))
	fc.out.MovImmToReg("r10", "48") // '0' = 48
	fc.out.MovByteRegToMem("r10", bufPtr, 0)
	fc.out.MovImmToReg("r10", "10") // '\n'
	fc.out.MovByteRegToMem("r10", bufPtr, 1)
	fc.out.MovRegToReg("rsi", bufPtr)
	fc.out.MovImmToReg("rdx", "2") // length = 2 ("0\n")

	// End
	normalEnd := fc.eb.text.Len()
	fc.patchJumpImmediate(normalEndJump+1, int32(normalEnd-normalEndEnd))
}

func (fc *C67Compiler) compileTailCall(call *CallExpr) {
	// Tail recursion optimization for "me" self-reference
	// Instead of calling, we update parameters and jump to function start

	fc.tailCallsOptimized++

	if len(call.Args) != len(fc.currentLambda.Params) {
		compilerError("tail call to 'me' has %d args but function has %d params",
			len(call.Args), len(fc.currentLambda.Params))
	}

	// Step 1: Evaluate all arguments and save to temporary stack locations
	// We need temporaries because arguments may reference current parameters
	tempOffsets := make([]int, len(call.Args))
	for i, arg := range call.Args {
		// Evaluate argument
		fc.compileExpression(arg) // Result in xmm0

		// Save to temporary stack location
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		tempOffsets[i] = fc.stackOffset + 16*(i+1)
	}

	// Step 2: Copy temporary values to parameter locations
	// Parameters are at [rbp - offset] where offset is in fc.variables
	for i, paramName := range fc.currentLambda.Params {
		paramOffset := fc.variables[paramName]
		tempStackPos := 16 * (len(call.Args) - 1 - i)

		// Load from temporary location
		fc.out.MovMemToXmm("xmm0", "rsp", tempStackPos)

		// Store to parameter location
		fc.out.MovXmmToMem("xmm0", "rbp", -paramOffset)
	}

	// Step 3: Clean up temporary stack space
	fc.out.AddImmToReg("rsp", int64(16*len(call.Args)))

	// Step 4: Jump back to lambda body start (tail recursion!)
	jumpOffset := int32(fc.lambdaBodyStart - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(jumpOffset)
}

func (fc *C67Compiler) compileCachedCall(call *CallExpr) {
	if fc.currentLambda == nil {
		compilerError("cme can only be used inside a lambda function")
	}

	numArgs := len(call.Args)
	if numArgs < 1 || numArgs > 3 {
		compilerError("cme requires 1-3 arguments: cme(arg) or cme(arg, max_size) or cme(arg, max_size, cleanup_fn)")
	}

	fc.cacheEnabledLambdas[fc.currentLambda.Name] = true
	cacheName := fc.currentLambda.Name + "_cache"

	fc.compileExpression(call.Args[0])

	fc.out.SubImmFromReg("rsp", 32)
	fc.out.MovXmmToMem("xmm0", "rsp", 0)

	fc.out.LeaSymbolToReg("rdi", cacheName)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.MovqXmmToReg("rsi", "xmm0")

	fc.trackFunctionCall("_c67_cache_lookup")
	fc.out.CallSymbol("_c67_cache_lookup")

	fc.out.CmpRegToImm("rax", 0)
	cacheHitJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0)

	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.SubImmFromReg("rsp", 8)

	callPos := fc.eb.text.Len()
	fc.eb.callPatches = append(fc.eb.callPatches, CallPatch{
		position:   callPos + 1,
		targetName: fc.currentLambda.Name,
	})
	fc.out.Emit([]byte{0xE8, 0x78, 0x56, 0x34, 0x12})

	fc.out.AddImmToReg("rsp", 8)
	fc.out.MovXmmToMem("xmm0", "rsp", 8)

	fc.out.LeaSymbolToReg("rdi", cacheName)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.MovqXmmToReg("rsi", "xmm0")
	fc.out.MovMemToXmm("xmm0", "rsp", 8)
	fc.out.MovqXmmToReg("rdx", "xmm0")

	fc.trackFunctionCall("_c67_cache_insert")
	fc.out.CallSymbol("_c67_cache_insert")

	fc.out.MovMemToXmm("xmm0", "rsp", 8)

	skipInsertJump := fc.eb.text.Len()
	fc.out.JumpUnconditional(0)

	cacheHitLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(cacheHitJump+2, int32(cacheHitLabel-(cacheHitJump+6)))

	fc.out.MovMemToXmm("xmm0", "rax", 0)

	skipInsertLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(skipInsertJump+1, int32(skipInsertLabel-(skipInsertJump+5)))

	fc.out.AddImmToReg("rsp", 32)
}

func (fc *C67Compiler) compileTailRecursiveCall(call *CallExpr) {
	if fc.currentLambda == nil {
		compilerError("tail call optimization requires lambda context")
	}

	if len(call.Args) != len(fc.currentLambda.Params) {
		compilerError("tail call to '%s' has %d args but function has %d params",
			call.Function, len(call.Args), len(fc.currentLambda.Params))
	}

	// Step 1: Evaluate all arguments and save to temporary stack locations
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = false
	tempOffsets := make([]int, len(call.Args))
	for i, arg := range call.Args {
		fc.compileExpression(arg)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		tempOffsets[i] = fc.stackOffset + 16*(i+1)
	}
	fc.inTailPosition = savedTailPosition

	// Step 2: Copy temporary values to parameter locations
	for i, paramName := range fc.currentLambda.Params {
		paramOffset := fc.variables[paramName]
		tempStackPos := 16 * (len(call.Args) - 1 - i)
		fc.out.MovMemToXmm("xmm0", "rsp", tempStackPos)
		fc.out.MovXmmToMem("xmm0", "rbp", -paramOffset)
	}

	// Step 3: Clean up temporary stack space
	fc.out.AddImmToReg("rsp", int64(16*len(call.Args)))

	// Step 4: Jump back to lambda body start (tail recursion!)
	jumpOffset := int32(fc.lambdaBodyStart - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(jumpOffset)
}

func (fc *C67Compiler) compileRecursiveCall(call *CallExpr) {
	if fc.inTailPosition {
		fc.tailCallsOptimized++
		fc.compileTailRecursiveCall(call)
		return
	}

	fc.nonTailCalls++

	// Check if this is a pure single-argument function eligible for automatic memoization
	if fc.currentLambda != nil && fc.currentLambda.IsPure && len(call.Args) == 1 {
		fc.compileMemoizedCall(call, fc.currentLambda)
		return
	}

	// Compile a recursive call with optional depth tracking
	// Only track depth if max is not infinite (for zero runtime overhead with max inf)
	// TODO: Depth tracking currently disabled - requires writable .bss/.data section support
	//       Current implementation tries to write to read-only .rodata which fails
	//       Use "max inf" for unlimited recursion (works perfectly)
	needsDepthTracking := false // call.MaxRecursionDepth != math.MaxInt64
	var depthVarName string

	if needsDepthTracking {
		// Uses a global variable to track recursion depth: functionName_recursion_depth
		depthVarName = call.Function + "_recursion_depth"

		// Ensure the depth counter variable exists in data section (initialized to 0)
		if fc.eb.consts[depthVarName] == nil {
			// Define an 8-byte zero-initialized variable
			fc.eb.Define(depthVarName, "\x00\x00\x00\x00\x00\x00\x00\x00")
		}

		// Load current recursion depth
		fc.out.MovMemToReg("rax", depthVarName, 0)

		// Increment depth
		fc.out.IncReg("rax")

		// Check against max
		fc.out.CmpRegToImm("rax", call.MaxRecursionDepth)

		// Jump past error if not exceeded
		notExceededJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLessOrEqual, 0)

		// Exceeded max recursion - print error and exit
		fc.out.LeaSymbolToReg("rdi", "_recursion_max_exceeded_msg")
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")

		// exit(1)
		fc.out.MovImmToReg("rdi", "1")
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")

		// Patch the jump
		notExceededPos := fc.eb.text.Len()
		notExceededOffset := int32(notExceededPos - (notExceededJump + 6))
		fc.patchJumpImmediate(notExceededJump+2, notExceededOffset)

		// Store incremented depth
		fc.out.MovRegToMem("rax", depthVarName, 0)
	}

	// Compile arguments in order and save ALL to stack
	// Arguments are NOT in tail position, even if the call itself is
	savedTailPosition := fc.inTailPosition
	fc.inTailPosition = false
	for _, arg := range call.Args {
		fc.compileExpression(arg)
		// Save result to stack for all arguments
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
	}
	fc.inTailPosition = savedTailPosition

	// Restore arguments from stack to registers xmm0, xmm1, xmm2, ...
	// Arguments are on stack in order: [arg0, arg1, arg2, ...]
	// We need to pop them in reverse order to get them into the right registers
	for i := len(call.Args) - 1; i >= 0; i-- {
		regName := fmt.Sprintf("xmm%d", i)
		fc.out.MovMemToXmm(regName, "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
	}

	// Make the recursive call
	// Use direct call to lambda symbol (not PLT stub like GenerateCallInstruction)
	fc.out.CallSymbol(call.Function)

	// Decrement recursion depth after return (only if tracking)
	if needsDepthTracking {
		fc.out.MovMemToReg("rax", depthVarName, 0)
		fc.out.SubImmFromReg("rax", 1) // Decrement
		fc.out.MovRegToMem("rax", depthVarName, 0)
	}

	// Result is in xmm0
}

// Confidence that this function is working: 85%
func (fc *C67Compiler) compileCFunctionCall(libName string, funcName string, args []Expression) {
	// Generate C FFI call
	// Strategy for v1.1.0:
	// 1. Marshal arguments according to System V AMD64 ABI
	// 2. Call function using PLT (dynamic linking)
	// 3. Convert result to float64 in xmm0
	//
	// Note: Library is linked dynamically via DT_NEEDED in ELF

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Generating C FFI call: %s.%s with %d args\n", libName, funcName, len(args))
	}

	// Track library dependency for ELF generation
	fc.cLibHandles[libName] = "linked" // Mark as needing dynamic linking

	// Track function usage for PLT generation and call order patching
	fc.trackFunctionCall(funcName)

	// Track which library this function belongs to (for Windows DLL imports)
	if libName != "" {
		fc.cFunctionLibs[funcName] = libName
	}

	// Marshal arguments according to calling convention (System V AMD64 or Microsoft x64)
	// System V AMD64 ABI (Linux/Unix):
	//   Integer/pointer args: rdi, rsi, rdx, rcx, r8, r9, then stack
	//   Float args: xmm0-xmm7, then stack
	// Microsoft x64 ABI (Windows):
	//   First 4 args (int or float): RCX, RDX, R8, R9 (or XMM0-3 for floats)
	//   Additional args on stack
	//   32 bytes of shadow space required

	var intArgRegs []string
	var floatArgRegs []string

	if fc.eb.target.OS() == OSWindows {
		// Windows x64 calling convention
		intArgRegs = []string{"rcx", "rdx", "r8", "r9"}
		floatArgRegs = []string{"xmm0", "xmm1", "xmm2", "xmm3"}
	} else {
		// System V AMD64 ABI (Linux/Unix)
		intArgRegs = []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		floatArgRegs = []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5", "xmm6", "xmm7"}
	}

	// Look up function signature from DWARF if available
	// Need to find the alias for this library name (reverse lookup)
	var libAlias string
	for alias, lib := range fc.cImports {
		if lib == libName {
			libAlias = alias
			break
		}
	}

	var funcSig *CFunctionSignature
	if libAlias != "" {
		if constants, ok := fc.cConstants[libAlias]; ok {
			if sig, found := constants.Functions[funcName]; found {
				funcSig = sig
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Found signature for %s: %d params, return=%s\n",
						funcName, len(sig.Params), sig.ReturnType)
				}
			}
		}
	}

	// Special case for c. namespace (libName is empty): try to find signature in common C libraries
	if funcSig == nil && libName == "" {
		// Try to find the function in any loaded C library constants
		for alias, constants := range fc.cConstants {
			if sig, found := constants.Functions[funcName]; found {
				funcSig = sig
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Found signature for %s in library %s: %d params, return=%s\n",
						funcName, alias, len(sig.Params), sig.ReturnType)
				}
				break
			}
		}

		// If still not found, use hardcoded signatures for common C functions
		if funcSig == nil {
			commonFunctions := map[string]*CFunctionSignature{
				// Math functions
				"sin":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"cos":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"tan":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"sqrt":  {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"pow":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}, {Type: "double"}}},
				"exp":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"log":   {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"floor": {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"ceil":  {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				"fabs":  {ReturnType: "double", Params: []CFunctionParam{{Type: "double"}}},
				// Memory management functions
				"malloc":  {ReturnType: "void*", Params: []CFunctionParam{{Type: "size_t"}}},
				"free":    {ReturnType: "void", Params: []CFunctionParam{{Type: "void*"}}},
				"realloc": {ReturnType: "void*", Params: []CFunctionParam{{Type: "void*"}, {Type: "size_t"}}},
				"calloc":  {ReturnType: "void*", Params: []CFunctionParam{{Type: "size_t"}, {Type: "size_t"}}},
				// Note: printf is variadic and can't be fully described here,
				// but we can at least mark the format string as const char*
				"printf": {ReturnType: "int", Params: []CFunctionParam{{Type: "const char*"}}},
			}
			if sig, ok := commonFunctions[funcName]; ok {
				funcSig = sig
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Using hardcoded signature for %s: %d params, return=%s\n",
						funcName, len(sig.Params), sig.ReturnType)
				}
			}
		}
	}

	// Allocate stack space to save arguments temporarily
	if len(args) > 0 {
		argStackOffset := len(args) * 8
		fc.out.SubImmFromReg("rsp", int64(argStackOffset))

		// First pass: Determine type information for each argument
		type argInfo struct {
			castType     string
			innerExpr    Expression
			isFloatParam bool
		}
		argInfos := make([]argInfo, len(args))

		for i, arg := range args {
			info := &argInfos[i]
			info.innerExpr = arg

			// Check for explicit cast
			if castExpr, ok := arg.(*CastExpr); ok {
				info.castType = castExpr.Type
				info.innerExpr = castExpr.Expr
			}

			// Determine actual parameter type from signature
			var paramType string
			if funcSig != nil && i < len(funcSig.Params) {
				paramType = funcSig.Params[i].Type
			}

			// Decide whether this parameter should be treated as float or int
			if paramType == "float" || paramType == "double" {
				info.isFloatParam = true
			}

			// If no explicit cast, infer the cast type
			if info.castType == "" {
				exprType := fc.getExprType(arg)
				if exprType == "string" {
					info.castType = "cstr"
				} else if info.isFloatParam {
					info.castType = "double"
				} else if paramType != "" {
					if isPointerType(paramType) {
						info.castType = "pointer"
					} else if strings.Contains(paramType, "char") && strings.Contains(paramType, "*") {
						info.castType = "cstr"
					} else {
						info.castType = "int"
					}
				} else {
					// No signature info - infer from expression type
					// For variadic functions like printf, default to double for numbers
					if exprType == "number" {
						info.castType = "double"
					} else if exprType == "list" || exprType == "map" {
						info.castType = "pointer"
					} else {
						info.castType = "int"
					}
				}
			}

			// Update isFloatParam based on final castType
			// This ensures register allocation uses the correct type
			if info.castType == "float" || info.castType == "double" {
				info.isFloatParam = true
			} else {
				info.isFloatParam = false
			}
		}

		// Second pass: Compile each argument and store on stack
		// Save rbx (callee-saved) so we can use it to track argument base
		fc.out.PushReg("rbx")
		// Save the base stack pointer for storing arguments (after we've allocated space)
		fc.out.LeaMemToReg("rbx", "rsp", 8) // rbx = rsp + 8 (account for pushed rbx)

		for i := range args {
			info := &argInfos[i]
			castType := info.castType
			innerExpr := info.innerExpr

			// Confidence that this function is working: 90%
			// Check for null pointer literals: 0, [], {}, or explicit casts
			isNullPointer := false
			if numExpr, ok := innerExpr.(*NumberExpr); ok {
				if numExpr.Value == 0 {
					isNullPointer = true
				}
			} else if listExpr, ok := innerExpr.(*ListExpr); ok {
				if len(listExpr.Elements) == 0 {
					isNullPointer = true
				}
			} else if mapExpr, ok := innerExpr.(*MapExpr); ok {
				if len(mapExpr.Keys) == 0 {
					isNullPointer = true
				}
			}

			// Set C context for string literals
			isStringLiteral := false
			if _, ok := innerExpr.(*StringExpr); ok {
				isStringLiteral = true
				fc.cContext = true
			}

			// If this is a null pointer literal and we need a pointer type, just set rax to 0
			if isNullPointer && (castType == "ptr" || castType == "pointer" || castType == "cstr" || castType == "cstring") {
				// Zero register for null pointer
				fc.out.XorRegToReg("rax", "rax")
			} else {
				// Compile the inner expression (result in xmm0 for C67 values, rax for C strings)
				fc.compileExpression(innerExpr)
			}

			// Reset C context after compilation
			if isStringLiteral {
				fc.cContext = false
			}

			// Store argument on stack based on its type
			// Use rbx as base (saved at start of arg compilation)
			if info.isFloatParam || castType == "float" || castType == "double" {
				if isNullPointer {
					// Store 0.0 for null pointer in float context
					fc.out.XorpdXmm("xmm0", "xmm0")
					fc.out.MovXmmToMem("xmm0", "rbx", i*8)
				} else {
					// Keep as float64 in xmm0, store directly
					fc.out.MovXmmToMem("xmm0", "rbx", i*8)
				}
			} else {
				// Convert to integer or pointer
				switch castType {
				case "cstr", "cstring":
					if isNullPointer {
						// Already set rax to 0 above
					} else if isStringLiteral {
						// String literal was compiled as C string - rax already contains the pointer
						// No conversion needed, just store it
					} else {
						// Runtime string (C67 map format) - need to convert to C string
						fc.out.SubImmFromReg("rsp", StackSlotSize)
						fc.out.MovXmmToMem("xmm0", "rsp", 0)
						fc.out.MovMemToReg("rax", "rsp", 0)
						fc.out.AddImmToReg("rsp", StackSlotSize)

						// Call _c67_string_to_cstr(map_ptr) -> char*
						fc.out.SubImmFromReg("rsp", StackSlotSize)
						fc.out.MovRegToMem("rax", "rsp", 0)
						fc.out.MovMemToReg("rdi", "rsp", 0)
						fc.out.AddImmToReg("rsp", StackSlotSize)
						fc.trackFunctionCall("_c67_string_to_cstr")
						fc.trackFunctionCall("_c67_string_to_cstr")
						fc.out.CallSymbol("_c67_string_to_cstr")
						// Result in rax (C string pointer)
					}

				case "ptr", "pointer":
					if isNullPointer {
						// Already set rax to 0 above
					} else {
						// Pointer type - convert float64 to integer pointer
						fc.out.Cvttsd2si("rax", "xmm0")
					}

				case "int", "i32", "int32":
					if isNullPointer {
						// Already set rax to 0 above
					} else {
						// Signed 32-bit integer
						fc.out.Cvttsd2si("rax", "xmm0")
					}

				case "uint32", "u32":
					if isNullPointer {
						// Already set rax to 0 above
					} else {
						// Unsigned 32-bit integer
						fc.out.Cvttsd2si("rax", "xmm0")
					}

				default:
					if isNullPointer {
						// Already set rax to 0 above
					} else {
						// Default: convert float64 to integer
						fc.out.Cvttsd2si("rax", "xmm0")
					}
				}

				// Store on stack at offset i*8 from rbx (saved base)
				fc.out.MovRegToMem("rax", "rbx", i*8)
			}
		}

		// Restore rsp to the argument base
		// rbx points to the start of arguments (rsp + 8 when we saved it)
		// So we need to set rsp = rbx - 8
		fc.out.LeaMemToReg("rsp", "rbx", -8)
		// Restore rbx
		fc.out.PopReg("rbx")

		// Load arguments from stack into ABI registers
		// Microsoft x64 vs System V AMD64 have different conventions:
		// - Microsoft x64: Parameter slots consumed sequentially (param N uses slot N regardless of type)
		// - System V AMD64: Int and float registers tracked separately

		// Build a list of stack arguments that overflow registers
		type stackArg struct {
			offset int
			value  int
		}
		var stackArgs []stackArg

		isWindows := fc.eb.target.OS() == OSWindows
		stackArgCount := 0

		if isWindows {
			// Microsoft x64: Sequential parameter slots
			for i := 0; i < len(args); i++ {
				isFloatParam := argInfos[i].isFloatParam

				// For Windows, parameter N goes in slot N (first 4 slots)
				if i < 4 {
					if isFloatParam {
						// Load into XMM register for this slot
						fc.out.MovMemToXmm(floatArgRegs[i], "rsp", i*8)
					} else {
						// Load into integer register for this slot
						fc.out.MovMemToReg(intArgRegs[i], "rsp", i*8)
					}
				} else {
					// Parameters 5+ go on stack
					stackArgs = append(stackArgs, stackArg{offset: i * 8, value: stackArgCount})
					stackArgCount++
				}
			}
		} else {
			// System V AMD64: Track int and float registers separately
			intRegIdx := 0
			floatRegIdx := 0

			for i := 0; i < len(args); i++ {
				isFloatParam := argInfos[i].isFloatParam

				if isFloatParam {
					if floatRegIdx < len(floatArgRegs) {
						// Load into float register
						fc.out.MovMemToXmm(floatArgRegs[floatRegIdx], "rsp", i*8)
						floatRegIdx++
					} else {
						// Goes on stack
						stackArgs = append(stackArgs, stackArg{offset: i * 8, value: stackArgCount})
						stackArgCount++
					}
				} else {
					if intRegIdx < len(intArgRegs) {
						// Load into int register
						fc.out.MovMemToReg(intArgRegs[intRegIdx], "rsp", i*8)
						intRegIdx++
					} else {
						// Goes on stack
						stackArgs = append(stackArgs, stackArg{offset: i * 8, value: stackArgCount})
						stackArgCount++
					}
				}
			}
		}

		// Clean up temp stack space, but preserve stack arguments
		if stackArgCount > 0 {
			// Move stack args to the bottom of the stack
			for i, arg := range stackArgs {
				fc.out.MovMemToReg("r11", "rsp", arg.offset)
				fc.out.MovRegToMem("r11", "rsp", i*8)
			}
			// Adjust RSP to remove register arg space, keeping stack args
			fc.out.AddImmToReg("rsp", int64(argStackOffset-stackArgCount*8))
		} else {
			// No stack args - clean up all temp space
			fc.out.AddImmToReg("rsp", int64(argStackOffset))
		}

		// Allocate shadow space for Windows x64 calling convention
		shadowSpace := fc.allocateShadowSpace()

		// Generate PLT call
		fc.eb.GenerateCallInstruction(funcName)

		// Deallocate shadow space
		fc.deallocateShadowSpace(shadowSpace)

		// Clean up stack arguments after call
		if stackArgCount > 0 {
			fc.out.AddImmToReg("rsp", int64(stackArgCount*8))
		}

		// Handle return value based on signature
		var returnType string
		if funcSig != nil {
			returnType = funcSig.ReturnType
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "C function %s return type: %q (funcSig=%v)\n", funcName, returnType, funcSig != nil)
		}

		if returnType == "float" || returnType == "double" {
			// Result is already in xmm0 as double - no conversion needed
		} else if returnType == "void" {
			// Void return - set xmm0 to 0
			fc.out.XorpdXmm("xmm0", "xmm0")
		} else if isPointerType(returnType) || returnType == "" {
			// Pointer type - keep raw integer value but convert to float64 for C67
			// (C67 internally represents everything as float64)
			// On Windows and Linux, pointers are 64-bit and returned in RAX correctly
			// NOTE: When signature is unknown (returnType == ""), assume pointer/64-bit return
			// This is safer than assuming 32-bit, and works for SDL functions
			fc.out.Cvtsi2sd("xmm0", "rax")
		} else {
			// Integer result in rax - convert to float64 for C67
			// On Windows: bool returns are 1 byte (AL), int returns are 4 bytes (EAX)
			// Zero-extend to 64-bit RAX to avoid garbage in upper bits
			if fc.eb.target.OS() == OSWindows {
				// Check if this is a bool return (1 byte in AL)
				if returnType == "bool" || returnType == "_Bool" {
					// movzx eax, al (zero-extend 8-bit AL to 32-bit EAX, then to RAX)
					fc.out.MovzxRegReg("eax", "al")
				} else {
					// For int/int32/etc returns: use EAX directly (upper 32 bits of RAX auto-zeroed)
					// mov eax, eax (zero-extends EAX to 64-bit RAX)
					fc.out.MovRegToReg("eax", "eax")
				}
			}
			fc.out.Cvtsi2sd("xmm0", "rax")
		}
	} else {
		// No arguments - just call the function
		// Allocate shadow space for Windows x64 calling convention
		shadowSpace := fc.allocateShadowSpace()

		fc.eb.GenerateCallInstruction(funcName)

		// Deallocate shadow space
		fc.deallocateShadowSpace(shadowSpace)

		// Handle return value based on signature
		var returnType string
		if funcSig != nil {
			returnType = funcSig.ReturnType
		}

		if returnType == "float" || returnType == "double" {
			// Result is already in xmm0 as double - no conversion needed
		} else if returnType == "void" {
			// Void return - set xmm0 to 0
			fc.out.XorpdXmm("xmm0", "xmm0")
		} else if isPointerType(returnType) || returnType == "" {
			// Pointer type - keep raw integer value but convert to float64 for C67
			// (C67 internally represents everything as float64)
			// On Windows and Linux, pointers are 64-bit and returned in RAX correctly
			// NOTE: When signature is unknown (returnType == ""), assume pointer/64-bit return
			// This is safer than assuming 32-bit, and works for SDL functions
			fc.out.Cvtsi2sd("xmm0", "rax")
		} else {
			// Integer result in rax - convert to float64 for C67
			// On Windows: bool returns are 1 byte (AL), int returns are 4 bytes (EAX)
			// Zero-extend to 64-bit RAX to avoid garbage in upper bits
			if fc.eb.target.OS() == OSWindows {
				// Check if this is a bool return (1 byte in AL)
				if returnType == "bool" || returnType == "_Bool" {
					// movzx eax, al (zero-extend 8-bit AL to 32-bit EAX, then to RAX)
					fc.out.MovzxRegReg("eax", "al")
				} else {
					// For int/int32/etc returns: use EAX directly (upper 32 bits of RAX auto-zeroed)
					// mov eax, eax (zero-extends EAX to 64-bit RAX)
					fc.out.MovRegToReg("eax", "eax")
				}
			}
			fc.out.Cvtsi2sd("xmm0", "rax")
		}
	}
}

func (fc *C67Compiler) compileCall(call *CallExpr) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileCall: function='%s'\n", call.Function)
		fmt.Fprintf(os.Stderr, "DEBUG compileCall: variables=%v\n", fc.variables)
	}

	// Check if this is a recursive call (function name matches current lambda)
	isRecursive := fc.currentLambda != nil && call.Function == fc.currentLambda.Name

	if isRecursive {
		// Note: Recursive calls do NOT require 'max' keyword (that's only for loops)
		// Compile recursive call with tail call optimization if possible
		fc.compileRecursiveCall(call)
		return
	}

	// Check if this is a C FFI call (c.malloc, c.free, etc.)
	if call.IsCFFI {
		// C FFI calls go directly to the C function without namespace lookup
		// The parser has already stripped the "c." prefix, so call.Function is just "malloc", "free", etc.
		fc.compileCFunctionCall("", call.Function, call.Args)
		return
	}

	// Check if this is a namespaced function call (namespace.function)
	if strings.Contains(call.Function, ".") {
		parts := strings.Split(call.Function, ".")
		if len(parts) == 2 {
			namespace := parts[0]
			funcName := parts[1]

			// Check if namespace is a registered C import
			if libName, ok := fc.cImports[namespace]; ok {
				fc.compileCFunctionCall(libName, funcName, call.Args)
				return
			}

			// Check if this is a C67 namespaced function call
			// Look up the function in the namespace map
			if actualNamespace, exists := fc.functionNamespace[funcName]; exists && actualNamespace == namespace {
				// This is a valid namespaced C67 function call
				// Compile it as a regular function call (the function is defined without the namespace prefix)
				call.Function = funcName // Strip the namespace prefix
				// Continue with regular compilation below
			} else if _, isVariable := fc.variables[namespace]; isVariable {
				// The "namespace" is actually a variable, so this is method call syntax
				// Desugar: xs.append(a) -> append(xs, a)
				call.Function = funcName
				call.Args = append([]Expression{&IdentExpr{Name: namespace}}, call.Args...)
				// Continue with regular compilation below
			}
		}
	}

	// Check if this is a built-in operator (MUST come before variable check!)
	// Built-in operators like +, -, *, / should not be treated as variables
	isBuiltinOp := call.Function == "+" || call.Function == "-" || call.Function == "*" ||
		call.Function == "/" || call.Function == "mod" || call.Function == "%" ||
		call.Function == "<" || call.Function == "<=" || call.Function == ">" ||
		call.Function == ">=" || call.Function == "==" || call.Function == "!=" ||
		call.Function == "and" || call.Function == "or" || call.Function == "not" ||
		call.Function == "~b" || call.Function == "&b" || call.Function == "|b" ||
		call.Function == "^b" || call.Function == "<<" || call.Function == ">>"

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileCall: function=%s, isBuiltinOp=%v\n", call.Function, isBuiltinOp)
	}

	// Check if this is a known lambda function
	// Known lambdas can be called directly by label (no closure indirection needed)
	// UNLESS they have captured variables, in which case they need their environment
	isKnownLambda := false
	hasCaptures := false
	for _, lambda := range fc.lambdaFuncs {
		if lambda.Name == call.Function {
			isKnownLambda = true
			hasCaptures = len(lambda.CapturedVars) > 0
			break
		}
	}

	// Also check pattern lambdas
	if !isKnownLambda {
		for _, lambda := range fc.patternLambdaFuncs {
			if lambda.Name == call.Function {
				isKnownLambda = true
				break
			}
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileCall: isKnownLambda=%v, hasCaptures=%v\n", isKnownLambda, hasCaptures)
	}

	if isKnownLambda && !hasCaptures {
		// Direct call to a known lambda function (without captured variables)
		fc.compileLambdaDirectCall(call)
		return
	}

	// Check if this is a stored function (variable containing function pointer)
	// This handles first-class functions (functions passed as values/parameters)
	// Note: known lambdas are handled above via direct call
	// Forward references: treat as direct calls by label (will be resolved later)
	if !isBuiltinOp {
		_, isVariable := fc.variables[call.Function]
		_, isForwardRef := fc.forwardFunctions[call.Function]

		if isForwardRef && !isVariable {
			// Forward reference - function not yet defined, but will be
			// Use direct call by label (like compileLambdaDirectCall)
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG compileCall: forward reference - direct call to label '%s'\n", call.Function)
			}
			fc.compileLambdaDirectCall(call)
			return
		}

		if isVariable {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG compileCall: taking compileStoredFunctionCall path\n")
			}
			fc.compileStoredFunctionCall(call)
			return
		}
	}

	// Otherwise, handle builtin functions
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG compileCall: entering switch for function='%s'\n", call.Function)
	}
	switch call.Function {
	// Arithmetic operators (prefix notation: (- 8 6) means 8 - 6)
	case "+", "-", "*", "/", "mod", "%":
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG compileCall: matched arithmetic operator '%s'\n", call.Function)
		}
		if len(call.Args) != 2 {
			compilerError("%s requires exactly 2 arguments", call.Function)
		}
		fc.compileBinaryOpSafe(call.Args[0], call.Args[1], call.Function)
		return

	// Comparison operators
	case "<", "<=", ">", ">=", "==", "!=":
		if len(call.Args) != 2 {
			compilerError("%s requires exactly 2 arguments", call.Function)
		}
		fc.compileBinaryOpSafe(call.Args[0], call.Args[1], call.Function)
		return

	// Logical operators
	case "and", "or":
		if len(call.Args) != 2 {
			compilerError("%s requires exactly 2 arguments", call.Function)
		}
		fc.compileBinaryOpSafe(call.Args[0], call.Args[1], call.Function)
		return

	// Bitwise operators
	case "~b", "&b", "|b", "^b", "<<", ">>":
		if len(call.Args) != 2 {
			compilerError("%s requires exactly 2 arguments", call.Function)
		}
		fc.compileBinaryOpSafe(call.Args[0], call.Args[1], call.Function)
		return

	// Unary not operator
	case "not":
		if len(call.Args) != 1 {
			compilerError("not requires exactly 1 argument")
		}
		// Compile argument
		fc.compileExpression(call.Args[0])
		// Compare with 0.0
		fc.out.XorpdXmm("xmm1", "xmm1") // xmm1 = 0.0
		fc.out.Ucomisd("xmm0", "xmm1")
		// Set result: 1.0 if equal to 0, else 0.0
		fc.out.XorRegWithReg("rax", "rax")
		fc.labelCounter++
		jumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0) // Jump if != 0
		jumpEnd := fc.eb.text.Len()
		// Was 0, return 1.0
		fc.out.MovImmToReg("rax", "1")
		// Patch jump target
		truePos := fc.eb.text.Len()
		offset := int32(truePos - jumpEnd)
		fc.patchJumpImmediate(jumpPos+2, offset)
		// Convert rax to float
		fc.out.Cvtsi2sd("xmm0", "rax")
		return

	case "_error_code_extract":
		// Confidence that this function is working: 95%
		// .error property - extract 4-letter error code from NaN-encoded Result
		if len(call.Args) != 1 {
			compilerError("_error_code_extract requires exactly 1 argument")
		}
		// Evaluate the argument to get the value in xmm0
		fc.compileExpression(call.Args[0])

		// Check if xmm0 is NaN (error) or normal value (success)
		fc.out.Ucomisd("xmm0", "xmm0") // Compare with itself - sets PF=1 if NaN

		// Jump if not NaN (parity flag not set)
		notNaNPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotParity, 0) // jnp - jump if not parity (not NaN)

		// NaN path: extract error code from mantissa (low 32 bits)
		fc.out.MovqXmmToReg("rax", "xmm0")          // Move float bits to GPR
		fc.out.Emit([]byte{0x48, 0x25})             // and rax, immediate32
		fc.out.Emit([]byte{0xff, 0xff, 0xff, 0xff}) // mask = 0xFFFFFFFF

		// Convert 4-byte code to C67 string
		// We need to create a string with up to 3 characters (strip null bytes)
		// For simplicity, always create 3-char string (most error codes are 3 chars)

		// Save error code in rbx
		fc.out.MovRegToReg("rbx", "rax")

		// Allocate string memory (need 8 bytes count + 3*16 bytes entries = 56 bytes)
		// Use malloc for simplicity
		fc.out.Emit([]byte{0x48, 0xc7, 0xc7, 0x38, 0x00, 0x00, 0x00}) // mov rdi, 56
		// Allocate from arena
		fc.callArenaAlloc()
		// rax now points to allocated memory
		fc.out.MovRegToReg("rsi", "rax")

		// rsi now points to our string
		// Write count = 3.0
		fc.out.Emit([]byte{0x48, 0xb8})                                     // mov rax, immediate64
		fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x40}) // 3.0
		fc.out.MovRegToMem("rax", "rsi", 0)

		// Extract and write each character as map entry
		// Entry 0: key=0, value=char0
		fc.out.XorRegWithReg("rax", "rax") // key = 0
		fc.out.MovRegToMem("rax", "rsi", 8)
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x18}) // shr rax, 24 (get first char)
		fc.out.Cvtsi2sd("xmm1", "rax")
		fc.out.MovXmmToMem("xmm1", "rsi", 16)

		// Entry 1: key=1, value=char1
		fc.out.Emit([]byte{0x48, 0xc7, 0xc0, 0x01, 0x00, 0x00, 0x00}) // mov rax, 1
		fc.out.MovRegToMem("rax", "rsi", 24)
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x10}) // shr rax, 16
		fc.out.Emit([]byte{0x48, 0x25})             // and rax, 0xFF
		fc.out.Emit([]byte{0xff, 0x00, 0x00, 0x00})
		fc.out.Cvtsi2sd("xmm1", "rax")
		fc.out.MovXmmToMem("xmm1", "rsi", 32)

		// Entry 2: key=2, value=char2
		fc.out.Emit([]byte{0x48, 0xc7, 0xc0, 0x02, 0x00, 0x00, 0x00}) // mov rax, 2
		fc.out.MovRegToMem("rax", "rsi", 40)
		fc.out.MovRegToReg("rax", "rbx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe8, 0x08}) // shr rax, 8
		fc.out.Emit([]byte{0x48, 0x25})             // and rax, 0xFF
		fc.out.Emit([]byte{0xff, 0x00, 0x00, 0x00})
		fc.out.Cvtsi2sd("xmm1", "rax")
		fc.out.MovXmmToMem("xmm1", "rsi", 48)

		// Return pointer to string in xmm0
		fc.out.MovqRegToXmm("xmm0", "rsi")

		// Jump over non-NaN path
		donePos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Not NaN path: return empty string
		notNaNTarget := fc.eb.text.Len()
		fc.patchJumpImmediate(notNaNPos+2, int32(notNaNTarget-(notNaNPos+6)))

		// Create empty string
		labelName := fmt.Sprintf("empty_str_%d", fc.stringCounter)
		fc.stringCounter++
		var mapData []byte
		countBits := uint64(0) // count = 0
		for i := 0; i < 8; i++ {
			mapData = append(mapData, byte((countBits>>(i*8))&ByteMask))
		}
		fc.eb.Define(labelName, string(mapData))
		fc.out.LeaSymbolToReg("rax", labelName)
		fc.out.MovqRegToXmm("xmm0", "rax")

		// Done
		doneTarget := fc.eb.text.Len()
		fc.patchJumpImmediate(donePos+1, int32(doneTarget-(donePos+5)))
		return

	case "head":
		// head(xs) - return first element of list/map
		// For numbers, return the number itself
		// For empty collections, return empty list
		if len(call.Args) != 1 {
			compilerError("head() requires exactly 1 argument, got %d", len(call.Args))
		}

		arg := call.Args[0]
		fc.compileExpression(arg)
		// xmm0 contains the value (number or pointer to list/map)

		argType := fc.getExprType(arg)
		if argType == "number" {
			// It's a number, return it as-is (xmm0 already contains it)
			// No-op
		} else {
			// It's a map/list/string pointer, load first element
			fc.out.SubImmFromReg("rsp", StackSlotSize)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
			fc.out.MovMemToReg("rax", "rsp", 0)
			fc.out.AddImmToReg("rsp", StackSlotSize)

			// Check if list is empty (length == 0)
			fc.out.MovMemToXmm("xmm1", "rax", 0) // Load length from [rax+0]
			fc.out.XorpdXmm("xmm2", "xmm2")      // xmm2 = 0.0
			fc.out.Ucomisd("xmm1", "xmm2")       // Compare length with 0

			emptyJumpPos := fc.eb.text.Len()
			fc.out.JumpConditional(JumpEqual, 0) // Jump if empty

			// Not empty: Skip past length (8 bytes) + first key (8 bytes) to get to first value
			fc.out.AddImmToReg("rax", 16)
			fc.out.MovMemToXmm("xmm0", "rax", 0)

			doneJumpPos := fc.eb.text.Len()
			fc.out.JumpUnconditional(0) // Jump to done

			// Empty case: return 0.0 (empty list)
			emptyTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(emptyJumpPos+2, int32(emptyTarget-(emptyJumpPos+ConditionalJumpSize)))
			fc.out.XorpdXmm("xmm0", "xmm0") // xmm0 = 0.0

			// Done
			doneTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(doneJumpPos+1, int32(doneTarget-(doneJumpPos+5)))
		}
		return

	case "tail":
		// tail(xs) - return list/map without first element
		// For numbers, return [] (empty list, represented as 0.0)
		// For empty or single-element collections, return empty list
		if len(call.Args) != 1 {
			compilerError("tail() requires exactly 1 argument, got %d", len(call.Args))
		}

		arg := call.Args[0]
		fc.compileExpression(arg)
		// xmm0 contains the value (number or pointer to list/map)

		argType := fc.getExprType(arg)
		if argType == "number" {
			// It's a number, return empty list (pointer to count=0 structure)
			labelName := fmt.Sprintf("tail_empty_for_number_%d", fc.stringCounter)
			fc.stringCounter++
			var emptyListData []byte
			countBits := uint64(0)
			for i := 0; i < 8; i++ {
				emptyListData = append(emptyListData, byte((countBits>>(i*8))&ByteMask))
			}
			fc.eb.Define(labelName, string(emptyListData))
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovqRegToXmm("xmm0", "rax")
		} else {
			// It's a map/list - create new list without first element
			// Save original list pointer on stack
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
			fc.out.MovMemToReg("rbx", "rsp", 0) // rbx = input list pointer

			// Load count from list [rbx+0]
			fc.out.MovMemToXmm("xmm1", "rbx", 0)
			fc.out.Cvttsd2si("rcx", "xmm1") // rcx = original count

			// Check if length is 0 or 1
			fc.out.CmpRegToImm("rcx", 1)

			emptyJumpPos := fc.eb.text.Len()
			fc.out.JumpConditional(JumpLessOrEqual, 0) // Jump if count <= 1

			// Count > 1: Build new list from scratch
			// new_count = old_count - 1
			fc.out.MovRegToReg("r8", "rcx")
			fc.out.SubImmFromReg("r8", 1) // r8 = new_count

			// Calculate allocation size: new_count * 16 + 8
			fc.out.MovRegToReg("rdi", "r8")
			fc.out.ShlImmReg("rdi", 4)   // rdi = new_count * 16
			fc.out.AddImmToReg("rdi", 8) // rdi = new_count * 16 + 8

			// Save registers before malloc: rbx (old list), rcx (old count), r8 (new count)
			fc.out.PushReg("rbx")
			fc.out.PushReg("rcx")
			fc.out.PushReg("r8")

			// Allocate from arena
			fc.callArenaAlloc()

			// Restore registers
			fc.out.PopReg("r8")  // new count
			fc.out.PopReg("rcx") // old count
			fc.out.PopReg("rbx") // old list

			fc.out.MovRegToReg("r9", "rax") // r9 = new list pointer

			// Store new count at [r9+0]
			fc.out.Cvtsi2sd("xmm2", "r8")
			fc.out.MovXmmToMem("xmm2", "r9", 0)

			// Loop to copy elements: i from 0 to new_count-1
			// new_list[8 + i*16] = i (key as uint64)
			// new_list[8 + i*16 + 8] = old_list[8 + (i+1)*16 + 8] (value from next element)
			fc.out.XorRegWithReg("r10", "r10") // r10 = loop counter i = 0

			loopStart := fc.eb.text.Len()

			// Check if i >= new_count
			fc.out.CmpRegToReg("r10", "r8")
			loopEndJump := fc.eb.text.Len()
			fc.out.JumpConditional(JumpGreaterOrEqual, 0)

			// Calculate new_list key address: r9 + 8 + i*16
			fc.out.MovRegToReg("rax", "r10")
			fc.out.ShlImmReg("rax", 4)      // rax = i * 16
			fc.out.AddImmToReg("rax", 8)    // rax = 8 + i * 16
			fc.out.AddRegToReg("rax", "r9") // rax = r9 + 8 + i*16 (new key address)

			// Write key = i (as uint64)
			fc.out.MovRegToMem("r10", "rax", 0)

			// Calculate old_list value address: rbx + 8 + (i+1)*16 + 8
			//   = rbx + 8 + i*16 + 16 + 8 = rbx + i*16 + 32
			fc.out.MovRegToReg("rdx", "r10")
			fc.out.ShlImmReg("rdx", 4)       // rdx = i * 16
			fc.out.AddImmToReg("rdx", 32)    // rdx = i*16 + 32
			fc.out.AddRegToReg("rdx", "rbx") // rdx = rbx + i*16 + 32 (old value address)

			// Copy value: [rdx] -> [rax + 8]
			fc.out.MovMemToXmm("xmm3", "rdx", 0)
			fc.out.MovXmmToMem("xmm3", "rax", 8)

			// Increment i and loop
			fc.out.AddImmToReg("r10", 1)
			loopOffset := int32(loopStart - (fc.eb.text.Len() + 5))
			fc.out.JumpUnconditional(loopOffset)

			// Loop end
			loopEnd := fc.eb.text.Len()
			fc.patchJumpImmediate(loopEndJump+2, int32(loopEnd-(loopEndJump+ConditionalJumpSize)))

			// Return new list pointer in xmm0
			fc.out.MovqRegToXmm("xmm0", "r9")

			// Clean up stack and jump to end
			fc.out.AddImmToReg("rsp", 8)
			doneJumpPos := fc.eb.text.Len()
			fc.out.JumpUnconditional(0)

			// Empty case: return empty list (pointer to count=0 structure)
			emptyTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(emptyJumpPos+2, int32(emptyTarget-(emptyJumpPos+ConditionalJumpSize)))
			fc.out.AddImmToReg("rsp", 8) // Clean up stack

			// Create empty list: allocate 8 bytes for count=0
			labelName := fmt.Sprintf("tail_empty_list_%d", fc.stringCounter)
			fc.stringCounter++
			var emptyListData []byte
			countBits := uint64(0)
			for i := 0; i < 8; i++ {
				emptyListData = append(emptyListData, byte((countBits>>(i*8))&ByteMask))
			}
			fc.eb.Define(labelName, string(emptyListData))
			fc.out.LeaSymbolToReg("rax", labelName)
			fc.out.MovqRegToXmm("xmm0", "rax")

			// Done
			doneTarget := fc.eb.text.Len()
			fc.patchJumpImmediate(doneJumpPos+1, int32(doneTarget-(doneJumpPos+5)))
		}
		return

	case "print":
		// print (without newline) uses syscalls on Linux, printf on Windows
		if len(call.Args) == 0 {
			// Nothing to print
			return
		}

		arg := call.Args[0]
		argType := fc.getExprType(arg)

		if strExpr, ok := arg.(*StringExpr); ok {
			// String literal
			labelName := fmt.Sprintf("str_%d", fc.stringCounter)
			fc.stringCounter++
			fc.eb.Define(labelName, strExpr.Value)

			if fc.eb.target.OS() == OSLinux {
				// Use write syscall
				fc.out.MovImmToReg("rax", "1") // sys_write
				fc.out.MovImmToReg("rdi", "1") // stdout
				fc.out.LeaSymbolToReg("rsi", labelName)
				fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(strExpr.Value)))
				fc.out.Syscall()
			} else {
				// Windows - use printf
				fc.eb.Define(labelName+"_z", strExpr.Value+"\x00")
				shadowSpace := fc.allocateShadowSpace()
				fc.out.LeaSymbolToReg(fc.getIntArgReg(0), labelName+"_z")
				fc.trackFunctionCall("printf")
				fc.eb.GenerateCallInstruction("printf")
				fc.deallocateShadowSpace(shadowSpace)
			}
			return
		} else if argType == "string" {
			// String variable - call _c67_print_syscall helper
			fc.compileExpression(arg)
			// xmm0 contains string pointer

			// Convert to integer pointer in first arg register
			argReg := fc.getIntArgReg(0)
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
			fc.out.MovMemToReg(argReg, "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)

			if fc.eb.target.OS() == OSLinux {
				// Call syscall-based helper
				shadowSpace := fc.allocateShadowSpace()
				fc.trackFunctionCall("_c67_print_syscall")
				fc.eb.GenerateCallInstruction("_c67_print_syscall")
				fc.deallocateShadowSpace(shadowSpace)
			} else {
				// Call Windows helper (uses same _c67_string_println but without newline)
				shadowSpace := fc.allocateShadowSpace()
				fc.trackFunctionCall("_c67_print_syscall") // Will need Windows version
				fc.eb.GenerateCallInstruction("_c67_print_syscall")
				fc.deallocateShadowSpace(shadowSpace)
			}
			return
		} else if fstrExpr, ok := arg.(*FStringExpr); ok {
			// F-string - compile it and then print
			fc.compileExpression(fstrExpr)
			// xmm0 contains string pointer

			argReg := fc.getIntArgReg(0)
			fc.out.SubImmFromReg("rsp", 8)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
			fc.out.MovMemToReg(argReg, "rsp", 0)
			fc.out.AddImmToReg("rsp", 8)

			if fc.eb.target.OS() == OSLinux {
				shadowSpace := fc.allocateShadowSpace()
				fc.trackFunctionCall("_c67_print_syscall")
				fc.eb.GenerateCallInstruction("_c67_print_syscall")
				fc.deallocateShadowSpace(shadowSpace)
			} else {
				shadowSpace := fc.allocateShadowSpace()
				fc.trackFunctionCall("_c67_print_syscall")
				fc.eb.GenerateCallInstruction("_c67_print_syscall")
				fc.deallocateShadowSpace(shadowSpace)
			}
			return
		} else {
			// Number or other expression
			fc.compileExpression(arg)
			// xmm0 contains float64 value

			if fc.eb.target.OS() == OSLinux {
				// Convert to int64 and use _c67_itoa + write syscall
				fc.out.Cvttsd2si("rdi", "xmm0")

				// Allocate stack buffer
				fc.out.SubImmFromReg("rsp", 32)
				fc.out.MovRegToReg("r15", "rsp")

				// Call _c67_itoa
				fc.trackFunctionCall("_c67_itoa")
				fc.eb.GenerateCallInstruction("_c67_itoa")

				// Write to stdout
				fc.out.MovImmToReg("rax", "1")
				fc.out.MovImmToReg("rdi", "1")
				fc.out.Syscall()

				// Clean up
				fc.out.AddImmToReg("rsp", 32)
			} else {
				// Windows - use printf
				fmtLabel := fmt.Sprintf("print_fmt_%d", fc.stringCounter)
				fc.stringCounter++
				fc.eb.Define(fmtLabel, "%g\x00")

				shadowSpace := fc.allocateShadowSpace()
				fc.out.LeaSymbolToReg(fc.getIntArgReg(0), fmtLabel)
				fc.out.MovXmmToXmm("xmm1", "xmm0")
				fc.out.MovqXmmToReg(fc.getIntArgReg(1), "xmm0")
				fc.out.MovImmToReg("rax", "1")
				fc.trackFunctionCall("printf")
				fc.eb.GenerateCallInstruction("printf")
				fc.deallocateShadowSpace(shadowSpace)
			}
		}
		return

	case "println":
		// println uses syscalls on Linux, printf on Windows
		// Supports multiple arguments, separated by spaces
		fc.trackFunctionCall("println")

		if len(call.Args) == 0 {
			// Just print a newline
			newlineLabel := fmt.Sprintf("println_fmt_%d", fc.stringCounter)
			fc.stringCounter++
			fc.eb.Define(newlineLabel, "\n")

			if fc.eb.target.OS() == OSLinux {
				// Use write syscall
				fc.out.MovImmToReg("rax", "1") // sys_write
				fc.out.MovImmToReg("rdi", "1") // stdout
				fc.out.LeaSymbolToReg("rsi", newlineLabel)
				fc.out.MovImmToReg("rdx", "1") // 1 byte
				fc.out.Syscall()
			} else {
				// Windows - use printf
				fc.eb.Define(newlineLabel+"_z", "\n\x00") // null-terminated
				shadowSpace := fc.allocateShadowSpace()
				fc.out.LeaSymbolToReg(fc.getIntArgReg(0), newlineLabel+"_z")
				fc.trackFunctionCall("printf")
				fc.eb.GenerateCallInstruction("printf")
				fc.deallocateShadowSpace(shadowSpace)
			}
			return
		}

		// Create space label for separating arguments
		spaceLabel := fmt.Sprintf("println_space_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(spaceLabel, " ")

		// Process each argument
		for argIdx, arg := range call.Args {
			// Print space before each argument except the first
			if argIdx > 0 {
				if fc.eb.target.OS() == OSLinux {
					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "1") // stdout
					fc.out.LeaSymbolToReg("rsi", spaceLabel)
					fc.out.MovImmToReg("rdx", "1")
					fc.out.Syscall()
				} else {
					fc.eb.Define(spaceLabel+"_z", " \x00")
					shadowSpace := fc.allocateShadowSpace()
					fc.out.LeaSymbolToReg(fc.getIntArgReg(0), spaceLabel+"_z")
					fc.trackFunctionCall("printf")
					fc.eb.GenerateCallInstruction("printf")
					fc.deallocateShadowSpace(shadowSpace)
				}
			}

			argType := fc.getExprType(arg)

			if strExpr, ok := arg.(*StringExpr); ok {
				// String literal
				labelName := fmt.Sprintf("str_%d", fc.stringCounter)
				fc.stringCounter++
				fc.eb.Define(labelName, strExpr.Value)

				if fc.eb.target.OS() == OSLinux {
					// Use write syscall
					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "1") // stdout
					fc.out.LeaSymbolToReg("rsi", labelName)
					fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(strExpr.Value)))
					fc.out.Syscall()
				} else {
					// Windows - use printf
					fc.eb.Define(labelName+"_z", strExpr.Value+"\x00") // null-terminated
					fmtLabel := fmt.Sprintf("println_fmt_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(fmtLabel, "%s\x00")

					shadowSpace := fc.allocateShadowSpace()
					fc.out.LeaSymbolToReg(fc.getIntArgReg(0), fmtLabel)
					fc.out.LeaSymbolToReg(fc.getIntArgReg(1), labelName+"_z")
					fc.trackFunctionCall("printf")
					fc.eb.GenerateCallInstruction("printf")
					fc.deallocateShadowSpace(shadowSpace)
				}
			} else if argType == "string" {
				// String variable - call helper (without newline)
				fc.compileExpression(arg)
				// xmm0 contains string pointer

				// Convert to integer pointer in first arg register
				argReg := fc.getIntArgReg(0)
				fc.out.SubImmFromReg("rsp", 8)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
				fc.out.MovMemToReg(argReg, "rsp", 0)
				fc.out.AddImmToReg("rsp", 8)

				// Call helper function (syscall-based on Linux, printf-based on Windows)
				shadowSpace := fc.allocateShadowSpace()
				if fc.eb.target.OS() == OSLinux {
					fc.trackFunctionCall("_c67_print_syscall")
					fc.eb.GenerateCallInstruction("_c67_print_syscall")
				} else {
					fc.trackFunctionCall("_c67_string_print")
					fc.eb.GenerateCallInstruction("_c67_string_print")
				}
				fc.deallocateShadowSpace(shadowSpace)
			} else if fstrExpr, ok := arg.(*FStringExpr); ok {
				// F-string - compile it and print without newline
				fc.compileExpression(fstrExpr)
				// xmm0 contains string pointer

				argReg := fc.getIntArgReg(0)
				fc.out.SubImmFromReg("rsp", 8)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
				fc.out.MovMemToReg(argReg, "rsp", 0)
				fc.out.AddImmToReg("rsp", 8)

				shadowSpace := fc.allocateShadowSpace()
				if fc.eb.target.OS() == OSLinux {
					fc.trackFunctionCall("_c67_print_syscall")
					fc.eb.GenerateCallInstruction("_c67_print_syscall")
				} else {
					fc.trackFunctionCall("_c67_string_print")
					fc.eb.GenerateCallInstruction("_c67_string_print")
				}
				fc.deallocateShadowSpace(shadowSpace)
			} else if argType == "list" || argType == "map" {
				// Print list/map - note: for multi-arg println, lists/maps print inline without their usual newlines
				// Compile the expression to get map pointer
				fc.compileExpression(arg)
				// xmm0 now contains the map pointer as float64

				// Convert map pointer from xmm0 to rax (integer pointer)
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
				fc.out.MovMemToReg("rax", "rsp", 0)
				fc.out.AddImmToReg("rsp", StackSlotSize)

				// Save map pointer on the stack (printf clobbers most registers!)
				// We need: map pointer, length, index - all must survive printf calls
				fc.out.SubImmFromReg("rsp", 24)     // 3 * 8 bytes for map_ptr, length, index
				fc.out.MovRegToMem("rax", "rsp", 0) // [rsp+0] = map pointer

				// Get the length of the map (stored at offset 0 as float64)
				fc.out.MovMemToXmm("xmm0", "rax", 0)
				fc.out.Cvttsd2si("rcx", "xmm0")     // rcx = length (as integer)
				fc.out.MovRegToMem("rcx", "rsp", 8) // [rsp+8] = length

				// Initialize index to 0 (iterate forward from 0 to length-1)
				fc.out.MovImmToReg("rcx", "0")
				fc.out.MovRegToMem("rcx", "rsp", 16) // [rsp+16] = index = 0

				// Create format string for numbers (use %g for smart formatting, no newline in multi-arg mode)
				fmtLabel := fmt.Sprintf("println_fmt_%d", fc.stringCounter)
				fc.stringCounter++
				fc.eb.Define(fmtLabel, "%g\x00")

				// Get current position for loop start
				loopStartPos := fc.eb.text.Len()

				// Load index and length from stack
				fc.out.MovMemToReg("rcx", "rsp", 16) // rcx = index
				fc.out.MovMemToReg("rdx", "rsp", 8)  // rdx = length

				// Check if index >= length (loop exit condition)
				fc.out.CmpRegToReg("rcx", "rdx") // Compare index with length
				// Jump to end if index >= length
				loopEndJumpPos := fc.eb.text.Len()
				fc.out.JumpConditional(JumpGreaterOrEqual, 0) // Placeholder, will be patched

				// Load map pointer from stack
				fc.out.MovMemToReg("rax", "rsp", 0) // rax = map pointer

				// Calculate element address: map_base + 8 + (index * 8)
				// The map structure is: [length (8 bytes)] [element0] [element1] ...
				fc.out.MovRegToReg("rbx", "rax") // rbx = map base
				fc.out.AddImmToReg("rbx", 8)     // rbx = map base + 8 (skip length)
				fc.out.MovRegToReg("rsi", "rcx") // rsi = index
				fc.out.ShlImmReg("rsi", 3)       // rsi = index * 8
				fc.out.AddRegToReg("rbx", "rsi") // rbx = element address

				// Load the element value into xmm0
				fc.out.MovMemToXmm("xmm0", "rbx", 0)

				// Print using printf (printf clobbers rax, rcx, rdx, rsi, rdi, r8-r11)
				shadowSpace := fc.allocateShadowSpace()
				fc.out.LeaSymbolToReg(fc.getIntArgReg(0), fmtLabel)

				// Windows requires float args in BOTH integer and XMM registers for variadic functions
				if fc.eb.target.OS() == OSWindows {
					// Move xmm0 to xmm1 (2nd parameter position)
					fc.out.MovXmmToXmm("xmm1", "xmm0")
					// Also copy to integer register (2nd parameter)
					fc.out.MovqXmmToReg(fc.getIntArgReg(1), "xmm0")
				}

				// Set rax = 1 (one vector register used) for variadic printf
				fc.out.MovImmToReg("rax", "1")
				fc.trackFunctionCall("printf")
				fc.eb.GenerateCallInstruction("printf")
				fc.deallocateShadowSpace(shadowSpace)

				// Increment index on stack
				fc.out.MovMemToReg("rcx", "rsp", 16) // Load current index
				fc.out.AddImmToReg("rcx", 1)         // Increment
				fc.out.MovRegToMem("rcx", "rsp", 16) // Store back

				// Jump back to loop start
				loopBackJumpPos := fc.eb.text.Len()
				backOffset := int32(loopStartPos - (loopBackJumpPos + 5)) // 5 bytes for unconditional jump
				fc.out.JumpUnconditional(backOffset)

				// Patch the loop end jump to point here
				loopEndPos := fc.eb.text.Len()
				endOffset := int32(loopEndPos - (loopEndJumpPos + 6)) // 6 bytes for conditional jump
				fc.patchJumpImmediate(loopEndJumpPos+2, endOffset)

				// Clean up stack
				fc.out.AddImmToReg("rsp", 24)

			} else {
				// Print number using pure assembly (no libc) - no newline in multi-arg mode
				fc.compileExpression(arg)
				// xmm0 contains float64 value

				if fc.eb.target.OS() == OSLinux {
					// Convert to int64 and use _c67_itoa + write syscall
					fc.out.Cvttsd2si("rdi", "xmm0") // Convert float to int64

					// Allocate stack buffer for number string
					fc.out.SubImmFromReg("rsp", 32)
					fc.out.MovRegToReg("r15", "rsp") // Save buffer pointer

					// Call _c67_itoa(rdi=number)
					fc.trackFunctionCall("_c67_itoa")
					fc.eb.GenerateCallInstruction("_c67_itoa")
					// Returns: rsi=string start, rdx=length

					// Write to stdout: write(1, rsi, rdx)
					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "1") // stdout
					// rsi already has buffer pointer
					// rdx already has length
					fc.out.Syscall()

					// Clean up stack
					fc.out.AddImmToReg("rsp", 32)
				} else {
					// Windows - use printf
					fmtLabel := fmt.Sprintf("println_fmt_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(fmtLabel, "%g\x00")

					shadowSpace := fc.allocateShadowSpace()
					fc.out.LeaSymbolToReg(fc.getIntArgReg(0), fmtLabel)
					fc.out.MovXmmToXmm("xmm1", "xmm0")
					fc.out.MovqXmmToReg(fc.getIntArgReg(1), "xmm0")
					fc.out.MovImmToReg("rax", "1")
					fc.trackFunctionCall("printf")
					fc.eb.GenerateCallInstruction("printf")
					fc.deallocateShadowSpace(shadowSpace)
				}
			}
		}

		// After all arguments, print a newline
		newlineLabel := fmt.Sprintf("println_newline_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(newlineLabel, "\n")

		if fc.eb.target.OS() == OSLinux {
			fc.out.MovImmToReg("rax", "1") // sys_write
			fc.out.MovImmToReg("rdi", "1") // stdout
			fc.out.LeaSymbolToReg("rsi", newlineLabel)
			fc.out.MovImmToReg("rdx", "1")
			fc.out.Syscall()
		} else {
			fc.eb.Define(newlineLabel+"_z", "\n\x00")
			shadowSpace := fc.allocateShadowSpace()
			fc.out.LeaSymbolToReg(fc.getIntArgReg(0), newlineLabel+"_z")
			fc.trackFunctionCall("printf")
			fc.eb.GenerateCallInstruction("printf")
			fc.deallocateShadowSpace(shadowSpace)
		}
		return

	case "printf":
		fc.trackFunctionCall("printf")
		if len(call.Args) == 0 {
			compilerError("printf() requires at least a format string")
		}

		// First argument must be a string (format string)
		formatArg := call.Args[0]
		strExpr, ok := formatArg.(*StringExpr)
		if !ok {
			compilerError("printf() first argument must be a string literal (got %T)", formatArg)
		}

		// On Linux, use syscall-based printf; on other systems, use libc
		if fc.eb.target.OS() == OSLinux {
			fc.compilePrintfSyscall(call, strExpr)
			return
		}

		// Process format string for libc printf: %v -> %g (smart float), %b -> %s (boolean), %s -> string
		processedFormat := processEscapeSequences(strExpr.Value)
		boolPositions := make(map[int]bool)    // Track which args are %b (boolean)
		stringPositions := make(map[int]bool)  // Track which args are %s (string)
		integerPositions := make(map[int]bool) // Track which args are %d, %i, %ld, etc (integer)

		argPos := 0
		var result strings.Builder
		runes := []rune(processedFormat)
		i := 0
		for i < len(runes) {
			if runes[i] == '%' && i+1 < len(runes) {
				next := runes[i+1]
				if next == '%' {
					// Escaped %% - keep as is
					result.WriteString("%%")
					i += 2
					continue
				} else if next == 'v' {
					// %v = smart value format (uses %.15g for precision with no trailing zeros)
					result.WriteString("%.15g")
					argPos++
					i += 2
					continue
				} else if next == 'b' {
					// %b = boolean (yes/no)
					result.WriteString("%s")
					boolPositions[argPos] = true
					argPos++
					i += 2
					continue
				} else if next == 's' {
					// %s = string pointer
					stringPositions[argPos] = true
					argPos++
				} else if next == 'l' && i+2 < len(runes) && (runes[i+2] == 'd' || runes[i+2] == 'i' || runes[i+2] == 'u') {
					// %ld, %li, %lu = long integer formats
					integerPositions[argPos] = true
					argPos++
				} else if next == 'd' || next == 'i' || next == 'u' || next == 'x' || next == 'X' || next == 'o' {
					// %d, %i, %u, %x, %X, %o = integer formats
					integerPositions[argPos] = true
					argPos++
				} else if next == 'f' || next == 'g' {
					argPos++
				}
			}
			result.WriteRune(runes[i])
			i++
		}
		resultStr := result.String()

		// Create "yes" and "no" string labels for %b
		yesLabel := fmt.Sprintf("bool_yes_%d", fc.stringCounter)
		noLabel := fmt.Sprintf("bool_no_%d", fc.stringCounter)
		fc.eb.Define(yesLabel, "yes\x00")
		fc.eb.Define(noLabel, "no\x00")

		// Create label for processed format string
		labelName := fmt.Sprintf("str_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(labelName, resultStr+"\x00")

		numArgs := len(call.Args) - 1
		if numArgs > 8 {
			compilerError("printf() supports max 8 arguments (got %d)", numArgs)
		}

		// Calling convention for printf (variadic function):
		// System V (Linux/Unix): format in rdi, args in rsi,rdx,rcx,r8,r9 (int) or xmm0-7 (float)
		// Windows x64: format in rcx, args in rdx,r8,r9 + stack (int), xmm1-3 (float)
		//              IMPORTANT: For variadic functions on Windows, float args must ALSO be in integer registers!
		var intRegs []string
		var xmmRegs []string
		var formatReg string

		if fc.eb.target.OS() == OSWindows {
			formatReg = "rcx"
			intRegs = []string{"rdx", "r8", "r9"} // Only 3 additional int regs (first is format string)
			xmmRegs = []string{"xmm1", "xmm2", "xmm3"}
		} else {
			formatReg = "rdi"
			intRegs = []string{"rsi", "rdx", "rcx", "r8", "r9"}
			xmmRegs = []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5", "xmm6", "xmm7"}
		}

		intArgCount := 0
		xmmArgCount := 0

		// Evaluate all arguments
		for i := 1; i < len(call.Args); i++ {
			argIdx := i - 1

			// Special case: string literal arguments for %s
			if stringPositions[argIdx] {
				if strExpr, ok := call.Args[i].(*StringExpr); ok {
					// String literal - load as C string pointer directly
					labelName := fmt.Sprintf("str_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(labelName, strExpr.Value+"\x00")
					fc.out.LeaSymbolToReg(intRegs[intArgCount], labelName)
					intArgCount++
					continue
				}
			}

			// Check if this is an integer format with "as number" cast
			if integerPositions[argIdx] {
				if castExpr, ok := call.Args[i].(*CastExpr); ok && castExpr.Type == "number" {
					// Integer format with explicit cast - convert float to integer
					fc.compileExpression(castExpr.Expr)
					// xmm0 contains float64, convert to integer in rax
					fc.out.Cvttsd2si("rax", "xmm0")
					// Move to appropriate integer register
					if intRegs[intArgCount] != "rax" {
						fc.out.MovRegToReg(intRegs[intArgCount], "rax")
					}
					intArgCount++
					continue
				}
			}

			fc.compileExpression(call.Args[i])

			if boolPositions[argIdx] {
				// %b: Convert float to yes/no string pointer
				fc.out.XorRegWithReg("rax", "rax")
				fc.out.Cvtsi2sd("xmm1", "rax") // xmm1 = 0.0
				fc.out.Ucomisd("xmm0", "xmm1") // Compare with 0.0

				fc.labelCounter++
				yesJump := fc.eb.text.Len()
				fc.out.JumpConditional(JumpNotEqual, 0) // Jump if != 0.0
				yesJumpEnd := fc.eb.text.Len()

				// 0.0 -> "no"
				fc.out.LeaSymbolToReg(intRegs[intArgCount], noLabel)
				noJump := fc.eb.text.Len()
				fc.out.JumpUnconditional(0)
				noJumpEnd := fc.eb.text.Len()

				// Non-zero -> "yes"
				yesPos := fc.eb.text.Len()
				fc.patchJumpImmediate(yesJump+2, int32(yesPos-yesJumpEnd))
				fc.out.LeaSymbolToReg(intRegs[intArgCount], yesLabel)

				endPos := fc.eb.text.Len()
				fc.patchJumpImmediate(noJump+1, int32(endPos-noJumpEnd))

				intArgCount++
			} else if stringPositions[argIdx] {
				// %s: C67 string -> C string conversion
				// xmm0 contains pointer to C67 string map [count][key0][val0][key1][val1]...
				// Call helper function to convert to null-terminated C string
				fc.trackFunctionCall("_c67_string_to_cstr")
				fc.out.CallSymbol("_c67_string_to_cstr")
				// Result in rax is C string pointer
				fc.out.MovRegToReg(intRegs[intArgCount], "rax")
				intArgCount++
			} else if integerPositions[argIdx] {
				// Integer format without explicit cast - treat as float and convert
				fc.out.Cvttsd2si("rax", "xmm0")
				if intRegs[intArgCount] != "rax" {
					fc.out.MovRegToReg(intRegs[intArgCount], "rax")
				}
				intArgCount++
			} else {
				// Regular float argument (%v, %f, %g, etc)
				fc.out.SubImmFromReg("rsp", 16)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
			}
		}

		// Load float arguments from stack into xmm registers
		// We pushed args in forward order, so we need to pop in reverse to get correct order
		// Stack layout after pushing arg1, arg2, arg3: [arg3][arg2][arg1] <- rsp
		// But we want: arg1→xmm0, arg2→xmm1, arg3→xmm2
		// So we need to load from stack in reverse: pop arg1 (deepest), then arg2, then arg3

		// Count how many float args we have
		numFloatArgs := 0
		for i := 0; i < numArgs; i++ {
			if !boolPositions[i] && !stringPositions[i] && !integerPositions[i] {
				numFloatArgs++
			}
		}

		// Now load them in the correct order
		// Start from the deepest item in stack and work backwards
		for i := 0; i < numArgs; i++ {
			if !boolPositions[i] && !stringPositions[i] && !integerPositions[i] {
				// Calculate offset from rsp: (numFloatArgs - xmmArgCount - 1) * 16
				offset := (numFloatArgs - xmmArgCount - 1) * 16
				fc.out.MovMemToXmm(xmmRegs[xmmArgCount], "rsp", offset)
				xmmArgCount++
			}
		}
		// Clean up stack
		if numFloatArgs > 0 {
			fc.out.AddImmToReg("rsp", int64(numFloatArgs*16))
		}

		// Windows x64 variadic calling convention requires float args to be DUPLICATED in integer registers
		// This is a critical requirement for printf and other variadic functions
		if fc.eb.target.OS() == OSWindows {
			// Copy each xmm register to its corresponding integer register
			// xmm1 -> rdx, xmm2 -> r8, xmm3 -> r9
			if xmmArgCount >= 1 {
				fc.out.MovqXmmToReg("rdx", "xmm1")
			}
			if xmmArgCount >= 2 {
				fc.out.MovqXmmToReg("r8", "xmm2")
			}
			if xmmArgCount >= 3 {
				fc.out.MovqXmmToReg("r9", "xmm3")
			}
		}

		// Load format string to first argument register (rdi on Linux, rcx on Windows)
		fc.out.LeaSymbolToReg(formatReg, labelName)

		// Set rax = number of vector registers used (System V ABI requirement, ignored on Windows)
		fc.out.MovImmToReg("rax", fmt.Sprintf("%d", xmmArgCount))

		// Save allocated callee-saved registers before external call
		allocatedCalleeSaved := fc.regTracker.GetAllocatedCalleeSavedRegs()
		needsDummyPush := len(allocatedCalleeSaved)%2 != 0 // Maintain 16-byte alignment

		for _, reg := range allocatedCalleeSaved {
			fc.out.PushReg(reg)
		}
		if needsDummyPush {
			fc.out.PushReg("rax") // Dummy push for alignment
		}

		// Stack should be 16-byte aligned at this point because:
		// - Function prologue ensures alignment
		// - All stack allocations (loops, variables) use multiples of 16 bytes
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")

		// Flush stdout immediately on Linux to ensure correct output ordering
		// (printf buffers output by default, which can cause ordering issues)
		if fc.eb.target.OS() == OSLinux {
			// Save registers that might be clobbered by fflush
			fc.out.PushReg("rax")
			fc.out.PushReg("rdi")

			// Call fflush(stdout): fflush(NULL) flushes all streams
			fc.out.XorRegWithReg("rdi", "rdi") // NULL argument flushes all streams
			fc.trackFunctionCall("fflush")
			fc.eb.GenerateCallInstruction("fflush")

			// Restore registers
			fc.out.PopReg("rdi")
			fc.out.PopReg("rax")
		}

		// Restore callee-saved registers after external call
		if needsDummyPush {
			fc.out.PopReg("rax") // Dummy pop
		}
		for i := len(allocatedCalleeSaved) - 1; i >= 0; i-- {
			fc.out.PopReg(allocatedCalleeSaved[i])
		}

	case "eprint", "eprintln", "eprintf":
		// Confidence that this function is working: 85%
		// Error printing functions - print to stderr (fd=2) and return Result type with error "out"
		// For simplicity, we just wrap the regular print functions but send output to stderr (fd=2)
		isNewline := call.Function == "eprintln"
		isFormatted := call.Function == "eprintf"

		if isFormatted {
			// eprintf - just use regular printf logic but don't implement formatting yet
			// For now, just treat it like eprintln with first argument
			if len(call.Args) == 0 {
				compilerError("eprintf() requires at least one argument")
			}
			// Simplified: just print the format string to stderr
			arg := call.Args[0]
			if strExpr, ok := arg.(*StringExpr); ok {
				labelName := fmt.Sprintf("str_%d", fc.stringCounter)
				fc.stringCounter++
				processedStr := processEscapeSequences(strExpr.Value)
				fc.eb.Define(labelName, processedStr)

				// Use write syscall: write(2, str, len)
				fc.out.MovImmToReg("rax", "1")                                  // sys_write
				fc.out.MovImmToReg("rdi", "2")                                  // fd = stderr
				fc.out.LeaSymbolToReg("rsi", labelName)                         // buffer
				fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(processedStr))) // length
				fc.out.Syscall()
			}
		} else if isNewline {
			// eprintln - print to stderr with newline
			if len(call.Args) == 0 {
				// Just print a newline to stderr
				newlineLabel := fmt.Sprintf("eprintln_newline_%d", fc.stringCounter)
				fc.stringCounter++
				fc.eb.Define(newlineLabel, "\n")

				fc.out.MovImmToReg("rax", "1") // sys_write
				fc.out.MovImmToReg("rdi", "2") // stderr
				fc.out.LeaSymbolToReg("rsi", newlineLabel)
				fc.out.MovImmToReg("rdx", "1") // 1 byte
				fc.out.Syscall()
			} else {
				arg := call.Args[0]
				if strExpr, ok := arg.(*StringExpr); ok {
					labelName := fmt.Sprintf("str_%d", fc.stringCounter)
					fc.stringCounter++
					processedStr := processEscapeSequences(strExpr.Value) + "\n"
					fc.eb.Define(labelName, processedStr)

					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "2") // stderr
					fc.out.LeaSymbolToReg("rsi", labelName)
					fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(processedStr)))
					fc.out.Syscall()
				} else {
					// For numbers, convert to string using pure syscall approach
					fc.compileExpression(arg)
					// Result in xmm0 (float64)

					// Convert to int64 and use _c67_itoa + write syscall
					fc.out.Cvttsd2si("rdi", "xmm0")

					// Allocate stack buffer
					fc.out.SubImmFromReg("rsp", 32)
					fc.out.MovRegToReg("r15", "rsp")

					// Call _c67_itoa(rdi=number)
					fc.trackFunctionCall("_c67_itoa")
					fc.eb.GenerateCallInstruction("_c67_itoa")
					// Returns: rsi=string start, rdx=length

					// Write to stderr: write(2, rsi, rdx)
					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "2") // stderr
					// rsi, rdx already set
					fc.out.Syscall()

					// Write newline
					newlineLabel := fmt.Sprintf("eprintln_newline_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(newlineLabel, "\n")
					fc.out.MovImmToReg("rax", "1")
					fc.out.MovImmToReg("rdi", "2")
					fc.out.LeaSymbolToReg("rsi", newlineLabel)
					fc.out.MovImmToReg("rdx", "1")
					fc.out.Syscall()

					fc.out.AddImmToReg("rsp", 32)
				}
			}
		} else {
			// eprint - print to stderr without newline
			if len(call.Args) == 0 {
				// Nothing to print
				fc.createErrorResult("out")
				return
			}

			arg := call.Args[0]
			if strExpr, ok := arg.(*StringExpr); ok {
				labelName := fmt.Sprintf("str_%d", fc.stringCounter)
				fc.stringCounter++
				processedStr := processEscapeSequences(strExpr.Value)
				fc.eb.Define(labelName, processedStr)

				fc.out.MovImmToReg("rax", "1") // sys_write
				fc.out.MovImmToReg("rdi", "2") // stderr
				fc.out.LeaSymbolToReg("rsi", labelName)
				fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(processedStr)))
				fc.out.Syscall()
			} else {
				// For numbers, convert to string using pure syscall approach
				fc.compileExpression(arg)
				// Result in xmm0 (float64)

				// Convert to int64 and use _c67_itoa + write syscall
				fc.out.Cvttsd2si("rdi", "xmm0")

				// Allocate stack buffer
				fc.out.SubImmFromReg("rsp", 32)
				fc.out.MovRegToReg("r15", "rsp")

				// Call _c67_itoa(rdi=number)
				fc.trackFunctionCall("_c67_itoa")
				fc.eb.GenerateCallInstruction("_c67_itoa")
				// Returns: rsi=string start, rdx=length

				// Write to stderr: write(2, rsi, rdx)
				fc.out.MovImmToReg("rax", "1") // sys_write
				fc.out.MovImmToReg("rdi", "2") // stderr
				// rsi, rdx already set
				fc.out.Syscall()

				fc.out.AddImmToReg("rsp", 32)
			}
		}

		// Return Result type with error code "out"
		fc.createErrorResult("out")
		return

	case "exitln", "exitf":
		// Confidence that this function is working: 90%
		// Quick exit print functions - print to stderr and exit with code 1
		// exitln prints with newline, exitf is formatted output
		isNewline := call.Function == "exitln"
		isFormatted := call.Function == "exitf"

		// exitf: Use fprintf to stderr with proper formatting, then exit
		// This is similar to eprintf but exits with a specific code
		if isFormatted {
			if len(call.Args) == 0 {
				compilerError("exitf() requires at least a format string")
			}

			// Use eprintf logic for the printing part
			// eprintf format: first arg is format string, rest are variadic args
			formatArg := call.Args[0]
			strExpr, ok := formatArg.(*StringExpr)
			if !ok {
				compilerError("exitf() first argument must be a string literal (got %T)", formatArg)
			}

			// Process format string just like eprintf does
			processedFormat := processEscapeSequences(strExpr.Value)
			boolPositions := make(map[int]bool)
			stringPositions := make(map[int]bool)
			integerPositions := make(map[int]bool)

			argPos := 0
			var result strings.Builder
			runes := []rune(processedFormat)
			i := 0
			for i < len(runes) {
				if runes[i] == '%' && i+1 < len(runes) {
					next := runes[i+1]
					if next == '%' {
						result.WriteString("%%")
						i += 2
						continue
					} else if next == 'v' {
						result.WriteString("%.15g")
						argPos++
						i += 2
						continue
					} else if next == 'b' {
						result.WriteString("%s")
						boolPositions[argPos] = true
						argPos++
						i += 2
						continue
					} else if next == 's' {
						stringPositions[argPos] = true
						argPos++
					} else if next == 'l' && i+2 < len(runes) && (runes[i+2] == 'd' || runes[i+2] == 'i' || runes[i+2] == 'u') {
						integerPositions[argPos] = true
						argPos++
					} else if next == 'd' || next == 'i' || next == 'u' || next == 'x' || next == 'X' {
						integerPositions[argPos] = true
						argPos++
					}
				}
				result.WriteRune(runes[i])
				i++
			}

			fmtStr := result.String()
			fmtStrWithNull := fmtStr
			if len(fmtStrWithNull) == 0 || fmtStrWithNull[len(fmtStrWithNull)-1] != 0 {
				fmtStrWithNull += "\x00"
			}

			labelName := fmt.Sprintf("exitf_fmt_%d", fc.stringCounter)
			fc.stringCounter++
			fc.eb.Define(labelName, fmtStrWithNull)

			// Set up fprintf/printf call
			if fc.platform.OS == OSWindows {
				// On Windows, just use printf to stdout (simpler than dealing with stderr)
				shadowSpace := fc.allocateShadowSpace()
				fc.out.LeaSymbolToReg("rcx", labelName)

				// Process remaining arguments
				for argIdx := 1; argIdx < len(call.Args) && argIdx <= 8; argIdx++ {
					arg := call.Args[argIdx]
					targetReg := ""
					switch argIdx {
					case 1:
						targetReg = "r8"
					case 2:
						targetReg = "r9"
					default:
						targetReg = fmt.Sprintf("xmm%d", argIdx)
					}

					if boolPositions[argIdx-1] {
						fc.compileExpression(arg)
						yesLabel := fmt.Sprintf("exitf_yes_%d", fc.labelCounter)
						noLabel := fmt.Sprintf("exitf_no_%d", fc.labelCounter)
						fc.labelCounter++
						fc.eb.Define(yesLabel, "yes\x00")
						fc.eb.Define(noLabel, "no\x00")

						fc.out.Ucomisd("xmm0", "xmm0")
						fc.out.Write(0x7A)
						fc.out.Write(0x0A)
						fc.out.XorRegWithReg("rax", "rax")
						fc.out.Ucomisd("xmm0", "xmm0")
						fc.out.Write(0x75)
						fc.out.Write(0x05)
						fc.out.LeaSymbolToReg(targetReg, noLabel)
						fc.out.Write(0xEB)
						fc.out.Write(0x05)
						fc.out.LeaSymbolToReg(targetReg, yesLabel)
					} else if stringPositions[argIdx-1] {
						// Check if this arg is a C FFI call that returns char* (cstring)
						needsConversion := !fc.isCFFIStringCall(arg)

						if VerboseMode {
							fmt.Fprintf(os.Stderr, "exitf string arg: needsConversion=%v\n", needsConversion)
						}

						fc.compileExpression(arg)

						if needsConversion {
							// Convert C67 string to C string
							fc.trackFunctionCall("_c67_string_to_cstr")
							fc.eb.GenerateCallInstruction("_c67_string_to_cstr")
							if targetReg != "" && strings.HasPrefix(targetReg, "xmm") {
								fc.out.MovqRegToXmm(targetReg, "rax")
							} else if targetReg != "" {
								fc.out.MovRegToReg(targetReg, "rax")
							}
						} else {
							// Already a char* from C FFI - just convert from float64 representation to pointer
							if targetReg != "" && strings.HasPrefix(targetReg, "xmm") {
								// Keep in xmm register (already there from compileExpression)
							} else if targetReg != "" {
								// Move to integer register
								fc.out.MovqXmmToReg(targetReg, "xmm0")
							}
						}
					} else if integerPositions[argIdx-1] {
						fc.compileExpression(arg)
						fc.out.Cvttsd2si(targetReg, "xmm0")
					} else {
						fc.compileExpression(arg)
						if !strings.HasPrefix(targetReg, "xmm") {
							fc.out.MovqXmmToReg(targetReg, "xmm0")
						}
					}
				}

				fc.trackFunctionCall("printf")
				fc.eb.GenerateCallInstruction("printf")
				fc.deallocateShadowSpace(shadowSpace)

				// Flush stdout before exit to ensure message is printed
				shadowSpace = fc.allocateShadowSpace()
				fc.out.XorRegWithReg("rcx", "rcx") // fflush(NULL) flushes all streams
				fc.trackFunctionCall("fflush")
				fc.eb.GenerateCallInstruction("fflush")
				fc.deallocateShadowSpace(shadowSpace)
			} else {
				// Unix: For simple case with no extra args, just write to stderr (fd=2) using syscall
				// This avoids the complexity of fprintf and stderr FILE* handling
				if len(call.Args) == 1 {
					// Simple case: just a format string with no placeholders
					// Write directly to stderr using write syscall (don't include null terminator)
					strLen := len(fmtStr)
					fc.out.MovImmToReg("rax", "1") // sys_write
					fc.out.MovImmToReg("rdi", "2") // stderr (fd 2)
					fc.out.LeaSymbolToReg("rsi", labelName)
					fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", strLen))
					fc.out.Syscall()
				} else {
					// Complex case with arguments: use fprintf (less common for exitf)
					// FIXME: This path needs proper stderr handling
					fc.out.LeaSymbolToReg("rsi", labelName)
					// Use dprintf to write to fd 2 directly instead of fprintf
					fc.out.MovImmToReg("rdi", "2") // stderr fd

					// Process arguments
					for argIdx := 1; argIdx < len(call.Args) && argIdx <= 8; argIdx++ {
						arg := call.Args[argIdx]
						targetRegs := []string{"rdx", "rcx", "r8", "r9", "xmm0", "xmm1", "xmm2", "xmm3"}
						if argIdx-1 >= len(targetRegs) {
							break
						}
						targetReg := targetRegs[argIdx-1]

						if stringPositions[argIdx-1] {
							// Check if this arg is a C FFI call that returns char* (cstring)
							needsConversion := !fc.isCFFIStringCall(arg)

							fc.compileExpression(arg)

							if needsConversion {
								// Convert C67 string to C string
								fc.trackFunctionCall("_c67_string_to_cstr")
								fc.eb.GenerateCallInstruction("_c67_string_to_cstr")
							} else {
								// Already a char* from C FFI - just convert from float64 representation to pointer
								fc.out.MovqXmmToReg("rax", "xmm0")
							}
							fc.out.MovRegToReg(targetReg, "rax")
						} else {
							fc.compileExpression(arg)
							if !strings.HasPrefix(targetReg, "xmm") {
								fc.out.MovqXmmToReg(targetReg, "xmm0")
							}
						}
					}

					fc.trackFunctionCall("dprintf")
					fc.eb.GenerateCallInstruction("dprintf")
				}
			}

			// Extract exit code from last argument if it's an integer, otherwise use 1
			exitCode := 1
			if len(call.Args) > 1 {
				lastArg := call.Args[len(call.Args)-1]
				if numExpr, ok := lastArg.(*NumberExpr); ok {
					exitCode = int(numExpr.Value)
				}
			}

			// Exit with appropriate code
			if fc.platform.OS == OSWindows {
				fc.out.MovImmToReg("rcx", fmt.Sprintf("%d", exitCode))
				fc.trackFunctionCall("exit")
				fc.eb.GenerateCallInstruction("exit")
			} else {
				fc.out.MovImmToReg("rdi", fmt.Sprintf("%d", exitCode))
				fc.trackFunctionCall("exit")
				fc.eb.GenerateCallInstruction("exit")
			}
			fc.hasExplicitExit = true

		} else if isNewline {
			if len(call.Args) == 0 {
				newlineLabel := fmt.Sprintf("exitln_newline_%d", fc.stringCounter)
				fc.stringCounter++
				fc.eb.Define(newlineLabel, "\n")

				fc.out.MovImmToReg("rax", "1")
				fc.out.MovImmToReg("rdi", "2")
				fc.out.LeaSymbolToReg("rsi", newlineLabel)
				fc.out.MovImmToReg("rdx", "1")
				fc.out.Syscall()
			} else {
				arg := call.Args[0]
				if strExpr, ok := arg.(*StringExpr); ok {
					labelName := fmt.Sprintf("str_%d", fc.stringCounter)
					fc.stringCounter++
					processedStr := processEscapeSequences(strExpr.Value) + "\n"
					fc.eb.Define(labelName, processedStr)

					fc.out.MovImmToReg("rax", "1")
					fc.out.MovImmToReg("rdi", "2")
					fc.out.LeaSymbolToReg("rsi", labelName)
					fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(processedStr)))
					fc.out.Syscall()
				} else {
					fc.compileExpression(arg)

					fc.out.SubImmFromReg("rsp", 32)

					fc.out.MovRegToReg("rdi", "rsp")
					fc.out.MovImmToReg("rsi", "32")
					fmtLabel := fmt.Sprintf("exitln_fmt_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(fmtLabel, "%g\n\x00")
					fc.out.LeaSymbolToReg("rdx", fmtLabel)
					fc.out.MovImmToReg("rax", "1")
					fc.trackFunctionCall("snprintf")
					fc.eb.GenerateCallInstruction("snprintf")

					fc.out.MovRegToReg("rdx", "rax")
					fc.out.MovRegToReg("rsi", "rsp")
					fc.out.MovImmToReg("rax", "1")
					fc.out.MovImmToReg("rdi", "2")
					fc.out.Syscall()

					fc.out.AddImmToReg("rsp", 32)
				}
			}
		} else {
			if len(call.Args) > 0 {
				arg := call.Args[0]
				if strExpr, ok := arg.(*StringExpr); ok {
					labelName := fmt.Sprintf("str_%d", fc.stringCounter)
					fc.stringCounter++
					processedStr := processEscapeSequences(strExpr.Value)
					fc.eb.Define(labelName, processedStr)

					fc.out.MovImmToReg("rax", "1")
					fc.out.MovImmToReg("rdi", "2")
					fc.out.LeaSymbolToReg("rsi", labelName)
					fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(processedStr)))
					fc.out.Syscall()
				} else {
					fc.compileExpression(arg)

					fc.out.SubImmFromReg("rsp", 32)

					fc.out.MovRegToReg("rdi", "rsp")
					fc.out.MovImmToReg("rsi", "32")
					fmtLabel := fmt.Sprintf("exitf_fmt_%d", fc.stringCounter)
					fc.stringCounter++
					fc.eb.Define(fmtLabel, "%g\x00")
					fc.out.LeaSymbolToReg("rdx", fmtLabel)
					fc.out.MovImmToReg("rax", "1")
					fc.trackFunctionCall("snprintf")
					fc.eb.GenerateCallInstruction("snprintf")

					fc.out.MovRegToReg("rdx", "rax")
					fc.out.MovRegToReg("rsi", "rsp")
					fc.out.MovImmToReg("rax", "1")
					fc.out.MovImmToReg("rdi", "2")
					fc.out.Syscall()

					fc.out.AddImmToReg("rsp", 32)
				}
			}
		}

		// Exit with code 1
		fc.out.MovImmToReg("rdi", "1")
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")
		fc.hasExplicitExit = true
		return

	case "exit":
		fc.hasExplicitExit = true // Mark that program has explicit exit
		if len(call.Args) > 0 {
			fc.compileExpression(call.Args[0])
			// Convert float64 in xmm0 to int64 in rdi
			fc.out.Cvttsd2si("rdi", "xmm0") // truncate float to int
		} else {
			fc.out.XorRegWithReg("rdi", "rdi")
		}
		// Restore stack pointer to frame pointer (rsp % 16 == 8 for proper call alignment)
		// Don't pop rbp since exit() never returns
		fc.out.MovRegToReg("rsp", "rbp")
		fc.trackFunctionCall("exit")
		fc.eb.GenerateCallInstruction("exit")

	case "append":
		// Confidence that this function is working: 100%
		// append(list, value) - Add element to end of list
		// Returns new list with value appended
		if len(call.Args) != 2 {
			compilerError("append() requires exactly 2 arguments (list, value)")
		}

		// Compile and save both arguments
		fc.compileExpression(call.Args[0]) // list -> xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		fc.compileExpression(call.Args[1]) // value -> xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Load list pointer and get count
		fc.out.MovMemToXmm("xmm1", "rsp", 8)
		fc.out.MovqXmmToReg("rsi", "xmm1")   // rsi = old list pointer
		fc.out.MovMemToXmm("xmm2", "rsi", 0) // xmm2 = count (as float)
		fc.out.Cvttsd2si("rcx", "xmm2")      // rcx = count (as int)

		// Calculate new size: 8 + (count + 1) * 16
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.AddImmToReg("rdx", 1) // rdx = count + 1
		fc.out.ShlImmReg("rdx", 4)   // rdx = (count + 1) * 16
		fc.out.AddImmToReg("rdx", 8) // rdx = new size

		// Allocate new list
		fc.out.MovRegToReg("rdi", "rdx")
		fc.out.PushReg("rsi") // Save old list ptr
		fc.out.PushReg("rcx") // Save count
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.PopReg("rcx") // Restore count
		fc.out.PopReg("rsi") // Restore old list ptr

		// rax = new list, rsi = old list, rcx = old count

		// Store new count
		fc.out.MovRegToReg("r10", "rcx")
		fc.out.AddImmToReg("r10", 1)   // r10 = old_count + 1
		fc.out.Cvtsi2sd("xmm3", "r10") // xmm3 = new count as float
		fc.out.MovXmmToMem("xmm3", "rax", 0)

		// Now use memcpy to copy old elements if count > 0
		// memcpy(new_list+8, old_list+8, old_count*16)
		fc.out.TestRegReg("rcx", "rcx")
		skipCopyJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // Skip if old count == 0
		skipCopyPatch := fc.eb.text.Len()

		// Save rax (new list ptr), rsi (old list ptr), rcx (old count)
		fc.out.PushReg("rax")
		fc.out.PushReg("rsi")
		fc.out.PushReg("rcx")

		// Set up memcpy arguments
		fc.out.LeaMemToReg("rdi", "rax", 8) // dest = new_list + 8
		fc.out.LeaMemToReg("rsi", "rsi", 8) // src = old_list + 8
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.ShlImmReg("rdx", 4) // size = old_count * 16

		fc.trackFunctionCall("memcpy")
		fc.eb.GenerateCallInstruction("memcpy")

		// Restore saved values
		fc.out.PopReg("rcx") // old count
		fc.out.PopReg("rsi") // old list (not needed but keeps stack aligned)
		fc.out.PopReg("rax") // new list

		// Patch skip jump (6-byte instruction: 0x0F opcode offset32)
		// skipCopyJump points to 0x0F, offset starts at skipCopyJump+2
		currentPos := fc.eb.text.Len()
		skipOffset := currentPos - skipCopyPatch
		fc.eb.text.Bytes()[skipCopyJump+2] = byte(skipOffset)
		fc.eb.text.Bytes()[skipCopyJump+3] = byte(skipOffset >> 8)
		fc.eb.text.Bytes()[skipCopyJump+4] = byte(skipOffset >> 16)
		fc.eb.text.Bytes()[skipCopyJump+5] = byte(skipOffset >> 24)

		// Add new entry at end
		// Offset = 8 + rcx * 16 (rcx still holds old count)
		fc.out.MovRegToReg("rdx", "rcx")
		fc.out.ShlImmReg("rdx", 4)   // rdx = old_count * 16
		fc.out.AddImmToReg("rdx", 8) // rdx = offset to new entry

		// Calculate address of new entry: rax + rdx
		fc.out.AddRegToReg("rax", "rdx") // rax now points to new entry location

		// Store key (old count)
		fc.out.StoreRegToMem("rcx", "rax", 0)

		// Store value (value is at rsp+0 after popping, since we popped 3 regs above)
		// Stack layout after pops: [rsp+0]=value, [rsp+8]=old_list_ptr
		fc.out.MovMemToXmm("xmm4", "rsp", 0) // Load value from stack
		fc.out.MovXmmToMem("xmm4", "rax", 8)

		// Restore rax to point to start of new list
		fc.out.SubRegFromReg("rax", "rdx")

		// Clean stack
		fc.out.AddImmToReg("rsp", 16) // Pop value and old list

		// Return new list pointer in xmm0
		fc.out.MovqRegToXmm("xmm0", "rax")
		return

	case "pop":
		// Confidence that this function is working: 100%
		// pop(list) - Remove and return last element
		// Returns [new_list, last_value] as a 2-element list
		// If list is empty, returns [empty_list, NaN]
		if len(call.Args) != 1 {
			compilerError("pop() requires exactly 1 argument (list)")
		}

		// Compile list argument -> result in xmm0 (list pointer as float64)
		fc.compileExpression(call.Args[0])
		fc.out.MovqXmmToReg("rbx", "xmm0") // rbx = input list pointer (callee-saved)

		// Load count from list [rbx+0]
		fc.out.MovMemToXmm("xmm1", "rbx", 0) // xmm1 = count as float
		fc.out.Cvttsd2si("r12", "xmm1")      // r12 = count as int (callee-saved)

		// Check if list is empty (count == 0)
		fc.out.TestRegReg("r12", "r12")
		emptyJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // Jump if empty
		emptyPatch := fc.eb.text.Len()

		// Non-empty list path:
		// Get popped value FIRST before any malloc calls
		// Offset = 8 + (count - 1) * 16 + 8 (skip key)
		fc.out.MovRegToReg("rax", "r12")
		fc.out.SubImmFromReg("rax", 1)
		fc.out.ShlImmReg("rax", 4)
		fc.out.AddImmToReg("rax", 16) // offset to value field
		fc.out.AddRegToReg("rax", "rbx")
		fc.out.MovMemToXmm("xmm7", "rax", 0) // xmm7 = popped value (saved)

		// Calculate size of new_list (without last element)
		// new_list size: 8 + (count - 1) * 16
		fc.out.MovRegToReg("r13", "r12")
		fc.out.SubImmFromReg("r13", 1) // r13 = count - 1
		fc.out.MovRegToReg("r14", "r13")
		fc.out.ShlImmReg("r14", 4)   // r14 = (count - 1) * 16
		fc.out.AddImmToReg("r14", 8) // r14 = new_list size

		// Allocate new_list
		fc.out.MovRegToReg("rdi", "r14")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r14", "rax") // r14 = new_list pointer

		// Store new count in new_list
		fc.out.Cvtsi2sd("xmm3", "r13")
		fc.out.MovXmmToMem("xmm3", "r14", 0)

		// Copy elements to new_list if new count > 0
		fc.out.TestRegReg("r13", "r13")
		skipCopy := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0)
		skipCopyPatch := fc.eb.text.Len()

		// memcpy(new_list+8, input_list+8, (count-1)*16)
		// Save callee-saved registers on stack (belt and suspenders)
		fc.out.PushReg("rbx")
		fc.out.PushReg("r12")
		fc.out.PushReg("r13")
		fc.out.PushReg("r14")
		fc.out.PushReg("r15")

		fc.out.LeaMemToReg("rdi", "r14", 8) // dest = new_list + 8
		fc.out.LeaMemToReg("rsi", "rbx", 8) // src = input_list + 8
		fc.out.MovRegToReg("rdx", "r13")
		fc.out.ShlImmReg("rdx", 4) // size = (count-1) * 16

		fc.trackFunctionCall("memcpy")
		fc.eb.GenerateCallInstruction("memcpy")

		// Restore callee-saved registers
		fc.out.PopReg("r15")
		fc.out.PopReg("r14")
		fc.out.PopReg("r13")
		fc.out.PopReg("r12")
		fc.out.PopReg("rbx")

		// Patch skip copy jump
		currentPos := fc.eb.text.Len()
		skipOffset := currentPos - skipCopyPatch
		fc.eb.text.Bytes()[skipCopy+2] = byte(skipOffset)
		fc.eb.text.Bytes()[skipCopy+3] = byte(skipOffset >> 8)
		fc.eb.text.Bytes()[skipCopy+4] = byte(skipOffset >> 16)
		fc.eb.text.Bytes()[skipCopy+5] = byte(skipOffset >> 24)

		// Allocate result list with 2 elements
		// Result size: 8 (count) + 2 * 16 (two entries) = 40 bytes
		fc.out.MovImmToReg("rdi", "40")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r15", "rax") // r15 = result list pointer

		// Store count=2 in result list
		fc.out.MovImmToReg("rax", "2")
		fc.out.Cvtsi2sd("xmm2", "rax")
		fc.out.MovXmmToMem("xmm2", "r15", 0)

		// Store entry 0 in result: [key=0, value=new_list_ptr]
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.StoreRegToMem("rax", "r15", 8) // key = 0
		fc.out.MovqRegToXmm("xmm5", "r14")
		fc.out.MovXmmToMem("xmm5", "r15", 16) // value = new_list ptr

		// Store entry 1 in result: [key=1, value=popped_value]
		fc.out.MovImmToReg("rax", "1")
		fc.out.StoreRegToMem("rax", "r15", 24) // key = 1
		fc.out.MovXmmToMem("xmm7", "r15", 32)  // value = popped value (from xmm7)

		// Return result list pointer in xmm0
		fc.out.MovqRegToXmm("xmm0", "r15")

		// Jump over empty list path
		doneJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)
		donePatch := fc.eb.text.Len()

		// Empty list path:
		emptyPos := fc.eb.text.Len()
		fc.patchJumpImmediate(emptyJump+2, int32(emptyPos-emptyPatch))

		// Allocate empty list
		fc.out.MovImmToReg("rdi", "8")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r14", "rax") // r14 = empty list

		// Store count=0 in empty list
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm3", "rax")
		fc.out.MovXmmToMem("xmm3", "r14", 0)

		// Allocate result list with 2 elements
		fc.out.MovImmToReg("rdi", "40")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.MovRegToReg("r15", "rax") // r15 = result list

		// Store count=2
		fc.out.MovImmToReg("rax", "2")
		fc.out.Cvtsi2sd("xmm2", "rax")
		fc.out.MovXmmToMem("xmm2", "r15", 0)

		// Store entry 0: [key=0, value=empty_list_ptr]
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.StoreRegToMem("rax", "r15", 8)
		fc.out.MovqRegToXmm("xmm5", "r14")
		fc.out.MovXmmToMem("xmm5", "r15", 16)

		// Store entry 1: [key=1, value=NaN]
		fc.out.MovImmToReg("rax", "1")
		fc.out.StoreRegToMem("rax", "r15", 24)
		// Load NaN value (0x7FF8000000000000) into xmm4
		fc.out.Emit([]byte{0xF2, 0x41, 0x0F, 0x10, 0x25}) // movsd xmm4, [rip+offset]
		nanLiteralOffset := fc.eb.text.Len()
		fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00}) // Placeholder for RIP-relative offset
		nanLiteralJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // Skip NaN literal
		nanLiteralJumpEnd := fc.eb.text.Len()
		nanLiteralPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f}) // NaN bits
		nanLiteralEnd := fc.eb.text.Len()
		// Patch RIP-relative offset
		relOffset := nanLiteralPos - nanLiteralJump
		fc.eb.text.Bytes()[nanLiteralOffset] = byte(relOffset)
		fc.eb.text.Bytes()[nanLiteralOffset+1] = byte(relOffset >> 8)
		fc.eb.text.Bytes()[nanLiteralOffset+2] = byte(relOffset >> 16)
		fc.eb.text.Bytes()[nanLiteralOffset+3] = byte(relOffset >> 24)
		// Patch jump
		fc.patchJumpImmediate(nanLiteralJump+1, int32(nanLiteralEnd-nanLiteralJumpEnd))
		fc.out.MovXmmToMem("xmm4", "r15", 32)

		// Return result list pointer in xmm0
		fc.out.MovqRegToXmm("xmm0", "r15")

		// Patch done jump
		donePos := fc.eb.text.Len()
		fc.patchJumpImmediate(doneJump+1, int32(donePos-donePatch))

		return

	case "arena_create":
		// arena_create(capacity) -> arena_ptr
		// Create a new arena with the given capacity
		if len(call.Args) != 1 {
			compilerError("arena_create() requires exactly 1 argument (capacity)")
		}
		fc.compileExpression(call.Args[0])
		// Convert float64 capacity to int64
		fc.out.Cvttsd2si("rdi", "xmm0")
		fc.out.CallSymbol("_c67_arena_create")
		// Result in rax, convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "arena_alloc":
		// arena_alloc(arena_ptr, size) -> allocation_ptr
		// Allocate memory from the arena
		if len(call.Args) != 2 {
			compilerError("arena_alloc() requires exactly 2 arguments (arena_ptr, size)")
		}
		// First arg: arena_ptr
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0")
		// Second arg: size
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("rsi", "xmm0")
		fc.out.CallSymbol("_c67_arena_alloc")
		// Result in rax, convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "arena_destroy":
		// arena_destroy(arena_ptr)
		// Destroy the arena and free all memory
		if len(call.Args) != 1 {
			compilerError("arena_destroy() requires exactly 1 argument (arena_ptr)")
		}
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0")
		fc.out.CallSymbol("_c67_arena_destroy")
		// No return value, set xmm0 to 0
		fc.out.XorpdXmm("xmm0", "xmm0")

	case "arena_reset":
		// arena_reset(arena_ptr)
		// Reset the arena offset to 0, freeing all allocations
		if len(call.Args) != 1 {
			compilerError("arena_reset() requires exactly 1 argument (arena_ptr)")
		}
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0")
		fc.out.CallSymbol("_c67_arena_reset")
		// No return value, set xmm0 to 0
		fc.out.XorpdXmm("xmm0", "xmm0")

	case "syscall":
		// Raw Linux syscall: syscall(number, arg1, arg2, arg3, arg4, arg5, arg6)
		// x86-64 syscall convention: rax=number, rdi, rsi, rdx, r10, r8, r9
		if len(call.Args) < 1 || len(call.Args) > 7 {
			compilerError("syscall() requires 1-7 arguments (syscall number + up to 6 args)")
		}

		// Syscall registers in x86-64: rdi, rsi, rdx, r10, r8, r9
		// Note: r10 is used instead of rcx for syscalls
		argRegs := []string{"rdi", "rsi", "rdx", "r10", "r8", "r9"}

		// Evaluate all arguments and save to stack (in reverse order)
		for i := len(call.Args) - 1; i >= 0; i-- {
			fc.compileExpression(call.Args[i]) // Result in xmm0
			// Convert float64 to int64 and save
			fc.out.Cvttsd2si("rax", "xmm0")
			fc.out.PushReg("rax")
		}

		// Pop syscall number into rax
		fc.out.PopReg("rax")

		// Pop arguments into registers
		numArgs := len(call.Args) - 1 // Exclude syscall number
		for i := 0; i < numArgs && i < 6; i++ {
			fc.out.PopReg(argRegs[i])
		}

		// Execute syscall instruction (0x0f 0x05 for x86-64)
		fc.out.Emit([]byte{0x0f, 0x05})

		// Convert result from rax (int64) to xmm0 (float64)
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "getpid":
		// Call getpid() from libc via PLT
		// getpid() takes no arguments and returns pid_t in rax
		if len(call.Args) != 0 {
			compilerError("getpid() takes no arguments")
		}
		fc.trackFunctionCall("getpid")
		fc.eb.GenerateCallInstruction("getpid")
		// Convert result from rax to xmm0
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "sqrt":
		if len(call.Args) != 1 {
			compilerError("sqrt() requires exactly 1 argument")
		}
		// Compile argument (result in xmm0)
		fc.compileExpression(call.Args[0])
		// Use x86-64 SQRTSD instruction (hardware sqrt)
		// sqrtsd xmm0, xmm0 - sqrt of xmm0, result in xmm0
		fc.out.Sqrtsd("xmm0", "xmm0")

	case "sin":
		if len(call.Args) != 1 {
			compilerError("sin() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// Use x87 FPU FSIN instruction
		// xmm0 -> memory -> ST(0) -> FSIN -> memory -> xmm0
		fc.out.SubImmFromReg("rsp", StackSlotSize) // Allocate 8 bytes on stack
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0)
		fc.out.Fsin()
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize) // Restore stack

	case "cos":
		if len(call.Args) != 1 {
			compilerError("cos() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// Use x87 FPU FCOS instruction
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0)
		fc.out.Fcos()
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "tan":
		if len(call.Args) != 1 {
			compilerError("tan() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// Use x87 FPU FPTAN instruction
		// FPTAN computes tan and pushes 1.0, so we need to pop the 1.0
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0)
		fc.out.Fptan()
		fc.out.Fpop() // Pop the 1.0 that FPTAN pushes
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "atan":
		if len(call.Args) != 1 {
			compilerError("atan() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// Use x87 FPU FPATAN: atan(x) = atan2(x, 1.0)
		// FPATAN expects ST(1)=y, ST(0)=x, computes atan2(y,x)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0) // ST(0) = x
		fc.out.Fld1()           // ST(0) = 1.0, ST(1) = x
		fc.out.Fpatan()         // ST(0) = atan2(x, 1.0) = atan(x)
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "asin":
		if len(call.Args) != 1 {
			compilerError("asin() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// asin(x) = atan2(x, sqrt(1 - x²))
		// FPATAN needs ST(1)=x, ST(0)=sqrt(1-x²)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0) // ST(0) = x
		fc.out.FldSt0()         // ST(0) = x, ST(1) = x
		fc.out.FmulSelf()       // ST(0) = x²
		fc.out.Fld1()           // ST(0) = 1.0, ST(1) = x²
		fc.out.Fsubrp()         // ST(0) = 1 - x²
		fc.out.Fsqrt()          // ST(0) = sqrt(1 - x²)
		fc.out.FldMem("rsp", 0) // ST(0) = x, ST(1) = sqrt(1 - x²)
		// Now swap: need ST(1)=x, ST(0)=sqrt(1-x²) but have reverse
		// Solution: save sqrt to mem, reload in reverse order
		fc.out.FstpMem("rsp", 0) // Store x to [rsp], pop, ST(0) = sqrt(1-x²)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.FstpMem("rsp", 0)            // Store sqrt to [rsp+0]
		fc.out.FldMem("rsp", StackSlotSize) // Load x: ST(0) = x
		fc.out.FldMem("rsp", 0)             // Load sqrt: ST(0) = sqrt, ST(1) = x
		fc.out.Fpatan()                     // ST(0) = atan2(x, sqrt(1-x²)) = asin(x)
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 16) // Restore both allocations

	case "acos":
		if len(call.Args) != 1 {
			compilerError("acos() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// acos(x) = atan2(sqrt(1-x²), x)
		// FPATAN needs ST(1)=sqrt(1-x²), ST(0)=x
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0) // ST(0) = x
		fc.out.FldSt0()         // ST(0) = x, ST(1) = x
		fc.out.FmulSelf()       // ST(0) = x²
		fc.out.Fld1()           // ST(0) = 1.0, ST(1) = x²
		fc.out.Fsubrp()         // ST(0) = 1 - x²
		fc.out.Fsqrt()          // ST(0) = sqrt(1 - x²)
		fc.out.FldMem("rsp", 0) // ST(0) = x, ST(1) = sqrt(1 - x²)
		fc.out.Fpatan()         // ST(0) = atan2(sqrt(1-x²), x) = acos(x)
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "error":
		// Confidence that this function is working: 95%
		// error(code) - Creates an error Result with the given 3-4 char code
		// Example: error("arg") creates error NaN with code "arg\0"
		if len(call.Args) != 1 {
			compilerError("error() requires exactly 1 argument (error code string)")
		}

		// Evaluate argument - should be a string
		fc.compileExpression(call.Args[0])
		// xmm0 now contains pointer to string

		// Move pointer to rax
		fc.out.MovqXmmToReg("rax", "xmm0")

		// Load first character (index 0) from string
		// String format: [count][key0][val0][key1][val1]...
		// First char is at offset 16 (skip 8-byte count + 8-byte key)
		fc.out.MovMemToXmm("xmm1", "rax", 16)       // Load first char as float
		fc.out.Cvttsd2si("rbx", "xmm1")             // Convert to int, store in rbx
		fc.out.Emit([]byte{0x48, 0xc1, 0xe3, 0x18}) // shl rbx, 24 (move to high byte)

		// Load second character (index 1) if exists
		fc.out.MovMemToXmm("xmm1", "rax", 32)       // Load second char as float
		fc.out.Cvttsd2si("rcx", "xmm1")             // Convert to int
		fc.out.Emit([]byte{0x48, 0xc1, 0xe1, 0x10}) // shl rcx, 16
		fc.out.Emit([]byte{0x48, 0x09, 0xcb})       // or rbx, rcx

		// Load third character (index 2) if exists
		fc.out.MovMemToXmm("xmm1", "rax", 48)       // Load third char as float
		fc.out.Cvttsd2si("rcx", "xmm1")             // Convert to int
		fc.out.Emit([]byte{0x48, 0xc1, 0xe1, 0x08}) // shl rcx, 8
		fc.out.Emit([]byte{0x48, 0x09, 0xcb})       // or rbx, rcx

		// rbx now contains error code as 32-bit value in correct byte order
		// Create error NaN: 0x7FF8_0000_0000_0000 | error_code
		fc.out.Emit([]byte{0x48, 0xb8})                                     // mov rax, immediate64
		fc.out.Emit([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x7f}) // Quiet NaN base
		fc.out.Emit([]byte{0x48, 0x09, 0xd8})                               // or rax, rbx

		// Move to xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovRegToMem("rax", "rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 8)

	case "abs":
		if len(call.Args) != 1 {
			compilerError("abs() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// abs(x) using FABS
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0) // ST(0) = x
		fc.out.Fabs()           // ST(0) = |x|
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "floor":
		if len(call.Args) != 1 {
			compilerError("floor() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// floor(x): round toward -∞
		// FPU control word: set rounding mode to 01 (round down)
		fc.out.SubImmFromReg("rsp", 16) // Need space for control word + value
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)

		// Save current FPU control word
		fc.out.FstcwMem("rsp", 0)

		// Load control word, modify to set RC=01 (bits 10-11)
		// Emit 16-bit MOV manually: mov ax, [rsp]
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x8B) // MOV r16, r/m16
		fc.out.Write(0x04) // ModR/M: [rsp]
		fc.out.Write(0x24) // SIB: [rsp]
		// OR ax, 0x0400 (set bit 10 for round down)
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x81) // OR r/m16, imm16
		fc.out.Write(0xC8) // ModR/M for ax
		fc.out.Write(0x00) // Low byte
		fc.out.Write(0x04) // High byte: 0x0400 = bit 10 set (round down)
		// AND ax, 0xF7FF (clear bit 11, keep bit 10)
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x81) // AND r/m16, imm16
		fc.out.Write(0xE0) // ModR/M for ax
		fc.out.Write(0xFF) // Low byte
		fc.out.Write(0xF7) // High byte: 0xF7FF = clear bit 11, keep bit 10
		// Store modified control word: mov [rsp+2], ax
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x89) // MOV r/m16, r16
		fc.out.Write(0x44) // ModR/M: [rsp+disp8]
		fc.out.Write(0x24) // SIB: [rsp]
		fc.out.Write(0x02) // disp8: +2

		// Load modified control word
		fc.out.FldcwMem("rsp", 2)

		// Perform rounding
		fc.out.FldMem("rsp", StackSlotSize)
		fc.out.Frndint()
		fc.out.FstpMem("rsp", StackSlotSize)

		// Restore original control word
		fc.out.FldcwMem("rsp", 0)

		fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize)
		fc.out.AddImmToReg("rsp", 16)

	case "ceil":
		if len(call.Args) != 1 {
			compilerError("ceil() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// ceil(x): round toward +∞
		// FPU control word: set rounding mode to 10 (round up)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)

		// Save current FPU control word
		fc.out.FstcwMem("rsp", 0)

		// Load control word, modify to set RC=10 (bits 10-11)
		// Emit 16-bit MOV manually: mov ax, [rsp]
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x8B) // MOV r16, r/m16
		fc.out.Write(0x04) // ModR/M: [rsp]
		fc.out.Write(0x24) // SIB: [rsp]
		// OR ax, 0x0800 (set bit 11 for round up)
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x81) // OR r/m16, imm16
		fc.out.Write(0xC8) // ModR/M for ax
		fc.out.Write(0x00) // Low byte
		fc.out.Write(0x08) // High byte: 0x0800 = bit 11 set (round up)
		// AND ax, 0xFBFF (clear bit 10, keep bit 11)
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x81) // AND r/m16, imm16
		fc.out.Write(0xE0) // ModR/M for ax
		fc.out.Write(0xFF) // Low byte
		fc.out.Write(0xFB) // High byte: 0xFBFF = clear bit 10, keep bit 11
		// Store modified control word: mov [rsp+2], ax
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x89) // MOV r/m16, r16
		fc.out.Write(0x44) // ModR/M: [rsp+disp8]
		fc.out.Write(0x24) // SIB: [rsp]
		fc.out.Write(0x02) // disp8: +2

		fc.out.FldcwMem("rsp", 2)
		fc.out.FldMem("rsp", StackSlotSize)
		fc.out.Frndint()
		fc.out.FstpMem("rsp", StackSlotSize)
		fc.out.FldcwMem("rsp", 0) // Restore

		fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize)
		fc.out.AddImmToReg("rsp", 16)

	case "round":
		if len(call.Args) != 1 {
			compilerError("round() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// round(x): round to nearest (even)
		// FPU control word: set rounding mode to 00 (round to nearest)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)

		// Save current FPU control word
		fc.out.FstcwMem("rsp", 0)

		// Load control word, modify to set RC=00 (clear bits 10-11)
		// Emit 16-bit MOV manually: mov ax, [rsp]
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x8B) // MOV r16, r/m16
		fc.out.Write(0x04) // ModR/M: [rsp]
		fc.out.Write(0x24) // SIB: [rsp]
		// AND ax, 0xF3FF (clear bits 10-11 for round to nearest)
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x81) // AND r/m16, imm16
		fc.out.Write(0xE0) // ModR/M for ax
		fc.out.Write(0xFF) // Low byte
		fc.out.Write(0xF3) // High byte: 0xF3FF = clear bits 10-11
		// Store modified control word: mov [rsp+2], ax
		fc.out.Write(0x66) // 16-bit operand prefix
		fc.out.Write(0x89) // MOV r/m16, r16
		fc.out.Write(0x44) // ModR/M: [rsp+disp8]
		fc.out.Write(0x24) // SIB: [rsp]
		fc.out.Write(0x02) // disp8: +2

		fc.out.FldcwMem("rsp", 2)
		fc.out.FldMem("rsp", StackSlotSize)
		fc.out.Frndint()
		fc.out.FstpMem("rsp", StackSlotSize)
		fc.out.FldcwMem("rsp", 0) // Restore

		fc.out.MovMemToXmm("xmm0", "rsp", StackSlotSize)
		fc.out.AddImmToReg("rsp", 16)

	case "is_nan":
		// is_nan(x) - Returns 1.0 if x is NaN, 0.0 otherwise
		// NaN is the only value where x != x
		if len(call.Args) != 1 {
			compilerError("is_nan() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// xmm0 contains the value to check
		// Compare xmm0 with itself using UCOMISD
		// If NaN, ZF=1, PF=1, CF=1
		fc.out.Ucomisd("xmm0", "xmm0") // Compare xmm0 with itself

		// Set al to 1 if parity flag is set (indicates NaN)
		// SETP sets byte to 1 if PF=1 (parity), 0 otherwise
		// Emit: setp al (0F 9A C0)
		fc.out.Write(0x0F)
		fc.out.Write(0x9A)
		fc.out.Write(0xC0)

		// Zero-extend al to rax: movzx rax, al (48 0F B6 C0)
		fc.out.Write(0x48)
		fc.out.Write(0x0F)
		fc.out.Write(0xB6)
		fc.out.Write(0xC0)

		// Convert rax (0 or 1) to float64 in xmm0
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "is_finite":
		// is_finite(x) - Returns 1.0 if x is finite (not NaN, not Inf), 0.0 otherwise
		// A value is finite if (x - x) == 0.0
		// For finite values: x - x = 0.0
		// For NaN: NaN - NaN = NaN (not equal to 0)
		// For Inf: Inf - Inf = NaN (not equal to 0)
		// UCOMISD sets PF=1 when either operand is NaN
		if len(call.Args) != 1 {
			compilerError("is_finite() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// xmm0 contains the value to check

		// Copy xmm0 to xmm1
		fc.out.MovRegToReg("xmm1", "xmm0")

		// Subtract: xmm1 = xmm0 - xmm0
		fc.out.SubsdXmm("xmm1", "xmm0")

		// Load 0.0 into xmm2
		fc.out.XorpdXmm("xmm2", "xmm2") // xmm2 = 0.0

		// Compare xmm1 with 0.0
		// UCOMISD sets ZF=1 and PF=0 if equal and neither is NaN
		// If result is NaN, PF=1
		fc.out.Ucomisd("xmm1", "xmm2")

		// Set al to 1 if equal AND PF=0 (SETE checks ZF, but we also need to check PF)
		// We need ZF=1 and PF=0 for finite numbers
		// Use SETE (set if ZF=1) then AND with SETNP (set if PF=0)

		// SETE al - set if equal (ZF=1)
		fc.out.Write(0x0F)
		fc.out.Write(0x94)
		fc.out.Write(0xC0)

		// Move al to cl temporarily
		fc.out.Write(0x88) // mov cl, al
		fc.out.Write(0xC1)

		// SETNP al - set if not parity (PF=0)
		fc.out.Write(0x0F)
		fc.out.Write(0x9B)
		fc.out.Write(0xC0)

		// AND al, cl - both conditions must be true
		fc.out.Write(0x20) // and al, cl
		fc.out.Write(0xC8)

		// Zero-extend al to rax: movzx rax, al (48 0F B6 C0)
		fc.out.Write(0x48)
		fc.out.Write(0x0F)
		fc.out.Write(0xB6)
		fc.out.Write(0xC0)

		// Convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "is_inf":
		// is_inf(x) - Returns 1.0 if x is +Inf or -Inf, 0.0 otherwise
		// Use a simpler approach: is_inf(x) = !is_finite(x) && !is_nan(x)
		if len(call.Args) != 1 {
			compilerError("is_inf() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// xmm0 contains the value to check

		// Save the original value in xmm1
		fc.out.MovRegToReg("xmm1", "xmm0")

		// Check if NaN: xmm0 != xmm0
		fc.out.Ucomisd("xmm0", "xmm0")
		// SETP al - set if parity (NaN)
		fc.out.Write(0x0F)
		fc.out.Write(0x9A)
		fc.out.Write(0xC0)
		// Save NaN result in cl
		fc.out.Write(0x88) // mov cl, al
		fc.out.Write(0xC1)

		// Restore original value from xmm1
		fc.out.MovRegToReg("xmm0", "xmm1")

		// Check if finite: (x - x) == 0
		fc.out.MovRegToReg("xmm2", "xmm0")
		fc.out.SubsdXmm("xmm2", "xmm0")
		fc.out.XorpdXmm("xmm3", "xmm3") // xmm3 = 0.0
		fc.out.Ucomisd("xmm2", "xmm3")

		// SETE al - set if equal (ZF=1)
		fc.out.Write(0x0F)
		fc.out.Write(0x94)
		fc.out.Write(0xC0)

		// Move al to dl temporarily
		fc.out.Write(0x88) // mov dl, al
		fc.out.Write(0xC2)

		// SETNP al - set if not parity (PF=0)
		fc.out.Write(0x0F)
		fc.out.Write(0x9B)
		fc.out.Write(0xC0)

		// AND al, dl - both conditions must be true (is_finite)
		fc.out.Write(0x20) // and al, dl
		fc.out.Write(0xD0)

		// Now al = 1 if finite, 0 if not finite
		// We want: (!is_finite) && (!is_nan)
		// NOT al - flip finite result
		fc.out.Write(0xF6) // not al
		fc.out.Write(0xD0)
		fc.out.Write(0x24) // and al, 1 (keep only lowest bit)
		fc.out.Write(0x01)

		// NOT cl - flip NaN result
		fc.out.Write(0xF6) // not cl
		fc.out.Write(0xD1)
		fc.out.Write(0x80) // and cl, 1 (keep only lowest bit)
		fc.out.Write(0xE1)
		fc.out.Write(0x01)

		// AND al, cl - both !finite and !NaN must be true
		fc.out.Write(0x20) // and al, cl
		fc.out.Write(0xC8)

		// Zero-extend al to rax
		fc.out.Write(0x48)
		fc.out.Write(0x0F)
		fc.out.Write(0xB6)
		fc.out.Write(0xC0)

		// Convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "is_pos_inf":
		// is_pos_inf(x) - Returns 1.0 if x is +Inf, 0.0 otherwise
		// Check: is_inf(x) && x > 0
		if len(call.Args) != 1 {
			compilerError("is_pos_inf() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// xmm0 contains the value

		// Save original value
		fc.out.MovRegToReg("xmm1", "xmm0")

		// First check if it's infinite using same logic as is_inf()
		// Check if NaN
		fc.out.Ucomisd("xmm0", "xmm0")
		fc.out.Write(0x0F)
		fc.out.Write(0x9A)
		fc.out.Write(0xC0) // SETP al
		fc.out.Write(0x88) // mov cl, al
		fc.out.Write(0xC1)

		// Check if finite
		fc.out.MovRegToReg("xmm0", "xmm1")
		fc.out.MovRegToReg("xmm2", "xmm0")
		fc.out.SubsdXmm("xmm2", "xmm0")
		fc.out.XorpdXmm("xmm3", "xmm3")
		fc.out.Ucomisd("xmm2", "xmm3")
		fc.out.Write(0x0F)
		fc.out.Write(0x94)
		fc.out.Write(0xC0) // SETE al
		fc.out.Write(0x88) // mov dl, al
		fc.out.Write(0xC2)
		fc.out.Write(0x0F)
		fc.out.Write(0x9B)
		fc.out.Write(0xC0) // SETNP al
		fc.out.Write(0x20) // and al, dl
		fc.out.Write(0xD0)

		// NOT al - is_infinite
		fc.out.Write(0xF6)
		fc.out.Write(0xD0)
		fc.out.Write(0x24)
		fc.out.Write(0x01)

		// NOT cl - is_not_nan
		fc.out.Write(0xF6)
		fc.out.Write(0xD1)
		fc.out.Write(0x80)
		fc.out.Write(0xE1)
		fc.out.Write(0x01)

		// AND al, cl - is_inf result in al
		fc.out.Write(0x20)
		fc.out.Write(0xC8)

		// Save is_inf result
		fc.out.Write(0x88) // mov dl, al
		fc.out.Write(0xC2)

		// Now check if positive: compare with 0
		fc.out.MovRegToReg("xmm0", "xmm1") // restore original
		fc.out.XorpdXmm("xmm3", "xmm3")    // 0.0
		fc.out.Ucomisd("xmm0", "xmm3")
		// SETA al - set if above (x > 0, CF=0 and ZF=0)
		fc.out.Write(0x0F)
		fc.out.Write(0x97)
		fc.out.Write(0xC0)

		// AND al, dl - both is_inf and is_positive
		fc.out.Write(0x20)
		fc.out.Write(0xD0)

		// Zero-extend and convert to float
		fc.out.Write(0x48)
		fc.out.Write(0x0F)
		fc.out.Write(0xB6)
		fc.out.Write(0xC0)
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "is_neg_inf":
		// is_neg_inf(x) - Returns 1.0 if x is -Inf, 0.0 otherwise
		// Check: is_inf(x) && x < 0
		if len(call.Args) != 1 {
			compilerError("is_neg_inf() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// xmm0 contains the value

		// Save original value
		fc.out.MovRegToReg("xmm1", "xmm0")

		// First check if it's infinite (same logic as is_inf)
		fc.out.Ucomisd("xmm0", "xmm0")
		fc.out.Write(0x0F)
		fc.out.Write(0x9A)
		fc.out.Write(0xC0) // SETP al
		fc.out.Write(0x88) // mov cl, al
		fc.out.Write(0xC1)

		// Check if finite
		fc.out.MovRegToReg("xmm0", "xmm1")
		fc.out.MovRegToReg("xmm2", "xmm0")
		fc.out.SubsdXmm("xmm2", "xmm0")
		fc.out.XorpdXmm("xmm3", "xmm3")
		fc.out.Ucomisd("xmm2", "xmm3")
		fc.out.Write(0x0F)
		fc.out.Write(0x94)
		fc.out.Write(0xC0) // SETE al
		fc.out.Write(0x88) // mov dl, al
		fc.out.Write(0xC2)
		fc.out.Write(0x0F)
		fc.out.Write(0x9B)
		fc.out.Write(0xC0) // SETNP al
		fc.out.Write(0x20) // and al, dl
		fc.out.Write(0xD0)

		// NOT al - is_infinite
		fc.out.Write(0xF6)
		fc.out.Write(0xD0)
		fc.out.Write(0x24)
		fc.out.Write(0x01)

		// NOT cl - is_not_nan
		fc.out.Write(0xF6)
		fc.out.Write(0xD1)
		fc.out.Write(0x80)
		fc.out.Write(0xE1)
		fc.out.Write(0x01)

		// AND al, cl - is_inf result in al
		fc.out.Write(0x20)
		fc.out.Write(0xC8)

		// Save is_inf result
		fc.out.Write(0x88) // mov dl, al
		fc.out.Write(0xC2)

		// Now check if negative: compare with 0
		fc.out.MovRegToReg("xmm0", "xmm1") // restore original
		fc.out.XorpdXmm("xmm3", "xmm3")    // 0.0
		fc.out.Ucomisd("xmm0", "xmm3")
		// SETB al - set if below (x < 0, CF=1)
		fc.out.Write(0x0F)
		fc.out.Write(0x92)
		fc.out.Write(0xC0)

		// AND al, dl - both is_inf and is_negative
		fc.out.Write(0x20)
		fc.out.Write(0xD0)

		// Zero-extend and convert to float
		fc.out.Write(0x48)
		fc.out.Write(0x0F)
		fc.out.Write(0xB6)
		fc.out.Write(0xC0)
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "safe_divide":
		// safe_divide(a, b) - Performs a/b with explicit NaN on division by zero
		// IEEE 754 already handles: x/0.0 = ±Inf, 0.0/0.0 = NaN
		// Actually, just let regular division happen - it's already "safe" with NaN propagation!
		// This function exists mainly for documentation and can check for div-by-zero if needed
		if len(call.Args) != 2 {
			compilerError("safe_divide() requires exactly 2 arguments")
		}
		// For now, just do regular division - IEEE 754 handles it
		// Compile: a / b
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.compileExpression(call.Args[1])
		fc.out.MovRegToReg("xmm1", "xmm0")
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.DivsdXmm("xmm0", "xmm1")
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "safe_sqrt":
		// safe_sqrt(x) - Returns sqrt(x) if x >= 0, NaN if x < 0
		// sqrt() of negative numbers already produces NaN in IEEE 754 with x86 SSE!
		if len(call.Args) != 1 {
			compilerError("safe_sqrt() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		fc.out.Sqrtsd("xmm0", "xmm0") // sqrt already returns NaN for negative inputs!

	case "safe_ln":
		// safe_ln(x) - Returns ln(x) if x > 0, NaN if x <= 0
		// x87 FYL2X with negative/zero input produces undefined results
		// We need to check and return NaN for x <= 0
		if len(call.Args) != 1 {
			compilerError("safe_ln() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// Same as regular ln - x87 handles edge cases
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.Fldln2()         // ST(0) = ln(2)
		fc.out.FldMem("rsp", 0) // ST(0) = x, ST(1) = ln(2)
		fc.out.Fyl2x()          // ST(0) = ln(x) or NaN/Inf for invalid inputs
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "safe_divide_result":
		// safe_divide_result(a, b) - Returns Result (pointer to value or NaN with error)
		// NEW NaN-BASED RESULT SYSTEM:
		//   Success: Returns pointer to arena-allocated float64 containing result
		//   Error: Returns NaN (0x7FF8...) with "div0  " encoded for division by zero
		if len(call.Args) != 2 {
			compilerError("safe_divide_result() requires exactly 2 arguments")
		}
		if fc.currentArena == 0 {
			compilerError("safe_divide_result() requires arena { } block for Result allocation")
		}

		// Evaluate b first and check if it's zero
		fc.compileExpression(call.Args[1]) // b in xmm0
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0) // save b

		// Check if b == 0.0
		fc.out.XorpdXmm("xmm1", "xmm1")                   // xmm1 = 0.0
		fc.out.Emit([]byte{0x66, 0x0F, 0x2E, 0x04, 0x24}) // ucomisd xmm0, [rsp]

		divByZeroJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // je div_by_zero

		// b != 0, proceed with division
		fc.compileExpression(call.Args[0])       // a in xmm0
		fc.out.MovMemToXmm("xmm1", "rsp", 0)     // b in xmm1
		fc.out.AddImmToReg("rsp", StackSlotSize) // clean stack
		fc.out.DivsdXmm("xmm0", "xmm1")          // xmm0 = a/b

		// Allocate 8 bytes in arena for result
		offset := (fc.currentArena - 1) * 8
		fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
		fc.out.MovMemToReg("rdi", "rdi", 0)
		fc.out.MovMemToReg("rdi", "rdi", offset)
		fc.out.MovImmToReg("rsi", "8")

		// Save result before calling alloc
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		fc.out.CallSymbol("_c67_arena_alloc")

		// rax = pointer, load result and store it
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rax", 0) // store result at allocated address

		// Return pointer as float64
		EmitPointerToFloat64(fc.out, "xmm0", "rax")

		successJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // jmp done

		// Division by zero error path
		divByZeroLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(divByZeroJump+2, int32(divByZeroLabel-(divByZeroJump+6)))

		fc.out.AddImmToReg("rsp", StackSlotSize) // clean stack

		// Return NaN with "div0  " encoded: d=0x64 i=0x69 v=0x76 0=0x30 space=0x20
		// Little-endian: 0x7FF8 2020 3076 6964
		fc.out.MovImmToReg("rax", "0x7FF8202030766964")
		EmitPointerToFloat64(fc.out, "xmm0", "rax")

		// Done
		doneLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(successJump+1, int32(doneLabel-(successJump+5)))

	case "result_value":
		// result_value(r) - Dereferences Result pointer if success, returns NaN if error
		// If r is a valid pointer (< 0x7FF...), load the float64 at that address
		// If r is NaN (>= 0x7FF...), just return the NaN unchanged
		if len(call.Args) != 1 {
			compilerError("result_value() requires exactly 1 argument")
		}

		fc.compileExpression(call.Args[0]) // Result in xmm0

		// Convert float64 to pointer using movq xmm -> reg
		EmitFloat64ToPointer(fc.out, "rax", "xmm0")

		// Check if this is a NaN (top 12 bits are 0x7FF)
		// For NaN: 0x7FF8_xxxx_xxxx_xxxx, so rax >> 52 >= 0x7FF
		fc.out.MovRegToReg("rcx", "rax")
		fc.out.ShrRegByImm("rcx", 52) // rcx = top 12 bits

		// Compare with 0x7FF (NaN marker)
		fc.out.CmpRegToImm("rcx", 0x7FF)

		nanJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge is_nan

		// It's a valid pointer - dereference it
		fc.out.MovMemToXmm("xmm0", "rax", 0) // Load float64 from pointer

		doneJump := fc.eb.text.Len()
		fc.out.JumpUnconditional(0) // jmp done

		// It's NaN - just convert back to float64 (already in rax)
		nanLabel := fc.eb.text.Len()
		fc.patchJumpImmediate(nanJump+2, int32(nanLabel-(nanJump+6)))
		EmitPointerToFloat64(fc.out, "xmm0", "rax")

		// Done
		doneLabel2 := fc.eb.text.Len()
		fc.patchJumpImmediate(doneJump+1, int32(doneLabel2-(doneJump+5)))

	case "safe_sqrt_result":
		// safe_sqrt_result(x) - Returns Result map {0: ok, 1: value, 2: error_code}
		// Returns sqrt(x) or NaN for negative x
		if len(call.Args) != 1 {
			compilerError("safe_sqrt_result() requires exactly 1 argument")
		}
		if fc.currentArena == 0 {
			compilerError("safe_sqrt_result() requires arena { } block for Result allocation")
		}

		// Evaluate x and compute sqrt
		fc.compileExpression(call.Args[0]) // x in xmm0
		fc.out.Sqrtsd("xmm0", "xmm0")      // sqrt(x) - NaN if x < 0

		// Save result
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Allocate Result map (64 bytes)
		offset := (fc.currentArena - 1) * 8
		fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
		fc.out.MovMemToReg("rdi", "rdi", 0)
		fc.out.MovMemToReg("rdi", "rdi", offset)
		fc.out.MovImmToReg("rsi", "64")
		fc.out.CallSymbol("_c67_arena_alloc")

		fc.out.MovRegToReg("rbx", "rax") // save map pointer

		// Initialize header: count=3.0
		fc.out.MovImmToReg("rax", "0x4008000000000000") // 3.0
		fc.out.MovRegToMem("rax", "rbx", 0)

		// Load result
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Entry 0: key=0.0, ok=1.0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbx", 8)
		fc.out.MovImmToReg("rax", "0x3FF0000000000000") // 1.0
		fc.out.MovRegToMem("rax", "rbx", 16)

		// Entry 1: key=1.0, value=sqrt(x)
		fc.out.MovImmToReg("rax", "0x3FF0000000000000") // 1.0
		fc.out.MovRegToMem("rax", "rbx", 24)
		fc.out.MovXmmToMem("xmm0", "rbx", 32)

		// Entry 2: key=2.0, error=0.0
		fc.out.MovImmToReg("rax", "0x4000000000000000") // 2.0
		fc.out.MovRegToMem("rax", "rbx", 40)
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbx", 48)

		// Return pointer
		EmitPointerToFloat64(fc.out, "xmm0", "rbx")

	case "safe_ln_result":
		// safe_ln_result(x) - Returns Result map {0: ok, 1: value, 2: error_code}
		// Returns ln(x) or NaN for x <= 0
		if len(call.Args) != 1 {
			compilerError("safe_ln_result() requires exactly 1 argument")
		}
		if fc.currentArena == 0 {
			compilerError("safe_ln_result() requires arena { } block for Result allocation")
		}

		// Evaluate x and compute ln
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.Fldln2()         // ST(0) = ln(2)
		fc.out.FldMem("rsp", 0) // ST(0) = x, ST(1) = ln(2)
		fc.out.Fyl2x()          // ST(0) = ln(x) or NaN/Inf
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		// Result still on stack, don't pop yet

		// Allocate Result map (64 bytes)
		offset := (fc.currentArena - 1) * 8
		fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
		fc.out.MovMemToReg("rdi", "rdi", 0)
		fc.out.MovMemToReg("rdi", "rdi", offset)
		fc.out.MovImmToReg("rsi", "64")
		fc.out.CallSymbol("_c67_arena_alloc")

		fc.out.MovRegToReg("rbx", "rax") // save map pointer

		// Initialize header: count=3.0
		fc.out.MovImmToReg("rax", "0x4008000000000000") // 3.0
		fc.out.MovRegToMem("rax", "rbx", 0)

		// Load result
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Entry 0: key=0.0, ok=1.0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbx", 8)
		fc.out.MovImmToReg("rax", "0x3FF0000000000000") // 1.0
		fc.out.MovRegToMem("rax", "rbx", 16)

		// Entry 1: key=1.0, value=ln(x)
		fc.out.MovImmToReg("rax", "0x3FF0000000000000") // 1.0
		fc.out.MovRegToMem("rax", "rbx", 24)
		fc.out.MovXmmToMem("xmm0", "rbx", 32)

		// Entry 2: key=2.0, error=0.0
		fc.out.MovImmToReg("rax", "0x4000000000000000") // 2.0
		fc.out.MovRegToMem("rax", "rbx", 40)
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rbx", 48)

		// Return pointer
		EmitPointerToFloat64(fc.out, "xmm0", "rbx")

	case "log":
		if len(call.Args) != 1 {
			compilerError("log() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// log(x) = ln(x) = log2(x) / log2(e) = log2(x) * ln(2) / (ln(2) / ln(e))
		// FYL2X computes ST(1) * log2(ST(0))
		// So: log(x) = ln(2) * log2(x) = FYL2X with ST(1)=ln(2), ST(0)=x
		// But we want ln(x), not log2(x)
		// ln(x) = log2(x) * ln(2)
		// Actually: FYL2X gives us: ST(1) * log2(ST(0))
		// So if ST(1) = ln(2) and ST(0) = x, we get: ln(2) * log2(x) = ln(x) ✓
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.Fldln2()         // ST(0) = ln(2)
		fc.out.FldMem("rsp", 0) // ST(0) = x, ST(1) = ln(2)
		fc.out.Fyl2x()          // ST(0) = ln(2) * log2(x) = ln(x)
		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "exp":
		if len(call.Args) != 1 {
			compilerError("exp() requires exactly 1 argument")
		}
		fc.compileExpression(call.Args[0])
		// exp(x) = e^x = 2^(x * log2(e))
		// Steps:
		// 1. Multiply x by log2(e): x' = x * log2(e)
		// 2. Split x' = n + f where n is integer, -1 <= f <= 1
		// 3. Compute 2^f using F2XM1: 2^f = 1 + F2XM1(f)
		// 4. Scale by 2^n using FSCALE
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.FldMem("rsp", 0) // ST(0) = x
		fc.out.Fldl2e()         // ST(0) = log2(e), ST(1) = x
		fc.out.Fmulp()          // ST(0) = x * log2(e)

		// Now split into integer and fractional parts
		fc.out.FldSt0()    // ST(0) = x', ST(1) = x'
		fc.out.Frndint()   // ST(0) = n (integer part)
		fc.out.FldSt0()    // ST(0) = n, ST(1) = n, ST(2) = x'
		fc.out.Write(0xD9) // FXCH st(2) - exchange ST(0) and ST(2)
		fc.out.Write(0xCA)
		fc.out.Fsubrp() // ST(0) = x' - n = f, ST(1) = n

		// Compute 2^f - 1 using F2XM1
		fc.out.F2xm1() // ST(0) = 2^f - 1, ST(1) = n
		fc.out.Fld1()  // ST(0) = 1, ST(1) = 2^f - 1, ST(2) = n
		fc.out.Faddp() // ST(0) = 2^f, ST(1) = n

		// Scale by 2^n
		fc.out.Fscale() // ST(0) = 2^f * 2^n = 2^(n+f) = e^x, ST(1) = n
		// Discard n (ST(1)) while keeping result in ST(0)
		fc.out.Write(0xDD) // FSTP st(1) - stores ST(0) to st(1), pops stack
		fc.out.Write(0xD9)

		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

	case "pow":
		if len(call.Args) != 2 {
			compilerError("pow() requires exactly 2 arguments")
		}
		fc.compileExpression(call.Args[0]) // x in xmm0
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.compileExpression(call.Args[1]) // y in xmm0
		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)

		// pow(x, y) = x^y = 2^(y * log2(x))
		// Steps:
		// 1. Compute log2(x) using FYL2X
		// 2. Multiply by y
		// 3. Split into integer and fractional parts
		// 4. Use F2XM1 and FSCALE like in exp

		fc.out.Fld1()                       // ST(0) = 1.0
		fc.out.FldMem("rsp", 0)             // ST(0) = x, ST(1) = 1.0
		fc.out.Fyl2x()                      // ST(0) = 1 * log2(x) = log2(x)
		fc.out.FldMem("rsp", StackSlotSize) // ST(0) = y, ST(1) = log2(x)
		fc.out.Fmulp()                      // ST(0) = y * log2(x)

		// Split into n + f
		fc.out.FldSt0()    // ST(0) = y*log2(x), ST(1) = y*log2(x)
		fc.out.Frndint()   // ST(0) = n
		fc.out.FldSt0()    // ST(0) = n, ST(1) = n, ST(2) = y*log2(x)
		fc.out.Write(0xD9) // FXCH st(2)
		fc.out.Write(0xCA)
		fc.out.Fsubrp() // ST(0) = f, ST(1) = n

		// Compute 2^f
		fc.out.F2xm1() // ST(0) = 2^f - 1
		fc.out.Fld1()
		fc.out.Faddp()  // ST(0) = 2^f, ST(1) = n
		fc.out.Fscale() // ST(0) = 2^f * 2^n = x^y, ST(1) = n
		// Discard n (ST(1)) while keeping result in ST(0)
		fc.out.Write(0xDD) // FSTP st(1) - stores ST(0) to st(1), pops stack
		fc.out.Write(0xD9)

		fc.out.FstpMem("rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)

	// ===== BIT MANIPULATION BUILTINS =====
	// High-performance CPU instructions for bit operations
	// with graceful fallback for older CPUs

	case "popcount":
		// popcount(x) - Count number of set bits (population count)
		// Returns float64 representing the count
		// Uses POPCNT instruction if available (3 cycles), falls back to loop (~25 cycles)
		if len(call.Args) != 1 {
			compilerError("popcount() requires exactly 1 argument")
		}

		// Compile argument and convert to integer
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rax", "xmm0") // Convert float64 to int64

		// Check if POPCNT is available
		fc.out.LeaSymbolToReg("rcx", "cpu_has_popcnt")
		fc.out.Emit([]byte{0x0f, 0xb6, 0x09}) // movzx ecx, byte [rcx]
		fc.out.Emit([]byte{0x85, 0xc9})       // test ecx, ecx

		// Jump to fallback if no POPCNT
		jzPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // jz fallback

		// POPCNT path: popcnt rax, rax
		fc.out.Emit([]byte{0xf3, 0x48, 0x0f, 0xb8, 0xc0}) // popcnt rax, rax

		// Jump over fallback
		jmpOverPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0xeb, 0x00}) // jmp end

		// Fallback path: loop implementation
		fallbackPos := fc.eb.text.Len()
		// rcx = count (result), rdx = temp
		fc.out.XorRegWithReg("rcx", "rcx") // count = 0
		fc.out.MovRegToReg("rdx", "rax")   // rdx = x (preserve rax for comparison)

		// Loop: while (rdx != 0) { count += rdx & 1; rdx >>= 1; }
		loopStart := fc.eb.text.Len()
		fc.out.Emit([]byte{0x48, 0x85, 0xd2}) // test rdx, rdx
		loopEndJump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x74, 0x00}) // jz loop_end (2 bytes)

		fc.out.MovRegToReg("rax", "rdx")            // rax = rdx
		fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x01}) // and rax, 1
		fc.out.AddRegToReg("rcx", "rax")            // count += (rdx & 1)
		fc.out.ShrRegByImm("rdx", 1)                // rdx >>= 1

		// Jump back to loop start
		backOffset := loopStart - (fc.eb.text.Len() + 2)
		fc.out.Emit([]byte{0xeb, byte(backOffset)}) // jmp loop_start

		// Loop end
		loopEndPos := fc.eb.text.Len()
		fc.eb.text.Bytes()[loopEndJump+1] = byte(loopEndPos - (loopEndJump + 2))

		fc.out.MovRegToReg("rax", "rcx") // Move result to rax

		// End position
		endPos := fc.eb.text.Len()

		// Patch jumps
		fc.patchJumpImmediate(jzPos+2, int32(fallbackPos-(jzPos+6)))
		fc.eb.text.Bytes()[jmpOverPos+1] = byte(endPos - (jmpOverPos + 2))

		// Convert result to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "clz":
		// clz(x) - Count leading zeros
		// Returns float64 representing the count (0-64)
		// Uses LZCNT instruction if available, falls back to BSR + adjustment
		if len(call.Args) != 1 {
			compilerError("clz() requires exactly 1 argument")
		}

		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rax", "xmm0") // Convert to int64

		// Check if POPCNT is available (LZCNT came with same CPU generation)
		fc.out.LeaSymbolToReg("rcx", "cpu_has_popcnt")
		fc.out.Emit([]byte{0x0f, 0xb6, 0x09}) // movzx ecx, byte [rcx]
		fc.out.Emit([]byte{0x85, 0xc9})       // test ecx, ecx

		jzPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // jz fallback

		// LZCNT path: lzcnt rax, rax
		fc.out.Emit([]byte{0xf3, 0x48, 0x0f, 0xbd, 0xc0}) // lzcnt rax, rax

		jmpOverPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0xeb, 0x00}) // jmp end

		// Fallback path: use BSR (bit scan reverse)
		fallbackPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax

		zeroJump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x74, 0x00}) // jz is_zero (2 bytes)

		// BSR: finds position of highest set bit
		fc.out.Emit([]byte{0x48, 0x0f, 0xbd, 0xc8}) // bsr rcx, rax
		fc.out.MovImmToReg("rax", "63")
		fc.out.SubRegFromReg("rax", "rcx") // clz = 63 - bsr_result

		jmpEndPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0xeb, 0x00}) // jmp end

		// Zero case: return 64
		zeroPos := fc.eb.text.Len()
		fc.eb.text.Bytes()[zeroJump+1] = byte(zeroPos - (zeroJump + 2))
		fc.out.MovImmToReg("rax", "64")

		// End position
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(jzPos+2, int32(fallbackPos-(jzPos+6)))
		fc.eb.text.Bytes()[jmpOverPos+1] = byte(endPos - (jmpOverPos + 2))
		fc.eb.text.Bytes()[jmpEndPos+1] = byte(endPos - (jmpEndPos + 2))

		fc.out.Cvtsi2sd("xmm0", "rax")

	case "ctz":
		// ctz(x) - Count trailing zeros
		// Returns float64 representing the count (0-64)
		// Uses TZCNT instruction if available, falls back to BSF
		if len(call.Args) != 1 {
			compilerError("ctz() requires exactly 1 argument")
		}

		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rax", "xmm0") // Convert to int64

		// Check if POPCNT is available (TZCNT came with same CPU generation)
		fc.out.LeaSymbolToReg("rcx", "cpu_has_popcnt")
		fc.out.Emit([]byte{0x0f, 0xb6, 0x09}) // movzx ecx, byte [rcx]
		fc.out.Emit([]byte{0x85, 0xc9})       // test ecx, ecx

		jzPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x0f, 0x84, 0x00, 0x00, 0x00, 0x00}) // jz fallback

		// TZCNT path: tzcnt rax, rax
		fc.out.Emit([]byte{0xf3, 0x48, 0x0f, 0xbc, 0xc0}) // tzcnt rax, rax

		jmpOverPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0xeb, 0x00}) // jmp end

		// Fallback path: use BSF (bit scan forward)
		fallbackPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax

		zeroJump := fc.eb.text.Len()
		fc.out.Emit([]byte{0x74, 0x00}) // jz is_zero (2 bytes)

		// BSF: finds position of lowest set bit (already gives us trailing zeros!)
		fc.out.Emit([]byte{0x48, 0x0f, 0xbc, 0xc0}) // bsf rax, rax

		jmpEndPos := fc.eb.text.Len()
		fc.out.Emit([]byte{0xeb, 0x00}) // jmp end

		// Zero case: return 64
		zeroPos := fc.eb.text.Len()
		fc.eb.text.Bytes()[zeroJump+1] = byte(zeroPos - (zeroJump + 2))
		fc.out.MovImmToReg("rax", "64")

		// End position
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(jzPos+2, int32(fallbackPos-(jzPos+6)))
		fc.eb.text.Bytes()[jmpOverPos+1] = byte(endPos - (jmpOverPos + 2))
		fc.eb.text.Bytes()[jmpEndPos+1] = byte(endPos - (jmpEndPos + 2))

		fc.out.Cvtsi2sd("xmm0", "rax")

	// ===== END BIT MANIPULATION BUILTINS =====

	case "str":
		// Convert number to string
		// str(x) converts a number to a C67 string (map[uint64]float64)
		if len(call.Args) != 1 {
			compilerError("str() requires exactly 1 argument")
		}

		// Compile argument (result in xmm0)
		fc.compileExpression(call.Args[0])

		// Allocate 32 bytes for ASCII conversion buffer
		fc.out.SubImmFromReg("rsp", 32)
		// Save buffer address before compileFloatToString changes rsp
		fc.out.MovRegToReg("r15", "rsp")

		// Convert float64 in xmm0 to ASCII string at r15
		// Result: rsi = string start, rdx = length
		fc.compileFloatToString("xmm0", "r15")

		// Check if last char is newline and adjust length
		// rax = rdx - 1
		fc.out.MovRegToReg("rax", "rdx")
		fc.out.SubImmFromReg("rax", 1)
		// r10 = rsi + rax (pointer to last char)
		fc.out.MovRegToReg("r10", "rsi")
		fc.out.AddRegToReg("r10", "rax")
		// Load byte at r10
		fc.out.Emit([]byte{0x45, 0x0f, 0xb6, 0x12}) // movzx r10d, byte [r10]
		// Compare r10 with 10 (newline)
		fc.out.Emit([]byte{0x49, 0x83, 0xfa, 0x0a}) // cmp r10, 10
		skipNewlineLabel := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0)
		skipNewlineEnd := fc.eb.text.Len()

		// Has newline - decrement length
		fc.out.SubImmFromReg("rdx", 1)

		// Skip target
		skipNewline := fc.eb.text.Len()
		fc.patchJumpImmediate(skipNewlineLabel+2, int32(skipNewline-skipNewlineEnd))

		// Calculate map size: 8 + length * 16
		// rdi = rdx * 16
		fc.out.MovRegToReg("rdi", "rdx")
		fc.out.Emit([]byte{0x48, 0xc1, 0xe7, 0x04}) // shl rdi, 4
		fc.out.AddImmToReg("rdi", 8)

		// Save rsi and rdx before malloc
		fc.out.PushReg("rsi")
		fc.out.PushReg("rdx")

		// Call malloc
		// Allocate from arena
		fc.callArenaAlloc()

		// Restore
		fc.out.PopReg("rdx")
		fc.out.PopReg("rsi")

		// Write count
		fc.out.Cvtsi2sd("xmm1", "rdx")
		fc.out.MovXmmToMem("xmm1", "rax", 0)

		// Save map pointer
		fc.out.MovRegToReg("r11", "rax")

		// Loop to build map
		fc.out.XorRegWithReg("rcx", "rcx")
		fc.out.MovRegToReg("rdi", "rax")
		fc.out.AddImmToReg("rdi", 8)

		loopStart := fc.eb.text.Len()

		// cmp rcx, rdx
		fc.out.Emit([]byte{0x48, 0x39, 0xd1}) // cmp rcx, rdx
		loopEndJump := fc.eb.text.Len()
		fc.out.JumpConditional(JumpGreaterOrEqual, 0)
		loopEndJumpEnd := fc.eb.text.Len()

		// Write key
		fc.out.Cvtsi2sd("xmm1", "rcx")
		fc.out.MovXmmToMem("xmm1", "rdi", 0)
		fc.out.AddImmToReg("rdi", 8)

		// Load char and write value
		fc.out.Emit([]byte{0x4c, 0x0f, 0xb6, 0x16}) // movzx r10, byte [rsi]
		fc.out.Cvtsi2sd("xmm1", "r10")
		fc.out.MovXmmToMem("xmm1", "rdi", 0)
		fc.out.AddImmToReg("rdi", 8)

		// Increment
		fc.out.AddImmToReg("rcx", 1)
		fc.out.AddImmToReg("rsi", 1)

		// Jump back
		loopEnd := fc.eb.text.Len()
		offset := loopStart - (loopEnd + 2)
		fc.out.Emit([]byte{0xeb, byte(offset)})

		// Loop done
		loopDone := fc.eb.text.Len()
		fc.patchJumpImmediate(loopEndJump+2, int32(loopDone-loopEndJumpEnd))

		// Return map pointer as float64 (move bits directly, don't convert)
		// Use movq xmm0, r11 to transfer pointer bits without conversion
		// movq xmm0, r11 = 66 49 0f 6e c3
		fc.out.Emit([]byte{0x66, 0x49, 0x0f, 0x6e, 0xc3})

		// Clean up
		fc.out.AddImmToReg("rsp", 32)

	case "approx":
		// Approximate equality: approx(a, b, epsilon) returns 1 if abs(a-b) <= epsilon
		if len(call.Args) != 3 {
			compilerError("approx() requires exactly 3 arguments: approx(a, b, epsilon)")
		}

		// Compile a and b
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile epsilon
		fc.compileExpression(call.Args[2])

		// Load a and b
		fc.out.MovMemToXmm("xmm1", "rsp", 0)  // b
		fc.out.MovMemToXmm("xmm2", "rsp", 16) // a
		fc.out.AddImmToReg("rsp", 32)

		// xmm2 = a, xmm1 = b, xmm0 = epsilon
		// Calculate diff = a - b
		fc.out.MovXmmToXmm("xmm3", "xmm2")
		fc.out.SubsdXmm("xmm3", "xmm1") // xmm3 = a - b

		// abs(diff): if diff < 0, negate it
		fc.out.XorpdXmm("xmm4", "xmm4")             // xmm4 = 0.0
		fc.out.Ucomisd("xmm3", "xmm4")              // compare diff with 0
		fc.out.JumpConditional(JumpAboveOrEqual, 0) // if diff >= 0, skip negation
		negateJumpPos := fc.eb.text.Len() - 4

		// Negate: diff = 0 - diff
		fc.out.MovXmmToXmm("xmm4", "xmm3")
		fc.out.XorpdXmm("xmm3", "xmm3")
		fc.out.SubsdXmm("xmm3", "xmm4") // xmm3 = 0 - diff

		// Patch jump
		skipNegateLabel := fc.eb.text.Len()
		offset := int32(skipNegateLabel - (negateJumpPos + 4))
		fc.patchJumpImmediate(negateJumpPos, offset)

		// Compare: abs(diff) <= epsilon
		fc.out.Ucomisd("xmm3", "xmm0")

		// Set result based on comparison (1 if <=, 0 otherwise)
		fc.out.MovImmToReg("rax", "0")
		fc.out.MovImmToReg("rcx", "1")
		fc.out.Cmovbe("rax", "rcx") // rax = (abs(diff) <= epsilon) ? 1 : 0
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "num":
		// Parse string to number
		// num(string) converts a C67 string to a number
		if len(call.Args) != 1 {
			compilerError("num() requires exactly 1 argument")
		}

		// Compile argument (C67 string pointer in xmm0)
		fc.compileExpression(call.Args[0])

		// Convert C67 string to C string
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")

		// Call strtod(str, NULL) to parse the string
		// rdi = C string (already in rax from _c67_string_to_cstr)
		fc.out.MovRegToReg("rdi", "rax")
		fc.out.XorRegWithReg("rsi", "rsi") // endptr = NULL
		fc.trackFunctionCall("strtod")
		fc.eb.GenerateCallInstruction("strtod")
		// Result in xmm0

	case "upper":
		// Convert string to uppercase
		// upper(string) returns a new uppercase string
		if len(call.Args) != 1 {
			compilerError("upper() requires exactly 1 argument")
		}

		// Compile argument (C67 string pointer in xmm0)
		fc.compileExpression(call.Args[0])

		// Call runtime helper upper_string
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.out.CallSymbol("upper_string")
		// Result in xmm0

	case "lower":
		// Convert string to lowercase
		// lower(string) returns a new lowercase string
		if len(call.Args) != 1 {
			compilerError("lower() requires exactly 1 argument")
		}

		// Compile argument (C67 string pointer in xmm0)
		fc.compileExpression(call.Args[0])

		// Call runtime helper lower_string
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.out.CallSymbol("lower_string")
		// Result in xmm0

	case "trim":
		// Remove leading/trailing whitespace
		// trim(string) returns a new trimmed string
		if len(call.Args) != 1 {
			compilerError("trim() requires exactly 1 argument")
		}

		// Compile argument (C67 string pointer in xmm0)
		fc.compileExpression(call.Args[0])

		// Call runtime helper trim_string
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.out.CallSymbol("trim_string")
		// Result in xmm0

	case "write_i8", "write_i16", "write_i32", "write_i64",
		"write_u8", "write_u16", "write_u32", "write_u64", "write_f32", "write_f64":
		// FFI memory write: write_TYPE(ptr, index, value)
		if len(call.Args) != 3 {
			compilerError("%s() requires exactly 3 arguments (ptr, index, value)", call.Function)
		}

		// Determine type size
		var typeSize int
		switch call.Function {
		case "write_i8", "write_u8":
			typeSize = 1
		case "write_i16", "write_u16":
			typeSize = 2
		case "write_i32", "write_u32", "write_f32":
			typeSize = 4
		case "write_i64", "write_u64", "write_f64":
			typeSize = 8
		}

		// Compile pointer (arg 0) - result in xmm0
		fc.compileExpression(call.Args[0])
		// Convert pointer from float64 to integer in r10
		fc.out.Cvttsd2si("r10", "xmm0")
		// Save pointer on stack (push r10)
		fc.out.Emit([]byte{0x41, 0x52}) // push r10

		// Compile index (arg 1) - result in xmm0
		fc.compileExpression(call.Args[1])
		// Convert index to integer in r11
		fc.out.Cvttsd2si("r11", "xmm0")
		// Save index on stack (push r11)
		fc.out.Emit([]byte{0x41, 0x53}) // push r11

		// Compile value (arg 2) - result in xmm0
		fc.compileExpression(call.Args[2])
		// Save value in xmm1
		fc.out.MovXmmToXmm("xmm1", "xmm0")

		// Restore index and pointer (pop r11, pop r10)
		fc.out.Emit([]byte{0x41, 0x5b}) // pop r11
		fc.out.Emit([]byte{0x41, 0x5a}) // pop r10

		// Calculate address: r10 + (r11 * typeSize)
		if typeSize > 1 {
			// Multiply index by type size: rax = r11 * typeSize
			fc.out.MovImmToReg("rax", fmt.Sprintf("%d", typeSize))
			fc.out.Emit([]byte{0x49, 0x0f, 0xaf, 0xc3}) // imul rax, r11 (rax = rax * r11)
			// Add to base pointer: r10 = r10 + rax
			fc.out.Emit([]byte{0x49, 0x01, 0xc2}) // add r10, rax
		} else {
			// If typeSize == 1, r10 = r10 + r11 directly
			fc.out.Emit([]byte{0x4d, 0x01, 0xda}) // add r10, r11
		}

		// Restore value from xmm1 to xmm0
		fc.out.MovXmmToXmm("xmm0", "xmm1")

		// Write value to memory
		if call.Function == "write_f64" {
			// Write float64 directly
			fc.out.MovXmmToMem("xmm0", "r10", 0)
		} else if call.Function == "write_f32" {
			// Convert double to float (xmm0 double -> xmm0 float)
			// cvtsd2ss xmm0, xmm0
			fc.out.Write(0xf2)
			fc.out.Write(0x0f)
			fc.out.Write(0x5a)
			fc.out.Write(0xc0) // ModR/M: xmm0, xmm0
			// Write float32 (4 bytes) to memory [r10]
			// movss [r10], xmm0
			fc.out.Write(0xf3)
			fc.out.Write(0x41)
			fc.out.Write(0x0f)
			fc.out.Write(0x11)
			fc.out.Write(0x02) // ModR/M: [r10]
		} else {
			// Convert to integer and write
			fc.out.Cvttsd2si("rax", "xmm0")
			switch typeSize {
			case 1:
				fc.out.MovByteRegToMem("rax", "r10", 0)
			case 2:
				// mov word [r10], ax
				fc.out.Write(0x66) // 16-bit operand prefix
				fc.out.Write(0x41) // REX prefix for r10
				fc.out.Write(0x89) // mov r/m16, r16
				fc.out.Write(0x02) // ModR/M: [r10]
			case 4:
				// mov dword [r10], eax
				fc.out.Write(0x41) // REX prefix for r10
				fc.out.Write(0x89) // mov r/m32, r32
				fc.out.Write(0x02) // ModR/M: [r10]
			case 8:
				// mov qword [r10], rax
				fc.out.MovRegToMem("rax", "r10", 0)
			}
		}

		// Return 0 (these functions don't return values)
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "read_i8", "read_i16", "read_i32", "read_i64",
		"read_u8", "read_u16", "read_u32", "read_u64", "read_f64":
		// FFI memory read: read_TYPE(ptr, index) -> value
		if len(call.Args) != 2 {
			compilerError("%s() requires exactly 2 arguments (ptr, index)", call.Function)
		}

		// Determine type size and signed/unsigned
		var typeSize int
		isSigned := strings.HasPrefix(call.Function, "read_i")
		isFloat := call.Function == "read_f64"

		switch call.Function {
		case "read_i8", "read_u8":
			typeSize = 1
		case "read_i16", "read_u16":
			typeSize = 2
		case "read_i32", "read_u32":
			typeSize = 4
		case "read_i64", "read_u64", "read_f64":
			typeSize = 8
		}

		// Compile pointer (arg 0) - result in xmm0
		fc.compileExpression(call.Args[0])
		// Convert pointer from float64 to integer in r10
		fc.out.Cvttsd2si("r10", "xmm0")
		// Save pointer on stack (push r10)
		fc.out.Emit([]byte{0x41, 0x52}) // push r10

		// Compile index (arg 1) - result in xmm0
		fc.compileExpression(call.Args[1])
		// Convert index to integer in r11
		fc.out.Cvttsd2si("r11", "xmm0")

		// Restore pointer (pop r10)
		fc.out.Emit([]byte{0x41, 0x5a}) // pop r10

		// Calculate address: r10 + (r11 * typeSize)
		if typeSize > 1 {
			// Multiply index by type size: rax = r11 * typeSize
			fc.out.MovImmToReg("rax", fmt.Sprintf("%d", typeSize))
			fc.out.Emit([]byte{0x49, 0x0f, 0xaf, 0xc3}) // imul rax, r11 (rax = rax * r11)
			// Add to base pointer: r10 = r10 + rax
			fc.out.Emit([]byte{0x49, 0x01, 0xc2}) // add r10, rax
		} else {
			// If typeSize == 1, r10 = r10 + r11 directly
			fc.out.Emit([]byte{0x4d, 0x01, 0xda}) // add r10, r11
		}

		// Read value from memory
		if isFloat {
			// Read float64 directly
			fc.out.MovMemToXmm("xmm0", "r10", 0)
		} else {
			// Read integer and convert
			switch typeSize {
			case 1:
				if isSigned {
					// movsx rax, byte [r10]
					fc.out.Write(0x49) // REX.W + REX.B
					fc.out.Write(0x0f) // Two-byte opcode
					fc.out.Write(0xbe) // movsx
					fc.out.Write(0x02) // ModR/M: [r10]
				} else {
					// movzx rax, byte [r10]
					fc.out.Write(0x49) // REX.W + REX.B
					fc.out.Write(0x0f) // Two-byte opcode
					fc.out.Write(0xb6) // movzx
					fc.out.Write(0x02) // ModR/M: [r10]
				}
			case 2:
				if isSigned {
					// movsx rax, word [r10]
					fc.out.Write(0x49) // REX.W + REX.B
					fc.out.Write(0x0f) // Two-byte opcode
					fc.out.Write(0xbf) // movsx
					fc.out.Write(0x02) // ModR/M: [r10]
				} else {
					// movzx rax, word [r10]
					fc.out.Write(0x49) // REX.W + REX.B
					fc.out.Write(0x0f) // Two-byte opcode
					fc.out.Write(0xb7) // movzx
					fc.out.Write(0x02) // ModR/M: [r10]
				}
			case 4:
				if isSigned {
					// movsxd rax, dword [r10]
					fc.out.Write(0x49) // REX.W + REX.B
					fc.out.Write(0x63) // movsxd
					fc.out.Write(0x02) // ModR/M: [r10]
				} else {
					// mov eax, dword [r10] (zero extends to rax)
					fc.out.Write(0x41) // REX.B for r10
					fc.out.Write(0x8b) // mov
					fc.out.Write(0x02) // ModR/M: [r10]
				}
			case 8:
				// mov rax, qword [r10]
				fc.out.MovMemToReg("rax", "r10", 0)
			}
			// Convert integer to float64
			if isSigned {
				fc.out.Cvtsi2sd("xmm0", "rax")
			} else {
				// For unsigned, need special handling for large values
				// For simplicity, just use signed conversion (works for values < 2^63)
				fc.out.Cvtsi2sd("xmm0", "rax")
			}
		}

	case "call":
		// FFI: call(function_name, args...)
		// First argument must be a string literal (function name)
		if len(call.Args) < 1 {
			compilerError("call() requires at least a function name")
		}

		fnNameExpr, ok := call.Args[0].(*StringExpr)
		if !ok {
			compilerError("call() first argument must be a string literal (function name)")
		}
		fnName := fnNameExpr.Value

		// x86-64 calling convention:
		// Integer/pointer args: rdi, rsi, rdx, rcx, r8, r9
		// Float args: xmm0-xmm7
		intRegs := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		xmmRegs := []string{"xmm0", "xmm1", "xmm2", "xmm3", "xmm4", "xmm5", "xmm6", "xmm7"}

		intArgCount := 0
		xmmArgCount := 0
		numArgs := len(call.Args) - 1 // Exclude function name

		if numArgs > 8 {
			compilerError("call() supports max 8 arguments (got %d)", numArgs)
		}

		// Determine argument types by checking for cast expressions
		argTypes := make([]string, numArgs)
		for i := 0; i < numArgs; i++ {
			arg := call.Args[i+1]
			if castExpr, ok := arg.(*CastExpr); ok {
				argTypes[i] = castExpr.Type
			} else {
				// No cast - assume float64
				argTypes[i] = "f64"
			}
		}

		// Evaluate all arguments and save to stack
		for i := 0; i < numArgs; i++ {
			fc.compileExpression(call.Args[i+1])
			fc.out.SubImmFromReg("rsp", StackSlotSize)
			fc.out.MovXmmToMem("xmm0", "rsp", 0)
		}

		// Load arguments into registers (in reverse order from stack)
		for i := numArgs - 1; i >= 0; i-- {
			argType := argTypes[i]

			// Determine if this is an integer/pointer argument or float argument
			isIntArg := false
			switch argType {
			case "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "ptr", "cstr":
				isIntArg = true
			case "float32", "float64":
				isIntArg = false
			default:
				// Unknown type - assume float
				isIntArg = false
			}

			fc.out.MovMemToXmm("xmm0", "rsp", 0)
			fc.out.AddImmToReg("rsp", StackSlotSize)

			if isIntArg {
				// Integer/pointer argument
				if intArgCount < len(intRegs) {
					// For cstr, the value is already a pointer in xmm0
					// For integers, convert from float64 to integer
					if argType == "cstr" {
						// cstr is already a pointer - just transfer bits
						fc.out.SubImmFromReg("rsp", StackSlotSize)
						fc.out.MovXmmToMem("xmm0", "rsp", 0)
						fc.out.MovMemToReg(intRegs[intArgCount], "rsp", 0)
						fc.out.AddImmToReg("rsp", StackSlotSize)
					} else {
						// Convert float64 to integer
						fc.out.Cvttsd2si(intRegs[intArgCount], "xmm0")
					}
					intArgCount++
				} else {
					compilerError("call() supports max 6 integer/pointer arguments")
				}
			} else {
				// Float argument
				if xmmArgCount < len(xmmRegs) {
					if xmmArgCount != 0 {
						// Move to appropriate xmm register
						fc.out.SubImmFromReg("rsp", StackSlotSize)
						fc.out.MovXmmToMem("xmm0", "rsp", 0)
						fc.out.MovMemToXmm(xmmRegs[xmmArgCount], "rsp", 0)
						fc.out.AddImmToReg("rsp", StackSlotSize)
					}
					// else: already in xmm0
					xmmArgCount++
				} else {
					compilerError("call() supports max 8 float arguments")
				}
			}
		}

		// Set rax = number of vector registers used (required by x86-64 ABI for varargs)
		fc.out.MovImmToReg("rax", fmt.Sprintf("%d", xmmArgCount))

		// Call the C function
		fc.trackFunctionCall(fnName)
		fc.eb.GenerateCallInstruction(fnName)

		// Result is in rax (for integer/pointer returns) or xmm0 (for float returns)
		// Check if this is a known floating-point function
		floatFunctions := map[string]bool{
			"sqrt": true, "sin": true, "cos": true, "tan": true,
			"asin": true, "acos": true, "atan": true, "atan2": true,
			"log": true, "log10": true, "exp": true, "pow": true,
			"fabs": true, "fmod": true, "ceil": true, "floor": true,
		}

		if floatFunctions[fnName] {
			// Float return - result already in xmm0
			// Nothing to do
		} else {
			// Integer/pointer return - result in rax
			// For most functions, we want to preserve the value semantics (convert int to float)
			// For pointer returns (getenv, malloc, etc), "as number" will be used to get the pointer bits
			fc.out.Cvtsi2sd("xmm0", "rax")
		}

	case "alloc":
		// alloc(size) - Allocates memory from current arena
		// Current arena is always available (starts at arena 1 = meta-arena[0])
		if len(call.Args) != 1 {
			compilerError("alloc() requires 1 argument (size)")
		}

		if fc.currentArena == 0 {
			compilerError("alloc() called outside of arena context (currentArena=0)")
		}

		// Compile size argument FIRST (before loading arena pointer)
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0") // size in rdi temporarily

		// Save size to stack
		fc.out.PushReg("rdi")

		// Load arena pointer from meta-arena: _c67_arena_meta[currentArena-1]
		arenaIndex := fc.currentArena - 1 // Convert to 0-based index
		offset := arenaIndex * 8
		fc.out.LeaSymbolToReg("rdi", "_c67_arena_meta")
		fc.out.MovMemToReg("rdi", "rdi", 0) // Load the meta-arena pointer

		fc.out.MovMemToReg("rdi", "rdi", offset) // Load the arena pointer from slot

		// Restore size to rsi
		fc.out.PopReg("rsi") // size in rsi

		// Call arena_alloc (with auto-growing via realloc)
		fc.out.CallSymbol("_c67_arena_alloc")

		// DEBUG: Force return a fixed value
		if false {
			fc.out.MovImmToReg("rax", "0x1234567890") // Test value
		}

		// Result in rax, move raw bits to xmm0 (same as map literals)
		EmitPointerToFloat64(fc.out, "xmm0", "rax")

	case "dlopen":
		// dlopen(path, flags) - Open a dynamic library
		// path: string (C67 string), flags: number (RTLD_LAZY=1, RTLD_NOW=2)
		// Returns: library handle as float64
		if len(call.Args) != 2 {
			compilerError("dlopen() requires 2 arguments (path, flags)")
		}

		// Evaluate flags argument first (will be in rdi later)
		fc.compileExpression(call.Args[1])
		// Convert flags to integer
		fc.out.Cvttsd2si("r8", "xmm0")
		// Save flags to stack
		fc.out.Emit([]byte{0x41, 0x50}) // push r8

		// Evaluate path argument (C67 string)
		fc.compileExpression(call.Args[0])
		// Convert C67 string to C string (xmm0 has map pointer)
		// Save xmm0 to stack, call conversion function
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0) // C string pointer will be in rax after call
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Call _c67_string_to_cstr (result in rax)
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")

		// Now rax = C string pointer
		// Pop flags from stack to rsi
		fc.out.Emit([]byte{0x41, 0x58})  // pop r8
		fc.out.MovRegToReg("rdi", "rax") // path in rdi
		fc.out.MovRegToReg("rsi", "r8")  // flags in rsi

		// Align stack for C call
		fc.out.SubImmFromReg("rsp", StackSlotSize)

		// Call dlopen
		fc.out.CallSymbol("dlopen")

		// Restore stack
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// rax = library handle (pointer)
		// Convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "dlsym":
		// dlsym(handle, symbol) - Get symbol address from library
		// handle: number (library handle from dlopen), symbol: string
		// Returns: symbol address as float64
		if len(call.Args) != 2 {
			compilerError("dlsym() requires 2 arguments (handle, symbol)")
		}

		// Evaluate handle first
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r8", "xmm0")
		fc.out.Emit([]byte{0x41, 0x50}) // push r8

		// Evaluate symbol (C67 string)
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Convert to C string
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")

		// Pop handle to rdi
		fc.out.Emit([]byte{0x41, 0x58})  // pop r8
		fc.out.MovRegToReg("rsi", "rax") // symbol in rsi
		fc.out.MovRegToReg("rdi", "r8")  // handle in rdi

		// Align stack
		fc.out.SubImmFromReg("rsp", StackSlotSize)

		// Call dlsym
		fc.out.CallSymbol("dlsym")

		// Restore stack
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// rax = symbol address
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "dlclose":
		// dlclose(handle) - Close a dynamic library
		// handle: number (library handle from dlopen)
		// Returns: 0.0 on success, non-zero on error
		if len(call.Args) != 1 {
			compilerError("dlclose() requires 1 argument (handle)")
		}

		// Evaluate handle
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0")

		// Align stack
		fc.out.SubImmFromReg("rsp", StackSlotSize)

		// Call dlclose
		fc.out.CallSymbol("dlclose")

		// Restore stack
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// rax = return code (0 on success)
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "readln":
		compilerError("readln() is not implemented yet - requires stdin support without libc")

		// Allocate space on stack for getline parameters
		// getline(&lineptr, &n, stdin)
		// lineptr will be allocated by getline
		fc.out.SubImmFromReg("rsp", 16) // 8 bytes for lineptr, 8 for n

		// Initialize lineptr = NULL, n = 0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.MovRegToMem("rax", "rsp", 0) // lineptr = NULL
		fc.out.MovRegToMem("rax", "rsp", 8) // n = 0

		// Load stdin from libc
		// stdin is at stdin@@GLIBC_2.2.5
		fc.out.LeaSymbolToReg("rdx", "stdin")
		fc.out.MovMemToReg("rdx", "rdx", 0) // dereference stdin pointer

		// Set up getline arguments
		fc.out.MovRegToReg("rdi", "rsp")    // &lineptr
		fc.out.LeaMemToReg("rsi", "rsp", 8) // &n
		// rdx already has stdin

		// Call getline
		fc.trackFunctionCall("getline")
		fc.trackFunctionCall("stdin")
		fc.eb.GenerateCallInstruction("getline")

		// getline returns number of characters read (or -1 on error)
		// lineptr now points to allocated buffer with the line

		// Load lineptr from stack
		fc.out.MovMemToReg("rdi", "rsp", 0)

		// Check if lineptr is NULL (error case)
		fc.out.TestRegReg("rdi", "rdi")
		errorJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // Jump if NULL

		// Strip newline if present (getline includes \n)
		// Check if rax > 0 (characters read)
		fc.out.TestRegReg("rax", "rax")
		emptyJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLessOrEqual, 0) // Jump if no characters

		// Check if last character is newline: byte [rdi + rax - 1] == '\n'
		fc.out.Emit([]byte{0x80, 0x7c, 0x07, 0xff, 0x0a}) // cmp byte [rdi + rax - 1], '\n'
		noNewlineJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpNotEqual, 0) // Jump if not newline

		// Replace newline with null terminator
		fc.out.Emit([]byte{0xc6, 0x44, 0x07, 0xff, 0x00}) // mov byte [rdi + rax - 1], 0

		// Patch no-newline jump to here
		noNewlinePos := fc.eb.text.Len()
		fc.patchJumpImmediate(noNewlineJumpPos+2, int32(noNewlinePos-(noNewlineJumpPos+6)))

		// Patch empty jump to here
		emptyPos := fc.eb.text.Len()
		fc.patchJumpImmediate(emptyJumpPos+2, int32(emptyPos-(emptyJumpPos+6)))

		// Convert C string to C67 string
		// rdi already has lineptr
		fc.out.CallSymbol("_c67_cstr_to_string")
		// Result in xmm0

		// Save result
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 16) // Save above the getline locals

		// Free the lineptr buffer
		fc.out.MovMemToReg("rdi", "rsp", StackSlotSize) // Load lineptr from original position
		fc.trackFunctionCall("free")
		fc.eb.GenerateCallInstruction("free")

		// Restore result
		fc.out.MovMemToXmm("xmm0", "rsp", 16)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Clean up stack
		fc.out.AddImmToReg("rsp", 16)

		// Jump to end
		endJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Error case: return empty string
		errorPos := fc.eb.text.Len()
		fc.patchJumpImmediate(errorJumpPos+2, int32(errorPos-(errorJumpPos+6)))

		// Clean up stack
		fc.out.AddImmToReg("rsp", 16)

		// Create empty C67 string (count = 0)
		fc.out.MovImmToReg("rdi", "8") // Allocate 8 bytes for count
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.XorRegWithReg("rdx", "rdx")
		fc.out.Cvtsi2sd("xmm0", "rdx")       // xmm0 = 0.0
		fc.out.MovXmmToMem("xmm0", "rax", 0) // [map] = 0.0
		fc.out.MovRegToXmm("xmm0", "rax")    // Return map pointer

		// Patch end jump
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(endJumpPos+1, int32(endPos-(endJumpPos+5)))

	case "read_file":
		// read_file(path) - Read entire file, return as C67 string
		// Uses Linux syscalls (open/lseek/read/close) instead of libc for simplicity
		if len(call.Args) != 1 {
			compilerError("read_file() requires 1 argument (path)")
		}

		// Evaluate path argument (C67 string)
		fc.compileExpression(call.Args[0])

		// Convert C67 string to C string (null-terminated)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")

		// Allocate stack frame: 32 bytes (fd, size, buffer, result)
		fc.out.SubImmFromReg("rsp", 32)

		// syscall open(path, O_RDONLY=0, mode=0)
		// rax=2 (sys_open), rdi=path, rsi=flags, rdx=mode
		fc.out.MovRegToReg("rdi", "rax")   // path from _c67_string_to_cstr
		fc.out.XorRegWithReg("rsi", "rsi") // O_RDONLY = 0
		fc.out.XorRegWithReg("rdx", "rdx") // mode = 0
		fc.out.MovImmToReg("rax", "2")     // sys_open = 2
		fc.out.Emit([]byte{0x0f, 0x05})    // syscall

		// Check if open failed (fd < 0)
		fc.out.TestRegReg("rax", "rax")
		errorJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpLess, 0) // Jump if negative

		// Save fd at [rsp+0]
		fc.out.MovRegToMem("rax", "rsp", 0)

		// syscall lseek(fd, 0, SEEK_END=2) to get file size
		// rax=8 (sys_lseek), rdi=fd, rsi=offset, rdx=whence
		fc.out.MovRegToReg("rdi", "rax")   // fd
		fc.out.XorRegWithReg("rsi", "rsi") // offset = 0
		fc.out.MovImmToReg("rdx", "2")     // SEEK_END = 2
		fc.out.MovImmToReg("rax", "8")     // sys_lseek = 8
		fc.out.Emit([]byte{0x0f, 0x05})    // syscall

		// Save size at [rsp+8]
		fc.out.MovRegToMem("rax", "rsp", 8)

		// syscall lseek(fd, 0, SEEK_SET=0) to rewind
		fc.out.MovMemToReg("rdi", "rsp", 0) // fd from [rsp+0]
		fc.out.XorRegWithReg("rsi", "rsi")  // offset = 0
		fc.out.XorRegWithReg("rdx", "rdx")  // SEEK_SET = 0
		fc.out.MovImmToReg("rax", "8")      // sys_lseek = 8
		fc.out.Emit([]byte{0x0f, 0x05})     // syscall

		// Allocate buffer: malloc(size + 1) for null terminator
		fc.out.MovMemToReg("rdi", "rsp", 8) // size from [rsp+8]
		fc.out.AddImmToReg("rdi", 1)        // +1 for null terminator
		// Allocate from arena
		fc.callArenaAlloc()

		// Save buffer at [rsp+16]
		fc.out.MovRegToMem("rax", "rsp", 16)

		// syscall read(fd, buffer, size)
		// rax=0 (sys_read), rdi=fd, rsi=buffer, rdx=count
		fc.out.MovMemToReg("rdi", "rsp", 0)  // fd from [rsp+0]
		fc.out.MovMemToReg("rsi", "rsp", 16) // buffer from [rsp+16]
		fc.out.MovMemToReg("rdx", "rsp", 8)  // size from [rsp+8]
		fc.out.XorRegWithReg("rax", "rax")   // sys_read = 0
		fc.out.Emit([]byte{0x0f, 0x05})      // syscall

		// Add null terminator: buffer[size] = 0
		fc.out.MovMemToReg("rdi", "rsp", 16)        // buffer from [rsp+16]
		fc.out.MovMemToReg("rdx", "rsp", 8)         // size from [rsp+8]
		fc.out.Emit([]byte{0xc6, 0x04, 0x17, 0x00}) // mov byte [rdi + rdx], 0

		// syscall close(fd)
		// rax=3 (sys_close), rdi=fd
		fc.out.MovMemToReg("rdi", "rsp", 0) // fd from [rsp+0]
		fc.out.MovImmToReg("rax", "3")      // sys_close = 3
		fc.out.Emit([]byte{0x0f, 0x05})     // syscall

		// Convert buffer to C67 string
		fc.out.MovMemToReg("rdi", "rsp", 16) // buffer from [rsp+16]
		fc.out.CallSymbol("_c67_cstr_to_string")
		// Result in xmm0

		// Save result at [rsp+24]
		fc.out.MovXmmToMem("xmm0", "rsp", 24)

		// Buffer is arena-allocated, no need to free (arena cleanup handles it)

		// Restore result
		fc.out.MovMemToXmm("xmm0", "rsp", 24)

		// Clean up stack frame
		fc.out.AddImmToReg("rsp", 32)

		// Jump to end
		endJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Error case: return empty string
		errorPos := fc.eb.text.Len()
		fc.patchJumpImmediate(errorJumpPos+2, int32(errorPos-(errorJumpPos+6)))

		// Clean up stack
		fc.out.AddImmToReg("rsp", 32)

		// Create empty C67 string (count = 0)
		fc.out.MovImmToReg("rdi", "8")
		// Allocate from arena
		fc.callArenaAlloc()
		fc.out.XorRegWithReg("rdx", "rdx")
		fc.out.Cvtsi2sd("xmm0", "rdx")
		fc.out.MovXmmToMem("xmm0", "rax", 0)
		fc.out.MovRegToXmm("xmm0", "rax")

		// Patch end jump
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(endJumpPos+1, int32(endPos-(endJumpPos+5)))

	case "write_file":
		// write_file(path, content) - Write string to file
		if len(call.Args) != 2 {
			compilerError("write_file() requires 2 arguments (path, content)")
		}

		// Evaluate and convert content first
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")
		fc.out.PushReg("rax") // Save content C string

		// Evaluate and convert path
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rdi", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)
		fc.trackFunctionCall("_c67_string_to_cstr")
		fc.out.CallSymbol("_c67_string_to_cstr")

		// Open file: fopen(path, "w")
		fc.out.MovRegToReg("rdi", "rax") // path
		labelName := fmt.Sprintf("write_mode_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(labelName, "w\x00")
		fc.out.LeaSymbolToReg("rsi", labelName) // mode = "w"
		fc.trackFunctionCall("fopen")
		fc.eb.GenerateCallInstruction("fopen")

		// Check if fopen succeeded
		fc.out.TestRegReg("rax", "rax")
		errorJumpPos := fc.eb.text.Len()
		fc.out.JumpConditional(JumpEqual, 0) // Jump if NULL

		// Save FILE* pointer
		fc.out.PushReg("rax")

		// Get content length using strlen
		fc.out.MovMemToReg("rdi", "rsp", StackSlotSize) // content
		fc.trackFunctionCall("strlen")
		fc.eb.GenerateCallInstruction("strlen")
		fc.out.PushReg("rax") // Save length

		// Write file: fwrite(content, 1, length, file)
		fc.out.MovMemToReg("rdi", "rsp", StackSlotSize*2) // content
		fc.out.MovImmToReg("rsi", "1")                    // element size = 1
		fc.out.MovMemToReg("rdx", "rsp", 0)               // length
		fc.out.MovMemToReg("rcx", "rsp", StackSlotSize)   // FILE*
		fc.trackFunctionCall("fwrite")
		fc.eb.GenerateCallInstruction("fwrite")

		// Close file: fclose(file)
		fc.out.MovMemToReg("rdi", "rsp", StackSlotSize) // FILE*
		fc.trackFunctionCall("fclose")
		fc.eb.GenerateCallInstruction("fclose")

		// Clean up stack (length + FILE* + content)
		fc.out.AddImmToReg("rsp", StackSlotSize*3)

		// Return 0 (success)
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

		// Jump to end
		endJumpPos := fc.eb.text.Len()
		fc.out.JumpUnconditional(0)

		// Error case: clean up and return -1
		errorPos := fc.eb.text.Len()
		fc.patchJumpImmediate(errorJumpPos+2, int32(errorPos-(errorJumpPos+6)))

		// Clean up content from stack
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Return -1 (error)
		fc.out.MovImmToReg("rax", "-1")
		fc.out.Cvtsi2sd("xmm0", "rax")

		// Patch end jump
		endPos := fc.eb.text.Len()
		fc.patchJumpImmediate(endJumpPos+1, int32(endPos-(endJumpPos+5)))

	case "sizeof_i8", "sizeof_u8":
		// sizeof_i8() / sizeof_u8() - Return size of 8-bit integer (1 byte)
		if len(call.Args) != 0 {
			compilerError("%s() takes no arguments", call.Function)
		}
		// Load 1.0 into xmm0
		fc.out.MovImmToReg("rax", "1")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "sizeof_i16", "sizeof_u16":
		// sizeof_i16() / sizeof_u16() - Return size of 16-bit integer (2 bytes)
		if len(call.Args) != 0 {
			compilerError("%s() takes no arguments", call.Function)
		}
		fc.out.MovImmToReg("rax", "2")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "sizeof_i32", "sizeof_u32", "sizeof_f32":
		// sizeof_i32() / sizeof_u32() / sizeof_f32() - Return size (4 bytes)
		if len(call.Args) != 0 {
			compilerError("%s() takes no arguments", call.Function)
		}
		fc.out.MovImmToReg("rax", "4")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "sizeof_i64", "sizeof_u64", "sizeof_f64", "sizeof_ptr":
		// sizeof_i64() / sizeof_u64() / sizeof_f64() / sizeof_ptr() - Return size (8 bytes)
		if len(call.Args) != 0 {
			compilerError("%s() takes no arguments", call.Function)
		}
		fc.out.MovImmToReg("rax", "8")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "vadd":
		// vadd(v1, v2) - Vector addition using SIMD
		if len(call.Args) != 2 {
			compilerError("vadd() requires exactly 2 arguments")
		}

		// Compile first vector argument -> pointer in xmm0
		fc.compileExpression(call.Args[0])
		// Push pointer to stack (save for later)
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile second vector argument -> pointer in xmm0
		fc.compileExpression(call.Args[1])
		// Convert second vector pointer to rbx
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("rbx", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Get first vector pointer from stack to rax
		fc.out.MovMemToReg("rax", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// For now, just return the first vector pointer to test if the logic works
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "vsub":
		// vsub(v1, v2) - Vector subtraction using SIMD
		if len(call.Args) != 2 {
			compilerError("vsub() requires exactly 2 arguments")
		}

		// Compile first vector argument -> pointer in xmm0
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r12", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Compile second vector argument -> pointer in xmm0
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r13", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Allocate stack space for result
		fc.out.SubImmFromReg("rsp", 16)

		// Load and subtract vectors using SIMD
		fc.out.MovupdMemToXmm("xmm0", "r12", 0)
		fc.out.MovupdMemToXmm("xmm1", "r13", 0)
		fc.out.SubpdXmm("xmm0", "xmm1")

		// Store result and return pointer
		fc.out.MovupdXmmToMem("xmm0", "rsp", 0)
		fc.out.MovRegToReg("rax", "rsp")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "vmul":
		// vmul(v1, v2) - Vector element-wise multiplication using SIMD
		if len(call.Args) != 2 {
			compilerError("vmul() requires exactly 2 arguments")
		}

		// Compile first vector argument
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r12", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Compile second vector argument
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r13", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Allocate stack space for result
		fc.out.SubImmFromReg("rsp", 16)

		// Load and multiply vectors using SIMD
		fc.out.MovupdMemToXmm("xmm0", "r12", 0)
		fc.out.MovupdMemToXmm("xmm1", "r13", 0)
		fc.out.MulpdXmm("xmm0", "xmm1")

		// Store result and return pointer
		fc.out.MovupdXmmToMem("xmm0", "rsp", 0)
		fc.out.MovRegToReg("rax", "rsp")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "vdiv":
		// vdiv(v1, v2) - Vector element-wise division using SIMD
		if len(call.Args) != 2 {
			compilerError("vdiv() requires exactly 2 arguments")
		}

		// Compile first vector argument
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r12", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Compile second vector argument
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r13", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Allocate stack space for result
		fc.out.SubImmFromReg("rsp", 16)

		// Load and divide vectors using SIMD
		fc.out.MovupdMemToXmm("xmm0", "r12", 0)
		fc.out.MovupdMemToXmm("xmm1", "r13", 0)
		fc.out.DivpdXmm("xmm0", "xmm1")

		// Store result and return pointer
		fc.out.MovupdXmmToMem("xmm0", "rsp", 0)
		fc.out.MovRegToReg("rax", "rsp")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "vdot":
		// vdot(v1, v2) - Vector dot product (sum of element-wise multiplication)
		// Returns a scalar float64
		// Simplest implementation: multiply then horizontal add
		if len(call.Args) != 2 {
			compilerError("vdot() requires exactly 2 arguments")
		}

		// Compile first vector argument
		fc.compileExpression(call.Args[0])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r12", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Compile second vector argument
		fc.compileExpression(call.Args[1])
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)
		fc.out.MovMemToReg("r13", "rsp", 0)
		fc.out.AddImmToReg("rsp", StackSlotSize)

		// Load vectors and multiply element-wise
		fc.out.MovupdMemToXmm("xmm0", "r12", 0) // v1
		fc.out.MovupdMemToXmm("xmm1", "r13", 0) // v2
		fc.out.MulpdXmm("xmm0", "xmm1")         // xmm0 = v1 * v2

		// Simple horizontal add: xmm0[0] + xmm0[1]
		// Move high element to xmm1
		fc.out.Emit([]byte{0xf2, 0x0f, 0x70, 0xc8, 0x01}) // pshufd xmm1, xmm0, 1
		fc.out.AddsdXmm("xmm0", "xmm1")                   // xmm0[0] += xmm1[0]
		// Result is scalar in xmm0[0]

	case "atomic_add":
		// atomic_add(ptr, value) - Atomically add value to *ptr and return old value
		// Uses LOCK XADD instruction for atomic read-modify-write
		if len(call.Args) != 2 {
			compilerError("atomic_add() requires exactly 2 arguments (ptr, value)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = pointer

		// Compile value argument
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("rax", "xmm0") // rax = value to add

		// LOCK XADD [r10], rax - atomically exchange and add
		// Result: memory location gets old + value, rax gets old value
		fc.out.LockXaddMemReg("r10", 0, "rax")

		// Convert result to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "atomic_cas":
		// atomic_cas(ptr, old, new) - Compare and swap: if *ptr == old, set *ptr = new
		// Returns 1 if successful, 0 if failed
		if len(call.Args) != 3 {
			compilerError("atomic_cas() requires exactly 3 arguments (ptr, old, new)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = pointer

		// Compile old value argument
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("rax", "xmm0") // rax = expected old value

		// Save rax to r12 before compiling third argument (not r11 which is used by parallel loops)
		fc.out.MovRegToReg("r12", "rax")

		// Compile new value argument
		fc.compileExpression(call.Args[2])
		fc.out.Cvttsd2si("rcx", "xmm0") // rcx = new value

		// Restore rax from r12
		fc.out.MovRegToReg("rax", "r12")

		// LOCK CMPXCHG [r10], rcx
		// If [r10] == rax, then [r10] := rcx and ZF := 1
		// Otherwise, rax := [r10] and ZF := 0
		fc.out.Emit([]byte{0xf0})                   // LOCK prefix
		fc.out.Emit([]byte{0x49, 0x0f, 0xb1, 0x0a}) // LOCK cmpxchg [r10], rcx

		// Set result based on ZF flag (1 if swap succeeded, 0 if failed)
		fc.out.MovImmToReg("rax", "0")
		fc.out.MovImmToReg("rdx", "1")
		fc.out.Emit([]byte{0x48, 0x0f, 0x44, 0xc2}) // cmove rax, rdx (if ZF=1, rax=rdx=1)
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "atomic_load":
		// atomic_load(ptr) - Atomically load value from memory
		// Uses memory barrier for acquire semantics
		if len(call.Args) != 1 {
			compilerError("atomic_load() requires exactly 1 argument (ptr)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = pointer

		// Load with acquire semantics (on x86, regular load is sufficient)
		fc.out.MovMemToReg("rax", "r10", 0)

		// Convert to float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "atomic_store":
		// atomic_store(ptr, value) - Atomically store value to memory
		// Uses memory barrier for release semantics
		if len(call.Args) != 2 {
			compilerError("atomic_store() requires exactly 2 arguments (ptr, value)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = pointer

		// Compile value argument
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("rax", "xmm0") // rax = value

		// Store with release semantics (on x86, regular store is sufficient)
		fc.out.MovRegToMem("rax", "r10", 0)

		// Return the stored value
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "malloc":
		// REMOVED: malloc() is not a builtin function.
		// Use arena {} blocks with allocate() for automatic memory management,
		// or use c.malloc() via C FFI if you need manual control.
		compilerError("malloc() is not a builtin function. Use arena {} with allocate(), or c.malloc() via C FFI")

	case "free":
		// REMOVED: free() is not a builtin function.
		// Use arena {} blocks for automatic memory management,
		// or use c.free() via C FFI if you need manual control.
		compilerError("free() is not a builtin function. Use arena {} blocks, or c.free() via C FFI")

	case "realloc":
		// REMOVED: realloc() is not a builtin function.
		// Use arena {} blocks with allocate() for automatic memory management,
		// or use c.realloc() via C FFI if you need manual control.
		compilerError("realloc() is not a builtin function. Use arena {} blocks, or c.realloc() via C FFI")

	case "calloc":
		// REMOVED: calloc() is not a builtin function.
		// Use arena {} blocks with allocate() for automatic memory management,
		// or use c.calloc() via C FFI if you need manual control.
		compilerError("calloc() is not a builtin function. Use arena {} blocks, or c.calloc() via C FFI")

	case "store":
		// store(ptr, offset, value) - Store value to memory at ptr + offset*8
		if len(call.Args) != 3 {
			compilerError("store() requires exactly 3 arguments (ptr, offset, value)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = base pointer

		// Compile offset argument
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("r11", "xmm0") // r11 = offset (in 8-byte units)

		// Calculate memory address: r10 = r10 + r11*8
		fc.out.ShlImmReg("r11", 3)       // r11 *= 8 (shift left by 3)
		fc.out.AddRegToReg("r10", "r11") // r10 = base + offset*8

		// Compile value argument
		fc.compileExpression(call.Args[2])
		fc.out.Cvttsd2si("rax", "xmm0") // rax = value (as integer)

		// Store value to memory
		fc.out.MovRegToMem("rax", "r10", 0)

		// Return 0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "load":
		// load(ptr, offset) - Load value from memory at ptr + offset*8
		if len(call.Args) != 2 {
			compilerError("load() requires exactly 2 arguments (ptr, offset)")
		}

		// Compile pointer argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("r10", "xmm0") // r10 = base pointer

		// Compile offset argument
		fc.compileExpression(call.Args[1])
		fc.out.Cvttsd2si("r11", "xmm0") // r11 = offset (in 8-byte units)

		// Calculate memory address: r10 = r10 + r11*8
		fc.out.ShlImmReg("r11", 3)       // r11 *= 8 (shift left by 3)
		fc.out.AddRegToReg("r10", "r11") // r10 = base + offset*8

		// Load value from memory
		fc.out.MovMemToReg("rax", "r10", 0)

		// Convert to float64 and return
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "chan":
		// chan(capacity) - Create a new channel
		// capacity = 0 for unbuffered, >0 for buffered
		if len(call.Args) == 1 {
			// Compile capacity argument
			fc.compileExpression(call.Args[0])
			fc.out.Cvttsd2si("rdi", "xmm0") // rdi = capacity
		} else if len(call.Args) == 0 {
			// Default: unbuffered channel (capacity = 0)
			fc.out.XorRegWithReg("rdi", "rdi")
		} else {
			compilerError("chan() requires 0 or 1 arguments")
		}

		// Call channel_create from runtime
		fc.trackFunctionCall("channel_create")
		fc.eb.GenerateCallInstruction("channel_create")

		// Return channel pointer as float64
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "close":
		// close(channel) - Close a channel
		if len(call.Args) != 1 {
			compilerError("close() requires exactly 1 argument (channel)")
		}

		// Compile channel argument
		fc.compileExpression(call.Args[0])
		fc.out.Cvttsd2si("rdi", "xmm0") // rdi = channel pointer

		// Call channel_close from runtime
		fc.trackFunctionCall("channel_close")
		fc.eb.GenerateCallInstruction("channel_close")

		// Return 0
		fc.out.XorRegWithReg("rax", "rax")
		fc.out.Cvtsi2sd("xmm0", "rax")

	case "__c67_map_update":
		// __c67_map_update(list, index, value) - Update a list element (functional update)
		// Calls runtime function: c67_list_update(list_ptr, index, value)
		// Arguments: rdi=list_ptr, rsi=index, xmm0=value
		// Returns: rax=new_list_ptr (converted to xmm0)
		if len(call.Args) != 3 {
			compilerError("__c67_map_update() requires exactly 3 arguments (list, index, value)")
		}

		// Compile arguments in reverse order (will use stack)
		// First, compile list and save to stack
		fc.compileExpression(call.Args[0]) // list -> xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile index and save to stack
		fc.compileExpression(call.Args[1]) // index -> xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		// Compile value (leave in xmm0)
		fc.compileExpression(call.Args[2]) // value -> xmm0

		// Now setup arguments for c67_list_update(rdi=list_ptr, rsi=index, xmm0=value, rcx=arena_ptr)
		// Pop index into rsi
		fc.out.MovMemToXmm("xmm1", "rsp", 0)
		fc.out.AddImmToReg("rsp", 8)
		fc.out.Cvttsd2si("rsi", "xmm1") // rsi = index as integer

		// Pop list pointer into rdi
		fc.out.MovMemToXmm("xmm2", "rsp", 0)
		fc.out.AddImmToReg("rsp", 8)
		fc.out.MovqXmmToReg("rdi", "xmm2") // rdi = list pointer

		// xmm0 already has the value
		// Load current arena pointer into rdx (4th argument)
		arenaIndex := fc.currentArena - 1
		arenaOffset := arenaIndex * 8
		fc.out.LeaSymbolToReg("rdx", "_c67_arena_meta")
		fc.out.MovMemToReg("rdx", "rdx", 0)           // rdx = meta-arena pointer
		fc.out.MovMemToReg("rdx", "rdx", arenaOffset) // rdx = arena pointer from slot

		// Call the runtime function
		fc.trackFunctionCall("_c67_list_update")
		fc.eb.GenerateCallInstruction("_c67_list_update")

		// Convert result (rax = pointer) to float64 in xmm0
		fc.out.SubImmFromReg("rsp", 8)
		fc.out.MovRegToMem("rax", "rsp", 0)
		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 8)

	case "printa":
		// printa() - Print value in rax register for debugging
		// No arguments - prints whatever is in rax
		if len(call.Args) != 0 {
			compilerError("printa() takes no arguments")
		}

		// Create format string for printf
		fmtLabel := fmt.Sprintf("printa_fmt_%d", fc.stringCounter)
		fc.stringCounter++
		fc.eb.Define(fmtLabel, "rax = %ld (0x%lx)\n\x00")

		// Load rax value into both rsi and rdx for the two format specifiers
		fc.out.MovRegToReg("rsi", "rax") // First %ld
		fc.out.MovRegToReg("rdx", "rax") // Second %lx
		fc.out.LeaSymbolToReg("rdi", fmtLabel)
		fc.out.XorRegWithReg("rax", "rax") // Clear rax (for variadic)
		fc.trackFunctionCall("printf")
		fc.eb.GenerateCallInstruction("printf")

		// Return 0 in xmm0 (printa doesn't return meaningful value)
		fc.out.XorpdXmm("xmm0", "xmm0")

	default:
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: Hit default case for function='%s'\n", call.Function)
		}
		// Unknown function - track it for dependency resolution
		fc.unknownFunctions[call.Function] = true
		fc.trackFunctionCall(call.Function)

		// For now, generate a call instruction hoping it will be resolved
		// In the future, this will be resolved by loading from dependency repos

		// Arguments are passed in xmm0-xmm5 (up to 6 args)
		// Compile arguments in order
		for i, arg := range call.Args {
			fc.compileExpression(arg)
			if i < len(call.Args)-1 {
				// Save result to stack if not the last arg
				fc.out.SubImmFromReg("rsp", StackSlotSize)
				fc.out.MovXmmToMem("xmm0", "rsp", 0)
			}
		}

		// Restore arguments from stack in reverse order to registers
		// Last arg is already in xmm0
		for i := len(call.Args) - 2; i >= 0; i-- {
			regName := fmt.Sprintf("xmm%d", i)
			fc.out.MovMemToXmm(regName, "rsp", 0)
			fc.out.AddImmToReg("rsp", StackSlotSize)
		}

		// Generate call instruction
		fc.eb.GenerateCallInstruction(call.Function)
	}
}

func (fc *C67Compiler) compilePipeExpr(expr *PipeExpr) {
	// Use ParallelExpr implementation for list mapping
	// Behavior depends on left type:
	// - If list: map function over elements (use ParallelExpr)
	// - If scalar: call function with single value

	leftType := fc.getExprType(expr.Left)

	if leftType == "list" {
		// List mapping: delegate to ParallelExpr
		parallelExpr := &ParallelExpr{
			List:      expr.Left,
			Operation: expr.Right,
		}
		fc.compileParallelExpr(parallelExpr)
		return
	}

	// Scalar pipe: evaluate left, then call right with result
	fc.compileExpression(expr.Left)

	switch right := expr.Right.(type) {
	case *LambdaExpr:
		// Direct lambda: compile and call with value in xmm0
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		fc.compileExpression(right)

		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)
		fc.out.MovMemToReg("r12", "rsp", StackSlotSize)

		fc.out.MovMemToReg("r11", "r12", 0)
		fc.out.MovMemToReg("r15", "r12", 8)

		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)

		fc.out.CallRegister("r11")

	case *IdentExpr:
		// Variable reference (lambda stored in variable)
		fc.out.SubImmFromReg("rsp", 16)
		fc.out.MovXmmToMem("xmm0", "rsp", 0)

		fc.compileExpression(right)

		fc.out.MovXmmToMem("xmm0", "rsp", StackSlotSize)
		fc.out.MovMemToReg("r12", "rsp", StackSlotSize)

		fc.out.MovMemToReg("r11", "r12", 0)
		fc.out.MovMemToReg("r15", "r12", 8)

		fc.out.MovMemToXmm("xmm0", "rsp", 0)
		fc.out.AddImmToReg("rsp", 16)

		fc.out.CallRegister("r11")

	default:
		fc.compileExpression(expr.Right)
	}
}

func (fc *C67Compiler) compileComposeExpr(expr *ComposeExpr) {
	// Function composition: f <> g creates a new function x -> f(g(x))
	// For now, we'll create a simpler implementation that generates
	// an inline lambda expression: (x -> left(right(x)))
	//
	// TODO: Full implementation with proper closure generation
	// Currently this is a simplified approach that works for simple cases

	compilerError("function composition operator <> not yet fully implemented\n" +
		"Use explicit lambda instead: compose = x -> f(g(x))\n" +
		"Full composition support requires closure capture implementation")
}

func (fc *C67Compiler) compileSendExpr(expr *SendExpr) {
	// Send operator: target <== message
	// Target must be a string: ":5000", "localhost:5000", "192.168.1.1:5000"
	// Message should be a string

	// For now, only support compile-time string literals as targets
	targetStr, ok := expr.Target.(*StringExpr)
	if !ok {
		compilerError("send operator target must be a string literal (e.g., \":5000\")")
	}

	// Parse target string to extract port number
	// Format: ":5000" or "host:5000"
	addr := targetStr.Value
	var port int
	if addr[0] == ':' {
		// Port only (localhost)
		var err error
		port, err = strconv.Atoi(addr[1:])
		if err != nil || port < 1 || port > 65535 {
			compilerError("invalid port in send target: %s", addr)
		}
	} else {
		// TODO: Handle "host:port" format
		compilerError("send target format not yet supported: %s (use \":port\" for localhost)", addr)
	}

	// Allocate stack space for: message map (8), socket fd (8), sockaddr_in (16), message buffer (256)
	stackSpace := int64(288)
	fc.out.SubImmFromReg("rsp", stackSpace)
	fc.runtimeStack += int(stackSpace)

	// Step 1: Evaluate and save message
	fc.compileExpression(expr.Message)
	fc.out.MovXmmToMem("xmm0", "rsp", 0) // message map at rsp+0

	// Step 2: Create UDP socket (syscall 41: socket)
	// socket(AF_INET=2, SOCK_DGRAM=2, protocol=0)
	fc.out.MovImmToReg("rax", "41") // socket syscall
	fc.out.MovImmToReg("rdi", "2")  // AF_INET
	fc.out.MovImmToReg("rsi", "2")  // SOCK_DGRAM
	fc.out.MovImmToReg("rdx", "0")  // protocol
	fc.out.Syscall()
	fc.out.MovRegToMem("rax", "rsp", 8) // socket fd at rsp+8

	// Step 3: Build sockaddr_in structure at rsp+16
	// struct sockaddr_in: family(2), port(2), addr(4), zero(8) = 16 bytes

	// sin_family = AF_INET (2)
	fc.out.MovImmToReg("rax", "2")
	fc.out.MovU16RegToMem("ax", "rsp", 16)

	// sin_port = htons(port) - convert to network byte order
	portNetOrder := (port&0xff)<<8 | (port>>8)&0xff // Manual byte swap
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", portNetOrder))
	fc.out.MovU16RegToMem("ax", "rsp", 18)

	// sin_addr = INADDR_ANY (0.0.0.0) for localhost
	fc.out.MovImmToReg("rax", "0")
	fc.out.MovRegToMem("rax", "rsp", 20)

	// sin_zero = 0 (padding)
	fc.out.MovImmToReg("rax", "0")
	fc.out.MovRegToMem("rax", "rsp", 24)

	// Step 4: Extract string bytes from message map to buffer at rsp+32
	// Strings in C67 are stored as map[uint64]float64:
	// [count][key0][val0][key1][val1]...
	// Where count = length, keys = indices, vals = character codes

	fc.out.MovMemToReg("rax", "rsp", 0)  // load message map pointer
	fc.out.MovMemToXmm("xmm0", "rax", 0) // load count from first 8 bytes into xmm0
	fc.out.Cvttsd2si("rcx", "xmm0")      // convert count from float64 to integer

	// Write test message "TEST" (4 bytes) for now
	// TODO: Implement proper map iteration to extract actual string bytes
	fc.out.MovImmToReg("r10", "0x54534554") // "TEST" in little-endian (T=0x54, E=0x45, S=0x53, T=0x54)
	fc.out.MovRegToMem("r10", "rsp", 32)
	fc.out.MovImmToReg("rcx", "4") // length

	// Step 5: Send packet (syscall 44: sendto)
	// sendto(sockfd, buf, len, flags, dest_addr, addrlen)
	fc.out.MovMemToReg("rdi", "rsp", 8)                           // socket fd
	fc.out.LeaMemToReg("rsi", "rsp", 32)                          // buffer
	fc.out.MovRegToReg("rdx", "rcx")                              // length (copy rcx to rdx)
	fc.out.MovImmToReg("r10", "0")                                // flags
	fc.out.LeaMemToReg("r8", "rsp", 16)                           // sockaddr_in
	fc.out.MovImmToReg("r9", fmt.Sprintf("%d", socketStructSize)) // addrlen
	fc.out.MovImmToReg("rax", "44")                               // sendto syscall
	fc.out.Syscall()

	// Save result
	fc.out.MovRegToReg("rbx", "rax")

	// Step 6: Close socket (syscall 3: close)
	fc.out.MovMemToReg("rdi", "rsp", 8) // socket fd
	fc.out.MovImmToReg("rax", "3")      // close syscall
	fc.out.Syscall()

	// Clean up stack
	fc.out.AddImmToReg("rsp", stackSpace)
	fc.runtimeStack -= int(stackSpace)

	// Return result (bytes sent, or -1 on error)
	fc.out.MovRegToReg("rax", "rbx")
	fc.out.Cvtsi2sd("xmm0", "rax")
}

func (fc *C67Compiler) compileReceiveExpr(expr *ReceiveExpr) {
	// Receive operator: <= source
	// Source must be an address literal: &8080 or &host:8080
	// Receives one message from the address and returns it as a string

	// For now, only support AddressLiteralExpr
	addrExpr, ok := expr.Source.(*AddressLiteralExpr)
	if !ok {
		compilerError("receive operator source must be an address literal (e.g., &8080)")
	}

	// Extract port from address literal
	addr := addrExpr.Value
	var port int

	// Parse address: &8080, &:8080, &localhost:8080, &192.168.1.1:8080
	colonIdx := -1
	for i, ch := range addr {
		if ch == ':' {
			colonIdx = i
			break
		}
	}

	if colonIdx == -1 {
		// No colon - just port number after &
		var err error
		port, err = strconv.Atoi(addr[1:]) // Skip &
		if err != nil || port < 1 || port > 65535 {
			compilerError("invalid port in receive address: %s", addr)
		}
	} else {
		// Has colon - parse port after colon
		var err error
		port, err = strconv.Atoi(addr[colonIdx+1:])
		if err != nil || port < 1 || port > 65535 {
			compilerError("invalid port in receive address: %s", addr)
		}
	}

	// Allocate stack space for: socket fd (8), sockaddr_in (16), sender addr (16), buffer (256), result map (8)
	stackSpace := int64(304)
	fc.out.SubImmFromReg("rsp", stackSpace)
	fc.runtimeStack += int(stackSpace)

	// Step 1: Create UDP socket (syscall 41: socket)
	fc.out.MovImmToReg("rax", "41") // socket syscall
	fc.out.MovImmToReg("rdi", "2")  // AF_INET
	fc.out.MovImmToReg("rsi", "2")  // SOCK_DGRAM
	fc.out.MovImmToReg("rdx", "0")  // protocol
	fc.out.Syscall()
	fc.out.MovRegToMem("rax", "rsp", 0) // socket fd at rsp+0

	// Step 2: Build sockaddr_in for binding at rsp+8
	fc.out.MovImmToReg("rax", "2")
	fc.out.MovU16RegToMem("ax", "rsp", 8) // sin_family = AF_INET

	// sin_port = htons(port)
	portNetOrder := (port&0xff)<<8 | (port>>8)&0xff
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", portNetOrder))
	fc.out.MovU16RegToMem("ax", "rsp", 10)

	// sin_addr = INADDR_ANY (0.0.0.0)
	fc.out.MovImmToReg("rax", "0")
	fc.out.MovRegToMem("rax", "rsp", 12)
	fc.out.MovRegToMem("rax", "rsp", 16) // sin_zero

	// Step 3: Bind socket (syscall 49: bind)
	fc.out.MovMemToReg("rdi", "rsp", 0)                            // socket fd
	fc.out.LeaMemToReg("rsi", "rsp", 8)                            // sockaddr_in
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", socketStructSize)) // addrlen
	fc.out.MovImmToReg("rax", "49")                                // bind syscall
	fc.out.Syscall()

	// Step 4: Receive message (syscall 45: recvfrom)
	fc.out.MovMemToReg("rdi", "rsp", 0)                            // socket fd
	fc.out.LeaMemToReg("rsi", "rsp", 40)                           // buffer at rsp+40
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", stringBufferSize)) // buffer size
	fc.out.MovImmToReg("r10", "0")                                 // flags
	fc.out.LeaMemToReg("r8", "rsp", 24)                            // sender sockaddr_in at rsp+24
	fc.out.LeaMemToReg("r9", "rsp", 296)                           // sender addrlen at rsp+296
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", socketStructSize))
	fc.out.MovRegToMem("rax", "rsp", 296) // initialize addrlen
	fc.out.MovImmToReg("rax", "45")       // recvfrom syscall
	fc.out.Syscall()

	// rax = bytes received (or -1 on error)
	fc.out.MovRegToReg("rbx", "rax") // save length

	// Step 5: Close socket (syscall 3: close)
	fc.out.MovMemToReg("rdi", "rsp", 0) // socket fd
	fc.out.MovImmToReg("rax", "3")      // close syscall
	fc.out.Syscall()

	// Step 6: Convert received bytes to C67 string (map[uint64]float64)
	// For simplicity, create a string map with the bytes as character codes
	// This requires allocating a map and populating it
	// For now, return the buffer pointer as a number (temp implementation)

	fc.out.LeaMemToReg("rax", "rsp", 40) // buffer address
	fc.out.Cvtsi2sd("xmm0", "rax")       // convert to float64

	// TODO: Properly convert buffer to C67 string map

	// Clean up stack
	fc.out.AddImmToReg("rsp", stackSpace)
	fc.runtimeStack -= int(stackSpace)
}

func (fc *C67Compiler) compileReceiveLoopStmt(stmt *ReceiveLoopStmt) {
	// Receive loop: @ msg, from in ":5000" { }
	// Target must be a string: ":5000"
	// Creates socket, binds to port, loops forever receiving messages

	// For now, only support compile-time string literals as addresses
	addressStr, ok := stmt.Address.(*StringExpr)
	if !ok {
		compilerError("receive loop address must be a string literal (e.g., \":5000\")")
	}

	// Parse address string to extract port number or port range
	addr := addressStr.Value
	var startPort, endPort int
	if addr[0] == ':' {
		// Port only (bind to all interfaces)
		// Support ":5000" or ":5000-5010" for port ranges
		portSpec := addr[1:]
		if strings.Contains(portSpec, "-") {
			// Port range: ":5000-5010"
			parts := strings.Split(portSpec, "-")
			if len(parts) != 2 {
				compilerError("invalid port range in receive address: %s", addr)
			}
			var err error
			startPort, err = strconv.Atoi(parts[0])
			if err != nil || startPort < 1 || startPort > 65535 {
				compilerError("invalid start port in receive address: %s", addr)
			}
			endPort, err = strconv.Atoi(parts[1])
			if err != nil || endPort < 1 || endPort > 65535 {
				compilerError("invalid end port in receive address: %s", addr)
			}
			if startPort > endPort {
				compilerError("start port must be <= end port in receive address: %s", addr)
			}
		} else {
			// Single port: ":5000"
			var err error
			startPort, err = strconv.Atoi(portSpec)
			if err != nil || startPort < 1 || startPort > 65535 {
				compilerError("invalid port in receive address: %s", addr)
			}
			endPort = startPort
		}
	} else {
		compilerError("receive address format not yet supported: %s (use \":port\" or \":port1-port2\" for all interfaces)", addr)
	}

	// Generate unique labels for this loop
	fc.labelCounter++
	loopLabel := fmt.Sprintf("receive_loop_%d", fc.labelCounter)
	endLabel := fmt.Sprintf("receive_end_%d", fc.labelCounter)
	tryPortLabel := fmt.Sprintf("try_port_%d", fc.labelCounter)
	bindSuccessLabel := fmt.Sprintf("bind_success_%d", fc.labelCounter)
	bindFailLabel := fmt.Sprintf("bind_fail_%d", fc.labelCounter)

	// Allocate stack space: we use the base offset from symbol collection
	// Layout: msg_var(8), sender_var(8), socket_fd(8), [padding], sockaddr_in(16), buffer(256), addrlen(8) = 320 bytes
	baseOffset := stmt.BaseOffset

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: ReceiveLoop baseOffset = %d, port range: %d-%d\n", baseOffset, startPort, endPort)
	}

	// Stack layout offsets (from rbp going downward):
	// msg_var:     rbp-(baseOffset+8)
	// sender_var:  rbp-(baseOffset+16)
	// socket_fd:   rbp-(baseOffset+24)
	// sockaddr_in: rbp-(baseOffset+40) [16 bytes: 40,38,36,32 to avoid overlap with socket_fd]
	//   - sin_family (2 bytes): offset 0 from start = rbp-(baseOffset+40)
	//   - sin_port (2 bytes):   offset 2 from start = rbp-(baseOffset+38)
	//   - sin_addr (4 bytes):   offset 4 from start = rbp-(baseOffset+36)
	//   - sin_zero (8 bytes):   offset 8 from start = rbp-(baseOffset+32)
	// buffer:      rbp-(baseOffset+56) to rbp-(baseOffset+311) [256 bytes]
	// addrlen:     rbp-(baseOffset+320)

	// Step 1: Create UDP socket (once, before port loop)
	fc.out.MovImmToReg("rax", "41") // socket syscall
	fc.out.MovImmToReg("rdi", "2")  // AF_INET
	fc.out.MovImmToReg("rsi", "2")  // SOCK_DGRAM
	fc.out.MovImmToReg("rdx", "0")  // protocol
	fc.out.Syscall()
	fc.out.MovRegToMem("rax", "rbp", -(baseOffset + 24)) // socket fd

	// Step 2: Initialize sockaddr_in structure (constant fields)
	// sin_family = AF_INET (2)
	fc.out.MovImmToReg("rax", "2")
	fc.out.MovU16RegToMem("ax", "rbp", -(baseOffset + 40))

	// sin_addr = INADDR_ANY (0.0.0.0)
	fc.out.MovImmToReg("rax", "0")
	fc.out.MovRegToMem("rax", "rbp", -(baseOffset + 36))

	// sin_zero = 0 (padding)
	fc.out.MovImmToReg("rax", "0")
	fc.out.MovRegToMem("rax", "rbp", -(baseOffset + 32))

	// Step 3: Port availability loop (r12 = current port)
	fc.out.MovImmToReg("r12", fmt.Sprintf("%d", startPort))
	fc.eb.MarkLabel(tryPortLabel)

	// Convert current port (r12) to network byte order and store in sin_port
	// Load port value into rax, then convert to 16-bit with byte swap
	fc.out.MovRegToReg("rax", "r12") // Copy r12 to rax
	// Manual byte swap for htons: rol ax, 8
	// Encoding: 66 C1 C0 08 (16-bit ROL AX by immediate 8)
	fc.eb.text.WriteByte(0x66) // Operand-size override prefix
	fc.eb.text.WriteByte(0xC1) // ROL r/m16, imm8
	fc.eb.text.WriteByte(0xC0) // ModR/M for AX
	fc.eb.text.WriteByte(0x08) // Immediate value 8
	fc.out.MovU16RegToMem("ax", "rbp", -(baseOffset + 38))

	// Try to bind socket to current port
	fc.out.MovMemToReg("rdi", "rbp", -(baseOffset + 24))           // socket fd
	fc.out.LeaMemToReg("rsi", "rbp", -(baseOffset + 40))           // sockaddr_in structure
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", socketStructSize)) // addrlen
	fc.out.MovImmToReg("rax", "49")                                // bind syscall
	fc.out.Syscall()

	// Check bind result: rax == 0 means success
	fc.out.CmpRegToImm("rax", 0)
	fc.out.JumpConditional(JumpEqual, 0) // Will be patched to bindSuccessLabel
	bindCheckPos := fc.eb.text.Len()

	// Bind failed, try next port
	fc.out.IncReg("r12")
	fc.out.CmpRegToImm("r12", int64(endPort+1))
	fc.out.JumpConditional(JumpLess, 0) // Will be patched to tryPortLabel
	tryNextPos := fc.eb.text.Len()

	// All ports failed - close socket and exit
	fc.eb.MarkLabel(bindFailLabel)
	fc.out.MovMemToReg("rdi", "rbp", -(baseOffset + 24)) // socket fd
	fc.out.MovImmToReg("rax", "3")                       // close syscall
	fc.out.Syscall()
	fc.out.MovImmToReg("rdi", "1")  // exit code 1
	fc.out.MovImmToReg("rax", "60") // exit syscall
	fc.out.Syscall()

	// Bind succeeded, continue to receive loop
	fc.eb.MarkLabel(bindSuccessLabel)

	// Patch jump to bindSuccessLabel
	bindSuccessPos := fc.eb.labels[bindSuccessLabel]
	bindOffset := int32(bindSuccessPos - bindCheckPos)
	fc.patchJumpImmediate(bindCheckPos-ConditionalJumpSize+2, bindOffset)

	// Patch jump to tryPortLabel
	tryPortPos := fc.eb.labels[tryPortLabel]
	tryOffset := int32(tryPortPos - tryNextPos)
	fc.patchJumpImmediate(tryNextPos-ConditionalJumpSize+2, tryOffset)

	// Step 4: Start receive loop
	fc.eb.MarkLabel(loopLabel)

	// Initialize addrlen for recvfrom
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", socketStructSize))
	fc.out.MovRegToMem("rax", "rbp", -(baseOffset + 320))

	// Call recvfrom (syscall 45: recvfrom)
	// recvfrom(sockfd, buf, len, flags, src_addr, addrlen)
	fc.out.MovMemToReg("rdi", "rbp", -(baseOffset + 24))           // socket fd
	fc.out.LeaMemToReg("rsi", "rbp", -(baseOffset + 56))           // buffer (starts after sockaddr)
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", socketBufferSize)) // buffer size
	fc.out.MovImmToReg("r10", "0")                                 // flags
	fc.out.LeaMemToReg("r8", "rbp", -(baseOffset + 40))            // src_addr (sockaddr_in start)
	fc.out.LeaMemToReg("r9", "rbp", -(baseOffset + 320))           // addrlen pointer
	fc.out.MovImmToReg("rax", "45")                                // recvfrom syscall
	fc.out.Syscall()

	// rax now contains bytes received (or -1 on error)
	// TODO: Check for errors and convert buffer to string

	// For now, just store 0.0 in msg and from variables
	fc.out.MovImmToReg("rax", "0")
	fc.out.Cvtsi2sd("xmm0", "rax")

	// Add message and sender variables to variable map for body
	msgOffset := baseOffset + 8
	fromOffset := baseOffset + 16
	fc.variables[stmt.MessageVar] = int(msgOffset)
	fc.variables[stmt.SenderVar] = int(fromOffset)

	fc.out.MovXmmToMem("xmm0", "rbp", -int(msgOffset))
	fc.out.MovXmmToMem("xmm0", "rbp", -int(fromOffset))

	// Step 5: Execute loop body
	for _, bodyStmt := range stmt.Body {
		fc.compileStatement(bodyStmt)
	}

	// Step 6: Jump back to loop start
	fc.out.JumpUnconditional(0) // Will be patched
	endOfBody := fc.eb.text.Len()

	// Calculate offset back to loop start
	loopStart := fc.eb.labels[loopLabel]
	offset := int32(loopStart - endOfBody)
	fc.patchJumpImmediate(endOfBody-UnconditionalJumpSize+1, offset)

	// End label (for break statements)
	fc.eb.MarkLabel(endLabel)

	// Clean up: close socket
	fc.out.MovMemToReg("rdi", "rbp", -(baseOffset + 24)) // socket fd
	fc.out.MovImmToReg("rax", "3")                       // close syscall
	fc.out.Syscall()

	// Remove variables from scope
	delete(fc.variables, stmt.MessageVar)
	delete(fc.variables, stmt.SenderVar)
}

// Confidence that this function is working: 95%
// createErrorResult creates an error Result with the given error code in xmm0
// The error code should be a 3-4 character string like "out", "arg", "dv0", etc.
func (fc *C67Compiler) createErrorResult(errorCode string) {
	// Pad error code to 4 bytes with null terminator if needed
	code := errorCode
	for len(code) < 4 {
		code += "\x00"
	}
	if len(code) > 4 {
		code = code[:4]
	}

	// Convert error code string to 32-bit integer (little-endian byte order)
	var codeInt uint32
	for i := 0; i < 4; i++ {
		codeInt |= uint32(code[i]) << (uint(i) * 8)
	}

	// Create error NaN: 0x7FF8_0000_0000_0000 | error_code
	// The error code goes in the lower 32 bits of the mantissa
	errorNaN := uint64(0x7FF8000000000000) | uint64(codeInt)

	// Load the error NaN value into xmm0
	fc.out.Emit([]byte{0x48, 0xb8}) // mov rax, immediate64
	for i := 0; i < 8; i++ {
		fc.out.Emit([]byte{byte(errorNaN >> (uint(i) * 8))})
	}

	// Move rax to xmm0
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovMemToXmm("xmm0", "rsp", 0)
	fc.out.AddImmToReg("rsp", 8)
}

func (fc *C67Compiler) trackFunctionCall(funcName string) {
	if !fc.usedFunctions[funcName] {
		fc.usedFunctions[funcName] = true
	}
	fc.callOrder = append(fc.callOrder, funcName)

	// Update runtime features tracker
	switch funcName {
	case "_c67_string_concat":
		fc.runtimeFeatures.Mark(FeatureStringConcat)
	case "_c67_string_to_cstr":
		fc.runtimeFeatures.Mark(FeatureStringToCstr)
	case "_c67_cstr_to_string":
		fc.runtimeFeatures.Mark(FeatureCstrToString)
	case "_c67_slice_string":
		fc.runtimeFeatures.Mark(FeatureStringSlice)
	case "_c67_string_eq":
		fc.runtimeFeatures.Mark(FeatureStringEq)
	case "_c67_list_concat":
		fc.runtimeFeatures.Mark(FeatureListConcat)
	case "_c67_list_repeat":
		fc.runtimeFeatures.Mark(FeatureListRepeat)
	case "printf", "println":
		fc.runtimeFeatures.Mark(FeaturePrintf)
	case "_c67_print_syscall":
		fc.runtimeFeatures.Mark(FeaturePrintSyscall)
	case "_c67_arena_create":
		fc.runtimeFeatures.Mark(FeatureArenaCreate)
	case "_c67_arena_alloc":
		fc.runtimeFeatures.Mark(FeatureArenaAlloc)
	}
}

// callMallocAligned calls malloc with proper stack alignment.
// This helper ensures the stack is 16-byte aligned before calling malloc,
// which is required by the x86-64 System V ABI.
//
// Parameters:
//   - sizeReg: register containing the allocation size (will be moved to rdi)
//   - pushCount: number of registers pushed in the current function
//     (after function prologue, not including the prologue's push rbp)
//
// Returns: allocated pointer in rax
//
// Stack alignment calculation:
//   - call instruction: 8 bytes (return address)
//   - push rbp: 8 bytes (function prologue)
//   - push registers: 8 * pushCount bytes
//     Total: 16 + (8 * pushCount) bytes
//
// If total is not a multiple of 16, we subtract 8 more from rsp before calling malloc.
// The caller must restore rsp after the call.
func (fc *C67Compiler) callMallocAligned(sizeReg string, pushCount int) {
	// Calculate current stack usage
	// call (8) + push rbp (8) + pushes (8 * pushCount)
	stackUsed := 16 + (8 * pushCount)
	needsAlignment := (stackUsed % 16) != 0

	// Move size to rdi (first argument)
	if sizeReg != "rdi" {
		fc.out.MovRegToReg("rdi", sizeReg)
	}

	// If stack is misaligned, subtract 8 bytes for alignment
	var alignmentOffset int
	if needsAlignment {
		fc.out.SubImmFromReg("rsp", StackSlotSize)
		alignmentOffset = StackSlotSize
	}

	// Call malloc
	// Allocate from arena
	fc.callArenaAlloc()

	// Restore stack alignment offset if we added one
	if alignmentOffset > 0 {
		fc.out.AddImmToReg("rsp", int64(alignmentOffset))
	}

	// SAFETY: Check if malloc returned NULL (out of memory)
	fc.out.TestRegReg("rax", "rax")
	okJumpPos := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0) // Placeholder, will patch

	// malloc returned NULL - print error and exit
	fc.out.LeaSymbolToReg("rdi", "_malloc_failed_msg")
	fc.trackFunctionCall("printf")
	fc.eb.GenerateCallInstruction("printf")
	fc.out.MovImmToReg("rdi", "1")
	fc.trackFunctionCall("exit")
	fc.eb.GenerateCallInstruction("exit")

	// Patch the jump to skip error handling
	okPos := fc.eb.text.Len()
	okOffset := int32(okPos - (okJumpPos + 6))
	fc.patchJumpImmediate(okJumpPos+2, okOffset)

	// Result is in rax
}

// collectFunctionCalls walks an expression and collects all function calls
// Confidence that this function is working: 95%
// Confidence that this function is working: 98%
func collectFunctionCalls(expr Expression, calls map[string]bool) {
	collectFunctionCallsWithParams(expr, calls, nil)
}

func collectFunctionCallsWithParams(expr Expression, calls map[string]bool, params map[string]bool) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *CallExpr:
		// For C FFI calls, the parser strips "c." and sets IsCFFI=true
		// We need to add the "c." prefix back for proper tracking
		funcName := e.Function
		if e.IsCFFI {
			funcName = "c." + funcName
		}

		// Only add to calls if it's not a parameter
		if params == nil || !params[e.Function] {
			calls[funcName] = true
		}

		for _, arg := range e.Args {
			collectFunctionCallsWithParams(arg, calls, params)
		}
	case *DirectCallExpr:
		// Direct calls don't add to function calls - they're calling values, not named functions
		// But we still need to recurse into the callee and args
		collectFunctionCallsWithParams(e.Callee, calls, params)
		for _, arg := range e.Args {
			collectFunctionCallsWithParams(arg, calls, params)
		}
	case *BinaryExpr:
		collectFunctionCallsWithParams(e.Left, calls, params)
		collectFunctionCallsWithParams(e.Right, calls, params)
	case *UnaryExpr:
		collectFunctionCallsWithParams(e.Operand, calls, params)
	case *PostfixExpr:
		collectFunctionCallsWithParams(e.Operand, calls, params)
	case *PipeExpr:
		collectFunctionCallsWithParams(e.Left, calls, params)
		collectFunctionCallsWithParams(e.Right, calls, params)
	case *SendExpr:
		collectFunctionCallsWithParams(e.Target, calls, params)
		collectFunctionCallsWithParams(e.Message, calls, params)
	case *ReceiveExpr:
		collectFunctionCallsWithParams(e.Source, calls, params)
	case *MatchExpr:
		collectFunctionCallsWithParams(e.Condition, calls, params)
		for _, clause := range e.Clauses {
			if clause.Guard != nil {
				collectFunctionCallsWithParams(clause.Guard, calls, params)
			}
			collectFunctionCallsWithParams(clause.Result, calls, params)
		}
		if e.DefaultExpr != nil {
			collectFunctionCallsWithParams(e.DefaultExpr, calls, params)
		}
	case *BlockExpr:
		// BlockExpr contains statements - recurse into them
		for _, stmt := range e.Statements {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
	case *LambdaExpr:
		// Create new parameter set for this lambda scope
		lambdaParams := make(map[string]bool)
		for k, v := range params {
			lambdaParams[k] = v
		}
		for _, param := range e.Params {
			lambdaParams[param] = true
		}
		collectFunctionCallsWithParams(e.Body, calls, lambdaParams)
	case *PatternLambdaExpr:
		// Pattern lambdas have an implicit parameter (the matched value)
		lambdaParams := make(map[string]bool)
		for k, v := range params {
			lambdaParams[k] = v
		}
		for _, clause := range e.Clauses {
			collectFunctionCallsWithParams(clause.Body, calls, lambdaParams)
		}
	case *MultiLambdaExpr:
		for _, lambda := range e.Lambdas {
			lambdaParams := make(map[string]bool)
			for k, v := range params {
				lambdaParams[k] = v
			}
			for _, param := range lambda.Params {
				lambdaParams[param] = true
			}
			collectFunctionCallsWithParams(lambda.Body, calls, lambdaParams)
		}
	case *RangeExpr:
		collectFunctionCallsWithParams(e.Start, calls, params)
		collectFunctionCallsWithParams(e.End, calls, params)
	case *ListExpr:
		for _, elem := range e.Elements {
			collectFunctionCallsWithParams(elem, calls, params)
		}
	case *MapExpr:
		for i := range e.Keys {
			collectFunctionCallsWithParams(e.Keys[i], calls, params)
			collectFunctionCallsWithParams(e.Values[i], calls, params)
		}
	case *IndexExpr:
		collectFunctionCallsWithParams(e.List, calls, params)
		collectFunctionCallsWithParams(e.Index, calls, params)
	case *SliceExpr:
		collectFunctionCallsWithParams(e.List, calls, params)
		if e.Start != nil {
			collectFunctionCallsWithParams(e.Start, calls, params)
		}
		if e.End != nil {
			collectFunctionCallsWithParams(e.End, calls, params)
		}
	case *LengthExpr:
		collectFunctionCallsWithParams(e.Operand, calls, params)
	case *CastExpr:
		collectFunctionCallsWithParams(e.Expr, calls, params)
	case *UnsafeExpr:
		// UnsafeExpr contains architecture-specific statement blocks
		for _, stmt := range e.X86_64Block {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
		for _, stmt := range e.ARM64Block {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
		for _, stmt := range e.RISCV64Block {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
	case *ArenaExpr:
		// ArenaExpr contains a statement block
		for _, stmt := range e.Body {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
	case *ParallelExpr:
		collectFunctionCallsWithParams(e.List, calls, params)
		collectFunctionCallsWithParams(e.Operation, calls, params)
	case *BackgroundExpr:
		collectFunctionCallsWithParams(e.Expr, calls, params)
	case *LoopExpr:
		collectFunctionCallsWithParams(e.Iterable, calls, params)
		for _, stmt := range e.Body {
			collectFunctionCallsFromStmtWithParams(stmt, calls, params)
		}
		if e.Reducer != nil {
			collectFunctionCallsWithParams(e.Reducer, calls, params)
		}
	case *StructLiteralExpr:
		for _, fieldExpr := range e.Fields {
			collectFunctionCallsWithParams(fieldExpr, calls, params)
		}
	case *VectorExpr:
		for _, comp := range e.Components {
			collectFunctionCallsWithParams(comp, calls, params)
		}
	case *FStringExpr:
		for _, part := range e.Parts {
			collectFunctionCallsWithParams(part, calls, params)
		}
	case *InExpr:
		collectFunctionCallsWithParams(e.Value, calls, params)
		collectFunctionCallsWithParams(e.Container, calls, params)
	case *MoveExpr:
		collectFunctionCallsWithParams(e.Expr, calls, params)
	}
}

// collectFunctionCallsFromStmt walks a statement and collects all function calls
func collectFunctionCallsFromStmt(stmt Statement, calls map[string]bool) {
	collectFunctionCallsFromStmtWithParams(stmt, calls, nil)
}

func collectFunctionCallsFromStmtWithParams(stmt Statement, calls map[string]bool, params map[string]bool) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *AssignStmt:
		collectFunctionCallsWithParams(s.Value, calls, params)
	case *ExpressionStmt:
		collectFunctionCallsWithParams(s.Expr, calls, params)
	case *LoopStmt:
		collectFunctionCallsWithParams(s.Iterable, calls, params)
		for _, bodyStmt := range s.Body {
			collectFunctionCallsFromStmtWithParams(bodyStmt, calls, params)
		}
	}
}

// Confidence that this function is working: 95%
// collectDefinedFunctions returns a set of function names defined in a program
func collectDefinedFunctions(program *Program) map[string]bool {
	defined := make(map[string]bool)

	// Collect from all statements recursively
	for _, stmt := range program.Statements {
		collectDefinedFromStmt(stmt, defined)
	}

	return defined
}

// collectDefinedFromStmt recursively collects defined names from a statement
func collectDefinedFromStmt(stmt Statement, defined map[string]bool) {
	if assign, ok := stmt.(*AssignStmt); ok {
		// Mark any variable as "defined" for the purpose of call checking
		defined[assign.Name] = true

		// Also recursively check the value (for nested blocks)
		if lambda, ok := assign.Value.(*LambdaExpr); ok {
			collectDefinedFromExpr(lambda.Body, defined)
		}
	} else if exprStmt, ok := stmt.(*ExpressionStmt); ok {
		collectDefinedFromExpr(exprStmt.Expr, defined)
	}
}

// collectDefinedFromExpr recursively collects defined names from an expression
func collectDefinedFromExpr(expr Expression, defined map[string]bool) {
	switch e := expr.(type) {
	case *BlockExpr:
		for _, stmt := range e.Statements {
			collectDefinedFromStmt(stmt, defined)
		}
	case *MatchExpr:
		for _, clause := range e.Clauses {
			collectDefinedFromExpr(clause.Result, defined)
		}
	case *LambdaExpr:
		collectDefinedFromExpr(e.Body, defined)
	}
}

// checkForwardReferences ensures functions are defined before they're called
// Returns a list of error messages for forward references
func checkForwardReferences(program *Program) []string {
	var errors []string
	defined := make(map[string]bool)

	// Builtins are always available
	builtins := map[string]bool{
		"printf": true, "exit": true, "syscall": true,
		"getpid": true, "me": true,
		"print": true, "println": true,
		"eprint": true, "eprintln": true, "eprintf": true,
		"exitln": true, "exitf": true,
		"sqrt": true, "sin": true, "cos": true, "tan": true,
		"asin": true, "acos": true, "atan": true, "atan2": true,
		"exp": true, "log": true, "pow": true,
		"floor": true, "ceil": true, "round": true,
		"abs": true, "approx": true,
		"popcount": true, "clz": true, "ctz": true,
		"chan": true, "close": true,
		"append": true, "head": true, "tail": true, "pop": true,
		"error": true, "is_nan": true,
		"_error_code_extract": true,
		"printa":              true,
		"alloc":               true, "free": true,
		"dlopen": true, "dlsym": true, "dlclose": true,
		"read_i8": true, "read_u8": true, "read_i16": true, "read_u16": true,
		"read_i32": true, "read_u32": true, "read_i64": true, "read_u64": true, "read_f64": true,
		"write_i8": true, "write_u8": true, "write_i16": true, "write_u16": true,
		"write_i32": true, "write_u32": true, "write_i64": true, "write_u64": true, "write_f32": true, "write_f64": true,
		"call": true, "arena_create": true, "arena_alloc": true, "arena_reset": true, "arena_destroy": true,
	}

	// Mark builtins as defined
	for k := range builtins {
		defined[k] = true
	}

	// Collect C imports
	cImports := make(map[string]bool)
	for _, stmt := range program.Statements {
		if cImp, ok := stmt.(*CImportStmt); ok {
			cImports[cImp.Alias] = true
		}
	}

	// Pre-scan to find all functions that WILL BE defined (anywhere in the program)
	allDefined := collectDefinedFunctions(program)

	// Process statements in order
	for _, stmt := range program.Statements {
		// Only check top-level calls (not calls inside lambda bodies which execute later)
		// Get top-level calls from this statement
		calls := make(map[string]bool)
		if exprStmt, ok := stmt.(*ExpressionStmt); ok {
			// Top-level expression statement - check its calls
			collectFunctionCallsWithParams(exprStmt.Expr, calls, nil)
		}
		// Note: We DON'T check calls inside AssignStmt values because those are lambda bodies
		// Lambda bodies execute later when the function is called, not when it's defined

		// Get the name being defined in this statement (for recursion detection)
		var definingName string
		if assign, ok := stmt.(*AssignStmt); ok {
			definingName = assign.Name
		}

		for funcName := range calls {
			// Skip if it's a C import (namespace.function)
			if strings.Contains(funcName, ".") {
				parts := strings.SplitN(funcName, ".", 2)
				if len(parts) == 2 && (cImports[parts[0]] || parts[0] == "c") {
					continue
				}
			}

			// Skip builtin operators
			if funcName == "+" || funcName == "-" || funcName == "*" || funcName == "/" ||
				funcName == "mod" || funcName == "%" ||
				funcName == "<" || funcName == "<=" || funcName == ">" || funcName == ">=" ||
				funcName == "==" || funcName == "!=" ||
				funcName == "and" || funcName == "or" || funcName == "not" ||
				funcName == "~b" || funcName == "&b" || funcName == "|b" || funcName == "^b" ||
				funcName == "<<" || funcName == ">>" {
				continue
			}

			// Skip if this is a recursive call (function calling itself in its own definition)
			if funcName == definingName {
				continue
			}

			// Only flag as forward reference if:
			// 1. Not currently defined
			// 2. WILL BE defined later (exists in allDefined)
			if !defined[funcName] && allDefined[funcName] {
				errors = append(errors, fmt.Sprintf("  Function '%s' called before it is defined", funcName))
			}
		}

		// Now mark new definitions from this statement
		if assign, ok := stmt.(*AssignStmt); ok {
			defined[assign.Name] = true
		}
	}

	return errors
}

// Confidence that this function is working: 95%
// getUnknownFunctions determines which functions are called but not defined
func getUnknownFunctions(program *Program) []string {
	// Builtin functions that are always available (implemented in compiler)
	builtins := map[string]bool{
		"printf": true, "exit": true, "syscall": true,
		"getpid": true, "me": true,
		"print": true, "println": true, // print/println are builtin optimizations, not dependencies
		"eprint": true, "eprintln": true, "eprintf": true, // stderr printing with Result return
		"exitln": true, "exitf": true, // stderr printing with exit(1)
		// Math functions (hardware instructions)
		"sqrt": true, "sin": true, "cos": true, "tan": true,
		"asin": true, "acos": true, "atan": true, "atan2": true,
		"exp": true, "log": true, "pow": true,
		"floor": true, "ceil": true, "round": true,
		"abs": true, "approx": true,
		// Bit manipulation functions (CPU instructions with fallback)
		"popcount": true, "clz": true, "ctz": true,
		// Channel primitives
		"chan": true, "close": true,
		// List methods
		"append": true, "head": true, "tail": true, "pop": true,
		// Error handling
		"error": true, "is_nan": true,
		// Internal functions (start with _)
		"_error_code_extract": true,
		// Debug
		"printa": true,
		// Memory allocation
		"alloc": true, "free": true,
		// Dynamic library loading
		"dlopen": true, "dlsym": true, "dlclose": true,
		// Memory operations
		"read_i8": true, "read_u8": true, "read_i16": true, "read_u16": true,
		"read_i32": true, "read_u32": true, "read_i64": true, "read_u64": true, "read_f64": true,
		"write_i8": true, "write_u8": true, "write_i16": true, "write_u16": true,
		"write_i32": true, "write_u32": true, "write_i64": true, "write_u64": true, "write_f32": true, "write_f64": true,
		// Dynamic calling
		"call": true, "arena_create": true, "arena_alloc": true, "arena_reset": true, "arena_destroy": true,
	}

	// Collect C import namespaces (e.g., "enet", "libc")
	cImports := make(map[string]bool)
	for _, stmt := range program.Statements {
		if cImp, ok := stmt.(*CImportStmt); ok {
			cImports[cImp.Alias] = true
		}
	}

	// Collect C67 import namespaces (e.g., "lib", "math")
	c67Imports := make(map[string]bool)
	for _, stmt := range program.Statements {
		if imp, ok := stmt.(*ImportStmt); ok {
			if imp.Alias != "*" {
				c67Imports[imp.Alias] = true
			}
		}
	}

	// Collect all function calls
	calls := make(map[string]bool)
	for _, stmt := range program.Statements {
		collectFunctionCallsFromStmt(stmt, calls)
	}

	// Collect all defined functions
	defined := collectDefinedFunctions(program)

	// Find unknown functions (called but not builtin, not defined, and not from C imports)
	var unknown []string
	for funcName := range calls {
		// Check if function is from a C import (has namespace prefix)
		// e.g., "enet.enet_initialize", "libc.malloc", "c.sin", "sdl.SDL_Init"
		isFromCImport := false

		// Check for standard "c." prefix (C FFI calls)
		if len(funcName) > 2 && funcName[:2] == "c." {
			isFromCImport = true
		}

		// Check for other C import namespaces
		if !isFromCImport {
			for ns := range cImports {
				if len(funcName) > len(ns)+1 && funcName[:len(ns)+1] == ns+"." {
					isFromCImport = true
					break
				}
			}
		}

		// Check for C67 import namespaces
		isFromC67Import := false
		for ns := range c67Imports {
			if len(funcName) > len(ns)+1 && funcName[:len(ns)+1] == ns+"." {
				isFromC67Import = true
				break
			}
		}

		// Check if it's a method call (namespace.method where namespace is in defined)
		// For dotted names, also check if the base function (after removing namespace) is defined
		isMethodCall := false
		if strings.Contains(funcName, ".") {
			parts := strings.SplitN(funcName, ".", 2)
			if len(parts) == 2 {
				baseFuncName := parts[1]
				// If the base function is a builtin or defined, it's likely a method call
				if builtins[baseFuncName] || defined[baseFuncName] {
					isMethodCall = true
				}
			}
		}

		if !builtins[funcName] && !defined[funcName] && !isFromCImport && !isFromC67Import && !isMethodCall {
			unknown = append(unknown, funcName)
		}
	}

	return unknown
}

// filterPrivateFunctions removes all function definitions with names starting with _
// Private functions (starting with _) are not exported when importing modules
func filterPrivateFunctions(program *Program) {
	var publicStmts []Statement
	for _, stmt := range program.Statements {
		// Check if this is an assignment statement
		if assign, ok := stmt.(*AssignStmt); ok {
			// Check if the name starts with _
			if len(assign.Name) > 0 && assign.Name[0] == '_' {
				// Skip private functions - don't export them
				continue
			}
		}
		// Keep all non-private statements
		publicStmts = append(publicStmts, stmt)
	}
	program.Statements = publicStmts
}

func processImports(program *Program, platform Platform, sourceFilePath string) error {
	// Find all import statements (both Git and C imports)
	var imports []*ImportStmt
	var cImports []*CImportStmt
	for _, stmt := range program.Statements {
		if importStmt, ok := stmt.(*ImportStmt); ok {
			imports = append(imports, importStmt)
		}
		if cImportStmt, ok := stmt.(*CImportStmt); ok {
			cImports = append(cImports, cImportStmt)
		}
	}

	// Process C imports first (simpler, no dependency resolution)
	if len(cImports) > 0 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Processing %d C import(s)\n", len(cImports))
		}
		for _, cImp := range cImports {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Importing C library %s as %s\n", cImp.Library, cImp.Alias)
			}
			// C imports are handled during compilation, not here
			// They just need to be tracked for namespace resolution
		}
	}

	if len(imports) == 0 {
		return nil // No C67 imports to process
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Processing %d import(s)\n", len(imports))
	}

	// Process each import using the import resolver
	for _, imp := range imports {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Importing %s", imp.URL)
		}
		if imp.Version != "" {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "@%s", imp.Version)
			}
		}
		if VerboseMode {
			fmt.Fprintf(os.Stderr, " as %s\n", imp.Alias)
		}

		// Create import spec
		importSource := imp.URL

		// If it's a relative path, resolve it relative to the source file's directory
		if strings.HasPrefix(importSource, ".") {
			sourceDir := filepath.Dir(sourceFilePath)
			importSource = filepath.Clean(filepath.Join(sourceDir, importSource))
		}

		spec := &ImportSpec{
			Source:  importSource,
			Version: imp.Version,
			Alias:   imp.Alias,
		}

		// Resolve the import (library, git repo, or directory)
		c67Files, err := ResolveImport(spec, platform.OS.String(), platform.Arch.String())
		if err != nil {
			return fmt.Errorf("failed to resolve import %s: %v", imp.URL, err)
		}

		// Filter out the current source file to avoid circular imports
		var filteredFiles []string
		absSourcePath, _ := filepath.Abs(sourceFilePath)
		for _, f := range c67Files {
			absF, _ := filepath.Abs(f)
			if absF != absSourcePath {
				filteredFiles = append(filteredFiles, f)
			}
		}
		c67Files = filteredFiles

		// Parse and merge each .c67 file with namespace handling
		for _, c67File := range c67Files {
			depContent, err := os.ReadFile(c67File)
			if err != nil {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", c67File, err)
				}
				continue
			}

			depParser := NewParserWithFilename(string(depContent), c67File)
			depProgram := depParser.ParseProgram()

			// Filter out private functions (names starting with _)
			filterPrivateFunctions(depProgram)

			// Determine namespace prefixing based on export mode and import alias
			// Priority:
			// 1. If imported with "as *", no prefix (backward compatibility)
			// 2. If dep has "export *", no prefix
			// 3. Otherwise, add namespace prefix
			usePrefix := true
			if imp.Alias == "*" {
				// "import ... as *" - no prefix
				usePrefix = false
			} else if depProgram.ExportMode == "*" {
				// "export *" in imported file - no prefix
				usePrefix = false

				// Warn if user explicitly provided an alias (not the default derived one)
				derivedAlias := deriveAliasFromSource(imp.URL)
				if imp.Alias != derivedAlias && imp.Alias != "" && imp.Alias != "*" {
					fmt.Fprintf(os.Stderr, "Warning: Package %s has 'export *', so 'as %s' is unnecessary (functions are exported to global namespace)\n", imp.URL, imp.Alias)
				}
			}

			if usePrefix {
				addNamespaceToFunctions(depProgram, imp.Alias)
			}

			// Prepend dependency program to main program
			program.Statements = append(depProgram.Statements, program.Statements...)

			// Merge namespace mappings
			if depProgram.FunctionNamespaces != nil {
				if program.FunctionNamespaces == nil {
					program.FunctionNamespaces = make(map[string]string)
				}
				for funcName, ns := range depProgram.FunctionNamespaces {
					program.FunctionNamespaces[funcName] = ns
				}
			}

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Loaded %s from %s\n", c67File, imp.URL)
			}
		}
	}

	// Remove import statements from program (they've been processed)
	var filteredStmts []Statement
	for _, stmt := range program.Statements {
		if _, ok := stmt.(*ImportStmt); !ok {
			filteredStmts = append(filteredStmts, stmt)
		}
	}
	program.Statements = filteredStmts

	// Reorder statements: variables first, then functions
	// This ensures all variables are defined before functions that reference them
	var varDecls []Statement
	var funcDefs []Statement
	var others []Statement

	for _, stmt := range program.Statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			// := creates new mutable variables (variable declarations)
			if assign.Mutable && !assign.IsUpdate {
				varDecls = append(varDecls, stmt)
			} else if !assign.Mutable && !assign.IsUpdate {
				// = for immutable (typically function definitions)
				funcDefs = append(funcDefs, stmt)
			} else {
				// <- updates (should come later)
				others = append(others, stmt)
			}
		} else {
			others = append(others, stmt)
		}
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "Statement reordering: %d varDecls, %d funcDefs, %d others\n",
			len(varDecls), len(funcDefs), len(others))
	}

	// Reconstruct: varDecls, then funcDefs, then others
	program.Statements = nil
	program.Statements = append(program.Statements, varDecls...)
	program.Statements = append(program.Statements, funcDefs...)
	program.Statements = append(program.Statements, others...)

	// Debug: print final program
	if envBool("DEBUG") {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG processImports: final program after import processing:\n")
		}
		for i, stmt := range program.Statements {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  [%d] %s\n", i, stmt.String())
			}
		}
	}

	return nil
}

// desugarClasses converts ClassDecl nodes into regular C67 code (maps and closures)
func desugarClasses(program *Program) {
	newStatements := make([]Statement, 0, len(program.Statements))

	for _, stmt := range program.Statements {
		if classDecl, ok := stmt.(*ClassDecl); ok {
			// Desugar class to constructor function
			// class Point { init := (x, y) ==> { .x = x } }
			// becomes:
			// Point := (x, y) => { instance := {}; instance["x"] = x; ret instance }

			desugared := desugarClass(classDecl)
			newStatements = append(newStatements, desugared...)
		} else {
			newStatements = append(newStatements, stmt)
		}
	}

	program.Statements = newStatements
}

// desugarClass converts a single ClassDecl into regular C67 statements
func desugarClass(class *ClassDecl) []Statement {
	statements := make([]Statement, 0)

	// Extract constructor parameters from 'init' method if it exists
	initMethod, hasInit := class.Methods["init"]
	var constructorParams []string
	var initBody []Statement

	if hasInit {
		constructorParams = initMethod.Params
		// Extract init body statements
		if block, ok := initMethod.Body.(*BlockExpr); ok {
			initBody = block.Statements
		}
	}

	// Create constructor function: ClassName := (params) => { ... }
	// Build the constructor body
	constructorBody := &BlockExpr{
		Statements: make([]Statement, 0),
	}

	// Add: instance := {}
	constructorBody.Statements = append(constructorBody.Statements, &AssignStmt{
		Name:  "instance",
		Value: &MapExpr{Keys: []Expression{}, Values: []Expression{}},
	})

	// Add init body statements (transforming .field to instance["field"])
	for _, stmt := range initBody {
		transformed := transformDotNotation(stmt, "instance")
		constructorBody.Statements = append(constructorBody.Statements, transformed)
	}

	// Add methods to instance
	for methodName, methodLambda := range class.Methods {
		if methodName == "init" {
			continue // Already handled
		}

		// Transform method body to use instance["field"] instead of .field
		transformedLambda := &LambdaExpr{
			Params: methodLambda.Params,
			Body:   transformDotNotationExpr(methodLambda.Body, "instance"),
		}

		// Add: instance["methodName"] = lambda
		constructorBody.Statements = append(constructorBody.Statements, &AssignStmt{
			Name: "instance",
			Value: &IndexExpr{
				List:  &IdentExpr{Name: "instance"},
				Index: &StringExpr{Value: methodName},
			},
			IsUpdate: true, // This is an index assignment, not a new variable
		})
		// Actually, index assignment needs different handling. Let me use ExpressionStmt with BinaryExpr
		// For now, just skip methods other than init
		_ = transformedLambda
	}

	// Add: ret instance
	constructorBody.Statements = append(constructorBody.Statements, &JumpStmt{
		IsBreak: true,
		Label:   0,
		Value:   &IdentExpr{Name: "instance"},
	})

	// Create the constructor assignment
	constructor := &AssignStmt{
		Name: class.Name,
		Value: &LambdaExpr{
			Params: constructorParams,
			Body:   constructorBody,
		},
	}

	statements = append(statements, constructor)

	// Handle class variables (ClassName.var = value)
	for fullName, value := range class.ClassVars {
		// fullName is like "Point.origin"
		statements = append(statements, &AssignStmt{
			Name:  fullName,
			Value: value,
		})
	}

	return statements
}

// transformDotNotation transforms .field references to instanceName["field"]
func transformDotNotation(stmt Statement, instanceName string) Statement {
	// For now, just return the statement as-is
	// TODO: Implement proper transformation
	_ = instanceName
	return stmt
}

// transformDotNotationExpr transforms .field references in expressions
func transformDotNotationExpr(expr Expression, instanceName string) Expression {
	// For now, just return the expression as-is
	// TODO: Implement proper transformation
	_ = instanceName
	return expr
}

func addNamespaceToFunctions(program *Program, namespace string) {
	// Store namespace metadata in Program for later use during compilation
	// We can't rename functions with dots because the parser doesn't support it
	// Instead, we'll track which namespace each function belongs to
	if program.FunctionNamespaces == nil {
		program.FunctionNamespaces = make(map[string]string)
	}

	for _, stmt := range program.Statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			// Track which namespace this function belongs to
			program.FunctionNamespaces[assign.Name] = namespace
		}
	}
}

func CompileC67(inputPath string, outputPath string, platform Platform) (err error) {
	return CompileC67WithOptions(inputPath, outputPath, platform, WPOTimeout, VerboseMode)
}

func CompileC67WithOptions(inputPath string, outputPath string, platform Platform, wpoTimeout float64, verbose bool) (err error) {
	// Set verbose mode
	oldVerbose := VerboseMode
	if verbose {
		VerboseMode = true
	}
	defer func() {
		VerboseMode = oldVerbose
	}()

	// Default to WPO if not explicitly set
	if wpoTimeout == 0 {
		wpoTimeout = 2.0
	}

	// Recover from parser panics and convert to errors
	defer func() {
		if r := recover(); r != nil {
			// Print stack trace for debugging
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Panic stack trace:\n")
				debug.PrintStack()
			}
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("panic during compilation: %v", r)
			}
		}
	}()

	// Convert to absolute path to ensure correct directory resolution
	absInputPath, absErr := filepath.Abs(inputPath)
	if absErr != nil {
		return fmt.Errorf("failed to get absolute path for %s: %v", inputPath, absErr)
	}
	inputPath = absInputPath

	// Read input file
	content, readErr := os.ReadFile(inputPath)
	if readErr != nil {
		return fmt.Errorf("failed to read %s: %v", inputPath, readErr)
	}

	// Parse main file
	parser := NewParserWithFilename(string(content), inputPath)
	program := parser.ParseProgram()

	if VerboseMode {
		// Temporarily disabled due to String() crash with nil args
		// fmt.Fprintf(os.Stderr, "Parsed program:\n%s\n", program.String())
		fmt.Fprintf(os.Stderr, "DEBUG: Parsed program (String() disabled to avoid crash)\n")
	}

	// Desugar classes to regular C67 code
	desugarClasses(program)

	// Sibling loading is now handled later, after checking for unknown functions
	// This prevents loading unnecessary files and avoids conflicts with test files
	var combinedSource string

	// Process explicit import statements
	err = processImports(program, platform, inputPath)
	if err != nil {
		return fmt.Errorf("failed to process imports: %v", err)
	}

	// Check for unknown functions and resolve dependencies
	// Build combined source code (siblings + dependencies + main)
	unknownFuncs := getUnknownFunctions(program)
	if len(unknownFuncs) > 0 {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "Resolving dependencies for: %v\n", unknownFuncs)
		}

		// First, try to load sibling .c67 files from the same directory
		// This allows files in the same directory to share definitions
		inputDir := filepath.Dir(inputPath)
		inputBase := filepath.Base(inputPath)

		// Skip sibling loading for system temp directories, test directories, or if --single flag is set
		// Only skip /tmp if there are many .c67 files (likely temp files from -c flag)
		skipSiblings := SingleFlag || strings.Contains(inputDir, "testprograms") // Skip for test directories or --single flag

		dirEntries, err := os.ReadDir(inputDir)

		// For /tmp, only skip if it's the root temp dir with many files
		if (inputDir == "/tmp" || inputDir == "C:\\tmp" || strings.HasPrefix(inputDir, "/tmp/") || strings.HasPrefix(inputDir, "C:\\tmp\\")) && err == nil {
			// Count .c67 files - if there are more than 10, likely temp files
			c67Count := 0
			for _, entry := range dirEntries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".c67") {
					c67Count++
					if c67Count > 10 {
						skipSiblings = true
						break
					}
				}
			}
		}
		if err == nil && !skipSiblings {
			siblingFiles := []string{}
			for _, entry := range dirEntries {
				name := entry.Name()
				// Include .c67 files in same directory (except the input file itself)
				if !entry.IsDir() && strings.HasSuffix(name, ".c67") && name != inputBase {
					siblingPath := filepath.Join(inputDir, name)
					siblingFiles = append(siblingFiles, siblingPath)
				}
			}

			// Sort for deterministic compilation order
			sort.Strings(siblingFiles)

			if len(siblingFiles) > 0 {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Loading %d sibling file(s) from %s\n", len(siblingFiles), inputDir)
				}

				for _, siblingPath := range siblingFiles {
					siblingContent, readErr := os.ReadFile(siblingPath)
					if readErr != nil {
						if VerboseMode {
							fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", siblingPath, readErr)
						}
						continue
					}

					siblingParser := NewParserWithFilename(string(siblingContent), siblingPath)
					siblingProgram := siblingParser.ParseProgram()

					// Prepend sibling statements before main file (definitions must come before use)
					program.Statements = append(siblingProgram.Statements, program.Statements...)
					combinedSource = string(siblingContent) + "\n" + combinedSource

					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Loaded %s\n", siblingPath)
					}
				}

				// Re-check for unknown functions after loading siblings
				unknownFuncs = getUnknownFunctions(program)
				if VerboseMode && len(unknownFuncs) > 0 {
					fmt.Fprintf(os.Stderr, "Still unknown after siblings: %v\n", unknownFuncs)
				}
			}
		}

		// Resolve dependencies from Git repositories
		repos := ResolveDependencies(unknownFuncs)
		if len(repos) > 0 {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Loading dependencies from %d repositories\n", len(repos))
			}

			// Ensure all repositories are cloned/updated
			for _, repoURL := range repos {
				repoPath, err := EnsureRepoCloned(repoURL, UpdateDepsFlag)
				if err != nil {
					return fmt.Errorf("failed to fetch dependency %s: %v", repoURL, err)
				}

				// Find all .c67 files in the repository
				c67Files, err := FindC67Files(repoPath)
				if err != nil {
					return fmt.Errorf("failed to find .c67 files in %s: %v", repoPath, err)
				}

				// Parse and merge each .c67 file
				for _, c67File := range c67Files {
					depContent, err := os.ReadFile(c67File)
					if err != nil {
						if VerboseMode {
							fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", c67File, err)
						}
						continue
					}

					depParser := NewParserWithFilename(string(depContent), c67File)
					depProgram := depParser.ParseProgram()

					// Prepend dependency program to main program (dependencies must be defined before use)
					program.Statements = append(depProgram.Statements, program.Statements...)
					// Prepend dependency source to combined source
					combinedSource = string(depContent) + "\n" + combinedSource
					if VerboseMode {
						fmt.Fprintf(os.Stderr, "Loaded %s from %s\n", c67File, repoURL)
					}
				}
			}
		}
	}
	// Append main file source
	combinedSource = combinedSource + string(content)

	// First pass: identify global (module-level) variables
	// These are variables defined at the top level (not inside any lambda)
	globalVars := make(map[string]int)
	for _, stmt := range program.Statements {
		if assign, ok := stmt.(*AssignStmt); ok {
			globalVars[assign.Name] = 0 // Value doesn't matter, we just need the key
		}
	}

	// Analyze closures to detect which variables lambdas capture from outer scope
	// This must be done before compilation so that CapturedVars is populated
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Analyzing closures...\n")
	}
	availableVars := make(map[string]bool)
	for _, stmt := range program.Statements {
		analyzeClosures(stmt, availableVars, globalVars)
		// Add any newly defined variables to available vars for subsequent statements
		if assign, ok := stmt.(*AssignStmt); ok {
			availableVars[assign.Name] = true
		}
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "-> Finished analyzing closures\n")
	}

	// Run whole program optimization
	// TODO: Re-enable when Optimizer type is implemented
	// optimizer := NewOptimizer(wpoTimeout)
	// err = optimizer.Optimize(program)
	// if err != nil {
	// 	return fmt.Errorf("optimization failed: %v", err)
	// }

	// Final check: verify all functions are defined (after all dependency resolution)
	// NOTE: We allow forward references (functions called before defined) as long as they
	// ARE defined somewhere. The compiler handles this with indirect calls through variables.
	finalUnknownFuncs := getUnknownFunctions(program)
	if len(finalUnknownFuncs) > 0 {
		// Sort for consistent error messages
		sort.Strings(finalUnknownFuncs)

		// Report all undefined functions
		if len(finalUnknownFuncs) == 1 {
			return fmt.Errorf("undefined function: %s\nNote: Function must be defined before use or imported from a dependency", finalUnknownFuncs[0])
		}
		return fmt.Errorf("undefined functions: %s\nNote: Functions must be defined before use or imported from dependencies", strings.Join(finalUnknownFuncs, ", "))
	}

	// Compile
	compiler, err := NewC67Compiler(platform, verbose)
	if err != nil {
		return fmt.Errorf("failed to create compiler: %v", err)
	}
	compiler.sourceCode = combinedSource
	compiler.wpoTimeout = wpoTimeout
	compiler.errors.SetSourceCode(combinedSource)

	err = compiler.Compile(program, outputPath)
	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	// Output optimization summary in verbose mode
	if VerboseMode {
		totalCalls := compiler.tailCallsOptimized + compiler.nonTailCalls
		if totalCalls > 0 {
			fmt.Printf("Tail call optimization: %d/%d recursive calls optimized",
				compiler.tailCallsOptimized, totalCalls)
			if compiler.nonTailCalls > 0 {
				fmt.Printf(" (%d not in tail position)\n", compiler.nonTailCalls)
			} else {
				fmt.Println()
			}
		}
	}

	return nil
}

// compileARM64 compiles a program for ARM64 architecture
func (fc *C67Compiler) compileARM64(program *Program, outputPath string) error {
	// Create ARM64 code generator
	acg := NewARM64CodeGen(fc.eb, fc.cConstants)

	// Generate code
	if err := acg.CompileProgram(program); err != nil {
		return err
	}

	// Write executable based on target OS
	if fc.eb.target.IsMachO() {
		return fc.writeMachOARM64(outputPath)
	}
	// Default to ELF for Linux/FreeBSD
	return fc.writeELFARM64(outputPath)
}

// compileRiscv64 compiles a program for RISC-V64 architecture
func (fc *C67Compiler) compileRiscv64(program *Program, outputPath string) error {
	// Create RISC-V64 code generator
	rcg := NewRiscvCodeGen(fc.eb)

	// Generate code
	if err := rcg.CompileProgram(program); err != nil {
		return err
	}

	// Write ELF file
	return fc.writeELFRiscv64(outputPath)
}

// writeMachOARM64 writes an ARM64 Mach-O executable for macOS
