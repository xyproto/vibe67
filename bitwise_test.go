package main

import (
	"testing"
)

func TestBitTestOperator(t *testing.T) {
	code := `
main = {
    x = 0b10110  // Binary 22 (bits: 1, 2, 4 are set)
    
    // Test each bit position
    bit0 = x ?b 0  // Should be 0 (bit 0 not set)
    bit1 = x ?b 1  // Should be 1 (bit 1 is set)
    bit2 = x ?b 2  // Should be 1 (bit 2 is set)
    bit3 = x ?b 3  // Should be 0 (bit 3 not set)
    bit4 = x ?b 4  // Should be 1 (bit 4 is set)
    bit5 = x ?b 5  // Should be 0 (bit 5 not set)
    
    // Verify results
    bit0 == 0 or exit(1)
    bit1 == 1 or exit(2)
    bit2 == 1 or exit(3)
    bit3 == 0 or exit(4)
    bit4 == 1 or exit(5)
    bit5 == 0 or exit(6)
}
`
	compileAndRun(t, code)
}

func TestBitTestWithVariablePosition(t *testing.T) {
	code := `
main = {
    value = 0b11111111  // All bits set in lower byte
    
    // Test with variable bit positions
    pos = 0
    result0 = value ?b pos
    result0 == 1 or exit(1)
    
    pos = 3
    result3 = value ?b pos
    result3 == 1 or exit(2)
    
    pos = 7
    result7 = value ?b pos
    result7 == 1 or exit(3)
    
    // Test with no bits set
    zero = 0
    pos = 5
    result_zero = zero ?b pos
    result_zero == 0 or exit(4)
}
`
	compileAndRun(t, code)
}

func TestBitTestInExpression(t *testing.T) {
	code := `
main = {
    flags = 0b1010  // Bits 1 and 3 set
    
    // Use bit test in conditional expressions
    has_bit1 = (flags ?b 1) == 1
    has_bit2 = (flags ?b 2) == 1
    
    has_bit1 or exit(1)
    not has_bit2 or exit(2)
    
    // Use in arithmetic
    count = (flags ?b 0) + (flags ?b 1) + (flags ?b 2) + (flags ?b 3)
    count == 2 or exit(3)  // Only bits 1 and 3 are set
}
`
	compileAndRun(t, code)
}
