// Completion: 100% - Utility module complete
package main

import "fmt"

// Architecture defines the interface for different CPU architectures
type Architecture interface {
	// Instruction generation
	MovImmediate(w Writer, dest, val string) error
	Syscall(w Writer) error

	// Register validation
	IsValidRegister(reg string) bool

	// ELF header information
	ELFMachineType() uint16

	// Architecture identification
	Name() string
}

// X86_64 implements Architecture for x86_64
type X86_64 struct{}

// ARM64 implements Architecture for aarch64
type ARM64 struct{}

// Riscv64 implements Architecture for riscv64
type Riscv64 struct{}

// NewArchitecture creates the appropriate architecture implementation
func NewArchitecture(machine string) (Architecture, error) {
	switch machine {
	case "x86_64":
		return &X86_64{}, nil
	case "aarch64":
		return &ARM64{}, nil
	case "riscv64":
		return &Riscv64{}, nil
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", machine)
	}
}









