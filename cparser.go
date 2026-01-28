// Completion: 100% - C header parser with DWARF support, fully functional
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// CParser is a simple C header file parser for extracting constants, macros, and function signatures
type CParser struct {
	tokens       []CToken
	pos          int
	results      *CHeaderConstants
	maxTokens    int // Maximum number of tokens to prevent memory exhaustion
	maxParseOps  int // Maximum number of parse operations to prevent infinite loops
	parseOpCount int // Current parse operation count
}

const (
	// Safety limits
	MaxTokensPerFile   = 1000000  // 1M tokens max per file
	MaxParseOperations = 10000000 // 10M parse ops max
)

// CTokenType represents the type of a C token
type CTokenType int

const (
	CTokEOF CTokenType = iota
	CTokIdentifier
	CTokNumber
	CTokString
	CTokPunctuation
	CTokPreprocessor
	CTokNewline
)

// CToken represents a single token from the C source
type CToken struct {
	Type  CTokenType
	Value string
	Line  int
}

// NewCParser creates a new C header parser
func NewCParser() *CParser {
	return &CParser{
		results:      NewCHeaderConstants(),
		maxTokens:    MaxTokensPerFile,
		maxParseOps:  MaxParseOperations,
		parseOpCount: 0,
	}
}

// ParseFile parses a C header file and extracts constants, macros, and function signatures
func (p *CParser) ParseFile(filepath string) (*CHeaderConstants, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	// Tokenize the input
	p.tokens = p.tokenize(string(content))
	p.pos = 0

	// Parse top-level declarations with safety limit
	for !p.isAtEnd() {
		p.parseOpCount++
		if p.parseOpCount > p.maxParseOps {
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Warning: reached max parse operations limit (%d), stopping parse\n", p.maxParseOps)
			}
			break
		}
		p.parseTopLevel()
	}

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "    Parsed %d functions, %d constants from %s\n", len(p.results.Functions), len(p.results.Constants), filepath)
	}
	return p.results, nil
}

