// Completion: 100% - Helper module complete
package main

import (
	"fmt"
)

// RegisterTracker manages register allocation and prevents clobbering
// It tracks which registers are currently in use and provides safe allocation/deallocation
type RegisterTracker struct {
	// XMM register availability (xmm0-xmm15)
	xmmInUse   [16]bool
	xmmPurpose [16]string // What each register is being used for (debugging)

	// Integer register availability
	// rax, rcx, rdx, rsi, rdi, r8-r15
	intInUse   map[string]bool
	intPurpose map[string]string

	// Stack for nested register usage
	xmmStack []int // Stack of allocated XMM registers
	intStack []string

	// Reserved registers (never allocated automatically)
	xmmReserved [16]bool
	intReserved map[string]bool

	// Statistics
	maxXmmUsed int
	maxIntUsed int
}

// NewRegisterTracker creates a new register tracker
func NewRegisterTracker() *RegisterTracker {
	rt := &RegisterTracker{
		intInUse:    make(map[string]bool),
		intPurpose:  make(map[string]string),
		intReserved: make(map[string]bool),
	}

	// Reserve special-purpose registers
	rt.ReserveInt("rsp") // Stack pointer
	rt.ReserveInt("rbp") // Frame pointer
	rt.ReserveInt("r15") // Environment pointer (for closures)

	// Reserve XMM0 as primary result register
	// (It can be used but must be explicitly requested)

	return rt
}

// Clone creates a copy of the tracker state (for nested scopes)
func (rt *RegisterTracker) Clone() *RegisterTracker {
	clone := &RegisterTracker{
		xmmInUse:    rt.xmmInUse,
		xmmPurpose:  rt.xmmPurpose,
		xmmReserved: rt.xmmReserved,
		intInUse:    make(map[string]bool),
		intPurpose:  make(map[string]string),
		intReserved: make(map[string]bool),
		maxXmmUsed:  rt.maxXmmUsed,
		maxIntUsed:  rt.maxIntUsed,
	}

	for k, v := range rt.intInUse {
		clone.intInUse[k] = v
	}
	for k, v := range rt.intPurpose {
		clone.intPurpose[k] = v
	}
	for k, v := range rt.intReserved {
		clone.intReserved[k] = v
	}

	return clone
}

// ReserveXMM marks an XMM register as reserved (never auto-allocated)
func (rt *RegisterTracker) ReserveXMM(index int) {
	if index >= 0 && index < 16 {
		rt.xmmReserved[index] = true
	}
}

// ReserveInt marks an integer register as reserved
func (rt *RegisterTracker) ReserveInt(reg string) {
	rt.intReserved[reg] = true
}

// AllocXMM allocates an available XMM register
// Returns register name (e.g., "xmm3") or empty string if none available
func (rt *RegisterTracker) AllocXMM(purpose string) string {
	// Try to find an available register
	// Start from xmm2 (xmm0 for results, xmm1 for temps in operations)
	for i := 2; i < 16; i++ {
		if !rt.xmmInUse[i] && !rt.xmmReserved[i] {
			rt.xmmInUse[i] = true
			rt.xmmPurpose[i] = purpose
			rt.xmmStack = append(rt.xmmStack, i)

			if i > rt.maxXmmUsed {
				rt.maxXmmUsed = i
			}

			return fmt.Sprintf("xmm%d", i)
		}
	}

	return "" // No registers available
}

// AllocSpecificXMM allocates a specific XMM register
// Returns true if successful, false if already in use
func (rt *RegisterTracker) AllocSpecificXMM(index int, purpose string) bool {
	if index < 0 || index >= 16 {
		return false
	}

	if rt.xmmInUse[index] || rt.xmmReserved[index] {
		return false
	}

	rt.xmmInUse[index] = true
	rt.xmmPurpose[index] = purpose
	rt.xmmStack = append(rt.xmmStack, index)

	if index > rt.maxXmmUsed {
		rt.maxXmmUsed = index
	}

	return true
}

