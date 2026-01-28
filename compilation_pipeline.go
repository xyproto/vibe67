// compilation_pipeline.go - Explicit compilation stages with validation
package main

import (
	"fmt"
	"os"
)

// CompilationStage represents a stage in the compilation pipeline (legacy)
type CompilationStage int

// CompilationPhase is the new name for CompilationStage
type CompilationPhase = CompilationStage

const (
	StageInit CompilationStage = iota
	StageFirstPassSymbolCollection
	StageFirstPassCodeGen
	StageFirstPassAddressAssignment
	StageELFStructureGeneration
	StageSecondPassSymbolCollection
	StageSecondPassCodeGen
	StageRuntimeHelperGeneration
	StagePCRelocationPatching
	StageELFFinalization
	StageComplete

	// New aliases for clearer names
	PhaseInitial        = StageInit
	PhaseParsing        = StageFirstPassSymbolCollection
	PhaseCodegenInitial = StageFirstPassCodeGen
	PhaseELFLayout      = StageFirstPassAddressAssignment
	PhaseCodegenFinal   = StageSecondPassCodeGen
	PhasePatching       = StagePCRelocationPatching
	PhaseWriting        = StageELFFinalization
	PhaseComplete       = StageComplete
)

func (s CompilationStage) String() string {
	switch s {
	case StageInit:
		return "Initialization"
	case StageFirstPassSymbolCollection:
		return "First Pass: Symbol Collection"
	case StageFirstPassCodeGen:
		return "First Pass: Code Generation"
	case StageFirstPassAddressAssignment:
		return "First Pass: Address Assignment"
	case StageELFStructureGeneration:
		return "ELF Structure Generation"
	case StageSecondPassSymbolCollection:
		return "Second Pass: Symbol Collection"
	case StageSecondPassCodeGen:
		return "Second Pass: Code Generation"
	case StageRuntimeHelperGeneration:
		return "Runtime Helper Generation"
	case StagePCRelocationPatching:
		return "PC Relocation Patching"
	case StageELFFinalization:
		return "ELF Finalization"
	case StageComplete:
		return "Compilation Complete"
	default:
		return fmt.Sprintf("Unknown Stage %d", s)
	}
}

// CompilationPipeline tracks the current stage and validates state transitions
type CompilationPipeline struct {
	currentStage CompilationStage
	stages       []CompilationStage // History of stages
	enabled      bool               // Can be disabled for performance
}

func NewCompilationPipeline() *CompilationPipeline {
	return &CompilationPipeline{
		currentStage: StageInit,
		stages:       []CompilationStage{StageInit},
		enabled:      true,
	}
}

func (cp *CompilationPipeline) AdvanceTo(stage CompilationStage) {
	if !cp.enabled {
		cp.currentStage = stage
		return
	}

	// Validate stage transitions
	validTransition := false
	switch cp.currentStage {
	case StageInit:
		validTransition = (stage == StageFirstPassSymbolCollection)
	case StageFirstPassSymbolCollection:
		validTransition = (stage == StageFirstPassCodeGen)
	case StageFirstPassCodeGen:
		validTransition = (stage == StageFirstPassAddressAssignment)
	case StageFirstPassAddressAssignment:
		validTransition = (stage == StageELFStructureGeneration)
	case StageELFStructureGeneration:
		validTransition = (stage == StageSecondPassSymbolCollection)
	case StageSecondPassSymbolCollection:
		validTransition = (stage == StageSecondPassCodeGen)
	case StageSecondPassCodeGen:
		validTransition = (stage == StageRuntimeHelperGeneration)
	case StageRuntimeHelperGeneration:
		validTransition = (stage == StagePCRelocationPatching)
	case StagePCRelocationPatching:
		validTransition = (stage == StageELFFinalization)
	case StageELFFinalization:
		validTransition = (stage == StageComplete)
	case StageComplete:
		validTransition = false // Can't advance from complete
	}

	if !validTransition {
		fmt.Fprintf(os.Stderr, "ERROR: Invalid stage transition: %s -> %s\n", cp.currentStage, stage)
		fmt.Fprintf(os.Stderr, "Stage history:\n")
		for i, s := range cp.stages {
			fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, s)
		}
		panic(fmt.Sprintf("Invalid compilation stage transition: %s -> %s", cp.currentStage, stage))
	}

	cp.currentStage = stage
	cp.stages = append(cp.stages, stage)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PIPELINE: Advanced to stage: %s\n", stage)
	}
}

func (cp *CompilationPipeline) CurrentStage() CompilationStage {
	return cp.currentStage
}

func (cp *CompilationPipeline) ValidateStage(expected CompilationStage, operation string) {
	if !cp.enabled {
		return
	}

	if cp.currentStage != expected {
		fmt.Fprintf(os.Stderr, "ERROR: Attempted '%s' at wrong stage\n", operation)
		fmt.Fprintf(os.Stderr, "  Expected: %s\n", expected)
		fmt.Fprintf(os.Stderr, "  Actual: %s\n", cp.currentStage)
		panic(fmt.Sprintf("Invalid operation '%s' at stage %s", operation, cp.currentStage))
	}
}

// Checkpoint creates a named checkpoint for debugging
func (cp *CompilationPipeline) Checkpoint(name string) {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "PIPELINE CHECKPOINT: %s at stage %s\n", name, cp.currentStage)
	}
}
