// Completion: 100% - Error handling complete, clear and helpful messages
package main

import (
	"fmt"
	"strings"
)

// ErrorLevel indicates the severity of an error
type ErrorLevel int

const (
	LevelWarning ErrorLevel = iota
	LevelError
	LevelFatal
)

func (l ErrorLevel) String() string {
	switch l {
	case LevelWarning:
		return "warning"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal error"
	default:
		return "unknown"
	}
}

// ErrorCategory classifies the type of error
type ErrorCategory int

const (
	CategorySyntax ErrorCategory = iota
	CategorySemantic
	CategoryCodegen
	CategoryInternal
)

func (c ErrorCategory) String() string {
	switch c {
	case CategorySyntax:
		return "syntax"
	case CategorySemantic:
		return "semantic"
	case CategoryCodegen:
		return "codegen"
	case CategoryInternal:
		return "internal"
	default:
		return "unknown"
	}
}

// SourceLocation represents a position in source code
type SourceLocation struct {
	File   string
	Line   int
	Column int
	Length int // Length of the problematic token/expression
}

func (loc SourceLocation) String() string {
	if loc.File == "" {
		return fmt.Sprintf("%d:%d", loc.Line, loc.Column)
	}
	return fmt.Sprintf("%s:%d:%d", loc.File, loc.Line, loc.Column)
}

// ErrorContext provides additional context for an error
type ErrorContext struct {
	SourceLine string // The actual line of source code
	Suggestion string // "Did you mean 'x'?"
	HelpText   string // Explanatory help text
}

// CompilerError represents a single compilation error
type CompilerError struct {
	Level    ErrorLevel
	Category ErrorCategory
	Message  string
	Location SourceLocation
	Context  ErrorContext
}

// Error implements the error interface
func (e CompilerError) Error() string {
	return fmt.Sprintf("%s: %s", e.Location, e.Message)
}

// Format returns a nicely formatted error message with context
func (e CompilerError) Format(useColor bool) string {
	var sb strings.Builder

	// Error header
	if useColor {
		sb.WriteString("\033[1;31m") // Bold red
	}
	sb.WriteString(e.Level.String())
	sb.WriteString(": ")
	if useColor {
		sb.WriteString("\033[0m") // Reset
	}
	sb.WriteString(e.Message)
	sb.WriteString("\n")

	// Location
	if useColor {
		sb.WriteString("\033[1;34m") // Bold blue
	}
	sb.WriteString("  --> ")
	sb.WriteString(e.Location.String())
	if useColor {
		sb.WriteString("\033[0m")
	}
	sb.WriteString("\n")

	// Source context
	if e.Context.SourceLine != "" {
		lineNum := fmt.Sprintf("%d", e.Location.Line)
		padding := strings.Repeat(" ", len(lineNum)+1)

		sb.WriteString(padding)
		sb.WriteString("|\n")
		sb.WriteString(lineNum)
		sb.WriteString(" | ")
		sb.WriteString(e.Context.SourceLine)
		sb.WriteString("\n")
		sb.WriteString(padding)
		sb.WriteString("| ")

		// Underline the error position
		if e.Location.Column > 0 {
			sb.WriteString(strings.Repeat(" ", e.Location.Column-1))
			if useColor {
				sb.WriteString("\033[1;31m") // Bold red
			}
			if e.Location.Length > 0 {
				sb.WriteString(strings.Repeat("^", e.Location.Length))
			} else {
				sb.WriteString("^")
			}
			if useColor {
				sb.WriteString("\033[0m")
			}
			sb.WriteString("\n")
		}
	}

	// Suggestion
	if e.Context.Suggestion != "" {
		if useColor {
			sb.WriteString("\033[1;32m") // Bold green
		}
		sb.WriteString("   help: ")
		if useColor {
			sb.WriteString("\033[0m")
		}
		sb.WriteString(e.Context.Suggestion)
		sb.WriteString("\n")
	}

	// Help text
	if e.Context.HelpText != "" {
		if useColor {
			sb.WriteString("\033[1;36m") // Bold cyan
		}
		sb.WriteString("   note: ")
		if useColor {
			sb.WriteString("\033[0m")
		}
		sb.WriteString(e.Context.HelpText)
		sb.WriteString("\n")
	}

	return sb.String()
}

// ErrorCollector accumulates errors during compilation
type ErrorCollector struct {
	errors     []CompilerError
	warnings   []CompilerError
	maxErrors  int
	sourceCode string // Full source code for context
}

// NewErrorCollector creates a new error collector
func NewErrorCollector(maxErrors int) *ErrorCollector {
	if maxErrors <= 0 {
		maxErrors = 10 // Default: stop after 10 errors
	}
	return &ErrorCollector{
		errors:    make([]CompilerError, 0),
		warnings:  make([]CompilerError, 0),
		maxErrors: maxErrors,
	}
}

// SetSourceCode stores the source code for error context
func (ec *ErrorCollector) SetSourceCode(source string) {
	ec.sourceCode = source
}

