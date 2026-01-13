// Completion: 95% - Core lexer complete, supports all Vibe67 3.0 tokens
package main

import (
	"strings"
	"unicode"
)

// Token types for Vibe67 language
type TokenType int

const (
	TOKEN_EOF TokenType = iota
	TOKEN_IDENT
	TOKEN_NUMBER
	TOKEN_STRING
	TOKEN_FSTRING // f"..." interpolated string
	TOKEN_PLUS
	TOKEN_MINUS
	TOKEN_STAR
	TOKEN_POWER // ** (exponentiation)
	TOKEN_CARET // ^ (exponentiation alias)
	TOKEN_SLASH
	TOKEN_MOD
	TOKEN_EQUALS
	TOKEN_COLON_EQUALS
	TOKEN_EQUALS_QUESTION     // =? (immutable assignment with error propagation)
	TOKEN_LEFT_ARROW_QUESTION // <-? (mutable update with error propagation)
	TOKEN_PLUS_EQUALS         // +=
	TOKEN_MINUS_EQUALS        // -=
	TOKEN_STAR_EQUALS         // *=
	TOKEN_POWER_EQUALS        // **=
	TOKEN_SLASH_EQUALS        // /=
	TOKEN_MOD_EQUALS          // %=
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_COMMA
	TOKEN_COLON
	TOKEN_SEMICOLON
	TOKEN_NEWLINE
	TOKEN_LT              // <
	TOKEN_GT              // >
	TOKEN_LE              // <= (less than or equal - comparison operator)
	TOKEN_GE              // >=
	TOKEN_EQ              // ==
	TOKEN_NE              // !=
	TOKEN_TILDE           // ~
	TOKEN_DEFAULT_ARROW   // ~>
	TOKEN_AT              // @
	TOKEN_AT_AT           // @@ (parallel loop with all cores)
	TOKEN_AT_PLUSPLUS     // @++
	TOKEN_IN              // in keyword
	TOKEN_LBRACE          // {
	TOKEN_RBRACE          // }
	TOKEN_LBRACKET        // [
	TOKEN_RBRACKET        // ]
	TOKEN_ARROW           // -> (lambda arrow, can be inferred in assignment context)
	TOKEN_FAT_ARROW       // => (match arm)
	TOKEN_LEFT_ARROW      // <- (update operator and ENet send/receive)
	TOKEN_COLONCOLON      // :: (list append/cons operator)
	TOKEN_AMPERSAND       // & (address operator)
	TOKEN_ADDRESS_LITERAL // &8080 or &host:port (ENet address literal)
	TOKEN_PIPE            // |
	TOKEN_PIPEPIPE        // ||
	TOKEN_HASH            // #
	TOKEN_AND             // and keyword
	TOKEN_OR              // or keyword
	TOKEN_NOT             // not keyword
	TOKEN_XOR             // xor keyword
	TOKEN_INCREMENT       // ++
	TOKEN_DECREMENT       // --
	TOKEN_FMA             // *+ (fused multiply-add)
	TOKEN_BANG            // ! (move operator - transfers ownership)
	TOKEN_OR_BANG         // or! (error handling / railway-oriented programming)
	TOKEN_AND_BANG        // and! (success handler)
	TOKEN_ERR_QUESTION    // err? (check if expression is error)
	TOKEN_VAL_QUESTION    // val? (check if expression has value)
	// TOKEN_ME and TOKEN_CME removed - recursive calls now use mandatory max
	TOKEN_RET        // ret keyword (return value from function/lambda)
	TOKEN_ERR        // err keyword (return error from function/lambda)
	TOKEN_AT_FIRST   // @first (first iteration)
	TOKEN_AT_LAST    // @last (last iteration)
	TOKEN_AT_COUNTER // @counter (iteration counter)
	TOKEN_AT_I       // @i (current element/item)
	TOKEN_PIPE_B     // |b (bitwise OR)
	TOKEN_AMP_B      // &b (bitwise AND)
	TOKEN_CARET_B    // ^b (bitwise XOR)
	TOKEN_TILDE_B    // ~b (bitwise NOT)
	TOKEN_AMP        // & (used in unsafe blocks, not for lists)
	TOKEN_UNDERSCORE // _ (wildcard for default match)
	TOKEN_DOLLAR     // $ (address value operator)
	TOKEN_MU         // µ (memory ownership/movement operator)
	TOKEN_LTLT_B     // <<b (shift left)
	TOKEN_GTGT_B     // >>b (shift right)
	TOKEN_LTLTLT_B   // <<<b (rotate left)
	TOKEN_GTGTGT_B   // >>>b (rotate right)
	TOKEN_QUESTION_B // ?b (bit test)
	TOKEN_AS         // as (type casting)
	TOKEN_AS_BANG    // as! (raw bitcast)
	// C type keywords
	TOKEN_I8   // i8
	TOKEN_I16  // i16
	TOKEN_I32  // i32
	TOKEN_I64  // i64
	TOKEN_U8   // u8
	TOKEN_U16  // u16
	TOKEN_U32  // u32
	TOKEN_U64  // u64
	TOKEN_F32  // f32
	TOKEN_F64  // f64
	TOKEN_CSTR // cstr
	TOKEN_CPTR // cptr (C pointer)
	// Vibe67 type keywords
	TOKEN_NUM      // num (number type)
	TOKEN_STR      // str (string type)
	TOKEN_LIST     // list (type)
	TOKEN_MAP      // map (type)
	TOKEN_CSTRING  // cstring (C char*)
	TOKEN_CINT     // cint (C int)
	TOKEN_CLONG    // clong (C long/int64_t)
	TOKEN_CFLOAT   // cfloat (C float)
	TOKEN_CDOUBLE  // cdouble (C double)
	TOKEN_CBOOL    // cbool (C bool)
	TOKEN_CVOID    // cvoid (C void)
	TOKEN_USE      // use (import)
	TOKEN_IMPORT   // import (with git URL)
	TOKEN_EXPORT   // export (export functions for import)
	TOKEN_DOT      // . (for namespaced calls)
	TOKEN_DOTDOT   // .. (inclusive range operator)
	TOKEN_DOTDOTLT // ..< (exclusive range operator)
	TOKEN_ELLIPSIS // ... (variadic parameter marker)
	TOKEN_UNSAFE   // unsafe (architecture-specific code blocks)
	TOKEN_SYSCALL  // syscall (system call in unsafe blocks)
	TOKEN_ARENA    // arena (arena memory blocks)
	TOKEN_DEFER    // defer (deferred execution)
	TOKEN_MAX      // max (maximum iterations for loops)
	TOKEN_INF      // inf (infinity, for unlimited iterations or numeric infinity)
	TOKEN_CSTRUCT  // cstruct (C-compatible struct definition)
	TOKEN_PACKED   // packed (no padding modifier for cstruct)
	TOKEN_ALIGNED  // aligned (alignment modifier for cstruct)
	TOKEN_ALIAS    // alias (create keyword aliases for language packs)
	TOKEN_ORBANG   // or! (unwrap with default value)
	TOKEN_SPAWN    // spawn (spawn background process with fork)
	TOKEN_HAS      // has (type/class definitions)
	TOKEN_CLASS    // class (class definition)
	TOKEN_LTGT     // <> (composition operator)
	TOKEN_RANDOM   // ?? (random number operator)
	TOKEN_SHADOW   // shadow (explicit shadowing declaration)
)

