// Printf runtime implementation for Vibe67
//
// STATUS: The functions in this file are stubs for a syscall-based printf implementation.
// Currently, printf is implemented inline in codegen.go by calling libc's printf function.
// This works correctly but depends on libc and generates PLT entry warnings.
//
// FUTURE: Implement a pure syscall-based printf similar to asm/printf.asm which:
//   - Parses format strings at runtime
//   - Converts arguments to strings using inline assembly
//   - Writes output using write(1, buf, len) syscall
//   - Eliminates dependency on libc's printf
//
// The assembly reference implementation in asm/printf.asm demonstrates the correct approach.
package main

import (
	"fmt"
	"os"
)

// PrintfRuntime generates the printf runtime function for all architectures.
// NOTE: These functions are currently stubs and not used by the compiler.
// See codegen.go case "printf" for the actual implementation.
//
// Planned function signature for syscall-based implementation:
//   _vibe67_printf(format_str_ptr, arg1, arg2, arg3, ...)
//
// Calling convention (x86-64 System V ABI):
//   rdi = format string pointer (C-style null-terminated)
//   rsi,rdx,rcx,r8,r9 = integer arguments
//   xmm0-xmm7 = float arguments
//   Preserves: rbx, rbp, r12-r15
//   Clobbers: rax, rcx, rdx, rsi, rdi, r8-r11, xmm0-xmm15
//
// Approach (from asm/printf.asm):
//   1. Parse format string character by character
//   2. For '%' specifiers, convert corresponding argument to string
//   3. Write output using syscall write(1, buf, len)
//   4. Return total bytes written

type PrintfBackend interface {
	// EmitPrintfRuntime generates the complete printf function
	EmitPrintfRuntime() error

	// Helper methods for each architecture to implement
	emitPrintfPrologue()
	emitParseFormatString()
	emitConvertArg(specifier rune)
	emitWriteOutput()
	emitPrintfEpilogue()
}

// X86_64PrintfBackend implements printf for x86-64
type X86_64PrintfBackend struct {
	eb  *ExecutableBuilder
	out *Out
}

// ARM64PrintfBackend implements printf for ARM64
type ARM64PrintfBackend struct {
	eb  *ExecutableBuilder
	out *Out
}

// RISCV64PrintfBackend implements printf for RISC-V 64
type RISCV64PrintfBackend struct {
	eb  *ExecutableBuilder
	out *Out
}

// EmitPrintfRuntimeX86_64 generates the printf function for x86-64
func EmitPrintfRuntimeX86_64(eb *ExecutableBuilder, out *Out) error {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Emitting _vibe67_printf for x86-64\n")
	}

	eb.MarkLabel("_vibe67_printf")

	// Prologue: set up stack frame and preserve callee-saved registers
	// We need significant stack space for:
	// - Converted argument strings (256 bytes per arg, max 8 args = 2048 bytes)
	// - Format string parsing state (64 bytes)
	// - Saved registers (64 bytes)
	// Total: ~2200 bytes, round to 2304 (divisible by 16)

	out.PushReg("rbp")
	out.MovRegToReg("rbp", "rsp")

	// Preserve callee-saved registers
	out.PushReg("rbx")
	out.PushReg("r12")
	out.PushReg("r13")
	out.PushReg("r14")
	out.PushReg("r15")

	// Allocate stack space (2304 bytes)
	out.SubImmFromReg("rsp", 2304)

	// Save float arguments to stack (xmm0-xmm7)
	// We need these accessible for conversion
	for i := 0; i < 8; i++ {
		offset := i * 8
		out.MovXmmToMem(fmt.Sprintf("xmm%d", i), "rsp", offset)
	}

	// rdi = format string pointer (Vibe67 map)
	// Convert map pointer to actual character data
	// Map format: [count:f64][key0:f64][val0:f64]...
	// For strings, keys are indices, values are character codes

	// Save format string pointer
	out.MovRegToReg("r12", "rdi") // r12 = format string map

	// Get string length from map
	out.MovMemToXmm("xmm0", "r12", 0) // Load count
	out.Cvttsd2si("r13", "xmm0")      // r13 = string length

	// r14 = current argument index (0-7)
	out.XorRegWithReg("r14", "r14")

	// r15 = output buffer position
	out.LeaMemToReg("r15", "rsp", 64) // Start after saved xmm regs

	// Simple implementation: iterate through format string
	// For now, just handle %v (value) and %s (string)
	// More complex formatting can be added later

	// Loop through format string
	out.XorRegWithReg("rbx", "rbx") // rbx = format string index

	eb.MarkLabel("_printf_loop")

	// Check if we've processed all characters
	out.CmpRegToReg("rbx", "r13")
	// Jump to output if done
	printfDoneLabel := "_printf_done"
	out.Write(0x0F) // JGE (jump if greater or equal)
	out.Write(0x8D)
	// Placeholder for offset (will be patched)
	doneOffset := int32(0) // TODO: Calculate proper offset
	out.Write(byte(doneOffset & 0xFF))
	out.Write(byte((doneOffset >> 8) & 0xFF))
	out.Write(byte((doneOffset >> 16) & 0xFF))
	out.Write(byte((doneOffset >> 24) & 0xFF))

	// Load current character from format string
	// Character is at: map_base + 8 + (index * 16) + 8 (value offset)
	out.MovRegToReg("rax", "rbx")
	out.ShlImmReg("rax", 4)       // rax = index * 16
	out.AddImmToReg("rax", 16)    // Skip count + key
	out.AddRegToReg("rax", "r12") // Add base
	out.MovMemToXmm("xmm0", "rax", 0)
	out.Cvttsd2si("rcx", "xmm0") // rcx = character code

	// Check if character is '%'
	out.CmpRegToImm("rcx", 37) // ASCII '%'
	notPercentLabel := "_printf_not_percent"
	out.Write(0x75) // JNE (jump if not equal)
	out.Write(0x20) // Placeholder offset

	// Handle '%' - check next character for format specifier
	out.AddImmToReg("rbx", 1) // Move to next character

	// Check if next char is 'v' (value format)
	out.MovRegToReg("rax", "rbx")
	out.ShlImmReg("rax", 4)
	out.AddImmToReg("rax", 16)
	out.AddRegToReg("rax", "r12")
	out.MovMemToXmm("xmm0", "rax", 0)
	out.Cvttsd2si("rcx", "xmm0")

	out.CmpRegToImm("rcx", 118) // ASCII 'v'
	notValueLabel := "_printf_not_v"
	out.Write(0x75) // JNE
	out.Write(0x30) // Placeholder

	// Convert current argument (in xmm[r14]) to string
	// Load argument from saved xmm registers on stack
	out.MovRegToReg("rax", "r14")
	out.ShlImmReg("rax", 3) // * 8
	out.AddRegToReg("rax", "rsp")
	out.MovMemToXmm("xmm0", "rax", 0)

	// Call float-to-string converter (simplified inline version)
	// For MVP, just write a placeholder or delegate to existing code
	// TODO: Implement proper float-to-string conversion

	// For now, write literal " [VAL] " as placeholder
	out.MovImmToReg("rax", "0x205D4C41565B2000") // " [VAL] "
	out.MovRegToMem("rax", "r15", 0)
	out.AddImmToReg("r15", 7)

	// Increment argument index
	out.AddImmToReg("r14", 1)

	// Move to next format character
	out.AddImmToReg("rbx", 1)

	// Jump back to loop
	out.Write(0xEB) // JMP
	out.Write(0x80) // Placeholder (jump back)

	eb.MarkLabel(notValueLabel)
	eb.MarkLabel(notPercentLabel)

	// Not a format specifier, just copy character to output
	out.MovByteRegToMem("rcx", "r15", 0)
	out.AddImmToReg("r15", 1)
	out.AddImmToReg("rbx", 1)

	// Jump back to loop
	out.Write(0xEB)
	out.Write(0x90) // Placeholder

	eb.MarkLabel(printfDoneLabel)

	// Calculate output length: r15 - (rsp + 64)
	out.LeaMemToReg("rax", "rsp", 64)
	out.MovRegToReg("rdx", "r15")
	out.SubRegFromReg("rdx", "rax")

	// Write output using syscall write(1, buf, len)
	out.MovImmToReg("rax", "1")       // sys_write
	out.MovImmToReg("rdi", "1")       // fd = stdout
	out.LeaMemToReg("rsi", "rsp", 64) // buf
	// rdx already has length
	out.Syscall()

	// Epilogue: restore registers and return
	out.AddImmToReg("rsp", 2304)
	out.PopReg("r15")
	out.PopReg("r14")
	out.PopReg("r13")
	out.PopReg("r12")
	out.PopReg("rbx")
	out.PopReg("rbp")
	out.Ret()

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: _vibe67_printf emitted successfully\n")
	}

	return nil
}