// FreeXMM frees an XMM register
func (rt *RegisterTracker) FreeXMM(reg string) {
	var index int
	_, err := fmt.Sscanf(reg, "xmm%d", &index)
	if err != nil || index < 0 || index >= 16 {
		return
	}

	rt.xmmInUse[index] = false
	rt.xmmPurpose[index] = ""

	// Remove from stack
	for i := len(rt.xmmStack) - 1; i >= 0; i-- {
		if rt.xmmStack[i] == index {
			rt.xmmStack = append(rt.xmmStack[:i], rt.xmmStack[i+1:]...)
			break
		}
	}
}

// AllocInt allocates an integer register
func (rt *RegisterTracker) AllocInt(purpose string) string {
	// Prefer caller-saved registers for temporaries
	callerSaved := []string{"rax", "rcx", "rdx", "rsi", "rdi", "r8", "r9", "r10", "r11"}

	for _, reg := range callerSaved {
		if !rt.intInUse[reg] && !rt.intReserved[reg] {
			rt.intInUse[reg] = true
			rt.intPurpose[reg] = purpose
			rt.intStack = append(rt.intStack, reg)

			used := len(rt.intInUse)
			if used > rt.maxIntUsed {
				rt.maxIntUsed = used
			}

			return reg
		}
	}

	// Try callee-saved if desperate
	calleeSaved := []string{"rbx", "r12", "r13", "r14"}
	for _, reg := range calleeSaved {
		if !rt.intInUse[reg] && !rt.intReserved[reg] {
			rt.intInUse[reg] = true
			rt.intPurpose[reg] = purpose
			rt.intStack = append(rt.intStack, reg)

			used := len(rt.intInUse)
			if used > rt.maxIntUsed {
				rt.maxIntUsed = used
			}

			return reg
		}
	}

	return "" // No registers available
}

// Confidence that this function is working: 100%
// AllocIntCalleeSaved allocates an available callee-saved integer register
// Used for loop counters that need to survive function calls
// Returns empty string if no callee-saved registers available
func (rt *RegisterTracker) AllocIntCalleeSaved(purpose string) string {
	// Only try callee-saved registers (these survive across operations)
	// Do NOT fall back to caller-saved registers - they get clobbered
	calleeSaved := []string{"r12", "r13", "r14", "rbx"}
	for _, reg := range calleeSaved {
		if !rt.intInUse[reg] && !rt.intReserved[reg] {
			rt.intInUse[reg] = true
			rt.intPurpose[reg] = purpose
			rt.intStack = append(rt.intStack, reg)

			used := len(rt.intInUse)
			if used > rt.maxIntUsed {
				rt.maxIntUsed = used
			}

			return reg
		}
	}

	// No callee-saved registers available - caller must use stack
	return ""
}

// AllocSpecificInt allocates a specific integer register
func (rt *RegisterTracker) AllocSpecificInt(reg string, purpose string) bool {
	if rt.intInUse[reg] || rt.intReserved[reg] {
		return false
	}

	rt.intInUse[reg] = true
	rt.intPurpose[reg] = purpose
	rt.intStack = append(rt.intStack, reg)

	used := len(rt.intInUse)
	if used > rt.maxIntUsed {
		rt.maxIntUsed = used
	}

	return true
}

// FreeInt frees an integer register
func (rt *RegisterTracker) FreeInt(reg string) {
	delete(rt.intInUse, reg)
	delete(rt.intPurpose, reg)

	// Remove from stack
	for i := len(rt.intStack) - 1; i >= 0; i-- {
		if rt.intStack[i] == reg {
			rt.intStack = append(rt.intStack[:i], rt.intStack[i+1:]...)
			break
		}
	}
}

// IsXMMInUse checks if an XMM register is currently in use
func (rt *RegisterTracker) IsXMMInUse(reg string) bool {
	var index int
	_, err := fmt.Sscanf(reg, "xmm%d", &index)
	if err != nil || index < 0 || index >= 16 {
		return false
	}
	return rt.xmmInUse[index]
}

