package main

import (
	"testing"
)

// TestRegisterAllocatorBasic tests basic register allocation
func TestRegisterAllocatorBasic(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// Simulate three variables with non-overlapping lifetimes
	ra.BeginVariable("x")
	ra.AdvancePosition()
	ra.UseVariable("x")
	ra.AdvancePosition()
	ra.EndVariable("x")

	ra.AdvancePosition()
	ra.BeginVariable("y")
	ra.AdvancePosition()
	ra.UseVariable("y")
	ra.AdvancePosition()
	ra.EndVariable("y")

	ra.AdvancePosition()
	ra.BeginVariable("z")
	ra.AdvancePosition()
	ra.UseVariable("z")
	ra.AdvancePosition()
	ra.EndVariable("z")

	// Allocate registers
	ra.AllocateRegisters()

	// All three should get registers (non-overlapping lifetimes can reuse)
	xReg, xOk := ra.GetRegister("x")
	yReg, yOk := ra.GetRegister("y")
	zReg, zOk := ra.GetRegister("z")

	if !xOk || !yOk || !zOk {
		t.Errorf("Expected all variables to get registers, got x=%v, y=%v, z=%v", xOk, yOk, zOk)
	}

	// Since they don't overlap, they should all get the same register
	if xReg != yReg || yReg != zReg {
		t.Logf("Non-overlapping variables got different registers (this is OK, but inefficient)")
	}

	if ra.IsSpilled("x") || ra.IsSpilled("y") || ra.IsSpilled("z") {
		t.Errorf("Expected no spilling for non-overlapping variables")
	}
}

// TestRegisterAllocatorOverlapping tests overlapping variable lifetimes
func TestRegisterAllocatorOverlapping(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// Simulate three variables with overlapping lifetimes
	ra.BeginVariable("x")
	ra.AdvancePosition()

	ra.BeginVariable("y")
	ra.AdvancePosition()

	ra.BeginVariable("z")
	ra.AdvancePosition()

	// Use all three
	ra.UseVariable("x")
	ra.UseVariable("y")
	ra.UseVariable("z")
	ra.AdvancePosition()

	ra.EndVariable("x")
	ra.EndVariable("y")
	ra.EndVariable("z")

	// Allocate registers
	ra.AllocateRegisters()

	// All three should get different registers (overlapping lifetimes)
	xReg, xOk := ra.GetRegister("x")
	yReg, yOk := ra.GetRegister("y")
	zReg, zOk := ra.GetRegister("z")

	if !xOk || !yOk || !zOk {
		t.Errorf("Expected all variables to get registers")
	}

	if xReg == yReg || yReg == zReg || xReg == zReg {
		t.Errorf("Overlapping variables should get different registers, got x=%s, y=%s, z=%s", xReg, yReg, zReg)
	}
}

// TestRegisterAllocatorSpilling tests register spilling
func TestRegisterAllocatorSpilling(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// x86_64 has 5 callee-saved registers: rbx, r12, r13, r14, r15
	// Create 6 overlapping variables to force spilling
	varNames := []string{"a", "b", "c", "d", "e", "f"}

	for _, name := range varNames {
		ra.BeginVariable(name)
		ra.AdvancePosition()
	}

	// Use all variables (make them all overlap)
	for _, name := range varNames {
		ra.UseVariable(name)
	}
	ra.AdvancePosition()

	for _, name := range varNames {
		ra.EndVariable(name)
	}

	// Allocate registers
	ra.AllocateRegisters()

	// Count how many were spilled
	spillCount := 0
	for _, name := range varNames {
		if ra.IsSpilled(name) {
			spillCount++
		}
	}

	// Should have at least 1 spilled (since we have 6 vars but only 5 registers)
	if spillCount < 1 {
		t.Errorf("Expected at least 1 variable to be spilled, got %d", spillCount)
	}

	// Check that spilled variables have valid spill slots
	for _, name := range varNames {
		if ra.IsSpilled(name) {
			slot, ok := ra.GetSpillSlot(name)
			if !ok || slot < 0 {
				t.Errorf("Spilled variable %s should have valid spill slot, got %d, ok=%v", name, slot, ok)
			}
		}
	}

	// Check stack frame size
	frameSize := ra.GetStackFrameSize()
	expectedSize := spillCount * 8
	if frameSize != expectedSize {
		t.Errorf("Expected stack frame size %d, got %d", expectedSize, frameSize)
	}
}

