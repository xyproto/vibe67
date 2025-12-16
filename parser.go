// parser.go - C67 Language Parser (Version 1.5.0)
// Completion: 95%
//
// Status: Canonical Implementation of GRAMMAR.md and LANGUAGESPEC.md v1.5.0
//
// This parser is the authoritative implementation of GRAMMAR.md and LANGUAGESPEC.md v1.5.0.
// It implements a complete recursive descent parser for the C67 programming
// language with direct machine code generation for x86_64, ARM64, and RISCV64.
//
// Key Features (C67 3.0):
// - Universal type system: map[uint64]float64
// - Block disambiguation: maps vs matches vs statements
// - Value match (with expression) and guard match (with |)
// - Minimal parentheses philosophy
// - Functions defined with = (not :=) by convention
// - Bitwise operators with 'b' suffix
// - ENet-style message passing
// - Direct machine code generation (no IR)
//
// Implementation Coverage:
// - All GRAMMAR.md grammar constructs
// - All statement types (cstruct, arena, unsafe, loops, assignments, ret, break, continue)
// - All expression types (literals, operators, lambdas, match blocks, blocks)
// - All operators (arithmetic, comparison, logical, bitwise, power, pipe)
// - Block disambiguation (map literal, match block, statement block)
// - Guard syntax (| at line start)
// - C FFI and syscall support
//
// This file contains the core parser that transforms C67 source code
// into an Abstract Syntax Tree (AST). It handles:
// - Tokenization and lexical analysis via Lexer
// - Recursive descent parsing with operator precedence
// - Expression parsing with proper precedence climbing
// - Statement parsing for all C67 constructs
// - AST node construction with semantic validation

package main

import (
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

var globalParseCallCount = 0
var debugParser = false // Set to true for parser debugging

const (
	// Parser recursion and iteration safety limits
	// These prevent infinite loops and stack overflows during parsing
	maxParseRecursion  = 1000  // Maximum recursion depth for parser calls
	maxBlockIterations = 10000 // Maximum iterations for parsing block statements
	maxASTIterations   = 1000  // Maximum iterations for AST traversal

	// Buffer sizes for runtime operations
	stringBufferSize = 256 // Maximum string buffer size for conversions
	socketBufferSize = 256 // Network socket buffer size
	socketStructSize = 16  // sizeof(struct sockaddr_in)
	sdlEventSize     = 56  // sizeof(SDL_Event) - used in examples and documentation

	// Stack alignment
	stackAlignment = 16 // x86_64 ABI requires 16-byte stack alignment

	// Hash table sizes
	defaultHashTableSize = 512 // Default size for internal hash tables

	// Clone syscall
	cloneSyscallNumber = 56 // Linux clone() syscall number on x86_64
)

// parseNumberLiteral parses a number literal which can be decimal, hex (0x...), or binary (0b...)
func (p *Parser) parseNumberLiteral(s string) float64 {
	if len(s) >= 2 {
		prefix := s[0:2]
		if prefix == "0x" || prefix == "0X" {
			// Hexadecimal
			val, err := strconv.ParseUint(s[2:], 16, 64)
			if err != nil {
				p.error(fmt.Sprintf("invalid hexadecimal literal: %s", s))
			}
			return float64(val)
		} else if prefix == "0b" || prefix == "0B" {
			// Binary
			val, err := strconv.ParseUint(s[2:], 2, 64)
			if err != nil {
				p.error(fmt.Sprintf("invalid binary literal: %s", s))
			}
			return float64(val)
		}
	}
	// Regular decimal number
	val, _ := strconv.ParseFloat(s, 64)
	return val
}

type Parser struct {
	lexer           *Lexer
	current         Token
	peek            Token
	filename        string
	source          string
	loopDepth       int                     // Current loop nesting level (0 = not in loop, 1 = outer loop, etc.)
	functionDepth   int                     // Current function nesting level (0 = module level, 1+ = inside function/lambda)
	constants       map[string]Expression   // Compile-time constants (immutable literals)
	aliases         map[string]TokenType    // Keyword aliases (e.g., "for" -> TOKEN_AT)
	cstructs        map[string]*CStructDecl // CStruct declarations for metadata access
	cImports        map[string]bool         // C import namespaces (e.g., "sdl", "c")
	speculative     bool                    // True when in speculative parsing mode (suppress errors)
	errors          *ErrorCollector         // Railway-oriented error collector
	inMatchBlock    bool                    // True when parsing inside a match block (prevents nested match parsing)
	inConditionLoop bool                    // True when parsing condition loop expression (prevents 'max' consumption)
	scopes          []map[string]bool       // Stack of variable scopes for shadow detection
	lambdaParams    []string                // Temporary storage for lambda parameters being parsed
}

type parserState struct {
	lexerPos  int
	lexerLine int
	current   Token
	peek      Token
}

func (p *Parser) saveState() parserState {
	return parserState{
		lexerPos:  p.lexer.pos,
		lexerLine: p.lexer.line,
		current:   p.current,
		peek:      p.peek,
	}
}

func (p *Parser) restoreState(state parserState) {
	p.lexer.pos = state.lexerPos
	p.lexer.line = state.lexerLine
	p.current = state.current
	p.peek = state.peek
}

func NewParser(input string) *Parser {
	globalParseCallCount = 0 // Reset global counter for each parser instance
	p := &Parser{
		lexer:     NewLexer(input),
		filename:  "<input>",
		source:    input,
		constants: make(map[string]Expression),
		aliases:   make(map[string]TokenType),
		cstructs:  make(map[string]*CStructDecl),
		cImports:  make(map[string]bool),
		errors:    NewErrorCollector(10),
		scopes:    []map[string]bool{make(map[string]bool)}, // Start with module scope
	}
	// Register built-in C namespace
	p.cImports["c"] = true
	p.errors.SetSourceCode(input)
	p.nextToken()
	p.nextToken()
	return p
}

func NewParserWithFilename(input, filename string) *Parser {
	globalParseCallCount = 0 // Reset global counter for each parser instance
	p := &Parser{
		lexer:     NewLexer(input),
		filename:  filename,
		source:    input,
		constants: make(map[string]Expression),
		aliases:   make(map[string]TokenType),
		cstructs:  make(map[string]*CStructDecl),
		cImports:  make(map[string]bool),
		errors:    NewErrorCollector(10),
		scopes:    []map[string]bool{make(map[string]bool)}, // Start with module scope
	}
	// Register built-in C namespace
	p.cImports["c"] = true
	p.errors.SetSourceCode(input)
	p.nextToken()
	p.nextToken()
	return p
}

// formatError creates a nicely formatted error message with source context
func (p *Parser) formatError(line int, msg string) string {
	lines := strings.Split(p.source, "\n")
	if line < 1 || line > len(lines) {
		return fmt.Sprintf("%s:%d: %s", p.filename, line, msg)
	}

	sourceLine := lines[line-1]
	lineNum := fmt.Sprintf("%4d | ", line)
	marker := strings.Repeat(" ", len(lineNum)) + strings.Repeat("^", len(sourceLine))

	return fmt.Sprintf("%s:%d: error: %s\n%s%s\n%s",
		p.filename, line, msg, lineNum, sourceLine, marker)
}

// error collects a parsing error in the ErrorCollector (railway-oriented approach)
// In speculative mode, errors are suppressed and parsing fails silently
func (p *Parser) error(msg string) {
	if p.speculative {
		// In speculative mode, don't panic - let the caller handle failure
		panic(speculativeError{})
	}

	// Railway-oriented: collect error and continue if possible
	err := SyntaxError(msg, SourceLocation{
		File:   p.filename,
		Line:   p.current.Line,
		Column: p.current.Column,
		Length: len(p.current.Value),
	})
	p.errors.AddError(err)

	// For backwards compatibility during transition: if we hit max errors, panic
	// This will be removed once all error handling is converted
	if p.errors.ShouldStop() {
		// Print all collected errors before panicking
		report := p.errors.Report(true) // Use color
		if report != "" {
			fmt.Fprintln(os.Stderr, report)
		}
		panic(fmt.Errorf("too many errors"))
	}
}

// parseError creates and collects a syntax error with custom location
func (p *Parser) parseError(msg string, loc SourceLocation) {
	if p.speculative {
		panic(speculativeError{})
	}
	err := SyntaxError(msg, loc)
	p.errors.AddError(err)
	if p.errors.ShouldStop() {
		// Print all collected errors before panicking
		report := p.errors.Report(true) // Use color
		if report != "" {
			fmt.Fprintln(os.Stderr, report)
		}
		panic(fmt.Errorf("too many errors"))
	}
}

// synchronize skips tokens until we reach a safe recovery point
// This allows the parser to continue after an error and find more errors
func (p *Parser) synchronize() {
	p.nextToken()

	for p.current.Type != TOKEN_EOF {
		// Synchronization points: statement boundaries
		switch p.current.Type {
		case TOKEN_NEWLINE:
			p.nextToken()
			return
		case TOKEN_SEMICOLON:
			p.nextToken()
			return
		case TOKEN_RBRACE: // End of block
			return
		case TOKEN_AT: // Loop statement
			return
		}

		// Keywords that start new statements (stored as identifiers)
		if p.peek.Type == TOKEN_IDENT {
			switch p.peek.Value {
			case "fn", "return", "break", "continue", "import", "from", "use", "defer":
				return
			}
		}

		p.nextToken()
	}
}

// speculativeError is used to signal parse failure during speculative parsing
type speculativeError struct{}

// compilerError prints an error message and panics (to be recovered by CompileC67)
// Use this instead of fmt.Fprintf + os.Exit in code generation
func compilerError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if VerboseMode {
		fmt.Fprintln(os.Stderr, "Error:", msg)
	}
	panic(fmt.Errorf("%s", msg))
}

func (p *Parser) nextToken() {
	p.current = p.peek
	p.peek = p.lexer.NextToken()

	// Apply aliases: if current token is an identifier that matches an alias, replace its type
	if p.current.Type == TOKEN_IDENT {
		if aliasTarget, exists := p.aliases[p.current.Value]; exists {
			p.current.Type = aliasTarget
		}
	}
	if p.peek.Type == TOKEN_IDENT {
		if aliasTarget, exists := p.aliases[p.peek.Value]; exists {
			p.peek.Type = aliasTarget
		}
	}
}

// Scope management for shadow keyword detection
func (p *Parser) pushScope() {
	p.scopes = append(p.scopes, make(map[string]bool))
}

func (p *Parser) popScope() {
	if len(p.scopes) > 1 {
		p.scopes = p.scopes[:len(p.scopes)-1]
	}
}

func (p *Parser) declareVariable(name string) {
	if len(p.scopes) > 0 {
		p.scopes[len(p.scopes)-1][name] = true
	}
}

func (p *Parser) wouldShadow(name string) bool {
	// Check if name exists in any outer scope (case-insensitive)
	nameLower := strings.ToLower(name)
	for i := len(p.scopes) - 2; i >= 0; i-- { // Skip current scope
		for varName := range p.scopes[i] {
			if strings.ToLower(varName) == nameLower {
				return true
			}
		}
	}
	return false
}

func (p *Parser) skipNewlines() {
	for p.current.Type == TOKEN_NEWLINE || p.current.Type == TOKEN_SEMICOLON {
		p.nextToken()
	}
}

func (p *Parser) ParseProgram() *Program {
	globalParseCallCount = 0 // Reset for each program parse
	program := &Program{}

	p.skipNewlines()
	for p.current.Type != TOKEN_EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			// Handle alias statements: process them immediately and don't add to AST
			if aliasStmt, ok := stmt.(*AliasStmt); ok {
				// Store the alias in the parser's alias map
				p.aliases[aliasStmt.NewName] = aliasStmt.Target
			} else if exportStmt, ok := stmt.(*ExportStmt); ok {
				// Handle export statements: store in program metadata
				if exportStmt.Mode == "*" {
					program.ExportMode = "*"
				} else {
					program.ExportedFuncs = append(program.ExportedFuncs, exportStmt.Functions...)
				}
				// Don't add export statements to the AST
			} else {
				// Regular statements are added to the program
				program.Statements = append(program.Statements, stmt)
			}
		}
		p.nextToken()
		p.skipNewlines()
	}

	// Check for parse errors
	if p.errors.HasErrors() {
		// Print all collected errors
		fmt.Fprintln(os.Stderr, p.errors.Report(true))
		panic(fmt.Errorf("compilation failed with %d error(s)", p.errors.ErrorCount()))
	}

	// Don't add automatic exit(0) statement - the compiler will emit exit code
	// after processing deferred statements (see lines 2658-2669 in compileStatement)

	// Apply optimizations
	program = optimizeProgram(program)

	return program
}

// optimizeProgram applies optimization passes to the AST
func (p *Parser) parseImport() Statement {
	p.nextToken() // skip 'import'

	// Parse import source (string literal or identifier chain)
	// Examples:
	// - import "sdl3" as sdl                                 (library)
	// - import "github.com/user/repo" as repo                (git)
	// - import "github.com/user/repo@v1.0.0" as repo         (git with version)
	// - import "." as local                                  (directory)
	// - import "/path/to/lib.so" as lib                      (library file)

	var source string
	var isLibraryFile bool

	if p.current.Type == TOKEN_STRING {
		source = p.current.Value

		// Check if it's a library file (.so, .dll, .dylib)
		isLibraryFile = strings.HasSuffix(source, ".so") ||
			strings.Contains(source, ".so.") ||
			strings.HasSuffix(source, ".dll") ||
			strings.HasSuffix(source, ".dylib")

		p.nextToken()
	} else if p.current.Type == TOKEN_IDENT {
		// Bare identifier for library: import sdl3 as sdl
		source = p.current.Value
		p.nextToken()
	} else {
		p.error("expected string or identifier after 'import'")
		return nil
	}

	// Parse optional 'as alias'
	var alias string
	if p.current.Type == TOKEN_AS {
		p.nextToken()

		if p.current.Type != TOKEN_IDENT && p.current.Type != TOKEN_STAR {
			p.error("expected alias or '*' after 'as'")
		}
		alias = p.current.Value
		p.nextToken()
	} else {
		// No "as" provided - derive alias from source
		// For "github.com/user/repo" -> "repo"
		// For "sdl3" -> "sdl3"
		alias = deriveAliasFromSource(source)
	}

	// Parse the import spec
	spec, err := ParseImportSource(source)
	if err != nil {
		p.error(fmt.Sprintf("invalid import source: %v", err))
		return nil
	}

	// Determine import type based on source
	// Library files are always treated as C imports
	if isLibraryFile {
		// Extract just the filename from the path
		filename := source
		if lastSlash := strings.LastIndex(source, "/"); lastSlash != -1 {
			filename = source[lastSlash+1:]
		} else if lastSlash := strings.LastIndex(source, "\\"); lastSlash != -1 {
			filename = source[lastSlash+1:]
		}

		// Register C import namespace
		p.cImports[alias] = true
		return &CImportStmt{Library: filename, Alias: alias, SoPath: source}
	}

	// If it's a local path (., ./path, /path) or has version or looks like git URL, it's ImportStmt
	if spec.IsLocal || spec.Version != "" || isGitURL(source) ||
		strings.Contains(source, "/") || strings.Contains(source, "\\") {
		// Git repository or directory import
		return &ImportStmt{URL: spec.Source, Version: spec.Version, Alias: alias}
	}

	// Otherwise, it's a library name (C import)
	p.cImports[alias] = true
	return &CImportStmt{Library: source, Alias: alias}
}

func (p *Parser) parseExport() Statement {
	p.nextToken() // skip 'export'

	// Check for "export *"
	if p.current.Type == TOKEN_STAR {
		p.nextToken()
		return &ExportStmt{Mode: "*", Functions: nil}
	}

	// Parse list of function names: "export func1 func2 func3"
	var functions []string
	for p.current.Type == TOKEN_IDENT {
		functions = append(functions, p.current.Value)
		p.nextToken()

		// Allow optional commas between function names
		if p.current.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}

	if len(functions) == 0 {
		p.error("expected '*' or function names after 'export'")
	}

	return &ExportStmt{Mode: "", Functions: functions}
}

func (p *Parser) parseArenaStmt() *ArenaStmt {
	p.nextToken() // skip 'arena'

	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' after 'arena'")
	}
	p.nextToken() // skip '{'
	p.skipNewlines()

	var body []Statement
	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			body = append(body, stmt)
		}
		p.nextToken()
		p.skipNewlines()
	}

	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of arena block")
	}

	return &ArenaStmt{Body: body}
}

func (p *Parser) parseDeferStmt() *DeferStmt {
	p.nextToken() // skip 'defer'

	// Parse the expression to be deferred (typically a function call)
	expr := p.parseExpression()
	if expr == nil {
		p.error("expected expression after 'defer'")
	}

	return &DeferStmt{Call: expr}
}

func (p *Parser) parseSpawnStmt() *SpawnStmt {
	p.nextToken() // skip 'spawn'

	// Parse the expression to spawn
	expr := p.parseExpression()
	if expr == nil {
		p.error("expected expression after 'spawn'")
	}

	// Check for optional pipe syntax: | params | block
	var params []string
	var block *BlockExpr

	if p.peek.Type == TOKEN_PIPE {
		p.nextToken() // move to PIPE
		p.nextToken() // skip PIPE

		// Parse parameter list (comma-separated identifiers)
		// For now, only support simple identifiers, not map destructuring
		for {
			if p.current.Type != TOKEN_IDENT {
				p.error("expected identifier in c67 pipe parameters")
			}
			params = append(params, p.current.Value)
			p.nextToken()

			if p.current.Type == TOKEN_COMMA {
				p.nextToken() // skip comma
			} else if p.current.Type == TOKEN_PIPE {
				break
			} else {
				p.error("expected ',' or '|' in c67 pipe parameters")
			}
		}

		p.nextToken() // skip final PIPE

		// Parse block
		if p.current.Type != TOKEN_LBRACE {
			p.error("expected block after c67 pipe parameters")
		}

		// Parse as BlockExpr
		block = &BlockExpr{}
		p.nextToken() // skip '{'
		p.skipNewlines()

		for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
			stmt := p.parseStatement()
			if stmt != nil {
				block.Statements = append(block.Statements, stmt)
			}
			p.nextToken()
			p.skipNewlines()
		}

		if p.current.Type != TOKEN_RBRACE {
			p.error("expected '}' at end of c67 block")
		}
	}

	return &SpawnStmt{
		Expr:   expr,
		Params: params,
		Block:  block,
	}
}

func (p *Parser) parseAliasStmt() *AliasStmt {
	p.nextToken() // skip 'alias'

	// Parse new keyword name
	if p.current.Type != TOKEN_IDENT {
		p.error("expected identifier after 'alias'")
	}
	newName := p.current.Value
	p.nextToken()

	// Expect '='
	if p.current.Type != TOKEN_EQUALS {
		p.error("expected '=' in alias declaration")
	}
	p.nextToken()

	// Parse target keyword/token
	targetName := p.current.Value
	targetType := p.current.Type

	// Validate that target is a valid keyword or operator
	validTargets := map[TokenType]bool{
		TOKEN_AT: true, TOKEN_IN: true, TOKEN_RET: true, TOKEN_ERR: true,
		TOKEN_UNSAFE: true, TOKEN_ARENA: true, TOKEN_DEFER: true,
		TOKEN_MAX: true, TOKEN_INF: true, TOKEN_AND: true, TOKEN_OR: true,
		TOKEN_NOT: true, TOKEN_XOR: true, TOKEN_AT_PLUSPLUS: true,
	}

	// Special handling for @ operators (break/continue)
	if targetName == "@-" {
		targetType = TOKEN_AT // Break will be handled by checking targetName
	} else if targetName == "@=" || targetName == "@++" {
		targetType = TOKEN_AT_PLUSPLUS
	} else if !validTargets[targetType] {
		p.error("alias target must be a valid keyword or operator (e.g., @, @-, @=, in, ret, etc.)")
	}

	p.nextToken()

	return &AliasStmt{
		NewName:    newName,
		TargetName: targetName,
		Target:     targetType,
	}
}

