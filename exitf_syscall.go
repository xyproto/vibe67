// exitf syscall implementation for Linux
package main

import (
	"fmt"
)

// compileExitfSyscall compiles exitf() using syscalls to stderr (fd=2)
// Simplified version that handles the common case: exitf("Error: %s\n", c_string)
func (fc *Vibe67Compiler) compileExitfSyscall(call *CallExpr, formatStr *StringExpr) {
	processedFormat := processEscapeSequences(formatStr.Value)

	argIndex := 0
	i := 0
	runes := []rune(processedFormat)

	for i < len(runes) {
		if runes[i] == '%' && i+1 < len(runes) {
			next := runes[i+1]

			if next == '%' {
				fc.emitStderrChar('%')
				i += 2
				continue
			}

			if next == 's' {
				// String argument - assume it's a C string (char*)
				if argIndex+1 < len(call.Args) {
					arg := call.Args[argIndex+1]
					fc.compileExpression(arg)
					// xmm0 contains pointer as float64, convert to rax
					fc.out.MovqXmmToReg("rax", "xmm0")
					fc.emitStderrCString()
				}
				argIndex++
				i += 2
			} else if next == 'd' || next == 'i' {
				// Integer argument
				if argIndex+1 < len(call.Args) {
					arg := call.Args[argIndex+1]
					fc.compileExpression(arg)
					fc.out.Cvttsd2si("rax", "xmm0")
					fc.emitStderrInteger()
				}
				argIndex++
				i += 2
			} else {
				// Unsupported format, just skip
				i += 2
			}
		} else {
			// Collect literal text
			start := i
			for i < len(runes) && !(runes[i] == '%' && i+1 < len(runes)) {
				i++
			}
			segment := string(runes[start:i])
			if len(segment) > 0 {
				fc.emitStderrLiteral(segment)
			}
		}
	}
}

// emitStderrLiteral writes a string literal to stderr
func (fc *Vibe67Compiler) emitStderrLiteral(str string) {
	labelName := fmt.Sprintf("_exitf_lit_%d", fc.stringCounter)
	fc.stringCounter++
	fc.eb.Define(labelName, str)

	fc.out.MovImmToReg("rax", "1") // sys_write
	fc.out.MovImmToReg("rdi", "2") // stderr
	fc.out.LeaSymbolToReg("rsi", labelName)
	fc.out.MovImmToReg("rdx", fmt.Sprintf("%d", len(str)))
	fc.out.Syscall()
}

// emitStderrChar writes a single character to stderr
func (fc *Vibe67Compiler) emitStderrChar(ch rune) {
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovImmToReg("rax", fmt.Sprintf("%d", ch))
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "2") // stderr
	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", 8)
}

// emitStderrCString writes a null-terminated C string to stderr
// Input: rax = pointer to C string
func (fc *Vibe67Compiler) emitStderrCString() {
	fc.out.PushReg("rbx")
	fc.out.MovRegToReg("rbx", "rax")
	fc.out.XorRegWithReg("rdx", "rdx")

	// Calculate strlen
	strlenStart := fc.eb.text.Len()
	fc.out.Emit([]byte{0x80, 0x3c, 0x13, 0x00}) // cmp byte [rbx+rdx], 0
	strlenDoneJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x74, 0x00}) // je
	fc.out.IncReg("rdx")
	strlenBackJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0xeb, 0x00}) // jmp back
	fc.patchShortJump(strlenBackJump+1, strlenStart)

	strlenDone := fc.eb.text.Len()
	fc.patchShortJump(strlenDoneJump+1, strlenDone)

	// Write using syscall
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.MovImmToReg("rdi", "2") // stderr
	fc.out.MovImmToReg("rax", "1")
	fc.out.Syscall()

	fc.out.PopReg("rbx")
}

// emitStderrInteger writes an integer to stderr
// Input: rax = integer value
func (fc *Vibe67Compiler) emitStderrInteger() {
	fc.out.PushReg("rbx")
	fc.out.PushReg("rcx")
	fc.out.PushReg("rdx")

	// Check if negative
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	positiveJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x79, 0x00}) // jns

	// Negative: print minus, negate
	fc.out.PushReg("rax")
	fc.out.SubImmFromReg("rsp", 8)
	fc.out.MovImmToReg("rax", "45") // '-'
	fc.out.MovRegToMem("rax", "rsp", 0)
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "2") // stderr
	fc.out.MovRegToReg("rsi", "rsp")
	fc.out.MovImmToReg("rdx", "1")
	fc.out.Syscall()
	fc.out.AddImmToReg("rsp", 8)
	fc.out.PopReg("rax")
	fc.out.NegReg("rax")

	// Patch positive jump
	positiveStart := fc.eb.text.Len()
	fc.eb.text.Bytes()[positiveJump+1] = byte(positiveStart - (positiveJump + 2))

	// Convert to string
	fc.out.SubImmFromReg("rsp", 32)
	fc.out.LeaMemToReg("rbx", "rsp", 31)
	fc.out.Emit([]byte{0xc6, 0x03, 0x00}) // mov byte [rbx], 0
	fc.out.MovImmToReg("rcx", "10")

	convertStart := fc.eb.text.Len()
	fc.out.XorRegWithReg("rdx", "rdx")
	fc.out.Emit([]byte{0x48, 0xf7, 0xf1}) // div rcx
	fc.out.AddImmToReg("rdx", 48)
	fc.out.DecReg("rbx")
	fc.out.Emit([]byte{0x88, 0x13})       // mov [rbx], dl
	fc.out.Emit([]byte{0x48, 0x85, 0xc0}) // test rax, rax
	convertJump := fc.eb.text.Len()
	fc.out.Emit([]byte{0x75, 0x00}) // jnz
	fc.eb.text.Bytes()[convertJump+1] = byte(convertStart - (convertJump + 2))

	// Calculate length
	fc.out.LeaMemToReg("rdx", "rsp", 32)
	fc.out.SubRegFromReg("rdx", "rbx")

	// Write
	fc.out.MovImmToReg("rax", "1")
	fc.out.MovImmToReg("rdi", "2") // stderr
	fc.out.MovRegToReg("rsi", "rbx")
	fc.out.Syscall()

	fc.out.AddImmToReg("rsp", 32)
	fc.out.PopReg("rdx")
	fc.out.PopReg("rcx")
	fc.out.PopReg("rbx")
}