// EmitPrintfRuntimeARM64 generates the printf function for ARM64
func EmitPrintfRuntimeARM64(eb *ExecutableBuilder, out *Out) error {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Emitting _vibe67_printf for ARM64\n")
	}

	// ARM64 printf implementation
	// Uses ARM64 calling convention:
	//   x0 = format string pointer
	//   d0-d7 = float arguments
	//   Preserves: x19-x28, sp

	// TODO: Implement ARM64-specific assembly
	// For now, emit a stub that returns 0

	eb.MarkLabel("_vibe67_printf")

	// Stub: just return 0
	// MOV x0, #0
	out.Write(0x00)
	out.Write(0x00)
	out.Write(0x80)
	out.Write(0xD2)

	// RET
	out.Write(0xC0)
	out.Write(0x03)
	out.Write(0x5F)
	out.Write(0xD6)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: ARM64 _vibe67_printf stub emitted\n")
	}

	return nil
}

// EmitPrintfRuntimeRISCV64 generates the printf function for RISC-V 64
func EmitPrintfRuntimeRISCV64(eb *ExecutableBuilder, out *Out) error {
	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: Emitting _vibe67_printf for RISC-V 64\n")
	}

	// RISC-V printf implementation
	// Uses RISC-V calling convention:
	//   a0 = format string pointer
	//   fa0-fa7 = float arguments
	//   Preserves: s0-s11, sp

	// TODO: Implement RISC-V-specific assembly
	// For now, emit a stub that returns 0

	eb.MarkLabel("_vibe67_printf")

	// Stub: just return 0
	// li a0, 0
	out.Write(0x13) // addi a0, x0, 0
	out.Write(0x05)
	out.Write(0x00)
	out.Write(0x00)

	// ret
	out.Write(0x67) // jalr x0, 0(ra)
	out.Write(0x80)
	out.Write(0x00)
	out.Write(0x00)

	if VerboseMode {
		fmt.Fprintf(os.Stderr, "DEBUG: RISC-V _vibe67_printf stub emitted\n")
	}

	return nil
}

// EmitPrintfRuntime is the main entry point that detects architecture
// and calls the appropriate backend
func EmitPrintfRuntime(eb *ExecutableBuilder, out *Out, arch string) error {
	switch arch {
	case "x86-64", "amd64", "x86_64":
		return EmitPrintfRuntimeX86_64(eb, out)
	case "arm64", "aarch64":
		return EmitPrintfRuntimeARM64(eb, out)
	case "riscv64", "riscv":
		return EmitPrintfRuntimeRISCV64(eb, out)
	default:
		return fmt.Errorf("unsupported architecture for printf: %s", arch)
	}
}
