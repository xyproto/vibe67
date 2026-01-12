// Completion: 100% - Syscall-based print implementation for Linux/Unix
package main

import (
	"fmt"
	"os"
)

// generatePrintSyscall generates a syscall-based print function
// This function writes directly to stdout (fd=1) using the write syscall
// Signature: _vibe67_print_syscall(str_ptr) -> void
func (fc *Vibe67Compiler) generatePrintSyscall() {
	if fc.eb.target.OS() != OSLinux {
		return
	}

	fc.eb.MarkLabel("_vibe67_print_syscall")

	// Prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")

	// rdi = string pointer (map: [count][0][char0][1][char1]...)
	fc.out.MovRegToReg("rbx", "rdi")

	// Get string length
	fc.out.MovMemToXmm("xmm0", "rbx", 0)
	fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

	// Allocate 1-byte buffer on stack
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToReg("r13", "rsp") // r13 = buffer address

	// Loop through characters (r14 = index, avoids rcx which syscall clobbers)
	fc.out.XorRegWithReg("r14", "r14")

	loopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("r14", "r12")
	loopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)
	loopEndPos := fc.eb.text.Len()

	// Calculate character offset: 16 + index * 16
	fc.out.MovRegToReg("rax", "r14")
	fc.out.ShlImmReg("rax", 4)       // rax = index * 16
	fc.out.AddImmToReg("rax", 16)    // rax = 16 + index * 16
	fc.out.AddRegToReg("rax", "rbx") // rax = string_ptr + offset

	// Load character code
	fc.out.MovMemToXmm("xmm0", "rax", 0)
	fc.out.Cvttsd2si("rdi", "xmm0")
	fc.out.MovRegToMem("rdi", "r13", 0)

	// write(1, buffer, 1)
	fc.out.MovImmToReg("rax", "1")   // syscall: write
	fc.out.MovImmToReg("rdi", "1")   // fd: stdout
	fc.out.MovRegToReg("rsi", "r13") // buffer
	fc.out.MovImmToReg("rdx", "1")   // length: 1
	fc.out.Syscall()

	// Increment index and loop
	fc.out.IncReg("r14")
	backOffset := int32(loopStart - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(backOffset)

	// Patch loop end jump
	donePos := fc.eb.text.Len()
	fc.patchJumpImmediate(loopEndJump+2, int32(donePos-loopEndPos))

	// Epilogue
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Generated _vibe67_print_syscall\n")
	}
}

// generatePrintlnSyscall generates a syscall-based println function
// This function writes to stdout followed by a newline
// Signature: _vibe67_println_syscall(str_ptr) -> void
func (fc *Vibe67Compiler) generatePrintlnSyscall() {
	if fc.eb.target.OS() != OSLinux {
		return
	}

	fc.eb.MarkLabel("_vibe67_println_syscall")

	// Prologue
	fc.out.PushReg("rbp")
	fc.out.MovRegToReg("rbp", "rsp")
	fc.out.PushReg("rbx")
	fc.out.PushReg("r12")
	fc.out.PushReg("r13")
	fc.out.PushReg("r14")

	// rdi = string pointer
	fc.out.MovRegToReg("rbx", "rdi")

	// Get string length
	fc.out.MovMemToXmm("xmm0", "rbx", 0)
	fc.out.Cvttsd2si("r12", "xmm0") // r12 = length

	// Allocate 1-byte buffer on stack
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovRegToReg("r13", "rsp")

	// Loop through characters
	fc.out.XorRegWithReg("r14", "r14")

	loopStart := fc.eb.text.Len()
	fc.out.CmpRegToReg("r14", "r12")
	loopEndJump := fc.eb.text.Len()
	fc.out.JumpConditional(JumpGreaterOrEqual, 0)
	loopEndPos := fc.eb.text.Len()

	// Calculate character offset
	fc.out.MovRegToReg("rax", "r14")
	fc.out.ShlImmReg("rax", 4)
	fc.out.AddImmToReg("rax", 16)
	fc.out.AddRegToReg("rax", "rbx")

	// Load character
	fc.out.MovMemToXmm("xmm0", "rax", 0)
	fc.out.Cvttsd2si("rdi", "xmm0")
	fc.out.MovRegToMem("rdi", "r13", 0)

	// write(1, buffer, 1)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "r13")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()

	// Increment and loop
	fc.out.IncReg("r14")
	backOffset := int32(loopStart - (fc.eb.text.Len() + 5))
	fc.out.JumpUnconditional(backOffset)

	// Patch loop end
	donePos := fc.eb.text.Len()
	fc.patchJumpImmediate(loopEndJump+2, int32(donePos-loopEndPos))

	// Write newline
	fc.out.MovImmToReg("rax", "10") // '\n'
	fc.out.MovRegToMem("rax", "r13", 0)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "1")
	fc.out.MovRegToReg("rsi", "r13")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()

	// Epilogue
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("r14")
	fc.out.PopReg("r13")
	fc.out.PopReg("r12")
	fc.out.PopReg("rbx")
	fc.out.PopReg("rbp")
	fc.out.Ret()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Generated _vibe67_println_syscall\n")
	}
}

// generatePrintfSyscall generates printf using print + f-string infrastructure
// This leverages the existing f-string compilation which calls _vibe67_concat_strings
// Signature: printf(fstring_ptr) -> void
func (fc *Vibe67Compiler) generatePrintfSyscall() {
	// printf is just print with f-string support
	// The f-string compilation handles formatting, then we print the result
	// This is already handled in compileBuiltinCall for "printf"
	// We don't need a separate runtime function since printf calls are inlined

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: printf uses inline f-string + print\n")
	}
}