func (p *Parser) parseCStructDecl() *CStructDecl {
	p.nextToken() // skip 'cstruct'

	// Parse struct name
	if p.current.Type != TOKEN_IDENT {
		p.error("expected struct name after 'cstruct'")
	}
	name := p.current.Value
	p.nextToken() // skip struct name

	// Check for optional 'packed' modifier
	packed := false
	if p.current.Type == TOKEN_PACKED {
		packed = true
		p.nextToken() // skip 'packed'
	}

	// Check for optional 'aligned(N)' modifier
	align := 0
	if p.current.Type == TOKEN_ALIGNED {
		p.nextToken() // skip 'aligned'
		if p.current.Type != TOKEN_LPAREN {
			p.error("expected '(' after 'aligned'")
		}
		p.nextToken() // skip '('
		if p.current.Type != TOKEN_NUMBER {
			p.error("expected alignment value")
		}
		alignVal, err := strconv.Atoi(p.current.Value)
		if err != nil || alignVal <= 0 {
			p.error("alignment must be a positive integer")
		}
		align = alignVal
		p.nextToken() // skip number
		if p.current.Type != TOKEN_RPAREN {
			p.error("expected ')' after alignment value")
		}
		p.nextToken() // skip ')'
	}

	// Expect '{'
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' after struct name")
	}
	p.nextToken() // skip '{'
	p.skipNewlines()

	// Parse field list: field1: type1, field2: type2, ...
	fields := []CStructField{}
	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		// Parse field name
		if p.current.Type != TOKEN_IDENT {
			p.error("expected field name in struct definition")
		}
		fieldName := p.current.Value
		p.nextToken() // skip field name

		// Expect 'as'
		if p.current.Type != TOKEN_AS {
			p.error("expected 'as' after field name")
		}
		p.nextToken() // skip 'as'

		// Parse field type
		if p.current.Type != TOKEN_IDENT {
			p.error("expected field type")
		}
		fieldType := p.current.Value

		// Validate C type
		validTypes := map[string]bool{
			"int8": true, "int16": true, "int32": true, "int64": true,
			"uint8": true, "uint16": true, "uint32": true, "uint64": true,
			"float32": true, "float64": true, "ptr": true, "cstr": true,
		}
		if !validTypes[fieldType] {
			p.error(fmt.Sprintf("invalid C type '%s' (must be int8/int16/int32/int64/uint8/uint16/uint32/uint64/float32/float64/ptr/cstr)", fieldType))
		}

		fields = append(fields, CStructField{
			Name: fieldName,
			Type: fieldType,
		})

		p.nextToken() // skip field type
		p.skipNewlines()

		// Comma is optional between fields (newlines separate them)
		if p.current.Type == TOKEN_COMMA {
			p.nextToken() // skip ',' if present
			p.skipNewlines()
		}
		// Just continue to next field or closing brace
	}

	// Expect '}'
	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of struct definition")
	}

	// Create struct declaration and calculate layout
	decl := &CStructDecl{
		Name:   name,
		Fields: fields,
		Packed: packed,
		Align:  align,
	}
	decl.CalculateStructLayout()

	// Store the cstruct declaration for metadata access (Type.size, Type.field.offset)
	p.cstructs[name] = decl

	// Register constants for struct size and field offsets
	// These can be used in expressions like: SDL_Rect_SIZEOF, SDL_Rect_x_OFFSET
	p.constants[name+"_SIZEOF"] = &NumberExpr{Value: float64(decl.Size)}
	for _, field := range decl.Fields {
		constantName := name + "_" + field.Name + "_OFFSET"
		p.constants[constantName] = &NumberExpr{Value: float64(field.Offset)}
	}

	return decl
}

func (p *Parser) parseClassDecl() *ClassDecl {
	p.nextToken() // skip 'class'

	// Parse class name
	if p.current.Type != TOKEN_IDENT {
		p.error("expected class name after 'class'")
	}
	name := p.current.Value
	p.nextToken() // skip class name

	// Parse optional compositions: <> Mixin1 <> Mixin2 ...
	compositions := []string{}
	for p.current.Type == TOKEN_LTGT {
		p.nextToken() // skip '<>'
		if p.current.Type != TOKEN_IDENT {
			p.error("expected identifier after '<>' in class declaration")
		}
		compositions = append(compositions, p.current.Value)
		p.nextToken() // skip identifier
	}

	// Expect '{'
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' after class name (and optional mixins)")
	}
	p.nextToken() // skip '{'
	p.skipNewlines()

	// Parse class body
	classVars := make(map[string]Expression)
	methods := make(map[string]*LambdaExpr)

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		// Parse identifier (for class var or method)
		if p.current.Type != TOKEN_IDENT {
			p.error("expected identifier in class body")
		}
		ident := p.current.Value
		p.nextToken() // skip identifier

		// Check for dot (class variable: ClassName.var = value)
		if p.current.Type == TOKEN_DOT {
			p.nextToken() // skip '.'
			if p.current.Type != TOKEN_IDENT {
				p.error("expected identifier after '.' in class variable")
			}
			varName := p.current.Value
			p.nextToken() // skip var name

			if p.current.Type != TOKEN_EQUALS {
				p.error("expected '=' after class variable name")
			}
			p.nextToken() // skip '='

			// Parse the value expression
			value := p.parseExpression()
			classVars[ident+"."+varName] = value
			p.nextToken() // move past expression
			p.skipNewlines()
			continue
		}

		// Check for method definition: identifier = lambda or identifier := lambda
		if p.current.Type == TOKEN_EQUALS || p.current.Type == TOKEN_COLON_EQUALS {
			p.nextToken() // skip '=' or ':='

			// Parse lambda expression - need to handle different lambda forms
			var lambda *LambdaExpr

			if p.current.Type == TOKEN_LPAREN {
				// Parenthesized lambda: (x, y) => body
				expr := p.parseExpression()
				var ok bool
				lambda, ok = expr.(*LambdaExpr)
				if !ok {
					p.error("expected lambda expression after '=' or ':=' in method definition")
				}
			} else if p.current.Type == TOKEN_IDENT {
				// Non-parenthesized lambda: x => body or x, y => body
				expr := p.tryParseNonParenLambda()
				if expr == nil {
					p.error("expected lambda expression after '=' or ':=' in method definition")
				}
				var ok bool
				lambda, ok = expr.(*LambdaExpr)
				if !ok {
					p.error("expected lambda expression after '=' or ':=' in method definition")
				}
			} else {
				p.error("expected lambda expression after '=' or ':=' in method definition")
			}

			methods[ident] = lambda
			p.skipNewlines()
			continue
		}

		p.error("expected '=' or ':=' for method, or '.' for class variable in class body")
	}

	// Expect '}'
	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of class definition")
	}

	return &ClassDecl{
		Name:         name,
		ClassVars:    classVars,
		Methods:      methods,
		Compositions: compositions,
	}
}

func (p *Parser) parseStructLiteral(structName string) *StructLiteralExpr {
	p.nextToken() // skip identifier (now on '{')
	p.nextToken() // skip '{'
	p.skipNewlines()

	fields := make(map[string]Expression)

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		if p.current.Type != TOKEN_IDENT {
			p.error("expected field name in struct literal")
		}
		fieldName := p.current.Value
		p.nextToken() // skip field name

		if p.current.Type != TOKEN_COLON {
			p.error("expected ':' after field name in struct literal")
		}
		p.nextToken() // skip ':'

		fieldValue := p.parseExpression()
		fields[fieldName] = fieldValue

		p.nextToken() // move past expression
		p.skipNewlines()

		if p.current.Type == TOKEN_COMMA {
			p.nextToken() // skip ','
			p.skipNewlines()
		} else if p.current.Type != TOKEN_RBRACE {
			p.error("expected ',' or '}' in struct literal")
		}
	}

	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of struct literal")
	}

	return &StructLiteralExpr{
		StructName: structName,
		Fields:     fields,
	}
}

// Confidence that this function is working: 100%
func (p *Parser) parseStatement() Statement {
	// Check for use keyword (imports)
	if p.current.Type == TOKEN_USE {
		p.nextToken() // skip 'use'
		if p.current.Type != TOKEN_STRING {
			p.error("expected string after 'use'")
		}
		path := p.current.Value
		return &UseStmt{Path: path}
	}

	// Check for export keyword
	if p.current.Type == TOKEN_EXPORT {
		return p.parseExport()
	}

	// Check for import keyword (git URL imports)
	if p.current.Type == TOKEN_IMPORT {
		return p.parseImport()
	}

	// Check for cstruct keyword (C-compatible struct definition)
	if p.current.Type == TOKEN_CSTRUCT {
		return p.parseCStructDecl()
	}

	// Check for class keyword (class definition)
	if p.current.Type == TOKEN_CLASS {
		return p.parseClassDecl()
	}

	// Check for arena keyword
	if p.current.Type == TOKEN_ARENA {
		return p.parseArenaStmt()
	}

	// Check for defer keyword
	if p.current.Type == TOKEN_DEFER {
		return p.parseDeferStmt()
	}

	// Check for alias keyword
	if p.current.Type == TOKEN_ALIAS {
		return p.parseAliasStmt()
	}

	// Check for spawn keyword (spawn process)
	if p.current.Type == TOKEN_SPAWN {
		return p.parseSpawnStmt()
	}

	// Check for ret/err keywords (but not if followed by assignment operator)
	if p.current.Type == TOKEN_RET || p.current.Type == TOKEN_ERR {
		// If TOKEN_RET followed by assignment operator, treat as identifier for assignment
		if p.current.Type == TOKEN_RET &&
			(p.peek.Type == TOKEN_COLON_EQUALS || p.peek.Type == TOKEN_EQUALS ||
				p.peek.Type == TOKEN_LEFT_ARROW || p.peek.Type == TOKEN_COLON ||
				p.peek.Type == TOKEN_PLUS_EQUALS || p.peek.Type == TOKEN_MINUS_EQUALS ||
				p.peek.Type == TOKEN_STAR_EQUALS || p.peek.Type == TOKEN_POWER_EQUALS || p.peek.Type == TOKEN_SLASH_EQUALS ||
				p.peek.Type == TOKEN_MOD_EQUALS) {
			// Treat TOKEN_RET as TOKEN_IDENT for assignment purposes
			// by converting the token type temporarily
			p.current.Type = TOKEN_IDENT
			return p.parseAssignment()
		}
		return p.parseJumpStatement()
	}

	// Check for @++ (continue current loop)
	if p.current.Type == TOKEN_AT_PLUSPLUS {
		return p.parseLoopStatement()
	}

	// Check for parallel loops: @@ or N @
	if p.current.Type == TOKEN_AT_AT {
		// @@ means parallel loop with all cores
		return p.parseLoopStatement()
	}

	// Check for N @ (parallel loop with N threads)
	if p.current.Type == TOKEN_NUMBER && p.peek.Type == TOKEN_AT {
		// This is N @ syntax for parallel loops
		return p.parseLoopStatement()
	}

	// Check for @ (loop)

	// Check for @ (either loop @N, loop @ ident, or jump @N)
	if p.current.Type == TOKEN_AT {
		// Look ahead to distinguish loop vs jump
		// Loop: @N identifier in ... or @ identifier in ...
		// Jump: @N (followed by newline, semicolon, or })
		if p.peek.Type == TOKEN_NUMBER || p.peek.Type == TOKEN_IDENT || p.peek.Type == TOKEN_LBRACE || p.peek.Type == TOKEN_NEWLINE {
			// We need to peek further to distinguish loop from jump
			// For now, let's just parse as loop if it matches the pattern
			// Otherwise treat as jump

			// Simple heuristic: if @ NUMBER IDENTIFIER or @ IDENTIFIER or @ {, it's a loop
			// We can't easily look 2 tokens ahead, so we'll just try parsing as loop first
			return p.parseLoopStatement()
		}
		p.error("expected number or identifier after @ (e.g., @1 i in..., @ i in...)")
	}

	// Check for indexed assignment: ptr[offset] <- value
	if p.current.Type == TOKEN_IDENT && p.peek.Type == TOKEN_LBRACKET {
		// Look ahead to see if this is an indexed assignment (ptr[...] <- ...)
		// We need to check if after the [...] there's a <-

		// Save lexer state for restoration
		lexerState := p.lexer.save()
		savedCurrent := p.current
		savedPeek := p.peek

		p.nextToken() // skip identifier
		p.nextToken() // skip '['

		// Skip over the index expression
		bracketDepth := 1
		for bracketDepth > 0 && p.current.Type != TOKEN_EOF {
			if p.current.Type == TOKEN_LBRACKET {
				bracketDepth++
			} else if p.current.Type == TOKEN_RBRACKET {
				bracketDepth--
			}
			p.nextToken()
		}

		// Check if followed by <-
		isIndexedAssignment := p.current.Type == TOKEN_LEFT_ARROW

		// Restore lexer state
		p.lexer.restore(lexerState)
		p.current = savedCurrent
		p.peek = savedPeek

		if isIndexedAssignment {
			return p.parseIndexedAssignment()
		}
	}

	// Check for `.field` assignment (this.field = value)
	if p.current.Type == TOKEN_DOT && p.peek.Type == TOKEN_IDENT {
		p.nextToken() // skip '.'
		fieldName := "this." + p.current.Value
		p.current.Value = fieldName
		p.current.Type = TOKEN_IDENT
		// Now handle as regular assignment
		if p.peek.Type == TOKEN_EQUALS || p.peek.Type == TOKEN_COLON_EQUALS || p.peek.Type == TOKEN_LEFT_ARROW ||
			p.peek.Type == TOKEN_PLUS_EQUALS || p.peek.Type == TOKEN_MINUS_EQUALS ||
			p.peek.Type == TOKEN_STAR_EQUALS || p.peek.Type == TOKEN_POWER_EQUALS || p.peek.Type == TOKEN_SLASH_EQUALS || p.peek.Type == TOKEN_MOD_EQUALS {
			return p.parseAssignment()
		}
	}

	// Check for shadow keyword (variable declaration)
	if p.current.Type == TOKEN_SHADOW {
		return p.parseAssignment()
	}

	// Check for assignment (=, :=, ->>, <-, with optional type annotation, and compound assignments)
	if p.current.Type == TOKEN_IDENT {
		// Check for multiple assignment: a, b, c = expr
		if p.peek.Type == TOKEN_COMMA {
			// Could be multiple assignment or lambda params - lookahead
			if stmt := p.tryParseMultipleAssignment(); stmt != nil {
				return stmt
			}
		}

		// Check for type annotation: x: num = value
		// This needs to be distinguished from map literal at module level
		// Type annotation: x: <type_keyword> = ...
		// Map literal starts with { or is part of an expression
		if p.peek.Type == TOKEN_COLON && p.functionDepth == 0 {
			// Look ahead to see if this is a type annotation
			// Save state
			saved := p.saveState()
			p.nextToken() // skip identifier
			p.nextToken() // skip ':'

			// Check if next token is a type keyword (contextual - comes as IDENT)
			isTypeAnnotation := false
			if p.current.Type == TOKEN_IDENT {
				switch p.current.Value {
				case "num", "str", "list", "map",
					"cstring", "cptr", "cint", "clong",
					"cfloat", "cdouble", "cbool", "cvoid":
					isTypeAnnotation = true
				}
			}

			// Restore state
			p.restoreState(saved)

			// If it's a type annotation, parse as assignment
			if isTypeAnnotation {
				return p.parseAssignment()
			}
		}

		if p.peek.Type == TOKEN_EQUALS || p.peek.Type == TOKEN_COLON_EQUALS || p.peek.Type == TOKEN_LEFT_ARROW || p.peek.Type == TOKEN_COLON ||
			p.peek.Type == TOKEN_PLUS_EQUALS || p.peek.Type == TOKEN_MINUS_EQUALS ||
			p.peek.Type == TOKEN_STAR_EQUALS || p.peek.Type == TOKEN_POWER_EQUALS || p.peek.Type == TOKEN_SLASH_EQUALS || p.peek.Type == TOKEN_MOD_EQUALS {
			return p.parseAssignment()
		}
	}

	// Otherwise, it's an expression statement (or match expression)
	expr := p.parseExpression()
	if expr != nil {
		// Only parse match blocks if we're not already inside one
		if !p.inMatchBlock && p.peek.Type == TOKEN_LBRACE {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG parseStatement: calling parseMatchBlock with expr=%T, inMatchBlock=%v\n", expr, p.inMatchBlock)
			}
			p.nextToken() // move to '{'
			p.nextToken() // skip '{'
			p.skipNewlines()
			matchExpr := p.parseMatchBlock(expr)
			return &ExpressionStmt{Expr: matchExpr}
		}

		return &ExpressionStmt{Expr: expr}
	}

	return nil
}

// tryParseNonParenLambda attempts to parse a lambda without parentheses: x => expr or x, y => expr
// Returns nil if current position doesn't look like a lambda
func (p *Parser) tryParseNonParenLambda() Expression {
	if p.current.Type != TOKEN_IDENT {
		return nil
	}

	// Single param: x ->
	firstParam := p.current.Value
	if p.peek.Type == TOKEN_ARROW {
		p.nextToken()                         // skip param
		p.nextToken()                         // skip '->'
		p.lambdaParams = []string{firstParam} // Store params for parseLambdaBody
		body := p.parseLambdaBody()
		p.lambdaParams = nil // Clear after use
		return &LambdaExpr{Params: []string{firstParam}, VariadicParam: "", Body: body}
	}

	// If we see => it's not a lambda (it's a match arrow), just return nil
	if p.peek.Type == TOKEN_FAT_ARROW {
		return nil
	}

	// Multi param: x, y, z =>
	// Parameters are comma-separated
	if p.peek.Type != TOKEN_COMMA {
		return nil
	}

	// Collect parameters until we find => or something else
	params := []string{firstParam}

	for p.peek.Type == TOKEN_COMMA {
		p.nextToken() // skip current param
		p.nextToken() // skip ','

		if p.current.Type != TOKEN_IDENT {
			p.error("expected parameter name after ','")
		}

		params = append(params, p.current.Value)

		if p.peek.Type == TOKEN_ARROW {
			// Found the arrow! This is a lambda
			p.nextToken()           // skip last param
			p.nextToken()           // skip '->'
			p.lambdaParams = params // Store params for parseLambdaBody
			body := p.parseLambdaBody()
			p.lambdaParams = nil // Clear after use
			return &LambdaExpr{Params: params, VariadicParam: "", Body: body}
		}

		// If we see => it's not a lambda, just return nil
		if p.peek.Type == TOKEN_FAT_ARROW {
			return nil
		}
	}

	// We have multiple identifiers separated by commas but no arrow following
	p.error(fmt.Sprintf("expected '->' after lambda parameters (%s), got %v", strings.Join(params, ", "), p.peek.Type))
	return nil
}