// IsIntInUse checks if an integer register is currently in use
func (rt *RegisterTracker) IsIntInUse(reg string) bool {
	return rt.intInUse[reg]
}

// GetXMMPurpose returns what an XMM register is being used for
func (rt *RegisterTracker) GetXMMPurpose(reg string) string {
	var index int
	_, err := fmt.Sscanf(reg, "xmm%d", &index)
	if err != nil || index < 0 || index >= 16 {
		return ""
	}
	return rt.xmmPurpose[index]
}

// GetIntPurpose returns what an integer register is being used for
func (rt *RegisterTracker) GetIntPurpose(reg string) string {
	return rt.intPurpose[reg]
}

// SaveState returns a snapshot of current allocations
func (rt *RegisterTracker) SaveState() *RegisterTrackerState {
	state := &RegisterTrackerState{
		xmmInUse:   rt.xmmInUse,
		xmmPurpose: rt.xmmPurpose,
		intInUse:   make(map[string]bool),
		intPurpose: make(map[string]string),
	}

	for k, v := range rt.intInUse {
		state.intInUse[k] = v
	}
	for k, v := range rt.intPurpose {
		state.intPurpose[k] = v
	}

	return state
}

// RestoreState restores a previous snapshot
func (rt *RegisterTracker) RestoreState(state *RegisterTrackerState) {
	rt.xmmInUse = state.xmmInUse
	rt.xmmPurpose = state.xmmPurpose
	rt.intInUse = make(map[string]bool)
	rt.intPurpose = make(map[string]string)

	for k, v := range state.intInUse {
		rt.intInUse[k] = v
	}
	for k, v := range state.intPurpose {
		rt.intPurpose[k] = v
	}
}

// Reset clears all allocations (use carefully!)
func (rt *RegisterTracker) Reset() {
	for i := range rt.xmmInUse {
		if !rt.xmmReserved[i] {
			rt.xmmInUse[i] = false
			rt.xmmPurpose[i] = ""
		}
	}

	rt.intInUse = make(map[string]bool)
	rt.intPurpose = make(map[string]string)
	rt.xmmStack = nil
	rt.intStack = nil

	// Restore reserved registers
	for reg := range rt.intReserved {
		rt.intInUse[reg] = true
		rt.intPurpose[reg] = "reserved"
	}
}

// Debug prints current register allocation state
func (rt *RegisterTracker) Debug() {
	fmt.Println("=== Register Tracker State ===")
	fmt.Println("XMM Registers:")
	for i := 0; i < 16; i++ {
		if rt.xmmInUse[i] {
			reserved := ""
			if rt.xmmReserved[i] {
				reserved = " [RESERVED]"
			}
			fmt.Printf("  xmm%d: %s%s\n", i, rt.xmmPurpose[i], reserved)
		}
	}

	fmt.Println("Integer Registers:")
	for reg, inUse := range rt.intInUse {
		if inUse {
			reserved := ""
			if rt.intReserved[reg] {
				reserved = " [RESERVED]"
			}
			fmt.Printf("  %s: %s%s\n", reg, rt.intPurpose[reg], reserved)
		}
	}

	fmt.Printf("Max XMM used: %d, Max Int used: %d\n", rt.maxXmmUsed, rt.maxIntUsed)
	fmt.Println("==============================")
}

// RegisterTrackerState represents a snapshot of register state
type RegisterTrackerState struct {
	xmmInUse   [16]bool
	xmmPurpose [16]string
	intInUse   map[string]bool
	intPurpose map[string]string
}

// SpillStrategy determines how to handle register exhaustion
type SpillStrategy int

const (
	SpillToStack SpillStrategy = iota
	SpillToMemory
	SpillError
)

// RegisterSpiller manages register spilling when all registers are in use
type RegisterSpiller struct {
	strategy      SpillStrategy
	spillSlots    int            // Number of stack slots used for spills
	spillMap      map[string]int // Register -> spill slot mapping
	nextSpillSlot int
}