// Code generation constants
const (
	// Jump instruction sizes on x86-64
	UnconditionalJumpSize = 5 // Size of JumpUnconditional (0xe9 + 4-byte offset)
	ConditionalJumpSize   = 6 // Size of JumpConditional (0x0f 0x8X + 4-byte offset)

	// Stack layout
	StackSlotSize = 8 // Size of a stack slot (8 bytes for float64/pointer)

	// Byte manipulation
	ByteMask = 0xFF // Mask for extracting a single byte
)

type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int // Column position (1-indexed) where the token starts
}

// isHexDigit checks if a byte is a valid hexadecimal digit
func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// processEscapeSequences converts escape sequences in a string to their actual characters
func processEscapeSequences(s string) string {
	// Handle UTF-8 properly by converting to runes first
	var result strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			switch runes[i+1] {
			case 'n':
				result.WriteRune('\n')
			case 't':
				result.WriteRune('\t')
			case 'r':
				result.WriteRune('\r')
			case '\\':
				result.WriteRune('\\')
			case '"':
				result.WriteRune('"')
			default:
				// Unknown escape sequence - keep backslash and the character
				result.WriteRune(runes[i])
				result.WriteRune(runes[i+1])
			}
			i++ // Skip the escaped character
		} else {
			result.WriteRune(runes[i])
		}
	}
	return result.String()
}