// parseFString parses an f-string and returns an FStringExpr
// F-strings have the format: f"text {expr} more text {expr2}"
// We convert this to alternating string literals and expressions
func (p *Parser) parseFString() Expression {
	raw := p.current.Value // Raw f-string content without f" and "

	var parts []Expression
	currentPos := 0

	for currentPos < len(raw) {
		// Find next {
		nextBrace := -1
		for i := currentPos; i < len(raw); i++ {
			if raw[i] == '{' {
				// Check if it's escaped {{
				if i+1 < len(raw) && raw[i+1] == '{' {
					i++ // Skip the second {
					continue
				}
				nextBrace = i
				break
			}
		}

		// If no more braces, add remaining text as string literal
		if nextBrace == -1 {
			if currentPos < len(raw) {
				text := raw[currentPos:]
				// Process escape sequences and unescape {{  }}
				text = strings.ReplaceAll(text, "{{", "{")
				text = strings.ReplaceAll(text, "}}", "}")
				text = processEscapeSequences(text)
				parts = append(parts, &StringExpr{Value: text})
			}
			break
		}

		// Add text before { as string literal
		if nextBrace > currentPos {
			text := raw[currentPos:nextBrace]
			// Process escape sequences and unescape {{ }}
			text = strings.ReplaceAll(text, "{{", "{")
			text = strings.ReplaceAll(text, "}}", "}")
			text = processEscapeSequences(text)
			parts = append(parts, &StringExpr{Value: text})
		}

		// Find matching }
		braceDepth := 1
		exprStart := nextBrace + 1
		exprEnd := exprStart
		for exprEnd < len(raw) && braceDepth > 0 {
			if raw[exprEnd] == '{' {
				braceDepth++
			} else if raw[exprEnd] == '}' {
				braceDepth--
			}
			if braceDepth > 0 {
				exprEnd++
			}
		}

		if braceDepth != 0 {
			p.error("unclosed { in f-string")
			return &StringExpr{Value: raw}
		}

		// Parse the expression inside {...}
		exprCode := raw[exprStart:exprEnd]
		exprLexer := NewLexer(exprCode)
		exprParser := NewParser(exprCode)
		exprParser.lexer = exprLexer
		exprParser.current = exprLexer.NextToken()
		exprParser.peek = exprLexer.NextToken()

		expr := exprParser.parseExpression()

		parts = append(parts, expr)

		currentPos = exprEnd + 1 // Skip past the }
	}

	// If only one part and it's a string, return a regular StringExpr
	if len(parts) == 1 {
		if strExpr, ok := parts[0].(*StringExpr); ok {
			return strExpr
		}
	}

	return &FStringExpr{Parts: parts}
}

// Confidence that this function is working: 100%
func (p *Parser) parseAssignment() *AssignStmt {
	// Check for shadow keyword
	hasShadow := false
	if p.current.Type == TOKEN_SHADOW {
		hasShadow = true
		p.nextToken() // skip 'shadow'
	}

	if p.current.Type != TOKEN_IDENT {
		p.error("expected identifier after 'shadow' keyword")
		return nil
	}

	name := p.current.Value
	p.nextToken() // skip identifier

	// Check for type annotation: name: type
	var precision string
	var typeAnnotation *C67Type
	if p.current.Type == TOKEN_COLON && p.peek.Type == TOKEN_IDENT {
		p.nextToken() // skip ':'

		// Try new type system first (num, str, cstring, etc.)
		typeAnnotation = p.parseTypeAnnotation()
		if typeAnnotation != nil {
			p.nextToken() // skip type keyword
		} else {
			// Fall back to legacy precision format (bNN or fNN)
			precision = p.current.Value
			// Validate precision format (bNN or fNN where NN is a number)
			if len(precision) < 2 || (precision[0] != 'b' && precision[0] != 'f') {
				p.error("invalid type annotation: expected num, str, list, map, cstring, cptr, cint, clong, cfloat, cdouble, cbool, cvoid, or legacy bNN/fNN")
			}
			p.nextToken() // skip precision identifier
		}
	}

	// Check for compound assignment operators (+=, -=, *=, **=, /=, %=)
	var compoundOp string
	switch p.current.Type {
	case TOKEN_PLUS_EQUALS:
		compoundOp = "+"
	case TOKEN_MINUS_EQUALS:
		compoundOp = "-"
	case TOKEN_STAR_EQUALS:
		compoundOp = "*"
	case TOKEN_POWER_EQUALS:
		compoundOp = "**"
	case TOKEN_SLASH_EQUALS:
		compoundOp = "/"
	case TOKEN_MOD_EQUALS:
		compoundOp = "%"
	}

	// Determine assignment type
	// := - mutable definition
	// = - immutable definition
	// <- - update (requires existing mutable variable)
	isUpdate := p.current.Type == TOKEN_LEFT_ARROW
	mutable := p.current.Type == TOKEN_COLON_EQUALS || isUpdate

	p.nextToken() // skip '=' or ':=' or '<-' or compound operator

	// For recursive functions at module level, declare the name BEFORE parsing the value
	// This allows the function to reference itself in its body
	// But only for module-level functions (functionDepth == 0)
	// Local variables in functions don't need this (they can't recurse anyway)
	declareNowForRecursion := !isUpdate && p.functionDepth == 0
	if declareNowForRecursion {
		// Declare function name in module scope for recursion
		p.declareVariable(name)
	}

	// Check for non-parenthesized lambda: x -> expr or x y -> expr
	var value Expression

	if p.current.Type == TOKEN_IDENT {
		value = p.tryParseNonParenLambda()
		if value == nil {
			value = p.parseExpression()
		}
	} else {
		value = p.parseExpression()
	}

	// Check for match block after expression
	if p.peek.Type == TOKEN_LBRACE {
		p.nextToken() // move to expression
		p.nextToken() // skip '{'
		p.skipNewlines()
		value = p.parseMatchBlock(value)
	}

	// According to GRAMMAR.md:
	// Zero-argument lambdas: When a statement block or match block is assigned directly
	// without parameters, it should be inferred as a zero-arg lambda.
	// This does NOT apply to map literals (they have : in them).
	//
	// Examples:
	//   main = { println("hello") }      // Inferred: main = -> { println("hello") }
	//   handler = { | x > 0 => "pos" }   // Inferred: handler = -> { | x > 0 => "pos" }
	//   config = { port: 8080 }          // NOT wrapped: map literal
	switch v := value.(type) {
	case *BlockExpr:
		// Statement block -> wrap in zero-arg lambda
		value = &LambdaExpr{Params: []string{}, VariadicParam: "", Body: v}
	case *MatchExpr:
		// Only wrap guardless match blocks in zero-arg lambda
		// Match expressions with a condition variable (e.g., `x { 5 => 42 }`) should execute immediately
		// Guardless matches that reference variables in guards should be wrapped
		// Check if condition is a simple variable reference that matches the assigned variable
		isConditionedMatch := false
		if identExpr, ok := v.Condition.(*IdentExpr); ok {
			// If condition is an identifier, this is a conditioned match like `x { 5 => 42 }`
			// It should execute immediately, not be wrapped in a lambda
			isConditionedMatch = true
			_ = identExpr // avoid unused warning
		}
		if isConditionedMatch {
			// Don't wrap - this is a value match that should execute immediately
		} else {
			// Guardless match block with only guards -> wrap in zero-arg lambda
			// Example: handler = { | x > 0 => "pos" | x < 0 => "neg" }
			value = &LambdaExpr{Params: []string{}, VariadicParam: "", Body: v}
		}
		// MapExpr is NOT wrapped - it's a literal value
	}

	// Check for multiple lambda dispatch: f = (x) -> x, (y) -> y + 1
	if lambda, ok := value.(*LambdaExpr); ok && p.peek.Type == TOKEN_COMMA {
		lambdas := []*LambdaExpr{lambda}

		for p.peek.Type == TOKEN_COMMA {
			p.nextToken() // move to comma
			p.nextToken() // skip comma

			// Try non-parenthesized lambda first
			var nextExpr Expression
			if p.current.Type == TOKEN_IDENT {
				nextExpr = p.tryParseNonParenLambda()
				if nextExpr == nil {
					nextExpr = p.parseExpression()
				}
			} else {
				nextExpr = p.parseExpression()
			}

			if nextLambda, ok := nextExpr.(*LambdaExpr); ok {
				lambdas = append(lambdas, nextLambda)
			} else {
				p.error("expected lambda expression after comma in multiple lambda dispatch")
			}
		}

		// Wrap in MultiLambdaExpr
		value = &MultiLambdaExpr{Lambdas: lambdas}
	}

	// Transform compound assignment: x += 5  =>  x = x + 5
	if compoundOp != "" {
		value = &BinaryExpr{
			Left:     &IdentExpr{Name: name},
			Operator: compoundOp,
			Right:    value,
		}
		// Compound assignments are updates
		isUpdate = true
		mutable = true
	}

	// Check shadowing rules
	// We already declared the variable above for recursion support
	// Now validate that shadowing is correct
	if !isUpdate {
		// We need to check if it WOULD shadow (ignoring the declaration we just made)
		// To do this, we temporarily remove it from current scope, check, then it's already there
		if len(p.scopes) > 0 {
			currentScope := p.scopes[len(p.scopes)-1]
			delete(currentScope, name) // Temporarily remove

			wouldShadowOuter := p.wouldShadow(name)

			// Re-add it
			currentScope[name] = true

			if wouldShadowOuter && !hasShadow {
				p.error(fmt.Sprintf("variable '%s' shadows an outer scope variable - use 'shadow %s = ...' to explicitly shadow", name, name))
			}

			if !wouldShadowOuter && hasShadow {
				p.error(fmt.Sprintf("'shadow' keyword used but '%s' doesn't shadow any outer variable", name))
			}
		}
	}

	// Check if this is a constant definition (uppercase immutable with literal value)
	// Store compile-time constants for substitution
	// Only uppercase identifiers are true constants (cannot be shadowed in practice)
	if !mutable && !isUpdate && isAllUppercase(name) {
		// Store numbers, strings, and lists as compile-time constants
		switch v := value.(type) {
		case *NumberExpr:
			p.constants[name] = v
		case *StringExpr:
			p.constants[name] = v
		case *ListExpr:
			// Only store lists that contain only literal values
			isLiteral := true
			for _, elem := range v.Elements {
				switch elem.(type) {
				case *NumberExpr, *StringExpr:
					// These are literals, OK
				default:
					// Contains expressions, not a pure literal list
					isLiteral = false
				}
			}
			if isLiteral {
				p.constants[name] = v
			}
		}
	}

	return &AssignStmt{
		Name:           name,
		Value:          value,
		Mutable:        mutable,
		IsUpdate:       isUpdate,
		Precision:      precision,
		TypeAnnotation: typeAnnotation,
	}
}

func (p *Parser) tryParseMultipleAssignment() Statement {
	// Try to parse: a, b, c = expr or a, b := expr
	// Must NOT confuse with lambda params: (a, b) => ...
	// Save lexer state in case this is not a multiple assignment
	lexerState := p.lexer.save()
	savedCurrent := p.current
	savedPeek := p.peek

	// Collect identifiers
	names := []string{p.current.Value}
	p.nextToken() // skip first identifier

	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','
		if p.current.Type != TOKEN_IDENT {
			// Not a valid multiple assignment, restore state
			p.lexer.restore(lexerState)
			p.current = savedCurrent
			p.peek = savedPeek
			return nil
		}
		names = append(names, p.current.Value)
		p.nextToken() // skip identifier
	}

	// Check if this is a lambda (=> follows the names)
	if p.current.Type == TOKEN_FAT_ARROW || p.current.Type == TOKEN_ARROW {
		// This is lambda params, not multiple assignment
		p.lexer.restore(lexerState)
		p.current = savedCurrent
		p.peek = savedPeek
		return nil
	}

	// Check for assignment operator
	isUpdate := p.current.Type == TOKEN_LEFT_ARROW
	mutable := p.current.Type == TOKEN_COLON_EQUALS || isUpdate
	isAssign := p.current.Type == TOKEN_EQUALS || mutable

	if !isAssign {
		// Not an assignment, restore state
		p.lexer.restore(lexerState)
		p.current = savedCurrent
		p.peek = savedPeek
		return nil
	}

	p.nextToken() // skip assignment operator

	// Parse the value expression
	value := p.parseExpression()
	if value == nil {
		p.error("expected expression after assignment operator in multiple assignment")
	}

	return &MultipleAssignStmt{
		Names:    names,
		Value:    value,
		Mutable:  mutable,
		IsUpdate: isUpdate,
	}
}

func (p *Parser) parseIndexedAssignment() Statement {
	// Parse: ptr[offset] <- value as type
	// This is syntactic sugar for: write_TYPE(ptr, offset, value)
	// Or for reading: value = ptr[offset] as type  =>  value = read_TYPE(ptr, offset)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseIndexedAssignment: current=%v, peek=%v\n", p.current, p.peek)
	}

	ptrName := p.current.Value
	p.nextToken() // skip identifier

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseIndexedAssignment: after skip ident, current=%v\n", p.current)
	}

	p.nextToken() // skip '['

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseIndexedAssignment: after skip '[', current=%v\n", p.current)
	}

	// Parse the index expression
	indexExpr := p.parseExpression()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseIndexedAssignment: after index expr, current=%v, peek=%v\n", p.current, p.peek)
	}

	// Move to ']'
	p.nextToken()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseIndexedAssignment: after nextToken, current=%v\n", p.current)
	}

	if p.current.Type != TOKEN_RBRACKET {
		p.error("expected ']' after index expression")
	}
	p.nextToken() // skip ']'

	if p.current.Type != TOKEN_LEFT_ARROW {
		p.error("expected '<-' for indexed assignment")
	}
	p.nextToken() // skip '<-'

	// Parse the value expression
	valueExpr := p.parseExpression()

	// Move to the last token of the expression
	p.nextToken()

	// Check if this is an unsafe memory write (with cast) or array update (without cast)
	castExpr, hasCast := valueExpr.(*CastExpr)

	if hasCast {
		// UNSAFE MEMORY WRITE: ptr[offset] <- value as TYPE
		// Transform into: write_TYPE(ptr, offset as int32, value)

		// Map cast type to write function name
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

		shortType, ok := typeMap[castExpr.Type]
		if !ok {
			p.error(fmt.Sprintf("unsupported type for indexed write: %s", castExpr.Type))
		}

		// Create a CallExpr to write_TYPE(ptr, offset as int32, value)
		funcName := "write_" + shortType

		// Cast offset to int32 for write functions
		offsetCast := &CastExpr{
			Expr: indexExpr,
			Type: "int32",
		}

		args := []Expression{
			&IdentExpr{Name: ptrName},
			offsetCast,
			castExpr.Expr,
		}

		writeCall := &CallExpr{
			Function: funcName,
			Args:     args,
		}

		return &ExpressionStmt{Expr: writeCall}
	}
	// ARRAY UPDATE: arr[idx] <- value
	// Create a direct map update statement
	return &MapUpdateStmt{
		MapName: ptrName,
		Index:   indexExpr,
		Value:   valueExpr,
	}
}

// BlockType represents the type of block as determined by disambiguation
type BlockType int

const (
	BlockTypeMap       BlockType = iota // {key: value, ...}
	BlockTypeMatch                      // {pattern -> result, ...} or {| guard -> result}
	BlockTypeStatement                  // {stmt; stmt; expr}
)

// parseMapLiteralBody parses the body of a map literal (assumes '{' already consumed)
// Supports both identifier keys (hashed) and expression keys
// Format: key: value, key2: value2, ...
func (p *Parser) parseMapLiteralBody() *MapExpr {
	keys := []Expression{}
	values := []Expression{}

	if p.current.Type != TOKEN_RBRACE {
		// Parse first key
		var key Expression
		if p.current.Type == TOKEN_IDENT && p.peek.Type == TOKEN_COLON {
			// String key: hash identifier to uint64
			hashValue := hashStringKey(p.current.Value)
			key = &NumberExpr{Value: float64(hashValue)}
			p.nextToken() // move past identifier
		} else {
			// Numeric key or expression
			key = p.parseExpression()
			p.nextToken() // move past key
		}

		// Must have ':'
		if p.current.Type != TOKEN_COLON {
			p.error("expected ':' in map literal")
		}
		p.nextToken() // skip ':'

		// Parse value
		value := p.parseExpression()
		keys = append(keys, key)
		values = append(values, value)

		// Parse additional key:value pairs
		for p.peek.Type == TOKEN_COMMA {
			p.nextToken() // skip current value
			p.nextToken() // skip ','

			// Parse key (string or numeric)
			if p.current.Type == TOKEN_IDENT && p.peek.Type == TOKEN_COLON {
				// String key: hash identifier to uint64
				hashValue := hashStringKey(p.current.Value)
				key = &NumberExpr{Value: float64(hashValue)}
				p.nextToken() // move past identifier
			} else {
				// Numeric key or expression
				key = p.parseExpression()
				p.nextToken() // move past key
			}

			if p.current.Type != TOKEN_COLON {
				p.error("expected ':' in map literal")
			}
			p.nextToken() // skip ':'

			value := p.parseExpression()
			keys = append(keys, key)
			values = append(values, value)
		}
	}

	// current should be on last value or on '{'
	// peek should be '}'
	p.nextToken() // move to '}'
	return &MapExpr{Keys: keys, Values: values}
}

// disambiguateBlock determines block type according to GRAMMAR.md rules:
// 1. Contains ':' before any arrows → Map literal
// 2. Contains '->' or '~>' → Match block
// 3. Otherwise → Statement block
func (p *Parser) disambiguateBlock() BlockType {
	// Quick check: if next token (after {) is }, it's an empty map
	if p.peek.Type == TOKEN_RBRACE {
		return BlockTypeMap
	}

	// Create temporary lexer for lookahead
	tempLexer := &Lexer{
		input:     p.lexer.input,
		pos:       p.lexer.pos,
		line:      p.lexer.line,
		column:    p.lexer.column,
		lineStart: p.lexer.lineStart,
	}

	braceDepth := 1 // Start at 1 because we're already inside the opening {
	foundColon := false
	foundArrow := false

	// Scan tokens within this block
	for i := 0; i < maxBlockIterations; i++ {
		tok := tempLexer.NextToken()

		if tok.Type == TOKEN_EOF {
			break
		}

		if tok.Type == TOKEN_LBRACE {
			braceDepth++
		} else if tok.Type == TOKEN_RBRACE {
			braceDepth--
			if braceDepth == 0 {
				// Exited the block
				break
			}
		} else if braceDepth == 1 {
			// At top level of this block
			if tok.Type == TOKEN_COLON && !foundArrow {
				// Check if this is a type annotation (x: num = ...) vs map literal (x: value)
				// Type annotations have a type keyword (as identifier) after the colon
				nextTok := tempLexer.NextToken()
				isTypeAnnotation := false
				// Type keywords are contextual - they come as TOKEN_IDENT with specific values
				if nextTok.Type == TOKEN_IDENT {
					switch nextTok.Value {
					case "num", "str", "list", "map",
						"cstring", "cptr", "cint", "clong",
						"cfloat", "cdouble", "cbool", "cvoid":
						isTypeAnnotation = true
					}
				}
				if !isTypeAnnotation {
					// Found ':' before any arrows and not a type annotation → map literal
					foundColon = true
				}
			} else if tok.Type == TOKEN_FAT_ARROW || tok.Type == TOKEN_DEFAULT_ARROW {
				// Found arrow → match block (=> or ~>)
				foundArrow = true
				break
			} else if tok.Type == TOKEN_UNDERSCORE {
				// Check if next token is =>
				nextTok := tempLexer.NextToken()
				if nextTok.Type == TOKEN_FAT_ARROW {
					// Found _ => → match block
					foundArrow = true
					break
				}
			}
		}
	}

	// Apply disambiguation rules in order
	if foundColon && !foundArrow {
		return BlockTypeMap
	}
	if foundArrow {
		return BlockTypeMatch
	}
	return BlockTypeStatement
}

