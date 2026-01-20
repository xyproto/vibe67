// Completion: 100% - Module complete
package main

// grow.go - Helper functions for dynamic arena growth

// generateMetaArenaGrowth generates code to grow the meta-arena capacity
// This doubles the meta-arena pointer array and initializes new arena structures
// Input registers:
//
//	r12 = required_depth
//	r13 = old_capacity
//
// Output:
//
//	r14 = new_capacity
//	r15 = new meta-arena pointer
//	r13 = updated to old len for initialization loop
func (fc *C67Compiler) generateMetaArenaGrowth() {
	// Calculate new capacity: max(capacity * 2, required_depth)
	fc.out.MovRegToReg("r14", "r13")
	fc.out.AddRegToReg("r14", "r13") // r14 = capacity * 2
	fc.out.CmpRegToReg("r14", "r12")
	skipMaxJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge skip (r14 >= required)
	fc.out.MovRegToReg("r14", "r12")              // r14 = max(capacity * 2, required)
	skipMaxLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(skipMaxJump+2, int32(skipMaxLabel-(skipMaxJump+6)))

	// r13 = old_capacity, r14 = new_capacity
	// Realloc meta-arena: realloc(old_ptr, new_capacity * 8)
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta")
	fc.out.MovMemToReg("rdi", "rbx", 0) // rdi = old meta-arena pointer
	fc.out.MovRegToReg("rsi", "r14")
	fc.out.ShlImmReg("rsi", 3) // rsi = new_capacity * 8
	fc.callFunction("realloc", "")

	// Check if realloc failed
	fc.out.TestRegReg("rax", "rax")
	reallocFailedJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je to error

	// Update meta-arena pointer
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta")
	fc.out.MovRegToMem("rax", "rbx", 0)
	fc.out.MovRegToReg("r15", "rax") // r15 = new meta-arena pointer

	// Load current len (number of initialized arenas) into r13
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta_len")
	fc.out.MovMemToReg("r13", "rbx", 0) // r13 = current len

	// Return the error jump position so caller can patch it
	fc.metaArenaGrowthErrorJump = reallocFailedJump
}

// generateArenaInitLoop generates code to initialize new arena structures
// Input registers:
//
//	r12 = required_depth (stop when we reach this)
//	r13 = start_index (current len)
//	r15 = meta-arena pointer
//
// Output:
//
//	r13 = updated len after initialization
func (fc *C67Compiler) generateArenaInitLoop() {
	// Initialize new slots (from len to required_depth)
	// r13 = len, r12 = required_depth, r15 = meta-arena pointer
	initLoopLabel := fc.eb.text.Len()
	fc.out.CmpRegToReg("r13", "r12")
	initDoneJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge done (r13 >= required)

	// Create arena with 4096 bytes capacity
	fc.out.PushReg("rdi") // Save registers across call
	fc.out.PushReg("rsi")
	if fc.eb.target.OS() == OSWindows {
		fc.out.MovImmToReg("rcx", "4096") // Windows: first arg in rcx
	} else {
		fc.out.MovImmToReg("rdi", "4096") // Linux: first arg in rdi
	}
	fc.trackFunctionCall("malloc") // Track for PLT
	fc.out.CallSymbol("_vibe67_arena_create")
	fc.out.PopReg("rsi")
	fc.out.PopReg("rdi")

	// Store arena pointer in meta-arena[r13]
	fc.out.MovRegToReg("rbx", "r13")
	fc.out.ShlImmReg("rbx", 3)          // rbx = r13 * 8
	fc.out.AddRegToReg("rbx", "r15")    // rbx = r15 + (r13 * 8)
	fc.out.MovRegToMem("rax", "rbx", 0) // Store at [rbx]

	// Increment counter
	fc.out.AddImmToReg("r13", 1)
	jumpOffset := int32(initLoopLabel - (fc.eb.text.Len() + 2))
	fc.out.Emit([]byte{0xeb, byte(jumpOffset)}) // jmp rel8

	// Done initializing
	initDoneLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(initDoneJump+2, int32(initDoneLabel-(initDoneJump+6)))

	// Update len (r13 now contains the new len)
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta_len")
	fc.out.MovRegToMem("r13", "rbx", 0)
}