// Lexer for Vibe67 language
type Lexer struct {
	input     string
	pos       int
	line      int
	column    int // Current column (1-indexed)
	lineStart int // Position where current line starts
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, pos: 0, line: 1, column: 1, lineStart: 0}

	// Skip shebang line if present (#!/usr/bin/vibe67)
	if len(input) >= 2 && input[0] == '#' && input[1] == '!' {
		// Skip until newline
		for l.pos < len(l.input) && l.input[l.pos] != '\n' {
			l.pos++
		}
		if l.pos < len(l.input) && l.input[l.pos] == '\n' {
			l.pos++ // Skip the newline too
			l.line++
			l.lineStart = l.pos
			l.column = 1
		}
	}

	return l
}

func (l *Lexer) peek() byte {
	if l.pos+1 < len(l.input) {
		return l.input[l.pos+1]
	}
	return 0
}

// peekAhead looks n characters ahead (0-indexed from current position)
func (l *Lexer) peekAhead(n int) byte {
	if l.pos+1+n < len(l.input) {
		return l.input[l.pos+1+n]
	}
	return 0
}

func (l *Lexer) advance() {
	if l.pos < len(l.input) {
		l.pos++
	}
}

// LexerState represents a saved lexer state for lookahead
type LexerState struct {
	pos  int
	line int
}

// save returns the current lexer state
func (l *Lexer) save() LexerState {
	return LexerState{pos: l.pos, line: l.line}
}

// restore restores a previously saved lexer state
func (l *Lexer) restore(state LexerState) {
	l.pos = state.pos
	l.line = state.line
}