// blockContainsMatchArrows scans ahead to check if the block contains => or ~> arrows
// Returns true if any match arrows are found, false otherwise
func (p *Parser) blockContainsMatchArrows() bool {
	// Create a new lexer from the same source at the current position
	// to scan ahead without modifying the parser state
	tempLexer := &Lexer{
		input:     p.lexer.input,
		pos:       p.lexer.pos,
		line:      p.lexer.line,
		column:    p.lexer.column,
		lineStart: p.lexer.lineStart,
	}

	tempParser := &Parser{
		lexer:    tempLexer,
		current:  p.current,
		peek:     p.peek,
		filename: p.filename,
		source:   p.source,
	}

	braceDepth := 0
	foundArrow := false

	// Scan through tokens until we exit the block
	for i := 0; i < maxASTIterations; i++ {
		if tempParser.current.Type == TOKEN_EOF {
			break
		}

		if tempParser.current.Type == TOKEN_LBRACE {
			braceDepth++
		} else if tempParser.current.Type == TOKEN_RBRACE {
			braceDepth--
			if braceDepth < 0 {
				// We've exited the block
				break
			}
		} else if braceDepth == 0 && (tempParser.current.Type == TOKEN_FAT_ARROW || tempParser.current.Type == TOKEN_ARROW || tempParser.current.Type == TOKEN_DEFAULT_ARROW ||
			(tempParser.current.Type == TOKEN_UNDERSCORE && tempParser.peek.Type == TOKEN_FAT_ARROW)) {
			// Found an arrow at the top level of the block (=>, ->, ~>, or _ =>)
			foundArrow = true
			break
		}

		tempParser.nextToken()
	}

	return foundArrow
}

// parseMatchBlock parses a match block according to GRAMMAR.md:
//
// TWO FORMS:
//
//  1. Value Match (with expression before {):
//     Evaluates expression once, matches result against patterns
//     Example: x { 0 -> "zero"  5 -> "five"  ~> "other" }
//     The condition parameter contains the evaluated expression
//
//  2. Guard Match (no expression, uses | at line start):
//     Each | branch evaluates independently (short-circuits)
//     Example: { | x == 0 -> "zero"  | x > 0 -> "positive"  ~> "negative" }
//     The condition parameter is typically nil or a boolean true
//
// Both forms support:
// - Match clauses: pattern -> result  or  | guard -> result
// - Default clause: ~> result
//
// The | is only a guard marker when at the start of a line.
// Otherwise | is the pipe operator.
func (p *Parser) parseMatchBlock(condition Expression) *MatchExpr {
	// Set flag to prevent nested match block parsing
	oldInMatchBlock := p.inMatchBlock
	p.inMatchBlock = true
	defer func() { p.inMatchBlock = oldInMatchBlock }()

	clauses := []*MatchClause{}
	defaultExpr := Expression(&NumberExpr{Value: 0})
	defaultExplicit := false

	p.skipNewlines()

	// Check if the block contains any match arrows (-> or ~>) to decide parsing mode
	hasMatchArrows := p.blockContainsMatchArrows()
	if debugParser {
		fmt.Fprintf(os.Stderr, "DEBUG: parseMatchBlock hasMatchArrows=%v current=%v\n", hasMatchArrows, p.current.Type)
	}

	// Simple conditional mode: if the block doesn't contain match arrows,
	// treat it as a simple conditional that should execute all statements as one block
	isDefaultMatch := p.current.Type == TOKEN_DEFAULT_ARROW || (p.current.Type == TOKEN_UNDERSCORE && p.peek.Type == TOKEN_FAT_ARROW)
	if !hasMatchArrows && p.current.Type != TOKEN_ARROW && !isDefaultMatch && p.current.Type != TOKEN_RBRACE {
		// Try parsing as simple conditional first
		var statements []Statement
		foundDefaultArrow := false

		for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
			// Check if we encounter default arrow at top level (~> or _ =>)
			if p.current.Type == TOKEN_DEFAULT_ARROW || (p.current.Type == TOKEN_UNDERSCORE && p.peek.Type == TOKEN_FAT_ARROW) {
				foundDefaultArrow = true
				break
			}

			// Check for explicit arrow (not allowed in simple mode)
			if p.current.Type == TOKEN_FAT_ARROW {
				p.error("mix of simple statements and pattern matching not supported - use explicit '=>' syntax")
			}

			// Parse as statement/expression
			// Try parseStatement first, which handles assignments and other statements
			startType := p.current.Type
			stmt := p.parseStatement()

			// Check if parseStatement actually consumed anything
			if stmt != nil {
				statements = append(statements, stmt)
				p.nextToken() // Advance past the statement
			} else if startType == p.current.Type {
				// parseStatement didn't consume anything, might be an expression
				// that should be treated as a statement (like a function call)
				expr := p.parseExpression()
				if expr != nil {
					statements = append(statements, &ExpressionStmt{Expr: expr})
				}
				p.nextToken()
			}

			// Skip separators between statements
			p.skipNewlines()
			if p.current.Type == TOKEN_SEMICOLON {
				p.nextToken()
				p.skipNewlines()
			}
		}

		// If we found a default arrow after statements, treat the statements as guardless clause
		if foundDefaultArrow && len(statements) > 0 {
			// Create a guardless clause with all statements before ~>
			clauses = append(clauses, &MatchClause{
				Result: &BlockExpr{Statements: statements},
			})
			// Fall through to continue parsing default clause and other clauses
		} else if !foundDefaultArrow && len(statements) > 0 {
			// If we didn't find any arrows and have statements, this is a simple conditional
			clauses = append(clauses, &MatchClause{
				Result: &BlockExpr{Statements: statements},
			})

			if p.current.Type != TOKEN_RBRACE {
				p.error("expected '}' after conditional block")
			}

			return &MatchExpr{
				Condition:       condition,
				Clauses:         clauses,
				DefaultExpr:     defaultExpr,
				DefaultExplicit: defaultExplicit,
			}
		}
	}

	// Pattern matching mode: parse clauses with guards/arrows
	loopCount := 0
	for {
		loopCount++
		if loopCount > 10 {
			p.error(fmt.Sprintf("infinite loop in parseMatchBlock: stuck at token type=%v value='%v' line=%d", p.current.Type, p.current.Value, p.current.Line))
		}

		if debugParser {
			fmt.Fprintf(os.Stderr, "DEBUG parseMatchBlock loop %d: current=%v peek=%v\n", loopCount, p.current, p.peek)
		}
		p.skipNewlines()

		if p.current.Type == TOKEN_RBRACE {
			if debugParser {
				fmt.Fprintf(os.Stderr, "DEBUG parseMatchBlock: breaking at RBRACE\n")
			}
			break
		}

		// Check for default match: ~> or _ =>
		if p.current.Type == TOKEN_DEFAULT_ARROW || (p.current.Type == TOKEN_UNDERSCORE && p.peek.Type == TOKEN_FAT_ARROW) {
			if defaultExplicit {
				p.error("duplicate default clause in match block")
			}
			defaultExplicit = true
			if p.current.Type == TOKEN_UNDERSCORE {
				p.nextToken() // skip '_'
				p.nextToken() // skip '=>'
			} else {
				p.nextToken() // skip '~>'
			}
			p.skipNewlines()
			defaultExpr = p.parseMatchTarget()
			p.skipNewlines()
			continue
		}

		clause, _ := p.parseMatchClause()

		// Convert value matches to equality checks
		if clause.IsValueMatch && clause.Guard != nil {
			// Transform: 0 -> "zero" into: condition == 0 -> "zero"
			clause.Guard = &BinaryExpr{
				Left:     condition,
				Operator: "==",
				Right:    clause.Guard,
			}
			clause.IsValueMatch = false
		}

		clauses = append(clauses, clause)
	}

	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' after match block")
	}

	if len(clauses) == 0 && !defaultExplicit {
		p.error("match block must contain a clause or default")
	}

	return &MatchExpr{
		Condition:       condition,
		Clauses:         clauses,
		DefaultExpr:     defaultExpr,
		DefaultExplicit: defaultExplicit,
	}
}

// parseMatchClause parses a single match clause:
//
// Forms:
// 1. Guardless: => result
// 2. Value pattern: value => result
// 3. Guard: | condition => result   (| only when at line start)
//
// Returns (clause, isBareExpression)
// where isBareExpression means no explicit arrow was used
func (p *Parser) parseMatchClause() (*MatchClause, bool) {
	// Guardless clause starting with '=>' (explicit)
	if p.current.Type == TOKEN_FAT_ARROW {
		p.nextToken() // skip '=>'
		p.skipNewlines()
		result := p.parseMatchTarget()
		p.skipNewlines()
		return &MatchClause{Result: result}, false
	}

	// Guardless clause without '->' (implicit): check for statement-only tokens
	// These tokens can only appear in match targets, not as guard expressions:
	// - ret, err (return statements)
	// - @++, @N (jump statements)
	// - { (block statements)
	// - identifier <- or identifier = (assignment statements)
	isStatementToken := p.current.Type == TOKEN_RET ||
		p.current.Type == TOKEN_ERR ||
		p.current.Type == TOKEN_AT_PLUSPLUS ||
		p.current.Type == TOKEN_LBRACE ||
		(p.current.Type == TOKEN_AT && p.peek.Type == TOKEN_NUMBER) ||
		(p.current.Type == TOKEN_IDENT && (p.peek.Type == TOKEN_LEFT_ARROW || p.peek.Type == TOKEN_EQUALS))

	if isStatementToken {
		// Treat as guardless clause (implicit '=>'), not a bare clause
		result := p.parseMatchTarget()
		p.skipNewlines()
		return &MatchClause{Result: result}, false
	}

	// Check for guard prefix | (only at line start in match blocks)
	// Important: | is guard marker ONLY at line start in match context
	// Otherwise | is the pipe operator
	isGuard := false
	if p.current.Type == TOKEN_PIPE {
		isGuard = true
		p.nextToken() // skip '|'
		p.skipNewlines()
	}

	// Parse the pattern or guard expression
	if debugParser {
		fmt.Fprintf(os.Stderr, "DEBUG parseMatchClause: before parseExpression, current=%v peek=%v\n", p.current, p.peek)
	}
	expr := p.parseExpression()
	if debugParser {
		fmt.Fprintf(os.Stderr, "DEBUG parseMatchClause: after parseExpression, current=%v peek=%v\n", p.current, p.peek)
	}

	p.nextToken()
	if debugParser {
		fmt.Fprintf(os.Stderr, "DEBUG parseMatchClause: after nextToken, current=%v peek=%v\n", p.current, p.peek)
	}
	p.skipNewlines()

	if p.current.Type == TOKEN_FAT_ARROW || p.current.Type == TOKEN_ARROW {
		p.nextToken() // skip '=>' or '->'
		p.skipNewlines()
		result := p.parseMatchTarget()
		p.skipNewlines()

		// If it's a guard, use the expression as-is
		// Otherwise, it's a value match - we'll need the condition from parseMatchBlock
		if isGuard {
			return &MatchClause{Guard: expr, Result: result}, false
		}
		// Store the value pattern in Guard for now - parseMatchBlock will convert it
		return &MatchClause{Guard: expr, Result: result, IsValueMatch: true}, false
	}

	// Bare expression clause (sugar for '=> expr')
	return &MatchClause{Result: expr}, true
}

func (p *Parser) parseMatchTarget() Expression {
	switch p.current.Type {
	case TOKEN_LBRACE:
		// Parse a block of statements as the match target
		// This allows multi-statement match arms like:
		//   condition {
		//       { stmt1; stmt2; stmt3 }
		//   }
		p.nextToken() // skip '{'
		p.skipNewlines()

		var statements []Statement
		loopCount := 0
		for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
			loopCount++
			if loopCount > maxASTIterations {
				p.error(fmt.Sprintf("infinite loop in parseMatchTarget block: stuck at token %v", p.current))
			}

			stmt := p.parseStatement()
			if stmt != nil {
				statements = append(statements, stmt)
			}

			// Skip separators between statements
			if p.peek.Type == TOKEN_NEWLINE || p.peek.Type == TOKEN_SEMICOLON {
				p.nextToken()
				p.skipNewlines()
			} else if p.peek.Type == TOKEN_RBRACE || p.peek.Type == TOKEN_EOF {
				p.nextToken() // move to '}'
				break
			} else {
				p.nextToken()
				p.skipNewlines()
			}
		}

		if p.current.Type != TOKEN_RBRACE {
			p.error("expected '}' at end of match block")
		}

		// Consume the closing '}'
		p.nextToken()

		return &BlockExpr{Statements: statements}

	case TOKEN_RET, TOKEN_ERR:
		// ret/err or ret @N or ret value or ret @N value
		p.nextToken() // skip 'ret'/'err'

		label := 0 // 0 means return from function
		var value Expression

		// Check for optional @N
		if p.current.Type == TOKEN_AT {
			p.nextToken() // skip '@'
			if p.current.Type != TOKEN_NUMBER {
				p.error("expected number after @ in ret statement")
			}
			labelNum, err := strconv.ParseFloat(p.current.Value, 64)
			if err != nil {
				p.error("invalid loop label number")
			}
			label = int(labelNum)
			if label < 1 {
				p.error("loop label must be >= 1 (use @1, @2, @3, etc.)")
			}
			p.nextToken() // skip number
		}

		// Check for optional value (stop at ~> or _ =>)
		isDefaultMatch := p.current.Type == TOKEN_DEFAULT_ARROW || (p.current.Type == TOKEN_UNDERSCORE && p.peek.Type == TOKEN_FAT_ARROW)
		if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF && !isDefaultMatch {
			value = p.parseExpression()
			p.nextToken()
		}

		// Return a JumpExpr with IsBreak semantics (ret exits loop)
		return &JumpExpr{Label: label, Value: value, IsBreak: true}
	case TOKEN_AT_PLUSPLUS:
		if p.loopDepth < 1 {
			p.error("@++ requires at least 1 loop")
		}
		p.nextToken() // skip '@++'
		// Check for optional return value: @++ value
		var value Expression
		if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
			value = p.parseExpression()
			p.nextToken()
		}
		return &JumpExpr{Label: p.loopDepth, Value: value, IsBreak: false}
	case TOKEN_AT:
		p.nextToken() // skip '@'
		if p.current.Type != TOKEN_NUMBER {
			p.error("expected number after @ in match block")
		}
		labelNum, err := strconv.ParseFloat(p.current.Value, 64)
		if err != nil {
			p.error("invalid label number")
		}
		label := int(labelNum)
		p.nextToken() // skip label number
		// Check for optional return value: @N value
		var value Expression
		if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
			value = p.parseExpression()
			p.nextToken()
		}
		// @N is continue (jump to top of loop N), not break
		return &JumpExpr{Label: label, Value: value, IsBreak: false}
	case TOKEN_IDENT:
		// Check if this is an assignment statement (x <- value or x = value)
		if p.peek.Type == TOKEN_LEFT_ARROW || p.peek.Type == TOKEN_EQUALS {
			// Parse as an assignment statement wrapped in a block
			stmt := p.parseStatement()
			// After parseStatement, p.current is at the last token of the statement
			// We need to advance past it for the caller
			p.nextToken()
			return &BlockExpr{Statements: []Statement{stmt}}
		}
		// Otherwise parse as expression
		fallthrough
	default:
		expr := p.parseExpression()

		// Check if this expression has a match block attached
		if p.peek.Type == TOKEN_LBRACE {
			p.nextToken() // move to expr
			p.nextToken() // move to '{'
			p.nextToken() // skip '{'
			p.skipNewlines()
			matchExpr := p.parseMatchBlock(expr)
			// parseMatchBlock leaves p.current on '}', we need to consume it
			if p.current.Type == TOKEN_RBRACE {
				p.nextToken() // consume '}'
			}
			return matchExpr
		}

		p.nextToken()
		return expr
	}
}