// AddError adds a compilation error
func (ec *ErrorCollector) AddError(err CompilerError) {
	// Auto-populate source line if not provided
	if err.Context.SourceLine == "" && ec.sourceCode != "" {
		err.Context.SourceLine = ec.getSourceLine(err.Location.Line)
	}

	if err.Level == LevelFatal || err.Level == LevelError {
		ec.errors = append(ec.errors, err)
	} else {
		ec.warnings = append(ec.warnings, err)
	}
}

// AddWarning adds a warning
func (ec *ErrorCollector) AddWarning(warn CompilerError) {
	warn.Level = LevelWarning
	if warn.Context.SourceLine == "" && ec.sourceCode != "" {
		warn.Context.SourceLine = ec.getSourceLine(warn.Location.Line)
	}
	ec.warnings = append(ec.warnings, warn)
}

// getSourceLine extracts a specific line from source code
func (ec *ErrorCollector) getSourceLine(lineNum int) string {
	if ec.sourceCode == "" || lineNum <= 0 {
		return ""
	}

	lines := strings.Split(ec.sourceCode, "\n")
	if lineNum > len(lines) {
		return ""
	}
	return lines[lineNum-1]
}

// HasErrors returns true if any errors were collected
func (ec *ErrorCollector) HasErrors() bool {
	return len(ec.errors) > 0
}

// HasFatalError returns true if any fatal errors were collected
func (ec *ErrorCollector) HasFatalError() bool {
	for _, err := range ec.errors {
		if err.Level == LevelFatal {
			return true
		}
	}
	return false
}

// ErrorCount returns the number of errors
func (ec *ErrorCollector) ErrorCount() int {
	return len(ec.errors)
}

// WarningCount returns the number of warnings
func (ec *ErrorCollector) WarningCount() int {
	return len(ec.warnings)
}

// ShouldStop returns true if we've hit the error limit
func (ec *ErrorCollector) ShouldStop() bool {
	return len(ec.errors) >= ec.maxErrors
}

// Report formats all errors and warnings for display
func (ec *ErrorCollector) Report(useColor bool) string {
	var sb strings.Builder

	// Report all errors
	for i, err := range ec.errors {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(err.Format(useColor))
	}

	// Report all warnings
	for i, warn := range ec.warnings {
		if i > 0 || len(ec.errors) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(warn.Format(useColor))
	}

	// Summary
	if len(ec.errors) > 0 || len(ec.warnings) > 0 {
		sb.WriteString("\n")
		if len(ec.errors) > 0 {
			if useColor {
				sb.WriteString("\033[1;31m")
			}
			sb.WriteString(fmt.Sprintf("%d error(s)", len(ec.errors)))
			if useColor {
				sb.WriteString("\033[0m")
			}
		}
		if len(ec.warnings) > 0 {
			if len(ec.errors) > 0 {
				sb.WriteString(", ")
			}
			if useColor {
				sb.WriteString("\033[1;33m")
			}
			sb.WriteString(fmt.Sprintf("%d warning(s)", len(ec.warnings)))
			if useColor {
				sb.WriteString("\033[0m")
			}
		}
		sb.WriteString(" found\n")
	}

	return sb.String()
}

// Clear resets the error collector
func (ec *ErrorCollector) Clear() {
	ec.errors = make([]CompilerError, 0)
	ec.warnings = make([]CompilerError, 0)
}

// Helper functions for creating common errors

// UndefinedVariableError creates an error for undefined variables
func UndefinedVariableError(name string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelError,
		Category: CategorySemantic,
		Message:  fmt.Sprintf("undefined variable '%s'", name),
		Location: loc,
		Context: ErrorContext{
			HelpText: "Variables must be declared before use",
		},
	}
}

// TypeMismatchError creates an error for type mismatches
func TypeMismatchError(expected, actual string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelError,
		Category: CategorySemantic,
		Message:  fmt.Sprintf("type mismatch: expected %s, got %s", expected, actual),
		Location: loc,
	}
}

// ImmutableUpdateError creates an error for updating immutable variables
func ImmutableUpdateError(name string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelError,
		Category: CategorySemantic,
		Message:  fmt.Sprintf("cannot update immutable variable '%s'", name),
		Location: loc,
		Context: ErrorContext{
			Suggestion: fmt.Sprintf("declare '%s' as mutable with ':='", name),
		},
	}
}

// SyntaxError creates a syntax error
func SyntaxError(message string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelError,
		Category: CategorySyntax,
		Message:  message,
		Location: loc,
	}
}

// UnexpectedTokenError creates an error for unexpected tokens
func UnexpectedTokenError(expected, got string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelError,
		Category: CategorySyntax,
		Message:  fmt.Sprintf("expected %s, got %s", expected, got),
		Location: loc,
	}
}

// FatalError creates a fatal internal error
func FatalError(message string, loc SourceLocation) CompilerError {
	return CompilerError{
		Level:    LevelFatal,
		Category: CategoryInternal,
		Message:  message,
		Location: loc,
		Context: ErrorContext{
			HelpText: "This is an internal compiler error. Please report this bug.",
		},
	}
}