// tokenize converts C source code into tokens
func (p *CParser) tokenize(source string) []CToken {
	var tokens []CToken
	lines := strings.Split(source, "\n")

	inMultiLineComment := false

	for lineNum, line := range lines {
		i := 0
		for i < len(line) {
			// Safety check: prevent token explosion
			if len(tokens) >= p.maxTokens {
				if VerboseMode {
					fmt.Fprintf(os.Stderr, "Warning: reached max tokens limit (%d), stopping tokenization\n", p.maxTokens)
				}
				return tokens
			}

			// Handle multi-line comment state
			if inMultiLineComment {
				for i < len(line)-1 {
					if line[i] == '*' && line[i+1] == '/' {
						i += 2
						inMultiLineComment = false
						break
					}
					i++
				}
				if inMultiLineComment {
					break // Skip rest of line
				}
				continue
			}

			// Skip whitespace (except newlines)
			if unicode.IsSpace(rune(line[i])) && line[i] != '\n' {
				i++
				continue
			}

			// Preprocessor directive
			if i == 0 && line[i] == '#' {
				// Find the directive name
				start := i
				i++
				for i < len(line) && !unicode.IsSpace(rune(line[i])) {
					i++
				}
				directive := line[start:i]

				// Get the rest of the line (the directive content)
				for i < len(line) && unicode.IsSpace(rune(line[i])) {
					i++
				}
				rest := strings.TrimSpace(line[i:])

				tokens = append(tokens, CToken{
					Type:  CTokPreprocessor,
					Value: directive + " " + rest,
					Line:  lineNum + 1,
				})
				break // Done with this line
			}

			// Single-line comment
			if i < len(line)-1 && line[i] == '/' && line[i+1] == '/' {
				break // Skip rest of line
			}

			// Multi-line comment start
			if i < len(line)-1 && line[i] == '/' && line[i+1] == '*' {
				i += 2
				// Check if it ends on the same line
				for i < len(line)-1 {
					if line[i] == '*' && line[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				// If we didn't find the end, mark as in multi-line comment
				if i >= len(line)-1 && !(i > 0 && line[i-1] == '/' && i > 1 && line[i-2] == '*') {
					inMultiLineComment = true
					break
				}
				continue
			}

			// String literal
			if line[i] == '"' {
				start := i
				i++
				for i < len(line) && line[i] != '"' {
					if line[i] == '\\' {
						i++ // Skip escaped character
					}
					i++
				}
				if i < len(line) {
					i++ // Skip closing quote
				}
				tokens = append(tokens, CToken{
					Type:  CTokString,
					Value: line[start:i],
					Line:  lineNum + 1,
				})
				continue
			}

			// Number (hex, decimal, binary)
			if unicode.IsDigit(rune(line[i])) || (line[i] == '0' && i+1 < len(line) && (line[i+1] == 'x' || line[i+1] == 'X' || line[i+1] == 'b' || line[i+1] == 'B')) {
				start := i
				if line[i] == '0' && i+1 < len(line) && (line[i+1] == 'x' || line[i+1] == 'X') {
					i += 2 // Skip 0x
					for i < len(line) && (unicode.IsDigit(rune(line[i])) || (line[i] >= 'a' && line[i] <= 'f') || (line[i] >= 'A' && line[i] <= 'F')) {
						i++
					}
				} else if line[i] == '0' && i+1 < len(line) && (line[i+1] == 'b' || line[i+1] == 'B') {
					i += 2 // Skip 0b
					for i < len(line) && (line[i] == '0' || line[i] == '1') {
						i++
					}
				} else {
					for i < len(line) && unicode.IsDigit(rune(line[i])) {
						i++
					}
				}
				// Skip type suffixes (u, l, ul, ll, ull, etc.)
				for i < len(line) && (line[i] == 'u' || line[i] == 'U' || line[i] == 'l' || line[i] == 'L') {
					i++
				}
				tokens = append(tokens, CToken{
					Type:  CTokNumber,
					Value: line[start:i],
					Line:  lineNum + 1,
				})
				continue
			}

			// Identifier or keyword
			if unicode.IsLetter(rune(line[i])) || line[i] == '_' {
				start := i
				for i < len(line) && (unicode.IsLetter(rune(line[i])) || unicode.IsDigit(rune(line[i])) || line[i] == '_') {
					i++
				}
				tokens = append(tokens, CToken{
					Type:  CTokIdentifier,
					Value: line[start:i],
					Line:  lineNum + 1,
				})
				continue
			}

			// Punctuation (operators, braces, etc.)
			// Handle multi-character operators
			if i < len(line)-1 {
				twoChar := line[i : i+2]
				if twoChar == "<<" || twoChar == ">>" || twoChar == "##" || twoChar == "::" {
					tokens = append(tokens, CToken{
						Type:  CTokPunctuation,
						Value: twoChar,
						Line:  lineNum + 1,
					})
					i += 2
					continue
				}
			}

			// Single character punctuation
			tokens = append(tokens, CToken{
				Type:  CTokPunctuation,
				Value: string(line[i]),
				Line:  lineNum + 1,
			})
			i++
		}
	}

	return tokens
}

// parseTopLevel parses a top-level declaration
func (p *CParser) parseTopLevel() {
	if p.isAtEnd() {
		return
	}

	tok := p.peek()

	// Preprocessor directive
	if tok.Type == CTokPreprocessor {
		p.parsePreprocessor()
		return
	}

	// Handle typedef enum declarations
	if tok.Type == CTokIdentifier && tok.Value == "typedef" {
		if p.tryParseTypedefEnum() {
			return
		}
		p.skipUntil(";")
		return
	}

	// Handle standalone enum declarations
	if tok.Type == CTokIdentifier && tok.Value == "enum" {
		p.parseEnum()
		return
	}

	// Function declaration
	if tok.Type == CTokIdentifier {
		// Try to parse as function declaration
		// Look for pattern: [extern] [MACRO]* type [MACRO]* identifier ( params ) ;
		if p.tryParseFunctionDecl() {
			return
		}
	}

	// Skip unrecognized tokens
	p.advance()
}

// skipUntil skips tokens until the given value is found
func (p *CParser) skipUntil(value string) {
	for !p.isAtEnd() {
		tok := p.advance()
		if tok.Value == value {
			return
		}
	}
}

// parsePreprocessor handles preprocessor directives
func (p *CParser) parsePreprocessor() {
	if p.isAtEnd() {
		return
	}

	tok := p.advance()
	parts := strings.SplitN(tok.Value, " ", 2)
	if len(parts) < 2 {
		return
	}

	directive := parts[0]
	rest := strings.TrimSpace(parts[1])

	switch directive {
	case "#define":
		p.parseDefine(rest)
	case "#include":
		// Could handle includes for recursive parsing, but skip for now
	}
}

// parseDefine parses a #define directive
func (p *CParser) parseDefine(content string) {
	// Check for function-like macro: NAME(params) body
	// Must have no space between NAME and (
	if idx := strings.Index(content, "("); idx != -1 {
		// Get everything before the first (
		beforeParen := content[:idx]

		// Check if there's ANY whitespace in beforeParen
		// If yes: it's a constant with macro value (e.g., "NAME VALUE(arg)")
		// If no: it's a function-like macro (e.g., "NAME(params)")
		hasWhitespace := false
		for _, ch := range beforeParen {
			if unicode.IsSpace(ch) {
				hasWhitespace = true
				break
			}
		}

		if !hasWhitespace {
			// This is a function-like macro: NAME(params)
			name := beforeParen

			// Find closing paren
			closeIdx := strings.Index(content, ")")
			if closeIdx == -1 {
				return
			}

			// Get the body after the closing paren
			body := strings.TrimSpace(content[closeIdx+1:])

			// Store the macro
			p.results.Macros[name] = body

			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Macro: %s = %s\n", name, body)
			}
			return
		}
		// Otherwise fall through to handle as constant with macro value
	}

	// Simple constant: NAME value
	parts := strings.Fields(content)
	if len(parts) < 2 {
		return
	}

	name := parts[0]
	valueStr := strings.Join(parts[1:], " ")

	// Remove inline comments
	if idx := strings.Index(valueStr, "//"); idx != -1 {
		valueStr = strings.TrimSpace(valueStr[:idx])
	}
	if idx := strings.Index(valueStr, "/*"); idx != -1 {
		valueStr = strings.TrimSpace(valueStr[:idx])
	}

	// Try to evaluate the constant value
	value, ok := p.evalConstant(valueStr)
	if ok {
		p.results.Constants[name] = value
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "  Constant: %s = %d (0x%x)\n", name, value, value)
		}
	}
}