// TestRegisterAllocatorARM64 tests ARM64 register allocation
func TestRegisterAllocatorARM64(t *testing.T) {
	ra := NewRegisterAllocator(ArchARM64)

	// ARM64 has 10 callee-saved registers: x19-x28
	// Create 5 overlapping variables
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.BeginVariable(name)
		ra.AdvancePosition()
	}

	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.UseVariable(name)
	}
	ra.AdvancePosition()

	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.EndVariable(name)
	}

	ra.AllocateRegisters()

	// All should get registers (5 vars, 10 available)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		_, ok := ra.GetRegister(name)
		if !ok {
			t.Errorf("Expected variable %s to get a register", name)
		}
		if ra.IsSpilled(name) {
			t.Errorf("Variable %s should not be spilled", name)
		}
	}
}

// TestRegisterAllocatorRISCV tests RISC-V register allocation
func TestRegisterAllocatorRISCV(t *testing.T) {
	ra := NewRegisterAllocator(ArchRiscv64)

	// RISC-V has 12 callee-saved registers: s0-s11
	// Create 5 overlapping variables
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.BeginVariable(name)
		ra.AdvancePosition()
	}

	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.UseVariable(name)
	}
	ra.AdvancePosition()

	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		ra.EndVariable(name)
	}

	ra.AllocateRegisters()

	// All should get registers (5 vars, 12 available)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		_, ok := ra.GetRegister(name)
		if !ok {
			t.Errorf("Expected variable %s to get a register", name)
		}
		if ra.IsSpilled(name) {
			t.Errorf("Variable %s should not be spilled", name)
		}
	}
}

// TestRegisterAllocatorReset tests that Reset properly clears state
func TestRegisterAllocatorReset(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// Allocate some variables
	ra.BeginVariable("x")
	ra.AdvancePosition()
	ra.UseVariable("x")
	ra.AdvancePosition()
	ra.AllocateRegisters()

	// Should have allocated a register
	if len(ra.intervals) == 0 {
		t.Error("Expected intervals to be non-empty before reset")
	}

	// Reset
	ra.Reset()

	// Should be cleared
	if len(ra.intervals) != 0 {
		t.Errorf("Expected intervals to be empty after reset, got %d", len(ra.intervals))
	}
	if len(ra.varToInterval) != 0 {
		t.Errorf("Expected varToInterval to be empty after reset, got %d", len(ra.varToInterval))
	}
	if ra.position != 0 {
		t.Errorf("Expected position to be 0 after reset, got %d", ra.position)
	}
	if ra.spillSlots != 0 {
		t.Errorf("Expected spillSlots to be 0 after reset, got %d", ra.spillSlots)
	}
}

// TestRegisterAllocatorLiveIntervals tests live interval computation
func TestRegisterAllocatorLiveIntervals(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// Variable x: lives from position 0 to 5
	ra.BeginVariable("x") // position 0
	ra.AdvancePosition()  // position 1
	ra.UseVariable("x")   // extend to position 1
	ra.AdvancePosition()  // position 2
	ra.AdvancePosition()  // position 3
	ra.UseVariable("x")   // extend to position 3
	ra.AdvancePosition()  // position 4
	ra.AdvancePosition()  // position 5
	ra.EndVariable("x")   // end at position 5

	interval := ra.varToInterval["x"]
	if interval.Start != 0 {
		t.Errorf("Expected interval start to be 0, got %d", interval.Start)
	}
	if interval.End != 5 {
		t.Errorf("Expected interval end to be 5, got %d", interval.End)
	}
}

// TestRegisterAllocatorUsedCalleeSaved tests tracking of used callee-saved registers
func TestRegisterAllocatorUsedCalleeSaved(t *testing.T) {
	ra := NewRegisterAllocator(ArchX86_64)

	// Create 2 variables
	ra.BeginVariable("x")
	ra.AdvancePosition()
	ra.BeginVariable("y")
	ra.AdvancePosition()

	ra.UseVariable("x")
	ra.UseVariable("y")
	ra.AdvancePosition()

	ra.EndVariable("x")
	ra.EndVariable("y")

	ra.AllocateRegisters()

	// Should have 2 used callee-saved registers
	usedRegs := ra.GetUsedCalleeSaved()
	if len(usedRegs) != 2 {
		t.Errorf("Expected 2 used callee-saved registers, got %d: %v", len(usedRegs), usedRegs)
	}

	// All used registers should be callee-saved
	for _, reg := range usedRegs {
		found := false
		for _, calleeSaved := range ra.calleeSaved {
			if reg == calleeSaved {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Register %s is not in callee-saved list", reg)
		}
	}
}