// NewRegisterSpiller creates a register spiller
func NewRegisterSpiller(strategy SpillStrategy) *RegisterSpiller {
	return &RegisterSpiller{
		strategy:      strategy,
		spillMap:      make(map[string]int),
		nextSpillSlot: 0,
	}
}

// SpillXMM spills an XMM register to stack
// Returns the spill slot offset
func (rs *RegisterSpiller) SpillXMM(reg string) int {
	if slot, exists := rs.spillMap[reg]; exists {
		return slot
	}

	slot := rs.nextSpillSlot
	rs.spillMap[reg] = slot
	rs.nextSpillSlot++
	rs.spillSlots++

	return slot
}

// RestoreXMM gets the spill slot for a register
func (rs *RegisterSpiller) RestoreXMM(reg string) (int, bool) {
	slot, exists := rs.spillMap[reg]
	return slot, exists
}

// GetTotalSpillSpace returns total bytes needed for spills
func (rs *RegisterSpiller) GetTotalSpillSpace() int {
	return rs.spillSlots * 16 // 16 bytes per XMM register
}

// GetAllocatedCalleeSavedRegs returns a list of callee-saved registers currently in use
// Callee-saved registers on x86-64: rbx, r12, r13, r14, r15 (r15 is reserved in Vibe67)
func (rt *RegisterTracker) GetAllocatedCalleeSavedRegs() []string {
	var allocated []string
	calleeSaved := []string{"rbx", "r12", "r13", "r14"}

	for _, reg := range calleeSaved {
		if rt.IsIntInUse(reg) {
			allocated = append(allocated, reg)
		}
	}

	return allocated
}

// GetRegisterPressure returns current register usage statistics
type RegisterPressureStats struct {
	CurrentXmmUsed int
	MaxXmmUsed     int
	TotalXmmRegs   int
	CurrentIntUsed int
	MaxIntUsed     int
	XmmPressure    float64 // 0.0 to 1.0
	IntPressure    float64 // 0.0 to 1.0
	IsSpillHeavy   bool    // True if pressure > 80%
}

func (rt *RegisterTracker) GetRegisterPressure() RegisterPressureStats {
	// Count currently used registers
	currentXmm := 0
	for _, inUse := range rt.xmmInUse {
		if inUse {
			currentXmm++
		}
	}

	currentInt := len(rt.intInUse)

	// Calculate pressure as percentage
	xmmPressure := float64(currentXmm) / 16.0
	intPressure := float64(currentInt) / 13.0 // 16 GPRs - 3 reserved (rsp, rbp, r15)

	stats := RegisterPressureStats{
		CurrentXmmUsed: currentXmm,
		MaxXmmUsed:     rt.maxXmmUsed,
		TotalXmmRegs:   16,
		CurrentIntUsed: currentInt,
		MaxIntUsed:     rt.maxIntUsed,
		XmmPressure:    xmmPressure,
		IntPressure:    intPressure,
		IsSpillHeavy:   xmmPressure > 0.8 || intPressure > 0.8,
	}

	return stats
}

// ReportRegisterPressure prints a summary of register usage (for debugging)
func (rt *RegisterTracker) ReportRegisterPressure(label string) {
	stats := rt.GetRegisterPressure()
	fmt.Printf("=== Register Pressure: %s ===\n", label)
	fmt.Printf("XMM: %d/%d used (%.1f%%), peak: %d\n",
		stats.CurrentXmmUsed, stats.TotalXmmRegs, stats.XmmPressure*100, stats.MaxXmmUsed)
	fmt.Printf("INT: %d/13 used (%.1f%%), peak: %d\n",
		stats.CurrentIntUsed, stats.IntPressure*100, stats.MaxIntUsed)
	if stats.IsSpillHeavy {
		fmt.Println("⚠️  HIGH PRESSURE - Consider register allocation optimization")
	}
	fmt.Println()
}