func (l *Lexer) NextToken() Token {
	// Update column based on current position
	l.column = l.pos - l.lineStart + 1

	// Skip whitespace (except newlines)
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\r') {
		l.pos++
	}

	// Skip comments (lines starting with //)
	if l.pos < len(l.input)-1 && l.input[l.pos] == '/' && l.input[l.pos+1] == '/' {
		for l.pos < len(l.input) && l.input[l.pos] != '\n' {
			l.pos++
		}
		// Recursively get the next token after the comment
		return l.NextToken()
	}

	// Record token start column
	tokenColumn := l.pos - l.lineStart + 1

	if l.pos >= len(l.input) {
		return Token{Type: TOKEN_EOF, Line: l.line, Column: tokenColumn}
	}
	ch := l.input[l.pos]

	// Newline
	if ch == '\n' {
		tok := Token{Type: TOKEN_NEWLINE, Line: l.line, Column: tokenColumn}
		l.pos++
		l.line++
		l.lineStart = l.pos
		l.column = 1
		return tok
	}

	// String literal
	if ch == '"' {
		l.pos++
		start := l.pos
		for l.pos < len(l.input) && l.input[l.pos] != '"' {
			// Skip escaped characters (including escaped quotes)
			if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
				l.pos += 2 // Skip backslash and next character
			} else {
				l.pos++
			}
		}
		value := l.input[start:l.pos]
		l.pos++ // skip closing "
		// Process escape sequences like \n, \t, etc.
		value = processEscapeSequences(value)
		return Token{Type: TOKEN_STRING, Value: value, Line: l.line, Column: tokenColumn}
	}

	// Number (including hex 0x... and binary 0b...)
	if unicode.IsDigit(rune(ch)) {
		start := l.pos

		// Check for hex or binary prefix
		if ch == '0' && l.pos+1 < len(l.input) {
			next := l.input[l.pos+1]
			if next == 'x' || next == 'X' {
				// Hexadecimal: 0x[0-9a-fA-F]+
				l.pos += 2 // skip '0x'
				if l.pos >= len(l.input) || !isHexDigit(l.input[l.pos]) {
					// Invalid hex literal
					return Token{Type: TOKEN_NUMBER, Value: "0", Line: l.line, Column: tokenColumn}
				}
				for l.pos < len(l.input) && isHexDigit(l.input[l.pos]) {
					l.pos++
				}
				return Token{Type: TOKEN_NUMBER, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
			} else if next == 'b' || next == 'B' {
				// Binary: 0b[01]+
				l.pos += 2 // skip '0b'
				if l.pos >= len(l.input) || (l.input[l.pos] != '0' && l.input[l.pos] != '1') {
					// Invalid binary literal
					return Token{Type: TOKEN_NUMBER, Value: "0", Line: l.line, Column: tokenColumn}
				}
				for l.pos < len(l.input) && (l.input[l.pos] == '0' || l.input[l.pos] == '1') {
					l.pos++
				}
				return Token{Type: TOKEN_NUMBER, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
			}
		}

		// Regular decimal number
		hasDot := false
		for l.pos < len(l.input) {
			if unicode.IsDigit(rune(l.input[l.pos])) {
				l.pos++
			} else if l.input[l.pos] == '.' && !hasDot {
				// Check if this is part of a range operator (..<  or ..=)
				if l.pos+1 < len(l.input) && l.input[l.pos+1] == '.' {
					// This is start of ..<  or ..=, stop number parsing
					break
				}
				hasDot = true
				l.pos++
			} else {
				break
			}
		}
		return Token{Type: TOKEN_NUMBER, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
	}

	// Identifier or keyword (cannot start with underscore or digit)
	if unicode.IsLetter(rune(ch)) {
		start := l.pos
		for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_') {
			l.pos++
		}
		value := l.input[start:l.pos]

		// Check for f-string: f"..."
		if value == "f" && l.pos < len(l.input) && l.input[l.pos] == '"' {
			l.pos++ // skip opening "
			fstringStart := l.pos
			for l.pos < len(l.input) && l.input[l.pos] != '"' {
				// Skip escaped characters (including escaped quotes)
				if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
					l.pos += 2
				} else {
					l.pos++
				}
			}
			fstringValue := l.input[fstringStart:l.pos]
			l.pos++ // skip closing "
			return Token{Type: TOKEN_FSTRING, Value: fstringValue, Line: l.line, Column: tokenColumn}
		}

		// Check for keywords
		switch value {
		case "in":
			return Token{Type: TOKEN_IN, Value: value, Line: l.line, Column: tokenColumn}
		case "and":
			// Check for and!
			if l.pos < len(l.input) && l.input[l.pos] == '!' {
				l.pos++ // consume the !
				return Token{Type: TOKEN_AND_BANG, Value: "and!", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_AND, Value: value, Line: l.line, Column: tokenColumn}
		case "or":
			// Check for or!
			if l.pos < len(l.input) && l.input[l.pos] == '!' {
				l.pos++ // consume the !
				return Token{Type: TOKEN_OR_BANG, Value: "or!", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_OR, Value: value, Line: l.line, Column: tokenColumn}
		case "not":
			return Token{Type: TOKEN_NOT, Value: value, Line: l.line, Column: tokenColumn}
		// "me" and "cme" removed - recursive calls now use function name with mandatory max
		case "ret":
			return Token{Type: TOKEN_RET, Value: value, Line: l.line, Column: tokenColumn}
		case "err":
			// Check for err?
			if l.pos < len(l.input) && l.input[l.pos] == '?' {
				l.pos++ // consume the ?
				return Token{Type: TOKEN_ERR_QUESTION, Value: "err?", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_ERR, Value: value, Line: l.line, Column: tokenColumn}
		case "val":
			// Check for val?
			if l.pos < len(l.input) && l.input[l.pos] == '?' {
				l.pos++ // consume the ?
				return Token{Type: TOKEN_VAL_QUESTION, Value: "val?", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_IDENT, Value: value, Line: l.line, Column: tokenColumn}
		case "use":
			return Token{Type: TOKEN_USE, Value: value, Line: l.line, Column: tokenColumn}
		case "import":
			return Token{Type: TOKEN_IMPORT, Value: value, Line: l.line, Column: tokenColumn}
		case "export":
			return Token{Type: TOKEN_EXPORT, Value: value, Line: l.line, Column: tokenColumn}
		case "as":
			// Check for "as!" (raw bitcast)
			if l.pos < len(l.input) && l.input[l.pos] == '!' {
				l.pos++
				return Token{Type: TOKEN_AS_BANG, Value: "as!", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_AS, Value: value, Line: l.line, Column: tokenColumn}
		case "unsafe":
			return Token{Type: TOKEN_UNSAFE, Value: value, Line: l.line, Column: tokenColumn}
		case "syscall":
			return Token{Type: TOKEN_SYSCALL, Value: value, Line: l.line, Column: tokenColumn}
		case "arena":
			return Token{Type: TOKEN_ARENA, Value: value, Line: l.line, Column: tokenColumn}
		case "defer":
			return Token{Type: TOKEN_DEFER, Value: value, Line: l.line, Column: tokenColumn}
		case "max":
			return Token{Type: TOKEN_MAX, Value: value, Line: l.line, Column: tokenColumn}
		case "inf":
			return Token{Type: TOKEN_INF, Value: value, Line: l.line, Column: tokenColumn}
		case "cstruct":
			return Token{Type: TOKEN_CSTRUCT, Value: value, Line: l.line, Column: tokenColumn}
		case "packed":
			return Token{Type: TOKEN_PACKED, Value: value, Line: l.line, Column: tokenColumn}
		case "aligned":
			return Token{Type: TOKEN_ALIGNED, Value: value, Line: l.line, Column: tokenColumn}
		case "alias":
			return Token{Type: TOKEN_ALIAS, Value: value, Line: l.line, Column: tokenColumn}
		case "or!":
			return Token{Type: TOKEN_ORBANG, Value: value, Line: l.line, Column: tokenColumn}
		case "spawn":
			return Token{Type: TOKEN_SPAWN, Value: value, Line: l.line, Column: tokenColumn}
		case "has":
			return Token{Type: TOKEN_HAS, Value: value, Line: l.line, Column: tokenColumn}
		case "class":
			return Token{Type: TOKEN_CLASS, Value: value, Line: l.line, Column: tokenColumn}
		case "shadow":
			return Token{Type: TOKEN_SHADOW, Value: value, Line: l.line, Column: tokenColumn}
		case "xor":
			return Token{Type: TOKEN_XOR, Value: value, Line: l.line, Column: tokenColumn}
			// Note: All type keywords (i8, i16, i32, i64, u8, u16, u32, u64, f32, f64,
			// cstr, ptr, number, string, list) are contextual keywords.
			// They are only treated as type keywords after "as" in cast expressions.
			// Otherwise they can be used as identifiers.
		}

		return Token{Type: TOKEN_IDENT, Value: value, Line: l.line, Column: tokenColumn}
	}

	// Operators and punctuation
	switch ch {
	case '+':
		l.pos++
		// Check for ++
		if l.pos < len(l.input) && l.input[l.pos] == '+' {
			l.pos++
			return Token{Type: TOKEN_INCREMENT, Value: "++", Line: l.line, Column: tokenColumn}
		}
		// Check for +=
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TOKEN_PLUS_EQUALS, Value: "+=", Line: l.line, Column: tokenColumn}
		}
		return Token{Type: TOKEN_PLUS, Value: "+", Line: l.line, Column: tokenColumn}
	case '-':
		// Check for -> (lambda arrow, can be inferred in assignment context)
		if l.peek() == '>' {
			l.pos += 2
			return Token{Type: TOKEN_ARROW, Value: "->", Line: l.line, Column: tokenColumn}
		}
		// Check for --
		if l.peek() == '-' {
			l.pos += 2
			return Token{Type: TOKEN_DECREMENT, Value: "--", Line: l.line, Column: tokenColumn}
		}
		// Check for -=
		if l.peek() == '=' {
			l.pos += 2
			return Token{Type: TOKEN_MINUS_EQUALS, Value: "-=", Line: l.line, Column: tokenColumn}
		}
		// Always emit MINUS as separate token - let parser handle unary negation
		l.pos++
		return Token{Type: TOKEN_MINUS, Value: "-", Line: l.line, Column: tokenColumn}
	case '*':
		l.pos++
		// Check for *+ (fused multiply-add)
		if l.pos < len(l.input) && l.input[l.pos] == '+' {
			l.pos++
			return Token{Type: TOKEN_FMA, Value: "*+", Line: l.line, Column: tokenColumn}
		}
		// Check for ** (power) and **= (power assignment)
		if l.pos < len(l.input) && l.input[l.pos] == '*' {
			l.pos++
			// Check for **=
			if l.pos < len(l.input) && l.input[l.pos] == '=' {
				l.pos++
				return Token{Type: TOKEN_POWER_EQUALS, Value: "**=", Line: l.line, Column: tokenColumn}
			}
			return Token{Type: TOKEN_POWER, Value: "**", Line: l.line, Column: tokenColumn}
		}
		// Check for *=
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TOKEN_STAR_EQUALS, Value: "*=", Line: l.line, Column: tokenColumn}
		}
		return Token{Type: TOKEN_STAR, Value: "*", Line: l.line, Column: tokenColumn}
	case '/':
		l.pos++
		// Check for /=
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TOKEN_SLASH_EQUALS, Value: "/=", Line: l.line, Column: tokenColumn}
		}
		return Token{Type: TOKEN_SLASH, Value: "/", Line: l.line, Column: tokenColumn}
	case '%':
		l.pos++
		// Check for %=
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TOKEN_MOD_EQUALS, Value: "%=", Line: l.line, Column: tokenColumn}
		}
		return Token{Type: TOKEN_MOD, Value: "%", Line: l.line, Column: tokenColumn}
	case ':':
		// Check for := and :: before advancing
		if l.peek() == '=' {
			l.pos += 2 // skip both ':' and '='
			return Token{Type: TOKEN_COLON_EQUALS, Value: ":=", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == ':' {
			l.pos += 2 // skip both ':'
			return Token{Type: TOKEN_COLONCOLON, Value: "::", Line: l.line, Column: tokenColumn}
		}
		// Regular colon for map literals and method calls
		l.pos++
		return Token{Type: TOKEN_COLON, Value: ":", Line: l.line, Column: tokenColumn}
	case '=':
		// Check for =>
		if l.peek() == '>' {
			l.pos += 2
			return Token{Type: TOKEN_FAT_ARROW, Value: "=>", Line: l.line, Column: tokenColumn}
		}
		// Check for ==
		if l.peek() == '=' {
			l.pos += 2
			return Token{Type: TOKEN_EQ, Value: "==", Line: l.line, Column: tokenColumn}
		}
		// Check for =?
		if l.peek() == '?' {
			l.pos += 2
			return Token{Type: TOKEN_EQUALS_QUESTION, Value: "=?", Line: l.line, Column: tokenColumn}
		}
		l.pos++
		return Token{Type: TOKEN_EQUALS, Value: "=", Line: l.line, Column: tokenColumn}
	case '<':
		// Check for <>, then <-?, then <-, then <<<b (rotate left), then <<b (shift left), then <=, then <
		if l.peek() == '>' {
			l.pos += 2
			return Token{Type: TOKEN_LTGT, Value: "<>", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == '-' {
			// Check for <-?
			if l.pos+2 < len(l.input) && l.input[l.pos+2] == '?' {
				l.pos += 3
				return Token{Type: TOKEN_LEFT_ARROW_QUESTION, Value: "<-?", Line: l.line, Column: tokenColumn}
			}
			l.pos += 2
			return Token{Type: TOKEN_LEFT_ARROW, Value: "<-", Line: l.line, Column: tokenColumn}
		}
		// Check for <<<b (rotate left) - must check before <<b
		if l.peek() == '<' && l.pos+2 < len(l.input) && l.input[l.pos+2] == '<' &&
			l.pos+3 < len(l.input) && l.input[l.pos+3] == 'b' {
			l.pos += 4
			return Token{Type: TOKEN_LTLTLT_B, Value: "<<<b", Line: l.line, Column: tokenColumn}
		}
		// Check for <<b (shift left)
		if l.peek() == '<' && l.pos+2 < len(l.input) && l.input[l.pos+2] == 'b' {
			l.pos += 3
			return Token{Type: TOKEN_LTLT_B, Value: "<<b", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == '=' {
			l.pos += 2
			return Token{Type: TOKEN_LE, Value: "<=", Line: l.line, Column: tokenColumn}
		}
		l.pos++
		return Token{Type: TOKEN_LT, Value: "<", Line: l.line, Column: tokenColumn}
	case '>':
		// Check for >>>b (rotate right), then >>b (shift right), then >=, then >
		// Check for >>>b (rotate right) - must check before >>b
		if l.peek() == '>' && l.pos+2 < len(l.input) && l.input[l.pos+2] == '>' &&
			l.pos+3 < len(l.input) && l.input[l.pos+3] == 'b' {
			l.pos += 4
			return Token{Type: TOKEN_GTGTGT_B, Value: ">>>b", Line: l.line, Column: tokenColumn}
		}
		// Check for >>b (shift right)
		if l.peek() == '>' && l.pos+2 < len(l.input) && l.input[l.pos+2] == 'b' {
			l.pos += 3
			return Token{Type: TOKEN_GTGT_B, Value: ">>b", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == '=' {
			l.pos += 2
			return Token{Type: TOKEN_GE, Value: ">=", Line: l.line, Column: tokenColumn}
		}
		l.pos++
		return Token{Type: TOKEN_GT, Value: ">", Line: l.line, Column: tokenColumn}
	case '!':
		// Check for !=, then !b
		if l.peek() == '=' {
			l.pos += 2
			return Token{Type: TOKEN_NE, Value: "!=", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_TILDE_B, Value: "!b", Line: l.line, Column: tokenColumn}
		}
		// Standalone ! is move operator
		l.pos++
		return Token{Type: TOKEN_BANG, Value: "!", Line: l.line, Column: tokenColumn}
	case '?':
		// Check for ?b (bit test operator)
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_QUESTION_B, Value: "?b", Line: l.line, Column: tokenColumn}
		}
		// Check for ?? (random number operator)
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '?' {
			l.pos += 2
			return Token{Type: TOKEN_RANDOM, Value: "??", Line: l.line, Column: tokenColumn}
		}
		// Single ? is not a valid token in Vibe67
		return Token{Type: TOKEN_EOF, Value: "", Line: l.line, Column: tokenColumn}
	case '~':
		// Check for ~> first, then ~b
		if l.peek() == '>' {
			l.pos += 2
			return Token{Type: TOKEN_DEFAULT_ARROW, Value: "~>", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_TILDE_B, Value: "~b", Line: l.line, Column: tokenColumn}
		}
		l.pos++
		return Token{Type: TOKEN_TILDE, Value: "~", Line: l.line, Column: tokenColumn}
	case '(':
		l.pos++
		return Token{Type: TOKEN_LPAREN, Value: "(", Line: l.line, Column: tokenColumn}
	case ')':
		l.pos++
		return Token{Type: TOKEN_RPAREN, Value: ")", Line: l.line, Column: tokenColumn}
	case ',':
		l.pos++
		return Token{Type: TOKEN_COMMA, Value: ",", Line: l.line, Column: tokenColumn}
	case ';':
		l.pos++
		return Token{Type: TOKEN_SEMICOLON, Value: ";", Line: l.line, Column: tokenColumn}
	case '.':
		// Check for ... (variadic marker) or ..< (exclusive) or .. (inclusive)
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '.' {
			if l.pos+2 < len(l.input) {
				if l.input[l.pos+2] == '.' {
					// ... (variadic parameter marker)
					l.pos += 3
					return Token{Type: TOKEN_ELLIPSIS, Value: "...", Line: l.line, Column: tokenColumn}
				} else if l.input[l.pos+2] == '<' {
					// ..<
					l.pos += 3
					return Token{Type: TOKEN_DOTDOTLT, Value: "..<", Line: l.line, Column: tokenColumn}
				}
			}
			// Just .. is inclusive range
			l.pos += 2
			return Token{Type: TOKEN_DOTDOT, Value: "..", Line: l.line, Column: tokenColumn}
		}
		// Single .
		l.pos++
		return Token{Type: TOKEN_DOT, Value: ".", Line: l.line, Column: tokenColumn}
	case '@':
		// Check for @@ (parallel loop)
		if l.peek() == '@' {
			l.pos += 2
			return Token{Type: TOKEN_AT_AT, Value: "@@", Line: l.line, Column: tokenColumn}
		}
		// Check for @++
		if l.peek() == '+' && l.pos+2 < len(l.input) && l.input[l.pos+2] == '+' {
			l.pos += 3
			return Token{Type: TOKEN_AT_PLUSPLUS, Value: "@++", Line: l.line, Column: tokenColumn}
		}

		// Check for special keywords: @first, @last, @counter, @i
		if (l.peek() >= 'a' && l.peek() <= 'z') || (l.peek() >= 'A' && l.peek() <= 'Z') {
			start := l.pos
			l.pos++ // skip @
			value := ""
			for l.pos < len(l.input) && ((l.input[l.pos] >= 'a' && l.input[l.pos] <= 'z') || (l.input[l.pos] >= 'A' && l.input[l.pos] <= 'Z')) {
				l.pos++
			}
			if l.pos > start+1 {
				value = l.input[start:l.pos]
			}

			if value == "@first" {
				return Token{Type: TOKEN_AT_FIRST, Value: value, Line: l.line, Column: tokenColumn}
			}
			if value == "@last" {
				return Token{Type: TOKEN_AT_LAST, Value: value, Line: l.line, Column: tokenColumn}
			}
			if value == "@counter" {
				return Token{Type: TOKEN_AT_COUNTER, Value: value, Line: l.line, Column: tokenColumn}
			}
			if value == "@i" {
				// Check if followed by a number (e.g., @i1, @i2)
				if l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
					numStart := l.pos
					for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
						l.pos++
					}
					fullValue := value + string(l.input[numStart:l.pos])
					return Token{Type: TOKEN_AT_I, Value: fullValue, Line: l.line, Column: tokenColumn}
				}
				return Token{Type: TOKEN_AT_I, Value: value, Line: l.line, Column: tokenColumn}
			}

			// Unknown @ keyword, backtrack
			l.pos = start + 1
		}

		l.pos++
		return Token{Type: TOKEN_AT, Value: "@", Line: l.line, Column: tokenColumn}
	case '{':
		l.pos++
		return Token{Type: TOKEN_LBRACE, Value: "{", Line: l.line, Column: tokenColumn}
	case '}':
		l.pos++
		return Token{Type: TOKEN_RBRACE, Value: "}", Line: l.line, Column: tokenColumn}
	case '[':
		l.pos++
		return Token{Type: TOKEN_LBRACKET, Value: "[", Line: l.line, Column: tokenColumn}
	case ']':
		l.pos++
		return Token{Type: TOKEN_RBRACKET, Value: "]", Line: l.line, Column: tokenColumn}
	case '|':
		// Check for ||, then |b, then |
		if l.peek() == '|' {
			l.pos += 2
			return Token{Type: TOKEN_PIPEPIPE, Value: "||", Line: l.line, Column: tokenColumn}
		}
		if l.peek() == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_PIPE_B, Value: "|b", Line: l.line, Column: tokenColumn}
		}
		l.pos++
		return Token{Type: TOKEN_PIPE, Value: "|", Line: l.line, Column: tokenColumn}
	case '&':
		// Check for &b (bitwise AND)
		if l.peek() == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_AMP_B, Value: "&b", Line: l.line, Column: tokenColumn}
		}

		// Check for address literals: &8080, &:8080, &localhost:8080, &192.168.1.100:7777
		nextChar := l.peek()
		if nextChar == ':' || (nextChar >= '0' && nextChar <= '9') || (nextChar >= 'a' && nextChar <= 'z') || (nextChar >= 'A' && nextChar <= 'Z') {
			start := l.pos
			l.pos++ // skip &

			// Check if it's a port-only address or IP address (starts with digit)
			if l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
				// Could be &8080 or &192.168.1.100:3000
				// Parse digits and dots (for IP addresses)
				for l.pos < len(l.input) && (l.input[l.pos] >= '0' && l.input[l.pos] <= '9' || l.input[l.pos] == '.') {
					l.pos++
				}

				// If followed by :port, parse the port
				if l.pos < len(l.input) && l.input[l.pos] == ':' {
					l.pos++ // skip :
					if l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
						for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
							l.pos++
						}
					}
				}
				return Token{Type: TOKEN_ADDRESS_LITERAL, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
			}

			// Check if it's :port format
			if l.pos < len(l.input) && l.input[l.pos] == ':' {
				l.pos++ // skip :
				if l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
					for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
						l.pos++
					}
					return Token{Type: TOKEN_ADDRESS_LITERAL, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
				}
			}

			// Parse hostname or IP address
			if l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos]))) {
				for l.pos < len(l.input) {
					ch := l.input[l.pos]
					if !unicode.IsLetter(rune(ch)) && !unicode.IsDigit(rune(ch)) && ch != '.' && ch != '-' {
						break
					}
					l.pos++
				}

				// Must have :port after hostname
				if l.pos < len(l.input) && l.input[l.pos] == ':' {
					l.pos++ // skip :
					if l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
						for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
							l.pos++
						}
						return Token{Type: TOKEN_ADDRESS_LITERAL, Value: l.input[start:l.pos], Line: l.line, Column: tokenColumn}
					}
				}
			}

			// Not an address literal, backtrack to just after &
			l.pos = start + 1
			return Token{Type: TOKEN_AMPERSAND, Value: "&", Line: l.line, Column: tokenColumn}
		}

		l.pos++
		return Token{Type: TOKEN_AMPERSAND, Value: "&", Line: l.line, Column: tokenColumn}
	case '^':
		// Check for ^b (bitwise XOR)
		if l.peek() == 'b' {
			l.pos += 2
			return Token{Type: TOKEN_CARET_B, Value: "^b", Line: l.line, Column: tokenColumn}
		}
		// Standalone ^ is exponentiation (alias for **)
		l.pos++
		return Token{Type: TOKEN_CARET, Value: "^", Line: l.line, Column: tokenColumn}
	case '#':
		l.pos++
		return Token{Type: TOKEN_HASH, Value: "#", Line: l.line, Column: tokenColumn}
	case '_':
		// Underscore used as wildcard in match expressions
		l.pos++
		return Token{Type: TOKEN_UNDERSCORE, Value: "_", Line: l.line, Column: tokenColumn}
	case '$':
		l.pos++
		return Token{Type: TOKEN_DOLLAR, Value: "$", Line: l.line, Column: tokenColumn}
	case 'µ':
		l.pos += len("µ") // µ is multi-byte UTF-8
		return Token{Type: TOKEN_MU, Value: "µ", Line: l.line, Column: tokenColumn}
	}

	return Token{Type: TOKEN_EOF, Line: l.line, Column: tokenColumn}
}
