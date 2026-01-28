// Completion: 10% - SIMD analysis module in progress
package main

import (
	"fmt"
	"os"
)

// LoopVectorizationInfo contains analysis results for a loop
type LoopVectorizationInfo struct {
	CanVectorize    bool     // Whether loop can be auto-vectorized
	VectorWidth     int      // Target vector width in elements
	Reason          string   // Reason for vectorization decision
	Iterator        string   // Loop iterator variable
	TripCount       int64    // Number of iterations (if known)
	HasDependencies bool     // True if loop carries dependencies
	MemoryAccesses  []string // Variables accessed in loop
	Operations      []string // Operations performed in loop
	IsParallel      bool     // True if iterations are independent
}

// SIMDAnalyzer analyzes loops for vectorization opportunities
type SIMDAnalyzer struct {
	target      Target                  // Target platform for vector width
	depAnalyzer *LoopDependencyAnalyzer // Dependency analyzer
}

// NewSIMDAnalyzer creates a new SIMD analyzer
func NewSIMDAnalyzer(target Target) *SIMDAnalyzer {
	return &SIMDAnalyzer{
		target:      target,
		depAnalyzer: NewLoopDependencyAnalyzer(),
	}
}

// AnalyzeLoop analyzes a loop statement for vectorization potential
func (sa *SIMDAnalyzer) AnalyzeLoop(loop *LoopStmt) *LoopVectorizationInfo {
	info := &LoopVectorizationInfo{
		CanVectorize:    false,
		VectorWidth:     sa.getVectorWidth(),
		Reason:          "",
		Iterator:        loop.Iterator,
		TripCount:       -1,
		HasDependencies: false,
		MemoryAccesses:  []string{},
		Operations:      []string{},
		IsParallel:      loop.NumThreads != 0,
	}

	// If loop is already marked parallel, it's a good vectorization candidate
	if loop.NumThreads != 0 {
		info.CanVectorize = true
		info.Reason = "Loop already marked as parallel"
		return info
	}

	// Analyze loop body for vectorization potential
	info.MemoryAccesses = sa.findMemoryAccesses(loop.Body)
	info.Operations = sa.findOperations(loop.Body)

	// Use sophisticated dependency analysis
	canVectorize, reason := sa.depAnalyzer.CanVectorize(loop)
	info.HasDependencies = !canVectorize

	if !canVectorize {
		info.CanVectorize = false
		info.Reason = reason
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Cannot vectorize - %s\n", reason)
		}
		return info
	}

	// Check if operations are SIMD-friendly
	if sa.hasVectorizableOperations(info.Operations) {
		info.CanVectorize = true
		info.Reason = "Loop has SIMD-friendly operations and no dependencies"
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD: Loop CAN be vectorized - %s\n", info.Reason)
		}
		return info
	}

	info.CanVectorize = false
	info.Reason = "Loop operations are not SIMD-friendly"
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD: Loop cannot be vectorized - %s (ops=%v)\n", info.Reason, info.Operations)
	}
	return info
}

// getVectorWidth returns the vector width for the target platform
func (sa *SIMDAnalyzer) getVectorWidth() int {
	if targetImpl, ok := sa.target.(*TargetImpl); ok {
		return targetImpl.GetVectorLaneCount()
	}
	return 2 // Default conservative estimate
}

// findMemoryAccesses finds all memory accesses in loop body
func (sa *SIMDAnalyzer) findMemoryAccesses(body []Statement) []string {
	accesses := []string{}

	for _, stmt := range body {
		// For now, collect variable names from assignments
		if assign, ok := stmt.(*AssignStmt); ok {
			accesses = append(accesses, assign.Name)
		}
	}

	return accesses
}

// findOperations finds all operations in loop body
func (sa *SIMDAnalyzer) findOperations(body []Statement) []string {
	ops := []string{}

	for _, stmt := range body {
		if assign, ok := stmt.(*AssignStmt); ok {
			// Analyze the expression to find operations
			ops = append(ops, sa.findExprOps(assign.Value)...)
		} else if mapUpdate, ok := stmt.(*MapUpdateStmt); ok {
			// Analyze the value expression for map/array updates
			ops = append(ops, sa.findExprOps(mapUpdate.Value)...)
		}
	}

	return ops
}

// findExprOps recursively finds operations in an expression
func (sa *SIMDAnalyzer) findExprOps(expr Expression) []string {
	ops := []string{}

	switch e := expr.(type) {
	case *BinaryExpr:
		ops = append(ops, e.Operator)
		ops = append(ops, sa.findExprOps(e.Left)...)
		ops = append(ops, sa.findExprOps(e.Right)...)
	case *UnaryExpr:
		ops = append(ops, e.Operator)
		ops = append(ops, sa.findExprOps(e.Operand)...)
	case *CallExpr:
		ops = append(ops, "call:"+e.Function)
	}

	return ops
}

// hasVectorizableOperations checks if operations can be vectorized
func (sa *SIMDAnalyzer) hasVectorizableOperations(ops []string) bool {
	vectorizableOps := map[string]bool{
		"+": true, "-": true, "*": true, "/": true,
		"<": true, ">": true, "<=": true, ">=": true, "==": true, "!=": true,
		"call:sqrt": true, "call:abs": true, "call:min": true, "call:max": true,
	}

	// Need at least one vectorizable operation
	for _, op := range ops {
		if vectorizableOps[op] {
			return true
		}
	}

	return false
}

// PrintAnalysis prints loop analysis for debugging
func (info *LoopVectorizationInfo) Print() {
	fmt.Println("=== Loop Vectorization Analysis ===")
	fmt.Printf("Iterator: %s\n", info.Iterator)
	fmt.Printf("Can Vectorize: %v\n", info.CanVectorize)
	fmt.Printf("Reason: %s\n", info.Reason)
	fmt.Printf("Vector Width: %d elements\n", info.VectorWidth)
	fmt.Printf("Is Parallel: %v\n", info.IsParallel)
	fmt.Printf("Has Dependencies: %v\n", info.HasDependencies)
	if len(info.MemoryAccesses) > 0 {
		fmt.Printf("Memory Accesses: %v\n", info.MemoryAccesses)
	}
	if len(info.Operations) > 0 {
		fmt.Printf("Operations: %v\n", info.Operations)
	}
}
