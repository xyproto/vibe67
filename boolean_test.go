package main

import (
	"testing"
)

// TestBooleanTypeLexing tests that yes/no/bool are recognized as tokens
func TestBooleanTypeLexing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected TokenType
	}{
		{"yes keyword", "yes", TOKEN_YES},
		{"no keyword", "no", TOKEN_NO},
		{"bool type", "bool", TOKEN_BOOL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := &Lexer{input: tt.input}
			token := lexer.NextToken()
			if token.Type != tt.expected {
				t.Errorf("Expected token type %v, got %v", tt.expected, token.Type)
			}
		})
	}
}

// TestBooleanTypeParsing tests that yes/no parse to BooleanExpr
func TestBooleanTypeParsing(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"parse yes", "a = yes", true},
		{"parse no", "b = no", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.code)
			program := parser.ParseProgram()
			if program == nil {
				t.Fatal("ParseProgram returned nil")
			}

			if len(program.Statements) == 0 {
				t.Fatal("No statements parsed")
			}

			assignStmt, ok := program.Statements[0].(*AssignStmt)
			if !ok {
				t.Fatalf("Expected AssignStmt, got %T", program.Statements[0])
			}

			boolExpr, ok := assignStmt.Value.(*BooleanExpr)
			if !ok {
				t.Fatalf("Expected BooleanExpr, got %T", assignStmt.Value)
			}

			if boolExpr.Value != tt.expected {
				t.Errorf("Expected boolean value %v, got %v", tt.expected, boolExpr.Value)
			}
		})
	}
}

// TestBooleanTypeCompilation tests that boolean code compiles without errors
func TestBooleanTypeCompilation(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"simple yes", "main = { a = yes\n }"},
		{"simple no", "main = { b = no\n }"},
		{"boolean in match", "main = { flag = yes\nflag { yes => 1 no => 0 } }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(tt.code)
			program := parser.ParseProgram()
			if program == nil {
				t.Fatal("ParseProgram returned nil")
			}

			// Just verify parsing succeeded
			if len(program.Statements) == 0 {
				t.Fatal("No statements parsed")
			}
		})
	}
}

// TestBooleanTypeInference tests that getExprType correctly identifies booleans
func TestBooleanTypeInference(t *testing.T) {
	platform := Platform{
		Arch: ArchX86_64,
		OS:   OSLinux,
	}

	compiler := &C67Compiler{
		platform: platform,
		varTypes: make(map[string]string),
	}

	// Check type inference
	yesExpr := &BooleanExpr{Value: true}
	noExpr := &BooleanExpr{Value: false}

	yesType := compiler.getExprType(yesExpr)
	noType := compiler.getExprType(noExpr)

	if yesType != "bool" {
		t.Errorf("Expected 'bool' type for yes, got '%s'", yesType)
	}

	if noType != "bool" {
		t.Errorf("Expected 'bool' type for no, got '%s'", noType)
	}
}

// TestBooleanString tests the String() method of BooleanExpr
func TestBooleanString(t *testing.T) {
	yes := &BooleanExpr{Value: true}
	no := &BooleanExpr{Value: false}

	if yes.String() != "yes" {
		t.Errorf("Expected 'yes', got '%s'", yes.String())
	}

	if no.String() != "no" {
		t.Errorf("Expected 'no', got '%s'", no.String())
	}
}









