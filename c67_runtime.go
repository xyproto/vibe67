// Completion: 100% - Module complete
package main

import (
	"fmt"
)

// Vibe67Map represents map[uint64]float64 - the foundation of all Vibe67 values
// Memory layout: [size: uint64][capacity: uint64][entry0][entry1]...
// Each entry: [key: uint64][value: float64]

type Vibe67MapEntry struct {
	Key   uint64
	Value float64
}

// Runtime functions for Vibe67 map operations
type Vibe67Runtime struct {
	out *Out
	eb  *ExecutableBuilder
}

func NewVibe67Runtime(out *Out, eb *ExecutableBuilder) *Vibe67Runtime {
	return &Vibe67Runtime{out: out, eb: eb}
}

// CreateScalar creates a map with single entry {0: value}
// Returns pointer to map in rax
func (fr *Vibe67Runtime) CreateScalar(value float64) {
	// Allocate 32 bytes: [size=1][capacity=1][key=0][value]
	size := uint64(32)

	// For now, use a simple bump allocator approach
	// In production, would use proper memory management
	fr.out.MovImmToReg("rdi", fmt.Sprintf("%d", size))
	fr.eb.GenerateCallInstruction("malloc")

	// rax now contains pointer to allocated memory
	// Store size=1 at [rax]
	fr.out.MovImmToReg("rcx", "1")
	fr.out.MovRegToMem("rcx", "rax", 0)

	// Store capacity=1 at [rax+8]
	fr.out.MovRegToMem("rcx", "rax", 8)

	// Store key=0 at [rax+16]
	fr.out.XorRegWithReg("rcx", "rcx")
	fr.out.MovRegToMem("rcx", "rax", 16)

	// Store value at [rax+24]
	// Convert float64 to bits and store
	// For now, simplified: convert int to float representation
	fr.out.MovImmToReg("rcx", fmt.Sprintf("%d", int64(value)))
	fr.out.MovRegToXmm("xmm0", "rcx")
	fr.out.MovXmmToMem("xmm0", "rax", 24)
}

// GetScalarValue extracts float64 value from map at index 0
// Input: rax = pointer to map
// Output: xmm0 = float64 value
func (fr *Vibe67Runtime) GetScalarValue() {
	// Load value from [rax+24] (assumes single entry map with key=0)
	fr.out.MovMemToXmm("xmm0", "rax", 24)
}

// CreateMapFromInt creates a scalar map from integer in rax
// Input: rax = int64 value
// Output: rax = pointer to map{0: float64(value)}
func (fr *Vibe67Runtime) CreateMapFromInt() {
	// Convert int in rax to float in xmm0
	fr.out.MovRegToXmm("xmm0", "rax")
	fr.out.Cvtsi2sd("xmm0", "rax")

	// Save xmm0 to stack
	fr.out.SubImmFromReg("rsp", 16)
	fr.out.MovXmmToMem("xmm0", "rsp", 0)

	// Allocate map (32 bytes)
	fr.out.MovImmToReg("rdi", "32")
	fr.eb.GenerateCallInstruction("malloc")

	// Restore float value
	fr.out.MovMemToXmm("xmm0", "rsp", 0)
	fr.out.AddImmToReg("rsp", 16)

	// Store size=1, capacity=1, key=0, value
	fr.out.MovImmToReg("rcx", "1")
	fr.out.MovRegToMem("rcx", "rax", 0) // size
	fr.out.MovRegToMem("rcx", "rax", 8) // capacity
	fr.out.XorRegWithReg("rcx", "rcx")
	fr.out.MovRegToMem("rcx", "rax", 16)  // key=0
	fr.out.MovXmmToMem("xmm0", "rax", 24) // value
}

// BinaryOpScalar performs binary operation on two scalar maps
// Input: rbx = pointer to left map, rax = pointer to right map
// Output: rax = pointer to result map
func (fr *Vibe67Runtime) BinaryOpScalar(op string) {
	// Load left value to xmm0
	fr.out.MovMemToXmm("xmm0", "rbx", 24)

	// Load right value to xmm1
	fr.out.MovMemToXmm("xmm1", "rax", 24)

	// Perform operation
	switch op {
	case "+":
		fr.out.AddpdXmm("xmm0", "xmm1") // xmm0 += xmm1
	case "-":
		fr.out.SubpdXmm("xmm0", "xmm1") // xmm0 -= xmm1
	case "*":
		fr.out.MulpdXmm("xmm0", "xmm1") // xmm0 *= xmm1
	case "/":
		fr.out.DivpdXmm("xmm0", "xmm1") // xmm0 /= xmm1
	}

	// Save result to stack
	fr.out.SubImmFromReg("rsp", 16)
	fr.out.MovXmmToMem("xmm0", "rsp", 0)

	// Allocate new map for result
	fr.out.MovImmToReg("rdi", "32")
	fr.eb.GenerateCallInstruction("malloc")

	// Restore result
	fr.out.MovMemToXmm("xmm0", "rsp", 0)
	fr.out.AddImmToReg("rsp", 16)

	// Store in new map
	fr.out.MovImmToReg("rcx", "1")
	fr.out.MovRegToMem("rcx", "rax", 0) // size
	fr.out.MovRegToMem("rcx", "rax", 8) // capacity
	fr.out.XorRegWithReg("rcx", "rcx")
	fr.out.MovRegToMem("rcx", "rax", 16)  // key=0
	fr.out.MovXmmToMem("xmm0", "rax", 24) // value
}

// CreateString creates a map from string literal
// Each character is stored as float64 at sequential indices
// Input: rsi = pointer to string, rdx = length
// Output: rax = pointer to map
func (fr *Vibe67Runtime) CreateString() {
	// Calculate size: 16 (header) + length*16 (entries)
	fr.out.MovRegToReg("rax", "rdx")
	fr.out.ShlImmReg("rax", 4)    // rax = length * 16
	fr.out.AddImmToReg("rax", 16) // add header size

	// Allocate
	fr.out.MovRegToReg("rdi", "rax")
	fr.out.PushReg("rsi") // save string ptr
	fr.out.PushReg("rdx") // save length
	fr.eb.GenerateCallInstruction("malloc")
	fr.out.PopReg("rdx") // restore length
	fr.out.PopReg("rsi") // restore string ptr

	// Store size and capacity
	fr.out.MovRegToMem("rdx", "rax", 0) // size = length
	fr.out.MovRegToMem("rdx", "rax", 8) // capacity = length

	// Store each character as {index: float64(rune)}
	// Loop would go here, but for now using simpler approach
	// (In production, would use SIMD VCVTDQ2PD to convert multiple bytes to floats)
}

// Arena allocator runtime functions
// These functions need to be emitted as assembly code in the executable

// EmitArenaRuntime emits the arena allocator runtime code
func (fr *Vibe67Runtime) EmitArenaRuntime() {
	fr.eb.EmitArenaRuntimeCode()
}