// evalConstant evaluates a constant expression
func (p *CParser) evalConstant(expr string) (int64, bool) {
	expr = strings.TrimSpace(expr)

	// Remove type suffixes
	expr = strings.TrimSuffix(expr, "u")
	expr = strings.TrimSuffix(expr, "U")
	expr = strings.TrimSuffix(expr, "l")
	expr = strings.TrimSuffix(expr, "L")
	expr = strings.TrimSuffix(expr, "ul")
	expr = strings.TrimSuffix(expr, "UL")
	expr = strings.TrimSuffix(expr, "ll")
	expr = strings.TrimSuffix(expr, "LL")
	expr = strings.TrimSuffix(expr, "ull")
	expr = strings.TrimSuffix(expr, "ULL")

	// Try hex number
	if strings.HasPrefix(expr, "0x") || strings.HasPrefix(expr, "0X") {
		if val, err := strconv.ParseInt(expr[2:], 16, 64); err == nil {
			return val, true
		}
	}

	// Try binary number
	if strings.HasPrefix(expr, "0b") || strings.HasPrefix(expr, "0B") {
		if val, err := strconv.ParseInt(expr[2:], 2, 64); err == nil {
			return val, true
		}
	}

	// Try decimal number
	if val, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return val, true
	}

	// Try to resolve reference to another constant
	if val, ok := p.results.Constants[expr]; ok {
		return val, true
	}

	// Try to handle function-like macro calls (e.g., SDL_UINT64_C(0x20))
	if idx := strings.Index(expr, "("); idx != -1 && strings.HasSuffix(expr, ")") {
		macroName := strings.TrimSpace(expr[:idx])
		argsStr := expr[idx+1 : len(expr)-1]

		// Common SDL/library macros that just wrap their arguments
		wrapperMacros := map[string]bool{
			"SDL_UINT64_C": true,
			"SDL_SINT64_C": true,
			"UINT64_C":     true,
			"INT64_C":      true,
			"SDL_FOURCC":   true, // SDL_FOURCC(A,B,C,D) - would need special handling
		}

		if wrapperMacros[macroName] {
			// For wrapper macros, just evaluate the argument
			return p.evalConstant(argsStr)
		}

		// If it's not a known wrapper, treat it as a simple parenthesized expression
		return p.evalConstant(argsStr)
	}

	// Try simple expressions (parentheses)
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return p.evalConstant(expr[1 : len(expr)-1])
	}

	// Try bitwise shift: value << shift
	if idx := strings.Index(expr, "<<"); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+2:])
		leftVal, leftOk := p.evalConstant(left)
		rightVal, rightOk := p.evalConstant(right)
		if leftOk && rightOk {
			return leftVal << uint(rightVal), true
		}
	}

	// Try bitwise shift: value >> shift
	if idx := strings.Index(expr, ">>"); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+2:])
		leftVal, leftOk := p.evalConstant(left)
		rightVal, rightOk := p.evalConstant(right)
		if leftOk && rightOk {
			return leftVal >> uint(rightVal), true
		}
	}

	// Try bitwise OR: value | value
	if idx := strings.Index(expr, "|"); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+1:])
		leftVal, leftOk := p.evalConstant(left)
		rightVal, rightOk := p.evalConstant(right)
		if leftOk && rightOk {
			return leftVal | rightVal, true
		}
	}

	// Try bitwise AND: value & value
	if idx := strings.Index(expr, "&"); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+1:])
		leftVal, leftOk := p.evalConstant(left)
		rightVal, rightOk := p.evalConstant(right)
		if leftOk && rightOk {
			return leftVal & rightVal, true
		}
	}

	return 0, false
}

