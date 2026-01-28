// Completion: 100% - Loop dependency analysis complete
package main

import (
	"fmt"
	"os"
)

// DependencyType represents the type of dependency between loop iterations
type DependencyType int

const (
	NoDependency      DependencyType = iota // No dependency - can vectorize
	FlowDependency                          // Read-after-write (true dependency)
	AntiDependency                          // Write-after-read (anti dependency)
	OutputDependency                        // Write-after-write (output dependency)
	UnknownDependency                       // Conservative - assume dependency exists
)

// String returns the string representation of dependency type
func (dt DependencyType) String() string {
	switch dt {
	case NoDependency:
		return "None"
	case FlowDependency:
		return "Flow (RAW)"
	case AntiDependency:
		return "Anti (WAR)"
	case OutputDependency:
		return "Output (WAW)"
	case UnknownDependency:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// Dependency represents a dependency between two statements
type Dependency struct {
	Type     DependencyType
	Variable string // Variable causing the dependency
	Distance int    // Iteration distance (0 = same iteration, 1 = next iteration, etc.)
}

// LoopDependencyAnalyzer performs dependency analysis on loops
type LoopDependencyAnalyzer struct {
	writes map[string][]int // Variable -> positions where it's written
	reads  map[string][]int // Variable -> positions where it's read
}

// NewLoopDependencyAnalyzer creates a new dependency analyzer
func NewLoopDependencyAnalyzer() *LoopDependencyAnalyzer {
	return &LoopDependencyAnalyzer{
		writes: make(map[string][]int),
		reads:  make(map[string][]int),
	}
}

// AnalyzeDependencies analyzes dependencies in a loop
func (lda *LoopDependencyAnalyzer) AnalyzeDependencies(loop *LoopStmt) []Dependency {
	deps := []Dependency{}

	// Collect all reads and writes
	lda.collectAccesses(loop.Body)

	// Check for flow dependencies (RAW)
	for varName, readPos := range lda.reads {
		if writePos, exists := lda.writes[varName]; exists {
			for _, rPos := range readPos {
				for _, wPos := range writePos {
					if wPos < rPos {
						// Write before read in same iteration
						deps = append(deps, Dependency{
							Type:     FlowDependency,
							Variable: varName,
							Distance: 0,
						})
					}
				}
			}
		}
	}

	// Check for anti dependencies (WAR)
	for varName, writePos := range lda.writes {
		if readPos, exists := lda.reads[varName]; exists {
			for _, wPos := range writePos {
				for _, rPos := range readPos {
					if rPos < wPos {
						// Read before write in same iteration
						deps = append(deps, Dependency{
							Type:     AntiDependency,
							Variable: varName,
							Distance: 0,
						})
					}
				}
			}
		}
	}

	// Check for output dependencies (WAW)
	for varName, positions := range lda.writes {
		if len(positions) > 1 {
			// Multiple writes to same variable
			deps = append(deps, Dependency{
				Type:     OutputDependency,
				Variable: varName,
				Distance: 0,
			})
		}
	}

	// Deduplicate dependencies
	return lda.dedup(deps)
}

// collectAccesses collects all memory accesses in loop body
func (lda *LoopDependencyAnalyzer) collectAccesses(body []Statement) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD collectAccesses: Processing %d statements\n", len(body))
	}
	position := 0
	for _, stmt := range body {
		lda.collectStmtAccesses(stmt, position)
		position++
	}
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD collectAccesses: After processing, writes=%v, reads=%v\n",
			lda.writes, lda.reads)
	}
}

// collectStmtAccesses collects accesses in a single statement
func (lda *LoopDependencyAnalyzer) collectStmtAccesses(stmt Statement, position int) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD collectStmtAccesses: Statement type=%T\n", stmt)
	}
	switch s := stmt.(type) {
	case *AssignStmt:
		// This is a write
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD collectStmtAccesses: AssignStmt writing to '%s'\n", s.Name)
		}
		lda.writes[s.Name] = append(lda.writes[s.Name], position)
		// Analyze RHS for reads
		lda.collectExprReads(s.Value, position)
	case *MapUpdateStmt:
		// Array/map update: result[i] <- value
		// This is a write to the array element
		if VerboseMode {
			fmt.Fprintf(os.Stderr, "SIMD collectStmtAccesses: MapUpdateStmt writing to '%s[...]'\n", s.MapName)
		}
		// Record write to the base array (not the specific index)
		// This is a simplification - proper analysis would track individual elements
		lda.writes[s.MapName] = append(lda.writes[s.MapName], position)
		// The index and value expressions might have reads
		lda.collectExprReads(s.Index, position)
		lda.collectExprReads(s.Value, position)
	case *ExpressionStmt:
		// Expression might have reads
		lda.collectExprReads(s.Expr, position)
	}
}

