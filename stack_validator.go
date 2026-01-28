// stack_validator.go - Track stack operations to detect corruption
package main

import (
	"fmt"
	"os"
)

// StackValidator tracks push/pop operations to ensure balanced stack
type StackValidator struct {
	depth      int      // Current stack depth (in 8-byte words)
	operations []string // History of operations for debugging
	enabled    bool     // Can be disabled for performance
}

func NewStackValidator() *StackValidator {
	return &StackValidator{
		depth:      0,
		operations: make([]string, 0, 100),
		enabled:    true,
	}
}

func (sv *StackValidator) Push(reg string) {
	if !sv.enabled {
		return
	}
	sv.depth++
	sv.operations = append(sv.operations, fmt.Sprintf("push %s (depth=%d)", reg, sv.depth))
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "STACK: push %s, depth now %d\n", reg, sv.depth)
	}
}

func (sv *StackValidator) Pop(reg string) {
	if !sv.enabled {
		return
	}
	if sv.depth <= 0 {
		fmt.Fprintf(os.Stderr, "ERROR: Stack underflow! Attempted pop %s with depth %d\n", reg, sv.depth)
		fmt.Fprintf(os.Stderr, "Recent operations:\n")
		start := len(sv.operations) - 10
		if start < 0 {
			start = 0
		}
		for i := start; i < len(sv.operations); i++ {
			fmt.Fprintf(os.Stderr, "  %s\n", sv.operations[i])
		}
		panic("Stack underflow detected")
	}
	sv.depth--
	sv.operations = append(sv.operations, fmt.Sprintf("pop %s (depth=%d)", reg, sv.depth))
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "STACK: pop %s, depth now %d\n", reg, sv.depth)
	}
}

func (sv *StackValidator) Sub(amount int) {
	if !sv.enabled {
		return
	}
	words := amount / 8
	sv.depth += words
	sv.operations = append(sv.operations, fmt.Sprintf("sub rsp, %d (depth=%d)", amount, sv.depth))
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "STACK: sub rsp, %d, depth now %d\n", amount, sv.depth)
	}
}

func (sv *StackValidator) Add(amount int) {
	if !sv.enabled {
		return
	}
	words := amount / 8
	if sv.depth < words {
		fmt.Fprintf(os.Stderr, "ERROR: Stack imbalance! Attempted add rsp, %d with depth %d\n", amount, sv.depth)
		panic("Stack imbalance detected")
	}
	sv.depth -= words
	sv.operations = append(sv.operations, fmt.Sprintf("add rsp, %d (depth=%d)", amount, sv.depth))
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "STACK: add rsp, %d, depth now %d\n", amount, sv.depth)
	}
}

func (sv *StackValidator) Checkpoint(label string) int {
	if !sv.enabled {
		return 0
	}
	sv.operations = append(sv.operations, fmt.Sprintf("checkpoint %s (depth=%d)", label, sv.depth))
	return sv.depth
}

func (sv *StackValidator) Validate(checkpointDepth int, label string) {
	if !sv.enabled {
		return
	}
	if sv.depth != checkpointDepth {
		fmt.Fprintf(os.Stderr, "ERROR: Stack imbalance at %s! Expected depth %d, got %d\n", label, checkpointDepth, sv.depth)
		fmt.Fprintf(os.Stderr, "Recent operations:\n")
		start := len(sv.operations) - 20
		if start < 0 {
			start = 0
		}
		for i := start; i < len(sv.operations); i++ {
			fmt.Fprintf(os.Stderr, "  %s\n", sv.operations[i])
		}
		panic(fmt.Sprintf("Stack imbalance at %s", label))
	}
}

func (sv *StackValidator) Reset() {
	sv.depth = 0
	sv.operations = sv.operations[:0]
}