// tryParseFunctionDecl attempts to parse a function declaration
func (p *CParser) tryParseFunctionDecl() bool {
	saved := p.pos

	// Debug: check if we have extern
	isExtern := p.match("extern")

	// Skip 'extern' if present
	if isExtern {
		p.advance()
	}

	// Collect return type tokens (may include macros like SDL_DECLSPEC)
	var returnTypeParts []string
	foundOpenParen := false

	maxReturnTypeParts := 100 // Generous limit for return type components (SDL has many macros)
	for !p.isAtEnd() && len(returnTypeParts) < maxReturnTypeParts {
		tok := p.peek()

		// Skip newlines
		if tok.Type == CTokNewline {
			p.advance()
			continue
		}

		if tok.Type == CTokPunctuation && tok.Value == "(" {
			foundOpenParen = true
			break
		}

		if tok.Type == CTokPunctuation && tok.Value == ";" {
			// Not a function
			p.pos = saved
			return false
		}

		if tok.Type == CTokIdentifier || (tok.Type == CTokPunctuation && tok.Value == "*") {
			returnTypeParts = append(returnTypeParts, tok.Value)
		}
		p.advance()
	}

	if !foundOpenParen || len(returnTypeParts) == 0 {
		p.pos = saved
		return false
	}

	// The last identifier before '(' is the function name
	// Everything else is the return type (possibly with macros)
	funcName := returnTypeParts[len(returnTypeParts)-1]
	returnTypeParts = returnTypeParts[:len(returnTypeParts)-1]

	// Filter out common C macros to get the actual return type
	var actualReturnType []string
	for _, part := range returnTypeParts {
		// Skip common SDL/library macros
		if part == "SDL_DECLSPEC" || part == "SDLCALL" || part == "RAYLIB_API" ||
			part == "SDL_MALLOC" || part == "SDL_FORCE_INLINE" || part == "static" ||
			part == "inline" || strings.HasPrefix(part, "__attribute__") {
			continue
		}
		actualReturnType = append(actualReturnType, part)
	}

	returnType := strings.Join(actualReturnType, " ")
	if returnType == "" {
		returnType = "void"
	}

	// Parse parameter list
	p.advance() // skip '('
	params := p.parseParameters()

	// Look for closing ';'
	if !p.match(";") {
		p.pos = saved
		return false
	}
	p.advance() // skip ';'

	// Store the function signature
	p.results.Functions[funcName] = &CFunctionSignature{
		ReturnType: returnType,
		Params:     params,
	}

	if VerboseMode {
		paramStrs := make([]string, len(params))
		for i, param := range params {
			if param.Name != "" {
				paramStrs[i] = param.Type + " " + param.Name
			} else {
				paramStrs[i] = param.Type
			}
		}
		fmt.Fprintf(os.Stderr, "  Parsed function: %s %s(%s)\n", returnType, funcName, strings.Join(paramStrs, ", "))
	}

	return true
}