func (p *Parser) parseLoopStatement() Statement {
	// Handle @++ token (continue current loop)
	if p.current.Type == TOKEN_AT_PLUSPLUS {
		// @++ means continue current loop (jump to @N where N is current loop depth)
		if p.loopDepth < 1 {
			p.error("@++ requires at least 1 loop")
		}
		// @++ is continue semantics (not break)
		return &JumpStmt{IsBreak: false, Label: p.loopDepth, Value: nil}
	}

	// Parse parallel loop prefix: @@ or N @
	numThreads := 0 // 0 = sequential, -1 = all cores, N = specific count
	label := p.loopDepth + 1

	// Handle @@ token (parallel loop with all cores)
	if p.current.Type == TOKEN_AT_AT {
		numThreads = -1
		p.nextToken() // skip '@@'

		// Skip newlines after '@@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// After @@, fall through to identifier parsing below
		// (we'll add the parsing code after the TOKEN_AT block)
	} else if p.current.Type == TOKEN_NUMBER {
		// Handle N @ syntax (parallel loop with N threads)
		threadCount, err := strconv.Atoi(p.current.Value)
		if err != nil || threadCount < 1 {
			p.error("thread count must be a positive integer")
		}
		numThreads = threadCount
		p.nextToken() // skip number

		// Expect @ token after the number
		if p.current.Type != TOKEN_AT {
			p.error("expected @ after thread count")
		}
		p.nextToken() // skip '@'

		// Skip newlines after '@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// After N @, fall through to identifier parsing below
	} else if p.current.Type == TOKEN_AT {
		// Handle @ token (start loop at @(N+1))
		// @ means start a loop at @(N+1) where N is current loop depth
		p.nextToken() // skip '@'

		// Skip newlines after '@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Check if this is @N (numbered loop) or @ ident (simple loop)
		// But also check for condition loop: @ NUMBER max N { }
		if p.current.Type == TOKEN_NUMBER {
			// Check if this is a condition loop: @ NUMBER max N {
			// If peek is 'max', it's a condition loop, not a jump
			if p.peek.Type != TOKEN_MAX {
				// This is @N jump syntax, handle it in the jump statement section
				p.current.Type = TOKEN_AT // restore token type
				goto handleJump
			}
			// Otherwise, fall through to condition loop parsing below
		}

		// Check for infinite loop syntax: @ { ... }
		if p.current.Type == TOKEN_LBRACE {
			// Skip newlines after '{'
			for p.peek.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Track loop depth for nested loops
			oldDepth := p.loopDepth
			p.loopDepth = label
			defer func() { p.loopDepth = oldDepth }()

			// Parse loop body
			var body []Statement
			for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
				p.nextToken()
				if p.current.Type == TOKEN_NEWLINE {
					continue
				}
				stmt := p.parseStatement()
				if stmt != nil {
					body = append(body, stmt)
				}
			}

			// Expect and consume '}'
			if p.peek.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of loop body")
			}
			p.nextToken() // consume the '}'

			// Check for optional 'max' clause after the loop body
			var maxIterations int64 = math.MaxInt64
			needsMaxCheck := true

			if p.peek.Type == TOKEN_MAX {
				p.nextToken() // advance to 'max'
				p.nextToken() // skip 'max'

				// Parse max iterations: either a number or 'inf'
				if p.current.Type == TOKEN_INF {
					maxIterations = math.MaxInt64
					p.nextToken()
				} else if p.current.Type == TOKEN_NUMBER {
					maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
					if err != nil || maxInt < 1 {
						p.error("max iterations must be a positive integer or 'inf'")
					}
					maxIterations = maxInt
					p.nextToken()
				} else {
					p.error("expected number or 'inf' after 'max' keyword")
				}
			}

			// Create synthetic range 0..<limit with max for infinite loop
			return &LoopStmt{
				Iterator:      "_",
				Iterable:      &RangeExpr{Start: &NumberExpr{Value: 0}, End: &NumberExpr{Value: 1000000}},
				Body:          body,
				MaxIterations: maxIterations,
				NeedsMaxCheck: needsMaxCheck,
				NumThreads:    numThreads,
			}
		}

		// At this point, we need to determine the loop type:
		// 1. @ ident in expr { } - for-each loop
		// 2. @ ident, ident in expr { } - receive loop
		// 3. @ expr max N { } - condition loop

		// Check for condition loop: if we don't have an identifier followed by 'in' or ','
		// then it's a condition expression
		isConditionLoop := false
		if p.current.Type != TOKEN_IDENT {
			// Not an identifier, must be start of condition expression (or error)
			isConditionLoop = true
		} else {
			// Have identifier - check what comes after
			if p.peek.Type != TOKEN_IN && p.peek.Type != TOKEN_COMMA {
				// Not followed by 'in' or ',' - must be condition loop
				isConditionLoop = true
			}
		}

		if isConditionLoop {
			// Condition loop: @ expr max N { ... }
			// Set flag to prevent parsePrimary from consuming 'max' as recursion limit
			oldInConditionLoop := p.inConditionLoop
			p.inConditionLoop = true
			defer func() { p.inConditionLoop = oldInConditionLoop }()

			// Parse the condition expression using parseComparison
			// This handles comparisons (i < 5), function calls (check()), etc.
			// but avoids match block parsing that would consume the { token
			condition := p.parseComparison()

			// After parsing postfix expression, peek should be on 'max'
			if p.peek.Type != TOKEN_MAX {
				p.error("condition loop requires 'max' clause (e.g., @ n < 5 max 10 { ... })")
			}

			p.nextToken() // move to 'max'
			p.nextToken() // skip 'max', now current is on the number/inf

			// Parse max iterations: either a number or 'inf'
			var maxIterations int64
			if p.current.Type == TOKEN_INF {
				maxIterations = math.MaxInt64
				p.nextToken() // skip 'inf'
			} else if p.current.Type == TOKEN_NUMBER {
				maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
				if err != nil || maxInt < 1 {
					p.error("max iterations must be a positive integer or 'inf'")
				}
				maxIterations = maxInt
				p.nextToken() // skip number
			} else {
				p.error("expected number or 'inf' after 'max' keyword")
			}

			// Skip newlines before '{'
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Expect '{'
			if p.current.Type != TOKEN_LBRACE {
				p.error("expected '{' to start loop body")
			}

			// Skip newlines after '{'
			for p.peek.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Track loop depth for nested loops
			oldDepth := p.loopDepth
			p.loopDepth = label
			defer func() { p.loopDepth = oldDepth }()

			// Parse loop body
			var body []Statement
			for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
				p.nextToken()
				if p.current.Type == TOKEN_NEWLINE {
					continue
				}
				stmt := p.parseStatement()
				if stmt != nil {
					body = append(body, stmt)
				}
			}

			// Expect and consume '}'
			if p.peek.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of loop body")
			}
			p.nextToken() // consume the '}'

			// Return a WhileStmt for condition-based loops
			return &WhileStmt{
				Condition:     condition,
				Body:          body,
				MaxIterations: maxIterations,
				NumThreads:    numThreads,
			}
		}

		// For-each or receive loop - we have an identifier
		firstIdent := p.current.Value
		p.nextToken() // skip identifier

		// Check if this is a receive loop: @ msg, from in ":5000"
		if p.current.Type == TOKEN_COMMA {
			p.nextToken() // skip comma

			// Skip newlines after comma
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Expect second identifier
			if p.current.Type != TOKEN_IDENT {
				p.error("expected identifier after comma in receive loop")
			}
			secondIdent := p.current.Value
			p.nextToken() // skip second identifier

			// Expect 'in' keyword
			if p.current.Type != TOKEN_IN {
				p.error("expected 'in' in receive loop")
			}
			p.nextToken() // skip 'in'

			// Parse address expression
			address := p.parseExpression()

			// Expect opening brace for body
			if p.peek.Type != TOKEN_LBRACE {
				p.error("expected '{' after receive loop address")
			}
			p.nextToken() // move to '{'

			// Track loop depth for nested loops
			oldDepth := p.loopDepth
			p.loopDepth = label
			defer func() { p.loopDepth = oldDepth }()

			// Parse loop body
			var body []Statement
			for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
				p.nextToken()
				if p.current.Type == TOKEN_NEWLINE {
					continue
				}
				stmt := p.parseStatement()
				if stmt != nil {
					body = append(body, stmt)
				}
			}

			// Consume closing brace
			if p.peek.Type == TOKEN_RBRACE {
				p.nextToken() // move to '}'
			}

			return &ReceiveLoopStmt{
				MessageVar: firstIdent,
				SenderVar:  secondIdent,
				Address:    address,
				Body:       body,
			}
		}

		// Check if this is a for-each loop (@ i in list) or a condition loop (@ i < 5)
		if p.current.Type == TOKEN_IN {
			// For-each loop: @ identifier in expression
			iterator := firstIdent
			p.nextToken() // skip 'in'

			// Parse iterable expression
			iterable := p.parseExpression()

			// Determine max iterations and whether runtime checking is needed
			var maxIterations int64
			needsRuntimeCheck := false

			// Check if max keyword is present
			if p.peek.Type == TOKEN_MAX {
				p.nextToken() // advance to 'max'
				p.nextToken() // skip 'max'

				// Explicit max always requires runtime checking
				needsRuntimeCheck = true

				// Parse max iterations: either a number or 'inf'
				if p.current.Type == TOKEN_INF {
					maxIterations = math.MaxInt64 // Use MaxInt64 for infinite iterations
					p.nextToken()                 // skip 'inf'
				} else if p.current.Type == TOKEN_NUMBER {
					// Parse the number
					maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
					if err != nil || maxInt < 1 {
						p.error("max iterations must be a positive integer or 'inf'")
					}
					maxIterations = maxInt
					p.nextToken() // skip number
				} else {
					p.error("expected number or 'inf' after 'max' keyword")
				}
			} else {
				// No explicit max - check if we can determine iteration count at compile time
				if rangeExpr, ok := iterable.(*RangeExpr); ok {
					// Try to calculate max from range: end - start
					startVal, startOk := rangeExpr.Start.(*NumberExpr)
					endVal, endOk := rangeExpr.End.(*NumberExpr)

					if startOk && endOk {
						// Literal range - known at compile time, no runtime check needed
						start := int64(startVal.Value)
						end := int64(endVal.Value)
						maxIterations = end - start
						if maxIterations < 0 {
							maxIterations = 0
						}
						needsRuntimeCheck = false
					} else {
						// Range bounds are not literals, require explicit max
						p.error("loop over non-literal range requires explicit 'max' clause")
					}
				} else if listExpr, ok := iterable.(*ListExpr); ok {
					// List literal - known at compile time, no runtime check needed
					maxIterations = int64(len(listExpr.Elements))
					needsRuntimeCheck = false
				} else if _, ok := iterable.(*IdentExpr); ok {
					// Variable (could be a list or map) - use runtime length check
					maxIterations = math.MaxInt64 // Use max value, will check length at runtime
					needsRuntimeCheck = true
				} else if _, ok := iterable.(*IndexExpr); ok {
					// Indexed expression (e.g., lists[0]) - use runtime length check
					maxIterations = math.MaxInt64
					needsRuntimeCheck = true
				} else {
					// Not a range expression or list literal, require explicit max
					p.error("loop requires 'max' clause (or use range expression like 0..<10 or list literal)")
				}
				// Advance to next token after iterable expression
				p.nextToken()
			}

			// Skip newlines before '{'
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Expect '{'
			if p.current.Type != TOKEN_LBRACE {
				p.error("expected '{' to start loop body")
			}

			// Skip newlines after '{'
			for p.peek.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Track loop depth for nested loops
			oldDepth := p.loopDepth
			p.loopDepth = label
			defer func() { p.loopDepth = oldDepth }()

			// Parse loop body
			var body []Statement
			for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
				p.nextToken()
				if p.current.Type == TOKEN_NEWLINE {
					continue
				}
				stmt := p.parseStatement()
				if stmt != nil {
					body = append(body, stmt)
				}
			}

			// Expect and consume '}'
			if p.peek.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of loop body")
			}
			p.nextToken() // consume the '}'

			return &LoopStmt{
				Iterator:      iterator,
				Iterable:      iterable,
				Body:          body,
				MaxIterations: maxIterations,
				NeedsMaxCheck: needsRuntimeCheck,
				NumThreads:    numThreads,
			}
		}
	}

	// Common identifier and loop body parsing for @@ and N @
	// Only execute this if we have parallel loop prefix
	if numThreads != 0 {
		// Expect identifier for loop variable
		if p.current.Type != TOKEN_IDENT {
			p.error("expected identifier after parallel loop prefix")
		}
		iterator := p.current.Value
		p.nextToken() // skip identifier

		// Check for receive loop syntax - not supported for parallel loops
		if p.current.Type == TOKEN_COMMA {
			p.error("receive loops (@ msg, from in ...) cannot be parallel")
		}

		// Expect 'in' keyword
		if p.current.Type != TOKEN_IN {
			p.error("expected 'in' in loop statement")
		}
		p.nextToken() // skip 'in'

		// Parse iterable expression
		iterable := p.parseExpression()

		// Determine max iterations and whether runtime checking is needed
		var maxIterations int64
		needsRuntimeCheck := false

		// Check if max keyword is present
		if p.peek.Type == TOKEN_MAX {
			p.nextToken() // advance to 'max'
			p.nextToken() // skip 'max'

			// Explicit max always requires runtime checking
			needsRuntimeCheck = true

			// Parse max iterations: either a number or 'inf'
			if p.current.Type == TOKEN_INF {
				maxIterations = math.MaxInt64 // Use MaxInt64 for infinite iterations
				p.nextToken()                 // skip 'inf'
			} else if p.current.Type == TOKEN_NUMBER {
				// Parse the number
				maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
				if err != nil || maxInt < 1 {
					p.error("max iterations must be a positive integer or 'inf'")
				}
				maxIterations = maxInt
				p.nextToken() // skip number
			} else {
				p.error("expected number or 'inf' after 'max' keyword")
			}
		} else {
			// No explicit max - check if we can determine iteration count at compile time
			if rangeExpr, ok := iterable.(*RangeExpr); ok {
				// Try to calculate max from range: end - start
				startVal, startOk := rangeExpr.Start.(*NumberExpr)
				endVal, endOk := rangeExpr.End.(*NumberExpr)

				if startOk && endOk {
					// Literal range - known at compile time, no runtime check needed
					start := int64(startVal.Value)
					end := int64(endVal.Value)
					maxIterations = end - start
					if maxIterations < 0 {
						maxIterations = 0
					}
					needsRuntimeCheck = false
				} else {
					// Range bounds are not literals, require explicit max
					p.error("loop over non-literal range requires explicit 'max' clause")
				}
			} else if listExpr, ok := iterable.(*ListExpr); ok {
				// List literal - known at compile time, no runtime check needed
				maxIterations = int64(len(listExpr.Elements))
				needsRuntimeCheck = false
			} else if _, ok := iterable.(*IdentExpr); ok {
				// Variable (could be a list or map) - use runtime length check
				maxIterations = math.MaxInt64 // Use max value, will check length at runtime
				needsRuntimeCheck = true
			} else if _, ok := iterable.(*IndexExpr); ok {
				// Indexed expression (e.g., lists[0]) - use runtime length check
				maxIterations = math.MaxInt64
				needsRuntimeCheck = true
			} else {
				// Not a range expression or list literal, require explicit max
				p.error("loop requires 'max' clause (or use range expression like 0..<10 or list literal)")
			}
			// Advance to next token after iterable expression
			p.nextToken()
		}

		// Skip newlines before '{'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Expect '{'
		if p.current.Type != TOKEN_LBRACE {
			p.error("expected '{' to start loop body")
		}

		// Skip newlines after '{'
		for p.peek.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Track loop depth for nested loops
		oldDepth := p.loopDepth
		p.loopDepth = label
		defer func() { p.loopDepth = oldDepth }()

		// Parse loop body
		var body []Statement
		for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
			p.nextToken()
			if p.current.Type == TOKEN_NEWLINE {
				continue
			}
			stmt := p.parseStatement()
			if stmt != nil {
				body = append(body, stmt)
			}
		}

		// Expect and consume '}'
		if p.peek.Type != TOKEN_RBRACE {
			p.error("expected '}' at end of loop body")
		}
		p.nextToken() // consume the '}'

		// Check for optional reducer: | a,b | { a + b }
		var reducer *LambdaExpr
		if p.peek.Type == TOKEN_PIPE {
			// Only allow reducers for parallel loops
			if numThreads == 0 {
				p.error("reducer syntax '| a,b | { expr }' only allowed for parallel loops (@@ or N @)")
			}

			p.nextToken() // advance to '|'
			p.nextToken() // consume '|', advance to first parameter

			// Parse parameter list
			var params []string
			if p.current.Type != TOKEN_IDENT {
				p.error("expected parameter name after '|'")
			}
			params = append(params, p.current.Value)
			p.nextToken()

			// Expect comma
			if p.current.Type != TOKEN_COMMA {
				p.error("reducer requires exactly two parameters (e.g., | a,b | ...)")
			}
			p.nextToken() // skip comma

			// Skip newlines after comma
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Parse second parameter
			if p.current.Type != TOKEN_IDENT {
				p.error("expected second parameter name after comma")
			}
			params = append(params, p.current.Value)
			p.nextToken()

			// Expect second '|'
			if p.current.Type != TOKEN_PIPE {
				p.error("expected '|' after reducer parameters")
			}
			p.nextToken() // skip second '|'

			// Skip newlines before '{'
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Expect '{'
			if p.current.Type != TOKEN_LBRACE {
				p.error("expected '{' to start reducer body")
			}
			p.nextToken() // skip '{'

			// Skip newlines after '{'
			for p.current.Type == TOKEN_NEWLINE {
				p.nextToken()
			}

			// Parse reducer body (single expression)
			reducerBody := p.parseExpression()

			// Expect '}'
			if p.peek.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of reducer body")
			}
			p.nextToken() // advance to '}'

			// Create lambda expression for reducer
			reducer = &LambdaExpr{
				Params:        params,
				VariadicParam: "",
				Body:          reducerBody,
			}
		}

		return &LoopStmt{
			Iterator:      iterator,
			Iterable:      iterable,
			Body:          body,
			MaxIterations: maxIterations,
			NeedsMaxCheck: needsRuntimeCheck,
			NumThreads:    numThreads,
			Reducer:       reducer,
		}
	}

handleJump:
	// If we reach here, must be @N for a jump statement
	p.nextToken() // skip '@'

	// Expect number for jump label
	if p.current.Type != TOKEN_NUMBER {
		p.error("expected number after @ (e.g., @0, @1, @2)")
	}

	labelNum, err := strconv.ParseFloat(p.current.Value, 64)
	if err != nil {
		p.error("invalid jump label number")
	}
	label = int(labelNum)

	p.nextToken() // skip label number

	// It's a jump statement: @N or @N value
	if label < 0 {
		p.error("jump label must be >= 0 (use @0, @1, @2, etc.)")
	}
	// Check for optional return value: @0 value
	var value Expression
	if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		value = p.parseExpression()
	}
	return &JumpStmt{IsBreak: true, Label: label, Value: value}
}

// parseJumpStatement parses ret statements
// ret - return from function
// ret value - return value from function
// ret @N - exit loop N and all inner loops
// ret @N value - exit loop N and return value
func (p *Parser) parseJumpStatement() Statement {
	p.nextToken() // skip 'ret'

	label := 0 // 0 means return from function
	var value Expression

	// Check for optional @ or @N label (for loop exit)
	if p.current.Type == TOKEN_AT {
		p.nextToken() // skip '@'

		// Check if followed by number (ret @N) or not (ret @)
		if p.current.Type == TOKEN_NUMBER {
			// ret @N - exit specific loop N
			labelNum, err := strconv.ParseFloat(p.current.Value, 64)
			if err != nil {
				p.error("invalid loop label number")
			}
			label = int(labelNum)
			if label < 1 {
				p.error("loop label must be >= 1 (use @1, @2, @3, etc.)")
			}
			p.nextToken() // skip number
		} else {
			// ret @ - exit current loop (label -1 means "current loop")
			label = -1
		}
	}

	// Check for optional value
	if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		value = p.parseExpression()
	}

	// ret is always a break/return (IsBreak=true)
	// label=0 means return from function
	// label=-1 means exit current loop
	// label>0 means exit loop N
	return &JumpStmt{IsBreak: true, Label: label, Value: value}
}

// parsePattern parses a single pattern (literal, variable, or wildcard)
func (p *Parser) parsePattern() Pattern {
	switch p.current.Type {
	case TOKEN_NUMBER:
		value := p.current.Value
		p.nextToken()
		// Convert string to float64
		numVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			p.error("invalid number in pattern: " + value)
			return nil
		}
		return &LiteralPattern{Value: &NumberExpr{Value: numVal}}
	case TOKEN_STRING:
		value := p.current.Value
		p.nextToken()
		return &LiteralPattern{Value: &StringExpr{Value: value}}
	case TOKEN_IDENT:
		if p.current.Value == "_" {
			p.nextToken()
			return &WildcardPattern{}
		}
		name := p.current.Value
		p.nextToken()
		return &VarPattern{Name: name}
	default:
		p.error("expected pattern (literal, variable, or _)")
		return nil
	}
}

// tryParsePatternLambda attempts to parse a pattern lambda starting from current position
// Returns nil if this is not a pattern lambda
func (p *Parser) tryParsePatternLambda() *PatternLambdaExpr {
	// Pattern lambda syntax: (pattern) => body, (pattern) => body, ...
	// We're at TOKEN_LPAREN

	// Enable speculative mode to suppress errors
	p.speculative = true
	defer func() {
		p.speculative = false
		// Recover from speculative errors (they indicate "not a pattern lambda")
		if r := recover(); r != nil {
			if _, ok := r.(speculativeError); !ok {
				// Re-panic if it's not a speculative error
				panic(r)
			}
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "DEBUG: Pattern lambda parse failed with speculative error\n")
			}
		}
	}()

	// Parse first clause
	clause := p.parseOnePatternClause()
	if clause == nil {
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: parseOnePatternClause returned nil\n")
		}
		return nil
	}

	// Check if there's a comma for additional clauses
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: After first clause, current token: %v\n", p.current.Type)
	}
	if p.current.Type != TOKEN_COMMA {
		// Not a pattern lambda, just a single clause (which could be regular lambda)
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "DEBUG: No comma after first clause, not a pattern lambda\n")
		}
		return nil
	}

	// It's a pattern lambda! Disable speculative mode now that we know
	p.speculative = false

	// Collect all clauses
	clauses := []*PatternClause{clause}

	for p.current.Type == TOKEN_COMMA {
		p.nextToken() // skip ','
		clause := p.parseOnePatternClause()
		if clause == nil {
			p.error("expected pattern clause after ','")
			break
		}
		clauses = append(clauses, clause)
	}

	return &PatternLambdaExpr{Clauses: clauses}
}

// parseOnePatternClause parses one pattern clause: (pattern, ...) => body
func (p *Parser) parseOnePatternClause() *PatternClause {
	if p.current.Type != TOKEN_LPAREN {
		return nil
	}
	p.nextToken() // skip '('

	var patterns []Pattern
	if p.current.Type == TOKEN_RPAREN {
		// Empty pattern list
	} else {
		patterns = append(patterns, p.parsePattern())
		for p.current.Type == TOKEN_COMMA {
			p.nextToken() // skip ','
			patterns = append(patterns, p.parsePattern())
		}
	}

	if p.current.Type != TOKEN_RPAREN {
		p.error("expected ')' after patterns")
		return nil
	}
	p.nextToken() // skip ')'

	if p.current.Type != TOKEN_ARROW {
		// Not a pattern clause
		return nil
	}
	p.nextToken() // skip '->'

	body := p.parseLambdaBody()

	// parseLambdaBody leaves current on the last token of the body
	// We need to advance to get to the token after the body (likely '|' or EOF)
	// For blocks, this advances past '}'; for expressions, past the expression
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseOnePatternClause: before advancing, current=%v ('%s') peek=%v ('%s')\n", p.current.Type, p.current.Value, p.peek.Type, p.peek.Value)
	}
	p.nextToken()
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG parseOnePatternClause: after advancing, current=%v ('%s')\n", p.current.Type, p.current.Value)
	}

	return &PatternClause{Patterns: patterns, Body: body}
}