// generateFirstMetaArenaAlloc generates code for the first meta-arena allocation
// Allocates 8 slots and initializes arena structures up to min(8, required_depth)
// Input registers:
//
//	r12 = required_depth
//
// Output:
//
//	r13 = number of arenas created (min(8, required))
//	r15 = meta-arena pointer
func (fc *C67Compiler) generateFirstMetaArenaAlloc() {
	// Allocate meta-arena with 8 slots initially
	fc.out.MovImmToReg("rdi", "64") // 8 slots * 8 bytes = 64
	fc.callFunction("malloc", "")

	// Check if malloc failed
	fc.out.TestRegReg("rax", "rax")
	firstMallocFailedJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je to error

	// Store meta-arena pointer
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta")
	fc.out.MovRegToMem("rax", "rbx", 0)
	fc.out.MovRegToReg("r15", "rax") // r15 = meta-arena pointer

	// Initialize arenas up to min(8, required_depth)
	// r12 = required_depth, r15 = meta-arena base pointer
	fc.out.XorRegWithReg("r13", "r13") // r13 = 0 (counter)
	firstInitLoopLabel := fc.eb.text.Len()
	fc.out.CmpRegToReg("r13", "r12") // Compare with required_depth
	firstInitDoneJump1 := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge done (r13 >= required)
	fc.out.CmpRegToImm("r13", 8)                  // Also check if we've done 8
	firstInitDoneJump2 := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge done (r13 >= 8)

	// Create arena with 4096 bytes capacity
	fc.out.PushReg("rdi")
	fc.out.PushReg("rsi")
	if fc.eb.target.OS() == OSWindows {
		fc.out.MovImmToReg("rcx", "4096") // Windows: first arg in rcx
	} else {
		fc.out.MovImmToReg("rdi", "4096") // Linux: first arg in rdi
	}
	fc.trackFunctionCall("malloc") // Will become _vibe67_arena_create
	fc.out.CallSymbol("_vibe67_arena_create")
	fc.out.PopReg("rsi")
	fc.out.PopReg("rdi")

	// Store in meta-arena[r13] - calculate offset without modifying r15
	fc.out.MovRegToReg("rbx", "r13")
	fc.out.ShlImmReg("rbx", 3)          // rbx = r13 * 8
	fc.out.AddRegToReg("rbx", "r15")    // rbx = r15 + offset
	fc.out.MovRegToMem("rax", "rbx", 0) // Store at [rbx]

	// Increment counter
	fc.out.AddImmToReg("r13", 1)
	jumpOffset2 := int32(firstInitLoopLabel - (fc.eb.text.Len() + 2))
	fc.out.Emit([]byte{0xeb, byte(jumpOffset2)}) // jmp rel8

	// Done with first allocation
	firstInitDoneLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(firstInitDoneJump1+2, int32(firstInitDoneLabel-(firstInitDoneJump1+6)))
	fc.patchJumpImmediate(firstInitDoneJump2+2, int32(firstInitDoneLabel-(firstInitDoneJump2+6)))

	// Set len to r13 (number of arenas we just created: min(8, required_depth))
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta_len")
	fc.out.MovRegToMem("r13", "rbx", 0)

	// Set capacity to 8
	fc.out.LeaSymbolToReg("rbx", "_vibe67_arena_meta_cap")
	fc.out.MovImmToReg("rax", "8")
	fc.out.MovRegToMem("rax", "rbx", 0)

	// Store malloc error jump for caller to patch
	fc.firstMetaArenaMallocErrorJump = firstMallocFailedJump
}

// generateIndividualArenaGrowth generates code to grow a single arena's buffer
// This is used within vibe67_arena_alloc when an arena runs out of space
// Input registers:
//
//	rdi = arena_ptr
//	rsi = needed_size (aligned)
//	r13 = current offset
//
// Modifies:
//
//	r8, r9, rdx, rax
//
// Calls realloc and updates arena structure
func (fc *C67Compiler) generateIndividualArenaGrowth(arenaGrowJump int) int {
	arenaGrowLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(arenaGrowJump+2, int32(arenaGrowLabel-(arenaGrowJump+6)))
	fc.eb.MarkLabel("_arena_alloc_grow")

	// Calculate new capacity: max(capacity * 2, aligned_offset + size)
	// r9 = capacity, rdx = aligned_offset + aligned_size
	fc.out.MovRegToReg("rdi", "r9")
	fc.out.AddRegToReg("rdi", "r9")  // rdi = capacity * 2
	fc.out.CmpRegToReg("rdi", "rdx") // compare with needed
	skipMaxJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0) // jge skip_max
	fc.out.MovRegToReg("rdi", "rdx")              // rdi = max(capacity * 2, needed)
	skipMaxLabel := fc.eb.text.Len()
	fc.patchJumpImmediate(skipMaxJump+2, int32(skipMaxLabel-(skipMaxJump+6)))

	// rdi = new_capacity, r8 = old buffer_ptr
	fc.out.MovRegToReg("rsi", "rdi") // rsi = new_capacity
	fc.out.MovRegToReg("rdi", "r8")  // rdi = old buffer_ptr
	fc.callFunction("realloc", "")

	// Check if realloc failed
	fc.out.TestRegReg("rax", "rax")
	arenaErrorJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je to error (realloc failed - rax==0)

	// Update arena structure with new buffer and capacity
	// [rax+0] = buffer_ptr, [rax+8] = capacity
	fc.out.MovMemToReg("rbx", "rbp", 16) // Load original arena_ptr
	fc.out.MovRegToMem("rax", "rbx", 0)  // Update buffer_ptr
	fc.out.MovRegToMem("rsi", "rbx", 8)  // Update capacity

	return arenaErrorJump
}