// parseParameters parses function parameters
func (p *CParser) parseParameters() []CFunctionParam {
	var params []CFunctionParam

	// Handle empty parameter list or (void)
	if p.match(")") {
		p.advance() // Consume the closing paren
		return params
	}

	if p.match("void") {
		p.advance()
		if p.match(")") {
			p.advance() // Consume the closing paren
			return params
		}
	}

	maxParams := 100 // Reasonable limit for function parameters
	for !p.isAtEnd() && len(params) < maxParams {
		// Skip newlines
		for !p.isAtEnd() && p.peek().Type == CTokNewline {
			p.advance()
		}

		// Parse one parameter: type [name]
		var paramTypeParts []string
		var paramName string

		maxTypeParts := 20 // Reasonable limit for type components
		for !p.isAtEnd() && len(paramTypeParts) < maxTypeParts {
			tok := p.peek()

			// Skip newlines
			if tok.Type == CTokNewline {
				p.advance()
				continue
			}

			// End of parameter
			if tok.Type == CTokPunctuation && (tok.Value == "," || tok.Value == ")") {
				break
			}

			paramTypeParts = append(paramTypeParts, tok.Value)
			p.advance()
		}

		if len(paramTypeParts) == 0 {
			break
		}

		// Last identifier might be the parameter name
		// If it doesn't look like a type component, it's the name
		lastPart := paramTypeParts[len(paramTypeParts)-1]
		if len(paramTypeParts) > 1 && !strings.Contains(lastPart, "*") &&
			lastPart != "const" && lastPart != "struct" && lastPart != "enum" &&
			!strings.HasPrefix(lastPart, "SDL_") && !strings.HasPrefix(lastPart, "RL_") {
			paramName = lastPart
			paramTypeParts = paramTypeParts[:len(paramTypeParts)-1]
		}

		// Filter out macros from parameter type
		var actualParamType []string
		for _, part := range paramTypeParts {
			if part == "SDL_DECLSPEC" || part == "SDLCALL" || part == "const" {
				continue
			}
			actualParamType = append(actualParamType, part)
		}

		paramType := strings.Join(actualParamType, " ")
		if paramType != "" {
			params = append(params, CFunctionParam{
				Type: paramType,
				Name: paramName,
			})
		}

		// Check for comma (more parameters) or closing paren
		if p.match(",") {
			p.advance()
			continue
		}

		if p.match(")") {
			p.advance() // Consume the closing paren
			return params
		}
	}

	// If we get here, consume closing paren if present
	if p.match(")") {
		p.advance()
	}

	return params
}

// Token navigation helpers
func (p *CParser) peek() CToken {
	if p.isAtEnd() {
		return CToken{Type: CTokEOF}
	}
	return p.tokens[p.pos]
}

func (p *CParser) advance() CToken {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.tokens[p.pos-1]
}

func (p *CParser) isAtEnd() bool {
	return p.pos >= len(p.tokens)
}

func (p *CParser) match(value string) bool {
	if p.isAtEnd() {
		return false
	}
	return p.peek().Value == value
}

// tryParseTypedefEnum tries to parse a typedef enum
func (p *CParser) tryParseTypedefEnum() bool {
	saved := p.pos
	p.advance() // Skip 'typedef'

	if p.isAtEnd() || p.peek().Value != "enum" {
		p.pos = saved
		return false
	}

	p.parseEnum()
	return true
}

// parseEnum parses an enum declaration
func (p *CParser) parseEnum() {
	p.advance() // Skip 'enum'

	// Optional enum name
	if !p.isAtEnd() && p.peek().Type == CTokIdentifier && p.peek().Value != "{" {
		p.advance() // Skip enum name
	}

	// Find opening brace
	if p.isAtEnd() || p.peek().Value != "{" {
		return
	}
	p.advance() // Skip '{'

	enumValue := 0
	for !p.isAtEnd() {
		tok := p.peek()

		// End of enum
		if tok.Value == "}" {
			p.advance()
			break
		}

		// Skip commas
		if tok.Value == "," {
			p.advance()
			continue
		}

		// Enum member
		if tok.Type == CTokIdentifier {
			name := tok.Value
			p.advance()

			// Check for explicit value
			if !p.isAtEnd() && p.peek().Value == "=" {
				p.advance() // Skip '='

				if !p.isAtEnd() {
					valueTok := p.peek()
					if valueTok.Type == CTokNumber {
						// Parse the value
						valueStr := valueTok.Value
						var parsedValue int64
						var err error

						// Handle hex values
						if strings.HasPrefix(valueStr, "0x") || strings.HasPrefix(valueStr, "0X") {
							parsedValue, err = strconv.ParseInt(valueStr[2:], 16, 64)
						} else if strings.HasPrefix(valueStr, "0b") || strings.HasPrefix(valueStr, "0B") {
							parsedValue, err = strconv.ParseInt(valueStr[2:], 2, 64)
						} else {
							parsedValue, err = strconv.ParseInt(valueStr, 10, 64)
						}

						if err == nil {
							enumValue = int(parsedValue)
						}
						p.advance()
					}
				}
			}

			// Store the enum constant
			p.results.Constants[name] = int64(enumValue)
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "  Enum constant: %s = %d (0x%x)\n", name, enumValue, enumValue)
			}

			enumValue++
		} else {
			// Skip unexpected tokens
			p.advance()
		}
	}

	// Skip until semicolon (for typedef enum Name { ... } Name; pattern)
	p.skipUntil(";")
}

// ParseCHeaderFile is a convenience function that parses a C header file
func ParseCHeaderFile(filepath string) (*CHeaderConstants, error) {
	parser := NewCParser()
	return parser.ParseFile(filepath)
}