// Confidence that this function is working: 100%
func (p *Parser) parseExpression() Expression {
	globalParseCallCount++
	if globalParseCallCount > maxParseRecursion {
		// Print stack trace
		debug.PrintStack()
		p.error(fmt.Sprintf("infinite recursion in parseExpression: count=%d, token type=%v value='%v' line=%d", globalParseCallCount, p.current.Type, p.current.Value, p.current.Line))
	}
	return p.parsePipe()
}

// parsePipe handles | and || operators (lowest precedence)
// Grammar: pipe_expr = reduce_expr { ("|" | "||") reduce_expr }
func (p *Parser) parsePipe() Expression {
	left := p.parseReduce()

	for p.peek.Type == TOKEN_PIPE || p.peek.Type == TOKEN_PIPEPIPE {
		op := p.peek.Type
		p.nextToken() // skip current
		p.nextToken() // skip '|' or '||'
		right := p.parseReduce()

		if op == TOKEN_PIPE {
			left = &PipeExpr{Left: left, Right: right}
		} else {
			// TOKEN_PIPEPIPE - parallel map
			left = &ParallelExpr{List: left, Operation: right}
		}
	}

	return left
}

// parseReduce handles reduce expressions (passthrough for now)
// Grammar: reduce_expr = receive_expr
func (p *Parser) parseReduce() Expression {
	return p.parseReceive()
}

// parseReceive handles the <= prefix operator for receiving from channels
// Grammar: receive_expr = "<=" pipe_expr | or_bang_expr
func (p *Parser) parseReceive() Expression {
	// Check for <= prefix (receive operator)
	// Only treat <= as receive if it appears at the beginning of an expression
	// (i.e., current token is not something that could be part of a binary expression)
	// This prevents "x <= y" from being parsed as "x" followed by "<= y"

	isExpressionStart := p.current.Type == TOKEN_NEWLINE ||
		p.current.Type == TOKEN_SEMICOLON ||
		p.current.Type == TOKEN_LPAREN ||
		p.current.Type == TOKEN_LBRACE ||
		p.current.Type == TOKEN_COMMA ||
		p.current.Type == TOKEN_EQUALS ||
		p.current.Type == TOKEN_COLON_EQUALS ||
		p.current.Type == TOKEN_LEFT_ARROW ||
		p.current.Type == TOKEN_PIPE ||
		p.current.Type == TOKEN_PIPEPIPE ||
		p.current.Type == TOKEN_FAT_ARROW ||
		p.current.Type == TOKEN_DEFAULT_ARROW

	if isExpressionStart && p.peek.Type == TOKEN_LE {
		p.nextToken()           // move to current (TOKEN_LE)
		p.nextToken()           // skip '<=', move to next
		source := p.parsePipe() // Note: recursive to allow nested receives
		return &ReceiveExpr{Source: source}
	}

	return p.parseOrBang()
}

// parseOrBang handles the or! operator
// Grammar: or_bang_expr = send_expr { "or!" send_expr }
func (p *Parser) parseOrBang() Expression {
	left := p.parseSend()

	// or! is right-associative
	if p.peek.Type == TOKEN_OR_BANG {
		p.nextToken() // move to left
		p.nextToken() // skip 'or!'

		var right Expression
		if p.peek.Type == TOKEN_LBRACE {
			// or! followed by a block: parse the block as a lambda
			right = p.parsePrimary()
		} else {
			// or! followed by an expression
			right = p.parseOrBang() // right-associative recursion
		}
		return &BinaryExpr{Left: left, Operator: "or!", Right: right}
	}

	return left
}

// parseSend handles the <- infix operator for sending to channels
// Grammar: send_expr = or_expr { "<-" or_expr }
func (p *Parser) parseSend() Expression {
	left := p.parseCompose()

	// Check for send operator: expr <- expr
	// Left side should be an address literal (e.g., &8080)
	for p.peek.Type == TOKEN_LEFT_ARROW {
		p.nextToken() // move to left
		p.nextToken() // skip '<-'
		right := p.parseCompose()
		left = &SendExpr{Target: left, Message: right}
	}

	return left
}

// parseCompose handles the <> (function composition) operator
// Right-associative: f <> g <> h means f <> (g <> h)
func (p *Parser) parseCompose() Expression {
	left := p.parseLogicalOr()

	if p.peek.Type == TOKEN_LTGT {
		p.nextToken()             // move to left
		p.nextToken()             // skip '<>'
		right := p.parseCompose() // right-associative recursion
		return &ComposeExpr{Left: left, Right: right}
	}

	return left
}