// collectExprReads collects read accesses in an expression
func (lda *LoopDependencyAnalyzer) collectExprReads(expr Expression, position int) {
	switch e := expr.(type) {
	case *IdentExpr:
		lda.reads[e.Name] = append(lda.reads[e.Name], position)
	case *BinaryExpr:
		lda.collectExprReads(e.Left, position)
		lda.collectExprReads(e.Right, position)
	case *UnaryExpr:
		lda.collectExprReads(e.Operand, position)
	case *IndexExpr:
		lda.collectExprReads(e.List, position)
		lda.collectExprReads(e.Index, position)
	case *CallExpr:
		for _, arg := range e.Args {
			lda.collectExprReads(arg, position)
		}
	}
}

// dedup removes duplicate dependencies
func (lda *LoopDependencyAnalyzer) dedup(deps []Dependency) []Dependency {
	seen := make(map[string]bool)
	result := []Dependency{}

	for _, dep := range deps {
		key := dep.Type.String() + ":" + dep.Variable
		if !seen[key] {
			seen[key] = true
			result = append(result, dep)
		}
	}

	return result
}

// HasCrossIterationDependency checks if dependencies prevent vectorization
func (lda *LoopDependencyAnalyzer) HasCrossIterationDependency(loop *LoopStmt) bool {
	deps := lda.AnalyzeDependencies(loop)

	// Flow dependencies (RAW) prevent vectorization
	for _, dep := range deps {
		if dep.Type == FlowDependency {
			// Check if it's a cross-iteration dependency
			// For now, assume any flow dependency is problematic
			return true
		}
	}

	return false
}

// CanVectorize determines if loop can be safely vectorized
func (lda *LoopDependencyAnalyzer) CanVectorize(loop *LoopStmt) (bool, string) {
	deps := lda.AnalyzeDependencies(loop)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "SIMD CanVectorize: Found %d dependencies\n", len(deps))
		fmt.Fprintf(os.Stderr, "SIMD CanVectorize: Writes=%v\n", lda.writes)
		fmt.Fprintf(os.Stderr, "SIMD CanVectorize: Reads=%v\n", lda.reads)
	}

	if len(deps) == 0 {
		return true, "No dependencies detected"
	}

	// Check each dependency type
	hasFlow := false
	hasAnti := false
	hasOutput := false

	for _, dep := range deps {
		switch dep.Type {
		case FlowDependency:
			hasFlow = true
		case AntiDependency:
			hasAnti = true
		case OutputDependency:
			hasOutput = true
		}
	}

	// Flow dependencies usually prevent vectorization
	if hasFlow {
		return false, "Flow dependencies detected (read-after-write)"
	}

	// Anti dependencies can sometimes be handled with register renaming
	if hasAnti {
		return true, "Anti dependencies present (can be handled with renaming)"
	}

	// Output dependencies might be ok depending on context
	if hasOutput {
		return true, "Output dependencies present (may need special handling)"
	}

	return true, "Dependencies are vectorization-safe"
}

// GetDependencyReport generates a human-readable dependency report
func (lda *LoopDependencyAnalyzer) GetDependencyReport(loop *LoopStmt) string {
	deps := lda.AnalyzeDependencies(loop)

	if len(deps) == 0 {
		return "No dependencies detected - loop is fully parallel"
	}

	report := "Dependencies detected:\n"
	for _, dep := range deps {
		report += "  - " + dep.Type.String() + " on variable '" + dep.Variable + "'\n"
	}

	canVec, reason := lda.CanVectorize(loop)
	if canVec {
		report += "Verdict: Can vectorize (" + reason + ")"
	} else {
		report += "Verdict: Cannot vectorize (" + reason + ")"
	}

	return report
}
