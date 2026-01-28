// compiler_state.go - Central state management for compilation
package main

import (
	"fmt"
	"os"
)

// CompilerState manages the overall compilation state and coordinates between components
type CompilerState struct {
	// Configuration
	target  Target
	options CompileOptions

	// Writers (one will be active based on target)
	elfWriter *ELFWriter
	peWriter  *PEWriter

	// Trackers
	regTracker   *RegisterTracker
	stackTracker *StackValidator

	// Pipeline
	pipeline *CompilationPipeline

	// Core builder
	builder *ExecutableBuilder

	// Current phase
	phase CompilationPhase
}

type CompileOptions struct {
	outputPath string
	verbose    bool
	optimize   bool
	targetArch Arch // Use existing Arch type
	targetOS   OS   // Use existing OS type
}

// NewCompilerState creates a new compiler state with all components initialized
func NewCompilerState(target Target, options CompileOptions, builder *ExecutableBuilder, isDynamic bool) *CompilerState {
	cs := &CompilerState{
		target:       target,
		options:      options,
		regTracker:   NewRegisterTracker(),
		stackTracker: NewStackValidator(),
		pipeline:     NewCompilationPipeline(),
		builder:      builder,
		phase:        PhaseInitial,
	}

	// Initialize appropriate writer based on target OS
	switch target.OS() {
	case OSLinux:
		// Determine if dynamic linking is needed
		cs.elfWriter = NewELFWriter(target, builder, isDynamic)
	case OSWindows:
		cs.peWriter = NewPEWriter(target, builder)
	}

	// No need to register stages - they're predefined in compilation_pipeline.go

	return cs
}

// GetBaseAddr returns the virtual base address from the appropriate writer
func (cs *CompilerState) GetBaseAddr() uint64 {
	if cs.elfWriter != nil {
		return cs.elfWriter.GetBaseAddr()
	}
	if cs.peWriter != nil {
		return cs.peWriter.GetBaseAddr()
	}
	return 0x400000 // Default fallback
}

// GetEstimatedRodataAddr returns estimated rodata address for first pass
func (cs *CompilerState) GetEstimatedRodataAddr() uint64 {
	if cs.elfWriter != nil {
		return cs.elfWriter.GetEstimatedRodataAddr()
	}
	if cs.peWriter != nil {
		return cs.peWriter.GetEstimatedRodataAddr()
	}
	return cs.GetBaseAddr() + 0x3100 // Fallback
}

// TransitionPhase transitions to a new compilation phase with validation
func (cs *CompilerState) TransitionPhase(newPhase CompilationPhase) error {
	// Simple phase transition for now - full validation can come later
	cs.phase = newPhase

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "=== Phase Transition: %v ===\n", newPhase)
	}

	return nil
}

// CurrentPhase returns the current compilation phase
func (cs *CompilerState) CurrentPhase() CompilationPhase {
	return cs.phase
}

// Validate performs comprehensive validation of compiler state
func (cs *CompilerState) Validate() error {
	// Simplified validation - can be expanded later
	return nil
}

// GetSummary returns a summary of the current compiler state
func (cs *CompilerState) GetSummary() string {
	return fmt.Sprintf(
		"CompilerState:\n"+
			"  Phase: %v\n"+
			"  BaseAddr: 0x%x\n",
		cs.phase,
		cs.GetBaseAddr(),
	)
}