// parseLogicalOr handles the 'or' and 'xor' keywords
// Grammar: or_expr = and_expr { "or" and_expr }
//
//	xor_expr = and_expr { "xor" and_expr }
func (p *Parser) parseLogicalOr() Expression {
	left := p.parseLogicalAnd()

	for p.peek.Type == TOKEN_OR || p.peek.Type == TOKEN_XOR {
		p.nextToken() // skip current
		op := p.current.Value
		p.nextToken() // skip operator
		right := p.parseLogicalAnd()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

func (p *Parser) parseLogicalAnd() Expression {
	left := p.parseComparison()

	for p.peek.Type == TOKEN_AND {
		p.nextToken() // skip current
		op := p.current.Value
		p.nextToken() // skip 'and'
		right := p.parseComparison()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

func (p *Parser) parseComparison() Expression {
	left := p.parseRange()

	// Check for 'in' operator (membership testing)
	if p.peek.Type == TOKEN_IN {
		p.nextToken() // move to left expr
		p.nextToken() // skip 'in'
		right := p.parseRange()
		return &InExpr{Value: left, Container: right}
	}

	for p.peek.Type == TOKEN_LT || p.peek.Type == TOKEN_GT ||
		p.peek.Type == TOKEN_LE || p.peek.Type == TOKEN_GE ||
		p.peek.Type == TOKEN_EQ || p.peek.Type == TOKEN_NE {
		p.nextToken()
		op := p.current.Value
		p.nextToken()
		right := p.parseRange()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

// parseRange handles range expressions (0..<10 or 0..=10)
func (p *Parser) parseRange() Expression {
	left := p.parseAdditive()

	// Check for range operators
	if p.peek.Type == TOKEN_DOTDOTLT || p.peek.Type == TOKEN_DOTDOT {
		p.nextToken() // move to left expr
		inclusive := p.current.Type == TOKEN_DOTDOT
		p.nextToken() // skip range operator
		right := p.parseAdditive()
		return &RangeExpr{Start: left, End: right, Inclusive: inclusive}
	}

	return left
}

// parseLambdaBody parses the body of a lambda expression according to GRAMMAR.md:
//
// Lambda body can be:
// 1. A block: { ... } (map, match, or statement block)
// 2. An expression followed by optional match block: expr { ... }
//
// For blocks, we use block disambiguation to determine type:
// - Contains ':' before arrows → map literal
// - Contains '->' or '~>' → match block (guard match if no expr before {)
// - Otherwise → statement block
func (p *Parser) parseLambdaBody() Expression {
	// Increment function depth and push scope when entering lambda body
	p.functionDepth++
	p.pushScope()
	defer func() {
		p.functionDepth--
		p.popScope()
	}()

	// Declare lambda parameters in the new scope
	for _, param := range p.lambdaParams {
		p.declareVariable(param)
	}

	// Check if lambda body is a block { ... }
	if p.current.Type == TOKEN_LBRACE {
		// Disambiguate block type
		blockType := p.disambiguateBlock()

		p.nextToken() // skip '{'
		p.skipNewlines()

		switch blockType {
		case BlockTypeMap:
			// Parse as map literal
			return p.parseMapLiteralBody()

		case BlockTypeMatch:
			// Parse as guard match block (no expression before {)
			// Create a dummy true condition for guard matches
			trueExpr := &NumberExpr{Value: 1.0}
			return p.parseMatchBlock(trueExpr)

		case BlockTypeStatement:
			// Parse statements until we hit '}'
			var statements []Statement
			for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
				stmt := p.parseStatement()
				if stmt != nil {
					statements = append(statements, stmt)
				}

				// Need to advance to the next statement
				// Skip newlines and semicolons between statements
				if p.peek.Type == TOKEN_NEWLINE || p.peek.Type == TOKEN_SEMICOLON {
					p.nextToken() // move to separator
					p.skipNewlines()
				} else if p.peek.Type == TOKEN_RBRACE || p.peek.Type == TOKEN_EOF {
					// At end of block
					p.nextToken() // move to '}'
					break
				} else {
					// No separator found - might be at end
					p.nextToken()
					p.skipNewlines()
				}
			}

			if p.current.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of lambda block")
			}
			// Don't skip the '}' - let the caller handle it

			// Return a BlockExpr containing the statements
			return &BlockExpr{Statements: statements}
		}
	}

	// Otherwise, parse the body expression
	expr := p.parseExpression()

	// Check for value match: expr { pattern -> result }
	if p.peek.Type == TOKEN_LBRACE {
		p.nextToken() // move to '{'
		p.nextToken() // skip '{'
		p.skipNewlines()
		return p.parseMatchBlock(expr)
	}

	return expr
}

func (p *Parser) parseAdditive() Expression {
	left := p.parseBitwise()

	for p.peek.Type == TOKEN_PLUS || p.peek.Type == TOKEN_MINUS {
		p.nextToken()
		op := p.current.Value
		p.nextToken()
		right := p.parseBitwise()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

func (p *Parser) parseBitwise() Expression {
	left := p.parseMultiplicative()

	for p.peek.Type == TOKEN_PIPE_B || p.peek.Type == TOKEN_AMP_B ||
		p.peek.Type == TOKEN_CARET_B || p.peek.Type == TOKEN_LTLT_B ||
		p.peek.Type == TOKEN_GTGT_B || p.peek.Type == TOKEN_LTLTLT_B ||
		p.peek.Type == TOKEN_GTGTGT_B || p.peek.Type == TOKEN_QUESTION_B {
		p.nextToken()
		op := p.current.Value
		p.nextToken()
		right := p.parseMultiplicative()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

func (p *Parser) parseMultiplicative() Expression {
	left := p.parsePower()

	for p.peek.Type == TOKEN_STAR || p.peek.Type == TOKEN_SLASH || p.peek.Type == TOKEN_MOD || p.peek.Type == TOKEN_FMA {
		p.nextToken()
		op := p.current.Value
		p.nextToken()
		right := p.parsePower()
		left = &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

// parsePower handles the ** (power/exponentiation) operator
// Power is right-associative: 2 ** 3 ** 2 = 2 ** (3 ** 2) = 512
func (p *Parser) parsePower() Expression {
	left := p.parseUnary()

	if p.peek.Type == TOKEN_POWER || p.peek.Type == TOKEN_CARET {
		p.nextToken() // move to ** or ^
		op := p.current.Value
		// Normalize ^ to ** for backend processing
		if op == "^" {
			op = "**"
		}
		p.nextToken() // move past ** or ^
		// Right-associative: recursively parse the right side
		right := p.parsePower()
		return &BinaryExpr{Left: left, Operator: op, Right: right}
	}

	return left
}

func (p *Parser) parseUnary() Expression {
	// Handle unary operators (not, ++, --, ~b, ^, &)
	if p.current.Type == TOKEN_NOT {
		p.nextToken() // skip 'not'
		operand := p.parseUnary()
		return &UnaryExpr{Operator: "not", Operand: operand}
	}

	// Handle prefix increment/decrement: ++x, --x
	if p.current.Type == TOKEN_INCREMENT || p.current.Type == TOKEN_DECREMENT {
		op := p.current.Value
		p.nextToken() // skip ++ or --
		operand := p.parseUnary()
		return &UnaryExpr{Operator: op, Operand: operand}
	}

	// Handle bitwise NOT: ~b or !
	if p.current.Type == TOKEN_TILDE_B || p.current.Type == TOKEN_BANG {
		p.nextToken() // skip '~b' or '!'
		operand := p.parseUnary()
		// Normalize both to "~b" internally
		return &UnaryExpr{Operator: "~b", Operand: operand}
	}

	// Handle prefix length operator: #xs
	if p.current.Type == TOKEN_HASH {
		p.nextToken() // skip '#'
		operand := p.parseUnary()
		return &UnaryExpr{Operator: "#", Operand: operand}
	}

	// Unary minus handled in parsePrimary for simplicity
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expression {
	expr := p.parsePrimary()

	// Handle postfix operations like indexing and function calls
	for {
		if p.peek.Type == TOKEN_LBRACKET {
			p.nextToken() // skip current expr
			p.nextToken() // skip '['

			// Check for empty indexing (syntax error)
			if p.current.Type == TOKEN_RBRACKET {
				p.error("empty indexing [] is not allowed")
			}

			// Check for slice syntax: [start:end], [:end], [start:], [:]
			// Parse the first expression (could be start or index)
			var firstExpr Expression
			var isSlice bool
			if p.current.Type == TOKEN_COLON {
				// Case: [:end] or [::step]
				firstExpr = nil
				isSlice = true
				p.nextToken() // skip ':'
			} else {
				firstExpr = p.parseExpression()
				// Check if this is a slice (has colon)
				isSlice = p.peek.Type == TOKEN_COLON
				if isSlice {
					p.nextToken() // move to colon
					p.nextToken() // skip ':'
				}
			}

			if isSlice {
				var endExpr Expression
				if p.current.Type == TOKEN_RBRACKET {
					// Case: [start:] or [:]
					endExpr = nil
				} else if p.current.Type == TOKEN_COLON {
					// Case: [start::step] or [::step]
					endExpr = nil
					// Don't skip the colon yet - let step handling do it
				} else {
					endExpr = p.parseExpression()
				}

				// Check for step parameter (second colon)
				var stepExpr Expression
				if p.peek.Type == TOKEN_COLON || p.current.Type == TOKEN_COLON {
					if p.current.Type != TOKEN_COLON {
						p.nextToken() // move to second colon
					}
					p.nextToken() // skip ':'

					if p.current.Type == TOKEN_RBRACKET {
						// Case: [start:end:] - step is nil
						stepExpr = nil
					} else {
						stepExpr = p.parseExpression()
						p.nextToken() // move to ']'
					}
				} else if endExpr != nil {
					// We parsed an end expression, need to move to ']'
					p.nextToken()
				}

				expr = &SliceExpr{List: expr, Start: firstExpr, End: endExpr, Step: stepExpr}
			} else {
				// Regular indexing
				p.nextToken() // move to ']'
				expr = &IndexExpr{List: expr, Index: firstExpr}
			}
		} else if p.peek.Type == TOKEN_LPAREN {
			// Handle direct lambda calls: ((x) -> x * 2)(5)
			// or chained calls: f(1)(2)
			p.nextToken() // skip current expr
			p.nextToken() // skip '('
			p.skipNewlines()
			args := []Expression{}

			if p.current.Type != TOKEN_RPAREN {
				args = append(args, p.parseExpression())
				for p.peek.Type == TOKEN_COMMA {
					p.nextToken() // skip current
					p.nextToken() // skip ','
					p.skipNewlines()
					args = append(args, p.parseExpression())
				}
				// current should be on last arg, peek should be ')'
				p.skipNewlines()
				p.nextToken() // move to ')'
			}
			// current is now on ')', whether we had args or not

			// TODO: Blocks-as-arguments disabled (conflicts with match expressions)

			// Wrap the expression in a CallExpr
			// If expr is a LambdaExpr, it will be compiled and called
			// If expr is an IdentExpr, it will be looked up and called
			if ident, ok := expr.(*IdentExpr); ok {
				// Special handling for vector constructors
				if ident.Name == "vec2" {
					if len(args) != 2 {
						p.error("vec2 requires exactly 2 arguments")
					}
					expr = &VectorExpr{Components: args, Size: 2}
				} else if ident.Name == "vec4" {
					if len(args) != 4 {
						p.error("vec4 requires exactly 4 arguments")
					}
					expr = &VectorExpr{Components: args, Size: 4}
				} else {
					expr = &CallExpr{Function: ident.Name, Args: args}
				}
			} else {
				// For lambda expressions or other callable expressions,
				// create a special call expression that compiles the lambda inline
				expr = &DirectCallExpr{Callee: expr, Args: args}
			}
		} else if p.peek.Type == TOKEN_INCREMENT || p.peek.Type == TOKEN_DECREMENT {
			// Handle postfix increment/decrement: x++, x--
			p.nextToken() // skip current expr
			op := p.current.Value
			expr = &PostfixExpr{Operator: op, Operand: expr}
		} else if p.peek.Type == TOKEN_BANG {
			// Handle move operator: x! (transfers ownership)
			p.nextToken() // skip to !
			expr = &MoveExpr{Expr: expr}
		} else if p.peek.Type == TOKEN_HASH {
			// Handle postfix length operator: xs#
			p.nextToken() // skip to #
			expr = &UnaryExpr{Operator: "#", Operand: expr}
		} else if p.peek.Type == TOKEN_AS {
			// Handle type cast: expr as type
			p.nextToken() // skip current expr
			p.nextToken() // skip 'as'

			// Parse the cast type
			var castType string
			if p.current.Type == TOKEN_IDENT {
				// All C67 and C types are valid after 'as'
				validTypes := map[string]bool{
					// C integer types
					"int8": true, "int16": true, "int32": true, "int64": true,
					"uint8": true, "uint16": true, "uint32": true, "uint64": true,
					"char": true, "short": true, "int": true, "long": true,
					"uchar": true, "ushort": true, "uint": true, "ulong": true,
					"size_t": true, "ssize_t": true, "ptrdiff_t": true,
					// C floating point types
					"float": true, "float32": true, "float64": true, "double": true,
					// C string/pointer types
					"cstr": true, "cstring": true, "ptr": true, "pointer": true,
					// C67 types
					"number": true, "num": true, "string": true, "str": true,
					"list": true, "map": true, "address": true, "addr": true,
					"bool": true, "boolean": true,
					// Type aliases
					"void": true,
				}
				if validTypes[p.current.Value] {
					castType = p.current.Value
				} else {
					p.error("expected valid type after 'as' (e.g., string, int, float, ptr)")
				}
			} else {
				p.error("expected type after 'as'")
			}

			expr = &CastExpr{Expr: expr, Type: castType}
		} else if p.peek.Type == TOKEN_DOT {
			// Handle dot notation: obj.field, namespace.func(), namespace.CONSTANT, Type.size, Type.field.offset
			p.nextToken() // skip current expr
			p.nextToken() // skip '.'

			if p.current.Type != TOKEN_IDENT {
				p.error("expected field name after '.'")
			}

			fieldName := p.current.Value

			// Check if this is a namespaced function call or constant: namespace.func() or namespace.CONSTANT
			// This requires expr to be an IdentExpr
			if ident, ok := expr.(*IdentExpr); ok {
				// Check if this is metadata access: Type.size or Type.field.offset
				if cstruct, exists := p.cstructs[ident.Name]; exists {
					if fieldName == "size" {
						// Type.size - return struct size as constant
						expr = &NumberExpr{Value: float64(cstruct.Size)}
					} else {
						// Check if this is Type.field.offset
						fieldFound := false
						for _, field := range cstruct.Fields {
							if field.Name == fieldName {
								fieldFound = true
								// Check if next token is .offset
								if p.peek.Type == TOKEN_DOT {
									p.nextToken() // skip field name
									p.nextToken() // skip '.'
									if p.current.Type == TOKEN_IDENT && p.current.Value == "offset" {
										// Type.field.offset - return field offset as constant
										expr = &NumberExpr{Value: float64(field.Offset)}
									} else {
										p.error("expected 'offset' after '" + cstruct.Name + "." + fieldName + ".'")
									}
								} else {
									p.error("expected '.offset' after '" + cstruct.Name + "." + fieldName + "'")
								}
								break
							}
						}
						if !fieldFound {
							p.error("cstruct '" + cstruct.Name + "' has no field '" + fieldName + "'")
						}
					}
				} else if p.peek.Type == TOKEN_LPAREN {
					// Check if this is a C import namespace (e.g., sdl, c) or a method call (e.g., xs.append)
					if p.cImports[ident.Name] {
						// Namespaced function call - combine identifiers
						namespacedName := ident.Name + "." + fieldName
						p.nextToken() // skip second identifier
						p.nextToken() // skip '('
						args := []Expression{}

						if p.current.Type != TOKEN_RPAREN {
							args = append(args, p.parseExpression())
							for p.peek.Type == TOKEN_COMMA {
								p.nextToken() // skip current
								p.nextToken() // skip ','
								args = append(args, p.parseExpression())
							}
							p.nextToken() // move to ')'
						}
						// Check if this is a C FFI call (c.malloc, c.free, etc.)
						isCFFI := ident.Name == "c"
						if isCFFI {
							// For C FFI calls, use just the function name without the "c." prefix
							expr = &CallExpr{Function: fieldName, Args: args, IsCFFI: true}
						} else {
							// Regular C library namespace call (e.g., sdl.SDL_Init)
							expr = &CallExpr{Function: namespacedName, Args: args}
						}
					} else {
						// Ambiguous: could be method call (xs.append) or C67 namespace (lib.hello)
						// Parse as namespaced call and let compiler resolve
						namespacedName := ident.Name + "." + fieldName
						p.nextToken() // skip second identifier
						p.nextToken() // skip '('
						args := []Expression{}

						if p.current.Type != TOKEN_RPAREN {
							args = append(args, p.parseExpression())
							for p.peek.Type == TOKEN_COMMA {
								p.nextToken() // skip current
								p.nextToken() // skip ','
								args = append(args, p.parseExpression())
							}
							p.nextToken() // move to ')'
						}
						// Store as namespace.function, compiler will handle both cases
						expr = &CallExpr{Function: namespacedName, Args: args}
					}
				} else {
					// Check for special property access on Result types
					if fieldName == "error" {
						// .error property - extract error code from Result type
						// This will be handled specially in codegen
						expr = &CallExpr{
							Function: "_error_code_extract",
							Args:     []Expression{expr},
						}
					} else {
						// Could be a C constant (sdl.SDL_INIT_VIDEO) or namespace access
						// We'll create a special NamespacedIdentExpr to distinguish at compile time
						expr = &NamespacedIdentExpr{Namespace: ident.Name, Name: fieldName}
					}
				}
			} else {
				// Check if this is a method call: obj.method(args)
				if p.peek.Type == TOKEN_LPAREN {
					// Method call syntax sugar: obj.method(args) -> method(obj, args)
					p.nextToken()              // skip field name
					p.nextToken()              // skip '('
					args := []Expression{expr} // receiver becomes first argument

					if p.current.Type != TOKEN_RPAREN {
						args = append(args, p.parseExpression())
						for p.peek.Type == TOKEN_COMMA {
							p.nextToken() // skip current
							p.nextToken() // skip ','
							args = append(args, p.parseExpression())
						}
						p.nextToken() // move to ')'
					}
					// Desugar to function call with receiver as first arg
					expr = &CallExpr{Function: fieldName, Args: args}
				} else if fieldName == "error" {
					// .error property - extract error code from Result type
					// This will be handled specially in codegen
					expr = &CallExpr{
						Function: "_error_code_extract",
						Args:     []Expression{expr},
					}
				} else {
					// Regular field access - hash the field name and create index expression
					hashValue := hashStringKey(fieldName)
					expr = &IndexExpr{
						List:  expr,
						Index: &NumberExpr{Value: float64(hashValue)},
					}
				}
			}
		} else {
			break
		}
	}

	return expr
}

func (p *Parser) parsePrimary() Expression {
	switch p.current.Type {
	case TOKEN_ARROW:
		// Explicit no-argument lambda: -> expr or -> { ... }
		p.nextToken()               // skip '->'
		p.lambdaParams = []string{} // No parameters
		body := p.parseLambdaBody()
		p.lambdaParams = nil
		return &LambdaExpr{Params: []string{}, VariadicParam: "", Body: body}

	case TOKEN_MINUS:
		// Unary minus: -expr
		p.nextToken() // skip '-'
		expr := p.parsePrimary()
		return &UnaryExpr{Operator: "-", Operand: expr}

	case TOKEN_HASH:
		// Length operator: #list
		p.nextToken() // skip '#'
		expr := p.parsePrimary()
		return &LengthExpr{Operand: expr}

	case TOKEN_NUMBER:
		val := p.parseNumberLiteral(p.current.Value)
		return &NumberExpr{Value: val}

	case TOKEN_INF:
		return &NumberExpr{Value: math.Inf(1)}

	case TOKEN_RANDOM:
		return &RandomExpr{}

	case TOKEN_STRING:
		return &StringExpr{Value: p.current.Value}

	case TOKEN_ADDRESS_LITERAL:
		// ENet address literal like &8080 or &localhost:8080
		return &AddressLiteralExpr{Value: p.current.Value}

	case TOKEN_DOLLAR:
		// Address value operator: $expr
		p.nextToken() // skip '$'
		expr := p.parsePrimary()
		return &UnaryExpr{Operator: "$", Operand: expr}

	case TOKEN_FSTRING:
		return p.parseFString()

	case TOKEN_IDENT:
		name := p.current.Value

		// Check if this is a constant reference (substitute with value)
		if expr, isConst := p.constants[name]; isConst {
			// Return a copy of the stored expression to avoid mutation issues
			switch e := expr.(type) {
			case *NumberExpr:
				return &NumberExpr{Value: e.Value}
			case *StringExpr:
				return &StringExpr{Value: e.Value}
			case *ListExpr:
				// Deep copy the list
				elements := make([]Expression, len(e.Elements))
				for i, elem := range e.Elements {
					switch el := elem.(type) {
					case *NumberExpr:
						elements[i] = &NumberExpr{Value: el.Value}
					case *StringExpr:
						elements[i] = &StringExpr{Value: el.Value}
					default:
						elements[i] = elem
					}
				}
				return &ListExpr{Elements: elements}
			}
			return expr
		}

		// TODO: Struct literal syntax conflicts with lambda match
		// Need to redesign syntax or add explicit keyword (e.g., new StructName { ... })
		// Temporarily disabled to fix lambda match expressions
		// if p.peek.Type == TOKEN_LBRACE {
		// 	return p.parseStructLiteral(name)
		// }

		// Check for lambda: x -> expr or x, y -> expr
		if p.peek.Type == TOKEN_ARROW {
			// Try to parse as non-parenthesized lambda
			if lambda := p.tryParseNonParenLambda(); lambda != nil {
				return lambda
			}
		}

		// If we see => it's not a lambda error (it's a match arrow), continue parsing
		// The => will be handled correctly in match clause parsing

		// Dot notation is now handled entirely in parsePostfix
		// This includes both field access (obj.field) and namespaced calls (namespace.func())

		// Check for function call
		if p.peek.Type == TOKEN_LPAREN {
			p.nextToken() // skip identifier
			p.nextToken() // skip '('
			args := []Expression{}

			if p.current.Type != TOKEN_RPAREN {
				args = append(args, p.parseExpression())
				for p.peek.Type == TOKEN_COMMA {
					p.nextToken() // skip current
					p.nextToken() // skip ','
					args = append(args, p.parseExpression())
				}
				// current should be on last arg, peek should be ')'
				p.nextToken() // move to ')'
			}
			// current is now on ')', whether we had args or not

			// TODO: Blocks-as-arguments feature disabled for now
			// It conflicts with match expressions like: func() { -> val }
			// Need to redesign to only apply when block doesn't start with -> or ~>

			// Special handling for vector constructors
			if name == "vec2" {
				if len(args) != 2 {
					p.error("vec2 requires exactly 2 arguments")
				}
				return &VectorExpr{Components: args, Size: 2}
			} else if name == "vec4" {
				if len(args) != 4 {
					p.error("vec4 requires exactly 4 arguments")
				}
				return &VectorExpr{Components: args, Size: 4}
			}

			// Check for optional 'max' keyword after function call
			// This will be validated during compilation to ensure it's present for recursive calls
			// Skip this if we're parsing a condition loop expression (to avoid consuming loop's max clause)
			var maxRecursion int64
			needsCheck := false
			if p.peek.Type == TOKEN_MAX && !p.inConditionLoop {
				p.nextToken() // advance to 'max'
				p.nextToken() // skip 'max', now on the value

				needsCheck = true
				if p.current.Type == TOKEN_INF {
					maxRecursion = math.MaxInt64
					// Don't advance - leave p.current on 'inf' for caller
				} else if p.current.Type == TOKEN_NUMBER {
					maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
					if err != nil || maxInt < 1 {
						p.error("max recursion depth must be a positive integer or 'inf'")
					}
					maxRecursion = maxInt
					// Don't advance - leave p.current on the number for caller
				} else {
					p.error("expected number or 'inf' after 'max' keyword in function call")
				}
			}

			return &CallExpr{
				Function:            name,
				Args:                args,
				MaxRecursionDepth:   maxRecursion,
				NeedsRecursionCheck: needsCheck,
			}
		}
		return &IdentExpr{Name: name}

	// TOKEN_ME and TOKEN_CME removed - recursive calls now use function name with mandatory max

	case TOKEN_AT_FIRST:
		// @first is true on the first iteration of a loop
		return &LoopStateExpr{Type: "first"}

	case TOKEN_AT_LAST:
		// @last is true on the last iteration of a loop
		return &LoopStateExpr{Type: "last"}

	case TOKEN_AT_COUNTER:
		// @counter is the loop iteration counter
		return &LoopStateExpr{Type: "counter"}

	case TOKEN_AT_I:
		// @i is the current loop, @i1 is outermost loop, @i2 is second loop, etc.
		value := p.current.Value
		level := 0
		if len(value) > 2 { // @iN where N is a number
			// Parse the number after @i
			numStr := value[2:] // Skip "@i"
			if num, err := strconv.Atoi(numStr); err == nil {
				level = num
			}
		}
		return &LoopStateExpr{Type: "i", LoopLevel: level}

	case TOKEN_LPAREN:
		// Could be:
		// 1. Pattern lambda: (0) => 1, (n) => n * fact(n-1)
		// 2. Regular lambda: (x) => x * 2
		// 3. Parenthesized expression: (x + y)

		// Try pattern lambda first with backtracking
		state := p.saveState()
		patternLambda := p.tryParsePatternLambda()
		if patternLambda != nil {
			return patternLambda
		}
		p.restoreState(state)

		p.nextToken() // skip '('

		// Check for empty parameter list: () -> or () {
		if p.current.Type == TOKEN_RPAREN {
			if p.peek.Type == TOKEN_ARROW || p.peek.Type == TOKEN_LBRACE {
				p.nextToken()               // skip ')'
				if p.current.Type == TOKEN_ARROW {
					p.nextToken()           // skip '->'
				}
				p.lambdaParams = []string{} // No parameters
				body := p.parseLambdaBody()
				p.lambdaParams = nil
				return &LambdaExpr{Params: []string{}, VariadicParam: "", Body: body}
			}
			// Empty parens without arrow or block is an error, but skip for now
			p.nextToken()
			return nil
		}

		// Try to parse as parameter list (identifiers separated by commas)
		// or as an expression. Use backtracking to handle type annotations.
		if p.current.Type == TOKEN_IDENT {
			// Save state in case this is not a lambda
			lambdaState := p.saveState()

			// Try to parse as lambda parameter list
			params := []string{p.current.Value}
			variadicParam := ""
			p.nextToken() // skip first ident

			// Check for variadic marker on first parameter
			if p.current.Type == TOKEN_ELLIPSIS {
				// First parameter is variadic (e.g., (args...) -> ...)
				variadicParam = params[0]
				params = []string{} // No regular params, only variadic
				p.nextToken()       // skip '...'
			}

			// Skip optional type annotation: as Type (if not variadic)
			if variadicParam == "" && p.current.Type == TOKEN_AS {
				p.nextToken() // skip 'as'
				if p.current.Type != TOKEN_IDENT {
					// Not a valid lambda, restore and parse as expression
					p.restoreState(lambdaState)
					expr := p.parseExpression()
					p.nextToken() // skip ')'
					return expr
				}
				p.nextToken() // skip type name
			}

			// Check if we have more parameters (only if first param wasn't variadic)
			if variadicParam == "" {
				for p.current.Type == TOKEN_COMMA {
					p.nextToken() // skip ','
					if p.current.Type != TOKEN_IDENT {
						// Not a valid lambda, restore and parse as expression
						p.restoreState(lambdaState)
						expr := p.parseExpression()
						p.nextToken() // skip ')'
						return expr
					}
					paramName := p.current.Value
					p.nextToken() // skip param

					// Check for variadic marker on this parameter
					if p.current.Type == TOKEN_ELLIPSIS {
						// This parameter is variadic (must be last)
						variadicParam = paramName
						p.nextToken() // skip '...'
						// No more parameters allowed after variadic
						break
					}

					params = append(params, paramName)

					// Skip optional type annotation
					if p.current.Type == TOKEN_AS {
						p.nextToken() // skip 'as'
						if p.current.Type != TOKEN_IDENT {
							// Not a valid lambda, restore and parse as expression
							p.restoreState(lambdaState)
							expr := p.parseExpression()
							p.nextToken() // skip ')'
							return expr
						}
						p.nextToken() // skip type name
					}
				}
			}

			// current should be ')'
			if p.current.Type != TOKEN_RPAREN {
				// Not a lambda, restore and parse as expression
				p.restoreState(lambdaState)
				expr := p.parseExpression()
				p.nextToken() // skip ')'
				return expr
			}

			// peek should be '->'
			if p.peek.Type == TOKEN_ARROW {
				// It's a lambda!
				p.nextToken()           // skip ')'
				p.nextToken()           // skip '->'
				p.lambdaParams = params // Store params for parseLambdaBody
				body := p.parseLambdaBody()
				p.lambdaParams = nil
				return &LambdaExpr{Params: params, VariadicParam: variadicParam, Body: body}
			}

			// Not a lambda after all, restore and parse as expression
			p.restoreState(lambdaState)
			expr := p.parseExpression()
			p.nextToken() // skip ')'
			return expr
		}

		// Not a lambda, parse as parenthesized expression
		expr := p.parseExpression()
		p.nextToken() // skip ')'
		return expr

	case TOKEN_LBRACKET:
		p.nextToken() // skip '['
		p.skipNewlines()
		elements := []Expression{}

		if p.current.Type != TOKEN_RBRACKET {
			elements = append(elements, p.parseExpression())
			for p.peek.Type == TOKEN_COMMA {
				p.nextToken() // skip current
				p.nextToken() // skip ','
				p.skipNewlines()
				elements = append(elements, p.parseExpression())
			}
			// current should be on last element
			// peek should be ']'
			p.nextToken() // move to ']'
		}
		// For empty list, current is already on ']' after first nextToken()
		return &ListExpr{Elements: elements}

	case TOKEN_LBRACE:
		// Disambiguate block type: map, match, or statement block
		blockType := p.disambiguateBlock()

		p.nextToken() // skip '{'
		p.skipNewlines()

		switch blockType {
		case BlockTypeMap:
			// Parse as map literal
			return p.parseMapLiteralBody()

		case BlockTypeMatch:
			// Parse as guard match block (no expression before {)
			// Create a dummy true condition for guard matches
			trueExpr := &NumberExpr{Value: 1.0}
			return p.parseMatchBlock(trueExpr)

		case BlockTypeStatement:
			// Parse as statement block
			// Statement blocks in expression position will be wrapped in lambdas, so increment depth and push scope
			p.functionDepth++
			p.pushScope()
			defer func() {
				p.functionDepth--
				p.popScope()
			}()

			var statements []Statement
			for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
				stmt := p.parseStatement()
				if stmt != nil {
					statements = append(statements, stmt)
				}

				// Need to advance to the next statement
				// Skip newlines and semicolons between statements
				if p.peek.Type == TOKEN_NEWLINE || p.peek.Type == TOKEN_SEMICOLON {
					p.nextToken() // move to separator
					p.skipNewlines()
				} else if p.peek.Type == TOKEN_RBRACE || p.peek.Type == TOKEN_EOF {
					// At end of block
					p.nextToken() // move to '}'
					break
				} else {
					// No separator found - might be at end
					p.nextToken()
					p.skipNewlines()
				}
			}

			if p.current.Type != TOKEN_RBRACE {
				p.error("expected '}' at end of block")
			}
			// Don't skip the '}' - let the caller handle it

			// Return a BlockExpr containing the statements
			return &BlockExpr{Statements: statements}
		}

	case TOKEN_AT_AT:
		// Parallel loop expression: @@ i in ... { ... } | a,b | { ... }
		return p.parseLoopExpr()

	case TOKEN_AT:
		// Could be loop expression (@ i in...) or jump expression (@N)
		// Look ahead to decide
		if p.peek.Type == TOKEN_NUMBER {
			// Jump expression: @N [value]
			// Returns JumpExpr for continuing loops (IsBreak=false)
			p.nextToken() // skip '@'
			if p.current.Type != TOKEN_NUMBER {
				p.error("expected number after @")
			}
			labelNum, _ := strconv.ParseFloat(p.current.Value, 64)
			label := int(labelNum)
			p.nextToken() // skip number
			var value Expression
			if p.current.Type != TOKEN_NEWLINE && p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
				value = p.parseExpression()
				p.nextToken()
			}
			return &JumpExpr{Label: label, Value: value, IsBreak: false}
		}
		// Must be loop expression: @ ident in...
		return p.parseLoopExpr()

	case TOKEN_UNSAFE:
		// unsafe { x86_64 } { arm64 } { riscv64 }
		return p.parseUnsafeExpr()

	case TOKEN_ARENA:
		// arena { ... }
		return p.parseArenaExpr()

	case TOKEN_DOT:
		// Dot notation for "this":
		// - `.field` means `this.field`
		// - `. ` (dot followed by space/newline) means `this`
		p.nextToken() // skip '.'

		// Check if followed by identifier
		if p.current.Type == TOKEN_IDENT {
			fieldName := p.current.Value
			// Return field access on "this"
			return &BinaryExpr{
				Operator: ".",
				Left:     &IdentExpr{Name: "this"},
				Right:    &IdentExpr{Name: fieldName},
			}
		}

		// Otherwise, just return "this"
		// Move back one token since we consumed the dot but there's no field
		p.current = Token{Type: TOKEN_DOT, Value: ".", Line: p.current.Line, Column: p.current.Column}
		return &IdentExpr{Name: "this"}
	}

	// Check if this is a structural/delimiter token that should just end the expression
	// These are valid tokens that signal the end of an expression, not syntax errors
	if p.current.Type == TOKEN_RBRACE || p.current.Type == TOKEN_RPAREN ||
		p.current.Type == TOKEN_RBRACKET || p.current.Type == TOKEN_COMMA ||
		p.current.Type == TOKEN_SEMICOLON || p.current.Type == TOKEN_NEWLINE ||
		p.current.Type == TOKEN_EOF {
		return nil // Valid delimiter, expression ends here
	}

	// Unrecognized token in expression position - this is a syntax error
	p.error(fmt.Sprintf("unexpected '%s' in expression", p.current.Value))
	return nil
}

func (p *Parser) parseArenaExpr() Expression {
	p.nextToken() // skip 'arena'

	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' after 'arena'")
	}
	p.nextToken() // skip '{'
	p.skipNewlines()

	var body []Statement
	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			body = append(body, stmt)
		}
		p.nextToken()
		p.skipNewlines()
	}

	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of arena block")
	}

	return &ArenaExpr{Body: body}
}

// isLoopExpr checks if current position looks like a loop expression
// Pattern: @ ident in
func (p *Parser) isLoopExpr() bool {
	// Loop expressions start with @
	return p.current.Type == TOKEN_AT
}

// parseLoopExpr parses a loop expression: @ i in iterable { body } or @@ i in iterable { body } | a,b | { a+b }
func (p *Parser) parseLoopExpr() Expression {
	// Parse parallel loop prefix: @@ or N @ or just @
	numThreads := 0 // 0 = sequential, -1 = all cores, N = specific count
	label := p.loopDepth + 1

	// Handle @@ token (parallel loop with all cores)
	if p.current.Type == TOKEN_AT_AT {
		numThreads = -1
		p.nextToken() // skip '@@'

		// Skip newlines after '@@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}
	} else if p.current.Type == TOKEN_NUMBER {
		// Handle N @ syntax (parallel loop with N threads)
		threadCount, err := strconv.Atoi(p.current.Value)
		if err != nil || threadCount < 1 {
			p.error("thread count must be a positive integer")
		}
		numThreads = threadCount
		p.nextToken() // skip number

		// Expect @ token after the number
		if p.current.Type != TOKEN_AT {
			p.error("expected @ after thread count")
		}
		p.nextToken() // skip '@'

		// Skip newlines after '@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}
	} else if p.current.Type == TOKEN_AT {
		// Regular loop: @
		p.nextToken() // skip '@'

		// Skip newlines after '@'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}
	} else {
		p.error("expected @ or @@ to start loop expression")
	}

	// Expect identifier for loop variable
	if p.current.Type != TOKEN_IDENT {
		p.error("expected identifier after loop prefix")
	}
	iterator := p.current.Value
	p.nextToken() // skip iterator

	// Expect 'in' keyword
	if p.current.Type != TOKEN_IN {
		p.error("expected 'in' keyword in loop expression")
	}
	p.nextToken() // skip 'in'

	iterable := p.parseExpression()
	p.nextToken() // move past iterable

	// Determine max iterations and whether runtime checking is needed
	var maxIterations int64
	needsRuntimeCheck := false

	// Check if max keyword is present
	if p.current.Type == TOKEN_MAX {
		p.nextToken() // skip 'max'

		// Explicit max always requires runtime checking
		needsRuntimeCheck = true

		// Parse max iterations: either a number or 'inf'
		if p.current.Type == TOKEN_INF {
			maxIterations = math.MaxInt64 // Use MaxInt64 for infinite iterations
			p.nextToken()                 // skip 'inf'
		} else if p.current.Type == TOKEN_NUMBER {
			// Parse the number
			maxInt, err := strconv.ParseInt(p.current.Value, 10, 64)
			if err != nil || maxInt < 1 {
				p.error("max iterations must be a positive integer or 'inf'")
			}
			maxIterations = maxInt
			p.nextToken() // skip number
		} else {
			p.error("expected number or 'inf' after 'max' keyword in loop expression")
		}
	} else {
		// No explicit max - check if we can determine iteration count at compile time
		if rangeExpr, ok := iterable.(*RangeExpr); ok {
			// Try to calculate max from range: end - start
			startVal, startOk := rangeExpr.Start.(*NumberExpr)
			endVal, endOk := rangeExpr.End.(*NumberExpr)

			if startOk && endOk {
				// Literal range - known at compile time, no runtime check needed
				start := int64(startVal.Value)
				end := int64(endVal.Value)
				maxIterations = end - start
				if maxIterations < 0 {
					maxIterations = 0
				}
				needsRuntimeCheck = false
			} else {
				// Range bounds are not literals, require explicit max
				p.error("loop expression over non-literal range requires explicit 'max' clause")
			}
		} else if listExpr, ok := iterable.(*ListExpr); ok {
			// List literal - known at compile time, no runtime check needed
			maxIterations = int64(len(listExpr.Elements))
			needsRuntimeCheck = false
		} else {
			// Not a range expression or list literal, require explicit max
			p.error("loop expression requires 'max' clause (or use range expression like 0..<10 or list literal)")
		}
	}

	// Expect '{'
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' to start loop body")
	}
	p.nextToken() // skip '{'

	// Parse loop body
	oldDepth := p.loopDepth
	p.loopDepth = label
	defer func() { p.loopDepth = oldDepth }()

	var body []Statement
	for p.peek.Type != TOKEN_RBRACE && p.peek.Type != TOKEN_EOF {
		p.nextToken()
		if p.current.Type == TOKEN_NEWLINE {
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			body = append(body, stmt)
		}
	}

	// Expect and consume '}'
	if p.peek.Type != TOKEN_RBRACE {
		p.error("expected '}' at end of loop body")
	}
	p.nextToken() // consume the '}'

	// Check for optional reducer: | a,b | { a + b }
	var reducer *LambdaExpr
	if p.peek.Type == TOKEN_PIPE {
		// Only allow reducers for parallel loops
		if numThreads == 0 {
			p.error("reducer syntax '| a,b | { expr }' only allowed for parallel loops (@@ or N @)")
		}

		p.nextToken() // advance to '|'
		p.nextToken() // consume '|', advance to first parameter

		// Parse parameter list
		var params []string
		if p.current.Type != TOKEN_IDENT {
			p.error("expected parameter name after '|'")
		}
		params = append(params, p.current.Value)
		p.nextToken()

		// Expect comma
		if p.current.Type != TOKEN_COMMA {
			p.error("reducer requires exactly two parameters (e.g., | a,b | ...)")
		}
		p.nextToken() // skip comma

		// Skip newlines after comma
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Parse second parameter
		if p.current.Type != TOKEN_IDENT {
			p.error("expected second parameter name after comma")
		}
		params = append(params, p.current.Value)
		p.nextToken()

		// Expect second '|'
		if p.current.Type != TOKEN_PIPE {
			p.error("expected '|' after reducer parameters")
		}
		p.nextToken() // skip second '|'

		// Skip newlines before '{'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Expect '{'
		if p.current.Type != TOKEN_LBRACE {
			p.error("expected '{' to start reducer body")
		}
		p.nextToken() // skip '{'

		// Skip newlines after '{'
		for p.current.Type == TOKEN_NEWLINE {
			p.nextToken()
		}

		// Parse reducer body (single expression)
		reducerBody := p.parseExpression()

		// Expect '}'
		if p.peek.Type != TOKEN_RBRACE {
			p.error("expected '}' at end of reducer body")
		}
		p.nextToken() // advance to '}'

		// Create lambda expression for reducer
		reducer = &LambdaExpr{
			Params:        params,
			VariadicParam: "",
			Body:          reducerBody,
		}
	}

	return &LoopExpr{
		Iterator:      iterator,
		Iterable:      iterable,
		Body:          body,
		MaxIterations: maxIterations,
		NeedsMaxCheck: needsRuntimeCheck,
		NumThreads:    numThreads,
		Reducer:       reducer,
	}
}

// parseUnsafeExpr parses: unsafe [type] { x86_64 block } { arm64 block } { riscv64 block } [as type]
// Example: unsafe { rax <- 42 } { x0 <- 42 } { a0 <- 42 } as int64
// Legacy: unsafe int64 { rax <- 42 } { x0 <- 42 } { a0 <- 42 }
// Default: unsafe { rax <- 42 } { x0 <- 42 } { a0 <- 42 } returns uint64
func (p *Parser) parseUnsafeExpr() Expression {
	p.nextToken() // skip 'unsafe'

	// Parse return type before blocks (legacy, optional, defaults to uint64)
	returnType := "uint64" // default
	if p.current.Type == TOKEN_IDENT {
		// Check if this is a type name or if it's a block
		// Type names we support: int8, int16, int32, int64, uint8, uint16, uint32, uint64, float64, ptr, pointer, cstr
		possibleType := p.current.Value
		if possibleType == "int8" || possibleType == "int16" || possibleType == "int32" || possibleType == "int64" ||
			possibleType == "uint8" || possibleType == "uint16" || possibleType == "uint32" || possibleType == "uint64" ||
			possibleType == "float64" || possibleType == "float32" ||
			possibleType == "ptr" || possibleType == "pointer" || possibleType == "cstr" {
			returnType = possibleType
			p.nextToken() // skip type
		}
	}

	// Parse x86_64 block
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' for x86_64 block in unsafe expression")
	}
	x86_64Stmts := p.parseUnsafeBlock()

	// Parse arm64 block
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' for arm64 block in unsafe expression")
	}
	arm64Stmts := p.parseUnsafeBlock()

	// Parse riscv64 block
	if p.current.Type != TOKEN_LBRACE {
		p.error("expected '{' for riscv64 block in unsafe expression")
	}
	riscv64Stmts := p.parseUnsafeBlock()

	// Check for 'as type' suffix (new syntax)
	if p.current.Type == TOKEN_IDENT && p.current.Value == "as" {
		p.nextToken() // skip 'as'
		if p.current.Type == TOKEN_IDENT {
			possibleType := p.current.Value
			if possibleType == "int8" || possibleType == "int16" || possibleType == "int32" || possibleType == "int64" ||
				possibleType == "uint8" || possibleType == "uint16" || possibleType == "uint32" || possibleType == "uint64" ||
				possibleType == "float64" || possibleType == "float32" ||
				possibleType == "ptr" || possibleType == "pointer" || possibleType == "cstr" {
				returnType = possibleType
				p.nextToken() // skip type
			} else {
				p.error("expected type name after 'as' in unsafe expression")
			}
		} else {
			p.error("expected type name after 'as' in unsafe expression")
		}
	}

	// Create return statements with the specified type
	x86_64Ret := &UnsafeReturnStmt{Register: "rax", AsType: returnType}
	arm64Ret := &UnsafeReturnStmt{Register: "x0", AsType: returnType}
	riscv64Ret := &UnsafeReturnStmt{Register: "a0", AsType: returnType}

	return &UnsafeExpr{
		X86_64Block:   x86_64Stmts,
		ARM64Block:    arm64Stmts,
		RISCV64Block:  riscv64Stmts,
		X86_64Return:  x86_64Ret,
		ARM64Return:   arm64Ret,
		RISCV64Return: riscv64Ret,
	}
}

// parseUnsafeBlock parses a single architecture block with extended syntax
// Returns: statements
func (p *Parser) parseUnsafeBlock() []Statement {
	p.nextToken() // skip '{'
	p.skipNewlines()

	statements := []Statement{}

	for p.current.Type != TOKEN_RBRACE && p.current.Type != TOKEN_EOF {
		// Check for syscall
		if p.current.Type == TOKEN_SYSCALL {
			statements = append(statements, &SyscallStmt{})
			p.nextToken() // skip 'syscall'
			p.skipNewlines()
			continue
		}

		// Check for memory store: [rax] <- value or [rax] <- value as uint8
		if p.current.Type == TOKEN_LBRACKET {
			// Parse: [rax + offset] <- value as type
			p.nextToken() // skip '['

			if p.current.Type != TOKEN_IDENT {
				p.error("expected register name in memory address")
			}
			storeAddr := p.current.Value
			p.nextToken() // skip register name

			// Check for offset: [rax + 16]
			var storeOffset int64
			if p.current.Type == TOKEN_PLUS {
				p.nextToken() // skip '+'
				if p.current.Type != TOKEN_NUMBER {
					p.error("expected number after '+' in memory address")
				}
				storeOffset = int64(p.parseNumberLiteral(p.current.Value))
				p.nextToken() // skip number
			}

			if p.current.Type != TOKEN_RBRACKET {
				p.error("expected ']' after memory address")
			}
			p.nextToken() // skip ']'

			if p.current.Type != TOKEN_LEFT_ARROW {
				p.error("expected '<-' after memory address")
			}
			p.nextToken() // skip '<-'

			// Parse value
			var value interface{}
			if p.current.Type == TOKEN_NUMBER {
				val := p.parseNumberLiteral(p.current.Value)
				value = &NumberExpr{Value: val}
				p.nextToken()
			} else if p.current.Type == TOKEN_IDENT {
				value = p.current.Value
				p.nextToken()
			} else {
				p.error("expected number or register after '<-' in memory store")
			}

			// Check for size cast: [rax] <- value as uint8
			storeSize := "uint64" // default to 64-bit
			if p.current.Type == TOKEN_AS {
				p.nextToken() // skip 'as'
				if p.current.Type != TOKEN_IDENT {
					p.error("expected type name after 'as'")
				}
				storeSize = p.current.Value
				p.nextToken() // skip type name
			}

			statements = append(statements, &MemoryStore{
				Size:    storeSize,
				Address: storeAddr,
				Offset:  storeOffset,
				Value:   value,
			})
			p.skipNewlines()
			continue
		}

		// Regular register assignment
		if p.current.Type != TOKEN_IDENT {
			p.error("expected register name, memory address, or syscall in unsafe block")
		}

		regName := p.current.Value
		p.nextToken() // skip register name

		if p.current.Type != TOKEN_LEFT_ARROW {
			p.error(fmt.Sprintf("expected '<-' after register %s in unsafe block", regName))
		}
		p.nextToken() // skip '<-'

		// Parse the right-hand side
		value := p.parseUnsafeValue()

		statements = append(statements, &RegisterAssignStmt{
			Register: regName,
			Value:    value,
		})

		p.skipNewlines()
	}

	if p.current.Type != TOKEN_RBRACE {
		p.error("expected '}' to close unsafe block")
	}
	p.nextToken() // skip '}'

	return statements
}

// parseUnsafeValue parses the RHS of a register assignment in unsafe blocks
func (p *Parser) parseUnsafeValue() interface{} {
	// Check for memory load: [rax] or [rax + offset]
	// Followed optionally by: as uint8, as int16, etc.
	if p.current.Type == TOKEN_LBRACKET {
		// [rax] or [rax + offset]
		p.nextToken() // skip '['
		if p.current.Type != TOKEN_IDENT {
			p.error("expected register name in memory load")
		}
		addrReg := p.current.Value
		p.nextToken() // skip register

		var offset int64
		if p.current.Type == TOKEN_PLUS {
			p.nextToken() // skip '+'
			if p.current.Type != TOKEN_NUMBER {
				p.error("expected number after '+' in memory address")
			}
			offset = int64(p.parseNumberLiteral(p.current.Value))
			p.nextToken() // skip number
		}

		if p.current.Type != TOKEN_RBRACKET {
			p.error("expected ']' after memory address")
		}
		p.nextToken() // skip ']'

		// Check for size cast: [rbx] as uint8
		size := "uint64" // default to 64-bit
		if p.current.Type == TOKEN_AS {
			p.nextToken() // skip 'as'
			if p.current.Type != TOKEN_IDENT {
				p.error("expected type name after 'as'")
			}
			size = p.current.Value
			p.nextToken() // skip type name
		}

		return &MemoryLoad{Size: size, Address: addrReg, Offset: offset}
	}

	// Check for unary operation: ~b rax (bitwise NOT)
	if p.current.Type == TOKEN_TILDE_B {
		p.nextToken() // skip '~b'
		if p.current.Type != TOKEN_IDENT {
			p.error("expected register name after '~b'")
		}
		reg := p.current.Value
		p.nextToken() // skip register
		return &RegisterOp{Left: "", Operator: "~b", Right: reg}
	}

	// Parse left operand (register or immediate)
	var left string
	var leftIsImmediate bool
	var leftValue *NumberExpr

	if p.current.Type == TOKEN_NUMBER {
		val := p.parseNumberLiteral(p.current.Value)
		leftValue = &NumberExpr{Value: val}
		leftIsImmediate = true
		p.nextToken() // skip number

		// Check for cast: 42 as uint8
		if p.current.Type == TOKEN_AS {
			p.nextToken() // skip 'as'
			if p.current.Type == TOKEN_IDENT {
				castType := p.current.Value
				p.nextToken() // skip type
				// Wrap in cast expression
				return &CastExpr{Expr: leftValue, Type: castType}
			}
			p.error("expected type after 'as'")
		}
	} else if p.current.Type == TOKEN_IDENT {
		left = p.current.Value
		p.nextToken() // skip register name

		// Check for cast: rax as pointer
		if p.current.Type == TOKEN_AS {
			p.nextToken() // skip 'as'
			if p.current.Type == TOKEN_IDENT {
				castType := p.current.Value
				p.nextToken() // skip type
				// Return cast of variable reference
				return &CastExpr{Expr: &IdentExpr{Name: left}, Type: castType}
			}
			p.error("expected type after 'as'")
		}
	} else {
		p.error("expected number, register, memory load, or unary operator")
	}

	// Check for binary operator
	var op string
	switch p.current.Type {
	case TOKEN_PLUS:
		op = "+"
	case TOKEN_MINUS:
		op = "-"
	case TOKEN_STAR:
		op = "*"
	case TOKEN_SLASH:
		op = "/"
	case TOKEN_MOD:
		op = "%"
	case TOKEN_AMP:
		op = "&"
	case TOKEN_PIPE:
		op = "|"
	case TOKEN_CARET_B:
		op = "^b"
	case TOKEN_LT:
		// Check if it's << (shift left)
		if p.peek.Type == TOKEN_LT {
			p.nextToken() // skip first '<'
			op = "<<"
		}
	case TOKEN_GT:
		// Check if it's >> (shift right)
		if p.peek.Type == TOKEN_GT {
			p.nextToken() // skip first '>'
			op = ">>"
		}
	}

	if op != "" {
		// Binary operation
		p.nextToken() // skip operator

		// Parse right operand
		var right interface{}
		if p.current.Type == TOKEN_NUMBER {
			val := p.parseNumberLiteral(p.current.Value)
			right = &NumberExpr{Value: val}
			p.nextToken()
		} else if p.current.Type == TOKEN_IDENT {
			right = p.current.Value
			p.nextToken()
		} else {
			p.error("expected number or register after operator")
		}

		if leftIsImmediate {
			p.error("left operand of binary operation must be a register")
		}

		return &RegisterOp{Left: left, Operator: op, Right: right}
	}

	// No operator - just a simple value
	if leftIsImmediate {
		return leftValue
	}
	return left
}

// parseTypeAnnotation parses a type annotation (after :)
// Returns nil if no valid type annotation found
func (p *Parser) parseTypeAnnotation() *C67Type {
	// Native C67 types
	switch p.current.Value {
	case "num":
		return &C67Type{Kind: TypeNumber}
	case "str":
		return &C67Type{Kind: TypeString}
	case "list":
		return &C67Type{Kind: TypeList}
	case "map":
		return &C67Type{Kind: TypeMap}
	// Foreign C types
	case "cstring":
		return &C67Type{Kind: TypeCString, CType: "char*"}
	case "cptr":
		return &C67Type{Kind: TypeCPointer, CType: "void*"}
	case "cint":
		return &C67Type{Kind: TypeCInt, CType: "int"}
	case "clong":
		return &C67Type{Kind: TypeCLong, CType: "long"}
	case "cfloat":
		return &C67Type{Kind: TypeCFloat, CType: "float"}
	case "cdouble":
		return &C67Type{Kind: TypeCDouble, CType: "double"}
	case "cbool":
		return &C67Type{Kind: TypeCBool, CType: "bool"}
	case "cvoid":
		return &C67Type{Kind: TypeCVoid}
	default:
		return nil
	}
}
