// codegen_guards.go - Runtime guards compiled into generated code
package main

import (
	"fmt"
	"os"
)

// GuardConfig controls which runtime guards are enabled
type GuardConfig struct {
	NullPointerChecks    bool // Insert null checks before pointer dereferences
	StackAlignmentChecks bool // Check stack alignment before calls
	BoundsChecks         bool // Check array bounds (when we add arrays)
}

var DefaultGuardConfig = GuardConfig{
	NullPointerChecks:    false, // Disabled for now - too aggressive
	StackAlignmentChecks: false,
	BoundsChecks:         false, // Not implemented yet
}

// EmitNullCheck generates code to check if a register is null and trap if so
// This is compiled INTO the binary for runtime checking
func (fc *C67Compiler) EmitNullCheck(reg, context string) {
	if !DefaultGuardConfig.NullPointerChecks {
		return
	}

	if fc.eb.target.OS() == OSWindows {
		// Windows: just test and continue (no good way to print error)
		return
	}

	// Linux: test register and exit with error message if null
	fc.out.TestRegReg(reg, reg)
	jmpNotNull := fc.eb.text.Len()
	fc.out.JumpConditional(JumpNotEqual, 0) // jne not_null

	// Register is null - print error and exit
	fc.out.MovImmToReg("rdi", "2") // stderr

	// Write error message on stack
	errorMsg := fmt.Sprintf("NULL POINTER: %s in %s\n", reg, context)
	msgLen := len(errorMsg)
	stackSpace := int64(((msgLen + 15) / 16) * 16) // Align to 16 bytes

	fc.out.SubImmFromReg("rsp", stackSpace)
	for i, ch := range errorMsg {
		fc.out.MovImmToMem(int64(ch), "rsp", i)
	}

	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", msgLen))
	fc.out.MovImmToReg("rax", "1") // write syscall
	fc.out.Syscall()

	// Exit with code 1
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovImmToReg("rax", "60") // exit syscall
	fc.out.Syscall()

	// not_null:
	notNullPos := fc.eb.text.Len()
	fc.patchJumpImmediate(jmpNotNull+2, int32(notNullPos-(jmpNotNull+6)))

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Inserted null check for %s in %s\n", reg, context)
	}
}

// EmitStackAlignmentCheck verifies RSP is 16-byte aligned before a call
func (fc *C67Compiler) EmitStackAlignmentCheck(context string) {
	if !DefaultGuardConfig.StackAlignmentChecks {
		return
	}

	if fc.eb.target.OS() == OSWindows {
		return // Skip on Windows for now
	}

	// Test if RSP & 0xF == 0 (16-byte aligned)
	fc.out.MovRegToReg("rax", "rsp")
	fc.out.Emit([]byte{0x48, 0x83, 0xe0, 0x0f}) // and rax, 0xF
	fc.out.TestRegReg("rax", "rax")

	jmpAligned := fc.eb.text.Len()
	fc.out.JumpConditional(JumpEqual, 0) // je aligned

	// Not aligned - print error
	fc.out.MovImmToReg("rdi", "2")

	errorMsg := fmt.Sprintf("MISALIGNED STACK in %s\n", context)
	msgLen := len(errorMsg)
	stackSpace := int64(((msgLen + 15) / 16) * 16)

	fc.out.SubImmFromReg("rsp", stackSpace)
	for i, ch := range errorMsg {
		fc.out.MovImmToMem(int64(ch), "rsp", i)
	}

	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", msgLen))
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()

	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovImmToReg("rax", "60")
	fc.out.Syscall()

	// aligned:
	alignedPos := fc.eb.text.Len()
	fc.patchJumpImmediate(jmpAligned+2, int32(alignedPos-(jmpAligned+6)))
}

// EmitDebugPrintReg prints a register value for debugging (only on Linux)
func (fc *C67Compiler) EmitDebugPrintReg(reg, label string) {
	if fc.eb.target.OS() == OSWindows {
		return
	}

	// Save registers we'll clobber
	fc.out.PushReg("rax")
	fc.out.PushReg("rdi")
	fc.out.PushReg("rsi")
	fc.out.PushReg("rdx")
	fc.out.PushReg("r10")

	// Print label
	fc.out.MovImmToReg("rdi", "2")
	msgLen := len(label)
	stackSpace := int64(((msgLen + 15) / 16) * 16)

	fc.out.SubImmFromReg("rsp", stackSpace)
	for i, ch := range label {
		fc.out.MovImmToMem(int64(ch), "rsp", i)
	}

	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", msgLen))
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", stackSpace)

	// Print register value as hex
	// For now, just print a marker - full hex formatting is complex
	fc.out.SubImmFromReg("rsp", 16)
	fc.out.MovImmToMem(int64('='), "rsp", 0)
	fc.out.MovImmToMem(int64('0'), "rsp", 1)
	fc.out.MovImmToMem(int64('x'), "rsp", 2)
	fc.out.MovImmToMem(int64('\n'), "rsp", 3)
	fc.out.MovImmToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "4")
	fc.out.MovImmToReg("rdi", "2")
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", 16)

	// Restore registers
	fc.out.PopReg("r10")
	fc.out.PopReg("rdx")
	fc.out.PopReg("rsi")
	fc.out.PopReg("rdi")
	fc.out.PopReg("rax")
}
